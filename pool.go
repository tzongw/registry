package main

import (
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type Factory interface {
	Open() (interface{}, error)
	Close(interface{}) error
}

type Options struct {
	PoolSize    int
	PoolTimeout time.Duration
	IdleTimeout time.Duration // TODO:
}

func DefaultOptions() *Options {
	return &Options{
		PoolSize:    128,
		PoolTimeout: 3 * time.Second,
		IdleTimeout: time.Hour,
	}
}

type PoolError struct {
	text string
}

func (e *PoolError) Error() string {
	return e.text
}

type pool struct {
	factory Factory
	opt     *Options
	ch      chan interface{}
	size    int
	closed  bool
	m       sync.Mutex
}

func NewPool(factory Factory, opt *Options) *pool {
	if opt == nil {
		opt = DefaultOptions()
	}
	return &pool{
		factory: factory,
		opt:     opt,
		ch:      make(chan interface{}, opt.PoolSize)}
}

func (p *pool) Close() {
	p.m.Lock()
	defer p.m.Unlock()
	p.closed = true
	for {
		select {
		case i := <-p.ch:
			p.factory.Close(i)
		default:
			break
		}
	}
}

func (p *pool) Get() (interface{}, error) {
	p.m.Lock()
	if len(p.ch) > 0 {
		defer p.m.Unlock()
		return <-p.ch, nil
	} else if p.size >= p.opt.PoolSize {
		p.m.Unlock()
		t := time.NewTimer(p.opt.PoolTimeout)
		defer t.Stop()
		select {
		case i := <-p.ch:
			return i, nil
		case <-t.C:
			log.Warnf("timeout %d", p.opt.PoolTimeout)
			return nil, &PoolError{"timeout"}
		}
	} else {
		p.size += 1
		p.m.Unlock() // Open may slow
		i, err := p.factory.Open()
		if err != nil {
			log.Error(err)
			p.m.Lock()
			p.size -= 1
			p.m.Unlock()
		}
		return i, err
	}
}

func (p *pool) Put(i interface{}, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	if err != nil || p.closed {
		p.factory.Close(i)
		p.size -= 1
		return
	}
	p.ch <- i
}
