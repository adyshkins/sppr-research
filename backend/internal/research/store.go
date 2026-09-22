package research

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxBundleBytes = 8 << 20
const MaxStoredBundles = 64

var ErrNotFound = errors.New("набор не найден")
var ErrQuota = errors.New("достигнут лимит 64 наборов; управление хранилищем выполняется локально")

type Store struct {
	root string
	mu   sync.Mutex
}
type RunInfo struct {
	ID            string `json:"id"`
	Series        string `json:"series"`
	Purpose       string `json:"purpose"`
	EngineVersion string `json:"engine_version"`
	EpisodeCount  int    `json:"episode_count"`
	ImportedAt    string `json:"imported_at"`
	Status        string `json:"status"`
}

func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("хранилище должно быть обычным каталогом")
	}
	return &Store{root: root}, nil
}
func (s *Store) Put(data []byte) (RunInfo, Validation, error) {
	if len(data) > MaxBundleBytes {
		return RunInfo{}, Validation{}, fmt.Errorf("слишком большой JSON")
	}
	b, v, err := ParseBundle(data)
	if err != nil {
		return RunInfo{}, v, err
	}
	id := fmt.Sprintf("%x", sha256.Sum256(data))
	path := filepath.Join(s.root, id+".json")
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, e := os.Lstat(path); e == nil {
		if !st.Mode().IsRegular() {
			return RunInfo{}, v, fmt.Errorf("небезопасный объект хранилища")
		}
		old, e := os.ReadFile(path)
		if e != nil || fmt.Sprintf("%x", sha256.Sum256(old)) != id {
			return RunInfo{}, v, fmt.Errorf("нарушена целостность хранилища")
		}
		return infoFor(id, b, st.ModTime()), v, nil
	} else if !os.IsNotExist(e) {
		return RunInfo{}, v, e
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return RunInfo{}, v, err
	}
	count := 0
	for _, e := range entries {
		if HashPattern.MatchString(strings.TrimSuffix(e.Name(), ".json")) && strings.HasSuffix(e.Name(), ".json") {
			count++
		}
	}
	if count >= MaxStoredBundles {
		return RunInfo{}, v, ErrQuota
	}
	file, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return RunInfo{}, v, err
	}
	temp := file.Name()
	defer os.Remove(temp)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return RunInfo{}, v, err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return RunInfo{}, v, err
	}
	if err = file.Close(); err != nil {
		return RunInfo{}, v, err
	}
	if err = os.Rename(temp, path); err != nil {
		return RunInfo{}, v, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return RunInfo{}, v, err
	}
	return infoFor(id, b, st.ModTime()), v, nil
}
func infoFor(id string, b Bundle, t time.Time) RunInfo {
	return RunInfo{id, b.Series, b.Purpose, b.Manifest.EngineVersion, len(b.Episodes), t.UTC().Format(time.RFC3339), "imported_unverified"}
}
func (s *Store) Load(id string) ([]byte, Bundle, Validation, error) {
	if !HashPattern.MatchString(id) {
		return nil, Bundle{}, Validation{}, ErrNotFound
	}
	path := filepath.Join(s.root, id+".json")
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, Bundle{}, Validation{}, ErrNotFound
	}
	if err != nil {
		return nil, Bundle{}, Validation{}, err
	}
	if !st.Mode().IsRegular() || st.Size() > MaxBundleBytes {
		return nil, Bundle{}, Validation{}, fmt.Errorf("недопустимый объект хранилища")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Bundle{}, Validation{}, err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != id {
		return nil, Bundle{}, Validation{}, fmt.Errorf("хэш сохранённого набора не совпадает с ID")
	}
	b, v, err := ParseBundle(data)
	return data, b, v, err
}
func (s *Store) List() ([]RunInfo, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []RunInfo{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if !HashPattern.MatchString(id) {
			continue
		}
		_, b, _, err := s.Load(id)
		if err != nil {
			return nil, err
		}
		st, err := e.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, infoFor(id, b, st.ModTime()))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ImportedAt == out[j].ImportedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].ImportedAt > out[j].ImportedAt
	})
	return out, nil
}
