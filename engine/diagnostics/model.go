// Package diagnostics implements manuscript §3.3, with all unrecovered graph
// and likelihood parameters supplied explicitly in a versioned model.
package diagnostics

import (
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/observation"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

const Version = "hypothesis-ranking-0.3.0"

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Hypothesis struct {
	ID         string     `json:"id"`
	Causes     []string   `json:"causes"`
	Prior      float64    `json:"prior"`
	Likelihood [3]float64 `json:"probability_feature_present"`
	Strength   [4]float64 `json:"kpi_link_strength"`
	Direction  [4]int     `json:"kpi_direction"`
}
type Provenance struct {
	Kind          string  `json:"kind"`
	Description   string  `json:"description"`
	DatasetID     string  `json:"dataset_id"`
	DatasetSHA256 string  `json:"dataset_sha256"`
	TrainingCount int     `json:"training_count"`
	Laplace       float64 `json:"laplace"`
}
type Model struct {
	Version      string       `json:"version"`
	GraphVersion string       `json:"graph_version"`
	Provenance   Provenance   `json:"provenance"`
	Hypotheses   []Hypothesis `json:"hypotheses"`
	Edges        []Edge       `json:"edges"`
	// Chosen full-graph centrality measure is outdegree. This is NEW, explicit,
	// fixed across cases; graph interpretation is not a causal identification.
	Centrality        string     `json:"centrality"`
	BaseDemand        [2]float64 `json:"base_demand"`
	BaseRaw           float64    `json:"base_raw"`
	BaseCapacity      float64    `json:"base_capacity"`
	FeatureRatios     [3]float64 `json:"feature_ratios"`
	KPIThreshold      [4]float64 `json:"kpi_threshold"`
	KPIWeights        [4]float64 `json:"kpi_weights"`
	ScoreWeights      [3]float64 `json:"score_weights"`
	Relevance         float64    `json:"relevance_threshold"`
	MinimumConfidence float64    `json:"minimum_confidence"`
	Epsilon           float64    `json:"epsilon"`
}

func (m Model) Validate() error {
	if m.Version == "" || m.GraphVersion == "" || m.Centrality != "outdegree" || len(m.Hypotheses) == 0 {
		return fmt.Errorf("model identity, hypotheses and outdegree measure required")
	}
	if m.Provenance.Description == "" || (m.Provenance.Kind != "synthetic_fixture" && m.Provenance.Kind != "training_fit" && m.Provenance.Kind != "declared_parameters") {
		return fmt.Errorf("explicit model provenance required")
	}
	if m.Provenance.Kind == "training_fit" && (m.Provenance.TrainingCount < 1 || m.Provenance.DatasetID == "" || len(m.Provenance.DatasetSHA256) != 64 || !positive(m.Provenance.Laplace)) {
		return fmt.Errorf("incomplete fit provenance")
	}
	if !positive(m.Epsilon) || !positive(m.BaseRaw) || !positive(m.BaseCapacity) {
		return fmt.Errorf("invalid scale")
	}
	for _, v := range m.BaseDemand {
		if !positive(v) {
			return fmt.Errorf("invalid baseline demand")
		}
	}
	for _, v := range m.FeatureRatios {
		if !positive(v) {
			return fmt.Errorf("invalid feature threshold")
		}
	}
	for _, v := range m.KPIThreshold {
		if !positive(v) {
			return fmt.Errorf("invalid KPI threshold")
		}
	}
	if !unit(m.Relevance) || !unit(m.MinimumConfidence) || !weights(m.ScoreWeights[:]) || !weights(m.KPIWeights[:]) {
		return fmt.Errorf("invalid ranking weights/thresholds")
	}
	edges := map[Edge]bool{}
	for _, e := range m.Edges {
		if e.From == "" || e.To == "" || e.From == e.To || edges[e] {
			return fmt.Errorf("invalid graph edge")
		}
		edges[e] = true
	}
	ids := map[string]bool{}
	sum := 0.0
	for _, h := range m.Hypotheses {
		if h.ID == "" || ids[h.ID] || len(h.Causes) == 0 || !positive(h.Prior) {
			return fmt.Errorf("invalid hypothesis identity/prior")
		}
		ids[h.ID] = true
		sum += h.Prior
		cs := map[string]bool{}
		for _, c := range h.Causes {
			if c == "" || cs[c] {
				return fmt.Errorf("invalid cause set")
			}
			cs[c] = true
		}
		for _, p := range h.Likelihood {
			if !positive(p) || p >= 1 {
				return fmt.Errorf("likelihood must lie strictly between zero and one")
			}
		}
		for i, a := range h.Strength {
			if !unit(a) || h.Direction[i] < -1 || h.Direction[i] > 1 {
				return fmt.Errorf("invalid structural score")
			}
			if a > 0 && !m.reaches(h, i) {
				return fmt.Errorf("positive strength has no graph path")
			}
		}
	}
	if math.Abs(sum-1) > 1e-12 {
		return fmt.Errorf("priors must sum to one")
	}
	return nil
}
func positive(x float64) bool { return observation.Finite(x) && x > 0 }
func unit(x float64) bool     { return observation.Finite(x) && x >= 0 && x <= 1 }
func weights(xs []float64) bool {
	sum := 0.0
	for _, x := range xs {
		if !unit(x) {
			return false
		}
		sum += x
	}
	return math.Abs(sum-1) <= 1e-12
}
func (m Model) reaches(h Hypothesis, k int) bool {
	target := fmt.Sprintf("K%d", k)
	seen := map[string]bool{}
	queue := append([]string(nil), h.Causes...)
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		if seen[v] {
			continue
		}
		seen[v] = true
		if v == target {
			return true
		}
		for _, e := range m.Edges {
			if e.From == v {
				queue = append(queue, e.To)
			}
		}
	}
	return false
}
func (m Model) centrality(h Hypothesis) float64 {
	c := 0.0
	for _, cause := range h.Causes {
		for _, e := range m.Edges {
			if e.From == cause {
				c++
			}
		}
	}
	return c / float64(len(h.Causes))
}
func CloneModel(m Model) Model {
	n := m
	n.Edges = append([]Edge(nil), m.Edges...)
	n.Hypotheses = append([]Hypothesis(nil), m.Hypotheses...)
	for i := range n.Hypotheses {
		n.Hypotheses[i].Causes = append([]string(nil), m.Hypotheses[i].Causes...)
	}
	return n
}

