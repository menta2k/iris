# iris mock API

A dev-only mock of the entire `/v1/*` API so the frontend can be developed and
polished **without the Go backend running**. Zero new dependencies; never affects
production builds.

## How it works

A Vite plugin (`mockApiPlugin` in `vite.config.ts`) installs a connect middleware
in `configureServer`. Every request to `/v1/*` is matched against the route table
in `router.ts` and dispatched to a handler in `handlers/`. Handlers read/write an
in-memory database (`db.ts`) seeded from `fixtures/`. Everything outside `/v1/*`
falls through to Vite unchanged (SPA, assets, HMR).

The HTTP glue (reading the request body, writing the JSON response) lives in
`vite.config.ts` so the rest of `src/mocks/` stays free of Node types and
typechecks cleanly under the app `tsconfig`. **All files here use relative
imports** (not the `@/` alias) because they're loaded at Vite config-load time,
before the alias is registered.

## Enable / disable

Controlled by the `VITE_MOCK` env var. On by default for `vite dev` via
`.env.development`:

```bash
pnpm dev                       # mock ON (no backend needed)
VITE_MOCK=false pnpm dev       # proxy to http://localhost:8080 instead
```

`vite build` is unaffected — `configureServer` never runs during a build, and the
mock files aren't imported by the app entry, so they're excluded from the bundle.

## Auth

Any email + password logs in as the seeded admin (`admin@iris.local`, `owner`
role, all permissions). MFA is skipped — login is a single step. The token is
stored in `localStorage` (`iris_session_token`) and persists across reloads, so
after the first login the app boots straight into the dashboard.

To start over: clear site data (or restart `pnpm dev` to reseed the in-memory DB).

## Layout

```
router.ts        pattern DSL + dispatch + response helpers (ok/notFound/…)
db.ts            seeded in-memory store + immutable CRUD + cursor paging
fixtures/        seed data + generators, one file per domain
handlers/        route handlers, one file per domain; handlers/index.ts aggregates
```

## Notes

- Lists paginate via the API's opaque `page.page_size` / `page.page_token` tokens
  (cursor = `mock:<offset>`); big lists (mail records, bounces, audit) have enough
  rows to exercise the pagination controls.
- Mutations (create/update/delete/status/pause/resume) change the in-memory store,
  so the UI reflects them live until the dev server restarts.
- Unmatched `/v1/*` requests return `404 { message }` so gaps surface visibly
  rather than silently rendering empty pages.
