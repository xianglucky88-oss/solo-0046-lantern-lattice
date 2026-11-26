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
    if mode not in ('product', 'persistence', 'contract', 'resume', 'all'):
        raise SystemExit('mode: product | persistence | contract | resume | all')
    with tempfile.TemporaryDirectory(prefix='media-workflow-') as root:
        binary, data = Path(root)/'server', Path(root)/'artifacts'
        offline = Path(root)/'offline'
        subprocess.run(['go','build','-o',str(binary),'./cmd/server'], check=True)
        subprocess.run(['go','build','-o',str(offline),'./cmd/offline'], check=True)
        with Service(binary, data) as s:
            if mode in ('contract', 'all'):
                contract(s)
            if mode in ('product', 'all'):
                check_product(s)
            if mode in ('persistence', 'all'):
                check_persistence(s, data, binary)
            if mode in ('resume', 'all'):
                check_resume(s, data, binary, offline)
    print(mode + ': passed; temporary listener closed')


def check_persistence(s, data, binary):
    example = s.demo()
    with concurrent.futures.ThreadPoolExecutor(2) as pool:
        results = list(pool.map(s.post, [example, example]))
    assert [receipt(x) for x in results][0] == [receipt(x) for x in results][1]
    row = receipt(results[0]); stock = fetch(s, row)
    assert not list(data.glob('.staging-*'))
    assert s.call(row['artifact'], headers={'If-None-Match':'"' + row['digest'] + '"'})[0] == 304
    code, headers, prefix = s.call(row['artifact'], headers={'Range':'bytes=0-7'})
    assert code == 206 and prefix == stock[:8]
    with Service(binary, data) as restarted:
        assert fetch(restarted,row) == stock
        (data/row['digest']).write_bytes(b'corrupted')
        assert restarted.call(row['artifact'])[0] == 409


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


def full_bake_digest(s, scene):
    return receipt(s.post(scene))['digest']


