// Package settings contains declared reconstruction parameters. The graph is a
// design assumption, NOT a learned graph or the recovered graph of the old code.
package settings

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/sufficiency"
)

func DiagnosticTemplate() diagnostics.Model {
	return diagnostics.Model{
		Version: "declared-template-04-unfitted", GraphVersion: "declared-graph-04",
		Provenance: diagnostics.Provenance{Kind: "declared_parameters", Description: "NEW declared graph, directions and equal priors; likelihood 0.5 is an unfitted placeholder, never a training result"},
		Hypotheses: []diagnostics.Hypothesis{
			{ID: "D", Causes: []string{"D"}, Prior: .25, Likelihood: [3]float64{.5, .5, .5}, Strength: [4]float64{1, 0, 0, 1}, Direction: [4]int{-1, 0, 0, -1}},
			{ID: "S", Causes: []string{"S"}, Prior: .25, Likelihood: [3]float64{.5, .5, .5}, Strength: [4]float64{1, 1, 1, 1}, Direction: [4]int{-1, -1, -1, -1}},
			{ID: "C", Causes: []string{"C"}, Prior: .25, Likelihood: [3]float64{.5, .5, .5}, Strength: [4]float64{1, 1, 1, 1}, Direction: [4]int{-1, -1, 1, 1}},
			{ID: "DS", Causes: []string{"D", "S"}, Prior: .25, Likelihood: [3]float64{.5, .5, .5}, Strength: [4]float64{1, 1, 1, 1}, Direction: [4]int{-1, -1, -1, -1}},
		},
		Edges:      []diagnostics.Edge{{From: "D", To: "K0"}, {From: "D", To: "K3"}, {From: "S", To: "K0"}, {From: "S", To: "K1"}, {From: "S", To: "K2"}, {From: "S", To: "K3"}, {From: "C", To: "K0"}, {From: "C", To: "K1"}, {From: "C", To: "K2"}, {From: "C", To: "K3"}},
		Centrality: "outdegree", BaseDemand: [2]float64{40, 25}, BaseRaw: 90, BaseCapacity: 100, FeatureRatios: [3]float64{1.1, .85, .85},
		KPIThreshold: [4]float64{.05, .05, .1, 2}, KPIWeights: [4]float64{.35, .25, .15, .25}, ScoreWeights: [3]float64{.65, .25, .1}, Relevance: .15, MinimumConfidence: .25, Epsilon: 1e-12,
	}
}
func Analysis(m diagnostics.Model) analysispipe.Config {
	return analysispipe.Config{Mode: analysispipe.EvidenceRequired, Monitor: monitor.DefaultConfig(), Admission: sufficiency.StrictConfig(), Diagnosis: diagnostics.CloneModel(m)}
}
