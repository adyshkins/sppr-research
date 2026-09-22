// Package r08 runs the preserved 0.8 engine in a bounded subprocess.
// The HTTP client selects archived jobs; it cannot supply model parameters or paths.
package r08

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dissertation.local/sppr-reconstruction/recoveryrun"
)

const PlanSHA256 = "b6e6ac5c67c0292dfe7d79b1c46ab47baf9fadd433a133f5eb76983956c3005b"
const maxJobs = 64
const maxStorageBytes int64 = 512 << 20

var ErrBusy = errors.New("another R08 job is running")
var ErrNotFound = errors.New("job or file not found")
var ErrQuota = errors.New("R08 storage quota reached")
var ErrConflict = errors.New("operation not available for this job")

type Request struct {
	Profile string   `json:"profile"`
	Repeat  int      `json:"repeat"`
	Arms    []string `json:"arms"`
}
type Episode struct {
	PlanIndex     int                  `json:"plan_index"`
	Arm           string               `json:"arm"`
	Summary       *recoveryrun.Summary `json:"summary,omitempty"`
	FileHashes    map[string]string    `json:"file_hashes,omitempty"`
	ReplayRecords *int                 `json:"replay_records,omitempty"`
}
type Job struct {
	ID           string    `json:"id"`
	Series       string    `json:"series"`
	Status       string    `json:"status"`
	Created      string    `json:"created"`
	Updated      string    `json:"updated"`
	Request      Request   `json:"request"`
	PlanSHA256   string    `json:"plan_sha256"`
	EngineSHA256 string    `json:"engine_sha256"`
	Episodes     []Episode `json:"episodes"`
	ReplayStatus string    `json:"replay_status"`
	Error        string    `json:"error,omitempty"`
}
type Manager struct {
	mu                             sync.Mutex
	root, binary, plan, binaryHash string
	jobs                           map[string]Job
	configs                        []recoveryrun.Config
	slot                           chan struct{}
	ctx                            context.Context
	cancel                         context.CancelFunc
	activeID                       string
	activeCancel                   context.CancelFunc
	wg                             sync.WaitGroup
	closed                         bool
}

