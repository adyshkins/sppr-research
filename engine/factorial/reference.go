package factorial

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/plant"
	"dissertation.local/sppr-reconstruction/randomstream"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

type ReferenceSpec struct {
	Notice           string         `json:"notice"`
	SourceRecordHash string         `json:"source_record_hash"`
	ClosedDay        int            `json:"closed_day"`
	TrueStart        plant.State    `json:"true_start_evaluator_only"`
	Environment      simenv.Config  `json:"environment_evaluator_only"`
	Actions          []plant.Action `json:"saved_actions_not_recomputed_from_truth"`
	Horizon          int            `json:"horizon"`
	Discount         float64        `json:"discount"`
	Alpha            float64        `json:"alpha"`
	Paths            int            `json:"paths"`
	Seed             uint64         `json:"independent_seed"`
}
type ReferenceResult struct {
	Notice      string                 `json:"notice"`
	Losses      map[string][]float64   `json:"horizon_losses"`
	Risks       map[string]core.Risk   `json:"environment_reference_risk"`
	BatchRisks  map[string][]core.Risk `json:"two_half_sample_estimates"`
	ForcingHash string                 `json:"continuation_forcing_hash"`
}

func (s ReferenceSpec) Validate() error {
	if s.Notice == "" || len(s.SourceRecordHash) != 64 || s.ClosedDay < 0 || s.ClosedDay >= s.Environment.Days || s.Horizon < 1 || s.Horizon > 60 || s.Paths < 2 || s.Paths > 65536 || s.Paths%2 != 0 {
		return fmt.Errorf("invalid reference bounds")
	}
	if e := s.Environment.Validate(); e != nil {
		return e
	}
	if math.IsNaN(s.Discount) || s.Discount <= 0 || s.Discount > 1 || math.IsNaN(s.Alpha) || s.Alpha <= 0 || s.Alpha >= 1 {
		return fmt.Errorf("invalid reference risk settings")
	}
	ids := map[string]bool{}
	if len(s.Actions) == 0 {
		return fmt.Errorf("missing actions")
	}
	for _, a := range s.Actions {
		if ids[a.ID] {
			return fmt.Errorf("duplicate action")
		}
		ids[a.ID] = true
		fixed, e := plant.FixedAction(a.ID, a.PermanentPlan)
		if e != nil {
			return e
		}
		if !equal(a, fixed) {
			return fmt.Errorf("noncanonical action")
		}
	}
	noop, _ := plant.FixedAction("u0", nil)
	_, e := plant.Step(s.TrueStart, plant.Exogenous{Demand: s.Environment.Parameters.BaseDemand, Capacity: s.Environment.BaseCapacity, RawArrival: s.Environment.BaseRaw}, noop, s.Environment.Parameters)
	return e
}

