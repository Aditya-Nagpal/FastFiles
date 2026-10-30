package db

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func setupStorageRecoveryTest(t *testing.T) (context.Context, int64) {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		cancel()
		t.Fatalf("connect to PostgreSQL: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cancel()
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	previousDB := DB
	DB = pool

	userID := time.Now().UnixNano()

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (
			user_id, quota_bytes, used_bytes, reserved_bytes
		)
		VALUES ($1, 1000000, 0, 0)
	`, userID)

	if err != nil {
		DB = previousDB
		pool.Close()
		cancel()
		t.Fatalf("create test storage: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM storage_reservations
			WHERE user_id = $1
		`, userID)

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, userID)

		DB = previousDB
		pool.Close()
		cancel()
	})

	return ctx, userID
}

func createExpiredTestReservation(
	t *testing.T,
	ctx context.Context,
	userID int64,
	size int64,
) int64 {
	t.Helper()

	reservationID, err := CreateStorageReservation(ctx, userID, size)
	if err != nil {
		t.Fatalf("create reservation: %v", err)
	}

	_, err = DB.Exec(ctx, `
		UPDATE storage_reservations
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 minute'
		WHERE id = $1
	`, reservationID)

	if err != nil {
		t.Fatalf("expire reservation: %v", err)
	}

	return reservationID
}

func getReservationStatus(
	t *testing.T,
	ctx context.Context,
	reservationID int64,
) string {
	t.Helper()

	var status string

	err := DB.QueryRow(ctx, `
		SELECT status
		FROM storage_reservations
		WHERE id = $1
	`, reservationID).Scan(&status)

	if err != nil {
		t.Fatalf("get reservation status: %v", err)
	}

	return status
}

func TestRecoverExpiredStorageReservations(t *testing.T) {
	ctx, userID := setupStorageRecoveryTest(t)

	expiredID := createExpiredTestReservation(t, ctx, userID, 100)

	activeID, err := CreateStorageReservation(ctx, userID, 200)
	if err != nil {
		t.Fatal(err)
	}

	recovered, err := RecoverExpiredStorageReservations(ctx, 10)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if recovered != 1 {
		t.Fatalf("expected 1 recovered reservation, got %d", recovered)
	}

	if got := getReservationStatus(t, ctx, expiredID); got != "RELEASED" {
		t.Fatalf("expected expired reservation RELEASED, got %s", got)
	}

	if got := getReservationStatus(t, ctx, activeID); got != "PENDING" {
		t.Fatalf("expected active reservation PENDING, got %s", got)
	}

	var reservedBytes int64

	err = DB.QueryRow(ctx, `
		SELECT reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&reservedBytes)

	if err != nil {
		t.Fatal(err)
	}

	if reservedBytes != 200 {
		t.Fatalf("expected reserved_bytes=200, got %d", reservedBytes)
	}

	t.Log("PASS: expired reservation recovered; active reservation untouched")
}

func TestRecoveryIgnoresFinalizedAndReleasedReservations(t *testing.T) {
	ctx, userID := setupStorageRecoveryTest(t)

	finalizedID, err := CreateStorageReservation(ctx, userID, 40)
	if err != nil {
		t.Fatal(err)
	}

	tx, err := DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = FinalizeStorageReservationUsingTransaction(
		tx, ctx, finalizedID, userID, 40,
	)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	releasedID, err := CreateStorageReservation(ctx, userID, 30)
	if err != nil {
		t.Fatal(err)
	}

	if err := ReleaseStorageReservationByID(ctx, releasedID); err != nil {
		t.Fatal(err)
	}

	recovered, err := RecoverExpiredStorageReservations(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}

	if recovered != 0 {
		t.Fatalf("expected 0 recovered reservations, got %d", recovered)
	}

	if got := getReservationStatus(t, ctx, finalizedID); got != "FINALIZED" {
		t.Fatalf("expected FINALIZED, got %s", got)
	}

	if got := getReservationStatus(t, ctx, releasedID); got != "RELEASED" {
		t.Fatalf("expected RELEASED, got %s", got)
	}

	var usedBytes, reservedBytes int64

	err = DB.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)

	if err != nil {
		t.Fatal(err)
	}

	if usedBytes != 40 || reservedBytes != 0 {
		t.Fatalf(
			"unexpected accounting: used=%d reserved=%d",
			usedBytes, reservedBytes,
		)
	}

	t.Log("PASS: finalized and released reservations were untouched")
}

func TestConcurrentReservationRecovery(t *testing.T) {
	ctx, userID := setupStorageRecoveryTest(t)

	reservationID := createExpiredTestReservation(t, ctx, userID, 100)

	const workers = 2

	var wg sync.WaitGroup
	var mu sync.Mutex

	totalRecovered := 0
	var firstErr error

	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()

			recovered, err := RecoverExpiredStorageReservations(ctx, 10)

			mu.Lock()
			defer mu.Unlock()

			if err != nil && firstErr == nil {
				firstErr = err
			}

			totalRecovered += recovered
		}()
	}

	wg.Wait()

	if firstErr != nil {
		t.Fatalf("concurrent recovery failed: %v", firstErr)
	}

	if totalRecovered != 1 {
		t.Fatalf("expected exactly 1 recovery, got %d", totalRecovered)
	}

	if got := getReservationStatus(t, ctx, reservationID); got != "RELEASED" {
		t.Fatalf("expected RELEASED, got %s", got)
	}

	var reservedBytes int64

	err := DB.QueryRow(ctx, `
		SELECT reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&reservedBytes)

	if err != nil {
		t.Fatal(err)
	}

	if reservedBytes != 0 {
		t.Fatalf("expected reserved_bytes=0, got %d", reservedBytes)
	}

	t.Log("PASS: concurrent workers recovered the reservation exactly once")
}

