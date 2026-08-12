#!/usr/bin/env bash
# cap-auth.sh - сохранение токена CrossFit Affiliate Programming для cap-pp-cli.
#
#   cap-auth.sh          пошагово помогает достать токен и сохраняет его
#   cap-auth.sh status   проверяет, жив ли сохранённый токен
#
# У CAP нет входа по логину и паролю, который можно было бы повторить из
# терминала: кабинет аффилиата использует OAuth2 PKCE и держит короткоживущий
# токен в браузере. Поэтому токен переносится из уже открытой сессии.
#
# Скрипт НИКОГДА не просит пароль и ничего не отправляет наружу: введённый
# токен уходит только в локальный конфиг cap-pp-cli с правами 0600.
set -euo pipefail

CAP_BIN="${CAP_BIN:-cap-pp-cli}"
TOOLKIT_URL="https://affiliate.crossfit.com/tools/home"

if ! command -v "$CAP_BIN" >/dev/null 2>&1; then
  echo "Не найден $CAP_BIN. Установите инструменты: scripts/install.sh" >&2
  exit 1
fi

check_token() {
  # Библиотека движений открыта и работает без токена, поэтому живость
  # проверяем именно командой программирования.
  "$CAP_BIN" cap day >/dev/null 2>&1
}

if [ "${1:-}" = status ]; then
  if check_token; then
    echo "Токен работает: программа CAP читается."
  else
    echo "Токен отсутствует или истёк. Запустите $0 без параметров."
    exit 4
  fi
  exit 0
fi

cat <<'STEPS'
Как достать токен (занимает полминуты):

  1. Откройте кабинет аффилиата и войдите, если ещё не вошли.
  2. Нажмите F12 (на Mac: Cmd+Option+I) - откроются инструменты разработчика.
  3. Вкладка Application (в Firefox - Storage), слева Local Storage,
     в нём строка affiliate.crossfit.com.
  4. Найдите ключ access_token и скопируйте его значение.

Токен живёт недолго - когда команды снова начнут отвечать «token expired»,
повторите эти шаги. Команды по движениям и бенчмаркам работают и без токена.

STEPS

if command -v open >/dev/null 2>&1; then
  open "$TOOLKIT_URL" >/dev/null 2>&1 || true
elif command -v xdg-open >/dev/null 2>&1; then
  xdg-open "$TOOLKIT_URL" >/dev/null 2>&1 || true
else
  echo "Откройте вручную: $TOOLKIT_URL"
  echo
fi

if [ ! -t 0 ]; then
  echo "Нет терминала для ввода. Сохраните токен напрямую:" >&2
  echo "  $CAP_BIN auth set-token <токен>" >&2
  exit 2
fi

# Ввод без эха: токен - это доступ к аккаунту, ему не место в истории терминала.
printf 'Вставьте токен и нажмите Enter: '
stty -echo
trap 'stty echo' EXIT INT TERM
read -r TOKEN
stty echo
trap - EXIT INT TERM
printf '\n'

TOKEN="$(printf '%s' "$TOKEN" | tr -d '[:space:]"'"'")"
if [ -z "$TOKEN" ]; then
  echo "Токен пустой, ничего не сохранено." >&2
  exit 2
fi

"$CAP_BIN" auth set-token "$TOKEN" >/dev/null
unset TOKEN

if check_token; then
  echo "Готово: токен сохранён и работает."
else
  echo "Токен сохранён, но программа не читается." >&2
  echo "Скорее всего скопирован не тот ключ - нужен именно access_token," >&2
  echo "или срок его действия уже истёк. Повторите: $0" >&2
  exit 4
fi
