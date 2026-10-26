package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func TestConcurrentUploadAndFolderDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load the service .env when DATABASE_URL isn't already set.
	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set; configure it in the environment or services/file-service/.env")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	// Use a dedicated, unique user ID for disposable test data.
	userID := time.Now().UnixNano()
	rootPublicID := fmt.Sprintf("t%xA", userID)
	childPublicID := fmt.Sprintf("t%xB", userID)
	filePublicID := fmt.Sprintf("t%xC", userID)

	// Preserve and restore the package-level DB connection.
	previousDB := DB
	DB = pool
	defer func() {
		DB = previousDB
	}()

	// Cleanup only this test user's data and its outbox events.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE aggregate_id = ANY($1::varchar[])
		`, []string{rootPublicID, childPublicID, filePublicID})

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM entries WHERE user_id = $1
		`, userID)

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage WHERE user_id = $1
		`, userID)
	})

	// Create an isolated quota record.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, $2, 0, 0)
	`, userID, int64(1<<30))
	if err != nil {
		t.Fatalf("create test storage record: %v", err)
	}

	// Create hierarchy A -> B.
	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'folder-A', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatalf("create root folder: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'folder-B', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatalf("create child folder: %v", err)
	}

	// Reserve quota before starting the competing operations.
	const fileSize int64 = 123

	if err := ReserveStorage(ctx, userID, fileSize); err != nil {
		t.Fatalf("reserve upload quota: %v", err)
	}

	parentID := childID
	now := time.Now().UTC()

	file := &models.EntryData{
		PublicId:    filePublicID,
		UserId:      userID,
		ParentId:    &parentID,
		Name:        "race-test.txt",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   "txt",
		Size:        fileSize,
		S3Key:       sql.NullString{String: "concurrency-test-key", Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Release both goroutines at approximately the same time.
	start := make(chan struct{})
	uploadResult := make(chan error, 1)
	deleteResult := make(chan error, 1)

	go func() {
		<-start
		_, err := InsertEntryAndFinalizeStorage(ctx, file)
		uploadResult <- err
	}()

	go func() {
		<-start
		deleteResult <- DeleteFolder(ctx, rootPublicID, userID)
	}()

	close(start)

	uploadErr := <-uploadResult
	deleteErr := <-deleteResult

	// Upload can succeed before deletion, or fail because deletion
	// won the race and made its destination hierarchy inactive.
	if uploadErr != nil {
		// A failed upload transaction must not leave reserved quota behind.
		releaseErr := ReleaseStorageReservation(ctx, userID, fileSize)
		if releaseErr != nil {
			t.Fatalf("release reservation after failed upload: %v", releaseErr)
		}

		if !errors.Is(uploadErr, ErrEntryNotFound) {
			t.Fatalf("unexpected upload error: %v", uploadErr)
		}
	}

	if deleteErr != nil {
		t.Fatalf("folder deletion failed: %v", deleteErr)
	}

	// Verify that the root folder was soft-deleted.
	var rootDeleted bool
	err = pool.QueryRow(ctx, `
		SELECT deleted_at IS NOT NULL
		FROM entries
		WHERE id = $1 AND user_id = $2
	`, rootID, userID).Scan(&rootDeleted)
	if err != nil {
		t.Fatalf("check root deletion: %v", err)
	}
	if !rootDeleted {
		t.Fatal("root folder is still active")
	}

	// No active entry may remain anywhere in the deleted subtree.
	var activeDescendants int64
	err = pool.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id
			FROM entries
			WHERE id = $1 AND user_id = $2

			UNION ALL

			SELECT e.id
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
		)
		SELECT COUNT(*)
		FROM entries e
		JOIN descendants d ON d.id = e.id
		WHERE e.deleted_at IS NULL
	`, rootID, userID).Scan(&activeDescendants)
	if err != nil {
		t.Fatalf("check active descendants: %v", err)
	}
	if activeDescendants != 0 {
		t.Fatalf("found %d active entries inside a deleted folder", activeDescendants)
	}

	// Verify quota against the actual active-file metadata.
	var usedBytes, reservedBytes, activeFileBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatalf("read quota: %v", err)
	}

	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(size), 0)
		FROM entries
		WHERE user_id = $1
		  AND type = 'FILE'
		  AND deleted_at IS NULL
	`, userID).Scan(&activeFileBytes)
	if err != nil {
		t.Fatalf("calculate active file bytes: %v", err)
	}

	if usedBytes != activeFileBytes {
		t.Fatalf("quota mismatch: used_bytes=%d, active file bytes=%d",
			usedBytes, activeFileBytes)
	}
	if reservedBytes != 0 {
		t.Fatalf("reservation leaked: reserved_bytes=%d", reservedBytes)
	}

	t.Logf(
		"PASS: upload error=%v, deleted subtree has no active entries, used=%d, reserved=%d",
		uploadErr,
		usedBytes,
		reservedBytes,
	)
}

func TestUploadLockFirstThenFolderDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	previousDB := DB

	appName := fmt.Sprintf("test_delete_%d", time.Now().UnixNano())

	deletePoolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}

	if deletePoolConfig.ConnConfig.RuntimeParams == nil {
		deletePoolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	deletePoolConfig.ConnConfig.RuntimeParams["application_name"] = appName

	deletePool, err := pgxpool.NewWithConfig(ctx, deletePoolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer deletePool.Close()

	if err := deletePool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	// DeleteFolder will use the uniquely identifiable pool.
	DB = deletePool
	defer func() { DB = previousDB }()

	userID := time.Now().UnixNano()
	rootPublicID := fmt.Sprintf("t%019d", userID)
	childPublicID := fmt.Sprintf("c%019d", userID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{rootPublicID, childPublicID},
		)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM entries WHERE user_id = $1`, userID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM user_storage WHERE user_id = $1`, userID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, 1000000, 0, 0)
	`, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'root', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate an upload transaction holding the destination hierarchy lock.
	uploadTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var uploadPID int
	if err := uploadTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&uploadPID); err != nil {
		_ = uploadTx.Rollback(ctx)
		t.Fatal(err)
	}

	if err := LockEntryHierarchy(uploadTx, ctx, userID, childID, "FOLDER", false); err != nil {
		_ = uploadTx.Rollback(ctx)
		t.Fatalf("upload could not lock destination: %v", err)
	}

	deleteStarted := make(chan struct{})
	deleteDone := make(chan error, 1)

	go func() {
		close(deleteStarted)
		deleteDone <- DeleteFolder(ctx, rootPublicID, userID)
	}()

	<-deleteStarted

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	blocked := false

	for !blocked {
		select {
		case deleteErr := <-deleteDone:
			_ = uploadTx.Rollback(ctx)
			t.Fatalf(
				"folder deletion finished before upload released its lock: %v",
				deleteErr,
			)

		case <-ticker.C:
			err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND application_name = $1
				  AND state = 'active'
				  AND $2 = ANY(pg_blocking_pids(pid))
			)
		`, appName, uploadPID).Scan(&blocked)

			if err != nil {
				_ = uploadTx.Rollback(ctx)
				t.Fatalf("check deletion lock blocker: %v", err)
			}

		case <-ctx.Done():
			_ = uploadTx.Rollback(ctx)
			t.Fatal("timed out waiting for deletion to block on upload transaction")
		}
	}

	t.Log("PASS: PostgreSQL confirms deletion is blocked by the upload transaction")

	if err := uploadTx.Commit(ctx); err != nil {
		t.Fatalf("commit upload-side transaction: %v", err)
	}

	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatalf("folder deletion failed after lock release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("folder deletion did not finish after upload released its lock")
	}

	var activeCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL
	`, userID).Scan(&activeCount)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("expected no active entries, got %d", activeCount)
	}
}

func TestFolderDeletionLockFirstThenUploadRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	previousDB := DB

	appName := fmt.Sprintf("test_upload_%d", time.Now().UnixNano())

	uploadPoolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if uploadPoolConfig.ConnConfig.RuntimeParams == nil {
		uploadPoolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	uploadPoolConfig.ConnConfig.RuntimeParams["application_name"] = appName

	uploadPool, err := pgxpool.NewWithConfig(ctx, uploadPoolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadPool.Close()

	if err := uploadPool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	DB = pool
	defer func() { DB = previousDB }()

	userID := time.Now().UnixNano()
	rootPublicID := fmt.Sprintf("t%019d", userID)
	childPublicID := fmt.Sprintf("c%019d", userID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{rootPublicID, childPublicID},
		)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM entries WHERE user_id = $1`, userID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM user_storage WHERE user_id = $1`, userID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, 1000000, 0, 0)
	`, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'root', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	// Hold an exclusive lock on the root, as the deletion flow does.
	deleteTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var deletePID int
	if err := deleteTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&deletePID); err != nil {
		_ = deleteTx.Rollback(ctx)
		t.Fatal(err)
	}

	if err := LockEntryHierarchy(deleteTx, ctx, userID, rootID, "FOLDER", true); err != nil {
		_ = deleteTx.Rollback(ctx)
		t.Fatalf("deletion could not lock root: %v", err)
	}

	uploadDone := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		uploadTx, err := uploadPool.Begin(ctx)
		if err != nil {
			uploadDone <- err
			return
		}
		defer uploadTx.Rollback(ctx)

		err = LockEntryHierarchy(uploadTx, ctx, userID, childID, "FOLDER", false)
		if err != nil {
			uploadDone <- err
			return
		}

		uploadDone <- uploadTx.Commit(ctx)
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	blocked := false

	for !blocked {
		select {
		case err := <-uploadDone:
			_ = deleteTx.Rollback(ctx)
			t.Fatalf("upload finished before deletion released its lock: %v", err)

		case <-ticker.C:
			err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE datname = current_database()
				  AND application_name = $1
				  AND state = 'active'
				  AND $2 = ANY(pg_blocking_pids(pid))
			)
		`, appName, deletePID).Scan(&blocked)

			if err != nil {
				_ = deleteTx.Rollback(ctx)
				t.Fatalf("check upload lock blocker: %v", err)
			}

		case <-ctx.Done():
			_ = deleteTx.Rollback(ctx)
			t.Fatal("timed out waiting for upload to block on deletion lock")
		}
	}

	t.Log("PASS: PostgreSQL confirms upload is blocked by deletion transaction")

	if _, err := deleteTx.Exec(ctx, `
		UPDATE entries
		SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, rootID, userID); err != nil {
		_ = deleteTx.Rollback(ctx)
		t.Fatal(err)
	}

	if err := deleteTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	err = <-uploadDone
	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound after deletion, got: %v", err)
	}

	var activeCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1 AND deleted_at IS NULL
	`, userID).Scan(&activeCount)
	if err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected child folder to remain active in this isolated test, got %d active entries", activeCount)
	}
}

func TestConcurrentInsertEntryAndFinalizeStorageVsDeleteFolder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	previousDB := DB
	DB = pool
	defer func() { DB = previousDB }()

	userID := time.Now().UnixNano()
	suffix := fmt.Sprintf("%019d", userID%1000000000000000000)
	rootPublicID := "r" + suffix
	childPublicID := "c" + suffix
	filePublicID := "f" + suffix

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{rootPublicID, childPublicID, filePublicID},
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`,
			userID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, 1000000, 0, 0)
	`, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'root', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	const fileSize int64 = 123

	if err := ReserveStorage(ctx, userID, fileSize); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	parentID := childID

	file := &models.EntryData{
		PublicId:    filePublicID,
		UserId:      userID,
		ParentId:    &parentID,
		Name:        "race-test.txt",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   "txt",
		Size:        fileSize,
		S3Key:       sql.NullString{String: "test/" + filePublicID, Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	start := make(chan struct{})
	uploadDone := make(chan error, 1)
	deleteDone := make(chan error, 1)

	go func() {
		<-start
		_, err := InsertEntryAndFinalizeStorage(ctx, file)
		uploadDone <- err
	}()

	go func() {
		<-start
		deleteDone <- DeleteFolder(ctx, rootPublicID, userID)
	}()

	close(start)

	uploadErr := <-uploadDone
	deleteErr := <-deleteDone

	if deleteErr != nil {
		// If deletion failed, clean up any outstanding reservation.
		_ = ReleaseStorageReservation(ctx, userID, fileSize)
		t.Fatalf("DeleteFolder failed: %v", deleteErr)
	}

	if uploadErr != nil {
		// The upload transaction did not finalize the reservation.
		if releaseErr := ReleaseStorageReservation(ctx, userID, fileSize); releaseErr != nil {
			t.Fatalf("upload failed (%v), and reservation cleanup failed: %v",
				uploadErr, releaseErr)
		}
		if !errors.Is(uploadErr, ErrEntryNotFound) {
			t.Fatalf("unexpected upload error: %v", uploadErr)
		}
	}

	var activeDescendants int64
	err = pool.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id
			FROM entries
			WHERE id = $1 AND user_id = $2

			UNION ALL

			SELECT e.id
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
		)
		SELECT COUNT(*)
		FROM entries e
		JOIN descendants d ON d.id = e.id
		WHERE e.deleted_at IS NULL
	`, rootID, userID).Scan(&activeDescendants)
	if err != nil {
		t.Fatal(err)
	}
	if activeDescendants != 0 {
		t.Fatalf("found %d active entries under deleted root", activeDescendants)
	}

	var usedBytes, reservedBytes, activeFileBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(size), 0)
		FROM entries
		WHERE user_id = $1
		  AND type = 'FILE'
		  AND deleted_at IS NULL
	`, userID).Scan(&activeFileBytes)
	if err != nil {
		t.Fatal(err)
	}

	if usedBytes != activeFileBytes {
		t.Fatalf("quota mismatch: used=%d active_file_bytes=%d",
			usedBytes, activeFileBytes)
	}
	if reservedBytes != 0 {
		t.Fatalf("reservation leak: reserved_bytes=%d", reservedBytes)
	}

	t.Logf("PASS: upload error=%v, used=%d, reserved=%d",
		uploadErr, usedBytes, reservedBytes)
}

func TestSuccessfulUploadThenFolderDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	previousDB := DB
	DB = pool
	defer func() { DB = previousDB }()

	userID := time.Now().UnixNano()
	suffix := fmt.Sprintf("%019d", userID%1000000000000000000)
	rootPublicID := "r" + suffix
	childPublicID := "c" + suffix
	filePublicID := "f" + suffix

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{rootPublicID, childPublicID, filePublicID},
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`,
			userID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, 1000000, 0, 0)
	`, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'root', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	const fileSize int64 = 123
	if err := ReserveStorage(ctx, userID, fileSize); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	parentID := childID

	file := &models.EntryData{
		PublicId:    filePublicID,
		UserId:      userID,
		ParentId:    &parentID,
		Name:        "successful-upload.txt",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   "txt",
		Size:        fileSize,
		S3Key:       sql.NullString{String: "test/" + filePublicID, Valid: true},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Phase 1: upload must succeed before deletion begins.
	newID, err := InsertEntryAndFinalizeStorage(ctx, file)
	if err != nil {
		_ = ReleaseStorageReservation(ctx, userID, fileSize)
		t.Fatalf("expected upload to succeed, got: %v", err)
	}

	var usedBytes, reservedBytes int64
	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatal(err)
	}
	if usedBytes != fileSize || reservedBytes != 0 {
		t.Fatalf("after upload: used=%d reserved=%d; want used=%d reserved=0",
			usedBytes, reservedBytes, fileSize)
	}

	// Confirm FILE_CREATED was inserted into the outbox.
	var createdEvents int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_id = $1
		  AND event_type = 'FILE_CREATED'
	`, filePublicID).Scan(&createdEvents)
	if err != nil {
		t.Fatal(err)
	}
	if createdEvents != 1 {
		t.Fatalf("expected one FILE_CREATED outbox event, got %d", createdEvents)
	}

	// Phase 2: deletion begins only after successful upload commit.
	if err := DeleteFolder(ctx, rootPublicID, userID); err != nil {
		t.Fatalf("delete folder after successful upload: %v", err)
	}

	// Verify FOLDER_DELETED was recorded in the outbox.
	var deletedEvents int

	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_id = $1
		AND event_type = 'FOLDER_DELETED'
	`, rootPublicID).Scan(&deletedEvents)

	if err != nil {
		t.Fatal(err)
	}

	if deletedEvents != 1 {
		t.Fatalf("expected one FOLDER_DELETED outbox event, got %d", deletedEvents)
	}

	var fileDeleted bool
	err = pool.QueryRow(ctx, `
		SELECT deleted_at IS NOT NULL
		FROM entries
		WHERE id = $1 AND user_id = $2
	`, newID, userID).Scan(&fileDeleted)
	if err != nil {
		t.Fatal(err)
	}
	if !fileDeleted {
		t.Fatal("uploaded file should be soft-deleted with its parent folder")
	}

	err = pool.QueryRow(ctx, `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes, &reservedBytes)
	if err != nil {
		t.Fatal(err)
	}
	if usedBytes != 0 || reservedBytes != 0 {
		t.Fatalf("after folder deletion: used=%d reserved=%d; want both zero",
			usedBytes, reservedBytes)
	}

	t.Logf("PASS: entry_id=%d; upload committed, FILE_CREATED outbox recorded, folder deletion released quota",
		newID)
}

