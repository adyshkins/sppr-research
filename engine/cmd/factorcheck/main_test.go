package main

import (
	"path/filepath"
	"testing"
)

func TestRejectsAmbiguousOrAbsentMode(t *testing.T) {
	for _, args := range [][]string{{}, {"-job", "0", "-replay", "x"}, {"-job", "0"}, {"-job", "0", "extra"}} {
		if e := run(args); e == nil {
			t.Fatal("accepted bad CLI", args)
		}
	}
}
func TestCreatesPlanAndRejectsOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "plan.json")
	args := []string{"-init-model", "../../results/fit_v04/diagnostic_model.json", "-plan", p, "-repeats", "1", "-days", "32"}
	if e := run(args); e != nil {
		t.Fatal(e)
	}
	if e := run(args); e == nil {
		t.Fatal("overwrote plan")
	}
}
