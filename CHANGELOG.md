# Changelog

All notable changes to tinkerdown will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Earlier releases (v0.1.x) are documented in the
[GitHub releases page](https://github.com/livetemplate/tinkerdown/releases).


## [Unreleased]

### Fixed — `tinkerdown cli` reaches WASM sources; wasm docs use the real config keys

`tinkerdown cli` now constructs `wasm` sources, so a WASM source has the same CRUD
surface on the CLI as under `serve`: `list` works for any module, and
`add`/`update`/`delete` work when the module exports `write` (a read-only module
refuses a write with a clear error rather than dropping it). `exec` remains
intentionally out of the CLI CRUD surface (it is a read-oriented, `--allow-exec`-gated
command runner), now documented in the code rather than a silent fallthrough. The
WASM source docs and the `wasm-source` template were also corrected: they used a
non-existent `module:` key (the real key is `path:`) and `config:` (real: `options:`),
so following them failed to load — plus a new *Read-only vs. writable* section
documents the `write`-export contract and the CLI parity.

### Added — a saved-skills gallery (`examples/gallery/`)

A discoverable home for captured workflows: the gallery is itself a plain
Tinkerdown app (an `lvt-source` over a read-only CSV, no custom JavaScript) that
lists each captured skill with what it stands up and where its stand-up steps
live. Because no source type reads a directory of `SKILL.md` frontmatter, the
gallery renders a committed static index (`skills.csv`) rather than scanning
`skills/` live; a test (`TestSavedSkillsGalleryInSync`) keeps that index in step
with the captured skills on disk — every `skills/` dir except the
framework-authoring skills — so a new capture cannot silently go unlisted.

### Changed — a saved skill now carries its house style

`/tinkerdown:save` captures the manifest **whole**, not trimmed to sources +
actions: the `styling` block (theme + design `tokens`) and any
`generation.style_guide` → `style-guide.md` travel with the capture, under the
same bundle-or-point rule as `seed.sql` / `app.md`. So a saved workflow re-runs
**on-brand** instead of falling back to bare defaults — closing the gap where a
capture kept a console's data surface but dropped its palette. The PII reference
skill demonstrates the point case: it points at the committed
`examples/pii-access-approval/`, whose manifest carries the house style, and a
test now guards that the style file travels and the manifest still round-trips
its `styling.tokens` + `generation.style_guide`.

### Changed — the generation skill now consumes the house style

The `/tinkerdown` generation skill reads a project's house style and authors
against it, closing a declared-but-unconsumed gap: `generation.style_guide` (a
markdown file of house tone/layout/do-don't) was a config field nothing ever
read. The skill now reads it — plus the `styling` block (`theme` +
`styling.tokens`) — and carries the on-brand rule into generation: **write
semantic HTML, never hardcode colours** and let the design tokens skin it, follow
the project's `style-guide.md`. The token vocabulary is documented in the skill
`reference.md` (guarded against `KnownStyleTokens` so a new token can't go
untaught). The whole style block stays optional — with none, generation falls
back to on-brand defaults. The PII reference app now ships a `style-guide.md` +
`styling.tokens` demonstrating a governed, on-brand console.

### Added — declarative design tokens (`styling.tokens`)

A project can now set its on-brand palette declaratively, without writing raw
CSS. `styling.tokens` in `tinkerdown.yaml` maps snake_case design-token names to
values — e.g. `accent: "#5a67d8"`, `card_bg: "#ffffff"` — and each drives the
matching CSS custom property (`--accent`, `--card-bg`, …) in every page's
`:root`, overriding the built-in default via the same mechanism `primary_color`
already uses for `--accent`. Because the override skins the *semantic* HTML the
generator emits, a generated UI is on-brand by construction rather than by
careful prompting. An unknown token key fails loudly at config load (naming the
key and listing the known tokens) so a typo can't silently skin nothing; token
values are sanitized against CSS-injection at render. Absent, built-in defaults
apply — existing projects are unaffected. A token override is a single value that
applies to **both** light and dark themes (same as `primary_color`); per-theme
palettes are not yet expressible.

### Added — runtime approved-surface enforcement

When a project declares a `generation:` block, Tinkerdown now enforces the
approved surface at **runtime**, not only at generation time: a running app — or
a webhook / crafted-message caller — may only invoke an **approved** action, and
a builtin write may only touch an **approved** source. The check runs on every
path a caller can reach (WebSocket custom actions, WebSocket builtin writes, and
the webhook endpoint, which share no chokepoint), so a caller bypassing the
browser cannot reach past the surface the operator approved — the server-side
gate the client-only `confirm:` never was. Projects with no `generation:` block
are unaffected (approval is opt-in).

### Changed — `validate` catches unresolved sources and incomplete action params

`tinkerdown validate` now reports two more "passes validate, breaks at serve"
classes:

- **Bound references** — a source bound via `lvt-source="…"` that resolves to no
  declared source (a typo) is reported with the list of declared sources, instead
  of erroring only at serve as "source not found". The declared universe is the
  file's frontmatter sources plus the tinkerdown.yaml governing it.
- **Action-param completeness** — for a `kind: sql` action a form invokes, every
  `:param` its statement references must be supplied by a form field or a `data-*`
  attribute (`:operator` is server-set and exempt); a missing one is named,
  instead of erroring at serve on the first substitution.

The state-ref diagnostic ("interactive block has no state reference") now teaches
the real fix — add `lvt-source` to the block's container — rather than an
undefined `state="block-id"`.

### Changed — `lvt-persist` reports a migration hint

Using `lvt-persist` now reports "unknown attribute — use lvt-source" instead of a
confusing "no state reference": Tinkerdown's server no longer supports the persist
model (use `lvt-source` with `type: sqlite` for a writable store).

### Changed — `validate` now checks that every lvt block compiles as a template

`tinkerdown validate` runs each lvt code block through the template parser
(livetemplate `v0.21.0`'s new `Validate`), so a template-syntax or composition
error inside a block — an unclosed `{{range}}`, an unknown function, an
unresolved component — is now reported with a line and message instead of
surfacing only at serve time as a block that silently renders nothing.
Validation resolves against the same component templates and helper functions
serve uses, so a block that renders under serve validates clean, and vice versa.

### Added — `split` block helper

Templates in an lvt block can call `{{split "a, b, c" ", "}}` to turn a
delimited string into a slice (e.g. comma-separated tags into individual values
for a `{{range}}`) — Tinkerdown's first base block helper, available to every
block at both parse and render.

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
