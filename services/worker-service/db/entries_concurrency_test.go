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

func TestConcurrentRecalculateAncestorFolderSizes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
		_ = godotenv.Load("../../.env")
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
	rootPublicID := fmt.Sprintf("r%019d", userID)
	childPublicID := fmt.Sprintf("c%019d", userID)
	filePublicID1 := fmt.Sprintf("f%018d", userID)
	filePublicID2 := fmt.Sprintf("g%018d", userID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`, userID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO entries
			(public_id, user_id, name, type, extension, size)
		VALUES ($1, $2, 'root', 'FOLDER', '', 0)
	`, rootPublicID, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID int64
	err = pool.QueryRow(ctx, `
		SELECT id FROM entries WHERE user_id = $1 AND public_id = $2
	`, userID, rootPublicID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO entries
			(public_id, user_id, parent_id, name, type, extension, size)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '', 0)
	`, childPublicID, userID, rootID)
	if err != nil {
		t.Fatal(err)
	}

	var childID int64
	err = pool.QueryRow(ctx, `
		SELECT id FROM entries WHERE user_id = $1 AND public_id = $2
	`, userID, childPublicID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	const size1 int64 = 100
	const size2 int64 = 250

	_, err = pool.Exec(ctx, `
		INSERT INTO entries
			(public_id, user_id, parent_id, name, type, extension, size)
		VALUES
			($1, $2, $3, 'file1', 'FILE', 'txt', $4),
			($5, $2, $3, 'file2', 'FILE', 'txt', $6)
	`, filePublicID1, userID, childID, size1,
		filePublicID2, size2)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	var fileID int64
	err = pool.QueryRow(ctx, `
		SELECT id
		FROM entries
		WHERE user_id = $1 AND public_id = $2
	`, userID, filePublicID1).Scan(&fileID)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- RecalculateAncestorFolderSizes(ctx, userID, fileID)
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent recalculation failed: %v", err)
		}
	}

	var childSize, rootSize int64
	err = pool.QueryRow(ctx, `
		SELECT
			(SELECT size FROM entries WHERE id = $1),
			(SELECT size FROM entries WHERE id = $2)
	`, childID, rootID).Scan(&childSize, &rootSize)
	if err != nil {
		t.Fatal(err)
	}

	const expectedSize = size1 + size2
	if childSize != expectedSize {
		t.Fatalf("child size = %d, want %d", childSize, expectedSize)
	}
	if rootSize != expectedSize {
		t.Fatalf("root size = %d, want %d", rootSize, expectedSize)
	}

	t.Logf("PASS: concurrent recalculations produced child=%d root=%d",
		childSize, rootSize)
}

func TestConcurrentFileMutationsAndFolderSizeRecalculation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
		_ = godotenv.Load("../../.env")
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
	rootPublicID := fmt.Sprintf("r%019d", userID)
	childPublicID := fmt.Sprintf("c%019d", userID)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`, userID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO entries
			(public_id, user_id, name, type, extension, size)
		VALUES ($1, $2, 'root', 'FOLDER', '', 0)
	`, rootPublicID, userID)
	if err != nil {
		t.Fatal(err)
	}

	var rootID int64
	err = pool.QueryRow(ctx, `
		SELECT id FROM entries
		WHERE user_id = $1 AND public_id = $2
	`, userID, rootPublicID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO entries
			(public_id, user_id, parent_id, name, type, extension, size)
		VALUES ($1, $2, $3, 'child', 'FOLDER', '', 0)
	`, childPublicID, userID, rootID)
	if err != nil {
		t.Fatal(err)
	}

	var childID int64
	err = pool.QueryRow(ctx, `
		SELECT id FROM entries
		WHERE user_id = $1 AND public_id = $2
	`, userID, childPublicID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	const fileCount = 10

	// Create initial files that will be soft-deleted concurrently.
	deleteIDs := make([]int64, fileCount)
	for i := 0; i < fileCount; i++ {
		publicID := fmt.Sprintf("d%018d", userID+int64(i))
		size := int64(100 + i)

		err = pool.QueryRow(ctx, `
			INSERT INTO entries
				(public_id, user_id, parent_id, name, type, extension, size)
			VALUES ($1, $2, $3, $4, 'FILE', 'txt', $5)
			RETURNING id
		`, publicID, userID, childID, fmt.Sprintf("deleted-%d", i), size,
		).Scan(&deleteIDs[i])
		if err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	errs := make(chan error, fileCount*2)
	var wg sync.WaitGroup

	// Concurrently soft-delete existing files and recalculate sizes.
	for i, fileID := range deleteIDs {
		wg.Add(1)
		go func(i int, fileID int64) {
			defer wg.Done()
			<-start

			_, err := pool.Exec(ctx, `
				UPDATE entries
				SET deleted_at = CURRENT_TIMESTAMP,
				    updated_at = CURRENT_TIMESTAMP
				WHERE id = $1
				  AND user_id = $2
				  AND deleted_at IS NULL
			`, fileID, userID)
			if err != nil {
				errs <- fmt.Errorf("soft-delete file %d: %w", fileID, err)
				return
			}

			errs <- RecalculateAncestorFolderSizes(ctx, userID, fileID)
		}(i, fileID)
	}

	// Concurrently create new files and recalculate sizes.
	for i := 0; i < fileCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start

			publicID := fmt.Sprintf("n%018d", userID+int64(i))
			size := int64(200 + i)

			var fileID int64
			err := pool.QueryRow(ctx, `
				INSERT INTO entries
					(public_id, user_id, parent_id, name, type, extension, size)
				VALUES ($1, $2, $3, $4, 'FILE', 'txt', $5)
				RETURNING id
			`, publicID, userID, childID, fmt.Sprintf("new-%d", i), size,
			).Scan(&fileID)
			if err != nil {
				errs <- fmt.Errorf("create file %d: %w", i, err)
				return
			}

			errs <- RecalculateAncestorFolderSizes(ctx, userID, fileID)
		}(i)
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Independently calculate expected sizes from active files.
	var expectedChildSize, childSize, rootSize int64

	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(size), 0)
		FROM entries
		WHERE user_id = $1
		  AND parent_id = $2
		  AND type = 'FILE'
		  AND deleted_at IS NULL
	`, userID, childID).Scan(&expectedChildSize)
	if err != nil {
		t.Fatal(err)
	}

	err = pool.QueryRow(ctx, `
		SELECT
			(SELECT size FROM entries WHERE id = $1),
			(SELECT size FROM entries WHERE id = $2)
	`, childID, rootID).Scan(&childSize, &rootSize)
	if err != nil {
		t.Fatal(err)
	}

	if childSize != expectedChildSize {
		t.Errorf("child size = %d, expected active file total = %d",
			childSize, expectedChildSize)
	}
	if rootSize != expectedChildSize {
		t.Errorf("root size = %d, expected active file total = %d",
			rootSize, expectedChildSize)
	}

	if childSize == expectedChildSize && rootSize == expectedChildSize {
		t.Logf("PASS: child=%d root=%d active-file-total=%d",
			childSize, rootSize, expectedChildSize)
	}
}

func waitForBlockedBackend(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	blockedPID int32,
	blockingPID int32,
) {
	t.Helper()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		var blocked bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE pid = $1
				  AND cardinality(pg_blocking_pids(pid)) > 0
				  AND $2 = ANY(pg_blocking_pids(pid))
			)
		`, blockedPID, blockingPID).Scan(&blocked)
		if err != nil {
			t.Fatalf("check blocked backend: %v", err)
		}

		if blocked {
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf("backend %d was not blocked by backend %d: %v",
				blockedPID, blockingPID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestFolderSizeUpdateWaitsForNoKeyUpdateLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env")
		_ = godotenv.Load("../../.env")
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

	userID := time.Now().UnixNano()
	publicID := fmt.Sprintf("l%019d", userID)

	var folderID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO entries (public_id, user_id, name, type, extension, size)
		VALUES ($1, $2, 'lock-test', 'FOLDER', '', 0)
		RETURNING id
	`, publicID, userID).Scan(&folderID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`, userID)
	})

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var blockingPID int32
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockingPID); err != nil {
		t.Fatal(err)
	}

	var lockedID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM entries WHERE id = $1 FOR NO KEY UPDATE
	`, folderID).Scan(&lockedID)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()

	var blockedPID int32
	if err := conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockedPID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := conn.Exec(ctx, `
			UPDATE entries SET size = 123 WHERE id = $1
		`, folderID)
		done <- err
	}()

	waitForBlockedBackend(ctx, t, pool, blockedPID, blockingPID)

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	t.Log("PASS: UPDATE waited until FOR NO KEY UPDATE lock was released")
}
