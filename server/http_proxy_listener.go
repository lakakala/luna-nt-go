package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/lakakala/luna-nt-go/utils/log"
)

type HttpProxyListener struct {
	id                    uint64
	bindAddr              string
	client                *Client
	clientListenerManager *ClientListenerManager
	listener              net.Listener
	allowHostMap          map[string]struct{}
}

func newHttpProxyListener(id uint64, bindAddr string, allowHostList []string, client *Client,
	clientListenerManager *ClientListenerManager) ClientListener {

	allowHostMap := make(map[string]struct{})
	for _, alloallowHost := range allowHostList {
		allowHostMap[alloallowHost] = struct{}{}
	}

	return &HttpProxyListener{
		id:                    id,
		bindAddr:              bindAddr,
		allowHostMap:          allowHostMap,
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

	h.listener = listener

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

	bufConn := NewBuferConn(conn)

	for {
		req, err := http.ReadRequest(bufConn.GetReader())
		if err != nil {
			return err
		}

		proxyAddr := req.Host

		if _, ok := h.allowHostMap[proxyAddr]; !ok {
			return errors.New(fmt.Sprintf("proxyAddr %s not allow", proxyAddr))
		}

		if req.Method == http.MethodConnect {
			if err := func() error {
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

				if err := resp.Write(bufConn); err != nil {
					return err
				}

				if err := bufConn.Flush(); err != nil {
					return err
				}

				channel, err := h.client.connect(ctx, proxyAddr)
				if err != nil {
					return err
				}

				waitGroup := sync.WaitGroup{}
				waitGroup.Add(2)
				go func() {
					buf := make([]byte, 10)
					for {
						readN, readErr := channel.Read(buf)

						_, writeErr := bufConn.Write(buf[:readN])

						bufConn.Flush()
						if readErr != nil {
							break
						}

						if writeErr != nil {
							break
						}
					}

					waitGroup.Done()
				}()

				go func() {
					buf := make([]byte, 1024)

					for {
						readN, readErr := bufConn.Read(buf)

						_, writeErr := channel.Write(buf[:readN])

						if readErr != nil {
							break
						}

						if writeErr != nil {
							break
						}
					}

					waitGroup.Done()
				}()
				// break
				waitGroup.Wait()

				channel.Close()
				return nil
			}(); err != nil {
				log.CtxWarnf(ctx, "HttpProxyListener.handleConn failed err %s", err)
			}

			log.CtxInfof(ctx, "HttpProxyListener handleConn connect to %s end", proxyAddr)
			break
		}

		req.Header.Set("Proxy-Connection", "Connection")

		proxyConn, err := h.client.connect(ctx, proxyAddr)
		if err != nil {
			return err
		}

		proxyReader := bufio.NewReader(proxyConn)

		err = func() error {
			url := req.URL

			newUrl, err := url.Parse(url.Path + "?" + url.RawQuery + "#" + url.RawFragment)
			if err != nil {
				return err
			}

			log.CtxInfof(ctx, "HttpProxyListener.handleConn req.URL %s newUrl %s", req.URL, newUrl)

			req.URL = newUrl

			if err := req.Write(proxyConn); err != nil {
				return err
			}

			resp, err := http.ReadResponse(proxyReader, req)
			if err != nil {
				return err
			}

			if err := resp.Write(bufConn); err != nil {
				return err
			}

			bufConn.Flush()

			return nil
		}()

		proxyConn.Close()

		if err != nil {
			log.CtxWarnf(ctx, "HttpProxyListener.handleConn failed err %s", err)
			break
		}
	}

	bufConn.Close()
	return nil
}

// Close implements [ClientListener].
func (h *HttpProxyListener) Close(ctx context.Context) {
	h.listener.Close()

	h.clientListenerManager.RemoveListener(h.ID())
}

var _ ClientListener = (*HttpProxyListener)(nil)
