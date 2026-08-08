# Architecture Decision Records

One decision per file, numbered, never renumbered. Status is one of **Proposed**
(written, awaiting the owner's call), **Accepted** (binding — code follows it),
**Superseded by NNNN**.

These records exist because the 2026-08-05 architecture review (main-thread +
cross-family round: Gemini 3.1 Pro seat, degraded Fable seat, every finding
re-verified against source) surfaced decisions that were implicit, contradictory
between two parts of the code, or overclaimed by a comment. Each ADR names the
tracking issue that implements it; the issue carries the full defect context,
the ADR carries the decision and its rationale.

| # | Title | Status | Tracking issue |
|---|---|---|---|
| [0001](0001-presence-follows-encoding-json-key-matching.md) | Request key matching is strict: a case-variant key is an error, never a silent no-op | **Accepted** | [#58](https://github.com/githonllc/entdomain/issues/58) |
| [0002](0002-deterministic-pagination-pk-tiebreak.md) | Every generated list order ends with a primary-key tiebreak | **Accepted** | [#59](https://github.com/githonllc/entdomain/issues/59) |
| [0003](0003-per-run-atomic-generation.md) | Generation is atomic per run, not per file | **Accepted** | [#61](https://github.com/githonllc/entdomain/issues/61) |
| [0004](0004-cleanup-ownership-by-marker.md) | The ownership marker, not the current schema, decides what cleanup may delete | **Accepted** | [#63](https://github.com/githonllc/entdomain/issues/63) |
| [0005](0005-contains-operators-gated-by-searchable.md) | Substring operators require the Searchable marker, not just Filterable | **Accepted** (+ 2026-08-08 addendum: enforcement point under op-in-value) | [#64](https://github.com/githonllc/entdomain/issues/64) |
| [0006](0006-silence-by-default-annotation-model.md) | Silence by default: five deviation words replace the field-scope model | **Accepted** | — |
| [0007](0007-op-in-value-query-wire-format.md) | Query wire format: op-in-value with an underscore-reserved namespace | **Accepted** | — |
| [0008](0008-http-layer-topology-and-brain-swap.md) | HTTP layer: generated into the consumer's package, overridden whole-unit at wiring signatures | **Accepted** | — |
| [0009](0009-rfc9457-errors-bare-success.md) | Errors are RFC 9457 problem+json; success responses are bare | **Accepted** | — |
| [0010](0010-softdelete-registration-generated-init.md) | Soft-delete registration: generated init in the consumer's package, never Mount | **Accepted** (spike-gated) | — |
| [0011](0011-rename-module-to-entapi.md) | Rename the module: entdomain → entapi | **Accepted** (executed) | — |
| [0012](0012-with-composes-as-functional-options.md) | `With` composes as functional options: last-wins, nil panics | **Accepted** | — |
| [0013](0013-duplicate-params-and-value-parse-failures.md) | Repeated filter parameters AND-merge; a value that will not parse is a 400 | **Accepted** | — |

0006–0010 record the DESIGN-v3 adjudications (grilling session 2026-08-07/08 +
one cross-family adversarial review round; process log in `../DESIGN-v3.md`,
final design in `../DESIGN-v3-final.md`). Their tracking issues are assigned
when the implementation arc opens; ADR-0004 also carries a DESIGN-v3 addendum
(marker ownership extends to `openapi.yaml`). 0013 comes from the third
grilling round (2026-08-08, process log `../DESIGN-v3.md` §13) and extends
0007; the round's other twelve rulings are recorded in the final design in
place rather than as ADRs — they are either mechanical consequences of an
existing charter or cheap to reverse before release.

Review-only findings without an ADR (plain bugs or documentation): [#60](https://github.com/githonllc/entdomain/issues/60) offset overflow, [#62](https://github.com/githonllc/entdomain/issues/62) reserved generated names, [#65](https://github.com/githonllc/entdomain/issues/65) `{Entity}ListResponse` role (owner decision), [#66](https://github.com/githonllc/entdomain/issues/66) stale dto.tmpl header, [#67](https://github.com/githonllc/entdomain/issues/67) no-op builder godoc.
