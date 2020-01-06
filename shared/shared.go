package shared

import (
	"context"
	"github.com/go-redis/redis"
	"registry/common"
	"registry/gen-go/service"
)

var (
	Registry                = common.NewRegistry(redis.NewClient(&redis.Options{}))
	DefaultCtx              = context.Background()
	UserClient service.User = service.NewUserClient(common.NewServiceClient(Registry, common.RpcUser, nil))
	//GateClient              = service.NewGateClient(common.NewServiceClient(Registry, common.RpcGate, nil))
)
