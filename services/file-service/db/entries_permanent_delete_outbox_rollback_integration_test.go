package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
)

func TestPermanentlyDeleteEntry_OutboxFailureRollsBack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := setupS3CleanupTest(t)

	// Preserve the existing application configuration.
	previousConfig := config.AppConfig
	config.AppConfig = &config.Config{
		BucketName: "test-bucket",
	}

	userID := time.Now().UnixNano()
	publicID := fmt.Sprintf("f%019d", userID%1_000_000_000_000_000_000)
	s3Key := fmt.Sprintf("test/outbox-rollback/%d.txt", userID)

	const fileSize int64 = 500

	triggerName := fmt.Sprintf("reject_cleanup_%d", userID)
	functionName := fmt.Sprintf("reject_cleanup_fn_%d", userID)

	// Cleanup runs before setupS3CleanupTest closes the DB pool.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		_, _ = DB.Exec(cleanupCtx, fmt.Sprintf(
			`DROP TRIGGER IF EXISTS %s ON outbox_events`,
			triggerName,
		))

		_, _ = DB.Exec(cleanupCtx, fmt.Sprintf(
			`DROP FUNCTION IF EXISTS %s()`,
			functionName,
		))

		_, _ = DB.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE aggregate_id = $1
		`, publicID)

		_, _ = DB.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
		`, userID)

		_, _ = DB.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, userID)

		config.AppConfig = previousConfig
	})

	// 1. Create user storage with existing used quota.
	_, err := DB.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes
		)
		VALUES ($1, 1000000, $2, 0)
	`, userID, fileSize)

	if err != nil {
		t.Fatalf("create user storage: %v", err)
	}

	// 2. Create a file entry.
	var entryID int64

	err = DB.QueryRow(ctx, `
		INSERT INTO entries (
			public_id,
			user_id,
			parent_id,
			name,
			type,
			content_type,
			extension,
			size,
			s3_key,
			deleted_at
		)
		VALUES (
			$1, $2, NULL, 'rollback-test',
			'FILE', 'text/plain', 'txt',
			$3, $4, CURRENT_TIMESTAMP
		)
		RETURNING id
	`, publicID, userID, fileSize, s3Key).Scan(&entryID)

	if err != nil {
		t.Fatalf("create trashed file: %v", err)
	}

	// 3. Create a trigger that deliberately rejects S3_CLEANUP events.
	_, err = DB.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s()
		RETURNS TRIGGER
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.event_type = 'S3_CLEANUP' THEN
				RAISE EXCEPTION 'simulated S3 cleanup outbox failure';
			END IF;

			RETURN NEW;
		END;
		$$
	`, functionName))

	if err != nil {
		t.Fatalf("create trigger function: %v", err)
	}

	_, err = DB.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER %s
		BEFORE INSERT ON outbox_events
		FOR EACH ROW
		EXECUTE FUNCTION %s()
	`, triggerName, functionName))

	if err != nil {
		t.Fatalf("create outbox trigger: %v", err)
	}

	// 4. Attempt permanent deletion.
	// Entry deletion and quota release happen before outbox insertion.
	err = PermanentlyDeleteEntry(ctx, publicID, userID)

	if err == nil {
		t.Fatal("expected permanent deletion to fail because outbox insertion is rejected")
	}

	t.Logf("received expected permanent deletion error: %v", err)

	// 5. Verify the entry still exists.
	var entryCount int

	err = DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE id = $1
		  AND user_id = $2
	`, entryID, userID).Scan(&entryCount)

	if err != nil {
		t.Fatalf("check entry existence: %v", err)
	}

	if entryCount != 1 {
		t.Fatalf(
			"entry count = %d; want 1 (transaction should rollback)",
			entryCount,
		)
	}

	// 6. Verify quota was not released.
	var usedBytes, reservedBytes int64

	err = DB.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)

	if err != nil {
		t.Fatalf("read storage usage: %v", err)
	}

	if usedBytes != fileSize {
		t.Fatalf("used_bytes = %d; want %d", usedBytes, fileSize)
	}

	if reservedBytes != 0 {
		t.Fatalf("reserved_bytes = %d; want 0", reservedBytes)
	}

	// 7. Verify no cleanup event was committed.
	var eventCount int

	err = DB.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_id = $1
		  AND event_type = 'S3_CLEANUP'
	`, publicID).Scan(&eventCount)

	if err != nil {
		t.Fatalf("check cleanup outbox events: %v", err)
	}

	if eventCount != 0 {
		t.Fatalf("cleanup event count = %d; want 0", eventCount)
	}

	t.Log(
		"PASS: outbox failure rolled back permanent deletion, quota release and cleanup event",
	)
}
