// OAuth consent sends a signed-out browser to `#/signin?next=` to sign in and
// come back. The path is fixed: only the query (validated again by the consent
// endpoint) can vary. An emailed login link usually opens in a new tab, so the
// target is also stored for as long as the one-time password it accompanies.
const signinPrefix = "#/signin?next=";
const oauthTargetMaxLength = 16384;
const returnKey = "pocketcontext.atlas.login_return";
const returnLifetimeMs = 15 * 60 * 1000;

/** Accepts only `/oauth/authorize?<query>` on this origin. */
export function safeOAuthTarget(raw: unknown): string | null {
  if (typeof raw !== "string" || raw.length > oauthTargetMaxLength) return null;
  if (!/^\/oauth\/authorize\?[^\s\\#]+$/.test(raw)) return null;
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f]/.test(raw)) return null;
  try {
    const url = new URL(raw, location.origin);
    return url.origin === location.origin &&
      url.pathname === "/oauth/authorize" &&
      url.pathname + url.search === raw
      ? raw
      : null;
  } catch {
    return null;
  }
}

export const isSigninHash = (hash: string) => /^#\/signin(?:\?|$)/.test(hash);

/** The validated return target of a `#/signin?next=` address. */
export function oauthTargetFromHash(hash: string): string | null {
  const match = /^#\/signin\?(.*)$/.exec(hash);
  if (!match) return null;
  const next = new URLSearchParams(match[1]).getAll("next");
  return next.length === 1 ? safeOAuthTarget(next[0]) : null;
}

const signinHash = (target: string) =>
  signinPrefix + encodeURIComponent(target);

/** Seam for tests: jsdom cannot perform this navigation. */
export const browserNavigation = {
  replace: (target: string) => location.replace(target),
};

export function clearLoginReturn() {
  try {
    localStorage.removeItem(returnKey);
  } catch {
    /* Optional storage. */
  }
}

/** The normalized sign-in route to remember, or null for any other address. */
export function loginReturnHash(hash: string): string | null {
  const target = isSigninHash(hash) ? oauthTargetFromHash(hash) : null;
  return target ? signinHash(target) : null;
}

export function rememberLoginReturn(hash: string) {
  try {
    localStorage.setItem(
      returnKey,
      JSON.stringify({ hash, expires: Date.now() + returnLifetimeMs }),
    );
  } catch {
    /* Optional storage: sign-in still works and lands on Overview. */
  }
}

/** Only the exact form this module writes; anything else in storage is refused. */
function isStoredSignin(hash: string): boolean {
  const target = oauthTargetFromHash(hash);
  return target !== null && hash === signinHash(target);
}

/** Returns a validated sign-in route at most once; the stored value is always cleared. */
export function takeLoginReturn(): string | null {
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(returnKey);
  } catch {
    return null;
  }
  clearLoginReturn();
  if (!raw) return null;
  try {
    const { hash, expires } = JSON.parse(raw) as {
      hash?: unknown;
      expires?: unknown;
    };
    const now = Date.now();
    return typeof hash === "string" &&
      isStoredSignin(hash) &&
      typeof expires === "number" &&
      expires > now &&
      expires <= now + returnLifetimeMs
      ? hash
      : null;
  } catch {
    return null;
  }
}
