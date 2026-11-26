#!/usr/bin/env python3
"""End-to-end checks for coarse/fine tiered baking and offline queries.

Starts a temporary loopback server, exercises:
  * legacy untiered response/artifact unchanged,
  * separately downloadable coarse/fine receipts,
  * unpacked byte accounting vs the requested cap,
  * state / support-weight parity and linear error / zero guarantees over
    arbitrary positions (using a second reference bake),
  * a thin-wall red-vs-blue two-room scene plus a valid-black dark room,
  * all-neighbors-fine exactness and all-or-nothing partial fallback,
  * per-probe fine batch endpoints,
  * explicit coarse infeasibility (422) and opt-in partial delivery,
  * restart persistence of digest-addressed tier queries,
  * a pure-Python offline engine that decodes only coarse.zip.

Run: python3 -B tools/verify_tiers.py
"""

import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
import zipfile

sys.path.insert(0, str(Path(__file__).resolve().parent))
import coarse_query  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent
GO = os.environ.get("GO", "go")


class Service:
    def __init__(self, binary, data):
        self.binary, self.data = binary, data

    def __enter__(self):
        self.proc = subprocess.Popen(
            [self.binary, "-port", "0", "-data", str(self.data)],
            stderr=subprocess.PIPE, text=True)
        line = self.proc.stderr.readline().strip()
        self.url = "http://" + line.split("listening ")[1]
        return self

    def __exit__(self, *exc):
        if self.proc.poll() is None:
            self.proc.terminate()
        try:
            self.proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()

    def call(self, path, data=None, headers=None, method=None):
        req = urllib.request.Request(self.url + path, data=data,
                                    headers=headers or {}, method=method)
        try:
            with urllib.request.urlopen(req, timeout=20) as r:
                return r.status, dict(r.headers), r.read()
        except urllib.error.HTTPError as e:
            return e.code, dict(e.headers), e.read()

    def json(self, path, body):
        st, _, raw = self.call(path, json.dumps(body).encode(),
                               {"Content-Type": "application/json"})
        try:
            return st, json.loads(raw)
        except Exception:
            return st, {"_raw": raw.decode(errors="replace")}

    def get(self, path):
        return self.call(path)


def receipt(row):
    assert row["artifact"] == "/v1/artifacts/" + row["digest"]
    return row


def fetch(s, row):
    st, hdr, blob = s.get(row["artifact"])
    assert st == 200 and len(blob) == row["bytes"]
    assert hashlib.sha256(blob).hexdigest() == row["digest"]
    return blob


def demo():
    return json.loads((ROOT / "internal/service/demo.json").read_text())


def reference_layout(s, scene, points):
    ref = dict(scene)
    ref["Samples"] = points
    st, row = s.json("/v1/build", ref)
    assert st in (200, 201), (st, row)
    blob = fetch(s, receipt(row))
    with zipfile.ZipFile(io.BytesIO(blob)) as z:
        return json.loads(z.read("layout.json"))["Samples"]


def assert_parity(ref_rows, got_rows, eps, label, check_tier=None):
    assert len(ref_rows) == len(got_rows)
    worst = 0.0
    for i, (r, q) in enumerate(zip(ref_rows, got_rows)):
        assert r["State"] == q["State"], (label, i, r["State"], q["State"])
        assert abs(r["Weight"] - q["Weight"]) <= 1e-12, (label, i, r["Weight"], q["Weight"])
        if r["State"] != "sampled":
            continue
        if check_tier:
            assert q["Tier"] == check_tier, (label, i, q["Tier"], check_tier)
        for face in range(6):
            for c in range(3):
                rv = r["Irradiance"][face][c]
                qv = q["Irradiance"][face][c]
                if rv == 0.0:
                    assert qv == 0.0, (label, "zero channel", i, face, c, qv)
                worst = max(worst, abs(rv - qv))
                assert abs(rv - qv) <= eps + 1e-9, (label, i, face, c, rv, qv)
    return worst


def thin_wall_scene():
    return {
        "Grid": {"Origin": [0, 0, 0], "Step": 1, "Size": [7, 3, 3]},
        "Exposure": 3,
        "Lights": [
            {"Position": [0.5, 1, 1], "RGB": [1, 0, 0], "Radius": 8, "Power": 8},
            {"Position": [3.5, 1, 1], "RGB": [0, 0.1, 1], "Radius": 8, "Power": 8},
        ],
        "Blocks": [
            {"Min": [2.45, -1, -1], "Max": [2.55, 3, 3]},
            {"Min": [4.45, -1, -1], "Max": [4.55, 3, 3]},
        ],
        "Samples": [],
    }


def main():
    with tempfile.TemporaryDirectory(prefix="tiers-") as root:
        root = Path(root)
        binary, data = root / "server", root / "art"
        print("building server ...")
        subprocess.run([GO, "build", "-o", str(binary), "./cmd/server"],
                       cwd=str(ROOT), check=True)
        with Service(binary, data) as s:
            run_checks(s, restart_data=data, binary=binary)
    print("verify_tiers: all passed")


