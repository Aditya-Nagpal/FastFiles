package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
)

const (
	reservationLeaseDuration     = 5 * time.Minute
	reservationHeartbeatInterval = 1 * time.Minute
	reservationRenewalTimeout    = 10 * time.Second
)

func StartStorageReservationHeartbeat(parentCtx context.Context, reservationID int64) (context.Context, func() error) {

	// S3 upload gets its own cancellable context.
	uploadCtx, cancelUpload := context.WithCancel(parentCtx)

	// Heartbeat has a separate context so stopping it does not
	// prematurely cancel the upload context.
	heartbeatCtx, cancelHeartbeat := context.WithCancel(parentCtx)

	stop := make(chan struct{})
	done := make(chan struct{})

	var heartbeatErr error

	go func() {
		defer close(done)

		ticker := time.NewTicker(reservationHeartbeatInterval)
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

				err := db.RenewStorageReservation(
					renewCtx,
					reservationID,
					reservationLeaseDuration,
				)

				cancel()

				if err != nil {
					heartbeatErr = fmt.Errorf(
						"failed to renew storage reservation %d: %w",
						reservationID,
						err,
					)

					// Stop the ongoing S3 upload.
					cancelUpload()
					return
				}
			}
		}
	}()

	stopAndWait := func() error {
		close(stop)

		// Wait until any ongoing renewal finishes.
		// After this, no heartbeat can race with finalization.
		<-done

		cancelHeartbeat()
		cancelUpload()

		return heartbeatErr
	}

	return uploadCtx, stopAndWait
}
