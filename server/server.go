package server

import (
	"github.com/micro/go-micro/util/addr"
	"net"
)

func hostPort(hp string) (host, port string, err error)  {
	if host, port, err = net.SplitHostPort(hp); err != nil {
		return
	}
	host, err = addr.Extract(host)
	return
}
