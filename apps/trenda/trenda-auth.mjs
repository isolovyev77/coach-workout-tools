#!/usr/bin/env node
// trenda-auth.mjs - вход в Trenda и управление локальной сессией.
//
//   trenda-auth login     вход по email и паролю, сохранение cookie-сессии
//   trenda-auth status    проверка живой сессии (кто вошёл, когда сохранено)
//   trenda-auth refresh   принудительное обновление сессии
//   trenda-auth logout    завершение сессии на сервере и удаление локальных cookie
//
// Пароль спрашивается в терминале без эха и на диск не попадает: сохраняются
// только cookie, выданные сервером. Для неинтерактивного входа можно передать
// TRENDA_EMAIL и TRENDA_PASSWORD в окружении (пароль всё равно не сохраняется).

import { createInterface } from 'node:readline';
import { stdin, stdout } from 'node:process';
import { ROUTES, post, rawPost, ApiError, BASE_URL } from './lib/api.mjs';
import { load, save, clear, CREDENTIALS_PATH } from './lib/store.mjs';

function ask(question, { hidden = false } = {}) {
  return new Promise((resolve, reject) => {
    if (!stdin.isTTY) {
      reject(new Error(`нет терминала для ввода. Передайте TRENDA_EMAIL и TRENDA_PASSWORD в окружении`));
      return;
    }
    const rl = createInterface({ input: stdin, output: stdout, terminal: true });
    if (hidden) {
      // Гасим эхо: readline пишет символы сам, поэтому подменяем _writeToOutput.
      rl._writeToOutput = function (chunk) {
        if (chunk.includes(question)) rl.output.write(question);
      };
    }
    rl.question(question, (answer) => {
      rl.close();
      if (hidden) stdout.write('\n');
      resolve(answer.trim());
    });
  });
}

function coachSummary(coach) {
  if (!coach || typeof coach !== 'object') return null;
  const c = coach.coach || coach.data || coach;
  const name = [c.firstName, c.lastName].filter(Boolean).join(' ');
  return {
    id: c.id ?? null,
    email: c.email ?? null,
    name: name || null,
  };
}

async function cmdLogin() {
  const email = process.env.TRENDA_EMAIL || (await ask('Email: '));
  const password = process.env.TRENDA_PASSWORD || (await ask('Пароль: ', { hidden: true }));
  if (!email || !password) throw new Error('email и пароль обязательны');

  const res = await rawPost(ROUTES.login, { login: email, password }, {});
  if (res.status < 200 || res.status >= 300) throw new ApiError(res.status, res.body, ROUTES.login);
  if (!Object.keys(res.jar).length) {
    throw new Error('сервер не выдал cookie сессии, вход не удался');
  }

  const state = { jar: res.jar, baseUrl: BASE_URL };
  const current = await post(ROUTES.getCurrent, {}, state);
  const coach = coachSummary(current);
  save({ jar: state.jar, baseUrl: BASE_URL, coach });

  const who = coach && (coach.name || coach.email) ? `${coach.name || ''} ${coach.email ? `<${coach.email}>` : ''}`.trim() : 'коуч';
  console.log(`Вход выполнен: ${who}`);
  console.log(`Cookie сессии: ${Object.keys(state.jar).join(', ')}`);
  console.log(`Сохранено: ${CREDENTIALS_PATH} (0600)`);
}

async function cmdStatus() {
  const saved = load();
  if (!saved) {
    console.log('Сессии нет. Выполните: trenda-auth login');
    process.exitCode = 1;
    return;
  }
  const state = { jar: saved.jar };
  try {
    const current = await post(ROUTES.getCurrent, {}, state, {
      onJarChange: (jar) => save({ ...saved, jar }),
    });
    const coach = coachSummary(current);
    console.log('Сессия активна');
    if (coach) console.log(`Коуч: ${coach.name || '-'} ${coach.email ? `<${coach.email}>` : ''} (id ${coach.id ?? '-'})`);
    console.log(`Cookie: ${Object.keys(state.jar).join(', ')}`);
    console.log(`Обновлено: ${saved.savedAt || '-'}`);
  } catch (err) {
    console.error(`Сессия недействительна: ${err.message}`);
    process.exitCode = 1;
  }
}

async function cmdRefresh() {
  const saved = load();
  if (!saved) throw new Error('сессии нет, выполните: trenda-auth login');
  const res = await rawPost(ROUTES.refresh, {}, saved.jar);
  if (res.status < 200 || res.status >= 300) {
    throw new ApiError(res.status, res.body, ROUTES.refresh);
  }
  save({ ...saved, jar: res.jar });
  console.log(`Сессия обновлена. Cookie: ${Object.keys(res.jar).join(', ')}`);
}

async function cmdLogout() {
  const saved = load();
  if (saved) {
    try {
      await rawPost(ROUTES.logout, {}, saved.jar);
    } catch {
      // Сервер мог уже завершить сессию - локальные cookie всё равно удаляем.
    }
  }
  clear();
  console.log('Локальная сессия удалена');
}

const commands = { login: cmdLogin, status: cmdStatus, refresh: cmdRefresh, logout: cmdLogout };

const cmd = process.argv[2] || 'status';
if (cmd === '--help' || cmd === '-h' || cmd === 'help') {
  console.log('Использование: trenda-auth <login|status|refresh|logout>');
  process.exit(0);
}
if (!commands[cmd]) {
  console.error(`неизвестная команда: ${cmd}. Доступны: ${Object.keys(commands).join(', ')}`);
  process.exit(2);
}
try {
  await commands[cmd]();
} catch (err) {
  console.error(`Ошибка: ${err.message}`);
  process.exit(1);
}
