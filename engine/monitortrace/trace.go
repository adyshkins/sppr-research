// Package monitortrace replays MONITOR-ONLY sequential test fixtures.
// No environment, forecast or decision policy is executed in replay.
package monitortrace

import (
	"bytes"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const Schema = "sppr.monitor-trace.v2"
const Evidence = "synthetic_monitor_fixture_not_effectiveness_experiment"

type Suite struct {
	EvidenceClass string               `json:"evidence_class"`
	StartDay      int                  `json:"start_day"`
	Config        monitor.Config       `json:"config"`
	Packets       []observation.Packet `json:"packets"`
}
type Payload struct {
	Sequence     int                `json:"sequence"`
	PreviousHash string             `json:"previous_hash"`
	Input        observation.Packet `json:"input"`
	Result       monitor.Result     `json:"result"`
	After        monitor.State      `json:"after"`
}
type Record struct {
	Payload Payload `json:"payload"`
	Hash    string  `json:"sha256"`
}
type Bundle struct {
	Schema        string         `json:"schema"`
	EvidenceClass string         `json:"evidence_class"`
	CodeVersion   string         `json:"code_version"`
	StartDay      int            `json:"start_day"`
	Config        monitor.Config `json:"config"`
	Count         int            `json:"count"`
	Records       []Record       `json:"records"`
	FinalHash     string         `json:"final_hash"`
}

func digest(x any) (string, error) {
	b, e := json.Marshal(x)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func copyConfig(c monitor.Config) monitor.Config {
	v := map[observation.Field]float64{}
	for k, x := range c.InitialValues {
		v[k] = x
	}
	c.InitialValues = v
	return c
}
func Build(s Suite) (Bundle, error) {
	b := Bundle{Schema: Schema, EvidenceClass: Evidence, CodeVersion: monitor.Version, StartDay: s.StartDay, Config: copyConfig(s.Config), Records: []Record{}}
	if s.EvidenceClass != Evidence || len(s.Packets) == 0 {
		return b, fmt.Errorf("explicit nonempty monitor fixture required")
	}
	m, e := monitor.New(s.Config, s.StartDay)
	if e != nil {
		return b, e
	}
	previous := ""
	for i, p := range s.Packets {
		r, e := m.Observe(p)
		if e != nil {
			return b, fmt.Errorf("step %d: %w", i, e)
		}
		payload := Payload{i, previous, observation.Clone(p), r, m.Snapshot()}
		h, e := digest(payload)
		if e != nil {
			return b, e
		}
		b.Records = append(b.Records, Record{payload, h})
		previous = h
	}
	b.Count = len(b.Records)
	b.FinalHash = previous
	return b, nil
}

// Verify checks internal integrity and independently recalculates every step
// from the initial monitor. External SHA-256 is still needed to bind this bundle
// to a previously saved file. Neither kind of hash proves data authenticity.
func Verify(b Bundle) error {
	if b.Schema != Schema || b.EvidenceClass != Evidence || b.CodeVersion != monitor.Version || b.Count < 1 || len(b.Records) != b.Count {
		return fmt.Errorf("invalid trace metadata/count")
	}
	m, e := monitor.New(b.Config, b.StartDay)
	if e != nil {
		return e
	}
	previous := ""
	for i, r := range b.Records {
		if r.Payload.Sequence != i || r.Payload.PreviousHash != previous || r.Payload.Input.Day != b.StartDay+i {
			return fmt.Errorf("trace order/day mismatch %d", i)
		}
		h, e := digest(r.Payload)
		if e != nil {
			return e
		}
		if h != r.Hash {
			return fmt.Errorf("content hash mismatch %d", i)
		}
		got, e := m.Observe(r.Payload.Input)
		if e != nil {
			return e
		}
		actual, e := json.Marshal(struct {
			Result monitor.Result
			State  monitor.State
		}{got, m.Snapshot()})
		if e != nil {
			return e
		}
		stored, e := json.Marshal(struct {
			Result monitor.Result
			State  monitor.State
		}{r.Payload.Result, r.Payload.After})
		if e != nil {
			return e
		}
		if !bytes.Equal(actual, stored) {
			return fmt.Errorf("recalculation mismatch %d", i)
		}
		previous = r.Hash
	}
	if previous != b.FinalHash {
		return fmt.Errorf("final hash mismatch")
	}
	return nil
}
