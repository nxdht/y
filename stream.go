package y

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type IParser interface {
	OnNewData(client IClient, buf []byte) error
	PackData(data interface{}) []byte
}

type IHandler interface {
	HandleNewData(client IClient, data interface{}) error
	PackData(data interface{}) []byte
}

type IClient interface {
	Close()
	Send([]byte)
	ConnId() uint64
	UserId() interface{}
	LastTime() time.Time
}

// 在同一协程中调用
type IClientChanged interface {
	OnAdded(connId uint64, userId interface{})
	OnRemoved(connId uint64, userId interface{}, beKickedClient IClient)
}

type IState interface {
	OnOpen(client IClient)
	OnClose(client IClient)
}

const (
	maxPayloadLength = 10240
)

var (
	InvalidDataError    = fmt.Errorf("invalid data error")
	InvalidServiceError = fmt.Errorf("invalid service error")
)

type DefaultParser struct {
	handler   IHandler
	maxLength int
	buffer    []byte
}

func NewDefaultParser(handler IHandler, maxLength int) *DefaultParser {
	if maxLength == 0 {
		maxLength = maxPayloadLength
	}
	return &DefaultParser{
		handler:   handler,
		maxLength: maxLength,
	}
}

func (p *DefaultParser) OnNewData(client IClient, buf []byte) error {
	p.buffer = append(p.buffer, buf...)

	for {
		data := p.buffer
		l := len(data)

		if l <= 4 {
			return nil
		}

		length := int(((uint32(data[0]) << 24) & 0xFF000000) |
			((uint32(data[1]) << 16) & 0xFF0000) |
			((uint32(data[2]) << 8) & 0xFF00) |
			(uint32(data[3]) & 0xFF))
		// 不能为0，不能超过限制
		if length == 0 || length > p.maxLength {
			return InvalidDataError
		}
		// 检查是否所有数据都收到了
		if length+4 > l {
			return nil
		}

		err := p.handler.HandleNewData(client, p.buffer[4:4+length])
		if err != nil {
			return nil
		}

		p.buffer = p.buffer[length+4:]
	}
}

func (p *DefaultParser) PackData(data interface{}) []byte {
	buf := p.handler.PackData(data)

	l := len(buf)
	if l > 0 {

		r := make([]byte, 4)
		r[0] = byte((l >> 24) & 0xFF)
		r[1] = byte((l >> 16) & 0xFF)
		r[2] = byte((l >> 8) & 0xFF)
		r[3] = byte((l) & 0xFF)

		r = append(r, buf...)
		return r
	}

	return nil
}

type JsonMsg struct {
	Client IClient
	MsgMap map[string]interface{}
}

// json format data
type JsonMsgHandler struct {
	router map[string]func(jsonMsg *JsonMsg)
}

func NewJsonMsgHandler(router map[string]func(jsonMsg *JsonMsg)) *JsonMsgHandler {
	return &JsonMsgHandler{
		router: router,
	}
}

func (h *JsonMsgHandler) HandleNewData(client IClient, data interface{}) error {
	defer RecoverError()

	var jsonMsg JsonMsg
	err := json.Unmarshal(data.([]byte), &jsonMsg.MsgMap)
	if err != nil {
		Errorf("JsonMsgHandler.HandleNewData %+v", err)
		return InvalidDataError
	}

	if service, ok := jsonMsg.MsgMap["service"]; !ok {
		Errorf("JsonMsgHandler.HandleNewData %+v", err)
		return InvalidServiceError
	} else if s, ok := service.(string); ok {
		if h.router != nil {
			if f, ok := h.router[s]; ok {
				jsonMsg.Client = client
				f(&jsonMsg)
			}
		}
	} else {
		Errorf("JsonMsgHandler.HandleNewData service is not string %v", service)
		return InvalidDataError
	}

	return nil
}

func (h *JsonMsgHandler) PackData(data interface{}) []byte {
	msgMap := data.(map[string]interface{})
	buf, err := json.Marshal(msgMap)
	Panic(err)
	return buf
}

type MultiThreadHandler struct {
	receiveChan chan *receiveData
	handler     IHandler
	threadCount int
}

type receiveData struct {
	client IClient
	data   interface{}
}

func NewMultiThreadHandler(ctx context.Context, handler IHandler, threadCount int) *MultiThreadHandler {
	if handler == nil || threadCount <= 0 {
		return nil
	}

	h := &MultiThreadHandler{
		receiveChan: make(chan *receiveData, 20000),
		handler:     handler,
		threadCount: threadCount,
	}

	go h.loop(ctx)

	return h
}

func (h *MultiThreadHandler) loop(ctx context.Context) {
	defer RecoverError()

	for i := 0; i < h.threadCount; i++ {
		go func(x int) {
			for {
				select {
				case d, ok := <-h.receiveChan:
					if ok {
						h.handler.HandleNewData(d.client, d.data)
					}
				case <-ctx.Done():
					goto END
				}
			}

		END:
			Infof("go routine receive data %d ended", x)
		}(i)
	}
}

func (h *MultiThreadHandler) HandleNewData(client IClient, data interface{}) error {
	h.receiveChan <- &receiveData{
		client: client,
		data:   data,
	}
	return nil
}

func (h *MultiThreadHandler) PackData(data interface{}) []byte {
	return h.handler.PackData(data)
}
