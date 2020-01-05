package shared

import (
	"context"
	"github.com/go-redis/redis"
	"registry/common"
	"registry/gen-go/service"
)

var (
	Registry   = common.NewRegistry(redis.NewClient(&redis.Options{}))
	UserClient service.User = service.NewUserClient(common.NewServiceClient(Registry, common.RpcUser, nil))
	GateClient service.Gate  = service.NewGateClient(common.NewServiceClient(Registry, common.RpcGate, nil))
	DefaultCtx = context.Background()
)