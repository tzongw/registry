package server

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"net"
	"net/http"
	"github.com/tzongw/registry/shared"
	"sync/atomic"
)

var upgrader = websocket.Upgrader{}

func wsHandle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		return
	}
	defer conn.Close()
	connId := uuid.New().String()
	client := newClient(connId, conn)
	clients.Store(connId, client)
	count := atomic.AddInt64(&clientCount, 1)
	log.Info("++ client count ", count)
	defer func() {
		cleanClient(client)
		count := atomic.AddInt64(&clientCount, -1)
		log.Info("-- client count ", count)
	}()
	v := r.URL.Query()
	params := make(map[string]string, len(v))
	for k := range v {
		params[k] = v.Get(k)
	}
	err = shared.UserClient.Login(shared.DefaultCtx, rpcAddr, connId, params)
	if err != nil {
		log.Error(err)
		return
	}
	client.Serve()
}

func WsServe(addr string) string {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		wsHandle(w, r)
	})
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	addr = ln.Addr().String()
	log.Info("listen ", addr)
	host, port, err := common.HostPort(addr)
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: addr, Handler: nil}
	go func() {
		if err := server.Serve(ln); err != nil {
			log.Fatal(err)
		}
	}()
	return host + ":" + port
}