func TestReservationRecoveryRollsBackOnAccountingFailure(t *testing.T) {
	ctx, userID := setupStorageRecoveryTest(t)

	firstID := createExpiredTestReservation(t, ctx, userID, 40)
	secondID := createExpiredTestReservation(t, ctx, userID, 60)

	// Deliberately corrupt the aggregate counter.
	// There are 100 bytes reserved across these records,
	// but only 60 bytes recorded in user_storage.
	_, err := DB.Exec(ctx, `
		UPDATE user_storage
		SET reserved_bytes = 60
		WHERE user_id = $1
	`, userID)

	if err != nil {
		t.Fatal(err)
	}

	_, err = RecoverExpiredStorageReservations(ctx, 10)

	if err == nil {
		t.Fatal("expected recovery to fail due to accounting inconsistency")
	}

	// The entire batch must roll back.
	if got := getReservationStatus(t, ctx, firstID); got != "PENDING" {
		t.Fatalf("first reservation should remain PENDING, got %s", got)
	}

	if got := getReservationStatus(t, ctx, secondID); got != "PENDING" {
		t.Fatalf("second reservation should remain PENDING, got %s", got)
	}

	var reservedBytes int64

	err = DB.QueryRow(ctx, `
		SELECT reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&reservedBytes)

	if err != nil {
		t.Fatal(err)
	}

	if reservedBytes != 60 {
		t.Fatalf("expected rollback to preserve reserved_bytes=60, got %d", reservedBytes)
	}

	t.Logf("PASS: recovery batch rolled back correctly: %v", err)
}

func TestRecoveryRejectsInvalidBatchSize(t *testing.T) {
	ctx, _ := setupStorageRecoveryTest(t)

	_, err := RecoverExpiredStorageReservations(ctx, 0)
	if err == nil {
		t.Fatal("expected error for zero batch size")
	}

	_, err = RecoverExpiredStorageReservations(ctx, -1)
	if err == nil {
		t.Fatal("expected error for negative batch size")
	}

	t.Log("PASS: invalid batch sizes rejected")
}

var _ = fmt.Sprintf
