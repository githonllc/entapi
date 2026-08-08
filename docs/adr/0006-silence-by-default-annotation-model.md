# 0006 — Silence by default: five deviation words replace the field-scope model

**Status:** Accepted (2026-08-07, owner-adjudicated in the DESIGN-v3 grilling
session; survived one cross-family adversarial round 2026-08-08) ·
**Date:** 2026-08-08 · **Tracking issue:** — (assigned when the
implementation arc opens) · **Design:** `docs/DESIGN-v3-final.md` §1

## Context

The v1 annotation model enumerated per-field membership in each HTTP surface
(`ScopeCreate/Update/Response/Query` plus six preset builders), costing a 3–4
call chain per field while restating facts the ent schema already declares:
`field.String("title")` alone already fixes type, requiredness, mutability and
sensitivity. Meanwhile ent-side facts (`Sensitive()`) were ignored entirely —
the generator has zero references to `Sensitive`, so annotated sensitive
fields leak into responses today.

## Decision

Annotations express **deviations only**; everything derivable from ent facts
is derived. The surface is:

- Entity level: `api.Resource()` (sole opt-in) and
  `api.Resource().Except(api.Op…)` (fine-grained operation subsetting).
- Field level, exactly five words: `Hidden`, `ReadOnly`, `Searchable`,
  `Filterable`, `Sortable`. Edge level, exactly one: `Expand`.
- Derived, not declared: create/patch presence from
  Optional/Default/Nillable/Immutable (#26 rules), response exclusion from
  `Sensitive()`, the ID from `$.ID`, no-PATCH from empty `MutableFields`
  (refused unless `Except(OpUpdate)` is written).
- Validator values (`MinLen`, `Match`, …) stay ent-side: they are erased
  before codegen (serialization boundary + closure opacity), so the HTTP
  layer classifies ent's own `ValidationError` instead of re-declaring rules.
- **No preset sugar.** Three of the six expressible shape combinations are
  ent field-builder methods, which an annotation cannot alias truthfully;
  the rest are already single words. The reverse lookup table
  (effect → spelling) lives in docs, not in the API.

Contradictions between a deviation and an ent fact fail generation, naming
both facts (DESIGN-v2 §9.2 policy; the full refusal matrix is
`DESIGN-v3-final.md` §1.4).

## Consequences

- Old model (`DomainField`, presets, scopes) deleted in the same arc, no
  coexistence window (v0, DESIGN-v2 §9.5).
- Reused ent facts (`Immutable`, `Sensitive`) also bind the service layer and
  logging — deliberate: the declaration lives in the layer that owns it.
  Two combinations become inexpressible (in-patch-not-create, and
  patch-only-invisible); left unfilled until a real consumer needs them.
- Annotation scarcity becomes signal: any annotation in review is a true
  deviation.

## Alternatives considered

- **Keep scopes, add derivation on top** — two overlapping vocabularies for
  one fact; rejected.
- **Preset sugar over the five words** — cannot alias ent builder methods;
  a generator-side lookalike (`api.InputOnly()` ≈ Sensitive minus its
  service-layer/log semantics) is two spellings with two meanings; rejected.
