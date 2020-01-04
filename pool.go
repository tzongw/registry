package main

import (
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type Options struct {
	Create func() (interface{}, error)
	Close  func(interface{}) error

	PoolSize    int
	PoolTimeout time.Duration
	IdleTimeout time.Duration // TODO:
}

func DefaultOptions() *Options {
	return &Options{
		Create:      nil,
		Close:       nil,
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
	opt  *Options
	ch   chan interface{}
	size int
	m    sync.Mutex
}

func NewPool(opt *Options) *pool {
	return &pool{
		opt: opt,
		ch:  make(chan interface{}, opt.PoolSize)}
}

func Close() {

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
		p.m.Unlock() // create may slow
		i, err := p.opt.Create()
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
	if err != nil || i == nil {
		p.m.Lock()
		p.size -= 1
		p.m.Unlock()
		return
	}
	p.ch <- i
}
