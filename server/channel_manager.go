package server

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type ChannelManager struct {
	mutex         sync.Mutex
	nextChannelID *atomic.Uint64
	channelMap    map[uint64]*Channel
}

func newChannelManager() *ChannelManager {
	return &ChannelManager{
		mutex:         sync.Mutex{},
		nextChannelID: &atomic.Uint64{},
		channelMap:    make(map[uint64]*Channel),
	}
}

func (c *ChannelManager) NextChannelID() uint64 {
	return c.nextChannelID.Add(1)
}

func (c *ChannelManager) GetChannel(channelID uint64) (*Channel, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	channel, ok := c.channelMap[channelID]
	if !ok {
		return nil, errors.New("channel not found")
	}

	return channel, nil
}

func (c *ChannelManager) AddChannel(ctx context.Context, channel *Channel) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_, ok := c.channelMap[channel.channelID]
	if ok {
		return errors.New("channel already exist")
	}

	log.CtxInfof(ctx, "ChannelManager AddChannel channelID %d", channel.channelID)

	c.channelMap[channel.channelID] = channel

	return nil
}

func (c *ChannelManager) RemoveChannel(ctx context.Context, channelID uint64) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	log.CtxInfof(ctx, "ChannelManager RemoveChannel channelID %d", channelID)
	delete(c.channelMap, channelID)

	return nil
}

type Channel struct {
	mutex          sync.Mutex
	conn           *conn.Conn
	channelManager *ChannelManager
	channelID      uint64
	remoteConn     *net.TCPConn

	writeChan chan []byte

	sendWindowManager *conn.ChannelSendWindowManager
	recvWindowManager *conn.ChannelRecvWindowManager
	batchSize         uint64

	readStatus  bool
	writeStatus bool
}

func NewChannel(ctx context.Context, channelManager *ChannelManager, channelID uint64, batchSize uint64, windowSize uint64, remoteConn net.Conn, connection *conn.Conn) *Channel {

	sendWindowManager := conn.NewChannelSendWindowManager(windowSize)
	recvWindowManager := conn.NewChannelRecvWindowManager(windowSize)

	channel := &Channel{
		mutex:          sync.Mutex{},
		conn:           connection,
		channelManager: channelManager,
		channelID:      channelID,
		remoteConn:     remoteConn.(*net.TCPConn),

		writeChan:         make(chan []byte, windowSize),
		sendWindowManager: sendWindowManager,
		recvWindowManager: recvWindowManager,

		batchSize: batchSize,
	}

	return channel
}

func (c *Channel) ChannelID() uint64 {
	return c.channelID
}

func (c *Channel) recvData(ctx context.Context, data []byte) error {
	c.writeChan <- data
	return nil
}

func (c *Channel) start(ctx context.Context) {

	go func() {

		gracefulClose := false
		var closeMsg string
		for {

			c.sendWindowManager.Acquire()

			buf := make([]byte, c.batchSize)

			n, re := c.remoteConn.Read(buf)

			data := buf[:n]

			_, err := c.conn.Send(ctx, message.MakeDataNoti(c.channelID, data))
			if err != nil {
				log.CtxErrorf(ctx, "Channel chnnelID %d send data failed err %s", c.ChannelID(), err)
				gracefulClose = false
				closeMsg = err.Error()
				break
			}

			if re != nil {

				if re == io.EOF {
					log.CtxInfof(ctx, "Channel %d read EOF", c.ChannelID())

					_, err := c.conn.Send(ctx, message.MakeDataNoti(c.channelID, nil))
					if err != nil {
						log.CtxErrorf(ctx, "Channel chnnelID %d send data failed err %s", c.ChannelID(), err)
						gracefulClose = false
						closeMsg = err.Error()
						break
					}

					gracefulClose = true
				} else {
					log.CtxWarnf(ctx, "Channel %d read failed err %s", c.ChannelID(), re)
				}

				break
			}
		}

		if !gracefulClose {
			c.close(ctx, closeMsg)
		} else {

			func() {

				c.mutex.Lock()
				defer c.mutex.Unlock()

				c.readStatus = true
			}()

			c.tryGracefulClose(ctx)
		}
	}()

	go func() {

		gracefulClose := false
		var closeMsg string
		for data := range c.writeChan {

			if len(data) == 0 {

				c.remoteConn.CloseWrite()

				gracefulClose = true
				break
			}

			c.recvWindowManager.Acquire()

			_, err := c.remoteConn.Write(data)
			if err != nil {
				log.CtxErrorf(ctx, "Channel chnnelID %d write data failed err %s", c.ChannelID(), err)
				closeMsg = err.Error()
				gracefulClose = false
				break
			}

			needAckWindowSize := c.recvWindowManager.Release(0)
			if needAckWindowSize > 0 {
				_, err := c.conn.Send(ctx, message.MakeChanelWindowUpdateNoti(c.channelID, needAckWindowSize))
				if err != nil {
					log.CtxErrorf(ctx, "Channel chnnelID %d send window update failed err %s", c.ChannelID(), err)
					closeMsg = err.Error()
					gracefulClose = false
					break
				}
			}
		}

		if !gracefulClose {
			c.close(ctx, closeMsg)
		} else {

			func() {
				c.mutex.Lock()
				defer c.mutex.Unlock()

				c.writeStatus = true
			}()

			c.tryGracefulClose(ctx)
		}
	}()

}

func (c *Channel) Release(ctx context.Context, ackWindowSize uint64) {
	c.sendWindowManager.Release(ackWindowSize)
}

func (c *Channel) tryGracefulClose(ctx context.Context) {
	gracefulClose := func() bool {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		return c.readStatus && c.writeStatus
	}()

	if gracefulClose {
		c.close(ctx, "graceful close")
	}
}

func (c *Channel) close(ctx context.Context, msg string) {

	_, err := c.conn.Send(ctx, message.MakeChannelCloseReq(c.channelID, msg))
	if err != nil {
		log.CtxErrorf(ctx, "Channel chnnelID %d send close failed err %s", c.ChannelID(), err)
	}

	c.channelManager.RemoveChannel(ctx, c.channelID)

	c.remoteConn.Close()
}
