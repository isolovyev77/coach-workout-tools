// store.mjs - персистентное хранилище cookie-сессии Trenda.
//
// Пароль НИКОГДА не сохраняется: на диск кладутся только cookie, выданные
// сервером при логине, плюс минимум метаданных для диагностики. Файл создаётся
// с правами 0600 в каталоге 0700.

import { mkdirSync, readFileSync, writeFileSync, existsSync, unlinkSync, chmodSync } from 'node:fs';
import { join } from 'node:path';
import { homedir } from 'node:os';

export const CONFIG_DIR = process.env.TRENDA_CONFIG_DIR || join(homedir(), '.config', 'pp-trenda');
export const CREDENTIALS_PATH = join(CONFIG_DIR, 'credentials.json');

export function load() {
  if (!existsSync(CREDENTIALS_PATH)) return null;
  try {
    return JSON.parse(readFileSync(CREDENTIALS_PATH, 'utf8'));
  } catch (err) {
    throw new Error(`не читается ${CREDENTIALS_PATH}: ${err.message}`);
  }
}

export function save(state) {
  mkdirSync(CONFIG_DIR, { recursive: true, mode: 0o700 });
  const payload = { ...state, savedAt: new Date().toISOString() };
  writeFileSync(CREDENTIALS_PATH, JSON.stringify(payload, null, 2) + '\n', { mode: 0o600 });
  chmodSync(CREDENTIALS_PATH, 0o600);
  return payload;
}

export function clear() {
  if (existsSync(CREDENTIALS_PATH)) unlinkSync(CREDENTIALS_PATH);
}
