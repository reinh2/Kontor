# Kontor Agent Trace Screen — build notes (research complete)

## Environment
- DS bundle: `_ds_bundle.js` (namespace "KontorKanonDesignSystem"), icons: `assets/icons.js` (window.KKIcons — Lucide-style paths, S(['...']) helper).
- UI kits mount via window.KKKit (e.g. window.KKKit.OperatorDashboard). Existing ref impl: `ui_kits/operator-dashboard/OperatorDashboard.jsx` + `index.html` (React 18 UMD + babel standalone + bundle + icons, then mount).
- styles.css pulls all tokens. Theme via `<html data-theme="dark">`.

## Architecture decision
Build as a DC (`Kontor Agent Trace.dc.html`). Because the DS is a React bundle and this screen needs shared linked state (hovered message ↔ highlighted steps; selected step → scroll conversation), put ALL state in the DC logic class and mount DS components via `<x-import component-from-global-scope="...">`. If passing handlers through template holes for the linked panes proves too tangled, fall back to the operator-dashboard pattern: write one screen `.jsx` exporting the full component + a thin index, mounted through `<x-import>`. Prefer the logic-class-owns-state approach first.
- Load bundle + icons + styles.css in `<helmet>`. Confirm exact global namespace for components at start of next session (grep `_ds_bundle.js` header line 1 for exports map / `window.` assignment).

## Component APIs (verified)
### Timeline `{ steps, loading, empty }`  (components/structure/Timeline.jsx)
Step shape:
```
{ id, t:"14:32:07", title, state:"pending|running|success|retried|failed",
  duration:"120ms", tokens:1234,
  detail:"text", mono:true,             // mono => inset box
  payload:<string or obj>, payloadTitle:"arguments",   // Show/Hide payload -> CodeBlock
  retries:[ {step...} ]   // nested sub-attempts, rendered indented (USE for find_slots 504 retry) }
```
- `TimelineSkeleton({count})` exported — use for loading state.
- States map: pending=tertiary, running=accent(spinner), success=success-fg, retried=warning-fg, failed=error-fg.
- Timeline does NOT expose per-step onClick/hover — for linked highlight/scroll I must wrap steps myself OR extend. Plan: render Timeline as-is for payload/retry fidelity, but wrap each step row in a container that adds hover/select + data-step-id, OR build a thin custom step list reusing CodeBlock + same state styles. Simplest for linking: build custom step rows (copy the state styling above) so I control onMouseEnter/onClick + ref for scroll. Reuse CodeBlock for payloads and reuse the retries-nesting layout.

### CodeBlock `{ code, title, collapsedLines=6 }` — collapsed-by-default, copy btn, --viz-* tinting. code can be string or object (JSON.stringify'd).

### DataTable `{ columns, rows, rowKey="id", onRowClick, empty, dense, sortKey, sortDir, onSort }`
Column `{ key, header, width, align, mono, render(value,row), sortable }`. Uncontrolled sort works if omit sortKey/sortDir/onSort. Use for runs list.

### Badge — tones: neutral, + semantic (success/warning/error/info/accent). Pill. Use for outcome.
### KeyValue `{...}` + KeyValueList (components/data/KeyValue.jsx) — label/value rows, value can be mono.
### Sidebar + SidebarSection + SidebarItem (compose). ErrorState `{ title, description, onRetry, ... }`. Skeleton `{ width,height,circle }`. Tabs (underline, controlled). Drawer `{ sizes sm320/md400/lg520 }`. Avatar `{ name, size sm24/md32/lg40, src, status }`.

## Tokens (use ONLY these)
- Fonts: --font-sans, --font-mono. Sizes: --fs-micro, --fs-caption, --fs-body-sm, --fs-body, ... --fw-medium. --ls-caps, --lh-normal.
- Color vars: --text-primary/secondary/tertiary/accent, --surface-1/2/inset, --border-subtle/default, --status-success-fg/warning-fg/error-fg (+ neutral/info/accent), --viz-1..5.
- --radius-md etc (radius.css). motion.css has durations/eases.

## Scenario content (render EXACTLY)
- run_7f3a9c21. Customer msg Thursday afternoon: haircut "sometime Thursday evening". Channel Telegram.
- Steps in order:
  1. list_services — success 120ms, returns 4 services.
  2. list_staff — success 95ms.
  3. find_slots — attempt 1 FAILS calendar provider timeout (504); retry -> success on 2nd attempt; 2.1s total; returns 3 slots. Retry = nested under parent (retries[]).
  4. Agent asks customer to choose; customer picks 18:30. (conversation turn between tool steps)
  5. create_booking — success, payload shows idempotency_key.
  6. upsert_crm_contact — success.
  7. Final step STILL RUNNING at screenshot (state:"running").
- Summary header: total duration, total tokens, step count, outcome Badge (success).
- KeyValue block: customer, channel (Telegram), service, staff member, resulting booking id.
- Mono everywhere: run id, timestamps, per-step durations, token counts, model name.

## Variants (same DC, in-screen switcher — canvas mode NOT needed; use a small toolbar/tabs to switch states)
1. Trace success (main).
2. Runs list (DataTable: run id, customer, channel, outcome badge, duration, timestamp; sortable) — leads into trace.
3. Loading (skeletons mirroring timeline — TimelineSkeleton + conversation skeleton).
4. Error state (trace SERVICE failed to load — ErrorState with onRetry).
5. Failed run (create_booking errored after all retries -> agent escalated to human; outcome badge error/warning; escalation step).
6. Narrow layout: two panes stack (CSS: media/container query or width toggle). Desktop-first 1440px.

## Layout
App shell: sidebar + topbar. Body two-pane: LEFT customer conversation (chat bubbles, agent vs customer, timestamps mono, avatars), RIGHT run timeline. Linked:
- hover conversation message -> highlight the agent steps it triggered (map message->stepIds).
- select a step -> scroll conversation to the causing message (refs + scrollTo, NOT scrollIntoView).
Summary header spans top of right pane (or full body top). KeyValue block in a side/drawer or under summary.

## Next actions
1. grep bundle header for exact component global namespace.
2. dc_write the screen with logic class holding {variant, hoveredMsgId, selectedStepId} + data model for the run.
3. Add narrow/stacked responsive handling.
4. ready_for_verification.
