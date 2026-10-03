package tmdb

import (
	"container/list"
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

const (
	DefaultCacheMaxBytes   int64 = 16 << 20
	DefaultCacheMaxEntries       = 512
	DefaultCacheTTL              = time.Hour
)

// CacheConfig 配置单个 Client 的内存缓存；零值启用默认配置，不预分配正文空间。
// MaxBytes 只限制缓存 JSON 正文，不包括索引、在途请求及调用方持有的结果。
type CacheConfig struct {
	Disabled   bool          // 禁用缓存及并发请求合并，每次调用直接请求 TMDB。
	MaxBytes   int64         // 默认 16 MiB；单条正文超过预算时不缓存。
	MaxEntries int           // 默认 512 条；搜索、外部 ID 反查和详情共用预算，超限按 LRU 淘汰。
	TTL        time.Duration // 默认 1 小时，从成功响应入缓存起计算，命中不续期。
}

// CacheStats 是当前缓存快照；Bytes 是 JSON 正文的总字节数，并非进程内存用量。
type CacheStats struct {
	Entries int   `json:"entries"`
	Bytes   int64 `json:"bytes"`
}

// ClearCache 清除缓存。已在等待的请求继续完成，但不会将清除前的响应重新入缓存；
// 清除后的调用会发起新请求。不会修改调用方已经取得的结果。
func (c *Client) ClearCache() {
	if c.cache == nil {
		return
	}
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	c.cache.entries = make(map[responseKey]*list.Element)
	c.cache.lru.Init()
	c.cache.bytes = 0
	c.cache.flights = make(map[responseKey]*responseFlight)
}

// CacheStats 返回使用量，同时移除过期条目。缓存禁用时返回零值。
func (c *Client) CacheStats() CacheStats {
	if c.cache == nil {
		return CacheStats{}
	}
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	c.cache.expireLocked(c.cache.now())
	return CacheStats{Entries: len(c.cache.entries), Bytes: c.cache.bytes}
}

// 固定长度键避免超长搜索文本常驻缓存；凭证不参与键，不同 Client 自然隔离。
type responseKey [sha256.Size]byte

type responseEntry struct {
	key       responseKey
	data      []byte // 仅在包内只读使用；公开结果每次重新解码。
	expiresAt time.Time
}

type responseFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	data    []byte
	err     error
}

type responseCache struct {
	mu      sync.Mutex
	config  CacheConfig
	entries map[responseKey]*list.Element
	lru     list.List
	bytes   int64
	flights map[responseKey]*responseFlight
	now     func() time.Time
}

func newResponseCache(config CacheConfig) (*responseCache, error) {
	if config.MaxBytes < 0 || config.MaxEntries < 0 || config.TTL < 0 {
		return nil, errors.New("缓存 MaxBytes、MaxEntries 和 TTL 不能为负数")
	}
	if config.Disabled {
		return nil, nil
	}
	if config.MaxBytes == 0 {
		config.MaxBytes = DefaultCacheMaxBytes
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = DefaultCacheMaxEntries
	}
	if config.TTL == 0 {
		config.TTL = DefaultCacheTTL
	}
	return &responseCache{config: config, entries: make(map[responseKey]*list.Element),
		flights: make(map[responseKey]*responseFlight), now: time.Now}, nil
}

func (cache *responseCache) get(ctx context.Context, key responseKey, load func(context.Context) ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cache == nil {
		return load(ctx)
	}
	cache.mu.Lock()
	if err := ctx.Err(); err != nil {
		cache.mu.Unlock()
		return nil, err
	}
	cache.expireLocked(cache.now())
	if element := cache.entries[key]; element != nil {
		cache.lru.MoveToFront(element)
		data := element.Value.(*responseEntry).data
		cache.mu.Unlock()
		return data, nil
	}
	flight := cache.flights[key]
	if flight == nil {
		// 每个等待者的取消/超时独立生效；所有等待者离开时才取消共享请求。
		// HTTPClient.Timeout 仍限制网络请求，保留首个调用 context 中的值。
		shared, cancel := context.WithCancel(context.WithoutCancel(ctx))
		flight = &responseFlight{done: make(chan struct{}), cancel: cancel}
		cache.flights[key] = flight
		go cache.load(shared, key, flight, load)
	}
	flight.waiters++
	cache.mu.Unlock()

	select {
	case <-ctx.Done():
		cache.mu.Lock()
		flight.waiters--
		if flight.waiters == 0 {
			if cache.flights[key] == flight {
				delete(cache.flights, key)
			}
			flight.cancel()
		}
		cache.mu.Unlock()
		return nil, ctx.Err()
	case <-flight.done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return flight.data, flight.err
	}
}

func (cache *responseCache) load(ctx context.Context, key responseKey, flight *responseFlight, load func(context.Context) ([]byte, error)) {
	data, err := load(ctx)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	flight.data, flight.err = data, err
	if cache.flights[key] == flight {
		delete(cache.flights, key)
		if err == nil && ctx.Err() == nil {
			flight.data = cache.storeLocked(key, data)
		}
	}
	close(flight.done)
	flight.cancel()
}

func (cache *responseCache) storeLocked(key responseKey, data []byte) []byte {
	size := int64(len(data))
	if size > cache.config.MaxBytes {
		return data
	}
	now := cache.now()
	cache.expireLocked(now)
	for len(cache.entries) >= cache.config.MaxEntries || cache.bytes > cache.config.MaxBytes-size {
		cache.removeLocked(cache.lru.Back())
	}
	// io.ReadAll 的缓冲区容量可能大于正文，复制到等长切片，按实际保留字节计费。
	owned := make([]byte, len(data))
	copy(owned, data)
	entry := &responseEntry{key: key, data: owned, expiresAt: now.Add(cache.config.TTL)}
	cache.entries[key] = cache.lru.PushFront(entry)
	cache.bytes += size
	return owned
}

func (cache *responseCache) expireLocked(now time.Time) {
	for element := cache.lru.Back(); element != nil; {
		previous := element.Prev()
		if !now.Before(element.Value.(*responseEntry).expiresAt) {
			cache.removeLocked(element)
		}
		element = previous
	}
}

func (cache *responseCache) removeLocked(element *list.Element) {
	entry := element.Value.(*responseEntry)
	delete(cache.entries, entry.key)
	cache.bytes -= int64(len(entry.data))
	cache.lru.Remove(element)
}
