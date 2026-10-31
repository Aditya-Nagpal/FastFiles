package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStorageReservationHeartbeatRenewsSuccessfully(t *testing.T) {
	renewed := make(chan struct{}, 1)

	renew := func(
		ctx context.Context,
		reservationID int64,
		leaseDuration time.Duration,
	) error {
		if reservationID != 123 {
			t.Errorf("unexpected reservation ID: %d", reservationID)
		}

		if leaseDuration != 5*time.Minute {
			t.Errorf("unexpected lease duration: %v", leaseDuration)
		}

		select {
		case renewed <- struct{}{}:
		default:
		}

		return nil
	}

	uploadCtx, stopHeartbeat := StartStorageReservationHeartbeatWith(
		context.Background(),
		123,
		20*time.Millisecond,
		5*time.Minute,
		renew,
	)

	// Wait for at least one successful renewal.
	select {
	case <-renewed:
		// Renewal succeeded.

	case <-time.After(2 * time.Second):
		stopHeartbeat()
		t.Fatal("heartbeat did not renew the reservation")
	}

	if err := stopHeartbeat(); err != nil {
		t.Fatalf("unexpected heartbeat error: %v", err)
	}

	// Stopping heartbeat also cancels its upload context.
	if !errors.Is(uploadCtx.Err(), context.Canceled) {
		t.Fatalf("expected upload context to be canceled, got: %v", uploadCtx.Err())
	}

	t.Log("PASS: heartbeat successfully renewed the reservation")
}

func TestStorageReservationHeartbeatCancelsUploadOnRenewalFailure(t *testing.T) {
	expectedErr := errors.New("database connection failed")
	renewalCalled := make(chan struct{})

	renew := func(
		ctx context.Context,
		reservationID int64,
		leaseDuration time.Duration,
	) error {
		close(renewalCalled)
		return expectedErr
	}

	uploadCtx, stopHeartbeat := StartStorageReservationHeartbeatWith(
		context.Background(),
		456,
		20*time.Millisecond,
		5*time.Minute,
		renew,
	)

	// Wait until renewal is attempted.
	select {
	case <-renewalCalled:
	case <-time.After(2 * time.Second):
		stopHeartbeat()
		t.Fatal("heartbeat did not attempt renewal")
	}

	// Renewal failure must cancel the upload context.
	select {
	case <-uploadCtx.Done():
	case <-time.After(2 * time.Second):
		stopHeartbeat()
		t.Fatal("upload context was not canceled after renewal failure")
	}

	err := stopHeartbeat()

	if err == nil {
		t.Fatal("expected heartbeat renewal error, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	if !strings.Contains(err.Error(), "456") {
		t.Fatalf("error should contain reservation ID, got: %v", err)
	}

	t.Log("PASS: renewal failure correctly canceled the upload")
}

func TestStorageReservationHeartbeatStopsOnParentCancellation(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())

	renewalCalled := make(chan struct{}, 1)

	renew := func(
		ctx context.Context,
		reservationID int64,
		leaseDuration time.Duration,
	) error {
		renewalCalled <- struct{}{}
		return nil
	}

	uploadCtx, stopHeartbeat := StartStorageReservationHeartbeatWith(
		parentCtx,
		789,
		1*time.Second,
		5*time.Minute,
		renew,
	)

	// Cancel the parent request/application context.
	cancelParent()

	select {
	case <-uploadCtx.Done():
	case <-time.After(2 * time.Second):
		stopHeartbeat()
		t.Fatal("upload context was not canceled with parent context")
	}

	if err := stopHeartbeat(); err != nil {
		t.Fatalf("unexpected heartbeat error: %v", err)
	}

	select {
	case <-renewalCalled:
		t.Fatal("heartbeat should not renew after parent cancellation")
	default:
	}

	t.Log("PASS: heartbeat stopped after parent context cancellation")
}
