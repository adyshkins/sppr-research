package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }
func fixture(series string) Bundle {
	spec, _ := Specification(series)
	h := strings.Repeat("a", 64)
	b := Bundle{Schema: Schema, Purpose: "software_test", Series: series, Manifest: Manifest{EngineVersion: "SOFTWARE-TEST-ONLY", SourceSHA256: h, ConfigSHA256: h, InputSHA256: h, PlanSHA256: h, Notes: "Artificial engineering fixture, never article evidence."}}
	for gi, g := range spec.Groups {
		for r := 0; r < 24; r++ {
			for pi, p := range spec.Policies {
				loss := 10 + float64(gi) + float64(r)/100 + float64(pi)/10
				b.Episodes = append(b.Episodes, Episode{Group: g, Replication: r, Policy: p, HorizonDays: 60, EnvironmentSHA256: h, TotalLoss: f64(loss + .5), KPILoss: f64(loss), ActionCost: f64(.5), ForecastSteps: i64(100 + int64(pi))})
			}
		}
	}
	return b
}
func encoded(t *testing.T, b Bundle) []byte {
	t.Helper()
	x, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	return x
}
func csvFixture() []byte {
	var s strings.Builder
	s.WriteString("date,test_day,window,day_in_window,A_material_code,A_demand,B_material_code,B_demand\n")
	start := time.Date(2019, 12, 5, 0, 0, 0, 0, time.UTC)
	for d := 0; d < 240; d++ {
		fmt.Fprintf(&s, "%s,%d,W%d,%d,264,40,486,25\n", start.AddDate(0, 0, d).Format("2006-01-02"), d+1, d/60+1, d%60+1)
	}
	return []byte(s.String())
}
func TestCompleteDesigns(t *testing.T) {
	for _, series := range []string{"F10", "E11"} {
		t.Run(series, func(t *testing.T) {
			b, v, e := ParseBundle(encoded(t, fixture(series)))
			if e != nil {
				t.Fatal(e)
			}
			s, _ := Specification(series)
			if len(b.Episodes) != s.ExpectedEpisodes || v.EpisodeCount != s.ExpectedEpisodes || v.PairedBlocks != len(s.Groups)*24 {
				t.Fatal(v)
			}
			if v.Scope != "structure_and_summary_arithmetic" {
				t.Fatal(v)
			}
		})
	}
}
func TestRejectInvalidBundle(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Bundle)
	}{
		{"schema", func(b *Bundle) { b.Schema = "unknown" }}, {"purpose", func(b *Bundle) { b.Purpose = "certified" }}, {"series", func(b *Bundle) { b.Series = "E12" }},
		{"version", func(b *Bundle) { b.Manifest.EngineVersion = "" }}, {"hash", func(b *Bundle) { b.Manifest.ConfigSHA256 = "abc" }},
		{"missing_episode", func(b *Bundle) { b.Episodes = b.Episodes[1:] }}, {"duplicate_episode", func(b *Bundle) { b.Episodes[1] = b.Episodes[0] }},
		{"unknown_policy", func(b *Bundle) { b.Episodes[0].Policy = "base" }}, {"unknown_group", func(b *Bundle) { b.Episodes[0].Group = "S" }},
		{"bad_rep", func(b *Bundle) { b.Episodes[0].Replication = 24 }}, {"horizon", func(b *Bundle) { b.Episodes[0].HorizonDays = 59 }},
		{"pairing", func(b *Bundle) { b.Episodes[0].EnvironmentSHA256 = strings.Repeat("b", 64) }},
		{"missing_loss", func(b *Bundle) { b.Episodes[0].TotalLoss = nil }}, {"negative", func(b *Bundle) { b.Episodes[0].KPILoss = f64(-1) }},
		{"nan", func(b *Bundle) { b.Episodes[0].KPILoss = f64(math.NaN()) }}, {"infinity", func(b *Bundle) { b.Episodes[0].KPILoss = f64(math.Inf(1)) }},
		{"overflow", func(b *Bundle) { b.Episodes[0].KPILoss = f64(1e308) }}, {"arithmetic", func(b *Bundle) { b.Episodes[0].TotalLoss = f64(200) }},
		{"missing_steps", func(b *Bundle) { b.Episodes[0].ForecastSteps = nil }}, {"negative_steps", func(b *Bundle) { b.Episodes[0].ForecastSteps = i64(-1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := fixture("E11")
			test.change(&b)
			if _, e := ValidateBundle(b); e == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
}
func TestStrictJSON(t *testing.T) {
	valid := encoded(t, fixture("E11"))
	tests := map[string][]byte{
		"unknown":               bytes.Replace(valid, []byte(`"schema":`), []byte(`"extra":1,"schema":`), 1),
		"duplicate":             bytes.Replace(valid, []byte(`"series":"E11"`), []byte(`"series":"F10","series":"E11"`), 1),
		"trailing":              append(append([]byte{}, valid...), []byte(`{}`)...),
		"missing_rep_zero":      bytes.Replace(valid, []byte(`"replication":0,`), nil, 1),
		"null_rep_zero":         bytes.Replace(valid, []byte(`"replication":0`), []byte(`"replication":null`), 1),
		"unknown_episode_field": bytes.Replace(valid, []byte(`"horizon_days":60`), []byte(`"horizon_days":60,"sneaky":true`), 1),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, _, e := ParseBundle(data); e == nil {
				t.Fatal("invalid JSON accepted")
			}
		})
	}
	var value any
	if e := DecodeStrict([]byte(strings.Repeat("[", 40)+"0"+strings.Repeat("]", 40)), &value); e == nil {
		t.Fatal("deep JSON accepted")
	}
}
func TestProfile(t *testing.T) {
	data := csvFixture()
	r := ValidateProfile(data)
	if !r.ValidStructure || r.Rows != 240 || len(r.Windows) != 4 {
		t.Fatal(r)
	}
	if r.ReferenceMatch || r.EligibleForReference {
		t.Fatal("test profile falsely certified")
	}
	if r.Windows[0].MeanA < 39.999 || r.Windows[3].End != "2020-07-31" {
		t.Fatal(r)
	}
	tests := map[string][]byte{
		"missing_header":   []byte("date\n2019-12-05\n"),
		"duplicate_header": bytes.Replace(data, []byte("B_demand"), []byte("A_demand"), 1),
		"date_gap":         bytes.Replace(data, []byte("2019-12-06"), []byte("2019-12-07"), 1),
		"negative":         bytes.Replace(data, []byte(",264,40,"), []byte(",264,-40,"), 1),
		"nan":              bytes.Replace(data, []byte(",264,40,"), []byte(",264,NaN,"), 1),
		"inf":              bytes.Replace(data, []byte(",264,40,"), []byte(",264,+Inf,"), 1),
		"wrong_material":   bytes.Replace(data, []byte(",264,40,"), []byte(",777,40,"), 1),
		"wrong_window":     bytes.Replace(data, []byte(",W2,"), []byte(",W1,"), 1),
		"short":            data[:bytes.LastIndex(data[:len(data)-1], []byte("\n"))+1],
		"too_long":         append(append([]byte{}, data...), []byte("2020-08-01,241,W5,1,264,40,486,25\n")...),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			r := ValidateProfile(input)
			if r.ValidStructure || r.EligibleForReference || len(r.Issues) == 0 {
				t.Fatal(r)
			}
		})
	}
	bom := ValidateProfile(append([]byte{0xef, 0xbb, 0xbf}, data...))
	if !bom.ValidStructure || bom.SHA256 == r.SHA256 {
		t.Fatal(bom)
	}
}
func TestAnalysisDeterministicPaired(t *testing.T) {
	b := fixture("F10")
	a, e := Analyze(context.Background(), b, 42)
	if e != nil {
		t.Fatal(e)
	}
	second, e := Analyze(context.Background(), b, 42)
	if e != nil || !reflect.DeepEqual(a, second) {
		t.Fatal("not deterministic")
	}
	for i, j := 0, len(b.Episodes)-1; i < j; i, j = i+1, j-1 {
		b.Episodes[i], b.Episodes[j] = b.Episodes[j], b.Episodes[i]
	}
	shuffled, e := Analyze(context.Background(), b, 42)
	if e != nil || !reflect.DeepEqual(a, shuffled) {
		t.Fatal("depends on row order")
	}
	if a.BlockCount != 24 || len(a.AnalysisGroups) != 4 || a.AnalysisGroups[0] == "normal" || a.Resamples != 20000 {
		t.Fatal(a)
	}
	for i, c := range a.Contrasts {
		want := .1
		if i == 3 {
			want = 1
		}
		if math.Abs(c.Mean-want) > 1e-10 || c.ContainsZero {
			t.Fatal(c)
		}
	}
}
func TestNormalExcluded(t *testing.T) {
	b := fixture("F10")
	a, _ := Analyze(context.Background(), b, 1)
	for i := range b.Episodes {
		e := &b.Episodes[i]
		if e.Group == "normal" {
			e.KPILoss = f64(1e8)
			e.TotalLoss = f64(1e8 + .5)
		}
	}
	other, _ := Analyze(context.Background(), b, 1)
	if !reflect.DeepEqual(a, other) {
		t.Fatal("normal affected primary contrasts")
	}
}
func TestZeroContrastAndCancellation(t *testing.T) {
	b := fixture("E11")
	for i := range b.Episodes {
		b.Episodes[i].TotalLoss = f64(1)
		b.Episodes[i].KPILoss = f64(.5)
		b.Episodes[i].ForecastSteps = i64(100)
	}
	a, e := Analyze(context.Background(), b, 0)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range a.Contrasts {
		if c.Mean != 0 || c.Lower != 0 || c.Upper != 0 || !c.ContainsZero {
			t.Fatal(c)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := Analyze(ctx, b, 0); e == nil {
		t.Fatal("cancel ignored")
	}
	if quantile([]float64{0, 10}, .25) != 2.5 {
		t.Fatal("wrong interpolation")
	}
}
func TestStorePersistenceAndTamper(t *testing.T) {
	root := t.TempDir()
	s, e := NewStore(root)
	if e != nil {
		t.Fatal(e)
	}
	data := encoded(t, fixture("E11"))
	one, _, e := s.Put(data)
	if e != nil {
		t.Fatal(e)
	}
	two, _, e := s.Put(data)
	if e != nil || one.ID != two.ID {
		t.Fatal(e)
	}
	s2, _ := NewStore(root)
	list, e := s2.List()
	if e != nil || len(list) != 1 || list[0].Status != "imported_unverified" {
		t.Fatal(list, e)
	}
	restored, _, _, e := s2.Load(one.ID)
	if e != nil || !bytes.Equal(data, restored) {
		t.Fatal(e)
	}
	if _, _, _, e := s2.Load("../../etc/passwd"); e == nil {
		t.Fatal("path traversal")
	}
	if err := os.WriteFile(filepath.Join(root, one.ID+".json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, e := s2.Load(one.ID); e == nil {
		t.Fatal("tamper accepted")
	}
	if _, _, e := s2.Put(data); e == nil {
		t.Fatal("tamper overwritten")
	}
}
func TestStoreRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	if _, e := NewStore(link); e == nil {
		t.Fatal("symlink store accepted")
	}
	s, _ := NewStore(root)
	id := strings.Repeat("a", 64)
	if err := os.Symlink(filepath.Join(target, "secret"), filepath.Join(root, id+".json")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, e := s.Load(id); e == nil {
		t.Fatal("symlink file accepted")
	}
}
func TestAggregateOrderAndValues(t *testing.T) {
	b := fixture("E11")
	rows := AggregateBundle(b)
	if len(rows) != 28 || rows[0].Count != 24 || rows[0].Group != "W1" || math.Abs(rows[0].MeanLoss-10.615) > 1e-9 {
		t.Fatal(rows[0])
	}
}

func TestConcurrentImports(t *testing.T) {
	s, e := NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	data := encoded(t, fixture("E11"))
	done := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { _, _, err := s.Put(data); done <- err }()
	}
	for i := 0; i < 12; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	list, e := s.List()
	if e != nil || len(list) != 1 {
		t.Fatal(list, e)
	}
}
