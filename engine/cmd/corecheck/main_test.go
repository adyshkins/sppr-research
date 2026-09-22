package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefusesOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "result.json")
	if e := writeNew(p, []byte("original")); e != nil {
		t.Fatal(e)
	}
	if e := writeNew(p, []byte("changed")); e == nil {
		t.Fatal("overwrote result")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != "original" {
		t.Fatal("original changed")
	}
}
func TestRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	for _, text := range []string{`{"known":1,"extra":2}`, `{"known":1} {"known":2}`, `{"known":1} garbage`} {
		p := filepath.Join(t.TempDir(), "in.json")
		if e := os.WriteFile(p, []byte(text), 0644); e != nil {
			t.Fatal(e)
		}
		var x struct {
			Known int `json:"known"`
		}
		if _, e := decode(p, &x); e == nil {
			t.Fatal("accepted malformed schema")
		}
	}
}
