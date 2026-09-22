package main

import (
	"testing"
)

func TestRequiresModel(t *testing.T) {
	if e := run(nil); e == nil {
		t.Fatal("implicit model")
	}
}
func TestBadArgs(t *testing.T) {
	if e := run([]string{"-days", "1"}); e == nil {
		t.Fatal("invalid preview")
	}
	if e := run([]string{"-unknown"}); e == nil {
		t.Fatal("invalid flag")
	}
	if e := run([]string{"unexpected"}); e == nil {
		t.Fatal("positional")
	}
}
func TestMissingTrace(t *testing.T) {
	if e := run([]string{"-replay", "missing.gz"}); e == nil {
		t.Fatal("missing trace")
	}
}
