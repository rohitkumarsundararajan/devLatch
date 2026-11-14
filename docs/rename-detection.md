# Rename detection (Phase 5)

When a key disappears and another appears at the same place, DevLatch asks: is it the same field under a new name?
If the answer is likely yes, `check` reports one `POSSIBLE_RENAME` (with confidence and evidence) instead of
`REMOVED_KEY` + `ADDED_KEY`. The severity is the same as the removal it replaces, so exit codes do not change.
Turn it off with `devlatch check --renames=false`.

## How a pair is scored (0..1)
Only type-level features are used. Values are never looked at.

| signal | weight | what it measures |
|---|---|---|
| name | 0.35 | camelCase/snake_case-insensitive similarity; abbreviations expanded (`qty` = `quantity`); generic words (`id`, `count`, `name`, `date`...) only scale the score |
| structure | 0.15 | same kinds; for objects, overlap of child key names |
| position | 0.35 | closeness of the key's index among its siblings (left out when order is unknown) |
| co-occurrence | 0.15 | both keys seen in the same fraction of samples |

A hard type gate comes first: incompatible kinds never match (INTEGER and NUMBER are compatible).
Pairs at or above `DefaultThreshold` (0.60) are accepted greedily, best first, one new name per old name.
Only removals that would be reported anyway are candidates, so renames never add a violation.

## Measured quality
Labeled set: 139 hand-labeled (removed, added) pairs, 46 held out. Thresholds are tuned on the 93 training pairs
to the highest recall with precision >= 0.9. Reproduce with `go test ./internal/rename -run TestAblation -v`.

| config | threshold | precision | recall | F1 |
|---|---|---|---|---|
| name only (baseline) | 0.61 | 0.94 | 0.62 | 0.75 |
| + type gate | 0.61 | 0.94 | 0.62 | 0.75 |
| + structure | 0.73 | 0.93 | 0.58 | 0.72 |
| + position | 0.51 | 0.96 | 1.00 | 0.98 |
| + co-occurrence (full) | 0.59 | 1.00 | 1.00 | 1.00 |

Sibling position is the signal that matters most. Structure alone did not help on scalar fields.

## Be careful with these numbers
- The labeled set was written by the author, so it is easier than real traffic. A perfect held-out score on 46 pairs
  means the set is easy, not that the detector is perfect.
- The name rules were adjusted after a real false positive (`role_id` -> `user_id`, which share only the word
  "identifier"), so the set is no longer fully independent of the model.
- Real numbers come from the real-drift case study (Phase 7).

## Known limits
- Contracts recorded before key order was kept have no positions. Re-run `devlatch record` and `devlatch approve`;
  until then position is left out and only obvious name matches are caught.
- A field that moves and changes names at once is not detected.
- `role_id` -> `user_id` in the same slot, with the same type, is reported as a possible rename at confidence ~0.65.
  That is the intended behavior: it is a possible rename, not a certain one.
- Only siblings in the same object are compared. Moving a field into a nested object is not a rename here.
- Optional keys: the contract keeps only whether a key was required, so seen-fraction is approximate for them.