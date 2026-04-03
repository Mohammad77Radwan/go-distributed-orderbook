package engine

import (
	"strconv"
	"testing"
	"time"
)

func TestAddOrderSortsBidsByPriceThenTime(t *testing.T) {
	book := NewOrderBook()
	now := time.Now().UTC()

	book.AddOrder(Order{ID: "bid-1", Type: BuyOrder, Price: 100.0, Quantity: 10, CreatedAt: now.Add(2 * time.Second)})
	book.AddOrder(Order{ID: "bid-2", Type: BuyOrder, Price: 101.5, Quantity: 10, CreatedAt: now.Add(3 * time.Second)})
	book.AddOrder(Order{ID: "bid-3", Type: BuyOrder, Price: 100.0, Quantity: 10, CreatedAt: now.Add(1 * time.Second)})

	snapshot := book.Snapshot()
	if len(snapshot.Bids) != 3 {
		t.Fatalf("expected 3 bids, got %d", len(snapshot.Bids))
	}

	if snapshot.Bids[0].ID != "bid-2" {
		t.Fatalf("expected highest priced bid first, got %s", snapshot.Bids[0].ID)
	}

	if snapshot.Bids[1].ID != "bid-3" || snapshot.Bids[2].ID != "bid-1" {
		t.Fatalf("expected time-priority order for equal prices, got %s then %s", snapshot.Bids[1].ID, snapshot.Bids[2].ID)
	}
}

func TestAddOrderSortsAsksByPriceThenTime(t *testing.T) {
	book := NewOrderBook()
	now := time.Now().UTC()

	book.AddOrder(Order{ID: "ask-1", Type: SellOrder, Price: 100.0, Quantity: 10, CreatedAt: now.Add(2 * time.Second)})
	book.AddOrder(Order{ID: "ask-2", Type: SellOrder, Price: 99.5, Quantity: 10, CreatedAt: now.Add(3 * time.Second)})
	book.AddOrder(Order{ID: "ask-3", Type: SellOrder, Price: 100.0, Quantity: 10, CreatedAt: now.Add(1 * time.Second)})

	snapshot := book.Snapshot()
	if len(snapshot.Asks) != 3 {
		t.Fatalf("expected 3 asks, got %d", len(snapshot.Asks))
	}

	if snapshot.Asks[0].ID != "ask-2" {
		t.Fatalf("expected lowest priced ask first, got %s", snapshot.Asks[0].ID)
	}

	if snapshot.Asks[1].ID != "ask-3" || snapshot.Asks[2].ID != "ask-1" {
		t.Fatalf("expected time-priority order for equal prices, got %s then %s", snapshot.Asks[1].ID, snapshot.Asks[2].ID)
	}
}

func TestSnapshotTruncatesToTop10PerSide(t *testing.T) {
	book := NewOrderBook()
	now := time.Now().UTC()

	for i := 0; i < 15; i++ {
		book.AddOrder(Order{
			ID:        "bid-" + strconv.Itoa(i),
			Type:      BuyOrder,
			Price:     100 + float64(i),
			Quantity:  1,
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		})
		book.AddOrder(Order{
			ID:        "ask-" + strconv.Itoa(i),
			Type:      SellOrder,
			Price:     100 + float64(i),
			Quantity:  1,
			CreatedAt: now.Add(time.Duration(i) * time.Millisecond),
		})
	}

	snapshot := book.Snapshot()
	if len(snapshot.Bids) != 10 {
		t.Fatalf("expected 10 bids in snapshot, got %d", len(snapshot.Bids))
	}
	if len(snapshot.Asks) != 10 {
		t.Fatalf("expected 10 asks in snapshot, got %d", len(snapshot.Asks))
	}
}

func TestAddOrderSetsCreatedAtWhenMissing(t *testing.T) {
	book := NewOrderBook()

	snapshot := book.AddOrder(Order{ID: "missing-created-at", Type: BuyOrder, Price: 100, Quantity: 2})
	if len(snapshot.Bids) != 1 {
		t.Fatalf("expected 1 bid, got %d", len(snapshot.Bids))
	}

	if snapshot.Bids[0].CreatedAt.IsZero() {
		t.Fatalf("expected createdAt to be auto-populated")
	}
}
