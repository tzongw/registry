package common

import (
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/go-redis/redis"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/gen-go/service"
)

type tUserClient struct {
	*base.ServiceClient
	*service.UserClient
}

type tGateClient struct {
	*base.ServiceClient
	*service.GateClient
}

var (
	Redis      *redis.Client
	Registry   *base.Registry
	UserClient *tUserClient
	GateClient *tGateClient
)

func NewUserClient(c *base.ServiceClient) *tUserClient {
	return &tUserClient{
		ServiceClient: c,
		UserClient:    service.NewUserClient(c),
	}
}

func NewGateClient(c *base.ServiceClient) *tGateClient {
	return &tGateClient{
		ServiceClient: c,
		GateClient:    service.NewGateClient(c),
	}
}

func (c *tGateClient) ConnClient(addr string, f func(gate service.Gate) error) error {
	return c.ServiceClient.ConnClient(addr, func(conn thrift.TClient) error {
		return f(service.NewGateClient(conn))
	})
}

func InitShared() {
	Redis = redis.NewClient(&redis.Options{Addr: *redisAddr})
	Registry = base.NewRegistry(Redis)
	UserClient = NewUserClient(base.NewServiceClient(Registry, RpcUser, nil))
	GateClient = NewGateClient(base.NewServiceClient(Registry, RpcGate, nil))
}
