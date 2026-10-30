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

// Run starts the storage reservation recovery worker.
// It performs an immediate recovery pass and then runs every minute.
func Run(ctx context.Context) {
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()

	log.Println("Storage reservation recovery worker started")

	// Run once immediately on startup.
	runRecoveryPass(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Println("Storage reservation recovery worker stopped")
			return

		case <-ticker.C:
			runRecoveryPass(ctx)
		}
	}
}

// runRecoveryPass processes expired reservations in batches.
func runRecoveryPass(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		recovered, err := db.RecoverExpiredStorageReservations(
			ctx,
			recoveryBatchSize,
		)

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

		if recovered < recoveryBatchSize {
			return
		}
	}
}
