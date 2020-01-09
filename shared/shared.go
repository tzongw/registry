package shared

import (
	"github.com/go-redis/redis"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/gen-go/service"
)

var (
	Registry                = common.NewRegistry(redis.NewClient(&redis.Options{}))
	UserClient service.User = service.NewUserClient(common.NewServiceClient(Registry, common.RpcUser, nil))
	GateClient service.Gate = service.NewGateClient(common.NewServiceClient(Registry, common.RpcGate, nil))
)
