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

```go include="./_app/counter.go" lines="13-59"
```

```html include="./_app/counter.tmpl" lines="10-13"
```

## Region 1 — counter

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" session="tour"
```

## Region 2 — same counter, lower on the page

Click `+` in either region; the count moves in lockstep. The
`session="tour"` attribute groups the two embeds as authoring intent;
the actual state sharing is delivered by the Mount-side
`ctx.Subscribe(ctx.SelfTopic())` opt-in plus the handler's
`ctx.Publish(ctx.SelfTopic(), "Increment", nil)` line above — that
pair tells the runtime to apply `Increment` on every other connected
client that subscribed when one client clicks. Without the Subscribe,
no peer registers as a receiver; without the Publish, no fan-out
happens; with both, the two regions stay synced.

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" session="tour"
```

> **Note:** start the counter app first (`PORT=9090 go run ./_app`)
> in a sibling terminal, then `tinkerdown serve` against this
> directory. See [`literate-counter-include`](../literate-counter-include/)
> for the full setup walkthrough.
