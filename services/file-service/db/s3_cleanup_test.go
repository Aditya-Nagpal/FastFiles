package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func setupS3CleanupTest(t *testing.T) context.Context {
	t.Helper()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

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

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM s3_cleanup_tasks
			WHERE s3_key LIKE $1
		`, testS3CleanupKeyPrefix(t))

		DB = previousDB
		pool.Close()
		cancel()
	})

	return ctx
}

func testS3CleanupKeyPrefix(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-cleanup/%s/%%", t.Name())
}

func createTestS3CleanupTask(t *testing.T, ctx context.Context) int64 {
	t.Helper()

	key := fmt.Sprintf(
		"test-cleanup/%s/%d.txt",
		t.Name(),
		time.Now().UnixNano(),
	)

	id, err := CreateS3CleanupTask(ctx, "test-bucket", key)
	if err != nil {
		t.Fatalf("create S3 cleanup task: %v", err)
	}

	return id
}

func TestCreateS3CleanupTask(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	var bucket, key, status string
	var attempts, maxAttempts int

	err := DB.QueryRow(ctx, `
		SELECT bucket_name, s3_key, status, attempts, max_attempts
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(
		&bucket,
		&key,
		&status,
		&attempts,
		&maxAttempts,
	)

	if err != nil {
		t.Fatalf("fetch created task: %v", err)
	}

	if bucket != "test-bucket" {
		t.Fatalf("bucket = %s; want test-bucket", bucket)
	}

	if status != "PENDING" {
		t.Fatalf("status = %s; want PENDING", status)
	}

	if attempts != 0 {
		t.Fatalf("attempts = %d; want 0", attempts)
	}

	if maxAttempts != 10 {
		t.Fatalf("max_attempts = %d; want 10", maxAttempts)
	}

	if key == "" {
		t.Fatal("expected non-empty S3 key")
	}

	t.Log("PASS: S3 cleanup task created successfully")
}

func TestCreateS3CleanupTaskRejectsInvalidInput(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	_, err := CreateS3CleanupTask(ctx, "", "some-key")
	if err == nil {
		t.Fatal("expected error for empty bucket name")
	}

	_, err = CreateS3CleanupTask(ctx, "test-bucket", "")
	if err == nil {
		t.Fatal("expected error for empty S3 key")
	}

	t.Log("PASS: invalid cleanup task input rejected")
}

func TestClaimNextS3CleanupTask(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	task, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatalf("claim task: %v", err)
	}

	if task == nil {
		t.Fatal("expected a task, got nil")
	}

	if task.ID != taskID {
		t.Fatalf("claimed task ID = %d; want %d", task.ID, taskID)
	}

	if task.Status != "PROCESSING" {
		t.Fatalf("status = %s; want PROCESSING", task.Status)
	}

	if task.Attempts != 1 {
		t.Fatalf("attempts = %d; want 1", task.Attempts)
	}

	// The same task must not be claimed again while processing.
	secondTask, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}

	if secondTask != nil {
		t.Fatalf("expected no second task, got ID %d", secondTask.ID)
	}

	t.Log("PASS: task claimed exactly once")
}

