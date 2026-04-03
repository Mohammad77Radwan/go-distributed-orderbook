package gateway

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"quantum-engine/engine"

	"github.com/gorilla/websocket"
)

const (
	clientSendBuffer   = 16
	maxWebSocketMessage = 8 * 1024
	readDeadline       = 60 * time.Second
	writeDeadline      = 10 * time.Second
	pingPeriod         = 54 * time.Second
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
	closed  bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func newClient(conn *websocket.Conn) *client {
	return &client{
		conn: conn,
		send: make(chan []byte, clientSendBuffer),
		done: make(chan struct{}),
	}
}

func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *client) enqueue(message []byte) bool {
	select {
	case <-c.done:
		return false
	default:
	}

	select {
	case c.send <- message:
		return true
	default:
	}

	select {
	case <-c.send:
	default:
	}

	select {
	case <-c.done:
		return false
	case c.send <- message:
		return true
	default:
		return false
	}
}

func (c *client) writePump(hub *Hub) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer hub.removeClient(c)

	for {
		select {
		case <-c.done:
			return
		case message, ok := <-c.send:
			if !ok {
				return
			}
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
				return
			}
			if err := c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(writeDeadline)); err != nil {
				return
			}
		}
	}
}

func (hub *Hub) addClient(conn *websocket.Conn) *client {
	wrapped := newClient(conn)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.closed {
		wrapped.close()
		return wrapped
	}
	hub.clients[wrapped] = struct{}{}
	return wrapped
}

func (hub *Hub) removeClient(target *client) {
	hub.mu.Lock()
	delete(hub.clients, target)
	hub.mu.Unlock()
	target.close()
}

func (hub *Hub) broadcast(message []byte) {
	hub.mu.RLock()
	clients := make([]*client, 0, len(hub.clients))
	for conn := range hub.clients {
		clients = append(clients, conn)
	}
	hub.mu.RUnlock()

	for _, target := range clients {
		if !target.enqueue(message) {
			hub.removeClient(target)
		}
	}
}

func (hub *Hub) closeAll() {
	hub.mu.Lock()
	hub.closed = true
	clients := make([]*client, 0, len(hub.clients))
	for conn := range hub.clients {
		clients = append(clients, conn)
	}
	hub.clients = make(map[*client]struct{})
	hub.mu.Unlock()

	for _, target := range clients {
		target.close()
	}
}

type Server struct {
	hub              *Hub
	upgrader         websocket.Upgrader
	ctx              context.Context
	snapshotProvider func() engine.OrderBookSnapshot
	allowedOrigins   map[string]struct{}
}

func NewServer(ctx context.Context, snapshotProvider func() engine.OrderBookSnapshot) *Server {
	server := &Server{
		hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return false },
		},
		ctx:              ctx,
		snapshotProvider: snapshotProvider,
		allowedOrigins:   parseAllowedOrigins(os.Getenv("WS_ALLOWED_ORIGINS")),
	}

	server.upgrader.CheckOrigin = server.isOriginAllowed
	return server
}

func (server *Server) Close() error {
	server.hub.closeAll()
	return nil
}

func (server *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !server.isOriginAllowed(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	conn, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	conn.SetReadLimit(maxWebSocketMessage)
	client := server.hub.addClient(conn)
	if client == nil {
		_ = conn.Close()
		return
	}

	client.conn.SetReadDeadline(time.Now().Add(readDeadline))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(readDeadline))
	})

	if server.snapshotProvider != nil {
		server.broadcastSnapshot(client, server.snapshotProvider())
	}

	go client.writePump(server.hub)
	go server.readPump(client)
}

func (server *Server) BroadcastSnapshot(target *client, snapshot engine.OrderBookSnapshot) {
	server.broadcastSnapshot(target, snapshot)
}

func (server *Server) BroadcastLiveSnapshot(snapshot engine.OrderBookSnapshot) {
	message, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("failed to encode websocket snapshot: %v", err)
		return
	}

	server.hub.broadcast(message)
}

func (server *Server) broadcastSnapshot(target *client, snapshot engine.OrderBookSnapshot) {
	message, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("failed to encode websocket snapshot: %v", err)
		return
	}

	if !target.enqueue(message) {
		server.hub.removeClient(target)
	}
}

func (server *Server) readPump(target *client) {
	defer server.hub.removeClient(target)
	for {
		if _, _, err := target.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func parseAllowedOrigins(raw string) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, origin := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

func (server *Server) isOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}

	if len(server.allowedOrigins) > 0 {
		_, ok := server.allowedOrigins[origin]
		return ok
	}

	parsedOrigin, err := url.Parse(origin)
	if err != nil {
		return false
	}

	requestHost := strings.ToLower(strings.TrimSpace(r.Host))
	originHost := strings.ToLower(strings.TrimSpace(parsedOrigin.Host))
	if originHost == requestHost {
		return true
	}

	return false
}