package base

import (
	"net"
)

func HostPort(hp string) (host, port string, err error) {
	if host, port, err = net.SplitHostPort(hp); err != nil {
		return
	}
	host, err = Extract(host)
	return
}
