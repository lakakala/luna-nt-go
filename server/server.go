package server

import (
	"context"
	"net"

	"github.com/lakakala/luna-nt-go/utils/log"
)

var ser *Server

func RunServer(conf *Config) error {
	ser = newServer(conf)
	return ser.start(context.Background())
}

func CloseServer() {
	if ser != nil {
		ser.close()
		ser = nil
	}
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

func (ser *Server) close() {

}

func (ser *Server) handleConn(ctx context.Context, rawConn net.Conn) {
	cli, err := newClient(ctx, ser.conf, ser.clientManager, rawConn)
	if err != nil {
		log.CtxErrorf(ctx, "newClient err: %v", err)
		return
	}

	cli.start(ctx)
}
