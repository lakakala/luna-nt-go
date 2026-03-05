package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/pb"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type ClientStatus int

const (
	ClientStatusUninit   ClientStatus = 1
	ClientStatusInited   ClientStatus = 2
	ClientStatusCloseing ClientStatus = 3
	ClientStatusClosed   ClientStatus = 4
)

type Client struct {
	mutex      sync.Mutex
	conf       *Config
	clientConf *ClientConfig

	clientManager         *ClientManager
	clientListenerManager *ClientListenerManager

	status   ClientStatus
	conn     *conn.Conn
	clientID uint64
}

func newClient(ctx context.Context, conf *Config, clientManager *ClientManager, rawConn net.Conn) (*Client, error) {

	conn, err := conn.NewConnFromRawConn(ctx, rawConn)
	if err != nil {
		return nil, err
	}

	clientListenerManager := newClientListenerManager()
	return &Client{
		mutex:      sync.Mutex{},
		conf:       conf,
		clientConf: nil,

		clientManager:         clientManager,
		clientListenerManager: clientListenerManager,

		clientID: 0,
		status:   ClientStatusUninit,
		conn:     conn,
	}, nil
}

func (c *Client) ClientID() uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.clientID
}

func (c *Client) setStatus(status ClientStatus) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.status = status
}

func (c *Client) GetStatus() ClientStatus {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	return c.status
}

func (c *Client) start(ctx context.Context) {
	c.acceptLoop(ctx)
}

func (c *Client) acceptLoop(ctx context.Context) error {
	for {
		recvCtx, err := c.conn.Accept(ctx)
		if err != nil {
			break
		}

		frame := recvCtx.Frame()
		switch frame.Command() {
		case message.COMMAND_AUTH_REQ:
			c.handleAuthReq(ctx, recvCtx)
		default:
			return errors.New(fmt.Sprintf("unknown command %d", frame.Command()))
		}

	}

	c.Close(ctx)

	return nil
}

func (c *Client) handleAuthReq(ctx context.Context, recvCtx *conn.RecvMessageContext) {
	frame := recvCtx.Frame()

	authReq := frame.Msg().Msg().(*pb.AuthReq)
	log.CtxInfof(ctx, "authReq: %v", authReq)

	clientID := authReq.GetClientId()

	func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		c.clientID = clientID
		c.clientConf = c.conf.Clients[c.clientID]
	}()

	if !c.clientManager.AddClient(ctx, c) {
		recvCtx.SendResp(ctx, message.MakeAuthResp(1, "client already exists"))
		return
	}

	if err := c.startRemoteListener(ctx); err != nil {
		recvCtx.SendResp(ctx, message.MakeAuthResp(1, "start remote listener failed"))
		return
	}

	c.setStatus(ClientStatusInited)

	recvCtx.SendResp(ctx, message.MakeAuthResp(0, "success"))

	log.CtxInfof(ctx, "handleAuthReq client %d success", c.ClientID())
}

func (c *Client) connect(ctx context.Context, localAddr string) (*conn.Channel, error) {
	channel, err := c.conn.CreateChannel(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.conn.Send(ctx, message.MakeConnectReq(channel.ChannelID(), localAddr))
	if err != nil {
		return nil, err
	}

	connectResp := resp.Msg().(*pb.ConnectResp)

	if connectResp.BaseResp.GetCode() != 0 {
		log.CtxWarnf(ctx, "Client %d alloc channel failed code %d msg %s",
			c.ClientID(), connectResp.BaseResp.GetCode(), connectResp.BaseResp.GetMsg())

		return nil, errors.New(connectResp.BaseResp.GetMsg())
	}

	log.CtxInfof(ctx, "Client.connect clientID %d connect to localAddr %s success", c.ClientID(), localAddr)

	return channel, nil
}

func (c *Client) startRemoteListener(ctx context.Context) error {

	for _, clientBind := range c.clientConf.Binds {

		var listener ClientListener
		if clientBind.Type == BindTypeTcp {
			tcpClientListener, err := newTcpClientListener(ctx, clientBind.ID, clientBind.BindAddr, clientBind.LocalAddr, c, c.clientListenerManager)
			if err != nil {
				return err
			}

			listener = tcpClientListener
		} else if clientBind.Type == BindTypeHttpProxy {
			httpProxyListener := newHttpProxyListener(clientBind.ID, clientBind.HttpProxyBindAddr, clientBind.HttpProxyAllowHostList, c, c.clientListenerManager)

			listener = httpProxyListener
		} else if clientBind.Type == BindTypeHttpReverseProxy {
			httpReverseProxyListener := newHttpReverseProxyListener(clientBind.ID, clientBind.HttpReverseProxyLocalAddr, clientBind.HttpReverseProxyBindAddr, c)

			listener = httpReverseProxyListener
		} else {
			return errors.New("")
		}

		if err := c.clientListenerManager.AddListener(ctx, listener); err != nil {
			return err
		}

		go listener.Start(ctx)

		log.CtxInfof(ctx, "Client %d start listener listenderID %d bindAddr %s localAddr %s success",
			c.ClientID(), clientBind.ID, clientBind.BindAddr, clientBind.LocalAddr)
	}

	return nil
}

func (c *Client) Close(ctx context.Context) {
	c.setStatus(ClientStatusCloseing)

	c.setStatus(ClientStatusClosed)
	c.conn.Close(ctx)

	c.clientListenerManager.CloseAll(ctx)
	c.clientManager.RemoveClient(ctx, c.ClientID())

	log.CtxInfof(ctx, "Client %d close", c.ClientID())
}
