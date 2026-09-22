// Package httpapi exposes the evidence workbench; no scientific engine is emulated.
package httpapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"

	"sppr-prototype/research/internal/r08"
	"sppr-prototype/research/internal/research"
)

//go:embed openapi.json
var openAPI []byte

type Server struct {
	store        *research.Store
	origins      map[string]bool
	analysisSlot chan struct{}
	engine       *r08.Manager
}

type Options struct {
	Engine    *r08.Manager
	StaticDir string
}

func New(store *research.Store, origins []string) http.Handler {
	return NewWithOptions(store, origins, Options{})
}
func NewWithOptions(store *research.Store, origins []string, options Options) http.Handler {
	s := &Server{store: store, origins: map[string]bool{}, analysisSlot: make(chan struct{}, 1), engine: options.Engine}
	for _, o := range origins {
		s.origins[o] = true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok", "service": "sppr-research", "version": research.Version, "engine_ready": false, "r08_ready": s.engine != nil})
	})
	mux.HandleFunc("GET /api/research/capabilities", func(w http.ResponseWriter, r *http.Request) {
		c := research.Capabilities()
		c["r08_ready"] = s.engine != nil
		c["can_run_r08"] = s.engine != nil
		c["can_replay_r08"] = s.engine != nil
		if s.engine != nil {
			c["engine_status"] = "r08_only"
		}
		writeJSON(w, 200, c)
	})
	mux.HandleFunc("GET /api/research/spec", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, research.Catalog()) })
	mux.HandleFunc("GET /api/research/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(openAPI)
	})
	mux.HandleFunc("POST /api/research/profile/validate", s.profile)
	mux.HandleFunc("GET /api/research/runs", s.list)
	mux.HandleFunc("POST /api/research/runs", s.importBundle)
	mux.HandleFunc("GET /api/research/runs/{id}", s.detail)
	mux.HandleFunc("GET /api/research/runs/{id}/download", s.download)
	mux.HandleFunc("POST /api/research/runs/{id}/analysis", s.analysis)
	// No environment variable can silently turn this into a fake successful run.
	blocked := func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 409, "ENGINE_NOT_CONNECTED", "Исходный расчётный стенд не подключён. Запуск и replay недоступны; импорт сводок не заменяет воспроизведение.", nil)
	}
	mux.HandleFunc("POST /api/research/experiments", blocked)
	mux.HandleFunc("POST /api/research/runs/{id}/replay", blocked)
	s.registerR08(mux)
	if options.StaticDir != "" {
		mux.Handle("GET /", http.FileServer(http.Dir(options.StaticDir)))
	}
	return s.security(mux)
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		// Reject DNS-rebinding names even when a browser omits Origin for a GET.
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			writeError(w, 403, "HOST_NOT_ALLOWED", "Сервис доступен только через localhost или loopback IP.", nil)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !s.origins[origin] {
				writeError(w, 403, "ORIGIN_NOT_ALLOWED", "Источник браузерного запроса не разрешён.", nil)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func readBody(w http.ResponseWriter, r *http.Request, limit int64, expected string) ([]byte, bool) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != expected {
		writeError(w, 415, "CONTENT_TYPE", "Ожидается Content-Type: "+expected, nil)
		return nil, false
	}
	body := http.MaxBytesReader(w, r.Body, limit)
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			writeError(w, 413, "PAYLOAD_TOO_LARGE", "Превышен размер файла.", nil)
		} else {
			writeError(w, 400, "READ_FAILED", "Не удалось прочитать запрос.", nil)
		}
		return nil, false
	}
	return data, true
}
func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	data, ok := readBody(w, r, 4<<20, "text/csv")
	if !ok {
		return
	}
	report := research.ValidateProfile(data)
	writeJSON(w, 200, report)
}
func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.List()
	if err != nil {
		writeError(w, 500, "STORAGE_ERROR", "Ошибка чтения или целостности хранилища.", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) importBundle(w http.ResponseWriter, r *http.Request) {
	data, ok := readBody(w, r, research.MaxBundleBytes, "application/json")
	if !ok {
		return
	}
	if _, _, err := research.ParseBundle(data); err != nil {
		writeError(w, 422, "INVALID_BUNDLE", err.Error(), nil)
		return
	}
	info, validation, err := s.store.Put(data)
	if errors.Is(err, research.ErrQuota) {
		writeError(w, 507, "STORAGE_QUOTA", err.Error(), nil)
		return
	}
	if err != nil {
		writeError(w, 500, "STORAGE_ERROR", "Набор не сохранён: ошибка хранилища.", nil)
		return
	}
	writeJSON(w, 201, map[string]any{"run": info, "validation": validation})
}
func (s *Server) load(w http.ResponseWriter, r *http.Request) ([]byte, research.Bundle, research.Validation, bool) {
	data, b, v, err := s.store.Load(r.PathValue("id"))
	if errors.Is(err, research.ErrNotFound) {
		writeError(w, 404, "NOT_FOUND", "Набор не найден.", nil)
		return nil, b, v, false
	}
	if err != nil {
		writeError(w, 500, "STORAGE_ERROR", "Нарушена целостность или недоступен набор.", nil)
		return nil, b, v, false
	}
	return data, b, v, true
}
func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	_, b, v, ok := s.load(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"id": r.PathValue("id"), "series": b.Series, "purpose": b.Purpose, "status": "imported_unverified", "manifest": b.Manifest, "validation": v, "aggregates": research.AggregateBundle(b)})
}
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	data, _, _, ok := s.load(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sppr-`+r.PathValue("id")+`.json"`)
	w.Write(data)
}
func (s *Server) analysis(w http.ResponseWriter, r *http.Request) {
	data, ok := readBody(w, r, 1024, "application/json")
	if !ok {
		return
	}
	var request struct {
		Seed *int64 `json:"seed"`
	}
	if err := research.DecodeStrict(data, &request); err != nil || request.Seed == nil {
		writeError(w, 422, "INVALID_SEED", "Задайте целочисленный seed; он относится к новому анализатору, а не к исходному F10.", nil)
		return
	}
	if *request.Seed < -9007199254740991 || *request.Seed > 9007199254740991 {
		writeError(w, 422, "INVALID_SEED", "seed должен точно представляться как целое число JavaScript.", nil)
		return
	}
	_, b, _, ok := s.load(w, r)
	if !ok {
		return
	}
	select {
	case s.analysisSlot <- struct{}{}:
		defer func() { <-s.analysisSlot }()
	default:
		writeError(w, 429, "ANALYSIS_BUSY", "Другой анализ ещё выполняется.", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	result, err := research.Analyze(ctx, b, *request.Seed)
	if err != nil {
		writeError(w, 408, "ANALYSIS_CANCELLED", "Анализ отменён или превысил время ожидания.", nil)
		return
	}
	writeJSON(w, 200, result)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "JSON encoding failed", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
	w.Write([]byte("\n"))
}
func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}
