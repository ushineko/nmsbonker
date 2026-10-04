# Spec 017: The amount audit says what to change, and can change it

**Issue**: #20

## Status: INCOMPLETE

## Context

The Report section's amount audit says how many reward amounts are over the
limits ("417 of 4064 reward amount(s) are above the configured limits") and
names each one's contributors, but not what to adjust. When the contributors
are built-in tweaks, the adjustment can be calculated: in the build that
prompted this, 408 flags were x150 against the x100 ratio limit, from
`ChestAndLootMaterials10x` x10 under `MissionBoardRewards` `ITEM_MULT` x15, and
lowering that one parameter to 10 clears all 408.

## Requirements

- R1 A built-in parameter declares the audit categories it multiplies with a
  `scales="..."` attribute on its `@param` line (`product`, `substance`,
  `units`, `nanites`, `specials`). Only such a parameter is ever lowered.
- R2 `audit.Recommend(flags, thresholds, knobs)` returns a plan. For each flag
  over a limit, the latest contributor in its chain with a declared parameter
  for the flag's category and room above its minimum is lowered to the largest
  whole value that brings the predicted amount within every limit, or to its
  minimum, after which the contributor before it is tried. A shared parameter
  takes the lowest value any flag needs. A lowered contributor's factor is
  predicted as min(new value, applied factor), so a cap that held still holds.
- R3 The plan lists each change (mod, parameter, old, new, flags it brings
  under the limits), how many flags remain, and the mods behind those that it
  cannot lower (library scripts, or built-ins already at their minimum).
- R4 Only multipliers are lowered. No limit is raised and no cap is set.
- R5 The plan is made against the parameter values the audited build used. If
  any candidate parameter has changed since that build, no plan is made and the
  front ends say to rebuild first (`ErrParamsChangedSinceBuild`).
- R6 Core: `PlanAuditFix` (plan for an audit result in hand) and `AuditFix`
  (re-run the audit, plan, and with `Apply` save the new values). Nothing
  rebuilds.
- R7 CLI: `nmsbonker audit` prints the plan under the audit; `--fix` saves it
  and says to rebuild.
- R8 GUI: the Report section's audit block replaces the advice paragraph with
  one line on why the limits exist and the plan's lines. A Lower multipliers…
  button, enabled when the plan has changes, opens a confirmation listing every
  change and saying that a new value applies to every reward the tweak
  multiplies and that nothing is rebuilt. Confirming saves through
  `SetTweakParam` and says to rebuild.
- R9 The report file's advice points at `nmsbonker audit` / `--fix`.

## Acceptance Criteria

- [x] AC1 Unit: the last multiplier in a chain is lowered to the boundary
  value; an absolute limit and a ratio limit are both met; a parameter at its
  minimum hands the remainder to the one before it; a library script is named
  and never changed; a parameter does not move for a category it does not
  scale; a cap-held contributor is predicted as still held
  (`internal/build/audit/fix_test.go`).
- [x] AC2 Integration, real game and compiler: a compounding set of built-ins
  is flagged, `AuditFix` with `Apply` lowers parameters, a second `AuditFix`
  refuses with `ErrParamsChangedSinceBuild`, and the rebuild's audit has no
  flags (`TestTheAuditFixMakesTheNextBuildClean`).
- [x] AC3 On the build that prompted this (417 flags), `nmsbonker audit`
  proposes Mission board items 15 -> 10, Mission board units 100 -> 16 and
  Nexus units 15 -> 11, bringing all 417 under the limits.
- [x] AC4 GUI: with a plan, the audit block shows the headline and each change
  and offers Lower multipliers… (`TestAFlaggedAmountNamesItsContributorsAndTheWayOut`);
  rendered off-screen and inspected.
- [ ] AC5 In the window: Lower multipliers…, then rebuild, and the audit
  reports no flagged amounts. Recorded by the user.

## Risks & Assumptions

- The prediction assumes a parameter scales amounts linearly, true of every
  `scales` parameter today (each is a `*` on AmountMin/AmountMax, optionally
  capped). A future non-linear one must not declare `scales`.
- A lowered value applies to every reward the tweak multiplies, not only the
  flagged ones. The confirmation says so.
- Int32-saturated amounts are predicted from their clamped value, so the plan
  may be short for them; the rebuild's audit is the check.
- Rollback: the values are ordinary parameter overrides; Reset on the Tweaks
  page or `nmsbonker tweaks reset NAME` restores them.

## E2E Test Plan

1. On a build with flagged amounts from built-ins, open Report: the audit block
   lists the multipliers to lower (AC4).
2. Lower multipliers…, confirm, rebuild: the audit reports no flagged amounts
   (AC5).
