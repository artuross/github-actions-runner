package livelogs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"golang.org/x/net/proxy"
)

type Connection struct {
	mu            sync.Mutex
	conn          net.Conn
	rd            *bufio.Reader
	lastStepID    string
	lastLineIndex int
}

func NewConnection(ctx context.Context, connURL string, authToken string) (*Connection, error) {
	dialer := ws.Dialer{
		Header: ws.HandshakeHeaderHTTP{
			"Authorization": []string{fmt.Sprintf("Bearer %s", authToken)},
		},
		NetDial: proxy.Dial,
	}

	conn, reader, _, err := dialer.Dial(ctx, strings.ReplaceAll(connURL, "https://", "wss://"))
	if err != nil {
		return nil, fmt.Errorf("connect to WebSocket server: %w", err)
	}

	connection := Connection{
		conn: conn,
		rd:   reader,
	}

	time.Sleep(time.Second)

	// fmt.Println(ws.ReadHeader(reader))
	// fmt.Println(ws.ReadFrame(reader))

	return &connection, nil
}

func (c *Connection) Close() error {
	err := ws.WriteFrame(c.conn, ws.NewCloseFrame(nil))
	if err != nil {
		return fmt.Errorf("write close frame: %w", err)
	}

	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("close conn: %w", err)
	}

	return nil
}

func (c *Connection) SendLines(stepID string, lines []string) error {
	c.mu.Lock()

	if c.lastStepID != stepID {
		c.lastStepID = stepID
		c.lastLineIndex = 0
	}

	msg := Message{
		Count:     len(lines),
		StartLine: c.lastLineIndex + 1,
		StepID:    stepID,
		Value:     lines,
	}

	c.lastLineIndex += len(lines)

	c.mu.Unlock()

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if err := ws.WriteFrame(c.conn, ws.MaskFrame(ws.NewTextFrame(data))); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

type Message struct {
	Value     []string `json:"value"`
	StepID    string   `json:"stepId"`
	StartLine int      `json:"startLine"`
	Count     int      `json:"count"`
}
