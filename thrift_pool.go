package main

import (
	"github.com/apache/thrift/lib/go/thrift"
)

type thriftPool struct {
	addr string
	transportFactory thrift.TTransportFactory
	protocolFactory thrift.TProtocolFactory
}

func NewThriftPool(addr string) *thriftPool  {
	return &thriftPool{
		addr:addr,
		transportFactory : thrift.NewTBufferedTransportFactory(8192),
		protocolFactory : thrift.NewTBinaryProtocolFactoryDefault(),
	}
}

func (p *thriftPool) Open() (interface{}, error) {
	var transport thrift.TTransport
	transport, err := thrift.NewTSocket(p.addr)
	if err != nil {
		return nil, err
	}
	transport, err = p.transportFactory.GetTransport(transport)
	if err != nil {
		return nil, err
	}
	if err := transport.Open(); err != nil {
		return nil, err
	}
	return p.protocolFactory.GetProtocol(transport), nil
}

func (p *thriftPool) Close(conn interface{}) error {
	trans := conn.(thrift.TProtocol)
	return trans.Transport().Close()
}


