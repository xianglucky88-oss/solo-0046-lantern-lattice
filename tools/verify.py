import concurrent.futures
import hashlib
import json
import math
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
    if mode not in ('product', 'persistence', 'contract', 'tiled', 'all'):
        raise SystemExit('mode: product | persistence | contract | tiled | all')
    with tempfile.TemporaryDirectory(prefix='media-workflow-') as root:
        binary, data = Path(root)/'server', Path(root)/'artifacts'
        subprocess.run(['go','build','-o',str(binary),'./cmd/server'], check=True)
        with Service(binary, data) as s:
            if mode in ('contract', 'all'):
                contract(s)
            if mode in ('product', 'all'):
                check_product(s)
            if mode in ('tiled', 'all'):
                check_tiled(s)
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


def write_png(w, h, rows):
    def chunk(type_, payload):
        return (struct.pack('>I', len(payload)) + type_ + payload
                + struct.pack('>I', zlib.crc32(type_ + payload) & 0xffffffff))
    raw = bytearray()
    for row in rows:
        raw.append(0)
        for r, g, b, a in row:
            raw.extend((r, g, b, a))
    return (b'\x89PNG\r\n\x1a\n'
            + chunk(b'IHDR', struct.pack('>IIBBBBB', w, h, 8, 6, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(bytes(raw), 9))
            + chunk(b'IEND', b''))


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
            assert bits == 8 and interlaced == 0 and typ in (2, 6)
            channels = 4 if typ == 6 else 3
        if type_ == b'IDAT': compressed += payload
        offset += n+12
    raw = zlib.decompress(compressed)
    rows, stride = [], channels*w
    prev = [0]*stride
    bpp = channels
    for y in range(h):
        first = y*(stride+1); method = raw[first]
        line = list(raw[first+1:first+1+stride])
        for i in range(stride):
            a,b,c = line[i-bpp] if i>=bpp else 0,prev[i],prev[i-bpp] if i>=bpp else 0
            if method == 4:
                p=a+b-c
                delta=min((abs(p-k),j,k) for j,k in enumerate((a,b,c)))[2]
            else:
                delta=(0,a,b,(a+b)//2)[method]
            line[i]=(line[i]+delta)&255
        row = []
        for x in range(0, stride, channels):
            px = tuple(line[x:x+channels])
            row.append(px if channels == 4 else px + (255,))
        rows.append(row)
        prev=line
    return w,h,rows


def check_product(s):
    req = s.demo()
    row = receipt(s.post(req))
    with zipfile.ZipFile(io.BytesIO(fetch(s,row))) as z:
        assert set(z.namelist()) == {'layout.json','probes.png'} and z.testzip() is None
        layout=json.loads(z.read('layout.json'))
        w,h,rgba=pixels(z.read('probes.png'))
    assert (w,h)==(18,4)==(layout['TextureWidth'],layout['TextureHeight'])
    assert layout['Statistics']['Valid']==11 and layout['Statistics']['OccludedRays']>0
    assert layout['Axes']==['+X','-X','+Y','-Y','+Z','-Z']
    assert rgba[0][3][0]>rgba[0][3][1]>0 and rgba[0][3][3]==255
    assert rgba[0][4]==(0,0,0,0) and rgba[0][5]==(0,0,0,255)
    states=[x['State'] for x in layout['Samples']]
    assert states == ['sampled','sampled','solid','sampled','outside'], states
    assert 0 < layout['Samples'][1]['Weight'] < 1
    assert not any(any(axis) for axis in layout['Samples'][3]['Irradiance'])
    req['Exposure']=6
    assert receipt(s.post(req))['digest'] != row['digest']
    req['Grid']['Size']=[0,2,2]
    assert s.post(req)[0]==422
    req['Grid']['Size']=[3,2,2]; req['Blocks'][0]['Min']=req['Blocks'][0]['Max']
    assert s.post(req)[0]==422


def open_zip(blob):
    z = zipfile.ZipFile(io.BytesIO(blob))
    assert z.testzip() is None
    return z, {n: z.read(n) for n in z.namelist()}


def logical_atlas(gx, gy, gz, pages, tiles):
    width, height = gx * 6, gy * gz
    atlas = [[None] * width for _ in range(height)]
    decoded = {}
    for page in pages:
        pw, ph, rows = pixels(page['blob'])
        assert (pw, ph) == (page['Width'], page['Height'])
        assert pw >= 2 and ph >= 2
        decoded[page['File']] = rows
    for tile in tiles:
        src, tgt, page = tile['Source'], tile['Target'], tile['Page']
        page_rows = decoded[pages[page]['File']]
        assert src['W'] == tgt['W'] and src['H'] == tgt['H']
        for dy in range(src['H']):
            for dx in range(src['W']):
                px = page_rows[tgt['Y'] + dy][tgt['X'] + dx]
                lx, ly = src['X'] + dx, src['Y'] + dy
                if atlas[ly][lx] is None:
                    atlas[ly][lx] = px
                else:
                    assert atlas[ly][lx] == px
    assert all(cell is not None for line in atlas for cell in line)
    return atlas


def check_tiled(s):
    req = s.demo()
    legacy_blob = fetch(s, receipt(s.post(req)))
    with zipfile.ZipFile(io.BytesIO(legacy_blob)) as z:
        legacy_layout = json.loads(z.read('layout.json'))
        _, _, legacy_rgba = pixels(z.read('probes.png'))

    req['MaxTextureWidth'], req['MaxTextureHeight'] = 2, 2
    assert s.post(dict(req, MaxTextureWidth=1))[0] == 422
    assert s.post(dict(req, MaxTextureHeight=1))[0] == 422
    only_one = dict(req); del only_one['MaxTextureHeight']
    assert s.post(only_one)[0] == 422

    row = receipt(s.post(req))
    blob = fetch(s, row)
    z, files = open_zip(blob)
    names = set(files)
    assert 'probes.png' not in names and 'layout.json' in names
    layout = json.loads(files['layout.json'])
    assert layout['Schema'] == 'lantern-probes/v2'
    pages, tiles = layout['Pages'], layout['Tiles']
    assert pages and tiles
    assert names == {'layout.json'} | {p['File'] for p in pages}
    for page in pages:
        assert page['Width'] <= 2 and page['Height'] <= 2
        page['blob'] = files[page['File']]
    for tile in tiles:
        assert 0 <= tile['Face'] < 6 and 0 <= tile['Depth'] < req['Grid']['Size'][2]
        face_x = tile['Face'] * req['Grid']['Size'][0]
        assert face_x <= tile['Source']['X'] < face_x + req['Grid']['Size'][0]
        assert tile['Depth'] * req['Grid']['Size'][1] <= tile['Source']['Y'] < (tile['Depth'] + 1) * req['Grid']['Size'][1]

    atlas = logical_atlas(*req['Grid']['Size'], pages, tiles)
    assert atlas == legacy_rgba
    assert layout['Statistics'] == legacy_layout['Statistics']

    # Reordering fragments must not affect import.
    shuffled = json.loads(files['layout.json'])
    shuffled['Tiles'] = list(reversed(shuffled['Tiles']))
    repacked = io.BytesIO()
    with zipfile.ZipFile(repacked, 'w', zipfile.ZIP_STORED) as out:
        for name, data in files.items():
            out.writestr(name, json.dumps(shuffled).encode() if name == 'layout.json' else data)
    status, _, report_blob = s.call('/v1/import', repacked.getvalue(), {'Content-Type': 'application/zip'})
    assert status == 200
    report = json.loads(report_blob)
    assert report['Schema'] == 'lantern-probes/v2' and len(report['Tiles']) == len(tiles)

    # The legacy single-page artifact enters the same import entry.
    assert s.call('/v1/import', legacy_blob, {'Content-Type': 'application/zip'})[0] == 200
    assert s.call('/v1/import', b'not a zip', {'Content-Type': 'application/zip'})[0] == 422
    assert s.call('/v1/import', blob, {'Content-Type': 'application/json'})[0] == 415

    # Every declared defect is rejected at import time, before any query.
    def defect(mutate):
        zf, parts = open_zip(blob)
        bad_layout = json.loads(parts['layout.json'])
        mutate(bad_layout, parts)
        buf = io.BytesIO()
        with zipfile.ZipFile(buf, 'w', zipfile.ZIP_STORED) as out:
            for name, data in parts.items():
                out.writestr(name, json.dumps(bad_layout).encode() if name == 'layout.json' else data)
        assert s.call('/v1/import', buf.getvalue(), {'Content-Type': 'application/zip'})[0] == 422

    defect(lambda m, parts: parts.pop(m['Pages'][0]['File']))
    defect(lambda m, parts: m['Tiles'][0].__setitem__('Face', 9))

    def shrink(m, parts):
        # Last fragment of face 5 holds the rightmost logical column with no
        # overlap neighbor; shrinking it opens a coverage hole.
        last = m['Tiles'][-1]
        last['Source']['W'] -= 1
        last['Target']['W'] -= 1
    defect(shrink)

    def oob(m, parts):
        m['Tiles'][0]['Target']['X'] = m['Pages'][m['Tiles'][0]['Page']]['Width']
    defect(oob)

    def wrong_size(m, parts):
        m['Pages'][0]['Width'] += 1
    defect(wrong_size)

    def conflict(m, parts):
        page = m['Pages'][m['Tiles'][0]['Page']]
        pw, ph, rows = pixels(parts[page['File']])
        rows[1][0] = (201, 2, 3, 255)
        parts[page['File']] = write_png(pw, ph, rows)
        dup = dict(m['Tiles'][0])
        dup['Source'] = dict(dup['Source'])
        dup['Target'] = dict(dup['Target'])
        # One-pixel duplicate reading the conflicting texel for the same
        # logical pixel as the original fragment's corner.
        dup['Source']['W'] = dup['Source']['H'] = 1
        dup['Target']['W'] = dup['Target']['H'] = 1
        dup['Target']['X'], dup['Target']['Y'] = 0, 1
        m['Tiles'].insert(0, dup)
    defect(conflict)

    # Sampling through the service matches the legacy logical texture.
    gx, gy, gz = req['Grid']['Size']
    for face in range(6):
        for depth in range(gz):
            for u, v in ((0.0, 0.0), (0.37, 0.61), (0.5, 0.5), (1.0, 1.0)):
                got = sample_service(s, row['digest'], face, depth, u, v)
                want = bilinear_legacy(legacy_rgba, gx, gy, face, depth, u, v)
                if want['Weight'] == 0:
                    assert not got['Available'] and got['Weight'] == 0
                else:
                    assert got['Available']
                    for key in ('R', 'G', 'B', 'Weight'):
                        assert abs(got[key] - want[key]) < 1e-9, (face, depth, u, v, key, got, want)
    assert s.call(sample_path(row['digest'], 9, 0, 0, 0))[0] == 400
    assert s.call(sample_path(row['digest'], 0, 0, 2, 0))[0] == 400
    assert s.call('/v1/artifacts/missing/sample?face=0&depth=0&u=0&v=0')[0] == 404


def sample_path(digest, face, depth, u, v):
    return f'/v1/artifacts/{digest}/sample?face={face}&depth={depth}&u={u}&v={v}'


def sample_service(s, digest, face, depth, u, v):
    status, _, body = s.call(sample_path(digest, face, depth, u, v))
    assert status == 200, body
    return json.loads(body)


def bilinear_legacy(rgba, gx, gy, face, depth, u, v):
    fx, fy = u * (gx - 1), v * (gy - 1)
    x0, y0 = int(math.floor(fx)), int(math.floor(fy))
    x1, y1 = min(x0 + 1, gx - 1), min(y0 + 1, gy - 1)
    tx, ty = fx - x0, fy - y0
    ox, oy = face * gx, depth * gy
    total = r = g = b = 0.0
    for x, wx in ((x0, 1 - tx), (x1, tx)):
        for y, wy in ((y0, 1 - ty), (y1, ty)):
            pr, pg, pb, pa = rgba[oy + y][ox + x]
            if pa == 0:
                continue
            w = wx * wy
            total += w
            r += w * pr
            g += w * pg
            b += w * pb
    if total == 0:
        return {'Available': False, 'R': 0.0, 'G': 0.0, 'B': 0.0, 'Weight': 0.0}
    return {'Available': True, 'R': r / total, 'G': g / total, 'B': b / total, 'Weight': total}


if __name__ == '__main__':
    main()
