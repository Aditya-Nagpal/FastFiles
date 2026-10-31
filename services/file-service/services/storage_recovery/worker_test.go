package storage_recovery

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerRunsRecoveryImmediatelyOnStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	called := make(chan struct{}, 1)
	done := make(chan struct{})

	recover := func(ctx context.Context, batchSize int) (int, error) {
		if batchSize != 100 {
			t.Errorf("batch size = %d; want 100", batchSize)
		}

		called <- struct{}{}
		return 0, nil
	}

	go func() {
		defer close(done)
		runWith(ctx, time.Hour, 100, recover)
	}()

	// The worker must recover immediately without waiting for the ticker.
	select {
	case <-called:
		// Immediate recovery succeeded.

	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("worker did not perform immediate recovery")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}

	t.Log("PASS: worker performed recovery immediately on startup")
}

func TestWorkerRunsRecoveryPeriodically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32

	done := make(chan struct{})

	recover := func(ctx context.Context, batchSize int) (int, error) {
		calls.Add(1)
		return 0, nil
	}

	go func() {
		defer close(done)
		runWith(ctx, 30*time.Millisecond, 100, recover)
	}()

	// Wait for the immediate pass and at least one ticker pass.
	deadline := time.After(2 * time.Second)

	for calls.Load() < 2 {
		select {
		case <-deadline:
			cancel()
			t.Fatalf(
				"expected at least 2 recovery calls; got %d",
				calls.Load(),
			)
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}

	if calls.Load() < 2 {
		t.Fatalf("recovery calls = %d; want at least 2", calls.Load())
	}

	t.Logf(
		"PASS: worker performed %d recovery passes",
		calls.Load(),
	)
}

func TestWorkerStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	called := make(chan struct{}, 1)
	done := make(chan struct{})

	recover := func(ctx context.Context, batchSize int) (int, error) {
		called <- struct{}{}
		return 0, nil
	}

	go func() {
		defer close(done)
		runWith(ctx, time.Hour, 100, recover)
	}()

	// Ensure the worker has started its immediate recovery.
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("worker did not start")
	}

	cancel()

	select {
	case <-done:
		// Worker exited successfully.

	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}

	t.Log("PASS: worker stopped after context cancellation")
}

func TestRecoveryPassProcessesMultipleBatches(t *testing.T) {
	ctx := context.Background()

	batchSize := 2
	calls := 0
	totalRecovered := 0

	recover := func(ctx context.Context, receivedBatchSize int) (int, error) {
		if receivedBatchSize != batchSize {
			t.Errorf(
				"batch size = %d; want %d",
				receivedBatchSize,
				batchSize,
			)
		}

		calls++

		// Simulate 5 expired reservations:
		// Batch 1: 2
		// Batch 2: 2
		// Batch 3: 1
		switch calls {
		case 1, 2:
			return 2, nil
		case 3:
			return 1, nil
		default:
			return 0, nil
		}
	}

	runRecoveryPass(ctx, batchSize, func(
		ctx context.Context,
		size int,
	) (int, error) {
		recovered, err := recover(ctx, size)
		totalRecovered += recovered
		return recovered, err
	})

	if calls != 3 {
		t.Fatalf("recovery calls = %d; want 3", calls)
	}

	if totalRecovered != 5 {
		t.Fatalf("total recovered = %d; want 5", totalRecovered)
	}

	t.Logf(
		"PASS: recovery processed %d reservations across %d batches",
		totalRecovered,
		calls,
	)
}
