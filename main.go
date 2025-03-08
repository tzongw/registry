package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/tzongw/registry/common"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	// WebSocket 服务器的地址
	wsURL = "ws://localhost:18080/ws"
	// 压测的并发数
	concurrentConnections = 2000
	// 每个连接发送的消息数量
	messagesPerConnection = 10000
)

// 客户端连接结构体
type Client struct {
	conn *websocket.Conn
}

// 发送消息到 WebSocket 服务器
func (c *Client) sendMessage(typ int, message string) error {
	return c.conn.WriteMessage(typ, []byte(message))
}

func (c *Client) readMessage() string {
	_ = c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, content, _ := c.conn.ReadMessage()
	return string(content)
}

// 客户端逻辑
func clientRoutine(ctx context.Context, id int) {
	url := fmt.Sprintf("%s?uid=%d&token=token", wsURL, id)
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		log.Printf("client %d: dial error: %v", id, err)
		return
	}
	defer c.Close()

	client := &Client{conn: c}
	for i := 0; i < messagesPerConnection; i++ {
		for {
			s := client.readMessage()
			if s == "" {
				break
			}
			log.Printf("client %d: recv: %s", id, s)
		}
		time.Sleep(common.PingInterval)
		if i == 0 {
			if err := client.sendMessage(websocket.TextMessage, "join"); err != nil {
				log.Printf("client %d: send error: %v", id, err)
				return
			}
		} else {
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

	var wg sync.WaitGroup
	for i := 0; i < concurrentConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			clientRoutine(ctx, id)
		}(i)
		if i%100 == 0 {
			time.Sleep(1 * time.Second)
		}
	}
	wg.Wait()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch
}
