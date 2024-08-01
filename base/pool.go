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
	Stale    time.Duration
}

func PoolDefaultOptions() Options {
	return Options{
		PoolSize: 64,
		Timeout:  time.Second,
	}
}

type Elem[T any] struct {
	item  T
	stale *time.Timer
}

type Pool[T any] struct {
	factory Factory[T]
	opt     Options
	idle    chan Elem[T]
	timers  *CircularQueue[*time.Timer]
	size    int
	queue   int
	closed  bool
	mu      sync.Mutex
}

func NewPool[T any](factory Factory[T], opt Options) *Pool[T] {
	p := &Pool[T]{
		factory: factory,
		opt:     opt,
		idle:    make(chan Elem[T], opt.PoolSize),
	}
	if opt.Stale > 0 {
		p.timers = NewCircularQueue[*time.Timer](opt.PoolSize)
	}
	return p
}

func (p *Pool[T]) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for {
		select {
		case elem := <-p.idle:
			if elem.stale != nil {
				elem.stale.Stop()
				p.timers.Enqueue(elem.stale)
			}
			_ = p.factory.Close(elem.item)
			p.size--
		default:
			return
		}
	}
}

func (p *Pool[T]) Get() (T, error) {
	p.mu.Lock()
	if p.queue == 0 && len(p.idle) > 0 {
		defer p.mu.Unlock()
		elem := <-p.idle // NEVER block
		if elem.stale != nil {
			elem.stale.Stop()
			p.timers.Enqueue(elem.stale)
		}
		return elem.item, nil
	} else if p.size >= p.opt.PoolSize {
		p.queue++
		p.mu.Unlock()
		var elem Elem[T]
		defer func() {
			p.mu.Lock()
			p.queue--
			if elem.stale != nil {
				elem.stale.Stop()
				p.timers.Enqueue(elem.stale)
			}
			p.mu.Unlock()
		}()
		t := time.NewTimer(p.opt.Timeout)
		select {
		case elem = <-p.idle:
			t.Stop()
			return elem.item, nil
		case <-t.C:
			var empty T
			return empty, ErrTimeout
		}
	} else {
		p.size++
		p.mu.Unlock() // Open may slow
		item, err := p.factory.Open()
		if err != nil {
			p.mu.Lock()
			p.size--
			p.mu.Unlock()
		}
		return item, err
	}
}

func (p *Pool[T]) Put(item T, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil || p.closed {
		_ = p.factory.Close(item)
		p.size--
		return
	}
	elem := Elem[T]{item: item}
	if p.opt.Stale > 0 {
		if p.timers.Size() > 0 {
			elem.stale = p.timers.Dequeue()
			elem.stale.Reset(p.opt.Stale)
		} else {
			elem.stale = time.AfterFunc(p.opt.Stale, p.reapStale)
		}
	}
	p.idle <- elem
}

func (p *Pool[T]) reapStale() {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case elem := <-p.idle:
		elem.stale.Stop()
		p.timers.Enqueue(elem.stale)
		_ = p.factory.Close(elem.item)
		p.size--
	default:
	}
}
