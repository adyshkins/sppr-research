package monitor

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/observation"
	"fmt"
)

// Fork makes an independent sequential monitor. The v0.2 Observe method is
// unchanged; this supports atomic new pipeline steps without changing replay.
func (m *Monitor) Fork() *Monitor {
	c := m.config
	c.InitialValues = cloneValues(c.InitialValues)
	return &Monitor{config: c, state: m.Snapshot()}
}

// Block consumes a well-formed day's packet without claiming a KPI value or
// updating measurement history/statistics. Admission reason is external to the
// v0.2 additive quality rule. Malformed packets never consume a day.
func (m *Monitor) Block(p observation.Packet, reason string) (Result, error) {
	r := Result{Day: p.Day, Statistics: m.state.Statistics, Warnings: []string{}}
	if p.Day != m.state.NextDay {
		return r, fmt.Errorf("expected day %d, got %d", m.state.NextDay, p.Day)
	}
	if reason == "" {
		return r, fmt.Errorf("explicit admission reason required")
	}
	q, err := observation.EvaluateQuality(p, m.config.Quality)
	if err != nil {
		return r, err
	}
	r.Quality = q
	r.Status = core.DataIncident
	r.Reason = reason
	next := m.Snapshot()
	next.NextDay++
	b := false
	next.PreviousComputed = &b
	m.state = next
	return r, nil
}
