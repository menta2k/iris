// Pure mock router: compiles route patterns, matches a request, and dispatches to
// a handler. No Node/http types here so this module typechecks cleanly under the
// app tsconfig (strict) and is safe to import from vite.config.ts at load time.

import { routes } from './handlers'

export interface Query {
  [key: string]: string
}

export interface RouteCtx {
  /** Path params captured from `:name` segments, e.g. { id: '...' }. */
  params: Record<string, string>
  /** Flat query params (dotted keys like `page.page_size` survive verbatim). */
  query: Query
  /** Parsed JSON request body (undefined for GET/DELETE). */
  body: unknown
  /** Raw Authorization header value, or null when absent. */
  token: string | null
}

export interface MockResult {
  status: number
  body: unknown
}

export interface Route {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  /** Path pattern. A segment starting with `:name` is a param capturing one
   *  segment; any trailing text in that segment is literal. This matches both
   *  `/vmtas/:id` and gRPC-transcoder verbs like `/auth:login`, `/queues:action`,
   *  and the glued `/automation-rules/:id:status`. */
  pattern: string
  handler: (ctx: RouteCtx) => MockResult
}

// -- response helpers -------------------------------------------------------

export const ok = (body: unknown = {}): MockResult => ({ status: 200, body })
export const created = (body: unknown): MockResult => ({ status: 201, body })
export const noContent = (): MockResult => ({ status: 200, body: {} })
export const notFound = (message = 'Not found'): MockResult => ({
  status: 404,
  body: { message },
})
export const unauthorized = (): MockResult => ({
  status: 401,
  body: { message: 'Unauthenticated' },
})

// -- pattern compilation ----------------------------------------------------

function escapeRe(segment: string): string {
  return segment.replace(/[.+*?^${}()|[\]\\]/g, '\\$&')
}

function compile(pattern: string): RegExp {
  // Escape regex specials in each segment, then turn `:name` params (a `:`
  // immediately following a `/`) into named capture groups. A `:` NOT preceded
  // by `/` (e.g. `auth:login`, the `:status` in `:id:status`) stays literal.
  const segments = pattern.split('/')
  const built = segments
    .map((segment) => {
      if (segment.startsWith(':')) {
        const param = /^:(\w*)(.*)$/.exec(segment)
        const name = param?.[1] ?? 'param'
        const rest = escapeRe(param?.[2] ?? '')
        return `(?<${name}>[^/]+)${rest}`
      }
      return escapeRe(segment)
    })
    .join('/')
  return new RegExp(`^${built}$`)
}

// -- query / url parsing ----------------------------------------------------

function splitUrl(url: string): { pathname: string; search: string } {
  const q = url.indexOf('?')
  if (q === -1) return { pathname: url, search: '' }
  return { pathname: url.slice(0, q), search: url.slice(q + 1) }
}

export function parseQuery(search: string): Query {
  const params: Query = {}
  if (!search) return params
  for (const pair of search.split('&')) {
    if (!pair) continue
    const eq = pair.indexOf('=')
    const key = eq === -1 ? pair : pair.slice(0, eq)
    const value = eq === -1 ? '' : pair.slice(eq + 1)
    params[decodeURIComponent(key)] = decodeURIComponent(value.replace(/\+/g, ' '))
  }
  return params
}

// -- dispatch ---------------------------------------------------------------

const compiledRoutes = routes.map((route) => ({
  method: route.method,
  regex: compile(route.pattern),
  handler: route.handler,
}))

export function dispatch(
  method: string,
  url: string,
  body: unknown,
  token: string | null,
): MockResult {
  const { pathname, search } = splitUrl(url)
  // Route patterns are written without the `/v1` BASE_URL prefix (matching the
  // service paths), so strip it before matching.
  const path = pathname.replace(/^\/v1(?=\/|$)/, '') || '/'
  const query = parseQuery(search)
  for (const route of compiledRoutes) {
    if (route.method !== method) continue
    const match = route.regex.exec(path)
    if (!match) continue
    const ctx: RouteCtx = {
      params: match.groups ?? {},
      query,
      body,
      token,
    }
    try {
      return route.handler(ctx)
    } catch (err) {
      return {
        status: 500,
        body: {
          message: `Mock handler error: ${err instanceof Error ? err.message : String(err)}`,
        },
      }
    }
  }
  return { status: 404, body: { message: `No mock route for ${method} ${pathname}` } }
}
