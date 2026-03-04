package client

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lakakala/luna-nt-go/conn"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type ConnManager struct {
	mutex        sync.Mutex
	nextConnID   *atomic.Uint64
	localConnMap map[uint64]*LocalConn
}

func newConnManager() *ConnManager {
	return &ConnManager{
		mutex:        sync.Mutex{},
		nextConnID:   &atomic.Uint64{},
		localConnMap: make(map[uint64]*LocalConn),
	}
}

func (cm *ConnManager) NewConn(ctx context.Context, channel *conn.Channel, localAddr string) (*LocalConn, error) {

	localConnID := cm.nextConnID.Add(1)
	lc, err := NewLocalConn(ctx, channel, localAddr)
	if err != nil {
		return nil, err
	}

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	log.CtxInfof(ctx, "NewConn connID %d create success", localConnID)
	cm.localConnMap[localConnID] = lc

	return lc, nil
}

func (cm *ConnManager) CloseAll(ctx context.Context) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for connID, lc := range cm.localConnMap {
		lc.Close(ctx)
		delete(cm.localConnMap, connID)
	}
}

type LocalConn struct {
	localAddr string
	localConn net.Conn
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

	lc.localConn = connection

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

func (lc *LocalConn) Close(ctx context.Context) {
	lc.channel.Close()
	lc.localConn.Close()
}
