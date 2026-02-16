package conn

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/lakakala/luna-nt-go/message"
)

type Conn struct {
	mutex     sync.Mutex
	nextMsgID *atomic.Uint64
	conn      net.Conn

	sendMsgMap map[uint64]*SendMessage

	readChan  chan *RecvMessageContext
	writeChan chan *SendMessage
}

func NewConnFromRawConn(ctx context.Context, rawConn net.Conn) (*Conn, error) {
	nextMsgID := &atomic.Uint64{}
	nextMsgID.Store(1)

	conn := &Conn{
		nextMsgID: nextMsgID,
		conn:      rawConn,

		sendMsgMap: make(map[uint64]*SendMessage),

		readChan:  make(chan *RecvMessageContext, 10),
		writeChan: make(chan *SendMessage, 10),
	}

	conn.start(ctx)

	return conn, nil
}

func NewConn(ctx context.Context, addr string) (*Conn, error) {
	rawConn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return NewConnFromRawConn(ctx, rawConn)
}

func (conn *Conn) start(ctx context.Context) {
	go conn.readLoop(ctx)
	go conn.writeLoop(ctx)
}

func (conn *Conn) readLoop(ctx context.Context) {

	frame, err := message.Decode(ctx, conn.conn)
	if err != nil {
		conn.readChan <- makeErrorRecvMessage(err)
		return
	}

	msgID := frame.MsgID()

	ok := func() bool {
		conn.mutex.Lock()
		defer conn.mutex.Unlock()

		_, ok := conn.sendMsgMap[msgID]

		return ok
	}()

	if ok {
		conn.recvSendMessageResult(makeSuccessSendResult(frame))
	} else {
		conn.handleFrame(frame)
	}
}

func (conn *Conn) handleFrame(frame *message.Frame) {
	conn.readChan <- makeSuccessRecvMessage(frame, conn.writeChan)
}

func (conn *Conn) writeLoop(ctx context.Context) {
	for sendMsg := range conn.writeChan {
		conn.handleSendMessage(ctx, sendMsg)
	}
}

func (conn *Conn) handleSendMessage(ctx context.Context, sendMsg *SendMessage) {
	func() {
		conn.mutex.Lock()
		defer conn.mutex.Unlock()

		msgID := sendMsg.frame.MsgID()

		conn.sendMsgMap[msgID] = sendMsg
	}()

	if err := message.Encode(ctx, conn.conn, sendMsg.frame); err != nil {
		conn.recvSendMessageResult(makeErrorSendResult(err))
		return
	}
}

func (conn *Conn) recvSendMessageResult(sendResult *SendResult) {

	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	msgID := sendResult.frame.MsgID()

	sendMsg, ok := conn.sendMsgMap[msgID]
	if !ok {
		return
	}

	delete(conn.sendMsgMap, msgID)
	sendMsg.respChan <- sendResult
}

func (conn *Conn) Send(ctx context.Context, msg message.Message) (message.Message, error) {
	msgID := conn.nextMsgID.Add(1)

	frame, err := message.MakeFrame(ctx, msgID, msg)
	if err != nil {
		return nil, err
	}

	sendMsg, respChan := MakeSendMessage(frame)

	conn.writeChan <- sendMsg

	select {
	case resp := <-respChan:
		if resp.err != nil {
			return nil, resp.err
		}
		return resp.frame.Msg(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (conn *Conn) Accept() (*RecvMessageContext, error) {
	recvMsg, ok := <-conn.readChan
	if !ok {
		return nil, nil
	}

	if recvMsg.err != nil {
		return nil, recvMsg.err
	}

	return recvMsg, nil
}

type SendResult struct {
	frame *message.Frame
	err   error
}

func makeSuccessSendResult(frame *message.Frame) *SendResult {
	return &SendResult{
		frame: frame,
	}
}

func makeErrorSendResult(err error) *SendResult {
	return &SendResult{
		err: err,
	}
}

type SendMessage struct {
	frame    *message.Frame
	respChan chan *SendResult
}

func MakeSendMessage(frame *message.Frame) (*SendMessage, chan *SendResult) {

	respChan := make(chan *SendResult, 1)

	return &SendMessage{
		frame:    frame,
		respChan: respChan,
	}, respChan
}

type RecvMessageContext struct {
	frame    *message.Frame
	respChan chan *SendMessage
	err      error
}

func makeSuccessRecvMessage(frame *message.Frame, respChan chan *SendMessage) *RecvMessageContext {
	return &RecvMessageContext{
		frame:    frame,
		respChan: respChan,
	}
}

func makeErrorRecvMessage(err error) *RecvMessageContext {
	return &RecvMessageContext{
		err: err,
	}
}

func (recvCtx *RecvMessageContext) Frame() *message.Frame {
	return recvCtx.frame
}

func (recvCtx *RecvMessageContext) SendResp(ctx context.Context, resp message.Message) error {
	frame, err := message.MakeFrame(ctx, recvCtx.frame.MsgID(), resp)
	if err != nil {
		return err
	}

	sendMsg, _ := MakeSendMessage(frame)

	recvCtx.respChan <- sendMsg
	return nil
}
