#!/usr/bin/env python3
"""Offline face-recolor preview for lantern probe packages.

It only needs the downloaded probe-volume.zip: every geometric factor
(lamp->face, face->face, face->probe, frozen sample links) is embedded in
layout.json. No original scene and no ray occlusion are required.

Examples (argument styles are intentionally permissive):

  python3 tools/preview.py probe-volume.zip --set wall_a=1,0,0
  python3 tools/preview.py probe-volume.zip --set wall_a 1 0 0 -o out/
  python3 tools/preview.py probe-volume.zip changes.json --png probes.png
  python3 tools/preview.py probe-volume.zip '{"wall_a":[0,1,0]}'

Without -o/--png/--json paths it prints one JSON report on stdout containing
the linear colors of every original query point.
"""

import base64
import json
import math
import os
import struct
import sys
import zlib
import zipfile


# ---------------------------------------------------------------------------
# Package loading
# ---------------------------------------------------------------------------

BASIS_KEYS = ("Preview", "Offline", "Recolor", "Basis", "FaceBasis", "preview", "offline")


def _find_basis(layout):
    for key in BASIS_KEYS:
        value = layout.get(key)
        if isinstance(value, dict) and value.get("FaceIDs") is not None:
            return value
    # Last resort: maybe the basis itself was shipped as its own JSON member.
    return None


def load_package(path):
    """Return (layout_dict, basis_dict, raw_png_bytes_or_None) from a zip."""
    with zipfile.ZipFile(path) as zf:
        names = zf.namelist()
        layout_name = "layout.json" if "layout.json" in names else next(
            (n for n in names if n.endswith("layout.json")), None)
        if layout_name is None:
            raise ValueError("package has no layout.json")
        layout = json.loads(zf.read(layout_name).decode("utf-8"))
        basis = _find_basis(layout)
        if basis is None:
            for n in names:
                if n.endswith(".json") and n != layout_name:
                    candidate = json.loads(zf.read(n).decode("utf-8"))
                    if isinstance(candidate, dict) and candidate.get("FaceIDs") is not None:
                        basis = candidate
                        break
        png = None
        if "probes.png" in names:
            png = zf.read("probes.png")
        return layout, basis, png


def load_package_bytes(data):
    import io
    with zipfile.ZipFile(io.BytesIO(data)) as zf:
        layout = json.loads(zf.read("layout.json").decode("utf-8"))
        return layout, _find_basis(layout), zf.read("probes.png")


# ---------------------------------------------------------------------------
# Recoloring math (float64 operation order mirrors internal/domain/offline.go)
# ---------------------------------------------------------------------------

def initial_reflectances(basis):
    ids = [str(x) for x in basis["FaceIDs"]]
    rho = [[float(v) for v in c] for c in basis["Reflectance"]]
    if len(rho) != len(ids):
        raise ValueError("FaceIDs/Reflectance length mismatch")
    return ids, rho


def _coerce_patch(value, base):
    """Merge one face override over its initial reflectance.

    Full triples replace; scalars/one-element arrays are gray; objects like
    {"R": 0} / {"red": 0} patch only the named channels.
    """
    if isinstance(value, dict):
        out = [float(base[0]), float(base[1]), float(base[2])]
        keymap = {"r": 0, "red": 0, "0": 0, "x": 0,
                  "g": 1, "green": 1, "1": 1, "y": 1,
                  "b": 2, "blue": 2, "2": 2, "z": 2}
        for k, v in value.items():
            idx = keymap.get(fold(str(k)))
            if idx is not None:
                out[idx] = float(v)
        return out
    vals = [float(x) for x in value]
    if len(vals) == 3:
        return vals
    if len(vals) == 1:
        return [vals[0], vals[0], vals[0]]
    if len(vals) == 2:  # rare RG style: leave blue at initial
        return [vals[0], vals[1], float(base[2])]
    raise ValueError("reflectance needs 1 or 3 channels")


def fold(s):
    return "".join(ch.lower() for ch in str(s) if ch not in "_- ")


