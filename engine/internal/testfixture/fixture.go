// Package testfixture contains deliberately synthetic parameters and readings.
// It is imported only by engineering test/demo code, not by inference packages.
package testfixture

import (
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/sufficiency"
)

func Model() diagnostics.Model {
	return diagnostics.Model{
		Version: "synthetic-parameter-fixture-03", GraphVersion: "synthetic-graph-fixture-03",
		Provenance: diagnostics.Provenance{Kind: "synthetic_fixture", Description: "Hand-set Bernoulli probabilities 0.9/0.1 and example graph, ONLY for software verification; not learned or validated on a company"},
		Hypotheses: []diagnostics.Hypothesis{
			{ID: "D", Causes: []string{"D"}, Prior: .25, Likelihood: [3]float64{.9, .1, .1}, Strength: [4]float64{1, 0, 0, 1}, Direction: [4]int{-1, 0, 0, -1}},
			{ID: "S", Causes: []string{"S"}, Prior: .25, Likelihood: [3]float64{.1, .9, .1}, Strength: [4]float64{1, 1, 1, 1}, Direction: [4]int{-1, -1, -1, -1}},
			{ID: "C", Causes: []string{"C"}, Prior: .25, Likelihood: [3]float64{.1, .1, .9}, Strength: [4]float64{1, 1, 1, 1}, Direction: [4]int{-1, -1, 1, 1}},
			{ID: "DS", Causes: []string{"D", "S"}, Prior: .25, Likelihood: [3]float64{.9, .9, .1}, Strength: [4]float64{1, 1, 1, 1}, Direction: [4]int{-1, -1, -1, -1}},
		},
		Edges:      []diagnostics.Edge{{From: "D", To: "K0"}, {From: "D", To: "K3"}, {From: "S", To: "K0"}, {From: "S", To: "K1"}, {From: "S", To: "K2"}, {From: "S", To: "K3"}, {From: "C", To: "K0"}, {From: "C", To: "K1"}, {From: "C", To: "K2"}, {From: "C", To: "K3"}},
		Centrality: "outdegree", BaseDemand: [2]float64{40, 25}, BaseRaw: 90, BaseCapacity: 100, FeatureRatios: [3]float64{1.1, .85, .85},
		KPIThreshold: [4]float64{.05, .05, .1, 2}, KPIWeights: [4]float64{.35, .25, .15, .25}, ScoreWeights: [3]float64{.65, .25, .1}, Relevance: .15, MinimumConfidence: .25, Epsilon: 1e-12,
	}
}
func Config(mode string) analysispipe.Config {
	return analysispipe.Config{Mode: mode, Monitor: monitor.DefaultConfig(), Admission: sufficiency.StrictConfig(), Diagnosis: Model()}
}
func Packet(day int) observation.Packet {
	p := observation.Packet{Schema: observation.Schema, Day: day, Context: observation.Context{ExecutedPlan: [2]float64{40, 25}, ActionID: "u0", PlanVersion: "test-plan", ConstraintVersion: "test-constraints"}, Readings: map[observation.Field]observation.Measurement{}}
	for f, v := range monitor.DefaultConfig().InitialValues {
		p.Readings[f] = observation.Read(v, 0)
	}
	return p
}

// These are technical input records, NOT trajectories of a production process.
func Packets() []observation.Packet {
	ps := []observation.Packet{}
	for day := 0; day < 24; day++ {
		p := Packet(day)
		switch day {
		case 0, 5:
			for f := range p.Readings {
				p.Readings[f] = observation.Read(-1, 0)
			}
		case 2:
			for f := range p.Readings {
				p.Readings[f] = observation.Read(*p.Readings[f].Value, 1)
			}
		case 3, 4, 16:
			p.Readings[observation.RawEnd] = observation.Missing()
		case 7:
			p.Readings[observation.ShippedA] = observation.Read(1000, 0)
		case 8, 9:
			p.Readings[observation.DemandA] = observation.Read(60, 0)
			p.Readings[observation.DemandB] = observation.Read(35, 0)
		case 10, 11:
			p.Readings[observation.RawArrival] = observation.Read(40, 0)
			p.Readings[observation.RawEnd] = observation.Read(90, 0)
		case 12:
			p.Readings[observation.Capacity] = observation.Read(75, 0)
			p.Readings[observation.RawEnd] = observation.Read(700, 0)
		case 13:
			p.Readings[observation.DemandA] = observation.Read(60, 0)
			p.Readings[observation.DemandB] = observation.Read(35, 0)
			p.Readings[observation.RawArrival] = observation.Read(40, 0)
			p.Readings[observation.RawEnd] = observation.Read(90, 0)
		case 14:
			p.Readings[observation.RawArrival] = observation.Missing()
			p.Readings[observation.RawEnd] = observation.Read(90, 0)
		case 15:
			p.Readings[observation.RawArrival] = observation.Read(40, 1)
			p.Readings[observation.RawEnd] = observation.Read(90, 0)
		case 18:
			for f := range p.Readings {
				p.Readings[f] = observation.Missing()
			}
		case 20:
			p.Readings[observation.OutputA] = observation.Read(-1, 0)
		case 22:
			p.Readings[observation.Capacity] = observation.Read(0, 0)
		}
		ps = append(ps, p)
	}
	return ps
}
