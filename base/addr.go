package base

import (
	"net"
	"strings"
)

var (
	LocalIP string
)

func init() {
	ip := net.ParseIP("8.8.8.8")
	dstAddr := &net.UDPAddr{IP: ip, Port: 9}
	conn, err := net.DialUDP("udp", nil, dstAddr)
	if err != nil {
		panic(err)
	}
	addr := conn.LocalAddr().String()
	ss := strings.SplitN(addr, ":", 2)
	LocalIP = ss[0]
	_ = conn.Close()
}

// Extract returns a real ip
func Extract(host string) (string, error) {
	// if host specified then its returned
	if len(host) > 0 && host != "0.0.0.0" && host != "[::]" && host != "::" {
		return host, nil
	}

	return LocalIP, nil
}
