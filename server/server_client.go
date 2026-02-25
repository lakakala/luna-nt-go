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

	channelManager *ChannelManager

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

	channelManager := newChannelManager()

	clientListenerManager := newClientListenerManager()
	return &Client{
		mutex:      sync.Mutex{},
		conf:       conf,
		clientConf: nil,

		channelManager: channelManager,

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

func (c *Client) start(ctx context.Context) error {
	for {
		recvCtx, err := c.conn.Accept(ctx)
		if err != nil {
			break
		}

		if err := func() error {
			frame, err := recvCtx.Frame()
			if err != nil {
				return err
			}

			switch frame.Command() {
			case message.COMMAND_AUTH_REQ:
				return c.handleAuthReq(ctx, recvCtx)

			case message.COMMAND_DATA_NOTI:
				return c.handleData(ctx, recvCtx)

			case message.COMMAND_CHANNEL_WINDOW_UPDATE_NOTI:
				return c.handleChannelWindowUpdate(ctx, recvCtx)

			case message.COMMAND_CHANNEL_CLOSE_REQ:
				return c.handleChannelCloseReq(ctx, recvCtx)

			default:
				return errors.New(fmt.Sprintf("unknown command %d", frame.Command()))
			}
		}(); err != nil {
			log.CtxWarnf(ctx, "Client %d accpet handler failed err %s", c.ClientID(), err)
			break
		}
	}

	c.Close(ctx)

	return nil
}

func (c *Client) handleAuthReq(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}
	authReq := frame.Msg().Msg().(*pb.AuthReq)
	log.CtxInfof(ctx, "authReq: %v", authReq)
	if authReq.ClientId == nil {
		log.CtxErrorf(ctx, "clientID is nil")
		return nil
	}

	func() {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		c.clientID = *authReq.ClientId
		c.clientConf = c.conf.Clients[c.clientID]
	}()

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

	c.setStatus(ClientStatusInited)

	if err := recvCtx.SendResp(ctx, message.MakeAuthResp(0, "success")); err != nil {
		return err
	}

	return nil
}

func (c *Client) handleData(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}
	dataNoti := frame.Msg().Msg().(*pb.DataNoti)

	channelID := dataNoti.GetChannelId()

	channel, err := c.channelManager.GetChannel(channelID)
	if err != nil {
		log.CtxWarnf(ctx, "Client %d handleData unknown channelID %d", c.ClientID(), channelID)
		return nil
	}

	if err := channel.recvData(ctx, dataNoti.Data); err != nil {
		return err
	}

	return nil
}

func (c *Client) handleChannelWindowUpdate(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}
	windowUpdateNoti := frame.Msg().Msg().(*pb.ChannelWindowUpdateNoti)

	channelID := windowUpdateNoti.GetChannelId()

	channel, err := c.channelManager.GetChannel(channelID)
	if err != nil {
		log.CtxWarnf(ctx, "Client %d handleChannelWindowUpdate unknown channelID %d", c.ClientID(), channelID)
		return nil
	}

	channel.Release(ctx, windowUpdateNoti.GetWindowSize())

	return nil
}

func (c *Client) handleChannelCloseReq(ctx context.Context, recvCtx *conn.RecvMessageContext) error {
	frame, err := recvCtx.Frame()
	if err != nil {
		return err
	}
	closeReq := frame.Msg().Msg().(*pb.CloseChannelReq)

	channelID := closeReq.GetChannelId()

	channel, err := c.channelManager.GetChannel(channelID)
	if err != nil {
		log.CtxWarnf(ctx, "Client %d handleChannelCloseReq unknown channelID %d", c.ClientID(), channelID)
		return nil
	}

	channel.passivelyClose(ctx, closeReq.GetMsg())

	recvCtx.SendResp(ctx, message.MakeChannelCloseResp(0, "success"))

	return nil
}

func (c *Client) connect(ctx context.Context, remoteConn net.Conn, localAddr string) error {

	if c.GetStatus() != ClientStatusInited {
		return errors.New("client not inited")
	}

	channelID := c.channelManager.NextChannelID()

	channel := NewChannel(ctx, c.channelManager, channelID, conn.BATCH_SIZE, conn.WINDOW_SIZE, remoteConn, c.conn)

	log.CtxInfof(ctx, "Client %d alloc channel success channelID %d", c.ClientID(), channel.ChannelID())

	if err := c.channelManager.AddChannel(ctx, channel); err != nil {
		return err
	}

	resp, err := c.conn.Send(ctx, message.MakeConnectReq(channelID, localAddr, conn.BATCH_SIZE, conn.WINDOW_SIZE))
	if err != nil {
		return err
	}

	connectResp := resp.Msg().(*pb.ConnectResp)
	if connectResp.BaseResp.GetCode() != 0 {
		log.CtxWarnf(ctx, "Client %d alloc channel failed code %d msg %s",
			c.ClientID(), connectResp.BaseResp.GetCode(), connectResp.BaseResp.GetMsg())

		c.channelManager.CloseChannel(ctx, channelID)

		return errors.New(connectResp.BaseResp.GetMsg())
	}

	channel.start(ctx)

	return nil
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

	c.channelManager.CloseAll(ctx)

	c.setStatus(ClientStatusClosed)
	c.conn.Close(ctx)

	c.clientListenerManager.CloseAll(ctx)
	c.clientManager.RemoveClient(ctx, c.ClientID())

	log.CtxInfof(ctx, "Client %d close", c.ClientID())
}
