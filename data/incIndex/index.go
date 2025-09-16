package incindex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const baseDir = "./db/inc_tables"

var (
	ErrDuplicateKey = errors.New("incindex: duplicate entry_key")
)

type tableIndex struct {
	mu        sync.RWMutex
	path      string
	keys      []string
	positions map[string]uint64
}

var (
	indices sync.Map // map[string]*tableIndex
)

type diskPayload struct {
	Keys []string `json:"keys"`
}

func getIndex(tableFile string) (*tableIndex, error) {
	if v, ok := indices.Load(tableFile); ok {
		return v.(*tableIndex), nil
	}

	idx := &tableIndex{
		path:      filepath.Join(baseDir, tableFile+".idx"),
		keys:      make([]string, 0, 128),
		positions: make(map[string]uint64),
	}

	if err := idx.load(); err != nil {
		return nil, err
	}

	actual, _ := indices.LoadOrStore(tableFile, idx)
	return actual.(*tableIndex), nil
}

func (t *tableIndex) load() error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var payload diskPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	t.keys = append(t.keys[:0], payload.Keys...)
	t.positions = make(map[string]uint64, len(payload.Keys))
	for i, key := range payload.Keys {
		if key == "" {
			continue
		}
		t.positions[key] = uint64(i)
	}
	return nil
}

func (t *tableIndex) saveLocked() error {
	payload := diskPayload{Keys: t.keys}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tmp := t.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.path)
}

func (t *tableIndex) ensureLength(length uint64) {
	for uint64(len(t.keys)) < length {
		t.keys = append(t.keys, "")
	}
}

func (t *tableIndex) reindexFrom(pos uint64) {
	if pos > uint64(len(t.keys)) {
		pos = uint64(len(t.keys))
	}
	for i := int(pos); i < len(t.keys); i++ {
		key := t.keys[i]
		if key == "" {
			continue
		}
		t.positions[key] = uint64(i)
	}
}

func Insert(tableFile string, pos uint64, key string) error {
	if key == "" {
		return nil
	}
	idx, err := getIndex(tableFile)
	if err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, exists := idx.positions[key]; exists {
		return ErrDuplicateKey
	}

	idx.ensureLength(pos)
	idx.keys = append(idx.keys, "")
	copy(idx.keys[pos+1:], idx.keys[pos:])
	idx.keys[pos] = key
	idx.reindexFrom(pos)
	return idx.saveLocked()
}

func Set(tableFile string, pos uint64, key string) error {
	if key == "" {
		return nil
	}
	idx, err := getIndex(tableFile)
	if err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.ensureLength(pos + 1)

	current := idx.keys[pos]
	if current == key {
		idx.positions[key] = pos
		return idx.saveLocked()
	}

	if _, exists := idx.positions[key]; exists {
		return ErrDuplicateKey
	}

	if current != "" {
		delete(idx.positions, current)
	}

	idx.keys[pos] = key
	idx.positions[key] = pos
	return idx.saveLocked()
}

func Lookup(tableFile, key string) (uint64, bool, error) {
	idx, err := getIndex(tableFile)
	if err != nil {
		return 0, false, err
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	pos, ok := idx.positions[key]
	return pos, ok, nil
}

func Remove(tableFile, key string) error {
	if key == "" {
		return nil
	}
	idx, err := getIndex(tableFile)
	if err != nil {
		return err
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	pos, ok := idx.positions[key]
	if !ok {
		return nil
	}

	delete(idx.positions, key)
	if int(pos) < len(idx.keys) {
		idx.keys[pos] = ""
	}
	return idx.saveLocked()
}

func DropTable(tableFile string) error {
	indices.Delete(tableFile)
	path := filepath.Join(baseDir, tableFile+".idx")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ResetForTests() {
	indices.Range(func(key, value any) bool {
		idx := value.(*tableIndex)
		idx.mu.Lock()
		idx.keys = nil
		idx.positions = make(map[string]uint64)
		_ = os.Remove(idx.path)
		idx.mu.Unlock()
		indices.Delete(key)
		return true
	})
}
