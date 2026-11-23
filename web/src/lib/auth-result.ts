// Outcome of a login, key login or setup request. The backend answers these
// with distinct status codes; the views turn the reason into a message.
export type AuthFailureReason =
  | 'unauthorized' // 401: wrong credentials or unknown key
  | 'invalid' // 400: validation failed (message: the server's text)
  | 'forbidden' // 403: free key disabled / setup already completed
  | 'rate-limited' // 429, with Retry-After
  | 'busy' // 503, with Retry-After
  | 'server' // 500 and anything else unexpected
  | 'network' // no response at all

export type AuthResult =
  | { ok: true }
  | { ok: false; reason: AuthFailureReason; message?: string; retryAfter?: number }

export type AuthFailure = Extract<AuthResult, { ok: false }>

// Refresh outcome: only 'unauthorized' (a 401) ends the session; a server
// error, busy server, rate limit or network error says nothing about it.
export type RefreshResult = 'refreshed' | 'unauthorized' | 'unavailable'

/** Retry-After in whole seconds (the backend sends the delta-seconds form). */
function retryAfterSecs(res: Response): number | undefined {
  const raw = res.headers?.get('Retry-After')
  if (!raw || !/^\d+$/.test(raw.trim())) return undefined
  return Number(raw.trim())
}

async function errorMessage(res: Response): Promise<string | undefined> {
  try {
    const data = await res.json()
    return data && typeof data.error === 'string' && data.error ? data.error : undefined
  } catch {
    return undefined
  }
}

/** Classify a non-ok response from an auth endpoint. */
export async function authFailure(res: Response): Promise<AuthFailure> {
  const withRetry = (reason: AuthFailureReason): AuthFailure => {
    const secs = retryAfterSecs(res)
    return secs === undefined ? { ok: false, reason } : { ok: false, reason, retryAfter: secs }
  }
  const withMessage = async (reason: AuthFailureReason): Promise<AuthFailure> => {
    const message = await errorMessage(res)
    return message === undefined ? { ok: false, reason } : { ok: false, reason, message }
  }
  switch (res.status) {
    case 401:
      return { ok: false, reason: 'unauthorized' }
    case 400:
      return withMessage('invalid')
    case 403:
      return withMessage('forbidden')
    case 429:
      return withRetry('rate-limited')
    case 503:
      return withRetry('busy')
    default:
      return { ok: false, reason: 'server' }
  }
}

function retryIn(secs: number | undefined): string {
  return secs === undefined ? 'Try again shortly.' : `Try again in ${secs} s.`
}

/** Message for the failures every auth form words the same way; the
 *  form-specific ones (401, 400, 403) are left to the caller. */
export function commonAuthMessage(r: AuthFailure): string | null {
  switch (r.reason) {
    case 'rate-limited':
      return `Too many attempts. ${retryIn(r.retryAfter)}`
    case 'busy':
      return `The server is busy. ${retryIn(r.retryAfter)}`
    case 'server':
      return 'Server error. Try again later.'
    case 'network':
      return 'Could not reach the server. Check your connection and try again.'
    default:
      return null
  }
}
