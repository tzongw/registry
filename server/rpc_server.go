package server

import (
	"context"
	"github.com/apache/thrift/lib/go/thrift"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/gen-go/service"
)

var rpcAddr string

type gateHandler struct {
}

func (g *gateHandler) SetContext(ctx context.Context, connId string, key string, value string) (err error) {
	c, err := findClient(connId)
	if err != nil {
		return
	}
	c.SetContext(key, value)
	return
}

func (g *gateHandler) UnsetContext(ctx context.Context, connId string, key string, value string) (err error) {
	c, err := findClient(connId)
	if err != nil {
		return
	}
	c.UnsetContext(key, value)
	return
}

func (g *gateHandler) RemoveConn(ctx context.Context, connId string) (err error) {
	c, err := findClient(connId)
	if err != nil {
		return
	}
	c.Stop()
	return
}

func (g *gateHandler) SendText(ctx context.Context, connId string, message string) (err error) {
	c, err := findClient(connId)
	if err != nil {
		return
	}
	c.SendText(message)
	return
}

func (g *gateHandler) SendBinary(ctx context.Context, connId string, message []byte) (err error) {
	c, err := findClient(connId)
	if err != nil {
		return
	}
	c.SendBinary(message)
	return
}

func (g *gateHandler) JoinGroup(ctx context.Context, connId string, group string) (err error) {
	err = joinGroup(connId, group)
	return
}

func (g *gateHandler) LeaveGroup(ctx context.Context, connId string, group string) (err error) {
	err = leaveGroup(connId, group)
	return
}

func (g *gateHandler) BroadcastBinary(ctx context.Context, group string, exclude []string, message []byte) (err error) {
	broadcastBinary(group, exclude, message)
	return
}

func (g *gateHandler) BroadcastText(ctx context.Context, group string, exclude []string, message string) (err error) {
	broadcastText(group, exclude, message)
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
	log.Info("listen ", addr)
	host, port, err := base.HostPort(addr)
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
		if err := server.AcceptLoop(); err != nil {
			log.Fatal(err)
		}
	}()
	return
}
