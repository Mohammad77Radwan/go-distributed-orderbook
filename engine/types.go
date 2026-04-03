package engine

import (
	"sync"
	"time"
)

type OrderType string

const (
	BuyOrder  OrderType = "buy"
	SellOrder OrderType = "sell"
)

type Order struct {
	ID        string    `json:"id"`
	Type      OrderType `json:"type"`
	Price     float64   `json:"price"`
	Quantity  int       `json:"quantity"`
	CreatedAt time.Time `json:"createdAt"`
}

type OrderBook struct {
	Bids []Order
	Asks []Order
	Mu   sync.RWMutex
}

type OrderBookSnapshot struct {
	Bids      []Order   `json:"bids"`
	Asks      []Order   `json:"asks"`
	Timestamp time.Time `json:"timestamp"`
}
