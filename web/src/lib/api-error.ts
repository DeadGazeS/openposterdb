/** Extract the `{"error": ...}` message from a failed JSON response. */
export async function parseApiError(res: Response, fallback = 'Request failed'): Promise<string> {
  try {
    const data = await res.json()
    return (data && typeof data.error === 'string' && data.error) || fallback
  } catch {
    return fallback
  }
}
