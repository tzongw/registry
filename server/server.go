package server

import (
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

type client struct {
	id      string
	conn    *websocket.Conn
	writeC  chan interface{}
	stopped int32
	context map[string]string
}

func newClient(id string, conn *websocket.Conn) *client {
	return &client{
		id:      id,
		conn:    conn,
		writeC:  make(chan interface{}, writeChannelSize),
		context: make(map[string]string),
	}
}

func (c *client) Serve() {
	log.Debug("serve start ", c.id)
	defer func() {
		log.Debug("serve stop ", c.id)
		shared.UserClient.Disconnect(shared.DefaultCtx, rpcAddr, c.id, c.context)
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
			log.Error(err)
			break
		}
		switch mType {
		case websocket.BinaryMessage:
			shared.UserClient.RecvBinary(shared.DefaultCtx, rpcAddr, c.id, c.context, message)
		case websocket.TextMessage:
			shared.UserClient.RecvText(shared.DefaultCtx, rpcAddr, c.id, c.context, string(message))
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

}

func (c *client) UnsetContext(context []string) {

}

func (c *client) SendMessage(message interface{}) {
	if atomic.LoadInt32(&c.stopped) == 1 {
		return
	}
	defer func() {
		if v := recover(); v != nil {
			log.Warnf("panic %+v %+v", v, c.id) // race: channel closed
			return
		}
	}()
	select {
	case c.writeC <- message:
	default:
		log.Errorf("channel full +%v", c.id)
		c.Stop()
	}
}

func (c *client) ping() {
	log.Debug("ping start ", c.id)
	defer log.Debug("ping stop ", c.id)
	for {
		time.Sleep(common.PingInterval)
		if atomic.LoadInt32(&c.stopped) == 1 {
			break
		}
		shared.UserClient.Ping(shared.DefaultCtx, rpcAddr, c.id, c.context)
	}
}

func (c *client) write() {
	log.Debug("write start ", c.id)
	defer log.Debug("write stop ", c.id)
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
			break
		}
		c.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := c.conn.WriteMessage(mType, message); err != nil {
			log.Error(err)
			break
		}
	}
}
