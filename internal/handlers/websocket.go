package handlers

import (
	"net/http"

	"gosocket/internal/hub"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type WSHandler struct {
	hub *hub.Hub
}

func NewWSHandler(h *hub.Hub) *WSHandler {
	return &WSHandler{hub: h}
}

// GET /ws — upgrade to WebSocket.
//
// Client messages:
//
//	{"type":"subscribe",   "video_id":"<id>"}
//	{"type":"unsubscribe", "video_id":"<id>"}
//	{"type":"playback_event", "video_id":"<id>", "payload":{"event":"play","position":12.5}}
//
// Server messages (broadcast to all subscribers of video_id):
//
//	{"type":"upload_progress",    "video_id":"<id>", "payload":{"percent":45.2,"bytes":...,"total":...}}
//	{"type":"transcoding_started","video_id":"<id>", "payload":{"duration":120.0}}
//	{"type":"transcoding_update", "video_id":"<id>", "payload":{"resolution":"720p","percent":30.5}}
//	{"type":"transcoding_complete","video_id":"<id>","payload":{"resolution":"720p"}}
//	{"type":"video_ready",        "video_id":"<id>", "payload":{"resolutions":["360p","720p"],"duration":120.0}}
//	{"type":"playback_event",     "video_id":"<id>", "payload":{"event":"play","position":12.5}}
func (wh *WSHandler) Handle(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := wh.hub.NewClient(conn)
	go client.WritePump()
	client.ReadPump(wh.onMessage)
}

func (wh *WSHandler) onMessage(c *hub.Client, msg hub.IncomingMsg) {
	switch msg.Type {
	case "subscribe":
		c.Subscribe(msg.VideoID)
	case "unsubscribe":
		c.Unsubscribe(msg.VideoID)
	case "playback_event":
		// Relay to all other clients watching the same video.
		wh.hub.Broadcast(msg.VideoID, "playback_event", msg.Payload, c)
	}
}
