package server

import (
	"context"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/common"
	"net"
	"net/http"
	"os"
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
	connId := uuid.New().String()
	client := newClient(connId, conn)
	clients.Store(connId, client)
	count := atomic.AddInt64(&clientCount, 1)
	log.Debug("++ client count ", count)
	defer func() {
		_ = conn.Close()
		cleanClient(client)
		count := atomic.AddInt64(&clientCount, -1)
		log.Debug("-- client count ", count)
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
	err = common.UserClient.Login(context.Background(), rpcAddr, connId, params)
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
	if strings.HasPrefix(addr, "/") {
		_ = os.Remove(addr)
		ln, err := net.Listen("unix", addr)
		if err != nil {
			log.Fatal(err)
		}
		if err = os.Chmod(addr, 0777); err != nil {
			log.Fatal(err)
		}
		log.Info("listen unix ", addr)
		server := &http.Server{Addr: addr, Handler: nil}
		go func() {
			if err := server.Serve(ln); err != nil {
				log.Fatal(err)
			}
		}()
		return "unix://" + addr
	} else {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatal(err)
		}
		addr = ln.Addr().String()
		log.Info("listen tcp ", addr)
		host, port, err := base.HostPort(addr)
		if err != nil {
			log.Fatal(err)
		}
		server := &http.Server{Addr: addr, Handler: nil}
		go func() {
			if err := server.Serve(ln); err != nil {
				log.Fatal(err)
			}
		}()
		return net.JoinHostPort(host, port)
	}
}
