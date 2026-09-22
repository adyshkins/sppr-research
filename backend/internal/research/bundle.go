package research

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
)

var HashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Manifest struct {
	EngineVersion string `json:"engine_version"`
	SourceSHA256  string `json:"source_sha256"`
	ConfigSHA256  string `json:"config_sha256"`
	InputSHA256   string `json:"input_sha256"`
	PlanSHA256    string `json:"plan_sha256"`
	Notes         string `json:"notes,omitempty"`
}

type Episode struct {
	Group             string `json:"group"`
	Replication       int    `json:"replication"`
	Policy            string `json:"policy"`
	HorizonDays       int    `json:"horizon_days"`
	EnvironmentSHA256 string `json:"environment_sha256"`
	// Pointers distinguish a measured zero from an absent/null value.
	TotalLoss     *float64 `json:"total_loss"`
	KPILoss       *float64 `json:"kpi_loss"`
	ActionCost    *float64 `json:"action_cost"`
	ForecastSteps *int64   `json:"forecast_steps"`
}

type Bundle struct {
	Schema   string    `json:"schema"`
	Purpose  string    `json:"purpose"`
	Series   string    `json:"series"`
	Manifest Manifest  `json:"manifest"`
	Episodes []Episode `json:"episodes"`
}

type Validation struct {
	Scope               string   `json:"scope"`
	Checks              int      `json:"checks"`
	EpisodeCount        int      `json:"episode_count"`
	PairedBlocks        int      `json:"paired_blocks"`
	ReferenceInputMatch *bool    `json:"reference_input_match,omitempty"`
	Warnings            []string `json:"warnings"`
}

// DecodeStrict rejects duplicate keys, unknown fields and trailing documents.
func DecodeStrict(data []byte, target any) error {
	scan := json.NewDecoder(bytes.NewReader(data))
	scan.UseNumber()
	if err := uniqueValue(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return fmt.Errorf("ожидался один JSON-документ")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return fmt.Errorf("JSON: %w", err)
	}
	return nil
}
func uniqueValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("превышена глубина JSON")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok {
				return fmt.Errorf("некорректный ключ JSON")
			}
			if seen[s] {
				return fmt.Errorf("повторный ключ JSON %q", s)
			}
			seen[s] = true
			if err := uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("неожиданный разделитель JSON")
	}
	_, err = d.Token()
	return err
}

func ParseBundle(data []byte) (Bundle, Validation, error) {
	var b Bundle
	if err := DecodeStrict(data, &b); err != nil {
		return b, Validation{}, err
	}
	v, err := ValidateBundle(b)
	return b, v, err
}

