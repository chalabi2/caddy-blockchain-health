package blockchain_health

import (
	"time"
)

// NewHealthCache creates a new health cache with the specified duration
func NewHealthCache(duration time.Duration) *HealthCache {
	cache := &HealthCache{
		cache:    make(map[string]*CacheEntry),
		duration: duration,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a cached health result
func (hc *HealthCache) Get(nodeName string) *NodeHealth {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	entry, exists := hc.cache[nodeName]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		// Don't delete here to avoid write lock, let cleanup handle it
		return nil
	}

	return entry.Health
}

// Set stores a health result in the cache
func (hc *HealthCache) Set(nodeName string, health *NodeHealth) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	entry := &CacheEntry{
		Health:    health,
		ExpiresAt: time.Now().Add(hc.duration),
	}

	hc.cache[nodeName] = entry
}

// Delete removes a cached entry
func (hc *HealthCache) Delete(nodeName string) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	delete(hc.cache, nodeName)
}

// Clear removes all cached entries
func (hc *HealthCache) Clear() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	hc.cache = make(map[string]*CacheEntry)
}

// Size returns the number of cached entries
func (hc *HealthCache) Size() int {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	return len(hc.cache)
}

// cleanup periodically removes expired entries
func (hc *HealthCache) cleanup() {
	ticker := time.NewTicker(hc.duration / 2) // Cleanup twice per cache duration
	defer ticker.Stop()

	for range ticker.C {
		hc.removeExpired()
	}
}

// removeExpired removes all expired entries from the cache
func (hc *HealthCache) removeExpired() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	now := time.Now()
	for nodeName, entry := range hc.cache {
		if now.After(entry.ExpiresAt) {
			delete(hc.cache, nodeName)
		}
	}
}

// GetStats returns cache statistics
func (hc *HealthCache) GetStats() map[string]interface{} {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	stats := make(map[string]interface{})
	stats["total_entries"] = len(hc.cache)
	stats["cache_duration"] = hc.duration.String()

	// Count expired entries
	now := time.Now()
	expiredCount := 0
	for _, entry := range hc.cache {
		if now.After(entry.ExpiresAt) {
			expiredCount++
		}
	}
	stats["expired_entries"] = expiredCount
	stats["valid_entries"] = len(hc.cache) - expiredCount

	return stats
}
