package cache

// import obj z fs
import (
	types "github.com/PAW122/TsunamiDB/types"
	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	lruCacheSize = 500_000
)

var (
	lruCache *lru.Cache[string, types.GetElement_output]
)

func init() {
	lruCache, _ = lru.New[string, types.GetElement_output](lruCacheSize)
}

func AddElement(key string, value types.GetElement_output) {
	lruCache.Add(key, value)
}

func GetElement(key string) (types.GetElement_output, bool) {
	return lruCache.Get(key)
}

func RemoveElement(key string) {
	lruCache.Remove(key)
}

func ClearCache() {
	lruCache.Purge()
}

func ResizeCache(size int) {
	lruCache.Resize(size)
}

func GetCacheSize() int {
	return lruCache.Len()
}
