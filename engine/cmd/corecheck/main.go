// corecheck runs deterministic fixture checks, not dissertation experiments.
package main

import (
	"bytes"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/protocol"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

const maxInputBytes = 16 << 20

func decode(path string, dst any) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxInputBytes)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return nil, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("extra content after JSON object")
	}
	return b, nil
}
func writeNew(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
func run() error {
	in := flag.String("in", "", "input JSON suite (unit fixtures)")
	out := flag.String("out", "", "new output file; existing files are refused")
	verify := flag.String("verify", "", "replay saved static-core bundle")
	expected := flag.String("expected-sha256", "", "optional independently saved whole-file checksum")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *verify != "" {
		if *in != "" || *out != "" {
			return fmt.Errorf("choose either verify or in/out")
		}
		var b protocol.Bundle
		data, err := decode(*verify, &b)
		if err != nil {
			return err
		}
		if *expected != "" {
			h := sha256.Sum256(data)
			if hex.EncodeToString(h[:]) != *expected {
				return fmt.Errorf("external checksum mismatch")
			}
		}
		if err := protocol.Verify(b); err != nil {
			return err
		}
		fmt.Printf("Verified %d static fixture cases (%d factor decisions); not a production or trajectory experiment.\n", b.Count, 4*b.Count)
		return nil
	}
	if *in == "" || *out == "" || *expected != "" {
		return fmt.Errorf("use -in <suite.json> -out <new.json>, or -verify <bundle.json>")
	}
	var s protocol.Suite
	if _, err := decode(*in, &s); err != nil {
		return err
	}
	b, err := protocol.Build(s)
	if err != nil {
		return err
	}
	if err := protocol.Verify(b); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeNew(*out, data); err != nil {
		return err
	}
	h := sha256.Sum256(data)
	fmt.Printf("Saved %d static fixture cases; SHA256=%s\n", b.Count, hex.EncodeToString(h[:]))
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "corecheck:", err)
		os.Exit(1)
	}
}