func TestConcurrentCreateFolderAndDeleteAncestor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	previousDB := DB
	DB = pool
	defer func() { DB = previousDB }()

	userID := time.Now().UnixNano()
	suffix := fmt.Sprintf("%019d", userID%1000000000000000000)
	rootPublicID := "r" + suffix
	childPublicID := "b" + suffix
	newFolderPublicID := "c" + suffix

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{rootPublicID, childPublicID, newFolderPublicID},
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`,
			userID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, 1000000, 0, 0)
	`, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'root', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	createDone := make(chan error, 1)
	deleteDone := make(chan error, 1)

	go func() {
		<-start

		parentID := childID
		now := time.Now().UTC()

		newFolder := &models.EntryData{
			PublicId:  newFolderPublicID,
			UserId:    userID,
			ParentId:  &parentID,
			Name:      "nested-folder-C",
			Type:      "FOLDER",
			Extension: "",
			CreatedAt: now,
			UpdatedAt: now,
		}

		createDone <- InsertEntryData(ctx, newFolder)
	}()

	go func() {
		<-start
		deleteDone <- DeleteFolder(ctx, rootPublicID, userID)
	}()

	close(start)

	createErr := <-createDone
	deleteErr := <-deleteDone

	if deleteErr != nil {
		t.Fatalf("DeleteFolder failed: %v", deleteErr)
	}

	// Creation can succeed before deletion, or be rejected if deletion wins.
	if createErr != nil && !errors.Is(createErr, ErrEntryNotFound) {
		t.Fatalf("unexpected folder creation error: %v", createErr)
	}

	var activeDescendants int64
	err = pool.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id
			FROM entries
			WHERE id = $1 AND user_id = $2

			UNION ALL

			SELECT e.id
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
		)
		SELECT COUNT(*)
		FROM entries e
		JOIN descendants d ON d.id = e.id
		WHERE e.deleted_at IS NULL
	`, rootID, userID).Scan(&activeDescendants)
	if err != nil {
		t.Fatal(err)
	}

	if activeDescendants != 0 {
		t.Fatalf("found %d active entries under deleted root", activeDescendants)
	}

	t.Logf("PASS: folder creation error=%v; deleted hierarchy has no active entries",
		createErr)
}

func TestSuccessfulFolderCreationThenAncestorDeletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	previousDB := DB
	DB = pool
	defer func() { DB = previousDB }()

	userID := time.Now().UnixNano()
	suffix := fmt.Sprintf("%019d", userID%1000000000000000000)
	rootPublicID := "r" + suffix
	childPublicID := "b" + suffix
	newFolderPublicID := "c" + suffix

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = ANY($1::varchar[])`,
			[]string{rootPublicID, childPublicID, newFolderPublicID},
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`,
			userID,
		)
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (user_id, quota_bytes, used_bytes, reserved_bytes)
		VALUES ($1, 1000000, 0, 0)
	`, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID, childID int64

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension)
		VALUES ($1, $2, 'root', 'FOLDER', '')
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, parent_id, name, type, extension)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '')
		RETURNING id
	`, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	// Phase 1: create the nested folder and commit successfully.
	parentID := childID
	now := time.Now().UTC()

	newFolder := &models.EntryData{
		PublicId:  newFolderPublicID,
		UserId:    userID,
		ParentId:  &parentID,
		Name:      "nested-folder-C",
		Type:      "FOLDER",
		Extension: "",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := InsertEntryData(ctx, newFolder); err != nil {
		t.Fatalf("expected folder creation to succeed: %v", err)
	}

	var newFolderID int64
	err = pool.QueryRow(ctx, `
		SELECT id
		FROM entries
		WHERE public_id = $1
		  AND user_id = $2
		  AND deleted_at IS NULL
	`, newFolderPublicID, userID).Scan(&newFolderID)
	if err != nil {
		t.Fatalf("created folder not found: %v", err)
	}

	// Phase 2: delete the ancestor after folder creation commits.
	if err := DeleteFolder(ctx, rootPublicID, userID); err != nil {
		t.Fatalf("delete ancestor after folder creation: %v", err)
	}

	// The newly created folder must now be soft-deleted.
	var isDeleted bool
	err = pool.QueryRow(ctx, `
		SELECT deleted_at IS NOT NULL
		FROM entries
		WHERE id = $1 AND user_id = $2
	`, newFolderID, userID).Scan(&isDeleted)
	if err != nil {
		t.Fatal(err)
	}
	if !isDeleted {
		t.Fatal("newly created folder should be soft-deleted with its ancestor")
	}

	// Verify FOLDER_DELETED was recorded for the root.
	var deletedEvents int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_id = $1
		  AND event_type = 'FOLDER_DELETED'
	`, rootPublicID).Scan(&deletedEvents)
	if err != nil {
		t.Fatal(err)
	}
	if deletedEvents != 1 {
		t.Fatalf("expected one FOLDER_DELETED event, got %d", deletedEvents)
	}

	// No active entry should remain under the deleted root.
	var activeDescendants int64
	err = pool.QueryRow(ctx, `
		WITH RECURSIVE descendants AS (
			SELECT id
			FROM entries
			WHERE id = $1 AND user_id = $2

			UNION ALL

			SELECT e.id
			FROM entries e
			JOIN descendants d ON e.parent_id = d.id
			WHERE e.user_id = $2
		)
		SELECT COUNT(*)
		FROM entries e
		JOIN descendants d ON d.id = e.id
		WHERE e.deleted_at IS NULL
	`, rootID, userID).Scan(&activeDescendants)
	if err != nil {
		t.Fatal(err)
	}
	if activeDescendants != 0 {
		t.Fatalf("found %d active descendants after ancestor deletion", activeDescendants)
	}

	t.Logf("PASS: folder %d created, ancestor deleted, FOLDER_DELETED event recorded",
		newFolderID)
}
