package main

import (
	"bytes"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/monitortrace"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIEndToEndAndExternalHash(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "fixture.json")
	out := filepath.Join(dir, "trace.json")
	var log bytes.Buffer
	if e := run([]string{"-make-fixture", in}, &log); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"-in", in, "-out", out}, &log); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(out)
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if e = run([]string{"-verify", out, "-expected-sha256", hex.EncodeToString(h[:])}, &log); e != nil {
		t.Fatal(e)
	}
	if e = run([]string{"-verify", out, "-expected-sha256", "wrong"}, &log); e == nil {
		t.Fatal("wrong external digest accepted")
	}
	if e = run([]string{"-in", in, "-out", out}, &log); e == nil {
		t.Fatal("existing output overwritten")
	}
	if e = run([]string{"-make-fixture", in}, &log); e == nil {
		t.Fatal("fixture overwritten")
	}
}
func TestStrictJSONRejectsTrailingAndUnknown(t *testing.T) {
	for _, raw := range []string{`{"unknown":1}`, `{} {}`} {
		var s monitortrace.Suite
		if e := strict([]byte(raw), &s); e == nil {
			t.Fatal("unexpected JSON accepted")
		}
	}
}
func TestInvalidCLIArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"-out", "x"}, {"-in", "x"}, {"-make-fixture", "a", "-verify", "b"}, {"-verify", "x", "-out", "y"}} {
		var b bytes.Buffer
		if run(args, &b) == nil {
			t.Fatal(args)
		}
	}
}
func TestDemoContainsRealIncidentsAndNulls(t *testing.T) {
	s, e := demoSuite()
	if e != nil {
		t.Fatal(e)
	}
	b, e := monitortrace.Build(s)
	if e != nil {
		t.Fatal(e)
	}
	if len(b.Records) != 40 {
		t.Fatal("fixture length")
	}
	for _, day := range []int{12, 13, 14, 20} {
		r := b.Records[day].Payload.Result
		if r.KPI != nil || r.Event != nil || r.Computed {
			t.Fatalf("day %d: %+v", day, r)
		}
	}
	if !b.Records[15].Payload.Result.Resumed {
		t.Fatal("no resume")
	}
	raw, _ := json.Marshal(b)
	if !bytes.Contains(raw, []byte(`"kpi":null`)) {
		t.Fatal("no null KPI")
	}
}
