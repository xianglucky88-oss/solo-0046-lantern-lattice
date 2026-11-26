#!/usr/bin/env python3
"""Offline query engine for lantern coarse tiers.

Uses ONLY a coarse-tier zip (coarse.json): no lights, no fine data, no server
calls. It reproduces the bake's grid location, closed-box occupancy, tangent
blocking ray tests, visibility-filtered trilinear support and normalization.

Can be imported (CoarseVolume) or used as a CLI:

    python3 -B tools/coarse_query.py coarse.zip queries.json [fine.json]

queries.json is [[x,y,z], ...] or {"positions": [...]}. Prints results JSON.
"""

import base64
import json
import math
import struct
import sys
import zipfile


def _u(data, off, width):
    return int.from_bytes(data[off:off + width], "little", signed=False)


class CoarseVolume:
    def __init__(self, doc):
        self.doc = doc
        self.grid = doc["Grid"]
        self.origin = self.grid["Origin"]
        self.step = self.grid["Step"]
        self.size = self.grid["Size"]
        self.blocks = doc.get("Blocks") or []
        self.probes = self._runtime_probes(doc)

    @classmethod
    def from_zip(cls, path_or_bytes):
        if isinstance(path_or_bytes, (bytes, bytearray)):
            zf = zipfile.ZipFile(__import__("io").BytesIO(path_or_bytes))
        else:
            zf = zipfile.ZipFile(path_or_bytes)
        with zf:
            doc = json.loads(zf.read("coarse.json"))
        return cls(doc)

    def _index(self, x, y, z):
        sx, sy, _ = self.size
        return x + sx * (y + sy * z)

    def _position(self, x, y, z):
        return [self.origin[a] + (x, y, z)[a] * self.step for a in range(3)]

    def _runtime_probes(self, doc):
        total = self.size[0] * self.size[1] * self.size[2]
        probes = [None] * total
        for z in range(self.size[2]):
            for y in range(self.size[1]):
                for x in range(self.size[0]):
                    i = self._index(x, y, z)
                    p = self._position(x, y, z)
                    probes[i] = {
                        "position": p,
                        "valid": not any(inside(p, b) for b in self.blocks),
                        "lobes": [[0.0, 0.0, 0.0] for _ in range(6)],
                    }
        if doc.get("FaceCount", 0) == 0:
            return probes
        enc = doc["Encoding"]
        colors = base64.b64decode(enc.get("ColorsBase64", ""))
        lookup = base64.b64decode(enc.get("LookupBase64", ""))
        fcount = doc["FaceCount"]
        faces = [[0.0, 0.0, 0.0] for _ in range(fcount)]
        mode = enc["Mode"]
        if mode == "exact-float64-v1":
            for ch in range(3):
                for f in range(fcount):
                    off = 8 * (ch * fcount + f)
                    faces[f][ch] = struct.unpack_from("<d", colors, off)[0]
        else:
            widths = enc["CodeWidths"]
            scale = enc["Scale"]
            if mode == "direct-codes-v1":
                off = 0
                for ch in range(3):
                    w = widths[ch]
                    for f in range(fcount):
                        faces[f][ch] = _u(colors, off + f * w, w) * scale[ch]
                    off += fcount * w
            elif mode == "palette-codes-v1":
                iw = enc["IndexWidth"]
                kcount = len(colors) // sum(widths)
                palette = [[0.0, 0.0, 0.0] for _ in range(kcount)]
                off = 0
                planes = []
                for ch in range(3):
                    w = widths[ch]
                    plane = []
                    for _k in range(kcount):
                        plane.append(_u(colors, off, w))
                        off += w
                    planes.append(plane)
                for k in range(kcount):
                    for ch in range(3):
                        palette[k][ch] = planes[ch][k] * scale[ch]
                for f in range(fcount):
                    idx = _u(lookup, f * iw, iw)
                    faces[f] = list(palette[idx])
            else:
                raise ValueError("unknown coarse mode " + mode)
        f = 0
        for i in range(total):
            if not probes[i]["valid"]:
                continue
            for face in range(6):
                probes[i]["lobes"][face] = list(faces[f])
                f += 1
        return probes

    def locate(self, p):
        coords = []
        for a in range(3):
            at = (p[a] - self.origin[a]) / self.step
            top = float(self.size[a] - 1)
            if at < -1e-9 or at > top + 1e-9:
                return None
            at = max(0.0, min(top, at))
            lo = min(int(math.floor(at)), self.size[a] - 2)
            coords.append((lo, at - float(lo)))
        return coords

    def corners(self, cell):
        out = []
        for mask in range(8):
            w = 1.0
            ii = [0, 0, 0]
            for a, (lo, frac) in enumerate(cell):
                if mask & (1 << a):
                    ii[a] = lo + 1
                    w *= frac
                else:
                    ii[a] = lo
                    w *= 1.0 - frac
            if w > 0.0:
                out.append((self._index(*ii), w))
        return out

    def query(self, p, fine=None):
        """fine: optional dict {probe_index: 6x3 lobes}. All-or-nothing."""
        cell = self.locate(p)
        result = {"Position": list(p), "State": "outside", "Weight": 0.0,
                  "Irradiance": [[0.0, 0.0, 0.0] for _ in range(6)]}
        if cell is None:
            result["Tier"] = "none"
            return result
        if any(inside(p, b) for b in self.blocks):
            result["State"] = "solid"
            result["Tier"] = "none"
            return result
        used = []
        support = 0.0
        for idx, w in self.corners(cell):
            pr = self.probes[idx]
            if not pr["valid"]:
                continue
            if not visible(p, pr["position"], self.blocks):
                continue
            used.append((idx, w))
            support += w
        if not (support > 0.0):
            result["State"] = "unsupported"
            result["Tier"] = "none"
            return result
        all_fine = fine is not None and all(idx in fine for idx, _ in used)
        irr = [[0.0, 0.0, 0.0] for _ in range(6)]
        for idx, w in used:
            lobes = fine[idx] if all_fine else self.probes[idx]["lobes"]
            for face in range(6):
                for c in range(3):
                    irr[face][c] += lobes[face][c] * w
        inv = 1.0 / support
        for face in range(6):
            for c in range(3):
                irr[face][c] *= inv
        result.update(State="sampled", Weight=support, Irradiance=irr,
                      Tier="fine" if all_fine else "coarse",
                      RequiredFineProbes=[idx for idx, _ in used])
        return result


