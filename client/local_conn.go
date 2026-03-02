package client

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/lakakala/luna-nt-go/conn"
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
	localAddr string
	channel   *conn.Channel
}

func (lc *LocalConn) GetChannelID() uint64 {
	return lc.channel.ChannelID()
}

func NewLocalConn(ctx context.Context, channel *conn.Channel, localAddr string) (*LocalConn, error) {

	localConn := &LocalConn{
		localAddr: localAddr,
		channel:   channel,
	}

	return localConn, nil
}

func (lc *LocalConn) start(ctx context.Context) {
	if err := lc.doStart(ctx); err != nil {
		log.CtxErrorf(ctx, "LocalConn start failed err %s", err)
	}
}

func (lc *LocalConn) doStart(ctx context.Context) error {
	connection, err := net.DialTimeout("tcp", lc.localAddr, 3*time.Second)
	if err != nil {
		return err
	}

	tcpConn := connection.(*net.TCPConn)

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(2)
	go func() {
		_, err := io.Copy(lc.channel, tcpConn)
		if err != nil {
			log.CtxErrorf(ctx, "LocalConn  channelID %d -> localAddr %s copy data failed err %s", lc.channel.ChannelID(), lc.localAddr, err)
		}

		tcpConn.CloseRead()
		lc.channel.CloseWrite()
		waitGroup.Done()
	}()

	go func() {

		_, err := io.Copy(tcpConn, lc.channel)
		if err != nil {
			log.CtxErrorf(ctx, "LocalConn localAddr %s -> channelID %d copy data failed err %s", lc.localAddr, lc.channel.ChannelID(), err)
		}

		tcpConn.CloseWrite()
		lc.channel.CloseRead()
		waitGroup.Done()
	}()

	waitGroup.Wait()

	log.CtxInfof(ctx, "LocalConn channelID %d -> localAddr %s close success", lc.channel.ChannelID(), lc.localAddr)

	return nil
}

func (lc *LocalConn) close(ctx context.Context, msg string) {

}
