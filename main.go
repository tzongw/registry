package main

import (
	log "github.com/sirupsen/logrus"
	"registry/common"
	"registry/server"
	"registry/shared"
)

func main() {
	log.SetLevel(log.InfoLevel)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	shared.Registry.Start(map[string]string{
		common.RpcGate: server.RpcServe(),
	})
	select {}
}
