package conn

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/lakakala/luna-nt-go/message"
	"github.com/lakakala/luna-nt-go/utils/log"
)

type ReadBuf struct {
	ctx   context.Context
	mutex *sync.Mutex
	cond  *sync.Cond

	conn *Conn

	channelID uint64

	buf []byte
	cap int
	r   int // 读指针 指向最后一次读位置
	// remoteR int // 对端读指针 指向最后一次对端读位置
	w      int // 写指针 指向下次写位置
	closed bool
}

func NewReadBuf(ctx context.Context, conn *Conn, channelID uint64, cap int) *ReadBuf {

	mutex := &sync.Mutex{}
	return &ReadBuf{
		ctx: ctx,

		mutex: mutex,
		cond:  sync.NewCond(mutex),

		conn: conn,

		channelID: channelID,

		buf: make([]byte, cap),
		cap: cap,
		r:   0,
		w:   0,
	}
}

func (readBuf *ReadBuf) Read(b []byte) (int, error) {
	n, err := readBuf.doRead(b)
	readBuf.check()
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (readBuf *ReadBuf) check() {
	readBuf.mutex.Lock()
	defer readBuf.mutex.Unlock()

	readBuf.conn.Send(readBuf.ctx, message.MakeChanelWindowUpdateNoti(readBuf.channelID, 0, uint64(readBuf.r)))
}

func (readBuf *ReadBuf) doRead(b []byte) (int, error) {
	readBuf.mutex.Lock()
	defer readBuf.mutex.Unlock()

	for {

		if readBuf.readRemaining() == 0 {
			if readBuf.closed {
				return 0, io.EOF
			}

			readBuf.cond.Wait()
			continue
		}

		readIndex := readBuf.r % readBuf.cap
		writeIndex := readBuf.w % readBuf.cap

		if readIndex < writeIndex {

			n := copy(b, readBuf.buf[readIndex:writeIndex])
			readBuf.r += n

			return n, nil
		} else {

			n1 := copy(b, readBuf.buf[readIndex:readBuf.cap])
			readBuf.r += n1

			if n1 == len(b) {
				return n1, nil
			}

			n2 := copy(b[n1:], readBuf.buf[0:writeIndex])
			readBuf.r += n2

			return n1 + n2, nil
		}
	}
}

func (readBuf *ReadBuf) put(ctx context.Context, code uint32, b []byte) error {

	if err := func() error {
		readBuf.mutex.Lock()
		defer readBuf.mutex.Unlock()

		if err := readBuf.doPut(ctx, code, b); err != nil {
			readBuf.cond.Broadcast()
			return err
		}

		readBuf.cond.Broadcast()
		return nil
	}(); err != nil {
		log.CtxInfof(ctx, "ReadBuf.put failed err %s", err)
		return err
	}

	return nil
}

func (readBuf *ReadBuf) doPut(ctx context.Context, code uint32, b []byte) error {
	if code != 0 {

		log.CtxInfof(readBuf.ctx, "ReadBuf.doPut channelID %d close", readBuf.channelID)

		readBuf.closed = true
		return nil
	}

	if readBuf.closed {
		return errors.New("readBuf.put readBuf is close")
	}

	writeRemgin := readBuf.writeRemaining()

	if writeRemgin < len(b) {
		return errors.New("ReadBuf.put writeRemgin < len(b)")
	}

	readIndex := readBuf.r % readBuf.cap
	writeIndex := readBuf.w % readBuf.cap

	if writeIndex < readIndex {
		n := copy(readBuf.buf[writeIndex:readIndex], b)

		readBuf.w += n

		return nil
	} else {
		n1 := copy(readBuf.buf[writeIndex:], b)
		readBuf.w += n1

		if n1 == len(b) {
			return nil
		}

		n2 := copy(readBuf.buf[0:readIndex], b[n1:])

		if n1+n2 != len(b) {
			return errors.New("ReadBuf.doPut n1+n2 != len(b)")
		}

		readBuf.w += n2

		return nil
	}
}

func (readBuf *ReadBuf) WriteRemaining() int {
	readBuf.mutex.Lock()
	defer readBuf.mutex.Unlock()

	return readBuf.writeRemaining()
}

func (readBuf *ReadBuf) writeRemaining() int {

	return readBuf.cap - readBuf.readRemaining()
}

func (readBuf *ReadBuf) readRemaining() int {

	return readBuf.w - readBuf.r
}

func (readBuf *ReadBuf) Close() {

	log.CtxInfof(readBuf.ctx, "ReadBuf.Close channelID %d close", readBuf.channelID)

	func() {
		readBuf.mutex.Lock()
		defer readBuf.mutex.Unlock()

		readBuf.closed = true
		readBuf.cond.Broadcast()
	}()

	readBuf.conn.Send(readBuf.ctx, message.MakeChanelWindowUpdateNoti(readBuf.channelID, 1, uint64(readBuf.r)))
}

type WriteBuf struct {
	ctx context.Context

	channelID uint64

	mutex *sync.Mutex
	cond  *sync.Cond

	conn   *Conn
	cap    int
	w      int
	r      int // 读指针
	closed bool
}

func newWriteBuf(ctx context.Context, channelID uint64, conn *Conn, cap int) *WriteBuf {

	mutex := &sync.Mutex{}
	return &WriteBuf{
		ctx:       ctx,
		channelID: channelID,

		mutex: mutex,
		cond:  sync.NewCond(mutex),

		conn: conn,
		cap:  cap,
	}
}

func (writeBuf *WriteBuf) Write(b []byte) (int, error) {

	totalWrite := 0
	for len(b) > 0 {
		n, err := writeBuf.doWrite(b)
		totalWrite += n
		if err != nil {
			return totalWrite, err
		}

		b = b[n:]
	}

	return totalWrite, nil
}

func (writeBuf *WriteBuf) doWrite(b []byte) (int, error) {
	writeBuf.mutex.Lock()
	defer writeBuf.mutex.Unlock()

	for {
		if writeBuf.closed {
			return 0, errors.New("WriteBuf.Write writeBuf is close")
		}

		if len(b) == 0 {
			return 0, nil
		}

		writeRemaining := writeBuf.writeRemaining()

		if writeRemaining == 0 {
			writeBuf.cond.Wait()
			continue
		}

		sendLen := len(b)
		if sendLen > writeRemaining {
			sendLen = writeRemaining
		}

		sendData := make([]byte, sendLen)

		copy(sendData, b)

		_, err := writeBuf.conn.Send(writeBuf.ctx, message.MakeDataNoti(writeBuf.channelID, 0, sendData))
		if err != nil {
			return 0, err
		}

		writeBuf.w += len(sendData)

		return len(sendData), nil
	}
}

func (writeBuf *WriteBuf) ack(ctx context.Context, code uint32, readIndex int64) {
	writeBuf.mutex.Lock()
	defer writeBuf.mutex.Unlock()

	writeBuf.r = int(readIndex)

	if code != 0 {
		log.CtxInfof(ctx, "WriteBuf.Ack channelID %d close", writeBuf.channelID)
		writeBuf.closed = true
	}

	writeBuf.cond.Broadcast()
}

func (writeBuf *WriteBuf) Close() error {

	log.CtxInfof(writeBuf.ctx, "WriteBuf.Close channelID %d close", writeBuf.channelID)

	func() {
		writeBuf.mutex.Lock()
		defer writeBuf.mutex.Unlock()

		writeBuf.closed = true
		writeBuf.cond.Broadcast()
	}()

	_, err := writeBuf.conn.Send(writeBuf.ctx, message.MakeDataNoti(writeBuf.channelID, 1, nil))
	if err != nil {
		return err
	}

	return nil
}

func (writeBuf *WriteBuf) WriteRemaining() int {
	writeBuf.mutex.Lock()
	defer writeBuf.mutex.Unlock()

	return writeBuf.writeRemaining()
}

func (writeBuf *WriteBuf) writeRemaining() int {
	return writeBuf.cap - writeBuf.readRemaining()
}

func (writeBuf *WriteBuf) readRemaining() int {
	return writeBuf.w - writeBuf.r
}

type Channel struct {
	conn *Conn

	readBuf  *ReadBuf
	writeBuf *WriteBuf

	channelID uint64

	windowSize uint64
}

func newChannel(ctx context.Context, channelID uint64, conn *Conn, windowSize uint64) *Channel {
	return &Channel{
		conn: conn,

		channelID: channelID,
		readBuf:   NewReadBuf(ctx, conn, channelID, int(windowSize)),
		writeBuf:  newWriteBuf(ctx, channelID, conn, int(windowSize)),
	}
}

func (channel *Channel) put(ctx context.Context, code uint32, b []byte) {
	channel.readBuf.put(ctx, code, b)
}

func (channel *Channel) Ack(ctx context.Context, code uint32, windowSize int64) {
	channel.writeBuf.ack(ctx, code, windowSize)
}

func (channel *Channel) Read(b []byte) (n int, err error) {
	return channel.readBuf.Read(b)
}

func (channel *Channel) Write(b []byte) (n int, err error) {
	return channel.writeBuf.Write(b)
}

func (channel *Channel) CloseRead() error {
	channel.readBuf.Close()
	return nil
}

func (channel *Channel) CloseWrite() error {
	channel.writeBuf.Close()
	return nil
}

func (channel *Channel) Close(ctx context.Context) error {
	if err := channel.CloseRead(); err != nil {
		return err
	}

	if err := channel.CloseWrite(); err != nil {
		return err
	}

	channel.conn.channelManager.RemoveChannel(ctx, channel.channelID)

	return nil
}

func (channel *Channel) ChannelID() uint64 {
	return channel.channelID
}
