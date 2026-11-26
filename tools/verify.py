import concurrent.futures
import hashlib
import io
import json
import math
import os
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
import zipfile
import zlib


class Service:
    def __init__(self, binary, data):
        self.binary, self.data = binary, data

    def __enter__(self):
        self.proc = subprocess.Popen([self.binary, '-port', '0', '-data', self.data], stderr=subprocess.PIPE, text=True)
        line = self.proc.stderr.readline().strip()
        try:
            self.url = 'http://' + line.split('listening ')[1]
        except IndexError:
            self.__exit__()
            raise RuntimeError(line)
        return self

    def __exit__(self, *exc):
        if self.proc.poll() is None:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=3)
            except subprocess.TimeoutExpired:
                self.proc.kill()
                self.proc.wait()
        self.proc.stderr.close()

    def call(self, path, data=None, headers=None):
        request = urllib.request.Request(self.url + path, data=data, headers=headers or {})
        try:
            with urllib.request.urlopen(request, timeout=15) as response:
                return response.status, response.headers, response.read()
        except urllib.error.HTTPError as error:
            with error:
                return error.code, error.headers, error.read()

    def post(self, value):
        return self.call('/v1/build', json.dumps(value).encode(), {'Content-Type': 'application/json'})

    def demo(self):
        status, _, data = self.call('/v1/example')
        assert status == 200
        return json.loads(data)


def receipt(result):
    status, headers, body = result
    assert status in (200, 201), (status, body)
    row = json.loads(body)
    assert headers['Location'] == row['artifact'] == '/v1/artifacts/' + row['digest']
    return row


def fetch(s, row):
    status, headers, blob = s.call(row['artifact'])
    assert status == 200 and len(blob) == row['bytes']
    assert hashlib.sha256(blob).hexdigest() == row['digest']
    assert headers['ETag'] == '"' + row['digest'] + '"'
    return blob


def contract(s):
    assert s.call('/healthz')[0] == 200
    assert s.call('/v1/build', b'{}')[0] == 415
    json_type = {'Content-Type': 'application/json'}
    assert s.call('/v1/build', b'{"unknown": 1}', json_type)[0] == 400
    assert s.call('/v1/build', b'{} {}', json_type)[0] == 400
    assert s.call('/v1/build', b'{"space":"' + b' '*((3<<20)+1) + b'"}', json_type)[0] == 413
    assert s.call('/v1/artifacts/absent')[0] == 404


