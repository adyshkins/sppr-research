// Package simenv is an EXPERIMENT-SIDE synthetic generator for the reconstruction.
// Labels, event windows, state and future forcing must stay outside inference.
package simenv

import (
	"dissertation.local/sppr-reconstruction/forecast"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/randomstream"
	"dissertation.local/sppr-reconstruction/sensor"
	"fmt"
	"math"
	"strconv"
)

const Version = "synthetic-fixed-environment-0.4.0"

type Event struct {
	Label         string  `json:"label"`
	StartDay      int     `json:"start_day_zero_based"`
	EndDay        int     `json:"end_day_zero_based"`
	DemandRatio   float64 `json:"demand_ratio"`
	SupplyRatio   float64 `json:"supply_ratio"`
	CapacityRatio float64 `json:"capacity_ratio"`
}

// Source table 4.3: day 20 is the twentieth day => index 19, not index 20.
func EventFor(label string) (Event, error) {
	switch label {
	case "":
		return Event{"", 0, 0, 1, 1, 1}, nil
	case "D":
		return Event{"D", 19, 24, 1.35, 1, 1}, nil
	case "S":
		return Event{"S", 19, 23, 1, .30, 1}, nil
	case "C":
		return Event{"C", 19, 21, 1, 1, .50}, nil
	case "DS":
		return Event{"DS", 19, 24, 1.25, .60, 1}, nil
	default:
		return Event{}, fmt.Errorf("unknown declared synthetic label")
	}
}

type Config struct {
	Version            string           `json:"version"`
	Dataset            string           `json:"dataset"`
	EpisodeID          string           `json:"episode_id"`
	Seed               uint64           `json:"seed"`
	Days               int              `json:"days"`
	Event              Event            `json:"event"`
	Initial            plant.State      `json:"initial_state"`
	Parameters         plant.Parameters `json:"parameters"`
	BaseCapacity       float64          `json:"base_capacity"`
	BaseRaw            float64          `json:"base_raw"`
	EnvironmentSD      float64          `json:"environment_relative_sd_before_clipping"`
	ObservationSD      float64          `json:"observation_relative_sd"`
	MissingProbability float64          `json:"missing_probability_per_channel"`
	NoiseLaw           string           `json:"noise_law"`
}

func DefaultConfig(dataset, label string, seed uint64) (Config, error) {
	e, err := EventFor(label)
	if err != nil {
		return Config{}, err
	}
	return Config{Version: Version, Dataset: dataset, EpisodeID: fmt.Sprintf("%s-%s-%d", dataset, label, seed), Seed: seed, Days: 60, Event: e, Initial: plant.DissertationInitialState(), Parameters: plant.DissertationParameters(), BaseCapacity: 100, BaseRaw: 90, EnvironmentSD: .03, ObservationSD: .05, MissingProbability: .05, NoiseLaw: "environment_clipped_gaussian_observation_unclipped_gaussian_independent_v1"}, nil
}
func (c Config) Validate() error {
	if c.Version != Version || c.Dataset == "" || c.EpisodeID == "" || c.Days < 1 || c.Days > 365 || c.NoiseLaw != "environment_clipped_gaussian_observation_unclipped_gaussian_independent_v1" {
		return fmt.Errorf("invalid environment identity/bounds")
	}
	if err := c.Parameters.Validate(); err != nil {
		return err
	}
	for _, v := range []float64{c.BaseCapacity, c.BaseRaw} {
		if !observation.Finite(v) || v <= 0 {
			return fmt.Errorf("invalid exogenous baseline")
		}
	}
	for _, v := range []float64{c.EnvironmentSD, c.ObservationSD, c.MissingProbability} {
		if !observation.Finite(v) || v < 0 || v > 1 {
			return fmt.Errorf("invalid noise parameter")
		}
	}
	if _, err := EventFor(c.Event.Label); err != nil {
		return err
	}
	if c.Event.StartDay < 0 || c.Event.EndDay < c.Event.StartDay || c.Event.EndDay >= c.Days {
		return fmt.Errorf("invalid event interval")
	}
	for _, v := range []float64{c.Event.DemandRatio, c.Event.SupplyRatio, c.Event.CapacityRatio} {
		if !observation.FiniteNonnegative(v) {
			return fmt.Errorf("invalid event size")
		}
	}
	a, _ := plant.FixedAction("u0", nil)
	_, err := plant.Step(c.Initial, plant.Exogenous{Demand: c.Parameters.BaseDemand, Capacity: c.BaseCapacity, RawArrival: c.BaseRaw}, a, c.Parameters)
	return err
}

