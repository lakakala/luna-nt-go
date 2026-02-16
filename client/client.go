package client

import (
	"context"
	"sync"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/pb"
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

	localConnMap map[uint64]*LocalConn

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
		conf:   conf,
		conn:   conn,
		status: ClientStatusUninit,
	}

	return cli, nil
}

func (cli *Client) start(ctx context.Context) {

	cli.auth(ctx)

	for {

		recvCtx, err := cli.conn.Accept()
		if err != nil {
			cli.Close(err)
			return
		}

		switch recvCtx.Frame().Command() {
		case message.COMMAND_CONNECT_REQ:
			cli.handleConnect(ctx, recvCtx)
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

	req := recvCtx.Frame().Msg().Msg().(*pb.ConnectReq)

	localConn, err := NewLocalConn(req.GetAddr(), req.GetSessionId())
	if err != nil {
		recvCtx.SendResp(ctx, message.MakeConnectResp(-1, err.Error()))
		return nil
	}

	func() {
		cli.mutex.Lock()
		defer cli.mutex.Unlock()

		cli.localConnMap[localConn.sessionID] = localConn
	}()

	recvCtx.SendResp(ctx, message.MakeConnectResp(0, ""))
	return nil
}

func (cli *Client) Close(err error) {
	// return cli.conn.Close()
}
