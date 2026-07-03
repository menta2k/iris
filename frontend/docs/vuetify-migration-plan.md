# Frontend Migration Plan — Tailwind → Vuetify 3 (Vuexy look & feel)

Re-skin the iris KumoMTA admin UI to the **Vuexy** look & feel — a Material component
library, an organized collapsible menu, and a live settings drawer — **incrementally**,
with the app shipping the whole way through.

- **From:** Vue 3 + TS + Vite, Tailwind 3 + hand-rolled `cva`/`clsx` components (`components/ui/*`), lucide icons, vue-router, **no Pinia**, echarts.
- **To:** Vuetify 3, primary `#7367F0` (Vuexy palette).
- **Strategy:** strangler-fig, shell-first.

> Interactive version of this plan (palette swatches, code, tables):
> https://claude.ai/code/artifact/b5a7c4bd-f908-4374-83bc-1d3a22aa2abc

---

## Three commitments that make this safe

A design-system swap fails when it becomes a months-long branch that can't ship. These keep it mergeable at every step.

1. **Strangler-fig, not big-bang.** Vuetify and Tailwind run side-by-side. The new shell wraps existing pages; pages convert one domain at a time. `main` stays releasable throughout.
2. **Shell & theme first.** The three headline asks — organized menu, Vuexy colors, settings drawer — all live in the shell. Land them in the first two phases, before touching a data table.
3. **Rebuild, don't copy Vuexy.** Vuexy is proprietary. Use the MIT-licensed **Materio** (same vendor, same stack) as the reference to rebuild the drawer & layout — clean-room, no license risk.

---

## Target palette

Lifted verbatim from `~/projects/vuexy-vuejs-admin-template/typescript-version/full-version/src/plugins/vuetify/theme.ts`. These become the Vuetify theme tokens in Phase 0.

| Token         | Hex       | Token          | Hex       |
| ------------- | --------- | -------------- | --------- |
| primary       | `#7367F0` | warning        | `#FF9F43` |
| primary-d1    | `#675DD8` | error          | `#FF4C51` |
| success       | `#28C76F` | background      | `#F8F7FA` |
| info          | `#00BAD1` | on-surface     | `#2F2B3D` |

Border color `#2F2B3D` @ opacity `0.12`; surface `#fff`.

---

## The work — 6 phases

Phases 0–2 deliver everything asked for. Phases 3–5 are the long tail — converting
components and pages behind the already-shipped new look, then deleting Tailwind.

