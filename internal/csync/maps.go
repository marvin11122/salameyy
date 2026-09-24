package csync

import (
	"encoding/json"
	"iter"
	"maps"
	"sync"
	"sync/atomic"
	"unsafe"
)

type Map[K comparable, V any] struct {
	inner map[K]V
	mu    unsafe.Pointer
}

type mapState struct {
	mu sync.RWMutex
}

func (m *Map[K, V]) mutex() *sync.RWMutex {
	state := (*mapState)(atomic.LoadPointer(&m.mu))
	if state != nil {
		return &state.mu
	}

	state = &mapState{}
	if atomic.CompareAndSwapPointer(&m.mu, nil, unsafe.Pointer(state)) {
		return &state.mu
	}
	state = (*mapState)(atomic.LoadPointer(&m.mu))
	return &state.mu
}

func NewMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{
		inner: make(map[K]V),
		mu:    unsafe.Pointer(&mapState{}),
	}
}

func NewMapFrom[K comparable, V any](m map[K]V) *Map[K, V] {
	return &Map[K, V]{
		inner: m,
		mu:    unsafe.Pointer(&mapState{}),
	}
}

func NewLazyMap[K comparable, V any](load func() map[K]V) *Map[K, V] {
	m := &Map[K, V]{inner: make(map[K]V), mu: unsafe.Pointer(&mapState{})}
	mu := m.mutex()
	mu.Lock()
	go func() {
		defer mu.Unlock()
		m.inner = load()
	}()
	return m
}

func (m *Map[K, V]) Reset(input map[K]V) {
	mu := m.mutex()
	mu.Lock()
	defer mu.Unlock()
	m.inner = input
}

func (m *Map[K, V]) Set(key K, value V) {
	mu := m.mutex()
	mu.Lock()
	defer mu.Unlock()
	m.inner[key] = value
}

func (m *Map[K, V]) Del(key K) {
	mu := m.mutex()
	mu.Lock()
	defer mu.Unlock()
	delete(m.inner, key)
}

func (m *Map[K, V]) Get(key K) (V, bool) {
	mu := m.mutex()
	mu.RLock()
	defer mu.RUnlock()
	v, ok := m.inner[key]
	return v, ok
}

func (m *Map[K, V]) Len() int {
	mu := m.mutex()
	mu.RLock()
	defer mu.RUnlock()
	return len(m.inner)
}

func (m *Map[K, V]) GetOrSet(key K, fn func() V) V {
	got, ok := m.Get(key)
	if ok {
		return got
	}
	value := fn()
	m.Set(key, value)
	return value
}

func (m *Map[K, V]) Take(key K) (V, bool) {
	mu := m.mutex()
	mu.Lock()
	defer mu.Unlock()
	v, ok := m.inner[key]
	delete(m.inner, key)
	return v, ok
}

func (m *Map[K, V]) Copy() map[K]V {
	mu := m.mutex()
	mu.RLock()
	defer mu.RUnlock()
	return maps.Clone(m.inner)
}

func (m *Map[K, V]) Seq2() iter.Seq2[K, V] {
	dst := m.Copy()
	return func(yield func(K, V) bool) {
		for k, v := range dst {
			if !yield(k, v) {
				return
			}
		}
	}
}

func (m *Map[K, V]) Seq() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range m.Seq2() {
			if !yield(v) {
				return
			}
		}
	}
}

var (
	_ json.Unmarshaler = &Map[string, any]{}
	_ json.Marshaler   = &Map[string, any]{}
)

func (Map[K, V]) JSONSchemaAlias() any {
	return map[K]V{}
}

func (m *Map[K, V]) UnmarshalJSON(data []byte) error {
	mu := m.mutex()
	mu.Lock()
	defer mu.Unlock()
	m.inner = make(map[K]V)
	return json.Unmarshal(data, &m.inner)
}

func (m *Map[K, V]) MarshalJSON() ([]byte, error) {
	mu := m.mutex()
	mu.RLock()
	defer mu.RUnlock()
	return json.Marshal(m.inner)
}
