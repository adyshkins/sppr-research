package diagnostics_test

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"fmt"
	"reflect"
	"testing"
)

func examples() []diagnostics.TrainingExample {
	xs := []diagnostics.TrainingExample{}
	for _, h := range testfixture.Model().Hypotheses {
		for i := 0; i < 3; i++ {
			xs = append(xs, diagnostics.TrainingExample{ID: fmt.Sprintf("%s-%d", h.ID, i), Label: h.ID, Features: [3]bool{i < 2, false, true}})
		}
	}
	return xs
}
func TestLaplaceFitAgainstHandCounts(t *testing.T) {
	m, e := diagnostics.FitBernoulli(testfixture.Model(), examples(), 1, "synthetic-count-test", "test-fit-v1")
	if e != nil {
		t.Fatal(e)
	}
	for _, h := range m.Hypotheses {
		near(t, h.Prior, .25)
		near(t, h.Likelihood[0], 3.0/5)
		near(t, h.Likelihood[1], 1.0/5)
		near(t, h.Likelihood[2], 4.0/5)
	}
	if m.Provenance.TrainingCount != 12 || m.Provenance.Kind != "training_fit" || len(m.Provenance.DatasetSHA256) != 64 {
		t.Fatal(m.Provenance)
	}
}
func TestTrainingOrderDoesNotChangeModel(t *testing.T) {
	xs := examples()
	m, e := diagnostics.FitBernoulli(testfixture.Model(), xs, 1, "synthetic-count-test", "test-fit-v1")
	if e != nil {
		t.Fatal(e)
	}
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
	n, e := diagnostics.FitBernoulli(testfixture.Model(), xs, 1, "synthetic-count-test", "test-fit-v1")
	if e != nil || !reflect.DeepEqual(m, n) {
		t.Fatal(e)
	}
}
func TestFitRequiresEachClassAndUniqueIDs(t *testing.T) {
	xs := examples()
	bad := [][]diagnostics.TrainingExample{xs[:3], append(xs, xs[0]), append(xs, diagnostics.TrainingExample{ID: "new", Label: "unknown"}), nil}
	for _, x := range bad {
		if _, e := diagnostics.FitBernoulli(testfixture.Model(), x, 1, "test", "v1"); e == nil {
			t.Fatal("invalid training accepted")
		}
	}
}
func TestFitDoesNotChangeInputModel(t *testing.T) {
	m := testfixture.Model()
	before := diagnostics.CloneModel(m)
	if _, e := diagnostics.FitBernoulli(m, examples(), 1, "test", "v1"); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(m, before) {
		t.Fatal("mutated template")
	}
}

func TestFitPreservesDeclaredPriorsOnImbalancedData(t *testing.T) {
	m := testfixture.Model()
	priors := []float64{.1, .2, .3, .4}
	for i := range m.Hypotheses {
		m.Hypotheses[i].Prior = priors[i]
	}
	xs := examples()
	xs = append(xs, diagnostics.TrainingExample{ID: "extra-D", Label: "D"})
	out, e := diagnostics.FitBernoulli(m, xs, 1, "synthetic-imbalanced-test", "test-fit-v2")
	if e != nil {
		t.Fatal(e)
	}
	for i, h := range out.Hypotheses {
		near(t, h.Prior, priors[i])
	}
}
