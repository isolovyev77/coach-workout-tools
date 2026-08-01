# 🏋️ Coach Workout Tools

## 🇷🇺 По-русски

Набор помогает тренеру переносить и составлять тренировки между BTWB и
Trenda. В него входят две команды и навык для Codex или Claude:

- `btwb-pp-cli` - читает программу BTWB, планирует тренировку на доступный
  трек и безопасно работает в режиме агента;
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

### 🧭 Возможности BTWB CLI

**Чтение программы:** `wod today`, `wod day --date`, `wod week` и
`wod event <id>` читают whiteboard и полный текст отдельной тренировки.
Фильтры `--track`, `--track-id`, `--planned-only` и `--details` помогают
получить только нужные данные. Есть также низкоуровневые команды для треков,
whiteboard и собственных залогированных результатов.

**Запись:** `wod plan` отдаёт обычный текст тренировки самому BTWB, который
разбирает его в структурированные движения. CLI не придумывает структуру
сам. `wod tracks` показывает только доступные для записи треки, а
`wod unplan <id>` удаляет конкретную запланированную запись.

**Безопасность и интеграции:** `auth login` запрашивает пароль интерактивно и
сохраняет только cookie с правами владельца файла. Вывод можно получать в
едином JSON-конверте, а режим `--agent` удобен для Codex и Claude. Вход и
удаление не показываются агенту через MCP; планирование требует отдельного
явного `--yes`.

### 🗓 Планирование в BTWB

Сначала посмотрите только те треки, в которые ваш аккаунт вправе вносить
тренировки:

```bash
btwb-pp-cli wod tracks
```

Без прав администратора BTWB обычно разрешает запись лишь в личный трек.
Трек зала появится в этом списке только после выдачи прав администратором
зала. Перед записью всегда сделайте просмотр, который ничего не меняет:

```bash
btwb-pp-cli wod plan --date YYYY-MM-DD --track "Personal" \
  --workout-file wod.txt --dry-run
```

После проверки и явного подтверждения добавьте `--yes`. Команда показывает
идентификатор созданной записи, по которому её можно проверить в BTWB.

### 🔑 Что дадут дополнительные доступы BTWB

#### 1. Права администратора зала

Их выдаёт держатель подписки в BTWB: **Admin Console -> Members -> Manage**.
После выдачи прав ничего переустанавливать не нужно: клубные треки появятся в
`btwb-pp-cli wod tracks`, и в них можно будет планировать тренировку той же
командой `wod plan`. Для первого запуска на новом треке достаточно сделать
`--dry-run`, проверить предпросмотр и затем подтвердить запись.

В перспективе эти права позволят расширить CLI функциями инструкций для
атлетов, заметок тренера, других типов записей, редактирования существующей
тренировки и пакетного планирования недели. Это направления разработки, а не
текущие команды.

#### 2. Ключ Web Widgets

Ключ берётся в BTWB в **Admin Console -> Extras -> Webwidgets**. В некоторых
версиях интерфейса этот раздел называется **Website Integration**. Это ключ
**только для чтения**: он не даёт права планировать или редактировать
программу. Зато он включает готовые команды
`webwidgets widget-wods`, `webwidgets widget-activities` и
`webwidgets widget-leaderboard`, которым не нужна личная сессия тренера.

#### Как действовать безопасно

- **Права администратора не нужно пересылать.** Держатель подписки назначает
  их обычному BTWB-аккаунту нужного тренера. Затем тренер входит в CLI под
  своим аккаунтом и запускает `btwb-pp-cli wod tracks`: клубный трек должен
  появиться в списке.
- **Ключ Web Widgets не нужно отправлять в общий чат, вам или ИИ-модели.**
  Администратор настраивает его локально на своём компьютере. Своя модель
  может объяснить шаги установки, но сам ключ тренер вводит самостоятельно,
  не вставляя его в переписку. После настройки ключ остаётся только в
  локальной конфигурации этого тренера.
- Пароли BTWB, cookies, ключи и данные клиентов никогда не передаются между
  тренерами. Для включения возможностей достаточно сообщить: «доступ выдан»
  или «ключ настроен».

Для локальной настройки ключа тренер открывает Терминал на своём компьютере
и вводит:

