package fileSystem_v1

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Database interface {
	Get(ctx context.Context, table, key string) (interface{}, error)
	Put(ctx context.Context, table, key string, value interface{}) error
}

type Manager struct {
	db    Database
	cache sync.Map
}

func NewManager(db Database) *Manager { return &Manager{db: db} }

type resolveEntry struct{ key, table string }

func isPointer(v interface{}) (resolveEntry, bool) {
	a, ok := v.([]interface{})
	if !ok || len(a) != 2 {
		return resolveEntry{}, false
	}
	k, ok1 := a[0].(string)
	t, ok2 := a[1].(string)
	if !ok1 || !ok2 {
		return resolveEntry{}, false
	}
	return resolveEntry{key: k, table: t}, true
}

func (m *Manager) ResolveAll(ctx context.Context, node interface{}, visited map[string]struct{}) (interface{}, error) {
	switch n := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(n))
		for k, v := range n {
			if entry, ok := isPointer(v); ok {
				loopID := entry.table + "/" + entry.key
				if cached, ok := m.cache.Load(loopID); ok {
					out[stripPrefix(k)] = cached
					continue
				}

				if _, seen := visited[loopID]; seen {
					return nil, fmt.Errorf("pointer loop detected at %s", loopID)
				}
				visited[loopID] = struct{}{}

				raw, err := m.db.Get(ctx, entry.table, entry.key)
				if err != nil {
					out[stripPrefix(k)] = nil
					delete(visited, loopID)
					continue
				}
				resolved, err := m.ResolveAll(ctx, raw, visited)
				delete(visited, loopID)
				if err != nil {
					return nil, err
				}
				m.cache.Store(loopID, resolved)
				out[stripPrefix(k)] = resolved
			} else {
				r, err := m.ResolveAll(ctx, v, visited)
				if err != nil {
					return nil, err
				}
				out[k] = r
			}
		}
		return out, nil

	case []interface{}:
		arr := make([]interface{}, len(n))
		for i, v := range n {
			r, err := m.ResolveAll(ctx, v, visited)
			if err != nil {
				return nil, err
			}
			arr[i] = r
		}
		return arr, nil
	default:
		return n, nil
	}
}

func stripPrefix(k string) string {
	const p = "$ptr_"
	if strings.HasPrefix(k, p) {
		return strings.TrimPrefix(k, p)
	}
	return k
}

func ReadPtrAll(ctx context.Context, db Database, table, key string) (map[string]interface{}, error) {
	raw, err := db.Get(ctx, table, key)
	if err != nil {
		return nil, err
	}
	root, ok := raw.(map[string]interface{})
	if !ok {
		return nil, errors.New("expected JSON object at root")
	}
	mgr := NewManager(db)
	resolved, err := mgr.ResolveAll(ctx, root, map[string]struct{}{})
	if err != nil {
		return nil, err
	}
	return resolved.(map[string]interface{}), nil
}

func ReadPtrSome(ctx context.Context, db Database, table, key string, fields []string) (map[string]interface{}, error) {
	raw, err := db.Get(ctx, table, key)
	if err != nil {
		return nil, err
	}
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, errors.New("expected JSON object at root")
	}

	mgr := NewManager(db)
	out := make(map[string]interface{}, len(obj))
	set := make(map[string]struct{})
	for _, f := range fields {
		set[f] = struct{}{}
	}

	for k, v := range obj {
		_, want := set[k]
		if !want {
			_, want = set[stripPrefix(k)]
		}
		if want {
			if entry, ok := isPointer(v); ok {
				rawChild, _ := db.Get(ctx, entry.table, entry.key)
				if rawChild == nil {
					out[stripPrefix(k)] = nil
				} else {
					r, err := mgr.ResolveAll(ctx, rawChild, map[string]struct{}{})
					if err != nil {
						return nil, err
					}
					out[stripPrefix(k)] = r
				}
				continue
			}
		}
		out[k] = v
	}
	return out, nil
}

func CreatePtrObj(ctx context.Context, db Database, table, key string, ptrJson map[string][]string) error {
	for field, target := range ptrJson {
		if !strings.HasPrefix(field, "$ptr_") {
			return fmt.Errorf("pointer field %s must start with $ptr_", field)
		}
		if len(target) != 2 {
			return fmt.Errorf("pointer %s must be [key,table]", field)
		}
		trgKey, trgTable := target[0], target[1]
		if _, err := db.Get(ctx, trgTable, trgKey); err != nil {
			return fmt.Errorf("target %s/%s does not exist", trgTable, trgKey)
		}
	}

	data := make(map[string]interface{}, len(ptrJson))
	for k, v := range ptrJson {
		data[k] = []interface{}{v[0], v[1]}
	}
	return db.Put(ctx, table, key, data)
}
