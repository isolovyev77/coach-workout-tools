#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ -n "${COACH_TOOLS_BIN:-}" ]; then
  BIN_DIR="$COACH_TOOLS_BIN"
elif [ -x "$HOME/bin/btwb-pp-cli" ] || [ -x "$HOME/bin/trenda-pp-cli" ]; then
  # Preserve the location already exposed in the trainer's shell PATH.
  BIN_DIR="$HOME/bin"
else
  BIN_DIR="$HOME/.local/bin"
fi
# Both skills install the same way; listed once so a third one is a one-line
# change rather than a hunt through the script.
SKILL_NAMES="populating-trenda-workouts pp-cap"
INSTALL_CODEX_SKILL=auto
INSTALL_CLAUDE_SKILL=auto
CONFIGURE_PATH=0

for option in "$@"; do
  case "$option" in
    --no-skills) INSTALL_CODEX_SKILL=0; INSTALL_CLAUDE_SKILL=0 ;;
    --codex-only) INSTALL_CODEX_SKILL=1; INSTALL_CLAUDE_SKILL=0 ;;
    --claude-only) INSTALL_CODEX_SKILL=0; INSTALL_CLAUDE_SKILL=1 ;;
    --configure-path) CONFIGURE_PATH=1 ;;
    *) echo "Неизвестный параметр: $option" >&2; exit 2 ;;
  esac
done

if [ "$INSTALL_CODEX_SKILL" = auto ]; then
  [ -d "${CODEX_HOME:-$HOME/.codex}" ] && INSTALL_CODEX_SKILL=1 || INSTALL_CODEX_SKILL=0
fi
if [ "$INSTALL_CLAUDE_SKILL" = auto ]; then
  [ -d "${CLAUDE_HOME:-$HOME/.claude}" ] && INSTALL_CLAUDE_SKILL=1 || INSTALL_CLAUDE_SKILL=0
fi

# A pre-existing regular directory may be a trainer's hand-maintained skill.
# Preserve it by default instead of replacing it during a CLI update.
for parent in "${CODEX_HOME:-$HOME/.codex}/skills" "${CLAUDE_HOME:-$HOME/.claude}/skills"; do
  case "$parent" in
    "${CODEX_HOME:-$HOME/.codex}/skills") enabled="$INSTALL_CODEX_SKILL" ;;
    *) enabled="$INSTALL_CLAUDE_SKILL" ;;
  esac
  [ "$enabled" -eq 1 ] || continue
  for skill in $SKILL_NAMES; do
    target="$parent/$skill"
    if [ -e "$target" ] && [ ! -L "$target" ]; then
      echo "Существующий навык сохранён без замены: $target"
      if [ "$parent" = "${CODEX_HOME:-$HOME/.codex}/skills" ]; then
        INSTALL_CODEX_SKILL=0
      else
        INSTALL_CLAUDE_SKILL=0
      fi
    fi
  done
done

GO_BIN="${GO_BIN:-$(command -v go 2>/dev/null || true)}"
if [ -z "$GO_BIN" ] && [ -x "$HOME/sdk/go/bin/go" ]; then
  GO_BIN="$HOME/sdk/go/bin/go"
fi
[ -n "$GO_BIN" ] || { echo "Нужен Go для сборки CLI." >&2; exit 1; }
command -v node >/dev/null 2>&1 || { echo "Нужен Node.js для входа в Trenda." >&2; exit 1; }

mkdir -p "$BIN_DIR"
STAGE_DIR="$(mktemp -d "${TMPDIR:-/tmp}/coach-workout-tools-install.XXXXXX")"
trap 'rm -rf "$STAGE_DIR"' EXIT
(cd "$ROOT_DIR/cli/btwb" && "$GO_BIN" build -o "$STAGE_DIR/btwb-pp-cli" ./cmd/btwb-pp-cli)
(cd "$ROOT_DIR/cli/trenda" && "$GO_BIN" build -o "$STAGE_DIR/trenda-pp-cli" ./cmd/trenda-pp-cli)
(cd "$ROOT_DIR/cli/cap" && "$GO_BIN" build -o "$STAGE_DIR/cap-pp-cli" ./cmd/cap-pp-cli)
(cd "$ROOT_DIR/cli/cap" && "$GO_BIN" build -o "$STAGE_DIR/cap-pp-mcp" ./cmd/cap-pp-mcp)

cat > "$STAGE_DIR/trenda" <<EOF
#!/usr/bin/env bash
exec "$ROOT_DIR/apps/trenda/trenda" "\$@"
EOF
cat > "$STAGE_DIR/trenda-auth" <<EOF
#!/usr/bin/env bash
exec node "$ROOT_DIR/apps/trenda/trenda-auth.mjs" "\$@"
EOF
chmod +x "$STAGE_DIR/trenda" "$STAGE_DIR/trenda-auth"

BACKUP_DIR="${COACH_TOOLS_BACKUP_DIR:-$HOME/.local/share/coach-workout-tools/backups}/$(date +%Y%m%d-%H%M%S)"
for name in btwb-pp-cli trenda-pp-cli cap-pp-cli cap-pp-mcp trenda trenda-auth; do
  if [ -e "$BIN_DIR/$name" ] || [ -L "$BIN_DIR/$name" ]; then
    mkdir -p "$BACKUP_DIR"
    cp -pR "$BIN_DIR/$name" "$BACKUP_DIR/$name"
  fi
done
for name in btwb-pp-cli trenda-pp-cli cap-pp-cli cap-pp-mcp trenda trenda-auth; do
  mv -f "$STAGE_DIR/$name" "$BIN_DIR/$name"
done

configure_shell_path() {
  local profile="${ZDOTDIR:-$HOME}/.zprofile"
  local marker="# coach-workout-tools PATH"
  case ":$PATH:" in
    *":$BIN_DIR:"*) return 0 ;;
  esac
  if ! grep -Fqs "$marker" "$profile" 2>/dev/null; then
    printf '\n%s\nexport PATH="%s:$PATH"\n' "$marker" "$BIN_DIR" >> "$profile"
  fi
  echo "PATH будет доступен в новых окнах терминала через $profile"
}

if [ "$CONFIGURE_PATH" -eq 1 ]; then
  configure_shell_path
fi

install_skill_link() {
  local parent="$1"
  mkdir -p "$parent"
  local skill target
  for skill in $SKILL_NAMES; do
    target="$parent/$skill"
    if [ -e "$target" ] && [ ! -L "$target" ]; then
      echo "Навык уже есть и не является ссылкой: $target" >&2
      echo "Удали или перемести его, затем повтори установку." >&2
      exit 1
    fi
    ln -sfn "$ROOT_DIR/skills/$skill" "$target"
  done
}

if [ "$INSTALL_CODEX_SKILL" -eq 1 ]; then
  install_skill_link "${CODEX_HOME:-$HOME/.codex}/skills"
fi
if [ "$INSTALL_CLAUDE_SKILL" -eq 1 ]; then
  install_skill_link "${CLAUDE_HOME:-$HOME/.claude}/skills"
fi

echo "Установлено в $BIN_DIR"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "Чтобы команды были доступны в новых терминалах: ./scripts/install.sh --configure-path" ;;
esac
if [ -d "$BACKUP_DIR" ]; then
  echo "Предыдущие CLI сохранены в $BACKUP_DIR"
fi
if [ "$INSTALL_CODEX_SKILL" -eq 1 ] || [ "$INSTALL_CLAUDE_SKILL" -eq 1 ]; then
  echo "Навык подключён для Codex и/или Claude из этого репозитория."
fi
echo "Следующий шаг: trenda-pp-cli auth login"
