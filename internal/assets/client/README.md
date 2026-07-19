# Generated client bundle — do not hand-edit

These four files are **build output committed to git**, produced from `client/`
by esbuild and embedded into the binary by `internal/assets/assets.go`:

    tinkerdown-client.browser.js[.map]
    tinkerdown-client.browser.css[.map]

## Regenerating

```sh
make build-client      # runs `npm ci && npm run build`, then copies the output here
```

**Never run `npm run build` without installing first.** `npm ci` installs exactly
what `client/package-lock.json` pins and fails loudly on drift; a bare
`npm run build` bundles whatever happens to be in `client/node_modules`, which
can be an older release. That is not hypothetical — see
[#295](https://github.com/livetemplate/tinkerdown/issues/295), where a checkout
whose `node_modules` had drifted three minors behind the lockfile silently
regenerated this bundle from the stale copy and reverted shipped fixes,
including the `data-lvt-force-update` handling the checkbox e2e tests rely on.
`make build-client` now does the right thing; the trap is only reachable by
running the raw npm command yourself.

## Verifying a regenerated bundle

Two things make this asset unusually easy to get silently wrong: it is generated
yet tracked (so it can disagree with the lockfile that supposedly produced it),
and **CI cannot catch a regression in it** — the e2e tests that would are
`//go:build !ci` and do not run there. A local check is the only gate.

1. **Diff against a clean build if in doubt.** A fresh `npm ci` build should
   reproduce the committed bytes exactly.
2. **Check direction, not just difference.** "It differs from HEAD" proves
   nothing — a downgrade differs too. Pick a marker tied to a known changelog
   entry in the version you are moving to and confirm it appeared.
3. **Run the e2e suite locally** (`go test ./...` — the `!ci` tests run by
   default), having built `./tinkerdown` first, since several shell out to it.
   The checkbox-toggle tests (`TestAutoTasks_*`, `TestLvtSourceMarkdownToggle*`)
   are the sensitive ones: they depend on client-side server-authoritative
   behavior and fail in a way that looks like ordinary flakiness. The tell is
   that a bundle problem reproduces every time.

The client version is also a **wire contract**: livetemplate exports a
`ClientVersion` constant naming the `@livetemplate/client` release it is
wire-compatible with, because there is no runtime server↔client handshake.
When bumping the server, match the client to that constant rather than to
whatever npm calls `latest`.
