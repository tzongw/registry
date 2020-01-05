package server

import (
	log "github.com/sirupsen/logrus"
	"net/http"
)

func serveWs(w http.ResponseWriter, r *http.Request) {

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
