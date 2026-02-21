package server

import (
	"context"
	"errors"
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

func (c *ChannelManager) AddChannel(channel *Channel) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.channelMap[channel.channelID] = channel

	return nil
}

type Channel struct {
	conn           *conn.Conn
	channelManager *ChannelManager
	channelID      uint64
	remoteConn     net.Conn

	writeChan chan []byte
}

func NewChannel(ctx context.Context, channelManager *ChannelManager, channelID uint64, remoteConn net.Conn, conn *conn.Conn) *Channel {
	channel := &Channel{
		conn:           conn,
		channelManager: channelManager,
		channelID:      channelID,
		remoteConn:     remoteConn,

		writeChan: make(chan []byte, 10),
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

		for {

			buf := make([]byte, 1024)

			n, re := c.remoteConn.Read(buf)

			data := buf[:n]

			log.CtxInfof(ctx, "Channel %d read data %v", c.channelID, data)

			_, err := c.conn.Send(ctx, message.MakeDataNoti(c.channelID, data))
			if err != nil {
				log.CtxErrorf(ctx, "Channel chnnelID %d send data failed err %s", c.ChannelID(), err)
				break
			}

			if re != nil {
				log.CtxErrorf(ctx, "Channel %d read failed err %s", c.ChannelID(), re)
				break
			}

			log.CtxInfof(ctx, "Channel %d remoteConn read %d byte", c.channelID, n)
		}

	}()

	go func() {
		for data := range c.writeChan {

			log.CtxInfof(ctx, "Channel %d write data %v", c.channelID, data)

			n, err := c.remoteConn.Write(data)
			if err != nil {
				log.CtxErrorf(ctx, "Channel chnnelID %d write data failed err %s", c.ChannelID(), err)
				return
			}

			log.CtxInfof(ctx, "Channel %d remoteConn recv %d write %d byte", c.channelID, len(data), n)
		}
	}()

}

func (c *Channel) close(ctx context.Context) {

}
