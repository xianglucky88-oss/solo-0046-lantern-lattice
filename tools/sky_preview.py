#!/usr/bin/env python3
"""Compatibility entry point; forwards to tools/preview.py."""
import os
import runpy
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
_TARGET = os.path.join(_HERE, "preview.py")

sys.argv[0] = _TARGET
runpy.run_path(_TARGET, run_name="__main__")
