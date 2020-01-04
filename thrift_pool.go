package main

import (
	"context"
	"github.com/apache/thrift/lib/go/thrift"
)

type client struct {
	*thrift.TStandardClient
	trans thrift.TTransport
}

type Client struct {
	p *pool
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
	switch err.(type) {
	case thrift.TException:
		c.p.Put(cc, err)
	default:
		c.p.Put(cc, nil)
	}
	return err
}

type thriftFactory struct {
	addr             string
	transportFactory thrift.TTransportFactory
	protocolFactory  thrift.TProtocolFactory
}

func NewThriftFactory(addr string) *thriftFactory {
	return &thriftFactory{
		addr:             addr,
		transportFactory: thrift.NewTBufferedTransportFactory(8192),
		protocolFactory:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
}

func (t *thriftFactory) Open() (interface{}, error) {
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

func (t *thriftFactory) Close(conn interface{}) error {
	c := conn.(*client)
	return c.trans.Close()
}

type ServiceClient struct {
	service  string
	registry *Registry
	clients  map[string]*Client
}

func NewServiceClient(registry *Registry, service string, opt *Options) *ServiceClient {
	return &ServiceClient{
		registry: registry,
		service:  service,
		clients:  make(map[string]*Client),
	}
}

func (c *ServiceClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	return nil
}
