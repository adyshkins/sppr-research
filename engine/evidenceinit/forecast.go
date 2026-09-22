// Package evidenceinit implements ONLY the documented E rule of v0.7 on the
// verified v0.6 source base. The missing v0.7 implementation is not claimed recovered.
package evidenceinit

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/randomstream"
	"fmt"
	"math"
	"strconv"
)

const Version = "E-evidence-initialization-reimplemented-0.8.0"

// Generate preserves hypothesis weights, RNG domains, draw order and state.
// The legacy generator is called for its complete input validation. An absent
// observed feature neutralizes only that factor's declared fallback amplitude.
func Generate(x forecast.Estimate, d diagnostics.Result, c forecast.Config, seed uint64) (forecast.Ensemble, error) {
	e, err := forecast.Generate(x, d, c, seed)
	if err != nil {
		return e, err
	}
	base := plant.Exogenous{Demand: c.Parameters.BaseDemand, Capacity: c.BaseCapacity, RawArrival: c.BaseRaw}
	groups := map[string]forecast.Group{}
	for i, g := range e.Groups {
		for j, f := range d.Features {
			if !*f.Value && g.SeveritySource[j] == "declared_fallback" {
				g.SeveritySource[j] = "baseline_absent_current_feature"
				switch j {
				case 0:
					g.ActiveMeans.Demand = base.Demand
				case 1:
					g.ActiveMeans.RawArrival = base.RawArrival
				case 2:
					g.ActiveMeans.Capacity = base.Capacity
				}
			}
		}
		e.Groups[i] = g
		groups[g.ID] = g
	}
	for i, path := range e.Paths {
		// The legacy ordering is hypothesis then path number. IDs retain their index.
		var n int
		if _, err = fmt.Sscanf(path.ID, path.Hypothesis+"/%d", &n); err != nil {
			return e, err
		}
		key := []string{strconv.Itoa(x.ClosedDay), path.Hypothesis, strconv.Itoa(n)}
		noise, er := randomstream.New(seed, "forecast/noise/v04", key...)
		if er != nil {
			return e, er
		}
		recovery, er := randomstream.New(seed, "forecast/recovery/v04", key...)
		if er != nil {
			return e, er
		}
		active := true
		means := groups[path.Hypothesis].ActiveMeans
		for h := range path.Days {
			mu := base
			if active {
				mu = means
			}
			jitter := func(v float64) float64 { return v * math.Max(0, 1+c.RelativeSD*noise.NormFloat64()) }
			w := plant.Exogenous{Demand: plant.Pair{jitter(mu.Demand[0]), jitter(mu.Demand[1])}, Capacity: jitter(mu.Capacity), RawArrival: jitter(mu.RawArrival)}
			if !observation.FiniteNonnegative(w.Demand[0]) || !observation.FiniteNonnegative(w.Demand[1]) || !observation.FiniteNonnegative(w.Capacity) || !observation.FiniteNonnegative(w.RawArrival) {
				return e, fmt.Errorf("nonfinite E forcing")
			}
			path.Days[h] = w
			draw := recovery.Float64()
			if active && draw < c.RecoveryProbability {
				active = false
			}
		}
		e.Paths[i] = path
	}
	return e, nil
}
func Build(p observation.Packet, k forecast.KnownPlan, d diagnostics.Result, c forecast.Config, seed uint64) (forecast.Result, error) {
	r := forecast.Result{Version: Version, RequiresExpert: d.RequiresExpert, Alternatives: []forecast.ActionForecast{}}
	a, err := forecast.Assess(p, k, c)
	if err != nil {
		return r, err
	}
	r.Admission = a
	if d.Status == "not_required" {
		r.Status = "not_required"
		r.Reason = "NO_DEVIATION"
		return r, nil
	}
	if !a.Passed {
		r.Status = "data_check"
		r.Reason = "FORECAST_INFORMATION_INSUFFICIENT"
		return r, nil
	}
	if !d.ForecastAllowed {
		r.Status = d.Status
		r.Reason = "DIAGNOSIS_DOES_NOT_ALLOW_FORECAST"
		return r, nil
	}
	e, err := Generate(*a.Estimate, d, c, seed)
	if err != nil {
		return r, err
	}
	as, err := forecast.Alternatives(*a.Estimate, c)
	if err != nil {
		return r, err
	}
	r.Alternatives, err = forecast.Evaluate(*a.Estimate, e, c, as)
	if err != nil {
		return r, err
	}
	r.Ensemble = &e
	r.Status = "ready"
	r.Reason = "MODEL_CONDITIONAL_PREDICTIONS_NOT_EXECUTION_PERMISSION"
	return r, nil
}
