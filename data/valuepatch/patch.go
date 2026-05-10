package valuepatch

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNoOps         = errors.New("patch: empty ops")
	ErrInvalidOffset = errors.New("patch: invalid offset")
	ErrInvalidDelete = errors.New("patch: invalid delete length")
	ErrInvalidInsert = errors.New("patch: insert and insert_b64 are mutually exclusive")
)

type Operation struct {
	Offset       int    `json:"offset"`
	Delete       int    `json:"delete,omitempty"`
	Insert       string `json:"insert,omitempty"`
	InsertBase64 string `json:"insert_b64,omitempty"`
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

var keyLocks = struct {
	sync.Mutex
	locks map[string]*keyedLock
}{
	locks: make(map[string]*keyedLock),
}

func LockKey(table, key string) func() {
	lockKey := table + "\x00" + key

	keyLocks.Lock()
	l := keyLocks.locks[lockKey]
	if l == nil {
		l = &keyedLock{}
		keyLocks.locks[lockKey] = l
	}
	l.refs++
	keyLocks.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()

		keyLocks.Lock()
		l.refs--
		if l.refs == 0 {
			delete(keyLocks.locks, lockKey)
		}
		keyLocks.Unlock()
	}
}

func Apply(base []byte, ops []Operation) ([]byte, error) {
	if len(ops) == 0 {
		return nil, ErrNoOps
	}

	out := append([]byte(nil), base...)
	for i, op := range ops {
		insert, err := opInsertBytes(op)
		if err != nil {
			return nil, fmt.Errorf("op %d: %w", i, err)
		}
		if op.Offset < 0 || op.Offset > len(out) {
			return nil, fmt.Errorf("op %d: %w", i, ErrInvalidOffset)
		}
		if op.Delete < 0 || op.Offset+op.Delete > len(out) {
			return nil, fmt.Errorf("op %d: %w", i, ErrInvalidDelete)
		}

		next := make([]byte, 0, len(out)-op.Delete+len(insert))
		next = append(next, out[:op.Offset]...)
		next = append(next, insert...)
		next = append(next, out[op.Offset+op.Delete:]...)
		out = next
	}

	return out, nil
}

func opInsertBytes(op Operation) ([]byte, error) {
	if op.Insert != "" && op.InsertBase64 != "" {
		return nil, ErrInvalidInsert
	}
	if op.InsertBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(op.InsertBase64)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInsert, err)
		}
		return data, nil
	}
	return []byte(op.Insert), nil
}
