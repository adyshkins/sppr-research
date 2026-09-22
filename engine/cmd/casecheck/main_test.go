package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestCLICompleteRoundtripAndNoOverwrite(t *testing.T) {
	d := t.TempDir()
	in := filepath.Join(d, "in.json")
	out := filepath.Join(d, "out.json")
	var log bytes.Buffer
	for _, args := range [][]string{{"-make-fixture", in}, {"-in", in, "-out", out}, {"-verify", out}} {
		if e := run(args, &log); e != nil {
			t.Fatal(e)
		}
	}
	raw, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if e = run([]string{"-verify", out, "-expected-sha256", hex.EncodeToString(h[:])}, &log); e != nil {
		t.Fatal(e)
	}
	if run([]string{"-verify", out, "-expected-sha256", "wrong"}, &log) == nil {
		t.Fatal("checksum ignored")
	}
	if run([]string{"-make-fixture", in}, &log) == nil || run([]string{"-in", in, "-out", out}, &log) == nil {
		t.Fatal("overwrote evidence")
	}
}
func TestCLIBadArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"-mode", "bad", "-make-fixture", filepath.Join(t.TempDir(), "x.json")}, {"-in", "a"}, {"-verify", "a", "-out", "b"}, {"-in", "a", "-mode", "aggregate_only_v0_2"}, {"-make-fixture", "a", "-out", "b"}, {"-in", "a", "-expected-sha256", "x"}, {"-verify", "a", "-in", "b"}, {"-unknown"}, {"position"}} {
		var out bytes.Buffer
		if run(args, &out) == nil {
			t.Fatal(args)
		}
	}
}
func TestStrictJSONRejectsUnknownAndTrailing(t *testing.T) {
	var dst struct {
		Value int `json:"value"`
	}
	for _, raw := range []string{"{\"unknown\":1}", "{\"value\":1} {}", "{\"value\":1} trailing", "{", "null {}"} {
		if decode([]byte(raw), &dst) == nil {
			t.Fatal(raw)
		}
	}
}
func TestCLIInvalidInput(t *testing.T) {
	d := t.TempDir()
	in := filepath.Join(d, "bad.json")
	if e := os.WriteFile(in, []byte("{}"), 0644); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if run([]string{"-in", in, "-out", filepath.Join(d, "out.json")}, &out) == nil {
		t.Fatal("empty config accepted")
	}
	if run([]string{"-verify", in}, &out) == nil {
		t.Fatal("invalid trace accepted")
	}
}
