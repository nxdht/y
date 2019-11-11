package y

import (
	"container/list"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultNoResponseDuration    = 90 * time.Second
	defaultCheckResponseDuration = 60 * time.Second
	defaultOnConnChangedChanSize = 20000
)

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
)

type IAuth interface {
	OnAuth(r *http.Request) interface{}
}

type WSServer struct {
	server *http.Server
	ctx    context.Context
	cancel context.CancelFunc

	caFile   string
	certFile string
	keyFile  string

	clientPing bool

	idGenerator IDGenerator
	clientState clientState

	onClientChangedChan chan *clientChanged

	hubMutex sync.RWMutex
	hub      map[uint64]IClient
	hubUsers map[interface{}]IClient

	Auth          IAuth
	Parser        IParser
	ClientChanged IClientChanged
}

type WSServerConfig struct {
	AccessPath     string
	ListenPort     int
	Auth           IAuth
	Parser         IParser
	ClientCallback IClientChanged

	CaFile   string
	CertFile string
	KeyFile  string

	ClientPing bool
}

type clientChanged struct {
	isAdd          bool
	connId         uint64
	userId         interface{}
	beKickedClient IClient
}

func NewWSServer(ctx context.Context, config *WSServerConfig) (*WSServer, error) {
	if config == nil {
		return nil, errors.New("nil parameter config")
	}
	if config.ListenPort <= 0 {
		return nil, errors.New("invalid listen port")
	}
	if !strings.HasPrefix(config.AccessPath, "/") {
		config.AccessPath = "/" + config.AccessPath
	}

	server := &WSServer{
		Auth:                config.Auth,
		Parser:              config.Parser,
		ClientChanged:       config.ClientCallback,
		caFile:              config.CaFile,
		certFile:            config.CertFile,
		keyFile:             config.KeyFile,
		clientPing:          config.ClientPing,
		idGenerator:         NewUint64IDGenerator(),
		onClientChangedChan: make(chan *clientChanged, defaultOnConnChangedChanSize),
		hub:                 make(map[uint64]IClient),
		hubUsers:            make(map[interface{}]IClient),
	}

	server.ctx, server.cancel = context.WithCancel(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc(config.AccessPath, server.handleHttpConnect)

	server.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.ListenPort),
		Handler:      mux,
		ReadTimeout:  time.Duration(readTimeout),
		WriteTimeout: time.Duration(writeTimeout),
	}
	server.clientState.server = server

	return server, nil
}

func NewWSServerFromConfigFile(ctx context.Context, fileName string) (*WSServer, error) {
	var cfg WSServerConfig
	err := LoadXml(fileName, &cfg)
	if err != nil {
		return nil, err
	}

	return NewWSServer(ctx, &cfg)
}

func (ws *WSServer) handleHttpConnect(w http.ResponseWriter, r *http.Request) {
	var userId interface{}
	if ws.Auth != nil {
		if userId = ws.Auth.OnAuth(r); userId == nil {
			w.Write([]byte("Auth failed"))
			return
		}
	} else {
		userId = ws.idGenerator.Next()
	}

	ws.hubMutex.Lock()
	if _, ok := ws.hubUsers[userId]; ok {
		ws.hubMutex.Unlock()
		w.Write([]byte("user conflict"))
		return
	}
	ws.hubMutex.Unlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		Error(err)
		return
	}

	client, err := newWSClientServerSide(ws.ctx, ws.idGenerator.Next().(uint64), conn, userId, ws.clientPing, ws.Parser, &ws.clientState)
	if err != nil {
		Error(err)
		return
	}

	client.State.OnOpen(client)
}

func (ws *WSServer) Start() {
	go func() {
		ticker := time.NewTicker(defaultCheckResponseDuration)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ws.checkInvalidConn()
			case <-ws.ctx.Done():
				goto END
			}
		}

	END:
		Info("WSServer.Start go routine check invalid conn ended")
	}()

	go func() {
		for {
			select {
			case temp, ok := <-ws.onClientChangedChan:
				if ok {
					if temp.isAdd {
						if ws.ClientChanged != nil {
							ws.ClientChanged.OnAdded(temp.connId, temp.userId)
						}
					} else {
						if ws.ClientChanged != nil {
							ws.ClientChanged.OnRemoved(temp.connId, temp.userId, temp.beKickedClient)
						}
					}
				}
			case <-ws.ctx.Done():
				goto END
			}
		}

	END:
		Info("WSServer.Start go routine hub ended")
	}()

	go func() {
		ws.server.BaseContext = func(net.Listener) context.Context {
			return ws.ctx
		}
		ws.server.ConnContext = func(context.Context, net.Conn) context.Context {
			return ws.ctx
		}

		var err error
		if ws.certFile != "" && ws.keyFile != "" {
			if ws.caFile != "" {
				pool := x509.NewCertPool()
				caCrt, err := ioutil.ReadFile(ws.caFile)
				Panic(err)
				pool.AppendCertsFromPEM(caCrt)
				ws.server.TLSConfig = &tls.Config{
					ClientCAs:  pool,
					ClientAuth: tls.RequireAndVerifyClientCert,
				}
			}
			err = ws.server.ListenAndServeTLS(ws.certFile, ws.keyFile)
		} else {
			err = ws.server.ListenAndServe()
		}
		Error(err)
	}()
}

func (ws *WSServer) Close() {
	ws.cancel()
	ws.server.Close()
}

func (ws *WSServer) checkInvalidConn() {
	defer RecoverError()

	t := time.Now().Add(-defaultNoResponseDuration).Unix()
	l := list.New()

	ws.hubMutex.RLock()
	for _, client := range ws.hub {
		if client.LastTime().Unix() < t {
			l.PushBack(client)
		}
	}
	ws.hubMutex.RUnlock()

	for e := l.Front(); e != nil; e = e.Next() {
		e.Value.(IClient).Close()
	}
}

func (ws *WSServer) IsTls() bool {
	return ws.certFile != "" && ws.keyFile != ""
}

func (ws *WSServer) IClientByUserId(userId interface{}) IClient {
	ws.hubMutex.RLock()
	defer ws.hubMutex.RUnlock()

	return ws.hubUsers[userId]
}

func (ws *WSServer) IClientById(id uint64) IClient {
	ws.hubMutex.RLock()
	defer ws.hubMutex.RUnlock()

	return ws.hub[id]
}

type clientState struct {
	server *WSServer
}

func (c *clientState) OnOpen(client IClient) {
	c.server.hubMutex.Lock()
	defer c.server.hubMutex.Unlock()

	c.server.hub[client.ConnId()] = client
	if client.UserId() != nil {
		c.server.hubUsers[client.UserId()] = client
	}

	c.server.onClientChangedChan <- &clientChanged{
		isAdd:  true,
		connId: client.ConnId(),
		userId: client.UserId(),
	}
}

func (c *clientState) OnClose(client IClient) {
	c.server.hubMutex.Lock()
	defer c.server.hubMutex.Unlock()

	delete(c.server.hub, client.ConnId())
	if client.UserId() != nil {
		delete(c.server.hubUsers, client.UserId())
	}

	c.server.onClientChangedChan <- &clientChanged{
		isAdd:  false,
		connId: client.ConnId(),
		userId: client.UserId(),
	}
}
