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
	readWait         = common.MissTimes * common.PingInterval
	writeWait        = 3 * time.Second
	maxMessageSize   = 100 * 1024
	writeChannelSize = 128
)

var clients sync.Map
var clientCount int64

var ErrNotExist = errors.New("not exist")

func findClient(connId string) (*client, error) {
	v, ok := clients.Load(connId)
	if !ok {
		return nil, ErrNotExist
	}
	return v.(*client), nil
}

type message struct {
	Type    int
	Content []byte
}

var pingMessage = &message{Type: websocket.PingMessage}

type client struct {
	Id      string
	conn    *websocket.Conn
	writeC  chan *message
	stopped int32
	ctx     atomic.Value
	Groups  map[string]struct{} // protected by GLOBAL groupsMutex
}

func newClient(id string, conn *websocket.Conn) *client {
	return &client{
		Id:     id,
		conn:   conn,
		writeC: make(chan *message, writeChannelSize),
	}
}

func (c *client) String() string {
	return fmt.Sprintf("%+v %+v", c.Id, c.Context())
}

func (c *client) Serve() {
	log.Info("serve start ", c)
	defer func() {
		log.Info("serve stop ", c)
		shared.UserClient.Disconnect(common.RandomCtx, rpcAddr, c.Id, c.Context())
		c.Stop()
	}()
	go c.ping()
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
			shared.UserClient.RecvBinary(common.RandomCtx, rpcAddr, c.Id, c.Context(), message)
		case websocket.TextMessage:
			shared.UserClient.RecvText(common.RandomCtx, rpcAddr, c.Id, c.Context(), string(message))
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

func (c *client) Context() map[string]string {
	if v := c.ctx.Load(); v != nil {
		return v.(map[string]string)
	}
	return nil
}

func (c *client) SetContext(context map[string]string) {
	m := common.MergeMap(c.Context(), context)
	c.ctx.Store(m)
	log.Info("set context ", c)
}

func (c *client) UnsetContext(context []string) {
	m := common.MergeMap(c.Context(), nil) // make a copy, DONT modify content
	for _, k := range context {
		delete(m, k)
	}
	c.ctx.Store(m)
	log.Info("unset context ", c)
}

func (c *client) SendText(content string) {
	c.sendMessage(&message{Type: websocket.TextMessage, Content: []byte(content)})
}

func (c *client) SendBinary(content []byte) {
	c.sendMessage(&message{Type: websocket.BinaryMessage, Content: content})
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

func (c *client) ping() {
	log.Debug("ping start ", c)
	defer log.Debug("ping stop ", c)
	for {
		time.Sleep(common.PingInterval)
		if atomic.LoadInt32(&c.stopped) == 1 {
			return
		}
		c.sendMessage(pingMessage)
		shared.UserClient.Ping(common.RandomCtx, rpcAddr, c.Id, c.Context())
	}
}

func (c *client) write() {
	log.Debug("write start ", c)
	defer log.Debug("write stop ", c)
	defer c.conn.Close()
	for m := range c.writeC {
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(m.Type, m.Content); err != nil {
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
	if _, ok := c.Groups[group]; ok {
		return ErrAlreadyInGroup // maybe join multi times
	}
	if c.Groups == nil {
		c.Groups = make(map[string]struct{}, 1)
	}
	c.Groups[group] = struct{}{}
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
	if _, ok := c.Groups[group]; !ok {
		return ErrNotInGroup // maybe leave multi times
	}
	delete(c.Groups, group)
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
	broadcastMessage(group, exclude, &message{Type: websocket.TextMessage, Content: []byte(content)})
}

func broadcastBinary(group string, exclude []string, content []byte) {
	broadcastMessage(group, exclude, &message{Type: websocket.BinaryMessage, Content: content})
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
			return c.Id == exclude[i]
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
	clients.Delete(c.Id) // join & leave no-op after this
	for group := range c.Groups {
		removeFromGroup(c, group)
	}
}
