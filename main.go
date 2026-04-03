package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"quantum-engine/engine"
	"quantum-engine/gateway"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	redisAddr := getenv("REDIS_ADDR", "")
	engineService := engine.NewEngine(ctx, redisAddr)
	defer func() {
		if err := engineService.Close(); err != nil {
			log.Printf("engine shutdown close failed: %v", err)
		}
	}()

	wsServer := gateway.NewServer(ctx, engineService.Snapshot)
	defer func() {
		if err := wsServer.Close(); err != nil {
			log.Printf("websocket server close failed: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsServer.HandleWebSocket)
	mux.HandleFunc("/snapshot", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(engineService.Snapshot()); err != nil {
			http.Error(w, "failed to encode snapshot", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/", newFrontendHandler())

	server := &http.Server{
		Addr:              ":" + getenv("PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go simulateTraffic(ctx, engineService, wsServer)

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("server listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func simulateTraffic(ctx context.Context, engineService *engine.Engine, wsServer *gateway.Server) {
	seeded := rand.New(rand.NewSource(time.Now().UnixNano()))
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			orderType := engine.BuyOrder
			if seeded.Intn(2) == 0 {
				orderType = engine.SellOrder
			}

			order := engine.Order{
				ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
				Type:      orderType,
				Price:     99.0 + seeded.Float64()*2.0,
				Quantity:  1 + seeded.Intn(1000),
				CreatedAt: time.Now().UTC(),
			}
			snapshot := engineService.AddOrder(order)
			wsServer.BroadcastLiveSnapshot(snapshot)
		}
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func newFrontendHandler() http.Handler {
	buildDir := filepath.Join("frontend", "build")
	indexPath := filepath.Join(buildDir, "index.html")
	staticServer := http.FileServer(http.Dir(buildDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(indexPath); err != nil {
			serveBuildMissingPage(w)
			return
		}

		relPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
		candidate := filepath.Join(buildDir, relPath)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			staticServer.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, indexPath)
	})
}

func serveBuildMissingPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = template.Must(template.New("missing-build").Parse(`<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>Frontend Build Missing</title>
	<style>
		body {
			margin: 0;
			font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
			background: #0b0f14;
			color: #d8e1eb;
			display: grid;
			place-items: center;
			min-height: 100vh;
		}
		main {
			max-width: 760px;
			padding: 24px;
			border: 1px solid #26313f;
			border-radius: 12px;
			background: #111823;
			box-shadow: 0 10px 30px rgba(0,0,0,0.35);
		}
		h1 { margin-top: 0; color: #8ee6a3; font-size: 1.4rem; }
		code { color: #93d5ff; }
		a { color: #93d5ff; }
		ul { line-height: 1.7; }
	</style>
</head>
<body>
	<main>
		<h1>Backend is running, but frontend build is missing</h1>
		<p>Build the Svelte app once, then refresh this page.</p>
		<ul>
			<li><code>cd frontend && npm install</code></li>
			<li><code>npm run build</code></li>
		</ul>
		<p>Health check: <a href="/healthz">/healthz</a></p>
	</main>
</body>
</html>`)).Execute(w, nil)
}