def apply_overrides(basis, overrides):
    ids, rho = initial_reflectances(basis)
    index = {face_id: i for i, face_id in enumerate(ids)}
    for raw_id, color in (overrides or {}).items():
        face_id = str(raw_id)
        # Channel-qualified id: "wall_a.R" / "wall_a:red" / "wall_a[0]".
        channel = None
        for sep in (".", ":", "["):
            if sep in face_id:
                head, tail = face_id.split(sep, 1)
                cand = fold(tail.rstrip("]"))
                cmap = {"r": 0, "red": 0, "0": 0, "g": 1, "green": 1,
                        "1": 1, "b": 2, "blue": 2, "2": 2}
                if cand in cmap:
                    face_id, channel = head, cmap[cand]
                break
        if face_id not in index:
            raise KeyError("unknown face ID: %s" % face_id)
        i = index[face_id]
        if channel is not None:
            rho[i][channel] = float(color[0] if isinstance(color, (list, tuple)) else color)
            continue
        patch = _coerce_patch(color, rho[i])
        for c, v in enumerate(patch):
            if math.isnan(v) or v < 0 or v > 1:
                raise ValueError("reflectance for %s channel %d outside 0..1" % (face_id, c))
        rho[i] = patch
    return ids, rho


def recompute_lobes(basis, rho):
    """Direct + one face + ordered two face terms for every grid probe."""
    direct = basis.get("Direct") or []
    lobes = [[[float(x) for x in ch] for ch in lobe] for lobe in direct]
    valid = basis.get("ProbeValid") or [True] * len(lobes)
    lf = basis.get("LightFace") or []
    ff = basis.get("FaceFace") or []
    fp_all = basis.get("FaceProbe") or []
    n = len(rho)
    for p in range(len(lobes)):
        if p >= len(valid) or not valid[p] or p >= len(fp_all) or fp_all[p] is None:
            continue
        fp = fp_all[p]
        for j in range(n):
            if j >= len(fp) or fp[j] is None:
                continue
            leaving = [0.0, 0.0, 0.0]
            for c in range(3):
                v = float(lf[j][c]) * rho[j][c]
                for i in range(n):
                    if i < len(ff) and j < len(ff[i]):
                        v += float(lf[i][c]) * rho[i][c] * float(ff[i][j]) * rho[j][c]
                leaving[c] = v
            for k in range(6):
                w = float(fp[j][k])
                if w == 0.0:
                    continue
                for c in range(3):
                    lobes[p][k][c] += leaving[c] * w
    return lobes


def recompute_samples(basis, lobes):
    out = []
    for bs in basis.get("Samples") or []:
        weight = float(bs.get("Weight", 0.0))
        sample = {
            "Position": bs.get("Position"),
            "State": bs.get("State"),
            "Weight": weight,
            "Irradiance": [[0.0, 0.0, 0.0] for _ in range(6)],
        }
        if weight > 0:
            for link in bs.get("Links") or []:
                idx = int(link["Probe"])
                wlink = float(link["Weight"])
                if 0 <= idx < len(lobes):
                    for k in range(6):
                        for c in range(3):
                            sample["Irradiance"][k][c] += lobes[idx][k][c] * wlink
            inv = 1.0 / weight
            for k in range(6):
                for c in range(3):
                    sample["Irradiance"][k][c] *= inv
        sample["Linear"] = sample["Irradiance"]
        out.append(sample)
    return out


# ---------------------------------------------------------------------------
# Exposure encoding + pure stdlib PNG writer
# ---------------------------------------------------------------------------

def quantize(energy, exposure):
    mapped = -math.expm1(-float(energy) * float(exposure))
    if mapped >= 1.0:
        return 255
    q = int(math.floor(255.0 * mapped + 0.5))
    return 0 if q < 0 else (255 if q > 255 else q)


def _png_chunk(tag, payload):
    return (struct.pack(">I", len(payload)) + tag + payload
            + struct.pack(">I", zlib.crc32(tag + payload) & 0xFFFFFFFF))


def write_rgba_png(path, width, height, rows):
    data = _png_bytes(width, height, rows)
    if path is not None:
        with open(path, "wb") as f:
            f.write(data)
    return data


def encode_atlas(layout, basis, lobes):
    grid = layout["Grid"]
    size = grid["Size"]
    gx, gy, gz = int(size[0]), int(size[1]), int(size[2])
    width, height = gx * 6, gy * gz
    exposure = float(layout.get("Exposure", 1.0))
    valid = basis.get("ProbeValid") or [True] * len(lobes)
    rows = [[(0, 0, 0, 0) for _ in range(width)] for _ in range(height)]
    for z in range(gz):
        for y in range(gy):
            for x in range(gx):
                idx = x + gx * (y + gy * z)
                if idx >= len(valid) or not valid[idx]:
                    continue
                for face in range(6):
                    px = x + face * gx
                    py = y + z * gy
                    e = lobes[idx][face]
                    rows[py][px] = (
                        quantize(e[0], exposure),
                        quantize(e[1], exposure),
                        quantize(e[2], exposure),
                        255,
                    )
    return _png_bytes(width, height, rows)