def revisions(s, body):
    request = urllib.request.Request(s.url + '/v1/revisions',
                                     data=json.dumps(body).encode(),
                                     headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            return response.status, response.headers, response.read()
    except urllib.error.HTTPError as error:
        with error:
            return error.code, error.headers, error.read()


def resumable_scene():
    scene = {
        'Grid': {'Origin': [0, 0, 0], 'Step': 1, 'Size': [3, 2, 2]},
        'Exposure': 3,
        'Resumable': True,
        'Lights': [
            {'Position': [-1, 0, 0], 'RGB': [1, 0.55, 0.2], 'Radius': 6, 'Power': 4},
        ],
        'Blocks': [
            {'Min': [0.8, -0.2, -0.2], 'Max': [1.2, 0.2, 0.2]},
        ],
        'Samples': [[0, 0, 0], [1, 0, 0], [2, 0, 0]],
    }
    return scene


def check_resume(s, data, binary, offline):
    scene = resumable_scene()
    row = receipt(s.post(scene))
    assert row['version'].startswith('rv_') and len(row['version']) == 67
    assert row['lights'] == [{'id': 'light-0', 'position': 0}]
    root_zip = fetch(s, row)

    # A non-resumable build of the same scene keeps the original receipt shape.
    plain = dict(scene)
    plain['Resumable'] = False
    plain_row = receipt(s.post(plain))
    assert 'version' not in plain_row and plain_row['digest'] == row['digest']

    # Deterministic root identity across replays.
    assert receipt(s.post(scene))['version'] == row['version']

    # Continuation: add a second light and remove nothing; grid/blocks/samples
    # are preserved by the service.
    add_body = {
        'base': row['version'],
        'budget': 11,
        'operations': [
            {'action': 'add', 'id': 'light-1',
             'light': {'Position': [2, 0, 0], 'RGB': [0.2, 1.0, 0.4], 'Radius': 4, 'Power': 3}},
        ],
    }
    status, headers, body = revisions(s, add_body)
    assert status == 201, (status, body)
    v2 = json.loads(body)
    assert headers['Location'] == v2['artifact'] == '/v1/artifacts/' + v2['digest']
    assert v2['base'] == row['version']
    assert v2['lights'] == [
        {'id': 'light-0', 'position': 0},
        {'id': 'light-1', 'position': 1},
    ]
    assert len(v2['resolved']) == 10
    assert all(len(p) == 3 for p in v2['resolved'])
    # Probe (0,0,0) never receives energy from the new light (blocked in the
    # new state, beyond range in the old state), so only its occlusion count
    # changes: it is reported as occlusion-only and spends no budget.
    assert v2['occlusion_only'] == [[0, 0, 0]]
    assert v2['patch'] == '/v1/patches/' + v2['version']
    v2_zip = fetch(s, v2)

    # The new whole package must be byte-identical to a full bake with the
    # edited light list.
    edited = json.loads(json.dumps(scene))
    edited['Resumable'] = False
    edited['Lights'].append(add_body['operations'][0]['light'])
    assert full_bake_digest(s, edited) == v2['digest']

    # Replaying the same continuation returns the same version and delivery.
    status2, _, body2 = revisions(s, add_body)
    assert status2 == 200
    replay = json.loads(body2)
    assert replay['version'] == v2['version']
    assert replay['digest'] == v2['digest']
    assert replay['resolved'] == v2['resolved']
    assert replay['occlusion_only'] == v2['occlusion_only']
    assert fetch(s, replay) == v2_zip

    # Budget enforcement: the union exceeds a deliberately small budget and no
    # new version is created.
    over = dict(add_body, budget=9)
    assert revisions(s, over)[0] == 422
    missing = {'base': 'rv_' + '0' * 64, 'budget': 1, 'operations': []}
    assert revisions(s, missing)[0] == 404
    bad_action = dict(add_body, operations=[{'action': 'explode', 'id': 'light-9'}])
    assert revisions(s, bad_action)[0] == 422
    unknown_light = {'base': row['version'], 'budget': 1,
                     'operations': [{'action': 'remove', 'id': 'light-7'}]}
    assert revisions(s, unknown_light)[0] == 422
    duplicate = dict(add_body, operations=add_body['operations'] * 2)
    assert revisions(s, duplicate)[0] == 422
    assert s.call('/v1/revisions', b'{}', {'Content-Type': 'text/plain'})[0] == 415
    assert s.call('/v1/revisions', b'{"unknown": 1}', {'Content-Type': 'application/json'})[0] == 400
    assert revisions(s, {'base': row['version'], 'budget': -1, 'operations': []})[0] == 422
    assert revisions(s, {'base': row['version'], 'budget': 1, 'operations': []})[0] == 422

    # Chained continuation: remove a light, building on the successful v2.
    removal = {'base': v2['version'], 'budget': 11,
               'operations': [{'action': 'remove', 'id': 'light-0'}]}
    v3 = json.loads(revisions(s, removal)[2])
    assert v3['base'] == v2['version']
    assert v3['lights'] == [{'id': 'light-1', 'position': 0}]
    edited3 = json.loads(json.dumps(edited))
    edited3['Resumable'] = False
    edited3['Lights'].pop(0)
    assert full_bake_digest(s, edited3) == v3['digest']

    # Occlusion-only probes use a dedicated root: a short-radius light reaches
    # nobody, then a move behind the blocker raises occlusion counts at probes
    # where it still delivers no unoccluded energy. These probes consume no
    # linear re-solve budget.
    occ_scene = resumable_scene()
    occ_scene['Lights'].append(
        {'Position': [0.4, 0, 0], 'RGB': [0.2, 1.0, 0.4], 'Radius': 0.2, 'Power': 3})
    occ_root = receipt(s.post(occ_scene))
    behind = {
        'base': occ_root['version'], 'budget': 0,
        'operations': [{'action': 'change', 'id': 'light-1',
                        'light': {'Position': [1, 0, 0], 'RGB': [0.2, 1.0, 0.4],
                                  'Radius': 1.1, 'Power': 3}}],
    }
    status, _, body = revisions(s, behind)
    assert status == 201, (status, body)
    occ_v2 = json.loads(body)
    assert occ_v2.get('resolved', []) == []
    assert len(occ_v2['occlusion_only']) >= 1
    occ_edited = json.loads(json.dumps(occ_scene))
    occ_edited['Resumable'] = False
    occ_edited['Lights'][1] = behind['operations'][0]['light']
    assert full_bake_digest(s, occ_edited) == occ_v2['digest']

    # Distinct light states may bake to the same ZIP when a light contributes
    # zero energy everywhere.
    inert = {
        'base': row['version'], 'budget': 0,
        'operations': [{'action': 'add', 'id': 'light-9',
                        'light': {'Position': [99, 99, 99], 'RGB': [1, 1, 1],
                                  'Radius': 0.02, 'Power': 4}}],
    }
    v5 = json.loads(revisions(s, inert)[2])
    assert v5['digest'] == row['digest'] and v5['version'] != row['version']
    assert s.call(v5['artifact'])[0] == 200

    # The original download summary still addresses the original ZIP.
    assert fetch(s, row) == root_zip

    # Offline reconstruction with only patch + previous receipt + previous ZIP.
    work = Path(tempfile.mkdtemp(prefix='offline-apply-'))
    patch_v2 = work / 'patch-v2.json'
    receipt_v1 = work / 'receipt-v1.json'
    zip_v1 = work / 'v1.zip'
    code, _, patch_bytes = s.call(v2['patch'])
    assert code == 200
    patch_v2.write_bytes(patch_bytes)
    receipt_v1.write_text(json.dumps({'digest': row['digest'], 'version': row['version']}))
    zip_v1.write_bytes(root_zip)
    rebuilt = work / 'rebuilt-v2.zip'
    subprocess.run([str(offline), '-patch', str(patch_v2), '-receipt', str(receipt_v1),
                    '-zip', str(zip_v1), '-out', str(rebuilt)], check=True)
    assert rebuilt.read_bytes() == v2_zip

    # Offline refusal: mismatched old light-version identity.
    bad_receipt = work / 'bad-receipt.json'
    bad_receipt.write_text(json.dumps({'digest': row['digest'], 'version': 'rv_' + 'f' * 64}))
    reject = subprocess.run([str(offline), '-patch', str(patch_v2), '-receipt', str(bad_receipt),
                             '-zip', str(zip_v1), '-out', str(work / 'x.zip')])
    assert reject.returncode == 1 and not (work / 'x.zip').exists()

    # Offline refusal: mismatched old ZIP bytes.
    bad_zip = work / 'bad.zip'
    bad_zip.write_bytes(root_zip + b'tampered')
    reject = subprocess.run([str(offline), '-patch', str(patch_v2), '-receipt', str(receipt_v1),
                             '-zip', str(bad_zip), '-out', str(work / 'y.zip')])
    assert reject.returncode == 1 and not (work / 'y.zip').exists()

    # Chained offline apply: root -> v2 -> v3 using only each delivered patch.
    chain = [v2, v3]
    previous_zip, previous_receipt = root_zip, row
    for cur in chain:
        p = work / ('patch-' + cur['version'] + '.json')
        r = work / ('receipt-' + cur['version'] + '.json')
        z = work / (cur['version'] + '.zip')
        p.write_bytes(s.call(cur['patch'])[2])
        r.write_text(json.dumps({'digest': previous_receipt['digest'],
                                 'version': previous_receipt['version']}))
        z.write_bytes(previous_zip)
        out = work / (cur['version'] + '-out.zip')
        subprocess.run([str(offline), '-patch', str(p), '-receipt', str(r),
                        '-zip', str(z), '-out', str(out)], check=True)
        assert out.read_bytes() == fetch(s, cur)
        previous_zip, previous_receipt = out.read_bytes(), cur

    # The occlusion-only continuation is also reproducible offline.
    p = work / 'patch-occ.json'
    r = work / 'receipt-occ.json'
    z = work / 'occ-root.zip'
    p.write_bytes(s.call(occ_v2['patch'])[2])
    r.write_text(json.dumps({'digest': occ_root['digest'], 'version': occ_root['version']}))
    z.write_bytes(fetch(s, occ_root))
    out = work / 'occ-out.zip'
    subprocess.run([str(offline), '-patch', str(p), '-receipt', str(r),
                    '-zip', str(z), '-out', str(out)], check=True)
    assert out.read_bytes() == fetch(s, occ_v2)

    # Patch endpoint rejects unknown ids; damaged base blocks continuation but
    # leaves existing versions retrievable.
    assert s.call('/v1/patches/rv_' + 'a' * 64)[0] == 404
    record = data / ('version-' + v3['version'] + '.json')
    backup = record.read_bytes()
    record.write_bytes(b'{broken')
    assert revisions(s, {'base': v3['version'], 'budget': 1, 'operations': []})[0] == 409
    assert s.call(v3['artifact'])[0] == 200
    record.write_bytes(backup)

    # Versions survive a restart and can continue again.
    with Service(binary, data) as restarted:
        assert fetch(restarted, v3) == fetch(s, v3)
        again = {'base': v3['version'], 'budget': 11,
                 'operations': [{'action': 'change', 'id': 'light-1',
                                 'light': {'Position': [0, 1, 0], 'RGB': [0.1, 0.2, 0.9],
                                           'Radius': 5, 'Power': 2}}]}
        v6 = json.loads(revisions(restarted, again)[2])
        edited6 = json.loads(json.dumps(edited3))
        edited6['Resumable'] = False
        edited6['Lights'][0] = again['operations'][0]['light']
        assert full_bake_digest(restarted, edited6) == v6['digest']



if __name__ == '__main__':
    main()
