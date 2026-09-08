package livereload

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

// websocketGUID is the constant RFC 6455 appends to Sec-WebSocket-Key before
// hashing to produce the Sec-WebSocket-Accept response header.
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opcodeContinuation = 0x0
	opcodeText         = 0x1
	opcodeBinary       = 0x2
	opcodeClose        = 0x8
	opcodePing         = 0x9
	opcodePong         = 0xa
)

var errUnsupportedFrame = errors.New("livereload: unsupported websocket frame")

type conn struct {
	raw    net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	closed bool
}

func isWebSocketUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade") &&
		req.Header.Get("Sec-WebSocket-Key") != ""
}

func acceptWebSocket(w http.ResponseWriter, req *http.Request) (*conn, error) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("livereload: connection cannot be hijacked")
	}
	raw, buffered, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}

	sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + websocketGUID))
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	if _, err := raw.Write([]byte(response)); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return &conn{raw: raw, reader: buffered.Reader}, nil
}

func (c *conn) readText() (string, error) {
	for {
		opcode, payload, err := c.readFrame()
		if err != nil {
			return "", err
		}
		switch opcode {
		case opcodeText:
			return string(payload), nil
		case opcodePing:
			if err := c.writeFrame(opcodePong, payload); err != nil {
				return "", err
			}
		case opcodeClose:
			return "", io.EOF
		}
	}
}

func (c *conn) readFrame() (byte, []byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		return 0, nil, err
	}
	opcode := header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)

	switch length {
	case 126:
		var extended [2]byte
		if _, err := io.ReadFull(c.reader, extended[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err := io.ReadFull(c.reader, extended[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(extended[:])
	}
	if length > 1<<20 {
		return 0, nil, errUnsupportedFrame
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, mask[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, nil
}

func (c *conn) writeText(message string) error {
	return c.writeFrame(opcodeText, []byte(message))
}

func (c *conn) writeFrame(opcode byte, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return io.ErrClosedPipe
	}

	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, byte(length))
	case length <= 0xffff:
		header = append(header, 126, byte(length>>8), byte(length))
	default:
		header = append(header, 127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}
	if _, err := c.raw.Write(append(header, payload...)); err != nil {
		return err
	}
	return nil
}

func (c *conn) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.raw.Close()
}
