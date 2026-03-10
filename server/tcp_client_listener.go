package server

import (
	"context"
	"net"
	"sync"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type TcpClientListener struct {
	id        uint64
	bindAddr  string
	localAddr string
	listener  net.Listener
	client    *Client
}

func newTcpClientListener(ctx context.Context, id uint64, bindAddr string, localAddr string, client *Client) (ClientListener, error) {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	return &TcpClientListener{
		id:        id,
		bindAddr:  bindAddr,
		localAddr: localAddr,
		listener:  listener,
		client:    client,
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

		log.CtxInfof(ctx, "TcpClientListener %d accept conn remoteAddr %s localAddr %s", cl.ID(), conn.RemoteAddr().String(), conn.LocalAddr().String())

		go cl.handleConn(ctx, conn)
	}

	if err := cl.cleanup(ctx); err != nil {
		log.CtxWarnf(ctx, "TcpClientListener.cleanup failed err %s", err)
	}

	log.CtxInfof(ctx, "TcpClientListener %d start loop end", cl.ID())
}

func (cl *TcpClientListener) Close(ctx context.Context) {
	if cl.listener != nil {
		cl.listener.Close()
	}

	cl.listener = nil
}

func (cl *TcpClientListener) handleConn(ctx context.Context, conn net.Conn) {

	if err := cl.doHandleConn(ctx, conn.(*net.TCPConn)); err != nil {
		log.CtxErrorf(ctx, "doHandleConn failed err %s", err)
	}

}
func (cl *TcpClientListener) doHandleConn(ctx context.Context, conn *net.TCPConn) error {

	channel, err := cl.client.connect(ctx, cl.localAddr)
	if err != nil {
		return err
	}

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(2)

	go func() {

		buf := make([]byte, 10)
		for {
			readN, readErr := channel.Read(buf)

			_, writeErr := conn.Write(buf[:readN])

			if readErr != nil {
				break
			}

			if writeErr != nil {
				break
			}
		}

		waitGroup.Done()
	}()

	go func() {
		buf := make([]byte, 1024)

		for {
			readN, readErr := conn.Read(buf)

			_, writeErr := channel.Write(buf[:readN])

			if readErr != nil {
				break
			}

			if writeErr != nil {
				break
			}
		}

		waitGroup.Done()
	}()

	waitGroup.Wait()

	conn.Close()
	channel.Close(ctx)
	return nil
}

func (cl *TcpClientListener) cleanup(ctx context.Context) error {

	if cl.listener != nil {
		defer func() {
			cl.listener = nil
		}()
		cl.listener.Close()
	}

	return nil
}
