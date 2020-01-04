package main

import (
	"context"
	"errors"
	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"sync"
	"time"
)

type client struct {
	*thrift.TStandardClient
	trans thrift.TTransport
}

type Client struct {
	p *Pool
}

func NewClient(addr string, opt *Options) *Client {
	return &Client{
		p: NewPool(NewThriftFactory(addr), opt),
	}
}

func (c *Client) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	cc, err := c.p.Get()
	if err != nil {
		return err
	}
	err = cc.(*client).Call(ctx, method, args, result)
	c.p.Put(cc, err)
	return err
}

func (c *Client) Close() {
	c.p.Close()
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

func (t *ThriftFactory) Open() (interface{}, error) {
	var transport thrift.TTransport
	transport, err := thrift.NewTSocket(t.addr)
	if err != nil {
		return nil, err
	}
	transport, err = t.transportFactory.GetTransport(transport)
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

func (t *ThriftFactory) Close(conn interface{}) error {
	c := conn.(*client)
	return c.trans.Close()
}

var ErrNoneAvailable = errors.New("none available")

func init() {
	rand.Seed(time.Now().UnixNano())
}

type ServiceClient struct {
	service  string
	registry *Registry
	opt      *Options
	m        sync.Mutex
	clients  map[string]*Client
}

func NewServiceClient(registry *Registry, service string, opt *Options) *ServiceClient {
	c := &ServiceClient{
		registry: registry,
		service:  service,
		opt:      opt,
		clients:  make(map[string]*Client),
	}
	go c.watch()
	return c
}

func (c *ServiceClient) watch() {
	cond := c.registry.C
	for {
		cond.L.Lock()
		cond.Wait()
		cond.L.Unlock()
		log.Infof("watch %+v", c.service)
		addresses := c.registry.Addresses(c.service)
		c.m.Lock()
		for addr, client := range c.clients {
			index := FindIndex(len(addresses), func(i int) bool {
				return addresses[i] == c.service
			})
			if index < 0 {
				log.Warnf("close client %+v %+v", c.service, addr)
				client.Close()
				delete(c.clients, addr)
			}
		}
		c.m.Unlock()
	}
}

func (c *ServiceClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	addresses := c.registry.Addresses(c.service)
	if len(addresses) == 0 {
		return ErrNoneAvailable
	}
	i := rand.Intn(len(addresses))
	addr := addresses[i]
	c.m.Lock()
	client, ok := c.clients[addr]
	if !ok {
		client = NewClient(addr, c.opt)
		c.clients[addr] = client
	}
	c.m.Unlock()
	return client.Call(ctx, method, args, result)
}