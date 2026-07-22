# Kontor & DocMind — Design System

A shared design system for **Kontor & DocMind**, a family of self-hosted AI automation products for service businesses:

- **Kontor** — an AI agent that books appointments and acts inside the CRM.
- **DocMind** — a RAG assistant that answers strictly from a company's own documents.

The system covers three surfaces in one visual language: an **embeddable customer chat widget**, **dense operator dashboards** (calendar, agent-trace timeline, analytics), and **marketing landing pages**. Audience: small business owners and technical buyers. Tone: precise, engineering-grade, calm — closer to Linear and Vercel than to a playful consumer app.

## Sources
This system was authored from a written brand brief only. **No codebase, Figma file, logo, or font binaries were provided.** Where a real asset would normally be lifted from source, a documented substitution was made (see Caveats). If you have the real brand assets, drop them in and re-run the compiler.

---

## Content fundamentals
How Kontor & DocMind writes copy.

- **Voice:** precise and calm. Short declaratives. State what the product does, not how it feels. *"Answers from your docs."* not *"Amazing AI-powered answers!"*
- **Person:** address the customer as **you** ("book a visit", "your documents"); refer to the product by name (Kontor, DocMind) rather than "we" in-product. Marketing may use "we" sparingly.
- **Casing:** sentence case everywhere — buttons, headings, menu items ("New agent", not "New Agent"). Reserve UPPERCASE for micro eyebrow labels (11px, 0.06em tracking).
- **Numbers & IDs:** always monospace — run ids (`run_7f3a9c`), timestamps (`2026-07-20T14:32Z`), token counts, latencies (`1.24s`), confidence (`0.96`), JSON. This is a load-bearing part of the aesthetic, not decoration.
- **Tone toward errors:** factual and actionable. Name what failed and give the next step; surface a trace id for technical buyers. Never cute, never blaming.
- **No emoji.** Not in product, not in marketing. Status is carried by the semantic color set + dot, never an emoji.
- **Examples:** "Agent online" · "Needs review" · "Grounded in Acme Plumbing docs" · "Couldn't load traces — the observability service didn't respond. (504 · trace 7f3a…c21)".

---

## Visual foundations

- **Theme:** dark by default (`:root`), light as a variant (`[data-theme="light"]`). The dark base is a **deep blue-grey (`#0a0d12`), never pure black**, with several distinct surface levels rising in tone with elevation (`--bg-base` → `--surface-1/2/3` → `--surface-hover`).
- **Color:** one accent — **Iris**, a calm periwinkle-indigo (`--accent #6e78f0`, hover `#8a92ff`, active `#5b64d6`) — plus a strict semantic set: success (green `#3fb95a`), warning (amber `#e3b341`), error (red `#f0616d`), and a neutral/info alias. No other hues. Max one or two background tones per screen.
- **Type:** a single variable grotesk (**Geist**) carries the design. Large headings use **tight negative tracking** (−0.02em to −0.03em); a strict scale from 72px display down to 11px micro-labels; generous whitespace. **Geist Mono** for all IDs, timestamps, counts and JSON.
- **Backgrounds:** flat surface tones. No photography, no illustration, no gradients in-product. Marketing pages may use extremely subtle two-color radial glows (iris + teal at ~6-8% opacity) — never loud gradients.
- **Depth:** comes from **surface tone + 1px borders**, essentially no shadow. `--border-subtle / -default / -strong` are the primary depth cue. Shadows (`--shadow-overlay`) appear only on elements that truly float — menus, dialogs, the chat panel, toasts.
- **Corners:** moderate radii — controls `7px` (`--radius-md`), cards/panels `10px` (`--radius-lg`), large surfaces `14px`, pills/avatars full. Nothing sharp, nothing bubbly.
- **Motion:** restrained. **150–250ms, ease-out** (`cubic-bezier(.22,1,.36,1)`), on **state changes only** — hover, focus, open/close, tab indicator. No bounces, no parallax, no entrance animations on load. Everything respects `prefers-reduced-motion` (globally neutralized in `motion.css`).
- **Hover / press:** hover raises the surface one tone (`--surface-hover`) or the border one step; primary buttons shift to the lighter accent. Press goes one tone darker. No scale/shrink transforms on buttons.
- **Focus:** 2px accent outline (`:focus-visible`) or a 3px `--focus-ring` glow on inputs.
- **Loading:** **skeletons, never spinners**, for content. Skeletons mirror the final layout and shimmer at 1.4s (static tint under reduced-motion). The only spinner is the inline one inside a `loading` Button.
- **Density:** Linear-like — compact rows (13px), tight table padding — but never cramped; generous outer whitespace balances the dense data.
- **Cards:** `--surface-1` fill, 1px `--border-default`, `10px` radius, no shadow; interactive cards raise border contrast on hover.
- **Transparency / blur:** used sparingly — semantic badge backgrounds are `color-mix` tints of their hue; not a glassmorphism system.