func TestMarkS3CleanupCompleted(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	task, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task == nil || task.ID != taskID {
		t.Fatal("failed to claim expected task")
	}

	if err := MarkS3CleanupCompleted(ctx, taskID, *task.LeaseToken); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	var status string
	err = DB.QueryRow(ctx, `
		SELECT status
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(&status)

	if err != nil {
		t.Fatal(err)
	}

	if status != "COMPLETED" {
		t.Fatalf("status = %s; want COMPLETED", status)
	}

	claimed, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if claimed != nil {
		t.Fatal("completed task should not be claimed again")
	}

	t.Log("PASS: completed task is not claimable")
}

func TestMarkS3CleanupFailedSchedulesRetry(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	task, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("expected task")
	}

	cleanupErr := fmt.Errorf("simulated S3 deletion failure")

	if err := MarkS3CleanupFailed(ctx, taskID, *task.LeaseToken, cleanupErr); err != nil {
		t.Fatalf("mark cleanup failed: %v", err)
	}

	var status, lastError string
	var attempts int
	var nextRetryAt time.Time

	err = DB.QueryRow(ctx, `
		SELECT status, attempts, next_retry_at, last_error
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(
		&status,
		&attempts,
		&nextRetryAt,
		&lastError,
	)

	if err != nil {
		t.Fatal(err)
	}

	if status != "PENDING" {
		t.Fatalf("status = %s; want PENDING", status)
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d; want 1", attempts)
	}

	if lastError != cleanupErr.Error() {
		t.Fatalf("last_error = %s; want %s", lastError, cleanupErr.Error())
	}

	// First retry should be scheduled approximately one second later.
	if time.Until(nextRetryAt) < 0 {
		t.Fatal("next retry should be in the future")
	}

	// Make it eligible immediately for the next claim.
	_, err = DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET next_retry_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, taskID)

	if err != nil {
		t.Fatal(err)
	}

	retriedTask, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if retriedTask == nil {
		t.Fatal("expected task to be claimable after retry scheduling")
	}

	if retriedTask.Attempts != 2 {
		t.Fatalf("attempts = %d; want 2", retriedTask.Attempts)
	}

	t.Log("PASS: failed task scheduled and claimed for retry")
}

func TestMarkS3CleanupFailedAfterMaxAttempts(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	_, err := DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET max_attempts = 2
		WHERE id = $1
	`, taskID)

	if err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		task, err := ClaimNextS3CleanupTask(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if task == nil {
			t.Fatalf("attempt %d: expected task", attempt)
		}

		if err := MarkS3CleanupFailed(
			ctx,
			taskID,
			*task.LeaseToken,
			fmt.Errorf("simulated failure %d", attempt),
		); err != nil {
			t.Fatal(err)
		}

		if attempt == 1 {
			_, err = DB.Exec(ctx, `
				UPDATE s3_cleanup_tasks
				SET next_retry_at = CURRENT_TIMESTAMP
				WHERE id = $1
			`, taskID)

			if err != nil {
				t.Fatal(err)
			}
		}
	}

	var status string
	err = DB.QueryRow(ctx, `
		SELECT status
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(&status)

	if err != nil {
		t.Fatal(err)
	}

	if status != "FAILED" {
		t.Fatalf("status = %s; want FAILED", status)
	}

	task, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if task != nil {
		t.Fatal("permanently failed task should not be claimable")
	}

	t.Log("PASS: task permanently failed after maximum attempts")
}

