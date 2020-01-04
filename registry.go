package main

import (
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	Prefix          = "service"
	RefreshInterval = 3 * time.Second
	TTL             = MissTimes * RefreshInterval
	CoolDown        = TTL + RefreshInterval
)

type ServiceMap map[string]sort.StringSlice

type Registry struct {
	services   map[string]string
	serviceMap atomic.Value
	client     *redis.Client
	C          *sync.Cond
}

func keyPrefix(name string) string {
	return Prefix + ":" + name
}

func fullKey(name, address string) string {
	return keyPrefix(name) + ":" + address
}

func unpack(key string) (string, string) {
	ss := strings.SplitN(key, ":", 3)
	return ss[1], ss[2]
}

func NewRegistry(client *redis.Client) *Registry {
	return &Registry{
		client: client,
		C:      sync.NewCond(&sync.Mutex{}),
	}
}

func (s *Registry) Start(services map[string]string) {
	log.Info("start")
	s.services = services
	s.unregister()
	s.refresh()
	time.AfterFunc(time.Second, s.run)
}

func (s *Registry) Stop() {
	s.unregister()
}

func (s *Registry) Addresses(name string) sort.StringSlice {
	m, ok := s.serviceMap.Load().(ServiceMap)
	if !ok {
		return nil
	}
	return m[name]
}

func (s *Registry) unregister() {
	log.Debug("unregister")
	if len(s.services) == 0 {
		return
	}
	keys := make([]string, 0, len(s.services))
	for name, address := range s.services {
		keys = append(keys, fullKey(name, address))
	}
	s.client.Del(keys...)
	s.client.Publish(Prefix, "unregister")
}

func (s *Registry) refresh() {
	log.Debug("refresh")
	keys, err := s.client.Keys(Prefix + "*").Result()
	if err != nil {
		log.Error(err)
		return
	}
	sort.Strings(keys) // DeepEqual needs
	log.Debug(keys)
	sm := make(ServiceMap)
	for _, key := range keys {
		name, address := unpack(key)
		sm[name] = append(sm[name], address)
	}
	if m := s.serviceMap.Load(); !reflect.DeepEqual(m, sm) {
		log.Infof("update %+v -> %+v", m, sm)
		s.serviceMap.Store(sm)
		s.C.L.Lock()
		s.C.Broadcast()
		s.C.L.Unlock()
	}
}

func (s *Registry) run() {
	log.Debug("run")
	published := false
	sub := s.client.Subscribe(Prefix)
	for {
		if len(s.services) > 0 {
			s.client.Pipelined(func(p redis.Pipeliner) error {
				for name, addr := range s.services {
					key := fullKey(name, addr)
					p.Set(key, "", TTL)
				}
				return nil
			})
			if !published {
				published = true
				log.Info("publish ", s.services)
			}
		}
		s.refresh()
		if m, _ := sub.ReceiveTimeout(RefreshInterval); m != nil {
			log.Info(m)
		}
	}
}
