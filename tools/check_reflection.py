#!/usr/bin/env python3
"""End to end checks for recolorable reflection faces + offline preview.

Builds the server, posts a two wall corner request, then exercises
tools/preview.py against only the downloaded package. Run:

  python3 -B tools/check_reflection.py
"""

import json
import math
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "tools"))

import verify  # noqa: E402


def corner_request():
    return {
        "Grid": {"Origin": [-2, -1, -1], "Step": 1, "Size": [6, 5, 3]},
        "Exposure": 2,
        "Lights": [{
            "Position": [3, 1, 0], "RGB": [1, 1, 1],
            "Radius": 40, "Power": 2,
        }],
        "Blocks": [
            # Wall A: x in [-.2,0], y 0..4 ; lit face +X at center (0,2,0)
            {"Min": [-0.2, 0, -1], "Max": [0, 4, 1]},
            # Wall B: x 0..4, y [4,4.2] ; facing -Y at center (2,4,0)
            {"Min": [0, 4, -1], "Max": [4, 4.2, 1]},
            # Blocker that hides the corner probe from the lamp directly.
            {"Min": [1.2, 1.6, -1], "Max": [1.5, 2.0, 1]},
        ],
        "Faces": [
            {"ID": "wall_a", "Center": [0, 2, 0], "Normal": [1, 0, 0],
             "Reflectance": [0.9, 0.9, 0.9]},
            {"ID": "wall_b", "Center": [2, 4, 0], "Normal": [0, -1, 0],
             "Reflectance": [0.8, 0.8, 0.8]},
        ],
        "Samples": [
            [0.8, 3.2, 0],
            [1, 2, 0],
            [3, 1, 0],
            [-1.5, 2, 0],
        ],
    }


def run_preview_cli(pkg, extra, outdir):
    cmd = [sys.executable, "-B", str(ROOT / "tools" / "preview.py"),
           str(pkg), "-o", str(outdir)] + extra
    proc = subprocess.run(cmd, capture_output=True, text=True)
    assert proc.returncode == 0, proc.stderr + proc.stdout
    return json.loads(proc.stdout)


