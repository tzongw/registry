package common

import "time"

const (
	PingInterval = 20 * time.Second
	MissTimes    = 3
	RpcUser      = "rpc_user"
	RpcGate      = "rpc_gate"
	WsGate       = "ws_gate"
)
