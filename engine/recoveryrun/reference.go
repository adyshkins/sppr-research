package recoveryrun

import (
	"compress/gzip"
	"dissertation.local/sppr-reconstruction/closedrun"
	"dissertation.local/sppr-reconstruction/control"
	"dissertation.local/sppr-reconstruction/core"
	"dissertation.local/sppr-reconstruction/factorial"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func ReferenceFromEpisode(dir string, day int, out string, seed uint64, paths int) error {
	var cfg Config
	var done Completion
	if e := factorial.ReadJSON(filepath.Join(dir, "experiment_config.json"), &cfg); e != nil {
		return e
	}
	if e := factorial.ReadJSON(filepath.Join(dir, "COMPLETED.json"), &done); e != nil {
		return e
	}
	cfgHash, e := control.Hash(cfg)
	if e != nil || cfgHash != done.ConfigHash {
		return fmt.Errorf("source configuration mismatch")
	}
	if day < 0 || day >= done.Records {
		return fmt.Errorf("reference day outside episode")
	}
	cf, e := os.Open(filepath.Join(dir, "controller.jsonl.gz"))
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
	pf, e := os.Open(filepath.Join(dir, "physical.jsonl.gz"))
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
	var row Record
	var pr closedrun.PhysicalRecord
	previous := cfgHash
	for i := 0; i <= day; i++ {
		// JSON map decoding merges keys: reset the record before each read.
		// Otherwise empty hard_checks retain keys from a preceding ready case.
		row = Record{}
		pr = closedrun.PhysicalRecord{}
		if e = cd.Decode(&row); e != nil {
			return e
		}
		if e = pd.Decode(&pr); e != nil {
			return e
		}
		h, e := Hash(row)
		if e != nil || row.Hash != h || row.Base.Previous != previous || row.Base.Day != i || pr.Day != i {
			return fmt.Errorf("source chain mismatch")
		}
		previous = row.Hash
	}
	if !Equal(row.Base.Execution, pr.Execution) {
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
	if row.Base.ForecastStatus != "ready" || row.Base.Selection.Decision == nil || row.Base.Selection.Reason != "RISK_EMPTY" {
		if e = closedrun.Save(filepath.Join(tmp, "SKIPPED.json"), map[string]any{"day": day, "source_hash": row.Hash, "forecast_status": row.Base.ForecastStatus, "reason": row.Base.ForecastReason, "selection_reason": row.Base.Selection.Reason, "notice": "Preselected checkpoint not replaced by a more convenient state"}); e != nil {
			return e
		}
		return os.Rename(tmp, out)
	}
	spec := factorial.ReferenceSpec{
		Notice:           "DEV-R08 independent evaluator; conditioning differs from prediction",
		SourceRecordHash: row.Hash, ClosedDay: day, TrueStart: pr.Result.Next,
		Environment: cfg.Base.Environment, Actions: row.Base.Actions,
		Horizon: cfg.Base.Controller.Choice.Horizon, Discount: cfg.Base.Controller.Choice.Discount,
		Alpha: cfg.Base.Controller.Choice.Alpha, Paths: paths, Seed: seed,
	}
	if e = closedrun.Save(filepath.Join(tmp, "source_record.json"), row); e != nil {
		return e
	}
	r, e := factorial.Reference(spec)
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
	for _, a := range row.Base.Selection.Decision.Evaluations {
		pred[a.ID] = a.Risk
	}
	if e = closedrun.Save(filepath.Join(tmp, "summary.json"), map[string]any{"day": day, "source_hash": row.Hash, "notice": r.Notice, "paths": paths, "predictor_risk": pred, "environment_reference_risk": r.Risks, "two_half_sample_estimates": r.BatchRisks, "same_information_choices": row.Base.Shadows, "risk_limit": cfg.Base.Controller.Choice.RiskLimit, "forcing_hash": r.ForcingHash}); e != nil {
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
