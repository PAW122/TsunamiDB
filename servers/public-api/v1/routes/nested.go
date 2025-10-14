package routes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

const (
	pointerPrefix           = "@ptr:"
	pointerUnresolvedMarker = "*"
)

type pendingNestedEntry struct {
	PointerID string
	JSONValue []byte
	HasNested bool
}

func nestedDataFile(table string) string {
	return filepath.Join(table, "nested_values")
}

func isPointerPlaceholder(val string) bool {
	return strings.HasPrefix(val, pointerPrefix) && len(val) > len(pointerPrefix)
}

func extractPointerID(val string) string {
	if !isPointerPlaceholder(val) {
		return ""
	}
	return val[len(pointerPrefix):]
}

func generatePointerID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("ptr_%s", hex.EncodeToString(buf)), nil
}

func processNestedPayload(raw []byte) ([]byte, []pendingNestedEntry, error) {
	if len(raw) == 0 {
		return raw, nil, nil
	}

	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, err
	}

	processed, entries, err := walkAndExtractNested(payload)
	if err != nil {
		return nil, nil, err
	}

	normalized, err := json.Marshal(processed)
	if err != nil {
		return nil, nil, err
	}
	return normalized, entries, nil
}

func walkAndExtractNested(node interface{}) (interface{}, []pendingNestedEntry, error) {
	switch val := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		var entries []pendingNestedEntry
		for k, v := range val {
			processed, nestedEntries, err := walkAndExtractNested(v)
			if err != nil {
				return nil, nil, err
			}
			out[k] = processed
			if len(nestedEntries) > 0 {
				entries = append(entries, nestedEntries...)
			}
		}
		return out, entries, nil
	case []interface{}:
		out := make([]interface{}, len(val))
		var entries []pendingNestedEntry
		for i, v := range val {
			processed, nestedEntries, err := walkAndExtractNested(v)
			if err != nil {
				return nil, nil, err
			}
			out[i] = processed
			if len(nestedEntries) > 0 {
				entries = append(entries, nestedEntries...)
			}
		}
		return out, entries, nil
	case string:
		if strings.HasPrefix(val, "@") && len(val) > 1 {
			payload := strings.TrimSpace(val[1:])
			nestedValue := parseNestedSource(payload)
			processedNested, nestedEntries, err := walkAndExtractNested(nestedValue)
			if err != nil {
				return nil, nil, err
			}
			pointerID, err := generatePointerID()
			if err != nil {
				return nil, nil, err
			}
			nestedJSON, err := json.Marshal(processedNested)
			if err != nil {
				return nil, nil, err
			}
			entry := pendingNestedEntry{
				PointerID: pointerID,
				JSONValue: nestedJSON,
				HasNested: len(nestedEntries) > 0,
			}
			combined := append(nestedEntries, entry)
			return pointerPrefix + pointerID, combined, nil
		}
		return val, nil, nil
	default:
		return node, nil, nil
	}
}

func parseNestedSource(src string) interface{} {
	if src == "" {
		return ""
	}
	var out interface{}
	if err := json.Unmarshal([]byte(src), &out); err == nil {
		return out
	}
	return src
}

func pointerIDsFromJSON(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	ids := make(map[string]struct{})
	collectPointerIDs(payload, ids)
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func collectPointerIDs(node interface{}, acc map[string]struct{}) {
	switch val := node.(type) {
	case map[string]interface{}:
		for _, v := range val {
			collectPointerIDs(v, acc)
		}
	case []interface{}:
		for _, v := range val {
			collectPointerIDs(v, acc)
		}
	case string:
		if id := extractPointerID(val); id != "" {
			acc[id] = struct{}{}
		}
	}
}

func parsePathHeader(value string) ([][]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "\"") && strings.HasSuffix(trimmed, "\"") && len(trimmed) >= 2 {
		trimmed = strings.Trim(trimmed, "\"")
	}

	var raw []string
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, err
		}
	} else if strings.Contains(trimmed, ",") {
		raw = strings.Split(trimmed, ",")
	} else {
		raw = strings.Fields(trimmed)
	}

	paths := make([][]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		segments := strings.Split(item, ".")
		cleaned := make([]string, 0, len(segments))
		for _, seg := range segments {
			seg = strings.TrimSpace(seg)
			if seg != "" {
				cleaned = append(cleaned, seg)
			}
		}
		if len(cleaned) > 0 {
			paths = append(paths, cleaned)
		}
	}
	return paths, nil
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

func cloneWithPlaceholders(node interface{}) interface{} {
	switch val := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			out[k] = cloneWithPlaceholders(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			out[i] = cloneWithPlaceholders(v)
		}
		return out
	case string:
		if extractPointerID(val) != "" {
			return pointerUnresolvedMarker
		}
		return val
	default:
		return val
	}
}

func pointerIDAtPath(node interface{}, path []string) (string, bool) {
	current := node
	for _, segment := range path {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", false
		}
		next, exists := m[segment]
		if !exists {
			return "", false
		}
		current = next
	}
	if str, ok := current.(string); ok {
		if id := extractPointerID(str); id != "" {
			return id, true
		}
	}
	return "", false
}

