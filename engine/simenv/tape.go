package simenv

import (
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/randomstream"
	"dissertation.local/sppr-reconstruction/sensor"
	"math"
)

// TapeDay is experiment-side input. Never pass it to a controller or replay.
type TapeDay struct {
	Day         int                                     `json:"day"`
	Active      bool                                    `json:"introduced_cause_active"`
	Forcing     plant.Exogenous                         `json:"forcing"`
	Distortions map[observation.Field]sensor.Distortion `json:"sensor_distortions"`
}

// Tape uses exactly the v0.4 stream domains and draw order. It is generated
// before feedback execution, so actions cannot consume or shift environment RNG.
func Tape(c Config) ([]TapeDay, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	env, e := randomstream.New(c.Seed, "environment/v04", c.Dataset)
	if e != nil {
		return nil, e
	}
	obs, e := randomstream.New(c.Seed, "observation/noise/v04", c.Dataset)
	if e != nil {
		return nil, e
	}
	missing, e := randomstream.New(c.Seed, "observation/missing/v04", c.Dataset)
	if e != nil {
		return nil, e
	}
	out := make([]TapeDay, c.Days)
	for day := 0; day < c.Days; day++ {
		active := c.Event.Label != "" && day >= c.Event.StartDay && day <= c.Event.EndDay
		dr, sr, cr := 1., 1., 1.
		if active {
			dr, sr, cr = c.Event.DemandRatio, c.Event.SupplyRatio, c.Event.CapacityRatio
		}
		draw := func(mu float64) float64 { return mu * math.Max(0, 1+c.EnvironmentSD*env.NormFloat64()) }
		w := plant.Exogenous{Demand: plant.Pair{draw(c.Parameters.BaseDemand[0] * dr), draw(c.Parameters.BaseDemand[1] * dr)}, Capacity: draw(c.BaseCapacity * cr), RawArrival: draw(c.BaseRaw * sr)}
		d := map[observation.Field]sensor.Distortion{}
		for _, f := range observation.Fields() {
			d[f] = sensor.Distortion{RelativeError: c.ObservationSD * obs.NormFloat64(), Missing: missing.Float64() < c.MissingProbability}
		}
		out[day] = TapeDay{day, active, w, d}
	}
	return out, nil
}
