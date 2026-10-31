package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
)

const (
	reservationLeaseDuration     = 5 * time.Minute
	reservationHeartbeatInterval = 1 * time.Minute
	reservationRenewalTimeout    = 10 * time.Second
)

// reservationRenewFunc allows us to inject renewal behaviour in tests.
type reservationRenewFunc func(
	ctx context.Context,
	reservationID int64,
	leaseDuration time.Duration,
) error

func StartStorageReservationHeartbeat(
	parentCtx context.Context,
	reservationID int64,
) (context.Context, func() error) {
	return StartStorageReservationHeartbeatWith(
		parentCtx,
		reservationID,
		reservationHeartbeatInterval,
		reservationLeaseDuration,
		db.RenewStorageReservation,
	)
}

// startStorageReservationHeartbeatWith contains the actual heartbeat logic.
// It accepts configurable intervals and a renewal function for testing.
func StartStorageReservationHeartbeatWith(
	parentCtx context.Context,
	reservationID int64,
	heartbeatInterval time.Duration,
	leaseDuration time.Duration,
	renew reservationRenewFunc,
) (context.Context, func() error) {

	uploadCtx, cancelUpload := context.WithCancel(parentCtx)
	heartbeatCtx, cancelHeartbeat := context.WithCancel(parentCtx)

	stop := make(chan struct{})
	done := make(chan struct{})

	var heartbeatErr error
	var stopOnce sync.Once

	go func() {
		defer close(done)

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return

			case <-heartbeatCtx.Done():
				return

			case <-ticker.C:
				renewCtx, cancel := context.WithTimeout(
					heartbeatCtx,
					reservationRenewalTimeout,
				)

				err := renew(
					renewCtx,
					reservationID,
					leaseDuration,
				)

				cancel()

				if err != nil {
					heartbeatErr = fmt.Errorf(
						"failed to renew storage reservation %d: %w",
						reservationID,
						err,
					)

					cancelUpload()
					return
				}
			}
		}
	}()

	stopAndWait := func() error {
		stopOnce.Do(func() {
			close(stop)

			// Wait for any ongoing renewal to finish.
			<-done

			cancelHeartbeat()
			cancelUpload()
		})

		return heartbeatErr
	}

	return uploadCtx, stopAndWait
}
