package server

import (
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/shared"
)

const (
	readWait       = 3 * common.PingInterval
	writeWait      = time.Second
	maxMessageSize = 100 * 1024
	idleWait       = 5 * time.Second
)

var clients sync.Map
var clientCount int64

var errNotExist = errors.New("not exist")

func findClient(connId string) (*client, error) {
	v, ok := clients.Load(connId)
	if !ok {
		return nil, errNotExist
	}
	return v.(*client), nil
}

type message struct {
	typ     int
	content []byte
}

var pingMessage = &message{typ: websocket.PingMessage}

type client struct {
	id      string
	conn    *websocket.Conn
	ctx     map[string]string
	mu      sync.Mutex
	ch      chan *message
	backlog []*message
	writing bool                // write goroutine is running
	groups  map[string]struct{} // protected by GLOBAL groupsMutex
}

func newClient(id string, conn *websocket.Conn) *client {
	return &client{
		id:   id,
		conn: conn,
		ch:   make(chan *message, 1),
	}
}

func (c *client) String() string {
	return c.id
}

func (c *client) Serve() {
	log.Debug("serve start ", c)
	timer := time.AfterFunc(common.PingInterval, c.ping)
	defer func() {
		log.Debug("serve stop ", c)
		timer.Stop()
		c.Stop()
		_ = shared.UserClient.Disconnect(common.RandomCtx, rpcAddr, c.id, c.context())
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
		mType, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Debug(err)
			return
		}
		switch mType {
		case websocket.BinaryMessage:
			_ = shared.UserClient.RecvBinary(common.RandomCtx, rpcAddr, c.id, c.context(), message)
		case websocket.TextMessage:
			_ = shared.UserClient.RecvText(common.RandomCtx, rpcAddr, c.id, c.context(), string(message))
		default:
			log.Errorf("unknown message %+v, %+v", mType, message)
		}
	}
}

func (c *client) Stop() {
	// some msg may in backlog, can not close ch,  send nil msg as close
	c.sendMessage(nil)
}

func (c *client) context() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ctx
}

func (c *client) SetContext(key string, value string) {
	log.Debugf("%+v: %+v %+v", c, key, value)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ctx = common.MergeMap(c.ctx, map[string]string{key: value}) // make a copy, DONT modify content
}

func (c *client) UnsetContext(key string, value string) {
	log.Debugf("%+v: %+v %+v", c, key, value)
	c.mu.Lock()
	defer c.mu.Unlock()
	if value == "" || c.ctx[key] == value {
		m := common.MergeMap(c.ctx, nil) // make a copy, DONT modify content
		delete(m, key)
		c.ctx = m
	}
}

func (c *client) SendText(content string) {
	c.sendMessage(&message{typ: websocket.TextMessage, content: []byte(content)})
}

func (c *client) SendBinary(content []byte) {
	c.sendMessage(&message{typ: websocket.BinaryMessage, content: content})
}

func (c *client) sendMessage(msg *message) {
	c.mu.Lock()
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
			c.mu.Unlock()
			return
		default:
		}
	}
	c.backlog = append(c.backlog, msg)
	c.mu.Unlock()
}

func (c *client) ping() {
	c.sendMessage(pingMessage)
	_ = shared.UserClient.Ping(common.RandomCtx, rpcAddr, c.id, c.context())
}

func (c *client) writeOne(m *message) bool {
	if m == nil {
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
	if err := c.conn.WriteMessage(m.typ, m.content); err != nil {
		log.Infof("write error %+v %+v", err, c)
		_ = c.conn.Close()
		return false
	}
	return true
}

func (c *client) exitWrite() bool {
	c.mu.Lock()
	if len(c.ch) == 0 {
		c.writing = false
		c.mu.Unlock()
		return true
	}
	c.mu.Unlock()
	return false
}

func (c *client) longWrite() {
	t := time.NewTimer(idleWait)
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

func (c *client) shortWrite() {
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

type groupInfo struct {
	clients sync.Map // broadcast can iterate without lock
	count   int
}

var groups = make(map[string]*groupInfo)
var groupsMutex sync.Mutex

var errAlreadyInGroup = errors.New("already in group")
var errNotInGroup = errors.New("not in group")

func joinGroup(connId, group string) error {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	if _, ok := c.groups[group]; ok {
		return errAlreadyInGroup // maybe join multi times
	}
	if c.groups == nil {
		c.groups = make(map[string]struct{})
	}
	c.groups[group] = struct{}{}
	g, ok := groups[group]
	if !ok {
		g = &groupInfo{}
		groups[group] = g
		log.Debugf("create group %+v, groups: %d", group, len(groups))
	}
	g.clients.Store(c, nil)
	g.count++
	return nil
}

func leaveGroup(connId, group string) error {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	if _, ok := c.groups[group]; !ok {
		return errNotInGroup // maybe leave multi times
	}
	delete(c.groups, group)
	removeFromGroup(c, group)
	return nil
}

// ONLY use by leaveGroup & cleanClient; c MUST in group
func removeFromGroup(c *client, group string) {
	g := groups[group]
	g.clients.Delete(c)
	g.count--
	if g.count == 0 {
		delete(groups, group)
		log.Debugf("delete group %+v, groups: %d", group, len(groups))
	}
}

func broadcastText(group string, exclude []string, content string) {
	broadcastMessage(group, exclude, &message{typ: websocket.TextMessage, content: []byte(content)})
}

func broadcastBinary(group string, exclude []string, content []byte) {
	broadcastMessage(group, exclude, &message{typ: websocket.BinaryMessage, content: content})
}

func broadcastMessage(group string, exclude []string, msg *message) {
	groupsMutex.Lock()
	g, ok := groups[group]
	groupsMutex.Unlock()
	if !ok {
		return
	}
	// this may take a while
	go g.clients.Range(func(key, _ any) bool {
		c := key.(*client)
		if !common.Contains(exclude, c.id) {
			c.sendMessage(msg)
		}
		return true
	})
}

func cleanClient(c *client) {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	clients.Delete(c.id) // join & leave no-op after this
	for group := range c.groups {
		removeFromGroup(c, group)
	}
}
