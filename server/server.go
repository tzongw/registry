package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/base"
	"github.com/tzongw/registry/common"
)

const (
	readWait       = 3 * common.PingInterval
	writeWait      = time.Second
	maxMessageSize = 100 * 1024
	groupShards    = 29
)

var clients = base.NewMap[string, *Client](base.StringHash[string], 389)

var errNotExist = errors.New("not exist")

func findClient(connId string) (*Client, error) {
	v, ok := clients.Load(connId)
	if !ok {
		return nil, errNotExist
	}
	return v, nil
}

type message struct {
	typ        int
	recyclable bool
	content    []byte
}

var messagePool = sync.Pool{New: func() any { return &message{recyclable: true} }}

type Client struct {
	id       string
	conn     *websocket.Conn
	mu       sync.Mutex
	ctx      map[string]string
	groups   map[string]struct{}
	messages []*message
	writing  bool         // write goroutine is running
	exiting  bool         // client is exiting
	step     atomic.Int32 // ping step
}

func newClient(id string, conn *websocket.Conn) *Client {
	return &Client{
		id:   id,
		conn: conn,
	}
}

func (c *Client) Serve() {
	defer func() {
		c.Stop()
		_ = common.UserClient.Disconnect(context.Background(), rpcAddr, c.id, c.context())
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPingHandler(func(appData string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(readWait))
		c.rpcPing()
		return nil
	})
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(readWait))
		typ, content, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		ctx := base.WithHint(context.Background(), c.id)
		switch typ {
		case websocket.BinaryMessage:
			if err = common.UserClient.RecvBinary(ctx, rpcAddr, c.id, c.context(), content); err != nil {
				log.Errorf("service not available %+v", err)
				return
			}
		case websocket.TextMessage:
			if err = common.UserClient.RecvText(ctx, rpcAddr, c.id, c.context(), string(content)); err != nil {
				log.Errorf("service not available %+v", err)
				return
			}
		default:
			log.Errorf("unknown message %+v, %+v", typ, content)
		}
	}
}

func (c *Client) Stop() {
	// send nil msg as close
	c.sendMessage(nil)
}

func (c *Client) context() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctx
}

func (c *Client) SetContext(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ctx = base.MergeMap(c.ctx, map[string]string{key: value}) // make a copy, DONT modify content
}

func (c *Client) UnsetContext(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if value == "" || c.ctx[key] == value {
		m := base.MergeMap(c.ctx, nil) // make a copy, DONT modify content
		delete(m, key)
		c.ctx = m
	}
}

func (c *Client) SendText(content string) {
	var msg = messagePool.Get().(*message)
	msg.typ = websocket.TextMessage
	msg.content = []byte(content)
	c.sendMessage(msg)
}

func (c *Client) SendBinary(content []byte) {
	var msg = messagePool.Get().(*message)
	msg.typ = websocket.BinaryMessage
	msg.content = content
	c.sendMessage(msg)
}

func (c *Client) sendMessage(msg *message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
	if !c.writing {
		c.writing = true
		go c.writer()
	}
}

func (c *Client) rpcPing() {
	if c.step.Add(1) < common.RpcPingStep {
		return
	}
	c.step.Store(0)
	ctx := base.WithHint(context.Background(), c.id)
	if err := common.UserClient.Ping(ctx, rpcAddr, c.id, c.context()); err != nil {
		log.Errorf("service not available %+v", err)
		return
	}
}

func (c *Client) writeOne(msg *message) bool {
	if msg == nil {
		_ = c.conn.Close()
		return false
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if msg.recyclable {
		defer messagePool.Put(msg)
	}
	if err := c.conn.WriteMessage(msg.typ, msg.content); err != nil {
		_ = c.conn.Close()
		return false
	}
	return true
}

func (c *Client) writer() {
	for {
		c.mu.Lock()
		if len(c.messages) == 0 {
			c.writing = false
			c.mu.Unlock()
			return
		}
		messages := c.messages
		c.messages = nil
		c.mu.Unlock()
		for _, m := range messages {
			if !c.writeOne(m) {
				return // keep writing status true
			}
		}
	}
}

type Group struct {
	*base.Map[*Client, struct{}]
}

var groups = base.NewMap[string, Group](base.StringHash[string], 769)

var errAlreadyInGroup = errors.New("already in group")
var errNotInGroup = errors.New("not in group")
var errClientExiting = errors.New("client exiting")

func joinGroup(connId, group string) error {
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exiting {
		return errClientExiting
	}
	if _, ok := c.groups[group]; ok {
		return errAlreadyInGroup // maybe join multi times
	}
	if c.groups == nil {
		c.groups = make(map[string]struct{})
	}
	c.groups[group] = struct{}{}
	groups.CreateOrOperate(group, func() Group {
		g := Group{base.NewMap[*Client, struct{}](base.PointerHash[Client], groupShards)}
		g.Store(c, struct{}{})
		return g
	}, func(g Group) bool {
		g.Store(c, struct{}{})
		return false
	})
	return nil
}

func leaveGroup(connId, group string) error {
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.groups[group]; !ok {
		return errNotInGroup // maybe leave multi times
	}
	delete(c.groups, group)
	removeFromGroup(c, group)
	return nil
}

// ONLY use by leaveGroup & cleanClient; c MUST in group, so will NEVER create
func removeFromGroup(c *Client, group string) {
	groups.CreateOrOperate(group, nil, func(g Group) bool {
		g.Delete(c)
		return g.Size() == 0
	})
}

func broadcastText(group string, exclude []string, content string) {
	broadcastMessage(group, exclude, &message{typ: websocket.TextMessage, content: []byte(content)})
}

func broadcastBinary(group string, exclude []string, content []byte) {
	broadcastMessage(group, exclude, &message{typ: websocket.BinaryMessage, content: content})
}

func broadcastMessage(group string, exclude []string, msg *message) {
	g, ok := groups.Load(group)
	if !ok {
		return
	}
	g.Range(func(c *Client, _ struct{}) bool {
		if !base.Contains(exclude, c.id) {
			c.sendMessage(msg)
		}
		return true
	})
}

func cleanClient(c *Client) {
	clients.Delete(c.id)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exiting = true // NO joinGroup after this
	for group := range c.groups {
		removeFromGroup(c, group)
	}
	c.groups = nil // NO leaveGroup after this
}
