// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package middleware —— Prometheus 指标：HTTP 请求计数/耗时直方图 + 进程/Go 运行时。
// 指标语义：
//
//	ptpatronus_http_requests_total{method,path,code}   累计请求数（path 已按 :param 归并，防标签爆炸）
//	ptpatronus_http_request_duration_seconds{method,path}  耗时直方图（Prometheus 官方 bucket）
//	ptpatronus_http_requests_in_flight                 在途请求数（Gauge）
//	加 client_golang 默认 collector：go_* / process_*（GC、内存、fd、CPU）
package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ptpatronus_http_requests_total",
			Help: "HTTP requests processed, labeled by method, resolved route path and status code.",
		},
		[]string{"method", "path", "code"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ptpatronus_http_request_duration_seconds",
			Help:    "HTTP request latency distribution.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ptpatronus_http_requests_in_flight",
			Help: "Number of HTTP requests currently being served.",
		},
	)
)

func init() {
	prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, httpRequestsInFlight)
}

// Prometheus 记录每请求指标。path 用 gin 路由模板（/sites/:id）而非实际 URL
// （/sites/123），否则高基数标签会打爆 Prometheus；未匹配路由的 404 记为 "unmatched"。
func Prometheus() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 静态资源与 /metrics 自身不计数（自观测噪音 + 防自激）
		p := c.FullPath()
		if p == "" {
			p = "unmatched"
		}
		if p == "/metrics" || (len(p) > 8 && p[:8] == "/assets/") {
			c.Next()
			return
		}
		start := time.Now()
		httpRequestsInFlight.Inc()
		c.Next()
		httpRequestsInFlight.Dec()
		httpRequestsTotal.WithLabelValues(c.Request.Method, p, strconv.Itoa(c.Writer.Status())).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, p).Observe(time.Since(start).Seconds())
	}
}
