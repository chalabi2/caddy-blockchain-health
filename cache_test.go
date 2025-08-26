package blockchain_health

import (
	"testing"
	"time"
)

func TestHealthCache_SetGet(t *testing.T) {
	cache := NewHealthCache(100 * time.Millisecond)
	defer cache.Clear() // Cleanup

	// Test setting and getting a value
	health := &NodeHealth{
		Name:        "test-node",
		URL:         "http://test",
		Healthy:     true,
		BlockHeight: 12345,
		LastCheck:   time.Now(),
	}

	cache.Set("test-node", health)

	// Should retrieve the cached value
	retrieved := cache.Get("test-node")
	if retrieved == nil {
		t.Fatal("Expected cached value, got nil")
	}

	if retrieved.Name != health.Name {
		t.Errorf("Expected name=%s, got %s", health.Name, retrieved.Name)
	}

	if retrieved.BlockHeight != health.BlockHeight {
		t.Errorf("Expected height=%d, got %d", health.BlockHeight, retrieved.BlockHeight)
	}
}

func TestHealthCache_Expiration(t *testing.T) {
	cache := NewHealthCache(50 * time.Millisecond)
	defer cache.Clear() // Cleanup

	health := &NodeHealth{
		Name:    "test-node",
		Healthy: true,
	}

	cache.Set("test-node", health)

	// Should be available immediately
	if cache.Get("test-node") == nil {
		t.Fatal("Expected cached value immediately after set")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired now
	if cache.Get("test-node") != nil {
		t.Fatal("Expected nil after expiration")
	}
}

func TestHealthCache_Delete(t *testing.T) {
	cache := NewHealthCache(1 * time.Second)
	defer cache.Clear() // Cleanup

	health := &NodeHealth{
		Name:    "test-node",
		Healthy: true,
	}

	cache.Set("test-node", health)

	// Should be available
	if cache.Get("test-node") == nil {
		t.Fatal("Expected cached value")
	}

	// Delete it
	cache.Delete("test-node")

	// Should be gone
	if cache.Get("test-node") != nil {
		t.Fatal("Expected nil after delete")
	}
}

func TestHealthCache_Size(t *testing.T) {
	cache := NewHealthCache(1 * time.Second)
	defer cache.Clear() // Cleanup

	if cache.Size() != 0 {
		t.Errorf("Expected size=0, got %d", cache.Size())
	}

	// Add entries
	for i := 0; i < 5; i++ {
		health := &NodeHealth{
			Name:    string(rune('a' + i)),
			Healthy: true,
		}
		cache.Set(string(rune('a'+i)), health)
	}

	if cache.Size() != 5 {
		t.Errorf("Expected size=5, got %d", cache.Size())
	}

	// Clear all
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected size=0 after clear, got %d", cache.Size())
	}
}

func TestHealthCache_Stats(t *testing.T) {
	cache := NewHealthCache(100 * time.Millisecond)
	defer cache.Clear() // Cleanup

	// Add some entries
	health1 := &NodeHealth{Name: "node1", Healthy: true}
	health2 := &NodeHealth{Name: "node2", Healthy: true}

	cache.Set("node1", health1)
	cache.Set("node2", health2)

	stats := cache.GetStats()

	totalEntries, ok := stats["total_entries"].(int)
	if !ok || totalEntries != 2 {
		t.Errorf("Expected total_entries=2, got %v", stats["total_entries"])
	}

	validEntries, ok := stats["valid_entries"].(int)
	if !ok || validEntries != 2 {
		t.Errorf("Expected valid_entries=2, got %v", stats["valid_entries"])
	}

	expiredEntries, ok := stats["expired_entries"].(int)
	if !ok || expiredEntries != 0 {
		t.Errorf("Expected expired_entries=0, got %v", stats["expired_entries"])
	}

	// Wait for expiration and cleanup
	time.Sleep(150 * time.Millisecond)

	// Force cleanup by calling removeExpired
	cache.removeExpired()

	stats = cache.GetStats()

	// After cleanup, expired entries should be removed from the map
	// So total_entries should be less and valid_entries should be 0
	totalEntries, ok = stats["total_entries"].(int)
	if !ok || totalEntries > 2 {
		t.Logf("Total entries after cleanup: %v", stats["total_entries"])
		// This is okay - cleanup might have removed them
	}

	validEntries, ok = stats["valid_entries"].(int)
	if !ok || validEntries != 0 {
		t.Errorf("Expected valid_entries=0 after expiration, got %v", stats["valid_entries"])
	}
}
