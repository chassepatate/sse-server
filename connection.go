package sse

import (
	"errors"
	"github.com/google/uuid"
	"net/http"
	"time"
)

var heartbeatMessage = Event{Event: "heartbeat"}.format()

type Connection struct {
	id string

	responseWriter http.ResponseWriter
	request        *http.Request
	flusher        http.Flusher

	heartbeatInterval time.Duration

	msg     chan []byte
	onClose func()
	closed  bool
}

func newConnection(w http.ResponseWriter, r *http.Request, heartbeatInterval time.Duration) (*Connection, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	return &Connection{
		id:                uuid.New().String(),
		responseWriter:    w,
		request:           r,
		flusher:           flusher,
		msg:               make(chan []byte),
		onClose:           func() {},
		heartbeatInterval: heartbeatInterval,
	}, nil
}

func (c *Connection) ID() string {
	return c.id
}

// Serve cannot be used to reopen a closed connection
func (c *Connection) Serve() error {
	if c.closed {
		return errors.New("can't serve closed connection")
	}
	defer c.Close()

	go c.handleHeartBeat()

writeLoop:
	for {
		select {
		case <-c.request.Context().Done():
			break writeLoop
		case msg, open := <-c.msg:
			if !open {
				return errors.New("msg chan closed")
			}
			_, err := c.responseWriter.Write(msg)
			if err != nil {
				return errors.New("write failed")
			}
			c.flusher.Flush()
		}
	}

	return nil
}

func (c *Connection) handleHeartBeat() {
	if c.heartbeatInterval <= 0 {
		return
	}

	heartbeats := time.NewTicker(c.heartbeatInterval)
	defer heartbeats.Stop()

	for {
		select {
		case <-heartbeats.C:
			c.msg <- heartbeatMessage
		case <-c.request.Context().Done():
			return
		}
	}
}

func (c *Connection) Closed() bool {
	return c.closed
}

func (c *Connection) Write(event Event) {
	c.msg <- event.format()
}

func (c *Connection) Close() {
	if c.closed {
		return
	}

	if c.onClose != nil {
		c.onClose()
	}

	close(c.msg)
	c.closed = true
}
