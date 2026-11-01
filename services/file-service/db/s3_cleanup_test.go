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
	ctx := setupS3CleanupTest(t)
	taskID := createTestS3CleanupTask(t, ctx)

	// Worker A claims the task.
	workerA, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if workerA == nil || workerA.ID != taskID || workerA.LeaseToken == nil {
		t.Fatal("worker A failed to claim expected task")
	}
	oldToken := *workerA.LeaseToken

	// Expire worker A's lease.
	_, err = DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 minute'
		WHERE id = $1
	`, taskID)
	if err != nil {
		t.Fatal(err)
	}

	// Worker B reclaims the task.
	workerB, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if workerB == nil || workerB.ID != taskID || workerB.LeaseToken == nil {
		t.Fatal("worker B failed to reclaim expected task")
	}

	newToken := *workerB.LeaseToken
	if oldToken == newToken {
		t.Fatal("worker B must receive a new lease token")
	}

	// Worker A must no longer be allowed to complete the task.
	err = MarkS3CleanupCompleted(ctx, taskID, oldToken)
	if err == nil {
		t.Fatal("expected old worker's completion to be rejected")
	}

	// Verify worker B still owns the task.
	var status string
	var tokenMatches bool

	err = DB.QueryRow(ctx, `
		SELECT status, lease_token = $2
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID, newToken).Scan(&status, &tokenMatches)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" {
		t.Fatalf("status = %s; want PROCESSING", status)
	}
	if !tokenMatches {
		t.Fatal("worker B's lease token was unexpectedly changed")
	}

	// Worker B should be able to complete it.
	if err := MarkS3CleanupCompleted(ctx, taskID, newToken); err != nil {
		t.Fatalf("worker B should be able to complete task: %v", err)
	}

	err = DB.QueryRow(ctx, `
		SELECT status FROM s3_cleanup_tasks WHERE id = $1
	`, taskID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "COMPLETED" {
		t.Fatalf("status = %s; want COMPLETED", status)
	}

	t.Log("PASS: old lease token rejected; new lease token completed task")
}

func TestOldLeaseTokenCannotFailReclaimedTask(t *testing.T) {
	ctx := setupS3CleanupTest(t)
	taskID := createTestS3CleanupTask(t, ctx)

	// Worker A claims the task.
	workerA, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if workerA == nil || workerA.ID != taskID || workerA.LeaseToken == nil {
		t.Fatal("worker A failed to claim expected task")
	}
	oldToken := *workerA.LeaseToken

	// Expire worker A's lease.
	_, err = DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_expires_at = CURRENT_TIMESTAMP - INTERVAL '1 minute'
		WHERE id = $1
	`, taskID)
	if err != nil {
		t.Fatal(err)
	}

	// Worker B reclaims the task.
	workerB, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if workerB == nil || workerB.ID != taskID || workerB.LeaseToken == nil {
		t.Fatal("worker B failed to reclaim expected task")
	}

	newToken := *workerB.LeaseToken
	if oldToken == newToken {
		t.Fatal("worker B must receive a new lease token")
	}

	// Worker A must no longer be allowed to mark the task failed.
	err = MarkS3CleanupFailed(
		ctx,
		taskID,
		oldToken,
		fmt.Errorf("failure from stale worker A"),
	)
	if err == nil {
		t.Fatal("expected old worker's failure update to be rejected")
	}

	// Verify worker B still owns the task.
	var status string
	var tokenMatches bool

	err = DB.QueryRow(ctx, `
		SELECT status, lease_token = $2
		FROM s3_cleanup_tasks
		WHERE id = $1
	`, taskID, newToken).Scan(&status, &tokenMatches)
	if err != nil {
		t.Fatal(err)
	}

	if status != "PROCESSING" {
		t.Fatalf("status = %s; want PROCESSING", status)
	}
	if !tokenMatches {
		t.Fatal("worker B's lease token was unexpectedly changed")
	}

	// Worker B should be able to mark the task failed.
	if err := MarkS3CleanupFailed(
		ctx,
		taskID,
		newToken,
		fmt.Errorf("failure from current worker B"),
	); err != nil {
		t.Fatalf("worker B should be able to mark task failed: %v", err)
	}

	err = DB.QueryRow(ctx, `
		SELECT status FROM s3_cleanup_tasks WHERE id = $1
	`, taskID).Scan(&status)
	if err != nil {
		t.Fatal(err)
	}
	if status != "PENDING" {
		t.Fatalf("status = %s; want PENDING", status)
	}

	t.Log("PASS: old lease token rejected; new lease token failed task successfully")
}

func TestClaimS3CleanupTaskWithNullLease(t *testing.T) {
	ctx := setupS3CleanupTest(t)

	taskID := createTestS3CleanupTask(t, ctx)

	// First claim: simulate an older worker claiming the task.
	task, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if task == nil || task.ID != taskID {
		t.Fatal("failed to claim expected task")
	}

	// Simulate a legacy PROCESSING row from before lease columns existed.
	_, err = DB.Exec(ctx, `
		UPDATE s3_cleanup_tasks
		SET lease_token = NULL,
		    lease_expires_at = NULL
		WHERE id = $1
	`, taskID)
	if err != nil {
		t.Fatal(err)
	}

	// The NULL lease should be treated as expired and reclaimed.
	reclaimed, err := ClaimNextS3CleanupTask(ctx)
	if err != nil {
		t.Fatalf("claim task with NULL lease: %v", err)
	}

	if reclaimed == nil {
		t.Fatal("expected task with NULL lease to be reclaimed")
	}

	if reclaimed.ID != taskID {
		t.Fatalf("reclaimed ID = %d; want %d", reclaimed.ID, taskID)
	}

	if reclaimed.Status != "PROCESSING" {
		t.Fatalf("status = %s; want PROCESSING", reclaimed.Status)
	}

	if reclaimed.Attempts != 2 {
		t.Fatalf("attempts = %d; want 2", reclaimed.Attempts)
	}

	if reclaimed.LeaseToken == nil {
		t.Fatal("reclaimed task should have a fresh lease token")
	}

	if reclaimed.LeaseExpiresAt == nil {
		t.Fatal("reclaimed task should have a lease expiry")
	}

	if !reclaimed.LeaseExpiresAt.After(time.Now()) {
		t.Fatal("reclaimed lease expiry should be in the future")
	}

	t.Log("PASS: legacy PROCESSING task with NULL lease successfully reclaimed")
}
