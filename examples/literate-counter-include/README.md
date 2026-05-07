# literate-counter-include — read the code, see it run

This example demonstrates the `LANG include="..." lines="N-M"` fence
attribute paired with `embed-lvt`. The docs page (`index.md`) cites
real source files in `_app/`; tinkerdown reads those files at render
time and emits standard syntax-highlighted code blocks. The same
files are also a deployable LiveTemplate counter app — the same code
the reader is reading.

## Run it

Two terminals, two minutes.

### 1. Start the counter app on port 9090

The `_app/` directory is a self-contained Go module. From there:

```bash
cd _app
go mod tidy   # first time only — pulls livetemplate + lvt
PORT=9090 go run .
```

### 2. Start tinkerdown serving this example

In another terminal, from the tinkerdown repo root:

```bash
go run ./cmd/tinkerdown serve examples/literate-counter-include
```

The `embed-lvt` block in `index.md` declares `upstream="http://127.0.0.1:9090"`
so tinkerdown auto-registers a reverse-proxy at `/apps/counter/`. No
`tinkerdown.yaml` needed.

### 3. Open the page

`http://localhost:8080/`. You'll see four code blocks (state, handler,
template, running widget) followed by a clickable counter. The first
three are sliced from `_app/counter.go` and `_app/counter.tmpl` at
render time. The fourth is the deployed counter, fetched and inlined
by tinkerdown.

## What you'll experience

- Clicking `+` increments the counter. The action runs in the
  deployed app on `:9090`; the resulting DOM diff flows back over
  the proxied WebSocket.
- Editing `_app/counter.go` while the page is open: the docs page
  reloads with the new snippet content (file watcher catches it).
  The deployed app needs a restart for *its* behavior to update —
  that's a normal Go-deploy concern, not a docs concern.
- The reader cannot see *stale* code: the snippets render directly
  from the files at request time, no copy-paste drift.

## Note on the sibling `literate-linked-include/` example

The sibling `examples/literate-linked-include/` example contains a
near-identical `_app/` (counter.go, counter.tmpl, main.go) with its
own `go.mod`. The duplication is intentional: each example must be
clone-and-run independently, so each owns its module graph. When
bumping `livetemplate` or `lvt`, update both `_app/go.mod` files
together to keep them in sync.
