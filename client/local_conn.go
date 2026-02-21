package client

import (
	"context"
	"net"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type LocalConn struct {
	localConn net.Conn
	conn      *conn.Conn
	channelID uint64
	windows   uint64
	writeChan chan []byte
}

func (lc *LocalConn) GetChannelID() uint64 {
	return lc.channelID
}

func NewLocalConn(ctx context.Context, remoteConn *conn.Conn, addr string, channelID uint64) (*LocalConn, error) {

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	localConn := &LocalConn{
		localConn: conn,
		channelID: channelID,
		conn:      remoteConn,
		windows:   100,
		writeChan: make(chan []byte, 10),
	}

	return localConn, nil
}

func (lc *LocalConn) start(ctx context.Context) {

	go func() {

		for data := range lc.writeChan {

			n, err := lc.localConn.Write(data)
			if err != nil {
				log.CtxErrorf(ctx, "LocalConn channelID %d write data failed err %s", lc.channelID, err)
				break
			}

			log.CtxInfof(ctx, "LocalConn %d write data %v", lc.channelID, data)

			log.CtxInfof(ctx, "LocalConn channelID %d recv %d write %d byte", lc.channelID, len(data), n)
		}
	}()

	go func() {

		for {

			buf := make([]byte, 100)
			n, err := lc.localConn.Read(buf)
			data := buf[:n]

			log.CtxInfof(ctx, "LocalConn %d read data %v", lc.channelID, data)

			lc.conn.Send(ctx, message.MakeDataNoti(lc.channelID, data))

			if err != nil {
				log.CtxErrorf(ctx, "LocalConn channelID %d read data failed err %s", lc.channelID, err)
				break
			}

			log.CtxInfof(ctx, "LocalConn channelID %d read %d byte", lc.channelID, n)
		}

		lc.conn.Send(ctx, message.MakeChannelCloseReq(lc.GetChannelID()))
	}()
}

func (lc *LocalConn) Close() {
	// return lc.localConn.Close()
	return
}

func (lc *LocalConn) writeData(ctx context.Context, data []byte) error {
	lc.writeChan <- data
	return nil
}