func TestConcurrentS3CleanupTaskClaim(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	const taskCount = 10
	const workerCount = 2

	for i := 0; i < taskCount; i++ {
		createTestS3CleanupTask(t, ctx)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	claimedIDs := make(map[int64]bool)
	var firstErr error

	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func() {
			defer wg.Done()

			for {
				task, err := ClaimNextS3CleanupTask(ctx)

				mu.Lock()

				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}

				if task == nil {
					mu.Unlock()
					return
				}

				if claimedIDs[task.ID] {
					if firstErr == nil {
						firstErr = fmt.Errorf(
							"task %d claimed more than once",
							task.ID,
						)
					}
					mu.Unlock()
					return
				}

				claimedIDs[task.ID] = true
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if firstErr != nil {
		t.Fatalf("concurrent claiming failed: %v", firstErr)
	}

	if len(claimedIDs) != taskCount {
		t.Fatalf(
			"claimed %d unique tasks; want %d",
			len(claimedIDs),
			taskCount,
		)
	}

	t.Logf(
		"PASS: %d tasks claimed exactly once by %d workers",
		taskCount,
		workerCount,
	)
}

func TestClaimStaleS3CleanupTask(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	// Simulate a worker claiming the task and then crashing.
	claimedTask, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatalf("initial claim: %v", err)
	}

	if claimedTask == nil {
		t.Fatal("expected task to be claimed")
	}

	if claimedTask.ID != taskID {
		t.Fatalf("claimed task ID = %d; want %d", claimedTask.ID, taskID)
	}

	if claimedTask.Attempts != 1 {
		t.Fatalf("initial attempts = %d; want 1", claimedTask.Attempts)
	}

	if claimedTask.LeaseToken == nil {
		t.Fatal("initial claim has no lease token")
	}

	oldToken := *claimedTask.LeaseToken

	// Simulate a worker crash by expiring its lease.
	_, err = DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 minute'
		WHERE id = $1
	`, taskID)

	if err != nil {
		t.Fatalf("make cleanup task stale: %v", err)
	}

	// The stale PROCESSING task should now be reclaimed.
	reclaimedTask, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatalf("reclaim stale cleanup task: %v", err)
	}

	if reclaimedTask == nil {
		t.Fatal("expected stale task to be reclaimed, got nil")
	}

	if reclaimedTask.ID != taskID {
		t.Fatalf("reclaimed task ID = %d; want %d", reclaimedTask.ID, taskID)
	}

	if reclaimedTask.Status != "PROCESSING" {
		t.Errorf("status = %s; want PROCESSING", reclaimedTask.Status)
	}

	if reclaimedTask.Attempts != 2 {
		t.Errorf("attempts = %d; want 2", reclaimedTask.Attempts)
	}

	if reclaimedTask.LeaseToken == nil {
		t.Fatal("reclaimed task has no lease token")
	}

	if *reclaimedTask.LeaseToken == oldToken {
		t.Fatal("reclaimed task must receive a new lease token")
	}

	if reclaimedTask.LeaseExpiresAt == nil {
		t.Fatal("reclaimed task has no lease expiry")
	}

	if !reclaimedTask.LeaseExpiresAt.After(time.Now()) {
		t.Fatal("reclaimed task lease should be in the future")
	}

	// Verify the persisted database state.
	var status string
	var attempts int

	err = DB.QueryRow(ctx, `
		SELECT status, attempts
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(&status, &attempts)

	if err != nil {
		t.Fatalf("verify reclaimed task: %v", err)
	}

	if status != "PROCESSING" {
		t.Errorf("database status = %s; want PROCESSING", status)
	}

	if attempts != 2 {
		t.Errorf("database attempts = %d; want 2", attempts)
	}

	t.Logf(
		"PASS: stale task reclaimed; task_id=%d, status=%s, attempts=%d",
		taskID,
		status,
		attempts,
	)
}

func TestOldLeaseTokenCannotCompleteReclaimedTask(t *testing.T) {
	ctx, taskID, oldToken, _ := setupReclaimedS3CleanupTask(t)

	err := MarkS3CleanupCompleted(ctx, taskID, oldToken)
	if err == nil {
		t.Fatal("expected old lease token completion to fail, got nil")
	}

	var status string
	err = DB.QueryRow(ctx, `
        SELECT status
        FROM s3_cleanup_tasks
        WHERE id = $1
    `, taskID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to read task status: %v", err)
	}

	if status != "PROCESSING" {
		t.Fatalf("expected status PROCESSING, got %s", status)
	}

	t.Log("PASS: old lease token cannot complete a reclaimed task")
}

func TestOldLeaseTokenCannotFailReclaimedTask(t *testing.T) {
	ctx, taskID, oldToken, _ := setupReclaimedS3CleanupTask(t)

	simulatedErr := errors.New("simulated S3 deletion failure")

	err := MarkS3CleanupFailed(ctx, taskID, oldToken, simulatedErr)
	if err == nil {
		t.Fatal("expected old lease token failure update to fail, got nil")
	}

	var status string
	err = DB.QueryRow(ctx, `
        SELECT status
        FROM s3_cleanup_tasks
        WHERE id = $1
    `, taskID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to read task status: %v", err)
	}

	if status != "PROCESSING" {
		t.Fatalf("expected status PROCESSING, got %s", status)
	}

	t.Log("PASS: old lease token cannot fail or reschedule a reclaimed task")
}

func TestClaimS3CleanupTaskWithNullLease(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	task, err := ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("expected task to be claimed")
	}

	_, err = DB.Exec(ctx, `
        UPDATE s3_cleanup_tasks
        SET lease_token = NULL,
            lease_expires_at = NULL
        WHERE id = $1
    `, taskID)

	if err == nil {
		t.Fatal("expected database to reject PROCESSING task without lease")
	}

	t.Log("PASS: PostgreSQL prevents PROCESSING tasks from having NULL leases")
}

