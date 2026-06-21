// Lightweight fetch-based API client with consistent error handling.

import { clearToken, getToken } from './token'

// Handler invoked when an authenticated request is rejected as unauthenticated
// (expired/invalid session). The router registers this to redirect to /login.
// It is NOT fired for anonymous requests (e.g. the login call itself), so a
// bad-credentials 401 surfaces to the caller instead of redirecting.
let unauthorizedHandler: (() => void) | null = null

export function setUnauthorizedHandler(fn: (() => void) | null): void {
  unauthorizedHandler = fn
}

export class ApiError extends Error {
  status: number
  /** True for "not implemented yet" backend responses (501/503). */
  notImplemented: boolean
  body: unknown

  constructor(message: string, status: number, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.notImplemented = status === 501 || status === 503
    this.body = body
  }
}

const BASE_URL = '/v1'

type QueryValue = string | number | boolean | undefined | null

export interface RequestOptions {
  query?: Record<string, QueryValue>
  signal?: AbortSignal
}

function buildUrl(path: string, query?: Record<string, QueryValue>): string {
  const url = `${BASE_URL}${path}`
  if (!query) return url
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') continue
    params.set(key, String(value))
  }
  const qs = params.toString()
  return qs ? `${url}?${qs}` : url
}

async function parseBody(res: Response): Promise<unknown> {
  const text = await res.text()
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  opts: RequestOptions = {},
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (token) headers['Authorization'] = `Bearer ${token}`

  let res: Response
  try {
    res = await fetch(buildUrl(path, opts.query), {
      method,
      headers: Object.keys(headers).length ? headers : undefined,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal: opts.signal,
    })
  } catch (err) {
    // Network failure (backend down, CORS, abort, etc.)
    throw new ApiError(
      err instanceof Error ? err.message : 'Network request failed',
      0,
    )
  }

  const parsed = await parseBody(res)

  if (!res.ok) {
    // An authenticated request rejected as unauthenticated means the session
    // expired or was revoked: drop the token and let the app redirect to login.
    // Anonymous 401s (e.g. bad login credentials) carry no token and fall
    // through to the caller so the form can show the error.
    if (res.status === 401 && token) {
      clearToken()
      unauthorizedHandler?.()
    }
    const message =
      (parsed && typeof parsed === 'object' && 'message' in parsed
        ? String((parsed as { message: unknown }).message)
        : undefined) ?? `Request failed with status ${res.status}`
    throw new ApiError(message, res.status, parsed)
  }

  return parsed as T
}

export const http = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, undefined, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('POST', path, body, opts),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('PUT', path, body, opts),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('PATCH', path, body, opts),
  delete: <T>(path: string, opts?: RequestOptions) =>
    request<T>('DELETE', path, undefined, opts),
}

/**
 * Generate a client-side confirmation id used for destructive operations.
 * The backend echoes/validates this to guard against accidental double-submits.
 */
export function newConfirmationId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `cfm-${Date.now()}-${Math.random().toString(16).slice(2)}`
}
