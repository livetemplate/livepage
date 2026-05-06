# embed-counter — try the `embed-lvt` block end-to-end

This example demonstrates `embed-lvt` against a real LiveTemplate
counter app. Two terminals, two minutes.

## 1. Start the counter app on port 9090

The companion repo ships a minimal counter at
`livetemplate/examples/counter`. Run it on a non-default port so it
doesn't collide with tinkerdown:

```bash
cd ../../../examples/counter      # path is relative to this README
PORT=9090 go run .
```

## 2. Start tinkerdown serving this example

In another terminal:

```bash
cd path/to/tinkerdown
go run ./cmd/tinkerdown serve examples/embed-counter
```

The `embed-lvt` block in `index.md` declares the upstream itself:

```markdown
```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```
```

Tinkerdown reads the `upstream=` attribute at startup and auto-registers
a reverse-proxy from `/apps/counter/` to the counter on `:9090`. Both
the HTTP fetch and the WebSocket upgrade flow through the proxy with
zero CORS configuration. No `tinkerdown.yaml` needed.

## 3. Open the page

Navigate to `http://localhost:8080/` (or `http://devbox:8080/` over
Tailscale for iPhone testing). You should see the counter rendered
inline; clicking the buttons round-trips through the upstream app.

## What's happening under the hood

1. Tinkerdown discovers `index.md`, sees the `embed-lvt` block's
   `upstream=` attribute, and registers an auto-proxy at
   `/apps/counter/` → `http://127.0.0.1:9090`.
2. On request, `ProcessEmbedLvt` issues a server-side GET directly to
   `http://127.0.0.1:9090/apps/counter/` (skipping the self-proxy hop).
3. The counter's HTML response contains a `<div data-lvt-id="…">`
   wrapper; tinkerdown extracts and inlines it (renaming the id to
   `data-lvt-id-pending` to keep `LiveTemplateClient`'s autoInit out of
   the way).
4. In the browser, tinkerdown's `EmbedLvtBlock` renames the id back,
   instantiates a `LiveTemplateClient` pointed at the proxied
   `ws://localhost:8080/apps/counter/`, and connects. The proxy
   forwards the WebSocket upgrade to the counter on `:9090`.
5. Action round-trips and DOM patches flow through the proxy.

When the counter is stopped mid-session, the embed becomes a
"live updates unavailable" badge but the rest of the docs page keeps
rendering.
