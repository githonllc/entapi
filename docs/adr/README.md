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
| [0001](0001-presence-follows-encoding-json-key-matching.md) | Request key matching is strict: a case-variant key is an error, never a silent no-op | Proposed | TBD |
| [0002](0002-deterministic-pagination-pk-tiebreak.md) | Every generated list order ends with a primary-key tiebreak | Proposed | TBD |
| [0003](0003-per-run-atomic-generation.md) | Generation is atomic per run, not per file | Proposed | TBD |
| [0004](0004-cleanup-ownership-by-marker.md) | The ownership marker, not the current schema, decides what cleanup may delete | Proposed | TBD |
| [0005](0005-contains-operators-gated-by-searchable.md) | Substring operators require the Searchable marker, not just Filterable | Proposed | TBD |
