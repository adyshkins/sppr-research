// Package protocol implements a NEW static-core replay format. It does not
// parse, replace, or validate the lost 180 daily records from the manuscript.
package protocol

import (
	"bytes"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/core"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const Schema = "sppr.static-core-check.v1"
const EvidenceClass = "unit_fixture_not_research_experiment"

type Suite struct {
	EvidenceClass string      `json:"evidence_class"`
	Cases         []core.Case `json:"cases"`
}
type Payload struct {
	Sequence     int             `json:"sequence"`
	PreviousHash string          `json:"previous_hash"`
	Input        core.Case       `json:"input"`
	Decisions    []core.Decision `json:"decisions"`
}
type Record struct {
	Payload Payload `json:"payload"`
	Hash    string  `json:"sha256"`
}
type Bundle struct {
	Schema        string   `json:"schema"`
	EvidenceClass string   `json:"evidence_class"`
	CodeVersion   string   `json:"code_version"`
	Count         int      `json:"count"`
	Records       []Record `json:"records"`
	FinalHash     string   `json:"final_hash"`
}

func hash(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func Build(s Suite) (Bundle, error) {
	b := Bundle{Schema: Schema, EvidenceClass: EvidenceClass, CodeVersion: core.Version, Records: []Record{}}
	if s.EvidenceClass != EvidenceClass || len(s.Cases) == 0 {
		return b, fmt.Errorf("explicit nonempty unit fixture suite required")
	}
	ids := map[string]bool{}
	previous := ""
	for i, c := range s.Cases {
		if ids[c.ID] {
			return b, fmt.Errorf("duplicate case ID")
		}
		ids[c.ID] = true
		ds, err := core.CompareFactors(c)
		if err != nil {
			return b, fmt.Errorf("case %s: %w", c.ID, err)
		}
		p := Payload{i, previous, c, ds}
		h, err := hash(p)
		if err != nil {
			return b, err
		}
		b.Records = append(b.Records, Record{p, h})
		previous = h
	}
	b.Count = len(b.Records)
	b.FinalHash = previous
	return b, nil
}

// Verify checks count, ordering, chained hashes, and recalculation of all static
// decisions. An attacker rewriting both data and hashes is NOT authenticated.
// An independently saved checksum/signature would be needed for provenance.
func Verify(b Bundle) error {
	if b.Schema != Schema || b.CodeVersion != core.Version || b.EvidenceClass != EvidenceClass {
		return fmt.Errorf("unsupported schema, version or evidence class")
	}
	if b.Count <= 0 || b.Count != len(b.Records) {
		return fmt.Errorf("record count mismatch")
	}
	ids := map[string]bool{}
	previous := ""
	for i, r := range b.Records {
		if r.Payload.Sequence != i || r.Payload.PreviousHash != previous {
			return fmt.Errorf("record order/chain mismatch at %d", i)
		}
		if ids[r.Payload.Input.ID] {
			return fmt.Errorf("duplicate case ID")
		}
		ids[r.Payload.Input.ID] = true
		h, err := hash(r.Payload)
		if err != nil {
			return err
		}
		if h != r.Hash {
			return fmt.Errorf("content hash mismatch at %d", i)
		}
		ds, err := core.CompareFactors(r.Payload.Input)
		if err != nil {
			return err
		}
		want, err := json.Marshal(ds)
		if err != nil {
			return err
		}
		got, err := json.Marshal(r.Payload.Decisions)
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			return fmt.Errorf("recalculation mismatch at %d", i)
		}
		previous = r.Hash
	}
	if b.FinalHash != previous {
		return fmt.Errorf("final hash mismatch")
	}
	return nil
}