func TestS3CleanupProcessingRequiresLease(t *testing.T) {
	ctx := setupS3CleanupTest(t)
	taskID := createTestS3CleanupTask(t, ctx)

	// Claim task to give it a valid PROCESSING lease.
	task, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task == nil || task.ID != taskID {
		t.Fatal("failed to claim expected task")
	}

	// Attempt to remove the lease while keeping PROCESSING status.
	_, err = DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_token = NULL,
		    lease_expires_at = NULL
		WHERE id = $1
	`, taskID)

	if err == nil {
		t.Fatal("expected database constraint to reject PROCESSING task without lease")
	}

	// Verify the original valid lease remains intact after rollback.
	var tokenPresent bool
	var expiryPresent bool

	err = DB.QueryRow(ctx, `
		SELECT lease_token IS NOT NULL,
		       lease_expires_at IS NOT NULL
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(&tokenPresent, &expiryPresent)

	if err != nil {
		t.Fatal(err)
	}

	if !tokenPresent || !expiryPresent {
		t.Fatal("valid lease should remain unchanged after rejected update")
	}

	t.Log("PASS: PostgreSQL rejects PROCESSING task without lease")
}

func TestConcurrentClaimS3CleanupTaskByID(t *testing.T) {
	ctx := setupS3CleanupTest(t)
	taskID := createTestS3CleanupTask(t, ctx)

	const claimantCount = 2

	var wg sync.WaitGroup
	var mu sync.Mutex

	var successfulClaims int
	var firstErr error

	wg.Add(claimantCount)

	for i := 0; i < claimantCount; i++ {
		go func() {
			defer wg.Done()

			task, err := ClaimS3CleanupTaskByID(ctx, taskID)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}

			if task != nil {
				successfulClaims++

				if task.ID != taskID {
					firstErr = fmt.Errorf(
						"claimed unexpected task ID: %d",
						task.ID,
					)
				}

				if task.LeaseToken == nil {
					firstErr = fmt.Errorf("claimed task has no lease token")
				}
			}
		}()
	}

	wg.Wait()

	if firstErr != nil {
		t.Fatalf("concurrent claim failed: %v", firstErr)
	}

	if successfulClaims != 1 {
		t.Fatalf(
			"successful claims = %d; want exactly 1",
			successfulClaims,
		)
	}

	var attempts int
	var status string

	err := DB.QueryRow(ctx, `
		SELECT attempts, status
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(&attempts, &status)

	if err != nil {
		t.Fatal(err)
	}

	if attempts != 1 {
		t.Fatalf("attempts = %d; want 1", attempts)
	}

	if status != "PROCESSING" {
		t.Fatalf("status = %s; want PROCESSING", status)
	}

	t.Log("PASS: task claimed exactly once by concurrent claimants")
}

func setupReclaimedS3CleanupTask(t *testing.T) (
	context.Context,
	int64,
	uuid.UUID,
	uuid.UUID,
) {
	t.Helper()

	ctx := setupS3CleanupTest(t)

	s3Key := fmt.Sprintf(
		"lease-ownership-tests/%s/%d.txt",
		t.Name(),
		time.Now().UnixNano(),
	)

	taskID, err := CreateS3CleanupTask(ctx, "test-bucket", s3Key)
	if err != nil {
		t.Fatalf("failed to create cleanup task: %v", err)
	}

	// Worker A claims the task.
	firstClaim, err := ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}
	if firstClaim == nil || firstClaim.LeaseToken == nil {
		t.Fatal("first claim did not return a lease token")
	}

	oldToken := *firstClaim.LeaseToken

	// Simulate Worker A's lease expiring.
	_, err = DB.Exec(ctx, `
        UPDATE s3_cleanup_tasks
        SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 second'
        WHERE id = $1
    `, taskID)
	if err != nil {
		t.Fatalf("failed to expire lease: %v", err)
	}

	// Worker B reclaims the expired task.
	secondClaim, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatalf("second claim failed: %v", err)
	}

	if secondClaim == nil {
		t.Fatal("expected expired task to be reclaimed, got nil")
	}

	if secondClaim.ID != taskID {
		t.Fatalf(
			"reclaimed wrong task: got ID %d, want %d",
			secondClaim.ID,
			taskID,
		)
	}

	if secondClaim.LeaseToken == nil {
		t.Fatal("reclaimed task has no lease token")
	}

	newToken := *secondClaim.LeaseToken

	if oldToken == newToken {
		t.Fatal("lease token was not rotated after reclaim")
	}

	return ctx, taskID, oldToken, newToken
}

func TestNewLeaseTokenCanCompleteReclaimedTask(t *testing.T) {
	ctx, taskID, _, newToken := setupReclaimedS3CleanupTask(t)

	err := MarkS3CleanupCompleted(ctx, taskID, newToken)
	if err != nil {
		t.Fatalf("new lease token should complete task: %v", err)
	}

	var status string
	var leaseTokenCleared bool
	var leaseExpiryCleared bool

	err = DB.QueryRow(ctx, `
		SELECT
			status,
			lease_token IS NULL,
			lease_expires_at IS NULL
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID).Scan(&status, &leaseTokenCleared, &leaseExpiryCleared)

	if err != nil {
		t.Fatalf("failed to read completed task: %v", err)
	}

	if status != "COMPLETED" {
		t.Fatalf("expected status COMPLETED, got %s", status)
	}

	if !leaseTokenCleared || !leaseExpiryCleared {
		t.Fatal("expected completed task to have its lease fields cleared")
	}

	t.Log("PASS: new lease token can complete a reclaimed task")
}

