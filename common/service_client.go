package common

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
)

type client struct {
	*thrift.TStandardClient
	trans thrift.TTransport
}

type ThriftFactory struct {
	addr             string
	transportFactory thrift.TTransportFactory
	protocolFactory  thrift.TProtocolFactory
}

func NewThriftFactory(addr string) *ThriftFactory {
	return &ThriftFactory{
		addr:             addr,
		transportFactory: thrift.NewTBufferedTransportFactory(8192),
		protocolFactory:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
}

func (t *ThriftFactory) Open() (*client, error) {
	sock, err := thrift.NewTSocket(t.addr)
	if err != nil {
		return nil, err
	}
	transport, err := t.transportFactory.GetTransport(sock)
	if err != nil {
		return nil, err
	}
	if err := transport.Open(); err != nil {
		return nil, err
	}
	iprot := t.protocolFactory.GetProtocol(transport)
	oprot := t.protocolFactory.GetProtocol(transport)
	return &client{TStandardClient: thrift.NewTStandardClient(iprot, oprot), trans: transport}, nil
}

func (t *ThriftFactory) Close(c *client) error {
	return c.trans.Close()
}

type NodeClient struct {
	p *Pool[client]
}

func NewNodeClient(addr string, opt *Options) *NodeClient {
	return &NodeClient{
		p: NewPool[client](NewThriftFactory(addr), opt),
	}
}

func (c *NodeClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	cc, err := c.p.Get()
	if err != nil {
		return err
	}
	err = cc.Call(ctx, method, args, result)
	c.p.Put(cc, err)
	return err
}

func (c *NodeClient) Close() {
	c.p.Close()
}

var ErrUnavailable = errors.New("unavailable")

func init() {
	rand.Seed(time.Now().UnixNano())
}

type ServiceClient struct {
	service        string
	registry       *Registry
	opt            *Options
	m              sync.Mutex
	clients        map[string]*NodeClient
	localAddresses sort.StringSlice
}

func NewServiceClient(registry *Registry, service string, opt *Options) *ServiceClient {
	c := &ServiceClient{
		registry: registry,
		service:  service,
		opt:      opt,
		clients:  make(map[string]*NodeClient),
	}
	registry.AddCallback(c.clean)
	return c
}

func (c *ServiceClient) clean() {
	log.Infof("watch %+v", c.service)
	addresses := c.registry.Addresses(c.service)
	c.m.Lock()
	defer c.m.Unlock()
	c.localAddresses = nil
	for _, addr := range addresses {
		host, _, _ := HostPort(addr)
		if host == localIP {
			c.localAddresses = append(c.localAddresses, addr)
		}
	}
	for addr, client := range c.clients {
		index := FindIndex(len(addresses), func(i int) bool {
			return addresses[i] == addr
		})
		if index < 0 {
			log.Infof("close client %+v %+v", c.service, addr)
			client.Close()
			delete(c.clients, addr)
		}
	}
}

var RandomCtx = context.Background()

type tNodeSelector int

var selector interface{} = tNodeSelector(0)
var broadcast = 0
var BroadcastCtx = context.WithValue(context.Background(), selector, broadcast)

func WithNode(addr string) context.Context {
	return context.WithValue(context.Background(), selector, addr)
}

func (c *ServiceClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	addresses := c.registry.Addresses(c.service)
	if len(addresses) == 0 {
		return ErrUnavailable
	}
	if v := ctx.Value(selector); v != nil {
		if addr, ok := v.(string); ok {
			client := c.client(addr)
			return client.Call(ctx, method, args, result)
		} else {
			if result != nil {
				panic("broadcast MUST be oneway")
			}
			var err error
			for _, addr := range addresses {
				client := c.client(addr)
				e := client.Call(ctx, method, args, result)
				if err == nil { // first error
					err = e
				}
			}
			return err
		}
	} else {
		c.m.Lock()
		if len(c.localAddresses) > 0 {
			// pick local address if available
			addresses = c.localAddresses
		}
		c.m.Unlock()
		i := rand.Intn(len(addresses))
		addr := addresses[i]
		client := c.client(addr)
		return client.Call(ctx, method, args, result)
	}
}

func (c *ServiceClient) client(addr string) *NodeClient {
	c.m.Lock()
	defer c.m.Unlock()
	client, ok := c.clients[addr]
	if !ok {
		client = NewNodeClient(addr, c.opt)
		c.clients[addr] = client
	}
	return client
}

func (c *ServiceClient) ConnClient(addr string, f func(conn thrift.TClient) error) error {
	nodeClient := c.client(addr)
	conn, err := nodeClient.p.Get()
	if err != nil {
		return err
	}
	err = f(conn)
	nodeClient.p.Put(conn, err)
	return err
}
