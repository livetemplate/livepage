# Plan: Tinkerdown — ephemeral, LLM-generated internal UIs from your approved data, policies, and style

> **How to read this plan — pick your path.**
> - **Human reviewer (~5 min):** read **§ Deliverables at a glance → § Context → § The reference demo → § Roadmap**, then stop at the **"⬇ Execution contract"** divider. That's the whole story: what's being built, why, and in what order. (§ four inputs, § Upstream gap analysis, § Architecture, § Package layout, § Skills delivered are optional depth.)
> - **Executing LLM:** everything below the divider — **LLM session guide, delivery protocol, Implementation phases, Tech stack, skeleton deltas, Verification, Risks, Appendices** — is your contract; the top matter is context.
>
> **Target acceptance test (the whole plan exists to make this true):** an operator runs the Claude Code skill `/tinkerdown "a console to approve PII / data-export access requests"`, reviews and OKs the operations the generated app will run, and a working, live UI is serving in **~30 seconds**.

---

## Deliverables at a glance

What this reframe ships, most-important first. **M1 is the headline** (makes the target acceptance test real); M0 is the foundation; M2–M5 harden. **See the concrete expected output — the `tinkerdown.yaml` manifest + the generated `app.md` — in [§ Appendix B](#appendix-b--expected-output-worked-example-skip-on-phase-execution).**

**M1 — the demo (the core deliverable):**
1. **`/tinkerdown`** — a Claude Code skill that turns an intent into a live, policy-gated UI (generate → validate loop → review → ephemeral serve).
2. **Workspace manifest** — a team declares *approved* data sources + named `confirm:` actions once (extends `tinkerdown.yaml`), plus an **optional** house style (sane PicoCSS/theme defaults if omitted).
3. **Policy enforcement + operation transparency** — the real safeguard is an automated **policy lint** in `tinkerdown validate` that stops a generated app from wiring any source/action the manifest didn't approve (not a re-approval of your intent). On top, for apps that perform *privileged* operations (exec/write/network/sensitive data), generation **surfaces the concrete operations** the LLM-authored app will run — e.g. `SELECT … FROM orders_pii LIMIT 500`, `gh pr create` — before it serves, so you review the actual behavior, not just the one-line intent. Proportional: read-only UIs get no prompt.
4. **PII / data-export access-approval console** — the reference app: request queue → Approve (scoped export + durable audit record, optional grant PR) / Deny. Removes real, high-stakes friction.
5. **`skills/pii-access-approval/`** — the workflow captured as a reusable skill (re-run in seconds), **plus** the general **`/tinkerdown:save` capture capability** — the *persist* path (convention 13): ephemeral UIs are throwaway by default; **when the user chooses to keep one**, capture it as a reusable skill from the dev conversation rather than just saving the `app.md`. Opt-in, never automatic. *(Naming: the generator is the primary `/tinkerdown`; other capabilities are namespaced `/tinkerdown:<verb>`.)*

**M0 — foundation:** repositioned narrative (README/SKILL/llms.txt) + upstream version bump to latest (`livetemplate` v0.10→latest, client, components).

**M2–M5 — hardening (upstream-first):** `livetemplate.Validate()` API + policy lint · `WithActionPolicy` runtime authz + source introspection · richer `lvt/components` + enforceable design-token style guide · save/share gallery (stores captured skills) + external-embed handshake.

---

## Context

**The reframe.** Tinkerdown today is positioned as *"write markdown, serve HTML — one-file apps."* This plan reframes it as **the tool teams use to let LLMs generate contextual, specific, ephemeral UIs on the fly against their approved data sources — under policy and a house style.** The **UI is disposable**; the **substrate** it is generated from — approved sources, policy, house style, saved skills — is what is durable and reshaped over time.

**The problem it kills.** Internal tooling is built like shipping a phone OS: one bloated "UI for all," never enough eng capacity, a permanent feature backlog, and every builder (PM, SDE, TPM) wanting their own twist. The investment per variation is too high, so most never get built. With LLMs we no longer need the one-UI-for-all pattern. If a team defines its **approved data sources, policies, and a UX style guide** once, plus a **small, constrained UI stack**, an LLM can generate a bespoke, use-and-throw UI in seconds. **The UI is ephemeral; the substrate — sources, policy, style, saved skills — is durable**, so you regenerate from an up-to-date source of truth instead of maintaining a stale app.

**Where existing tools fall short** *(full landscape + data in [§ Appendix C](#appendix-c--competitive-landscape-skip-on-phase-execution))*. Three mature families each solve one axis and none solve all. **Low-code internal-tool builders** (Retool, Appsmith, Superblocks, ToolJet, Budibase, Windmill) are built on the opposite premise — you hand-build and *maintain* one durable catch-all app per workflow; their new AI features generate, and Superblocks **Clark** even *governs* (design system, permissions, audit) — but the output is a **persistent** app in **freeform React**, not a disposable artifact. **AI UI generators** (v0, bolt.new, Lovable, Replit Agent, Val Town, Claude Artifacts) nail prompt-to-UI and sometimes ephemerality, but emit **opaque code you re-prompt** rather than edit, bind to **no governed data** (Claude Artifacts' sandbox *blocks* live data; MCP-connected generative UI reaches whatever is wired, ungoverned), and offer a thousand ways to be subtly wrong. **Declarative data-app frameworks** (Streamlit, Gradio, Evidence.dev, Datasette) are lighter and live-data-friendly but still **human-authored, human-maintained code**. The two closest near-misses each miss one axis: **Superblocks Clark** (governed generation, but persistent + freeform-React) and **Anthropic's generative UI + MCP** (generation + ephemeral + live data, but no approved-sources gate + opaque source). **No tool combines all four of {LLM-generates-bespoke-UI · governed/approved data sources · ephemeral/disposable · editable non-opaque source}** — that intersection is tinkerdown's whitespace, held by three properties nobody else combines: a **constrained `lvt-*` single-file vocabulary** (deterministic *and* hand-editable), **live server-authoritative data over WebSocket** against approved sources, and a **policy gate + ephemeral-by-default** model. It sits in the "malleable / disposable software" lineage — Ink & Switch's *Malleable Software*, Geoffrey Litt's LLM-end-user-programming work, and Thariq Shihipar's *Unreasonable Effectiveness of HTML* (already cited in the README) — but supplies the governed, constrained, live-data substrate those essays don't.

**Ephemerality vs. malleability — and what ephemerality actually buys us.** The malleable-software vision is about *reshaping one tool at the point of use* — the tool persists and bends to you. Tinkerdown is subtly different and, for internal tooling, stronger: **the individual UI is ephemeral; malleability lives in the *substrate*.** You don't evolve one dashboard forever — you evolve the durable assets (**approved sources, policy, house style, saved skills**) and *disposably generate* a fresh, hyper-specific UI from them whenever you need one. What ephemerality gives us that persistent-malleable software doesn't:
- **Zero maintenance / no accumulation.** The pain we kill is *maintaining* internal tooling. A UI you throw away never becomes a liability — no updates, no backlog, no tech debt. A malleable tool that persists still accumulates and must be kept.
- **Tractable governance.** A per-generation policy gate re-evaluates safety *every time* against a bounded, short-lived surface. A long-lived tool that's continuously reshaped has permissions + behavior that drift and are harder to audit.
- **Specificity without generality pressure.** *Durability* is what forces the "one UI for all" bloat ("since we keep it, it must handle every case"). Throwaway removes that pressure — each UI is sharp and minimal for *its* moment.
- **A flipped cost model.** Generation is cheap (seconds); preservation is expensive (forever). We bet on **cheap regeneration from an up-to-date source of truth** over expensive upkeep of a stale UI — the data + policy + style are the durable assets; the UI is disposable.

Malleability is still there for the ones worth keeping — but it means **editing the human-/AI-readable markdown source, or re-running a saved skill** (Design goal #4), not maintaining a frozen app. Net: **ephemerality at the UI layer, malleability at the substrate layer** — the opposite of the one-big-app model, and what makes governance and zero-maintenance both achievable.

**Why now.** Three of the four pillars already exist in tinkerdown:
1. **A constrained vocabulary** — the fixed `lvt-*` attribute set (predictable for LLMs; no generic-LLM-output aesthetic).
2. **Live data adapters** — sqlite, postgres, rest, graphql, file, csv/json, markdown, exec (shell), wasm, computed.
3. **Structural auto-generation** — `auto_tables.go` / `auto_tasks.go` already infer interactive CRUD UI from a source + a plain markdown table/task-list, with **no LLM in the loop**.

The missing fourth pillar is the **policy + generation layer**: a way for a team to declare *approved* sources/actions and a house style once, and a skill that lets an LLM generate a bespoke UI against them, validated deterministically, served ephemerally. This plan builds that fourth pillar and repositions everything around it.

**Design goals (the re-design bar — an ephemeral UI is only worth it if all of these hold):**
1. **Fast to generate.** `/tinkerdown "<intent>"` → live UI in **~30s**; re-run from a saved skill in **seconds**. The framework leg is near-instant — parse + first SSR + WebSocket upgrade ≈ **low tens of ms** (see § Upstream gap analysis), so the budget is the LLM, and "fast" means **one-shot generation reliability**. The levers, all first-class in this plan: the **constrained `lvt-*` single-file vocabulary** (small output surface → fewer tokens, fewer ways to be wrong, fewer retries); **pre-approved sources** in the manifest (no discovery step — the LLM binds to named, known sources); a **rich generation context** (manifest + attribute reference + few-shot corpus); a **fast deterministic `validate` self-correction loop** (real-parser feedback the LLM converges on); and the **capture-as-skill** path (re-run needs *no* LLM at all). Speed is measured + tracked in M1 Phase 5.
2. **Governed.** Generate only against **approved sources + actions**, under policy; the policy lint enforces it, and the operator OKs privileged operations before serving.
3. **Deterministic / correct.** The constrained vocabulary + `tinkerdown validate` (the *real* parser) gate every generated doc to a **clean pass before serving**; the running UI is **server-authoritative** (no client/server drift).
4. **Ephemeral UI layer, malleable substrate layer.** The individual UI is thrown away after use; malleability lives in the durable *substrate* — approved sources, policy, house style, saved skills. Persist a UI on demand by capturing it as a reusable skill; otherwise just regenerate from the (updated) substrate. (See § Context — Ephemerality vs. malleability.)
5. **Editable, non-opaque source.** The artifact is a single **human- and AI-editable markdown file**, not opaque generated code (the differentiator vs. every AI code generator — see § Context "Where existing tools fall short").
6. **Fix gaps upstream, not in tinkerdown** (`../livetemplate` server + `../client`); where a capability legitimately belongs in tinkerdown (generation→serve orchestration) the plan says so explicitly.

*(1–3 are the load-bearing triad: **fast + governed + deterministic**. If any fails, generated-on-the-fly UIs aren't usable — too slow, unsafe against real data, or unreliable.)*

---

## The reference demo — a PII / data-export access-approval console (locked)

This is the concrete M1 target. Chosen from a research pass over real request→human-approval workflows (see Appendix A for the rejected alternatives and why). It removes **high-stakes, real friction** and exercises the entire reframe.

**The friction (real, high-stakes).** An analyst or support agent needs access to sensitive data — a PII export, a prod-DB read — to resolve a chargeback dispute, service a GDPR data-subject request, or debug a customer issue. Today the loop is: **Slack-ping someone with prod access → they hand-run a query → weak/absent audit trail → compliance exposure** (who accessed which PII, when, why, for how long — hard to prove to an auditor). This is over-broad, slow, and unauditable. Real HITL-approval tooling frames PII/data-export as the case that *must* route to a review tier with a durable audit record.

**The generated console (what the skill produces).** *(The concrete `app.md` + the workspace manifest it references are in [§ Appendix B — Expected output](#appendix-b--expected-output-worked-example-skip-on-phase-execution).)*
- **Pending-requests queue** (a data source). Each row shows what an approver needs to decide safely: requester + team, the dataset/resource requested (e.g. `orders_pii`, `users_email`), the scope (row estimate / filter / a preview of the exact scoped query), business justification, linked ticket/incident, requested duration/TTL, requested-at, and sensitivity/compliance tags (PII/PCI/GDPR).
- **Approve** (`confirm:` action) → produces a **concrete, auditable output**: (1) a durable **audit record** (approver, timestamp, exact scope, reason, TTL) appended to a writable source, and (2) the concrete action — a **scoped, parameterized export job** (a bounded query, delivered as a time-boxed artifact) *and/or* an optional **PR to an access-grants-as-code file** (`access-grants/<date>-<user>-<dataset>.yaml`) for GitOps-minded teams. Primary output is the grant + audit record; the PR is an optional git-native variant that reuses the same `gh pr create` primitive.
- **Deny** (`confirm:` action) → transitions the request to `denied`, records approver + reason, notifies the requester. No data access granted.
- **Request intake** → the app's own "Request access" form (a writable source), or a Slack slash-command / webhook POST for realism.

**Why it fits the reframe perfectly (existing primitives only):**
- **Requests queue** → a data source (sqlite/markdown/rest). *(reuse `auto_tables`.)*
- **Approve / Deny** → named `confirm:` actions in the manifest (`sql`/`exec`/`http` with typed params). *(reuse `config.Action` + `execargs` typed form.)*
- **Scoped export job** → a parameterized `sql`/`exec` action (bounded query). *(reuse `execargs`/`SQLExecutor`.)*
- **Audit record** → a writable source append (sqlite/markdown). *(reuse `WritableSource`.)*
- **House style** → `styling.site_css` + theme.
- **Ephemeral** → served via the disk-free `ParseString` + playground-session path.

**The two safety layers (generic machinery, not a separate demo).** Every generated UI passes through the same gate; here it's shown applied to the PII console. (1) The **policy lint** (automated, always on) rejects a generated app that references any source/action the manifest didn't approve — the non-redundant safeguard, not a re-approval of the operator's intent. (2) **Operation transparency** (proportional): because an *LLM* authored the implementation, generation surfaces the *concrete* privileged operations the generated app will run before it serves — for this PII console: read the requests store, run the **scoped** export `SELECT … FROM orders_pii LIMIT 500`, append audit records, and (optionally) `gh pr create`. The operator reviews the actual behavior their one-line intent didn't specify; a read-only UI would get no such prompt. Both layers are **generic skill machinery** (manifest + generate skill, built once), not demo-specific code — per the "generic core library code" rule.

**Locally runnable E2E (no real prod DB, no real PII).** A fixture SQLite of *synthetic* "PII" rows makes the export job real but harmless; the requests queue + audit log are seeded/writable SQLite (or markdown); the optional access-grant PR uses a fixture git repo + the `gh` CLI. Approve runs a genuinely scoped export against the fixture DB, writes a real audit entry, and (optionally) opens a real PR — all demoable without touching production.

---

## The four team-defined inputs — and the house-style gap

The brief names four things a team defines once so LLMs can safely generate against them: **(1) approved data sources, (2) policies, (3) a UX style guide, (4) a small UI stack.** Tinkerdown covers 1, 2, and 4 well (source adapters; the manifest's approved-set + `confirm:` actions; the fixed `lvt-*` vocabulary + `lvt/components`). **(3) is the weakest today** — the exploration flagged it: house style is just `styling.theme` (clean/dark/minimal) + `styling.site_css`, with *no design-token schema and no enforced component set*. That's a problem, because "no generic LLM-output aesthetic" is a core selling point — a bespoke UI that ignores the house style is off-brand.

**How the plan threads the UX style guide:**
- **M1 (this milestone) — make house style LLM-consumable, coarse enforcement.** The manifest's style block = `theme` + `site_css` **plus a short `style-guide.md`** (house tone, layout conventions, which `lvt/components` to prefer, do/don't) that the `/tinkerdown` skill injects into its generation context so output conforms by construction. **The whole style block is optional** — with none, generation falls back to tinkerdown's sane defaults (PicoCSS semantic styling + default theme), so a team can adopt generation with zero style setup and add house style later. Enforcement in M1 is coarse (the shared `site_css` skins every generated page; the visual-conformance check in Phase 5). *This is a conscious M1 scope: consume the style guide, don't yet mechanically enforce tokens.*
- **M4 — elevate to an enforceable spec.** Alongside the `lvt/components` enrichment, promote house style to **design tokens + an enforced component set** (the real "style guide object" the exploration found missing), so generated UIs can't drift off-brand even without a careful prompt.

---

## Upstream gap analysis — what to fix in `livetemplate`/`client`/`lvt` vs tinkerdown

*(From a direct read at plan-authoring time: `../livetemplate` @ v0.16.0, `../client` @ v0.16.2, `../lvt/components`. Targets below are "latest at execution" — see § Tech stack.)*

**Version reality — the cheapest lever first.** Tinkerdown pins **`livetemplate v0.10.0`** (`go.mod:11`) and an old client; upstream was **v0.16.0 / client 0.16.2 at plan-authoring time** (and keeps moving — M0 Phase 0 targets whatever is latest at execution, not these numbers). Six+ minors of relevant fixes already shipped and are simply unconsumed: **ephemeral sweep-TTL** (`WithEphemeralSweepTTL`), **verbatim dynamic content** (no whitespace-collapse corrupting code/`pre-wrap` — matters when generated UIs echo code), **app-level heartbeat/liveness**, and **`data-lvt-force-update`** (one-shot server-authoritative override for checkboxes/radios/`<details>`). **First move is a version bump, not new code.**

| # | Gap | Home (fix upstream, not tinkerdown) | Needed for | Milestone |
|---|-----|------|-----------|-----------|
| 0 | Old pins hide shipped fixes | Bump `livetemplate`, client, `lvt/components` → latest tags | Ephemeral speed + determinism | **M0** |
| 1 | No validation API for a generated attribute-doc (allowed tags/`lvt-*`, diff-cleanliness, bound refs) | `livetemplate.Validate(templateText)` [server] | Deterministic first-pass correctness | **M2** |
| 2 | No state/source **introspection** (fields + action names as metadata) | Reflective metadata surface [server] | LLM binds to real fields | **M3** |
| 3 | Permission enforcement is coarse; no per-action/field authz | Declarative `WithActionPolicy` hook [server] | Runtime safety (defense in depth) | **M3** |
| 4 | Primitive library too thin (no charts, badges, cards, stat tiles, alerts, empty-states) | Enrich `lvt/components` [components] | "LLMs rarely need custom HTML" | **M4** |
| 5 | External-app embedding is iframe-bridge only | Embed handshake [client] | Sandboxed external embeds | **M5** |
| 6 | Generation→validate→serve **orchestration** | **Legitimately tinkerdown** (all 3 exploration agents concur) | The skill itself | **M1** |

**The load-bearing decision (per operator sign-off):** M1 ships the demo on primitives that already exist (items 6 + M0's bump); items 1–5 are explicit *hardening* milestones **after** the demo runs. This is a deliberate sequencing choice, not a shortcut — the generation orchestration is the one layer that always lived in tinkerdown.

---

## Architecture overview

```
   Operator (in Claude Code)
        │  /tinkerdown "a console to approve PII / data-export access requests"
        ▼
  ┌─────────────────────────────────────────────────┐
  │  Claude Code skill (skills/tinkerdown)          │  [M1 — tinkerdown]
  │  1. read workspace manifest (approved sources,  │
  │     actions, style guide) + attribute reference │
  │     + few-shot corpus                           │
  │  2. emit app.md (constrained lvt-* vocabulary)  │
  │  3. `tinkerdown validate` ── loop until clean ──┐│  ← deterministic gate
  │  4. if privileged: show operation summary,      ││    [M1: real parser;
  │     operator OKs the concrete ops (else skip)   ││     M2: + Validate() API]
  │  5. `tinkerdown serve` (ephemeral session)      ││
  └─────────────────────────────────────────────────┘│
        │                                            │
        ▼                                            │
  ┌───────────────────────────┐                      │
  │ tinkerdown runtime (Go)   │                      │
  │ ParseString → live page   │◀── validate ─────────┘
  │ ephemeral session (TTL)   │      (same real parser as serve)
  │ approved sources only ────┼──▶ requests store / scoped export / audit
  │ confirm: actions ─────────┼──▶ approve → export job + audit record
  │                           │      (+ optional gh pr create grant)
  └───────────────────────────┘
        │  SSR HTML + WebSocket tree-diffs
        ▼
  Browser: live UI (server-authoritative; data-lvt-force-update)
        └── @livetemplate/client (constrained lvt-* vocabulary)  [upstream]
```

**Key flows.**
1. **Generate** — skill reads manifest + reference → emits `app.md` → `validate` loop → review → `serve`. *(M1)*
2. **Approve** — a `confirm:` action that runs a scoped export + appends a durable audit record (+ optionally opens a GitOps grant PR via `gh pr create`). *(M1)*
3. **Policy gate at generation** — the skill may only wire up sources/actions the manifest approves; `validate` (M1) + `livetemplate.Validate()` (M2) enforce doc cleanliness; `WithActionPolicy` (M3) enforces at runtime. *(M1→M3)*
4. **Ephemeral lifecycle** — disk-free `ParseString` + TTL session; thrown away or saved to a gallery. *(M1 render; M5 save/share)*

---

## Package layout (shape only; per-file detail lives in each phase)

**tinkerdown (this repo):**
- `skills/tinkerdown/` — the **existing** skill, extended so **generation is its default action** (`/tinkerdown "<intent>"`); sub-capabilities are **namespaced** (`/tinkerdown:save`, …). Adds the generation context (reference + few-shot corpus) + validate loop.
- `internal/config/` — extend for the **workspace manifest** (approved sources, named actions, optional style guide + `describes:` metadata). *(reuse `config.go` `SourceConfig`, `AuthConfig`, `Action`, `StylingConfig`.)*
- `internal/server/playground.go` — the disk-free ephemeral render substrate (`ParseString`, `PlaygroundSession`) the generate path serves through.
- `cmd/tinkerdown/commands/validate.go` — the deterministic feedback gate (already parses via real `ParseFileInSite`); extend with **policy validation** (approved-source/action lint).
- `internal/source/{exec.go,sqlite.go}`, `auto_tables.go`, `execargs` (typed action forms), `WritableSource`/`SQLExecutor` — the queue + scoped-export + audit-append + approve-and-run primitives the demo reuses.

**upstream (fix here per the rule):**
- `../livetemplate` — version bump (M0); `Validate()` API (M2); introspection + `WithActionPolicy` (M3).
- `../client` — consume via bump (M0); embed handshake (M5).
- `../lvt/components` — enrich vocabulary (M4).

---

## Skills delivered (a first-class deliverable, per convention 13)

**Naming convention:** the generator is the **primary default skill `/tinkerdown`** (generation is what you get by default); every other tinkerdown capability is **namespaced** `/tinkerdown:<verb>` (e.g. `/tinkerdown:save`). Captured workflow skills are their own standalone skills. The reframe ships:

1. **`/tinkerdown` — the primary generator** (M1 Phase 3). The existing `skills/tinkerdown/` skill, extended so its **default action** is: read the manifest + attribute reference + few-shot corpus → emit `app.md` → validate loop → review → ephemeral serve. Produces *any* bespoke UI against approved sources.
2. **`/tinkerdown:save` — the capture/persist capability** (M1 Phase 6) — a **namespaced sub-skill**, **invoked when the user chooses to persist** an ephemeral UI, that distils the just-completed generation/development conversation into a durable, re-runnable skill following Anthropic best practices. This is the *persist* path of convention 13 — **opt-in, never automatic** (ephemeral UIs are throwaway by default).
3. **`skills/pii-access-approval/` — the concrete captured artifact** (M1 Phase 6). The multi-step PII / data-export approval workflow *deliberately persisted* (via `/tinkerdown:save`) as a standalone reusable skill: the manifest, the golden `app.md`, the approve/deny/export/audit wiring, and how to serve it — so an operator gets the working console in seconds (the "save it — the need recurs" path), not by re-generating. Both the demo artifact and the worked example of capture-on-persist.

All follow the Anthropic skill shape already used by `skills/tinkerdown/` (concise `SKILL.md` + `reference.md` + `examples/` + a `validate` step + explicit triggers).

---

## Roadmap

Ordered milestones. **M0 and M1 are fully detailed** (full phase blocks); M2–M5 are outline-only and get expanded at their kickoff (see LLM session guide convention 9).

- **M0 — Reframe + upstream bump + a trustworthy vocabulary.** Reposition the narrative (README/SKILL/llms.txt/ai-generation) around ephemeral generated UIs; **bump `livetemplate` + client + components to their latest tags** and absorb behavior changes; **reconcile the `lvt-*` attribute reference with what the client actually implements** (Phase 2 — added after Phase 1's Audit found 8 of 11 sampled attributes stale). *Lights up:* the fast, correct runtime substrate, the honest story, and a vocabulary reference that is safe to hand an LLM — the last being a hard prerequisite for M1, not polish. *(no demo yet)*
- **M1 — THE DEMO (thin vertical slice).** The five M1 deliverables (see § Deliverables at a glance) on existing primitives. *Lights up:* the target acceptance test — generate a real, friction-removing UI in ~30s — *and* re-run it from a skill in seconds.
- **M2 — Deterministic validation (upstream).** `livetemplate.Validate(templateText)` (diff-cleanliness + attribute diagnostics) + tinkerdown **policy lint** in `validate`. *Lights up:* first-pass generation reliability; reject unknown `lvt-*` / non-approved refs before serving.
- **M3 — Runtime policy + introspection (upstream).** `WithActionPolicy` per-action/field authz + state/source introspection metadata. *Lights up:* defense-in-depth (the running app can't exceed granted access) + LLM binds to real fields.
- **M4 — Component vocabulary + enforceable house style (upstream `lvt/components` + tinkerdown).** Charts, badges, cards, stat tiles, alerts, empty-states; **plus** promoting the UX style guide from coarse (`site_css` + `style-guide.md`) to **design tokens + an enforced component set** (the "style guide object" the exploration found missing). *Lights up:* richer generated dashboards with no free-form HTML, guaranteed on-brand.
- **M5 — Persist to the malleable substrate (save & share).** This is where **malleability lives**: persist a generated UI to the repo + a gallery + share link by storing the captured **skill** (per convention 13 / M1 Phase 6), not just the `app.md` — so "save" means a re-runnable workflow you evolve, while individual UIs stay ephemeral; plus **substrate extensibility** (teams can add their own approved source types) and the external-app embed handshake. *Lights up:* "throw away the UI, keep + reshape the substrate" (folds issues #223 host read-only apps, #282 review mode, #249 external embed, #216 writable WASM sources, #222 custom Go+WASM sources).

**Open-issue walkthrough.** Triaged into three buckets against this reframe. *(Counts drift: **48 open** at plan authoring, **82** by 2026-07-19 — the repo files `from-review` issues continuously. The named issue→milestone mappings below stay valid; **re-run the triage at M0 kickoff** rather than trusting the totals, same as the version pins.)*
- **Folded into milestones (reframe-aligned):** #223 host read-only apps → **M5** gallery; #282 review mode → **M5**; #249 `lvt-external` embed any framework → **M5** embed handshake; #216 writable WASM sources → **M5** sources; #222 custom sources in Go+WASM → **M5**; #226 ROADMAP lifecycle-attr pattern + #230 `lvt-preserve`→`lvt-ignore` doc fixes → **M0** reframe pass.
- **Reliability/tech-debt, pulled in only if a phase's Audit finds it blocking:** #269 hot-reload serves stale `/`, #275 Kroki timeout blocks discovery, #259 multi-range highlight overlay misalign (all P2) — none sit on the M1 critical path, but #269 touches serve/hot-reload which M0's bump also touches, so M0 Audit checks it.
- **Orthogonal `from-review` follow-ups (~35 at authoring and growing — #258–#273 etc., mostly P3/P4):** refactors/test-coverage/perf nits with no bearing on the reframe. Not scheduled here; left to normal backlog grooming. Explicitly *out of scope* so the reframe stays a thin vertical, not a debt-paydown.

---

## ⬇ Execution contract (human reviewers can stop above)

*Everything below is for the implementing LLM — the session rules, the per-phase checklists, verification, and the decisions log. A product reviewer doesn't need to read it end-to-end.*

---

## LLM session guide — how to execute this plan

Consumed by Claude Code, **one phase per session**. This is the executing LLM's contract.

1. **Read only what this phase needs.** Each phase's **Design refs** cite specific sub-sections. Read those + the phase block. Sharpen a vague Design ref in the same commit if you had to read more.
2. **Audit first, code second.** Every phase opens with **Audit** — verify prerequisites, surface ambiguity, enrich the Implementation list inline with new `[ ]` items. Complete it before implementation.
3. **Acceptance criteria are the success bar** — not the Implementation checkboxes. First criterion is always **/simplify** against the diff.
4. **Persist drift to the plan** in the same commit as the code. The plan is the source of truth.
5. **Learn forward — surprises only.** End each phase with **Learn** (4 prompts). "No surprises" is valid. Predicted findings belong in Audit or Risks, not Learn.
6. **Stay in scope.** Only the Implementation checklist + audit-derived additions. Ask before adding anything else.
7. **One coherent commit per phase.**
8. **When in doubt, ask.** Prefer a clarifying question over an invented design decision.
9. **Outline expansion is a milestone's first task.** M2–M5 designs are outline-only; the first phase of each milestone expands its design (informed by the prior milestone's Learn) + produces any mockups, then re-points that milestone's Design refs, before implementation.
10. **"Work on the next phase" → auto-position.** Scan the progress tracker top-to-bottom for the first phase with unchecked `[ ]` Implementation boxes; surface any incomplete prior-phase Learn; read Goal + Design refs + prior Learn; begin with Audit. A named phase overrides.
11. **Cross-repo phases target the upstream repo explicitly.** For M0/M2/M3/M4/M5 work living in `../livetemplate`, `../client`, `../lvt`: point `/simplify`, `/prereview`, and the delivery protocol at the external repo (`git -C <repo> diff`); run the delivery loop once per upstream repo; **cut a tagged release** of each upstream package (via its `release.sh`) so tinkerdown can pin it; verify the consuming demo builds against the **published** dep (no `go.work` shim committed).
12. **Dogfood the ecosystem — no custom JS, no extra UI deps.** All interactivity via `lvt-*` (client + `lvt/components`). If a UI primitive is missing, **stop the phase, fix upstream first, ship it, then resume** — add the upstream work as a checked Implementation item so the dependency is visible. Necessary service clients (GitHub API, etc.) are exempt.
13. **Persisting an ephemeral UI = capturing it as a reusable skill (opt-in, not automatic).** Ephemeral UIs are **throwaway by default** — that's the point of the reframe. But **when the user decides an ephemeral UI is worth keeping** (the "save if the need recurs" path), don't just save the `app.md`: **capture the whole workflow as a reusable Claude Code skill** so it re-runs *efficiently, effectively, and quickly* (near-instant) instead of being re-generated. The capture distils the working artifact + the steps that produced it (from the development/generation conversation) into a `SKILL.md`, following **Anthropic's skill-authoring best practices**: progressive disclosure (concise `SKILL.md` + deeper `reference.md`), explicit `name`/`description`/`triggers` frontmatter, bundled few-shot examples + any helper scripts, a clear "when to use / when not to," and a validation step. This is a reusable capability — the namespaced **`/tinkerdown:save`** sub-skill (see § Skills delivered) — **invoked on the user's request to persist, never fired automatically after generation.**

Sections marked `[skip on phase execution]` (Appendix A) are historical context for humans.

---

## Per-phase delivery protocol (defined once)

**Precondition:** `prereview` CLI reachable; `gh` authenticated. After Implementation + Acceptance pass locally, every phase runs the same loop:

1. **Manual verification in a real browser** at the devbox URL (never `curl`) — watch browser console + server stderr + WS frames while clicking through. Non-optional for any UI-touching phase.
2. **`/prereview` hand-off** → iterate until the user signs off.
3. **Open PR** → the Claude Code review bot auto-reviews.
4. **`/prcommentsfix` convergence loop** against bot comments until clean.
5. **Final post-bot signoff → merge.** Never self-merge without explicit signoff.

---

## Implementation phases — progress tracker

> Tick boxes as work completes. Each phase ends with `go test ./...` green, and — for E2E — the CLAUDE.md four-channel capture: browser console, server stderr, WebSocket frames, rendered HTML + screenshot.
>
> **How the e2e tests are actually gated** (corrected in Phase 0 — there is no `browser` build tag in this repo): the 32 e2e files are `//go:build !ci`, so they run **by default** locally and are excluded in CI, which runs `go test -tags=ci -skip='E2E|e2e' ./...` — a belt-and-braces exclusion via both the build tag and a name skip. Two consequences worth internalizing: a plain local `go test ./...` **is** the browser suite (no opt-in flag), and **CI structurally cannot catch an e2e regression**, so the local run is the only gate.
>
> Verification that consumes an upstream dependency must run under **`GOWORK=off`** — a `go.work` one directory up redirects `livetemplate`/`lvt` to local checkouts that are typically *ahead* of the published tag, so without it you are not testing what you pinned (session-guide convention 11).

**Phase block shape (every phase):** `Goal at end` · `Design refs` (specific sub-sections) · `Audit` (first task; starts with a Design-ref completeness check) · `Implementation` (`[ ]` checkboxes) · `Acceptance criteria` (Simplify → Unit → Integration → E2E) · `Learn` (4 prompts: what surprised us / plan drift fixed / feed-forward to next Audit / new-or-changed risks).

**Phase sizing:** each fits one Claude Code session (~1–3 days human-equiv). Overrun → split.

---

### M0 phases — reframe + upstream bump (full detail)

#### Phase 0 (M0) — Upstream version bump: `livetemplate` + client + components to latest (~1 session)

> **Goal at end:** tinkerdown builds and all tests (incl. browser e2e) pass green against **the latest tagged `livetemplate` release**, the matching `@livetemplate/client`, and the latest `lvt/components` — with any behavior changes absorbed. *(v0.16.x is the floor at plan-authoring time; upstream will have advanced by execution — the Audit re-checks the current latest and targets that.)*

**Design refs:**
- § Upstream gap analysis (version reality; the 4 shipped-but-unconsumed fixes)
- `go.mod:11` (server pin), the embedded client version, `internal/server/` (render/ws), `parser.go`/`page.go` (attribute passthrough)
- Upstream `../livetemplate/CHANGELOG.md` v0.11→latest entries (kebab action routing, comment stripping, verbatim dynamic content, heartbeat, ephemeral sweep-TTL, force-update — plus anything shipped after v0.16)

**Audit:**
- [x] **Design-ref completeness check** — each Implementation item maps to a cited sub-section.
- [x] **Re-check the current latest upstream versions first** — upstream advanced well past the plan-time floor: `livetemplate` **v0.19.1** (not v0.16), `lvt/components` now has a real **v0.2.0** tag (no more pseudo-version), `@livetemplate/client` **0.18.2**.
- [x] Read the upstream CHANGELOG from tinkerdown's current pin **through the latest tag** in full. The feared breakers (comment stripping, kebab action routing, verbatim dynamic content) predate v0.16 and were already absorbed. The live range v0.16→v0.19.1 is additive: `__ping__` heartbeat, `WithParseFS`, `ClientVersion` pinning, scoped method precompute, per-item recursive range diffs, and two silent-update-loss fixes.
- [x] Confirm the embedded/pinned client version and how it's shipped — **embedded asset**, not CDN: `client/` bundles `@livetemplate/client` via esbuild into `internal/assets/client/tinkerdown-client.browser.js`, which is tracked in git. So the bump is `client/package.json` + reinstall + rebuild + commit the regenerated bundle.
- [x] Check `lvt/components` pseudo-version → **yes**, `v0.2.0` is tagged and pinnable.
- [x] Inventory tinkerdown tests that assert on exact rendered HTML/whitespace — **none assert on WS frames or tree-diff bytes**, so v0.19.0's range-diff format change has no test surface. The exact-HTML assertions that exist are all in unit tests over tinkerdown's own parser output, below the livetemplate boundary.
- [x] **(added)** Verify the removed-API risk: upstream removed exactly one symbol in range (`WithStore`, v0.19.0). tinkerdown's livetemplate API surface is 10 symbols and does not include it.

**Implementation:**
- [x] Bump `go.mod` server pin to the **latest tagged release**: `livetemplate v0.10.0 → v0.19.1`, `lvt/components → v0.2.0`. `go mod tidy`.
- [x] Bump the client: `client/package.json` `^0.14.3 → ^0.18.2`, `npm install`, rebuild, copy the regenerated bundle into `internal/assets/client/`.
- [x] Fix compile/API breaks; absorb behavior changes — **none required**. Build and full test suite are green with zero source changes.
- [x] ~~Wire up `WithEphemeralSweepTTL` on the playground/ephemeral session path~~ — **dropped, see Learn.** It is a `HandleOption` and tinkerdown never calls `tmpl.Handle()`; there is no call site. Confirmed `data-lvt-force-update` survives the rebuild (the #295 canary).
- [x] **(added)** Fix `make build` to run `npm ci` before `npm run build` (issue #295) — without it the next `make build` silently reverts this phase's bundle.
- [x] Update `CHANGELOG.md`.

**Acceptance criteria:**
- [x] **Simplify:** N/A in substance — the diff is a dependency bump plus a one-word Makefile fix and CHANGELOG prose; there is no hand-written logic to simplify. Recorded rather than ticked hollow.
- [x] **Unit:** `go test ./internal/... ./cmd/...` green (20 packages).
- [x] **Integration:** source integration tests (sqlite/rest/exec) green.
- [x] **E2E:** the discriminating set — `TestAutoTables_*`, `TestAutoTasks_*` (checkbox toggle), `TestExecToolbar*`, `TestLvtSourceMarkdownToggle*` — **14/14 green**, WebSocket connecting and diffs applying. Full `GOWORK=off go test ./...` green (root package, holding all 32 `!ci` e2e files, 827s).
- [x] **Bundle provenance (directional, not just "it changed"):** `ctrlKey` occurrences **1 → 5**, matching v0.18.2's client fix for bare-key `lvt-on:keydown` firing while Ctrl/Meta/Alt is held; `data-lvt-force-update` canary intact at 2 (the #295 signature would have been a drop to 0). A size delta alone would not have distinguished an upgrade from a downgrade — both differ from HEAD.
- [x] **CSS half of the bundle verified** (a JS-only check would have missed it): `tinkerdown-client.browser.css` is tracked, was copied, and is byte-identical to the fresh build — correctly so, because the bump does not touch tinkerdown's own CSS sources. The client package's separate `livetemplate.css` is deliberately *not* bundled; it contains only CSS custom-property defaults for `lvt-fx:*`, a directive family tinkerdown uses nowhere.
- [x] **Manual visual verification** (delivery-protocol step 1, browser not `curl`): served `examples/action-buttons` and captured a full-page screenshot + console. Theme toggle, card/table styling, styled form controls, three button colour variants, and syntax highlighting with copy buttons all render correctly; console shows block discovery → WS connect with **no JS exceptions**. This is the visual layer the chromedp assertions do not cover.

**Learn:**

*What surprised us.*
1. **The bump was a non-event in code and the entire risk lived in the JS bundle.** Nine minor versions of upstream drift produced *zero* required source changes — tinkerdown's livetemplate API surface is 10 stable symbols, and the sole removed API (`WithStore`) was unused. The real hazard was issue #295's trap: `client/node_modules` was stale at **0.11.9** while the lockfile pinned 0.14.3, so any rebuild-without-install silently downgrades the committed bundle. Working in a fresh worktree defused this structurally — `node_modules` is gitignored, so the worktree had none and `npm install` produced the correct version by construction.
2. **The client version is a wire contract, not a preference.** livetemplate v0.18.0 added `ClientVersion` *because there is no runtime server↔client version handshake*; v0.19.0 declares `0.18.2` as the compatible pair. "Latest client" and "correct client" happened to coincide here, but the reasoning must stay pinned to the declared pair.
3. **Verifying a regenerated bundle needs a *directional* check, and it must cover CSS too.** "The bundle differs from HEAD" is worthless when the failure mode is a silent *downgrade* — #295's bad bundle also differed. What discriminates is a marker tied to a known changelog entry (here `ctrlKey` 1→5 for a v0.18.2 fix). Separately, the first provenance pass covered only `.js`/`.js.map` and would have missed a stale stylesheet entirely; the CSS turned out identical *for a good reason*, but that had to be established, not assumed.
4. **A `go.work` at `/home/adnaan/code/livetemplate/` silently redirects builds to the local upstream checkouts** (currently `v0.19.1-5-g…`, i.e. *ahead* of the tag). Local builds were never testing the published dependency. All verification here ran under `GOWORK=off` — which is what session-guide convention 11 ("verify against the **published** dep") actually requires in practice. **This should be the default for any upstream-consuming verification in M2–M5.**

*PLAN.md drift fixed in this commit.*
- The `WithEphemeralSweepTTL` Implementation item assumed tinkerdown serves through `tmpl.Handle()`. It does not — it calls `livetemplate.New()` directly from its own WebSocket handler, and the playground runs an independent session-cleanup loop. The item is struck through with the reason rather than deleted, so M1 does not re-propose it.
- § Tech stack version-pin table updated from the plan-time floor to the versions actually adopted.
- Added the `make build` / #295 fix as an audit-derived Implementation item.
- **`go test -tags=browser ./...` was fiction** — no `browser` build tag exists anywhere in this repo. It appeared in the tracker preamble and § Verification, implying the browser suite is opt-in behind a flag. It is the reverse: the 32 e2e files are `//go:build !ci` and run **by default** locally, while CI excludes them twice over (`-tags=ci` *and* `-skip='E2E|e2e'`). Corrected in both places, with the gating mechanism spelled out — a future session that believed the tag was load-bearing could have concluded it had run the browser suite when it had not, or that skipping the flag was a legitimate option.
- § Verification now records the `GOWORK=off` requirement and the "build the binary before e2e" precondition.

*Feed-forward to **Phase 1 (M0)**'s Audit.*
- **A latent contradiction with the reframe's "disk-free ephemeral path" claim:** `internal/server/websocket.go:467` writes every inline block to `/tmp/lvt-<blockID>.tmpl`, with the comment *"livetemplate.New() requires template files"*. That constraint is now obsolete — v0.17.0 shipped `WithParseFS(fsys, patterns...)`, so an in-memory `fs.FS` removes the per-block disk round-trip entirely. This bears directly on the plan's **speed** non-negotiable and on M1's ephemeral serve path. Deliberately *not* done here: refactoring a WS hot path inside a version-bump phase would destroy the phase's isolability. **Size it in M1 Phase 3's Audit**, where "which ephemeral serve mechanism, measured" is already an open question.
- Phase 1 (M0) rewrites the narrative around ephemerality; the `WithParseFS` finding is the concrete engineering fact behind that claim — don't write "disk-free" into the README until it is.

*New / changed risks.*
- **Retire the "Multi-minor upstream bump may ripple" risk** — it did not ripple; the bump is green with no source changes. What *did* bite is bundle provenance, now mitigated by the `npm ci` fix.
- **New (low, M2–M5):** the `go.work` shim means local upstream work can pass against unreleased code and fail against the published tag. Convention 11 already mandates verifying against the published dep; the operational form of that is `GOWORK=off`.

#### Phase 1 (M0) — Reframe the narrative (README, SKILL, llms.txt, ai-generation) (~1 session)

> **Goal at end:** the top-line story leads with ephemeral, LLM-generated, policy-gated internal UIs; the exec-source "use sparingly" framing is replaced by "approved + gated"; stale docs (#226 ROADMAP, #230 lvt-preserve→lvt-ignore) fixed.
>
> **Why this is mostly repositioning, not net-new writing:** the repo already half-believes the reframe (archived `docs/archive/ROADMAP.md` vision: *"platform… AI systems generating functional apps from natural language"*; `docs/research/examples/llm-generated-apps/`; a ready `docs/llm-system-prompt.md`; the `basic` scaffold is literally "k8s pods via exec"). The gap is that the **top-line story** (README/SKILL/llms.txt) still leads with "one-file markdown apps," and the LLM prompt *steers away* from exec because nothing gates it. This phase also makes good on a promise the docs already made: `docs/guides/ai-generation.md` advertises `/lvt-plan`, `/new-app`, `/add-resource` commands **that were never built** — redirect them to the real `/tinkerdown` (M1).

**Design refs:**
- § Context
- `README.md`, `skills/tinkerdown/SKILL.md`, `docs/llms.txt`, `docs/llm-system-prompt.md`, `docs/guides/ai-generation.md` (advertises unbuilt commands), `docs/archive/ROADMAP.md`, **`docs/reference/lvt-attributes.md`** *(added during Audit — #230's primary target; the original ref listed only the archived ROADMAP copy)*

**The governing constraint (operator decision at kickoff): claim only what is true at M0.** The reframe's *destination* is policy-gated generation, but M0 ships none of it. Every edit in this phase states the present tense honestly and forward-references the rest. Three edits were at risk of writing M1 capability as shipped — the phantom-command redirect, the exec framing, and any `/tinkerdown "<intent>"` example — and all three are handled below.

**Audit:**
- [x] **Design-ref completeness check** — **incomplete as written**: #230's real target is `docs/reference/lvt-attributes.md` (live reference), not just the archived ROADMAP the ref listed. Ref sharpened above, per convention 1.
- [x] Read the current README/SKILL/llms.txt lead sections; list the "one-file markdown app" framings to reposition. Repositioned the leads and `SKILL.md`'s frontmatter `description` + `triggers` (which told the old story and would otherwise contradict the rewritten body). Progressive-complexity tiers, concrete examples, and the "Why not just ask Claude for an HTML file?" section kept — they already carried the reframe.
- [x] Confirm which advertised-but-unbuilt commands in `ai-generation.md` to remove vs. redirect — **all five are phantom** (`/lvt-plan`, `/new-app`, `/add-resource`, `/quickstart`, `/troubleshoot`), not the three the plan assumed; `skills/tinkerdown` is the only skill in the repo. Redirecting them to `/tinkerdown` would have swapped five phantoms for one, since `/tinkerdown`-as-generator is itself M1. Replaced with the flow that works today.
- [x] Decide: does `llm-system-prompt.md` seed the M1 skill's generation context? **Yes** — it is already a 285-line attribute-and-source primer, and its only anti-reframe line was the exec framing, now fixed. Feed-forward to M1 Phase 3's "decide the skill's generation context" Audit item: adopt it as the seed rather than authoring a new prompt.
- [x] **(added)** Verify the `lvt-*` renames against the client repo before applying them, per project CLAUDE.md. Necessary: `lvt-ignore`/`lvt-ignore-attrs` are real (`livetemplate-client.ts:1917`, checked on `fromEl`), but the client also has a *separate* `lvt-form:preserve` (`state/form-lifecycle-manager.ts:67`) that a careless reading of #230 could have confused it with.

**Implementation:**
- [x] Rewrite README lead + add a "The problem" section framing the reframe (internal tooling's cost-per-variation, and what changes when an LLM can hit a constrained vocabulary). Augmented rather than replaced — the existing bullets already carried half the story.
- [x] Update `SKILL.md` (frontmatter + lead + a "Generate, then validate" section) and `llms.txt` to describe the generate-and-throw-away model, keeping `llms.txt`'s required-header contract intact.
- [x] Fix `ai-generation.md`: **removed** the five-command table; documented the real loop (describe → assistant writes `app.md` → `validate` → fix → `serve`) and forward-referenced the dedicated generate skill as in development, linking this plan.
- [x] Doc fixes: #226 (ROADMAP lifecycle pattern → `lvt-el:{method}:on:{state}`), #230 (`lvt-preserve` → `lvt-ignore` in `docs/reference/lvt-attributes.md` + the archived copy).
- [x] Do **not** change exec gating — reframed the *narrative* only: `docs/llm-system-prompt.md` went from "use sparingly" to exec as a **privileged first-class source gated behind `--allow-exec`** (today's real gate, verified at `serve.go:58`, `websocket.go:1293`, `webhook.go:712`). The section describes that gate and promises nothing further — M1's manifest approval is deliberately *not* forward-referenced here, since an LLM system prompt should state current rules, not roadmap.

**Acceptance criteria:**
- [x] **Simplify:** prose pass — each file made internally coherent (lead, frontmatter, and triggers telling one story), not just its opening paragraph.
- [x] **Unit:** `skill_examples_test.go` + `TestLLMSTxtExists` green (5/5; `llms.txt` retains `# Tinkerdown`, `## Quick Start`, `## Key Attributes`, `name=`, `lvt-source`).
- [x] **Integration/E2E:** N/A (docs). No code snippet changed in a way that alters its validity; the edits are prose, headings, and attribute names.

**Learn:**

*What surprised us.*
1. **The plan under-counted the phantom commands and mis-scoped the fix.** Five, not three — and the Implementation's "redirect to the real `/tinkerdown` (M1)" would have replaced five non-existent commands with one that also does not exist yet. The trap is subtle because the redirect *reads* like a fix. The operator's "claim only what is true at M0" call is what caught it; without that constraint the phase would have shipped a doc advertising an M1 command.
2. **`docs/reference/lvt-attributes.md` has systematically rotted, far beyond #230.** Sampling 11 documented attributes against the client, **8 are stale** — `lvt-scroll`, `lvt-highlight`, `lvt-animate` (now `lvt-fx:*`), `lvt-throttle` (`lvt-mod:*`), `lvt-disable-with` (`lvt-form:*`), plus `lvt-click-away`, `lvt-focus-trap`, `lvt-modal-open` absent entirely. The lifecycle section also documents the same dead `lvt-{action}-on:{event}` form that #226 flags in the *archive*, and its "Available events" table lists `loading` where the client has `pending`/`done`. The doc predates the client's Tier-2 namespace migration. **Deliberately not fixed here** — that is a reference audit, not a narrative reframe, and mid-phase scope expansion is how a docs phase becomes three. Tracked separately; see § Risks.
3. **#230 is not a pure rename, and the client has a near-miss attribute.** The entry described `lvt-preserve` as "preserve form values," but `lvt-ignore` is a general morphdom escape hatch (skip element + subtree, Phoenix `phx-update="ignore"` equivalent) of which form-value preservation is one *use case*. Separately, `lvt-form:preserve` genuinely exists and is a different thing. Renaming without reading the client would have produced a correctly-named entry with a wrong definition.

*PLAN.md drift fixed in this commit.*
- Design refs gained `docs/reference/lvt-attributes.md` (#230's actual target).
- "advertised-but-unbuilt commands" corrected from three to **five**, with the redirect trap named.
- The operator's M0-honesty constraint recorded at the top of the phase, since it governed three separate edits rather than just the "disk-free" line it was raised about.

*Feed-forward to **M1 Phase 1**'s Audit.*
- `docs/llm-system-prompt.md` is confirmed as the seed for M1 Phase 3's generation context — adopt, don't re-author.
- The exec narrative now says "privileged, gated behind `--allow-exec`, manifest approval coming in M1." M1 Phase 1 must actually deliver that, or the doc becomes a promise instead of a description.
- `README.md` deliberately does **not** claim a disk-free ephemeral path (M0 Phase 0 finding 4). When M1 Phase 3 lands `WithParseFS`, the README's "cheap to throw away" bullet is where that claim belongs.

*New / changed risks.*
- **New (medium): live reference documentation is materially out of date** — 8 of 11 sampled attributes. Users and generating agents both read `docs/reference/lvt-attributes.md`, and M1's whole premise is that an agent can hit the vocabulary correctly. A reference that names attributes the client no longer implements actively degrades that. **Scheduled as Phase 2 (M0) below** rather than left as a backlog issue, because M1 Phase 3 consumes this reference as generation context — it is a prerequisite, not adjacent debt.

#### Phase 2 (M0) — Attribute-reference audit: reconcile the docs with the client (~1 session)

> **Goal at end:** every `lvt-*` attribute documented in tinkerdown is one the current `@livetemplate/client` actually implements, under its current name — so the reference is safe to feed an LLM as generation context.

**Why this is a phase and not a doc chore:** M1's central claim is that a *constrained, well-documented* vocabulary makes generation reliable first-try, and M1 Phase 3 plans to hand this reference to the generating agent. A reference naming attributes the client dropped would teach the agent to emit invalid output — the exact failure the reframe exists to prevent. Every hour spent here is bought back in M1's first-pass validity rate.

**Design refs:**
- Phase 1 (M0) Learn, findings 2 and 3 (the sampled staleness; the `lvt-ignore` vs `lvt-form:preserve` near-miss)
- `docs/reference/lvt-attributes.md` (primary), `docs/llms.txt` "Key Attributes", `skills/tinkerdown/reference.md`, `docs/llm-system-prompt.md` — the four surfaces that teach the vocabulary
- Upstream `../client`: `livetemplate-client.ts`, `dom/reactive-attributes.ts` (the `lvt-el:` method dispatch), `dom/event-delegation.ts`, `state/form-lifecycle-manager.ts`; `CHANGELOG.md` "New Tier 2 namespaces" + "Backward-compat shims"
- Project CLAUDE.md: check the client before documenting any `lvt-*` attribute

**Audit:**
- [x] **Design-ref completeness check.**
- [x] Build the **full** inventory, not a sample — 35 distinct attribute tokens across the four surfaces, cross-referenced against the client. Phase 1's 8-of-11 sample proved *representative but incomplete*: the full pass found two attributes it had missed entirely, both of a different and worse kind (below).
- [x] Classify each stale entry: **renamed** (`lvt-scroll`/`lvt-highlight`/`lvt-animate` → `lvt-fx:*`, `lvt-throttle`/`lvt-debounce` → `lvt-mod:*`, `lvt-disable-with` → `lvt-form:disable-with`, `lvt-{action}-on:{event}` → `lvt-el:{method}:on:{state}`), **removed** (`lvt-click-away`, `lvt-window-{event}`, `lvt-focus-trap`, `lvt-modal-open`, `lvt-modal-close`), **wrong description** (`lvt-ignore`), or — the category the plan did not anticipate — **never implemented at all** (`lvt-filter`, `lvt-value-*`).
- [x] Check whether **backward-compat shims** accept the old names. **They do not.** The client carries exactly one warn-once shim (`utils/legacy-attr.ts`, for `lvt-no-intercept`); every other superseded name resolves to nothing. So the docs were **actively broken**, not stale-but-working — old markup fails silently, with no console warning.
- [x] Verify corrected values against source, not CHANGELOG: `lvt-el:` methods are `reset`/`addClass`/`removeClass`/`toggleClass`/`setAttr`/`toggleAttr`; states are `pending`/`success`/`error`/`done` (`dom/reactive-attributes.ts:44`). The documented `loading` state and the `disable`/`enable`/`focus`/`blur` methods never existed.
- [x] Check the "Attribute Ownership" split — it was broadly right about *which side* owns what, but listed the pre-migration names on the client side. Corrected, and `lvt-datatable` added (it was absent despite being the opt-in for sorting/pagination).

**Implementation:**
- [x] Correct `docs/reference/lvt-attributes.md`: renames applied, removed entries replaced with explicit "Removed." callouts (naming the replacement — e.g. native `<dialog>` for modals), lifecycle methods/states tables rewritten, `lvt-form:preserve` documented as distinct from `lvt-ignore`.
- [x] Reconcile the other surfaces — `llms.txt` and `llm-system-prompt.md` were already clean; `skills/tinkerdown/reference.md` had the `lvt-filter` claim plus two sibling claims that were wrong in a subtler way (sorting/pagination attributed to plain `lvt-columns` when they require opt-in `lvt-datatable`).
- [x] Add a § Namespace migration table so existing apps can find the rename, with the not-shimmed warning stated.
- [x] ~~Update `CHANGELOG.md`~~ — not needed: no shipped *behavior* changed or broke. The rot was confined to documentation; **no example, template, or skill fixture used a dead attribute**, so nothing user-facing regressed.
- [x] **(added)** `TestDocumentedAttributesExist` — the scripted guard (see Acceptance).

**Acceptance criteria:**
- [x] **Simplify:** prose pass — deleted entries rather than documenting attributes nobody should use; each removal names what to use instead.
- [x] **Unit:** `skill_examples_test.go` + `TestLLMSTxtExists` green (5/5); every skill example still validates.
- [x] **Integration:** `TestDocumentedAttributesExist` checks every documented `lvt-*` against the **vendored bundle** (`internal/assets/client/tinkerdown-client.browser.js`) plus production Go — deliberately not a sibling `../client` checkout, so it runs in CI and tests the client that actually ships. **Verified it fails when rot is reintroduced**, not merely that it passes today.
- [x] **E2E:** N/A — no example markdown changed (none used a dead attribute), so there is nothing whose runtime behavior could differ.

**Learn:**

*What surprised us.*
1. **The worst category was one nobody predicted: attributes that never existed.** The phase was scoped around *renames* — find what moved, update the name. But `lvt-filter` and `lvt-value-*` had no destination; they were taught, and did nothing, apparently always. Renames are self-announcing (something changed upstream); fabrications are invisible, because there is no upstream event to notice. `lvt-value-*` was missed even by Phase 1's 11-attribute sample and surfaced only once the check was mechanical.
2. **`tinkerdown validate` does not validate attribute names — proved empirically.** A document using `lvt-filter`, `lvt-scroll`, and a literal `lvt-totally-made-up` validated **clean, zero errors**. Unknown `lvt-*` attributes pass through as inert HTML. So nothing downstream would ever have caught the rot, and — more importantly — nothing will catch a *generated* app that uses a hallucinated attribute. See feed-forward.
3. **I deleted two working attributes, and my own guard could not have caught it.** Review found `lvt-focus-trap` documented as removed while a live Tab-cycling handler for it sits in the shipped bundle; re-verifying the whole batch then turned up `lvt-debounce` as a second false removal (it overrides the auto-wired change-binding debounce, distinct from `lvt-mod:debounce`). Both restored.

   Two distinct failures compounded. **The classification error:** I hand-grepped for quoted literals (`"lvt-focus-trap"`), but the client uses the attribute in a *selector* — `querySelectorAll("[lvt-focus-trap]")` — so the literal is `"[lvt-focus-trap]"` and my pattern missed it. Any attribute used only via selector syntax would have been misclassified the same way. **The structural failure, which is the real lesson:** `documentedAsRemoved` and `neverImplemented` are *skip lists* — `TestDocumentedAttributesExist` deliberately does not check them, so "this attribute is gone" was the single claim in the entire guard that nothing verified. A guard with an unfalsifiable escape hatch will eventually be wrong inside it. Fixed by `TestRemovedAttributesAreReallyGone`, which runs the removed-list through the *same* matcher — and `implemented()` handles selectors correctly, so the mechanical check catches precisely what manual searching missed. Verified by reintroducing the mistake and watching it fail.
4. **The guard's first version was self-certifying and passed for the wrong reason.** It scanned all `.go` files for evidence an attribute exists, including *itself* — and its own doc comment names `lvt-filter` while explaining the bug. So the test found "`lvt-filter` in Go source" and passed. Excluding `_test.go` fixed it and is right anyway: a fixture using a made-up attribute must not vouch for the docs that invented it. Caught only by deliberately reintroducing the rot to check the guard fails — a passing guard proves nothing until you have seen it fail.
5. **The docs were actively broken, not merely stale.** The Audit posed this as an open question; the answer (one shim, for one unrelated attribute) is the harsher branch.

*PLAN.md drift fixed in this commit.*
- The phase's classification scheme gained a fourth category, **never implemented** — the plan assumed renamed/removed/wrong-description exhausted the space.
- The CHANGELOG item is struck through with the reason (no behavior regressed; rot was documentation-only).
- Phase 1's "8 of 11 stale" is recorded as representative-but-incomplete rather than the final figure.

*Feed-forward to **M1 Phase 3**'s Audit.*
- **The reference is now *accurate*, and mechanically kept so — but it is not *complete*, and only one of those was fixed here.** `TestDocumentedAttributesExist` fails the build if a doc surface names an attribute nothing implements, so M1 Phase 3 can wire `reference.md` into the skill's context without inheriting the emit-invalid-output bug. What the guard structurally cannot see is the reverse direction: attributes that exist and are documented nowhere. Review found `lvt-scroll-away` — live, wired up, validating its `top`/`bottom` value and warning otherwise — absent from all four doc surfaces, and a longer tail behind it (`lvt-spy`, `lvt-upload`, `lvt-redact`, `lvt-fx:region-select`, `lvt-fx:auto-click`; six of seven sampled were undocumented). **The two gaps differ in severity and should not be conflated:** a documented-but-absent attribute makes an agent emit a silently broken page, while an undocumented-but-real one merely means the agent never reaches for a capability that exists. The first is a correctness bug, the second a ceiling on quality. Only the first is closed.
- **But the reverse direction is still wide open, and it is M1's problem.** The guard protects *docs → implementation*. It does nothing about *generated app → implementation*: an agent that hallucinates `lvt-sortable` gets a clean `validate` and a silently broken page. M1 Phase 3's validate loop **cannot detect vocabulary errors**, so "self-correct until validate is clean" is a weaker guarantee than the plan assumes. Either M1 accepts that gap explicitly, or it pulls forward part of M2's `Validate()` attribute diagnostics.
- Measured staleness prior: of 35 documented tokens, ~10 were wrong — **roughly a quarter of this repo's own attribute documentation was untrue** before this phase. Worth carrying as a prior for how far to trust any doc surface that lacks a mechanical check.

*New / changed risks.*
- **Sharpened (was M2 nice-to-have, now M1-relevant): no vocabulary validation anywhere in the stack.** § Risks updated — this is the empirical case for M2's `Validate()` API, and it lands *inside* M1's critical path rather than after it.
- **Retired:** the Phase-1 risk "live reference documentation is materially out of date" — corrected and now guarded by a test.

---

### M1 phases — THE DEMO (full detail)

> Reference app locked: the **PII / data-export access-approval console** (§ The reference demo). M1 delivers it end-to-end via `/tinkerdown`.

#### Phase 1 (M1) — Workspace manifest: approved sources + named actions + style guide (~1 session)

> **Goal at end:** a project can declare, once, a set of **approved** data sources + named `confirm:` actions + (optionally) a house style — the surface an LLM is allowed to wire up — and tinkerdown loads it. **House style is optional**: absent one, generation uses tinkerdown's sane defaults (PicoCSS semantic styling + the default theme).

**Design refs:**
- § Context (the four pillars) · § The reference demo (what the manifest must express for the PII console)
- `internal/config/config.go` — `SourceConfig` (`:143`), `AuthConfig`/`APIKeyConfig` permissions (`:570`), `Action` + `confirm:` (`:296`), `StylingConfig`/`site_css` (`:516`), `LoadFromDir` (`:786`)
- `docs/reference/config.md` (shared-sources semantics, frontmatter-overrides-config priority)

**Audit:**
- [ ] **Design-ref completeness check.**
- [x] Confirm the recommended shape: **extend `tinkerdown.yaml`** (reuse `SourceConfig`/`Action`/`AuthConfig`/`StylingConfig`) — confirmed, no new file.
- [x] **Decide the `approved:` semantics — resolved, and the plan's framing was wrong.** *(operator decision; see the finding below for why the question needed re-asking.)*

  **The finding.** `parser.go:98`'s `Frontmatter` carries its own `Sources` and `Actions` maps, so a generated `app.md` can declare sources the manifest never approved — **and can redefine an approved name to mean something else.** That defeats approval-by-declaration: Phase 2's spec ("every source/action *referenced* by the doc must be in the approved set") passes a doc that redefines `requests` and then references `requests`. Name-based approval loses to name-shadowing.

  **Correction — the mechanism is not where this originally said it was.** The first version of this entry cited `page.go:245` `MergeFromFrontmatter` overwriting site config by name. That is wrong: `parseFile` never loads `tinkerdown.yaml`, so the map that merge writes into holds no site values to overwrite. The real shadow point is **`internal/server/websocket.go` `getEffectiveSource`**, which resolved page-frontmatter sources *first* and fell back to site config. Right conclusion, wrong evidence — which is exactly what the "code-verified, not runtime-verified" caveat was hedging, and why the caveat was worth writing down. Sources and actions resolve by *different* mechanisms, so each needs its own pin; do not assume one fix covers both.

  *Evidence quality: now verified by a failing-then-passing test.* `TestGetEffectiveSourcePrecedence` covers all three tiers, and the shadow case was confirmed to fail with the pin removed. (Four earlier attempts at an end-to-end demo failed on my own probe-construction errors — nonexistent db, wrong source type, `path` vs `file`, an auto-table that would not populate from `json`. Those were neither counter-evidence nor confirmation; the unit test is what settled it.)

  **The decision — a precedence tier, not a prohibition.** The rejected alternative was "when a manifest is present, frontmatter may not declare `sources:`/`actions:` at all." Rejected because a conditional ban is confusing and forces per-field tracking of what is required versus forbidden. Instead, extend the precedence order the docs already define (`docs/reference/config.md` § Priority: Frontmatter vs Config File — "frontmatter wins / config provides defaults") with one new top tier:

  1. **Manifest-approved definitions — pinned; frontmatter cannot override them**
  2. Frontmatter
  3. Project config (defaults)

  Existing behavior is untouched when no manifest exists (2 beats 3, exactly as today); the manifest adds a single name-level rule with no field-level bookkeeping. **This splits the job cleanly across two mechanisms:** precedence is the *runtime* guarantee (an approved name cannot be hijacked), and Phase 2's lint is the *generation-time* gate (a doc introducing unapproved names is flagged). Neither is asked to do the other's work.
- [ ] Determine how a manifest source/action carries the metadata the operation summary needs: a human-readable description of what it touches (dataset, scope, whether it writes/execs/network) so the summary is meaningful.
- [x] Verify `Action` supports typed params + `confirm:` for the demo's actions — **yes**: `Kind` (`sql`/`http`/`exec`), `Params map[string]ParamDef` (`string`/`number`/`date`/`bool`, `required`, `default`), and `Confirm` all exist (`config.go`). Scoped export, approve, deny and the grant-PR action are each expressible.
- [x] **(added) Gap found — one `Action` is one statement.** `Action.Statement` is a single string handed straight to `substituteParams` then executed (`internal/runtime/actions.go:326`, `internal/server/webhook.go:559`); nothing splits on `;`. **This breaks Phase 4's Approve as specified** ("runs the scoped export **and** appends an audit record" — two operations). Splitting it into two actions makes the export and the audit record independently failable, so an approve can succeed while its audit append does not — destroying the durable-audit guarantee that is the demo's entire justification. **Feed-forward to Phase 4's Audit: pick deliberately** — a single `exec` action running a script that does both in one transaction, a multi-statement capability on `Action`, or an explicitly accepted (and documented) non-atomic pair. Do not discover this while wiring the demo.

**Implementation:**
- [ ] Extend the project config with an explicit **approved-for-generation** notion (marker or a dedicated `generation:` block listing approved source/action names) + per-item human-readable `describes:` metadata for the operation summary. Keep backward-compat (existing `tinkerdown.yaml` still loads).
- [x] **Implement the precedence tier for sources** — done in `internal/server/websocket.go` `getEffectiveSource` (*not* `MergeFromFrontmatter`; see the Audit correction). An approved name resolves to its manifest definition even when page frontmatter declares the same name, and the shadowing attempt is logged rather than silently dropped so a generating agent sees why its definition had no effect.
- [ ] **Implement the same pin for actions.** Actions resolve through a *different* path (`getPageActions` at `websocket.go:1209`, plus `server.go:186`), so the source fix does not cover them. Confirm the resolution order for actions before writing the pin — the source assumption was wrong once already.
- [ ] **No manifest → no behavior change.** The pin only engages when a generation/approved block is present; a plain `tinkerdown.yaml` keeps frontmatter-wins exactly as documented today.
- [ ] Update `docs/reference/config.md` § *Priority: Frontmatter vs Config File* to document the three tiers — it currently states a two-tier rule that this change makes incomplete.
- [x] ~~A `Manifest` accessor bundling approved sources, actions and style guide into one struct.~~ **Deferred to Phase 2 (operator decision).** The config already exposes all three directly, and the runtime consumes `ApprovedSource`/`ApprovedAction`/`IsManifest`. Building the struct now means guessing the shape Phase 2 and 3 want and then maintaining a parallel representation of data that already exists. **Phase 2's Audit decides:** define it against a real consumer's needs, or strike the item if the config surface turns out to be sufficient. The style-guide default (PicoCSS + project theme when `style_guide` is absent) moves with it.
- [x] ~~Seed the demo manifest under the reference-app fixture.~~ **Deferred to Phase 4 (operator decision)** — the phase that actually builds the PII console fixtures. Seeding it here means inventing sources Phase 4 would then rewrite. The precedence semantics are already covered by tests using in-memory configs, so the fixture would add no verification this phase lacks.

**Acceptance criteria:**
- [x] **Simplify:** the diff is a config struct, two resolution functions, and their tests; no redundancy to collapse.
- [x] **Unit:** approval accessors are nil-safe (no generation block → never approved), and an approved name the project never defined resolves to nothing rather than a synthesised entry.
- [x] **Unit (the pin):** `TestGetEffectiveSourcePrecedence` (6 cases) and `TestGetPageActionsPrecedence` (5 cases) cover both resolution paths — pinned against shadowing, unchanged without a manifest, unapproved names unaffected, no leak of unapproved site actions to pages. **Each was verified to fail with the implementation neutralised**, not merely to pass with it.
- [x] ~~**Integration:** load the demo manifest; assert the `Manifest` accessor…~~ — moot; both the fixture and the accessor are deferred (above).
- [ ] **E2E — not done, and I wrote this criterion myself during the Audit precisely because the runtime demo was missing.** Recording the gap rather than quietly ticking it. What *is* verified: the unit tests call `getEffectiveSource` and `getPageActions` directly — the real resolution functions the server uses, not reimplementations — so the precedence logic is proven at the exact point it runs. What is **not** verified: that a served page reaches those functions with a shadowing frontmatter and renders the approved data. The wiring is visible (`websocket.go:210` calls `getEffectiveSource` from `initializeSourceBlocks`), but visible is not tested. **Carry to Phase 4**, which stands up the demo fixture this test needs — and note that four Audit probes failed to build a working one, so budget for it rather than assuming it is quick.

**Learn:**

*What surprised us.*
1. **`generation.actions` would have been inert.** Sources have a site-config fallback; actions never did — a page could only invoke actions declared in its own frontmatter. So the approved-action list named a surface no generated page could reach, forcing it to declare those actions itself, which is exactly what approval exists to prevent. And *all the privilege lives in actions* (scoped export, approve, deny, audit append), so the manifest would have governed the harmless half of the system. The plan assumed this worked; it did not.
2. **Sources and actions resolve through different code paths**, so "add the pin" was two unrelated fixes, and one of them was a new capability rather than a guard. Any future rule of the form "approved things behave differently" must be applied twice and verified twice.
3. **I cited the wrong mechanism for the shadowing finding and had to correct it mid-phase.** I reported `MergeFromFrontmatter` overwriting site config; in fact `parseFile` never loads `tinkerdown.yaml`, so that merge has nothing site-level to overwrite. The real shadow point is `getEffectiveSource`'s lookup order. Right conclusion, wrong evidence — which the "code-verified, not runtime-verified" caveat had explicitly hedged, and which is the argument for writing such caveats rather than rounding to "verified".
4. **Four attempts at a runtime probe all failed on my own construction errors** (nonexistent db, wrong source type, `path` vs `file`, an auto-table that would not populate from `json`). A unit test at the real resolution function settled in minutes what the probes could not. Worth remembering when the instinct says "just serve it and look" — for a lookup-order question, the function is the cheaper and sharper target.

*PLAN.md drift fixed in this commit.*
- Approval semantics rewritten from "declare approved sources" to the precedence tier, with the rejected ban recorded in Appendix A.
- The shadowing finding's cited mechanism corrected (`MergeFromFrontmatter` → `getEffectiveSource`).
- `Action` verified to support typed params + `confirm:`; **new gap recorded** — `Action.Statement` is a single statement, which breaks Phase 4's Approve ("export *and* audit append") as specified.
- `Manifest` accessor and demo fixture deferred to their consumers (Phase 2, Phase 4) rather than built speculatively.
- Phase 2's lint spec sharpened: lint *declarations*, not only references, and do not re-implement the pin.

*Feed-forward to **Phase 2**'s Audit.*
- **Decide the `Manifest` accessor against a real consumer.** If `validate` and the skill can read `Generation` + `Sources` + `Actions` directly, strike the item; a parallel struct is only worth its sync cost if something needs a shape the config lacks.
- **The lint's job is narrower than it looks.** Precedence already guarantees runtime safety for approved names, so the lint handles (a) references to unapproved names and (b) *declarations* of unapproved names — the second being the one the plan originally missed. For a shadowing attempt its output is a diagnostic, not enforcement.
- The runtime logs a line when a page tries to shadow an approved name; consider whether `validate` should surface the same condition at generation time so the agent self-corrects before serving.

*New / changed risks.*
- **New (M4, low):** the action fallback is deliberately limited to *approved* site actions. If a later milestone wants general site-action reach from pages, that is a separate decision with a real privilege-expansion cost — schedule- and webhook-only actions becoming page-callable — and should not be slipped in as a convenience.
- **Sharpened (M1 Phase 4):** `Action.Statement` being single-statement means Approve cannot atomically export *and* append its audit record. Split into two actions, an approve can succeed while its audit append fails — destroying the durable-audit guarantee that is the demo's justification. Decide deliberately in Phase 4: one `exec` script in a transaction, multi-statement support, or an explicitly documented non-atomic pair.

#### Phase 2 (M1) — Generation-time policy lint + operation summary (~1 session)

> **Goal at end:** `tinkerdown validate` rejects a generated `app.md` that references any source/action the manifest hasn't approved (the enforcement); and it can emit an **operation summary** — the concrete privileged operations the app performs (sources touched, actions run, whether they exec/write/network/touch sensitive data) — which the skill surfaces **proportionally** (privileged apps only; read-only apps skip it).

**Design refs:**
- § The reference demo (policy lint + operation transparency) · § Deliverables (#3) · Phase 1 Learn (manifest shape)
- `cmd/tinkerdown/commands/validate.go` (`ParseFileInSite` gate `:81`), `errors.go` (`ParseError` with `File/Line/Hint` — the LLM-friendly feedback), `parser.go` `getLvtSourceFromContent` (`:531`)

**Audit:**
- [ ] **Design-ref completeness check.**
- [x] **Confirm `validate` can extract every source ref + action ref — partly, and the missing half is real work.**
  - **Source refs: available.** `ServerBlock.Metadata["lvt-source"]` already carries them; no new extraction needed.
  - **Action refs: no existing extraction to reuse.** Action names reach the server *from the client* at click time (`GenericState.HandleAction(action string, …)`); nothing ever enumerates the actions a document references. The plan's "reuse the parser's existing extraction, don't re-regex" cannot be followed because there is nothing to reuse — this must be built. `golang.org/x/net` is already a dependency (`go.mod:18`), so it can be a real HTML parse rather than a regex over markup.
  - **`lvt-persist` does not exist.** `page.go:585`: *"NOTE: lvt-persist has been removed. Use lvt-source with type: sqlite instead."* The Audit item named a dead attribute — the fourth time a plan block has asserted something that isn't there.
- [x] **(added) `validate` does not load `tinkerdown.yaml` at all** — it calls `ParseFileInSite` and discards the result (`_, err =`). It therefore has no access to the approved set, and no access to the parsed `Page` either. Both are prerequisites for the lint, and neither is mentioned in the plan. This is the same shape as Phase 1's finding that `parseFile` never loads site config: **the parse layer is deliberately config-free**, so anything policy-aware has to load config itself.
- [x] **(added) Declarations are directly available.** `Page.Config.Sources` / `Page.Config.Actions` hold what the doc declared in frontmatter, so linting *declarations* — the check the plan originally missed — needs no new extraction at all. It is the cheaper half of the lint, and the more important one.
- [ ] Decide the operation-summary output format (JSON on a `--summary` flag?) so the Phase-3 skill can consume it deterministically, and decide the **proportionality rule** (what counts as "privileged" → surfaced; read-only → skipped).
- [ ] Confirm policy failures surface as `ParseError`-quality diagnostics (line + hint: "source `X` is not approved in tinkerdown.yaml; approved: [...]") so the skill's validate loop can self-correct.

**Implementation:**
- [ ] `validate` gains a manifest-aware **policy lint**: every source/action referenced by the doc must be in the approved set; unapproved refs → a clear diagnostic (line, offending ref, approved list, hint). This is the *generation-time* gate.
- [ ] **Lint the doc's own `sources:`/`actions:` declarations, not only its references** (per Phase 1's Audit). A doc can declare its own sources in frontmatter, so a reference-only check passes a doc that declares an unapproved source and then references it. Flag any frontmatter-declared name outside the approved set.
- [ ] **Do not re-implement shadow protection here.** Phase 1's precedence tier already pins approved names at runtime, so the lint's job for a *shadowing* attempt is a clear diagnostic ("`requests` is approved and cannot be redefined; your definition will be ignored"), not enforcement. Runtime safety and generation-time feedback are separate mechanisms — keep them that way.
- [ ] `validate --summary` (or equivalent) emits the **operation summary**: the approved sources/actions the doc actually uses + their `describes:` metadata + risk flags (execs? writes? network? PII?), as structured output, with a `privileged` bit so the skill knows whether to surface it.
- [ ] Wire the lint so it runs only when a manifest with a generation/approved block is present (plain projects behave as today — no regression).

**Acceptance criteria:**
- [ ] **Simplify:** `/simplify` the diff.
- [ ] **Unit:** lint accepts an all-approved doc; rejects an unapproved-source doc + an unapproved-action doc with a line+hint; operation summary lists exactly the used approved items with correct risk flags + `privileged` bit (read-only doc → not privileged).
- [ ] **Integration:** run `validate` + `validate --summary` against the demo app.md fixture.
- [ ] **E2E:** N/A (CLI) — exercised in Phase 5.

**Learn:**

*What surprised us.*
1. **The phase was three pieces, and two were prerequisites the plan never mentions.** "Add a lint to `validate`" presumed `validate` had the approved set and the parsed page. It had neither — it called `ParseFileInSite` and discarded the result, and never loaded `tinkerdown.yaml` at all. Same shape as Phase 1's finding: **the parse layer is deliberately config-free**, so anything policy-aware must load config itself. Treat that as a standing property of this codebase rather than a fact rediscovered per phase.
2. **The plan's "reuse the parser's existing extraction, don't re-regex" could not be followed, because nothing extracts action references.** An action name reaches the server *from the client* when a control is used (`GenericState.HandleAction`), so the parse pipeline never enumerates them — and policy runs before anything is served, which is far too early for a click to have happened. `Page.Refs()` is new machinery, not reuse.
3. **The cheap half of the lint is the important half.** Declarations sit right there on `Page.Config`, needing no extraction at all — and declarations are what close the shadowing hole. The expensive half (action-reference extraction) guards the *less* dangerous case. Worth remembering when a security check looks costly: check which half actually carries the risk.
4. **An HTML parse, not a pattern match.** `name` is a legitimate attribute on `<input>` and `<select>`, where it is a form field. A regex over markup would report every form field as an action reference — noise that would train an operator to ignore the diagnostics. `golang.org/x/net` was already a dependency, so the correct approach cost nothing.

*PLAN.md drift fixed in this commit.*
- The Audit item naming `lvt-persist` — **removed from the codebase** (`page.go:585`), the fourth plan block asserting something absent.
- "Reuse the parser's existing extraction" corrected: there is nothing to reuse for actions.
- Recorded that `validate` was not config-aware, which the phase had to fix first.
- **`Manifest` accessor: struck, not deferred again.** Phase 2 produced its two real consumers — `CheckPolicy` and `Summarize` — and neither needed anything beyond `Generation` plus `ApprovedSource`/`ApprovedAction`. A bundling struct would be a parallel representation of data the config already exposes, kept in sync for no consumer. If Phase 3's skill turns out to need a different shape, build it then against that need.

*Feed-forward to **Phase 3**'s Audit.*
- `validate --summary` emits **only JSON on stdout** (warnings go to stderr) precisely so the skill can parse it. That property is load-bearing and easy to break — a stray `fmt.Printf` in the validate path would silently corrupt it. If Phase 3 adds output there, re-verify by piping through a parser, not by eye.
- **The `privileged` bit is the proportionality rule.** Surface the summary to the operator when it is true; skip straight through when false. A prompt on every generated page is a prompt nobody reads.
- Policy diagnostics carry a `Hint` naming the approved alternatives, which is what the skill's self-correction loop consumes. If a new violation kind is added without a hint, the loop has nothing to act on.
- **Still open from M0 Phase 2, and now urgent:** `validate` does not check attribute *names*. Phase 3's loop is specified as "self-correct until validate is clean", and clean still does not mean the attributes exist. Decide there: accept the gap explicitly, or pull forward M2's allowlist.

*New / changed risks.*
- **New (low):** the summary marks every action as a write, because an action exists to change something and parsing SQL to guess would be confidently wrong on the cases that matter. A genuinely read-only `sql` action would therefore over-report as privileged. Over-reporting is the safe direction, but if M4 adds read-only actions this deserves revisiting rather than tuning by guesswork.

#### Phase 3 (M1) — The `/tinkerdown` Claude Code skill (~1 session)

> **Goal at end:** the **existing `skills/tinkerdown/` skill is extended so generation is its default action** — running `/tinkerdown "<intent>"` reads the manifest + attribute reference + few-shot corpus, emits `app.md`, loops on `tinkerdown validate` until clean, surfaces the operation summary (if privileged), then serves ephemerally. (Sub-capabilities land as namespaced `/tinkerdown:<verb>` skills — e.g. `/tinkerdown:save` in Phase 6.)

**Design refs:**
- § Architecture (the generate flow) · Phase 1–2 Learn (manifest + operation summary)
- `skills/tinkerdown/SKILL.md` + `reference.md` + `examples/*` (existing skill format + few-shot corpus; note the mandated "Prompt to Generate This" sections), `docs/llm-system-prompt.md` (285-line seed prompt), `docs/llms.txt`
- `internal/server/playground.go` (`ParseString`, `PlaygroundSession`, TTL) — the ephemeral serve substrate

**Audit:**
- [x] **Design-ref completeness check.**
- [x] Decide the skill's generation context: adopted `docs/llm-system-prompt.md` (seeded in M0 Phase 1) + the existing `skills/tinkerdown/reference.md` + `examples/` corpus + the loaded manifest. Corpus few-shot pairs confirmed current post-M0 (M0 Phase 2 already reconciled them against the client).
- [x] Decide ephemeral serve mechanism — **chose `tinkerdown serve` on a scratch directory, *not* the playground's `ParseString` path.** The playground is not the disk-free win the plan assumed: it is an HTTP endpoint requiring a running server + a POST + a session id, and `websocket.go:495` still writes each block to `/tmp` regardless. A scratch directory the operator can re-run and inspect is more useful, and a UI worth keeping is already a file. (The genuine disk-free path is the `WithParseFS` refactor flagged in M0 Phase 0's Learn — deferred, not on Phase 3's critical path.)
- [x] Define the validate loop's stop condition + max iterations — **~5 rounds.** A request still failing after five likely needs a capability the vocabulary lacks; saying so beats substituting attributes until something passes.

**Implementation:**
- [x] `skills/tinkerdown/SKILL.md` — the full generation workflow: read the approved surface → write the document (leading with the ```lvt fence requirement) → `tinkerdown validate` and self-correct (~5 rounds) → check the operation summary if privileged → serve on a scratch dir.
- [x] Bundle the generation context assets — the existing `skills/tinkerdown/reference.md` is referenced, **not forked**; it stays the single source of truth.
- [x] ~~Redirect the phantom `/new-app` etc. references.~~ **Moot — already removed in M0 Phase 1** (which deleted the five-command table and documented the real describe→validate→serve loop). Nothing left to redirect by Phase 3.
- [x] **(added) `vocabulary.go` — validate now checks attribute *names*** (M2's allowlist pulled forward, per operator decision). A document using `lvt-filter`/`lvt-scroll`/a literal `lvt-totally-made-up` previously validated with zero errors — unknown `lvt-*` was emitted as inert HTML, so a hallucinated attribute survived every loop iteration. It now fails, with a migration hint only where a real one exists. `knownDataAttributes` is a closed 22-entry exact-match set (not a `data-lvt-` prefix, which let `data-lvt-sortable` through in review).
- [x] **(added) `InertAttributes()` gate — the third silent-failure class, found by dogfooding.** Following the workflow produced a page that validated clean, summarised privileged, served without error, and rendered nothing: `lvt-*` only binds inside a ```lvt fence; in the markdown body it is inert HTML, with no console error and no server warning. `validate` now flags `lvt-*` markup outside a fence.

**Acceptance criteria:**
- [x] **Simplify:** prose + asset pass.
- [x] **Unit/structural:** `vocabulary_test.go` (`TestKnownAttributesAreReal`, `TestUnknownAttributes`, `TestInertAttributes`) + the extended `attribute_docs_test.go`; the existing `skill_examples_test.go` continues to validate the skill bundle + assets. Both new guards verified to fail when their conditions are violated.
- [x] **Integration:** `TestUnknownAttributes` is the dry-run — a deliberately-broken doc (`lvt-scroll`, `lvt-sortable`) produces line+hint diagnostics an agent converges on; a clean doc passes.
- [x] **E2E:** covered in Phase 5 (full generate→serve).

**Learn:**

*What surprised us.*
1. **"Validate is clean" did not mean "the page works" — in three distinct ways, and only one was on the plan.** The plan scoped Phase 3 as a skill-authoring task; it became a validation-hardening task once dogfooding exposed that a document could pass `validate` and still do nothing. Three silent-failure classes, all closed at the gate: (a) **unknown/hallucinated attribute names** (`lvt-totally-made-up` validated clean) — the one the plan's Risk 755 predicted, fixed by pulling M2's allowlist forward; (b) **`data-lvt-` as a blanket-allowed prefix** let `data-lvt-sortable` through — fixed by enumerating the closed 22-name set; (c) **`lvt-*` markup outside a ```lvt fence is inert HTML** — no error anywhere, found only by *following my own SKILL.md instructions* and getting a blank page.
2. **The self-certifying-test failure mode recurred a third time, and knowing the mode was not enough — only running the falsification test caught it.** `TestKnownAttributesAreReal` scanned `vocabulary.go`, which *contains* the allowlist keys, so every entry matched itself and `"lvt-totally-made-up": true` passed. I had fixed this exact pattern in `collectGo` in the same commit and still failed to apply it to the new scanner. Fixed by excluding `vocabulary.go`; the lesson is procedural, not conceptual — verify a guard can fail before trusting it.

*PLAN.md drift fixed in this commit.*
- **This whole block is the drift.** #303 merged the code without updating the tracker or writing this Learn; reconstructed here as its own commit (from the #303 diff + commit message), boxes ticked only against what the diff actually did.
- The serve-mechanism decision diverged from the plan's two options (serve-temp-file vs. playground) — recorded with the reason (playground isn't disk-free) rather than silently picking one.
- The "redirect phantom commands" item was already satisfied by M0 Phase 1; struck rather than re-done.

*Feed-forward to **Phase 4**'s Audit.*
- The golden `app.md` Phase 4 authors **must pass the now-stricter `validate`** — real attribute names only. Appendix B's illustrative golden uses `lvt-filter` and `lvt-value` (both non-existent per M0 Phase 2 / Phase 3's allowlist) and the wrong action schema (`type:`/`sql:` instead of `kind:`/`statement:`); do not copy it verbatim — author against the real vocabulary and let `validate` gate it. Model on `examples/team-tasks/app.md`.
- The operation summary is what the operator reviews. Phase 4 must ensure the summary names the sensitive operations — see Phase 4's Audit finding on `orders_pii` being invisible to a name-only summary.

*New / changed risks.*
- **Retire Risk 755's "M1 must pull forward the allowlist or accept the gap"** — pulled forward and closed for exact-match attribute names. **Residual (still open, M2):** namespace *members* are unvalidated (`lvt-el:bogus:on:success` passes) — a deliberate limit, since `lvt-on:` takes arbitrary DOM events and hard-coding a member set would create a second list to rot.

#### Phase 4 (M1) — The PII / data-export access-approval reference app (~1 session)

> **Goal at end:** a locally-runnable fixture (synthetic-PII SQLite + requests store + audit log + optional fixture git repo) against which the generated console renders a pending-requests queue and Approve/Deny work end-to-end.

**Design refs:**
- § The reference demo (the full console spec: queue fields, Approve/Deny outputs, intake, local-runnable E2E) · **§ Appendix B — the target `tinkerdown.yaml` + golden `app.md` this phase authors**
- `internal/source/{sqlite.go,markdown.go}` (queue + audit sources), `config.Action` `confirm:` + typed params (`config.go:296`), `execargs` typed-form primitive (`execargs_e2e_test.go`), `WritableSource`/`SQLExecutor` (`internal/source/source.go`)
- `auto_tables.go` (queue table + Add form inference), `styling.site_css` (house style)

**Audit:**
- [x] **Design-ref completeness check.**
- [x] Build the fixtures — one `access.db` (built at test time from `seed.sql`, gitignored, per the repo's 0-byte-`.db` convention) with `access_requests` (queue), `audit_log` (append-only), `datasets` (catalog, read-only), `orders_pii` (synthetic PII — **8 rows, more than any request's cap so a bounded LIMIT genuinely bites**), `exports` (bounded artifacts), and `decoy_requests` (for the shadowing test).
- [x] Confirm the scoped-export is genuinely *bounded* — **and server-authoritative**: the button sends only the request `id`; the row cap and dataset are read from the request row via SQL subqueries (`LIMIT (SELECT row_cap FROM access_requests WHERE id = :id)`), so a tampered client cannot widen the export. `LIMIT (subquery)` verified empirically against modernc in `internal/source/exectx_test.go`.
- [x] Confirm the request-intake path — the app's own `<form name="Add" lvt-source="access_requests">` (a built-in writable-source insert), the simplest demoable option.
- [x] **(added) Gap resolved — one `Action` was one statement.** Approve is three operations (bounded export, audit append, status change) that must be atomic; a partial success (access granted, audit missing) destroys the durable-audit guarantee. **Chose multi-statement atomicity over the alternatives:** added `Action.Statements []string` + a transactional `ExecTx` on `SQLExecutor` (implemented once in `SQLiteSource`; only sqlite implements the interface). Rejected a DB trigger — a trigger would hide the audit+export side-effects from the *operation-summary review surface*, and transparency is the demo's point.
- [x] **(added) Finding — the operation summary omits `orders_pii`.** `Summarize`/`Refs` list only sources the *document* references; `orders_pii` appears only inside the approve action's SQL subquery, and nothing parses SQL — so the most sensitive operation in a PII-transparency demo would be invisible. Fixed by making the action's `describes:` enumerate it verbatim (`describes:` renders in the summary); structural subquery-read detection is deferred to M2/M3.
- [x] **(added) Finding — config `confirm:` is inert.** It is copied into the runtime action but never read and never shipped to the client; the client's only confirm path is the `data-confirm` HTML attribute. So the manifest's `confirm:` produces no dialog today. **M1 decision:** the click-time affordance is `data-confirm` authored on the golden app.md's buttons; the manifest `confirm:` stays as the declared intent that **M3's `WithActionPolicy` will enforce regardless of render path**. Risk updated (below).

**Implementation:**
- [x] Fixtures + the demo `tinkerdown.yaml` manifest under `examples/pii-access-approval/` (approved: `access_requests`, `audit_log`, `datasets`; actions `approve-export`, `deny-request`). ~~optional grant-PR action~~ **deferred** (explicitly optional; adds `gh` + a fixture git repo without exercising anything new).
- [x] The golden `app.md` — modeled on `examples/team-tasks/app.md` (hand-written `{{range .Data}}` rows for the rich decision context Appendix B's bare `lvt-columns` could not format), **not** Appendix B's illustrative markup, which uses non-existent attributes (`lvt-filter`, `lvt-value`) and the wrong action schema. Per-row `<button name="approve-export" data-id data-confirm>` / `deny-request`; a Request-access form; a refreshable audit view; a datasets catalog.
- [x] Approve → `ExecTx` of three statements (bounded `INSERT … SELECT … LIMIT (subquery)` export, audit append, status change), atomic.
- [x] Deny → audit append + status→denied, atomic. Free-text deny reason deferred (a per-row text input needs a per-row form); the audit records approver + decision + the request's own scope/reason.
- [x] **(added) Core fix — the policy lint flagged built-in source affordances as unapproved actions.** `<form name="Add">` failed `validate` ("action Add not in the approved set"). Add/Delete/Toggle/Refresh/… are intrinsic writable-source affordances governed by *source* approval + writability, not by the approved-*action* set. Added `runtime.IsBuiltinAction` (single source of truth beside the dispatch switch) and made `collectRefs` skip built-ins. Generic fix — any approved writable source's Add/Delete controls now lint clean.

**Acceptance criteria:**
- [x] **Simplify:** `/simplify` against the diff.
- [x] **Unit:** `exectx_test.go` (commit applies all statements; **rollback undoes a partial batch** — the durable-audit guarantee; readonly rejected; bounded LIMIT bites: 5 PII rows, cap 3 → exactly 3 exported); `validate_actions_test.go` (exactly-one-of statement/statements); `refs_test.go` (built-in action names not collected).
- [x] **Integration:** the golden `app.md` passes the stricter `validate` clean and `validate --summary` reports `privileged: true` with `orders_pii` surfaced via `describes:`.
- [x] **E2E (chromedp, four-channel):** `TestPIIAccessApproval` — queue renders 3 pending; Approve through the `data-confirm` dialog runs the export + audit + status atomically; asserts **exported rows == the request's cap** (never the whole table), audit row appended, status flipped, and the Approve button gone (server-authoritative); Deny records a decision and exports nothing; the audit trail **renders** after Refresh. `TestPIIAccessApprovalShadowing` — the runtime demo Phase 1 (M1) carried here: a frontmatter shadow of the approved `access_requests` resolves to the **approved** rows, not the decoy (two distinguishable sources, so a pass is not hollow). Four channels captured (console, server log, WS frames, HTML + screenshot).

**Learn:**

*What surprised us.*
1. **The plan's central UX primitive — `confirm:`-gated actions — does nothing.** The manifest declares `confirm:` on every approve/deny action, the reference demo says each "pops the manifest's confirm: dialog," and Risk 758 lists `confirm:` as an M1 safety layer — but `confirm:` is inert: copied into the runtime action, never read, never sent to the client. The only working confirm is the `data-confirm` HTML attribute. The tempting "fix" — make the `lvt-actions` table generator emit `data-confirm` from the manifest — was **rejected** because it would enforce confirm *only for that one render path*, which is false assurance; render-path-independent manifest enforcement is exactly M3's `WithActionPolicy`, by name. M1's honest story: `data-confirm` is the affordance now; M3 enforces the manifest gate.
2. **The shadowing E2E "failed" first for a harness reason, not a pin bug — and the harness trap is worth internalizing.** Bare `server.New(dir)` uses `DefaultConfig()` and does **not** load `tinkerdown.yaml`; the manifest (and its `generation:` block) only loads via `LoadFromDir` + `NewWithConfig`, which is what `tinkerdown serve` does. So the first run served with no approved set, the frontmatter shadow won correctly, and the decoy rendered. The pin was never broken. Any manifest-dependent test must serve the way `serve` does — bare `server.New` silently drops the manifest.
3. **Appendix B's golden `app.md` was doubly fictional** (predicted by Phase 3's feed-forward, confirmed here): fabricated attributes (`lvt-filter`, `lvt-value`) *and* the wrong action schema (`type:`/`sql:` instead of `kind:`/`statement(s):`). The real vocabulary lives in `examples/team-tasks/app.md`. The stricter `validate` (Phase 3) is what makes copying Appendix B verbatim fail loudly — the reframe eating its own dogfood.
4. **The review bot found the class of bug the happy-path tests structurally could not: what happens to an id that is *not* a live pending row.** Three real findings (PR #304): (a) the bounded export's `LIMIT (SELECT row_cap …)` returns `LIMIT NULL` for a non-matching/already-decided id, which is undefined and unbounded on some SQLite builds — empirically 0 rows on the pinned modernc, but relying on it is fragile, so guarded with `COALESCE(…, 0)`; (b) **no idempotency** — a replay/double-click/direct call re-ran the export and appended a second audit row, since nothing restricted the statements to `status = 'pending'`; (c) the webhook exec path never injected `:operator`, a *direct symptom* of the two exec paths being hand-duplicated. The first two are pure-SQL fixes (the guards); the third was fixed at the root by extracting one `source.RunSQLAction` (+ `SubstituteParams`) both paths call, so operator injection and the atomic branch cannot diverge again. The lesson: my `exectx_test` only ever passed a *matching* id — a happy-path fixture cannot surface a not-found-row bug. `TestPIIActionsAreBoundedAndIdempotent` now exercises the real manifest SQL against a non-matching id and a replay.

*PLAN.md drift fixed in this commit.*
- **§ Appendix B corrected** — the manifest now uses `kind:`/`statements:`/`describes:` and the golden `app.md` uses real attributes (hand-written rows), matching what was actually built.
- **§ Risks — the `confirm:` line (758) corrected**: `confirm:` is not an enforced M1 safety layer; `data-confirm` is the M1 click-time affordance and `WithActionPolicy` (M3) is the enforcement. The M1 safety rests on the policy lint + operation-summary review + `--allow-exec` + a human approver.
- The optional grant-PR was deferred (recorded, not silently dropped).
- Phase 4 built the **runtime shadowing demo Phase 1 (M1) flagged as not-met** (its Acceptance E2E box and the line-480 carry-forward) — now closed by `TestPIIAccessApprovalShadowing`.
- **Review-round fixes (PR #304 follow-up commits):**
  - Round 1: idempotency + bounded-LIMIT guards on the approve/deny SQL; `source.RunSQLAction`/`SubstituteParams` extracted so the runtime and webhook exec paths share one implementation (fixes the webhook `:operator` gap and prevents future drift); `TestPIIActionsAreBoundedAndIdempotent` added (a non-browser CI test over the real manifest SQL).
  - Round 2 (found *because* the extraction made `RunSQLAction` load-bearing for a security property): **`operator` is now reserved server-set** — `RunSQLAction` unconditionally overwrites any client-supplied `operator`, so a client cannot attribute an approval to another identity (the audit trail's whole point); and **`IsBuiltinAction` is exact-match**, dropping the dispatch prefixes (`sort`/`nextpage`/`prevpage`) that would otherwise let a custom action like `sort-by-priority` bypass the approved-action lint by naming coincidence. Both regression-tested (audit-approver spoof; `sort-*` custom-action collection).
  - Round 3: (a) **dispatch precedence** — round 2 fixed only the *lint* half of the prefix collision; `HandleAction` still routed a `sort-*` name to the datatable handler before custom actions, so an approved `sort-by-priority` still could not run. Reordered so custom actions win over the datatable-prefix fallback (test: `TestHandleAction_CustomActionWinsOverDatatablePrefix`). (b) **Intake forgery (operator decision: fix in M1, not defer to M3)** — the intake used the built-in `Add`, which inserts any client column, so a direct call could file a `status='approved'` row with a spoofed approver and *no* audit trail, forging the appearance of a governed decision. Fixed with M1 primitives: the three display sources (`access_requests`/`audit_log`/`datasets`) are now **read-only**; a separate **writable `access_store`** (deliberately *not* in `generation.sources`, so no generated app can bind it and re-expose `Add`) backs the actions; and intake is a governed **`request-access`** action that hard-codes `status='pending'` and no approver. Net: no source the client can reach accepts a forged decision, and the operation summary is cleaner (three read-only sources + three governed write actions). Regression-tested (a spoofed `status`/`approver` in the intake payload is ignored).

*Feed-forward to **Phase 5**'s Audit.*
- Phase 5's generate→serve acceptance runs against **these** fixtures/manifest; the golden `app.md` is the target the `/tinkerdown` skill should be able to produce. The skill must author `data-confirm` on privileged buttons itself (the manifest `confirm:` won't produce a dialog) — worth a line in `skills/tinkerdown/SKILL.md` when Phase 5 iterates the generation context.
- The framework-leg latency Phase 5 measures should exclude the one-time Docker-Chrome spin-up the e2e harness pays (~1s); the served-page first paint + WS upgrade is the number that matters.

*New / changed risks.*
- **New (low, UX): cross-block live refresh.** The audit view is a separate source/block, so it does not live-update when an approve/deny in the queue block writes to `audit_log`; the durable trail is in the DB (E2E-verified) and the app exposes a manual **Refresh**. Live cross-source sync is a livetemplate concern, not an M1 blocker.
- **Changed: `confirm:` reclassified** from an M1 safety layer to an M3-enforced declaration (Risk 758).

#### Phase 5 (M1) — End-to-end acceptance: generate → review → live in ~30s (~1 session)

> **Goal at end:** `/tinkerdown "a console to approve PII / data-export access requests"` produces a validated console, surfaces the operation summary, and serves a live UI within ~30s — the whole-plan acceptance test — with generation-reliability + latency recorded.

**Design refs:**
- § Verification (M1 acceptance) · all M1 phase Learn outputs · § Risks ("30s" is a generation-reliability target)

**Audit:**
- [ ] **Design-ref completeness check.**
- [ ] Confirm every prior M1 phase is closed (manifest, lint+summary, skill, reference fixtures). Surface any incomplete Learn.
- [ ] Define the measurement: LLM-generation wall-time vs. framework leg (validate + parse + first SSR + WS upgrade); framework leg target = low tens of ms.

**Implementation:**
- [ ] Run the full skill flow against the demo manifest; iterate the generation context (reference/corpus/manifest describes-metadata) until first-pass validity is reliably high — the generation-context assets are the lever (per § Risks).
- [ ] Record: first-pass validate-clean rate across N runs; median generate→serving latency; framework-leg latency.
- [ ] **Visual spec + conformance.** Author a one-screen visual spec for the console (a `mockups/pii-console.html` + screenshot — the *visual* target, per skeleton convention 11, scoped to this single generated screen; **created at execution time, not in plan mode**). Acceptance includes a visual-regression check of the served console against it, and a house-style-conformance check (the generated app used the manifest theme + `site_css` + preferred components — not off-brand free-form HTML).
- [ ] Capture a demo transcript/screencast of the acceptance test for the README reframe.

**Acceptance criteria:**
- [ ] **Simplify:** N/A (integration) — but fold any skill-asset cleanups.
- [ ] **Unit/Integration:** the reliability + latency harness runs and reports.
- [ ] **E2E (chromedp, four-channel):** the whole-plan test — generate → review → live console; Approve runs export+audit (+ optional PR); verify server-authoritative state; framework leg within target; **visual-regression + house-style conformance** vs. the one-screen spec; screenshot.

**Learn:** what surprised us / plan drift / feed-forward to M2's Audit (does the generation loop need the upstream `Validate()` API's richer diagnostics — quantify where the real parser's feedback was insufficient) / new-or-changed risks.

#### Phase 6 (M1) — Capture the PII workflow as a reusable skill + the `/tinkerdown:save` capability (~1 session)

> **Goal at end:** (a) the PII / data-export approval workflow re-runs from a durable **`skills/pii-access-approval/`** skill in seconds (not by re-generating); and (b) the namespaced **`/tinkerdown:save`** sub-skill exists — **invoked on the user's request to persist** an ephemeral UI (opt-in; not automatic) — distilling that session into a well-formed skill, operationalizing convention 13.

**Design refs:**
- § Skills delivered · LLM session guide convention 13 · Phase 3–5 Learn (the generate skill + golden `app.md` + fixtures)
- `skills/tinkerdown/{SKILL.md,reference.md,examples/,scripts/validate.sh}` (the Anthropic skill shape to mirror), `skill_examples_test.go` (the structural contract: frontmatter, `lvt` block, "How It Works", "Prompt to Generate This")
- Anthropic Claude skill-authoring best practices: progressive disclosure (concise `SKILL.md` + deep `reference.md`), explicit `name`/`description`/`triggers`, bundled examples + scripts, clear when-to-use, a validation step

**Audit:**
- [ ] **Design-ref completeness check.**
- [ ] Decide the boundary between the *generic* `/tinkerdown` and the *specific* `skills/pii-access-approval/`: the latter bundles the manifest + golden `app.md` + fixtures + serve steps so re-running is near-instant and needs no LLM generation at all. Confirm it degrades cleanly if a team's data differs (parameterize the dataset/queue names).
- [ ] `/tinkerdown:save` is a **namespaced sub-skill** (naming decided — see § Skills delivered + Appendix A). Define what it extracts from a conversation (the final `app.md`, the manifest deltas, the approve/deny wiring, the validate loop) and how it maps to `SKILL.md` + `reference.md` + `examples/`.
- [ ] Confirm the generated skills pass the existing `skill_examples_test.go`-style structural contract (or an adapted one).

**Implementation:**
- [ ] `skills/pii-access-approval/` — `SKILL.md` (triggers: PII access, data-export approval, access request queue) + `reference.md` + `examples/` (the golden console) + the manifest + fixtures + a one-command serve. Re-running it stands up the working console in seconds.
- [ ] `/tinkerdown:save` (namespaced sub-skill): given a completed generation session the user chose to persist, emit a well-formed skill (progressive disclosure, frontmatter, bundled example + validate step) following the Anthropic shape. Dogfood it by using it to (re)produce `skills/pii-access-approval/` from the Phase-4/5 session.
- [ ] A short authoring guide (or extend `docs/guides/ai-generation.md`) documenting "ephemeral by default; persist on request by capturing the session as a skill via `/tinkerdown:save`" — the convention-13 doc.

**Acceptance criteria:**
- [ ] **Simplify:** `/simplify` + skill-asset pass.
- [ ] **Unit/structural:** the two new skills pass the structural contract (frontmatter, required sections, referenced assets exist); a captured skill is well-formed.
- [ ] **Integration:** run `skills/pii-access-approval/` end-to-end → working console in seconds; run skill-capture against the Phase-5 session → a valid skill that itself reproduces the console.
- [ ] **E2E (chromedp, four-channel):** the re-run-from-skill path serves the console live; Approve/Deny work; screenshot.

**Learn:** what surprised us / plan drift / feed-forward to M5's Audit (the "save if the need recurs" gallery in M5 should store *skills*, not just `app.md` — connect the capture capability to the gallery) / new-or-changed risks.

---

### M2–M5 phases — outline only (expanded at milestone kickoff per convention 9)

The *what* + *why* of each is in § Roadmap; here is only each milestone's **kickoff design checklist** — the sections to write into full phase blocks when that milestone starts (per convention 9):

- **M2** (upstream `livetemplate` + tinkerdown): the `Validate(templateText)` API surface; the attribute-allowlist source-of-truth; how the skill's loop consumes richer diagnostics.
- **M3** (upstream `livetemplate`): the `WithActionPolicy` hook signature; the introspection surface; how the manifest maps to runtime policy.
- **M4** (upstream `lvt/components` + tinkerdown): the component list + options; how the skill's reference advertises them; the design-token/enforced-component style-guide schema.
- **M5** (tinkerdown + `client`): the persistence model (stores captured *skills*, not just `app.md`); gallery UX; the external-embed handshake protocol; **substrate extensibility** — writable WASM sources (#216) + custom Go+WASM source types (#222), and how a team-authored source enters the *approved* set.

#### Standing Audit item for milestones that bump `@livetemplate/client` (M2, M3, M5) — artifact provenance

**M0 Phase 0 closed the #295 mechanism, not the class** (see its Learn + § Risks). `make build` now runs `npm ci`, so the committed bundle can no longer be built from stale `node_modules`. What no install-time check can catch is a bundle that is correct-by-construction yet **behaviorally** regressive — a genuine upstream client regression, or a server↔client wire mismatch (livetemplate exports `ClientVersion` precisely because there is no runtime handshake). Only running the live UI catches that, and **CI does not run the live UI**: it excludes the e2e suite twice over (`-tags=ci` *and* `-skip='E2E|e2e'`).

**The trigger is a bump to `@livetemplate/client` in `client/package.json`, not "an upstream bump" generally** — the two are easy to conflate and only one regenerates the bundle:

| Milestone | Upstream it bumps | Regenerates the client bundle? |
|---|---|---|
| **M1** (the demo) | none — builds on existing primitives | **No.** This is the breathing room that makes deferring [#297](https://github.com/livetemplate/tinkerdown/issues/297) reasonable rather than negligent. |
| **M2** `Validate()` API | Go `livetemplate` | **Check `ClientVersion`** — it is a wire contract with no runtime handshake, so a server bump that moves it obliges matching the client (what M0 Phase 0 did: server v0.19.1 → client 0.18.2). A release touching only server-side APIs may leave `ClientVersion` unchanged — `Validate()` is arguably one — in which case this is legitimately a **No**. Read the constant, don't assume. |
| **M3** `WithActionPolicy` | Go `livetemplate` | **Check `ClientVersion`** — same wire-contract reasoning. |
| **M4** component vocabulary | Go `lvt/components` | **No.** `github.com/livetemplate/lvt/components` is a server-side Go module consumed by `internal/server/websocket.go` and `internal/runtime/state.go`; it appears nowhere in `client/src` or `client/package.json`. Skip this checklist unless M4 also bumps `@livetemplate/client` for some client-side affordance a new component needs. |
| **M5** embed handshake | `client` | **Certainly** — it ships a client-side feature. |

When the table says yes, the executing session must:
- [ ] Re-read § Risks → *Committed-artifact provenance* and M0 Phase 0's Learn before touching `client/package.json` or `go.mod`.
- [ ] Verify the regenerated bundle **directionally**, not just that it differs from HEAD — a downgrade differs too. Pick a marker tied to a known changelog entry in the target version and confirm it appeared. (M0 Phase 0 used `ctrlKey` 1→5 for a v0.18.2 keydown fix; note this is a *proxy* for one fix, not whole-diff verification — diff against a clean `npm ci` build when in doubt.)
- [ ] Match the client to livetemplate's declared `ClientVersion`, **not** to whatever npm calls `latest`.
- [ ] Run the full local suite under `GOWORK=off` with `./tinkerdown` pre-built, and treat a checkbox-toggle failure as a bundle problem until proven otherwise — it mimics the flakiness #292/#293 fixed, and the tell is 100% reproducibility.
- [ ] **Decide explicitly whether [#297](https://github.com/livetemplate/tinkerdown/issues/297) (browser e2e smoke subset in CI) lands before or alongside this bump.** M2 is the first milestone to face this. Deferred out of M0 deliberately: the natural smoke subset *is* the checkbox suite that #292/#293 had to stabilize, so rushing headless Chrome into CI risks trading a quiet problem for a flaky one — that call is better made with M1's test-stability learnings in hand than at M0.

The full regeneration procedure lives in `internal/assets/client/README.md`, beside the artifact.

---

## Tech stack & version pins

The plan hinges on pins (M0 is a version bump); this is the canonical table. **Targets are "latest tagged at execution time"** — the version numbers below are the plan-authoring floor; upstream keeps releasing, so M0 Phase 0's Audit re-checks and bumps to the current latest (not these).

| Piece | Was | **Adopted** (M0 Phase 0, 2026-07-19) | Where |
|---|---|---|---|
| `github.com/livetemplate/livetemplate` | v0.10.0 | **v0.19.1** ✅ | `go.mod:11` — M0 Phase 0 |
| `@livetemplate/client` | 0.14.3 (lockfile); `node_modules` had drifted to 0.11.9 | **0.18.2** ✅ — the version server v0.19.0 declares wire-compatible via `ClientVersion`, not merely "latest" | `client/package.json` → bundled into `internal/assets/client/` — M0 Phase 0 |
| `github.com/livetemplate/lvt/components` | pseudo `2026-02-28` | **v0.2.0** ✅ (a real tag now exists) | `go.mod:12` — M0 Phase 0 |
| Go | 1.26.x | unchanged | `go.mod` |
| Data sources | sqlite (`modernc.org/sqlite`), pg, rest, graphql, file, csv/json, markdown, exec, wasm, computed | unchanged | `internal/source` |
| E2E | chromedp (headless Chrome, four-channel capture) | unchanged | `*_e2e_test.go` |
| Demo externals | `gh` CLI (grant PR), fixture git repo, synthetic-PII SQLite | — | M1 Phase 4 fixtures |
| Generator | **Claude Code** (the skill; no LLM in the Go binary) | — | `skills/tinkerdown` |

---

## Plan-skeleton deltas (conscious deviations from the CLAUDE.md "Writing plan files" skeleton)

This plan follows the skeleton's load-bearing parts (LLM session guide, per-phase Audit/Implementation/Acceptance/Learn, progress tracker, delivery protocol, risks, decisions log) and **consciously deviates** where the skeleton's shape (designed for a 10-milestone SaaS) would be cargo-culting for a thin generation-tool reframe:

- **No multi-screen "Product flow & UI requirements" section.** The deliverable is *one generated console*, not a product with many screens. Its visual spec is a single hand-authored golden `app.md` (Phase 4) + a one-screen mockup + visual-regression check (Phase 5) — the applicable slice of **skeleton** convention 11 (end-state UI mockups).
- **`## Mn design` sections folded into the phases.** For milestones this small, a separate design section per milestone duplicates the phase blocks. M2–M5 keep the skeleton's outline-only convention (expanded at kickoff, convention 9); M1's design lives in § The reference demo + the phase blocks.
- **No Deployment section.** Tinkerdown is a CLI/library, not a deployed service; there is no `fly.toml` analog. Release mechanics (tagged upstream releases per **session-guide** convention 11) are the only "deploy" and live in the session guide.

---

## Verification

**Per-phase:** `GOWORK=off go test ./...` (this includes the e2e suite — the `!ci`-tagged tests run by default locally; see § Implementation phases for the gating detail); `tinkerdown validate` on any changed example/doc snippet; the four-channel e2e capture. E2E needs `go build -o tinkerdown ./cmd/tinkerdown` first — several tests shell out to the binary and fail confusingly without it.

**M1 acceptance (the whole-plan test):**
1. In a repo with the demo workspace manifest, run the Claude Code skill `/tinkerdown "a console to approve PII / data-export access requests"`.
2. Confirm the **operation summary** lists exactly the operations the UI will run (reads the requests store, runs the *scoped* export query against the fixture PII DB, appends audit records, and — if in scope — `gh pr create`), with risk flags.
3. Since the console is privileged, OK the operation summary → the UI is serving live within ~30s (measure: LLM gen time vs. framework leg; framework leg must be low tens of ms).
4. In the browser: pending requests render with full decision context (requester, dataset, scope/query-preview, justification, TTL); **Approve** runs the scoped export + appends a durable audit record (verify the audit row; and a real PR via `gh pr list` if the grant-PR path is on); **Deny** records a reason; state is server-authoritative (no drift).
5. Record generation first-pass validity rate across N runs (the real "30s" target is generation reliability, since the framework leg is near-instant).
6. **Re-run from the captured skill:** invoke `skills/pii-access-approval/` and confirm the same working console stands up in **seconds** with no LLM generation — the "save it, the need recurs" path (convention 13 / M1 Phase 6).

---

## Risks & open questions

- **[M1] Reference app locked = PII / data-export access-approval console** (§ The reference demo). Rejected alternatives (K8s access-grant-via-PR, feature-flag approval) + why in Appendix A.
- **[M1] Scoped-export must be genuinely bounded.** The demo's whole point is *scoped* access (row cap + filter), not blanket — a scoped-export action that quietly returns everything defeats the friction-removal story. Phase 4 Audit gates this.
- ~~**[M0] Multi-minor upstream bump may ripple.**~~ **Retired — Phase 0 closed 2026-07-19.** Nine minors (v0.10.0 → v0.19.1) landed green with **zero source changes**: the feared breakers (kebab action routing, comment stripping, verbatim dynamic content) all predate v0.16 and were already absorbed, and tinkerdown's 10-symbol livetemplate API surface avoided the one removed API. The risk was mis-aimed — it watched the Go boundary, and the actual hazard was **JS bundle provenance** (below).
- ~~**[M1] `docs/reference/lvt-attributes.md` is materially out of date.**~~ **Retired — Phase 2 (M0) closed.** ~10 of 35 documented attribute tokens were wrong; all corrected, and the invariant is now held by `TestDocumentedAttributesExist`, which fails the build if any doc surface names an attribute neither the vendored client bundle nor production Go implements. Documentation rot in this area is now a test failure rather than a discovery.
- **[M1 — quality ceiling, not a correctness bug] The attribute reference is accurate but incomplete: nothing enumerates the client's real surface and diffs it against the docs.** Found in Phase 2's review. `lvt-scroll-away` is live in the shipped bundle — it reads the attribute, validates `top`/`bottom`, and warns on anything else — yet appears in none of the four doc surfaces; `lvt-spy`, `lvt-upload`, `lvt-redact`, `lvt-fx:region-select` and `lvt-fx:auto-click` look the same (six of seven sampled were undocumented). Phase 2's guard walks **docs → implementation** only, so this direction is structurally invisible to it. **Why it is a lower tier than the risk below:** an undocumented-but-real attribute costs the generating agent a capability it never reaches for; a documented-but-absent one makes it emit a page that silently does nothing. Ceiling versus correctness. **For M1 Phase 3:** the reference is safe to use as generation context, but treat its coverage as a floor — if the demo needs a capability the reference omits, check the bundle before concluding it does not exist. A full implementation → docs sweep is its own phase; do not let it expand M1.
- **[M1 — sharpened by Phase 2; was scoped as an M2 improvement] Nothing in the stack validates attribute *names*.** Proved empirically in Phase 2: a document using `lvt-filter`, `lvt-scroll`, and a literal `lvt-totally-made-up` passes `tinkerdown validate` with **zero errors** — unknown `lvt-*` attributes are emitted as inert HTML. Phase 2's guard closes the *docs → implementation* direction, but the direction M1 depends on is the reverse: **generated app → implementation**, which is entirely unguarded. M1 Phase 3's design has the skill "self-correct on `validate` diagnostics until clean," and a clean pass demonstrably does **not** mean the attributes exist — an agent that hallucinates `lvt-sortable` gets a green validate and a silently dead page. This is the empirical case for M2's `Validate()` attribute diagnostics, and it sits *inside* M1's critical path rather than after it. **M1 Phase 3 must either accept the gap explicitly (documented, with the demo's attributes hand-checked) or pull forward the attribute-allowlist portion of M2.** Do not let "validate is clean" stand in for "the app works."
- **[all milestones] Committed-artifact provenance.** `internal/assets/client/tinkerdown-client.browser.js` is a *generated file tracked in git*, so it can silently disagree with the lockfile that supposedly produced it — exactly what issue #295 recorded (`node_modules` stale at 0.11.9 vs a 0.14.3 lockfile, so any rebuild reverted shipped fixes). Mitigated in Phase 0 by `make build` running `npm ci` first. **Residual:** CI cannot catch a regression here, because the e2e tests that would are `//go:build !ci` and do not run there — so a local e2e run before committing a rebuilt bundle is the only gate, not a formality. Tracked as [#297](https://github.com/livetemplate/tinkerdown/issues/297) (browser e2e smoke subset in CI), with a standing pre-bump checklist under § M2–M5 phases so the milestone that next regenerates the artifact makes the call deliberately rather than rediscovering this.
- **[M1] "30 seconds" is a generation-reliability target, not a framework-latency target.** The framework leg is tens of ms; the budget is spent on the LLM. M1 must treat the generation-context assets (manifest + style guide + attribute reference + few-shot corpus) as first-class — that's what makes generation one-shot.
- **[M1] Generated-app safety in M1 rests on the manifest + policy lint + proportional operation-summary review + `--allow-exec`, not yet on runtime `WithActionPolicy` (M3).** Acceptable for the demo with a human approver in the loop; M3 hardens it. State this explicitly so M1 isn't mistaken for production-grade authz. **Correction (Phase 4 finding): config `confirm:` is *not* an enforced M1 safety layer** — it is inert (copied into the runtime action but never read or shipped to the client). The M1 click-time affordance is the `data-confirm` HTML attribute authored on the button; enforcing the manifest's `confirm:` *regardless of render path* is precisely what M3's `WithActionPolicy` adds. Until then, `confirm:` in a manifest is a declaration of intent, not a gate.
- **[cross-repo] Upstream-first milestones (M2–M4) require tagged releases before tinkerdown can pin them** — adds release overhead per **session-guide** convention 11.

---

## Appendix A — Decisions log `[skip on phase execution]`

- **Demo-first over upstream-first** (operator decision): M1 ships on existing primitives; upstream hardening (Validate API, WithActionPolicy, introspection, components) sequenced as M2–M4. Rejected upstream-first because it pushes the acceptance test several milestones out and front-loads 3-repo cross-cutting work.
- **Claude Code skill over a CLI-embedded LLM** (operator decision): the generator is Claude Code itself; tinkerdown stays a pure Go validate/serve tool with no API-key/network dependency. A standalone `tinkerdown generate` CLI is a possible later addition, not M1.
- **Skill command name — DECIDED (operator, in prereview): the generator is the primary default skill `/tinkerdown`.** Generation is what you get by default (`/tinkerdown "<intent>"`); the existing `skills/tinkerdown/` skill is *extended* so generation is its default action (not a separate `/tinkerdown-generate`). Every other tinkerdown capability is **namespaced** `/tinkerdown:<verb>` (e.g. `/tinkerdown:save` for persist/capture). Captured workflow skills (e.g. `skills/pii-access-approval/`) are their own standalone skills. A future standalone `tinkerdown generate` CLI remains a possible later addition, not M1. *(Rejected: a separate `/tinkerdown-generate` skill — it splits the primary action off the default namespace.)*
- **Manifest = extend `tinkerdown.yaml`, not a new file** — **confirmed** in M1 Phase 1's Audit. Reuses the existing project-config surface (`SourceConfig`, `AuthConfig`, `Action`, `StylingConfig`) rather than introducing a parallel policy file.
- **Approval is enforced by a precedence tier, not by banning frontmatter declarations** (operator decision, M1 Phase 1). Phase 1's Audit found that frontmatter can declare its own `sources:`/`actions:` and that `MergeFromFrontmatter` overwrites config entries by name — so a generated doc could redefine an approved name and defeat approval-by-declaration. *Rejected:* "when a manifest is present, frontmatter may not declare sources/actions at all" — a conditional prohibition is confusing and forces per-field tracking of what is required versus forbidden. *Chosen:* extend the precedence order the docs already define with a top tier — **approved definitions are pinned** (1. manifest-approved · 2. frontmatter · 3. config defaults). One name-level rule, no field-level bookkeeping, and no behavior change when no manifest exists. Runtime safety (precedence) and generation-time feedback (lint) stay separate mechanisms.
- **Reference-demo alternatives rejected** (operator picked PII / data-export approval; research-grounded):
  - *Break-glass K8s JIT access → PR* — **rejected.** Research finding: real JIT tools (Sym, Teleport, Indent, ConductorOne) deliberately **bypass git** and provision directly with auto-expiry because incident-time speed beats a merge cycle. "JIT→PR" is the niche/awkward case (searches came back empty — the absence *is* the finding).
  - *Durable K8s access-grant → PR (GitOps)* — real and mainstream, but narrower than PII-export friction and less universal; kept as the optional grant-PR variant of the Approve action, not the headline.
  - *Feature-flag / config change approval → PR* — the research's *tightest* "approval→git" fit with no speed tension; strong runner-up, but less tied to the high-stakes compliance friction the operator prioritized.
  - **PII / data-export access approval — CHOSEN**: highest-stakes real friction (analyst/support needs PII/prod-DB access; today = Slack-ping + hand-run query + weak audit + compliance exposure). Approval output is a durable, auditable, scoped grant + audit record (git-native grant PR optional). Exercises the full reframe.

---

## Appendix B — Expected output (worked example) `[skip on phase execution]`

The two concrete artifacts of the M1 demo. **(1)** is authored once by the team (the workspace manifest); **(2)** is what `/tinkerdown` *generates and serves* for the PII console — the golden `app.md`. **Both are now committed and validated at `examples/pii-access-approval/`** (M1 Phase 4); the snippets below are **corrected to match what was actually built** — the pre-execution drafts here used non-existent attributes and the wrong action schema, exactly the fiction Phase 3's stricter `validate` now rejects.

### (1) `tinkerdown.yaml` — the workspace manifest (authored once)

```yaml
# Approved data sources + named actions an LLM may wire up. Everything else is off-limits.
sources:
  access_requests:                 # the pending/approved/denied queue
    describes: "The pending / approved / denied queue of PII access requests."
    type: sqlite
    db: ./data/access.db
    table: access_requests
    readonly: false
  audit_log:  { describes: "Append-only decision trail.", type: sqlite, db: ./data/access.db, table: audit_log, readonly: false }
  datasets:   { describes: "Requestable-dataset catalog (read-only).", type: sqlite, db: ./data/access.db, table: datasets, readonly: true }

actions:                           # kind:/statements: — a batch runs atomically (ExecTx)
  approve-export:
    describes: "Runs a bounded SELECT of up to the request's row cap from orders_pii (synthetic PII) into exports, appends an audit row, marks the request approved — atomically."
    kind: sql
    source: access_requests
    confirm: "Approve this request and run the bounded PII export?"   # declared intent; enforced at click-time via data-confirm in app.md; M3 WithActionPolicy enforces server-side
    params: { id: { type: number, required: true } }
    statements:                    # only :id (+ :operator) come from the client — cap/dataset are read from the row (server-authoritative)
      - "INSERT INTO exports (request_id, exported_at, name, email)
           SELECT :id, datetime('now'), name, email FROM orders_pii
           LIMIT (SELECT row_cap FROM access_requests WHERE id = :id)"
      - "INSERT INTO audit_log (ts, approver, requester, dataset, decision, scope, reason, ttl)
           SELECT datetime('now'), :operator, requester, dataset, 'approved', scope, reason, ttl
           FROM access_requests WHERE id = :id"
      - "UPDATE access_requests SET status = 'approved', approver = :operator WHERE id = :id"
  deny-request:
    describes: "Marks the request denied and appends an audit row. No data access granted."
    kind: sql
    source: access_requests
    params: { id: { type: number, required: true } }
    statements:
      - "INSERT INTO audit_log (ts, approver, requester, dataset, decision, scope, reason, ttl)
           SELECT datetime('now'), :operator, requester, dataset, 'denied', scope, reason, ttl
           FROM access_requests WHERE id = :id"
      - "UPDATE access_requests SET status = 'denied', approver = :operator WHERE id = :id"

generation:                        # the approved-for-generation surface (keys: sources / actions)
  sources: [access_requests, audit_log, datasets]
  actions: [approve-export, deny-request]

styling: { theme: clean }          # optional — omit for sane PicoCSS/theme defaults
```

### (2) The generated `app.md` — what `/tinkerdown` emits + serves (the golden artifact)

Modeled on `examples/team-tasks/app.md`: hand-written `{{range .Data}}` rows (the rich per-row decision context a bare `lvt-columns` table cannot format), per-row buttons that send only `data-id`, and `data-confirm` for the click-time dialog. **No `lvt-filter`/`lvt-actions`/`lvt-value`** — those do not exist. Abridged (full file committed):

````markdown
## [Pending] status = pending | [All] | [Approved] status = approved | [Denied] status = denied

```lvt
<article lvt-source="access_requests">
  <figure><table>
    <thead><tr><th>Requester</th><th>Dataset</th><th>Scope (bounded)</th>…<th>Decision</th></tr></thead>
    <tbody>
      {{range .Data}}
      <tr data-key="{{.Id}}">
        <td><kbd>{{.Requester}}</kbd></td><td><code>{{.Dataset}}</code></td><td><small>{{.Scope}}</small></td>…
        <td>{{if eq .Status "pending"}}
          <button name="approve-export" data-id="{{.Id}}"
                  data-confirm="Approve {{.Requester}}'s bounded export of {{.Dataset}}?">Approve</button>
          <button name="deny-request" data-id="{{.Id}}" data-confirm="Deny this request?">Deny</button>
        {{else}}<small>by {{.Approver}}</small>{{end}}</td>
      </tr>
      {{end}}
    </tbody>
  </table></figure>
  <footer><details><summary>Request access</summary>
    <form name="Add" lvt-el:reset:on:success> … <button type="submit">Request access</button></form>
  </details></footer>
</article>
```
````

**Operation summary the operator OKs before this serves** (privileged → surfaced; from `tinkerdown validate --summary`, emitting JSON): `access_requests` (write), `audit_log` (write), `datasets` (read), action `approve-export` → its `describes:` verbatim (**names the `orders_pii` read** — the sensitive op a name-only summary would miss), action `deny-request` → writes audit. No `exec`/network unless the optional grant-PR variant adds `gh pr create`.

---

## Appendix C — Competitive landscape `[skip on phase execution]`

*(Data-backed support for § Context "Where existing tools fall short". Capabilities as of 2025–2026; many tools added AI features recently — re-verify pricing/features at execution. Sources at the end.)*

**The whitespace, precisely.** No single tool does all four of **{LLM-generates-bespoke-UI · governed/approved data sources · ephemeral/disposable · editable non-opaque source}**. The two closest near-misses and the axis each fails:
- **Superblocks Clark — 2/4** (strongest governance in market): LLM-generation *with* governance (design system, permissions, SSO, audit auto-applied), but output is a **persistent** production app in **freeform React** → fails *ephemeral* + *constrained-vocabulary*. Tinkerdown's line vs Clark is ephemerality + the single-file constrained vocabulary, **not** governance.
- **Anthropic generative UI / Claude Cowork + MCP — 3/4**: generation + ephemeral + (since Apr 2026) live data via MCP, but **no approved-sources policy gate** (MCP connects to whatever is wired) + emits **opaque HTML/JS** you re-prompt. Fails *governance gate* + *editable non-opaque source*.

Tinkerdown's genuinely unique combination: the **constrained `lvt-*` single-file vocabulary**, **live server-authoritative WebSocket data**, and **the governance gate + ephemeral-by-default** together.

**① Internal-tool builders (low-code) — premise: build & *maintain* one durable app.**

| Tool | AI capability | OSS / price | Gap vs the thesis |
|---|---|---|---|
| **Retool** | Retool AI + hourly-billed AI Agents bolted on | Commercial; Team $10, Business $50 /user/mo | Maintains one persistent big app; AI assists editing, not throwaway governed UIs |
| **Superblocks (Clark)** | Clark: "first AI agent to build internal enterprise apps" w/ auto governance | Commercial ($60M funded) | **Closest on governance (2/4)** but persistent app + freeform React — not ephemeral, not constrained-vocabulary |
| **Appsmith** | Limited AI code assist | Apache-2.0 self-host; Business $15/user/mo | Human-built catch-all app; no generate-and-discard, no per-generation policy |
| **ToolJet** | NL → full app (PRD+UI+DB+CRUD) | AGPL self-host; paid $19/builder/mo | Generation targets a *kept* app; no ephemeral/governed-per-generation model; freeform JS |
| **Budibase / Windmill / UI Bakery** | Agents / code-gen / NL app generator | OSS self-host + paid tiers | Durable maintained apps; AI assists authoring, not disposable governed UIs |
| **Airplane** | — | **Acquired by Airtable Dec 2023; sunset Mar 2024** | Defunct — historical data point, not a live competitor |

**② AI UI/app generators (prompt → code) — premise: emit code from a prompt.**

| Tool | What it emits | OSS / price | Gap vs the thesis |
|---|---|---|---|
| **v0 (Vercel)** | Production React (Next/Tailwind/shadcn) | Commercial; free $5, Pro $20/mo | Opaque React (re-prompt to change); no live binding to *your* sources; no governance; no constrained vocab |
| **bolt.new / Lovable / Replit Agent** | Full-stack app in freeform code | Commercial; ~$20–25/mo+ | Opaque code, re-prompt workflow; no approved-source policy; persistent, not governed-ephemeral |
| **Val Town** | Editable "vals" (code); save≈deploy ~100ms; MCP | Commercial; free tier, Pro $100/yr | Emits *editable code* but ungoverned/freeform; no approved-source policy, no constrained declarative vocab |
| **Claude Artifacts** | Interactive HTML/JS in a side panel | Consumer/Pro sub | Sandbox **blocks external API/DB calls** (no live data to your sources); opaque code; no policy gate |
| **ChatGPT Canvas** | Code in a side editor (doesn't run the app) | Consumer/Pro sub | No live data, no governance, no constrained vocabulary |

**③ Declarative data-app frameworks (lighter code) — premise: human writes & maintains it.**

| Tool | How it works | OSS | Gap vs the thesis |
|---|---|---|---|
| **Streamlit / Gradio** | Python script / function → data app | Apache-2.0 | Human-authored & maintained; not LLM-emitted-and-discarded; no generate-on-demand governance |
| **Reflex / Marimo / Anvil** | Pure-Python full-stack / reactive notebook | OSS | Developer-maintained app; not disposable, not LLM-authored-per-use |
| **Evidence.dev / Observable / Datasette / Quarto** | SQL-in-markdown / JS data site / SQLite UI / polyglot docs | OSS | Human-maintained; not ephemeral or LLM-emitted; no per-generation policy |

**④ The malleable / disposable / on-demand-software lineage (the conceptual frontier tinkerdown productizes).**

| Work | What it argues | Where it stops short |
|---|---|---|
| **Ink & Switch — *Malleable Software*** (2025) | Reshape tools at point of use; "tools not apps"; AI coding promising but not sufficient alone | Vision + prototypes, not a governed live-data product; no policy gate |
| **Geoffrey Litt — malleable software in the age of LLMs** | LLMs enable personal, adaptable software | Conceptual lineage; no constrained-vocabulary governed-data implementation |
| **Thariq Shihipar — *Unreasonable Effectiveness of HTML*** (2026) | HTML > Markdown as agent output: interactive, shareable artifacts | Argues the *format*; no live server-authoritative data, no approved-sources policy, no constrained vocab |
| **Anthropic generative UI / Cowork + MCP** (2026) | Living HTML+JS widgets, live via MCP | **3/4** — but no approved-sources policy gate + opaque emitted source |

**Sources** (re-verify at execution): Retool [pricing](https://retool.com/pricing); Superblocks Clark [announcement](https://www.superblocks.com/blog/announcing-clark-ai), [launch (BusinessWire)](https://www.businesswire.com/news/home/20250520713435/en/); Appsmith [pricing](https://www.appsmith.com/pricing); ToolJet [comparison](https://blog.tooljet.com/appsmith-vs-budibase-vs-tooljet/); Windmill [pricing](https://www.windmill.dev/pricing); UI Bakery [pricing](https://uibakery.io/pricing); Airplane [sunset (Failory)](https://newsletter.failory.com/p/airplanes-last-flight); v0/Bolt/Lovable [comparison](https://www.digitalapplied.com/blog/v0-lovable-bolt-ai-app-builder-comparison); Replit [effort-based pricing](https://replit.com/blog/effort-based-pricing); [Val Town](https://www.val.town/pricing); Claude generative UI vs Canvas vs Artifacts [(MindStudio)](https://www.mindstudio.ai/blog/what-is-claude-generative-ui-vs-canvas-artifacts), [live dashboards (VentureBeat)](https://venturebeat.com/data/anthropics-claude-code-artifacts-update-brings-live-shared-dashboards-and-interactive-workspaces-to-enterprises); [Streamlit vs Gradio](https://www.modern-datatools.com/compare/streamlit-vs-gradio); [Evidence](https://docs.evidence.dev/); Ink & Switch [Malleable Software](https://www.inkandswitch.com/essay/malleable-software/); Geoffrey Litt [LLM end-user programming](https://www.geoffreylitt.com/2023/03/25/llm-end-user-programming.html); Simon Willison on [the HTML piece](https://simonwillison.net/2026/May/8/unreasonable-effectiveness-of-html/).
