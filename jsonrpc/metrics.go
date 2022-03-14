package jsonrpc

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "rpc",
		Name:      "requests_total",
		Help:      "Number of RPC requests sent to Pythian",
	}, []string{"method"})
	metricCallbacks = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "pythian",
		Subsystem: "rpc",
		Name:      "callbacks_total",
		Help:      "Number of RPC callbacks delivered from Pythian to client",
	}, []string{"method"})
	metricWSConns = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "pythian",
		Subsystem: "rpc",
		Name:      "websocket_conns",
		Help:      "Number of active WebSocket conns to Pythian",
	})
)
