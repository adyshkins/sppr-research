// Package casetrace saves and replays the new admission/monitor/diagnostic
// pipeline only. It does not call the environment or regenerate future events.
package casetrace

import (
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/monitor"
	"dissertation.local/sppr-reconstruction/observation"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const Schema = "sppr.analysis-trace.v3"
const Evidence = "synthetic_analysis_fixture_not_effectiveness_experiment"

type Suite struct {
	EvidenceClass string               `json:"evidence_class"`
	StartDay      int                  `json:"start_day"`
	Config        analysispipe.Config  `json:"config"`
	Packets       []observation.Packet `json:"packets"`
}
type Header struct {
	Schema        string              `json:"schema"`
	Version       string              `json:"version"`
	EvidenceClass string              `json:"evidence_class"`
	StartDay      int                 `json:"start_day"`
	Count         int                 `json:"count"`
	Config        analysispipe.Config `json:"config"`
}
type Payload struct {
	Sequence     int                 `json:"sequence"`
	PreviousHash string              `json:"previous_hash"`
	Input        observation.Packet  `json:"input"`
	Result       analysispipe.Result `json:"result"`
	After        monitor.State       `json:"after"`
}
type Record struct {
	Payload Payload `json:"payload"`
	Hash    string  `json:"sha256"`
}
type Bundle struct {
	Header     Header   `json:"header"`
	HeaderHash string   `json:"header_hash"`
	Records    []Record `json:"records"`
	FinalHash  string   `json:"final_hash"`
}

func digest(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func Build(s Suite) (Bundle, error) {
	b := Bundle{Records: []Record{}}
	if s.EvidenceClass != Evidence || len(s.Packets) == 0 {
		return b, fmt.Errorf("explicit nonempty engineering fixture required")
	}
	p, e := analysispipe.New(s.Config, s.StartDay)
	if e != nil {
		return b, e
	}
	b.Header = Header{Schema, analysispipe.Version, Evidence, s.StartDay, len(s.Packets), analysispipe.CloneConfig(s.Config)}
	b.HeaderHash, e = digest(b.Header)
	if e != nil {
		return b, e
	}
	prev := b.HeaderHash
	for i, in := range s.Packets {
		r, e := p.Process(in)
		if e != nil {
			return b, fmt.Errorf("step %d: %w", i, e)
		}
		payload := Payload{i, prev, observation.Clone(in), r, p.Snapshot()}
		h, e := digest(payload)
		if e != nil {
			return b, e
		}
		b.Records = append(b.Records, Record{payload, h})
		prev = h
	}
	b.FinalHash = prev
	return b, nil
}
func Verify(b Bundle) error {
	h := b.Header
	if h.Schema != Schema || h.Version != analysispipe.Version || h.EvidenceClass != Evidence || h.Count < 1 || len(b.Records) != h.Count {
		return fmt.Errorf("invalid header or count")
	}
	dh, e := digest(h)
	if e != nil {
		return e
	}
	if dh != b.HeaderHash {
		return fmt.Errorf("header hash mismatch")
	}
	p, e := analysispipe.New(h.Config, h.StartDay)
	if e != nil {
		return e
	}
	prev := dh
	for i, r := range b.Records {
		if r.Payload.Sequence != i || r.Payload.PreviousHash != prev || r.Payload.Input.Day != h.StartDay+i {
			return fmt.Errorf("record order mismatch: %d", i)
		}
		stored, e := digest(r.Payload)
		if e != nil {
			return e
		}
		if stored != r.Hash {
			return fmt.Errorf("record digest mismatch: %d", i)
		}
		result, e := p.Process(r.Payload.Input)
		if e != nil {
			return e
		}
		actual, e := digest(Payload{i, prev, observation.Clone(r.Payload.Input), result, p.Snapshot()})
		if e != nil {
			return e
		}
		if actual != r.Hash {
			return fmt.Errorf("recalculation mismatch: %d", i)
		}
		prev = r.Hash
	}
	if prev != b.FinalHash {
		return fmt.Errorf("final hash mismatch")
	}
	return nil
}
