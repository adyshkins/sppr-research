package research

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"
)

type ProfileWindow struct {
	Window string  `json:"window"`
	Start  string  `json:"start"`
	End    string  `json:"end"`
	Count  int     `json:"count"`
	MeanA  float64 `json:"mean_a"`
	MeanB  float64 `json:"mean_b"`
}
type ProfileReport struct {
	SHA256               string          `json:"sha256"`
	ExpectedSHA256       string          `json:"expected_sha256"`
	Rows                 int             `json:"rows"`
	ValidStructure       bool            `json:"valid_structure"`
	ReferenceMatch       bool            `json:"reference_match"`
	EligibleForReference bool            `json:"eligible_for_reference"`
	Issues               []string        `json:"issues"`
	Windows              []ProfileWindow `json:"windows"`
	Notice               string          `json:"notice"`
}

func ValidateProfile(data []byte) ProfileReport {
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	report := ProfileReport{SHA256: hash, ExpectedSHA256: ProfileSHA256, ReferenceMatch: hash == ProfileSHA256, Issues: []string{}, Windows: []ProfileWindow{}, Notice: "Проверяются байты CSV и контракт внешнего входа. Пересчёт профиля из исходных сделок, промышленная валидация и проверка контроллера не выполняются."}
	// A BOM is accepted for parsing, but remains part of the original-byte hash.
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	header, err := reader.Read()
	if err != nil {
		report.Issues = append(report.Issues, "не удалось прочитать CSV-заголовок")
		return report
	}
	idx := map[string]int{}
	for i, h := range header {
		if _, ok := idx[h]; ok {
			report.Issues = append(report.Issues, "повтор столбца "+h)
			return report
		}
		idx[h] = i
	}
	required := []string{"date", "test_day", "window", "day_in_window", "A_material_code", "A_demand", "B_material_code", "B_demand"}
	for _, h := range required {
		if _, ok := idx[h]; !ok {
			report.Issues = append(report.Issues, "нет столбца "+h)
		}
	}
	if len(report.Issues) > 0 {
		return report
	}
	start := time.Date(2019, 12, 5, 0, 0, 0, 0, time.UTC)
	windows := make([]ProfileWindow, 4)
	for i := range windows {
		windows[i].Window = fmt.Sprintf("W%d", i+1)
	}
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			report.Issues = append(report.Issues, fmt.Sprintf("ошибка CSV в строке данных %d", report.Rows+1))
			return report
		}
		report.Rows++
		n := report.Rows
		if n > 240 {
			report.Issues = append(report.Issues, "более 240 строк")
			return report
		}
		get := func(k string) string { return row[idx[k]] }
		issue := func(msg string) {
			if len(report.Issues) < 20 {
				report.Issues = append(report.Issues, fmt.Sprintf("строка %d: %s", n, msg))
			}
		}
		expectedDate := start.AddDate(0, 0, n-1).Format("2006-01-02")
		if get("date") != expectedDate {
			issue("ожидается дата " + expectedDate)
		}
		w := (n - 1) / 60
		day := (n-1)%60 + 1
		if get("window") != windows[w].Window {
			issue("неверное окно")
		}
		if get("test_day") != strconv.Itoa(n) || get("day_in_window") != strconv.Itoa(day) {
			issue("неверная нумерация test_day/day_in_window")
		}
		if get("A_material_code") != "264" || get("B_material_code") != "486" {
			issue("ожидаются material_code 264 и 486")
		}
		vals := [2]float64{}
		for j, key := range []string{"A_demand", "B_demand"} {
			x, e := strconv.ParseFloat(get(key), 64)
			if e != nil || math.IsNaN(x) || math.IsInf(x, 0) || x < 0 || x > 1e12 {
				issue("некорректное значение " + key)
			} else {
				vals[j] = x
			}
		}
		win := &windows[w]
		win.Count++
		if win.Start == "" {
			win.Start = get("date")
		}
		win.End = get("date")
		win.MeanA += vals[0] / 60
		win.MeanB += vals[1] / 60
	}
	if report.Rows != 240 {
		report.Issues = append(report.Issues, fmt.Sprintf("ожидается 240 строк, получено %d", report.Rows))
	}
	report.ValidStructure = len(report.Issues) == 0
	// Do not show partial/invalid window means as valid scientific data.
	if report.ValidStructure {
		report.Windows = windows
	}
	report.EligibleForReference = report.ValidStructure && report.ReferenceMatch
	return report
}
