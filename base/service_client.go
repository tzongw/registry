package base

import (
	"context"
	"errors"
	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"sort"
	"sync"
	"time"
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

type AddrClient struct {
	p *Pool[client]
}

func NewAddrClient(addr string, opt *Options) *AddrClient {
	return &AddrClient{
		p: NewPool[client](NewThriftFactory(addr), opt),
	}
}

func (c *AddrClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	cc, err := c.p.Get()
	if err != nil {
		return err
	}
	err = cc.Call(ctx, method, args, result)
	c.p.Put(cc, err)
	return err
}

func (c *AddrClient) Close() {
	c.p.Close()
}

var ErrUnavailable = errors.New("unavailable")

type ServiceClient struct {
	service        string
	registry       *Registry
	opt            *Options
	m              sync.Mutex
	clients        map[string]*AddrClient
	localAddresses sort.StringSlice
	goodAddresses  sort.StringSlice
	coolDown       map[string]time.Time
}

func NewServiceClient(registry *Registry, service string, opt *Options) *ServiceClient {
	c := &ServiceClient{
		registry: registry,
		service:  service,
		opt:      opt,
		clients:  make(map[string]*AddrClient),
	}
	registry.AddCallback(c.updateAddresses)
	go c.reapCoolDown()
	return c
}

func (c *ServiceClient) reapCoolDown() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.m.Lock()
		count := len(c.coolDown)
		c.m.Unlock()
		if count > 0 {
			c.updateAddresses()
		}
	}
}

func (c *ServiceClient) updateAddresses() {
	log.Infof("update %+v", c.service)
	addresses := c.registry.Addresses(c.service)
	c.m.Lock()
	defer c.m.Unlock()
	now := time.Now()
	for addr, cd := range c.coolDown {
		if now.After(cd) {
			delete(c.coolDown, addr)
		}
	}
	c.goodAddresses = nil
	c.localAddresses = nil
	for _, addr := range addresses {
		if _, ok := c.coolDown[addr]; ok {
			continue
		}
		c.goodAddresses = append(c.goodAddresses, addr)
		host, _, _ := HostPort(addr)
		if host == LocalIP {
			c.localAddresses = append(c.localAddresses, addr)
		}
	}
	for addr, client := range c.clients {
		if !Contains(addresses, addr) {
			log.Infof("close client %+v %+v", c.service, addr)
			client.Close()
			delete(c.clients, addr)
		}
	}
}

func (c *ServiceClient) addCoolDown(addr string) {
	c.m.Lock()
	_, ok := c.coolDown[addr]
	c.coolDown[addr] = time.Now().Add(CoolDown)
	c.m.Unlock()
	if !ok {
		log.Warnf("+ cool down %+v %+v", c.service, addr)
		c.updateAddresses()
	}
}

var RandomCtx = context.Background()

type tNodeSelector int

var selector any = tNodeSelector(0)
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
			err := client.Call(ctx, method, args, result)
			if err != nil {
				c.addCoolDown(addr)
			}
			return err
		} else {
			if result != nil {
				panic("broadcast MUST be oneway")
			}
			var err error
			for _, addr := range addresses {
				client := c.client(addr)
				e := client.Call(ctx, method, args, result)
				if e != nil {
					c.addCoolDown(addr)
					err = e
				}
			}
			return err
		}
	} else {
		c.m.Lock()
		if len(c.localAddresses) > 0 {
			addresses = c.localAddresses
		} else if len(c.goodAddresses) > 0 {
			addresses = c.goodAddresses
		}
		c.m.Unlock()
		i := rand.Intn(len(addresses))
		addr := addresses[i]
		client := c.client(addr)
		err := client.Call(ctx, method, args, result)
		if err != nil {
			c.addCoolDown(addr)
		}
		return err
	}
}

func (c *ServiceClient) client(addr string) *AddrClient {
	c.m.Lock()
	defer c.m.Unlock()
	client, ok := c.clients[addr]
	if !ok {
		client = NewAddrClient(addr, c.opt)
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
