# 🏋️ Coach Workout Tools

## 🇷🇺 По-русски

Набор помогает тренеру переносить и составлять тренировки между BTWB и
Trenda. В него входят две команды и навык для Codex или Claude:

- `btwb-pp-cli` - читает тренировки и треки в BTWB;
- `trenda-pp-cli` и команда `trenda` - работают с аккаунтом тренера в
  Trenda;
- `populating-trenda-workouts` - помогает перенести тренировку, подготовить
  разминку под движения и проверить сохранённый результат.

### 🚀 Быстрый старт

1. Установите Go и Node.js.
2. Склонируйте этот репозиторий и выполните:

   ```bash
   ./scripts/verify-public-release.py
   ./scripts/install.sh --configure-path
   ```

3. Войдите в свои личные аккаунты BTWB и Trenda:

   ```bash
   btwb-pp-cli auth login
   trenda-pp-cli auth login
   ```

4. Откройте Codex или Claude и попросите перенести тренировку либо составить
   новую. Навык найдёт клиента только в вашем аккаунте и перед записью
   уточнит дату и содержание, если они неоднозначны.

### 📦 Готовые файлы установки

На странице [Releases](../../releases) есть готовые архивы, в которых Go уже
не нужен. Выберите файл по результату `uname -s` и `uname -m`:

- `coach-workout-tools_darwin_arm64.tar.gz` - Mac с Apple Silicon;
- `coach-workout-tools_darwin_amd64.tar.gz` - Mac Intel;
- `coach-workout-tools_linux_amd64.tar.gz` - Linux на Intel/AMD;
- `coach-workout-tools_linux_arm64.tar.gz` - Linux на ARM.

Распакуйте подходящий архив, откройте его папку и выполните
`./scripts/install.sh --configure-path`. Node.js всё равно нужен для входа в
Trenda. На Linux установите его штатным менеджером пакетов дистрибутива.

### 🤖 Инструкция для ИИ-модели

Попросите Codex, Claude или другую модель прочитать
[AGENTS.md](AGENTS.md) в корне репозитория. Там есть краткая инструкция, как
объяснить возможности набора, выбрать архив и безопасно установить его.

### 🔄 Обновление

В папке репозитория выполните `./scripts/update.sh`. Он получает только
быстрые обновления, проверяет пакет на случайно попавшие личные данные и
пересобирает команды. Ваши cookies, авторизация и локальные данные клиентов
при обновлении не копируются и не перезаписываются.

### 🔒 Конфиденциальность

В репозитории нет клиентов, номеров клиентов, паролей, cookies, токенов,
скриншотов или истории тренировок. Каждый тренер проходит авторизацию сам и
работает только со своими данными. Перед публикацией или обновлением запускайте
`./scripts/verify-public-release.py`.

Open macOS toolkit for coaches who use BTWB and Trenda. It includes:

- `btwb-pp-cli` - read workouts and tracks from BTWB;
- `trenda-pp-cli` plus the `trenda` session wrapper - work with a coach's own
  Trenda account;
- `populating-trenda-workouts` - an agent skill for transferring or composing
  workouts safely.

This repository contains source code and generic examples only. It deliberately
does not contain client data, coaching accounts, cookies, passwords, personal
names, session exports, or preconfigured client IDs.

## 🎯 What the workout skill does

`populating-trenda-workouts` guides Codex or Claude through a safe coaching
workflow:

1. read a workout from BTWB or take a coach's written workout brief;
2. find the intended client inside that coach's own Trenda account;
3. draft a workout, warm-up and notes adapted to the movements;
4. show the proposed text before making changes when the task is ambiguous;
5. create or update only the selected client's workout, then read it back for
   verification.

The skill never contains a client list or account data. Each trainer signs in
to their own services and identifies the client inside their own Trenda account
at the time of the task. Writes to Trenda are intentional, narrow and checked
afterwards.

## 🧰 Requirements

- macOS or Linux on Intel/AMD (`amd64`) or ARM (`arm64`)
- Node.js, required for Trenda sign-in and automatic session refresh
- Go 1.26.3 or newer, required only when building from source
- a separate BTWB and Trenda account for each coach

## ⚙️ Install

```bash
git clone https://github.com/<account>/coach-workout-tools.git
cd coach-workout-tools
./scripts/verify-public-release.py
./scripts/install.sh --configure-path
```

The installer builds both binaries and places small launchers in
`~/.local/bin`. If an earlier installation already uses `~/bin`, it updates
that same location instead and saves the previous files in a timestamped local
backup. `--configure-path` adds the selected directory to the user's
`~/.zprofile` once, so new Terminal windows, Codex and Claude can find the
commands. Without that option the installer prints the exact next step and
does not edit shell configuration.

### 📦 Install from a ready-made release archive

Ready-made macOS and Linux archives are available from
[Releases](../../releases). Extract the archive for the operating system and
CPU architecture, then run:

```bash
./scripts/verify-public-release.py
./scripts/install.sh --configure-path
```

The archive includes prebuilt CLI binaries, so Go is not required. Node.js is
still required for Trenda sign-in.

### 🛠️ Build from source

For macOS, install dependencies with Homebrew:

```bash
brew install go node
```

For Linux, install Node.js with the distribution's package manager and install
Go 1.26.3 or newer from [go.dev](https://go.dev/dl/) if the distribution ships
an older version.

Sign in to BTWB and Trenda interactively. Passwords are never written to disk;
each CLI stores only its local session data with owner-only permissions.

```bash
btwb-pp-cli auth login
trenda-pp-cli auth login
trenda-pp-cli doctor
btwb-pp-cli auth status
```

## 🔄 Update

Run this from an unmodified clone:

```bash
./scripts/update.sh
```

It only accepts fast-forward Git updates, reruns the public-release scanner,
and rebuilds the local binaries. It never copies another trainer's settings or
touches session files stored in the user's home directory.

## 🤖 Codex and Claude

The default installer connects the same skill to both Codex and Claude using
safe local symbolic links:

- Codex: `~/.codex/skills/populating-trenda-workouts`
- Claude: `~/.claude/skills/populating-trenda-workouts`

So one clone serves both agents, and `./scripts/update.sh` updates the skill
for both on the next conversation. The CLI commands are shared too. To install
only one integration, use `./scripts/install.sh --codex-only` or
`./scripts/install.sh --claude-only`.

Keep the repository in place after installation, or rerun the installer after
moving it. You may set `COACH_TOOLS_HOME` to its absolute path when invoking
the skill manually.

## 🔐 Security and privacy

- Do not commit `.env` files, config files, cookies, client exports, screenshots,
  or chat logs.
- Run `./scripts/verify-public-release.py` before every push or release.
- Each coach must authenticate personally and must resolve clients from their
  own Trenda account before any write.
- This project is an independent integration. BTWB and Trenda names are used
  only to identify the services the tools interact with.

## 📜 License

MIT. See [LICENSE](LICENSE).

The bundled CLI source keeps its Apache-2.0 license and required notices. See
the `LICENSE` and `NOTICE` files in each CLI directory.
