package httpapi

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"sppr-prototype/research/internal/r08"
	"sppr-prototype/research/internal/research"
)

func (s *Server) registerR08(m *http.ServeMux) {
	ready := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if s.engine == nil {
				writeError(w, 503, "R08_NOT_CONNECTED", "Соберите recoverycheck и запустите API через tools/run.py.", nil)
				return
			}
			h(w, r)
		}
	}
	m.HandleFunc("GET /api/research/r08/jobs", ready(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"items": s.engine.List(), "series": "DEV-R08", "article_reproduction_verified": false})
	}))
	m.HandleFunc("POST /api/research/r08/experiments", ready(func(w http.ResponseWriter, r *http.Request) {
		data, ok := readBody(w, r, 2048, "application/json")
		if !ok {
			return
		}
		var wire struct {
			Profile string   `json:"profile"`
			Repeat  *int     `json:"repeat"`
			Arms    []string `json:"arms"`
		}
		if e := research.DecodeStrict(data, &wire); e != nil {
			writeError(w, 422, "INVALID_REQUEST", e.Error(), nil)
			return
		}
		if wire.Repeat == nil {
			writeError(w, 422, "INVALID_REQUEST", "Поле repeat обязательно и должно быть целым числом.", nil)
			return
		}
		req := r08.Request{Profile: wire.Profile, Repeat: *wire.Repeat, Arms: wire.Arms}
		if e := r08.Validate(req); e != nil {
			writeError(w, 422, "INVALID_REQUEST", e.Error(), nil)
			return
		}
		job, e := s.engine.Start(req)
		if e != nil {
			s.r08Error(w, e)
			return
		}
		writeJSON(w, 202, job)
	}))
	m.HandleFunc("GET /api/research/r08/jobs/{id}", ready(func(w http.ResponseWriter, r *http.Request) {
		j, e := s.engine.Get(r.PathValue("id"))
		if e != nil {
			s.r08Error(w, e)
			return
		}
		writeJSON(w, 200, j)
	}))
	m.HandleFunc("POST /api/research/r08/jobs/{id}/replay", ready(func(w http.ResponseWriter, r *http.Request) {
		if !emptyJSON(w, r) {
			return
		}
		j, e := s.engine.Replay(r.PathValue("id"))
		if e != nil {
			s.r08Error(w, e)
			return
		}
		writeJSON(w, 202, j)
	}))
	m.HandleFunc("POST /api/research/r08/jobs/{id}/cancel", ready(func(w http.ResponseWriter, r *http.Request) {
		if !emptyJSON(w, r) {
			return
		}
		if e := s.engine.Cancel(r.PathValue("id")); e != nil {
			s.r08Error(w, e)
			return
		}
		writeJSON(w, 202, map[string]string{"status": "cancellation_requested"})
	}))
	m.HandleFunc("GET /api/research/r08/jobs/{id}/episodes/{index}/files/{name}", ready(func(w http.ResponseWriter, r *http.Request) {
		index, e := strconv.Atoi(r.PathValue("index"))
		if e != nil {
			writeError(w, 404, "NOT_FOUND", "Файл не найден.", nil)
			return
		}
		path, e := s.engine.File(r.PathValue("id"), index, r.PathValue("name"))
		if e != nil {
			s.r08Error(w, e)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
		http.ServeFile(w, r, path)
	}))
}
func emptyJSON(w http.ResponseWriter, r *http.Request) bool {
	b, ok := readBody(w, r, 256, "application/json")
	if !ok {
		return false
	}
	var request *struct{}
	if e := research.DecodeStrict(b, &request); e != nil || request == nil {
		writeError(w, 422, "INVALID_REQUEST", "Ожидается пустой JSON-объект {}.", nil)
		return false
	}
	return true
}
func (s *Server) r08Error(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, r08.ErrBusy):
		writeError(w, 429, "R08_BUSY", "Другой запуск или replay ещё выполняется.", nil)
	case errors.Is(e, r08.ErrNotFound):
		writeError(w, 404, "NOT_FOUND", "Запуск или файл не найден.", nil)
	case errors.Is(e, r08.ErrQuota):
		writeError(w, 507, "R08_QUOTA", "Лимит локального хранилища: 64 запуска или 512 МиБ. Сохраните данные вне хранилища и перезапустите API.", nil)
	case errors.Is(e, r08.ErrConflict):
		writeError(w, 409, "R08_CONFLICT", "Операция недоступна в текущем состоянии или версия исполняемого ядра изменилась.", nil)
	default:
		writeError(w, 500, "R08_ERROR", e.Error(), nil)
	}
}
