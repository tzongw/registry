package server

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/shared"
	"net"
	"net/http"
	"strings"
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
	params := make(map[string]string)
	for k := range r.Header {
		if strings.HasPrefix(k, "X-") {
			params[k[len("X-"):]] = r.Header.Get(k)
		} else if k == "Cookie" {
			params[k] = r.Header.Get(k)
		}
	}
	query := r.URL.Query()
	for k := range query {
		params[k] = query.Get(k)
	}
	err = shared.UserClient.Login(common.RandomCtx, rpcAddr, connId, params)
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
	go pinger()
	return net.JoinHostPort(host, port)
}
