package common

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
	conn.Close()
}

// Extract returns a real ip
func Extract(addr string) (string, error) {
	// if addr specified then its returned
	if len(addr) > 0 && (addr != "0.0.0.0" && addr != "[::]" && addr != "::") {
		return addr, nil
	}

	return LocalIP, nil
}
