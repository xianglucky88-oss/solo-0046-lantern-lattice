#!/usr/bin/env python3
"""Compatibility alias for tools/preview.py.

Usage is identical, e.g.:
  python3 tools/recolor.py probe-volume.zip --set wall_a=1,0,0 -o out/
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import preview  # noqa: E402

if __name__ == "__main__":
    try:
        raise SystemExit(preview.main(sys.argv[1:]))
    except (KeyError, ValueError, OSError) as exc:
        print("recolor: %s" % exc, file=sys.stderr)
        raise SystemExit(1)
