package conn_v2

import (
	"context"
	"net"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type ConnV2 struct {
	conn *net.TCPConn
}

func (c *ConnV2) Start(ctx context.Context) error {

	go c.doRead(ctx)
	go c.doWrite(ctx)

	return nil
}

func (c *ConnV2) doRead(ctx context.Context) {

	buf := make([]byte, 1024)
	for {

		n, err := c.conn.Read(buf)
		if err != nil {
			log.CtxErrorf(ctx, "ConnV2 doRead failed err %s", err)
			break
		}

		data := buf[:n]

	}
}

func (c *ConnV2) doWrite(ctx context.Context) {

}

func (c *ConnV2) Close(ctx context.Context) {
}

func (c *ConnV2) Accept(ctx context.Context) (*Channel, error) {
	return nil, nil
}

func (c *ConnV2) CreateChannel(ctx context.Context, priority uint64) (*Channel, error) {
	return nil, nil
}

type Channel struct {
	channelID uint64
	priority  uint64
}
