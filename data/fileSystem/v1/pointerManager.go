package fileSystem_v1

// Pointer Manager + High-Level API dla pointerów w TsunamiDB.
// -----------------------------------------------------------
// Moduł dostarcza:
//   • niskopoziomowy rekurencyjny resolver pointerów (eager/lazy),
//   • funkcje ReadPtrAll, ReadPtrSome – zgodne z wymaganiami użytkownika,
//   • funkcję CreatePtrObj do tworzenia obiektów pointera z walidacją.
//
// Format pointera zapisany w bazie:
//   "$ptr_<custom_name>": ["key", "table"]
// Klucz musi zaczynać się od "$ptr_".  W wyniku ReadPtrAll / ReadPtrSome
// nazwa pola jest zwracana *bez* tego prefiksu.
//
// Zależności od reszty TsunamiDB są wyrażone przez interfejs Database ↓
// – podłącz tu swój konkretny storage.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// -----------------------------------------------------------------------------
//  Minimalny interfejs do TsunamiDB (wstrzykuj konkretną implementację)
// -----------------------------------------------------------------------------

type Database interface {
	// Get zwraca niezdeserializowany obiekt JSON (map[string]any lub []any).
	Get(ctx context.Context, table, key string) (interface{}, error)

	// Put zapisuje obiekt pod dany klucz.
	Put(ctx context.Context, table, key string, value interface{}) error
}

// -----------------------------------------------------------------------------
//  Niskopoziomowy resolver pointerów (eager)
// -----------------------------------------------------------------------------

type Manager struct {
	db    Database
	cache sync.Map // map[string]interface{}
}

func NewManager(db Database) *Manager { return &Manager{db: db} }

// resolveEntry to {key,table} pointer.

type resolveEntry struct{ key, table string }

// isPointer sprawdza czy v jest tablicą [key, table].
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

// ResolveAll – rekurencyjnie rozwija pointery (używana przez ReadPtrAll).
func (m *Manager) ResolveAll(ctx context.Context, node interface{}, visited map[string]struct{}) (interface{}, error) {
	switch n := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(n))
		for k, v := range n {
			if entry, ok := isPointer(v); ok {
				// Cykl?
				loopID := entry.table + "/" + entry.key
				if _, seen := visited[loopID]; seen {
					return nil, fmt.Errorf("pointer loop detected at %s", loopID)
				}
				visited[loopID] = struct{}{}

				// Spróbuj z cache.
				if cached, ok := m.cache.Load(loopID); ok {
					out[stripPrefix(k)] = cached
					continue
				}

				raw, err := m.db.Get(ctx, entry.table, entry.key)
				if err != nil {
					// jeśli brak – zwróć null (nil).
					out[stripPrefix(k)] = nil
					continue
				}
				resolved, err := m.ResolveAll(ctx, raw, visited)
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

// stripPrefix usuwa "$ptr_" jeśli występuje.
func stripPrefix(k string) string {
	const p = "$ptr_"
	if strings.HasPrefix(k, p) {
		return strings.TrimPrefix(k, p)
	}
	return k
}

// -----------------------------------------------------------------------------
//  Publiczne API zgodne z koncepcją użytkownika
// -----------------------------------------------------------------------------

// ReadPtrAll – pobiera obiekt i rozwija wszystkie pointery.  Braki ⇒ null.
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

// ReadPtrSome – rozwija tylko wybrane pola.  Inne pozostają surowe.
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
		if _, want := set[k]; want {
			if entry, ok := isPointer(v); ok {
				// resolve
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
		// kopiuj w stanie niezmienionym
		out[k] = v
	}
	return out, nil
}

// CreatePtrObj – zapisuje "obiekt pointera" (ptrJson) pod (table,key).
// ptrJson musi być map[string][]string (dokładnie 2 elementy: key, table).
// Walidacja: unikalne klucze + istnienie wskazywanych rekordów.
func CreatePtrObj(ctx context.Context, db Database, table, key string, ptrJson map[string][]string) error {
	seen := make(map[string]struct{})
	for field, target := range ptrJson {
		if !strings.HasPrefix(field, "$ptr_") {
			return fmt.Errorf("pointer field %s must start with $ptr_", field)
		}
		if _, dup := seen[field]; dup {
			return fmt.Errorf("duplicate pointer field %s", field)
		}
		seen[field] = struct{}{}

		if len(target) != 2 {
			return fmt.Errorf("pointer %s must be [key,table]", field)
		}
		trgKey, trgTable := target[0], target[1]
		if _, err := db.Get(ctx, trgTable, trgKey); err != nil {
			return fmt.Errorf("target %s/%s does not exist", trgTable, trgKey)
		}
	}
	// Konwersja []string → []interface{} aby trzymać się jednego typu JSON.
	data := make(map[string]interface{}, len(ptrJson))
	for k, v := range ptrJson {
		data[k] = []interface{}{v[0], v[1]}
	}
	return db.Put(ctx, table, key, data)
}