```bash
read -rs WIDGET_KEY
printf '\n'
btwb-pp-cli auth set-widget-key "$WIDGET_KEY"
unset WIDGET_KEY
```

Первая строка попросит вставить ключ, но не покажет его на экране. В истории
терминала останутся команды, а не значение ключа. После этого read-only
команды Web Widgets будут работать только на этом компьютере; для другого
тренера настройку повторяют отдельно.

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

Open toolkit for coaches who use BTWB and Trenda. It includes:

- `btwb-pp-cli` - read workouts and tracks from BTWB, and plan a workout onto
  a permitted track;
- `trenda-pp-cli` plus the `trenda` session wrapper - work with a coach's own
  Trenda account;
- `populating-trenda-workouts` - an agent skill for transferring or composing
  workouts safely.

This repository contains source code and generic examples only. It deliberately
does not contain client data, coaching accounts, cookies, passwords, personal
names, session exports, or preconfigured client IDs.

## 🧭 BTWB CLI capabilities

**Read programming:** `wod today`, `wod day --date`, `wod week`, and
`wod event <id>` read the whiteboard and full workout text. Use `--track`,
`--track-id`, `--planned-only`, and `--details` to narrow the result. The
lower-level commands also expose tracks, whiteboard data, and the coach's own
logged results.

**Plan workouts:** `wod plan` submits plain workout text to BTWB, which parses
it into structured movements. The CLI replays BTWB's resulting form instead of
inventing a workout structure. `wod tracks` lists writable tracks and
`wod unplan <id>` removes one explicitly identified planned event. A plan is
previewed with `--dry-run` and writes only with a separate explicit `--yes`.

**Safety and agents:** `auth login` reads the password interactively and saves
only an owner-readable session cookie. Commands support a consistent JSON
envelope and `--agent`. Login and deletion are not exposed through MCP;
planning still needs an explicit `--yes`.

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

### 🗓 Plan a workout in BTWB

List only tracks the signed-in account may modify:

```bash
btwb-pp-cli wod tracks
```

Without BTWB gym-admin rights, this is normally the personal track only. A gym
track appears only after an administrator grants permission. Always preview a
write first:

```bash
btwb-pp-cli wod plan --date YYYY-MM-DD --track "Personal" \
  --workout-file wod.txt --dry-run
```

After checking the preview and receiving the coach's explicit confirmation,
repeat the command with `--yes` to write it.

### 🔑 Additional BTWB access

#### 1. Gym administrator rights

The BTWB subscription holder grants these in **Admin Console -> Members ->
Manage**. Once granted, gym tracks automatically appear in
`btwb-pp-cli wod tracks`; no reinstall is needed. Plan into such a track with
the same `wod plan` command, beginning with `--dry-run` on the first use.

These rights can also support future work on athlete instructions, coach notes,
other event types, editing existing workouts, and batch planning. Those are
roadmap items, not currently shipped commands.

#### 2. Web Widgets key

Find this key at **Admin Console -> Extras -> Webwidgets**; some BTWB UI
versions label it **Website Integration**. It is read-only and does not grant
planning or editing permission. It enables the existing
`webwidgets widget-wods`, `webwidgets widget-activities`, and
`webwidgets widget-leaderboard` commands without a coach's personal session.

#### Safe setup

- **Do not send administrator access to anyone.** The subscription holder
  assigns it to the intended trainer's ordinary BTWB account. That trainer
  then signs in locally and runs `btwb-pp-cli wod tracks` to confirm the gym
  track is available.
- **Do not send a Web Widgets key in a group chat, to a coordinator, or to an
  AI model.** An administrator configures it locally on their own computer.
  A model may explain the steps, but the trainer enters the key themselves,
  outside the conversation. The key remains only in that trainer's local
  configuration.
- Never share BTWB passwords, cookies, keys, or client data. To coordinate,
  it is enough to say that access was granted or the key was configured.

To configure the key locally, the trainer opens Terminal on their own computer
and enters:

```bash
read -rs WIDGET_KEY
printf '\n'
btwb-pp-cli auth set-widget-key "$WIDGET_KEY"
unset WIDGET_KEY
```

The first line accepts a pasted key without echoing it. Terminal history keeps
the commands, not the key value. Web Widgets read-only commands then work only
on that computer; repeat the setup separately for every other trainer.

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
