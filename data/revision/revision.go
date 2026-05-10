package revision

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PAW122/TsunamiDB/data/valuepatch"
)

type Mode string

const (
	ModeOff     Mode = "off"
	ModeCurrent Mode = "current"
	ModeHistory Mode = "history"
)

var (
	ErrInvalidMode        = errors.New("revision: invalid mode")
	ErrMissingBaseRev     = errors.New("revision: missing base_rev")
	ErrRevisionConflict   = errors.New("revision: conflict")
	ErrHistoryUnavailable = errors.New("revision: history unavailable")
)

type State struct {
	Mode           Mode   `json:"mode"`
	Rev            uint64 `json:"rev"`
	HistoryFromRev uint64 `json:"history_from_rev,omitempty"`
}

type PatchRecord struct {
	Table     string                 `json:"table"`
	Key       string                 `json:"key"`
	BaseRev   uint64                 `json:"base_rev"`
	Rev       uint64                 `json:"rev"`
	Ops       []valuepatch.Operation `json:"ops"`
	CreatedAt string                 `json:"created_at"`
}

type HistoryLimit struct {
	MaxPatches int `json:"max_patches"`
}

type ConflictError struct {
	BaseRev    uint64
	CurrentRev uint64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("revision conflict: base_rev=%d current_rev=%d", e.BaseRev, e.CurrentRev)
}

func (e *ConflictError) Is(target error) bool {
	return target == ErrRevisionConflict
}

var mu sync.Mutex

func SetPolicy(table, key string, mode Mode) (State, error) {
	mode, err := normalizeMode(mode)
	if err != nil {
		return State{}, err
	}

	mu.Lock()
	defer mu.Unlock()

	if mode == ModeOff {
		_ = os.Remove(statePath(table, key))
		_ = os.Remove(historyPath(table, key))
		return State{Mode: ModeOff}, nil
	}

	state, err := readStateLocked(table, key)
	if err != nil {
		return State{}, err
	}
	state.Mode = mode
	if mode == ModeHistory && state.HistoryFromRev > state.Rev {
		state.HistoryFromRev = state.Rev
	}
	if mode == ModeHistory && state.HistoryFromRev == 0 && state.Rev > 0 {
		state.HistoryFromRev = state.Rev
	}
	if mode == ModeCurrent {
		state.HistoryFromRev = 0
	}

	if err := writeStateLocked(table, key, state); err != nil {
		return State{}, err
	}
	return state, nil
}

func GetState(table, key string) (State, error) {
	mu.Lock()
	defer mu.Unlock()
	return readStateLocked(table, key)
}

