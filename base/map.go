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

func IntegerHash[K Integer](k K) uint {
	// https://stackoverflow.com/questions/664014/what-integer-hash-function-are-good-that-accepts-an-integer-hash-key
	x := uint(k)
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x = x ^ (x >> 31)
	return x
}

func StringHash[K ~string](k K) uint {
	h := fnv.New32a()
	_, _ = h.Write([]byte(k))
	return uint(h.Sum32())
}

func PointerHash[T any](k *T) uint {
	return IntegerHash(uintptr(unsafe.Pointer(k)))
}

type Shard[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

type Map[K comparable, V any] struct {
	hash   func(k K) uint
	shards []Shard[K, V]
	size   atomic.Int64
}

func NewMap[K comparable, V any](hash func(k K) uint, shards uint) *Map[K, V] {
	return &Map[K, V]{hash: hash, shards: make([]Shard[K, V], shards, shards)}
}

func (m *Map[K, V]) Store(k K, v V) {
	i := m.hash(k) % uint(len(m.shards))
	shard := &m.shards[i]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if _, ok := shard.m[k]; !ok {
		m.size.Add(1)
	}
	if shard.m == nil {
		shard.m = make(map[K]V)
	}
	shard.m[k] = v
}

func (m *Map[K, V]) Delete(k K) {
	i := m.hash(k) % uint(len(m.shards))
	shard := &m.shards[i]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if _, ok := shard.m[k]; !ok {
		return
	}
	m.size.Add(-1)
	delete(shard.m, k)
	if len(shard.m) == 0 {
		shard.m = nil
	}
}

func (m *Map[K, V]) CreateOrOperate(k K, create func() V, operate func(V) bool) {
	i := m.hash(k) % uint(len(m.shards))
	shard := &m.shards[i]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if v, ok := shard.m[k]; ok {
		if operate(v) {
			delete(shard.m, k)
			m.size.Add(-1)
			if len(shard.m) == 0 {
				shard.m = nil
			}
		}
	} else {
		if shard.m == nil {
			shard.m = make(map[K]V)
		}
		shard.m[k] = create()
		m.size.Add(1)
	}
}

func (m *Map[K, V]) Load(k K) (v V, ok bool) {
	i := m.hash(k) % uint(len(m.shards))
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
