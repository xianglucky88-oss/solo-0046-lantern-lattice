#!/usr/bin/env python3
"""Repository-root compatibility entry point for the artifact preview tool."""
import os
import runpy
import sys

_ROOT = os.path.dirname(os.path.abspath(__file__))
_TARGET = os.path.join(_ROOT, "tools", "preview.py")

sys.argv[0] = _TARGET
runpy.run_path(_TARGET, run_name="__main__")