> **Progress (2026-07-03): ALL PHASES COMPLETE** on branch `vuetify` — Tailwind,
> `class-variance-authority`, `clsx`, `tailwind-merge` and `lucide-vue-next` removed;
> the app is 100% Vuetify 3. Notable execution deltas: cascade layers reverted (Chromium bug — see P0
> note); `Select` stayed native through P3/P4 because component tests drive real
> `HTMLSelectElement`s, converted together with their tests in P5; Toaster kept custom
> (v-snackbar doesn't stack); Mail Logs detail drawer needs `disable-route-watcher`
> (temporary drawers close on any route change, including the `?record=` deep-link
> `router.replace`).

### P0 — Foundations & coexistence (~2–3 days)

**Goal:** Install Vuetify 3 next to Tailwind and make them stop fighting — *without*
visually changing any existing page yet.

- Add `vuetify@^3`, `@mdi/font`, `vite-plugin-vuetify`; wire `createVuetify` in a new `plugins/vuetify/index.ts`.
- Add `pinia` (none today) — it backs the settings store in P2.
- **Disable Tailwind Preflight** and order both systems with CSS cascade layers.
- Port the Vuexy palette into `theme.ts` as the `light`/`dark` Vuetify themes.
- Smoke-test: an existing Tailwind page and a throwaway `<v-btn>` render correctly on the same route.

```js
// tailwind.config.js — stop Preflight resetting the whole page
module.exports = {
  // Vuetify already ships a reset; two global resets collide.
  corePlugins: { preflight: false },
}
```

```css
/* styles: Vuetify's official cascade-layer order (RFC #22443).
   Tailwind utilities win over Vuetify components, but Vuetify's
   transitions/a11y overrides in `vuetify-final` still win last. */
@layer tailwind-theme, tailwind-reset,
       vuetify-core, vuetify-components, vuetify-overrides,
       vuetify-utilities, tailwind-utilities, vuetify-final;
```

> ⚠️ **This is the #1 make-or-break step.** Preflight strips margins, heading sizes,
> and list bullets page-wide; left on, it silently mangles Material typography.
> Confirm the exact `@layer` names against your installed Vuetify version — they've
> shifted between releases.

> ❌ **Deviation (2026-07, implemented):** cascade layers were tried and **reverted**.
> With Vuetify 3.12 compiled via `$layers: true`, Chromium (tested v149) never
> resolves `var(--v-layout-*)` inside the layered `.v-main` rule against the inline
> custom properties Vuetify's layout engine writes — `v-main` loses its app-bar and
> drawer offsets (forcing a style invalidation fixes it, confirming a browser
> invalidation bug, not a cascade-order issue). Coexistence instead relies on:
> **(1)** Preflight off, **(2)** source order — Vuetify styles imported before
> `style.css` so Tailwind utilities win ties, **(3)** theme `unimportant: true` plus
> **aligning the Tailwind CSS-variable palette to the same Vuexy tokens**, so the
> colliding class names both systems generate (`bg-primary`, `bg-background`, …)
> render identical colors regardless of which wins.

### P1 — App shell & organized menu (~4–6 days)

**Goal:** Replace `AdminLayout.vue` + `SidebarNav.vue` with a Vuetify layout. Existing
pages render unchanged inside `<v-main>`.

- `AdminLayout` → `v-app` ▸ `v-navigation-drawer` + `v-app-bar` + `v-main`.
- Drive drawer `permanent` (desktop) vs `temporary` (mobile) off the `useDisplay()` composable.
- Extend the existing `nav-items.ts` schema with `icon` (MDI) and optional `children` — the menu data model is already clean and permission-gated, so this is additive.
- Render with `v-list` ▸ `v-list-subheader` / `v-list-group` / `v-list-item`, keeping the current permission filter.
- Move the topbar bits (timezone picker, user menu, drift banner) into `v-app-bar`.

```ts
// nav-items.ts — additive: icons + collapsible groups
export interface NavItem {
  label: string; to?: string; icon?: string   // e.g. 'mdi-send'
  permission?: Permission
  children?: NavItem[]                          // renders as a v-list-group
}
```

> 🔧 **Verify nav specifics against the official docs.** Research surfaced two *refuted*
> blog patterns: the `v-model`-on-`v-list-group` expansion trick and the rail
> "expand-on-hover" recipe. Also the v2 `app` prop is **gone** in v3 (the drawer
> auto-registers with `v-app`). Build sub-menus and rail mode from
> https://vuetifyjs.com/en/components/navigation-drawers/, not tutorials.

### P2 — The settings drawer ("customizer") (~3–5 days)

**Goal:** A right-hand drawer for theme mode, dynamic primary color, and skin —
persisted across reloads, Vuexy-style.

- Build a `useConfigStore` (Pinia) holding `themeMode` (light/dark/system), `primaryColor`, `skin`.
- Persist to `localStorage` with Vuexy's precedence: **stored value → config default → markup default**.
- Drive Vuetify at runtime via `useTheme()` — switch theme name for light/dark, mutate `colors.primary` for the live color picker.
- Respect `prefers-color-scheme` when mode is "system".

```ts
// Runtime theming — light/dark + dynamic primary via useTheme()
const theme = useTheme()
// light/dark: flip the active theme name
theme.global.name.value = isDark ? 'dark' : 'light'
// dynamic primary: mutate the live theme's color token
theme.themes.value[theme.global.name.value].colors.primary = picked
```

> ✅ **End of P2 = the stated goals are done.** Organized menu ✓, Vuexy colors ✓,
> settings drawer ✓ — all shipped, with every page still working on Tailwind underneath.

### P3 — Shared component swap (~1 sprint)

**Goal:** Replace the hand-rolled `components/ui/*` primitives (built on `cva`/`clsx`)
with Vuetify equivalents, one primitive at a time.

- Map & migrate: `ui/button`→`v-btn`, `toast`→`v-snackbar`, dialogs→`v-dialog`, inputs→`v-text-field`/`v-select`, tabs/menus/cards likewise.
- Keep the same import paths where possible (thin wrappers) so pages don't churn.
- Keep **echarts** as-is — framework-agnostic; only restyle its container.

### P4 — Page-by-page conversion (2–3 sprints)

**Goal:** Convert the ~30 pages by domain folder, lowest-risk first, deleting Tailwind
classes as each page flips to Vuetify components.

- Order by blast radius: *Tools → Security → Domain Safety → Inbound → Outbound → Operations* (the big data tables) last.
- Heaviest lift: Mail Logs / DMARC tables → `v-data-table-server`, reusing the existing `usePagedList` server pagination.
- Each page is its own PR; the drift banner & permissions carry over untouched.

### P5 — Remove Tailwind & retire the layers (~2 days)

**Goal:** Once no page imports Tailwind classes, delete the scaffolding needed only for
coexistence.

- Drop `tailwindcss`, `class-variance-authority`, `clsx`, `tailwind-merge` from deps.
- Remove the cascade-layer declaration and the old `ui/` folder.
- Final visual-regression pass across all domains in light & dark.

---

## "Better organized menu" — concrete proposal

Today the sidebar is eight flat, icon-less sections, and the *KumoMTA* group is a
grab-bag. Vuetify's `v-list-group` lets you collapse, icon, and regroup it without
changing routes.

**Now — flat, icon-less**

- Overview
- Outbound Config *(7 items)*
- Operations *(8 items)*
- KumoMTA *(Config, Global Settings, Subject Classifications, Feedback Loops, TLS Certs, Domain Readiness…)*
- Domain Safety · Inbound · Tools · Security

**Proposed — grouped, iconed, collapsible**

- **Dashboard** — `mdi-view-dashboard`
- **Sending** ▸ Listeners, VMTAs, Groups, Routing, Warmup, Blueprints, Automation
- **Monitoring** ▸ Mail Logs, Bounces, Feedback, DMARC, Queues, Worker Errors
- **Configuration** ▸ KumoMTA Config, Global Settings, Classifications, Feedback Loops
- **Deliverability** ▸ DKIM, TLS Certs, Require-TLS, Suppressions, Domain Readiness
- **Inbound · Tools · Access** (collapsed by default)

> 💡 **Why this regroups cleanly:** it moves the scattered *KumoMTA* items into
> intent-based groups (things you *configure* vs. things that keep you *deliverable*),
> and pulls TLS Certs + Domain Readiness next to DKIM where operators expect them. Pure
> presentation — every `to:` route and permission stays identical.

---

## Layout usability & data-table UX

Runs *alongside* P1–P4 (not a separate phase). The driving problem: read-heavy wide
tables — **Mail Logs has 11 columns** (Time, Message ID, Mailclass, From, envelope
Sender, Recipient, VMTA, Status, Type, Class, Reason) — are hard to scan and have no way
to see a full record. Below is a verified decision framework and how it maps onto iris.

### Detail-view decision framework

The evidence (Adobe Commerce pattern library, NN/G, Carbon, Smashing/Pencil&Paper)
converges on:

| Pattern | Use when | Avoid when |
| ------- | -------- | ---------- |
| **Right-hand detail drawer** *(default for inspection)* | Viewing one wide, read-heavy record; needs subtabs/lots of fields; operator keeps table context and compares across rows | — |
| **Modal dialog** | Concise, self-contained task or confirmation (edit form, delete) | Operator needs to **compare/reference other rows** — the modal blocks the table |
| **Expandable / master-detail row** | Preserving *full* table context is paramount; a few extra fields | Record needs lots of space; deep-linking a selection |
| **Dedicated full page** | Complex multi-step process | Quick look-up / inspection |

> **Key takeaway for iris:** Mail Logs and DMARC are *inspection + compare* workflows, so
> the default is a **right-hand detail drawer**, not a modal. Modals block the table,
> which is exactly wrong when an operator is scanning/correlating records.

### Per-table recommendation (grounded in current pages)

| Page / table | Detail pattern | Rationale |
| ------------ | -------------- | --------- |
| **Mail Logs** (11 cols) | **Right detail drawer** + column-visibility menu + density toggle | Read-heavy, operators compare rows; drawer shows the full record **plus the message's lifecycle** (all rows sharing `message_id`) |
| **DMARC Reports** | **Right detail drawer** (report → per-source records) | Aggregate report has nested child records |
| **Bounces / Worker Errors** | **Expandable row** or drawer | Long `Reason` / DSN / stack-trace payload |
| **DKIM · Inbound Routes · Suppressions** (edit) | **Keep modal dialog** | Self-contained *edit task* — correct per the framework; these already use modals |
| **Queues / small tables** | No detail view | Fit on screen as-is |

> The Mail Logs drawer showing "all rows with this `message_id`" is the same correlation
> the backend does per-event — it motivates the **`message_id` index** flagged in the
> backend audit (currently a full hypertable scan). Worth landing that index alongside.

### Vuetify 3 implementation

```vue
<!-- Right detail drawer wired to row-click (v-data-table-server) -->
<v-data-table-server :headers="visibleHeaders" :items="rows" @click:row="onRowClick" />
<v-navigation-drawer v-model="detailOpen" location="right" temporary width="480"
  role="dialog" aria-label="Mail record detail">
  <MailRecordDetail v-if="selected" :record="selected" />
</v-navigation-drawer>
```

```vue
<!-- Expandable master-detail row (Bounces/Worker Errors) -->
<v-data-table :headers="headers" :items="rows" show-expand expand-strategy="single"
  item-value="id">
  <template #expanded-row="{ columns, item }">
    <tr><td :colspan="columns.length"><ErrorDetail :item="item" /></td></tr>
  </template>
</v-data-table>
```

- Two expand slots exist: `expanded` (auto-wrapped in one `colspan`-all `<td>`) vs
  `expanded-row` (you supply the `<tr>/<td>` — use this for multi-column detail).
- `expand-strategy="single"` keeps only one row open.

### Managing many columns

- **Column visibility toggle** is **not** built into Vuetify — implement by filtering the
  `headers` array from a checkbox menu (`v-menu` + `v-list` of `v-checkbox`). Persist the
  choice in the Pinia config store (same store as the settings drawer).
- **Responsive priority columns:** drop low-priority columns below a breakpoint via
  `useDisplay()` (e.g. hide Type/Class/Sender on `smAndDown`).
- **Density modes:** offer compact/comfortable via Vuetify's `density` prop (Carbon ships
  five row sizes; expose at least compact ↔ default).
