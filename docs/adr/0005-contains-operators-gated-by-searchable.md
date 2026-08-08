# 0005 — Substring operators require the Searchable marker, not just Filterable

**Status:** Accepted (2026-08-06, owner-ratified via arbitration — panel
`[degraded]`: Gemini 3.1 Pro + Fable seats, both → this option; deciding fact:
the repo has **zero released tags**, local and remote, and the one documented
consumer has no `_contains` dependency, so pre-tag is the only window in which
a URL-contract fix costs nothing) · **Date:** 2026-08-06 ·
**Tracking issue:** [#64](https://github.com/githonllc/entdomain/issues/64)

## Context

The query surface gates its three dimensions per field, and two of the three
justify the gate with the same threat: an unchecked *sort* is "an
unindexed-scan trigger and an ordering oracle" (`funcs_filter.go`,
`isSortable`), and free-text *search* requires `AsSearchable()` per field. Yet
`AsFilterable()` emits **every** operator ent derives for the type
(`funcs_filter.go`, `filterParams`), which for a string includes `_contains`,
`_icontains` and `_suffix` — `LIKE '%x%'` shapes that defeat B-tree indexes
exactly like an unchecked sort, plus case-folding variants that are
unindexable on most collations.

So one annotation (`AsFilterable()`, documented as "exact matching" in the
Searchable conflict message) silently grants the scan surface the other two
markers exist to withhold. The emission-side rationale ("emitting an operator
costs nothing; adding one later breaks a URL contract") cuts both ways: every
emitted operator **is** a permanent URL contract, including the expensive
ones.

## Decision

Operator classes, not one bucket:

- `AsFilterable()` emits the index-friendly class: EQ, NEQ, In, NotIn, GT,
  GTE, LT, LTE, IsNil/NotNil, and `_prefix` (left-anchored LIKE uses the
  index).
- `AsSearchable()` on the same field additionally emits the substring class:
  `_contains`, `_icontains`, `_ieq`, `_suffix` — the operators whose cost
  profile matches the free-text disjunction that marker already owns.

Two classification notes, recorded from the ratification audit:

- `_ieq` (EqualFold) is **exact-match semantics**, not substring — it sits in
  the expensive class purely for its cost profile: `LOWER(x) = LOWER(?)` is
  unindexable without a functional index, exactly like a substring scan.
- A field that is Searchable but **not** Filterable emits **no structured
  parameters** (unchanged from today): Filterable is what opens the structured
  surface; Searchable widens an already-open surface with the substring class,
  and alone it still only feeds the free-text `q` disjunction.

`schema_conflicts.go` keeps refusing markers the type cannot honour; the
conflict message for Searchable-without-Contains stays accurate under the new
split.

## Consequences

- **Breaking** for consumers already using `_contains` parameters from a
  Filterable-only field: the parameter disappears from the struct, and
  `encoding/json`/form binding starts ignoring it silently. This is the
  strongest argument against — it converts a working (if expensive) query
  into an unfiltered one without an error. Mitigations: migration notes in
  both READMEs per the established convention, and a major-version tag.
- Field annotations become honest: the expensive surface is opt-in per field,
  consistent with the sort allow-list's security posture.
- **An accepted coupling, stated rather than silent:** wanting the structured
  `_contains` parameter now requires `AsSearchable()`, which *also* enrolls
  the field in the free-text `q` disjunction — the two are not separately
  expressible. That is the price of refusing a third marker, and it is paid
  knowingly; a consumer who needs one without the other writes their own
  predicate against ent directly.
- `queryconflict` fixture gains rows; `query` fixture regenerates with a
  Searchable+Filterable field demonstrating both classes.

## Alternatives considered

- **Keep one bucket, document it** ("Filterable = the full operator surface,
  including scans") — zero breakage; the inconsistency becomes a stated
  contract. The honest fallback if the break is judged too expensive.
- **A third marker (`AsSubstringFilterable`)** — taxonomy growth for a
  distinction Searchable already encodes; rejected.

## Addendum (2026-08-08, DESIGN-v3)

Under the op-in-value wire format (ADR-0007) the per-operator parameters
disappear, so the gate's **enforcement point moves** from "the parameter does
not exist" to the generated parser: a substring operator prefix
(`like:`/`ilike:`/`prefix:`/`suffix:`) on a field without `Searchable` is a
**400**, never a silent fallback to an eq literal — parse rule 4 exists
precisely so this gate stays loud while unknown prefixes fall back (rule 5).
The gating semantics of this ADR are unchanged.
