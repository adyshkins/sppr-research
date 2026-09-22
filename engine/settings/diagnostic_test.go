package settings

import (
	"testing"
)

func TestDeclaredTemplate(t *testing.T) {
	m := DiagnosticTemplate()
	if e := m.Validate(); e != nil {
		t.Fatal(e)
	}
	if m.Provenance.Kind != "declared_parameters" {
		t.Fatal("incorrect source")
	}
	for _, h := range m.Hypotheses {
		if h.Prior != .25 {
			t.Fatal("prior")
		}
		for _, p := range h.Likelihood {
			if p != .5 {
				t.Fatal("placeholder")
			}
		}
	}
}
func TestSettingsDoNotAlias(t *testing.T) {
	m := DiagnosticTemplate()
	c := Analysis(m)
	c.Diagnosis.Hypotheses[0].Causes[0] = "other"
	if m.Hypotheses[0].Causes[0] != "D" {
		t.Fatal("alias")
	}
}
