import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request


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


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else 'all'
    if mode not in ('product', 'persistence', 'contract', 'all'):
        raise SystemExit('mode: product | persistence | contract | all')
    with tempfile.TemporaryDirectory(prefix='media-workflow-') as root:
        binary, data = Path(root)/'server', Path(root)/'artifacts'
        subprocess.run(['go','build','-o',str(binary),'./cmd/server'], check=True)
        with Service(binary, data) as s:
            if mode in ('contract', 'all'):
                contract(s)
            if mode in ('product', 'all'):
                check_product(s)
            if mode in ('persistence', 'all'):
                example = s.demo()
                with concurrent.futures.ThreadPoolExecutor(2) as pool:
                    results = list(pool.map(s.post, [example, example]))
                assert [receipt(x) for x in results][0] == [receipt(x) for x in results][1]
                row = receipt(results[0]); stock = fetch(s, row)
                assert not list(data.glob('.staging-*'))
                assert s.call(row['artifact'], headers={'If-None-Match':'"' + row['digest'] + '"'})[0] == 304
                code, headers, prefix = s.call(row['artifact'], headers={'Range':'bytes=0-7'})
                assert code == 206 and prefix == stock[:8]
        if mode in ('persistence', 'all'):
            with Service(binary, data) as s:
                assert fetch(s,row) == stock
                (data/row['digest']).write_bytes(b'corrupted')
                assert s.call(row['artifact'])[0] == 409
    print(mode + ': passed; temporary listener closed')


import io
import struct
import zipfile
import zlib


