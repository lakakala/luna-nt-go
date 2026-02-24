package conn

import (
	"context"

	"github.com/lakakala/luna-nt-go/message"
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

func (recvCtx *RecvMessageContext) Frame() (*message.Frame, error) {
	return recvCtx.frame, recvCtx.err
}

func (recvCtx *RecvMessageContext) SendResp(ctx context.Context, resp message.Message) error {

	reqFrame, err := recvCtx.Frame()
	if err != nil {
		return err
	}
	frame, err := message.MakeFrame(ctx, reqFrame.MsgID(), resp)
	if err != nil {
		return err
	}

	sendMsg, _ := MakeSendMessage(frame)

	recvCtx.respChan <- sendMsg
	return nil
}
