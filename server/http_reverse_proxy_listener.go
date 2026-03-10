package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type HttpReverseProxyListener struct {
	id        uint64
	localAddr string
	bindAddr  string
	listener  net.Listener
	client    *Client
}

var _ ClientListener = (*HttpReverseProxyListener)(nil)

func newHttpReverseProxyListener(id uint64, localAddr, bindAddr string, client *Client) *HttpReverseProxyListener {
	return &HttpReverseProxyListener{
		id:        id,
		localAddr: localAddr,
		bindAddr:  bindAddr,
		client:    client,
	}
}

// ID implements [ClientListener].
func (h *HttpReverseProxyListener) ID() uint64 {
	return h.id
}

// Start implements [ClientListener].
func (h *HttpReverseProxyListener) Start(ctx context.Context) {
	if err := h.doStart(ctx); err != nil {
		log.CtxWarnf(ctx, "HttpReverseProxyListener.doStart failed err %s", err)
	}
}

var hotToHotHeaders = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"TE":                  {},
	"Trailers":            {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func (h *HttpReverseProxyListener) doStart(ctx context.Context) error {

	listener, err := net.Listen("tcp", h.bindAddr)
	if err != nil {
		return err
	}

	h.listener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		log.CtxInfof(ctx, "HttpReverseProxyListener accept conn remoteAddr %s localAddr %s", conn.RemoteAddr().String(), conn.LocalAddr().String())

		go func() {
			if err := h.handleConn(ctx, conn); err != nil {
				log.CtxWarnf(ctx, "HttpReverseProxyListener.handleConn failed err %s", err)
			}
		}()
	}

	if err := h.cleanup(ctx); err != nil {
		log.CtxWarnf(ctx, "HttpReverseProxyListener.cleanup failed err %s", err)
	}

	return nil
}

func (h *HttpReverseProxyListener) handleConn(ctx context.Context, conn net.Conn) error {
	bufConn := NewBuferConn(conn)

	for {
		req, err := http.ReadRequest(bufConn.GetReader())
		if err != nil {
			return err
		}

		connectionHeaderVal := req.Header.Get("Connection")
		if connectionHeaderVal != "" {
			connectionHeaderVals := strings.Split(connectionHeaderVal, ",")
			for _, val := range connectionHeaderVals {
				val = strings.TrimSpace(val)
				req.Header.Del(val)

				log.CtxInfof(ctx, "HttpReverseProxyListener handleConn del header %s", val)
			}
		}

		for hotToHotHeader := range hotToHotHeaders {
			req.Header.Del(hotToHotHeader)
			log.CtxInfof(ctx, "HttpReverseProxyListener handleConn del header %s", hotToHotHeader)
		}

		req.Host = h.localAddr

		channel, err := h.client.connect(ctx, h.localAddr)
		if err != nil {
			return err
		}

		channelReader := bufio.NewReader(channel)

		err = req.Write(channel)
		if err != nil {
			return err
		}

		resp, err := http.ReadResponse(channelReader, req)
		if err != nil {
			return err
		}

		err = resp.Write(bufConn)
		if err != nil {
			return err
		}

		if err := bufConn.Flush(); err != nil {
			return err
		}
	}
}

func (h *HttpReverseProxyListener) cleanup(ctx context.Context) error {
	if h.listener != nil {
		defer func() {
			h.listener = nil
		}()
		return h.listener.Close()
	}

	return nil
}

// Close implements [ClientListener].
func (h *HttpReverseProxyListener) Close(ctx context.Context) {
	_ = h.cleanup(ctx)
}