def pixels(data):
    assert data[:8] == b'\x89PNG\r\n\x1a\n'
    compressed = b''
    offset = 8
    while offset < len(data):
        n, type_ = struct.unpack_from('>I4s', data, offset)
        payload = data[offset+8:offset+8+n]
        assert zlib.crc32(type_+payload) == struct.unpack_from('>I', data, offset+8+n)[0]
        if type_ == b'IHDR':
            w, h, bits, typ, _, _, interlaced = struct.unpack('>IIBBBBB', payload)
            assert bits == 8 and typ in (2, 6) and interlaced == 0
            channels = 3 if typ == 2 else 4
        if type_ == b'IDAT':
            compressed += payload
        offset += n + 12
    raw = zlib.decompress(compressed)
    rows, prev, stride = [], [0] * (channels * w), channels * w
    for y in range(h):
        first = y * (stride + 1)
        method = raw[first]
        line = list(raw[first + 1:first + 1 + stride])
        for i in range(stride):
            a, b, c = line[i - channels] if i >= channels else 0, prev[i], prev[i - channels] if i >= channels else 0
            if method == 4:
                p = a + b - c
                delta = min((abs(p - k), j, k) for j, k in enumerate((a, b, c)))[2]
            else:
                delta = (0, a, b, (a + b) // 2)[method]
            line[i] = (line[i] + delta) & 255
        pixels_in_row = [tuple(line[x:x + channels]) for x in range(0, stride, channels)]
        if channels == 3:
            pixels_in_row = [px + (255,) for px in pixels_in_row]
        rows.append(pixels_in_row)
        prev = line
    return w, h, rows


def check_product(s):
    req = s.demo()
    row = receipt(s.post(req))
    with zipfile.ZipFile(io.BytesIO(fetch(s, row))) as z:
        assert set(z.namelist()) == {'layout.json', 'probes.png'} and z.testzip() is None
        layout = json.loads(z.read('layout.json'))
        w, h, rgba = pixels(z.read('probes.png'))
    assert (w, h) == (18, 4) == (layout['TextureWidth'], layout['TextureHeight'])
    assert layout['Statistics']['Valid'] == 11 and layout['Statistics']['OccludedRays'] > 0
    assert layout['Axes'] == ['+X', '-X', '+Y', '-Y', '+Z', '-Z']
    assert rgba[0][3][0] > rgba[0][3][1] > 0 and rgba[0][3][3] == 255
    assert rgba[0][4] == (0, 0, 0, 0) and rgba[0][5] == (0, 0, 0, 255)
    states = [x['State'] for x in layout['Samples']]
    assert states == ['sampled', 'sampled', 'solid', 'sampled', 'outside'], states
    assert 0 < layout['Samples'][1]['Weight'] < 1
    assert not any(any(axis) for axis in layout['Samples'][3]['Irradiance'])
    assert 'Animation' not in layout and 'Timeline' not in layout
    req['Exposure'] = 6
    assert receipt(s.post(req))['digest'] != row['digest']
    req['Grid']['Size'] = [0, 2, 2]
    assert s.post(req)[0] == 422
    req['Grid']['Size'] = [3, 2, 2]
    req['Blocks'][0]['Min'] = req['Blocks'][0]['Max']
    assert s.post(req)[0] == 422


# ---------------------------------------------------------------------------
# Independent offline engine. It consumes only the downloaded artifact plus a
# caller-chosen shutter/sample: no light list and no re-baking are required.
# Geometry predicates are derived from scratch via slab fraction intervals.
# ---------------------------------------------------------------------------

EPS = 1e-9


def sub(a, b):
    return [a[i] - b[i] for i in range(3)]


def box_at(mover, t):
    keys = mover['Keys']
    size = mover['Size']
    if t <= keys[0]['Time']:
        lo = keys[0]['Position']
    elif t >= keys[-1]['Time']:
        lo = keys[-1]['Position']
    else:
        for k in range(len(keys) - 1):
            a, b = keys[k], keys[k + 1]
            if a['Time'] <= t <= b['Time']:
                f = (t - a['Time']) / (b['Time'] - a['Time'])
                lo = [a['Position'][i] + f * (b['Position'][i] - a['Position'][i]) for i in range(3)]
                break
    return [list(lo), [lo[i] + size[i] for i in range(3)]]


def static_boxes(layout):
    meta = layout['Animation']
    return [[b['Min'], b['Max']] for b in meta['StaticBlocks']]


def all_boxes(layout, t):
    meta = layout['Animation']
    boxes = [[b['Min'], b['Max']] for b in meta['StaticBlocks']]
    boxes.extend(box_at(m, t) for m in meta['Movers'])
    return boxes


def point_inside(p, box):
    return all(box[0][i] - EPS <= p[i] <= box[1][i] + EPS for i in range(3))


def seg_intersects(p, q, box):
    # Closed-box, tangent-blocking slab test, parameter u in [0, 1].
    near, far = 0.0, 1.0
    for i in range(3):
        d = q[i] - p[i]
        if d == 0:
            if p[i] < box[0][i] or p[i] > box[1][i]:
                return False
            continue
        lo = (box[0][i] - p[i]) / d
        hi = (box[1][i] - p[i]) / d
        if lo > hi:
            lo, hi = hi, lo
        near, far = max(near, lo), min(far, hi)
        if near > far:
            return False
    return True


def merge_intervals(intervals):
    intervals = sorted((a, b) for a, b in intervals if b - a > 1e-12)
    out = []
    for a, b in intervals:
        if out and a <= out[-1][1] + 1e-10:
            out[-1][1] = max(out[-1][1], b)
        else:
            out.append([a, b])
    return out


def invert_intervals(intervals, lo, hi):
    out, cursor = [], lo
    for a, b in merge_intervals(intervals):
        if a > cursor:
            out.append([cursor, min(a, hi)])
        cursor = max(cursor, b)
        if cursor >= hi:
            break
    if cursor < hi:
        out.append([cursor, hi])
    return [(a, b) for a, b in out if b - a > 1e-12]


def solid_intervals(layout, p, lo, hi):
    # Times at which p lies in a static block or inside any moving box.
    blocked = []
    for box in static_boxes(layout):
        if point_inside(p, box):
            blocked.append([lo, hi])
    for mover in layout['Animation']['Movers']:
        size = mover['Size']
        for k in range(len(mover['Keys']) - 1):
            a, b = mover['Keys'][k], mover['Keys'][k + 1]
            if b['Time'] <= lo or a['Time'] >= hi:
                continue
            seg_lo, seg_hi = max(lo, a['Time']), min(hi, b['Time'])
            entry, leave = [], []
            for i in range(3):
                v = (b['Position'][i] - a['Position'][i]) / (b['Time'] - a['Time'])
                # p inside [pa+v*(t-ta), pa+v*(t-ta)+size]
                pa = a['Position'][i]
                if v == 0:
                    if not (pa - EPS <= p[i] <= pa + size[i] + EPS):
                        entry, leave = None, None
                        break
                    continue
                t_lo = (p[i] - pa) / v + a['Time']
                t_hi = (p[i] - pa - size[i]) / v + a['Time']
                entry.append(min(t_lo, t_hi))
                leave.append(max(t_lo, t_hi))
            if entry is None:
                continue
            blocked.append([max(seg_lo, max(entry)), min(seg_hi, min(leave))])
    return merge_intervals(blocked)


def occlusion_intervals(layout, p, q, lo, hi):
    # Times at which the closed segment p->q meets any box. Static geometry
    # either blocks for the whole range or never; movers are swept analytically
    # via the affine slab-fraction boundaries (tangent contacts included).
    blocked = []
    for box in static_boxes(layout):
        if seg_intersects(p, q, box):
            blocked.append([lo, hi])
    d = sub(q, p)
    for mover in layout['Animation']['Movers']:
        for k in range(len(mover['Keys']) - 1):
            a, b = mover['Keys'][k], mover['Keys'][k + 1]
            if b['Time'] <= lo or a['Time'] >= hi:
                continue
            seg_lo, seg_hi = max(lo, a['Time']), min(hi, b['Time'])
            events = [seg_lo, seg_hi]
            for i in range(3):
                if d[i] == 0:
                    # Fixed segment coordinate: only the two face crossings
                    # can change the slab predicate.
                    v = (b['Position'][i] - a['Position'][i]) / (b['Time'] - a['Time'])
                    pa, size = a['Position'][i], mover['Size'][i]
                    if v != 0:
                        for edge in (pa, pa + size):
                            t = (edge - p[i]) / v + a['Time']
                            if seg_lo < t < seg_hi:
                                events.append(t)
                    continue
                v = (b['Position'][i] - a['Position'][i]) / (b['Time'] - a['Time'])
                pa = a['Position'][i]
                size = mover['Size'][i]
                # Slab fraction bounds are affine in t.
                lines = [
                    ((pa - p[i]) / d[i] - v * a['Time'] / d[i], v / d[i]),
                    ((pa + size - p[i]) / d[i] - v * a['Time'] / d[i], v / d[i]),
                    (0.0, 0.0),
                    (1.0, 0.0),
                ]
                for x in range(len(lines)):
                    for y in range(x + 1, len(lines)):
                        dv = lines[y][1] - lines[x][1]
                        if dv != 0:
                            t = (lines[x][0] - lines[y][0]) / dv
                            if seg_lo < t < seg_hi:
                                events.append(t)
            ts = sorted(set(events))
            for j in range(len(ts) - 1):
                mid = (ts[j] + ts[j + 1]) / 2
                if seg_intersects(p, q, box_at(mover, mid)):
                    blocked.append([ts[j], ts[j + 1]])
    return merge_intervals(blocked)


def grid_position(grid, x, y, z):
    origin, step = grid['Origin'], grid['Step']
    return [origin[i] + step * coord for i, coord in enumerate((x, y, z))]


def locate(grid, p):
    step, size, origin = grid['Step'], grid['Size'], grid['Origin']
    coords = []
    for i in range(3):
        at = (p[i] - origin[i]) / step
        top = size[i] - 1
        if at < -1e-9 or at > top + 1e-9:
            return None
        at = min(max(at, 0.0), float(top))
        lo = min(math.floor(at), size[i] - 2)
        coords.append((int(lo), at - lo))
    return coords


def corners(grid, cell):
    out = []
    for mask in range(8):
        w, idx = 1.0, []
        for i, (lo, frac) in enumerate(cell):
            c = lo
            if mask & (1 << i):
                c += 1
                w *= frac
            else:
                w *= 1 - frac
            idx.append(c)
        if w > 0:
            linear = idx[0] + size0(grid) * (idx[1] + grid['Size'][1] * idx[2])
            out.append((linear, w))
    return out


def size0(grid):
    return grid['Size'][0]


def lobes_at(timeline, probe_index, t):
    track = timeline['Tracks'][probe_index]
    assert track['Index'] == probe_index
    for run in track['Runs']:
        if timeline['Times'][run['From']] <= t <= timeline['Times'][run['To']]:
            return run['Lobes']
    return track['Runs'][-1]['Lobes']


def run_breakpoints(timeline, probe_index, lo, hi):
    out = []
    for run in timeline['Tracks'][probe_index]['Runs']:
        for idx in (run['From'], run['To']):
            t = timeline['Times'][idx]
            if lo <= t <= hi:
                out.append(t)
    return out


def split_at(times, breakpoints):
    out = []
    for a, b in times:
        cuts = [a] + sorted(t for t in set(breakpoints) if a < t < b) + [b]
        for i in range(len(cuts) - 1):
            if cuts[i + 1] > cuts[i]:
                out.append((cuts[i], cuts[i + 1]))
    return out


def preview_probe(layout, timeline, probe_index, shutter):
    grid = layout['Grid']
    x = probe_index % grid['Size'][0]
    y = (probe_index // grid['Size'][0]) % grid['Size'][1]
    z = probe_index // (grid['Size'][0] * grid['Size'][1])
    p = grid_position(grid, x, y, z)
    lo, hi = shutter
    valid = invert_intervals(solid_intervals(layout, p, lo, hi), lo, hi)
    duration = sum(b - a for a, b in valid)
    if duration <= EPS:
        return 0.0, [[0.0, 0.0, 0.0] for _ in range(6)]
    avg = [[0.0, 0.0, 0.0] for _ in range(6)]
    for a, b in split_at(valid, run_breakpoints(timeline, probe_index, lo, hi)):
        lobes = lobes_at(timeline, probe_index, (a + b) / 2)
        for face in range(6):
            for c in range(3):
                avg[face][c] += lobes[face][c] * (b - a)
    for face in range(6):
        for c in range(3):
            avg[face][c] /= duration
    return duration, avg


def evaluate_sample(layout, timeline, p, shutter):
    grid = layout['Grid']
    lo, hi = shutter
    total = hi - lo
    cell = locate(grid, p)
    if cell is None:
        return {'State': 'outside', 'Weight': 0.0, 'SupportDuration': 0.0,
                'SolidDuration': 0.0, 'Irradiance': [[0.0, 0.0, 0.0] for _ in range(6)]}
    solid = solid_intervals(layout, p, lo, hi)
    solid_duration = sum(min(b, hi) - max(a, lo) for a, b in solid)
    if solid_duration >= total - 1e-9:
        return {'State': 'solid', 'Weight': 0.0, 'SupportDuration': 0.0,
                'SolidDuration': solid_duration, 'Irradiance': [[0.0, 0.0, 0.0] for _ in range(6)]}
    support = invert_intervals(solid, lo, hi)
    # Restrict further by per-neighbor visibility/occupancy.
    block_events = [lo, hi]
    neighbor_data = []
    for n_index, n_weight in corners(grid, cell):
        x = n_index % grid['Size'][0]
        y = (n_index // grid['Size'][0]) % grid['Size'][1]
        z = n_index // (grid['Size'][0] * grid['Size'][1])
        probe_pos = grid_position(grid, x, y, z)
        nb = solid_intervals(layout, probe_pos, lo, hi)
        vis = occlusion_intervals(layout, p, probe_pos, lo, hi)
        neighbor_data.append((n_index, n_weight, probe_pos, nb, vis))
        for intervals in (nb, vis):
            for a, b in intervals:
                block_events += [a, b]
        # Lobe changes need not move any geometry predicate.
        block_events += run_breakpoints(timeline, n_index, lo, hi)
    ts = sorted(t for t in set(block_events) if lo <= t <= hi)
    support_duration, color_weight = 0.0, 0.0
    color_integral = [[0.0, 0.0, 0.0] for _ in range(6)]
    for j in range(len(ts) - 1):
        a, b = ts[j], ts[j + 1]
        if b - a <= 1e-12:
            continue
        mid = (a + b) / 2
        if any(x0 <= mid <= x1 for x0, x1 in solid):
            continue
        moment_weight, moment_color = 0.0, [[0.0, 0.0, 0.0] for _ in range(6)]
        for n_index, n_weight, probe_pos, nb, vis in neighbor_data:
            if any(x0 - EPS <= mid <= x1 + EPS for x0, x1 in nb):
                continue
            if any(x0 - EPS <= mid <= x1 + EPS for x0, x1 in vis):
                continue
            moment_weight += n_weight
            lobes = lobes_at(timeline, n_index, mid)
            for face in range(6):
                for c in range(3):
                    moment_color[face][c] += lobes[face][c] * n_weight
        if moment_weight <= 0:
            continue
        length = b - a
        support_duration += length
        color_weight += length
        for face in range(6):
            for c in range(3):
                color_integral[face][c] += moment_color[face][c] / moment_weight * length
    if support_duration <= 1e-9:
        return {'State': 'unsupported', 'Weight': 0.0, 'SupportDuration': 0.0,
                'SolidDuration': solid_duration, 'Irradiance': [[0.0, 0.0, 0.0] for _ in range(6)]}
    irradiance = [[v / color_weight for v in face] for face in color_integral]
    return {'State': 'sampled', 'Weight': support_duration / total,
            'SupportDuration': support_duration, 'SolidDuration': solid_duration,
            'Irradiance': irradiance}


def quantize(energy, exposure):
    mapped = -math.expm1(-energy * exposure)
    if mapped >= 1:
        return 255
    return int(round(255 * mapped))


def sliding_scene():
    return {
        'Grid': {'Origin': [0, 0, 0], 'Step': 1, 'Size': [4, 2, 2]},
        'Exposure': 3,
        'Lights': [{'Position': [-2, 0, 0], 'RGB': [1, 1, 1], 'Radius': 12, 'Power': 4}],
        'Blocks': [
            # Two static slabs leave a gap on y in (0.1, 0.9); every probe at
            # x=1 is inside one of them, but sample (1, 0.5, 0) sits in the gap.
            {'Min': [0.9, 0.9, -0.5], 'Max': [1.1, 1.5, 0.5]},
            {'Min': [0.9, -0.5, -0.5], 'Max': [1.1, 0.1, 0.5]},
        ],
        'Animation': {
            'Range': [0.0, 3.0],
            'Shutter': {'Start': 0.0, 'End': 3.0},
            'Movers': [
                # Sliding door: thin x slab moving +x at 1 unit/s.
                {'Size': [0.2, 2.0, 2.0],
                 'Keys': [{'Time': 0.0, 'Position': [-0.1, -1.0, -1.0]},
                          {'Time': 3.0, 'Position': [2.9, -1.0, -1.0]}]},
            ],
        },
        'Samples': [
            [2.0, 0.0, 0.0],    # door passes through; solid for a 0.2s window
            [1.0, 0.5, 0.0],    # both y-neighbors are static-invalid -> unsupported
            [1.0, 1.0, 0.0],    # inside the upper static slab for the whole shutter -> solid
            [3.5, 0.0, 0.0],    # outside the grid
            [0.0, 0.0, 0.0],    # always supported corner
        ],
    }


def tangent_scene():
    return {
        'Grid': {'Origin': [0, 0, 0], 'Step': 1, 'Size': [3, 2, 2]},
        'Exposure': 3,
        'Lights': [{'Position': [-2, 0, 0], 'RGB': [1, 1, 1], 'Radius': 12, 'Power': 4}],
        'Blocks': [],
        'Animation': {
            'Range': [0.0, 3.0],
            'Shutter': {'Start': 0.0, 'End': 3.0},
            'Movers': [
                # Door sweeps in z one unit away from the probe plane: at
                # t=1.5 its Max face reaches z=0 exactly and never crosses it,
                # so contact with the z=0 probe is an isolated tangent time
                # with zero duration.
                {'Size': [2.0, 2.0, 1.0],
                 'Keys': [{'Time': 0.0, 'Position': [-1.0, -1.0, -3.0]},
                          {'Time': 3.0, 'Position': [-1.0, -1.0, -1.0]}]},
            ],
        },
        'Samples': [[0.0, 0.0, 0.0]],
    }


def shading_scene():
    # The slab sweeps through y above every probe (y <= 1) and above every
    # sample->probe segment, so it never changes an occupancy or neighbor
    # visibility predicate. It does cross the lamp->probe sightlines, which is
    # visible only as a lobe breakpoint in the baked timeline.
    return {
        'Grid': {'Origin': [0, 0, 0], 'Step': 1, 'Size': [3, 2, 2]},
        'Exposure': 3,
        'Lights': [{'Position': [-3, 2, 0], 'RGB': [1, 1, 1], 'Radius': 20, 'Power': 4}],
        'Blocks': [],
        'Animation': {
            'Range': [0.0, 3.0],
            'Shutter': {'Start': 1.8, 'End': 2.4},
            'Movers': [
                {'Size': [4.0, 0.2, 4.0],
                 'Keys': [{'Time': 0.0, 'Position': [-2, 0.0, -2]},
                          {'Time': 3.0, 'Position': [-2, 2.6, -2]}]}],
        },
        'Samples': [[1.0, 0.5, 0.0]],
    }


def dark_scene():
    # No light reaches anything (radius 0.1 around a far-away lamp), but the
    # mover never covers a probe: valid duration exists while energy is zero.
    return {
        'Grid': {'Origin': [0, 0, 0], 'Step': 1, 'Size': [2, 2, 2]},
        'Exposure': 3,
        'Lights': [{'Position': [-50, -50, -50], 'RGB': [1, 1, 1], 'Radius': 0.1, 'Power': 1}],
        'Blocks': [],
        'Animation': {
            'Range': [0.0, 2.0],
            'Shutter': {'Start': 0.0, 'End': 2.0},
            'Movers': [
                {'Size': [0.2, 0.2, 0.2],
                 'Keys': [{'Time': 0.0, 'Position': [10.0, 10.0, 10.0]},
                          {'Time': 2.0, 'Position': [12.0, 12.0, 12.0]}]},
            ],
        },
        'Samples': [[0.0, 0.0, 0.0]],
    }


def check_animation(s):
    scene = sliding_scene()
    row = receipt(s.post(scene))
    blob = fetch(s, row)
    with zipfile.ZipFile(io.BytesIO(blob)) as z:
        assert set(z.namelist()) == {'layout.json', 'probes.png', 'timeline.json'}
        assert z.testzip() is None
        layout = json.loads(z.read('layout.json'))
        timeline = json.loads(z.read('timeline.json'))
        w, h, rgba = pixels(z.read('probes.png'))
    assert layout['Schema'] == 'lantern-probes/v2'
    assert timeline['Schema'] == 'lantern-timeline/v2' and timeline['Version'] == 2
    assert timeline['Range'] == [0.0, 3.0]
    assert timeline['Times'] == sorted(timeline['Times'])
    assert len(timeline['Tracks']) == 16
    for track in timeline['Tracks']:
        assert track['Runs'][0]['From'] == 0
        assert track['Runs'][-1]['To'] == len(timeline['Times']) - 1
        for a, b in zip(track['Runs'], track['Runs'][1:]):
            assert a['To'] == b['From'] and a['Lobes'] != b['Lobes']
    assert (w, h) == (24, 4)
    meta = layout['Animation']
    assert meta['Shutter'] == scene['Animation']['Shutter']
    assert meta['Movers'] == scene['Animation']['Movers']
    assert meta['StaticBlocks'] == scene['Blocks']

    # Preview pixel semantics: every x=1 probe is static-invalid for the
    # whole shutter -> transparent; (2,0,0) has valid time -> opaque.
    def pixel_of(x, y, z, face):
        return rgba[y + z * 2][x + face * 4]
    assert pixel_of(1, 1, 0, 0) == (0, 0, 0, 0)
    assert pixel_of(1, 0, 0, 0) == (0, 0, 0, 0)
    assert pixel_of(2, 0, 0, 0)[3] == 255

    # Independent engine reproduces every baked sample from the artifact alone.
    for baked, point in zip(layout['Samples'], scene['Samples']):
        mine = evaluate_sample(layout, timeline, point, (0.0, 3.0))
        assert baked['State'] == mine['State'], (point, baked, mine)
        assert abs(baked['Weight'] - mine['Weight']) < 1e-9
        assert abs(baked.get('SupportDuration', 0.0) - mine['SupportDuration']) < 1e-9
        assert abs(baked.get('SolidDuration', 0.0) - mine['SolidDuration']) < 1e-9
        for face in range(6):
            for c in range(3):
                assert abs(baked['Irradiance'][face][c] - mine['Irradiance'][face][c]) < 1e-9

    states = [x['State'] for x in layout['Samples']]
    assert states == ['sampled', 'unsupported', 'solid', 'outside', 'sampled'], states
    passing = layout['Samples'][0]
    # Door reaches x=2 at t=2: point x=2 is solid for 0.2s of the 3s shutter.
    assert abs(passing['SolidDuration'] - 0.2) < 1e-9
    assert abs(passing['Weight'] - 2.8 / 3.0) < 1e-9
    assert passing['SupportDuration'] > 0 and passing['State'] == 'sampled'

    # The same package answers a different shutter without re-baking. From
    # t=1.7 the slab shades the only trilinear neighbor until it passes the
    # point at t=2.3; with 0.2s solid in the middle, the whole window has no
    # supported time at all.
    alt = evaluate_sample(layout, timeline, [2.0, 0.0, 0.0], (1.7, 2.5))
    assert alt['State'] == 'unsupported'
    assert abs(alt['SolidDuration'] - 0.2) < 1e-9
    assert abs(alt['SupportDuration']) < 1e-9
    # A shutter covering only the solid window has no supported time.
    narrow = evaluate_sample(layout, timeline, [2.0, 0.0, 0.0], (1.95, 2.05))
    assert narrow['State'] == 'solid'
    # Before the slab reaches x=2 the point is never solid or occluded, yet
    # the probe is supported-but-black while the door shades the lamp: black
    # supported time still counts in the average.
    side = evaluate_sample(layout, timeline, [2.0, 0.0, 0.0], (1.4, 1.6))
    assert side['State'] == 'sampled'
    assert abs(side['SolidDuration']) < 1e-9 and abs(side['SupportDuration'] - 0.2) < 1e-9
    assert abs(side['Weight'] - 1.0) < 1e-9
    assert not any(any(v) for v in side['Irradiance'])

    # Preview texture matches the independently averaged, exposure-encoded lobes.
    exposure = layout['Exposure']
    for index in range(16):
        duration, avg = preview_probe(layout, timeline, index, (0.0, 3.0))
        x = index % 4
        y = (index // 4) % 2
        z = index // 8
        for face in range(6):
            r, g, b, a = pixel_of(x, y, z, face)
            if duration <= EPS:
                assert (r, g, b, a) == (0, 0, 0, 0)
            else:
                assert a == 255
                assert abs(r - quantize(avg[face][0], exposure)) <= 1
                assert abs(g - quantize(avg[face][1], exposure)) <= 1
                assert abs(b - quantize(avg[face][2], exposure)) <= 1

    # Tangent-only contact at an isolated time contributes no duration.
    tangent = tangent_scene()
    trow = receipt(s.post(tangent))
    with zipfile.ZipFile(io.BytesIO(fetch(s, trow))) as z:
        tlayout = json.loads(z.read('layout.json'))
        ttimeline = json.loads(z.read('timeline.json'))
    mine = evaluate_sample(tlayout, ttimeline, [0.0, 0.0, 0.0], (0.0, 3.0))
    assert mine['State'] == 'sampled' and abs(mine['Weight'] - 1.0) < 1e-9
    assert abs(mine['SupportDuration'] - 3.0) < 1e-9
    assert tlayout['Samples'][0]['State'] == 'sampled'
    assert abs(tlayout['Samples'][0]['Weight'] - 1.0) < 1e-9

    # A shutter that ends exactly at an isolated tangent moment still gains
    # no occlusion duration: endpoint contacts have zero length.
    edge = evaluate_sample(tlayout, ttimeline, [0.0, 0.0, 0.0], (0.0, 1.5))
    assert edge['State'] == 'sampled' and abs(edge['Weight'] - 1.0) < 1e-9

    # Lamp-only shading: the mover never touches a probe or neighbor
    # sightline, so the color must still switch exactly at the baked lobe
    # breakpoint inside the shutter. Both engines must integrate the two
    # constant-color pieces by their true durations.
    shading = shading_scene()
    srow = receipt(s.post(shading))
    with zipfile.ZipFile(io.BytesIO(fetch(s, srow))) as z:
        slayout = json.loads(z.read('layout.json'))
        stimeline = json.loads(z.read('timeline.json'))
    sbaked = slayout['Samples'][0]
    smine = evaluate_sample(slayout, stimeline, [1.0, 0.5, 0.0], (1.8, 2.4))
    assert sbaked['State'] == smine['State'] == 'sampled'
    assert abs(sbaked['Weight'] - 1.0) < 1e-9
    for face in range(6):
        for c in range(3):
            assert abs(sbaked['Irradiance'][face][c] - smine['Irradiance'][face][c]) < 1e-9
    # A lobe breakpoint really does fall inside this shutter.
    neighbor_indices = [n[0] for n in corners(slayout['Grid'], locate(slayout['Grid'], [1.0, 0.5, 0.0]))]
    internal = [t for ni in neighbor_indices for t in run_breakpoints(stimeline, ni, 1.8, 2.4)
                if 1.8 < t < 2.4]
    assert internal, 'shading scene must straddle a lamp-sightline breakpoint'
    # The preview texture obeys the same breakpoint-aware average.
    with zipfile.ZipFile(io.BytesIO(fetch(s, srow))) as z:
        _, _, srgb = pixels(z.read('probes.png'))
    for ni in neighbor_indices:
        duration, avg = preview_probe(slayout, stimeline, ni, (1.8, 2.4))
        assert duration > 0
        x, y = ni % 3, (ni // 3) % 2
        for face in range(6):
            r, g, b, a = srgb[y][x + face * 3]
            assert a == 255
            assert abs(r - quantize(avg[face][0], slayout['Exposure'])) <= 1

    # Valid but lightless probes must render as opaque black pixels.
    drow = receipt(s.post(dark_scene()))
    with zipfile.ZipFile(io.BytesIO(fetch(s, drow))) as z:
        dlayout = json.loads(z.read('layout.json'))
        _, _, drgba = pixels(z.read('probes.png'))
    assert dlayout['Statistics']['Valid'] == 8 and dlayout['Statistics']['Dark'] == 8
    assert all(pixel == (0, 0, 0, 255) for row in drgba for pixel in row)
    assert dlayout['Samples'][0]['State'] == 'sampled'
    assert abs(dlayout['Samples'][0]['Weight'] - 1.0) < 1e-9

    # Validation boundaries for animated requests.
    def reject(mutate):
        bad = json.loads(json.dumps(scene))
        mutate(bad)
        assert s.post(bad)[0] == 422, bad
    reject(lambda b: b['Animation'].update({'Range': [3.0, 0.0]}))
    reject(lambda b: b['Animation']['Shutter'].update({'Start': 2.0, 'End': 1.0}))
    reject(lambda b: b['Animation']['Shutter'].update({'Start': -0.1, 'End': 1.0}))
    reject(lambda b: b['Animation']['Shutter'].update({'Start': 0.0, 'End': 3.5}))
    reject(lambda b: b['Animation']['Movers'][0]['Keys'][1].update({'Time': 2.0}))
    reject(lambda b: b['Animation']['Movers'][0].update({'Size': [0.0, 2.0, 2.0]}))
    reject(lambda b: b['Animation'].update({'Movers': []}))
    # Endpoint shutters touching the range are accepted.
    ok = json.loads(json.dumps(scene))
    ok['Animation']['Shutter'] = {'Start': 0.0, 'End': 3.0}
    assert receipt(s.post(ok))
    # Unknown fields are still contract-level 400s.
    bad = json.loads(json.dumps(scene))
    bad['Animation']['Movers'][0]['Color'] = [1, 0, 0]
    assert s.post(bad)[0] == 400


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else 'all'
    if mode not in ('product', 'persistence', 'contract', 'animation', 'all'):
        raise SystemExit('mode: product | persistence | contract | animation | all')
    with tempfile.TemporaryDirectory(prefix='media-workflow-') as root:
        binary, data = Path(root) / 'server', Path(root) / 'artifacts'
        subprocess.run(['go', 'build', '-o', str(binary), './cmd/server'], check=True)
        with Service(binary, data) as s:
            if mode in ('contract', 'all'):
                contract(s)
            if mode in ('product', 'all'):
                check_product(s)
            if mode in ('animation', 'all'):
                check_animation(s)
            if mode in ('persistence', 'all'):
                example = s.demo()
                with concurrent.futures.ThreadPoolExecutor(2) as pool:
                    results = list(pool.map(s.post, [example, example]))
                assert [receipt(x) for x in results][0] == [receipt(x) for x in results][1]
                row = receipt(results[0])
                stock = fetch(s, row)
                assert not list(data.glob('.staging-*'))
                assert s.call(row['artifact'], headers={'If-None-Match': '"' + row['digest'] + '"'})[0] == 304
                code, headers, prefix = s.call(row['artifact'], headers={'Range': 'bytes=0-7'})
                assert code == 206 and prefix == stock[:8]
        if mode in ('persistence', 'all'):
            with Service(binary, data) as s:
                assert fetch(s, row) == stock
                (data / row['digest']).write_bytes(b'corrupted')
                assert s.call(row['artifact'])[0] == 409
    print(mode + ': passed; temporary listener closed')


if __name__ == '__main__':
    main()
