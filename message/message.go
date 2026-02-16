package message

import "google.golang.org/protobuf/proto"

type CommandType uint16

const (
	COMMAND_AUTH_REQ     CommandType = 1
	COMMAND_AUTH_RESP    CommandType = 2
	COMMAND_CONNECT_REQ  CommandType = 3
	COMMAND_CONNECT_RESP CommandType = 4
	COMMAND_DATA         CommandType = 5
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
