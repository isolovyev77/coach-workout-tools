#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
  echo "Для обновления нужен клон Git-репозитория." >&2; exit 1;
}
if [ -n "$(git status --porcelain)" ]; then
  echo "Есть локальные изменения. Обновление остановлено, чтобы ничего не перезаписать." >&2
  exit 1
fi

git pull --ff-only
./scripts/verify-public-release.py
./scripts/install.sh
