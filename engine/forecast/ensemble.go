package forecast

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/randomstream"
	"fmt"
	"math"
	"sort"
	"strconv"
)

type ScenarioPath struct {
	ID          string            `json:"id"`
	Hypothesis  string            `json:"hypothesis"`
	Probability float64           `json:"probability"`
	Days        []plant.Exogenous `json:"forecast_forcing"`
}
type Group struct {
	ID             string          `json:"id"`
	Probability    float64         `json:"conditional_posterior"`
	Paths          int             `json:"path_count"`
	SeveritySource [3]string       `json:"severity_source"`
	ActiveMeans    plant.Exogenous `json:"active_means"`
}
type Ensemble struct {
	Version   string         `json:"version"`
	Seed      uint64         `json:"forecast_seed"`
	ClosedDay int            `json:"closed_day"`
	Groups    []Group        `json:"groups"`
	Paths     []ScenarioPath `json:"paths"`
}

func validateDiagnosis(d diagnostics.Result, x Estimate, c Config) error {
	if !d.ForecastAllowed || !d.Ran || len(d.ForecastWeights) == 0 || len(d.Features) != 3 {
		return fmt.Errorf("diagnosis is not ready for forecasting")
	}
	expected := [3]bool{x.Demand[0]+x.Demand[1] > c.FeatureRatios[0]*(c.Parameters.BaseDemand[0]+c.Parameters.BaseDemand[1]), x.Arrival < c.FeatureRatios[1]*c.BaseRaw, x.Capacity < c.FeatureRatios[2]*c.BaseCapacity}
	ids := [3]string{"demand_high", "supply_low", "capacity_low"}
	for j, f := range d.Features {
		if f.ID != ids[j] || f.Value == nil || *f.Value != expected[j] {
			return fmt.Errorf("diagnostic feature inconsistent with observation estimate")
		}
	}
	relevant := map[string]bool{}
	for _, id := range d.Relevant {
		if relevant[id] {
			return fmt.Errorf("duplicate relevant ID")
		}
		relevant[id] = true
	}
	posterior := map[string]float64{}
	mass := 0.0
	total := 0.0
	for _, v := range d.Ranked {
		if _, ok := posterior[v.ID]; ok || !unit(v.Posterior) {
			return fmt.Errorf("invalid posterior")
		}
		posterior[v.ID] = v.Posterior
		total += v.Posterior
		if relevant[v.ID] {
			mass += v.Posterior
		}
	}
	if !close(total, 1) || mass <= 0 {
		return fmt.Errorf("posterior probability mass invalid")
	}
	seen := map[string]bool{}
	sum := 0.0
	for _, w := range d.ForecastWeights {
		if !relevant[w.ID] || seen[w.ID] || !unit(w.Probability) || !close(w.Probability, posterior[w.ID]/mass) {
			return fmt.Errorf("weights must be posterior normalized on relevant set")
		}
		seen[w.ID] = true
		sum += w.Probability
	}
	if len(seen) != len(relevant) || math.Abs(sum-1) > 1e-12 {
		return fmt.Errorf("incomplete or invalid forecast weights")
	}
	return nil
}

// Generate builds ONE action-independent ensemble. A hypothesis's paths remain
// unchanged when another hypothesis or action is added. DS recovery is JOINT;
// that and the absent-symptom fallback are explicit new assumptions.
func Generate(x Estimate, d diagnostics.Result, c Config, seed uint64) (Ensemble, error) {
	e := Ensemble{Version: Version, Seed: seed, ClosedDay: x.ClosedDay, Groups: []Group{}, Paths: []ScenarioPath{}}
	if err := c.Validate(); err != nil {
		return e, err
	}
	if err := x.validate(); err != nil {
		return e, err
	}
	if err := validateDiagnosis(d, x, c); err != nil {
		return e, err
	}
	templates := map[string]Template{}
	for _, t := range c.Templates {
		templates[t.ID] = t
	}
	weights := append([]diagnostics.Weighted(nil), d.ForecastWeights...)
	sort.Slice(weights, func(i, j int) bool { return weights[i].ID < weights[j].ID })
	for _, w := range weights {
		t, ok := templates[w.ID]
		if !ok {
			return e, fmt.Errorf("hypothesis %s has no forecast template", w.ID)
		}
		base := plant.Exogenous{Demand: c.Parameters.BaseDemand, Capacity: c.BaseCapacity, RawArrival: c.BaseRaw}
		means := base
		origin := [3]string{"baseline", "baseline", "baseline"}
		if t.Active[0] {
			origin[0] = "declared_fallback"
			means.Demand = plant.Pair{base.Demand[0] * t.Fallback[0], base.Demand[1] * t.Fallback[0]}
			if *d.Features[0].Value {
				means.Demand = plant.Pair(x.Demand)
				origin[0] = "current_observation"
			}
		}
		if t.Active[1] {
			origin[1] = "declared_fallback"
			means.RawArrival = base.RawArrival * t.Fallback[1]
			if *d.Features[1].Value {
				means.RawArrival = x.Arrival
				origin[1] = "current_observation"
			}
		}
		if t.Active[2] {
			origin[2] = "declared_fallback"
			means.Capacity = base.Capacity * t.Fallback[2]
			if *d.Features[2].Value {
				means.Capacity = x.Capacity
				origin[2] = "current_observation"
			}
		}
		e.Groups = append(e.Groups, Group{w.ID, w.Probability, c.PathsPerHypothesis, origin, means})
		for i := 0; i < c.PathsPerHypothesis; i++ {
			key := []string{strconv.Itoa(x.ClosedDay), w.ID, strconv.Itoa(i)}
			noise, err := randomstream.New(seed, "forecast/noise/v04", key...)
			if err != nil {
				return e, err
			}
			recovery, err := randomstream.New(seed, "forecast/recovery/v04", key...)
			if err != nil {
				return e, err
			}
			path := ScenarioPath{ID: fmt.Sprintf("%s/%04d", w.ID, i), Hypothesis: w.ID, Probability: w.Probability / float64(c.PathsPerHypothesis), Days: make([]plant.Exogenous, c.Horizon)}
			active := true
			for h := 0; h < c.Horizon; h++ {
				mu := base
				if active {
					mu = means
				}
				jitter := func(value float64) float64 { return value * math.Max(0, 1+c.RelativeSD*noise.NormFloat64()) }
				next := plant.Exogenous{Demand: plant.Pair{jitter(mu.Demand[0]), jitter(mu.Demand[1])}, Capacity: jitter(mu.Capacity), RawArrival: jitter(mu.RawArrival)}
				for _, v := range []float64{next.Demand[0], next.Demand[1], next.Capacity, next.RawArrival} {
					if !finite(v) {
						return e, fmt.Errorf("forecast forcing overflow")
					}
				}
				path.Days[h] = next
				// Recovery is AFTER this day; even p=1 does not erase the first day's event.
				draw := recovery.Float64()
				if active && draw < c.RecoveryProbability {
					active = false
				}
			}
			e.Paths = append(e.Paths, path)
		}
	}
	return e, nil
}
