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
	"github.com/lakakala/luna-nt-go/pb"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type Conn struct {
	mutex sync.Mutex

	nextMsgID *atomic.Uint64
	conn      net.Conn

	sendMsgMap map[uint64]*SendMessage

	readChan  chan *RecvMessage
	writeChan chan *SendMessage

	innerReadChan chan *message.Frame

	channelManager *ChannelManager
}

func NewConnFromRawConn(ctx context.Context, rawConn net.Conn) (*Conn, error) {
	nextMsgID := &atomic.Uint64{}

	channelManager := newChannelManager()

	conn := &Conn{
		nextMsgID: nextMsgID,
		conn:      rawConn,

		sendMsgMap: make(map[uint64]*SendMessage),

		channelManager: channelManager,

		readChan:      make(chan *RecvMessage, 1000),
		writeChan:     make(chan *SendMessage, 1000),
		innerReadChan: make(chan *message.Frame, 1000),
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
	go conn.accptInner(ctx)
	go conn.readLoop(ctx)
	go conn.writeLoop(ctx)
}

func (conn *Conn) readLoop(ctx context.Context) {

readLoop:
	for {
		frame, err := message.Decode(ctx, conn.conn)
		if err != nil {
			log.CtxWarnf(ctx, "Conn.readLoop Decode failed err %s", err)
			conn.readChan <- makeErrorRecvMessage(err)
			break readLoop
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
				log.CtxWarnf(ctx, "Conn readLoop msgID %d can not find req", msgID)
				break readLoop
			}

			conn.handleRespMessage(ctx, makeSendResult(frame))
		} else if msgType == message.MessageTypeInnerReq || msgType == message.MessageTypeInnerNoti {
			conn.innerReadChan <- frame
		} else {
			conn.readChan <- makeRecvMessage(frame)
		}
	}

	close(conn.readChan)
	log.CtxInfof(ctx, "Conn readLoop end")
}