func setValueAtPath(target interface{}, path []string, value interface{}) interface{} {
	if len(path) == 0 {
		return value
	}

	root, ok := target.(map[string]interface{})
	if !ok || root == nil {
		root = make(map[string]interface{})
	}

	current := root
	for i, segment := range path {
		if i == len(path)-1 {
			current[segment] = value
			break
		}
		next, exists := current[segment]
		nextMap, mapOK := next.(map[string]interface{})
		if !exists || !mapOK {
			nextMap = make(map[string]interface{})
			current[segment] = nextMap
		}
		current = nextMap
	}
	return root
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
		return pointerUnresolvedMarker, nil
	}
	if val, ok := nr.cache[id]; ok {
		return cloneStructure(val), nil
	}

	nestedFile := nestedDataFile(nr.table)
	meta, err := fileSystem_v1.GetElementByKey(nestedFile, id)
	if err != nil {
		return nil, err
	}
	raw, err := dataManager_v2.ReadDataFromFileAsync(meta.FileName, int64(meta.StartPtr), int64(meta.EndPtr))
	if err != nil {
		return nil, err
	}
	decoded := encoder_v1.Decode(raw)
	var parsed interface{}
	if err := json.Unmarshal([]byte(decoded.Data), &parsed); err != nil {
		return nil, err
	}

	visited[id] = struct{}{}
	resolved, err := resolveNodeFully(nr, parsed, visited)
	delete(visited, id)
	if err != nil {
		return nil, err
	}
	nr.cache[id] = resolved
	return cloneStructure(resolved), nil
}

func resolveNodeFully(nr *nestedResolver, node interface{}, visited map[string]struct{}) (interface{}, error) {
	switch val := node.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, v := range val {
			resolved, err := resolveNodeFully(nr, v, visited)
			if err != nil {
				return nil, err
			}
			out[k] = resolved
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			resolved, err := resolveNodeFully(nr, v, visited)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	case string:
		if id := extractPointerID(val); id != "" {
			resolved, err := nr.resolvePointer(id, visited)
			if err != nil {
				return nil, err
			}
			return resolved, nil
		}
		return val, nil
	default:
		return val, nil
	}
}

func resolveAllNested(table string, root interface{}) (interface{}, error) {
	resolver := newNestedResolver(table)
	return resolveNodeFully(resolver, root, make(map[string]struct{}))
}

func resolveNestedPaths(table string, root interface{}, paths [][]string) (interface{}, error) {
	if len(paths) == 0 {
		return cloneWithPlaceholders(root), nil
	}

	base := cloneWithPlaceholders(root)
	resolver := newNestedResolver(table)
	visited := make(map[string]struct{})

	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		id, ok := pointerIDAtPath(root, path)
		if !ok {
			continue
		}
		resolved, err := resolver.resolvePointer(id, visited)
		if err != nil {
			return nil, err
		}
		base = setValueAtPath(base, path, resolved)
	}

	return base, nil
}

func extractNestedOnly(table string, root interface{}, paths [][]string) (interface{}, error) {
	resolver := newNestedResolver(table)
	visited := make(map[string]struct{})

	var result interface{} = make(map[string]interface{})
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		id, ok := pointerIDAtPath(root, path)
		if !ok {
			continue
		}
		resolved, err := resolver.resolvePointer(id, visited)
		if err != nil {
			return nil, err
		}
		result = setValueAtPath(result, path, resolved)
	}
	return result, nil
}

func cleanupRecordNested(table string, meta fileSystem_v1.GetElement_output) {
	raw, err := dataManager_v2.ReadDataFromFileAsync(meta.FileName, int64(meta.StartPtr), int64(meta.EndPtr))
	if err != nil {
		debug.LogExtra(fmt.Sprintf("cleanup nested read error (table=%s key=%s): %v", table, meta.Key, err))
		return
	}
	decoded := encoder_v1.Decode(raw)
	ids, err := pointerIDsFromJSON([]byte(decoded.Data))
	if err != nil {
		debug.LogExtra(fmt.Sprintf("cleanup nested parse error (table=%s key=%s): %v", table, meta.Key, err))
		return
	}
	if len(ids) == 0 {
		return
	}
	visited := make(map[string]struct{})
	cleanupNestedPointers(table, ids, visited)
}

func cleanupNestedPointers(table string, pointerIDs []string, visited map[string]struct{}) {
	if len(pointerIDs) == 0 {
		return
	}
	nestedFile := nestedDataFile(table)
	for _, id := range pointerIDs {
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}

		pointerMeta, err := fileSystem_v1.GetElementByKey(nestedFile, id)
		if err == nil {
			raw, err := dataManager_v2.ReadDataFromFileAsync(pointerMeta.FileName, int64(pointerMeta.StartPtr), int64(pointerMeta.EndPtr))
			if err == nil {
				decoded := encoder_v1.Decode(raw)
				childIDs, err := pointerIDsFromJSON([]byte(decoded.Data))
				if err == nil && len(childIDs) > 0 {
					cleanupNestedPointers(table, childIDs, visited)
				}
			}
			_ = fileSystem_v1.RemoveElementByKey(nestedFile, id)
			_ = defragmentationManager.MarkAsFree(id, pointerMeta.FileName, int64(pointerMeta.StartPtr), int64(pointerMeta.EndPtr))
		}
	}
}
