package main

import (
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
	"time"
)

const (
	PREFIX           = "service"
	REFRESH_INTERVAL = 3
	TTL              = MISS_TIMES * REFRESH_INTERVAL
	COOL_DOWN        = TTL + REFRESH_INTERVAL
)

type tServices map[string]string
type tAddresses map[string]bool
type tServiceMap map[string]tAddresses

type service struct {
	services   tServices
	serviceMap tServiceMap
	client *redis.Client
}

func NewService(client *redis.Client) *service {
	s := &service{
		services:   make(tServices),
		serviceMap: make(tServiceMap),
		client:client,
	}
	return s
}

func (s *service) Register(name, address string) {
	s.services[name] = address
}

func (s *service) Start() {
	log.Info("start")
	if len(s.services) > 0 {
		s.unregister()
	}
	s.refresh()
	time.AfterFunc(time.Second, s.run)
}

func (s *service) unregister() {
	log.Debug("unregister")
}

func (s *service) refresh() {
	log.Debug("refresh")
}

func (s *service) run() {
	log.Debug("run")
}
