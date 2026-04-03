package engine

import "sort"

func NewOrderBook() *OrderBook {
	return &OrderBook{
		Bids: make([]Order, 0),
		Asks: make([]Order, 0),
	}
}

func (book *OrderBook) AddOrder(order Order) OrderBookSnapshot {
	book.Mu.Lock()
	defer book.Mu.Unlock()

	if order.CreatedAt.IsZero() {
		order.CreatedAt = orderCreatedAtNow()
	}

	switch order.Type {
	case BuyOrder:
		book.Bids = append(book.Bids, order)
		book.sortBidsLocked()
	case SellOrder:
		book.Asks = append(book.Asks, order)
		book.sortAsksLocked()
	default:
		return book.snapshotLocked()
	}

	return book.snapshotLocked()
}

func (book *OrderBook) Snapshot() OrderBookSnapshot {
	book.Mu.RLock()
	defer book.Mu.RUnlock()

	return book.snapshotLocked()
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
