package base

import (
	"context"
	"errors"
	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

type client struct {
	*thrift.TStandardClient
	thrift.TTransport
}

type ThriftFactory struct {
	addr             string
	timeout          time.Duration
	transportFactory thrift.TTransportFactory
	protocolFactory  thrift.TProtocolFactory
}

func NewThriftFactory(addr string, timeout time.Duration) *ThriftFactory {
	return &ThriftFactory{
		addr:             addr,
		timeout:          timeout,
		transportFactory: thrift.NewTBufferedTransportFactory(8192),
		protocolFactory:  thrift.NewTBinaryProtocolFactoryDefault(),
	}
}

func (t *ThriftFactory) Open() (*client, error) {
	sock, err := thrift.NewTSocketTimeout(t.addr, t.timeout)
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
	return &client{TStandardClient: thrift.NewTStandardClient(iprot, oprot), TTransport: transport}, nil
}

func (t *ThriftFactory) Close(c *client) error {
	return c.Close()
}

type AddrClient struct {
	*Pool[*client]
}

func NewAddrClient(addr string, opt Options) *AddrClient {
	return &AddrClient{
		NewPool[*client](NewThriftFactory(addr, opt.Timeout), opt),
	}
}

func (c *AddrClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	cc, err := c.Get()
	if err != nil {
		return err
	}
	err = cc.Call(ctx, method, args, result)
	c.Put(cc, err)
	return err
}

var ErrUnavailable = errors.New("unavailable")

type ServiceClient struct {
	service        string
	registry       *Registry
	opt            Options
	mu             sync.Mutex
	clients        map[string]*AddrClient
	localAddresses sort.StringSlice
	goodAddresses  sort.StringSlice
	coolDown       map[string]time.Time
}

func NewServiceClient(registry *Registry, service string, opt Options) *ServiceClient {
	c := &ServiceClient{
		registry: registry,
		service:  service,
		opt:      opt,
		clients:  make(map[string]*AddrClient),
		coolDown: make(map[string]time.Time),
	}
	registry.AddCallback(c.updateAddresses)
	go c.reapCoolDown()
	return c
}

func (c *ServiceClient) reapCoolDown() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		count := len(c.coolDown)
		c.mu.Unlock()
		if count > 0 {
			c.updateAddresses()
		}
	}
}

func (c *ServiceClient) updateAddresses() {
	addresses := c.registry.Addresses(c.service)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for addr, cd := range c.coolDown {
		if now.After(cd) {
			delete(c.coolDown, addr)
			log.Infof("- cool down %+v %+v", c.service, addr)
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
	c.mu.Lock()
	_, ok := c.coolDown[addr]
	c.coolDown[addr] = time.Now().Add(CoolDown)
	c.mu.Unlock()
	if !ok {
		log.Errorf("+ cool down %+v %+v", c.service, addr)
		c.updateAddresses()
	}
}

type tSelector int

var selector any = tSelector(0)

func WithAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, selector, "addr:"+addr)
}

func WithHint(ctx context.Context, hint string) context.Context {
	return context.WithValue(ctx, selector, "hint:"+hint)
}

func Broadcast(ctx context.Context) context.Context {
	return context.WithValue(ctx, selector, "broadcast")
}

func (c *ServiceClient) preferredAddresses() sort.StringSlice {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.localAddresses) > 0 {
		return c.localAddresses
	}
	return c.goodAddresses
}

func (c *ServiceClient) Call(ctx context.Context, method string, args, result thrift.TStruct) error {
	value := "random"
	if v := ctx.Value(selector); v != nil {
		value = v.(string)
	}
	if value == "broadcast" {
		if result != nil {
			panic("broadcast MUST be oneway")
		}
		var err error
		addresses := c.registry.Addresses(c.service)
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
	var addr string
	if strings.HasPrefix(value, "addr:") {
		addr = value[5:]
		if _, ok := c.coolDown[addr]; ok && !Contains(c.registry.Addresses(c.service), addr) {
			return ErrUnavailable // cooling down unhealthy address
		}
	} else {
		addresses := c.preferredAddresses()
		if len(addresses) == 0 {
			return ErrUnavailable
		}
		if strings.HasPrefix(value, "hint:") {
			h := fnv.New64a()
			hint := value[5:]
			if _, err := h.Write([]byte(hint)); err != nil {
				return err
			}
			addr = addresses[h.Sum64()%uint64(len(addresses))]
		} else if value == "random" {
			i := rand.Intn(len(addresses))
			addr = addresses[i]
		} else {
			panic("UNKNOWN value: " + value)
		}
	}
	client := c.client(addr)
	err := client.Call(ctx, method, args, result)
	if err != nil {
		c.addCoolDown(addr)
	}
	return err
}

func (c *ServiceClient) client(addr string) *AddrClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, ok := c.clients[addr]
	if !ok {
		client = NewAddrClient(addr, c.opt)
		c.clients[addr] = client
	}
	return client
}

func (c *ServiceClient) ConnClient(addr string, f func(conn thrift.TClient) error) error {
	addrClient := c.client(addr)
	conn, err := addrClient.Get()
	if err != nil {
		return err
	}
	err = f(conn)
	addrClient.Put(conn, err)
	return err
}
