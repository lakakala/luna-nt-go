package server

import (
	"context"
	"net"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type TcpClientListener struct {
	id                    uint64
	bindAddr              string
	localAddr             string
	listener              net.Listener
	client                *Client
	clientListenerManager *ClientListenerManager
}

func newTcpClientListener(ctx context.Context, id uint64, bindAddr string, localAddr string, client *Client, clientListenerManager *ClientListenerManager) (ClientListener, error) {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	return &TcpClientListener{
		id:                    id,
		bindAddr:              bindAddr,
		localAddr:             localAddr,
		listener:              listener,
		client:                client,
		clientListenerManager: clientListenerManager,
	}, nil
}

func (cl *TcpClientListener) ID() uint64 {
	return cl.id
}

func (cl *TcpClientListener) Start(ctx context.Context) {
	for {
		conn, err := cl.listener.Accept()
		if err != nil {
			log.CtxErrorf(ctx, "listener.Accept err: %v", err)
			break
		}

		log.CtxInfof(ctx, "ClientListener %d accept conn remoteAddr %s localAddr %s", cl.ID(), conn.RemoteAddr().String(), conn.LocalAddr().String())

		go cl.handleConn(ctx, conn)
	}

	log.CtxInfof(ctx, "ClientListener %d start loop end", cl.ID())
}

func (cl *TcpClientListener) Close(ctx context.Context) {
	cl.listener.Close()

	cl.clientListenerManager.RemoveListener(cl.ID())
}

func (cl *TcpClientListener) handleConn(ctx context.Context, conn net.Conn) {

	bufConn := NewBuferConn(conn)
	if err := cl.client.connect(ctx, bufConn, cl.localAddr); err != nil {
		log.CtxErrorf(ctx, "cl.client.Connect err: %v", err)
		return
	}
}
