// Package forecast implements A5 from OBSERVATIONS and a registered plan.
// plant.Step is reused as a PURE balance model, initialized from an observation
// estimate. No simulator, sensor, training labels or future tape is accepted.
package forecast

import (
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/plant"
	"fmt"
	"math"
)

const Version = "scenario-forecast-0.4.0"
const GaussianNoise = "independent_gaussian_multiplier_clipped_at_zero_v1"
const JointRecovery = "joint_geometric_recovery_after_day_v1"

type Template struct {
	ID       string     `json:"id"`
	Active   [3]bool    `json:"active_demand_supply_capacity"`
	Fallback [3]float64 `json:"declared_fallback_ratios"`
}
type Config struct {
	FeatureRatios       [3]float64                `json:"feature_ratios"`
	Version             string                    `json:"version"`
	Parameters          plant.Parameters          `json:"balance_parameters"`
	BaseCapacity        float64                   `json:"base_capacity"`
	BaseRaw             float64                   `json:"base_raw"`
	Horizon             int                       `json:"horizon"`
	PathsPerHypothesis  int                       `json:"paths_per_hypothesis"`
	RecoveryProbability float64                   `json:"recovery_probability"`
	RelativeSD          float64                   `json:"relative_sd_before_clipping"`
	NoiseLaw            string                    `json:"noise_law"`
	RecoveryLaw         string                    `json:"recovery_law"`
	Templates           []Template                `json:"templates"`
	Discount            float64                   `json:"discount"`
	Alpha               float64                   `json:"alpha"`
	Quantiles           [2]float64                `json:"interval_quantiles"`
	Quality             observation.QualityConfig `json:"quality"`
	BalanceTolerance    float64                   `json:"observed_balance_relative_tolerance"`
}

func DefaultConfig() Config {
	return Config{FeatureRatios: [3]float64{1.1, .85, .85}, Version: Version, Parameters: plant.DissertationParameters(), BaseCapacity: 100, BaseRaw: 90, Horizon: 7, PathsPerHypothesis: 64, RecoveryProbability: .25, RelativeSD: .03, NoiseLaw: GaussianNoise, RecoveryLaw: JointRecovery,
		Templates: []Template{{"D", [3]bool{true, false, false}, [3]float64{1.35, 1, 1}}, {"S", [3]bool{false, true, false}, [3]float64{1, .30, 1}}, {"C", [3]bool{false, false, true}, [3]float64{1, 1, .50}}, {"DS", [3]bool{true, true, false}, [3]float64{1.25, .60, 1}}}, Discount: .97, Alpha: .95, Quantiles: [2]float64{.025, .975}, Quality: observation.DissertationQualityConfig(), BalanceTolerance: .20}
}
func (c Config) Validate() error {
	for _, x := range c.FeatureRatios {
		if !finite(x) || x <= 0 {
			return fmt.Errorf("invalid feature threshold")
		}
	}
	if c.Version != Version || c.Horizon < 1 || c.Horizon > 60 || c.PathsPerHypothesis < 1 || c.PathsPerHypothesis > 1024 {
		return fmt.Errorf("invalid forecast version or computational bounds")
	}
	if e := c.Parameters.Validate(); e != nil {
		return e
	}
	if e := c.Quality.Validate(); e != nil {
		return e
	}
	if !finite(c.BaseCapacity) || c.BaseCapacity <= 0 || !finite(c.BaseRaw) || c.BaseRaw <= 0 || !unit(c.RecoveryProbability) || !finite(c.RelativeSD) || c.RelativeSD < 0 || c.RelativeSD > 1 || c.NoiseLaw != GaussianNoise || c.RecoveryLaw != JointRecovery {
		return fmt.Errorf("invalid declared forecast distribution")
	}
	if !finite(c.Discount) || c.Discount <= 0 || c.Discount > 1 || !unit(c.Alpha) || c.Alpha == 0 || c.Alpha == 1 || !unit(c.Quantiles[0]) || !unit(c.Quantiles[1]) || c.Quantiles[0] >= c.Quantiles[1] || !unit(c.BalanceTolerance) {
		return fmt.Errorf("invalid summary parameters")
	}
	if c.Quality.BaseCapacity != c.BaseCapacity || c.Quality.CapacityPerUnit != [2]float64(c.Parameters.CapacityPerUnit) || c.Quality.RawDaysDenominator != c.Parameters.RawDaysDenominator {
		return fmt.Errorf("quality/balance parameter mismatch")
	}
	canonical := map[string][3]bool{"D": {true, false, false}, "S": {false, true, false}, "C": {false, false, true}, "DS": {true, true, false}}
	seen := map[string]bool{}
	if len(c.Templates) != 4 {
		return fmt.Errorf("four explicit hypothesis templates required")
	}
	for _, t := range c.Templates {
		want, ok := canonical[t.ID]
		if !ok || seen[t.ID] || t.Active != want {
			return fmt.Errorf("invalid/duplicate hypothesis template")
		}
		seen[t.ID] = true
		for j, x := range t.Fallback {
			if !finite(x) || x < 0 {
				return fmt.Errorf("invalid fallback")
			}
			if !t.Active[j] && x != 1 {
				return fmt.Errorf("inactive cause must have neutral fallback")
			}
		}
		if (t.Active[0] && t.Fallback[0] <= 1) || (t.Active[1] && t.Fallback[1] >= 1) || (t.Active[2] && t.Fallback[2] >= 1) {
			return fmt.Errorf("fallback direction inconsistent")
		}
	}
	return nil
}
func CloneConfig(c Config) Config {
	n := c
	n.Templates = append([]Template(nil), c.Templates...)
	return n
}
func finite(x float64) bool { return observation.Finite(x) }
func unit(x float64) bool   { return finite(x) && x >= 0 && x <= 1 }
func close(a, b float64) bool {
	return math.Abs(a-b) <= 1e-10*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}
func ranges() ([4]float64, [4]float64) { return [4]float64{.95, .95, .7, 3}, [4]float64{1, 1, .9, 7} }

// Risk uses the unchanged discrete formula from the already tested core.
func risk(z, p []float64, alpha float64) (core.Risk, error) { return core.EvaluateRisk(z, p, alpha) }
