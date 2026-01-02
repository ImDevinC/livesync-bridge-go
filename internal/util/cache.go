package util

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

// Cache is a thread-safe LRU cache for storing file hashes
type Cache struct {
	cache *lru.Cache[string, string]
}

// NewCache creates a new LRU cache with the specified size
func NewCache(size int) (*Cache, error) {
	cache, err := lru.New[string, string](size)
	if err != nil {
		return nil, err
	}
	return &Cache{cache: cache}, nil
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (string, bool) {
	return c.cache.Get(key)
}

// Set adds a value to the cache
func (c *Cache) Set(key, value string) {
	c.cache.Add(key, value)
}

// Has checks if a key exists in the cache
func (c *Cache) Has(key string) bool {
	return c.cache.Contains(key)
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.cache.Purge()
}

// Len returns the number of items in the cache
func (c *Cache) Len() int {
	return c.cache.Len()
}
