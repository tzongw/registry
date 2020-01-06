package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/server"
	"github.com/tzongw/registry/shared"
)

var addr = flag.String("addr", ":40001", "ws service address")

func main() {
	flag.Parse()
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	shared.Registry.Start(map[string]string{
		common.RpcGate: server.RpcServe(),
		common.WsGate:  server.WsServe(*addr),
	})
	select {}
}
