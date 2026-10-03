package tmdb

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func cacheForTest(t *testing.T, config CacheConfig) *responseCache {
	t.Helper()
	cache, err := newResponseCache(config)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}

func testResponseKey(value string) responseKey { return responseKey(sha256.Sum256([]byte(value))) }

func TestCacheByteAndEntryBudgetsUseLRU(t *testing.T) {
	for _, config := range []CacheConfig{{MaxBytes: 8, MaxEntries: 10}, {MaxBytes: 100, MaxEntries: 2}} {
		cache := cacheForTest(t, config)
		client := &Client{cache: cache}
		loads := map[string]int{}
		get := func(key string) {
			t.Helper()
			if _, err := cache.get(context.Background(), testResponseKey(key), func(context.Context) ([]byte, error) {
				loads[key]++
				return []byte("data"), nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		get("a")
		get("b")
		get("a") // a 最近使用，c 入缓存必须淘汰 b。
		get("c")
		get("a")
		if stats := client.CacheStats(); stats.Entries != 2 || stats.Bytes != 8 {
			t.Fatalf("cache exceeded budget: %+v", stats)
		}
		get("b")
		if loads["a"] != 1 || loads["b"] != 2 || loads["c"] != 1 {
			t.Fatalf("wrong LRU eviction: %+v", loads)
		}
	}
}

func TestCacheOversizeBypassAndExactBufferSize(t *testing.T) {
	cache := cacheForTest(t, CacheConfig{MaxBytes: 4})
	client := &Client{cache: cache}
	large := func(context.Context) ([]byte, error) { return []byte("large"), nil }
	for i := 0; i < 2; i++ {
		if _, err := cache.get(context.Background(), testResponseKey("large"), large); err != nil {
			t.Fatal(err)
		}
		if client.CacheStats() != (CacheStats{}) {
			t.Fatal("oversize response entered cache")
		}
	}
	buffer := make([]byte, 4, 4096)
	copy(buffer, "data")
	data, err := cache.get(context.Background(), testResponseKey("small"), func(context.Context) ([]byte, error) { return buffer, nil })
	if err != nil || cap(data) != len(data) || client.CacheStats().Bytes != 4 {
		t.Fatalf("retained unaccounted buffer capacity: len=%d cap=%d err=%v", len(data), cap(data), err)
	}
	buffer[0] = '!'
	if string(data) != "data" {
		t.Fatal("cache retained loader's mutable buffer")
	}
}

func TestCacheTTLStartsAtCompletionAndDoesNotSlide(t *testing.T) {
	cache := cacheForTest(t, CacheConfig{TTL: time.Minute})
	client := &Client{cache: cache}
	now := time.Unix(1000, 0)
	cache.now = func() time.Time { return now }
	loads := 0
	get := func() {
		t.Helper()
		if _, err := cache.get(context.Background(), testResponseKey("a"), func(context.Context) ([]byte, error) {
			loads++
			now = now.Add(time.Minute) // 网络请求时间不计入缓存 TTL。
			return []byte("data"), nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	get()
	now = now.Add(30 * time.Second)
	get()
	if loads != 1 {
		t.Fatal("TTL started before response completion")
	}
	now = now.Add(30 * time.Second)
	if stats := client.CacheStats(); stats != (CacheStats{}) {
		t.Fatalf("expired response still counted: %+v", stats)
	}
	get()
	if loads != 2 {
		t.Fatal("cache hit extended TTL")
	}
}

func waitForCacheWaiters(t *testing.T, cache *responseCache, key responseKey, count int) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		cache.mu.Lock()
		flight := cache.flights[key]
		ready := flight != nil && flight.waiters == count
		cache.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("did not get %d concurrent waiters", count)
		case <-tick.C:
		}
	}
}

func TestCacheCoalescesConcurrentMissesAndIsolatesCancellation(t *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "follower", true: "initiator"}[cancelFirst], func(t *testing.T) {
			cache := cacheForTest(t, CacheConfig{})
			key := testResponseKey("show")
			ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			first, cancelFirstCall := context.WithCancel(ctx)
			defer cancelFirstCall()
			follower, cancelFollower := context.WithCancel(ctx)
			defer cancelFollower()
			started, release := make(chan struct{}), make(chan struct{})
			var loads atomic.Int32
			load := func(shared context.Context) ([]byte, error) {
				if loads.Add(1) == 1 {
					close(started)
				}
				select {
				case <-release:
					return []byte("show"), nil
				case <-shared.Done():
					return nil, shared.Err()
				}
			}
			firstResult, followerResult := make(chan error, 1), make(chan error, 1)
			go func() { _, err := cache.get(first, key, load); firstResult <- err }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("request did not start")
			}
			go func() { _, err := cache.get(follower, key, load); followerResult <- err }()
			waitForCacheWaiters(t, cache, key, 2)
			canceled, remaining := followerResult, firstResult
			if cancelFirst {
				cancelFirstCall()
				canceled, remaining = firstResult, followerResult
			} else {
				cancelFollower()
			}
			if err := <-canceled; !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled caller got %v", err)
			}
			close(release)
			if err := <-remaining; err != nil {
				t.Fatalf("one caller canceled the other's request: %v", err)
			}
			if loads.Load() != 1 || (&Client{cache: cache}).CacheStats().Entries != 1 {
				t.Fatalf("concurrent requests not merged/cached: %d", loads.Load())
			}
		})
	}
}

