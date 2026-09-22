// Package core implements a NEW reconstruction of the dissertation's finite
// scenario decision procedure. It is not the lost sppr_experiment source.
package core

import (
	"fmt"
	"math"
	"sort"
)

const Version = "reconstruction-0.1.0"
const WeightTolerance = 1e-12

func finite(x float64) bool      { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func nonnegative(x float64) bool { return finite(x) && x >= 0 }

// normalizedWeights accepts probabilities, not arbitrary scores. Only floating
// point roundoff within WeightTolerance is normalized; larger errors are rejected.
func normalizedWeights(p []float64) ([]float64, error) {
	if len(p) == 0 {
		return nil, fmt.Errorf("empty probability distribution")
	}
	var sum, correction float64
	for _, w := range p {
		if !nonnegative(w) {
			return nil, fmt.Errorf("invalid probability: %v", w)
		}
		y := w - correction
		t := sum + y
		correction = (t - sum) - y
		sum = t
	}
	if !finite(sum) || sum <= 0 || math.Abs(sum-1) > WeightTolerance {
		return nil, fmt.Errorf("probabilities must sum to 1, got %.17g", sum)
	}
	out := make([]float64, len(p))
	for i, w := range p {
		out[i] = w / sum
	}
	return out, nil
}

type Risk struct {
	Expected float64 `json:"expected"`
	VaR      float64 `json:"var"`
	CVaR     float64 `json:"cvar"`
}

// EvaluateRisk evaluates (3.65)-(3.67) on nonnegative scenario losses.
// The variational formula is evaluated at the discrete weighted quantile,
// accounting for fractional mass at the VaR boundary.
// This is NOT E[Z | Z >= VaR], which is incorrect at atoms in general.
func EvaluateRisk(losses, weights []float64, alpha float64) (Risk, error) {
	if !finite(alpha) || alpha <= 0 || alpha >= 1 {
		return Risk{}, fmt.Errorf("alpha must be strictly between 0 and 1")
	}
	if len(losses) != len(weights) {
		return Risk{}, fmt.Errorf("loss/weight size mismatch")
	}
	p, err := normalizedWeights(weights)
	if err != nil {
		return Risk{}, err
	}
	type atom struct{ z, p float64 }
	a := make([]atom, 0, len(losses))
	mean := 0.0
	for i, z := range losses {
		if !nonnegative(z) {
			return Risk{}, fmt.Errorf("invalid loss at %d", i)
		}
		if p[i] > 0 {
			a = append(a, atom{z, p[i]})
			mean += p[i] * z
		}
	}
	sort.Slice(a, func(i, j int) bool { return a[i].z < a[j].z })
	q := a[len(a)-1].z
	cumulative := 0.0
	for _, v := range a {
		cumulative += v.p
		if cumulative >= alpha {
			q = v.z
			break
		}
	}
	// VaR is a minimizer of (3.67). Evaluating at it avoids the rounded
	// weighted average of identical tail atoms drifting above their value.
	// In particular, a constant loss exactly equal to a limit stays equal.
	tail := 1 - alpha
	cvar := q
	for _, v := range a {
		if v.z > q {
			cvar += (v.p / tail) * (v.z - q)
		}
	}
	if !finite(mean) || !finite(cvar) {
		return Risk{}, fmt.Errorf("numerical failure in risk calculation")
	}
	return Risk{mean, q, cvar}, nil
}

// HorizonLoss implements (3.63). Costs belong here exactly once, not again
// in the objective. Period losses must exclude this one-off action cost.
func HorizonLoss(periodLosses []float64, gamma, actionCost float64) (float64, error) {
	if len(periodLosses) == 0 || !finite(gamma) || gamma <= 0 || gamma > 1 || !nonnegative(actionCost) {
		return 0, fmt.Errorf("invalid horizon, discount or action cost")
	}
	z := actionCost
	discount := 1.0
	for _, v := range periodLosses {
		if !nonnegative(v) {
			return 0, fmt.Errorf("period loss must be finite and nonnegative")
		}
		z += discount * v
		discount *= gamma
	}
	if !finite(z) {
		return 0, fmt.Errorf("horizon loss overflow")
	}
	return z, nil
}
