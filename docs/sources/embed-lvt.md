# `embed-lvt` block reference

Embed a separately deployed LiveTemplate app inside a tinkerdown
markdown page. Unlike an `<iframe>`, the embedded app shares the docs
page's DOM, CSS, and fonts — it reads as part of the page.

## Authoring

Auto-proxy mode (recommended — tinkerdown handles the proxy for you):

````markdown
```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```
````

Tinkerdown auto-registers a reverse-proxy from `/apps/counter/` to the
upstream at startup. No `tinkerdown.yaml` route needed; both HTTP and
WebSocket flow through the proxy.

Same-origin via manually-configured proxy (when you'd rather declare
routes once in `tinkerdown.yaml` and reference the path from many
pages):

````markdown
```embed-lvt path="/apps/counter/"
```
````

Sibling subdomain (cross-origin direct):

````markdown
```embed-lvt server="https://counter.yoursite.com"
```
````

The block has no body. Any non-empty body is a parse error — embed
blocks are pointers, not template containers.

## Attributes

| Attribute | Default | Purpose |
|---|---|---|
| `upstream` | (none) | HTTP origin of the deployed app (e.g. `http://127.0.0.1:9090`). When set, tinkerdown auto-registers a reverse-proxy from `path` to this upstream — no `tinkerdown.yaml` entry needed. The server-side fetch goes directly here, the browser-side WebSocket flows through the auto-registered proxy. |
| `server` | docs origin | Cross-origin direct mode: browser connects straight to this remote origin instead of through tinkerdown's proxy. The remote app must list the docs origin in `AllowedOrigins`. |
| `path` | `/` | Path the docs page proxies and fetches. Required when `upstream=` is set (otherwise the auto-proxy would intercept `/`). |
| `session` | unique-per-block | Two blocks with the same `session` value share one upstream session — useful for splitting one app's UI across the page. |
| `height` | (auto) | CSS `min-height` for layout stability before the embed arrives. |
| `timeout` | `2s` | Server-side fetch deadline. On timeout, the slot becomes a "live demo unavailable" badge and the page renders without it. |

## How it works

1. At parse time, tinkerdown emits a placeholder `<div
   class="tinkerdown-embed-lvt" data-tinkerdown-block …></div>` where
   the block was authored.
2. At request time, `ProcessEmbedLvt` (in `embed_lvt.go`) scans the
   HTML for these placeholders and, for each:
   - Builds the upstream URL: `<server><path>` (or `<docs-origin><path>`
     when `server` is empty).
   - Issues a GET with the configured timeout, forwarding `Cookie` and
     `Accept-Language` from the docs reader's request.
   - Parses the response and extracts the first `<div data-lvt-id=…>`
     wrapper, discarding `<head>`, `<script>`, and other chrome.
   - Inlines the wrapper into the docs page HTML inside a tinkerdown
     embed container that carries the upstream coordinates.
3. The browser receives the assembled HTML and paints it immediately.
4. Tinkerdown's client discovers `[data-tinkerdown-block][data-block-type="embed-lvt"]`
   elements, instantiates a dedicated `LiveTemplateClient` for each
   pointing at the upstream's `/live` WebSocket, and attaches it to
   the inlined wrapper. From there the embed behaves like a normal
   LiveTemplate session — actions, DOM patches, reconnect.

The shared tinkerdown WebSocket (used by local `lvt` blocks) is **not**
involved. Each embed has its own connection to its own upstream.

## Same-origin via tinkerdown proxy

The recommended same-origin setup uses tinkerdown's existing
reverse-proxy support (`internal/server/proxy_routes.go`). Add a route
in `tinkerdown.yaml`:

```yaml
routes:
  - pattern: /apps/counter/
    type: proxy
    upstream: http://localhost:9090
```

Then any `embed-lvt path="/apps/counter/"` block on any docs page
reaches the counter app on port 9090 with zero CORS configuration.

> **Security note — Origin rewriting.** For the upstream's same-origin
> WebSocket check to accept proxied handshakes, tinkerdown rewrites the
> `Origin` header on WebSocket upgrade requests to the upstream's own
> origin (non-WebSocket requests are untouched). This means the upstream
> sees its own origin, not the real browser origin. An upstream that uses
> the `Origin` header for *authorization* (rather than just same-origin
> CSRF protection) should not be fronted by a proxy route without
> additional authentication.

## Cross-origin (sibling subdomain)

If your upstream app is on a different host, two things are needed:

1. The upstream app must list the docs origin in `AllowedOrigins`
   (`livetemplate/config.go:32`):
   ```go
   livetemplate.New("counter",
       livetemplate.WithAllowedOrigins([]string{"https://docs.yoursite.com"}),
   )
   ```
2. The browser will connect directly to the upstream's `/live`
   WebSocket on its own origin. Cookie-based auth requires a shared
   cookie domain (e.g. `.yoursite.com`).

## Multiple embeds on one page

Each embed-lvt block is independent by default — even when two blocks
point at the same upstream, they get separate WebSocket connections.

To split *one* upstream session across multiple regions of the docs
page (e.g. control panel up top, output further down), give both
blocks the same `session` value:

````markdown
Pick a date range:

```embed-lvt path="/apps/dashboard/" session="tour" height="60px"
```

The chart, driven by the range above:

```embed-lvt path="/apps/dashboard/" session="tour" height="300px"
```
````

## Trade-offs

- **Slow upstream slows docs render.** The fetch is on the page-render
  hot path; a flapping app means flapping docs. Use the `timeout`
  knob.
- **No author-supplied fallback HTML.** When the upstream is down the
  reader sees the badge. Pointer-only blocks keep the authoring
  surface honest.
- **Cookies/auth follow the docs origin.** Same-origin (proxy) is the
  cleanest setup. Sibling-subdomain works when the cookie domain is
  shared.

## Failure mode: badge on unavailable upstream

If the upstream returns a 5xx, takes longer than `timeout`, or the
response contains no `<div data-lvt-id="…">`, tinkerdown emits a
small badge in place of the embed:

```html
<div class="tinkerdown-embed-lvt unavailable" title="<reason>">
  live demo unavailable
</div>
```

The rest of the page renders normally. The reader is not blocked.

## Companion: literate templates

For inline template demos that don't need a separate runtime, see the
[`show-source`](../guides/literate-docs.md) flag on `lvt` blocks.
The two compose: a single docs page can host both literate template
listings and embedded full apps.
