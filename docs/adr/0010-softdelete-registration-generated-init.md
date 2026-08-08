# 0010 — Soft-delete registration: generated init in the consumer's package, never Mount

**Status:** Accepted, spike-gated (2026-08-08, owner-adjudicated after
cross-family review killed the mixin-native premise) · **Date:** 2026-08-08 ·
**Tracking issue:** [#70](https://github.com/githonllc/entapi/issues/70) ·
**Design:** `docs/DESIGN-v3-final.md` §4.3

## Context

Today a consumer calls the generated `RegisterSoftDelete(client)` once — one
line of ritual the IDEAL wants gone. Two candidate homes were eliminated:

- **Inside `API(client)`/Mount** — soft-delete semantics would then depend on
  whether the process builds an HTTP surface: a cron or worker binary that
  only uses the service layer would silently hard-delete. Data-loss class
  trap; violates the scope charter (HTTP wiring must never change service
  behaviour).
- **Ent-native mixin `Hooks()`/`Interceptors()`** — proven dead before the
  spike by two independent blockers (review findings C1/C2): a hook cannot
  redirect a DELETE by `SetOp` because `next` has already captured the DELETE
  execution closure, and re-dispatching needs the consumer's `*Client`,
  which a framework-level mixin cannot name (ent's own soft-delete example
  is consumer-side for exactly this reason); and schema-declared hooks make
  ent demand a blank import of `ent/runtime`, on pain of an initialization
  error on every mutation (`ent.go:508`) — the ritual line survives, in a
  more obscure spelling.

## Decision

The framework **generates the registration into the consumer's ent package**
— generated code can name `*Client` and the per-entity hook slots — and hooks
it via `init()`. Zero consumer ritual; active in every process that holds a
client (HTTP, cron, tests alike). A spike validates hook-slot attachment
timing and multi-client scenarios before templating (the #22 method:
hand-write the target first). If the spike fails, fall back to the explicit
`RegisterSoftDelete(client)` status quo.

## Consequences

- IDEAL's "no `init()`" aesthetic holds for hand-written code only;
  generated code carries one.
- Registration is unconditional per generated package — a consumer who wants
  hard deletes back removes the mixin (schema-level intent), not a call site.
- The same placement argument pre-decides OwnedBy's future home: ent
  extension points wired from generated consumer-side code, never
  wiring/handler-level filtering (DESIGN-v2 §0.2).

## Alternatives considered

- **Register inside Mount** — rejected outright (data loss in non-HTTP
  processes).
- **Mixin-native hooks** — infeasible as established above.
- **Keep the explicit call** — the honest fallback; retained as the spike's
  failure branch.

## Addendum (2026-08-08, second adversarial round — v3 §12)

- **Third spike scenario, adjudicated with the entrest comparison review:
  coexistence with ent privacy.** Deny-by-default privacy rules must not
  reject the soft-delete hook's own reads and writes. The design doc carried
  this from day one; this ADR now does too.
- **Mechanism constraint (findings X2/X3, verified against
  `softdeleteent/client.go:69,191`):** at `init()` time no `*Client` exists,
  and `hooks`/`inters` are per-instance state created fresh by `newConfig` —
  there is no global registry an init can write into. "Hooks it via `init()`"
  therefore concretely means: init populates an **indirection consulted at
  mutation time** (e.g. a mixin-declared hook slot whose implementation
  recovers the typed client via `m.Client()`). Whether that attachment point
  holds is the spike's first question; the fallback is unchanged.