func TestCacheCancelsAbandonedRequestAndAllowsRetry(t *testing.T) {
	cache := cacheForTest(t, CacheConfig{})
	key := testResponseKey("show")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started, abandoned, completed := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := cache.get(ctx, key, func(shared context.Context) ([]byte, error) {
			close(started)
			<-shared.Done()
			close(abandoned)
			return nil, shared.Err()
		})
		completed <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("request did not start")
	}
	cancel()
	if err := <-completed; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-abandoned:
	case <-time.After(5 * time.Second):
		t.Fatal("abandoned network request was not canceled")
	}
	if stats := (&Client{cache: cache}).CacheStats(); stats != (CacheStats{}) {
		t.Fatalf("abandoned result cached: %+v", stats)
	}
	data, err := cache.get(context.Background(), key, func(context.Context) ([]byte, error) { return []byte("fresh"), nil })
	if err != nil || string(data) != "fresh" {
		t.Fatalf("retry joined abandoned request: %s err=%v", data, err)
	}
}

func TestClearCacheExcludesInflightResponseFromNewGeneration(t *testing.T) {
	cache := cacheForTest(t, CacheConfig{})
	client := &Client{cache: cache}
	key := testResponseKey("show")
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	started, release := make(chan struct{}), make(chan struct{})
	oldResult := make(chan string, 1)
	go func() {
		data, _ := cache.get(ctx, key, func(shared context.Context) ([]byte, error) {
			close(started)
			select {
			case <-release:
				return []byte("old"), nil
			case <-shared.Done():
				return nil, shared.Err()
			}
		})
		oldResult <- string(data)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("request did not start")
	}
	client.ClearCache()
	loads := 0
	fresh := func(context.Context) ([]byte, error) { loads++; return []byte("fresh"), nil }
	if data, err := cache.get(ctx, key, fresh); err != nil || string(data) != "fresh" {
		t.Fatalf("request after clear reused old response: %s err=%v", data, err)
	}
	close(release)
	if old := <-oldResult; old != "old" {
		t.Fatal("clear canceled an existing caller", old)
	}
	if data, err := cache.get(ctx, key, fresh); err != nil || string(data) != "fresh" || loads != 1 {
		t.Fatalf("old inflight response overwrote fresh cache: %s loads=%d err=%v", data, loads, err)
	}
	if stats := client.CacheStats(); stats.Entries != 1 || stats.Bytes != 5 {
		t.Fatalf("wrong accounting after clear: %+v", stats)
	}
}

func TestDefaultCacheBudget(t *testing.T) {
	cache := cacheForTest(t, CacheConfig{})
	if cache.config.MaxBytes != 16<<20 || cache.config.MaxEntries != 512 || cache.config.TTL != time.Hour || cache.bytes != 0 {
		t.Fatalf("unexpected defaults: %+v", cache.config)
	}
}
