package base

import (
	"hash/fnv"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type Integer interface {
	Signed | Unsigned
}

type Float interface {
	~float32 | ~float64
}

func NumericHash[K Integer | Float](k K) int {
	return int(k)
}

func StringHash[K ~string](k K) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(k))
	return int(h.Sum32())
}

func PointerHash[T any](k *T) int {
	return int(uintptr(unsafe.Pointer(k)))
}

type Shard[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

type Map[K comparable, V any] struct {
	hash   func(k K) int
	shards []Shard[K, V]
	size   atomic.Int64
}

func NewMap[K comparable, V any](hash func(k K) int, shards int) *Map[K, V] {
	return &Map[K, V]{hash: hash, shards: make([]Shard[K, V], shards, shards)}
}

func (m *Map[K, V]) Store(k K, v V) {
	i := m.hash(k) % len(m.shards)
	shard := &m.shards[i]
	shard.mu.Lock()
	if _, ok := shard.m[k]; !ok {
		m.size.Add(1)
	}
	if shard.m == nil {
		shard.m = make(map[K]V)
	}
	shard.m[k] = v
	shard.mu.Unlock()
}

func (m *Map[K, V]) Delete(k K) {
	i := m.hash(k) % len(m.shards)
	shard := &m.shards[i]
	shard.mu.Lock()
	if _, ok := shard.m[k]; ok {
		m.size.Add(-1)
	}
	delete(shard.m, k)
	if len(shard.m) == 0 {
		shard.m = nil
	}
	shard.mu.Unlock()
}

func (m *Map[K, V]) Load(k K) (v V, ok bool) {
	i := m.hash(k) % len(m.shards)
	shard := &m.shards[i]
	shard.mu.RLock()
	v, ok = shard.m[k]
	shard.mu.RUnlock()
	return
}

func (m *Map[K, V]) Range(f func(k K, v V) bool) {
	done := 0
	for i := 0; i < len(m.shards); i++ {
		if done >= 4096 {
			done = 0
			runtime.Gosched()
		}
		shard := &m.shards[i]
		shard.mu.RLock()
		for k, v := range shard.m {
			if !f(k, v) {
				shard.mu.RUnlock()
				return
			}
		}
		done += len(shard.m)
		shard.mu.RUnlock()
	}
}

func (m *Map[K, V]) Size() int64 {
	return m.size.Load()
}
