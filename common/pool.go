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
	PoolTimeout time.Duration
	IdleTimeout time.Duration
}

func DefaultOptions() *Options {
	return &Options{
		PoolSize:    128,
		PoolTimeout: 3 * time.Second,
		IdleTimeout: 30 * time.Minute,
	}
}

type Pool struct {
	factory Factory
	opt     *Options
	idleC   chan *Elem
	size    int
	queue   int
	closed  bool
	m       sync.Mutex
}

type Elem struct {
	i    interface{}
	used time.Time
}

func NewPool(factory Factory, opt *Options) *Pool {
	if opt == nil {
		opt = DefaultOptions()
	}
	p := &Pool{
		factory: factory,
		opt:     opt,
		idleC:   make(chan *Elem, opt.PoolSize)}
	if opt.IdleTimeout > 0 {
		go p.idleCheck()
	}
	return p
}

func (p *Pool) Close() {
	p.m.Lock()
	defer p.m.Unlock()
	p.closed = true
	for {
		select {
		case elem := <-p.idleC:
			p.factory.Close(elem.i)
		default:
			return
		}
	}
}

func (p *Pool) nextCheck() bool {
	p.m.Lock()
	defer p.m.Unlock()
	if p.queue > 0 {
		return true
	}
	if p.closed {
		return false
	}
	elems := make([]*Elem, 0, len(p.idleC))
	now := time.Now()
loop:
	for {
		select {
		case elem := <-p.idleC: // sorted by used
			if elem.used.Add(p.opt.IdleTimeout).Before(now) {
				p.factory.Close(elem.i)
				p.size -= 1
				log.Debugf("idle close %d", p.size)
			} else {
				elems = append(elems, elem)
			}
		default:
			break loop
		}
	}
	for _, elem := range elems {
		p.idleC <- elem
	}
	return true
}

func (p *Pool) idleCheck() {
	defer log.Debugf("idle check exit")
	d := p.opt.IdleTimeout / 10
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	tick := time.NewTicker(d)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if !p.nextCheck() {
				return
			}
		}
	}
}

func (p *Pool) Get() (interface{}, error) {
	p.m.Lock()
	if p.queue == 0 && len(p.idleC) > 0 {
		defer p.m.Unlock()
		select {
		case elem := <-p.idleC:
			return elem.i, nil
		default:
			panic("idleC blocked!!")
		}
	} else if p.size >= p.opt.PoolSize {
		p.queue += 1
		p.m.Unlock()
		t := time.NewTimer(p.opt.PoolTimeout)
		defer func() {
			t.Stop()
			p.m.Lock()
			p.queue -= 1
			p.m.Unlock()
		}()
		select {
		case elem := <-p.idleC:
			return elem.i, nil
		case <-t.C:
			log.Warnf("timeout %d", p.opt.PoolTimeout)
			return nil, ErrTimeout
		}
	} else {
		p.size += 1
		log.Debugf("create %d", p.size)
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
	p.idleC <- &Elem{i: i, used: time.Now()}
}
