#!/usr/bin/env python3
"""Root level shim for tools/preview.py.

Usage:
  python3 preview.py probe-volume.zip --set wall_a=1,0,0 -o out/
"""

import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, "tools"))

import preview  # noqa: E402

if __name__ == "__main__":
    raise SystemExit(preview.main(sys.argv[1:]))
