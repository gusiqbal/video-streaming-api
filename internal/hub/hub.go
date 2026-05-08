package hub

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// IncomingMsg is sent by the client to the server.
type IncomingMsg struct {
	Type    string          `json:"type"`
	VideoID string          `json:"video_id"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// OutgoingMsg is sent by the server to clients.
type OutgoingMsg struct {
	Type    string      `json:"type"`
	VideoID string      `json:"video_id"`
	Payload interface{} `json:"payload,omitempty"`
}

// Client represents a connected WebSocket client.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	subs map[string]bool
	mu   sync.RWMutex
}

type broadcastReq struct {
	videoID string
	data    []byte
	exclude *Client
}

// Hub manages all WebSocket clients and routes messages.
type Hub struct {
	clients    map[*Client]bool
	mu         sync.RWMutex
	broadcastC chan broadcastReq
	registerC  chan *Client
	unregC     chan *Client
}

func New() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcastC: make(chan broadcastReq, 512),
		registerC:  make(chan *Client),
		unregC:     make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.registerC:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()

		case c := <-h.unregC:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()

		case req := <-h.broadcastC:
			h.mu.RLock()
			for c := range h.clients {
				if c == req.exclude {
					continue
				}
				c.mu.RLock()
				subscribed := c.subs[req.videoID]
				c.mu.RUnlock()
				if !subscribed {
					continue
				}
				select {
				case c.send <- req.data:
				default:
					// Slow client; drop message rather than blocking the hub.
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all clients subscribed to videoID, except exclude (may be nil).
func (h *Hub) Broadcast(videoID, msgType string, payload interface{}, exclude *Client) {
	msg := OutgoingMsg{Type: msgType, VideoID: videoID, Payload: payload}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.broadcastC <- broadcastReq{videoID: videoID, data: data, exclude: exclude}
}

// NewClient registers a new WebSocket connection and returns its Client.
func (h *Hub) NewClient(conn *websocket.Conn) *Client {
	c := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
		subs: make(map[string]bool),
	}
	h.registerC <- c
	return c
}

// Subscribe adds videoID to the client's subscription set.
func (c *Client) Subscribe(videoID string) {
	c.mu.Lock()
	c.subs[videoID] = true
	c.mu.Unlock()
}

// Unsubscribe removes videoID from the client's subscription set.
func (c *Client) Unsubscribe(videoID string) {
	c.mu.Lock()
	delete(c.subs, videoID)
	c.mu.Unlock()
}

// WritePump drains the send channel and writes messages to the WebSocket.
func (c *Client) WritePump() {
	defer c.conn.Close()
	for data := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("ws write: %v", err)
			return
		}
	}
}

// ReadPump reads messages from the WebSocket and calls onMsg for each valid message.
func (c *Client) ReadPump(onMsg func(*Client, IncomingMsg)) {
	defer func() {
		c.hub.unregC <- c
		c.conn.Close()
	}()
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read: %v", err)
			}
			return
		}
		var msg IncomingMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		onMsg(c, msg)
	}
}