def pixels(data):
    assert data[:8] == b'\x89PNG\r\n\x1a\n'
    compressed = b''
    offset = 8
    while offset < len(data):
        n, type_ = struct.unpack_from('>I4s',data,offset)
        payload = data[offset+8:offset+8+n]
        assert zlib.crc32(type_+payload) == struct.unpack_from('>I',data,offset+8+n)[0]
        if type_ == b'IHDR':
            w,h,bits,typ,_,_,interlaced = struct.unpack('>IIBBBBB',payload)
            assert (bits,typ,interlaced) == (8,6,0)
        if type_ == b'IDAT': compressed += payload
        offset += n+12
    raw = zlib.decompress(compressed)
    rows, prev, stride = [], [0]*(4*w), 4*w
    for y in range(h):
        first = y*(stride+1); method = raw[first]
        line = list(raw[first+1:first+1+stride])
        for i in range(stride):
            a,b,c = line[i-4] if i>=4 else 0,prev[i],prev[i-4] if i>=4 else 0
            if method == 4:
                p=a+b-c
                delta=min((abs(p-k),j,k) for j,k in enumerate((a,b,c)))[2]
            else:
                delta=(0,a,b,(a+b)//2)[method]
            line[i]=(line[i]+delta)&255
        rows.append([tuple(line[x:x+4]) for x in range(0,stride,4)])
        prev=line
    return w,h,rows


def bundle(blob):
    z = zipfile.ZipFile(io.BytesIO(blob))
    assert z.testzip() is None
    return set(z.namelist()), json.loads(z.read('layout.json')), z


def check_legacy(s):
    req = {
        'Grid': {'Origin': [0, 0, 0], 'Step': 1, 'Size': [3, 2, 2]},
        'Exposure': 3,
        'Lights': [{'Position': [-1, 0, 0], 'RGB': [1, 0.55, 0.2], 'Radius': 6, 'Power': 4}],
        'Blocks': [
            {'Min': [0.8, -0.2, -0.2], 'Max': [1.2, 0.2, 0.2]},
            {'Min': [1.4, -0.2, -0.2], 'Max': [1.6, 1.2, 1.2]},
        ],
        'Samples': [[0, 0, 0], [0.4, 0, 0], [1, 0, 0], [2, 0, 0], [3, 0, 0]],
    }
    row = receipt(s.post(req))
    names, layout, z = bundle(fetch(s, row))
    assert names == {'layout.json', 'probes.png'}
    w, h, rgba = pixels(z.read('probes.png'))
    assert (w, h) == (18, 4) == (layout['TextureWidth'], layout['TextureHeight'])
    assert layout['Schema'] == 'lantern-probes/v1'
    assert layout['Statistics']['Valid'] == 11 and layout['Statistics']['OccludedRays'] > 0
    assert layout['Axes'] == ['+X', '-X', '+Y', '-Y', '+Z', '-Z']
    assert rgba[0][3][0] > rgba[0][3][1] > 0 and rgba[0][3][3] == 255
    assert rgba[0][4] == (0, 0, 0, 0) and rgba[0][5] == (0, 0, 0, 255)
    states = [x['State'] for x in layout['Samples']]
    assert states == ['sampled', 'sampled', 'solid', 'sampled', 'outside'], states
    assert 0 < layout['Samples'][1]['Weight'] < 1


def check_crossdoor(s):
    req = s.demo()
    row = receipt(s.post(req))
    names, layout, z = bundle(fetch(s, row))
    grids = layout['Grids']
    assert names == {'layout.json', 'probes-0.png', 'probes-1.png'} == {'layout.json'} | {g['Texture'] for g in grids}
    assert layout['Schema'] == 'lantern-probes/v2'
    assert [g['Side'] for g in grids] == [-1, 1]
    assert grids[0]['Step'] != grids[1]['Step']          # mismatched spacing
    assert layout['Wall']['Opening'] == {'Min': [-1, -1], 'Max': [1, 1]}
    assert layout['Wall']['Normal'] == [1, 0, 0]

    # Each atlas is a six-face strip of the grid's own slot count, and every
    # slot is transparent exactly when it sits on the closed wall or in/on a
    # solid box.
    for g in grids:
        w, h, rgba = pixels(z.read(g['Texture']))
        assert (w, h) == (g['Size'][0] * 6, g['Size'][1] * g['Size'][2])
        assert (w, h) == (g['TextureWidth'], g['TextureHeight'])
        assert any(px[3] == 255 for row_ in rgba for px in row_)
        assert any(px[3] == 0 for row_ in rgba for px in row_)
        for zc in range(g['Size'][2]):
            for yc in range(g['Size'][1]):
                for xc in range(g['Size'][0]):
                    pos = [g['Origin'][a] + st * c for a, (c, st) in
                           enumerate(zip((xc, yc, zc), (g['Step'],) * 3))]
                    solid = on_closed_wall(layout['Wall'], pos) or any(
                        inside_box(b, pos) for b in req['Blocks'])
                    for face in range(6):
                        alpha = rgba[yc + zc * g['Size'][1]][xc + face * g['Size'][0]][3]
                        assert alpha == (0 if solid else 255), (g['Texture'], pos, face, alpha)

    # Distinct states must not collapse: solid, outside and unsupported differ.
    states = {tuple(x['Position']): x['State'] for x in layout['Samples']}
    assert states[(0, 1.5, 0)] == 'solid'      # on the closed wall
    assert states[(1.2, 0, -0.2)] == 'solid'   # inside a solid box
    assert states[(3, 0, 0)] == 'outside'      # beyond both grids
    assert states[(-2, 0, 0)] == 'outside'

    # Door centre: both mismatched grids contribute visible support.
    door = next(x for x in layout['Samples'] if tuple(x['Position']) == (0, 0, 0))
    assert door['State'] == 'sampled'
    c = door['Contributions']
    assert [x['Side'] for x in c] == [-1, 1]
    assert all(x['Covered'] and x['Support'] > 0 for x in c)
    assert abs(c[0]['Nominal'] - 0.5) < 1e-9 and abs(c[1]['Nominal'] - 0.5) < 1e-9
    assert abs(sum(x['Effective'] for x in c) - door['Weight']) < 1e-9
    assert door['Weight'] > 0 and any(any(axis) for axis in door['Irradiance'])

    # The two grids read different colours: each side's own nominal weight
    # dominates once the query sits off-centre but still in the band.
    left = next(x for x in layout['Samples'] if tuple(x['Position']) == (-0.6, 0, 0))
    right = next(x for x in layout['Samples'] if tuple(x['Position']) == (0.4, 0, 0))
    assert left['Contributions'][0]['Nominal'] > left['Contributions'][1]['Nominal']
    assert right['Contributions'][1]['Nominal'] > right['Contributions'][0]['Nominal']
    assert energy(left) > energy(right)        # light lives on the -Side

    # Beyond the band on a side, the nominal share saturates at the end value:
    # at x = -0.6 (outside d = -0.5) the +Side grid contributes exactly zero,
    # while x = 0.4 is still inside the band so its -Side share is fractional.
    assert left['Contributions'][1]['Nominal'] == 0 and left['Contributions'][1]['Effective'] == 0
    assert 0 < right['Contributions'][0]['Nominal'] < 0.5

    # Behind the wall, off to the frame side, the walled-off part is gone: the
    # grid on the far side of the wall loses blocked neighbours, so its own
    # support and the blended total both drop below full.
    shadow = next(x for x in layout['Samples'] if tuple(x['Position']) == (0.25, 1.3, 0))
    assert shadow['State'] == 'sampled' and shadow['Weight'] < 1
    assert shadow['Contributions'][0]['Support'] < 1
    assert shadow['Contributions'][1]['Support'] == 1

    # A point only one grid covers keeps that grid's original rule: full share.
    single = next(x for x in layout['Samples'] if tuple(x['Position']) == (1.5, 0, 0))
    assert single['State'] == 'sampled'
    assert [x['Covered'] for x in single['Contributions']] == [False, True]
    assert single['Contributions'][1]['Nominal'] == 1

    # On the far side, only the -Side grid covers, with no wall transition.
    far = next(x for x in layout['Samples'] if tuple(x['Position']) == (-1.2, 0, 0))
    assert far['State'] == 'sampled'
    assert far['Contributions'][0]['Covered'] and far['Contributions'][0]['Nominal'] == 1
    assert not far['Contributions'][1]['Covered']

    # Wall occlusion only: the closed wall in isolation darkens points behind
    # it that can still see a light, while the doorway lets light through.
    open_hole = next(x for x in layout['Samples'] if tuple(x['Position']) == (0, 0.6, 0.6))
    assert open_hole['State'] == 'sampled' and energy(open_hole) > 0

    # Distinct digests for distinct scenes; invalid two-grid requests reject.
    req['Exposure'] = 6
    assert receipt(s.post(req))['digest'] != row['digest']
    req['Exposure'] = 3
    req['Grids'] = req['Grids'][:1]
    assert s.post(req)[0] == 422                          # one grid + wall
    req['Grids'] = s.demo()['Grids']
    req['Grids'][1]['Side'] = -1
    assert s.post(req)[0] == 422                          # both grids same side
    req['Grids'] = s.demo()['Grids']
    req['Wall']['Opening']['Min'] = [-3, -3]
    assert s.post(req)[0] == 422                          # hole outside the wall


def energy(sample):
    return sum(sum(axis) for axis in sample['Irradiance'])


def inside_box(box, p):
    return all(box['Min'][a] <= p[a] <= box['Max'][a] for a in range(3))


def on_closed_wall(wall, p, eps=1e-9):
    c, n, r = wall['Center'], wall['Normal'], wall['Right']
    u = wall['Up']
    d = sum((p[a] - c[a]) * n[a] for a in range(3))
    if abs(d) > eps:
        return False
    lu = sum((p[a] - c[a]) * r[a] for a in range(3))
    lv = sum((p[a] - c[a]) * u[a] for a in range(3))
    if abs(lu) > wall['Half'][0] + eps or abs(lv) > wall['Half'][1] + eps:
        return False
    hole = wall['Opening']
    strict = hole['Min'][0] + eps < lu < hole['Max'][0] - eps and \
        hole['Min'][1] + eps < lv < hole['Max'][1] - eps
    return not strict


def check_unsupported(s):
    # Both grids cover the query, but every neighbour that could feed it is
    # invalid: one grid's window touches a solid box, the other's window lies on
    # the closed wall. The result must read "unsupported", not "outside"/"solid".
    scene = {
        'Grids': [
            {'Side': -1, 'Origin': [-0.4, -0.8, -0.8], 'Step': 0.4, 'Size': [4, 5, 5]},
            {'Side': 1, 'Origin': [-0.8, -0.8, -0.8], 'Step': 0.4, 'Size': [4, 5, 5]},
        ],
        'Wall': {'Center': [0, 0, 0], 'Normal': [1, 0, 0], 'Up': [0, 0, 1],
                 'Half': [2, 2], 'Opening': {'Min': [-1, 1], 'Max': [1, 1.0001]}},
        'Transition': 1, 'Exposure': 3,
        'Lights': [{'Position': [3, 0, 0], 'RGB': [1, 0.8, 0.5], 'Radius': 20, 'Power': 20}],
        'Blocks': [{'Min': [-0.5, -0.85, -0.85], 'Max': [-0.35, 0.85, 0.85]}],
        'Samples': [[-0.1, 0.05, 0.05], [0.3, 0.05, 0.05]],
    }
    row = receipt(s.post(scene))
    _, layout, _ = bundle(fetch(s, row))
    blocked = layout['Samples'][0]
    assert blocked['State'] == 'unsupported' and blocked['Weight'] == 0
    assert all(c['Covered'] for c in blocked['Contributions'])
    open_point = layout['Samples'][1]
    assert open_point['State'] == 'sampled' and open_point['Weight'] > 0
    assert any(any(axis) for axis in open_point['Irradiance'])


def check_shadow_gap(s):
    # A small doorway with coarse grids: at the door centre both mismatched
    # grids see the light through the hole, but one step beside the frame the
    # walled-off light and the blocked neighbours both vanish.
    scene = {
        'Grids': [
            {'Side': -1, 'Origin': [-1.0, -0.8, -0.8], 'Step': 0.8, 'Size': [3, 4, 4]},
            {'Side': 1, 'Origin': [0.2, -0.8, -0.8], 'Step': 0.8, 'Size': [3, 4, 4]},
        ],
        'Wall': {'Center': [0, 0, 0], 'Normal': [1, 0, 0], 'Up': [0, 0, 1],
                 'Half': [2, 2], 'Opening': {'Min': [-0.3, -0.3], 'Max': [0.3, 0.3]}},
        'Transition': 0.6, 'Exposure': 3,
        'Lights': [{'Position': [-0.6, 0, 0], 'RGB': [1, 1, 1], 'Radius': 10, 'Power': 6}],
        'Blocks': [],
        'Samples': [[0.2, 0, 0], [0.2, 0.8, 0], [0.2, -0.8, 0], [0.2, 0, 0.8], [0.2, 0, -0.8]],
    }
    row = receipt(s.post(scene))
    _, layout, _ = bundle(fetch(s, row))
    door, around = layout['Samples'][0], layout['Samples'][1:]
    assert door['State'] == 'sampled' and energy(door) > 0
    assert all(c['Support'] > 0 for c in door['Contributions'])
    for beside in around:
        assert beside['State'] == 'sampled'
        assert energy(beside) == 0                       # blocked light gone
        assert beside['Weight'] < door['Weight']         # blocked support gone
        assert beside['Contributions'][0]['Support'] < 1


def check_overlap(s):
    # The grids may overlap in space. Where both cover but the query is outside
    # the transition band, the side the point falls on carries full nominal
    # weight while the other side saturates at zero.
    scene = {
        'Grids': [
            {'Side': -1, 'Origin': [-2.0, -1, -1], 'Step': 0.5, 'Size': [7, 5, 5]},
            {'Side': 1, 'Origin': [-1.5, -1, -1], 'Step': 0.5, 'Size': [7, 5, 5]},
        ],
        'Wall': {'Center': [0, 0, 0], 'Normal': [1, 0, 0], 'Up': [0, 0, 1],
                 'Half': [2, 2], 'Opening': {'Min': [-1, -1], 'Max': [1, 1]}},
        'Transition': 0.4, 'Exposure': 3,
        'Lights': [{'Position': [-1.5, 0, 0], 'RGB': [1, 1, 1], 'Radius': 10, 'Power': 5}],
        'Blocks': [],
        'Samples': [[-1.4, 0, 0], [-0.3, 0, 0], [0.3, 0, 0], [0.9, 0, 0]],
    }
    row = receipt(s.post(scene))
    _, layout, _ = bundle(fetch(s, row))
    for sample in layout['Samples']:
        assert sample['State'] == 'sampled'
        assert all(c['Covered'] for c in sample['Contributions'])
        sides = {c['Side']: c['Nominal'] for c in sample['Contributions']}
        x = sample['Position'][0]
        dominant = -1 if x < 0 else 1
        assert sides[dominant] == 1 and sides[-dominant] == 0


def check_product(s):
    check_legacy(s)
    check_crossdoor(s)
    check_unsupported(s)
    check_shadow_gap(s)
    check_overlap(s)


if __name__ == '__main__':
    main()
