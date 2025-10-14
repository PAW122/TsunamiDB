package ui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
)

const pointerPrefix = "@ptr:"
const pointerPlaceholder = "*"

type pointerLocation struct {
	ID    string      `json:"id"`
	Path  string      `json:"path"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

type nestedSnapshot struct {
	Resolved interface{}       `json:"resolved,omitempty"`
	Pointers []pointerLocation `json:"pointers"`
}

func buildNestedSnapshot(table string, raw string) (*nestedSnapshot, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	var root interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return nil, fmt.Errorf("parse root json: %w", err)
	}

	resolver := newNestedResolver(table)
	pointers := collectPointerLocations(root, nil, resolver)

	resolved, err := resolver.resolveNode(root, make(map[string]struct{}))
	if err != nil {
		return &nestedSnapshot{Pointers: pointers}, err
	}

	return &nestedSnapshot{
		Resolved: resolved,
		Pointers: pointers,
	}, nil
}

func collectPointerLocations(node interface{}, path []string, resolver *nestedResolver) []pointerLocation {
	out := make([]pointerLocation, 0)

	switch val := node.(type) {
	case map[string]interface{}:
		for k, v := range val {
			out = append(out, collectPointerLocations(v, appendPath(path, k), resolver)...)
		}
	case []interface{}:
		for idx, v := range val {
			segment := fmt.Sprintf("[%d]", idx)
			out = append(out, collectPointerLocations(v, appendPath(path, segment), resolver)...)
		}
	case string:
		id := extractPointerID(val)
		if id == "" {
			return out
		}
		info := pointerLocation{
			ID:   id,
			Path: pathToString(path),
		}
		resolved, err := resolver.resolvePointer(id, make(map[string]struct{}))
		if err != nil {
			info.Error = err.Error()
		} else {
			info.Data = resolved
		}
		out = append(out, info)
	}

	return out
}

func appendPath(path []string, segment string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = segment
	return next
}

func pathToString(path []string) string {
	if len(path) == 0 {
		return ""
	}
	var b strings.Builder
	for i, segment := range path {
		if strings.HasPrefix(segment, "[") {
			b.WriteString(segment)
			continue
		}
		if i > 0 && !strings.HasPrefix(segment, "[") {
			b.WriteRune('.')
		}
		b.WriteString(segment)
	}
	return b.String()
}

func nestedDataFile(table string) string {
	return filepath.Join(table, "nested_values")
}

func extractPointerID(val string) string {
	if strings.HasPrefix(val, pointerPrefix) && len(val) > len(pointerPrefix) {
		return val[len(pointerPrefix):]
	}
	return ""
}

type nestedResolver struct {
	table string
	cache map[string]interface{}
}

func newNestedResolver(table string) *nestedResolver {
	return &nestedResolver{
		table: table,
		cache: make(map[string]interface{}),
	}
}

func (nr *nestedResolver) resolvePointer(id string, visited map[string]struct{}) (interface{}, error) {
	if _, seen := visited[id]; seen {
		return pointerPlaceholder, nil
	}
	if cached, ok := nr.cache[id]; ok {
		return cloneStructure(cached), nil
	}

	meta, err := fileSystem_v1.GetElementByKey(nestedDataFile(nr.table), id)
	if err != nil {
		return nil, err
	}

	raw, err := dataManager_v2.ReadDataFromFileAsync(
		meta.FileName,
		int64(meta.StartPtr),
		int64(meta.EndPtr),
	)
	if err != nil {
		return nil, err
	}

	decoded := encoder_v1.Decode(raw)

	var parsed interface{}
	if err := json.Unmarshal([]byte(decoded.Data), &parsed); err != nil {
		parsed = decoded.Data
	}

	visited[id] = struct{}{}
	resolved, err := nr.resolveNode(parsed, visited)
	delete(visited, id)
	if err != nil {
		return nil, err
	}

	nr.cache[id] = resolved
	return cloneStructure(resolved), nil
}

func (nr *nestedResolver) resolveNode(node interface{}, visited map[string]struct{}) (interface{}, error) {
	switch val := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			resolved, err := nr.resolveNode(v, visited)
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			resolved, err := nr.resolveNode(v, visited)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	case string:
		if id := extractPointerID(val); id != "" {
			return nr.resolvePointer(id, visited)
		}
		return val, nil
	default:
		return val, nil
	}
}

func cloneStructure(node interface{}) interface{} {
	switch val := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			out[k] = cloneStructure(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			out[i] = cloneStructure(v)
		}
		return out
	default:
		return val
	}
}