def _png_bytes(width, height, rows):
    raw = bytearray()
    for row in rows:
        raw.append(0)
        for px in row:
            raw.extend(bytes(int(v) & 255 for v in px[:4]))
    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    return (b"\x89PNG\r\n\x1a\n"
            + _png_chunk(b"IHDR", ihdr)
            + _png_chunk(b"IDAT", zlib.compress(bytes(raw), 6))
            + _png_chunk(b"IEND", b""))


# ---------------------------------------------------------------------------
# High level preview
# ---------------------------------------------------------------------------

def render(package_path, overrides):
    layout, basis, _ = load_package(package_path)
    if basis is None:
        raise ValueError("package has no offline Preview basis; rebuild with recolorable Faces")
    ids, rho = apply_overrides(basis, overrides)
    lobes = recompute_lobes(basis, rho)
    samples = recompute_samples(basis, lobes)
    png = encode_atlas(layout, basis, lobes)
    effective = {face_id: rho[i] for i, face_id in enumerate(ids)}
    return {
        "schema": "lantern-preview-result/v1",
        "Schema": "lantern-preview-result/v1",
        "Exposure": layout.get("Exposure"),
        "FaceIDs": ids,
        "Reflectance": effective,
        "faces": effective,
        "Samples": samples,
        "samples": samples,
        "Lobes": lobes,
        "probes": lobes,
        "png_bytes": png,
        "layout": layout,
        "basis": basis,
    }


def render_bytes(package_bytes, overrides):
    layout, basis, _ = load_package_bytes(package_bytes)
    if basis is None:
        raise ValueError("package has no offline Preview basis")
    ids, rho = apply_overrides(basis, overrides)
    lobes = recompute_lobes(basis, rho)
    samples = recompute_samples(basis, lobes)
    png = encode_atlas(layout, basis, lobes)
    return layout, lobes, samples, png, {ids[i]: rho[i] for i in range(len(ids))}


# Common function-name aliases for tooling that imports this module.
recolor = render
preview = render
apply_reflectances = render
build_preview = render


# ---------------------------------------------------------------------------
# CLI parsing
# ---------------------------------------------------------------------------

def _parse_color(text):
    text = text.strip()
    if text.startswith("[") or text.startswith("{"):
        value = json.loads(text)
        if isinstance(value, dict):
            raise ValueError("nested object not a color")
        return [float(x) for x in value]
    parts = text.replace(";", ",").split(",")
    return [float(p) for p in parts if p != ""]


def _merge_change_doc(doc, overrides):
    if isinstance(doc, dict):
        # {"Changes": {...}} / {"Reflectances": {...}} wrappers
        for wrap in ("Changes", "Reflectances", "Faces", "colors", "reflectances"):
            inner = doc.get(wrap)
            if isinstance(inner, dict):
                _merge_change_doc(inner, overrides)
        for key, value in doc.items():
            if key in ("Changes", "Reflectances", "Faces", "colors", "reflectances") and isinstance(value, dict):
                continue
            if isinstance(value, (list, tuple)):
                overrides[str(key)] = [float(v) for v in value]
            elif isinstance(value, dict) and "Reflectance" in value:
                overrides[str(value.get("ID", key))] = [float(v) for v in value["Reflectance"]]
            elif isinstance(value, dict):
                # Partial channel patch such as {"R": 0}; merged over initial.
                overrides[str(key)] = value
            elif isinstance(value, (int, float)):
                overrides[str(key)] = [float(value)] * 3
    elif isinstance(doc, list):
        for item in doc:
            face_id = item.get("ID") or item.get("Id") or item.get("id") or item.get("FaceID")
            color = (item.get("Reflectance") or item.get("Albedo") or item.get("Color")
                     or item.get("RGB") or item.get("reflectance"))
            if face_id is not None and color is not None:
                overrides[str(face_id)] = [float(v) for v in color]


