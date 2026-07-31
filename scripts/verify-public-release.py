#!/usr/bin/env python3
"""Fail closed when a public release tree contains likely personal data or secrets."""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SKIP_DIRS = {".git", ".build", "dist", "__pycache__"}
FORBIDDEN_NAMES = {".env", ".env.local", ".env.production", ".DS_Store"}
PRIVATE_MARKERS = ("Владимир", "Володя", "Ivan", "Solovyev", "isolo" + "vyev")
RULES = {
    "known personal marker": re.compile("|".join(map(re.escape, PRIVATE_MARKERS)), re.I),
    "sample client identifier": re.compile(r"client[_ -]?id.{0,12}\\b320\\b", re.I),
    "known BTWB member identifier": re.compile(r"\\b249397\\b"),
    "known BTWB track identifier": re.compile(r"\\b694243\\b"),
    "known BTWB personal-track identifier": re.compile(r"\\b277293\\b"),
    "hard-coded session value": re.compile(r"(?:TRENDA_SESSION|BTWB_SESSION)\\s*=\\s*['\"](?!<)[^'\"\\s]{12,}", re.I),
    "GitHub token": re.compile(r"(?:gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,})"),
    "OpenAI key": re.compile(r"sk-[A-Za-z0-9]{20,}"),
    "AWS access key": re.compile(r"AKIA[0-9A-Z]{16}"),
}

problems: list[str] = []
for path in ROOT.rglob("*"):
    if any(part in SKIP_DIRS for part in path.parts) or path.is_dir():
        continue
    rel = path.relative_to(ROOT)
    if path.name in FORBIDDEN_NAMES or path.suffix.lower() in {".cookie", ".session", ".sqlite", ".sqlite3"}:
        problems.append(f"forbidden file: {rel}")
        continue
    if path == Path(__file__).resolve():
        continue
    try:
        text = path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        continue
    for label, pattern in RULES.items():
        if pattern.search(text):
            problems.append(f"{label}: {rel}")

if problems:
    print("Публичная проверка не пройдена:", file=sys.stderr)
    print("\n".join(f"- {item}" for item in problems), file=sys.stderr)
    sys.exit(1)
print("Публичная проверка пройдена: секретов и известных персональных привязок не найдено.")
