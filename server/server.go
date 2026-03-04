package server

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/lakakala/luna-nt-go/utils/log"
)

func RunServer(conf *Config, cancelChan chan os.Signal) {

	ctx := context.Background()
	ser := newServer(conf)

	existChan := make(chan error, 1)
	go func() {
		err := ser.start(ctx)

		existChan <- err
	}()

	select {
	case signal := <-cancelChan:
		fmt.Printf("Server cancel signal: %v\n", signal)
	case err := <-existChan:
		if err != nil {
			fmt.Printf("Server exist err: %v\n", err)
		} else {
			fmt.Printf("Server exist\n")
		}
	}

	ser.close(ctx)

	fmt.Printf("Server close success\n")
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

func (ser *Server) close(ctx context.Context) {
}

func (ser *Server) handleConn(ctx context.Context, rawConn net.Conn) {
	cli, err := newClient(ctx, ser.conf, ser.clientManager, rawConn)
	if err != nil {
		log.CtxErrorf(ctx, "newClient err: %v", err)
		return
	}

	cli.start(ctx)
}
