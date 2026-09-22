// Package research validates imported evidence. It does not simulate the plant.
package research

const Version = "0.3.0-dev"
const Schema = "sppr-results-v1"
const ProfileSHA256 = "7aed99d67955589d4c7fcbaf5e03bbb3a32dd5242ef9857f97b7f4a403735db3"
const ArticleSHA256 = "9ca63ca64a1d07467d57fd9e2eaedb1725158c4b4b52a5de366cabf548fd70c0"

var policies = []string{"B00", "H0", "EM0", "F", "S", "A", "G"}

type SeriesSpec struct {
	ID               string   `json:"id"`
	Groups           []string `json:"groups"`
	AnalysisGroups   []string `json:"analysis_groups"`
	Policies         []string `json:"policies"`
	Replications     int      `json:"replications"`
	HorizonDays      int      `json:"horizon_days"`
	ExpectedEpisodes int      `json:"expected_episodes"`
}

func Specification(id string) (SeriesSpec, bool) {
	s := SeriesSpec{ID: id, Policies: append([]string(nil), policies...), Replications: 24, HorizonDays: 60}
	switch id {
	case "F10":
		s.Groups = []string{"normal", "S", "C", "D", "DS"}
		s.AnalysisGroups = []string{"S", "C", "D", "DS"}
		s.ExpectedEpisodes = 840
	case "E11":
		s.Groups = []string{"W1", "W2", "W3", "W4"}
		s.AnalysisGroups = append([]string(nil), s.Groups...)
		s.ExpectedEpisodes = 672
	default:
		return SeriesSpec{}, false
	}
	return s, true
}

type PolicySpec struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	DelayDays   int    `json:"delay_days"`
	Description string `json:"description"`
}

func Catalog() any {
	f, _ := Specification("F10")
	e, _ := Specification("E11")
	return struct {
		Version      string       `json:"version"`
		Source       string       `json:"source"`
		SourceSHA256 string       `json:"source_sha256"`
		Series       []SeriesSpec `json:"series"`
		Policies     []PolicySpec `json:"policies"`
		Notes        []string     `json:"notes"`
	}{Version, "Статья «Сценарно-имитационная оценка…», редакция после E11, разделы 2.1–2.4", ArticleSHA256,
		[]SeriesSpec{f, e}, []PolicySpec{
			{"B00", "Контроль по ожиданию", 0, "Выбор по математическому ожиданию без риск-фильтра."},
			{"H0", "Продолжение плана", 0, "При RISK_EMPTY новая команда не выдаётся; продолжается прежний план."},
			{"EM0", "Исключительное предложение", 0, "При RISK_EMPTY допускается явно маркированное исключительное действие; оно не становится риск-допустимым."},
			{"F", "Сохранённое действие", 2, "После общих проверок сохраняется рассчитанное действие без нового прогноза."},
			{"S", "Проверка кандидата", 2, "Повторно оценивается только сохранённый вектор действия."},
			{"A", "Полный новый выбор", 2, "После ожидания выполняется новый поиск по библиотеке."},
			{"G", "Условный пересчёт", 2, "Полный пересчёт при заданных триггерах изменения основания; иначе как F."},
		}, []string{
			"Это описание дизайна из статьи, а не подтверждение наличия расчётного ядра.",
			"Функция потерь нормирована; результаты не выражают денежный эффект предприятия.",
			"E11 заменяет только спросовой вход и не является промышленным внедрением.",
			"Обозначение S используется и для профиля снабжения, и для политики; поля group и policy разделены.",
		}}
}

func Capabilities() map[string]any {
	return map[string]any{
		"version": Version, "engine_ready": false, "engine_status": "source_missing",
		"can_import": true, "can_validate_profile": true, "can_analyze_imports": true,
		"can_run_experiments": false, "can_replay": false, "article_reproduction_verified": false,
		"blockers": []string{
			"Не подключены исходники автономного расчётного стенда F10/E11.",
			"Нет исходного FINAL10-манифеста, пространств случайных потоков и seeds для регрессионной проверки.",
			"Не загружены первичные журналы и исходные анализаторы для сверки с числами статьи.",
		},
		"notice": "Импорт, проверка структуры и анализ сводок не заменяют запуск модели, replay контроллера или аудит физических балансов.",
	}
}
