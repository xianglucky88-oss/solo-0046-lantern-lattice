#!/usr/bin/env python3
"""Alias entry point; see tools/preview.py."""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import preview  # noqa: E402

if __name__ == "__main__":
    raise SystemExit(preview.main(sys.argv[1:]))