func (conn *Conn) handleRespMessage(ctx context.Context, sendResult *SendResult) {

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

func (conn *Conn) writeLoop(ctx context.Context) {
writeLoop:
	for sendMsg := range conn.writeChan {
		msgID := sendMsg.frame.MsgID()

		msgType := message.MsgType(sendMsg.frame.Command())
		if msgType == message.MessageTypeReq || msgType == message.MessageTypeInnerReq {
			func() {
				conn.mutex.Lock()
				defer conn.mutex.Unlock()

				conn.sendMsgMap[msgID] = sendMsg
			}()
		}

		if err := message.Encode(ctx, conn.conn, sendMsg.frame); err != nil {
			log.CtxErrorf(ctx, "Conn writeLoop Encode failed err %s", err)
			break writeLoop
		}
	}

	log.CtxInfof(ctx, "Conn writeLoop end")
}

func (conn *Conn) accptInner(ctx context.Context) {
	for frame := range conn.innerReadChan {

		recvMsgCtx := makeRecvMessageContext(frame, conn.writeChan)
		switch frame.Command() {
		case message.COMMAND_CHANNEL_CREATE_REQ:
			conn.handleChannelCreateReq(ctx, recvMsgCtx)
		case message.COMMAND_CHANNEL_CLOSE_REQ:
			conn.handleChannelCloseReq(ctx, recvMsgCtx)
		case message.COMMAND_DATA_NOTI:
			conn.handleChannelDataNoti(ctx, recvMsgCtx)
		case message.COMMAND_CHANNEL_WINDOW_UPDATE_NOTI:
			conn.handleChannelWindowUpdateNoti(ctx, recvMsgCtx)
		}

	}
}

func (conn *Conn) handleChannelCreateReq(ctx context.Context, recvCtx *RecvMessageContext) {
	channelCreateReq := recvCtx.Frame().Msg().Msg().(*pb.ChannelCreateReq)

	channelID := channelCreateReq.GetChannelId()
	windowSize := channelCreateReq.GetWindowSize()

	channel := newChannel(ctx, channelID, conn, windowSize)
	if _, ok := conn.channelManager.AddChannel(ctx, channel); !ok {
		recvCtx.SendResp(ctx, message.MakeConnectResp(1, fmt.Sprintf("channelID %d alread exists", channelID)))
		return
	}

	recvCtx.SendResp(ctx, message.MakeConnectResp(0, fmt.Sprintf("channelID %d create success", channelID)))

	log.CtxInfof(ctx, "handleChannelCreateReq accept newChannel %d success", channelID)
}

func (conn *Conn) handleChannelCloseReq(ctx context.Context, recvCtx *RecvMessageContext) {
	channelCloseReq := recvCtx.Frame().Msg().Msg().(*pb.ChannelCloseReq)

	channelID := channelCloseReq.GetChannelId()

	channel := conn.channelManager.GetChannel(ctx, channelID)
	if channel == nil {
		log.CtxWarnf(ctx, "Conn.handleChannelCloseReq unknown channelID %d", channelID)
		return
	}

	recvCtx.SendResp(ctx, message.MakeChannelCloseResp(0, fmt.Sprintf("channelID %d close success", channelID)))
}

func (conn *Conn) handleChannelDataNoti(ctx context.Context, recvCtx *RecvMessageContext) {
	dataNoti := recvCtx.Frame().Msg().Msg().(*pb.DataNoti)

	channelID := dataNoti.GetChannelId()

	channel := conn.channelManager.GetChannel(ctx, channelID)
	if channel == nil {
		log.CtxWarnf(ctx, "Conn.handleChannelDataNoti unknown channelID %d", channelID)
		return
	}

	channel.put(ctx, dataNoti.GetCode(), dataNoti.GetData())
	// conn.channelManager.RemoveChannel(ctx, channel)
}

func (conn *Conn) handleChannelWindowUpdateNoti(ctx context.Context, recvCtx *RecvMessageContext) {
	windowUpdateNoti := recvCtx.Frame().Msg().Msg().(*pb.ChannelWindowUpdateNoti)

	channelID := windowUpdateNoti.GetChannelId()
	windowSize := windowUpdateNoti.GetWindowSize()

	channel := conn.channelManager.GetChannel(ctx, channelID)
	if channel == nil {
		log.CtxWarnf(ctx, "Conn.handleChannelWindowUpdateNoti unknown channelID %d", channelID)
		return
	}

	channel.Ack(ctx, windowUpdateNoti.GetCode(), int64(windowSize))
}

func (conn *Conn) Accept(ctx context.Context) (*RecvMessageContext, error) {
	recvMsg, ok := <-conn.readChan
	if !ok {
		return nil, errors.New("read chan close")
	}

	frame, err := recvMsg.Frame()
	if err != nil {
		return nil, err
	}

	return makeRecvMessageContext(frame, conn.writeChan), nil
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
	if msgType == message.MessageTypeReq || msgType == message.MessageTypeInnerReq {

		ctx, cancel := context.WithTimeout(ctx, time.Second*2)
		defer cancel()

		select {
		case resp := <-respChan:
			if resp == nil {
				return nil, errors.New("send chan close")
			}

			return resp.Frame().Msg(), nil
		case <-ctx.Done():
			log.CtxErrorf(ctx, "Conn Send msgID %d timeout", msgID)
			return nil, ctx.Err()
		}
	} else if msgType == message.MessageTypeNoti || msgType == message.MessageTypeInnerNoti {
		return nil, nil
	} else if msgType == message.MessageTypeResp {
		return nil, nil
	} else {
		return nil, errors.New(fmt.Sprintf("unknown msgType %d", msgType))
	}
}

func (conn *Conn) GetChannel(ctx context.Context, channelID uint64) *Channel {
	return conn.channelManager.GetChannel(ctx, channelID)
}

func (conn *Conn) CreateChannel(ctx context.Context) (*Channel, error) {

	var windowSize uint64 = 30

	channelID := conn.channelManager.NextChannelID()

	resp, err := conn.Send(ctx, message.MakeChannelCreateReq(channelID, windowSize))
	if err != nil {
		return nil, err
	}

	createChannelResp := resp.Msg().(*pb.ConnectResp)

	if createChannelResp.BaseResp.GetCode() != 0 {
		return nil, fmt.Errorf("create channel failed code %d msg %s", *createChannelResp.BaseResp.Code, *createChannelResp.BaseResp.Msg)
	}

	channel := newChannel(ctx, channelID, conn, windowSize)

	if _, ok := conn.channelManager.AddChannel(ctx, channel); !ok {
		return nil, fmt.Errorf("channelID %d alread exists", channelID)
	}

	log.CtxInfof(ctx, "Conn.CreateChannel channelID %d success", channelID)

	return channel, nil
}

func (conn *Conn) Close(ctx context.Context) {
	conn.closeSendMsgMap(ctx)

	conn.conn.Close()

	close(conn.readChan)
	close(conn.writeChan)

	log.CtxInfof(ctx, "Conn close done")
}

func (conn *Conn) closeSendMsgMap(ctx context.Context) {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	for _, sendMsg := range conn.sendMsgMap {

		close(sendMsg.respChan)
	}
}
