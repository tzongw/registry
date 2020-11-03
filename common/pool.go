package common

import (
	"errors"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

var ErrTimeout = errors.New("pool timeout")

type Factory interface {
	Open() (interface{}, error)
	Close(interface{}) error
}

type Options struct {
	PoolSize    int
	WaitTimeout time.Duration
	IdleTimeout time.Duration // TODO:
}

func DefaultOptions() *Options {
	return &Options{
		PoolSize:    128,
		WaitTimeout: time.Second,
		IdleTimeout: time.Hour,
	}
}

type Pool struct {
	factory Factory
	opt     *Options
	idleC   chan interface{}
	size    int
	queue   int
	closed  bool
	m       sync.Mutex
}

func NewPool(factory Factory, opt *Options) *Pool {
	if opt == nil {
		opt = DefaultOptions()
	}
	return &Pool{
		factory: factory,
		opt:     opt,
		idleC:   make(chan interface{}, opt.PoolSize)}
}

func (p *Pool) Close() {
	p.m.Lock()
	defer p.m.Unlock()
	p.closed = true
	for {
		select {
		case i := <-p.idleC:
			p.factory.Close(i)
		default:
			return
		}
	}
}

func (p *Pool) Get() (interface{}, error) {
	p.m.Lock()
	if p.queue == 0 && len(p.idleC) > 0 {
		defer p.m.Unlock()
		select {
		case i := <-p.idleC:
			return i, nil
		default:
			panic("idleC blocked!!")
		}
	} else if p.size >= p.opt.PoolSize {
		p.queue += 1
		p.m.Unlock()
		t := time.NewTimer(p.opt.WaitTimeout)
		defer func() {
			t.Stop()
			p.m.Lock()
			p.queue -= 1
			p.m.Unlock()
		}()
		select {
		case i := <-p.idleC:
			return i, nil
		case <-t.C:
			log.Warnf("timeout %d", p.opt.WaitTimeout)
			return nil, ErrTimeout
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

func (p *Pool) Put(i interface{}, err error) {
	p.m.Lock()
	defer p.m.Unlock()
	if err != nil || p.closed {
		p.factory.Close(i)
		p.size -= 1
		return
	}
	p.idleC <- i
}
