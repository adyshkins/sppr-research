# C12-R: locked computational protocol (2026-10-01)

C12-R is a NEW experiment. F10 and E11 outputs are not used in its tables.
Development: seeds 11,12 (180 episodes; these are excluded from inference).
Held-out: seeds 300001--300032; 32 independent seed blocks.
Factors: lambda=0,0.35,1; CVaR filter off/on; delays=0,2,5; profiles normal,
supply, capacity, demand, combined. Each episode is 60 physical days.
Main forecast: 64 equiprobable seven-day paths; alpha=0.95; risk limit=1.25;
nonnegative KPI losses plus a one-off action cost; no horizon discount.
Eight registered action types and two-product balances follow the public R08
plant specification. Observation and forecast generators and delayed scheduling
are NEW and explicitly defined in experiment/run.go. No real expert is involved.

Only one review is pending. A proposal formed after day t is reviewed after t+d,
and a new command executes on day t+d+1. No terminal unexecuted command is charged.
For d>0, initial proposal generation is identical and fully evaluated for every
comparison. All modes share the same observed state, catalog and forecast paths.
The full selector drives the physical episode; the three screening selectors are
run independently, with fresh rollout calls, at EVERY review of that trajectory.
A single mismatch aborts the experiment. No full matrix is supplied to screeners.

Selectors: full enumeration; constant-cost screening; block screening in natural
scenario order; block screening ordered by a precomputed forcing-severity score.
All share the same warm proposal priority and deterministic tie break.
The last two differ ONLY in scenario order. Block size=8; numerical separation
guard=1e-11*max(1,abs(values)); uncertain comparisons are not screened.

Primary endpoints: identical action and rejection reason; number of individual
(candidate,scenario) rollout calls. Four primary savings contrasts, all on the
STRESS profiles only and lambda=.35, filter=on: tail/full, tail/cost,
tail/natural, total(tail)/total(full) including identical proposal work. For each
seed average episode savings equally over 4 stress profiles and 3 delays.
32 seed-block values; 20,000 paired bootstrap resamples, RNG seed 880012;
individual nominal 98.75% percentile intervals (four-comparison Bonferroni
family 95%). No interval over independent daily rows, no treatment of 83,520
reviews as independent experimental replicates. Runtime is secondary/descriptive.

All six criterion/filter cells, all profiles and all delays will be reported;
negative or zero acceleration and any mismatch will not be hidden.
Selector runtime includes bound computation, candidate ordering and scenario
severity ordering (charged conservatively in full to the tail selector).
Common forecast-generation time is separately recorded; proposal time is recorded.
Neither secondary independent validation nor CSV/JSON output is timed as selection.
Warm-cache timing position rotates among the four modes.

Independent risk validation: on every chosen action for lambda=.35, filter=on,
delay=2, compute its risk on 1024 NEW conditional forecast paths, using a separate
seed stream (+9999991). These values NEVER influence choices or tuning and are
not physical-plant safety certificates or observations from an enterprise.

Machine-dependent timing values are not part of the deterministic replay hash.

RNG streams use disjoint addressing, as specified in history/stream_collision_correction.md. The initial C12 series is retained but excluded from final inference. No selector, model-balance, or screening parameter was changed in this correction.