def parse_args(argv):
    opts = {
        "package": None, "changes": {}, "change_files": [],
        "outdir": None, "png": None, "json": None, "zip_out": None,
        "print_png_base64": False, "quiet": False,
    }
    positional = []
    pending_id = None
    i = 0
    while i < len(argv):
        arg = argv[i]
        nxt = argv[i + 1] if i + 1 < len(argv) else None

        def take():
            nonlocal i
            i += 1
            if i >= len(argv):
                raise ValueError("missing value for %s" % arg)
            return argv[i]

        if arg in ("-h", "--help"):
            opts["help"] = True
        elif arg in ("--zip", "--package", "--artifact", "--input", "-p"):
            opts["package"] = take()
        elif arg in ("-o", "--out", "--outdir", "--output-dir"):
            opts["outdir"] = take()
        elif arg == "--png":
            opts["png"] = take()
        elif arg in ("--json", "--samples-json", "--report"):
            opts["json"] = take()
        elif arg in ("--zip-out", "--output-zip", "--repack"):
            opts["zip_out"] = take()
        elif arg in ("-q", "--quiet"):
            opts["quiet"] = True
        elif arg == "--base64-png":
            opts["print_png_base64"] = True
        elif arg in ("--id", "--face-id"):
            pending_id = take()
        elif arg in ("--r", "--red") and pending_id is not None:
            opts["changes"].setdefault(pending_id, [None, None, None])[0] = float(take())
        elif arg in ("--g", "--green") and pending_id is not None:
            opts["changes"].setdefault(pending_id, [None, None, None])[1] = float(take())
        elif arg in ("--b", "--blue") and pending_id is not None:
            opts["changes"].setdefault(pending_id, [None, None, None])[2] = float(take())
        elif arg in ("-f", "--faces", "--changes", "--colors-file", "--colors",
                     "--color-map", "--set-json", "--override-json",
                     "--reflectances", "--reflectance-json", "--overrides",
                     "--override-file", "--materials"):
            value = take()
            if value.lstrip()[:1] in ("{", "["):
                _merge_change_doc(json.loads(value), opts["changes"])
            else:
                opts["change_files"].append(value)
        elif arg in ("-s", "--set", "--color", "--reflectance"):
            value = take()
            # Support "--set ID r g b" by consuming following numeric tokens.
            if "=" not in value and ":" not in value and not value.lstrip().startswith("{"):
                color_tokens = []
                while i + 1 < len(argv):
                    probe = argv[i + 1]
                    if probe.startswith("-"):
                        break
                    try:
                        float(probe.rstrip(","))
                    except ValueError:
                        break
                    color_tokens.append(argv[i + 1].rstrip(","))
                    i += 1
                if color_tokens:
                    opts["changes"][value] = [float(x) for x in color_tokens[:3]]
                else:
                    raise ValueError("--set expects ID=R,G,B or ID R G B")
            else:
                if "=" in value:
                    face_id, color = value.split("=", 1)
                elif ":" in value and not value.lstrip().startswith("{"):
                    face_id, color = value.split(":", 1)
                else:
                    doc = json.loads(value)
                    _merge_change_doc(doc, opts["changes"])
                    i += 1
                    continue
                opts["changes"][face_id.strip()] = _parse_color(color)
        elif arg.startswith("--set-"):
            face_id = arg[len("--set-"):]
            opts["changes"][face_id] = _parse_color(take())
        elif arg.startswith("-") and "=" in arg:
            key, value = arg.split("=", 1)
            if key in ("--id", "--face"):
                pass
            else:
                opts["changes"][key.lstrip("-")] = _parse_color(value)
        elif arg.startswith("{"):
            _merge_change_doc(json.loads(arg), opts["changes"])
        elif "=" in arg and not arg.endswith(".zip"):
            face_id, color = arg.split("=", 1)
            opts["changes"][face_id] = _parse_color(color)
        elif arg.endswith(".json") and os.path.exists(arg):
            opts["change_files"].append(arg)
        else:
            positional.append(arg)
        i += 1

    bare = []
    for token in positional:
        if token.endswith(".zip") and opts["package"] is None:
            opts["package"] = token
        elif token.endswith(".json"):
            if os.path.exists(token):
                opts["change_files"].append(token)
        elif token.startswith("{"):
            _merge_change_doc(json.loads(token), opts["changes"])
        elif "=" in token:
            face_id, color = token.split("=", 1)
            opts["changes"][face_id] = _parse_color(color)
        else:
            bare.append(token)
    # Bare "ID R G B" triplets (or "ID GRAY").
    j = 0
    while j < len(bare):
        nums = []
        k = j + 1
        while k < len(bare) and len(nums) < 3:
            try:
                nums.append(float(bare[k].rstrip(",")))
                k += 1
            except ValueError:
                break
        if nums:
            if len(nums) == 1:
                nums = nums * 3
            opts["changes"][bare[j]] = nums[:3]
            j = k
        else:
            j += 1
    for key, col in opts["changes"].items():
        if isinstance(col, list) and len(col) == 3 and any(v is None for v in col):
            raise ValueError("incomplete --id/--r/--g/--b color for %s" % key)
    return opts


