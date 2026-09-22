package observation

import (
	"fmt"
	"math"
)

type QualityConfig struct {
	Weights            [3]float64 `json:"weights_completeness_freshness_consistency"`
	Minimum            float64    `json:"minimum"`
	CrossTolerance     float64    `json:"cross_tolerance"`
	BaseCapacity       float64    `json:"base_capacity"`
	CapacityPerUnit    [2]float64 `json:"capacity_per_unit"`
	RawDaysDenominator float64    `json:"raw_days_denominator"`
}

func DissertationQualityConfig() QualityConfig {
	return QualityConfig{[3]float64{.4, .3, .3}, .7, .2, 100, [2]float64{1, 1.6}, 90}
}
func (c QualityConfig) Validate() error {
	sum := 0.0
	for _, v := range c.Weights {
		if !FiniteNonnegative(v) {
			return fmt.Errorf("invalid quality weight")
		}
		sum += v
	}
	if math.Abs(sum-1) > 1e-12 || !Finite(c.Minimum) || c.Minimum <= 0 || c.Minimum > 1 || !FiniteNonnegative(c.CrossTolerance) || !FiniteNonnegative(c.BaseCapacity) || c.BaseCapacity == 0 || !FiniteNonnegative(c.RawDaysDenominator) || c.RawDaysDenominator == 0 {
		return fmt.Errorf("invalid quality configuration")
	}
	for _, v := range c.CapacityPerUnit {
		if !FiniteNonnegative(v) || v == 0 {
			return fmt.Errorf("invalid capacity coefficient")
		}
	}
	return nil
}

type Check struct {
	ID       string  `json:"id"`
	Passed   bool    `json:"passed"`
	Required []Field `json:"required"`
}
type Quality struct {
	Completeness float64 `json:"completeness"`
	Freshness    float64 `json:"freshness"`
	Consistency  float64 `json:"consistency"`
	Score        float64 `json:"score"`
	GatePassed   bool    `json:"gate_passed"`
	PresentCount int     `json:"present_count"`
	ValidCount   int     `json:"valid_count"`
	Checks       []Check `json:"checks"`
}

// EvaluateQuality preserves the manuscript's additive gate. Freshness is
// defined HERE as fresh/present (zero if no readings); this denominator is
// an explicit interpretation, not a recovered detail of the lost program.
// Invalid-but-present fresh values can compensate a zero consistency score.
// This weakness is deliberately measured, not silently corrected.
func EvaluateQuality(p Packet, c QualityConfig) (Quality, error) {
	var q Quality
	if err := p.Validate(); err != nil {
		return q, err
	}
	if err := c.Validate(); err != nil {
		return q, err
	}
	fresh := 0
	passed := 0
	q.Checks = make([]Check, 0, CheckCount)
	for _, f := range Fields() {
		m := p.Readings[f]
		if m.Present {
			q.PresentCount++
			if m.AgeDays == 0 {
				fresh++
			}
		}
		good := m.Valid()
		if good {
			q.ValidCount++
			passed++
		}
		q.Checks = append(q.Checks, Check{"valid:" + string(f), good, []Field{f}})
	}
	cross := func(id string, fs []Field, fn func([]float64) bool) {
		xs := make([]float64, len(fs))
		ok := true
		for i, f := range fs {
			m := p.Readings[f]
			if !m.Valid() {
				ok = false
				break
			}
			xs[i] = *m.Value
		}
		if ok {
			ok = fn(xs)
		}
		if ok {
			passed++
		}
		q.Checks = append(q.Checks, Check{id, ok, fs})
	}
	cross("capacity_use", []Field{OutputA, OutputB, Capacity}, func(x []float64) bool {
		use := c.CapacityPerUnit[0]*x[0] + c.CapacityPerUnit[1]*x[1]
		cap := x[2] * (1 + p.Context.Overtime)
		limit := (1 + c.CrossTolerance) * cap
		return Finite(use) && Finite(limit) && use <= limit
	})
	cross("shipment_demand_a", []Field{ShippedA, DemandA, BacklogBeforeA}, func(x []float64) bool {
		limit := (1 + c.CrossTolerance) * (x[1] + x[2])
		return Finite(limit) && x[0] <= limit
	})
	cross("shipment_demand_b", []Field{ShippedB, DemandB, BacklogBeforeB}, func(x []float64) bool {
		limit := (1 + c.CrossTolerance) * (x[1] + x[2])
		return Finite(limit) && x[0] <= limit
	})
	cross("capacity_upper", []Field{Capacity}, func(x []float64) bool {
		limit := (1 + c.CrossTolerance) * c.BaseCapacity
		return Finite(limit) && x[0] <= limit
	})
	q.Completeness = float64(q.PresentCount) / FieldCount
	if q.PresentCount > 0 {
		q.Freshness = float64(fresh) / float64(q.PresentCount)
	}
	q.Consistency = float64(passed) / CheckCount
	q.Score = c.Weights[0]*q.Completeness + c.Weights[1]*q.Freshness + c.Weights[2]*q.Consistency
	q.GatePassed = q.Score >= c.Minimum
	return q, nil
}

// ComputeKPI uses restored observations and the KNOWN plan executed in the
// closed day. It does not read plant.State or plant.Result.KPI.
// Contradictory zero denominators return an error, never fabricated zero KPI.
func ComputeKPI(v map[Field]float64, ctx Context, c QualityConfig) ([4]float64, error) {
	var k [4]float64
	if err := ctx.Validate(); err != nil {
		return k, err
	}
	if err := c.Validate(); err != nil {
		return k, err
	}
	if len(v) != FieldCount {
		return k, fmt.Errorf("incomplete restored observation")
	}
	for _, f := range Fields() {
		x, ok := v[f]
		if !ok || !FiniteNonnegative(x) {
			return k, fmt.Errorf("invalid restored %s", f)
		}
	}
	demand := v[DemandA] + v[DemandB] + v[BacklogBeforeA] + v[BacklogBeforeB]
	ship := v[ShippedA] + v[ShippedB]
	plan := ctx.ExecutedPlan[0] + ctx.ExecutedPlan[1]
	output := v[OutputA] + v[OutputB]
	used := c.CapacityPerUnit[0]*v[OutputA] + c.CapacityPerUnit[1]*v[OutputB]
	cap := v[Capacity] * (1 + ctx.Overtime)
	deviation := math.Abs(v[OutputA]-ctx.ExecutedPlan[0]) + math.Abs(v[OutputB]-ctx.ExecutedPlan[1])
	for _, x := range []float64{demand, ship, plan, output, used, cap, deviation} {
		if !Finite(x) {
			return k, fmt.Errorf("KPI aggregate overflow")
		}
	}
	k[0] = 1
	if demand > 0 {
		k[0] = ship / demand
	} else if ship != 0 {
		return k, fmt.Errorf("positive shipment with zero total demand")
	}
	k[1] = 1
	if plan > 0 {
		k[1] = math.Max(0, 1-deviation/plan)
	} else if output != 0 {
		return k, fmt.Errorf("positive output with zero plan")
	}
	if cap > 0 {
		k[2] = used / cap
	} else if used != 0 {
		return k, fmt.Errorf("positive capacity use with zero capacity")
	}
	k[3] = v[RawEnd] / c.RawDaysDenominator
	for _, x := range k {
		if !FiniteNonnegative(x) {
			return k, fmt.Errorf("KPI overflow")
		}
	}
	return k, nil
}
