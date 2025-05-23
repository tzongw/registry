package main

import (
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/server"
)

var debug = flag.Bool("debug", false, "debug state")
var addr = flag.String("addr", ":0", "http service address")

func main() {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
		go http.ListenAndServe("localhost:6060", nil)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	common.InitShared()
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	common.Registry.Start(map[string]string{
		common.RpcGate:  server.RpcServe(),
		common.HttpGate: server.WsServe(*addr),
	})
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	<-ch
	common.Registry.Stop()
	time.Sleep(time.Second) // wait requests done
	if strings.HasPrefix(*addr, "/") {
		_ = os.Remove(*addr)
	}
	log.Info("exit")
}
