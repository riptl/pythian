package schedule

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricBlockhashUpdates = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "solana",
		Name:      "blockhash_updates_total",
		Help:      "Number of block hash updates received",
	})
	metricSlotUpdates = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "solana",
		Name:      "slot_updates_total",
		Help:      "Number of slot updates received",
	})
	metricTxsSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "solana",
		Name:      "transactions_sent_total",
		Help:      "Number of Pyth transactions sent to Solana",
	}, []string{"pyth_publisher"})
	metricUpdatesDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "solana",
		Name:      "price_updates_dropped_total",
		Help:      "Number of Pyth price updates dropped",
	}, []string{"pyth_publisher", "pyth_price", "drop_reason"})
	metricUpdatesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "solana",
		Name:      "price_updates_sent_total",
		Help:      "Number of Pyth price updates sent",
	}, []string{"pyth_publisher", "pyth_price"})
)
