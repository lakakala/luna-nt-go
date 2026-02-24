package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/pb"
	"github.com/lakakala/luna-nt-go/utils/log"
)

func StartClient(conf *Config) error {
	cli, err := newClient(context.Background(), conf)
	if err != nil {
		return err
	}

	cli.start(context.Background())

	return nil
}

type ClientStatus int16

const (
	ClientStatusUninit   ClientStatus = 1
	ClientStatusInited   ClientStatus = 2
	ClientStatusCloseing ClientStatus = 3
	ClientStatusClosed   ClientStatus = 4
)

type Client struct {
	mutex sync.Mutex

	channelManager *ChannelManager

	status ClientStatus
	conf   *Config
	conn   *conn.Conn
}

func newClient(ctx context.Context, conf *Config) (*Client, error) {

	cli := &Client{
		mutex: sync.Mutex{},

		channelManager: newChannelManager(),

		status: ClientStatusUninit,
		conf:   conf,
	}

	return cli, nil
}

func (cli *Client) GetStatus() ClientStatus {
	cli.mutex.Lock()
	defer cli.mutex.Unlock()

	return cli.status
}

func (cli *Client) setStatus(status ClientStatus) {
	cli.mutex.Lock()
	defer cli.mutex.Unlock()

	cli.status = status
}

func (cli *Client) start(ctx context.Context) {

	for {
		if err := cli.doStart(ctx); err != nil {
			log.CtxWarnf(ctx, "Client %d start failed err %s", cli.conf.ClientID, err)
		}

		cli.Close(ctx)
		time.Sleep(time.Second * 5)
	}
}

func (cli *Client) doStart(ctx context.Context) error {

	conn, err := conn.NewConn(ctx, cli.conf.Addr)
	if err != nil {
		return err
	}

	func() {
		cli.mutex.Lock()
		defer cli.mutex.Unlock()

		cli.conn = conn
	}()

	cli.conn = conn

	err = cli.auth(ctx)
	if err != nil {
		return err
	}

	for {

		recvCtx, err := conn.Accept(ctx)
		if err != nil {
			break
		}

		if err := func() error {
			frame, err := recvCtx.Frame()
			if err != nil {
				return err
			}

			switch frame.Command() {
			case message.COMMAND_CONNECT_REQ:
				return cli.handleConnect(ctx, recvCtx)

			case message.COMMAND_DATA_NOTI:
				return cli.handleData(ctx, recvCtx)

			case message.COMMAND_CHANNEL_WINDOW_UPDATE_NOTI:
				return cli.handleChannelWindowUpdate(ctx, recvCtx)

			case message.COMMAND_CHANNEL_CLOSE_REQ:
				return cli.handleChannelCloseReq(ctx, recvCtx)

			default:
				return errors.New(fmt.Sprintf("unknown command %d", frame.Command()))
			}
		}(); err != nil {
			log.CtxWarnf(ctx, "Client %d accpet handler failed err %s", cli.conf.ClientID, err)
			return err
		}
	}

	return nil
}

func (cli *Client) auth(ctx context.Context) error {
	resp, err := cli.conn.Send(ctx, message.MakeAuthReq(cli.conf.Token, cli.conf.ClientID))
	if err != nil {
		return err
	}

	authResp := resp.Msg().(*pb.AuthResp)

	if authResp.BaseResp.GetCode() != 0 {
		return errors.New(authResp.BaseResp.GetMsg())
	}

	cli.setStatus(ClientStatusInited)

	log.CtxInfof(ctx, "Client %d auth success", cli.conf.ClientID)
	return nil
}

func (cli *Client) handleConnect(ctx context.Context, recvCtx *conn.RecvMessageContext) error {

	if cli.GetStatus() != ClientStatusInited {
		return errors.New("client not inited")
	}

	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}

	req := frame.Msg().Msg().(*pb.ConnectReq)

	channelID := req.GetChannelId()

	log.CtxInfof(ctx, "Client %d handleConnect channelID %d localAddr %s", cli.conf.ClientID, channelID, req.GetAddr())

	localConn, err := NewLocalConn(ctx, cli.channelManager, cli.conn, req.GetAddr(), channelID, req.GetWindowSize(), req.GetBatchSize())
	if err != nil {
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, err.Error()))
		return nil
	}

	if err := cli.channelManager.AddChannel(ctx, localConn); err != nil {
		log.CtxWarnf(ctx, "Client %d handleConnect unknown channelID %d", cli.conf.ClientID, channelID)
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, err.Error()))
		return nil
	}

	localConn.start(ctx)

	recvCtx.SendResp(ctx, message.MakeConnectResp(0, ""))
	return nil
}

func (cli *Client) handleData(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}

	dataNoti := frame.Msg().Msg().(*pb.DataNoti)

	localConn, err := cli.channelManager.GetChannel(dataNoti.GetChannelId())
	if err != nil {
		log.CtxWarnf(ctx, "Client %d handleData unknown channelID %d", cli.conf.ClientID, dataNoti.GetChannelId())
		return nil
	}

	if err := localConn.writeData(ctx, dataNoti.Data); err != nil {
		return err
	}

	return nil
}

func (cli *Client) handleChannelWindowUpdate(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}

	windowUpdateNoti := frame.Msg().Msg().(*pb.ChannelWindowUpdateNoti)

	localConn, err := cli.channelManager.GetChannel(windowUpdateNoti.GetChannelId())
	if err != nil {
		log.CtxWarnf(ctx, "Client %d handleChannelWindowUpdate unknown channelID %d", cli.conf.ClientID, windowUpdateNoti.GetChannelId())
		return nil
	}

	localConn.releaseWindow(ctx, windowUpdateNoti.GetWindowSize())

	return nil
}

func (cli *Client) handleChannelCloseReq(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}

	closeReq := frame.Msg().Msg().(*pb.CloseChannelReq)

	localConn, err := cli.channelManager.GetChannel(closeReq.GetChannelId())
	if err != nil {
		log.CtxWarnf(ctx, "Client %d unknown channelID %d", cli.conf.ClientID, closeReq.GetChannelId())
		return nil
	}

	localConn.passivelyClose(ctx, closeReq.GetMsg())

	recvCtx.SendResp(ctx, message.MakeChannelCloseResp(0, "success"))

	log.CtxInfof(ctx, "Client %d handleChannelCloseReq channelID %d msg %s success", cli.conf.ClientID, closeReq.GetChannelId(), closeReq.GetMsg())
	return nil
}

func (cli *Client) Close(ctx context.Context) {

	cli.setStatus(ClientStatusCloseing)

	cli.channelManager.CloseAll(ctx)

	if cli.conn != nil {
		cli.conn.Close(ctx)
		cli.conn = nil
	}

	cli.setStatus(ClientStatusClosed)

	log.CtxInfof(ctx, "Client %d Close success", cli.conf.ClientID)
}
