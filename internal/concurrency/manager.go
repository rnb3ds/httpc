package concurrency

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Manager struct {
	maxConcurrent int64
	queueSize     int

	active   int64
	queued   int64
	total    int64
	rejected int64

	semaphore chan struct{}
	queue     chan *Request
	metrics   *Metrics

	closed int32
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

type Request struct {
	Execute func() error
	Context context.Context
	Result  chan error
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
		queueSize = maxConcurrent * 2
	}

	m := &Manager{
		maxConcurrent: int64(maxConcurrent),
		queueSize:     queueSize,
		semaphore:     make(chan struct{}, maxConcurrent),
		queue:         make(chan *Request, queueSize),
		metrics:       &Metrics{},
	}

	for i := 0; i < maxConcurrent; i++ {
		m.wg.Add(1)
		go m.worker()
	}

	return m
}

func (m *Manager) Execute(ctx context.Context, fn func() error) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return fmt.Errorf("concurrency manager is closed")
	}

	req := &Request{
		Execute: fn,
		Context: ctx,
		Result:  make(chan error, 1),
	}

	atomic.AddInt64(&m.metrics.TotalRequests, 1)
	startTime := time.Now()

	select {
	case m.queue <- req:
		atomic.AddInt64(&m.queued, 1)
		atomic.AddInt64(&m.metrics.QueuedRequests, 1)
	case <-ctx.Done():
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return ctx.Err()
	default:
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return fmt.Errorf("request queue is full")
	}

	select {
	case err := <-req.Result:
		waitTime := time.Since(startTime).Nanoseconds()
		m.updateWaitTime(waitTime)
		return err
	case <-ctx.Done():
		atomic.AddInt64(&m.rejected, 1)
		atomic.AddInt64(&m.metrics.RejectedRequests, 1)
		return ctx.Err()
	}
}

func (m *Manager) worker() {
	defer m.wg.Done()

	for {
		// Check if manager is closed before waiting for requests
		if atomic.LoadInt32(&m.closed) == 1 {
			return
		}

		select {
		case req, ok := <-m.queue:
			if !ok {
				// Channel closed, exit worker
				return
			}
			if req == nil {
				return
			}
			m.processRequest(req)
		}
	}
}

func (m *Manager) processRequest(req *Request) {
	select {
	case m.semaphore <- struct{}{}:
		defer func() { <-m.semaphore }()
	case <-req.Context.Done():
		req.Result <- req.Context.Err()
		return
	}

	atomic.AddInt64(&m.active, 1)
	atomic.AddInt64(&m.queued, -1)
	atomic.AddInt64(&m.metrics.ActiveRequests, 1)
	atomic.AddInt64(&m.metrics.QueuedRequests, -1)

	defer func() {
		atomic.AddInt64(&m.active, -1)
		atomic.AddInt64(&m.metrics.ActiveRequests, -1)
	}()

	// Check context again before executing
	select {
	case <-req.Context.Done():
		req.Result <- req.Context.Err()
		return
	default:
	}

	startTime := time.Now()
	err := req.Execute()
	execTime := time.Since(startTime).Nanoseconds()

	m.updateExecTime(execTime)

	if err != nil {
		atomic.AddInt64(&m.metrics.FailedRequests, 1)
	} else {
		atomic.AddInt64(&m.metrics.CompletedRequests, 1)
	}

	select {
	case req.Result <- err:
	case <-req.Context.Done():
	}
}

func (m *Manager) updateWaitTime(waitTime int64) {
	for {
		current := atomic.LoadInt64(&m.metrics.AverageWaitTime)
		newAvg := (current*9 + waitTime) / 10
		if atomic.CompareAndSwapInt64(&m.metrics.AverageWaitTime, current, newAvg) {
			break
		}
	}

	for {
		current := atomic.LoadInt64(&m.metrics.MaxWaitTime)
		if waitTime <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.metrics.MaxWaitTime, current, waitTime) {
			break
		}
	}
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
		QueuedRequests:    atomic.LoadInt64(&m.queued),
		RejectedRequests:  atomic.LoadInt64(&m.rejected),
		CompletedRequests: atomic.LoadInt64(&m.metrics.CompletedRequests),
		FailedRequests:    atomic.LoadInt64(&m.metrics.FailedRequests),
		AverageWaitTime:   atomic.LoadInt64(&m.metrics.AverageWaitTime),
		AverageExecTime:   atomic.LoadInt64(&m.metrics.AverageExecTime),
		MaxWaitTime:       atomic.LoadInt64(&m.metrics.MaxWaitTime),
		MaxExecTime:       atomic.LoadInt64(&m.metrics.MaxExecTime),
		CurrentQueueSize:  int64(len(m.queue)),
		LastUpdate:        time.Now().Unix(),
	}
}

func (m *Manager) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return fmt.Errorf("concurrency manager already closed")
	}

	close(m.queue)
	m.wg.Wait()

	return nil
}

func (m *Manager) IsHealthy() bool {
	metrics := m.GetMetrics()

	queueUtilization := float64(metrics.CurrentQueueSize) / float64(m.queueSize)
	if queueUtilization > 0.8 {
		return false
	}

	if metrics.TotalRequests > 100 {
		rejectionRate := float64(metrics.RejectedRequests) / float64(metrics.TotalRequests)
		if rejectionRate > 0.05 {
			return false
		}
	}

	return true
}
