package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sppr-prototype/research/internal/research"
)

func newTest(t *testing.T) http.Handler {
	t.Helper()
	s, e := research.NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return New(s, []string{"http://localhost:5173"})
}
func call(h http.Handler, method, path, media string, data []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:18081"+path, bytes.NewReader(data))
	if media != "" {
		r.Header.Set("Content-Type", media)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func httpFixture() []byte {
	h := strings.Repeat("c", 64)
	episodes := []map[string]any{}
	for w := 1; w <= 4; w++ {
		for r := 0; r < 24; r++ {
			for _, p := range []string{"B00", "H0", "EM0", "F", "S", "A", "G"} {
				episodes = append(episodes, map[string]any{"group": fmt.Sprintf("W%d", w), "replication": r, "policy": p, "horizon_days": 60, "environment_sha256": h, "total_loss": 1, "kpi_loss": .5, "action_cost": .5, "forecast_steps": 0})
			}
		}
	}
	data, _ := json.Marshal(map[string]any{"schema": research.Schema, "purpose": "software_test", "series": "E11", "manifest": map[string]string{"engine_version": "SOFTWARE-TEST-ONLY", "source_sha256": h, "config_sha256": h, "input_sha256": h, "plan_sha256": h}, "episodes": episodes})
	return data
}
func TestEndpointsAndBlockedEngine(t *testing.T) {
	h := newTest(t)
	for _, path := range []string{"/health", "/api/research/spec", "/api/research/capabilities", "/api/research/openapi.json", "/api/research/runs"} {
		t.Run(path, func(t *testing.T) {
			w := call(h, "GET", path, "", nil)
			if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, path := range []string{"/api/research/experiments", "/api/research/runs/fake/replay"} {
		w := call(h, "POST", path, "application/json", []byte(`{}`))
		if w.Code != 409 || !strings.Contains(w.Body.String(), "ENGINE_NOT_CONNECTED") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestOriginAndHost(t *testing.T) {
	h := newTest(t)
	for _, origin := range []string{"http://evil.test", "null", "http://localhost:5173.evil.test"} {
		r := httptest.NewRequest("GET", "http://localhost/health", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(origin, w.Code)
		}
	}
	r := httptest.NewRequest("OPTIONS", "http://localhost/api/research/runs", nil)
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatal(w)
	}
	r = httptest.NewRequest("GET", "http://evil.test/health", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("DNS rebinding host accepted")
	}
}
func TestHTTPImportLifecycle(t *testing.T) {
	h := newTest(t)
	data := httpFixture()
	w := call(h, "POST", "/api/research/runs", "application/json", data)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Run research.RunInfo `json:"run"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	id := response.Run.ID
	if !research.HashPattern.MatchString(id) || response.Run.Status != "imported_unverified" {
		t.Fatal(response)
	}
	w = call(h, "GET", "/api/research/runs/"+id, "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "structure_and_summary_arithmetic") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(h, "GET", "/api/research/runs/"+id+"/download", "", nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) {
		t.Fatal("download mismatch")
	}
	w = call(h, "POST", "/api/research/runs/"+id+"/analysis", "application/json", []byte(`{"seed":42}`))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"block_count":24`) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, seed := range []string{`{}`, `{"seed":null}`, `{"seed":1.1}`, `{"seed":1,"seed":2}`, `{"seed":9007199254740992}`} {
		w = call(h, "POST", "/api/research/runs/"+id+"/analysis", "application/json", []byte(seed))
		if w.Code != 422 {
			t.Fatal(seed, w.Code)
		}
	}
}
func TestHTTPValidationAndLimits(t *testing.T) {
	h := newTest(t)
	tests := []struct {
		path, media, body string
		status            int
	}{
		{"/api/research/runs", "text/plain", "{}", 415}, {"/api/research/runs", "application/json", "{}", 422},
		{"/api/research/profile/validate", "application/json", "{}", 415},
		{"/api/research/profile/validate", "text/csv", "invalid", 200},
	}
	for _, test := range tests {
		w := call(h, "POST", test.path, test.media, []byte(test.body))
		if w.Code != test.status {
			t.Fatal(test, w.Code)
		}
	}
	w := call(h, "POST", "/api/research/runs", "application/json", bytes.Repeat([]byte(" "), research.MaxBundleBytes+1))
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	w = call(h, "GET", "/api/research/runs/"+strings.Repeat("a", 64), "", nil)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