- **Sticky header** (`fixed-header` + height) so headers persist while scrolling long logs.
- Prefer **horizontal scroll of the full table** over stacked cards for operator
  log-scanning; keep the wide table in an `overflow-x:auto` container.

### Empty & loading states (NN/G)

Never render a blank table on zero results — it's ambiguous between *empty*, *loading*,
and *errored*. Use explicit copy in Vuetify's `no-data` slot, e.g.
**"No mail records for the selected filters and date range."** Distinguish loading
(`loading` prop → skeleton) from empty from error.

### Accessibility & deep-linking (W3C APG · MDN · Vue Router)

- **Drawer/dialog as detail:** `role="dialog"`, accessible name via `aria-labelledby`
  (or `aria-label`); **trap focus** inside and **return focus** to the triggering row on
  close; **Esc closes** (allow a guard when there's unsaved edit data). Set
  `aria-modal="true"` only when outside content is genuinely inert.
- **Table keyboard nav:** the roving-tabindex "single tab stop + arrow keys" pattern
  applies to *interactive* `role="grid"` tables — a read-only log table (`role="table"`)
  does **not** need it; keep row actions as normal tab stops.
- **Deep-link the selection:** sync the selected record id to the URL (route param or
  `?id=`), and react with a `watch(route.params)` / `beforeRouteUpdate` (same-route param
  changes reuse the component and skip lifecycle hooks). Consider
  `vue-datatable-url-sync` to also persist filters/sort/pagination — makes a filtered
  Mail Logs view **shareable and back-button friendly**. (Back-restore depends on
  `push` vs `replace` history mode.)

---

## Risks & gotchas, ranked

| Risk | Sev | Mitigation |
| ---- | --- | ---------- |
| Tailwind Preflight vs Vuetify reset — silent page-wide typography/spacing breakage | High | Disable Preflight (P0); rely on cascade layers, not `!important`. |
| Copying Vuexy `@core`/customizer code — proprietary license ($39 / $799) | High | Rebuild from MIT-licensed **Materio** reference; don't paste Vuexy source. |
| Nav sub-menu / rail patterns from blogs outdated or refuted (`app` prop gone, `v-model` group trick) | Med | Build nav strictly from official Vuetify 3 docs; spike it in P1. |
| Residual coexistence conflicts — `box-sizing`, focus outlines, overlay z-index | Med | Scope with `tailwindcss-scoped-preflight` if needed; audit overlays. |
| Data-table migration (Mail Logs/DMARC) is the heaviest lift | Med | Do last (P4); reuse `usePagedList` under `v-data-table-server`. |
| Introducing Pinia where there was none | Low | Scope it to the config store first; leave `useAuth` composable as-is. |
| Don't upgrade Tailwind to v4 mid-migration | Low | v4 removes `corePlugins`; stay on 3 until Tailwind is removed entirely. |

---

## Effort estimate

Scaled from a published design-system migration (≈16 tasks · 3–5 per sprint · ≈3.5
sprints) to iris's ~30 pages. Planning baseline, not a guarantee — verify against team
velocity.

| Phase | Scope | Est. | Ships value? |
| ----- | ----- | ---- | ------------ |
| P0 Foundations | Vuetify + Pinia + coexistence | 2–3 d | Internal |
| P1 Shell & menu | Layout + organized nav | 4–6 d | **Yes** ✦ |
| P2 Settings drawer | Customizer + theming | 3–5 d | **Yes** ✦ |
| P3 Components | ui/* → Vuetify primitives | ~1 sprint | Incremental |
| P4 Pages | ~30 pages by domain | 2–3 sprints | Incremental |
| P5 Cleanup | Remove Tailwind + layers | ~2 d | Tech debt |
| **Total (alongside normal feature work)** | | **~4–5 sprints** | — |

✦ Everything explicitly asked for is live by the end of P2 (~2 weeks of focused work).

---

## Sources

Verified via multi-vote adversarial checking (22 of 25 claims confirmed; 3 blog patterns
refuted and excluded above).

- [Vuetify + TailwindCSS — official coexistence & cascade layers](https://vuetifyjs.com/en/features/css-utilities/tailwindcss/) *(primary)*
- [Tailwind v3 — Preflight & disabling it](https://v3.tailwindcss.com/docs/preflight) *(primary)*
- [Vuetify 3 — Theme API & `useTheme`](https://vuetifyjs.com/en/features/theme/) *(primary)*
- [Vuetify 3 — Navigation drawers](https://vuetifyjs.com/en/components/navigation-drawers/) *(primary)*
- [Materio — MIT Vuetify 3 admin template (rebuild reference)](https://github.com/themeselection/materio-vuetify-vuejs-admin-template-free) *(primary)*
- [Vuexy — license terms ($39 / $799)](https://themeforest.net/item/vuexy-vuejs-html-laravel-admin-dashboard-template/23328599) *(primary)*
- [tailwindcss-scoped-preflight — scope the reset](https://github.com/Roman86/tailwindcss-scoped-preflight) *(primary)*
- [LogRocket — building dynamic Vuetify themes](https://blog.logrocket.com/building-dynamic-vuetify-themes/) *(blog)*
- [Cordova — lessons migrating to a design system (effort baseline)](https://dev.to/victorandcode/lessons-from-migrating-a-web-application-to-a-design-system-2701) *(blog)*

**Layout & data-table UX (second research pass — 24 of 25 claims confirmed):**

- [Adobe Commerce — slideouts, modals & overlays pattern library](https://developer.adobe.com/commerce/admin-developer/pattern-library/containers/slideouts-modals-overlays) *(primary)*
- [Vuetify 3 — Data tables (expand slots, density)](https://vuetifyjs.com/en/components/data-tables/) *(primary)*
- [Carbon Design System — Data table usage (row sizes/density)](https://carbondesignsystem.com/components/data-table/usage/) *(primary)*
- [NN/G — Empty state interface design](https://www.nngroup.com/articles/empty-state-interface-design/) *(primary)*
- [NN/G — Modal vs non-modal dialogs](https://www.nngroup.com/articles/modal-nonmodal-dialog/) *(primary)*
- [W3C ARIA APG — Dialog (modal) pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/) *(primary)*
- [W3C ARIA APG — Developing a keyboard interface (roving tabindex)](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/) *(primary)*
- [MDN — ARIA dialog role](https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Reference/Roles/dialog_role) *(primary)*
- [Vue Router — Dynamic route matching (deep-linking selection)](https://router.vuejs.org/guide/essentials/dynamic-matching) *(primary)*
- [vue-datatable-url-sync — sync filters/sort/pagination to URL](https://github.com/socotecio/vue-datatable-url-sync) *(primary)*
- [Pencil&Paper — UX pattern analysis of enterprise data tables](https://www.pencilandpaper.io/articles/ux-pattern-analysis-enterprise-data-tables) *(secondary)*
- [Smashing Magazine — modal vs separate page decision tree](https://www.smashingmagazine.com/2026/03/modal-separate-page-ux-decision-tree/) *(secondary)*
