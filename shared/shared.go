package shared

import (
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/go-redis/redis"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/gen-go/service"
)

var (
	Registry   = base.NewRegistry(redis.NewClient(&redis.Options{}))
	UserClient = service.NewUserClient(base.NewServiceClient(Registry, base.RpcUser, nil))
	GateClient = NewGateClient(base.NewServiceClient(Registry, base.RpcGate, nil))
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
