package message

import "google.golang.org/protobuf/proto"

type CommandType uint16

const (
	COMMAND_AUTH_REQ          CommandType = 1
	COMMAND_AUTH_RESP         CommandType = 2
	COMMAND_CONNECT_REQ       CommandType = 3
	COMMAND_CONNECT_RESP      CommandType = 4
	COMMAND_CLIENT_CLOSE_REQ  CommandType = 9
	COMMAND_CLIENT_CLOSE_RESP CommandType = 10

	COMMAND_CHANNEL_CREATE_REQ         CommandType = 21
	COMMAND_CHANNEL_CREATE_RESP        CommandType = 22
	COMMAND_CHANNEL_CLOSE_REQ          CommandType = 23
	COMMAND_CHANNEL_CLOSE_RESP         CommandType = 24
	COMMAND_DATA_NOTI                  CommandType = 25
	COMMAND_CHANNEL_WINDOW_UPDATE_NOTI CommandType = 26
)

type Message interface {
	Cmd() CommandType
	Msg() interface{}
}

type pbMessage struct {
	cmd CommandType
	msg proto.Message
}

func (p *pbMessage) Cmd() CommandType {
	return p.cmd
}

func (p *pbMessage) Msg() interface{} {
	return p.msg
}

func NewPbMessage(cmd CommandType, msg proto.Message) Message {
	return &pbMessage{
		cmd: cmd,
		msg: msg,
	}
}
