package server

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type HttpProxyListener struct {
	id                    uint64
	bindAddr              string
	client                *Client
	clientListenerManager *ClientListenerManager
}

func newHttpProxyListener(id uint64, bindAddr string, client *Client,
	clientListenerManager *ClientListenerManager) ClientListener {
	return &HttpProxyListener{
		id:                    id,
		bindAddr:              bindAddr,
		client:                client,
		clientListenerManager: clientListenerManager,
	}
}

// ID implements [ClientListener].
func (h *HttpProxyListener) ID() uint64 {
	return h.id
}

// Start implements [ClientListener].
func (h *HttpProxyListener) Start(ctx context.Context) {
	if err := h.doStart(ctx); err != nil {
		log.CtxWarnf(ctx, "HttpProxyListener.doStart failed err %s", err)
	}
}

func (h *HttpProxyListener) doStart(ctx context.Context) error {
	listener, err := net.Listen("tcp", h.bindAddr)
	if err != nil {
		return err
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.CtxWarnf(ctx, "")
			continue
		}

		go h.handleConn(ctx, conn)
	}
}

func (h *HttpProxyListener) handleConn(ctx context.Context, conn net.Conn) {
	if err := h.doHandleConn(ctx, conn); err != nil {
		log.CtxWarnf(ctx, "HttpProxyListener.doHandleConn failed err %s", err)
	}
}

func (h *HttpProxyListener) doHandleConn(ctx context.Context, conn net.Conn) error {

	readWriter := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	req, err := http.ReadRequest(readWriter.Reader)
	if err != nil {
		return err
	}

	proxyAddr := req.Host

	if req.Method != http.MethodConnect {
		return errors.New("")
	}

	resp := http.Response{
		Status:       http.StatusText(http.StatusOK),
		StatusCode:   http.StatusOK,
		Proto:        "HTTP/1.1",
		ProtoMajor:   1,
		ProtoMinor:   1,
		Header:       make(http.Header),
		Uncompressed: false,
		Trailer:      http.Header{},
		Request:      req,
	}

	if err := resp.Write(readWriter); err != nil {
		return err
	}

	if err := readWriter.Flush(); err != nil {
		return err
	}

	if err := h.client.connect(ctx, nil, proxyAddr); err != nil {
		return err
	}

	return nil
}

// Close implements [ClientListener].
func (h *HttpProxyListener) Close(ctx context.Context) {
	panic("unimplemented")
}

var _ ClientListener = (*HttpProxyListener)(nil)
