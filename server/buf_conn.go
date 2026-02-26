package server

import (
	"bufio"
	"net"
)

type BuferConn struct {
	conn   *net.TCPConn
	reader *bufio.Reader
	writer *bufio.Writer
}

func NewBuferConn(conn net.Conn) *BuferConn {

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	return &BuferConn{
		conn:   conn.(*net.TCPConn),
		reader: reader,
		writer: writer,
	}
}

func (buf *BuferConn) GetReader() *bufio.Reader {
	return buf.reader
}

func (buf *BuferConn) GetWriter() *bufio.Writer {
	return buf.writer
}

func (buf *BuferConn) Read(p []byte) (n int, err error) {
	return buf.reader.Read(p)
}

func (buf *BuferConn) Write(p []byte) (nn int, err error) {
	return buf.writer.Write(p)
}

func (buf *BuferConn) CloseWrite() error {
	buf.Flush()
	return buf.conn.CloseWrite()
}

func (buf *BuferConn) Close() error {
	buf.Flush()
	return buf.conn.Close()
}

func (buf *BuferConn) Flush() error {
	return buf.writer.Flush()
}
