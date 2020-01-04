package main

import (
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	PREFIX           = "service"
	REFRESH_INTERVAL = 3 * time.Second
	TTL              = MISS_TIMES * REFRESH_INTERVAL
	COOL_DOWN        = TTL + REFRESH_INTERVAL
)

type tAddresses []string
type tServiceMap map[string]tAddresses

type Registry struct {
	services   map[string]string
	serviceMap atomic.Value
	client     *redis.Client
}

func keyPrefix(name string) string {
	return PREFIX + ":" + name
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

func (s *Registry) Addresses(name string) tAddresses {
	m, ok := s.serviceMap.Load().(tServiceMap)
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
	s.client.Publish(PREFIX, "unregister")
}

func (s *Registry) refresh() {
	log.Debug("refresh")
	keys, err := s.client.Keys(PREFIX + "*").Result()
	if err != nil {
		log.Error(err)
		return
	}
	sort.Strings(keys) // DeepEqual needs
	log.Debug(keys)
	sm := make(tServiceMap)
	for _, key := range keys {
		name, address := unpack(key)
		sm[name] = append(sm[name], address)
	}
	if m := s.serviceMap.Load(); !reflect.DeepEqual(m, sm) {
		log.Infof("update %+v -> %+v", m, sm)
		s.serviceMap.Store(sm)
	}
}

func (s *Registry) run() {
	log.Debug("run")
	published := false
	sub := s.client.Subscribe(PREFIX)
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
		if m, _ := sub.ReceiveTimeout(REFRESH_INTERVAL); m != nil {
			log.Info(m)
		}
	}
}