def main():
    with tempfile.TemporaryDirectory(prefix="reflect-") as root:
        root = Path(root)
        binary, data = root / "server", root / "artifacts"
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/server"],
                       cwd=ROOT, check=True)
        with verify.Service(str(binary), str(data)) as s:
            req = corner_request()
            row = verify.receipt(s.post(req))
            blob = verify.fetch(s, row)
            pkg = root / "probe-volume.zip"
            pkg.write_bytes(blob)

            import zipfile
            import io
            with zipfile.ZipFile(io.BytesIO(blob)) as z:
                names = z.namelist()
                layout = json.loads(z.read("layout.json"))
                _, _, server_rows = verify.pixels(z.read("probes.png"))
            assert set(names) == {"layout.json", "probes.png"}, names
            basis = layout.get("Preview") or layout.get("Offline")
            assert basis and len(basis["FaceIDs"]) == 2
            assert layout["Offline"]["FaceIDs"] == ["wall_a", "wall_b"]

            # Alias wire schema must decode to the same artifact.
            alias_req = json.loads(json.dumps(req))
            alias_req.pop("Faces")
            alias_req["ReflectiveFaces"] = [
                {"id": "wall_a", "centroid": [0, 2, 0],
                 "outward_normal": [1, 0, 0], "albedo": [0.9, 0.9, 0.9]},
                {"face_id": "wall_b", "position": [2, 4, 0],
                 "normal": {"X": 0, "Y": -1, "Z": 0},
                 "diffuse": {"R": 0.8, "G": 0.8, "B": 0.8}},
            ]
            alias_row = verify.receipt(s.post(alias_req))
            assert alias_row["digest"] == row["digest"], "alias schema must be equivalent"

            # Domain rejection boundaries for the new face contract.
            too_many = json.loads(json.dumps(req))
            too_many["Faces"] = [
                {"ID": "f%d" % i, "Center": [0, 2, 0], "Normal": [1, 0, 0],
                 "Reflectance": [1, 1, 1]} for i in range(17)]
            assert s.post(too_many)[0] == 422
            bad_value = json.loads(json.dumps(req))
            bad_value["Faces"][0]["Reflectance"] = [2, 0, 0]
            assert s.post(bad_value)[0] == 422
            bad_face = json.loads(json.dumps(req))
            bad_face["Faces"] = [{"ID": "x", "Center": [9, 9, 9],
                                  "Normal": [1, 0, 0], "Reflectance": [1, 1, 1]}]
            assert s.post(bad_face)[0] == 422
            dup = json.loads(json.dumps(req))
            dup["Faces"][1]["ID"] = dup["Faces"][0]["ID"]
            assert s.post(dup)[0] == 422

            # Initial preview must reproduce the shipped pixels and samples.
            out0 = root / "out0"
            rep0 = run_preview_cli(pkg, [], out0)
            _, _, prev_rows = verify.pixels((out0 / "probes.png").read_bytes())
            max_delta = 0
            for ra, rb in zip(server_rows, prev_rows):
                for pa, pb in zip(ra, rb):
                    max_delta = max(max_delta, max(abs(a - b) for a, b in zip(pa, pb)))
            assert max_delta <= 1, ("initial preview pixels differ", max_delta)

            init_samples = layout["Samples"]
            for a, b in zip(init_samples, rep0["Samples"]):
                assert a["State"] == b["State"]
                assert abs(a["Weight"] - b["Weight"]) <= 1e-12
                for k in range(6):
                    for c in range(3):
                        assert abs(a["Irradiance"][k][c] - b["Irradiance"][k][c]) <= 1e-9

            # Recolor wall_a red to zero only through the offline package.
            out_red = root / "red"
            rep_red = run_preview_cli(
                pkg, ["--set", "wall_a=0,1,1", "--set", "wall_b", "0.25", "0", "1"],
                out_red)
            eff = rep_red["Reflectance"]
            assert eff["wall_a"] == [0.0, 1.0, 1.0]
            assert eff["wall_b"] == [0.25, 0.0, 1.0]

            # Independent recomputation straight from the frozen basis.
            sys.path.insert(0, str(ROOT / "tools"))
            import preview
            ids, rho = preview.apply_overrides(
                basis, {"wall_a": [0, 1, 1], "wall_b": [0.25, 0, 1]})
            lobes = preview.recompute_lobes(basis, rho)
            expect_samples = preview.recompute_samples(basis, lobes)
            for a, b in zip(expect_samples, rep_red["Samples"]):
                for k in range(6):
                    for c in range(3):
                        assert math.isclose(a["Irradiance"][k][c],
                                            b["Irradiance"][k][c], rel_tol=1e-12, abs_tol=1e-12)

            # Frozen state/support/validity survive recoloring.
            for a, b in zip(init_samples, rep_red["Samples"]):
                assert a["State"] == b["State"]
                assert a["Weight"] == b["Weight"]
            red_png = (out_red / "probes.png").read_bytes()
            w, h, red_rows = verify.pixels(red_png)
            assert (w, h) == (layout["TextureWidth"], layout["TextureHeight"])
            for z in range(layout["Grid"]["Size"][2]):
                for y in range(layout["Grid"]["Size"][1]):
                    for x in range(layout["Grid"]["Size"][0]):
                        idx = x + layout["Grid"]["Size"][0] * (
                            y + layout["Grid"]["Size"][1] * z)
                        if not basis["ProbeValid"][idx]:
                            px, py = x, y + z * layout["Grid"]["Size"][1]
                            assert red_rows[py][px][3] == 0

            # Channel kill on a pure two bounce chain: either face removes red
            # but leaves green. Build a synthetic basis through the real tool
            # math to pin the pathwise multiplication contract.
            chain = {
                "FaceIDs": ["A", "B"],
                "Reflectance": [[1, 1, 1], [1, 1, 1]],
                "ProbeValid": [True],
                "Direct": [[[0, 0, 0]] * 6],
                "LightFace": [[1, 1, 0.5], [0, 0, 0]],
                "FaceFace": [[0, 0.5], [0, 0]],
                # Only B reaches the probe, and B has no direct light, so the
                # sole path is lamp -> A -> B -> probe.
                "FaceProbe": [[[0, 0, 0, 0, 0, 0], [0.9, 0, 0, 0, 0, 0]]],
                "Samples": [],
            }
            white = preview.recompute_lobes(chain, [[1, 1, 1], [1, 1, 1]])[0][0]
            assert white[0] > 0 and white[1] > 0
            for kill_index, killed_id in ((0, "A"), (1, "B")):
                ov = {"A": [1, 1, 1], "B": [1, 1, 1]}
                ov[killed_id] = [0, 1, 1]
                _, rho_k = preview.apply_overrides(chain, ov)
                got = preview.recompute_lobes(chain, rho_k)[0][0]
                assert got[0] == 0.0, (kill_index, got)
                assert math.isclose(got[1], white[1], rel_tol=1e-12)
                assert got[2] > 0

            # Unknown face id must fail loudly.
            bad = subprocess.run(
                [sys.executable, str(ROOT / "tools" / "preview.py"),
                 str(pkg), "--set", "nope=1,0,0"],
                capture_output=True, text=True)
            assert bad.returncode != 0

    print("check_reflection: passed")


if __name__ == "__main__":
    main()
