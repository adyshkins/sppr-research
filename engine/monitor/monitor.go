// Package monitor implements the manuscript's sequential observation monitor.
// It cannot access a plant state, scenario label, true cause or future forcing.
package monitor

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/observation"
	"fmt"
	"math"
)

const Version = "reconstruction-monitor-0.2.0"

type Config struct {
	Quality            observation.QualityConfig     `json:"quality"`
	Targets            [4]float64                    `json:"targets"`
	Threshold          [4]float64                    `json:"threshold"`
	EWMAWeight         float64                       `json:"ewma_weight"`
	EWMALimit          [4]float64                    `json:"ewma_limit"`
	CUSUMSlack         [4]float64                    `json:"cusum_slack"`
	CUSUMLimit         [4]float64                    `json:"cusum_limit"`
	CriticalityWeights [4]float64                    `json:"criticality_weights"`
	InitialValues      map[observation.Field]float64 `json:"initial_values"`
}

// InitialValues is an explicit prior at the synthetic baseline, not measured
// evidence. Its use is recorded per channel as initial_value_imputation.
func DefaultConfig() Config {
	f := observation.Fields()
	x := [15]float64{40, 25, 100, 90, 40, 25, 40, 25, 450, 20, 12.5, 0, 0, 0, 0}
	initial := map[observation.Field]float64{}
	for i, k := range f {
		initial[k] = x[i]
	}
	return Config{observation.DissertationQualityConfig(), [4]float64{1, 1, .8, 5}, [4]float64{.05, .05, .1, 2}, .2, [4]float64{.04, .04, .08, 1.6}, [4]float64{.025, .025, .05, 1}, [4]float64{.15, .15, .3, 6}, [4]float64{.35, .25, .15, .25}, initial}
}
func (c Config) Validate() error {
	if err := c.Quality.Validate(); err != nil {
		return err
	}
	if !observation.Finite(c.EWMAWeight) || c.EWMAWeight <= 0 || c.EWMAWeight > 1 {
		return fmt.Errorf("invalid EWMA weight")
	}
	for _, a := range [][4]float64{c.Threshold, c.EWMALimit, c.CUSUMSlack, c.CUSUMLimit} {
		for _, v := range a {
			if !observation.Finite(v) || v <= 0 {
				return fmt.Errorf("invalid positive monitor parameter")
			}
		}
	}
	sum := 0.0
	for i, v := range c.CriticalityWeights {
		if !observation.FiniteNonnegative(v) || !observation.FiniteNonnegative(c.Targets[i]) {
			return fmt.Errorf("invalid target/weight")
		}
		sum += v
	}
	if math.Abs(sum-1) > 1e-12 {
		return fmt.Errorf("criticality weights must sum to 1")
	}
	if len(c.InitialValues) != observation.FieldCount {
		return fmt.Errorf("all initial channel estimates required")
	}
	for _, f := range observation.Fields() {
		v, ok := c.InitialValues[f]
		if !ok || !observation.FiniteNonnegative(v) {
			return fmt.Errorf("invalid initial value %s", f)
		}
	}
	return nil
}

