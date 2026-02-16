package message

import (
	"context"
	"encoding/binary"
	"io"

	"github.com/lakakala/luna-nt-go/pb"
	"google.golang.org/protobuf/proto"
)

var decoder = make(map[CommandType]decode)

var encoder = make(map[CommandType]encode)

func registerCodec(cmd CommandType, decode decode, encode encode) {

	decoder[cmd] = decode
	encoder[cmd] = encode
}

type decode = func(ctx context.Context, data []byte) (Message, error)

type encode = func(ctx context.Context, msg Message) ([]byte, error)

func init() {

	registerCodec(COMMAND_AUTH_REQ, func(ctx context.Context, data []byte) (Message, error) {
		var req = &pb.AuthReq{}

		if err := proto.Unmarshal(data, req); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_AUTH_REQ, req), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		req := msg.Msg().(*pb.AuthReq)

		data, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}

		return data, nil
	})

	registerCodec(COMMAND_AUTH_RESP, func(ctx context.Context, data []byte) (Message, error) {
		var resp = &pb.AuthResp{}

		if err := proto.Unmarshal(data, resp); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_AUTH_RESP, resp), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		resp := msg.Msg().(*pb.AuthResp)

		data, err := proto.Marshal(resp)
		if err != nil {
			return nil, err
		}

		return data, nil
	})

}

type Frame struct {
	command CommandType
	msgID   uint64
	msg     Message
}

func MakeFrame(ctx context.Context, msgID uint64, msg Message) (*Frame, error) {

	cmd := msg.Cmd()

	return &Frame{
		command: cmd,
		msgID:   msgID,
		msg:     msg,
	}, nil
}

func (f *Frame) Command() CommandType {
	return f.command
}

func (f *Frame) MsgID() uint64 {
	return f.msgID
}

func (f *Frame) Msg() Message {
	return f.msg
}

func Decode(ctx context.Context, reader io.Reader) (*Frame, error) {
	var frame = &Frame{}

	if err := binary.Read(reader, binary.BigEndian, &frame.command); err != nil {
		return nil, err
	}

	if err := binary.Read(reader, binary.BigEndian, &frame.msgID); err != nil {
		return nil, err
	}

	var len uint32

	if err := binary.Read(reader, binary.BigEndian, &len); err != nil {
		return nil, err
	}

	data := make([]byte, len)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	msg, err := decoder[frame.command](ctx, data)
	if err != nil {
		return nil, err
	}

	frame.msg = msg

	return frame, nil
}

func Encode(ctx context.Context, writer io.Writer, frame *Frame) error {
	if err := binary.Write(writer, binary.BigEndian, frame.command); err != nil {
		return err
	}

	if err := binary.Write(writer, binary.BigEndian, frame.msgID); err != nil {
		return err
	}

	data, err := encoder[frame.command](ctx, frame.msg)
	if err != nil {
		return err
	}

	var len uint32 = uint32(len(data))
	if err := binary.Write(writer, binary.BigEndian, len); err != nil {
		return err
	}

	if _, err := writer.Write(data); err != nil {
		return err
	}

	return nil
}
