package message

import "github.com/lakakala/luna-nt-go/pb"

func MakeAuthReq(token string, clientID uint64) Message {
	authReq := pb.AuthReq{}
	authReq.Token = &token
	authReq.ClientId = &clientID
	return NewPbMessage(COMMAND_AUTH_REQ, &authReq)
}

func MakeAuthResp(code int32, msg string) Message {
	authResp := pb.AuthResp{}
	authResp.BaseResp = &pb.BaseResp{
		Code: &code,
		Msg:  &msg,
	}

	return NewPbMessage(COMMAND_AUTH_RESP, &authResp)
}

func MakeConnectReq(channelID uint64, addr string) Message {
	connectReq := pb.ConnectReq{}
	connectReq.ChannelId = &channelID
	connectReq.Addr = &addr
	return NewPbMessage(COMMAND_CONNECT_REQ, &connectReq)
}

func MakeConnectResp(code int32, msg string) Message {
	connectResp := pb.ConnectResp{}
	connectResp.BaseResp = &pb.BaseResp{
		Code: &code,
		Msg:  &msg,
	}

	return NewPbMessage(COMMAND_CONNECT_RESP, &connectResp)
}

func MakeChannelCreateReq(channelID uint64, windowSize uint64) Message {
	channelCreateReq := pb.ChannelCreateReq{}
	channelCreateReq.ChannelId = &channelID
	channelCreateReq.WindowSize = &windowSize
	return NewPbMessage(COMMAND_CHANNEL_CREATE_REQ, &channelCreateReq)
}

func MakeChannelCreateResp(code int32, msg string) Message {
	channelCreateResp := pb.ChannelCreateResp{}
	channelCreateResp.BaseResp = &pb.BaseResp{
		Code: &code,
		Msg:  &msg,
	}
	return NewPbMessage(COMMAND_CHANNEL_CREATE_RESP, &channelCreateResp)
}

func MakeChannelCloseNoti(channelID uint64, msg string) Message {
	channelCloseNoti := pb.ChannelCloseNoti{}
	channelCloseNoti.ChannelId = &channelID
	channelCloseNoti.Msg = &msg
	return NewPbMessage(COMMAND_CHANNEL_CLOSE_NOTI, &channelCloseNoti)
}

func MakeDataNoti(channelID uint64, code uint32, data []byte) Message {
	dataNoti := pb.DataNoti{}
	dataNoti.ChannelId = &channelID
	dataNoti.Code = &code
	dataNoti.Data = data
	return NewPbMessage(COMMAND_DATA_NOTI, &dataNoti)
}

func MakeChanelWindowUpdateNoti(channelID uint64, code uint32, windowSize uint64) Message {
	noti := pb.ChannelWindowUpdateNoti{}
	noti.ChannelId = &channelID
	noti.WindowSize = &windowSize
	return NewPbMessage(COMMAND_CHANNEL_WINDOW_UPDATE_NOTI, &noti)
}
