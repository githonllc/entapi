# 0003 — Generation is atomic per run, not per file

**Status:** Accepted (2026-08-06, owner-delegated arbitration — panel `[degraded]`:
Gemini 3.1 Pro + Fable, unanimous, as written; cleanup was already staged after
success, so the two-phase write completes an existing symmetry, and the
SIGKILL residue is honestly priced) · **Date:** 2026-08-06 ·
**Tracking issue:** [#61](https://github.com/githonllc/entdomain/issues/61)

## Context

`writeFile` (`extension.go`) is atomic per file: render → `imports.Process` →
temp file → rename. But the per-node loop renames each file into place as it
goes, so a failure at entity B — a template bug goimports rejects, or plain
ENOSPC — aborts the run **after** entity A's files were already replaced. The
consumer's tree is then a mix of two generations: A current, B previous,
`entdomain_errors.go` whichever ran last.

The doc comment on `writeFile` overclaims: "a run that fails partway therefore
leaves the previous run's output untouched." That is true of one file, false of
the run. No test observes it, because every `wantGenErr` fixture fails in
`checkGraphConflicts`, before the first write.

Cleanup is already staged correctly (it runs only after every file of a
successful run is on disk); the write phase is the only half that is not.

## Decision

Two-phase generation. Phase 1 renders **and formats** every file of the run
into memory, aborting on the first failure with nothing touched — this is
where every realistic failure (template bug, refused schema) lands, because
`imports.Process` runs on the buffer, not on disk. Phase 2 writes: temp file +
rename per file, in one loop, after all buffers exist.

The rename loop itself remains a small non-atomic window (a hard crash between
renames still mixes generations). That residue is accepted and documented
honestly — closing it needs directory swaps that are not atomic across
platforms — and it is a millisecond window reachable only by SIGKILL, against
the current design where every *deterministic* failure lands in it.

## Consequences

- `writeFile` splits into `formatFile` (pure, phase 1) and `writeFormatted`
  (phase 2); the overclaiming comment moves to the run level, where it becomes
  true for every failure the process survives.
- Peak memory holds every rendered file of the run — generated files are tens
  of kilobytes; irrelevant.
- A new fixture-level test injects a failing template on the second entity and
  asserts the first entity's committed file is byte-identical afterwards.

## Alternatives considered

- **Render to a temp directory, then move files across** — same residual
  window, plus cross-device rename hazards; no gain over buffers.
- **Document per-file atomicity and drop the claim** — leaves consumers with
  mixed-generation trees on the most common generator failure class
  (a template bug), which `git status` presents as an inexplicable diff.