// Training records are accepted only here; the inference interface has neither
// true-cause labels nor any future trajectory. These are full binary records,
// not observations with missing values silently mapped to false.
type TrainingExample struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Features [3]bool `json:"features"`
}

func FitBernoulli(template Model, examples []TrainingExample, epsilon float64, datasetID, newVersion string) (Model, error) {
	m := CloneModel(template)
	if err := m.Validate(); err != nil {
		return m, err
	}
	if len(examples) == 0 || !positive(epsilon) || datasetID == "" || newVersion == "" {
		return m, fmt.Errorf("training data, positive smoothing and version required")
	}
	xs := append([]TrainingExample(nil), examples...)
	sort.Slice(xs, func(i, j int) bool { return xs[i].ID < xs[j].ID })
	known := map[string]int{}
	for i, h := range m.Hypotheses {
		known[h.ID] = i
	}
	counts := make([]int, len(m.Hypotheses))
	ones := make([][3]int, len(counts))
	seen := map[string]bool{}
	for _, x := range xs {
		i, ok := known[x.Label]
		if !ok || x.ID == "" || seen[x.ID] {
			return m, fmt.Errorf("unknown label or duplicate/missing record ID")
		}
		seen[x.ID] = true
		counts[i]++
		for j, b := range x.Features {
			if b {
				ones[i][j]++
			}
		}
	}
	total := float64(len(xs)) + epsilon*float64(len(counts))
	if !positive(total) {
		return m, fmt.Errorf("training count overflow")
	}
	for i := range counts {
		if counts[i] == 0 {
			return m, fmt.Errorf("no training examples for %s", m.Hypotheses[i].ID)
		}
		// Priors remain the explicitly supplied template priors (chapter 4 uses 0.25).
		// Imbalanced calibration availability must not silently become a new prior.
		for j := 0; j < 3; j++ {
			m.Hypotheses[i].Likelihood[j] = (float64(ones[i][j]) + epsilon) / (float64(counts[i]) + 2*epsilon)
		}
	}
	raw, err := json.Marshal(xs)
	if err != nil {
		return m, err
	}
	hash := sha256.Sum256(raw)
	m.Version = newVersion
	m.Provenance = Provenance{Kind: "training_fit", Description: "Bernoulli frequencies with Laplace smoothing and unchanged declared priors; not externally validated", DatasetID: datasetID, DatasetSHA256: hex.EncodeToString(hash[:]), TrainingCount: len(xs), Laplace: epsilon}
	return m, m.Validate()
}
