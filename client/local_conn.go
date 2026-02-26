package client

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type ChannelManager struct {
	mutex        sync.Mutex
	localConnMap map[uint64]*LocalConn
}

func newChannelManager() *ChannelManager {
	return &ChannelManager{
		mutex:        sync.Mutex{},
		localConnMap: make(map[uint64]*LocalConn),
	}
}

func (cm *ChannelManager) GetChannel(channelID uint64) (*LocalConn, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	lc, ok := cm.localConnMap[channelID]
	if !ok {
		return nil, errors.New("channel not found")
	}

	return lc, nil
}

func (cm *ChannelManager) AddChannel(ctx context.Context, lc *LocalConn) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	_, ok := cm.localConnMap[lc.GetChannelID()]
	if ok {
		return errors.New("channel already exist")
	}

	log.CtxInfof(ctx, "ChannelManager AddChannel channelID %d", lc.GetChannelID())

	cm.localConnMap[lc.GetChannelID()] = lc

	return nil
}

func (cm *ChannelManager) RemoveChannel(ctx context.Context, channelID uint64) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	_, ok := cm.localConnMap[channelID]
	if !ok {
		return errors.New("channel not found")
	}

	log.CtxInfof(ctx, "ChannelManager RemoveChannel channelID %d", channelID)

	delete(cm.localConnMap, channelID)

	return nil
}

func (cm *ChannelManager) CloseAll(ctx context.Context) {

	for _, lc := range cm.localConnMap {
		lc.close(ctx, "ChannelManager CloseAll")
	}

	log.CtxInfof(ctx, "ChannelManager CloseAll success")
}

type LocalConn struct {
	mutex sync.Mutex

	channelManager *ChannelManager

	addr string

	localConn *net.TCPConn
	conn      *conn.Conn
	channelID uint64

	writeChan chan []byte

	batchSize uint64

	recvWindowManager *conn.ChannelRecvWindowManager
	sendWindowManager *conn.ChannelSendWindowManager

	readStatus  bool
	writeStatus bool
}

func (lc *LocalConn) GetChannelID() uint64 {
	return lc.channelID
}

func NewLocalConn(ctx context.Context, channelManager *ChannelManager, remoteConn *conn.Conn, addr string, channelID uint64, windowSize uint64, batchSize uint64) (*LocalConn, error) {

	localConn := &LocalConn{
		mutex: sync.Mutex{},

		channelManager: channelManager,

		addr: addr,

		localConn: nil,
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
	connection, err := net.DialTimeout("tcp", lc.addr, 3*time.Second)
	if err != nil {
		// lc.close(ctx, "connect failed")
		return
	}

	lc.localConn = connection.(*net.TCPConn)

	go func() {

		gracefulClose := false
		var closeMsg string
		for {

			lc.sendWindowManager.Acquire()

			buf := make([]byte, lc.batchSize)

			n, re := lc.localConn.Read(buf)
			data := buf[:n]

			_, err := lc.conn.Send(ctx, message.MakeDataNoti(lc.channelID, data))

			if err != nil {
				log.CtxErrorf(ctx, "LocalConn channelID %d read data failed err %s", lc.channelID, err)
				closeMsg = err.Error()
				gracefulClose = false
				break
			}

			if re != nil {

				if re == io.EOF {

					_, err := lc.conn.Send(ctx, message.MakeDataNoti(lc.channelID, nil))
					if err != nil {
						log.CtxErrorf(ctx, "LocalConn channelID %d send data_niti failed err %s", lc.channelID, err)
						gracefulClose = false
						closeMsg = err.Error()
					} else {
						gracefulClose = true
					}
				} else {
					closeMsg = re.Error()
					gracefulClose = false
				}

				log.CtxErrorf(ctx, "LocalConn channelID %d read failed err %s", lc.channelID, re)
				break
			}

		}

		if gracefulClose {
			func() {
				lc.mutex.Lock()
				defer lc.mutex.Unlock()

				lc.readStatus = true
			}()

			lc.tryGracefulClose(ctx)
		} else {
			lc.close(ctx, closeMsg)
		}

	}()

	go func() {

		gracefulClose := false
		var closeMsg string

		for data := range lc.writeChan {

			if len(data) == 0 {

				lc.localConn.CloseWrite()
				gracefulClose = true
				break
			}

			lc.recvWindowManager.Acquire()

			_, err := lc.localConn.Write(data)

			if err != nil {
				log.CtxErrorf(ctx, "LocalConn channelID %d write data failed err %s", lc.channelID, err)
				closeMsg = err.Error()
				gracefulClose = false
				break
			}

			needAckWindowSize := lc.recvWindowManager.Release(0)
			if needAckWindowSize > 0 {
				_, err := lc.conn.Send(ctx, message.MakeChanelWindowUpdateNoti(lc.channelID, needAckWindowSize))
				if err != nil {
					log.CtxErrorf(ctx, "LocalConn chnnelID %d send window update failed err %s", lc.channelID, err)
					closeMsg = err.Error()
					gracefulClose = false
					break
				}

			}

		}

		if gracefulClose {

			func() {
				lc.mutex.Lock()
				defer lc.mutex.Unlock()

				lc.writeStatus = true
			}()

			lc.tryGracefulClose(ctx)
		} else {
			lc.close(ctx, closeMsg)
		}
	}()
}

func (lc *LocalConn) tryGracefulClose(ctx context.Context) {

	gracefulClose := func() bool {
		lc.mutex.Lock()
		defer lc.mutex.Unlock()

		return lc.readStatus && lc.writeStatus
	}()

	if gracefulClose {
		lc.close(ctx, "graceful close")
	}
}

func (lc *LocalConn) close(ctx context.Context, msg string) {

	_, err := lc.conn.Send(ctx, message.MakeChannelCloseReq(lc.channelID, msg))
	if err != nil {
		log.CtxErrorf(ctx, "LocalConn channelID %d send channel_close_req failed err %s", lc.channelID, err)
	}

	lc.passivelyClose(ctx, msg)

	log.CtxInfof(ctx, "LocalConn channelID %d close done msg %s", lc.channelID, msg)
}

func (lc *LocalConn) passivelyClose(ctx context.Context, msg string) {
	lc.channelManager.RemoveChannel(ctx, lc.channelID)

	log.CtxInfof(ctx, "LocalConn channelID %d close msg %s", lc.channelID, msg)

	if lc.localConn != nil {
		lc.localConn.Close()
	}
}

func (lc *LocalConn) writeData(ctx context.Context, data []byte) error {
	lc.writeChan <- data
	return nil
}

func (lc *LocalConn) releaseWindow(ctx context.Context, ackWindowSize uint64) {
	lc.sendWindowManager.Release(ackWindowSize)
}
