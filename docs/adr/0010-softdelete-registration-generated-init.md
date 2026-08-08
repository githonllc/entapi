# 0010 — Soft-delete registration: generated newConfig partial, never Mount

**Status:** Accepted, spike completed (2026-08-08) · **Date:** 2026-08-08 ·
**Tracking issue:** [#70](https://github.com/githonllc/entapi/issues/70) ·
**Design:** `docs/DESIGN-v3-final.md` §4.3

## Context

Before #70 a consumer called the generated `RegisterSoftDelete(client)` once.
That ritual could be forgotten, silently turning soft deletes into hard deletes.
Two candidate homes had already been eliminated:

- **Inside `API(client)`/Mount** — semantics would depend on whether a process
  builds an HTTP surface. A cron or worker could silently hard-delete.
- **Ent-native mixin `Hooks()`/`Interceptors()`** — a framework mixin cannot
  name the consumer's generated `*Client`, which is required to re-dispatch a
  DELETE as an UPDATE. Declaring mixin hooks also switches Ent to the runtime
  package format and replaces one ritual with a blank import of `ent/runtime`.

The original decision proposed generated `init()` wiring. The spike had to
prove whether such an indirection was actually consulted during mutation.

## Decision

Keep the architectural decision — generate registration into the consumer's
Ent package — but replace the dead `init()` mechanism with an Ent partial named
`config/init/fields/entapi_softdelete`.

In Ent v0.14.4, `entc/gen/template/client.tmpl:107` executes every
`config/init/fields/*` partial inside `newConfig`, after the fresh `hooks` and
`inters` values are allocated and before `cfg.options(opts...)`. The partial
appends `softDeleteHook()` and `softDeleteTraverser()` directly to each
soft-deletable entity's config slices. The helpers remain unexported in
`entapi_softdelete.go`; generated `RegisterSoftDelete` is removed.

This is a partial extending Ent's own `client.go`, not a standalone output
file. `entc/gen/template.go:172-208` recognizes the name as a partial and
`entc/gen/graph.go:1207` extends the existing template rather than emitting a
new file. A graph with no `SoftDeleteMixin` renders no bytes into `client.go`.

## Spike result against Ent v0.14.4

- **The `init()` indirection answer is no.** `meta.tmpl:53-71` emits package
  hook/interceptor arrays only for schema-declared hooks or interceptors, and
  `client.tmpl:441-446` consults those arrays only in that branch. The fixture
  has no reachable global slot; `DocClient.Hooks()` otherwise returns its
  per-instance slice. An `init()` has no client instance to mutate.
- **The `newConfig` injection answer is yes.** The generated `client.go`
  contains the marker and direct assignments, and a real SQLite delete through
  a plain `NewClient` leaves the row on disk while `Doc.Query()` excludes it.
- **All construction paths agree.** Real SQLite tests cover `NewClient`,
  `Open`, and `enttest.Open`; all funnel through `newConfig`, with no generated
  initialization ordering dependency.
- **Multiple clients agree.** Two live clients with independent databases each
  install and execute their own hook and interceptor slices.
- **Privacy coexists.** A fixture with Ent's privacy feature and an
  always-deny policy rejects an undecided mutation and query. With an explicit allow
  decision, the delete hook's re-dispatched UPDATE and the filtered read both
  succeed, proving the hook preserves the privacy context rather than bypassing
  or losing it.
- **The no-op path is byte-clean.** A guard generates a no-mixin fixture once
  with plain Ent and once with this extension and compares `client.go` byte for
  byte. A second guard pins the exact injection marker and assignments.

The behavioral evidence lives in `internal/softdeleteproof`; the generator
guards live in `softdelete_config_init_test.go`.

## Consequences

- Embedding `SoftDeleteMixin` is the whole consumer opt-in. No construction
  call and no generated `init()` remain.
- Registration is unconditional per generated client. A consumer that wants
  hard deletes removes the mixin or uses `WithHardDelete` for one call.
- Injection appends the soft-delete hook before any consumer `Use` call. It is
  therefore `hooks[0]`, which `base.tmpl:308-324` applies outermost. This is a
  behavior change from caller-controlled registration order.
- `RegisterSoftDelete` is a loud removal with migration notes, not a deprecated
  no-op or alias.
- The extension depends on an undocumented Ent partial point. The generated
  marker guard is intentionally required for every Ent upgrade.
- The same placement argument pre-decides future row policies: generated
  consumer-side Ent extension points, never HTTP wiring.

## Alternatives considered

- **Generated `init()`** — rejected by the spike: no reachable per-client slot
  exists, and manufacturing one through mixin hooks reintroduces Ent's runtime
  blank-import ritual without giving the hook a typed client.
- **Register inside Mount** — rejected because non-HTTP processes would not get
  the data-safety behavior.
- **Mixin-native hooks** — rejected because the framework cannot re-dispatch
  through the consumer's generated client.
- **Keep explicit `RegisterSoftDelete(client)`** — the fallback branch; not
  selected because the partial rendered, formatted, compiled, and passed the
  real SQLite mutation and privacy proofs.
