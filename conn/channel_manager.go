package conn

import (
	"context"
	"sync"
	"sync/atomic"
)

type ChannelManager struct {
	mutex sync.Mutex

	nextChannelID *atomic.Uint64
	channelMap    map[uint64]*Channel
}

func newChannelManager() *ChannelManager {
	return &ChannelManager{
		nextChannelID: &atomic.Uint64{},
		channelMap:    make(map[uint64]*Channel),
	}
}

func (channelManager *ChannelManager) NextChannelID() uint64 {
	return channelManager.nextChannelID.Add(1)
}

func (channelManager *ChannelManager) AddChannel(ctx context.Context, channel *Channel) (*Channel, bool) {
	channelManager.mutex.Lock()
	defer channelManager.mutex.Unlock()

	if _, ok := channelManager.channelMap[channel.ChannelID()]; ok {
		return nil, false
	}

	channelManager.channelMap[channel.ChannelID()] = channel

	return channel, true
}

func (channelManager *ChannelManager) GetChannel(ctx context.Context, channelID uint64) *Channel {
	channelManager.mutex.Lock()
	defer channelManager.mutex.Unlock()

	return channelManager.channelMap[channelID]
}

func (channelManager *ChannelManager) RemoveChannel(ctx context.Context, channel *Channel) {
	channelManager.mutex.Lock()
	defer channelManager.mutex.Unlock()

	delete(channelManager.channelMap, channel.ChannelID())

}
