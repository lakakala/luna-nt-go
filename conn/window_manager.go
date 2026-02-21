package conn

import "sync"

const (
	BATCH_SIZE  = 1024
	WINDOW_SIZE = 1024
)

type ChannelSendWindowManager struct {
	mutex          *sync.Mutex
	cond           *sync.Cond
	currWindowSize uint64
	windowSize     uint64
}

func NewChannelSendWindowManager(windowSize uint64) *ChannelSendWindowManager {
	mutex := &sync.Mutex{}
	return &ChannelSendWindowManager{
		mutex:          mutex,
		cond:           sync.NewCond(mutex),
		currWindowSize: 0,
		windowSize:     windowSize,
	}
}

func (c *ChannelSendWindowManager) Acquire() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.currWindowSize >= c.windowSize {
		c.cond.Wait()
	}

	c.currWindowSize += 1
}

func (c *ChannelSendWindowManager) Release(ackWindowSize uint64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.currWindowSize -= ackWindowSize
	c.cond.Broadcast()
}

type ChannelRecvWindowManager struct {
	mutex          *sync.Mutex
	currWindowSize uint64
	windowSize     uint64
}

func NewChannelRecvWindowManager(windowSize uint64) *ChannelRecvWindowManager {
	mutex := &sync.Mutex{}
	return &ChannelRecvWindowManager{
		mutex:          mutex,
		currWindowSize: 0,
		windowSize:     windowSize,
	}
}

func (c *ChannelRecvWindowManager) Release(minAckWindowSize uint64) uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if minAckWindowSize == 0 {
		minAckWindowSize = c.windowSize / 2
	}

	if c.currWindowSize < minAckWindowSize {
		return 0
	}

	ackWindownSize := c.currWindowSize
	c.currWindowSize = 0

	return ackWindownSize
}