// Reference uses a fresh random namespace, true current state and the DECLARED
// event schedule on the evaluator side. It is not risk conditional on observations
// and is not a calibrated or industrial ground truth. No controller is called.
func Reference(s ReferenceSpec) (r ReferenceResult, err error) {
	r = ReferenceResult{Notice: "Independent MC under true starting state + declared event schedule. Different conditioning from predictor; no future receding-horizon reoptimization; no human or industrial data.", Losses: map[string][]float64{}, Risks: map[string]core.Risk{}, BatchRisks: map[string][]core.Risk{}}
	if err = s.Validate(); err != nil {
		return r, err
	}
	weights := make([]float64, s.Paths)
	halfWeights := make([]float64, s.Paths/2)
	for i := range weights {
		weights[i] = 1 / float64(s.Paths)
	}
	for i := range halfWeights {
		halfWeights[i] = 2 / float64(s.Paths)
	}
	for _, a := range s.Actions {
		r.Losses[a.ID] = make([]float64, s.Paths)
	}
	noop, _ := plant.FixedAction("u0", nil)
	allForcing := make([][]plant.Exogenous, s.Paths)
	for i := 0; i < s.Paths; i++ {
		rng, e := randomstream.New(s.Seed, "reference/environment/v06", strconv.Itoa(i))
		if e != nil {
			return r, e
		}
		path := make([]plant.Exogenous, s.Horizon)
		for h := 0; h < s.Horizon; h++ {
			day := s.ClosedDay + 1 + h
			ev := s.Environment.Event
			dr, sr, cr := 1., 1., 1.
			if ev.Label != "" && day >= ev.StartDay && day <= ev.EndDay {
				dr, sr, cr = ev.DemandRatio, ev.SupplyRatio, ev.CapacityRatio
			}
			draw := func(v float64) float64 { return v * math.Max(0, 1+s.Environment.EnvironmentSD*rng.NormFloat64()) }
			path[h] = plant.Exogenous{Demand: plant.Pair{draw(s.Environment.Parameters.BaseDemand[0] * dr), draw(s.Environment.Parameters.BaseDemand[1] * dr)}, Capacity: draw(s.Environment.BaseCapacity * cr), RawArrival: draw(s.Environment.BaseRaw * sr)}
		}
		allForcing[i] = path
		for _, a := range s.Actions {
			state := s.TrueStart
			period := make([]float64, s.Horizon)
			for h, w := range path {
				u := noop
				if h == 0 {
					u = a
				}
				next, e := plant.Step(state, w, u, s.Environment.Parameters)
				if e != nil {
					return r, e
				}
				period[h], e = plant.PeriodLoss(next.KPI)
				if e != nil {
					return r, e
				}
				state = next.Next
			}
			z, e := core.HorizonLoss(period, s.Discount, a.Cost)
			if e != nil {
				return r, e
			}
			r.Losses[a.ID][i] = z
		}
	}
	r.ForcingHash, err = control.Hash(allForcing)
	if err != nil {
		return r, err
	}
	for _, a := range s.Actions {
		zs := r.Losses[a.ID]
		v, e := core.EvaluateRisk(zs, weights, s.Alpha)
		if e != nil {
			return r, e
		}
		r.Risks[a.ID] = v
		for b := 0; b < 2; b++ {
			v, e := core.EvaluateRisk(zs[b*(s.Paths/2):(b+1)*(s.Paths/2)], halfWeights, s.Alpha)
			if e != nil {
				return r, e
			}
			r.BatchRisks[a.ID] = append(r.BatchRisks[a.ID], v)
		}
	}
	return r, nil
}
func ReferenceFromEpisode(dir string, day int, out string, seed uint64, paths int) error {
	var cfg closedrun.Config
	var done Completion
	if e := ReadJSON(filepath.Join(dir, "experiment_config.json"), &cfg); e != nil {
		return e
	}
	if e := ReadJSON(filepath.Join(dir, "COMPLETED.json"), &done); e != nil {
		return e
	}
	cfgHash, e := control.Hash(cfg)
	if e != nil || cfgHash != done.ConfigHash {
		return fmt.Errorf("source configuration mismatch")
	}
	if day < 0 || day >= done.Records {
		return fmt.Errorf("reference day outside episode")
	}
	cf, e := os.Open(filepath.Join(dir, "compact_controller.jsonl.gz"))
	if e != nil {
		return e
	}
	defer cf.Close()
	cg, e := gzip.NewReader(cf)
	if e != nil {
		return e
	}
	defer cg.Close()
	cd := json.NewDecoder(cg)
	pf, e := os.Open(filepath.Join(dir, "physical_evaluation.jsonl.gz"))
	if e != nil {
		return e
	}
	defer pf.Close()
	pg, e := gzip.NewReader(pf)
	if e != nil {
		return e
	}
	defer pg.Close()
	pd := json.NewDecoder(pg)
	var row Compact
	var pr closedrun.PhysicalRecord
	previous := cfgHash
	for i := 0; i <= day; i++ {
		// JSON map decoding merges keys: reset the record before each read.
		// Otherwise empty hard_checks retain keys from a preceding ready case.
		row = Compact{}
		pr = closedrun.PhysicalRecord{}
		if e = cd.Decode(&row); e != nil {
			return e
		}
		if e = pd.Decode(&pr); e != nil {
			return e
		}
		h, e := compactHash(row)
		if e != nil || row.Hash != h || row.Previous != previous || row.Day != i || pr.Day != i {
			return fmt.Errorf("source chain mismatch")
		}
		previous = row.Hash
	}
	if !equal(row.Execution, pr.Execution) {
		return fmt.Errorf("source channels disagree")
	}
	if _, e = os.Stat(out); !os.IsNotExist(e) {
		return fmt.Errorf("reference destination must not exist")
	}
	if e = os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		return e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(out), ".reference-partial-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	if row.ForecastStatus != "ready" || row.Selection.Decision == nil {
		if e = closedrun.Save(filepath.Join(tmp, "SKIPPED.json"), map[string]any{"day": day, "source_hash": row.Hash, "forecast_status": row.ForecastStatus, "reason": row.ForecastReason, "selection_reason": row.Selection.Reason, "notice": "Preselected checkpoint not replaced by a more convenient state"}); e != nil {
			return e
		}
		return os.Rename(tmp, out)
	}
	spec := ReferenceSpec{"DEV-F06 independent evaluator; conditioning differs from prediction", row.Hash, day, pr.Result.Next, cfg.Environment, row.Actions, cfg.Controller.Choice.Horizon, cfg.Controller.Choice.Discount, cfg.Controller.Choice.Alpha, paths, seed}
	r, e := Reference(spec)
	if e != nil {
		return e
	}
	if e = closedrun.Save(filepath.Join(tmp, "reference_spec.json"), spec); e != nil {
		return e
	}
	f, e := os.Create(filepath.Join(tmp, "reference_losses.json.gz"))
	if e != nil {
		return e
	}
	g := gzip.NewWriter(f)
	e = json.NewEncoder(g).Encode(r)
	if e != nil {
		g.Close()
		f.Close()
		return e
	}
	if e = g.Close(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	pred := map[string]*core.Risk{}
	for _, a := range row.Selection.Decision.Evaluations {
		pred[a.ID] = a.Risk
	}
	if e = closedrun.Save(filepath.Join(tmp, "summary.json"), map[string]any{"day": day, "source_hash": row.Hash, "notice": r.Notice, "paths": paths, "predictor_risk": pred, "environment_reference_risk": r.Risks, "two_half_sample_estimates": r.BatchRisks, "same_information_choices": row.Shadows, "risk_limit": cfg.Controller.Choice.RiskLimit, "forcing_hash": r.ForcingHash}); e != nil {
		return e
	}
	hash, e := control.Hash(r)
	if e != nil {
		return e
	}
	if e = closedrun.Save(filepath.Join(tmp, "COMPLETED.json"), map[string]any{"spec_hash": mustHash(spec), "result_hash": hash}); e != nil {
		return e
	}
	return os.Rename(tmp, out)
}
func mustHash(v any) string {
	h, e := control.Hash(v)
	if e != nil {
		panic(e)
	}
	return h
}
func VerifyReference(dir string) error {
	var spec ReferenceSpec
	if e := ReadJSON(filepath.Join(dir, "reference_spec.json"), &spec); e != nil {
		return e
	}
	r, e := Reference(spec)
	if e != nil {
		return e
	}
	f, e := os.Open(filepath.Join(dir, "reference_losses.json.gz"))
	if e != nil {
		return e
	}
	defer f.Close()
	g, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer g.Close()
	var old ReferenceResult
	d := json.NewDecoder(g)
	if e = d.Decode(&old); e != nil {
		return e
	}
	var x any
	if e = d.Decode(&x); e != io.EOF {
		return fmt.Errorf("extra reference payload")
	}
	if !equal(r, old) {
		return fmt.Errorf("regenerated reference differs")
	}
	return nil
}
