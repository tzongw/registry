package base

import (
	"hash/fnv"
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

type shard[K comparable, V any] struct {
	m  map[K]V
	mu sync.Mutex
}

type Map[K comparable, V any] struct {
	shards []shard[K, V]
}

func NewMap[K comparable, V any](shards int) *Map[K, V] {
	return &Map[K, V]{shards: make([]shard[K, V], shards, shards)}
}

func (m *Map[K, V]) Store(k K, v V) {
	i := Hash(k) % len(m.shards)
	m.shards[i].mu.Lock()
	if m.shards[i].m == nil {
		m.shards[i].m = make(map[K]V, len(m.shards))
	}
	m.shards[i].m[k] = v
	m.shards[i].mu.Unlock()
}

func (m *Map[K, V]) Load(k K) (v V, ok bool) {
	i := Hash(k) % len(m.shards)
	m.shards[i].mu.Lock()
	v, ok = m.shards[i].m[k]
	m.shards[i].mu.Unlock()
	return
}

func (m *Map[K, V]) Delete(k K) {
	i := Hash(k) % len(m.shards)
	m.shards[i].mu.Lock()
	delete(m.shards[i].m, k)
	m.shards[i].mu.Unlock()
}

func (m *Map[K, V]) Range(f func(k K, v V) bool) {
	for i := 0; i < len(m.shards); i++ {
		m.shards[i].mu.Lock()
		for k, v := range m.shards[i].m {
			if !f(k, v) {
				return
			}
		}
		m.shards[i].mu.Unlock()
	}
}

func (m *Map[K, V]) Count() (count int) {
	for i := 0; i < len(m.shards); i++ {
		m.shards[i].mu.Lock()
		count += len(m.shards[i].m)
		m.shards[i].mu.Unlock()
	}
	return
}
