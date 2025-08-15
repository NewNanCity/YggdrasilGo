// Package utils 性能监控工具
package utils

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	// 请求统计
	RequestCount int64 // 总请求数
	ErrorCount   int64 // 错误请求数

	// 响应时间统计
	TotalResponseTime int64 // 总响应时间（纳秒）
	MaxResponseTime   int64 // 最大响应时间（纳秒）
	MinResponseTime   int64 // 最小响应时间（纳秒）

	// 数据库统计
	DBQueryCount int64 // 数据库查询次数
	TotalDBTime  int64 // 总数据库时间（纳秒）

	// 缓存统计
	CacheHits   int64 // 缓存命中次数
	CacheMisses int64 // 缓存未命中次数

	// 启动时间
	StartTime time.Time
}

// 全局性能指标实例
var GlobalMetrics = &PerformanceMetrics{
	StartTime:       time.Now(),
	MinResponseTime: int64(^uint64(0) >> 1), // 初始化为最大值
}

// RecordRequest 记录请求
func (m *PerformanceMetrics) RecordRequest(duration time.Duration, isError bool) {
	atomic.AddInt64(&m.RequestCount, 1)

	if isError {
		atomic.AddInt64(&m.ErrorCount, 1)
	}

	// 记录响应时间
	durationNanos := duration.Nanoseconds()
	atomic.AddInt64(&m.TotalResponseTime, durationNanos)

	// 更新最大响应时间
	for {
		current := atomic.LoadInt64(&m.MaxResponseTime)
		if durationNanos <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.MaxResponseTime, current, durationNanos) {
			break
		}
	}

	// 更新最小响应时间
	for {
		current := atomic.LoadInt64(&m.MinResponseTime)
		if durationNanos >= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.MinResponseTime, current, durationNanos) {
			break
		}
	}
}

// RecordDBQuery 记录数据库查询
func (m *PerformanceMetrics) RecordDBQuery(duration time.Duration) {
	atomic.AddInt64(&m.DBQueryCount, 1)
	atomic.AddInt64(&m.TotalDBTime, duration.Nanoseconds())
}

// RecordCacheHit 记录缓存命中
func (m *PerformanceMetrics) RecordCacheHit() {
	atomic.AddInt64(&m.CacheHits, 1)
}

// RecordCacheMiss 记录缓存未命中
func (m *PerformanceMetrics) RecordCacheMiss() {
	atomic.AddInt64(&m.CacheMisses, 1)
}

// GetStats 获取统计信息
func (m *PerformanceMetrics) GetStats() map[string]interface{} {
	requestCount := atomic.LoadInt64(&m.RequestCount)
	errorCount := atomic.LoadInt64(&m.ErrorCount)
	totalResponseTime := atomic.LoadInt64(&m.TotalResponseTime)
	maxResponseTime := atomic.LoadInt64(&m.MaxResponseTime)
	minResponseTime := atomic.LoadInt64(&m.MinResponseTime)
	dbQueryCount := atomic.LoadInt64(&m.DBQueryCount)
	totalDBTime := atomic.LoadInt64(&m.TotalDBTime)
	cacheHits := atomic.LoadInt64(&m.CacheHits)
	cacheMisses := atomic.LoadInt64(&m.CacheMisses)

	// 计算平均值
	var avgResponseTime float64
	var avgDBTime float64
	var cacheHitRate float64

	if requestCount > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(requestCount) / 1e6 // 转换为毫秒
	}

	if dbQueryCount > 0 {
		avgDBTime = float64(totalDBTime) / float64(dbQueryCount) / 1e6 // 转换为毫秒
	}

	totalCacheRequests := cacheHits + cacheMisses
	if totalCacheRequests > 0 {
		cacheHitRate = float64(cacheHits) / float64(totalCacheRequests) * 100
	}

	// 获取内存统计
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"uptime_seconds":       time.Since(m.StartTime).Seconds(),
		"request_count":        requestCount,
		"error_count":          errorCount,
		"error_rate":           float64(errorCount) / float64(requestCount) * 100,
		"avg_response_time_ms": avgResponseTime,
		"max_response_time_ms": float64(maxResponseTime) / 1e6,
		"min_response_time_ms": float64(minResponseTime) / 1e6,
		"db_query_count":       dbQueryCount,
		"avg_db_time_ms":       avgDBTime,
		"cache_hit_rate":       cacheHitRate,
		"cache_hits":           cacheHits,
		"cache_misses":         cacheMisses,
		"memory": map[string]interface{}{
			"alloc_mb":       float64(memStats.Alloc) / 1024 / 1024,
			"total_alloc_mb": float64(memStats.TotalAlloc) / 1024 / 1024,
			"sys_mb":         float64(memStats.Sys) / 1024 / 1024,
			"gc_count":       memStats.NumGC,
		},
	}
}

