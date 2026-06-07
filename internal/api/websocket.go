package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ashleyfullero/scrapeowl/internal/runner"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local use
	},
}

// Hub manages all WebSocket connections for real-time broadcasting
type Hub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
	logger  *slog.Logger
}

// wsClient represents a connected WebSocket client
type wsClient struct {
	conn   *websocket.Conn
	send   chan []byte
	hub    *Hub
	filter string // Optional: filter by job name
}

// NewHub creates a new WebSocket hub
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients: make(map[*wsClient]bool),
		logger:  logger,
	}
}

// Register adds a client to the hub
func (h *Hub) register(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
}

// Unregister removes a client from the hub
func (h *Hub) unregister(c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
}

// Broadcast sends an event to all connected WebSocket clients
func (h *Hub) Broadcast(event runner.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("marshaling ws event", "err", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// Apply job name filter if set
		if client.filter != "" && event.JobName != "" && client.filter != event.JobName {
			continue
		}
		select {
		case client.send <- data:
		default:
			// Client send buffer full, skip
		}
	}
}

// ServeWS handles WebSocket upgrade and connection lifecycle
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("ws upgrade", "err", err)
		return
	}

	filter := r.URL.Query().Get("job")

	client := &wsClient{
		conn:   conn,
		send:   make(chan []byte, 256),
		hub:    h,
		filter: filter,
	}
	h.register(client)

	// Send welcome message
	welcome, _ := json.Marshal(map[string]interface{}{
		"type":      "connected",
		"timestamp": time.Now(),
		"message":   "Connected to ScrapeOwl real-time stream",
	})
	client.send <- welcome

	go client.writePump()
	go client.readPump()
}

// writePump pumps messages from the send channel to the WebSocket connection
func (c *wsClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump reads incoming messages (handles pong, close)
func (c *wsClient) readPump() {
	defer c.hub.unregister(c)

	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
