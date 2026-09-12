package ws

import (
	"bytes"
	"sync"

	"github.com/gorilla/websocket"
)

// Bridge adapts a WebSocket connection to the newline-delimited JSON
// io.Reader/Writer expected by github.com/coder/acp-go-sdk.
type Bridge struct {
	conn *websocket.Conn

	readMu  sync.Mutex
	readBuf bytes.Buffer

	writeMu  sync.Mutex
	writeBuf bytes.Buffer
}

// NewBridge wraps conn with newline-delimited JSON framing.
func NewBridge(conn *websocket.Conn) *Bridge {
	return &Bridge{conn: conn}
}

func (b *Bridge) Read(p []byte) (int, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()

	for b.readBuf.Len() == 0 {
		_, data, err := b.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		_, _ = b.readBuf.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			_ = b.readBuf.WriteByte('\n')
		}
	}

	return b.readBuf.Read(p)
}

func (b *Bridge) Write(p []byte) (int, error) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	n, _ := b.writeBuf.Write(p)
	for {
		line := b.writeBuf.Bytes()
		newline := bytes.IndexByte(line, '\n')
		if newline < 0 {
			break
		}

		if err := b.conn.WriteMessage(websocket.TextMessage, line[:newline]); err != nil {
			return n, err
		}
		b.writeBuf.Next(newline + 1)
	}

	return n, nil
}

func (b *Bridge) Close() error {
	return b.conn.Close()
}
