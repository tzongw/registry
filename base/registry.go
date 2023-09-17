package base

import (
	"context"
	"errors"
	"github.com/go-redis/redis/v9"
	log "github.com/sirupsen/logrus"
	"net"
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
	TTL             = 3 * RefreshInterval
	CoolDown        = TTL + RefreshInterval
)

type ServiceMap map[string]sort.StringSlice

type Registry struct {
	services     map[string]string
	serviceMap   ServiceMap
	redis        *redis.Client
	stopped      atomic.Bool
	m            sync.Mutex
	afterRefresh []func()
}

func keyPrefix(name string) string {
	return Prefix + ":" + name
}

func fullKey(name, address string) string {
	return keyPrefix(name) + ":" + address
}

func unpack(key string) (string, string, error) {
	ss := strings.SplitN(key, ":", 3)
	if len(ss) != 3 {
		return "", "", errors.New("key not valid")
	}
	return ss[1], ss[2], nil
}

func NewRegistry(redis *redis.Client) *Registry {
	return &Registry{
		redis: redis,
	}
}

func (s *Registry) Start(services map[string]string) {
	log.Info("start ", services)
	s.services = services
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
	s.m.Lock()
	defer s.m.Unlock()
	return s.serviceMap[name]
}

func (s *Registry) unregister() {
	log.Info("unregister")
	if len(s.services) == 0 {
		return
	}
	keys := make([]string, 0, len(s.services))
	for name, address := range s.services {
		keys = append(keys, fullKey(name, address))
	}
	s.redis.Del(context.Background(), keys...)
	s.redis.Publish(context.Background(), Prefix, "unregister")
}

func (s *Registry) refresh() {
	log.Trace("refresh")
	var keys []string
	scan := s.redis.Scan(context.Background(), 0, Prefix+":*", 100)
	for i := scan.Iterator(); i.Next(context.Background()); {
		keys = append(keys, i.Val())
	}
	if err := scan.Err(); err != nil {
		log.Error(err)
		return
	}
	sort.Strings(keys) // DeepEqual needs
	log.Trace(keys)
	sm := make(ServiceMap)
	lastKey := ""
	for _, key := range keys {
		if key == lastKey { // scan may return duplicate keys
			continue
		}
		lastKey = key
		if name, address, err := unpack(key); err != nil {
			log.Error(err)
		} else {
			sm[name] = append(sm[name], address)
		}
	}
	s.m.Lock()
	defer s.m.Unlock()
	if !reflect.DeepEqual(sm, s.serviceMap) {
		log.Infof("update %+v -> %+v", s.serviceMap, sm)
		s.serviceMap = sm
		for _, cb := range s.afterRefresh {
			go cb()
		}
	}
}

func (s *Registry) AddCallback(cb func()) {
	s.m.Lock()
	s.afterRefresh = append(s.afterRefresh, cb)
	s.m.Unlock()
}

func (s *Registry) run() {
	log.Debug("run")
	sub := s.redis.Subscribe(context.Background(), Prefix)
	for {
		if len(s.services) > 0 && !s.stopped.Load() {
			cmds, _ := s.redis.Pipelined(context.Background(), func(p redis.Pipeliner) error {
				args := redis.SetArgs{
					Get: true,
					TTL: TTL,
				}
				for name, addr := range s.services {
					key := fullKey(name, addr)
					p.SetArgs(context.Background(), key, "", args)
				}
				return nil
			})
			if s.stopped.Load() { // race
				s.unregister()
			} else {
				for _, cmd := range cmds {
					statusCmd := cmd.(*redis.StatusCmd)
					if _, err := statusCmd.Result(); err == redis.Nil {
						log.Info("publish ", s.services)
						s.redis.Publish(context.Background(), Prefix, "register")
						break
					}
				}
			}
		}
		s.refresh()
		if m, err := sub.ReceiveTimeout(context.Background(), RefreshInterval); err != nil {
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			} else {
				log.Error(err)
				time.Sleep(RefreshInterval)
			}
		} else {
			log.Info(m)
		}
	}
}
