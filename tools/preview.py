#!/usr/bin/env python3
"""Artifact-only lantern preview tool.

Reads a downloaded probe-volume.zip once and relights it without the original
request or scene:

    python3 -I tools/preview.py probe-volume.zip \
        --sky 0.25 --lights 1.0 --at 0.4,0,0 -o out/

The tool always scales the *original* full-precision sky and light groups
embedded in the artifact, so moving a slider repeatedly never feeds a
quantized result back in.  It writes a relit probes.png and samples.json and
prints a JSON report (including a base64 PNG) on stdout.

Many flag/key spellings are accepted because preview clients use different
vocabularies.  The module also exposes relight_archive()/sample_positions() for
in-process use.
"""

import argparse
import base64
import json
import math
import os
import struct
import sys
import zlib
import zipfile

AXES = ["+X", "-X", "+Y", "-Y", "+Z", "-Z"]


# ---------------------------------------------------------------- geometry

def inside(p, b):
    return all(b["Min"][a] <= p[a] <= b["Max"][a] for a in range(3))


def occupied(p, blocks):
    return any(inside(p, b) for b in blocks)


def intersects(p, q, b):
    d = [q[a] - p[a] for a in range(3)]
    near, far = 0.0, 1.0
    for a in range(3):
        if d[a] == 0:
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


def locate(g, p):
    spans = []
    for a in range(3):
        at = (p[a] - g["Origin"][a]) / g["Step"]
        top = float(g["Size"][a] - 1)
        if at < -1e-9 or at > top + 1e-9:
            return None, False
        at = max(0.0, min(top, at))
        lo = min(int(math.floor(at)), g["Size"][a] - 2)
        spans.append((lo, at - lo))
    return spans, True


def corners(g, cell):
    out = []
    for mask in range(8):
        w = 1.0
        idx = [0, 0, 0]
        for a, (lo, frac) in enumerate(cell):
            if mask & (1 << a):
                idx[a] = lo + 1
                w *= frac
            else:
                idx[a] = lo
                w *= 1 - frac
        if w > 0:
            index = idx[0] + g["Size"][0] * (idx[1] + g["Size"][1] * idx[2])
            out.append((index, w))
    return out


# ---------------------------------------------------------------- artifact

def _first(d, *keys):
    low = {_norm(k): v for k, v in d.items()}
    for k in keys:
        if _norm(k) in low:
            return low[_norm(k)]
    return None


def _norm(k):
    return "".join(ch.lower() for ch in str(k) if ch not in " _-.")


def load_archive(path):
    with zipfile.ZipFile(path) as z:
        layout = json.loads(z.read("layout.json").decode("utf-8"))
        png = z.read("probes.png") if "probes.png" in z.namelist() else None
    return from_layout(layout), png


def from_layout(layout):
    basis = None
    for k in ("Preview", "Relight", "Lighting", "Basis", "Rig", "Base"):
        if isinstance(layout.get(k), dict) and layout[k].get("Probes"):
            basis = layout[k]
            break
    if basis is None and layout.get("Probes"):
        basis = layout
    if basis is None:
        basis = layout  # legacy envelope: Grid/Exposure/Blocks only
    g = basis["Grid"]
    probes = [None] * (g["Size"][0] * g["Size"][1] * g["Size"][2])
    for rec in basis.get("Probes", []):
        idx = rec.get("Index")
        if idx is None:
            idx = rec["X"] + g["Size"][0] * (rec["Y"] + g["Size"][1] * rec["Z"])
        light = rec.get("Light")
        if light is None or _zero(light):
            light = _first(rec, "Lights", "Lamp", "Lamps", "Direct") or _zero_lobes()
        sky = rec.get("Sky")
        if sky is None or _zero(sky):
            sky = _first(rec, "SkyLight", "Ambient") or _zero_lobes()
        pos = rec.get("Position")
        if not pos:
            pos = [
                g["Origin"][0] + rec.get("X", 0) * g["Step"],
                g["Origin"][1] + rec.get("Y", 0) * g["Step"],
                g["Origin"][2] + rec.get("Z", 0) * g["Step"],
            ]
        probes[idx] = {
            "valid": bool(rec.get("Valid", False)),
            "position": pos,
            "light": light,
            "sky": sky,
        }
    for i, pr in enumerate(probes):
        if pr is None:
            probes[i] = {"valid": False, "position": [0, 0, 0],
                         "light": _zero_lobes(), "sky": _zero_lobes()}
    return {
        "grid": g,
        "exposure": basis.get("Exposure", layout.get("Exposure", 1.0)),
        "blocks": basis.get("Blocks", layout.get("Blocks", [])) or [],
        "probes": probes,
    }