type Statistics struct {
	EWMA          [4]float64 `json:"ewma"`
	Plus          [4]float64 `json:"cusum_plus"`
	Minus         [4]float64 `json:"cusum_minus"`
	AcceptedSteps int        `json:"accepted_steps"`
}
type Cached struct {
	Value       float64 `json:"value"`
	SampleDay   int     `json:"sample_day"`
	DeliveryDay int     `json:"delivery_day"`
}
type State struct {
	NextDay          int                          `json:"next_day"`
	Statistics       Statistics                   `json:"statistics"`
	Cache            map[observation.Field]Cached `json:"cache"`
	PreviousComputed *bool                        `json:"previous_computed"`
}
type Resolution struct {
	Values           map[observation.Field]float64 `json:"values"`
	Historical       []observation.Field           `json:"historical_imputation"`
	Initial          []observation.Field           `json:"initial_imputation"`
	Stale            []observation.Field           `json:"stale_observations"`
	CacheNotUpdated  []observation.Field           `json:"older_than_cache"`
	SourceSampleDays map[observation.Field]*int    `json:"source_sample_days"`
}
type Signals struct {
	Threshold [4]bool `json:"threshold"`
	EWMA      [4]bool `json:"ewma"`
	Plus      [4]bool `json:"cusum_plus"`
	Minus     [4]bool `json:"cusum_minus"`
}
type Result struct {
	Day         int                 `json:"day"`
	Status      core.Status         `json:"status"`
	Reason      string              `json:"reason"`
	Quality     observation.Quality `json:"quality"`
	Computed    bool                `json:"computed"`
	KPI         *[4]float64         `json:"kpi"`
	Delta       *[4]float64         `json:"delta"`
	Event       *bool               `json:"event"`
	Criticality *float64            `json:"criticality"`
	Signals     *Signals            `json:"signals"`
	Resolution  *Resolution         `json:"resolution"`
	Statistics  Statistics          `json:"statistics"`
	Resumed     bool                `json:"resumed_after_incident"`
	Warnings    []string            `json:"warnings"`
}
type Monitor struct {
	config Config
	state  State
}

func New(c Config, startDay int) (*Monitor, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if startDay < 0 {
		return nil, fmt.Errorf("negative start day")
	}
	c.InitialValues = cloneValues(c.InitialValues)
	return &Monitor{c, State{NextDay: startDay, Cache: map[observation.Field]Cached{}}}, nil
}
func cloneValues(v map[observation.Field]float64) map[observation.Field]float64 {
	w := make(map[observation.Field]float64, len(v))
	for k, x := range v {
		w[k] = x
	}
	return w
}
func cloneCache(v map[observation.Field]Cached) map[observation.Field]Cached {
	w := make(map[observation.Field]Cached, len(v))
	for k, x := range v {
		w[k] = x
	}
	return w
}
func (m *Monitor) Snapshot() State {
	s := m.state
	s.Cache = cloneCache(s.Cache)
	if s.PreviousComputed != nil {
		b := *s.PreviousComputed
		s.PreviousComputed = &b
	}
	return s
}
func (m *Monitor) resolve(p observation.Packet) (Resolution, map[observation.Field]Cached) {
	r := Resolution{Values: map[observation.Field]float64{}, Historical: []observation.Field{}, Initial: []observation.Field{}, Stale: []observation.Field{}, CacheNotUpdated: []observation.Field{}, SourceSampleDays: map[observation.Field]*int{}}
	cache := cloneCache(m.state.Cache)
	for _, f := range observation.Fields() {
		x := p.Readings[f]
		if x.Valid() {
			sampleDay := p.Day - x.AgeDays
			r.Values[f] = *x.Value
			r.SourceSampleDays[f] = &sampleDay
			if x.AgeDays > 0 {
				r.Stale = append(r.Stale, f)
			}
			old, ok := cache[f]
			if !ok || sampleDay >= old.SampleDay {
				cache[f] = Cached{*x.Value, sampleDay, p.Day}
			} else {
				r.CacheNotUpdated = append(r.CacheNotUpdated, f)
			}
		} else if old, ok := cache[f]; ok {
			r.Values[f] = old.Value
			d := old.SampleDay
			r.SourceSampleDays[f] = &d
			r.Historical = append(r.Historical, f)
		} else {
			r.Values[f] = m.config.InitialValues[f]
			r.SourceSampleDays[f] = nil
			r.Initial = append(r.Initial, f)
		}
	}
	return r, cache
}

// Strict mathematical thresholds, with a fixed 8-ULP allowance at numerical
// boundaries. This is a NEW documented numerical convention, not tuning to data.
func above(v, limit float64) bool {
	margin := 8 * (math.Nextafter(1, 2) - 1) * math.Max(1, math.Max(math.Abs(v), math.Abs(limit)))
	return v-limit > margin
}

