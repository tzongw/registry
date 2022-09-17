package common

import (
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/go-redis/redis"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/gen-go/service"
)

var (
	Redis      *redis.Client
	Registry   *base.Registry
	UserClient *service.UserClient
	GateClient *tGateClient
)

type tGateClient struct {
	*service.GateClient
	client *base.ServiceClient
}

func NewGateClient(c *base.ServiceClient) *tGateClient {
	return &tGateClient{
		GateClient: service.NewGateClient(c),
		client:     c,
	}
}

func (c *tGateClient) ConnClient(addr string, f func(gate service.Gate) error) error {
	return c.client.ConnClient(addr, func(conn thrift.TClient) error {
		return f(service.NewGateClient(conn))
	})
}

func InitShared() {
	Redis = redis.NewClient(&redis.Options{Addr: *redisAddr})
	Registry = base.NewRegistry(Redis)
	UserClient = service.NewUserClient(base.NewServiceClient(Registry, RpcUser, nil))
	GateClient = NewGateClient(base.NewServiceClient(Registry, RpcGate, nil))
}