def run_checks(s, restart_data, binary):
    # ---- legacy untiered response shape ----------------------------------
    d = demo()
    st, row = s.json("/v1/build", d)
    assert st in (200, 201)
    assert set(row.keys()) == {"digest", "bytes", "artifact"}, row.keys()
    fetch(s, receipt(row))

    # ---- tiered demo build -----------------------------------------------
    eps, budget = 0.02, 4096
    st, t = s.json("/v1/build", {**d, "Tiering": {
        "CoarseByteLimit": budget, "MaxLinearError": eps}})
    assert st in (200, 201), (st, t)
    cdesc, fdesc = t["coarse"], t["fine"]
    assert cdesc["digest"] and fdesc["digest"]
    assert cdesc["digest"] != fdesc["digest"] != t["digest"]
    cblob = fetch(s, cdesc)
    fblob = fetch(s, fdesc)
    with zipfile.ZipFile(io.BytesIO(cblob)) as z:
        cdoc = json.loads(z.read("coarse.json"))
    acc = cdoc["ByteAccounting"]
    colors_n = len(base64_raw(cdoc["Encoding"].get("ColorsBase64", "")))
    lookup_n = len(base64_raw(cdoc["Encoding"].get("LookupBase64", "")))
    coeff_n = 8 * len(cdoc["Encoding"].get("Scale") or [])
    assert colors_n == acc["Colors"] and lookup_n == acc["Lookup"]
    assert coeff_n == acc["Coefficients"]
    assert acc["Total"] == colors_n + coeff_n + lookup_n
    assert acc["Total"] <= budget
    assert cdoc["MaxLinearErrorObserved"] <= eps

    # ---- arbitrary-position parity vs a reference bake --------------------
    points = []
    for x in [0.15, 0.5, 1.35, 2.4, 2.6, 3.5, 4.2, 5.5]:
        for y in [0.25, 1.0, 1.75]:
            for z in [0.3, 0.9]:
                points.append([x, y, z])
    points += [[-0.5, 1, 1], [6.5, 1, 1], [2.5, 1, 1], [4.5, 1, 1]]
    ref = reference_layout(s, d, points)
    st, q = s.json("/v1/query", {"coarse": cdesc["digest"], "positions": points})
    assert st == 200, q
    worst = assert_parity(ref, q["results"], eps, "demo-coarse")
    print(f"demo coarse parity worst error {worst:.5f}")

    # pure offline engine using only coarse.zip
    vol = coarse_query.CoarseVolume.from_zip(cblob)
    off = [vol.query(p) for p in points]
    assert_parity(ref, off, eps, "offline-coarse")

    # ---- fine: all exact, partial stays coarse, weight stable -------------
    st, qf = s.json("/v1/query", {"coarse": cdesc["digest"],
                                 "fine": fdesc["digest"], "positions": points})
    assert st == 200
    assert_parity(ref, qf["results"], 1e-12, "all-fine", check_tier="fine")
    gate_points = [[0.5, 0.5, 0.5], [1.4, 0.6, 0.7], [1.2, 0.4, 0.3],
                   [0.8, 0.6, 0.4], [1.6, 0.5, 0.6]]
    multi = 0
    for p in gate_points:
        q0 = s.json("/v1/query", {"coarse": cdesc["digest"], "positions": [p]})[1]["results"][0]
        if q0["State"] != "sampled":
            continue
        need = q0["requiredFineProbes"]
        assert need
        qnone = s.json("/v1/query", {"coarse": cdesc["digest"], "fine": fdesc["digest"],
                                     "fineProbes": [], "positions": [p]})[1]["results"][0]
        assert qnone["Tier"] == "coarse" and qnone["Weight"] == q0["Weight"]
        if len(need) >= 2:
            multi += 1
            partial = need[:-1]
            qp = s.json("/v1/query", {"coarse": cdesc["digest"], "fine": fdesc["digest"],
                                      "fineProbes": partial, "positions": [p]})[1]["results"][0]
            assert qp["Tier"] == "coarse"
            assert qp["Weight"] == q0["Weight"]
        qa = s.json("/v1/query", {"coarse": cdesc["digest"], "fine": fdesc["digest"],
                                  "fineProbes": need, "positions": [p]})[1]["results"][0]
        assert qa["Tier"] == "fine"
        assert qa["Weight"] == q0["Weight"]
    assert multi >= 1, "expected at least one multi-neighbor gate point"

    # ---- fine batch endpoints --------------------------------------------
    st, _, raw = s.call(f"/v1/artifacts/{fdesc['digest']}/fine",
                        json.dumps({"probes": [0, 1, 2]}).encode(),
                        {"Content-Type": "application/json"})
    assert st == 200
    fb = json.loads(raw)
    assert fb["count"] == 3 and [p["Index"] for p in fb["probes"]] == [0, 1, 2]
    st, _, raw = s.call(f"/v1/artifacts/{fdesc['digest']}/probes/4")
    assert st == 200 and json.loads(raw)["Index"] == 4

    # ---- thin wall red / blue / dark -------------------------------------
    scene = thin_wall_scene()
    wall_points = [[2.4, 1, 1], [2.6, 1, 1], [6, 1, 1], [2.5, 1, 1],
                   [7, 1, 1], [1.5, 0.5, 0.5], [5.5, 1.1, 0.9]]
    wall_ref = reference_layout(s, scene, wall_points)
    st, wall_t = s.json("/v1/build", {**scene, "Tiering": {
        "CoarseByteLimit": 20000, "MaxLinearError": 0.02}})
    assert st in (200, 201)
    wc, wf = wall_t["coarse"]["digest"], wall_t["fine"]["digest"]
    st, wq = s.json("/v1/query", {"coarse": wc, "positions": wall_points})
    assert_parity(wall_ref, wq["results"], 0.02, "wall-coarse")
    st, wqf = s.json("/v1/query", {"coarse": wc, "fine": wf, "positions": wall_points})
    assert_parity(wall_ref, wqf["results"], 1e-12, "wall-fine", check_tier="fine")
    red, blue, dark, solid, outside = wq["results"][:5]

    def channel_sum(row, c):
        return sum(row["Irradiance"][f][c] for f in range(6))

    assert red["State"] == "sampled" and 0 < red["Weight"] < 1
    assert blue["State"] == "sampled" and 0 < blue["Weight"] < 1
    assert channel_sum(red, 0) > 5 * channel_sum(red, 2)
    assert channel_sum(blue, 2) > 5 * channel_sum(blue, 0)
    assert dark["State"] == "sampled" and dark["Weight"] == 1.0
    assert all(v == 0.0 for lobe in dark["Irradiance"] for v in lobe)
    assert solid["State"] == "solid" and outside["State"] == "outside"

    # offline engine agrees on the wall scene too
    wc_blob = s.get("/v1/artifacts/" + wc)[2]
    wvol = coarse_query.CoarseVolume.from_zip(wc_blob)
    assert_parity(wall_ref, [wvol.query(p) for p in wall_points], 0.02, "wall-offline")

    # ---- unsupported parity ----------------------------------------------
    corners = [[x, y, z] for x in (0, 1) for y in (0, 1) for z in (0, 1)]
    us = {
        "Grid": {"Origin": [0, 0, 0], "Step": 1, "Size": [2, 2, 2]},
        "Exposure": 3,
        "Lights": [{"Position": [5, 5, 5], "RGB": [1, 1, 1], "Radius": 20, "Power": 4}],
        "Blocks": [{"Min": [c[0] - .15, c[1] - .15, c[2] - .15],
                    "Max": [c[0] + .15, c[1] + .15, c[2] + .15]} for c in corners],
        "Samples": [[0.5, 0.5, 0.5]],
    }
    ust, usrow = s.json("/v1/build", us)
    assert ust in (200, 201)
    usz = zipfile.ZipFile(io.BytesIO(fetch(s, usrow)))
    usref = json.loads(usz.read("layout.json"))["Samples"][0]
    assert usref["State"] == "unsupported"
    ust, ustier = s.json("/v1/build", {**us, "Samples": [], "Tiering": {
        "CoarseByteLimit": 4096, "MaxLinearError": 0.05}})
    assert ust in (200, 201)
    uq = s.json("/v1/query", {"coarse": ustier["coarse"]["digest"],
                             "positions": [[0.5, 0.5, 0.5]]})[1]["results"][0]
    assert uq["State"] == "unsupported" and uq["Weight"] == 0.0

    # ---- infeasible budget reports explicitly -----------------------------
    st, bad = s.json("/v1/build", {**d, "CoarseByteLimit": 8, "MaxLinearError": 0.001})
    assert st == 422 and bad.get("code") == "coarse_budget_infeasible", (st, bad)
    st, soft = s.json("/v1/build", {**d, "CoarseByteLimit": 8,
                                    "MaxLinearError": 0.001, "Partial": True})
    assert st in (200, 201)
    assert soft["coarse"]["available"] is False and soft.get("fineDigest")

    # ---- alias spellings --------------------------------------------------
    st, a1 = s.json("/v1/build", {**d, "coarse_byte_budget": 8192, "max_linear_error": 0.05})
    assert st in (200, 201) and a1["coarse"]["lightingBytes"] <= 8192
    st, a2 = s.json("/v1/build", {**d, "Coarse": {"ByteLimit": 2048, "MaxError": 0.1}})
    assert st in (200, 201)

    # ---- restart persistence for queries ----------------------------------
    coarse_digest = cdesc["digest"]
    with Service(binary, restart_data) as s2:
        st, q2 = s2.json("/v1/query", {"coarse": coarse_digest, "positions": points[:4]})
        assert st == 200
        assert len(q2["results"]) == 4
        blob = s2.get("/v1/artifacts/" + coarse_digest)[2]
        assert hashlib.sha256(blob).hexdigest() == coarse_digest


def base64_raw(text):
    import base64
    return base64.b64decode(text or "")


if __name__ == "__main__":
    main()
