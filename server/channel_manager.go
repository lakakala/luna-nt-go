package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/message"
)

type ChannelManager struct {
	mutex         sync.Mutex
	nextChannelID *atomic.Uint64
	channelMap    map[uint64]*Channel
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

func NewChannel(ctx context.Context, channelManager *ChannelManager, remoteConn net.Conn, conn *conn.Conn) *Channel {
	channel := &Channel{
		conn:           conn,
		channelManager: channelManager,
		channelID:      channelManager.NextChannelID(),
		remoteConn:     remoteConn,

		writeChan: make(chan []byte, 10),
	}

	channel.start(ctx)

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
		data := make([]byte, 1024)
		for {

			n, err := c.remoteConn.Read(data)
			if err != nil {
				return
			}

			_, err = c.conn.Send(ctx, message.MakeDataNoti(c.channelID, data[:n]))
			if err != nil {

			}
		}
	}()

	go func() {
		for data := range c.writeChan {
			_, err := c.remoteConn.Write(data)
			if err != nil {
				return
			}
		}
	}()

}
