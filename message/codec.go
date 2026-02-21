package message

import (
	"context"
	"encoding/binary"
	"io"

	"github.com/lakakala/luna-nt-go/pb"
	"google.golang.org/protobuf/proto"
)

type MessageType uint16

const (
	MessageTypeReq  MessageType = 1
	MessageTypeResp MessageType = 2
	MessageTypeNoti MessageType = 3
)

type decode = func(ctx context.Context, data []byte) (Message, error)

type encode = func(ctx context.Context, msg Message) ([]byte, error)

var (
	decoder     = make(map[CommandType]decode)
	encoder     = make(map[CommandType]encode)
	messageType = make(map[CommandType]MessageType)
)

func registerCodec(cmd CommandType, decode decode, encode encode, msgType MessageType) {
	decoder[cmd] = decode
	encoder[cmd] = encode
	messageType[cmd] = msgType
}

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
	}, MessageTypeReq)

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
	}, MessageTypeResp)

	registerCodec(COMMAND_CONNECT_REQ, func(ctx context.Context, data []byte) (Message, error) {
		var req = &pb.ConnectReq{}

		if err := proto.Unmarshal(data, req); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_CONNECT_REQ, req), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		req := msg.Msg().(*pb.ConnectReq)

		data, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}

		return data, nil
	}, MessageTypeReq)

	registerCodec(COMMAND_CONNECT_RESP, func(ctx context.Context, data []byte) (Message, error) {
		var resp = &pb.ConnectResp{}

		if err := proto.Unmarshal(data, resp); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_CONNECT_RESP, resp), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		resp := msg.Msg().(*pb.ConnectResp)

		data, err := proto.Marshal(resp)
		if err != nil {
			return nil, err
		}

		return data, nil
	}, MessageTypeResp)

	registerCodec(COMMAND_DATA_NOTI, func(ctx context.Context, data []byte) (Message, error) {
		var msg = &pb.DataNoti{}

		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_DATA_NOTI, msg), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		dataMsg := msg.Msg().(*pb.DataNoti)

		data, err := proto.Marshal(dataMsg)
		if err != nil {
			return nil, err
		}

		return data, nil
	}, MessageTypeNoti)

	registerCodec(COMMAND_CHANNEL_CLOSE_REQ, func(ctx context.Context, data []byte) (Message, error) {
		var req = &pb.CloseChannelReq{}

		if err := proto.Unmarshal(data, req); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_CHANNEL_CLOSE_REQ, req), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		req := msg.Msg().(*pb.CloseChannelReq)

		data, err := proto.Marshal(req)
		if err != nil {
			return nil, err
		}

		return data, nil
	}, MessageTypeReq)

	registerCodec(COMMAND_CHANNEL_CLOSE_RESP, func(ctx context.Context, data []byte) (Message, error) {
		var resp = &pb.CloseChannelResp{}

		if err := proto.Unmarshal(data, resp); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_CHANNEL_CLOSE_RESP, resp), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		resp := msg.Msg().(*pb.CloseChannelResp)

		data, err := proto.Marshal(resp)
		if err != nil {
			return nil, err
		}

		return data, nil
	}, MessageTypeResp)

	registerCodec(COMMAND_CHANNEL_WINDOW_UPDATE_NOTI, func(ctx context.Context, data []byte) (Message, error) {
		var resp = &pb.ChannelWindowUpdateNoti{}

		if err := proto.Unmarshal(data, resp); err != nil {
			return nil, err
		}

		return NewPbMessage(COMMAND_CHANNEL_WINDOW_UPDATE_NOTI, resp), nil
	}, func(ctx context.Context, msg Message) ([]byte, error) {
		resp := msg.Msg().(*pb.ChannelWindowUpdateNoti)

		data, err := proto.Marshal(resp)
		if err != nil {
			return nil, err
		}

		return data, nil
	}, MessageTypeNoti)

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

func MsgType(cmdType CommandType) MessageType {
	return messageType[cmdType]
}