---

## Iconography
- **Set:** [Lucide](https://lucide.dev) is the canonical icon set — 24×24, 2px stroke, round caps/joins. It matches the thin, engineering aesthetic. **In production use `lucide-react`.**
- **Substitution:** no brand icons were provided, so `assets/icons.js` ships a curated inline mirror of the Lucide glyphs the kits need (Home, Inbox, Calendar, Activity, Bot, Send, Shield, Database, …), exposed as `window.KKIcons`. They recolor via `currentColor`. Swap for real `lucide-react` imports in a real app.
- **No emoji, no unicode-as-icon.** Status uses a colored dot + Badge, not glyphs.
- **Sizing:** 16px in dense UI (sidebar, buttons, table), 18–20px in headers and empty/error states.

---

## Tokens
All tokens are CSS custom properties, shipped via `styles.css` (an `@import` list only). Base ramps (`--grey-*`, `--iris-*`, `--green/amber/red-*`) plus semantic aliases (`--bg-base`, `--surface-*`, `--text-primary/secondary/tertiary`, `--accent`, `--status-*`, `--border-*`, `--radius-*`, `--space-*`, `--dur-*`, `--ease-*`). Files: `tokens/{fonts,colors,typography,spacing,radius,motion,base}.css`.

### Tailwind
`tailwind.config.js` (root) maps the full semantic token set into `theme.extend` — colors, `fontSize` (paired with `lineHeight`/`letterSpacing`), `spacing`, `borderRadius`, `boxShadow`, `transitionDuration`, `transitionTimingFunction` — each pointing at the live CSS variable, so a token edit in `tokens/*.css` propagates without touching the config. Class names use semantic names, not the raw ramp:

```html
<!-- before: hand-rolled inline styles -->
<div style="background:var(--surface-1);border:1px solid var(--border-default);border-radius:10px;padding:16px">
  <div style="font-size:15px;font-weight:600;letter-spacing:-0.005em;color:var(--text-primary)">Bookings today</div>
  <div style="font-size:13px;color:var(--text-tertiary);margin-top:4px">Live · updated 2m ago</div>
</div>

<!-- after: Tailwind utilities against the same tokens -->
<div class="bg-surface-1 border border-border-default rounded-lg p-4">
  <div class="text-body font-semibold text-text-primary">Bookings today</div>
  <div class="text-body-sm text-text-tertiary mt-1">Live · updated 2m ago</div>
</div>
```

---

## Components
Reusable React primitives (`window.KontorKanonDesignSystem_452420.*`). Grouped by concern under `components/`:

- **core/** — **Button**, **IconButton**, **Card** (+ **CardHeader**), **Badge**
- **forms/** — **Input**, **Select**, **Checkbox**, **Switch**
- **navigation/** — **Sidebar** (+ **SidebarSection**, **SidebarItem**), **Tabs**
- **data/** — **DataTable**, **KeyValue** (+ **KeyValueList**), **CodeBlock**
- **feedback/** — **Skeleton** (+ **SkeletonText**, **SkeletonRow**), **EmptyState**, **ErrorState**
- **overlays/** — **Drawer**, **Modal**, **Toast** (+ **ToastStack**), **Tooltip**
- **identity/** — **Avatar**, **AvatarGroup**
- **structure/** — **Timeline** (+ **TimelineSkeleton**) and **Calendar** (**WeekCalendar**, **DayCalendar**, **MonthPicker**) — the two structural components the app screens are built on
- **charts/** — **Chart** (**Sparkline**, **BarChart**, **DonutChart**) — dependency-free inline SVG, no charting library

Each component directory has `<Name>.jsx`, `<Name>.d.ts` (props + starting-point tag), `<Name>.prompt.md` (usage), and a `*.card.html` specimen.

### Intentional additions
None beyond the requested inventory. The brief asked for buttons, inputs, status badges, cards, tabs, sidebar, data table, and the empty/loading/error states — all present. `IconButton`, `Select`, `Checkbox`, and `Switch` were added as the minimal form/control set the three surfaces require.

---

## UI kits
Full-screen, interactive recreations under `ui_kits/`:

- **operator-dashboard/** — the Kontor agent operator view: sidebar shell, live-status topbar, KPI cards, a runs `DataTable`, and a click-through **agent-trace timeline** built on `Timeline` inside a `Drawer`.
- **chat-widget/** — the embeddable DocMind RAG assistant: launcher + chat panel with **inline source citations**, suggested prompts, typing state, and thumbs feedback, over a faux host site.
- **marketing-landing/** — nav, hero, Kontor/DocMind product split, three-surface strip, stats, CTA.
- **week-calendar/** — the Kontor scheduling screen: `WeekCalendar` with staff columns, appointment detail in a `Drawer`, a destructive `Modal` to cancel a booking, a `Drawer` to create one from an empty slot, `ToastStack` confirmations, and a `MonthPicker` for navigation.

---

## File index (manifest)
- `styles.css` — global entry point (import this one file).
- `tokens/` — `fonts.css`, `colors.css`, `typography.css`, `spacing.css`, `radius.css`, `motion.css`, `base.css`.
- `components/` — `core/`, `forms/`, `navigation/`, `data/`, `feedback/`.
- `guidelines/` — foundation specimen cards (Colors, Type, Spacing, Brand).
- `ui_kits/` — `operator-dashboard/`, `chat-widget/`.
- `assets/` — `icons.js` (KKIcons Lucide mirror).
- `thumbnail.html` — homepage tile. `SKILL.md` — Agent Skills wrapper. `readme.md` — this file.

---

## Fonts to install
`tokens/fonts.css` now declares **self-hosted, subset-split `@font-face`** rules (variable weight axis, `font-display: swap`, latin / latin-ext split so a Latin-only page never fetches the extended-glyph file). The files aren't in the project yet — drop these four into `assets/fonts/` with these exact names:

- `assets/fonts/geist-variable-latin.woff2` / `geist-variable-latin-ext.woff2` — Geist variable (weights 100–900)
- `assets/fonts/geist-mono-variable-latin.woff2` / `geist-mono-variable-latin-ext.woff2` — Geist Mono variable

Get them from Vercel's [Geist repo](https://github.com/vercel/geist-font) (`woff2` variable builds) or run `fonttools` on the static weights to produce a variable woff2, then subset with `glyphhanger`/`fonttools subset` using the unicode ranges already written in `fonts.css`. Until the files exist, text falls back to the system sans/mono stack (no broken build). Preload the two above-the-fold files in each surface's `<head>`:

```html
<link rel="preload" href="/assets/fonts/geist-variable-latin.woff2" as="font" type="font/woff2" crossorigin>
<!-- only on screens that show mono above the fold, e.g. the operator dashboard -->
<link rel="preload" href="/assets/fonts/geist-mono-variable-latin.woff2" as="font" type="font/woff2" crossorigin>
```

## Accessibility
- **Tabs** — full `tablist`/`tab` pattern: `aria-selected`, roving `tabIndex`, Left/Right/Home/End keyboard nav. Consumers pair each tab with a panel via the `panel-<value>` / `tab-<value>` id convention (see `Tabs.prompt.md`).
- **Agent-trace drawer** (`TracePanel` in the operator dashboard) — `role="dialog"` + `aria-modal`, a real focus trap (Tab/Shift+Tab wraps inside the panel), `Escape` closes, and focus returns to the row that opened it.
- **DataTable** — real `<table>`/`<thead>`/`<th scope="col">` semantics (already present); columns can now opt into `sortable: true` for a clickable header with `aria-sort`; rows with `onRowClick` are `role="button"`, keyboard-focusable, Enter/Space-operable.
- **Sidebar** — the active `SidebarItem` carries `aria-current="page"`.
- **Chat widget** — the message list is `role="log"` `aria-live="polite"` so streamed replies are announced; the composer input has `aria-label="Message DocMind"`; all icon-only buttons already require a `label` prop (enforced by `IconButton`'s contract).
- **Contrast audit** (WCAG 2.1, computed against the actual token values): `--status-*-fg` on its own `--status-*-bg` ranges 5.68:1–8.13:1 in both themes — all pass. `--text-tertiary` **failed** at 3.9–4.1:1 (dark) and 2.9–3.3:1 (light) against `--bg-base`/`--surface-1`(-2); the token itself was adjusted (not overridden per-use) to `#767f8e` (dark) / `#656e7c` (light), now 4.53–5.17:1 everywhere it's used — comfortably over the 4.5:1 body-text floor.

## Caveats
- **No logo provided** → the brand is set in type; a type-only **"K&K"** monogram + "Kontor & DocMind" wordmark stands in wherever a mark would go. No logo was drawn or reconstructed.
- **No icons provided** → **Lucide** substituted (inline mirror in `assets/icons.js`); use `lucide-react` in production.
- `@startingPoint` tags on the two app UI kits are a legacy mechanism (templates have replaced it in current consuming projects) — offer to convert `ui_kits/operator-dashboard` and `ui_kits/chat-widget` into `templates/<slug>/` on request.