def _zero_lobes():
    return [[0.0, 0.0, 0.0] for _ in range(6)]


def _zero(lobes):
    return all(all(c == 0 for c in l) for l in lobes)


# ---------------------------------------------------------------- relight

def relit_lobes(art, sky_scale, light_scale):
    sky_scale = _clamp(sky_scale)
    light_scale = _clamp(light_scale)
    out = []
    for pr in art["probes"]:
        lobes = []
        for f in range(6):
            lobes.append([
                light_scale * pr["light"][f][c] + sky_scale * pr["sky"][f][c]
                for c in range(3)
            ])
        out.append(lobes)
    return out


def _clamp(v):
    try:
        v = float(v)
    except (TypeError, ValueError):
        return 0.0
    if math.isnan(v):
        return 0.0
    return max(0.0, min(1.0, v))


def sample_positions(art, positions, sky_scale=1.0, light_scale=1.0):
    g, blocks = art["grid"], art["blocks"]
    lobes = relit_lobes(art, sky_scale, light_scale)
    results = []
    for p in positions:
        s = {"Position": p, "State": "outside", "Weight": 0.0,
             "Irradiance": _zero_lobes()}
        cell, ok = locate(g, p)
        if not ok:
            results.append(s)
            continue
        if occupied(p, blocks):
            s["State"] = "solid"
            results.append(s)
            continue
        s["State"] = "unsupported"
        wsum = 0.0
        acc = _zero_lobes()
        for idx, w in corners(g, cell):
            pr = art["probes"][idx]
            if not pr["valid"] or not visible(p, pr["position"], blocks):
                continue
            wsum += w
            for f in range(6):
                for c in range(3):
                    acc[f][c] += w * lobes[idx][f][c]
        if wsum > 0:
            s["State"] = "sampled"
            s["Weight"] = wsum
            s["Irradiance"] = [[acc[f][c] / wsum for c in range(3)] for f in range(6)]
        results.append(s)
    return results


def quantize(energy, exposure):
    m = -math.expm1(-energy * exposure)
    if m >= 1:
        return 255
    return int(math.floor(255 * m + 0.5))


def encode_png(width, height, rgba_rows):
    raw = b"".join(b"\x00" + bytes(row) for row in rgba_rows)

    def chunk(kind, data):
        return (struct.pack(">I", len(data)) + kind + data
                + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF))

    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr)
            + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def render_png(art, sky_scale, light_scale):
    g = art["grid"]
    w, h = g["Size"][0] * 6, g["Size"][1] * g["Size"][2]
    lobes = relit_lobes(art, sky_scale, light_scale)
    rows = [bytearray(4 * w) for _ in range(h)]
    for z in range(g["Size"][2]):
        for y in range(g["Size"][1]):
            for x in range(g["Size"][0]):
                idx = x + g["Size"][0] * (y + g["Size"][1] * z)
                pr = art["probes"][idx]
                if not pr["valid"]:
                    continue
                for face in range(6):
                    px = x + face * g["Size"][0]
                    py = y + z * g["Size"][1]
                    e = lobes[idx][face]
                    o = 4 * px
                    row = rows[py]
                    row[o] = quantize(e[0], art["exposure"])
                    row[o + 1] = quantize(e[1], art["exposure"])
                    row[o + 2] = quantize(e[2], art["exposure"])
                    row[o + 3] = 255
    return encode_png(w, h, [bytes(r) for r in rows]), w, h


def relight_archive(path, sky=1.0, lights=1.0, positions=()):
    art, _ = load_archive(path)
    png, w, h = render_png(art, sky, lights)
    samples = sample_positions(art, list(positions), sky, lights) if positions else []
    return {
        "png": png, "width": w, "height": h, "samples": samples,
        "exposure": art["exposure"], "sky": _clamp(sky), "lights": _clamp(lights),
    }


# ---------------------------------------------------------------- CLI

SCALE_FLAGS = {
    "sky": ("--sky", "--sky-scale", "--sky-multiplier", "--sky-factor",
            "--sky-gain", "--sky-intensity", "--ambient", "--ambient-scale"),
    "lights": ("--lights", "--light", "--lamps", "--lamp", "--light-scale",
               "--lights-scale", "--lamp-scale", "--light-multiplier",
               "--lights-multiplier", "--dimmer", "--interior"),
}


def _parse_coord(s):
    try:
        if s.strip().startswith("["):
            v = json.loads(s)
            return [float(v[0]), float(v[1]), float(v[2])]
        parts = s.replace(";", ",").split(",")
        if len(parts) == 3:
            return [float(x) for x in parts]
    except Exception:
        return None
    return None


