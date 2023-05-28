package statussync

import (
	"errors"
	"sync"
)

var (
	ErrNotFound = errors.New("not Found")
)

type StatusInformer struct {
	sync.Mutex
	m map[string]chan struct{}
}

func NewManager() *StatusInformer {
	return &StatusInformer{
		m: make(map[string]chan struct{}),
	}
}

func (s *StatusInformer) Add(name string) <-chan struct{} {
	s.Lock()
	defer s.Unlock()
	ch := make(chan struct{}, 1)
	s.m[name] = ch
	return ch
}

func (s *StatusInformer) Delete(name string) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, name)
}

func (s *StatusInformer) Sync(name string) error {
	s.Lock()
	defer s.Unlock()
	if ch, ok := s.m[name]; ok {
		ch <- struct{}{}
		return nil
	} else {
		return ErrNotFound
	}
}
