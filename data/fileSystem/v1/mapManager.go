package fileSystem_v1

import (
	"crypto/sha1"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	lru "github.com/hashicorp/golang-lru"
)

type ShardConfig struct {
	NumShards int `json:"num_shards"`
	MinShards int `json:"min_shards"`
	MaxShards int `json:"max_shards"`
}

var (
	configPath   = "./db/maps/shard_config.json"
	configMu     sync.RWMutex
	shardConfig  *ShardConfig
	shardCache   *lru.Cache // LRU cache na shardy
	shardCacheMu sync.Mutex
	cacheSize    = 32
)

type GetElement_output struct {
	Key      string
	FileName string
	StartPtr int
	EndPtr   int
}

// --- Config loader ---

func loadShardConfig() (*ShardConfig, error) {
	configMu.Lock()
	defer configMu.Unlock()
	f, err := os.Open(configPath)
	if err != nil {
		// plik nie istnieje — utwórz defaultowy config!
		fmt.Println("[MapManager] Brak shard_config.json — tworzę domyślny")
		cfg := &ShardConfig{
			NumShards: 8,
			MinShards: 8,
			MaxShards: 256,
		}
		if err := saveShardConfig(cfg); err != nil {
			return nil, err
		}
		shardConfig = cfg
		return cfg, nil
	}
	defer f.Close()
	var cfg ShardConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	shardConfig = &cfg
	return &cfg, nil
}

func saveShardConfig(cfg *ShardConfig) error {
	configMu.Lock()
	defer configMu.Unlock()
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(cfg)
}

func getNumShards() int {
	configMu.RLock()
	defer configMu.RUnlock()
	return shardConfig.NumShards
}

// --- Shard utils ---

func shardIdForKeyWithN(key string, n int) string {
	h := sha1.Sum([]byte(key))
	idx := int(h[0]) % n
	return fmt.Sprintf("%02x", idx)
}
func shardIdForKey(key string) string {
	return shardIdForKeyWithN(key, getNumShards())
}
func allShardIds() []string {
	n := getNumShards()
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf("%02x", i)
	}
	return out
}

func shardBasePath(shardId string) string {
	return fmt.Sprintf("./db/maps/data_map_%s_base.gob", shardId)
}

// --- Map loader/saver ---

func loadShardMapFromDisk(shardId string) map[string]GetElement_output {
	m := make(map[string]GetElement_output)
	base := shardBasePath(shardId)
	loadGob(base, m)
	return m
}

func saveShardMapToDisk(shardId string, m map[string]GetElement_output) error {
	base := shardBasePath(shardId)
	return saveGob(base, m)
}

func loadGob(path string, m map[string]GetElement_output) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	dec := gob.NewDecoder(f)
	for {
		var entry GetElement_output
		if err := dec.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			return
		}
		m[entry.Key] = entry
	}
}

func saveGob(path string, m map[string]GetElement_output) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := gob.NewEncoder(f)
	for _, v := range m {
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	return nil
}

// --- LRU cache na shardy ---

type shardEntry struct {
	mu   sync.RWMutex
	Data map[string]GetElement_output
	// można rozbudować o dirty, deltaBuffer itd.
}

func loadShardToCache(shardId string) *shardEntry {
	shardCacheMu.Lock()
	defer shardCacheMu.Unlock()
	if v, ok := shardCache.Get(shardId); ok {
		return v.(*shardEntry)
	}
	m := &shardEntry{
		Data: loadShardMapFromDisk(shardId),
	}
	shardCache.Add(shardId, m)
	return m
}

// --- API ---

func SaveElementByKey(key, fileName string, startPtr, endPtr int) error {
	shardId := shardIdForKey(key)
	entry := loadShardToCache(shardId)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	elem := GetElement_output{
		Key:      key,
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
	}
	entry.Data[key] = elem
	// Save to disk (prosto, można zoptymalizować batch/flush L1-L2)
	return saveShardMapToDisk(shardId, entry.Data)
}

func RemoveElementByKey(key string) error {
	shardId := shardIdForKey(key)
	entry := loadShardToCache(shardId)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	_, ok := entry.Data[key]
	if !ok {
		return errors.New("key not found")
	}
	// TODO: MarkFree logic here (for free-list)
	delete(entry.Data, key)
	return saveShardMapToDisk(shardId, entry.Data)
}

func GetElementByKey(key string) (*GetElement_output, error) {
	shardId := shardIdForKey(key)
	entry := loadShardToCache(shardId)
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	elem, ok := entry.Data[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return &elem, nil
}

func GetKeysByRegex(regex string, max int) ([]string, error) {
	result := []string{}
	re, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	for _, shardId := range allShardIds() {
		entry := loadShardToCache(shardId)
		entry.mu.RLock()
		for key := range entry.Data {
			if re.MatchString(key) {
				result = append(result, key)
				if max > 0 && len(result) >= max {
					entry.mu.RUnlock()
					return result, nil
				}
			}
		}
		entry.mu.RUnlock()
	}
	return result, nil
}

// --- Re-sharding (dynamiczne zwiększanie liczby shardów) ---

func reshardDB(newNumShards int) error {
	fmt.Println("==> ROZPOCZYNAM MIGRACJĘ SHARDÓW: nowa liczba =", newNumShards)
	oldNum := getNumShards()
	if newNumShards == oldNum {
		return nil // nic nie zmieniamy
	}
	// 1. Wczytaj wszystkie stare shardy do pamięci
	allData := map[string]GetElement_output{}
	for _, oldShardId := range allShardIds() {
		m := loadShardMapFromDisk(oldShardId)
		for k, v := range m {
			allData[k] = v
		}
	}
	// 2. Przygotuj nowe mapy
	newMaps := map[string]map[string]GetElement_output{}
	for k, v := range allData {
		sid := shardIdForKeyWithN(k, newNumShards)
		if newMaps[sid] == nil {
			newMaps[sid] = make(map[string]GetElement_output)
		}
		newMaps[sid][k] = v
	}
	// 3. Zapisz nowe mapy (base) na dysk
	for sid, m := range newMaps {
		if err := saveShardMapToDisk(sid, m); err != nil {
			return err
		}
	}
	// 4. Skasuj stare mapy (base)
	oldFiles, _ := filepath.Glob("./db/maps/data_map_??_base.gob")
	for _, file := range oldFiles {
		os.Remove(file)
	}
	// 5. Zaktualizuj config
	shardConfig.NumShards = newNumShards
	if err := saveShardConfig(shardConfig); err != nil {
		return err
	}
	fmt.Println("==> MIGRACJA SHARDÓW ZAKOŃCZONA")
	return nil
}

func maybeReshard() error {
	// policz sumarycznie ilość kluczy
	total := 0
	for _, sid := range allShardIds() {
		m := loadShardMapFromDisk(sid)
		total += len(m)
	}
	kluczeNaShard := total / getNumShards()
	if kluczeNaShard > 10000 && getNumShards() < shardConfig.MaxShards {
		newShards := getNumShards() * 2
		if newShards > shardConfig.MaxShards {
			newShards = shardConfig.MaxShards
		}
		return reshardDB(newShards)
	}
	return nil
}

// --- Inicjalizacja ---

func InitMapManager() {
	cfg, err := loadShardConfig()
	if err != nil {
		panic("Brak configa shardów: " + err.Error())
	}
	fmt.Println("[MapManager] Liczba shardów:", cfg.NumShards)
	c, err := lru.New(cacheSize)
	if err != nil {
		panic("Nie można utworzyć LRU cache na shardy: " + err.Error())
	}
	shardCache = c
}