def inside(p, b):
    return all(b["Min"][a] <= p[a] <= b["Max"][a] for a in range(3))


def intersects(p, q, b):
    d = [q[a] - p[a] for a in range(3)]
    near, far = 0.0, 1.0
    for a in range(3):
        if d[a] == 0.0:
            if p[a] < b["Min"][a] or p[a] > b["Max"][a]:
                return False
            continue
        lo = (b["Min"][a] - p[a]) / d[a]
        hi = (b["Max"][a] - p[a]) / d[a]
        if lo > hi:
            lo, hi = hi, lo
        near = max(near, lo)
        far = min(far, hi)
        if near > far:
            return False
    return True


def visible(p, q, blocks):
    return not any(intersects(p, q, b) for b in blocks)


def load_fine(path_or_bytes, only=None):
    if isinstance(path_or_bytes, (bytes, bytearray)):
        zf = zipfile.ZipFile(__import__("io").BytesIO(path_or_bytes))
    else:
        zf = zipfile.ZipFile(path_or_bytes)
    with zf:
        doc = json.loads(zf.read("fine.json"))
    out = {}
    for pr in doc["Probes"]:
        if pr.get("Valid") and (only is None or pr["Index"] in only):
            out[pr["Index"]] = pr["Irradiance"]
    return out


def main(argv):
    if len(argv) < 3:
        print(__doc__)
        return 2
    vol = CoarseVolume.from_zip(argv[1])
    spec = json.load(open(argv[2], "rb"))
    positions = spec["positions"] if isinstance(spec, dict) else spec
    fine = load_fine(argv[3]) if len(argv) >= 4 else None
    rows = [vol.query(p, fine) for p in positions]
    json.dump(rows, sys.stdout, indent=1)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
