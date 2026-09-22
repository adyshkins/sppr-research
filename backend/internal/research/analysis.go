package research

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
)

const Resamples = 20_000
const Confidence = 0.9875

type Contrast struct {
	Name             string    `json:"name"`
	Metric           string    `json:"metric"`
	Mean             float64   `json:"mean"`
	Lower            float64   `json:"lower"`
	Upper            float64   `json:"upper"`
	ContainsZero     bool      `json:"contains_zero"`
	BlockDifferences []float64 `json:"block_differences"`
}
type Analysis struct {
	Series           string     `json:"series"`
	Purpose          string     `json:"purpose"`
	Seed             int64      `json:"seed"`
	Resamples        int        `json:"resamples"`
	Confidence       float64    `json:"confidence"`
	FamilyConfidence float64    `json:"family_confidence"`
	BlockCount       int        `json:"block_count"`
	AnalysisGroups   []string   `json:"analysis_groups"`
	AnalyzerVersion  string     `json:"analyzer_version"`
	Runtime          string     `json:"runtime"`
	Contrasts        []Contrast `json:"contrasts"`
	Notice           string     `json:"notice"`
}

// Analyze is a NEW analyzer of imported summaries, not the recovered F10 analyzer.
// A single bootstrap index vector is shared across all four paired contrasts.
func Analyze(ctx context.Context, b Bundle, seed int64) (Analysis, error) {
	if _, err := ValidateBundle(b); err != nil {
		return Analysis{}, err
	}
	spec, _ := Specification(b.Series)
	data := map[string]Episode{}
	for _, e := range b.Episodes {
		data[fmt.Sprintf("%s/%d/%s", e.Group, e.Replication, e.Policy)] = e
	}
	names := []string{"EM0-H0", "A-S", "G-A", "G-A"}
	left := []string{"EM0", "A", "G", "G"}
	right := []string{"H0", "S", "A", "A"}
	out := Analysis{Series: b.Series, Purpose: b.Purpose, Seed: seed, Resamples: Resamples, Confidence: Confidence, FamilyConfidence: .95, BlockCount: 24, AnalysisGroups: spec.AnalysisGroups, AnalyzerVersion: "paired-bootstrap-v1", Runtime: runtime.Version(), Contrasts: make([]Contrast, 4), Notice: "Новый анализ импортированных сводок: равные веса четырёх групп, 24 парных блока, 20 000 перевыборок, интервалы 98,75% (Бонферрони для четырёх контрастов). Генератор math/rand, seed задаёт пользователь; квантиль — линейная интерполяция позиции (n−1)p. Совпадение с исходным анализатором статьи и подлинность импортированных сводок не подтверждены. Интервал с нулём не доказывает эквивалентность."}
	for c := range out.Contrasts {
		ct := &out.Contrasts[c]
		ct.Name = names[c]
		ct.Metric = "total_loss"
		if c == 3 {
			ct.Metric = "forecast_steps"
		}
		ct.BlockDifferences = make([]float64, spec.Replications)
		for r := 0; r < spec.Replications; r++ {
			var difference float64
			for _, g := range spec.AnalysisGroups {
				l := data[fmt.Sprintf("%s/%d/%s", g, r, left[c])]
				rr := data[fmt.Sprintf("%s/%d/%s", g, r, right[c])]
				d := *l.TotalLoss - *rr.TotalLoss
				if c == 3 {
					d = float64(*l.ForecastSteps) - float64(*rr.ForecastSteps)
				}
				difference += d / float64(len(spec.AnalysisGroups))
			}
			ct.BlockDifferences[r] = difference
			ct.Mean += difference / float64(spec.Replications)
		}
	}
	rng := rand.New(rand.NewSource(seed))
	samples := make([][]float64, 4)
	for c := range samples {
		samples[c] = make([]float64, Resamples)
	}
	for k := 0; k < Resamples; k++ {
		if k%100 == 0 {
			select {
			case <-ctx.Done():
				return Analysis{}, ctx.Err()
			default:
			}
		}
		for j := 0; j < spec.Replications; j++ {
			r := rng.Intn(spec.Replications)
			for c := range samples {
				samples[c][k] += out.Contrasts[c].BlockDifferences[r] / float64(spec.Replications)
			}
		}
	}
	alpha := (1 - Confidence) / 2
	for c := range samples {
		sort.Float64s(samples[c])
		ct := &out.Contrasts[c]
		ct.Lower = quantile(samples[c], alpha)
		ct.Upper = quantile(samples[c], 1-alpha)
		ct.ContainsZero = ct.Lower <= 0 && ct.Upper >= 0
	}
	return out, nil
}
func quantile(sorted []float64, p float64) float64 {
	pos := float64(len(sorted)-1) * p
	i := int(math.Floor(pos))
	if i >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}
	return sorted[i] + (pos-float64(i))*(sorted[i+1]-sorted[i])
}
