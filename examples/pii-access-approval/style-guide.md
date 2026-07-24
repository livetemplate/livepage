# House style — PII access-approval console

This is a compliance tool. An approver makes an irreversible, audited decision
about who reaches sensitive data. The UI should read like a control panel, not a
marketing page: sober, high-contrast, and unambiguous about state.

## Tone

- Plain and factual. Label things by what they are (`Requester`, `Row cap`,
  `Justification`), never with cute copy.
- Every privileged action states its consequence before it fires
  (`data-confirm="Approve this request and run the bounded PII export?"`).

## Layout

- One decision per row. The pending queue is a table; each row carries the full
  context an approver needs to decide without leaving the page (requester, dataset,
  scope/row-cap, justification, ticket, TTL, sensitivity tags).
- Group the intake form and its queue inside a single `lvt-source` container.
- Lead with the pending queue; show approved/denied history below or behind a
  toggle, not competing for attention.

## Colour and tokens — do not hardcode

- **Never write a raw colour** (`#3949ab`, `rgb(...)`, `red`) in the markup. The
  house palette is set once in `styling.tokens` and applied through the page's
  design tokens; hardcoding a colour bypasses it and drifts off-brand.
- Use **semantic HTML** — `<table>`, `<article>`, `<mark>`, `<strong>`, headings —
  and let the tokens skin it. Semantic elements inherit `--accent`, `--card-bg`,
  `--text-heading` etc. automatically.
- Convey status (pending / approved / denied) with text and semantic emphasis
  (`<mark>`, `<strong>`), not with a hand-picked colour.

## Components

- Prefer a plain semantic `<table>` for the queue over any custom widget.
- Confirm-gate every state-changing button with `data-confirm`.
- No decorative imagery, gradients, or animation — this is a record of decisions.
