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

func MakeConnectReq(channelID uint64, localAddr string, batchSize uint64, windowSize uint64) Message {
	connectReq := pb.ConnectReq{}
	connectReq.Addr = &localAddr
	connectReq.ChannelId = &channelID
	connectReq.BatchSize = &batchSize
	connectReq.WindowSize = &windowSize
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

func MakeDataNoti(channelID uint64, data []byte) Message {
	dataNoti := pb.DataNoti{}
	dataNoti.ChannelId = &channelID
	dataNoti.Data = data
	return NewPbMessage(COMMAND_DATA_NOTI, &dataNoti)
}

func MakeChannelCloseReq(channelID uint64) Message {
	channelCloseReq := pb.CloseChannelReq{}
	channelCloseReq.ChannelId = &channelID
	return NewPbMessage(COMMAND_CHANNEL_CLOSE_REQ, &channelCloseReq)
}

func MakeChannelCloseResp(code int32, msg string) Message {
	channelCloseResp := pb.CloseChannelResp{}
	channelCloseResp.BaseResp = &pb.BaseResp{
		Code: &code,
		Msg:  &msg,
	}

	return NewPbMessage(COMMAND_CONNECT_RESP, &channelCloseResp)
}

func MakeChanelWindowUpdateNoti(channelID uint64, windowSize uint64) Message {
	noti := pb.ChannelWindowUpdateNoti{}
	noti.ChannelId = &channelID
	noti.WindowSize = &windowSize
	return NewPbMessage(COMMAND_CHANNEL_WINDOW_UPDATE_NOTI, &noti)
}
