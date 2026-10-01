# Correction before the final C12-R run

A post-run audit found repeated forecast RNG base seeds in the initial C12 driver:
seed + 31*profile maps (200001,supply) and (200032,normal) to 200032, with
analogous boundary overlaps for adjacent profiles. The selector equality and
raw counts remain observations of that initial run, but the intended independent
seed-block interpretation of its primary intervals is imperfect. Therefore those
intervals and tables are NOT used as final confirmatory evidence.

The old driver, pre-run protocol, source lock, initial raw results, and audit are
retained. The selector, model balances, screening thresholds, block size, targets,
and decision comparisons are unchanged. Only RNG stream addressing and option
bounds were corrected. No parameter was tuned to improve the effect.

C12-R is the final NEW held-out run, seeds 300001--300032. Stream seed:
  base = 1,000,000*master_seed + 150,000*profile
  main forecast = base + 1024*day + scenario
  independent forecast = base + 70,000 + 1024*day + scenario
  observations = base + 140,000 + day
  physical forcing = base + 149,999

For days <=64, scenarios<=1024, five profiles, and <=1000 consecutive master
seeds these addresses are distinct modulo 2^31-1 (the Go math/rand seed period):
within a profile the sub-bands are disjoint; profile bands have width150000;
master bands have width1000000; the full range is shorter than the modulus.
Identical addresses reused across policies on the same case are intentional
common random numbers, not independent repetitions. Development and previous C12
are not pooled with C12-R. A new protocol and source hash lock precede this run.
