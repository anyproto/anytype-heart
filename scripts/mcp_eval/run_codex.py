#!/usr/bin/env python3
"""Alias of run_scenario.py with the Codex host, so existing commands and
imports (`import run_codex as runner`) keep working."""
from run_scenario import *  # noqa: F401,F403
from run_scenario import main

if __name__ == "__main__":
    main(default_host="codex")
