package engine

import (
	"sort"
	"time"
)

const (
	maxOrderAge      = 4 * time.Second
	maxOrdersPerSide = 400
)

func NewOrderBook() *OrderBook {
	return &OrderBook{
		Bids: make([]Order, 0),
		Asks: make([]Order, 0),
	}
}

func (book *OrderBook) AddOrder(order Order) OrderBookSnapshot {
	book.Mu.Lock()
	defer book.Mu.Unlock()
	now := orderCreatedAtNow()
	book.pruneStaleLocked(now)

	if order.CreatedAt.IsZero() {
		order.CreatedAt = now
	}

	switch order.Type {
	case BuyOrder:
		book.Bids = append(book.Bids, order)
		book.sortBidsLocked()
		if len(book.Bids) > maxOrdersPerSide {
			book.Bids = book.Bids[:maxOrdersPerSide]
		}
	case SellOrder:
		book.Asks = append(book.Asks, order)
		book.sortAsksLocked()
		if len(book.Asks) > maxOrdersPerSide {
			book.Asks = book.Asks[:maxOrdersPerSide]
		}
	default:
		return book.snapshotLocked()
	}

	return book.snapshotLocked()
}

func (book *OrderBook) Snapshot() OrderBookSnapshot {
	book.Mu.Lock()
	defer book.Mu.Unlock()
	book.pruneStaleLocked(orderCreatedAtNow())

	return book.snapshotLocked()
}

func (book *OrderBook) pruneStaleLocked(now time.Time) {
	cutoff := now.Add(-maxOrderAge)
	book.Bids = filterRecentOrders(book.Bids, cutoff)
	book.Asks = filterRecentOrders(book.Asks, cutoff)
}

func filterRecentOrders(orders []Order, cutoff time.Time) []Order {
	filtered := orders[:0]
	for _, order := range orders {
		if order.CreatedAt.After(cutoff) {
			filtered = append(filtered, order)
		}
	}

	return filtered
}

func (book *OrderBook) snapshotLocked() OrderBookSnapshot {
	bids := make([]Order, len(book.Bids))
	copy(bids, book.Bids)
	if len(bids) > 10 {
		bids = bids[:10]
	}

	asks := make([]Order, len(book.Asks))
	copy(asks, book.Asks)
	if len(asks) > 10 {
		asks = asks[:10]
	}

	return OrderBookSnapshot{
		Bids:      bids,
		Asks:      asks,
		Timestamp: orderCreatedAtNow(),
	}
}

func (book *OrderBook) sortBidsLocked() {
	sort.SliceStable(book.Bids, func(i, j int) bool {
		if book.Bids[i].Price == book.Bids[j].Price {
			return book.Bids[i].CreatedAt.Before(book.Bids[j].CreatedAt)
		}
		return book.Bids[i].Price > book.Bids[j].Price
	})
}

func (book *OrderBook) sortAsksLocked() {
	sort.SliceStable(book.Asks, func(i, j int) bool {
		if book.Asks[i].Price == book.Asks[j].Price {
			return book.Asks[i].CreatedAt.Before(book.Asks[j].CreatedAt)
		}
		return book.Asks[i].Price < book.Asks[j].Price
	})
}
