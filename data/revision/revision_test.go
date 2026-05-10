package revision

import (
	"errors"
	"os"
	"testing"

	"github.com/PAW122/TsunamiDB/data/valuepatch"
)

func TestRevisionPolicyAndHistory(t *testing.T) {
	wd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})

	state, err := GetState("docs", "doc1")
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.Mode != ModeOff || state.Rev != 0 {
		t.Fatalf("default state = %+v", state)
	}

	state, err = SetPolicy("docs", "doc1", ModeHistory)
	if err != nil {
		t.Fatalf("SetPolicy() error = %v", err)
	}
	if state.Mode != ModeHistory || state.Rev != 0 {
		t.Fatalf("history state = %+v", state)
	}

	if _, _, err := CheckPatch("docs", "doc1", nil); !errors.Is(err, ErrMissingBaseRev) {
		t.Fatalf("missing base rev error = %v", err)
	}

	base := uint64(0)
	state, record, enabled, err := AdvancePatch("docs", "doc1", &base, []valuepatch.Operation{{Offset: 0, Insert: "x"}})
	if err != nil {
		t.Fatalf("AdvancePatch() error = %v", err)
	}
	if !enabled || state.Rev != 1 || record == nil || record.Rev != 1 {
		t.Fatalf("advance state=%+v record=%+v enabled=%v", state, record, enabled)
	}

	records, state, err := History("docs", "doc1", 0, 0)
	if err != nil {
		t.Fatalf("History() error = %v", err)
	}
	if state.Rev != 1 || len(records) != 1 || records[0].BaseRev != 0 {
		t.Fatalf("history state=%+v records=%+v", state, records)
	}

	_, _, err = History("docs", "doc1", 2, 0)
	if err != nil {
		t.Fatalf("future history should be empty, got error %v", err)
	}

	state, changed, err := AdvanceFullWrite("docs", "doc1")
	if err != nil {
		t.Fatalf("AdvanceFullWrite() error = %v", err)
	}
	if !changed || state.Rev != 2 || state.HistoryFromRev != 2 {
		t.Fatalf("full write state=%+v changed=%v", state, changed)
	}
	if _, _, err := History("docs", "doc1", 0, 0); !errors.Is(err, ErrHistoryUnavailable) {
		t.Fatalf("stale history error = %v", err)
	}
}

func TestRevisionValidation(t *testing.T) {
	wd := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(wd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})

	if _, err := SetPolicy("docs", "doc1", "bad"); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("invalid mode error = %v", err)
	}

	state, err := SetPolicy("docs", "doc1", ModeCurrent)
	if err != nil {
		t.Fatalf("SetPolicy current: %v", err)
	}
	if state.Mode != ModeCurrent {
		t.Fatalf("state = %+v", state)
	}

	base := uint64(3)
	if _, _, err := CheckPatch("docs", "doc1", &base); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("conflict error = %v", err)
	}

	state, err = SetPolicy("docs", "doc1", ModeOff)
	if err != nil {
		t.Fatalf("SetPolicy off: %v", err)
	}
	if state.Mode != ModeOff {
		t.Fatalf("off state = %+v", state)
	}
}
