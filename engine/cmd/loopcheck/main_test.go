package main

import (
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"testing"
)

func TestCLIRejectsNoBasisAndAmbiguity(t *testing.T) {
	for _, args := range [][]string{nil, {"-model", "x", "-replay", "y"}, {"-replay", "nonexistent"}, {"-verify", "nonexistent"}, {"-config", "nonexistent"}, {"-model", "nonexistent"}, {"extra"}} {
		if e := run(args); e == nil {
			t.Fatal(args)
		}
	}
}
func TestEngineeringSuiteRejectsFixtureAsTrainedModel(t *testing.T) {
	if _, e := engineeringConfigs(testfixture.Model(), 32); e == nil {
		t.Fatal("fixture as learned")
	}
}

func TestStrictConfigDecode(t *testing.T) {
	var v struct {
		A int `json:"a"`
	}
	for _, s := range []string{`{"a":1,"typo":2}`, `{"a":1} {"a":2}`, `{"a":"x"}`} {
		if decodeStrict([]byte(s), &v) == nil {
			t.Fatal("accepted", s)
		}
	}
	if e := decodeStrict([]byte(`{"a":1}`), &v); e != nil || v.A != 1 {
		t.Fatal(e)
	}
}
