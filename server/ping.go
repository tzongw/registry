package server

import (
	"github.com/tzongw/registry/common"
	"sync"
	"time"
)

type schedule struct {
	deadline time.Time
	client   *client
}

var schedules []schedule
var mutex sync.Mutex

func addPing(client *client) {
	mutex.Lock()
	schedules = append(schedules, schedule{time.Now().Add(common.PingInterval), client})
	mutex.Unlock()
}

func pinger() {
	for {
		mutex.Lock()
		duration := common.PingInterval
		now := time.Now()
		for len(schedules) > 0 && schedules[0].deadline.Before(now) {
			go func(client *client) {
				if client.Ping() {
					addPing(client)
				}
			}(schedules[0].client)
			schedules[0].client = nil // gc
			schedules = schedules[1:]
		}
		if len(schedules) > 0 {
			duration = schedules[0].deadline.Sub(now)
		}
		mutex.Unlock()
		time.Sleep(duration)
	}
}
