// monitorcheck is a local test/replay CLI, not an effectiveness experiment.
package main

import (
	"bytes"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/monitortrace"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/sensor"
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
func strict(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var tail any
	if e := d.Decode(&tail); e != io.EOF {
		return fmt.Errorf("trailing data")
	}
	return nil
}
func writeNew(path string, v any) error {
	data, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	data = append(data, '\n')
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	_, e = f.Write(data)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("monitorcheck", flag.ContinueOnError)
	in := flags.String("in", "", "fixture JSON")
	dst := flags.String("out", "", "new trace JSON")
	verify := flags.String("verify", "", "existing trace JSON")
	expected := flags.String("expected-sha256", "", "independent whole-file SHA-256")
	demo := flags.String("make-fixture", "", "write NEW 40-day technical fixture")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	modes := 0
	if *demo != "" {
		modes++
	}
	if *verify != "" {
		modes++
	}
	if *in != "" {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("choose one mode: -make-fixture, -in/-out, or -verify")
	}
	if *expected != "" && *verify == "" {
		return fmt.Errorf("expected digest is only for replay")
	}
	if *demo != "" {
		if *dst != "" {
			return fmt.Errorf("-out not used with -make-fixture")
		}
		s, e := demoSuite()
		if e != nil {
			return e
		}
		if e = writeNew(*demo, s); e != nil {
			return e
		}
		fmt.Fprintln(out, "Created 40-day synthetic MONITOR-ONLY fixture; not a research experiment.")
		return nil
	}
	if *verify != "" {
		if *dst != "" {
			return fmt.Errorf("-out not used with -verify")
		}
		raw, e := os.ReadFile(*verify)
		if e != nil {
			return e
		}
		if *expected != "" {
			h := sha256.Sum256(raw)
			if hex.EncodeToString(h[:]) != *expected {
				return fmt.Errorf("external file checksum mismatch")
			}
		}
		var b monitortrace.Bundle
		if e = strict(raw, &b); e != nil {
			return e
		}
		if e = monitortrace.Verify(b); e != nil {
			return e
		}
		fmt.Fprintf(out, "Replayed %d monitor records; state and results match.\n", b.Count)
		return nil
	}
	if *dst == "" {
		return fmt.Errorf("-out required")
	}
	raw, e := os.ReadFile(*in)
	if e != nil {
		return e
	}
	var s monitortrace.Suite
	if e = strict(raw, &s); e != nil {
		return e
	}
	b, e := monitortrace.Build(s)
	if e != nil {
		return e
	}
	if e = writeNew(*dst, b); e != nil {
		return e
	}
	fmt.Fprintf(out, "Calculated %d monitor records. No forecast, optimization or action selection executed.\n", b.Count)
	return nil
}

// Fixed forcing and fault dates are test fixtures, NOT calibrated scenario data.
// u0 is prescribed throughout; this is not a closed-loop controller comparison.
func demoSuite() (monitortrace.Suite, error) {
	suite := monitortrace.Suite{EvidenceClass: monitortrace.Evidence, StartDay: 0, Config: monitor.DefaultConfig(), Packets: []observation.Packet{}}
	state := plant.DissertationInitialState()
	ch, e := sensor.New(0)
	if e != nil {
		return suite, e
	}
	for day := 0; day < 40; day++ {
		w := plant.Exogenous{Demand: plant.Pair{40, 25}, Capacity: 100, RawArrival: 90}
		if day >= 5 && day <= 8 {
			w.Capacity = 50
		}
		a, e := plant.FixedAction("u0", nil)
		if e != nil {
			return suite, e
		}
		r, e := plant.Step(state, w, a, plant.DissertationParameters())
		if e != nil {
			return suite, e
		}
		fresh, e := sensor.ClosedDay(day, state, w, a, r, "baseline-plan-1", "baseline-constraints-1")
		if e != nil {
			return suite, e
		}
		d := map[observation.Field]sensor.Distortion{}
		if day >= 12 && day <= 14 {
			for _, f := range observation.Fields() {
				d[f] = sensor.Distortion{Missing: true}
			}
		}
		if day == 17 {
			d[observation.RawEnd] = sensor.Distortion{Missing: true}
		}
		if day == 18 {
			d[observation.RawEnd] = sensor.Distortion{LagDays: 2}
		}
		if day == 20 {
			d[observation.Capacity] = sensor.Distortion{RelativeError: -1}
		}
		if day == 24 {
			d[observation.RawEnd] = sensor.Distortion{Fault: "non_finite"}
		}
		if day == 28 {
			for _, f := range observation.Fields() {
				d[f] = sensor.Distortion{LagDays: 1}
			}
		}
		p, e := ch.Transmit(fresh, d)
		if e != nil {
			return suite, e
		}
		suite.Packets = append(suite.Packets, p)
		state = r.Next
	}
	return suite, nil
}
