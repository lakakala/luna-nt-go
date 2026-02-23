package server

import (
	"context"
	"net"

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
	cli, err := newClient(ctx, ser.conf, ser.clientManager, rawConn)
	if err != nil {
		log.CtxErrorf(ctx, "newClient err: %v", err)
		return
	}

	if err := cli.start(ctx); err != nil {
		log.CtxErrorf(ctx, "cli.start err: %v", err)
		return
	}
}
