package main

import (
	"testing"
)

func TestBadArgs(t *testing.T) {
	if e := run([]string{"-unknown"}); e == nil {
		t.Fatal("invalid flag")
	}
	if e := run([]string{"extra"}); e == nil {
		t.Fatal("positional")
	}
	if e := run([]string{"-verify", "none", "-config", "none"}); e == nil {
		t.Fatal("combined")
	}
}
func TestMissingInput(t *testing.T) {
	if e := run([]string{"-config", "missing-input.json"}); e == nil {
		t.Fatal("missing setup accepted")
	}
	if e := run([]string{"-verify", "missing-dir"}); e == nil {
		t.Fatal("missing archive accepted")
	}
}
