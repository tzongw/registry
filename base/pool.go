package base

import (
	"errors"
	"sync"
	"time"
)

var ErrTimeout = errors.New("pool timeout")

type Factory[T any] interface {
	Open() (T, error)
	Close(T) error
}

type Options struct {
	PoolSize int
	Timeout  time.Duration
}

func PoolDefaultOptions() Options {
	return Options{
		PoolSize: 64,
		Timeout:  time.Second,
	}
}

type Pool[T any] struct {
	factory Factory[T]
	opt     Options
	idle    *Channel[T]
	size    int
	waiting int
	closed  bool
	mu      sync.Mutex
}

func NewPool[T any](factory Factory[T], opt Options) *Pool[T] {
	return &Pool[T]{
		factory: factory,
		opt:     opt,
		idle:    NewChannel[T]()}
}

func (p *Pool[T]) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for {
		select {
		case i := <-p.idle.Get():
			p.idle.Load()
			_ = p.factory.Close(i)
			p.size--
		default:
			return
		}
	}
}

func (p *Pool[T]) Get() (i T, err error) {
	p.mu.Lock()
	ch := p.idle.Get()
	if p.waiting == 0 && len(ch) > 0 {
		i = <-ch // NEVER block
		p.idle.Load()
		p.mu.Unlock()
	} else if p.size >= p.opt.PoolSize {
		p.waiting++
		p.mu.Unlock()
		t := time.NewTimer(p.opt.Timeout)
		select { // WITHOUT lock
		case i = <-ch:
			t.Stop()
			p.mu.Lock()
			p.waiting--
			p.idle.Load()
			p.mu.Unlock()
		case <-t.C:
			var empty T
			i, err = empty, ErrTimeout
			p.mu.Lock()
			p.waiting--
			p.mu.Unlock()
		}
	} else {
		p.size++
		p.mu.Unlock() // Open may slow
		i, err = p.factory.Open()
		if err != nil {
			p.mu.Lock()
			p.size--
			p.mu.Unlock()
		}
	}
	return
}

func (p *Pool[T]) Put(i T, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil || p.closed {
		_ = p.factory.Close(i)
		p.size--
		return
	}
	p.idle.Put(i)
}
