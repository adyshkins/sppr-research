package diagnostics

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"fmt"
	"math"
	"sort"
)

type Feature struct {
	ID     string              `json:"id"`
	Value  *bool               `json:"value"`
	Reason string              `json:"reason"`
	Fields []observation.Field `json:"fields"`
}
type Ranked struct {
	ID            string   `json:"id"`
	Causes        []string `json:"causes"`
	Prior         float64  `json:"prior"`
	LogLikelihood float64  `json:"log_likelihood"`
	Posterior     float64  `json:"posterior"`
	Impact        float64  `json:"impact"`
	Structural    float64  `json:"structural"`
	Score         float64  `json:"score"`
}
type Weighted struct {
	ID          string  `json:"id"`
	Probability float64 `json:"probability"`
}
type Result struct {
	Version         string     `json:"version"`
	ModelVersion    string     `json:"model_version"`
	GraphVersion    string     `json:"graph_version"`
	ParameterOrigin string     `json:"parameter_origin"`
	Status          string     `json:"status"`
	Reason          string     `json:"reason"`
	Ran             bool       `json:"ran"`
	Features        []Feature  `json:"features"`
	Candidates      []string   `json:"candidate_ids"`
	Ranked          []Ranked   `json:"ranked"`
	Relevant        []string   `json:"relevant_ids"`
	Leading         *string    `json:"leading_id"`
	Confidence      *float64   `json:"confidence"`
	RequiresExpert  bool       `json:"requires_expert"`
	ForecastAllowed bool       `json:"forecast_allowed"`
	ForecastWeights []Weighted `json:"forecast_weights"`
}

