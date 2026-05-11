package common

import (
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
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
	Redis      *redis.Client
	Registry   *base.Registry
	UserClient *TUserClient
	GateClient *TGateClient
	UniqueId   *base.UniqueId
	AppId      int
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
	Redis = redis.NewClient(&redis.Options{Addr: *redisAddr})
	UniqueId = base.NewUniqueId(Redis)
	var err error
	AppId, err = UniqueId.Gen(*appName, 0, 1024)
	if err != nil {
		log.Fatal(err)
	}
	Registry = base.NewRegistry(Redis, Services)
	UserClient = NewUserClient(base.NewServiceClient(Registry, RpcUser, base.PoolDefaultOptions()))
	GateClient = NewGateClient(base.NewServiceClient(Registry, RpcGate, base.PoolDefaultOptions()))
}
