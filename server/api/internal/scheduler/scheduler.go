// Package scheduler runs background maintenance tasks on a fixed interval.
package scheduler

import (
	"sync"

	"vpnapp/server/api/internal/repository"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"time"
)

const cleanupInterval = 1 * time.Minute

// scheduler is the internal state for the background worker.
type scheduler struct {
	ticker *time.Ticker
	done   chan struct{}
	wg     sync.WaitGroup
}

// global instance — only one scheduler is expected per process.
var instance *scheduler
var mu sync.Mutex

// Start launches the background goroutine that cleans up expired sessions
// once per cleanupInterval. Calling Start more than once is safe — subsequent
// calls are no-ops if a scheduler is already running.
func Start(db *gorm.DB, logger *zap.Logger) {
	mu.Lock()
	defer mu.Unlock()

	if instance != nil {
		return
	}

	s := &scheduler{
		ticker: time.NewTicker(cleanupInterval),
		done:   make(chan struct{}),
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// Run once immediately so the first cleanup does not wait a full interval.
		runCleanup(db, logger)

		for {
			select {
			case <-s.ticker.C:
				runCleanup(db, logger)
			case <-s.done:
				return
			}
		}
	}()

	instance = s
	logger.Info("session cleanup scheduler started", zap.Duration("interval", cleanupInterval))
}

// Stop signals the background goroutine to exit and waits for it to finish.
// Safe to call even if Start was never called.
func Stop() {
	mu.Lock()
	s := instance
	instance = nil
	mu.Unlock()

	if s == nil {
		return
	}

	s.ticker.Stop()
	close(s.done)
	s.wg.Wait()
}

// runCleanup deletes expired sessions and stale connections, and logs results.
func runCleanup(db *gorm.DB, logger *zap.Logger) {
	// Clean expired sessions
	count, err := repository.DeleteExpiredSessions(db)
	if err != nil {
		logger.Error("session cleanup failed", zap.Error(err))
	} else if count > 0 {
		logger.Info("expired sessions cleaned up", zap.Int64("count", count))
	}

	// Clean stale reservations (connecting for >2 min)
	reservationCount, err := repository.CleanupStaleReservations(db, 2*time.Minute)
	if err != nil {
		logger.Error("stale reservation cleanup failed", zap.Error(err))
	} else if reservationCount > 0 {
		logger.Info("stale reservations cleaned up", zap.Int64("count", reservationCount))
	}

	// Clean stale connections (no heartbeat for >3 min)
	staleCount, err := repository.CleanupStaleConnections(db, 3*time.Minute)
	if err != nil {
		logger.Error("stale connection cleanup failed", zap.Error(err))
	} else if staleCount > 0 {
		logger.Info("stale connections cleaned up", zap.Int64("count", staleCount))
	}

	// Delete expired share/link codes so the table does not grow unbounded.
	// Each code is short-lived (5 min) and one-time-use anyway, so this only
	// catches codes that were generated but never redeemed.
	codeCount, err := repository.DeleteExpiredLinkCodes(db)
	if err != nil {
		logger.Error("expired link code cleanup failed", zap.Error(err))
	} else if codeCount > 0 {
		logger.Info("expired link codes cleaned up", zap.Int64("count", codeCount))
	}
}
