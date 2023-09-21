package server

import (
	"context"
	"errors"
	"math/rand"
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
	groupShards    = 32
)

var clients = base.NewMap[string, *Client](base.StringHash[string], 128)
var timerPool sync.Pool

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
var pingMessage = &message{typ: websocket.PingMessage}

type Client struct {
	id      string
	conn    *websocket.Conn
	mu      sync.Mutex
	ctx     map[string]string
	groups  map[string]struct{}
	ch      chan *message
	backlog []*message
	writing bool         // write goroutine is running
	exiting bool         // client is exiting
	step    atomic.Int32 // ping step
}

func newClient(id string, conn *websocket.Conn) *Client {
	return &Client{
		id:   id,
		conn: conn,
		ch:   make(chan *message, 1),
	}
}

func (c *Client) String() string {
	return c.id
}

func (c *Client) Serve() {
	log.Debug("serve start ", c)
	timer := time.AfterFunc(common.PingInterval, c.ping)
	defer func() {
		log.Debug("serve stop ", c)
		timer.Stop()
		c.Stop()
		_ = common.UserClient.Disconnect(context.Background(), rpcAddr, c.id, c.context())
	}()
	c.conn.SetReadLimit(maxMessageSize)
	h := c.conn.PongHandler()
	c.conn.SetPongHandler(func(appData string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(readWait))
		timer.Reset(common.PingInterval)
		return h(appData)
	})
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(readWait))
		typ, content, err := c.conn.ReadMessage()
		if err != nil {
			log.Debug(err)
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
	// some msg may in backlog, can not close ch,  send nil msg as close
	c.sendMessage(nil)
}

func (c *Client) context() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctx
}

func (c *Client) SetContext(key string, value string) {
	log.Debugf("%+v: %+v %+v", c, key, value)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ctx = base.MergeMap(c.ctx, map[string]string{key: value}) // make a copy, DONT modify content
}

func (c *Client) UnsetContext(key string, value string) {
	log.Debugf("%+v: %+v %+v", c, key, value)
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
	if len(c.backlog) == 0 {
		select {
		case c.ch <- msg:
			if !c.writing {
				c.writing = true
				if msg == pingMessage {
					go c.shortWrite()
				} else {
					go c.longWrite()
				}
			}
			return
		default:
		}
	}
	c.backlog = append(c.backlog, msg)
}

func (c *Client) ping() {
	c.sendMessage(pingMessage)
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
		log.Debug("stopped ", c)
		_ = c.conn.Close()
		return false
	}
	// try load next msg, ch may be filled
	c.mu.Lock()
	if len(c.backlog) > 0 {
		select {
		case c.ch <- c.backlog[0]:
			c.backlog[0] = nil
			c.backlog = c.backlog[1:]
		default:
		}
	}
	c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if msg.recyclable {
		defer messagePool.Put(msg)
	}
	if err := c.conn.WriteMessage(msg.typ, msg.content); err != nil {
		log.Infof("write error %+v %+v", err, c)
		_ = c.conn.Close()
		return false
	}
	return true
}

func (c *Client) exitWrite() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.ch) == 0 {
		c.writing = false
		return true
	}
	return false
}

func (c *Client) longWrite() {
	var t *time.Timer
	idleWait := common.PingInterval/4 + time.Duration(rand.Int63n(int64(common.PingInterval/2)))
	if v := timerPool.Get(); v != nil {
		t = v.(*time.Timer)
		t.Reset(idleWait)
	} else {
		t = time.NewTimer(idleWait)
	}
	defer timerPool.Put(t)
	for {
		select {
		case m := <-c.ch:
			if !t.Stop() {
				<-t.C
			}
			if !c.writeOne(m) {
				return
			}
		case <-t.C:
			if c.exitWrite() {
				return
			}
		}
		t.Reset(idleWait)
	}
}

func (c *Client) shortWrite() {
	for {
		select {
		case m := <-c.ch:
			if !c.writeOne(m) {
				return
			}
		default:
			if c.exitWrite() {
				return
			}
		}
	}
}

type Group struct {
	*base.Map[*Client, struct{}]
}

var groups = base.NewMap[string, Group](base.StringHash[string], 512)

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
	if c.exiting {
		return errClientExiting
	}
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
	go g.Range(func(c *Client, _ struct{}) bool {
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
	c.exiting = true // join & leave no-op after this
	for group := range c.groups {
		removeFromGroup(c, group)
	}
}
