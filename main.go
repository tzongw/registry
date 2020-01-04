package main

import (
	"github.com/go-redis/redis"
	log "github.com/sirupsen/logrus"
)

func main() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	client := redis.NewClient(&redis.Options{})
	s := NewService(client)
	s.Start(map[string]string{"aa": "bb", "cc": "dd"})
	select {}
}
