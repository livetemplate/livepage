---
title: "One app, two regions: shared embed session"
---

# Shared session: split UI across the page

Two `embed-lvt` blocks pointing at the same upstream with the same
`session` value share one upstream LiveTemplate session. State changes
in either block reflect in the other — useful when you want to lay out
a single app's UI across separate sections of a docs page rather than
cramming it into one container.

## Region 1 — controls up top

Click any of the buttons here:

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" session="tour" height="180px"
```

Below the prose, the same session keeps the counter rendering — flip
back and forth between the two regions and they stay in lockstep.

## Some explanatory prose

In a real docs page this is where you'd describe what the reader is
seeing — what state lives where, what each action does, why the
counter persists across regions even though it's two separate
`embed-lvt` blocks on the page.

The trick is the `session="tour"` attribute on both blocks. Tinkerdown
issues a single upstream session for that name and routes both browser
clients to it through cookie binding.

## Region 2 — same counter, restated

Same `session="tour"`, same `upstream`, same `path`:

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" session="tour" height="180px"
```

Notice the counter value matches Region 1. Click `+1` here and Region 1
also increments.

## When to skip `session`

Omit `session` (or use a different value) and each block gets its own
upstream session — independent state per region. That's the right
default when you want two unrelated demos of the same app on one page.

> **Note:** This example assumes the same upstream counter setup as
> [`embed-counter`](../embed-counter/) — start the counter on
> `:9090` first, then `tinkerdown serve examples/embed-shared-session`.
