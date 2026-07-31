#!/usr/bin/env node
// trenda-cookie.mjs - печатает в stdout актуальную строку Cookie для сессии.
//
// Аналог whoop-refresh.mjs из pp-whoop: перед каждым вызовом Go CLI обёртка
// bin/trenda получает отсюда готовое значение заголовка Cookie. Если сессия
// протухла, cookie обновляются через refresh-token и сохраняются обратно.
//
//   trenda-cookie            строка Cookie (при необходимости с refresh)
//   trenda-cookie --check    только проверка, что сессия жива (код возврата)

import { ROUTES, rawPost, jarToHeader } from './lib/api.mjs';
import { load, save } from './lib/store.mjs';

const args = process.argv.slice(2);
const checkOnly = args.includes('--check');
const force = args.includes('--force');

const saved = load();
if (!saved || !saved.jar || !Object.keys(saved.jar).length) {
  console.error('Сессии Trenda нет. Выполните: trenda-auth login');
  process.exit(1);
}

let jar = saved.jar;

async function refresh() {
  const res = await rawPost(ROUTES.refresh, {}, jar);
  if (res.status < 200 || res.status >= 300) return false;
  jar = res.jar;
  save({ ...saved, jar });
  return true;
}

// Дешёвая проверка: get-current стоит один запрос и точно показывает, жива ли
// сессия. Обновляемся только когда сервер ответил 401.
const probe = force ? { status: 401 } : await rawPost(ROUTES.getCurrent, {}, jar);
if (probe.status === 401) {
  if (!(await refresh())) {
    console.error('Сессия Trenda истекла. Выполните: trenda-auth login');
    process.exit(1);
  }
} else if (probe.status >= 200 && probe.status < 300 && probe.jar !== jar) {
  jar = probe.jar;
  save({ ...saved, jar });
}

if (checkOnly) {
  console.error('Сессия активна');
  process.exit(0);
}

process.stdout.write(jarToHeader(jar) + '\n');
