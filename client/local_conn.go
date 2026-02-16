package client

import (
	"net"

	"github.com/lakakala/luna-nt-go/conn"
)

type LocalConn struct {
	localConn net.Conn
	conn      *conn.Conn
	sessionID uint64
	windows   uint64
}

func NewLocalConn(addr string, sessionID uint64) (*LocalConn, error) {

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &LocalConn{
		localConn: conn,
		sessionID: sessionID,
	}, nil
}

func (lc *LocalConn) start() {
}

func (lc *LocalConn) Close() {
	// return lc.localConn.Close()
	return
}
