package store

import (
	"sync"
	"time"
)

type DataValue struct {
	Value           interface{}
	SourceTimestamp time.Time
	Quality         uint16
}

type Store struct {
	mu   sync.RWMutex
	data map[string]*DataValue
}

func New() *Store {
	return &Store{
		data: make(map[string]*DataValue),
	}
}

func (s *Store) Update(nodeID string, value interface{}, timestamp time.Time, quality uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[nodeID] = &DataValue{
		Value:           value,
		SourceTimestamp: timestamp,
		Quality:         quality,
	}
}

func (s *Store) Get(nodeID string) (*DataValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.data[nodeID]
	return val, ok
}

func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.data))
	for id := range s.data {
		ids = append(ids, id)
	}
	return ids
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.data)
}

func (s *Store) GetAll() map[string]*DataValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*DataValue, len(s.data))
	for k, v := range s.data {
		result[k] = v
	}
	return result
}

func (s *Store) SetOriginalID(opcNodeID, origNodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[opcNodeID] == nil {
		s.data[opcNodeID] = &DataValue{}
	}
}