func hash(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func hashFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func validID(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16 && strings.ToLower(id) == id
}
func now() string     { return time.Now().UTC().Format(time.RFC3339Nano) }
func clone(j Job) Job { b, _ := json.Marshal(j); var c Job; _ = json.Unmarshal(b, &c); return c }

func validatePlan(raw []byte) ([]recoveryrun.Config, error) {
	if hash(raw) != PlanSHA256 {
		return nil, fmt.Errorf("archived R08 plan SHA-256 mismatch")
	}
	var p []recoveryrun.Config
	if e := json.Unmarshal(raw, &p); e != nil {
		return nil, e
	}
	if len(p) != 240 {
		return nil, fmt.Errorf("expected 240 archived configurations")
	}
	seen := map[string]bool{}
	for _, c := range p {
		if e := c.Validate(); e != nil {
			return nil, e
		}
		if !validProfile(c.Profile) || !validArm(c.Arm) || c.Repeat < 0 || c.Repeat >= 12 {
			return nil, fmt.Errorf("invalid archived identity")
		}
		k := fmt.Sprintf("%s/%d/%s", c.Profile, c.Repeat, c.Arm)
		if seen[k] {
			return nil, fmt.Errorf("duplicate archived job")
		}
		seen[k] = true
	}
	return p, nil
}
func validProfile(s string) bool {
	return s == "normal" || s == "S" || s == "C" || s == "D" || s == "DS"
}
func validArm(s string) bool { return s == "B0" || s == "H" || s == "EM" || s == "ET" }
func Validate(r Request) error {
	if !validProfile(r.Profile) || r.Repeat < 0 || r.Repeat >= 12 || len(r.Arms) < 1 || len(r.Arms) > 4 {
		return fmt.Errorf("select profile normal/S/C/D/DS, repeat 0..11, and 1..4 arms B0/H/EM/ET")
	}
	seen := map[string]bool{}
	for _, a := range r.Arms {
		if !validArm(a) || seen[a] {
			return fmt.Errorf("invalid or duplicate arm")
		}
		seen[a] = true
	}
	return nil
}
func New(root, binary, plan string) (*Manager, error) {
	var e error
	if root, e = filepath.Abs(root); e != nil {
		return nil, e
	}
	if binary, e = filepath.Abs(binary); e != nil {
		return nil, e
	}
	if plan, e = filepath.Abs(plan); e != nil {
		return nil, e
	}
	info, e := os.Stat(binary)
	if e != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("build the recoverycheck binary before starting the API")
	}
	raw, e := os.ReadFile(plan)
	if e != nil {
		return nil, e
	}
	configs, e := validatePlan(raw)
	if e != nil {
		return nil, e
	}
	bh, e := hashFile(binary)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{root: root, binary: binary, plan: plan, binaryHash: bh, configs: configs, jobs: map[string]Job{}, slot: make(chan struct{}, 1), ctx: ctx, cancel: cancel}
	entries, e := os.ReadDir(root)
	if e != nil {
		cancel()
		return nil, e
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		data, e := os.ReadFile(filepath.Join(root, entry.Name(), "job.json"))
		if e != nil {
			cancel()
			return nil, fmt.Errorf("unreadable job metadata: %s", entry.Name())
		}
		var j Job
		if e = json.Unmarshal(data, &j); e != nil || j.ID != entry.Name() || j.Series != "DEV-R08" || Validate(j.Request) != nil {
			cancel()
			return nil, fmt.Errorf("invalid stored job: %s", entry.Name())
		}
		if j.Status == "running" || j.Status == "queued" {
			j.Status = "interrupted"
			j.Error = "The previous API process stopped before completion."
		}
		if j.ReplayStatus == "running" {
			j.ReplayStatus = "interrupted"
		}
		m.jobs[j.ID] = j
		if e = m.saveLocked(j); e != nil {
			cancel()
			return nil, e
		}
	}
	return m, nil
}
func (m *Manager) Close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, clone(j))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out
}
func (m *Manager) Get(id string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || !validID(id) {
		return Job{}, ErrNotFound
	}
	return clone(j), nil
}
func (m *Manager) saveLocked(j Job) error {
	data, e := json.MarshalIndent(j, "", "  ")
	if e != nil {
		return e
	}
	data = append(data, '\n')
	dir := filepath.Join(m.root, j.ID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".job-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(dir, "job.json"))
}
func (m *Manager) change(id string, fn func(*Job)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := clone(m.jobs[id])
	fn(&j)
	j.Updated = now()
	if e := m.saveLocked(j); e != nil {
		return e
	}
	m.jobs[id] = j
	return nil
}
func (m *Manager) quotaLocked() error {
	if len(m.jobs) >= maxJobs {
		return ErrQuota
	}
	var total int64
	e := filepath.WalkDir(m.root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in job storage")
		}
		if !d.IsDir() {
			i, e := d.Info()
			if e != nil {
				return e
			}
			total += i.Size()
			if total > maxStorageBytes-(16<<20) {
				return ErrQuota
			}
		}
		return nil
	})
	return e
}
func (m *Manager) currentInputs() error {
	h, e := hashFile(m.binary)
	if e != nil || h != m.binaryHash {
		return fmt.Errorf("engine binary changed; restart the API")
	}
	raw, e := os.ReadFile(m.plan)
	if e != nil || hash(raw) != PlanSHA256 {
		return fmt.Errorf("archived plan changed")
	}
	return nil
}
func (m *Manager) Start(r Request) (Job, error) {
	if e := Validate(r); e != nil {
		return Job{}, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Job{}, ErrConflict
	}
	select {
	case m.slot <- struct{}{}:
	default:
		return Job{}, ErrBusy
	}
	release := true
	defer func() {
		if release {
			<-m.slot
		}
	}()
	if e := m.quotaLocked(); e != nil {
		return Job{}, e
	}
	if e := m.currentInputs(); e != nil {
		return Job{}, e
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return Job{}, e
	}
	j := Job{ID: hex.EncodeToString(b), Series: "DEV-R08", Status: "queued", Created: now(), Updated: now(), Request: r, PlanSHA256: PlanSHA256, EngineSHA256: m.binaryHash, ReplayStatus: "not_run", Episodes: []Episode{}}
	// Archive order, not the caller's arm order, fixes execution order.
	for i, c := range m.configs {
		if c.Profile != r.Profile || c.Repeat != r.Repeat {
			continue
		}
		for _, a := range r.Arms {
			if c.Arm == a {
				j.Episodes = append(j.Episodes, Episode{PlanIndex: i, Arm: a})
			}
		}
	}
	if len(j.Episodes) != len(r.Arms) {
		return Job{}, fmt.Errorf("missing archived configuration")
	}
	if e := m.saveLocked(j); e != nil {
		return Job{}, e
	}
	m.jobs[j.ID] = clone(j)
	ctx, cancel := context.WithCancel(m.ctx)
	m.activeID = j.ID
	m.activeCancel = cancel
	m.wg.Add(1)
	release = false
	go m.work(ctx, j, false)
	return clone(j), nil
}
func (m *Manager) Replay(id string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	if m.closed || j.Status != "complete" || j.EngineSHA256 != m.binaryHash || j.PlanSHA256 != PlanSHA256 {
		return Job{}, ErrConflict
	}
	select {
	case m.slot <- struct{}{}:
	default:
		return Job{}, ErrBusy
	}
	if e := m.currentInputs(); e != nil {
		<-m.slot
		return Job{}, e
	}
	j = clone(j)
	j.ReplayStatus = "running"
	j.Error = ""
	j.Updated = now()
	for i := range j.Episodes {
		j.Episodes[i].ReplayRecords = nil
	}
	if e := m.saveLocked(j); e != nil {
		<-m.slot
		return Job{}, e
	}
	m.jobs[id] = j
	ctx, cancel := context.WithCancel(m.ctx)
	m.activeID = id
	m.activeCancel = cancel
	m.wg.Add(1)
	go m.work(ctx, j, true)
	return clone(j), nil
}
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeID != id || m.activeCancel == nil {
		return ErrConflict
	}
	m.activeCancel()
	return nil
}
func (m *Manager) command(ctx context.Context, args []string, logPath string) error {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	log, e := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := exec.CommandContext(cctx, m.binary, args...)
	cmd.Dir = filepath.Dir(m.binary)
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2")
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Run(); e != nil {
		if cctx.Err() != nil {
			return cctx.Err()
		}
		return fmt.Errorf("engine failed; see %s", filepath.Base(logPath))
	}
	return nil
}
func (m *Manager) work(ctx context.Context, j Job, replay bool) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		if m.activeCancel != nil {
			m.activeCancel()
		}
		m.activeCancel = nil
		m.activeID = ""
		m.mu.Unlock()
		<-m.slot
	}()
	var workErr error
	if !replay {
		workErr = m.change(j.ID, func(x *Job) { x.Status = "running" })
	}
	for i, ep := range j.Episodes {
		if workErr != nil {
			break
		}
		if e := ctx.Err(); e != nil {
			workErr = e
			break
		}
		dir := filepath.Join(m.root, j.ID, fmt.Sprintf("ep%03d", ep.PlanIndex))
		if replay {
			// Reject tampered output before asking the controller to recompute it.
			for name, expected := range ep.FileHashes {
				actual, e := hashFile(filepath.Join(dir, name))
				if e != nil || actual != expected {
					workErr = fmt.Errorf("stored file integrity failed: %s", name)
					break
				}
			}
			if workErr != nil {
				break
			}
			workErr = m.command(ctx, []string{"-replay", dir}, filepath.Join(m.root, j.ID, fmt.Sprintf("replay_%03d.log", ep.PlanIndex)))
			if workErr == nil {
				n := ep.Summary.Days
				workErr = m.change(j.ID, func(x *Job) { x.Episodes[i].ReplayRecords = &n })
			}
		} else {
			workErr = m.command(ctx, []string{"-plan", m.plan, "-job", strconv.Itoa(ep.PlanIndex), "-out", dir}, filepath.Join(m.root, j.ID, fmt.Sprintf("run_%03d.log", ep.PlanIndex)))
			if workErr != nil {
				break
			}
			var s recoveryrun.Summary
			data, e := os.ReadFile(filepath.Join(dir, "summary.json"))
			if e != nil {
				workErr = e
				break
			}
			if e = json.Unmarshal(data, &s); e != nil {
				workErr = e
				break
			}
			if s.Days != 60 || s.Profile != j.Request.Profile || s.Repeat != j.Request.Repeat || s.Arm != ep.Arm {
				workErr = fmt.Errorf("engine output identity mismatch")
				break
			}
			files := map[string]string{}
			for _, name := range outputFiles {
				h, e := hashFile(filepath.Join(dir, name))
				if e != nil {
					workErr = e
					break
				}
				files[name] = h
			}
			if workErr != nil {
				break
			}
			workErr = m.change(j.ID, func(x *Job) { x.Episodes[i].Summary = &s; x.Episodes[i].FileHashes = files })
		}
	}
	// Disk errors are not promoted into successful jobs. An unpersisted transition
	// remains incomplete and will be marked interrupted on restart.
	_ = m.change(j.ID, func(x *Job) {
		if workErr != nil {
			x.Error = workErr.Error()
			if replay {
				x.ReplayStatus = "failed"
			} else {
				x.Status = "failed"
			}
			if errors.Is(workErr, context.Canceled) {
				if replay {
					x.ReplayStatus = "cancelled"
				} else {
					x.Status = "cancelled"
				}
			}
		} else if replay {
			x.ReplayStatus = "verified"
		} else {
			x.Status = "complete"
		}
	})
}

var outputFiles = []string{"experiment_config.json", "controller.jsonl.gz", "physical.jsonl.gz", "summary.json", "COMPLETED.json"}

func (m *Manager) File(id string, index int, name string) (string, error) {
	allowed := false
	for _, n := range outputFiles {
		if n == name {
			allowed = true
		}
	}
	if !allowed {
		return "", ErrNotFound
	}
	j, e := m.Get(id)
	if e != nil {
		return "", e
	}
	expected := ""
	for _, ep := range j.Episodes {
		if ep.PlanIndex == index {
			expected = ep.FileHashes[name]
		}
	}
	if expected == "" {
		return "", ErrNotFound
	}
	p := filepath.Join(m.root, id, fmt.Sprintf("ep%03d", index), name)
	info, e := os.Lstat(p)
	if e != nil || !info.Mode().IsRegular() {
		return "", ErrNotFound
	}
	actual, e := hashFile(p)
	if e != nil || actual != expected {
		return "", fmt.Errorf("output integrity failed")
	}
	return p, nil
}
