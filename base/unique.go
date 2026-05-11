package base

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	UniquePrefix   = "unique"
	UniqueInterval = 10 * time.Second
	UniqueTTL      = 600
)

type UniqueId struct {
	redis *redis.Client
	keys  map[string]int
	mu    sync.Mutex
	stop  chan struct{}
}

func NewUniqueId(rds *redis.Client) *UniqueId {
	u := &UniqueId{
		redis: rds,
		keys:  make(map[string]int),
		stop:  make(chan struct{}),
	}
	go u.run()
	return u
}

func (u *UniqueId) Gen(biz string, start, stop int) (int, error) {
	key := UniquePrefix + ":" + biz
	u.mu.Lock()
	if _, exists := u.keys[key]; exists {
		u.mu.Unlock()
		return 0, fmt.Errorf("key %s already exists", key)
	}
	u.mu.Unlock()

	partition := start + rand.IntN(stop-start)
	ctx := context.Background()
	opt := &redis.HSetEXOptions{
		ExpirationType: redis.HSetEXExpirationEX,
		ExpirationVal:  UniqueTTL,
		Condition:      redis.HSetEXFNX,
	}

	for i := 0; i < stop-start; i++ {
		uniqueId := start + (partition-start+i)%(stop-start)
		field := fmt.Sprintf("%d", uniqueId)
		cmd := u.redis.HSetEXWithArgs(ctx, key, opt, field, "")
		if cmd.Err() != nil {
			log.Errorf("%s hsetex error: %v", key, cmd.Err())
			continue
		}
		// HSetEXWithArgs with FNX returns the number of fields added; 1 means success (new field)
		if cmd.Val() > 0 {
			log.Infof("%s got unique id %d", key, uniqueId)
			u.mu.Lock()
			u.keys[key] = uniqueId
			u.mu.Unlock()
			return uniqueId, nil
		}
		log.Infof("%s conflict id %d, retry next", key, uniqueId)
	}
	return 0, fmt.Errorf("no unique id available")
}

func (u *UniqueId) Stop() {
	close(u.stop)

	u.mu.Lock()
	defer u.mu.Unlock()

	if len(u.keys) == 0 {
		return
	}
	log.Infof("stop %v", u.keys)
	ctx := context.Background()
	u.redis.Pipelined(ctx, func(p redis.Pipeliner) error {
		for key, uniqueId := range u.keys {
			p.HDel(ctx, key, fmt.Sprintf("%d", uniqueId))
		}
		return nil
	})
	u.keys = make(map[string]int)
}

func (u *UniqueId) run() {
	ticker := time.NewTicker(UniqueInterval)
	defer ticker.Stop()
	for {
		select {
		case <-u.stop:
			return
		case <-ticker.C:
			u.mu.Lock()
			keys := make(map[string]int, len(u.keys))
			for k, v := range u.keys {
				keys[k] = v
			}
			u.mu.Unlock()

			if len(keys) == 0 {
				continue
			}
			ctx := context.Background()
			opt := &redis.HSetEXOptions{ExpirationType: redis.HSetEXExpirationEX, ExpirationVal: UniqueTTL}
			_, err := u.redis.Pipelined(ctx, func(p redis.Pipeliner) error {
				for key, uniqueId := range keys {
					p.HSetEXWithArgs(ctx, key, opt, fmt.Sprintf("%d", uniqueId), "")
				}
				return nil
			})
			if err != nil {
				log.Error(err)
			}
		}
	}
}
