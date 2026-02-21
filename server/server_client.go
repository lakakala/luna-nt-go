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
	ClientStatusUninit ClientStatus = 1
	ClientStatusInited ClientStatus = 2
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

func (c *Client) start(ctx context.Context) error {
	for {
		recvCtx, err := c.conn.Accept(ctx)
		if err != nil {
			return err
		}

		if err := func() error {
			frame, err := recvCtx.Frame()
			if err != nil {
				return err
			}

			switch frame.Command() {
			case message.COMMAND_AUTH_REQ:
				return c.handleAuthReq(ctx, recvCtx)
			case message.COMMAND_DATA:
				return c.handleData(ctx, recvCtx)
			default:
				return errors.New(fmt.Sprintf("unknown command %d", frame.Command()))
			}
		}(); err != nil {
			log.CtxErrorf(ctx, "Client %d accpet handler failed err %s", c.ClientID(), err)
			break
		}
	}

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

	c.status = ClientStatusInited

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
		log.CtxWarnf(ctx, "Client %d alloc channel failed code %d msg %s",
			c.ClientID(), connectResp.BaseResp.GetCode(), connectResp.BaseResp.GetMsg())
		return errors.New(connectResp.BaseResp.GetMsg())
	}

	channel := NewChannel(ctx, c.channelManager, channelID, remoteConn, c.conn)

	log.CtxInfof(ctx, "Client %d alloc channel success channelID %d", c.ClientID(), channel.ChannelID())

	if err := c.channelManager.AddChannel(channel); err != nil {
		return err
	}

	channel.start(ctx)

	return nil
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

		log.CtxInfof(ctx, "Client %d start listener listenderID %d bindAddr %s localAddr %s success",
			c.ClientID(), clientBind.ID, clientBind.BindAddr, clientBind.LocalAddr)
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

		log.CtxInfof(ctx, "ClientListener %d accept conn remoteAddr %s localAddr %s", cl.ID(), conn.RemoteAddr().String(), conn.LocalAddr().String())

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

func newClientListenerManager() *ClientListenerManager {
	return &ClientListenerManager{
		mutex:     sync.Mutex{},
		listeners: make(map[uint64]*ClientListener),
	}
}

func (m *ClientListenerManager) AddListener(ctx context.Context, listener *ClientListener) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.listeners[listener.ID()] = listener

	return nil
}
