import concurrent.futures
import hashlib
import json
import os
import shutil
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


def ensure_go_on_path():
    if shutil.which('go'):
        return
    candidates = (
        '/usr/local/go/bin',
        '/opt/go/bin',
        '/tmp/go-toolchain/go/bin',
        str(Path.home() / '.local' / 'go' / 'bin'),
    )
    for directory in candidates:
        if (Path(directory) / 'go').exists():
            os.environ['PATH'] = directory + os.pathsep + os.environ.get('PATH', '')
            return


def main():
    ensure_go_on_path()
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


if __name__ == '__main__':
    main()
