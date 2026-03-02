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

		frame := recvCtx.Frame()

		switch frame.Command() {
		case message.COMMAND_CONNECT_REQ:
			cli.handleConnect(ctx, recvCtx)
		default:
			return errors.New(fmt.Sprintf("unknown command %d", frame.Command()))
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

func (cli *Client) handleConnect(ctx context.Context, recvCtx *conn.RecvMessageContext) {

	if cli.GetStatus() != ClientStatusInited {
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, "client not inited"))
		return
	}

	frame := recvCtx.Frame()

	req := frame.Msg().Msg().(*pb.ConnectReq)

	channelID := req.GetChannelId()
	localAddr := req.GetAddr()

	channel := cli.conn.GetChannel(ctx, channelID)
	if channel == nil {
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, "unknown channelID"))
		return
	}

	localConn, err := NewLocalConn(ctx, channel, localAddr)
	if err != nil {
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, err.Error()))
		return
	}

	go localConn.start(ctx)

	recvCtx.SendResp(ctx, message.MakeConnectResp(0, "success"))

	log.CtxInfof(ctx, "Client %d handleConnect channelID %d localAddr %s success", cli.conf.ClientID, channelID, localAddr)
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