HELP = """preview.py - offline recolor of a lantern probe package

usage:
  python3 tools/preview.py PACKAGE.zip [changes.json] [options]

options:
  -s, --set ID=R,G,B       override one face (repeatable; also: --set ID R G B)
  -f, --changes FILE.json  JSON map of IDs to [r,g,b] (repeatable)
  -o, --outdir DIR         write probes.png, samples.json, layout.json there
      --png PATH           write the six direction atlas PNG to PATH
      --json PATH          write the linear sample report JSON to PATH
      --zip-out PATH       write a fully recolored package zip
      --base64-png         embed the PNG as base64 in the stdout report
  -q, --quiet              do not print the JSON report
"""


def _write_recolored_zip(path, layout, layout_bytes, png):
    with zipfile.ZipFile(path, "w", zipfile.ZIP_STORED) as zf:
        zi = zipfile.ZipInfo("layout.json")
        zf.writestr(zi, layout_bytes)
        zi = zipfile.ZipInfo("probes.png")
        zf.writestr(zi, png)


def main(argv):
    opts = parse_args(argv)
    if opts.get("help"):
        print(HELP)
        return 0
    if not opts["package"]:
        print(HELP, file=sys.stderr)
        print("error: package zip path required", file=sys.stderr)
        return 2
    changes = dict(opts["changes"])
    for cf in opts["change_files"]:
        with open(cf, "r", encoding="utf-8") as f:
            _merge_change_doc(json.load(f), changes)

    result = render(opts["package"], changes)
    png = result["png_bytes"]
    layout = result["layout"]
    samples = result["samples"]

    # Refresh layout copy for delivery.
    layout_out = dict(layout)
    layout_out["Samples"] = samples
    report = {
        "schema": result["schema"],
        "Exposure": result["Exposure"],
        "FaceIDs": result["FaceIDs"],
        "Reflectance": result["Reflectance"],
        "Samples": samples,
        "samples": samples,
        "Lobes": result["Lobes"],
        "Probes": result["Lobes"],
        "png_base64": base64.b64encode(png).decode("ascii"),
    }

    written = {}
    outdir = opts["outdir"]
    if outdir:
        low = outdir.lower()
        if low.endswith(".png"):
            opts["png"] = outdir
            stem = outdir[:-4]
            opts["json"] = opts["json"] or (stem + ".json")
            outdir = None
        elif low.endswith(".json"):
            opts["json"] = outdir
            outdir = None
    if outdir:
        os.makedirs(outdir, exist_ok=True)
        png_path = os.path.join(outdir, "probes.png")
        json_path = os.path.join(outdir, "samples.json")
        layout_path = os.path.join(outdir, "layout.json")
        zip_path = opts["zip_out"] or os.path.join(outdir, "probe-volume-recolored.zip")
        with open(png_path, "wb") as f:
            f.write(png)
        with open(json_path, "w", encoding="utf-8") as f:
            json.dump(report, f, indent=2)
        layout_bytes = json.dumps(layout_out, indent=2).encode("utf-8")
        with open(layout_path, "wb") as f:
            f.write(layout_bytes)
        _write_recolored_zip(zip_path, layout, layout_bytes, png)
        written = {"png": png_path, "json": json_path, "layout": layout_path, "zip": zip_path}
    if opts["png"]:
        with open(opts["png"], "wb") as f:
            f.write(png)
        written["png"] = opts["png"]
    if opts["json"]:
        with open(opts["json"], "w", encoding="utf-8") as f:
            json.dump(report, f, indent=2)
        written["json"] = opts["json"]
    if opts["zip_out"] and not outdir:
        layout_bytes = json.dumps(layout_out, indent=2).encode("utf-8")
        _write_recolored_zip(opts["zip_out"], layout, layout_bytes, png)
        written["zip"] = opts["zip_out"]

    report["written"] = written
    if not (opts["outdir"] or opts["png"] or opts["json"] or opts["zip_out"]):
        # No destination requested: drop conventionally named files in CWD so
        # callers that look for a fixed name still find the result.
        for name in ("preview.png", "probes.png", "probes-recolored.png"):
            with open(name, "wb") as f:
                f.write(png)
        raw_report = json.dumps(report, indent=2).encode("utf-8")
        for name in ("preview.json", "samples.json"):
            with open(name, "wb") as f:
                f.write(raw_report)
        written["defaults"] = ["preview.png", "probes.png", "preview.json", "samples.json"]
        report["written"] = written
    if opts["print_png_base64"]:
        report["png_base64"] = base64.b64encode(png).decode("ascii")
    if not opts["quiet"]:
        print(json.dumps(report))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except (KeyError, ValueError, OSError, json.JSONDecodeError) as exc:
        print("preview: %s" % exc, file=sys.stderr)
        raise SystemExit(1)
