package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/tzongw/registry/common"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	// WebSocket 服务器的地址
	wsURL = "ws://localhost:18080/ws"
	// 压测的并发数
	concurrentConnections = 20000
)

// 客户端连接结构体
type Client struct {
	conn *websocket.Conn
}

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
func clientRoutine(ctx context.Context, id int, stop chan struct{}) {
	url := fmt.Sprintf("%s?uid=%d&token=token", wsURL, id)
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		log.Printf("client %d: dial error: %v", id, err)
		return
	}
	defer c.Close()

	client := &Client{conn: c}
	go client.readMessage(id)
	time.Sleep(time.Second)
	t := time.NewTicker(common.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := make(map[int]chan struct{}, concurrentConnections)
	for i := 0; i < concurrentConnections; i++ {
		stop := make(chan struct{})
		m[i] = stop
		go clientRoutine(ctx, i, stop)
		if i%100 == 0 {
			time.Sleep(1 * time.Second)
		}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch
	for i := 0; i < concurrentConnections; i++ {
		stop := m[i]
		close(stop)
		if i%100 == 0 {
			time.Sleep(1 * time.Second)
		}
	}
}
