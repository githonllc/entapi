# 0010 — Soft-delete registration: generated init in the consumer's package, never Mount

**Status:** Accepted, spike-gated (2026-08-08, owner-adjudicated after
cross-family review killed the mixin-native premise) · **Date:** 2026-08-08 ·
**Tracking issue:** — (assigned when the spike opens) ·
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
