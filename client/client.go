package client

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	ClientStatusUninit ClientStatus = 1
	ClientStatusInited ClientStatus = 2
)

type Client struct {
	mutex sync.Mutex

	channelManager *ChannelManager

	status ClientStatus
	conf   *Config
	conn   *conn.Conn
}

func newClient(ctx context.Context, conf *Config) (*Client, error) {

	conn, err := conn.NewConn(ctx, conf.Addr)
	if err != nil {
		return nil, err
	}

	cli := &Client{
		mutex: sync.Mutex{},

		channelManager: newChannelManager(),

		status: ClientStatusUninit,
		conf:   conf,
		conn:   conn,
	}

	return cli, nil
}

func (cli *Client) start(ctx context.Context) {

	cli.auth(ctx)

	for {

		recvCtx, err := cli.conn.Accept(ctx)
		if err != nil {
			cli.Close(err)
			return
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
			log.CtxErrorf(ctx, "Client %d accpet handler failed err %s", cli.conf.ClientID, err)
		}
	}
}

func (cli *Client) auth(ctx context.Context) {
	resp, err := cli.conn.Send(ctx, message.MakeAuthReq(cli.conf.Token, cli.conf.ClientID))
	if err != nil {
		cli.Close(err)
		return
	}

	authResp := resp.Msg().(*pb.AuthResp)

	if authResp.BaseResp.GetCode() != 0 {
		cli.Close(err)
		return
	}

	func() {
		cli.mutex.Lock()
		defer cli.mutex.Unlock()

		cli.status = ClientStatusInited
	}()
}

func (cli *Client) handleConnect(ctx context.Context, recvCtx *conn.RecvMessageContext) error {

	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}

	req := frame.Msg().Msg().(*pb.ConnectReq)

	log.CtxInfof(ctx, "Client %d handleConnect channelID %d localAddr %s", cli.conf.ClientID, req.GetChannelId(), req.GetAddr())

	localConn, err := NewLocalConn(ctx, cli.channelManager, cli.conn, req.GetAddr(), req.GetChannelId(), req.GetWindowSize(), req.GetBatchSize())
	if err != nil {
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, err.Error()))
		return nil
	}

	if err := cli.channelManager.AddChannel(ctx, localConn); err != nil {
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
		log.CtxErrorf(ctx, "Client %d unknown channelID %d", cli.conf.ClientID, dataNoti.GetChannelId())
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
		log.CtxErrorf(ctx, "Client %d unknown channelID %d", cli.conf.ClientID, windowUpdateNoti.GetChannelId())
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
		log.CtxErrorf(ctx, "Client %d unknown channelID %d", cli.conf.ClientID, closeReq.GetChannelId())
		return nil
	}

	localConn.close(ctx, closeReq.GetMsg())

	return nil
}

func (cli *Client) Close(err error) {
	// return cli.conn.Close()
}