func AdvanceFullWrite(table, key string) (State, bool, error) {
	mu.Lock()
	defer mu.Unlock()

	state, err := readStateLocked(table, key)
	if err != nil {
		return State{}, false, err
	}
	if state.Mode == ModeOff {
		return state, false, nil
	}

	state.Rev++
	if state.Mode == ModeHistory {
		state.HistoryFromRev = state.Rev
		if err := truncateHistoryLocked(table, key); err != nil {
			return State{}, false, err
		}
	}
	if err := writeStateLocked(table, key, state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func AdvancePatch(table, key string, baseRev *uint64, ops []valuepatch.Operation) (State, *PatchRecord, bool, error) {
	mu.Lock()
	defer mu.Unlock()

	state, err := readStateLocked(table, key)
	if err != nil {
		return State{}, nil, false, err
	}
	if state.Mode == ModeOff {
		return state, nil, false, nil
	}
	if baseRev == nil {
		return State{}, nil, true, ErrMissingBaseRev
	}
	if *baseRev != state.Rev {
		return State{}, nil, true, &ConflictError{BaseRev: *baseRev, CurrentRev: state.Rev}
	}

	nextRev := state.Rev + 1
	record := &PatchRecord{
		Table:     table,
		Key:       key,
		BaseRev:   state.Rev,
		Rev:       nextRev,
		Ops:       append([]valuepatch.Operation(nil), ops...),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	state.Rev = nextRev
	if state.Mode == ModeHistory {
		if err := appendHistoryLocked(table, key, *record); err != nil {
			return State{}, nil, true, err
		}
		if err := enforceHistoryLimitLocked(table, key, &state); err != nil {
			return State{}, nil, true, err
		}
	}
	if err := writeStateLocked(table, key, state); err != nil {
		return State{}, nil, true, err
	}
	return state, record, true, nil
}

func CheckPatch(table, key string, baseRev *uint64) (State, bool, error) {
	mu.Lock()
	defer mu.Unlock()

	state, err := readStateLocked(table, key)
	if err != nil {
		return State{}, false, err
	}
	if state.Mode == ModeOff {
		return state, false, nil
	}
	if baseRev == nil {
		return State{}, true, ErrMissingBaseRev
	}
	if *baseRev != state.Rev {
		return State{}, true, &ConflictError{BaseRev: *baseRev, CurrentRev: state.Rev}
	}
	return state, true, nil
}

func History(table, key string, fromRev, toRev uint64) ([]PatchRecord, State, error) {
	mu.Lock()
	defer mu.Unlock()

	state, err := readStateLocked(table, key)
	if err != nil {
		return nil, State{}, err
	}
	if state.Mode != ModeHistory {
		return nil, state, ErrHistoryUnavailable
	}
	if fromRev < state.HistoryFromRev {
		return nil, state, ErrHistoryUnavailable
	}

	file, err := os.Open(historyPath(table, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, state, nil
	}
	if err != nil {
		return nil, State{}, err
	}
	defer file.Close()

	records := make([]PatchRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record PatchRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, State{}, err
		}
		if record.Rev <= fromRev {
			continue
		}
		if toRev > 0 && record.Rev > toRev {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, State{}, err
	}
	return records, state, nil
}

func ResetForTests() {
	mu.Lock()
	defer mu.Unlock()
	_ = os.RemoveAll(rootDir())
}

func normalizeMode(mode Mode) (Mode, error) {
	switch Mode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeCurrent:
		return ModeCurrent, nil
	case ModeHistory:
		return ModeHistory, nil
	default:
		return "", ErrInvalidMode
	}
}

func readStateLocked(table, key string) (State, error) {
	data, err := os.ReadFile(statePath(table, key))
	if errors.Is(err, os.ErrNotExist) {
		return State{Mode: ModeOff}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	mode, err := normalizeMode(state.Mode)
	if err != nil {
		return State{}, err
	}
	state.Mode = mode
	return state, nil
}

func writeStateLocked(table, key string, state State) error {
	if err := os.MkdirAll(filepath.Dir(statePath(table, key)), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(table, key), data, 0644)
}

func appendHistoryLocked(table, key string, record PatchRecord) error {
	if err := os.MkdirAll(filepath.Dir(historyPath(table, key)), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(historyPath(table, key), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func enforceHistoryLimitLocked(table, key string, state *State) error {
	limit := historyLimit()
	if limit.MaxPatches <= 0 {
		return nil
	}
	if state.Rev >= state.HistoryFromRev && state.Rev-state.HistoryFromRev <= uint64(limit.MaxPatches) {
		return nil
	}

	records, err := readHistoryRecordsLocked(table, key)
	if err != nil {
		return err
	}
	if len(records) <= limit.MaxPatches {
		return nil
	}

	keepFrom := len(records) - limit.MaxPatches
	records = records[keepFrom:]
	if len(records) > 0 {
		state.HistoryFromRev = records[0].BaseRev
	} else {
		state.HistoryFromRev = state.Rev
	}
	return writeHistoryRecordsLocked(table, key, records)
}

func readHistoryRecordsLocked(table, key string) ([]PatchRecord, error) {
	file, err := os.Open(historyPath(table, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	records := make([]PatchRecord, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record PatchRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func writeHistoryRecordsLocked(table, key string, records []PatchRecord) error {
	if err := os.MkdirAll(filepath.Dir(historyPath(table, key)), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(historyPath(table, key), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, record := range records {
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func historyLimit() HistoryLimit {
	return HistoryLimit{
		MaxPatches: intEnv("TSU_REVISION_HISTORY_MAX_PATCHES", 1000),
	}
}

func intEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func truncateHistoryLocked(table, key string) error {
	if err := os.MkdirAll(filepath.Dir(historyPath(table, key)), 0755); err != nil {
		return err
	}
	return os.WriteFile(historyPath(table, key), nil, 0644)
}

func rootDir() string {
	return filepath.Join("db", "revision")
}

func statePath(table, key string) string {
	return filepath.Join(rootDir(), encodeName(table), encodeName(key)+".json")
}

func historyPath(table, key string) string {
	return filepath.Join(rootDir(), encodeName(table), encodeName(key)+".patches.jsonl")
}

func encodeName(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
