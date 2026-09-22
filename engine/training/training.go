// Package training is offline/experiment-side. Working inference does not import
// this package. Only Bernoulli likelihoods are fitted; graph/priors stay declared.
package training

import (
	"compress/gzip"
	"crypto/sha256"
	"dissertation.local/sppr-reconstruction/analysispipe"
	"dissertation.local/sppr-reconstruction/diagnostics"
	"dissertation.local/sppr-reconstruction/observation"
	"dissertation.local/sppr-reconstruction/settings"
	"dissertation.local/sppr-reconstruction/simenv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

const Version = "new-synthetic-diagnostic-fit-0.4.0"

type Setup struct {
	Version    string            `json:"version"`
	Notice     string            `json:"notice"`
	Template   diagnostics.Model `json:"declared_template"`
	Laplace    float64           `json:"laplace"`
	Training   []simenv.Config   `json:"training"`
	Validation []simenv.Config   `json:"development_validation_not_final_holdout"`
}

func DefaultSetup() Setup {
	s := Setup{Version: Version, Notice: "NEW synthetic reconstruction data. Not old 1650 runs; not company data. Validation is development validation, not a final research holdout.", Template: settings.DiagnosticTemplate(), Laplace: 1, Training: []simenv.Config{}, Validation: []simenv.Config{}}
	for i, label := range []string{"D", "S", "C", "DS"} {
		for j := 0; j < 50; j++ {
			c, _ := simenv.DefaultConfig("training-v04", label, uint64(i*50+j+1))
			s.Training = append(s.Training, c)
		}
		for j := 0; j < 20; j++ {
			c, _ := simenv.DefaultConfig("development-validation-v04", label, uint64(41001+i*20+j))
			s.Validation = append(s.Validation, c)
		}
	}
	return s
}
func (s Setup) Validate() error {
	if s.Version != Version || s.Notice == "" || s.Laplace <= 0 || !observation.Finite(s.Laplace) || len(s.Training) == 0 || len(s.Validation) == 0 {
		return fmt.Errorf("invalid training setup")
	}
	if e := s.Template.Validate(); e != nil {
		return e
	}
	if s.Template.BaseDemand != [2]float64{40, 25} || s.Template.BaseRaw != 90 || s.Template.BaseCapacity != 100 {
		return fmt.Errorf("this fit version requires the declared two-product baseline")
	}
	ids := map[string]bool{}
	seeds := map[uint64]bool{}
	for _, set := range [][]simenv.Config{s.Training, s.Validation} {
		for _, c := range set {
			if e := c.Validate(); e != nil {
				return e
			}
			if ids[c.EpisodeID] || seeds[c.Seed] {
				return fmt.Errorf("duplicate episode or overlapping training/validation seeds")
			}
			ids[c.EpisodeID] = true
			seeds[c.Seed] = true
			if c.Event.Label == "" || c.Parameters.BaseDemand != [2]float64(s.Template.BaseDemand) || c.BaseRaw != s.Template.BaseRaw || c.BaseCapacity != s.Template.BaseCapacity {
				return fmt.Errorf("training labels/baselines inconsistent")
			}
		}
	}
	return nil
}

type ClassCounts struct {
	Active          int    `json:"active_days"`
	Included        int    `json:"included_days"`
	RejectedQuality int    `json:"rejected_quality"`
	RejectedFeature int    `json:"rejected_feature_availability"`
	Ones            [3]int `json:"positive_features"`
}
type Audit struct {
	Source   simenv.Record         `json:"synthetic_source"`
	Quality  observation.Quality   `json:"quality"`
	Features []diagnostics.Feature `json:"features"`
	Included bool                  `json:"included_in_fit"`
	Reason   string                `json:"reason"`
}
type ValidationCounts struct {
	Active                 int      `json:"active_days"`
	Evaluated              int      `json:"eligible_diagnostic_days"`
	Top1                   int      `json:"correct_top1"`
	Top2                   int      `json:"correct_top2"`
	NoCandidates           int      `json:"no_candidates"`
	Review                 int      `json:"requires_expert"`
	CandidateContainsTruth int      `json:"candidate_contains_truth"`
	RankReciprocalSum      float64  `json:"reciprocal_rank_sum"`
	Accuracy               *float64 `json:"conditional_top1_accuracy"`
	Coverage               *float64 `json:"eligible_coverage_of_active_days"`
}
type ValidationAudit struct {
	Source   simenv.Record       `json:"synthetic_source"`
	Analysis analysispipe.Result `json:"analysis"`
	Eligible bool                `json:"eligible_for_conditional_accuracy"`
}
type Artifact struct {
	Version            string                       `json:"version"`
	Evidence           string                       `json:"evidence_class"`
	TrainingEpisodes   int                          `json:"training_episodes"`
	TrainingDays       int                          `json:"training_days"`
	ValidationEpisodes int                          `json:"validation_episodes"`
	ValidationDays     int                          `json:"validation_days"`
	RetainedExamples   int                          `json:"retained_training_examples"`
	TrainingCounts     map[string]*ClassCounts      `json:"training_counts"`
	ValidationCounts   map[string]*ValidationCounts `json:"development_validation_counts"`
	Model              diagnostics.Model            `json:"model"`
	Files              map[string]string            `json:"file_sha256"`
}

func Available(fs []diagnostics.Feature) bool {
	if len(fs) != 3 {
		return false
	}
	for _, f := range fs {
		if f.Value == nil {
			return false
		}
	}
	return true
}
func Extract(rec simenv.Record, m diagnostics.Model) (Audit, *diagnostics.TrainingExample, error) {
	a := Audit{Source: rec, Reason: "outside_active_interval"}
	q, e := observation.EvaluateQuality(rec.Packet, settings.Analysis(m).Monitor.Quality)
	if e != nil {
		return a, nil, e
	}
	a.Quality = q
	fs, e := diagnostics.ExtractFeatures(rec.Packet, m)
	if e != nil {
		return a, nil, e
	}
	a.Features = fs
	if !rec.Active {
		return a, nil, nil
	}
	if !q.GatePassed {
		a.Reason = "quality"
		return a, nil, nil
	}
	if !Available(fs) {
		a.Reason = "features"
		return a, nil, nil
	}
	a.Included = true
	a.Reason = "included_active_current_features"
	x := diagnostics.TrainingExample{ID: rec.ID, Label: rec.Label}
	for j, f := range fs {
		x.Features[j] = *f.Value
	}
	return a, &x, nil
}
func writeJSON(path string, x any) error {
	b, e := json.MarshalIndent(x, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func stream(path string, body func(*json.Encoder) error) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()
	g := gzip.NewWriter(f)
	defer func() {
		if e := g.Close(); err == nil {
			err = e
		}
	}()
	return body(json.NewEncoder(g))
}
func Sum(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	x := float64(n) / float64(d)
	return &x
}

func Run(s Setup, out string) (Artifact, error) {
	a := Artifact{Version: Version, Evidence: "synthetic_training_and_development_validation_not_effectiveness", TrainingEpisodes: len(s.Training), ValidationEpisodes: len(s.Validation), TrainingCounts: map[string]*ClassCounts{}, ValidationCounts: map[string]*ValidationCounts{}, Files: map[string]string{}}
	if e := s.Validate(); e != nil {
		return a, e
	}
	if entries, e := os.ReadDir(out); e == nil && len(entries) > 0 {
		return a, fmt.Errorf("output directory is not empty; choose a new path")
	} else if e != nil && !os.IsNotExist(e) {
		return a, e
	}
	if e := os.MkdirAll(out, 0755); e != nil {
		return a, e
	}
	if e := writeJSON(filepath.Join(out, "setup.json"), s); e != nil {
		return a, e
	}
	examples := []diagnostics.TrainingExample{}
	err := stream(filepath.Join(out, "training_audit.jsonl.gz"), func(enc *json.Encoder) error {
		for _, c := range s.Training {
			if a.TrainingCounts[c.Event.Label] == nil {
				a.TrainingCounts[c.Event.Label] = &ClassCounts{}
			}
			count := a.TrainingCounts[c.Event.Label]
			if e := simenv.RunFixed(c, func(rec simenv.Record) error {
				a.TrainingDays++
				audit, x, e := Extract(rec, s.Template)
				if e != nil {
					return e
				}
				if rec.Active {
					count.Active++
					if x != nil {
						count.Included++
						examples = append(examples, *x)
						for j, b := range x.Features {
							if b {
								count.Ones[j]++
							}
						}
					} else if audit.Reason == "quality" {
						count.RejectedQuality++
					} else {
						count.RejectedFeature++
					}
				}
				return enc.Encode(audit)
			}); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return a, err
	}
	sort.Slice(examples, func(i, j int) bool { return examples[i].ID < examples[j].ID })
	a.RetainedExamples = len(examples)
	// This is the ONLY fit call. No validation examples or statistics are passed.
	a.Model, err = diagnostics.FitBernoulli(s.Template, examples, s.Laplace, "new-synthetic-training-v04", "bernoulli-fit-v04")
	if err != nil {
		return a, err
	}
	a.Model.Provenance.Description = "NEW synthetic active-day observed Bernoulli frequencies, Laplace smoothing; priors and graph remain declared; no company data, no recovered old model"
	if e := writeJSON(filepath.Join(out, "retained_examples.json"), examples); e != nil {
		return a, e
	}
	if e := writeJSON(filepath.Join(out, "diagnostic_model.json"), a.Model); e != nil {
		return a, e
	}
	ac := settings.Analysis(a.Model)
	err = stream(filepath.Join(out, "validation_audit.jsonl.gz"), func(enc *json.Encoder) error {
		for _, c := range s.Validation {
			if a.ValidationCounts[c.Event.Label] == nil {
				a.ValidationCounts[c.Event.Label] = &ValidationCounts{}
			}
			count := a.ValidationCounts[c.Event.Label]
			pipe, e := analysispipe.New(ac, 0)
			if e != nil {
				return e
			}
			if e := simenv.RunFixed(c, func(rec simenv.Record) error {
				a.ValidationDays++
				result, e := pipe.Process(rec.Packet)
				if e != nil {
					return e
				}
				fs, e := diagnostics.ExtractFeatures(rec.Packet, a.Model)
				if e != nil {
					return e
				}
				eligible := rec.Active && result.Monitor.Computed && result.Monitor.Event != nil && *result.Monitor.Event && Available(fs)
				if rec.Active {
					count.Active++
				}
				if eligible {
					count.Evaluated++
					if result.Diagnosis.RequiresExpert {
						count.Review++
					}
					if len(result.Diagnosis.Candidates) == 0 {
						count.NoCandidates++
					}
					for _, id := range result.Diagnosis.Candidates {
						if id == rec.Label {
							count.CandidateContainsTruth++
						}
					}
					for i, r := range result.Diagnosis.Ranked {
						if r.ID == rec.Label {
							count.RankReciprocalSum += 1 / float64(i+1)
							if i == 0 {
								count.Top1++
							}
							if i < 2 {
								count.Top2++
							}
							break
						}
					}
				}
				return enc.Encode(ValidationAudit{rec, result, eligible})
			}); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return a, err
	}
	for _, c := range a.ValidationCounts {
		c.Accuracy = ratio(c.Top1, c.Evaluated)
		c.Coverage = ratio(c.Evaluated, c.Active)
	}
	for _, p := range []string{"setup.json", "training_audit.jsonl.gz", "retained_examples.json", "diagnostic_model.json", "validation_audit.jsonl.gz"} {
		a.Files[p], err = Sum(filepath.Join(out, p))
		if err != nil {
			return a, err
		}
	}
	err = writeJSON(filepath.Join(out, "summary.json"), a)
	return a, err
}

// Verify checks saved byte hashes, then independently reruns generation, fitting
// and development validation in a temporary directory, comparing all artifacts.
func Verify(dir string) error {
	var s Setup
	var a Artifact
	b, e := os.ReadFile(filepath.Join(dir, "setup.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	b, e = os.ReadFile(filepath.Join(dir, "summary.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &a); e != nil {
		return e
	}
	required := []string{"setup.json", "training_audit.jsonl.gz", "retained_examples.json", "diagnostic_model.json", "validation_audit.jsonl.gz"}
	if len(a.Files) != len(required) {
		return fmt.Errorf("incomplete artifact list")
	}
	for _, p := range required {
		h, e := Sum(filepath.Join(dir, p))
		if e != nil {
			return e
		}
		if h != a.Files[p] {
			return fmt.Errorf("hash mismatch: %s", p)
		}
	}
	temp, e := os.MkdirTemp("", "sppr-fit-verify-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	rerun, e := Run(s, temp)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(a, rerun) {
		return fmt.Errorf("recomputed training/validation artifact differs")
	}
	return nil
}
