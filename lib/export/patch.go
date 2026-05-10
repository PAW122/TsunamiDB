package export

import (
	"fmt"

	"github.com/PAW122/TsunamiDB/data/revision"
	"github.com/PAW122/TsunamiDB/data/valuepatch"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
)

type PatchOperation = valuepatch.Operation
type RevisionMode = revision.Mode
type RevisionState = revision.State
type RevisionPatchRecord = revision.PatchRecord

const (
	RevisionModeOff     = revision.ModeOff
	RevisionModeCurrent = revision.ModeCurrent
	RevisionModeHistory = revision.ModeHistory
)

func Patch(key, table string, ops []valuepatch.Operation) ([]byte, error) {
	return patchWithOptionalRevision(key, table, nil, ops)
}

func PatchWithRevision(key, table string, baseRev uint64, ops []valuepatch.Operation) ([]byte, revision.State, error) {
	updated, state, _, _, err := patchWithOptionalRevisionState(key, table, &baseRev, ops)
	return updated, state, err
}

func SetRevisionPolicy(key, table string, mode revision.Mode) (revision.State, error) {
	if key == "" || table == "" {
		return revision.State{}, fmt.Errorf("Invalid key or table value")
	}
	unlock := valuepatch.LockKey(table, key)
	defer unlock()
	return setRevisionPolicy(table, key, mode)
}

func GetRevisionState(key, table string) (revision.State, error) {
	if key == "" || table == "" {
		return revision.State{}, fmt.Errorf("Invalid key or table value")
	}
	return getRevisionState(table, key)
}

func GetRevisionHistory(key, table string, fromRev, toRev uint64) ([]revision.PatchRecord, revision.State, error) {
	if key == "" || table == "" {
		return nil, revision.State{}, fmt.Errorf("Invalid key or table value")
	}
	return getRevisionHistory(table, key, fromRev, toRev)
}

func patchWithOptionalRevision(key, table string, baseRev *uint64, ops []valuepatch.Operation) ([]byte, error) {
	updated, _, _, _, err := patchWithOptionalRevisionState(key, table, baseRev, ops)
	return updated, err
}

func patchWithOptionalRevisionState(key, table string, baseRev *uint64, ops []valuepatch.Operation) ([]byte, revision.State, *revision.PatchRecord, bool, error) {
	if key == "" || table == "" {
		return nil, revision.State{}, nil, false, fmt.Errorf("Invalid key or table value")
	}
	unlock := valuepatch.LockKey(table, key)
	defer unlock()

	_, hasRevision, err := checkPatchRevision(table, key, baseRev)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	fsData, err := getElementByKey(table, key)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, fmt.Errorf("error retrieving element from map: %w", err)
	}

	data, err := readDataFromFileAsync(table, int64(fsData.StartPtr), int64(fsData.EndPtr))
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	decoded := decode(data)
	updated, err := valuepatch.Apply([]byte(decoded.Data), ops)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	encoded, meta := encode(updated, decoded.HasNested)
	startPtr, endPtr, err := saveDataToFileAsync(encoded, table)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	prevMeta, existed, err := saveElementByKey(table, key, int(startPtr), int(endPtr), meta.HasNested)
	if err != nil {
		_ = markAsFree(key, table, startPtr, endPtr)
		return nil, revision.State{}, nil, hasRevision, err
	}
	if existed {
		if prevMeta.FileName != table || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			_ = markAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			recordDefragFree()
		} else {
			recordDefragSkip()
		}
	}

	networkmanager.NotifyKVTable(table)
	revState, record, hasRevision, err := advancePatchRevision(table, key, baseRev, ops)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}
	if hasRevision && record != nil {
		go notifyTablePatchSubscribersWithRevision(table, key, ops, record.BaseRev, record.Rev)
	} else {
		go notifyTablePatchSubscribers(table, key, ops)
	}
	return updated, revState, record, hasRevision, nil
}
