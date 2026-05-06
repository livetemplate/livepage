---
title: "Embed a deployed LiveTemplate app"
---

# Embedding a counter

The block below points at a separately deployed LiveTemplate counter
app. Tinkerdown fetches the app's initial HTML server-side while
building this page and inlines the response inline. The reader sees a
fully painted page on first paint; the WebSocket attaches afterwards
for live updates and actions.

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090" show-source
```

When the upstream app is unavailable, tinkerdown shows a small badge in
place of the embed and the rest of the page renders normally.

## Running this example

This example assumes you've configured a tinkerdown proxy route that
forwards `/apps/counter/` to a LiveTemplate counter app. Add to your
`tinkerdown.yaml`:

```yaml
routes:
  - pattern: /apps/counter/
    type: proxy
    upstream: http://localhost:9090
```

Then run the LiveTemplate counter on `:9090` (e.g. from
`livetemplate/examples/counter`) and tinkerdown will reach it
through the proxy. No CORS configuration needed.

For sibling-subdomain deployments, point `server="https://..."`
directly at the upstream and add the docs origin to the upstream
app's `AllowedOrigins`.

See [`docs/sources/embed-lvt.md`](../../docs/sources/embed-lvt.md)
for the full reference.
