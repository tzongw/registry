package server

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"net/http"
	"registry/shared"
)

var upgrader = websocket.Upgrader{}

func serveWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		return
	}
	defer conn.Close()
	connId := uuid.New().String()
	client := newClient(connId, conn)
	clients.Store(connId, client)
	defer clients.Delete(connId)
	err = shared.UserClient.Login(shared.DefaultCtx, rpcAddr, connId, nil)
	if err != nil {
		log.Error(err)
		return
	}
	client.serve()
}

func WsServe(addr string) string {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(w, r)
	})
	go func() {
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatal(err)
		}
	}()
	host, port, err := hostPort(addr)
	if err != nil {
		log.Fatal(err)
	}
	return host + ":" + port
}
