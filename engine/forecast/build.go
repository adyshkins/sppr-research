package forecast

import (
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/observation"
)

type Result struct {
	Version        string           `json:"version"`
	Status         string           `json:"status"`
	Reason         string           `json:"reason"`
	RequiresExpert bool             `json:"requires_expert"`
	Admission      Admission        `json:"admission"`
	Ensemble       *Ensemble        `json:"ensemble"`
	Alternatives   []ActionForecast `json:"alternatives"`
}

func Build(p observation.Packet, k KnownPlan, d diagnostics.Result, c Config, seed uint64) (Result, error) {
	r := Result{Version: Version, RequiresExpert: d.RequiresExpert, Alternatives: []ActionForecast{}}
	a, err := Assess(p, k, c)
	if err != nil {
		return r, err
	}
	r.Admission = a
	if d.Status == "not_required" {
		r.Status = "not_required"
		r.Reason = "NO_DEVIATION"
		return r, nil
	}
	if !a.Passed {
		r.Status = "data_check"
		r.Reason = "FORECAST_INFORMATION_INSUFFICIENT"
		return r, nil
	}
	if !d.ForecastAllowed {
		r.Status = d.Status
		r.Reason = "DIAGNOSIS_DOES_NOT_ALLOW_FORECAST"
		return r, nil
	}
	e, err := Generate(*a.Estimate, d, c, seed)
	if err != nil {
		return r, err
	}
	as, err := Alternatives(*a.Estimate, c)
	if err != nil {
		return r, err
	}
	r.Alternatives, err = Evaluate(*a.Estimate, e, c, as)
	if err != nil {
		return r, err
	}
	r.Ensemble = &e
	r.Status = "ready"
	r.Reason = "MODEL_CONDITIONAL_PREDICTIONS_NOT_EXECUTION_PERMISSION"
	return r, nil
}
