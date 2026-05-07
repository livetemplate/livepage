# literate-linked-include — two embeds, one shared session

This example pairs the `include="..."` fence attribute with two
`embed-lvt` blocks pointing at the *same* upstream counter, so a
click in one region updates both. The shared-session illusion comes
from a constant-groupID authenticator on the deployed app, not from
any client wiring — see `_app/main.go`.

For setup (start `_app/` on `:9090`, then `tinkerdown serve`), the
flow is identical to the sibling
[`examples/literate-counter-include/`](../literate-counter-include/README.md)
example. The interesting difference here is the markdown shape:

```markdown
```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```

```embed-lvt path="/apps/counter/" upstream="http://127.0.0.1:9090"
```
```

Two embed blocks, same upstream → tinkerdown registers one auto-proxy
route. The deployed app's `sharedAuth` returns the same group ID for
every request, so all sessions broadcast to each other.

## Note on the duplicate `_app/`

This example's `_app/` is intentionally a near-clone of
`literate-counter-include/_app/` with its own `go.mod`
(`example.com/literate-linked-counter`) so that each example can be
cloned and run independently. When bumping `livetemplate` or `lvt`,
update both `_app/go.mod` files together to keep them in sync.
