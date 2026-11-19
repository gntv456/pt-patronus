#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 The PTPatronus Authors
"""Check user-facing GitHub links point to the real repository.

Background: frontend "feedback / changelog" links once hardcoded hxpt/ptpatronus,
which is 404; the real repo is gntv456/pt-patronus. Go module paths like
github.com/hxpt/ptpatronus/internal/... are package names, not links -- not checked.
Usage: python scripts/check-user-facing-links.py
"""
import os
import re
import sys

ROOT = os.path.join(os.path.dirname(__file__), "..")
CANONICAL_REPO = "gntv456/pt-patronus"
STALE_OWNER_REPOS = ["hxpt/ptpatronus"]

TARGETS = ["frontend/src", "mobile/lib", "backend/internal/media"]
EXT = {".ts", ".vue", ".dart", ".go"}
URL_RE = re.compile(r"https://github[.]com/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)")


def main():
    problems = []
    for target in TARGETS:
        base = os.path.join(ROOT, target)
        for dp, _, fns in os.walk(base):
            for fn in fns:
                if os.path.splitext(fn)[1] not in EXT:
                    continue
                p = os.path.join(dp, fn)
                with open(p, encoding="utf-8", errors="replace") as f:
                    for lineno, line in enumerate(f, 1):
                        for m in URL_RE.finditer(line):
                            owner_repo = m.group(1) + "/" + m.group(2)
                            if owner_repo in STALE_OWNER_REPOS:
                                rel = os.path.relpath(p, ROOT).replace("\\", "/")
                                problems.append((rel, lineno, owner_repo, line.strip()))
    if problems:
        print("[FAIL] stale user-facing links (canonical: %s):" % CANONICAL_REPO, file=sys.stderr)
        for rel, lineno, owner_repo, text in problems[:20]:
            print("  %s:%d  %s  -> %s" % (rel, lineno, owner_repo, text[:100]), file=sys.stderr)
        print("Fix: use https://github.com/" + CANONICAL_REPO, file=sys.stderr)
        return 1
    print("[OK] user-facing links clean (%s)" % CANONICAL_REPO)
    return 0


if __name__ == "__main__":
    sys.exit(main())
