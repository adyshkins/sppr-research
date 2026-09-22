// Package sensor is an ENVIRONMENT-SIDE adapter, not a controller dependency.
// Distortions are explicit deterministic inputs; no old noise distribution is
// claimed recovered. Random draws can be supplied by a later versioned generator.
package sensor

import (
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
)

// ClosedDay publishes only specified measurements after execution. True KPI,
// cause labels, scenario names and future perturbations are never serialized.
func ClosedDay(day int, before plant.State, w plant.Exogenous, a plant.Action, r plant.Result, planVersion, constraintVersion string) (observation.Packet, error) {
	ctx := observation.Context{ExecutedPlan: [2]float64(r.ExecutedPlan), Overtime: a.Overtime, ActionID: a.ID, PlanVersion: planVersion, ConstraintVersion: constraintVersion}
	p := observation.Packet{Schema: observation.Schema, Day: day, Context: ctx, Readings: map[observation.Field]observation.Measurement{}}
	values := [15]float64{w.Demand[0], w.Demand[1], w.Capacity, w.RawArrival, r.Output[0], r.Output[1], r.Shipped[0], r.Shipped[1], r.Next.Raw, r.Next.Finished[0], r.Next.Finished[1], before.Backlog[0], before.Backlog[1], r.Next.Backlog[0], r.Next.Backlog[1]}
	for i, f := range observation.Fields() {
		p.Readings[f] = observation.Read(values[i], 0)
	}
	return p, p.Validate()
}

type Distortion struct {
	Missing       bool    `json:"missing"`
	LagDays       int     `json:"lag_days"`
	RelativeError float64 `json:"relative_error"`
	AdditiveError float64 `json:"additive_error"`
	Fault         string  `json:"fault,omitempty"`
}
type Channel struct {
	startDay int
	nextDay  int
	history  []observation.Packet
}

func New(startDay int) (*Channel, error) {
	if startDay < 0 {
		return nil, fmt.Errorf("invalid starting day")
	}
	return &Channel{startDay: startDay, nextDay: startDay, history: []observation.Packet{}}, nil
}

// Noise is applied at sampling time. Delivery lag retrieves THAT earlier noisy
// reading, without resampling it. Missing means lost at delivery, not nonexistent
// in the environment's sampling history. Returned readings are deep copies.
func (c *Channel) Transmit(fresh observation.Packet, dist map[observation.Field]Distortion) (observation.Packet, error) {
	if err := fresh.Validate(); err != nil {
		return observation.Packet{}, err
	}
	if fresh.Day != c.nextDay {
		return observation.Packet{}, fmt.Errorf("nonsequential sensor day")
	}
	fields := map[observation.Field]bool{}
	for _, f := range observation.Fields() {
		fields[f] = true
		if !fresh.Readings[f].Valid() || fresh.Readings[f].AgeDays != 0 {
			return observation.Packet{}, fmt.Errorf("fresh environment reading required")
		}
	}
	for f, d := range dist {
		if !fields[f] || d.LagDays < 0 || !observation.Finite(d.RelativeError) || !observation.Finite(d.AdditiveError) {
			return observation.Packet{}, fmt.Errorf("invalid distortion")
		}
		if d.Fault != "" && d.Fault != "non_finite" && d.Fault != "sensor_error" {
			return observation.Packet{}, fmt.Errorf("unknown fault")
		}
	}
	sampled := observation.Clone(fresh)
	for _, f := range observation.Fields() {
		d := dist[f]
		v := *fresh.Readings[f].Value*(1+d.RelativeError) + d.AdditiveError
		sampled.Readings[f] = observation.Read(v, 0)
		if d.Fault != "" {
			sampled.Readings[f] = observation.Measurement{Present: true, Fault: d.Fault}
		}
	}
	delivered := observation.Clone(sampled)
	for _, f := range observation.Fields() {
		d := dist[f]
		index := fresh.Day - d.LagDays - c.startDay
		if d.Missing || index < 0 {
			delivered.Readings[f] = observation.Missing()
			continue
		}
		if d.LagDays > 0 {
			old := observation.Clone(c.history[index])
			m := old.Readings[f]
			m.AgeDays = d.LagDays
			delivered.Readings[f] = m
		}
	}
	if err := delivered.Validate(); err != nil {
		return observation.Packet{}, err
	}
	c.history = append(c.history, sampled)
	c.nextDay++
	return delivered, nil
}
