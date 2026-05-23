package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":18080", "http service address")
var start = flag.Int("start", 0, "start uid")
var count = flag.Int("count", 1000, "uid count")
var tick = flag.Int("tick", 100, "msg tick")

func main() {
	flag.Parse()
	log.SetFlags(0)
	wg := sync.WaitGroup{}
	wg.Add(*count)
	var joinCount atomic.Int64
	var exit atomic.Bool
	var groupTick *time.Ticker
	if (*tick) > 0 {
		groupTick = time.NewTicker(time.Duration(*tick) * time.Millisecond)
		defer groupTick.Stop()
	}
	var exitTick *time.Ticker
	for i := range *count {
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
				// 连接失败也计入 joinCount，避免其余连接因此永远等待
				joinCount.Add(1)
				return
			}
			defer c.Close()
			go func() {
				for {
					_, m, err := c.ReadMessage()
					if err != nil {
						log.Println(uid, " read: ", err)
						return
					}
					if uid == *start {
						log.Println(string(m))
					}
				}
			}()

			pingTick := time.NewTicker(45 * time.Second)
			defer pingTick.Stop()
			joined := false
			for {
				var ch <-chan time.Time
				if exit.Load() {
					ch = exitTick.C
				} else if groupTick != nil && (!joined || joinCount.Load() == int64(*count)) {
					ch = groupTick.C
				}
				select {
				case <-pingTick.C:
					err := c.WriteMessage(websocket.PingMessage, []byte{})
					if err != nil {
						log.Println(uid, " write: ", err)
						return
					}
				case <-ch:
					if exit.Load() {
						c.Close()
						return
					}
					var msg string
					if joined {
						msg = fmt.Sprintf("hello %d", uid)
					} else {
						time.Sleep(100 * time.Millisecond)
						msg = "join"
					}
					err := c.WriteMessage(websocket.TextMessage, []byte(msg))
					if err != nil {
						log.Println(uid, " write: ", err)
						return
					}
					if !joined {
						joined = true
						joinCount.Add(1)
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
		}(*start + i)
		time.Sleep(10 * time.Millisecond)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	<-ch
	exitTick = time.NewTicker(10 * time.Millisecond)
	exit.Store(true)
	wg.Wait()
}
