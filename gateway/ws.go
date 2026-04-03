package gateway

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"quantum-engine/engine"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

type client struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (hub *Hub) addClient(conn *websocket.Conn) *client {
	wrapped := &client{conn: conn}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.clients[wrapped] = struct{}{}
	return wrapped
}

func (hub *Hub) removeClient(target *client) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if _, exists := hub.clients[target]; exists {
		delete(hub.clients, target)
		_ = target.conn.Close()
	}
}

func (hub *Hub) broadcast(message []byte) {
	hub.mu.RLock()
	clients := make([]*client, 0, len(hub.clients))
	for conn := range hub.clients {
		clients = append(clients, conn)
	}
	hub.mu.RUnlock()

	for _, target := range clients {
		target := target
		go func() {
			target.writeMu.Lock()
			defer target.writeMu.Unlock()
			if err := target.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				hub.removeClient(target)
				return
			}
		}()
	}
}

type Server struct {
	hub              *Hub
	upgrader         websocket.Upgrader
	ctx              context.Context
	snapshotProvider func() engine.OrderBookSnapshot
}

func NewServer(ctx context.Context, snapshotProvider func() engine.OrderBookSnapshot) *Server {
	return &Server{
		hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		ctx:              ctx,
		snapshotProvider: snapshotProvider,
	}
}

func (server *Server) Close() error {
	return nil
}

func (server *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	client := server.hub.addClient(conn)
	if server.snapshotProvider != nil {
		server.BroadcastSnapshot(server.snapshotProvider())
	}
	go server.readPump(client)
}

func (server *Server) BroadcastSnapshot(snapshot engine.OrderBookSnapshot) {
	message, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("failed to encode websocket snapshot: %v", err)
		return
	}

	server.hub.broadcast(message)
}

func (server *Server) readPump(target *client) {
	defer server.hub.removeClient(target)
	_ = target.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	target.conn.SetPongHandler(func(string) error {
		return target.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		if _, _, err := target.conn.ReadMessage(); err != nil {
			return
		}
	}
}
