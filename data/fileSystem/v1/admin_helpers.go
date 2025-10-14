package fileSystem_v1

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ListTables returns the known logical table names.
// It combines already-loaded indices with table names discovered on disk.
func ListTables() ([]string, error) {
	seen := make(map[string]struct{})

	// Tables already loaded in memory.
	registryMu.RLock()
	for name := range indexRegistry {
		if strings.TrimSpace(name) != "" {
			seen[name] = struct{}{}
		}
	}
	registryMu.RUnlock()

	entries, err := os.ReadDir(baseMapsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(baseMapsDir, entry.Name())
		name := tableNameFromDir(dir, entry.Name())
		if strings.TrimSpace(name) != "" {
			seen[name] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func tableNameFromDir(dir, fallback string) string {
	if name := tableNameFromSnapshot(filepath.Join(dir, "index.snap")); name != "" {
		return name
	}
	if name := tableNameFromWal(filepath.Join(dir, "index.wal")); name != "" {
		return name
	}
	return fallback
}

func tableNameFromSnapshot(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			name := strings.TrimSpace(parts[1])
			if name != "" {
				return name
			}
		}
	}
	return ""
}

func tableNameFromWal(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 3 && parts[0] == "S" {
			name := strings.TrimSpace(parts[2])
			if name != "" {
				return name
			}
		}
	}
	return ""
}

// ListTableEntries returns the metadata for keys in the given table.
// When limit <= 0 the entire result set is returned.
// The returned next cursor is empty when there are no more keys.
func ListTableEntries(table, after string, limit int, filter *regexp.Regexp) ([]GetElement_output, string, error) {
	idx, err := getTableIndex(table)
	if err != nil {
		return nil, "", err
	}

	keys := idx.snapshotKeys()
	if filter != nil {
		dst := keys[:0]
		for _, k := range keys {
			if filter.MatchString(k) {
				dst = append(dst, k)
			}
		}
		keys = dst
	}

	sort.Strings(keys)

	start := 0
	if after != "" {
		pos := sort.SearchStrings(keys, after)
		for pos < len(keys) && keys[pos] <= after {
			pos++
		}
		start = pos
	}
	if start >= len(keys) {
		return []GetElement_output{}, "", nil
	}

	subset := keys[start:]
	nextCursor := ""
	if limit > 0 && len(subset) > limit {
		nextCursor = subset[limit-1]
		subset = subset[:limit]
	}

	results := make([]GetElement_output, 0, len(subset))
	for _, key := range subset {
		if entry, ok := idx.loadEntry(key); ok {
			results = append(results, entryToOutput(key, entry))
		}
	}

	return results, nextCursor, nil
}
