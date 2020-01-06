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
	writeWait        = common.PingInterval
	readWait         = common.MissTimes * common.PingInterval
	maxMessageSize   = 100 * 1024
	writeChannelSize = 256
)

var clients sync.Map

var ErrNotExist = errors.New("not exist")

func findClient(connId string) (*client, error) {
	v, ok := clients.Load(connId)
	if !ok {
		return nil, ErrNotExist
	}
	return v.(*client), nil
}

type client struct {
	Id      string
	conn    *websocket.Conn
	writeC  chan interface{}
	stopped int32
	ctxM    sync.Mutex
	ctx     map[string]string
	Groups  map[string]interface{} // protected by GLOBAL groupsMutex
}

func newClient(id string, conn *websocket.Conn) *client {
	return &client{
		Id:     id,
		conn:   conn,
		writeC: make(chan interface{}, writeChannelSize),
	}
}

func (c *client) String() string {
	return fmt.Sprintf("%s %+v", c.Id, c.Context())
}

func (c *client) Serve() {
	log.Info("serve start ", c)
	defer func() {
		log.Info("serve stop ", c)
		shared.UserClient.Disconnect(shared.DefaultCtx, rpcAddr, c.Id, c.Context())
		c.Stop()
	}()
	go c.ping()
	go c.write()
	c.conn.SetReadLimit(maxMessageSize)
	h := c.conn.PingHandler()
	c.conn.SetPingHandler(func(appData string) error {
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
			shared.UserClient.RecvBinary(shared.DefaultCtx, rpcAddr, c.Id, c.Context(), message)
		case websocket.TextMessage:
			shared.UserClient.RecvText(shared.DefaultCtx, rpcAddr, c.Id, c.Context(), string(message))
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

func (c *client) Context() map[string]string  {
	c.ctxM.Lock()
	c.ctxM.Unlock()
	return c.ctx
}

func (c *client) SetContext(context map[string]string) {
	c.ctxM.Lock()
	c.ctxM.Unlock()
	c.ctx = common.MergeMap(c.ctx, context)
	log.Info("set context ", c)
}

func (c *client) UnsetContext(context []string) {
	c.ctxM.Lock()
	c.ctxM.Unlock()
	m := common.MergeMap(c.ctx, nil) // make a copy, DONT modify c.ctx directly
	for _, k := range context {
		delete(m, k)
	}
	c.ctx = m
}

func (c *client) SendMessage(message interface{}) {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return
	}
	defer func() {
		if v := recover(); v != nil {
			log.Warnf("panic %+v %s", v, c) // race: channel may closed
			return
		}
	}()
	select {
	case c.writeC <- message:
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
		shared.UserClient.Ping(shared.DefaultCtx, rpcAddr, c.Id, c.Context())
	}
}

func (c *client) write() {
	log.Debug("write start ", c)
	defer log.Debug("write stop ", c)
	defer c.conn.Close()
	for m := range c.writeC {
		var mType int
		var message []byte
		switch t := m.(type) {
		case string:
			mType = websocket.TextMessage
			message = []byte(t)
		case []byte:
			mType = websocket.BinaryMessage
			message = t
		default:
			log.Errorf("unknown message %+v %s", m, c)
			return
		}
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(mType, message); err != nil {
			log.Error(err)
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

var ErrNotInGroup = errors.New("not in group")

func joinGroup(connId, group string) error {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	if c.Groups == nil {
		c.Groups = make(map[string]interface{}, 1)
	}
	c.Groups[group] = nil
	g, ok := groups[group]
	if !ok {
		log.Debug("create group ", group)
		g = &groupInfo{}
		groups[group] = g
	}
	g.clients.Store(c, nil)
	g.count += 1
	return nil
}

func leaveGroup(connId, group string) error {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	delete(c.Groups, group)
	return removeFromGroup(c, group)
}

// ONLY use by leaveGroup & cleanGroup
func removeFromGroup(c *client, group string) error {
	g, ok := groups[group]
	if !ok {
		return ErrNotInGroup
	}
	g.clients.Delete(c)
	g.count -= 1
	if g.count == 0 {
		delete(groups, group)
		log.Debug("delete group ", group)
	}
	return nil
}

func broadcastMessage(group string, exclude []string, message interface{}) {
	groupsMutex.Lock()
	g, ok := groups[group]
	groupsMutex.Unlock()
	if !ok {
		return
	}
	// this may take a while
	g.clients.Range(func(key, _ interface{}) bool {
		c := key.(*client)
		index := common.FindIndex(len(exclude), func(i int) bool {
			return c.Id == exclude[i]
		})
		if index < 0 {
			c.SendMessage(message)
		}
		return true
	})
}

func cleanGroup(c *client) {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	for group := range c.Groups {
		removeFromGroup(c, group)
	}
}
