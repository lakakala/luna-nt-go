package server

import (
	"context"
	"net"
	"sync"

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

		// _, err = io.Copy(channel, conn)
		// if err != nil {
		// 	log.CtxErrorf(ctx, "io.Copy channel %d -> conn %s failed err %s", channel.ChannelID(), conn.RemoteAddr().String(), err)
		// }

		// channel.CloseRead()
		// conn.CloseWrite()
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
		// _, err = io.Copy(conn, channel)
		// if err != nil {
		// 	log.CtxErrorf(ctx, "io.Copy conn %s -> channel %d failed err %s", conn.RemoteAddr().String(), channel.ChannelID(), err)
		// }

		// channel.CloseWrite()
		// channel.CloseRead()
		waitGroup.Done()
	}()

	waitGroup.Wait()

	conn.Close()
	channel.Close()
	return nil
}
