package r08

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var testBinary, testPlan, projectRoot string

func TestMain(m *testing.M) {
	_, file, _, _ := runtime.Caller(0)
	projectRoot = filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))
	tmp, e := os.MkdirTemp("", "sppr-r08-tests-")
	if e != nil {
		panic(e)
	}
	testBinary = filepath.Join(tmp, "recoverycheck")
	if runtime.GOOS == "windows" {
		testBinary += ".exe"
	}
	testPlan = filepath.Join(projectRoot, "engine/results/recovery_v08/plan.json")
	cmd := exec.Command("go", "build", "-o", testBinary, "./cmd/recoverycheck")
	cmd.Dir = filepath.Join(projectRoot, "engine")
	if output, e := cmd.CombinedOutput(); e != nil {
		fmt.Fprintln(os.Stderr, e, string(output))
		os.RemoveAll(tmp)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
func manager(t *testing.T) *Manager {
	t.Helper()
	m, e := New(t.TempDir(), testBinary, testPlan)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(m.Close)
	return m
}
func wait(t *testing.T, m *Manager, id string, replay bool) Job {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		j, e := m.Get(id)
		if e != nil {
			t.Fatal(e)
		}
		if replay && j.ReplayStatus != "running" {
			return j
		}
		if !replay && j.Status != "queued" && j.Status != "running" {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job timeout")
	return Job{}
}
func TestRequestValidation(t *testing.T) {
	good := Request{"S", 0, []string{"B0", "H", "EM", "ET"}}
	if e := Validate(good); e != nil {
		t.Fatal(e)
	}
	bad := []Request{{"E11", 0, []string{"H"}}, {"S", 12, []string{"H"}}, {"S", -1, []string{"H"}}, {"S", 0, nil}, {"S", 0, []string{"H", "H"}}, {"S", 0, []string{"F"}}, {"../../", 0, []string{"H"}}, {"S", 0, []string{"H;echo x"}}}
	for i, r := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if Validate(r) == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
}
func TestFrozenPlanAndMissingBinary(t *testing.T) {
	data, e := os.ReadFile(testPlan)
	if e != nil {
		t.Fatal(e)
	}
	if p, e := validatePlan(data); e != nil || len(p) != 240 {
		t.Fatal(e)
	}
	if _, e := validatePlan(append(data, ' ')); e == nil {
		t.Fatal("changed plan accepted")
	}
	if _, e := New(t.TempDir(), "missing-binary", testPlan); e == nil {
		t.Fatal("missing binary accepted")
	}
	p := filepath.Join(t.TempDir(), "plan.json")
	os.WriteFile(p, []byte("[]"), 0600)
	if _, e := New(t.TempDir(), testBinary, p); e == nil {
		t.Fatal("invalid plan accepted")
	}
}
func TestLifecycleReplayPersistenceAndIntegrity(t *testing.T) {
	m := manager(t)
	j, e := m.Start(Request{"normal", 0, []string{"H"}})
	if e != nil {
		t.Fatal(e)
	}
	j = wait(t, m, j.ID, false)
	if j.Status != "complete" || len(j.Episodes) != 1 || j.Episodes[0].Summary.Days != 60 {
		t.Fatal(j)
	}
	index := j.Episodes[0].PlanIndex
	p, e := m.File(j.ID, index, "summary.json")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.File(j.ID, index, "../job.json"); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = m.File(j.ID, 239, "summary.json"); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = m.Replay(j.ID); e != nil {
		t.Fatal(e)
	}
	j = wait(t, m, j.ID, true)
	if j.ReplayStatus != "verified" || *j.Episodes[0].ReplayRecords != 60 {
		t.Fatal(j)
	}
	j.Episodes[0].Summary.Days = 1
	again, _ := m.Get(j.ID)
	if again.Episodes[0].Summary.Days != 60 {
		t.Fatal("snapshot aliases state")
	}
	m.Close()
	restored, e := New(m.root, testBinary, testPlan)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	old, e := restored.Get(j.ID)
	if e != nil || old.Status != "complete" || old.ReplayStatus != "verified" {
		t.Fatal(e, old)
	}
	os.WriteFile(p, []byte("{}"), 0600)
	if _, e := restored.File(j.ID, index, "summary.json"); e == nil {
		t.Fatal("tampered file served")
	}
	if _, e := restored.Replay(j.ID); e != nil {
		t.Fatal(e)
	}
	bad := wait(t, restored, j.ID, true)
	if bad.ReplayStatus != "failed" || !strings.Contains(bad.Error, "integrity") {
		t.Fatal(bad)
	}
}
func TestCancellationAndBusy(t *testing.T) {
	m := manager(t)
	j, e := m.Start(Request{"DS", 0, []string{"B0", "H", "EM", "ET"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := m.Start(Request{"S", 0, []string{"H"}}); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	if e = m.Cancel(j.ID); e != nil {
		t.Fatal(e)
	}
	j = wait(t, m, j.ID, false)
	if j.Status != "cancelled" {
		t.Fatal(j)
	}
	if _, e = m.Replay(j.ID); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e = m.Cancel("not-a-job"); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}
func TestRestartMarksIncomplete(t *testing.T) {
	root := t.TempDir()
	id := strings.Repeat("a", 32)
	dir := filepath.Join(root, id)
	os.Mkdir(dir, 0700)
	j := Job{ID: id, Series: "DEV-R08", Status: "running", Request: Request{"S", 0, []string{"H"}}, Episodes: []Episode{}, ReplayStatus: "not_run"}
	b, _ := json.Marshal(j)
	os.WriteFile(filepath.Join(dir, "job.json"), b, 0600)
	m, e := New(root, testBinary, testPlan)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	got, _ := m.Get(id)
	if got.Status != "interrupted" {
		t.Fatal(got)
	}
}
func TestStorageQuotaAndShutdown(t *testing.T) {
	m := manager(t)
	m.mu.Lock()
	for i := 0; i < maxJobs; i++ {
		m.jobs[fmt.Sprint(i)] = Job{}
	}
	m.mu.Unlock()
	if _, e := m.Start(Request{"normal", 0, []string{"H"}}); !errors.Is(e, ErrQuota) {
		t.Fatal(e)
	}
	m.Close()
	if _, e := m.Start(Request{"normal", 0, []string{"H"}}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}
func TestIDsAndCorruptMetadata(t *testing.T) {
	for _, id := range []string{"", "../x", strings.Repeat("a", 31), strings.Repeat("A", 32), strings.Repeat("g", 32)} {
		if validID(id) {
			t.Fatal(id)
		}
	}
	m := manager(t)
	if _, e := m.Get(strings.Repeat("0", 32)); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	root := t.TempDir()
	dir := filepath.Join(root, strings.Repeat("0", 32))
	os.Mkdir(dir, 0700)
	os.WriteFile(filepath.Join(dir, "job.json"), []byte("{"), 0600)
	if _, e := New(root, testBinary, testPlan); e == nil {
		t.Fatal("corrupt metadata accepted")
	}
}
func TestCommandCancelled(t *testing.T) {
	m := manager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := m.command(ctx, []string{"-plan", testPlan, "-job", "48", "-out", filepath.Join(t.TempDir(), "new")}, filepath.Join(t.TempDir(), "log")); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
