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
		PoolSize: 128,
		Timeout:  5 * time.Second,
	}
}

type Pool[T any] struct {
	factory Factory[T]
	opt     Options
	idleC   chan T
	size    int
	queue   int
	closed  bool
	m       sync.Mutex
}

func NewPool[T any](factory Factory[T], opt Options) *Pool[T] {
	return &Pool[T]{
		factory: factory,
		opt:     opt,
		idleC:   make(chan T, opt.PoolSize)}
}

func (p *Pool[T]) Close() {
	p.m.Lock()
	defer p.m.Unlock()
	p.closed = true
	for {
		select {
		case i := <-p.idleC:
			_ = p.factory.Close(i)
		default:
			return
		}
	}
}

func (p *Pool[T]) Get() (T, error) {
	p.m.Lock()
	if p.queue == 0 && len(p.idleC) > 0 {
		defer p.m.Unlock()
		return <-p.idleC, nil // NEVER block
	} else if p.size >= p.opt.PoolSize {
		p.queue++
		p.m.Unlock()
		t := time.NewTimer(p.opt.Timeout)
		defer func() {
			t.Stop()
			p.m.Lock()
			p.queue--
			p.m.Unlock()
		}()
		select {
		case i := <-p.idleC:
			return i, nil
		case <-t.C:
			var empty T
			return empty, ErrTimeout
		}
	} else {
		p.size++
		p.m.Unlock() // Open may slow
		i, err := p.factory.Open()
		if err != nil {
			p.m.Lock()
			p.size--
			p.m.Unlock()
		}
		return i, err
	}
}

func (p *Pool[T]) Put(i T, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	if err != nil || p.closed {
		_ = p.factory.Close(i)
		p.size--
		return
	}
	p.idleC <- i
}