// ResetStats 重置统计信息
func (m *PerformanceMetrics) ResetStats() {
	atomic.StoreInt64(&m.RequestCount, 0)
	atomic.StoreInt64(&m.ErrorCount, 0)
	atomic.StoreInt64(&m.TotalResponseTime, 0)
	atomic.StoreInt64(&m.MaxResponseTime, 0)
	atomic.StoreInt64(&m.MinResponseTime, int64(^uint64(0)>>1))
	atomic.StoreInt64(&m.DBQueryCount, 0)
	atomic.StoreInt64(&m.TotalDBTime, 0)
	atomic.StoreInt64(&m.CacheHits, 0)
	atomic.StoreInt64(&m.CacheMisses, 0)
	m.StartTime = time.Now()
}

// GetQPS 获取每秒请求数
func (m *PerformanceMetrics) GetQPS() float64 {
	uptime := time.Since(m.StartTime).Seconds()
	if uptime <= 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.RequestCount)) / uptime
}

// GetCacheHitRate 获取缓存命中率
func (m *PerformanceMetrics) GetCacheHitRate() float64 {
	hits := atomic.LoadInt64(&m.CacheHits)
	misses := atomic.LoadInt64(&m.CacheMisses)
	total := hits + misses

	if total == 0 {
		return 0
	}

	return float64(hits) / float64(total) * 100
}

// GetAvgResponseTime 获取平均响应时间（毫秒）
func (m *PerformanceMetrics) GetAvgResponseTime() float64 {
	requestCount := atomic.LoadInt64(&m.RequestCount)
	if requestCount == 0 {
		return 0
	}

	totalTime := atomic.LoadInt64(&m.TotalResponseTime)
	return float64(totalTime) / float64(requestCount) / 1e6 // 转换为毫秒
}

// GetAvgDBTime 获取平均数据库查询时间（毫秒）
func (m *PerformanceMetrics) GetAvgDBTime() float64 {
	queryCount := atomic.LoadInt64(&m.DBQueryCount)
	if queryCount == 0 {
		return 0
	}

	totalTime := atomic.LoadInt64(&m.TotalDBTime)
	return float64(totalTime) / float64(queryCount) / 1e6 // 转换为毫秒
}

// PrintStats 打印统计信息
func (m *PerformanceMetrics) PrintStats() {
	stats := m.GetStats()

	fmt.Printf("\n📊 Performance Statistics:\n")
	fmt.Printf("  Uptime: %.2f seconds\n", stats["uptime_seconds"])
	fmt.Printf("  Requests: %d (QPS: %.2f)\n", stats["request_count"], m.GetQPS())
	fmt.Printf("  Errors: %d (Rate: %.2f%%)\n", stats["error_count"], stats["error_rate"])
	fmt.Printf("  Response Time: Avg=%.2fms, Max=%.2fms, Min=%.2fms\n",
		stats["avg_response_time_ms"], stats["max_response_time_ms"], stats["min_response_time_ms"])
	fmt.Printf("  DB Queries: %d (Avg: %.2fms)\n", stats["db_query_count"], stats["avg_db_time_ms"])
	fmt.Printf("  Cache: Hit Rate=%.2f%% (Hits=%d, Misses=%d)\n",
		stats["cache_hit_rate"], stats["cache_hits"], stats["cache_misses"])

	if memory, ok := stats["memory"].(map[string]interface{}); ok {
		fmt.Printf("  Memory: Alloc=%.2fMB, Total=%.2fMB, Sys=%.2fMB, GC=%d\n",
			memory["alloc_mb"], memory["total_alloc_mb"], memory["sys_mb"], memory["gc_count"])
	}
}
