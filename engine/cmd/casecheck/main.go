package main

import (
	"bytes"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/casetrace"
	"dissertation.local/sppr-reconstruction/internal/testfixture"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if e := run(os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func decode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var x any
	if e := d.Decode(&x); e != io.EOF {
		return fmt.Errorf("trailing data")
	}
	return nil
}
func writeNew(path string, v any) error {
	raw, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	_, e = f.Write(append(raw, '\n'))
	closed := f.Close()
	if e != nil {
		return e
	}
	return closed
}
func run(args []string, out io.Writer) error {
	f := flag.NewFlagSet("casecheck", flag.ContinueOnError)
	makeFile := f.String("make-fixture", "", "new 24-record engineering fixture (NOT an effectiveness experiment)")
	mode := f.String("mode", analysispipe.EvidenceRequired, "fixture admission mode")
	in := f.String("in", "", "input fixture")
	dst := f.String("out", "", "new trace file")
	verify := f.String("verify", "", "saved trace")
	expected := f.String("expected-sha256", "", "external whole-file checksum for replay")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	count := 0
	for _, x := range []string{*makeFile, *in, *verify} {
		if x != "" {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("choose one mode")
	}
	if *expected != "" && *verify == "" {
		return fmt.Errorf("checksum only for replay")
	}
	modeSet := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "mode" {
			modeSet = true
		}
	})
	if modeSet && *makeFile == "" {
		return fmt.Errorf("-mode is only for fixture generation; saved config controls runs")
	}
	if *makeFile != "" {
		if *dst != "" {
			return fmt.Errorf("-out not for fixture generation")
		}
		c := testfixture.Config(*mode)
		if e := c.Validate(); e != nil {
			return e
		}
		s := casetrace.Suite{EvidenceClass: casetrace.Evidence, StartDay: 0, Config: c, Packets: testfixture.Packets()}
		if e := writeNew(*makeFile, s); e != nil {
			return e
		}
		fmt.Fprintln(out, "Created 24 synthetic technical inputs with declared fixture parameters.")
		return nil
	}
	if *verify != "" {
		if *dst != "" {
			return fmt.Errorf("-out not for replay")
		}
		raw, e := os.ReadFile(*verify)
		if e != nil {
			return e
		}
		if *expected != "" {
			h := sha256.Sum256(raw)
			if hex.EncodeToString(h[:]) != *expected {
				return fmt.Errorf("external checksum mismatch")
			}
		}
		var b casetrace.Bundle
		if e = decode(raw, &b); e != nil {
			return e
		}
		if e = casetrace.Verify(b); e != nil {
			return e
		}
		fmt.Fprintf(out, "Replayed %d analysis records; admission, ranking and monitor state match.\n", b.Header.Count)
		return nil
	}
	if *dst == "" {
		return fmt.Errorf("-out required")
	}
	raw, e := os.ReadFile(*in)
	if e != nil {
		return e
	}
	var s casetrace.Suite
	if e = decode(raw, &s); e != nil {
		return e
	}
	b, e := casetrace.Build(s)
	if e != nil {
		return e
	}
	if e = writeNew(*dst, b); e != nil {
		return e
	}
	fmt.Fprintf(out, "Calculated %d engineering records. No forecasting, optimization or action execution.\n", b.Header.Count)
	return nil
}
