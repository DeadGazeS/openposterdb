/** Extract the `{"error": ...}` message from a failed JSON response. */
export async function parseApiError(res: Response, fallback = 'Request failed'): Promise<string> {
  try {
    const data = await res.json()
    return (data && typeof data.error === 'string' && data.error) || fallback
  } catch {
    return fallback
  }
}

/** Throw if the response is not 2xx, otherwise return its JSON body. Replaces
 *  the repeated `if (!res.ok) throw new Error(msg); return res.json()` idiom
 *  at the 6+ view call sites. */
export async function okOrThrow<T = unknown>(res: Response, msg: string): Promise<T> {
  if (!res.ok) throw new Error(msg)
  return (await res.json()) as T
}
