package server

import (
	"context"
	"errors"
	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
	"registry/gen-go/service"
)

var rpcAddr string
var ErrNotExist = errors.New("conn not exist")

type gateHandler struct {
}

func (g *gateHandler) client(conn_id string) (*client, error) {
	v, ok := clients.Load(conn_id)
	if !ok {
		log.Warnf("%+v not exist", conn_id)
		return nil, ErrNotExist
	}
	return v.(*client), nil
}

func (g *gateHandler) SetContext(ctx context.Context, conn_id string, context map[string]string) (err error) {
	c, err := g.client(conn_id)
	if err != nil {
		return
	}
	c.SetContext(context)
	return
}

func (g *gateHandler) UnsetContext(ctx context.Context, conn_id string, context []string) (err error) {
	c, err := g.client(conn_id)
	if err != nil {
		return
	}
	c.UnsetContext(context)
	return
}
func (g *gateHandler) RemoveConn(ctx context.Context, conn_id string) (err error) {
	c, err := g.client(conn_id)
	if err != nil {
		return
	}
	c.Stop()
	return
}
func (g *gateHandler) SendText(ctx context.Context, conn_id string, message string) (err error) {
	c, err := g.client(conn_id)
	if err != nil {
		return
	}
	c.SendMessage(message)
	return
}
func (g *gateHandler) SendBinary(ctx context.Context, conn_id string, message []byte) (err error) {
	c, err := g.client(conn_id)
	if err != nil {
		return
	}
	c.SendMessage(message)
	return
}

func (g *gateHandler) JoinGroup(ctx context.Context, conn_id string, group string) (err error) {
	return
}

func (g *gateHandler) LeaveGroup(ctx context.Context, conn_id string, group string) (err error) {
	return
}

func (g *gateHandler) BroadcastBinary(ctx context.Context, group string, exclude []string, message []byte) (err error) {
	return
}

func (g *gateHandler) BroadcastText(ctx context.Context, group string, exclude []string, message string) (err error) {
	return
}

func RpcServe() (addr string) {
	transport, err := thrift.NewTServerSocket(":0")
	if err != nil {
		log.Fatal(err)
	}
	err = transport.Listen()
	if err != nil {
		log.Fatal(err)
	}
	addr = transport.Addr().String()
	host, port, err := hostPort(addr)
	if err != nil {
		log.Fatal(err)
	}
	addr = host + ":" + port
	rpcAddr = addr
	transportFactory := thrift.NewTBufferedTransportFactory(8192)
	protocolFactory := thrift.NewTBinaryProtocolFactoryDefault()
	handler := &gateHandler{}
	processor := service.NewGateProcessor(handler)
	server := thrift.NewTSimpleServer4(processor, transport, transportFactory, protocolFactory)
	go func() {
		err = server.Serve()
		if err != nil {
			log.Fatal(err)
		}
	}()
	return
}
