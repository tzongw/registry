package server

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/tzongw/registry/common"
	"github.com/tzongw/registry/shared"
	"sync"
	"sync/atomic"
	"time"
)

const (
	readWait         = 3 * common.PingInterval
	writeWait        = time.Second
	maxMessageSize   = 100 * 1024
	writeChannelSize = 128
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
	writeC  chan *message
	stopped int32
	ctx     map[string]string
	ctxL    sync.Mutex
	groups  map[string]struct{} // protected by GLOBAL groupsMutex
}

func newClient(id string, conn *websocket.Conn) *client {
	return &client{
		id:     id,
		conn:   conn,
		writeC: make(chan *message, writeChannelSize),
	}
}

func (c *client) String() string {
	return fmt.Sprintf("%+v %+v", c.id, c.context())
}

func (c *client) Serve() {
	log.Info("serve start ", c)
	defer func() {
		log.Info("serve stop ", c)
		shared.UserClient.Disconnect(common.RandomCtx, rpcAddr, c.id, c.context())
		c.Stop()
	}()
	addPing(c)
	go c.write()
	c.conn.SetReadLimit(maxMessageSize)
	h := c.conn.PongHandler()
	c.conn.SetPongHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(readWait))
		return h(appData)
	})
	for {
		c.conn.SetReadDeadline(time.Now().Add(readWait))
		mType, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Info(err)
			return
		}
		switch mType {
		case websocket.BinaryMessage:
			shared.UserClient.RecvBinary(common.RandomCtx, rpcAddr, c.id, c.context(), message)
		case websocket.TextMessage:
			shared.UserClient.RecvText(common.RandomCtx, rpcAddr, c.id, c.context(), string(message))
		default:
			log.Errorf("unknown message %+v, %+v", mType, message)
		}
	}
}

func (c *client) Stop() {
	if atomic.CompareAndSwapInt32(&c.stopped, 0, 1) {
		close(c.writeC)
	}
}

func (c *client) context() map[string]string {
	c.ctxL.Lock()
	defer c.ctxL.Unlock()
	return c.ctx
}

func (c *client) SetContext(key string, value string) {
	log.Infof("%+v: %+v %+v", c, key, value)
	c.ctxL.Lock()
	defer c.ctxL.Unlock()
	c.ctx = common.MergeMap(c.ctx, map[string]string{key: value}) // make a copy, DONT modify content
}

func (c *client) UnsetContext(key string, value string) {
	log.Info("%+v: %+v %+v", c, key, value)
	c.ctxL.Lock()
	defer c.ctxL.Unlock()
	m := common.MergeMap(c.ctx, nil) // make a copy, DONT modify content
	if m[key] == value || value == "" {
		delete(m, key)
	}
	c.ctx = m
}

func (c *client) SendText(content string) {
	c.sendMessage(&message{typ: websocket.TextMessage, content: []byte(content)})
}

func (c *client) SendBinary(content []byte) {
	c.sendMessage(&message{typ: websocket.BinaryMessage, content: content})
}

func (c *client) sendMessage(msg *message) {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return
	}
	defer func() {
		if v := recover(); v != nil {
			log.Warnf("panic %+v %+v", v, c) // race: channel may closed
			return
		}
	}()
	select {
	case c.writeC <- msg:
	default:
		log.Error("channel full ", c)
		c.Stop()
	}
}

func (c *client) Ping() bool {
	if atomic.LoadInt32(&c.stopped) == 1 {
		log.Debug("ping stop ", c)
		return false
	}
	c.sendMessage(pingMessage)
	shared.UserClient.Ping(common.RandomCtx, rpcAddr, c.id, c.context())
	return true
}

func (c *client) write() {
	log.Debug("write start ", c)
	defer log.Debug("write stop ", c)
	defer c.conn.Close()
	for m := range c.writeC {
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(m.typ, m.content); err != nil {
			log.Infof("write error %+v %+v", err, c)
			return
		}
	}
}

type groupInfo struct {
	clients sync.Map // broadcast can iterate without lock
	count   int
}

var groups = make(map[string]*groupInfo)
var groupsMutex sync.Mutex

var ErrAlreadyInGroup = errors.New("already in group")
var ErrNotInGroup = errors.New("not in group")

func joinGroup(connId, group string) error {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	if _, ok := c.groups[group]; ok {
		return ErrAlreadyInGroup // maybe join multi times
	}
	if c.groups == nil {
		c.groups = make(map[string]struct{})
	}
	c.groups[group] = struct{}{}
	g, ok := groups[group]
	if !ok {
		g = &groupInfo{}
		groups[group] = g
		log.Infof("create group %+v, groups: %d", group, len(groups))
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
		return ErrNotInGroup // maybe leave multi times
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
		log.Infof("delete group %+v, groups: %d", group, len(groups))
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
	go g.clients.Range(func(key, _ interface{}) bool {
		c := key.(*client)
		index := common.FindIndex(len(exclude), func(i int) bool {
			return c.id == exclude[i]
		})
		if index < 0 {
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