func ValidateBundle(b Bundle) (Validation, error) {
	v := Validation{Scope: "structure_and_summary_arithmetic", Warnings: []string{
		"Хэши исходников, конфигурации, манифеста и внешней среды заявлены отправителем; их содержимое здесь не пересчитывается.",
		"Проверены только структура и согласованность сводок. Физические балансы, выбор действий и replay не проверены.",
		"Даже полный импорт не означает воспроизведение результатов статьи.",
	}}
	check := func(ok bool, msg string) error {
		v.Checks++
		if !ok {
			return fmt.Errorf("%s", msg)
		}
		return nil
	}
	if err := check(b.Schema == Schema, "неподдерживаемая schema"); err != nil {
		return v, err
	}
	if err := check(b.Purpose == "research_import" || b.Purpose == "software_test", "purpose: research_import или software_test"); err != nil {
		return v, err
	}
	spec, ok := Specification(b.Series)
	if err := check(ok, "series: F10 или E11"); err != nil {
		return v, err
	}
	if err := check(len(b.Manifest.EngineVersion) > 0 && len(b.Manifest.EngineVersion) <= 128, "engine_version обязателен, не более 128 символов"); err != nil {
		return v, err
	}
	hashes := []struct{ name, value string }{{"source", b.Manifest.SourceSHA256}, {"config", b.Manifest.ConfigSHA256}, {"input", b.Manifest.InputSHA256}, {"plan", b.Manifest.PlanSHA256}}
	for _, h := range hashes {
		if err := check(HashPattern.MatchString(h.value), h.name+"_sha256: нужны 64 строчные шестнадцатеричные цифры"); err != nil {
			return v, err
		}
	}
	if err := check(len(b.Manifest.Notes) <= 4096, "notes: не более 4096 байт"); err != nil {
		return v, err
	}
	if err := check(len(b.Episodes) == spec.ExpectedEpisodes, fmt.Sprintf("ожидается %d эпизодов, получено %d", spec.ExpectedEpisodes, len(b.Episodes))); err != nil {
		return v, err
	}
	groups := map[string]bool{}
	for _, g := range spec.Groups {
		groups[g] = true
	}
	ps := map[string]bool{}
	for _, p := range spec.Policies {
		ps[p] = true
	}
	keys := map[string]bool{}
	paired := map[string]string{}
	for i, e := range b.Episodes {
		prefix := fmt.Sprintf("эпизод %d: ", i)
		if err := check(groups[e.Group] && ps[e.Policy] && e.Replication >= 0 && e.Replication < spec.Replications, prefix+"неизвестная группа, политика или повтор"); err != nil {
			return v, err
		}
		if err := check(e.HorizonDays == spec.HorizonDays, prefix+"horizon_days должен быть 60"); err != nil {
			return v, err
		}
		key := fmt.Sprintf("%s/%d/%s", e.Group, e.Replication, e.Policy)
		if err := check(!keys[key], prefix+"повторный ключ "+key); err != nil {
			return v, err
		}
		keys[key] = true
		if err := check(HashPattern.MatchString(e.EnvironmentSHA256), prefix+"некорректный environment_sha256"); err != nil {
			return v, err
		}
		pair := fmt.Sprintf("%s/%d", e.Group, e.Replication)
		old, found := paired[pair]
		if err := check(!found || old == e.EnvironmentSHA256, prefix+"нарушена парность внешней ленты"); err != nil {
			return v, err
		}
		paired[pair] = e.EnvironmentSHA256
		for _, x := range []*float64{e.TotalLoss, e.KPILoss, e.ActionCost} {
			if err := check(x != nil && finiteNonnegative(*x), prefix+"total_loss, kpi_loss и action_cost обязательны, конечны, в пределах [0, 1e12]"); err != nil {
				return v, err
			}
		}
		sum := *e.KPILoss + *e.ActionCost
		if err := check(math.Abs(*e.TotalLoss-sum) <= 1e-9*math.Max(1, math.Abs(sum)), prefix+"total_loss не равен kpi_loss + action_cost"); err != nil {
			return v, err
		}
		if err := check(e.ForecastSteps != nil && *e.ForecastSteps >= 0 && *e.ForecastSteps <= 1_000_000_000_000, prefix+"forecast_steps обязателен, целый, в пределах [0, 1e12]"); err != nil {
			return v, err
		}
	}
	// Fixed cardinality plus unique legal tuples proves Cartesian completeness.
	v.EpisodeCount = len(b.Episodes)
	v.PairedBlocks = len(paired)
	if b.Series == "E11" {
		m := b.Manifest.InputSHA256 == ProfileSHA256
		v.ReferenceInputMatch = &m
		if !m {
			v.Warnings = append(v.Warnings, "Заявленный хэш спросового профиля не совпадает с E11 из статьи; это другой вход.")
		}
	}
	if b.Purpose == "software_test" {
		v.Warnings = append(v.Warnings, "ИНЖЕНЕРНЫЙ ТЕСТ: значения не являются результатами научного эксперимента.")
	}
	return v, nil
}
func finiteNonnegative(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0) && x >= 0 && x <= 1e12
}

type Aggregate struct {
	Group             string  `json:"group"`
	Policy            string  `json:"policy"`
	Count             int     `json:"count"`
	MeanLoss          float64 `json:"mean_loss"`
	MeanForecastSteps float64 `json:"mean_forecast_steps"`
}

func AggregateBundle(b Bundle) []Aggregate {
	spec, _ := Specification(b.Series)
	rows := append([]Episode(nil), b.Episodes...)
	sort.Slice(rows, func(i, j int) bool {
		a, c := rows[i], rows[j]
		if a.Group != c.Group {
			return a.Group < c.Group
		}
		if a.Policy != c.Policy {
			return a.Policy < c.Policy
		}
		return a.Replication < c.Replication
	})
	values := map[string]*Aggregate{}
	for _, r := range rows {
		key := r.Group + "/" + r.Policy
		v := values[key]
		if v == nil {
			v = &Aggregate{Group: r.Group, Policy: r.Policy}
			values[key] = v
		}
		v.Count++
		v.MeanLoss += *r.TotalLoss / float64(spec.Replications)
		v.MeanForecastSteps += float64(*r.ForecastSteps) / float64(spec.Replications)
	}
	out := make([]Aggregate, 0, len(values))
	for _, g := range spec.Groups {
		for _, p := range spec.Policies {
			out = append(out, *values[g+"/"+p])
		}
	}
	return out
}

// replication=0 is valid, but an omitted replication is not an observation.
func (e *Episode) UnmarshalJSON(data []byte) error {
	type wire Episode
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	raw, ok := fields["replication"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("replication обязателен")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode((*wire)(e))
}
