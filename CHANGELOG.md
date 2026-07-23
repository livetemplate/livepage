# Changelog

All notable changes to tinkerdown will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Earlier releases (v0.1.x) are documented in the
[GitHub releases page](https://github.com/livetemplate/tinkerdown/releases).


## [Unreleased]

### Added — Atomic multi-statement SQL actions (`statements:`)

A `kind: sql` action may now carry a `statements:` list instead of a single
`statement:`; the statements run in one transaction — all commit, or the first
error rolls all of them back:

```yaml
approve-export:
  kind: sql
  source: access_requests
  statements:
    - "INSERT INTO exports (...) SELECT ... FROM orders_pii LIMIT (SELECT row_cap FROM access_requests WHERE id = :id)"
    - "INSERT INTO audit_log (...) SELECT ... FROM access_requests WHERE id = :id"
    - "UPDATE access_requests SET status = 'approved' WHERE id = :id"
```

This exists so an action can change state **and** append its audit record
atomically — a partial success (state changed, audit missing) is impossible. An
action must set exactly one of `statement`/`statements`; both, or neither, is a
config error caught at load.

### Added — PII / data-export access-approval reference app

`examples/pii-access-approval/` — the M1 reference console: a request queue where
**Approve** runs a bounded, server-authoritative export (the row cap is read from
the request row, not sent by the client) plus a durable audit record, and **Deny**
records a decision without granting access. Built entirely on the manifest +
existing primitives.

### Fixed — policy lint no longer flags built-in source affordances

`tinkerdown validate` treated a writable source's own `Add`/`Delete`/`Toggle`/
`Refresh` controls as unapproved *actions*. These are intrinsic affordances,
governed by whether the source is approved and writable — not by the
approved-action set — so an approved writable source's Add form now lints clean.

### Added — `generation:` block: an approved surface for LLM-generated apps

A `tinkerdown.yaml` may now declare which of its sources and actions an LLM is allowed
to wire up when generating an app against the project:

```yaml
generation:
  sources: [requests]
  actions: [approve-request]
  style_guide: ./style-guide.md   # optional
```

Sources and actions also accept a `describes:` note — no runtime behavior, purely the
human-readable summary an operator reads when reviewing what a generated app does.

Approval is enforced as a **precedence tier**, not a prohibition. The documented
frontmatter-wins-over-config rule gains a top tier: an approved name is *pinned*, so a
page cannot redefine it. This is necessary because a page's frontmatter can declare its
own sources and actions — without pinning, a generated page could reference an approved
name while defining that name as something the operator never approved, defeating any
check that reasons about names alone. Attempts are logged rather than silently dropped.

Approval also makes site-level **actions reachable**. Previously a page could only
invoke actions declared in its own frontmatter, so an approved-actions list would have
named a surface no generated page could use. The fallback is limited to *approved*
actions: actions written for schedules or webhooks stay unreachable from pages.

An approved name that refers to nothing is rejected at config load. Such an entry would
otherwise be silently inert — and since approval is what pins a name, a typo in
`generation.sources` would *remove* a protection while appearing to add one.

**Projects without a `generation:` block are unaffected.**


### Changed — Upstream bump: livetemplate v0.10.0 → v0.19.1

Tinkerdown had been pinned to `livetemplate v0.10.0` while upstream shipped
nine minor releases. This adopts the current latest across the stack:

| Dependency | Was | Now |
|---|---|---|
| `github.com/livetemplate/livetemplate` | v0.10.0 | **v0.19.1** |
| `github.com/livetemplate/lvt/components` | pseudo-version `2026-02-28` | **v0.2.0** |
| `@livetemplate/client` | 0.14.3 | **0.18.2** |

The client version is not merely "latest" — livetemplate v0.18.0 added a
`ClientVersion` constant precisely because there is **no runtime server↔client
version handshake**, and v0.19.0 declares `0.18.2` as the wire-compatible pair
for this server release.

No source changes were required: tinkerdown's livetemplate API surface (10
symbols) is untouched by the range, and the one API removed upstream
(`WithStore`, v0.19.0) was never used here.

Behavior inherited from the bump, most consequentially two silent-update-loss
fixes in v0.18.1 (a range whose item statics changed had its update dropped,
leaving stale items on the page until a full reload; and a nil `lvt:"persist"`
field discarded the entire restored state on reconnect), plus v0.19.0's
per-item recursive range diffs, v0.18.0's client pinning, v0.17.0's
`WithParseFS` and scoped method precompute, and v0.16.0's `__ping__` liveness
heartbeat.

### Fixed — `make build` no longer rebuilds the client from stale `node_modules`

