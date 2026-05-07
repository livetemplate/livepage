---
title: "Linked widgets sharing one app"
source_repo: "https://github.com/livetemplate/tinkerdown"
source_path: "examples/literate-linked-include/index.md"
---

# Linked widgets, one source of truth

Two `embed-lvt` blocks pointing at the same upstream and sharing a
`session` value see the same state. Click `+` in either widget; both
update. The reader gets a control panel up top and a status display
further down — split-UI from one app.

## The state and handler (real source)

For reference — these are the actual files the deployed counter
runs:

```go include="./_app/counter.go" lines="5-8"
```

```go include="./_app/counter.go" lines="13-35"
```

```html include="./_app/counter.tmpl" lines="10-13"
```

## Region 1 — counter

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" session="tour"
```

## Region 2 — same counter, lower on the page

Click `+` in either region; the count moves in lockstep. The
`session="tour"` attribute groups the two embeds as authoring intent;
the actual state sharing is delivered by the handler's
`ctx.BroadcastAction("Increment", nil)` line above — that line tells
the runtime to apply `Increment` to every other connected client
when one client clicks. Without it, each region's session would have
its own count; with it, they stay synced.

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" session="tour"
```

> **Note:** start the counter app first (`PORT=9090 go run ./_app`)
> in a sibling terminal, then `tinkerdown serve` against this
> directory. See [`literate-counter-include`](../literate-counter-include/)
> for the full setup walkthrough.
