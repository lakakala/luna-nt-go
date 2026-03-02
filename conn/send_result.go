package conn

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type SendResult struct {
	frame *message.Frame
}

func makeSendResult(frame *message.Frame) *SendResult {
	return &SendResult{
		frame: frame,
	}
}

func (sendResult *SendResult) Frame() *message.Frame {
	return sendResult.frame
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

type RecvMessage struct {
	frame *message.Frame
	err   error
}

func makeRecvMessage(frame *message.Frame) *RecvMessage {
	return &RecvMessage{
		frame: frame,
		err:   nil,
	}
}

func makeErrorRecvMessage(err error) *RecvMessage {
	return &RecvMessage{
		frame: nil,
		err:   err,
	}
}

func (recvMsg *RecvMessage) Frame() (*message.Frame, error) {
	return recvMsg.frame, recvMsg.err
}

type RecvMessageContext struct {
	frame    *message.Frame
	respChan chan *SendMessage
}

func makeRecvMessageContext(frame *message.Frame, respChan chan *SendMessage) *RecvMessageContext {
	return &RecvMessageContext{
		frame:    frame,
		respChan: respChan,
	}
}

func (recvCtx *RecvMessageContext) Frame() *message.Frame {
	return recvCtx.frame
}

func (recvCtx *RecvMessageContext) SendResp(ctx context.Context, resp message.Message) error {

	reqJson, err := json.Marshal(recvCtx.frame.Msg().Msg())
	if err != nil {
		return err
	}

	log.CtxInfof(ctx, "RecvMessageContext.SendResp req %s msgID %d costs %dms", string(reqJson), recvCtx.frame.MsgID(), time.Since(recvCtx.frame.CreateTime()).Milliseconds())
	reqFrame := recvCtx.Frame()

	frame, err := message.MakeFrame(ctx, reqFrame.MsgID(), resp)
	if err != nil {
		return err
	}

	sendMsg, _ := MakeSendMessage(frame)

	recvCtx.respChan <- sendMsg
	return nil
}
