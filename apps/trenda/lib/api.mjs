// api.mjs - минимальный HTTP-клиент к внутреннему API app.trenda.coach.
//
// Приложение общается со своим бэкендом только методом POST с JSON-телом и
// авторизуется httpOnly-cookie. Здесь ровно это и воспроизводится: cookie
// хранятся в jar, при 401 выполняется refresh и запрос повторяется один раз.

export const BASE_URL = process.env.TRENDA_BASE_URL || 'https://app.trenda.coach';

export const ROUTES = {
  login: '/api/v1/coach/login',
  logout: '/api/v1/coach/logout',
  refresh: '/api/v1/coach/refresh-token',
  getCurrent: '/api/v1/coach/get-current',
};

const UA = 'pp-trenda/1.0 (+https://printingpress.dev)';

export class ApiError extends Error {
  constructor(status, body, path) {
    const code = body && body.code ? body.code : `HTTP ${status}`;
    const message = body && body.message ? body.message : String(body || '').slice(0, 200);
    super(`${code}: ${message} (${path})`);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = body && body.details;
    this.path = path;
  }
}

// --- cookie jar -------------------------------------------------------------
// Имена cookie сервер выбирает сам, поэтому jar намеренно не знает их заранее:
// сохраняется всё, что пришло в Set-Cookie.

export function jarToHeader(jar) {
  return Object.entries(jar || {})
    .map(([name, value]) => `${name}=${value}`)
    .join('; ');
}

function mergeSetCookie(jar, setCookieList) {
  const next = { ...(jar || {}) };
  for (const raw of setCookieList || []) {
    const [pair, ...attrs] = raw.split(';');
    const eq = pair.indexOf('=');
    if (eq < 0) continue;
    const name = pair.slice(0, eq).trim();
    const value = pair.slice(eq + 1).trim();
    const expired = attrs.some((a) => {
      const m = /^\s*max-age=(-?\d+)/i.exec(a);
      return m ? Number(m[1]) <= 0 : false;
    });
    if (expired || value === '' || value === 'deleted') delete next[name];
    else next[name] = value;
  }
  return next;
}

async function readBody(res) {
  const text = await res.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

// Один POST-запрос. Возвращает { status, body, jar } с jar, обновлённым из
// Set-Cookie ответа. Ошибки не выбрасывает - решение принимает вызывающий.
export async function rawPost(path, payload, jar, { timeoutMs = 60000 } = {}) {
  const headers = {
    'Content-Type': 'application/json',
    'User-Agent': UA,
    Accept: 'application/json',
    Origin: BASE_URL,
    Referer: `${BASE_URL}/`,
  };
  const cookie = jarToHeader(jar);
  if (cookie) headers.Cookie = cookie;

  const res = await fetch(`${BASE_URL}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(payload ?? {}),
    redirect: 'manual',
    signal: AbortSignal.timeout(timeoutMs),
  });

  const setCookie = typeof res.headers.getSetCookie === 'function' ? res.headers.getSetCookie() : [];
  return { status: res.status, body: await readBody(res), jar: mergeSetCookie(jar, setCookie) };
}

// POST с автоматическим refresh при 401. mutableState.jar обновляется на месте,
// onJarChange вызывается, когда jar действительно изменился (нужно сохранить).
export async function post(path, payload, state, { onJarChange } = {}) {
  let jar = state.jar || {};
  let res = await rawPost(path, payload, jar);
  let changed = false;

  if (res.jar !== jar) {
    jar = res.jar;
    changed = true;
  }

  if (res.status === 401 && path !== ROUTES.refresh && path !== ROUTES.login) {
    const refreshed = await rawPost(ROUTES.refresh, {}, jar);
    jar = refreshed.jar;
    changed = true;
    if (refreshed.status >= 200 && refreshed.status < 300) {
      res = await rawPost(path, payload, jar);
      jar = res.jar;
    } else {
      state.jar = jar;
      if (changed && onJarChange) onJarChange(jar);
      throw new ApiError(401, { code: 'SessionExpired', message: 'сессия истекла, выполните: trenda-auth login' }, path);
    }
  }

  state.jar = jar;
  if (changed && onJarChange) onJarChange(jar);

  if (res.status < 200 || res.status >= 300) throw new ApiError(res.status, res.body, path);
  return res.body;
}
