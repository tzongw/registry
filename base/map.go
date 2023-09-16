package base

import (
	"hash/fnv"
	"runtime"
	"sync"
)

func Hash(x any) int {
	switch v := x.(type) {
	case int:
		return v
	case string:
		return stringHash(v)
	default:
		panic("hash function of this type not implemented")
	}
}

func stringHash(x string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(x))
	return int(h.Sum32())
}

type Shard[K comparable, V any] struct {
	m  map[K]V
	mu sync.Mutex
}

type Map[K comparable, V any] struct {
	shards []Shard[K, V]
}

func NewMap[K comparable, V any](shards int) *Map[K, V] {
	return &Map[K, V]{shards: make([]Shard[K, V], shards, shards)}
}

func (m *Map[K, V]) Store(k K, v V) {
	i := Hash(k) % len(m.shards)
	shard := &m.shards[i]
	shard.mu.Lock()
	if shard.m == nil {
		shard.m = make(map[K]V)
	}
	shard.m[k] = v
	shard.mu.Unlock()
}

func (m *Map[K, V]) Load(k K) (v V, ok bool) {
	i := Hash(k) % len(m.shards)
	shard := &m.shards[i]
	shard.mu.Lock()
	v, ok = shard.m[k]
	shard.mu.Unlock()
	return
}

func (m *Map[K, V]) Delete(k K) {
	i := Hash(k) % len(m.shards)
	shard := &m.shards[i]
	shard.mu.Lock()
	delete(shard.m, k)
	if len(shard.m) == 0 {
		shard.m = nil
	}
	shard.mu.Unlock()
}

func (m *Map[K, V]) Range(f func(k K, v V) bool) {
	done := 0
	for i := 0; i < len(m.shards); i++ {
		if done >= 4096 {
			done = 0
			runtime.Gosched()
		}
		shard := &m.shards[i]
		shard.mu.Lock()
		for k, v := range shard.m {
			if !f(k, v) {
				return
			}
		}
		done += len(shard.m)
		shard.mu.Unlock()
	}
}

func (m *Map[K, V]) Count() (count int) {
	for i := 0; i < len(m.shards); i++ {
		shard := &m.shards[i]
		shard.mu.Lock()
		count += len(shard.m)
		shard.mu.Unlock()
	}
	return
}
