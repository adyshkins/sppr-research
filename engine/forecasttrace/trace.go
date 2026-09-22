// Package forecasttrace replays A3-A5 using saved observations/configuration.
// It imports NO environment, sensor, training or test fixture packages.
package forecasttrace

import (
	"compress/gzip"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/forecastpipe"
	"dissertation.local/sppr-reconstruction/monitor"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const Schema = "sppr.forecast-trace.v04"

type EpisodeSpec struct {
	ID       string `json:"id"`
	StartDay int    `json:"start_day"`
	Days     int    `json:"days"`
}
type Episode struct {
	Spec   EpisodeSpec          `json:"spec"`
	Inputs []forecastpipe.Input `json:"inputs"`
}
type Header struct {
	Type     string              `json:"type"`
	Schema   string              `json:"schema"`
	Notice   string              `json:"notice"`
	Config   forecastpipe.Config `json:"config"`
	Episodes []EpisodeSpec       `json:"episodes"`
	Hash     string              `json:"hash"`
}
type Payload struct {
	EpisodeID string              `json:"episode_id"`
	Ordinal   int                 `json:"ordinal"`
	Input     forecastpipe.Input  `json:"input"`
	Result    forecastpipe.Result `json:"result"`
	State     monitor.State       `json:"state"`
}
type Record struct {
	Type     string  `json:"type"`
	Previous string  `json:"previous_hash"`
	Payload  Payload `json:"payload"`
	Hash     string  `json:"hash"`
}
type Footer struct {
	Type     string `json:"type"`
	Records  int    `json:"records"`
	LastHash string `json:"last_hash"`
}
type Summary struct {
	Episodes              int            `json:"episodes"`
	Records               int            `json:"records"`
	Forecasts             int            `json:"forecasts"`
	ActionForecasts       int            `json:"action_forecasts"`
	ScenarioPaths         int            `json:"scenario_paths"`
	PredictedBalanceSteps int            `json:"predicted_balance_steps"`
	StatusCounts          map[string]int `json:"forecast_status_counts"`
	LastHash              string         `json:"last_hash"`
}

func Hash(x any) (string, error) {
	b, e := json.Marshal(x)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}
func headerHash(h Header) (string, error) { h.Hash = ""; return Hash(h) }
func recordHash(r Record) (string, error) { r.Hash = ""; return Hash(r) }
func (h Header) validate() error {
	if h.Type != "header" || h.Schema != Schema || h.Notice == "" || len(h.Episodes) == 0 || len(h.Episodes) > 100 {
		return fmt.Errorf("invalid trace header")
	}
	if e := h.Config.Validate(); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, x := range h.Episodes {
		if x.ID == "" || seen[x.ID] || x.StartDay < 0 || x.StartDay > 1000000 || x.Days < 1 || x.Days > 365 {
			return fmt.Errorf("invalid trace episode")
		}
		seen[x.ID] = true
	}
	want, e := headerHash(h)
	if e != nil {
		return e
	}
	if want != h.Hash {
		return fmt.Errorf("header hash mismatch")
	}
	return nil
}
func newSummary(n int) Summary { return Summary{Episodes: n, StatusCounts: map[string]int{}} }
func collect(s *Summary, r forecastpipe.Result) {
	s.Records++
	s.StatusCounts[r.Forecast.Status]++
	if r.Forecast.Ensemble != nil {
		s.Forecasts++
		s.ScenarioPaths += len(r.Forecast.Ensemble.Paths)
		s.ActionForecasts += len(r.Forecast.Alternatives)
		for _, a := range r.Forecast.Alternatives {
			for _, t := range a.Trajectories {
				s.PredictedBalanceSteps += len(t.KPI)
			}
		}
	}
}
func Write(path string, c forecastpipe.Config, episodes []Episode) (summary Summary, err error) {
	h := Header{Type: "header", Schema: Schema, Notice: "NEW synthetic engineering preview; no chosen or executed corrective actions and no claim of effectiveness", Config: forecastpipe.CloneConfig(c), Episodes: []EpisodeSpec{}}
	for _, e := range episodes {
		if len(e.Inputs) != e.Spec.Days {
			return summary, fmt.Errorf("incomplete input episode")
		}
		h.Episodes = append(h.Episodes, e.Spec)
	}
	h.Hash, err = headerHash(h)
	if err != nil {
		return summary, err
	}
	if err = h.validate(); err != nil {
		return summary, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return summary, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".forecast-partial-*")
	if err != nil {
		return summary, err
	}
	tmpname := tmp.Name()
	defer func() {
		tmp.Close()
		if err != nil {
			os.Remove(tmpname)
		}
	}()
	g := gzip.NewWriter(tmp)
	enc := json.NewEncoder(g)
	if err = enc.Encode(h); err != nil {
		g.Close()
		return summary, err
	}
	summary = newSummary(len(episodes))
	prev := h.Hash
	for _, ep := range episodes {
		p, e := forecastpipe.New(c, ep.Spec.StartDay)
		if e != nil {
			err = e
			g.Close()
			return summary, err
		}
		for i, input := range ep.Inputs {
			if input.Packet.Day != ep.Spec.StartDay+i {
				err = fmt.Errorf("nonsequential input")
				g.Close()
				return summary, err
			}
			out, e := p.Process(input)
			if e != nil {
				err = e
				g.Close()
				return summary, fmt.Errorf("%s day %d: %w", ep.Spec.ID, i, err)
			}
			r := Record{Type: "record", Previous: prev, Payload: Payload{ep.Spec.ID, i, input, out, p.Snapshot()}}
			r.Hash, err = recordHash(r)
			if err != nil {
				g.Close()
				return summary, err
			}
			if err = enc.Encode(r); err != nil {
				g.Close()
				return summary, err
			}
			prev = r.Hash
			collect(&summary, out)
		}
	}
	if err = enc.Encode(Footer{"footer", summary.Records, prev}); err != nil {
		g.Close()
		return summary, err
	}
	if err = g.Close(); err != nil {
		return summary, err
	}
	if err = tmp.Close(); err != nil {
		return summary, err
	}
	if err = os.Rename(tmpname, path); err != nil {
		return summary, err
	}
	summary.LastHash = prev
	return summary, nil
}
func Replay(path string) (Summary, error) {
	var s Summary
	f, e := os.Open(path)
	if e != nil {
		return s, e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return s, e
	}
	defer g.Close()
	dec := json.NewDecoder(io.LimitReader(g, 1<<30))
	dec.DisallowUnknownFields()
	var h Header
	if e = dec.Decode(&h); e != nil {
		return s, e
	}
	if e = h.validate(); e != nil {
		return s, e
	}
	prev := h.Hash
	s = newSummary(len(h.Episodes))
	for _, ep := range h.Episodes {
		p, e := forecastpipe.New(h.Config, ep.StartDay)
		if e != nil {
			return s, e
		}
		for i := 0; i < ep.Days; i++ {
			var r Record
			if e = dec.Decode(&r); e != nil {
				return s, e
			}
			if r.Type != "record" || r.Previous != prev || r.Payload.EpisodeID != ep.ID || r.Payload.Ordinal != i || r.Payload.Input.Packet.Day != ep.StartDay+i {
				return s, fmt.Errorf("sequence mismatch")
			}
			hs, e := recordHash(r)
			if e != nil || hs != r.Hash {
				return s, fmt.Errorf("record hash mismatch")
			}
			out, e := p.Process(r.Payload.Input)
			if e != nil {
				return s, e
			}
			computed := Payload{ep.ID, i, r.Payload.Input, out, p.Snapshot()}
			a, e := Hash(computed)
			if e != nil {
				return s, e
			}
			b, e := Hash(r.Payload)
			if e != nil {
				return s, e
			}
			if a != b {
				return s, fmt.Errorf("recomputed A3/A4/A5 mismatch at %s/%d", ep.ID, i)
			}
			prev = r.Hash
			collect(&s, out)
		}
	}
	var end Footer
	if e = dec.Decode(&end); e != nil {
		return s, e
	}
	if end.Type != "footer" || end.Records != s.Records || end.LastHash != prev {
		return s, fmt.Errorf("footer mismatch")
	}
	var extra any
	if e = dec.Decode(&extra); e != io.EOF {
		return s, fmt.Errorf("extra data or invalid compressed trailer: %v", e)
	}
	s.LastHash = prev
	return s, nil
}
