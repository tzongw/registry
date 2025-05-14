package base

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	Prefix          = "service"
	RefreshInterval = 10 * time.Second
	TTL             = 3 * RefreshInterval
	CoolDown        = TTL + RefreshInterval
)

type ServiceMap map[string]sort.StringSlice

type Registry struct {
	redis        *redis.Client
	services     []string
	registered   map[string]string
	stopped      atomic.Bool
	mu           sync.Mutex
	serviceMap   ServiceMap
	afterRefresh []func()
}

func fullKey(name string) string {
	return Prefix + ":" + name
}

func NewRegistry(redis *redis.Client, services []string) *Registry {
	return &Registry{
		redis:    redis,
		services: services,
	}
}

func (s *Registry) Start(services map[string]string) {
	log.Info("start ", services)
	s.registered = services
	s.unregister()
	s.refresh()
	go s.run()
}

func (s *Registry) Stop() {
	log.Info("stop")
	s.stopped.Store(true)
	s.unregister()
}

func (s *Registry) Addresses(name string) sort.StringSlice {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.serviceMap[name]
}

func (s *Registry) unregister() {
	log.Info("unregister")
	if len(s.registered) == 0 {
		return
	}
	s.redis.Pipelined(context.Background(), func(p redis.Pipeliner) error {
		for name, addr := range s.registered {
			p.HDel(context.Background(), fullKey(name), addr)
		}
		p.Publish(context.Background(), Prefix, "unregister")
		return nil
	})
}

func (s *Registry) refresh() {
	cmds, err := s.redis.Pipelined(context.Background(), func(p redis.Pipeliner) error {
		for _, name := range s.services {
			p.HKeys(context.Background(), fullKey(name))
		}
		return nil
	})
	if err != nil {
		log.Error(err)
		return
	}
	sm := make(ServiceMap, len(s.services))
	for i, cmd := range cmds {
		name := s.services[i]
		hkeysCmd := cmd.(*redis.StringSliceCmd)
		keys := hkeysCmd.Val()
		sort.Strings(keys) // DeepEqual needs
		sm[name] = keys
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !reflect.DeepEqual(sm, s.serviceMap) {
		log.Infof("update %+v -> %+v", s.serviceMap, sm)
		s.serviceMap = sm
		for _, cb := range s.afterRefresh {
			go cb()
		}
	}
}

func (s *Registry) AddCallback(cb func()) {
	s.mu.Lock()
	s.afterRefresh = append(s.afterRefresh, cb)
	s.mu.Unlock()
}

func (s *Registry) run() {
	pubsub := s.redis.Subscribe(context.Background(), Prefix)
	published := false
	for {
		if len(s.registered) > 0 && !s.stopped.Load() {
			_, err := s.redis.Pipelined(context.Background(), func(p redis.Pipeliner) error {
				opt := &redis.HSetEXOptions{ExpirationType: redis.HSetEXExpirationEX, ExpirationVal: int64(TTL / time.Second)}
				for name, addr := range s.registered {
					key := fullKey(name)
					p.HSetEXWithArgs(context.Background(), key, opt, addr, "")
				}
				return nil
			})
			if err != nil {
				log.Error(err)
			} else if s.stopped.Load() { // race
				s.unregister()
			} else if !published {
				log.Info("publish ", s.registered)
				s.redis.Publish(context.Background(), Prefix, "register")
				published = true
			}
		}
		s.refresh()
		timeout := RefreshInterval
		for {
			if m, err := pubsub.ReceiveTimeout(context.Background(), timeout); err != nil {
				var netErr net.Error
				if !(errors.As(err, &netErr) && netErr.Timeout()) {
					log.Error(err)
					time.Sleep(RefreshInterval)
				}
				break
			} else {
				log.Debug(m)
				if timeout == RefreshInterval {
					time.Sleep(10 * time.Millisecond) // exhaust all messages
					timeout = time.Millisecond        // timeout 0 will block forever
				}
			}
		}
	}
}
