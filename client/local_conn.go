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

	writeChan chan []byte

	batchSize uint64

	recvWindowManager *conn.ChannelRecvWindowManager
	sendWindowManager *conn.ChannelSendWindowManager
}

func (lc *LocalConn) GetChannelID() uint64 {
	return lc.channelID
}

func NewLocalConn(ctx context.Context, remoteConn *conn.Conn, addr string, channelID uint64, windowSize uint64, batchSize uint64) (*LocalConn, error) {

	connection, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	localConn := &LocalConn{
		localConn: connection,
		channelID: channelID,
		conn:      remoteConn,

		writeChan: make(chan []byte, windowSize),

		batchSize: batchSize,

		recvWindowManager: conn.NewChannelRecvWindowManager(windowSize),
		sendWindowManager: conn.NewChannelSendWindowManager(windowSize),
	}

	return localConn, nil
}

func (lc *LocalConn) start(ctx context.Context) {

	go func() {

		for {

			lc.sendWindowManager.Acquire()

			buf := make([]byte, lc.batchSize)

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

	go func() {

		for data := range lc.writeChan {

			n, err := lc.localConn.Write(data)
			if err != nil {
				log.CtxErrorf(ctx, "LocalConn channelID %d write data failed err %s", lc.channelID, err)
				break
			}

			needAckWindowSize := lc.recvWindowManager.Release(0)
			if needAckWindowSize > 0 {
				_, err := lc.conn.Send(ctx, message.MakeChanelWindowUpdateNoti(lc.channelID, needAckWindowSize))
				if err != nil {
					log.CtxErrorf(ctx, "LocalConn chnnelID %d send window update failed err %s", lc.channelID, err)
					return
				}

				log.CtxInfof(ctx, "LocalConn channelID %d send window update %d", lc.channelID, needAckWindowSize)
			}

			log.CtxInfof(ctx, "LocalConn channelID %d recv %d write %d byte", lc.channelID, len(data), n)
		}
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

func (lc *LocalConn) releaseWindow(ctx context.Context, ackWindowSize uint64) {
	log.CtxInfof(ctx, "LocalConn %d release window size %d", lc.channelID, ackWindowSize)
	lc.sendWindowManager.Release(ackWindowSize)
}
