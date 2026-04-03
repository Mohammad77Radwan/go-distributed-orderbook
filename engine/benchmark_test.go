package engine

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func BenchmarkOrderBookAddOrderSingleThreaded(b *testing.B) {
	book := NewOrderBook()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if i > 0 && i%2000 == 0 {
			book = NewOrderBook()
		}

		orderType := BuyOrder
		if i%2 == 0 {
			orderType = SellOrder
		}

		book.AddOrder(Order{
			ID:        strconv.Itoa(i),
			Type:      orderType,
			Price:     99 + float64(i%300)/100,
			Quantity:  1 + (i % 1000),
			CreatedAt: time.Unix(0, int64(i)),
		})
	}
}

func BenchmarkOrderBookAddOrderParallel(b *testing.B) {
	book := NewOrderBook()
	var counter uint64
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddUint64(&counter, 1)
			orderType := BuyOrder
			if i%2 == 0 {
				orderType = SellOrder
			}

			book.AddOrder(Order{
				ID:        strconv.FormatUint(i, 10),
				Type:      orderType,
				Price:     99 + float64(i%300)/100,
				Quantity:  1 + int(i%1000),
				CreatedAt: time.Unix(0, int64(i)),
			})
		}
	})
}

func BenchmarkOrderBookSnapshotTop10(b *testing.B) {
	book := NewOrderBook()
	now := time.Now().UTC()

	for i := 0; i < 200; i++ {
		book.AddOrder(Order{ID: "bid-" + strconv.Itoa(i), Type: BuyOrder, Price: 100 + float64(i)/100, Quantity: 100, CreatedAt: now})
		book.AddOrder(Order{ID: "ask-" + strconv.Itoa(i), Type: SellOrder, Price: 100 + float64(i)/100, Quantity: 100, CreatedAt: now})
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = book.Snapshot()
	}
}
