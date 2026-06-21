// Session-token storage. Kept in a tiny standalone module (not in useAuth) so
// the HTTP client can read it without importing the composable — avoiding an
// import cycle (http -> useAuth -> services -> http).

const STORAGE_KEY = 'iris_session_token'

function read(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

let current: string | null = read()

export function getToken(): string | null {
  return current
}

export function setToken(token: string): void {
  current = token
  try {
    localStorage.setItem(STORAGE_KEY, token)
  } catch {
    // Storage unavailable (private mode / SSR); the in-memory value still works
    // for the current page session.
  }
}

export function clearToken(): void {
  current = null
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
}
