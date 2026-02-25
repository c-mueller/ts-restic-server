package middleware

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- rDNS cache tests ---

func TestRDNSCache_Eviction(t *testing.T) {
	maxSize := 3
	cache := newRDNSCache(10*time.Minute, maxSize)

	// Fill cache to capacity.
	cache.Set("10.0.0.1", []string{"host1.example.com"})
	cache.Set("10.0.0.2", []string{"host2.example.com"})
	cache.Set("10.0.0.3", []string{"host3.example.com"})

	// All three should be present.
	for i := 1; i <= 3; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		if _, ok := cache.Get(ip); !ok {
			t.Fatalf("expected cache hit for %s", ip)
		}
	}

	// Insert a 4th entry — oldest (10.0.0.1) should be evicted.
	cache.Set("10.0.0.4", []string{"host4.example.com"})

	if _, ok := cache.Get("10.0.0.1"); ok {
		t.Fatal("expected 10.0.0.1 to be evicted, but got cache hit")
	}

	// 2, 3, 4 should still be present.
	for _, ip := range []string{"10.0.0.2", "10.0.0.3", "10.0.0.4"} {
		if _, ok := cache.Get(ip); !ok {
			t.Fatalf("expected cache hit for %s after eviction", ip)
		}
	}

	// Insert a 5th — oldest remaining (10.0.0.2) should be evicted.
	cache.Set("10.0.0.5", []string{"host5.example.com"})

	if _, ok := cache.Get("10.0.0.2"); ok {
		t.Fatal("expected 10.0.0.2 to be evicted, but got cache hit")
	}
	for _, ip := range []string{"10.0.0.3", "10.0.0.4", "10.0.0.5"} {
		if _, ok := cache.Get(ip); !ok {
			t.Fatalf("expected cache hit for %s after second eviction", ip)
		}
	}
}

