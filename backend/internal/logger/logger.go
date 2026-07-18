// Package logger provides an asynchronous, buffered logging pipeline that
// drains request telemetry to PostgreSQL via background workers.
package logger

import (
	"context"
	"log"
	"sync"
	"time"

	"rate-limiter/internal/config"
	"rate-limiter/internal/database"
)

// Async buffers log entries in-memory and flushes them in configurable batches.
type Async struct {
	db            *database.Container
	buffer        chan database.LogEntry
	flushInterval time.Duration
	batchSize     int
	workerCount   int
	wg            sync.WaitGroup
	quit          chan struct{}

	mu             sync.Mutex
	totalLogged    int64
	totalDropped   int64
	totalFlushErrs int64
}

// New initializes the logger and spawns background flush workers.
func New(db *database.Container, cfg *config.Config) *Async {
	al := &Async{
		db:            db,
		buffer:        make(chan database.LogEntry, cfg.LogBufferSize),
		flushInterval: cfg.LogFlushInterval,
		batchSize:     cfg.LogBatchSize,
		workerCount:   cfg.LogWorkerCount,
		quit:          make(chan struct{}),
	}

	for i := 0; i < al.workerCount; i++ {
		al.wg.Add(1)
		go al.worker(i)
	}

	log.Printf("AsyncLogger started with %d workers, buffer size %d, flush every %v",
		al.workerCount, cfg.LogBufferSize, al.flushInterval)
	return al
}

// Log enqueues a log entry. Drops silently if the buffer is full.
func (al *Async) Log(entry database.LogEntry) {
	select {
	case al.buffer <- entry:
	default:
		al.mu.Lock()
		al.totalDropped++
		al.mu.Unlock()
	}
}

// Stats returns current logger metrics for monitoring endpoints.
func (al *Async) Stats() (logged, dropped, flushErrors int64) {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.totalLogged, al.totalDropped, al.totalFlushErrs
}

// Close signals all workers to drain and exit.
func (al *Async) Close() {
	close(al.quit)
	al.wg.Wait()
	log.Println("AsyncLogger: all workers shut down")
}

func (al *Async) worker(id int) {
	defer al.wg.Done()

	batch := make([]database.LogEntry, 0, al.batchSize)
	ticker := time.NewTicker(al.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-al.buffer:
			if !ok {
				al.flush(batch)
				return
			}
			batch = append(batch, entry)
			if len(batch) >= al.batchSize {
				al.flush(batch)
				batch = make([]database.LogEntry, 0, al.batchSize)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				al.flush(batch)
				batch = make([]database.LogEntry, 0, al.batchSize)
			}

		case <-al.quit:
			al.drainAndFlush(batch)
			return
		}
	}
}

func (al *Async) flush(batch []database.LogEntry) {
	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := al.db.InsertLogsBatch(ctx, batch); err != nil {
		log.Printf("AsyncLogger: flush error (%d entries): %v", len(batch), err)
		al.mu.Lock()
		al.totalFlushErrs++
		al.mu.Unlock()
		return
	}

	al.mu.Lock()
	al.totalLogged += int64(len(batch))
	al.mu.Unlock()
}

func (al *Async) drainAndFlush(existing []database.LogEntry) {
	for {
		select {
		case entry, ok := <-al.buffer:
			if !ok {
				al.flush(existing)
				return
			}
			existing = append(existing, entry)
			if len(existing) >= al.batchSize {
				al.flush(existing)
				existing = make([]database.LogEntry, 0, al.batchSize)
			}
		default:
			al.flush(existing)
			return
		}
	}
}