type Record struct {
	ID          string             `json:"id"`
	EpisodeID   string             `json:"episode_id"`
	Day         int                `json:"day_zero_based"`
	CalendarDay int                `json:"calendar_day_one_based"`
	Active      bool               `json:"introduced_cause_active"`
	Label       string             `json:"experiment_label_not_inference_input"`
	Packet      observation.Packet `json:"observation"`
	Plan        forecast.KnownPlan `json:"known_plan"`
	TrueKPI     [4]float64         `json:"true_kpi_evaluation_only"`
}

// RunFixed executes a no-correction trajectory. The callback receives an
// experiment record; only Packet/Plan may be forwarded into working inference.
func RunFixed(c Config, emit func(Record) error) error {
	if e := c.Validate(); e != nil {
		return e
	}
	if emit == nil {
		return fmt.Errorf("record sink required")
	}
	env, e := randomstream.New(c.Seed, "environment/v04", c.Dataset)
	if e != nil {
		return e
	}
	obs, e := randomstream.New(c.Seed, "observation/noise/v04", c.Dataset)
	if e != nil {
		return e
	}
	missing, e := randomstream.New(c.Seed, "observation/missing/v04", c.Dataset)
	if e != nil {
		return e
	}
	channel, e := sensor.New(0)
	if e != nil {
		return e
	}
	s := c.Initial
	a, _ := plant.FixedAction("u0", nil)
	for day := 0; day < c.Days; day++ {
		active := c.Event.Label != "" && day >= c.Event.StartDay && day <= c.Event.EndDay
		dr, sr, cr := 1.0, 1.0, 1.0
		if active {
			dr = c.Event.DemandRatio
			sr = c.Event.SupplyRatio
			cr = c.Event.CapacityRatio
		}
		draw := func(mu float64) float64 { return mu * math.Max(0, 1+c.EnvironmentSD*env.NormFloat64()) }
		w := plant.Exogenous{Demand: plant.Pair{draw(c.Parameters.BaseDemand[0] * dr), draw(c.Parameters.BaseDemand[1] * dr)}, Capacity: draw(c.BaseCapacity * cr), RawArrival: draw(c.BaseRaw * sr)}
		result, e := plant.Step(s, w, a, c.Parameters)
		if e != nil {
			return e
		}
		p, e := sensor.ClosedDay(day, s, w, a, result, "registered-initial-plan-v04", "balance-parameters-v04")
		if e != nil {
			return e
		}
		dist := map[observation.Field]sensor.Distortion{}
		// Draw for every field on every day, even if missing, so changing dropout
		// cannot shift the noise stream and alter otherwise matched comparisons.
		for _, f := range observation.Fields() {
			dist[f] = sensor.Distortion{RelativeError: c.ObservationSD * obs.NormFloat64(), Missing: missing.Float64() < c.MissingProbability}
		}
		delivered, e := channel.Transmit(p, dist)
		if e != nil {
			return e
		}
		rec := Record{ID: c.EpisodeID + "/" + strconv.Itoa(day), EpisodeID: c.EpisodeID, Day: day, CalendarDay: day + 1, Active: active, Label: c.Event.Label, Packet: delivered, Plan: forecast.KnownPlan{Base: [2]float64(c.Initial.Plan), Version: p.Context.PlanVersion, ConstraintVersion: p.Context.ConstraintVersion, SnapshotDay: day}, TrueKPI: result.KPI}
		if e := emit(rec); e != nil {
			return e
		}
		s = result.Next
	}
	return nil
}