def main(argv=None):
    ap = argparse.ArgumentParser(add_help=True)
    ap.add_argument("archive", nargs="?")
    ap.add_argument("kv", nargs="*", help="optional sky=0.5 lights=1 tokens")
    for flag in sorted({f for flags in SCALE_FLAGS.values() for f in flags}):
        ap.add_argument(flag, dest=_dest(flag), type=str, default=None)
    ap.add_argument("--at", "--position", "--pos", action="append", default=[])
    ap.add_argument("--positions", default=None)
    ap.add_argument("--positions-file", default=None)
    ap.add_argument("-o", "--out", "--out-dir", dest="outdir", default=None)
    ap.add_argument("--png", dest="png_path", default=None)
    ap.add_argument("--json", dest="json_path", default=None)
    ap.add_argument("--changes", default=None)
    ap.add_argument("--set", dest="sets", action="append", default=[])
    args = ap.parse_args(argv)

    sky, lights = 1.0, 1.0
    for group, flags in SCALE_FLAGS.items():
        val = None
        ns = vars(args)
        for f in flags:
            v = ns.get(_dest(f).replace("-", "_"))
            if v is not None:
                val = v
                break
        if val is not None:
            parsed = _parse_scale_value(val)
            if group == "sky":
                sky = parsed
            else:
                lights = parsed

    # --changes JSON and --set sky=0.2 / positional sky=0.2 tokens.
    changes = {}
    if args.changes:
        with open(args.changes, "r", encoding="utf-8") as fh:
            changes = json.load(fh)
    for item in args.sets + list(args.kv or []):
        if item.startswith("--"):
            continue
        if "=" in item:
            k, v = item.split("=", 1)
            changes[k.strip()] = _parse_scale_value(v)
    for k, v in changes.items():
        nk = _norm(k)
        if nk in ("sky", "skyscale", "skymultiplier", "ambient"):
            sky = _parse_scale_value(v)
        elif nk in ("lights", "light", "lamps", "lightscale", "lightmultiplier"):
            lights = _parse_scale_value(v)

    positions = []
    for s in args.at:
        p = _parse_coord(s)
        if p:
            positions.append(p)
    if args.positions:
        node = json.loads(args.positions)
        positions.extend(_coords_from(node))
    if args.positions_file:
        with open(args.positions_file, "r", encoding="utf-8") as fh:
            positions.extend(_coords_from(json.load(fh)))

    if not args.archive:
        ap.error("archive zip path required")
    res = relight_archive(args.archive, sky, lights, positions)
    b64 = base64.b64encode(res["png"]).decode("ascii")
    report = {
        "ok": True,
        "Schema": "lantern-preview/v1",
        "Exposure": res["exposure"],
        "Width": res["width"], "Height": res["height"],
        "SkyScale": res["sky"], "LightScale": res["lights"],
        "Sky": res["sky"], "Lights": res["lights"],
        "PNG": b64, "png_base64": b64,
        "Samples": res["samples"],
    }

    if args.outdir:
        os.makedirs(args.outdir, exist_ok=True)
        with open(os.path.join(args.outdir, "probes.png"), "wb") as fh:
            fh.write(res["png"])
        with open(os.path.join(args.outdir, "samples.json"), "w", encoding="utf-8") as fh:
            json.dump({"Samples": res["samples"]}, fh, indent=2)
    if args.png_path:
        with open(args.png_path, "wb") as fh:
            fh.write(res["png"])
    if args.json_path:
        with open(args.json_path, "w", encoding="utf-8") as fh:
            json.dump(report, fh, indent=2)

    sys.stdout.write(json.dumps(report))
    return 0


def _dest(flag):
    return flag.lstrip("-").replace("-", "_")


def _parse_scale_value(v):
    if isinstance(v, (int, float)):
        return _clamp(v)
    if isinstance(v, dict):
        for k in ("scale", "Scale", "multiplier", "value"):
            if k in v:
                return _parse_scale_value(v[k])
    if isinstance(v, list) and v:
        return _clamp(v[0])
    return _clamp(str(v).strip())


def _coords_from(node):
    out = []
    if isinstance(node, list):
        if len(node) == 3 and all(isinstance(x, (int, float)) for x in node):
            return [node]
        for item in node:
            if isinstance(item, list) and len(item) == 3:
                out.append(item)
            elif isinstance(item, dict) and "Position" in item:
                out.append(item["Position"])
    return out


if __name__ == "__main__":
    raise SystemExit(main())
