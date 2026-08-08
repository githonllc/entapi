# 0008 — HTTP layer: generated into the consumer's package, overridden whole-unit at wiring signatures

**Status:** Accepted (2026-08-07, owner-adjudicated in the DESIGN-v3 grilling
session) · **Date:** 2026-08-08 · **Tracking issue:** [#73](https://github.com/githonllc/entapi/issues/73),
[#75](https://github.com/githonllc/entapi/issues/75) · **Design:** `docs/DESIGN-v3-final.md` §2

## Context

v3 takes over the HTTP layer (handlers, routing, mounting). Three forces
constrain where that code may live and how it may be customised: the repo's
load-bearing package split is *by when code runs* (generator vs stdlib-only
`runtime/`, pinned by `TestRuntimePackageIsGeneratorFree`); `entdomain/api`
is a schema-time annotation package, so it must not grow runtime symbols;
and #16/#29 established that customisation inside generated types
(embed + override + `SetSelf`) is the disease that killed the Base layer.

## Decision

- **Placement:** generated handlers, `API(client)` and route registration
  land in the consumer's `ent` package (same marker/cleanup/reserved-name
  machinery as the DTOs). Mechanical helpers (problem writer, status
  mapping, parser core) go to `entdomain/runtime` — `net/http` is stdlib, so
  the stdlib-only invariant holds. `entdomain/api` stays pure schema-time.
- **Entry point:** `ent.API(client)` returns an `http.Handler`
  (plus a `.Mount(mux)` convenience). No framework-side Mount symbol.
- **Handlers are three-step bodies** — bind → call one function → write —
  with **no override points**. The called function is a slot whose type is
  **verbatim the generated wiring function's signature**, and whose default
  value is that wiring function:
  `ent.API(client).With(ent.CreateArticleFn(myCreate))`. Generated code
  calls you; you never embed it.
- **The external door is zero mechanism:** non-CRUD endpoints are registered
  by the consumer directly on their own stdlib mux (Go 1.22+ pattern
  routing); generated DTOs/validation/wiring remain usable à la carte.
  The framework never generates a service skeleton.
- **URL face:** `snake(plural(Name))` paths, five endpoints
  (`GET /xs`, `POST /xs`, `GET/PATCH/DELETE /xs/{id}`); `DeleteBatch` stays
  service-layer only. The update operation is PATCH-only (partial, tri-state
  presence); there is no PUT — full replacement would turn every added schema
  field into a breaking change for deployed clients. **The operation is
  named Patch end to end** (`OpPatch`, `Patch{Entity}`, `Patch{Entity}Fn`,
  `{Entity}PatchRequest`): with no PUT there is no second member for an
  Update/Patch split to distinguish, so one concept gets one name
  (owner, 2026-08-08). ent's own builders keep their `Update*` names —
  that seam lies between two products, not inside this framework's surface.

## Consequences

- Swapping a brain is renaming a function — the default implementation and
  the replacement are the same type; the eager-load trap of
  `New{Entity}Response` surfaces in the author's own code at dev time.
- The List slot's signature changes together with `ListRequest` v2
  (ADR-0007) — they are one contract.
- New generated symbols (`API`, `Mount`, `{Op}{Entity}Fn`) join the #62
  reserved-name refusal.

## Alternatives considered

- **`api.Mount(mux, ent.API(client))`** (the IDEAL.md fiction) — puts a
  runtime symbol in the schema-time package; rejected to keep the
  when-it-runs split exception-free.
- **Slots returning ent entities** with handler-side conversion — forks the
  slot type from the wiring signature and hides the eager-load trap inside
  the framework; rejected.
- **Endpoint-level `http.HandlerFunc` slots** — that is the external door's
  job; merging the doors would discard generated binding/validation.

> Cross-reference: the combination semantics of `With` (variadic ≡ chained,
> last-wins, nil panics at construction) are recorded in ADR-0012.

## Addendum (2026-08-08): the route manifest

`API(client)` registers its endpoints from a **data-shaped manifest** and
exports it: `Routes() []entapi.Route{Method, Path string; Handler
http.Handler}` (stdlib pattern syntax, `Route` lives in the stdlib-only
runtime). Whole-tree mounting stays the default; the manifest exists so a
third-party router (gin/echo) can register routes natively — a ~10-line
consumer-side adapter injects its own path params via `r.SetPathValue`
(Go 1.22+) and calls the handler. The framework itself gains no router
dependency, and the manifest is a data export, not a behavior extension
point: overrides stay `With(...)`, endpoint subsetting stays
`Except`/external.
