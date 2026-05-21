---
title: "Build a counter"
source_repo: "https://github.com/livetemplate/tinkerdown"
source_path: "examples/literate-counter-include/index.md"
---

# Building a counter

This page demonstrates the literate pattern: the snippets you read
below are the **actual source files** of a real, deployed counter
app. The widget at the bottom is that same app running, embedded
inline through tinkerdown's auto-proxy.

## The state

A Go struct that holds whatever the app needs to track. LiveTemplate
clones it per session, so two readers don't see each other's count.

```go include="./_app/counter.go" lines="5-8"
```

## The handlers

Action handlers are methods on a controller. When the reader clicks
`+`, the runtime calls `Increment` with a clone of the current state
and stores whatever you return. `Decrement` and `Reset` follow the
same pattern. `Mount` opts each connection in to peer fan-out via
`ctx.Subscribe(ctx.SelfTopic())`; each action handler then calls
`ctx.Publish(ctx.SelfTopic(), ...)` so every connected embed and
direct visitor stays in lockstep.

```go include="./_app/counter.go" lines="13-67" highlight="46"
```

## The template

Plain HTML with a few `lvt-*` directives. The `name="increment"`
attribute on the button names the handler method to call.

```html include="./_app/counter.tmpl" lines="10-13"
```

## The running app

Click `+`; the action runs against the deployed counter and the patch
flows back over the WebSocket. Edit `_app/counter.go` while this
page is open and the snippets above auto-refresh — the running widget
needs a deploy, but the docs stay in sync.

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```
