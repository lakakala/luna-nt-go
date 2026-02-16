package server

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/pb"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type ClientStatus int

const (
	ClientStatusUninit ClientStatus = 1
	ClientStatusInited ClientStatus = 2
)

type Client struct {
	mutex      sync.Mutex
	clientConf *ClientConfig

	channelManager *ChannelManager

	clientManager         *ClientManager
	clientListenerManager *ClientListenerManager

	status   ClientStatus
	conn     *conn.Conn
	clientID uint64
}

func newClient(ctx context.Context, rawConn net.Conn) (*Client, error) {

	conn, err := conn.NewConnFromRawConn(ctx, rawConn)
	if err != nil {
		return nil, err
	}

	return &Client{
		status:   ClientStatusUninit,
		conn:     conn,
		clientID: 0,
	}, nil
}

func (c *Client) ClientID() uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.clientID
}

func (c *Client) start(ctx context.Context) error {
	for {
		recvCtx, err := c.conn.Accept()
		if err != nil {
			return err
		}

		switch recvCtx.Frame().Command() {
		case message.COMMAND_AUTH_REQ:
			c.handleAuthReq(ctx, recvCtx)
		case message.COMMAND_DATA:
			c.handleData(ctx, recvCtx)
		}
	}
}

func (c *Client) handleAuthReq(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	authReq := recvCtx.Frame().Msg().Msg().(*pb.AuthReq)
	log.CtxInfof(ctx, "authReq: %v", authReq)
	if authReq.ClientId == nil {
		log.CtxErrorf(ctx, "clientID is nil")
		return nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.clientID != 0 {
		log.CtxErrorf(ctx, "clientID %d already inited", c.clientID)
		return nil
	}

	c.clientID = *authReq.ClientId

	if !c.clientManager.AddClient(ctx, c) {

		if err := recvCtx.SendResp(ctx, message.MakeAuthResp(1, "client already exists")); err != nil {
			return err
		}
		return nil
	}

	if err := c.startRemoteListener(ctx); err != nil {
		if err := recvCtx.SendResp(ctx, message.MakeAuthResp(1, "client already exists")); err != nil {
			return err
		}
		return nil
	}

	c.status = ClientStatusInited

	if err := recvCtx.SendResp(ctx, message.MakeAuthResp(0, "success")); err != nil {
		return err
	}

	return nil
}

func (c *Client) handleData(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	dataNoti := recvCtx.Frame().Msg().Msg().(*pb.DataNoti)

	channelID := uint64(0)

	channel, err := c.channelManager.GetChannel(channelID)
	if err != nil {
		return err
	}

	if err := channel.recvData(ctx, dataNoti.Data); err != nil {
		return err
	}

	return nil
}

func (c *Client) Connect(ctx context.Context, remoteConn net.Conn, localAddr string) error {

	channelID := c.channelManager.NextChannelID()
	resp, err := c.conn.Send(ctx, message.MakeConnectReq(channelID, localAddr))
	if err != nil {
		return err
	}

	connectResp := resp.Msg().(*pb.ConnectResp)
	if connectResp.BaseResp.GetCode() != 0 {
		return errors.New(connectResp.BaseResp.GetMsg())
	}

	channel := NewChannel(ctx, c.channelManager, remoteConn, c.conn)

	return c.channelManager.AddChannel(channel)
}

func (c *Client) startRemoteListener(ctx context.Context) error {

	for _, clientBind := range c.clientConf.Binds {
		listener, err := newClientListener(ctx, clientBind.ID, clientBind.BindAddr, clientBind.LocalAddr, c)
		if err != nil {
			return err
		}

		if err := c.clientListenerManager.AddListener(ctx, listener); err != nil {
			return err
		}

		go listener.Start(ctx)
	}

	return nil
}

type ClientListener struct {
	id        uint64
	bindAddr  string
	localAddr string
	listener  net.Listener
	client    *Client
}

func newClientListener(ctx context.Context, id uint64, bindAddr string, localAddr string, client *Client) (*ClientListener, error) {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	return &ClientListener{
		id:        id,
		bindAddr:  bindAddr,
		localAddr: localAddr,
		listener:  listener,
		client:    client,
	}, nil
}

func (cl *ClientListener) ID() uint64 {
	return cl.id
}

func (cl *ClientListener) Start(ctx context.Context) {
	for {
		conn, err := cl.listener.Accept()
		if err != nil {
			log.CtxErrorf(ctx, "listener.Accept err: %v", err)
			continue
		}

		go cl.handleConn(ctx, conn)
	}
}

func (cl *ClientListener) handleConn(ctx context.Context, conn net.Conn) {
	if err := cl.client.Connect(ctx, conn, cl.localAddr); err != nil {
		log.CtxErrorf(ctx, "cl.client.Connect err: %v", err)
		return
	}
}

type ClientListenerManager struct {
	mutex     sync.Mutex
	listeners map[uint64]*ClientListener
}

func (m *ClientListenerManager) AddListener(ctx context.Context, listener *ClientListener) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.listeners[listener.ID()] = listener

	return nil
}
