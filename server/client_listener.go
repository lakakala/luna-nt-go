package server

import (
	"context"
	"net"
	"sync"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type ClientListener struct {
	id                    uint64
	bindAddr              string
	localAddr             string
	listener              net.Listener
	client                *Client
	clientListenerManager *ClientListenerManager
}

func newClientListener(ctx context.Context, id uint64, bindAddr string, localAddr string, client *Client, clientListenerManager *ClientListenerManager) (*ClientListener, error) {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	return &ClientListener{
		id:                    id,
		bindAddr:              bindAddr,
		localAddr:             localAddr,
		listener:              listener,
		client:                client,
		clientListenerManager: clientListenerManager,
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
			break
		}

		log.CtxInfof(ctx, "ClientListener %d accept conn remoteAddr %s localAddr %s", cl.ID(), conn.RemoteAddr().String(), conn.LocalAddr().String())

		go cl.handleConn(ctx, conn)
	}

	log.CtxInfof(ctx, "ClientListener %d start loop end", cl.ID())
}

func (cl *ClientListener) Close(ctx context.Context) {
	cl.listener.Close()

	cl.clientListenerManager.RemoveListener(cl.ID())
}

func (cl *ClientListener) handleConn(ctx context.Context, conn net.Conn) {
	if err := cl.client.connect(ctx, conn, cl.localAddr); err != nil {
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

func (m *ClientListenerManager) RemoveListener(id uint64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.listeners, id)

	return nil
}

func (m *ClientListenerManager) GetListener(id uint64) (*ClientListener, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	listener, ok := m.listeners[id]
	return listener, ok
}

func (m *ClientListenerManager) CloseAll(ctx context.Context) {

	for _, listener := range m.listeners {
		listener.Close(ctx)
	}

	log.CtxInfof(ctx, "ClientListenerManager CloseAll done")
}