func TestRDNSCache_TTLExpiry(t *testing.T) {
	cache := newRDNSCache(50*time.Millisecond, 100)

	cache.Set("10.0.0.1", []string{"host1.example.com"})

	// Should be present immediately.
	if _, ok := cache.Get("10.0.0.1"); !ok {
		t.Fatal("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	if _, ok := cache.Get("10.0.0.1"); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestRDNSCache_GetUpdatesNothing(t *testing.T) {
	// Verify that Get does not change FIFO order — this is pure FIFO, not LRU.
	maxSize := 2
	cache := newRDNSCache(10*time.Minute, maxSize)

	cache.Set("10.0.0.1", []string{"host1.example.com"})
	cache.Set("10.0.0.2", []string{"host2.example.com"})

	// Access the oldest entry — should NOT promote it.
	if _, ok := cache.Get("10.0.0.1"); !ok {
		t.Fatal("expected cache hit for 10.0.0.1")
	}

	// Insert a new entry — oldest (10.0.0.1) should still be evicted despite recent Get.
	cache.Set("10.0.0.3", []string{"host3.example.com"})

	if _, ok := cache.Get("10.0.0.1"); ok {
		t.Fatal("expected 10.0.0.1 to be evicted (FIFO, not LRU)")
	}
	if _, ok := cache.Get("10.0.0.2"); !ok {
		t.Fatal("expected cache hit for 10.0.0.2")
	}
	if _, ok := cache.Get("10.0.0.3"); !ok {
		t.Fatal("expected cache hit for 10.0.0.3")
	}
}

func TestRDNSCache_UpdateExistingKey(t *testing.T) {
	maxSize := 2
	cache := newRDNSCache(10*time.Minute, maxSize)

	cache.Set("10.0.0.1", []string{"host1.example.com"})
	cache.Set("10.0.0.2", []string{"host2.example.com"})

	// Update existing key — should move it to the back of FIFO order.
	cache.Set("10.0.0.1", []string{"host1-updated.example.com"})

	// Insert a 3rd entry — 10.0.0.2 (now oldest) should be evicted.
	cache.Set("10.0.0.3", []string{"host3.example.com"})

	if _, ok := cache.Get("10.0.0.2"); ok {
		t.Fatal("expected 10.0.0.2 to be evicted after 10.0.0.1 was updated")
	}

	hostnames, ok := cache.Get("10.0.0.1")
	if !ok {
		t.Fatal("expected cache hit for updated 10.0.0.1")
	}
	if len(hostnames) != 1 || hostnames[0] != "host1-updated.example.com" {
		t.Fatalf("expected updated hostnames, got %v", hostnames)
	}
}

// --- WhoIs cache tests ---

func TestWhoIsCache_Eviction(t *testing.T) {
	maxSize := 3
	cache := newWhoIsCache(10*time.Minute, maxSize)

	// Fill cache to capacity.
	for i := 1; i <= 3; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		cache.Set(ip, []string{ip, fmt.Sprintf("host%d", i)}, &WhoIsResult{
			FQDN: fmt.Sprintf("host%d.tailnet.ts.net", i),
		})
	}

	// All three should be present.
	for i := 1; i <= 3; i++ {
		ip := fmt.Sprintf("10.0.0.%d", i)
		if _, _, ok := cache.Get(ip); !ok {
			t.Fatalf("expected cache hit for %s", ip)
		}
	}

	// Insert a 4th entry — oldest (10.0.0.1) should be evicted.
	cache.Set("10.0.0.4", []string{"10.0.0.4", "host4"}, &WhoIsResult{
		FQDN: "host4.tailnet.ts.net",
	})

	if _, _, ok := cache.Get("10.0.0.1"); ok {
		t.Fatal("expected 10.0.0.1 to be evicted, but got cache hit")
	}

	// 2, 3, 4 should still be present.
	for _, ip := range []string{"10.0.0.2", "10.0.0.3", "10.0.0.4"} {
		if _, _, ok := cache.Get(ip); !ok {
			t.Fatalf("expected cache hit for %s after eviction", ip)
		}
	}
}

func TestWhoIsCache_TTLExpiry(t *testing.T) {
	cache := newWhoIsCache(50*time.Millisecond, 100)

	cache.Set("10.0.0.1", []string{"10.0.0.1", "host1"}, &WhoIsResult{
		FQDN: "host1.tailnet.ts.net",
	})

	// Should be present immediately.
	if _, _, ok := cache.Get("10.0.0.1"); !ok {
		t.Fatal("expected cache hit before TTL expiry")
	}

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	if _, _, ok := cache.Get("10.0.0.1"); ok {
		t.Fatal("expected cache miss after TTL expiry")
	}
}

func TestWhoIsCache_NegativeEntry(t *testing.T) {
	cache := newWhoIsCache(10*time.Minute, 100)

	// Store a negative cache entry (nil result).
	cache.Set("10.0.0.1", []string{"10.0.0.1"}, nil)

	ids, result, ok := cache.Get("10.0.0.1")
	if !ok {
		t.Fatal("expected cache hit for negative entry")
	}
	if result != nil {
		t.Fatal("expected nil result for negative cache entry")
	}
	if len(ids) != 1 || ids[0] != "10.0.0.1" {
		t.Fatalf("expected [10.0.0.1], got %v", ids)
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache := newRDNSCache(10*time.Minute, 100)

	var wg sync.WaitGroup
	const goroutines = 50
	const opsPerGoroutine = 100

	// Half the goroutines write, half read.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				ip := fmt.Sprintf("10.%d.%d.%d", id%256, (i/256)%256, i%256)
				if id%2 == 0 {
					cache.Set(ip, []string{fmt.Sprintf("host-%d-%d.example.com", id, i)})
				} else {
					cache.Get(ip)
				}
			}
		}(g)
	}

	wg.Wait()

	// If we get here without a race detector failure, concurrent access is safe.
}

func TestWhoIsCacheConcurrentAccess(t *testing.T) {
	cache := newWhoIsCache(10*time.Minute, 100)

	var wg sync.WaitGroup
	const goroutines = 50
	const opsPerGoroutine = 100

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				ip := fmt.Sprintf("10.%d.%d.%d", id%256, (i/256)%256, i%256)
				if id%2 == 0 {
					cache.Set(ip, []string{ip}, &WhoIsResult{
						FQDN: fmt.Sprintf("host-%d-%d.tailnet.ts.net", id, i),
					})
				} else {
					cache.Get(ip)
				}
			}
		}(g)
	}

	wg.Wait()
}
