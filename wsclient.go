package y

import (
	"context"
	"github.com/gorilla/websocket"
	"net"
	"time"
)

const (
	writeChanSize = 256
	readTimeout   = 10 * time.Second
	writeTimeout  = 10 * time.Second

	tagClient = "C"
	tagServer = "S"
)

type WSClient struct {
	conn      *websocket.Conn
	id        uint64
	closed    bool
	writeChan chan []byte
	hbChan    chan struct{}
	cancel    context.CancelFunc

	userID   interface{}
	lastTime time.Time

	Parser IParser
	State  IState
}

func (c *WSClient) read(tag string) {
	defer RecoverError()
	defer c.Close()

	if tag == tagServer {
		c.conn.SetPingHandler(func(message string) error {
			Debugf("%s IClient.read got ping id = %d", tag, c.id)

			c.lastTime = time.Now()

			err := c.conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(writeTimeout))
			if err == websocket.ErrCloseSent {
				return nil
			} else if e, ok := err.(net.Error); ok && e.Temporary() {
				return nil
			}

			//Debug("PONG")
			return err
		})
	} else if tag == tagClient {
		c.conn.SetPongHandler(func(message string) error {
			Debugf("%s IClient.read got pong id = %d", tag, c.id)
			return nil
		})
	}

	for {
		_, buf, err := c.conn.ReadMessage()
		if err != nil {
			if !c.closed {
				Errorf("%s IClient.read error id = %d, %+v", tag, c.id, err)
			}
			return
		}

		err = c.Parser.OnNewData(c, buf)
		if err != nil {
			Errorf("%s IClient.read %+v", tag, err)
			break
		}

		c.lastTime = time.Now()
	}
}

func (c *WSClient) write(ctx context.Context, tag string) {
	defer RecoverError()

	for {
		select {
		case buf := <-c.writeChan:
			c.conn.SetWriteDeadline(time.Now().Add(time.Duration(writeTimeout)))
			err := c.conn.WriteMessage(websocket.BinaryMessage, buf)
			if err != nil {
				Errorf("%s IClient.write error id = %d, %+v, %+v", tag, c.id, err, buf)
			}
		case <-c.hbChan:
			err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Duration(writeTimeout)))
			if err != nil {
				Errorf("%s IClient.write ping error id = %d, %+v, %+v", tag, c.id, err)
			}
		case <-ctx.Done():
			Debugf("%s IClient.write done id = %d", tag, c.id)
			return
		}
	}
}

func (c *WSClient) startHeartbeat(ctx context.Context, dur time.Duration) {
	defer RecoverError()

	ticker := time.NewTicker(dur)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.hbChan <- struct{}{}
		case <-ctx.Done():
			Debugf("IClient.StartHeartbeat done id = %d", c.id)
			return
		}
	}
}

func (c *WSClient) Close() {
	defer RecoverError()

	if !c.closed {
		c.closed = true
		c.cancel()
		c.conn.Close()

		if c.State != nil {
			c.State.OnClose(c)
		}
	}
}

func (c *WSClient) Send(data []byte) {
	if len(data) == 0 {
		return
	}
	c.writeChan <- data
}

func (c *WSClient) ConnId() uint64 {
	return c.id
}

func (c *WSClient) UserId() interface{} {
	return c.userID
}

func (c *WSClient) LastTime() time.Time {
	return c.lastTime
}

func newWSClientServerSide(ctx context.Context, id uint64, conn *websocket.Conn, userId interface{}, parser IParser, state IState) (client *WSClient, err error) {
	ctx, cancel := context.WithCancel(ctx)
	client = &WSClient{
		conn:      conn,
		id:        id,
		closed:    false,
		writeChan: make(chan []byte, writeChanSize),
		userID:    userId,
		lastTime:  time.Now(),
		cancel:    cancel,
		Parser:    parser,
		State:     state,
	}

	go client.read(tagServer)
	go client.write(ctx, tagServer)

	return
}

type WSClientDialer struct {
	*websocket.Dialer
	Heartbeat time.Duration
}

func (d *WSClientDialer) Dial(ctx context.Context, connection string, id uint64) (client *WSClient, err error) {
	ctx, cancel := context.WithCancel(ctx)
	conn, _, err := d.DialContext(ctx, connection, nil)
	if err != nil {
		return
	}

	client = &WSClient{
		conn:      conn,
		id:        id,
		closed:    false,
		writeChan: make(chan []byte, writeChanSize),
		hbChan:    make(chan struct{}, 1),
		lastTime:  time.Now(),
		cancel:    cancel,
	}

	go client.read(tagClient)
	go client.write(ctx, tagClient)
	if d.Heartbeat > 0 {
		go client.startHeartbeat(ctx, d.Heartbeat)
	}

	return
}
