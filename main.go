package main

import (
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.DebugLevel)
	client := redis.NewClient(&redis.Options{})
	s := NewService(client)
	s.Start(map[string]string{})
	select {}
}