func TestClaimS3CleanupTaskByID(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	// First claim should succeed.
	task, err := ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to claim task by ID: %v", err)
	}

	if task == nil {
		t.Fatal("expected task, got nil")
	}

	if task.ID != taskID {
		t.Errorf("expected task ID %d, got %d", taskID, task.ID)
	}

	if task.Status != "PROCESSING" {
		t.Errorf("expected status PROCESSING, got %s", task.Status)
	}

	if task.Attempts != 1 {
		t.Errorf("expected attempts 1, got %d", task.Attempts)
	}

	if task.LeaseToken == nil {
		t.Fatal("expected lease token, got nil")
	}

	if task.LeaseExpiresAt == nil {
		t.Fatal("expected lease expiry, got nil")
	}

	// Second claim should return nil because task is already claimed.
	secondTask, err := ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		t.Fatalf("unexpected error on second claim: %v", err)
	}

	if secondTask != nil {
		t.Fatal("expected nil for an already claimed task")
	}
}

func TestClaimS3CleanupTaskByIDRejectsInvalidID(t *testing.T) {
	ctx := context.Background()

	invalidIDs := []int64{0, -1}

	for _, id := range invalidIDs {
		_, err := ClaimS3CleanupTaskByID(ctx, id)
		if err == nil {
			t.Errorf("expected error for invalid task ID %d", id)
		}
	}
}
