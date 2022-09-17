package main

import (
	"flag"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/server"
	"github.com/tzongw/registry/shared"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var debug = flag.Bool("debug", false, "debug state")
var addr = flag.String("addr", ":0", "ws service address")

func main() {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
		go http.ListenAndServe("localhost:6060", nil)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	shared.Registry.Start(map[string]string{
		base.RpcGate: server.RpcServe(),
		base.WsGate:  server.WsServe(*addr),
	})
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch
	shared.Registry.Stop()
	time.Sleep(time.Second) // wait requests done
	log.Info("exit")
}
