package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":18080", "http service address")

func main() {
	flag.Parse()
	log.SetFlags(0)
	count := 10000
	wg := sync.WaitGroup{}
	wg.Add(count)
	for i := 0; i < count; i++ {
		go func(uid int) {
			defer wg.Done()
			q := url.Values{}
			q.Add("uid", fmt.Sprintf("%v", uid))
			q.Add("token", "pass")
			u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws", RawQuery: q.Encode()}
			log.Printf("connecting to %s", u.String())

			c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Println("dial:", err)
				return
			}
			defer c.Close()
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("%d recv: %s", uid, message)
			for {
				time.Sleep(10 * time.Second)
				err := c.WriteMessage(websocket.PingMessage, []byte{})
				if err != nil {
					log.Println("read:", err)
					return
				}
			}
		}(i)
		time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()
}
