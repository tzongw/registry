package shared

import (
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/go-redis/redis"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/gen-go/service"
)

var (
	Registry   = common.NewRegistry(redis.NewClient(&redis.Options{}))
	UserClient = service.NewUserClient(common.NewServiceClient(Registry, common.RpcUser, nil))
	GateClient = NewGateClient(common.NewServiceClient(Registry, common.RpcGate, nil))
)

type tGateClient struct {
	*service.GateClient
	client *common.ServiceClient
}

func NewGateClient(c *common.ServiceClient) *tGateClient {
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
