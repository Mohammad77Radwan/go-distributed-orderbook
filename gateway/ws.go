package gateway

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"quantum-engine/engine"

	"github.com/go-redis/redis/v8"
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
	hub         *Hub
	upgrader    websocket.Upgrader
	redisClient *redis.Client
	ctx         context.Context
}

func NewServer(ctx context.Context, redisAddr string) *Server {
	return &Server{
		hub: NewHub(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		redisClient: redis.NewClient(&redis.Options{Addr: redisAddr}),
		ctx:         ctx,
	}
}

func (server *Server) Close() error {
	return server.redisClient.Close()
}

func (server *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	client := server.hub.addClient(conn)
	go server.readPump(client)
}

func (server *Server) StartRedisBridge() {
	go func() {
		subscriber := server.redisClient.Subscribe(server.ctx, engine.MarketUpdatesChannel)
		defer subscriber.Close()

		channel := subscriber.Channel()
		for {
			select {
			case <-server.ctx.Done():
				return
			case message, ok := <-channel:
				if !ok {
					return
				}
				server.hub.broadcast([]byte(message.Payload))
			}
		}
	}()
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
