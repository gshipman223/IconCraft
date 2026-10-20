#!/usr/bin/env python3
"""Compatibility entrypoint for the current one-page UI test suite."""
import runpy
from pathlib import Path
runpy.run_path(str(Path(__file__).with_name('ui_checks_v04.py')),run_name='__main__')
