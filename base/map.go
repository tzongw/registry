package base

import (
	"hash/fnv"
	"runtime"
	"sync"
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

func IntegerHash[K Integer](k K) uint {
	return uint(k)
}

func PointerHash[T any](k *T) uint {
	return uint(uintptr(unsafe.Pointer(k))) >> 3 // Align 8 bytes
}

func StringHash[K ~string](k K) uint {
	h := fnv.New64a()
	_, _ = h.Write(StringToBytes(string(k)))
	return uint(h.Sum64())
}

type Shard[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

type Map[K comparable, V any] struct {
	hash   func(k K) uint
	shards []Shard[K, V]
	mask   uint
}

func NewMap[K comparable, V any](hash func(k K) uint, shards uint) *Map[K, V] {
	if shards == 0 || shards&(shards-1) != 0 {
		panic("shards should be power of 2")
	}
	return &Map[K, V]{hash: hash, shards: make([]Shard[K, V], shards), mask: shards - 1}
}

func (m *Map[K, V]) getShard(k K) *Shard[K, V] {
	i := m.hash(k) & m.mask
	return &m.shards[i]
}

func (m *Map[K, V]) Store(k K, v V) {
	shard := m.getShard(k)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.m == nil {
		shard.m = make(map[K]V)
	}
	shard.m[k] = v
}

func (m *Map[K, V]) Delete(k K) {
	shard := m.getShard(k)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.m, k)
	if len(shard.m) == 0 {
		shard.m = nil
	}
}

func (m *Map[K, V]) Load(k K) (v V, ok bool) {
	shard := m.getShard(k)
	shard.mu.RLock()
	v, ok = shard.m[k]
	shard.mu.RUnlock()
	return
}

func (m *Map[K, V]) Range(f func(k K, v V) bool) {
	done := 0
	for i := range m.shards {
		if done >= 2048 {
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
