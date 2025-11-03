package concurrency

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type Manager struct {
	maxConcurrent int64
	queueSize     int64

	active   int64
	rejected int64

	semaphore chan struct{}
	queue     chan struct{}
	metrics   *Metrics

	closed int32
}

type Metrics struct {
	TotalRequests     int64
	ActiveRequests    int64
	QueuedRequests    int64
	RejectedRequests  int64
	CompletedRequests int64
	FailedRequests    int64
	AverageWaitTime   int64
	AverageExecTime   int64
	MaxWaitTime       int64
	MaxExecTime       int64
	MaxQueueSize      int64
	CurrentQueueSize  int64
	LastUpdate        int64
}

func NewManager(maxConcurrent int, queueSize int) *Manager {
	if maxConcurrent <= 0 {
		maxConcurrent = 100
	}
	if queueSize <= 0 {
		queueSize = 1000
	}

	totalCapacity := maxConcurrent + queueSize

	m := &Manager{
		maxConcurrent: int64(maxConcurrent),
		queueSize:     int64(queueSize),
		semaphore:     make(chan struct{}, maxConcurrent),
		queue:         make(chan struct{}, totalCapacity),
		metrics:       &Metrics{},
	}

	return m
}

func (m *Manager) Execute(ctx context.Context, fn func() error) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return fmt.Errorf("concurrency manager is closed")
	}

	atomic.AddInt64(&m.metrics.TotalRequests, 1)

	select {
	case m.queue <- struct{}{}:
	case <-ctx.Done():
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return ctx.Err()
	default:
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return fmt.Errorf("request queue is full")
	}

	defer func() {
		<-m.queue
	}()

	select {
	case m.semaphore <- struct{}{}:
	case <-ctx.Done():
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return ctx.Err()
	}

	defer func() {
		<-m.semaphore
	}()

	atomic.AddInt64(&m.active, 1)
	atomic.AddInt64(&m.metrics.ActiveRequests, 1)

	defer func() {
		atomic.AddInt64(&m.active, -1)
		atomic.AddInt64(&m.metrics.ActiveRequests, -1)
	}()

	select {
	case <-ctx.Done():
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return ctx.Err()
	default:
	}

	startTime := time.Now()

	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic during request execution: %v", r)
			}
		}()
		err = fn()
	}()

	execTime := time.Since(startTime).Nanoseconds()
	m.updateExecTime(execTime)

	if err != nil {
		atomic.AddInt64(&m.metrics.FailedRequests, 1)
	} else {
		atomic.AddInt64(&m.metrics.CompletedRequests, 1)
	}

	return err
}



func (m *Manager) updateExecTime(execTime int64) {
	for {
		current := atomic.LoadInt64(&m.metrics.AverageExecTime)
		newAvg := (current*9 + execTime) / 10
		if atomic.CompareAndSwapInt64(&m.metrics.AverageExecTime, current, newAvg) {
			break
		}
	}

	for {
		current := atomic.LoadInt64(&m.metrics.MaxExecTime)
		if execTime <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.metrics.MaxExecTime, current, execTime) {
			break
		}
	}
}

func (m *Manager) GetMetrics() Metrics {
	return Metrics{
		TotalRequests:     atomic.LoadInt64(&m.metrics.TotalRequests),
		ActiveRequests:    atomic.LoadInt64(&m.active),
		QueuedRequests:    0,
		RejectedRequests:  atomic.LoadInt64(&m.rejected),
		CompletedRequests: atomic.LoadInt64(&m.metrics.CompletedRequests),
		FailedRequests:    atomic.LoadInt64(&m.metrics.FailedRequests),
		AverageWaitTime:   0,
		AverageExecTime:   atomic.LoadInt64(&m.metrics.AverageExecTime),
		MaxWaitTime:       0,
		MaxExecTime:       atomic.LoadInt64(&m.metrics.MaxExecTime),
		CurrentQueueSize:  0,
		LastUpdate:        time.Now().Unix(),
	}
}

func (m *Manager) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return fmt.Errorf("concurrency manager already closed")
	}

	return nil
}

func (m *Manager) IsHealthy() bool {
	metrics := m.GetMetrics()

	if metrics.TotalRequests > 100 {
		rejectionRate := float64(metrics.RejectedRequests) / float64(metrics.TotalRequests)
		if rejectionRate > 0.05 {
			return false
		}
	}

	return true
}
