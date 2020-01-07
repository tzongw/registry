package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/server"
	"github.com/tzongw/registry/shared"
	"os"
	"os/signal"
	"syscall"
	"time"
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
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch
	shared.Registry.Stop()
	time.Sleep(3 * time.Second) // wait requests done
	log.Info("exit")
}
