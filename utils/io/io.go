package io

import (
	"context"
	"errors"
	"io"
)

var (
	errInvalidWrite = errors.New("invalid write result")
)

func CtxCopy(ctx context.Context, dst io.Writer, src io.Reader) (written int64, err error) {
	buf := make([]byte, 3*1024)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errInvalidWrite
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}

	}

	return written, err
}
