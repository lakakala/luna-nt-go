package conn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/utils/log"
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

	conn := &Conn{
		nextMsgID: nextMsgID,
		conn:      rawConn,

		sendMsgMap: make(map[uint64]*SendMessage),

		readChan:  make(chan *RecvMessageContext, 10000),
		writeChan: make(chan *SendMessage, 10000),
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

	for {
		frame, err := message.Decode(ctx, conn.conn)
		if err != nil {
			conn.readChan <- makeErrorRecvMessage(err)
			break
		}

		msgID := frame.MsgID()

		msgType := message.MsgType(frame.Command())
		if msgType == message.MessageTypeResp {
			ok := func() bool {
				conn.mutex.Lock()
				defer conn.mutex.Unlock()

				_, ok := conn.sendMsgMap[msgID]

				return ok
			}()

			if !ok {
				log.CtxErrorf(ctx, "Conn readLoop msgID %d can not find req", msgID)
			}

			conn.recvSendMessageResult(ctx, makeSendResult(frame))
		} else {
			conn.readChan <- makeSuccessRecvMessage(frame, conn.writeChan)
		}
	}

	log.CtxInfof(ctx, "Conn readLoop end")
}

func (conn *Conn) writeLoop(ctx context.Context) {
	for sendMsg := range conn.writeChan {
		msgID := sendMsg.frame.MsgID()

		msgType := message.MsgType(sendMsg.frame.Command())
		if msgType == message.MessageTypeReq {
			func() {
				conn.mutex.Lock()
				defer conn.mutex.Unlock()

				conn.sendMsgMap[msgID] = sendMsg
			}()
		}

		if err := message.Encode(ctx, conn.conn, sendMsg.frame); err != nil {
			conn.readChan <- makeErrorRecvMessage(err)
			break
		}
	}

	log.CtxInfof(ctx, "Conn writeLoop end")
}

func (conn *Conn) recvSendMessageResult(ctx context.Context, sendResult *SendResult) {

	msgID := sendResult.frame.MsgID()

	sendMsg := func() *SendMessage {
		conn.mutex.Lock()
		defer conn.mutex.Unlock()

		sendMsg, ok := conn.sendMsgMap[msgID]
		if !ok {
			return nil
		}

		delete(conn.sendMsgMap, msgID)
		return sendMsg
	}()

	if sendMsg == nil {
		return
	}

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

	msgType := message.MsgType(msg.Cmd())
	if msgType == message.MessageTypeReq {

		ctx, cancel := context.WithTimeout(ctx, time.Second*20)
		defer cancel()

		select {
		case resp := <-respChan:
			return resp.Frame().Msg(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else if msgType == message.MessageTypeNoti {
		return nil, nil
	} else if msgType == message.MessageTypeResp {
		return nil, nil
	} else {
		return nil, errors.New(fmt.Sprintf("unknown msgType %d", msgType))
	}
}

func (conn *Conn) Accept(ctx context.Context) (*RecvMessageContext, error) {
	recvMsg, ok := <-conn.readChan
	if !ok {
		return nil, nil
	}

	return recvMsg, nil
}

func (conn *Conn) Close(ctx context.Context) {
	close(conn.readChan)
	close(conn.writeChan)

	conn.conn.Close()

	log.CtxInfof(ctx, "Conn close done")
}
