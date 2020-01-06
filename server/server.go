package server

import (
	"errors"
	"github.com/gorilla/websocket"
	"github.com/micro/go-micro/util/addr"
	log "github.com/sirupsen/logrus"
	"net"
	"registry/common"
	"registry/shared"
	"sync"
	"sync/atomic"
	"time"
)

func hostPort(hp string) (host, port string, err error) {
	if host, port, err = net.SplitHostPort(hp); err != nil {
		return
	}
	host, err = addr.Extract(host)
	return
}

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
	m       sync.Mutex
	context atomic.Value
}

var emptyContext map[string]string

func newClient(id string, conn *websocket.Conn) *client {
	c := &client{
		Id:     id,
		conn:   conn,
		writeC: make(chan interface{}, writeChannelSize),
	}
	c.context.Store(emptyContext)
	return c
}

func (c *client) Serve() {
	log.Debug("serve start ", c.Id)
	defer func() {
		log.Debug("serve stop ", c.Id)
		shared.UserClient.Disconnect(shared.DefaultCtx, rpcAddr, c.Id, c.context.Load().(map[string]string))
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
			log.Warn(err)
			return
		}
		switch mType {
		case websocket.BinaryMessage:
			shared.UserClient.RecvBinary(shared.DefaultCtx, rpcAddr, c.Id, c.context.Load().(map[string]string), message)
		case websocket.TextMessage:
			shared.UserClient.RecvText(shared.DefaultCtx, rpcAddr, c.Id, c.context.Load().(map[string]string), string(message))
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

func (c *client) SetContext(context map[string]string) {
	c.m.Lock() // in case covered by another
	c.m.Unlock()
	m := common.MergeMap(c.context.Load().(map[string]string), context)
	c.context.Store(m)
}

func (c *client) UnsetContext(context []string) {
	c.m.Lock() // in case covered by another
	c.m.Unlock()
	m := common.MergeMap(c.context.Load().(map[string]string), nil) // make a copy
	for _, k := range context {
		delete(m, k)
	}
	c.context.Store(m)
}

func (c *client) SendMessage(message interface{}) {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return
	}
	defer func() {
		if v := recover(); v != nil {
			log.Warnf("panic %+v %+v", v, c.Id) // race: channel closed
			return
		}
	}()
	select {
	case c.writeC <- message:
	default:
		log.Errorf("channel full +%v", c.Id)
		c.Stop()
	}
}

func (c *client) ping() {
	log.Debug("ping start ", c.Id)
	defer log.Debug("ping stop ", c.Id)
	for {
		time.Sleep(common.PingInterval)
		if atomic.LoadInt32(&c.stopped) == 1 {
			return
		}
		shared.UserClient.Ping(shared.DefaultCtx, rpcAddr, c.Id, c.context.Load().(map[string]string))
	}
}

func (c *client) write() {
	log.Debug("write start ", c.Id)
	defer log.Debug("write stop ", c.Id)
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
			log.Errorf("unknown message %+v", m)
			return
		}
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(mType, message); err != nil {
			log.Error(err)
			return
		}
	}
}

var groups sync.Map
var groupsMutex sync.Mutex

var ErrNotInGroup = errors.New("not in group")

func joinGroup(connId, group string) error {
	groupsMutex.Lock()
	defer groupsMutex.Unlock()
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	temp, _ := groups.LoadOrStore(group, &sync.Map{})
	groupClients := temp.(*sync.Map)
	groupClients.Store(c, nil)
	return nil
}

func leaveGroup(connId, group string) error {
	c, err := findClient(connId)
	if err != nil {
		return err
	}
	temp, ok := groups.Load(group)
	if !ok {
		return ErrNotInGroup
	}
	groupClients := temp.(*sync.Map)
	groupClients.Delete(c)
	return nil
}

func broadcastMessage(group string, exclude []string, message interface{}) {
	temp, ok := groups.Load(group)
	if !ok {
		return
	}
	groupClients := temp.(*sync.Map)
	groupClients.Range(func(key, _ interface{}) bool {
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
}
