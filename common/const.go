package common

import "time"

const (
	PingInterval = 45 * time.Second
	WsTimeout    = 60 * time.Second
	RpcPingStep  = 10
	RpcUser      = "rpc_user"
	RpcGate      = "rpc_gate"
	WsGate       = "ws_gate"
)

var Services = []string{RpcUser}
