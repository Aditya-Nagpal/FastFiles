package storage_recovery

import (
	"context"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
)

const (
	recoveryBatchSize = 100
	recoveryInterval  = time.Minute
)

type recoveryFunc func(ctx context.Context, batchSize int) (int, error)

// Run starts the production storage reservation recovery worker.
func Run(ctx context.Context) {
	runWith(
		ctx,
		recoveryInterval,
		recoveryBatchSize,
		db.RecoverExpiredStorageReservations,
	)
}

// runWith contains the worker lifecycle and allows test configuration.
func runWith(
	ctx context.Context,
	interval time.Duration,
	batchSize int,
	recover recoveryFunc,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Println("Storage reservation recovery worker started")

	// Perform recovery immediately on startup.
	runRecoveryPass(ctx, batchSize, recover)

	for {
		select {
		case <-ctx.Done():
			log.Println("Storage reservation recovery worker stopped")
			return

		case <-ticker.C:
			runRecoveryPass(ctx, batchSize, recover)
		}
	}
}

// runRecoveryPass processes expired reservations in batches.
func runRecoveryPass(
	ctx context.Context,
	batchSize int,
	recover recoveryFunc,
) {
	for {
		if ctx.Err() != nil {
			return
		}

		recovered, err := recover(ctx, batchSize)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("Storage reservation recovery failed: %v", err)
			}
			return
		}

		if recovered > 0 {
			log.Printf(
				"Recovered %d expired storage reservations",
				recovered,
			)
		}

		// Continue while a complete batch was recovered.
		if recovered < batchSize {
			return
		}
	}
}
