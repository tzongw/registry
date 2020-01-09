package server

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/shared"
	"math/rand"
	"net"
	"net/http"
	"sync/atomic"
	"time"
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
	//go testGateService()
	return host + ":" + port
}

func testGateService() {
	for {
		time.Sleep(3 * time.Second)
		count := int(atomic.LoadInt64(&clientCount))
		if count > 0 {
			selected := rand.Intn(count)
			i := 0
			clients.Range(func(connId, _ interface{}) bool {
				if selected == i {
					shared.GateClient.SendText(common.WithNode(rpcAddr), connId.(string), "unicast message")
					return false
				}
				i++
				return true
			})
		}
		shared.GateClient.BroadcastText(common.BroadcastCtx, "chat_room", nil, "broadcast message")
	}
}
