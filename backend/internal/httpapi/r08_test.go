package httpapi

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"sppr-prototype/research/internal/research"
	"strings"
	"testing"
)

func TestR08DisabledAndNotFabricated(t *testing.T) {
	h := newTest(t)
	for _, p := range []string{"/api/research/r08/jobs", "/api/research/r08/jobs/missing"} {
		w := call(h, "GET", p, "", nil)
		if w.Code != 503 || !strings.Contains(w.Body.String(), "R08_NOT_CONNECTED") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	w := call(h, "POST", "/api/research/r08/experiments", "application/json", []byte(`{"profile":"S","repeat":0,"arms":["H"]}`))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
func TestStaticFilesAndSameOrigin(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>test UI</html>"), 0600)
	store, e := research.NewStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	h := NewWithOptions(store, []string{"http://localhost:18081"}, Options{StaticDir: root})
	w := call(h, "GET", "/", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "test UI") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = call(h, "GET", "/api/research/does-not-exist", "", nil)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", "http://localhost:18081/health", nil)
	r.Header.Set("Origin", "http://localhost:18081")
	v := httptest.NewRecorder()
	h.ServeHTTP(v, r)
	if v.Code != 200 {
		t.Fatal(v.Code)
	}
}
