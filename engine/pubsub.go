package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

const MarketUpdatesChannel = "market_updates"

type Publisher struct {
	client *redis.Client
}

func NewPublisher(addr string) *Publisher {
	if addr == "" {
		return nil
	}

	return &Publisher{
		client: redis.NewClient(&redis.Options{Addr: addr}),
	}
}

func (publisher *Publisher) Close() error {
	if publisher == nil {
		return nil
	}

	return publisher.client.Close()
}

func (publisher *Publisher) PublishSnapshot(ctx context.Context, snapshot OrderBookSnapshot) error {
	if publisher == nil {
		return nil
	}

	message, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return publisher.client.Publish(ctx, MarketUpdatesChannel, message).Err()
}

type Engine struct {
	book      *OrderBook
	publisher *Publisher
	ctx       context.Context
}

func NewEngine(ctx context.Context, redisAddr string) *Engine {
	return &Engine{
		book:      NewOrderBook(),
		publisher: NewPublisher(redisAddr),
		ctx:       ctx,
	}
}

func (engine *Engine) Close() error {
	if engine.publisher == nil {
		return nil
	}

	return engine.publisher.Close()
}

func (engine *Engine) AddOrder(order Order) OrderBookSnapshot {
	snapshot := engine.book.AddOrder(order)
	if err := engine.publisher.PublishSnapshot(engine.ctx, snapshot); err != nil {
		log.Printf("failed to publish snapshot: %v", err)
	}

	return snapshot
}

func (engine *Engine) Snapshot() OrderBookSnapshot {
	return engine.book.Snapshot()
}

func orderCreatedAtNow() time.Time {
	return time.Now().UTC()
}