`build-client` ran `npm run build` with no install step, so it bundled whatever
happened to be in `client/node_modules` rather than what the lockfile pins. On a
checkout whose `node_modules` had drifted, this silently regenerated the
committed bundle from a stale dependency and reverted shipped fixes — including
the `data-lvt-force-update` handling that the checkbox e2e tests depend on. CI
never caught it because CI always installs fresh. Now runs `npm ci` first, which
installs exactly the lockfile and fails loudly on drift. ([#295])

[#295]: https://github.com/livetemplate/tinkerdown/issues/295


## [v0.2.1] - 2026-05-09

### Added — Site-wide include resolution

`tinkerdown serve` now resolves `include="..."` paths against the
**site root** (the content directory passed to `serve`) instead of
each markdown file's own parent directory. Cross-page references like
`include="../recipes/counter/_app/counter.go"` from a page in
`getting-started/` now succeed; paths that escape the site root are
still rejected.

Library API:

- New `tinkerdown.ParseFileInSite(path, siteRoot)` — uses `siteRoot`
  as the include-confinement boundary. Pass `siteRoot=""` to fall
  back to page-root confinement.
- Existing `tinkerdown.ParseFile(path)` is unchanged — single-page
  callers (CLI tools, library users rendering one .md file) keep the
  v0.2.0 page-root-confined default.

Motivation: the v1 page-root confinement (per the comment in
`page.go:103-109` of v0.2.0) made it impossible for a multi-page docs
site to maintain a single source of truth for code samples shared
across pages. The classic case: a "Getting Started" tutorial and a
"Recipes" deep-dive that both want to include from the same
deployable `_app/`. Site-wide root resolution unblocks that without
removing the security boundary — paths still must stay under the
site directory.

The `serve` and `validate` commands automatically use `ParseFileInSite`
with the configured content directory; no per-page frontmatter is
needed.


## [v0.2.0] - 2026-05-07

### Added — Literate authoring primitives

A toolkit for documentation that *shows the real code, running*. Three
composable primitives let an author pair source listings with live
widgets without any drift between docs and production.

#### `show-source` / `hide-source` flag on `lvt` blocks

Add `show-source` to a fenced ` ```lvt ` block to render the template
both as a syntax-highlighted listing and as the live, interactive
widget it produces. Page-level default via `lvt_show_source: true`
in frontmatter; individual blocks can opt out with `hide-source`.

#### `embed-lvt` block

Embeds a deployed LiveTemplate app inline. Tinkerdown fetches the
upstream HTML server-side at render time so the page paints fully
on first paint; the WebSocket attaches afterwards for live updates.

Attributes:
- `path=` — docs-side path to expose (default `/`)
- `upstream=` — HTTP origin of the deployed app; auto-registers a
  reverse-proxy from `path=` (no `tinkerdown.yaml` route entry needed)
- `server=` — alternative cross-origin direct mode (no proxy);
  remote app must include the docs origin in its `AllowedOrigins`
- `session=` — authoring intent for linked widgets (real cross-region
  state sharing comes from the upstream app's `BroadcastAction` calls
  + a constant-groupID authenticator, not from this attribute)
- `timeout=` — server-side fetch deadline (default `2s`)
- `height=` — CSS `min-height` for layout stability
- `show-source` — display upstream HTML alongside the live preview

#### `include="..."` fence attribute

Slices a real source file into a rendered code block:

- `lines="N-M"` — single 1-indexed inclusive range; auto-clamps if the
  file shrinks below the upper bound
- `lines="N-M,P-Q"` — multiple ranges, joined with a language-aware
  ellipsis comment (`// ...` for Go, `# ...` for Python/YAML,
  `<!-- ... -->` for HTML, etc.)
- `region="name"` — extracts content between
  `// >>> region:name` / `// <<< region:name` markers; survives
  line-number drift across edits
- `highlight="N-M"` — Prism Line Highlight overlay; numbers are
  file-absolute and match the rendered gutter

#### Auto source-link footer

Each `include=` block automatically renders a `counter.go:13-35`
footer link to the cited file at the cited range. Activated by
frontmatter:

```yaml
source_repo: https://github.com/yourorg/yourrepo
source_path: examples/literate-counter/index.md
```

Default git ref tracks the running tinkerdown binary's release
version (set via build-time ldflags); override per-page with
`source_ref:`. For `dev` builds, the ref falls back to `main`. URL
schemes are restricted to `http(s)://` to prevent XSS via author-set
`javascript:` or `data:` URLs.

#### Path confinement, dedent, and hot reload

- Include paths are resolved relative to the markdown file's
  directory, canonicalised (symlinks followed), and confined to
  that tree. Escape attempts (`../../etc/passwd`) are rejected with
  a warning; the page still renders.
- The longest common leading whitespace across non-blank lines is
  stripped, so a function-body slice renders flush-left without
  mangling its relative indentation.
- The file watcher tracks every included file via `SetExtraFiles`.
  Editing `_app/counter.go` while the page is open auto-refreshes
  the rendered snippet (and the source-link footer).
- File-size cap (`maxIncludeBytes = 10 MB`) on `include=` reads
  guards against accidentally pulling vendored binaries or large
  logs into the docs build.

#### Vendored Prism plugins

Prism Line Numbers + Line Highlight are vendored from prismjs@1.29.0
and loaded only on pages where `len(IncludedFiles) > 0`. Pages
without source-includes pay zero asset cost.

### Added — Reference examples

- `examples/literate-counter/`, `examples/literate-linked/` —
  `show-source` on inline `lvt` blocks bound to markdown data sources
- `examples/literate-counter-include/` — `include=` slicing a real
  `_app/counter.go` paired with `embed-lvt`; canonical A2 shape
- `examples/literate-linked-include/` — A3 pattern: two `embed-lvt`
  blocks sharing one upstream session via constant-groupID
  authenticator + `BroadcastAction` in handlers
- `examples/embed-counter/`, `examples/embed-shared-session/`,
  `examples/mixed-tracks/` — embed-only and combined-tracks examples
- In-tree authoring guide at `docs/guides/literate-docs.md` covering
  `show-source`, `include=`, and pairing with `embed-lvt`

### Fixed

- Embed event-delegator collision when two `embed-lvt` blocks point
  at the same upstream — wrapper IDs are now suffixed with the
  per-block id so livetemplate's event delegator routes correctly to
  each region.
- CommonMark-compliant fence opener recognition in the include
  preprocessor — `include=` fences inside lists and quotes are now
  resolved (previously skipped).
- Race in `isIncludedFile` watcher callback — acquire `s.mu.RLock`
  around page-route iteration; short-circuit on `.md` events so
  watcher doesn't redundantly scan the include set during Discover.

[v0.2.1]: https://github.com/livetemplate/tinkerdown/compare/v0.2.0...v0.2.1
[v0.2.0]: https://github.com/livetemplate/tinkerdown/compare/v0.1.20...v0.2.0
