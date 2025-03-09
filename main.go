package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/tzongw/registry/common"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// WebSocket 服务器的地址
var wsURLs = []string{"ws://[::1]:18080/ws", "ws://127.0.0.1:18080/ws"}

// 压测的并发数
const concurrentConnections = 30000

// 客户端连接结构体
type Client struct {
	conn *websocket.Conn
}

var wg sync.WaitGroup

// 发送消息到 WebSocket 服务器
func (c *Client) sendMessage(typ int, message string) error {
	return c.conn.WriteMessage(typ, []byte(message))
}

func (c *Client) readMessage(id int) {
	for {
		_, s, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("client %d: exit", id)
			return
		}
		log.Printf("client %d: recv: %s", id, s)
	}
}

// 客户端逻辑
func clientRoutine(ctx context.Context, id int) {
	defer wg.Done()
	url := fmt.Sprintf("%s?uid=%d&token=token", wsURLs[id%len(wsURLs)], id)
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		log.Printf("client %d: dial error: %v", id, err)
		return
	}
	defer c.Close()

	client := &Client{conn: c}
	go client.readMessage(id)
	time.Sleep(time.Second)
	d := rand.Int63n(int64(common.WsTimeout - common.PingInterval - 5*time.Second))
	t := time.NewTicker(common.PingInterval + time.Duration(d))
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := client.sendMessage(websocket.PingMessage, "ping"); err != nil {
				log.Printf("client %d: ping error: %v", id, err)
				return
			}
		}
	}
}

func main() {
	m := make(map[int]context.CancelFunc, concurrentConnections)
	wg.Add(concurrentConnections)
	for i := 0; i < concurrentConnections; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		m[i] = cancel
		go clientRoutine(ctx, i)
		time.Sleep(10 * time.Millisecond)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch
	for i := 0; i < concurrentConnections; i++ {
		cancel := m[i]
		cancel()
		time.Sleep(10 * time.Millisecond)
	}
	wg.Wait()
}
