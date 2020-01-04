package common

import (
	"github.com/go-redis/redis"
	"registry/gen-go/service"
)

var (
	registry   = NewRegistry(redis.NewClient(&redis.Options{}))
	userClient = service.NewUserClient(NewServiceClient(registry, RpcUser, nil))
	gateClient = service.NewGateClient(NewServiceClient(registry, RpcGate, nil))
)
