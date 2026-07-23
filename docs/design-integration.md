# Claude Design integration inventory

**Source package:** `Kontor agent trace screen/` (received 2026-07-23).

The package is design-source material, not a runtime dependency. The embedded
operator UI continues to use the repository's vendored React/DS runtime and
`web/widget/kontor.js` remains dependency-free. No files from the package are
served directly, copied into the binary, or loaded from a third party.

## Provenance and assets

- The package identifies the visual language as Kontor & DocMind and includes
  a local copy of the design-system bundle and token files under `_ds/`.
- Kontor already vendors the same design-system family under `design/` and
  `web/operator/`; production code uses those existing copies to avoid a second
  bundle and to retain the self-hosted CSP policy.
- The supplied HTML references only local `_ds/` files and inline SVG. No
  external font, image, script, or licence file is referenced by the specimens.
  The included `readme.md` should remain with the incoming package as its
  provenance record. Before redistributing the package itself, confirm its
  upstream licence; no upstream asset has been redistributed by this change.
- Finder metadata (`.DS_Store`, `.thumbnail`) is intentionally not product
  source and is not part of the inventory below.

## Mapping and adopted behaviour

| Source HTML | Product destination | Adopted behaviour |
| --- | --- | --- |
| `Kontor Operator Dashboard.dc.html` | `#/overview` | KPI cards, charts, attention panel, six-item sidebar direction |
| `Kontor Agent Trace.dc.html` | `#/runs/:id` | two-pane conversation/trace, payload and retry nesting, escalation status |
| `Kontor Week Calendar.dc.html` | `#/calendar` | responsive week grid, booking detail, create/reschedule/cancel entry points |
| `Kontor Customer Chat.dc.html` | `web/widget/kontor.js` | launcher, accessible dialog/thread, compact responsive shell |
| `Slot Picker.dc.html` | Widget thread | buttons are created only from exact times in a real assistant response; selection sends a normal customer message and cannot mutate a booking |
| `Confirmation Card.dc.html` | Widget thread | fact table, explicit confirmation action, and “nothing is booked” safety copy |
| `Booking Card.dc.html` | Calendar booking detail drawer and trace booking facts | confirmed booking status/facts are shown from live calendar/trace data; the widget does not fabricate booking facts absent from its API response |
| `Acting Indicator.dc.html` | Widget thread | live `role=status` progress indicator while a server turn is active |
| `Escalation Break.dc.html` | Widget thread and `#/inbox` | visible hand-off marker and tenant-scoped list of escalated runs |

`#/overview` is canonical terminology. `#/` and `#/dashboard` remain aliases
for existing bookmarks and Dashboard-era links; `#/runs` and `#/calendar` are
unchanged.

All adopted UI uses existing design tokens, keyboard-native controls or
semantic links, explicit labels, focus-visible styles, and the existing mobile
navigation breakpoint. Inbox, Analytics, and Settings independently render
loading, empty where applicable, and retryable error states from live API
requests.