// Observe commits state atomically. Bad packets (schema, chronology) are API
// errors and do not consume a day. Well-formed low-quality packets consume the
// day but do not update cached estimates, EWMA or CUSUM.
func (m *Monitor) Observe(p observation.Packet) (Result, error) {
	r := Result{Day: p.Day, Warnings: []string{}, Statistics: m.state.Statistics}
	if p.Day != m.state.NextDay {
		return r, fmt.Errorf("expected day %d, got %d", m.state.NextDay, p.Day)
	}
	q, err := observation.EvaluateQuality(p, m.config.Quality)
	if err != nil {
		return r, err
	}
	r.Quality = q
	next := m.Snapshot()
	next.NextDay++
	incident := func(reason string) Result {
		r.Status = core.DataIncident
		r.Reason = reason
		computed := false
		next.PreviousComputed = &computed
		m.state = next
		return r
	}
	if !q.GatePassed {
		return incident("QUALITY_BELOW_MINIMUM"), nil
	}
	if q.ValidCount == 0 {
		r.Warnings = append(r.Warnings, "ADDITIVE_GATE_PASSED_WITH_NO_VALID_READINGS")
	}
	if q.ValidCount < q.PresentCount {
		r.Warnings = append(r.Warnings, "INVALID_PRESENT_READINGS")
	}
	for _, c := range q.Checks {
		if !c.Passed && len(c.Required) > 1 {
			r.Warnings = append(r.Warnings, "FAILED_CROSS_CHECK:"+c.ID)
		}
	}
	resolved, cache := m.resolve(p)
	k, err := observation.ComputeKPI(resolved.Values, p.Context, m.config.Quality)
	if err != nil {
		r.Warnings = append(r.Warnings, err.Error())
		return incident("KPI_UNDEFINED"), nil
	}
	var delta [4]float64
	signals := Signals{}
	stats := m.state.Statistics
	event := false
	criticality := 0.0
	for i, v := range k {
		delta[i] = v - m.config.Targets[i]
		stats.EWMA[i] = m.config.EWMAWeight*delta[i] + (1-m.config.EWMAWeight)*stats.EWMA[i]
		stats.Plus[i] = math.Max(0, stats.Plus[i]+delta[i]-m.config.CUSUMSlack[i])
		stats.Minus[i] = math.Max(0, stats.Minus[i]-delta[i]-m.config.CUSUMSlack[i])
		if !observation.Finite(stats.EWMA[i]) || !observation.Finite(stats.Plus[i]) || !observation.Finite(stats.Minus[i]) {
			return incident("MONITOR_OVERFLOW"), nil
		}
		signals.Threshold[i] = above(math.Abs(delta[i]), m.config.Threshold[i])
		signals.EWMA[i] = above(math.Abs(stats.EWMA[i]), m.config.EWMALimit[i])
		signals.Plus[i] = above(stats.Plus[i], m.config.CUSUMLimit[i])
		signals.Minus[i] = above(stats.Minus[i], m.config.CUSUMLimit[i])
		event = event || signals.Threshold[i] || signals.EWMA[i] || signals.Plus[i] || signals.Minus[i]
		criticality += m.config.CriticalityWeights[i] * math.Min(1, math.Abs(delta[i])/m.config.Threshold[i])
	}
	stats.AcceptedSteps++
	r.Computed = true
	r.KPI = &k
	r.Delta = &delta
	r.Event = &event
	r.Signals = &signals
	r.Resolution = &resolved
	r.Statistics = stats
	r.Status = core.Normal
	r.Reason = "NO_DEVIATION"
	if event {
		r.Status = core.Deviation
		r.Reason = "MONITOR_SIGNAL"
		r.Criticality = &criticality
	}
	r.Resumed = m.state.PreviousComputed != nil && !*m.state.PreviousComputed
	next.Cache = cache
	next.Statistics = stats
	computed := true
	next.PreviousComputed = &computed
	m.state = next
	return r, nil
}
