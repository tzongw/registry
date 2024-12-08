package common

import (
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/redis/go-redis/v9"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/gen-go/service"
)

type TUserClient struct {
	*base.ServiceClient
	*service.UserClient
}

type TGateClient struct {
	*base.ServiceClient
	*service.GateClient
}

var (
	Redis      *redis.ClusterClient
	Registry   *base.Registry
	UserClient *TUserClient
	GateClient *TGateClient
)

func NewUserClient(c *base.ServiceClient) *TUserClient {
	return &TUserClient{
		ServiceClient: c,
		UserClient:    service.NewUserClient(c),
	}
}

func NewGateClient(c *base.ServiceClient) *TGateClient {
	return &TGateClient{
		ServiceClient: c,
		GateClient:    service.NewGateClient(c),
	}
}

func (c *TGateClient) ConnClient(addr string, f func(gate service.Gate) error) error {
	return c.ServiceClient.ConnClient(addr, func(conn thrift.TClient) error {
		return f(service.NewGateClient(conn))
	})
}

func InitShared() {
	Redis = redis.NewClusterClient(&redis.ClusterOptions{Addrs: []string{*redisAddr}})
	Registry = base.NewRegistry(Redis, Services)
	UserClient = NewUserClient(base.NewServiceClient(Registry, RpcUser, base.PoolDefaultOptions()))
	GateClient = NewGateClient(base.NewServiceClient(Registry, RpcGate, base.PoolDefaultOptions()))
}
