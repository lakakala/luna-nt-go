package server

import (
	"context"
	"net"
	"sync"

	"github.com/lakakala/luna-nt-go/utils/log"
)

func RunServer(conf *Config) error {
	ser := newServer(conf)
	return ser.start(context.Background())
}

type Server struct {
	conf          *Config
	clientManager *ClientManager
}

func newServer(conf *Config) *Server {
	return &Server{
		conf:          conf,
		clientManager: newClientManager(),
	}
}

func (ser *Server) start(ctx context.Context) error {

	listener, err := net.Listen("tcp", ser.conf.Addr)
	if err != nil {
		return err
	}

	for {

		rawConn, err := listener.Accept()
		if err != nil {
			return err
		}

		go ser.handleConn(ctx, rawConn)
	}
}

func (ser *Server) handleConn(ctx context.Context, rawConn net.Conn) {
	cli, err := newClient(ctx, rawConn)
	if err != nil {
		log.CtxErrorf(ctx, "newClient err: %v", err)
		return
	}

	if err := cli.start(ctx); err != nil {
		log.CtxErrorf(ctx, "cli.start err: %v", err)
		return
	}
}

type ClientManager struct {
	mutex     sync.Mutex
	clientMap map[uint64]*Client
}

func newClientManager() *ClientManager {
	return &ClientManager{
		clientMap: make(map[uint64]*Client),
	}
}

func (cm *ClientManager) AddClient(ctx context.Context, client *Client) bool {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if _, ok := cm.clientMap[client.ClientID()]; ok {
		log.CtxErrorf(ctx, "clientID %d already exists", client.ClientID())
		return false
	}

	cm.clientMap[client.ClientID()] = client
	return true
}