func ExtractFeatures(p observation.Packet, m Model) ([]Feature, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	fs := []Feature{{ID: "demand_high", Fields: []observation.Field{observation.DemandA, observation.DemandB}}, {ID: "supply_low", Fields: []observation.Field{observation.RawArrival}}, {ID: "capacity_low", Fields: []observation.Field{observation.Capacity}}}
	for i := range fs {
		good := true
		for _, f := range fs[i].Fields {
			x := p.Readings[f]
			if !x.Valid() {
				good = false
				fs[i].Reason = "INVALID_OR_MISSING_FEATURE"
				break
			}
			if x.AgeDays != 0 {
				good = false
				fs[i].Reason = "STALE_FEATURE"
				break
			}
		}
		if !good {
			continue
		}
		var value bool
		switch i {
		// Aggregate two-product demand is an explicit NEW interpretation.
		case 0:
			total := *p.Readings[observation.DemandA].Value + *p.Readings[observation.DemandB].Value
			base := m.BaseDemand[0] + m.BaseDemand[1]
			if !observation.Finite(total) || !observation.Finite(base) {
				fs[i].Reason = "FEATURE_OVERFLOW"
				continue
			}
			value = total/base > m.FeatureRatios[0]
		case 1:
			value = *p.Readings[observation.RawArrival].Value/m.BaseRaw < m.FeatureRatios[1]
		case 2:
			value = *p.Readings[observation.Capacity].Value/m.BaseCapacity < m.FeatureRatios[2]
		}
		fs[i].Value = &value
		fs[i].Reason = "CURRENT_MEASUREMENTS"
	}
	return fs, nil
}
func Diagnose(p observation.Packet, mon monitor.Result, m Model) (Result, error) {
	r := Result{Version: Version, ModelVersion: m.Version, GraphVersion: m.GraphVersion, ParameterOrigin: m.Provenance.Kind, Features: []Feature{}, Candidates: []string{}, Ranked: []Ranked{}, Relevant: []string{}, ForecastWeights: []Weighted{}}
	if err := m.Validate(); err != nil {
		return r, err
	}
	if err := p.Validate(); err != nil {
		return r, err
	}
	if p.Day != mon.Day {
		return r, fmt.Errorf("diagnostic day mismatch")
	}
	switch mon.Status {
	case core.DataIncident:
		r.Status = "data_check"
		r.Reason = "MONITOR_INFORMATION_UNAVAILABLE"
		return r, nil
	case core.Normal:
		if !mon.Computed || mon.Event == nil || *mon.Event {
			return r, fmt.Errorf("inconsistent normal result")
		}
		r.Status = "not_required"
		r.Reason = "NO_MONITOR_EVENT"
		return r, nil
	case core.Deviation:
		if !mon.Computed || mon.Event == nil || !*mon.Event || mon.Delta == nil || mon.Signals == nil || !unit(mon.Quality.Score) {
			return r, fmt.Errorf("inconsistent deviation result")
		}
	default:
		return r, fmt.Errorf("unknown monitor status")
	}
	fs, err := ExtractFeatures(p, m)
	if err != nil {
		return r, err
	}
	r.Features = fs
	for _, f := range fs {
		if f.Value == nil {
			r.Status = "data_check"
			r.Reason = "DIAGNOSTIC_FEATURES_UNAVAILABLE"
			return r, nil
		}
	}
	hypotheses := append([]Hypothesis(nil), m.Hypotheses...)
	sort.Slice(hypotheses, func(i, j int) bool { return hypotheses[i].ID < hypotheses[j].ID })
	candidates := []Hypothesis{}
	priorSum := 0.0
	maxCentrality := 0.0
	for _, h := range hypotheses {
		maxCentrality = math.Max(maxCentrality, m.centrality(h))
		included := false
		for i := 0; i < 4; i++ {
			s := mon.Signals
			if (s.Threshold[i] || s.EWMA[i] || s.Plus[i] || s.Minus[i]) && m.reaches(h, i) {
				included = true
			}
		}
		if included {
			candidates = append(candidates, h)
			r.Candidates = append(r.Candidates, h.ID)
			priorSum += h.Prior
		}
	}
	if len(candidates) == 0 {
		r.Status = "expert_review"
		r.Reason = "NO_STRUCTURAL_CANDIDATES"
		r.RequiresExpert = true
		return r, nil
	}
	logPost := []float64{}
	maxLog := math.Inf(-1)
	for _, h := range candidates {
		ll := 0.0
		for i, f := range fs {
			prob := h.Likelihood[i]
			if !*f.Value {
				prob = 1 - prob
			}
			ll += math.Log(prob)
		}
		prior := h.Prior / priorSum
		logp := math.Log(prior) + ll
		logPost = append(logPost, logp)
		maxLog = math.Max(maxLog, logp)
		num, den := 0.0, m.Epsilon
		for i, a := range h.Strength {
			d := mon.Delta[i]
			if !observation.Finite(d) {
				return r, fmt.Errorf("nonfinite deviation")
			}
			v := math.Min(1, math.Abs(d)/m.KPIThreshold[i])
			w := m.KPIWeights[i] * a
			den += w
			if h.Direction[i] == 0 || float64(h.Direction[i])*d > 0 {
				num += w * v
			}
		}
		r.Ranked = append(r.Ranked, Ranked{ID: h.ID, Causes: append([]string(nil), h.Causes...), Prior: prior, LogLikelihood: ll, Impact: num / den, Structural: m.centrality(h) / (maxCentrality + m.Epsilon)})
	}
	norm := 0.0
	for _, x := range logPost {
		norm += math.Exp(x - maxLog)
	}
	for i := range r.Ranked {
		x := &r.Ranked[i]
		x.Posterior = math.Exp(logPost[i]-maxLog) / norm
		x.Score = m.ScoreWeights[0]*x.Posterior + m.ScoreWeights[1]*x.Impact + m.ScoreWeights[2]*x.Structural
	}
	sort.Slice(r.Ranked, func(i, j int) bool {
		a, b := r.Ranked[i], r.Ranked[j]
		if a.Score == b.Score {
			return a.ID < b.ID
		}
		return a.Score > b.Score
	})
	leading := r.Ranked[0].ID
	r.Leading = &leading
	conf := mon.Quality.Score * r.Ranked[0].Score
	if len(r.Ranked) > 1 {
		conf = mon.Quality.Score * (r.Ranked[0].Score - r.Ranked[1].Score)
	}
	r.Confidence = &conf
	r.Ran = true
	mass := 0.0
	for _, x := range r.Ranked {
		if x.Score >= m.Relevance {
			r.Relevant = append(r.Relevant, x.ID)
			mass += x.Posterior
		}
	}
	if len(r.Relevant) == 0 {
		r.Status = "expert_review"
		r.Reason = "NO_RELEVANT_HYPOTHESES"
		r.RequiresExpert = true
		return r, nil
	}
	if mass <= 0 || !observation.Finite(mass) {
		r.Status = "expert_review"
		r.Reason = "RELEVANT_POSTERIOR_UNDERFLOW"
		r.RequiresExpert = true
		return r, nil
	}
	for _, x := range r.Ranked {
		if x.Score >= m.Relevance {
			r.ForecastWeights = append(r.ForecastWeights, Weighted{x.ID, x.Posterior / mass})
		}
	}
	r.ForecastAllowed = true
	r.Status = "completed"
	r.Reason = "RANKED_KNOWN_HYPOTHESES"
	if conf < m.MinimumConfidence {
		r.Status = "expert_review"
		r.Reason = "LOW_RANK_MARGIN"
		r.RequiresExpert = true
	}
	return r, nil
}
