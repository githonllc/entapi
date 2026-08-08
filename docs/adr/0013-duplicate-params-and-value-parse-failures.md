# 0013 — Repeated filter parameters AND-merge; a value that will not parse is a 400

**Status:** Accepted (2026-08-08, owner-adjudicated in the third grilling
round) · **Date:** 2026-08-08 ·
**Tracking issue:** [#72](https://github.com/githonllc/entapi/issues/72) ·
**Design:** `docs/DESIGN-v3-final.md` §3.1 · **Extends:** ADR-0007

## Context

ADR-0007 fixed the op-in-value wire syntax and six parse rules. Both rules and
ADR were about the **operator prefix**: is it known, is it allowed on this
field. Two questions the wire format cannot avoid were left open.

**Repeated parameters.** `r.URL.Query()` returns `[]string`, so
`?score=gt:30&score=lt:50` is a shape the parser meets on day one.
DESIGN-v3 §8.7 recorded the semantics as undecided ("AND-merge or 400, decide
at implementation").

**Values that will not parse.** `?score=gt:abc` on an int column,
`?status=eq:banana` on `Enum("active","suspended")`, `?created_at=from:2026/08/08`,
`?id=in:not-a-uuid`. ADR-0007's Consequences already asserted "typed fields
still fail value-parse to 400" as a side note; nothing normative said so, the
six-rule table did not mention it, and closed-set membership (enum, uuid) was
never covered at all.

Both questions have an answer in the owner's prior library, entigo
(`~/workspace/githon/entigo`), and this ADR departs from it on both counts —
which is the reason it exists rather than being a footnote.

- entigo AND-merges nothing: multiple values for one parameter become
  `IN (?)` (`tag_filter.go:136-139`), i.e. repeated name = OR.
- entigo skips unparseable values silently:
  `if v, err := strconv.ParseInt(value, 10, 64); err == nil { qb.And(…) }`
  (`tag_filter.go:154-165`) — the predicate is simply not added.

## Decision

1. **A repeated filter parameter AND-merges.** Every occurrence contributes an
   independent predicate; all of them are ANDed. `?score=gt:30&score=le:50`
   is a half-open range — which `between:` (an inclusive gte+lte sugar) cannot
   express.
2. **A value that will not convert to the field's Go type is a 400**, naming
   the field and the offending value. This covers numeric and time parse
   failures, uuid syntax, and **enum membership** — the legal member set is
   fully known at codegen, so it is emitted as one more arm of the generated
   parse switch at zero runtime cost. This promotes ADR-0007's Consequences
   note to a normative rule and extends it to closed sets.

## Consequences

- **Two migration breaks for entigo callers, and they must be named together
  in the migration notes.** `?status=active&status=archived` used to mean
  `IN ('active','archived')` and now means
  `status='active' AND status='archived'` — an **always-empty result set**,
  returned as a 200 with no warning. And a caller that relied on "a bad value
  just means no filter" now gets a 400 where it used to get a 200.
- The always-empty case is accepted deliberately. Refusing repeated `eq`
  specifically was considered and rejected: it patches a special case onto a
  uniform rule, which is the shape this repository's engineering taste
  explicitly rejects.
- Silent predicate-dropping is gone. That is the point of decision 2: a
  vanished filter means **more rows come back than the caller believes**,
  which under row-level authorization is a data-disclosure path presented to
  the caller as a normal-looking 200.
- Decision 2 joins three refusals already decided the same way: an invalid
  `_sort` field is a 400, a disallowed operator prefix is a 400 (ADR-0005),
  and `_q` on an entity with zero Searchable fields is a 400.
- Rule 5 of ADR-0007 (unknown prefix falls back to an eq literal) is
  unchanged and now composes visibly with decision 2: `?score=12:30` on an
  int column falls back to the literal `"12:30"`, fails to parse, and is a
  400 — which is the right answer for that input anyway.
- OpenAPI cannot express "this parameter may repeat and the occurrences AND"
  in a way generators honour; the filter parameter's `description` carries it
  as prose, alongside the operator-prefix enumeration ADR-0007 already put
  there.

## Alternatives considered

- **Repeated parameter → 400.** Equally uniform and has no always-empty trap,
  but half-open ranges become inexpressible with no replacement. Rejected:
  the trap costs a migration note, the missing range costs a capability.
- **Repeated parameter → `IN` (keep the entigo semantics).** Rejected because
  op-in-value already gives OR a first-class spelling (`in:a,b`), and a second
  path to one meaning contradicts the "one spelling, no aliases" rule that
  ADR-0007's reserved namespace rests on.
- **Silently skip unparseable values (keep the entigo behaviour).** Rejected
  as fails-open; see Consequences.
- **Tier it: 400 for closed sets (enum, uuid), silent skip for numeric and
  time.** Rejected — the split has no principle behind it, and the
  disclosure argument applies identically to a dropped numeric predicate.
