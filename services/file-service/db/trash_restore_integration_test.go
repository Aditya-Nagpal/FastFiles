package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testIDCounter atomic.Int64

type testEnv struct {
	pool   *pgxpool.Pool
	userID int64
}

func setupIntegrationTest(t *testing.T) *testEnv {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("database ping failed: %v", err)
	}

	previousDB := DB
	DB = pool

	// Generate a unique user ID for this test.
	userID := time.Now().UnixNano()

	// Automatically create storage for this test user.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes
		)
		VALUES ($1, 100000000, 0, 0)
	`, userID)

	if err != nil {
		DB = previousDB
		pool.Close()
		t.Fatalf("failed to create test user storage: %v", err)
	}

	env := &testEnv{
		pool:   pool,
		userID: userID,
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			15*time.Second,
		)
		defer cleanupCancel()

		// Delete only outbox events belonging to this test user's entries.
		_, err := pool.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE aggregate_type = 'ENTRY'
			  AND aggregate_id IN (
				  SELECT public_id
				  FROM entries
				  WHERE user_id = $1
			  )
		`, userID)
		if err != nil {
			t.Errorf("failed to clean outbox events: %v", err)
		}

		// Delete only this test user's entries.
		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean entries: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM storage_reservations
			WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean reservations: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean user storage: %v", err)
		}

		DB = previousDB
		pool.Close()
	})

	return env
}

func (e *testEnv) publicID() string {
	n := testIDCounter.Add(1)
	return fmt.Sprintf("t%020d", n)
}

func (e *testEnv) createFolder(
	t *testing.T,
	parentID *int64,
	name string,
) (int64, string) {
	t.Helper()

	publicID := e.publicID()

	var id int64
	err := DB.QueryRow(context.Background(), `
		INSERT INTO entries (
			public_id,
			user_id,
			parent_id,
			name,
			type,
			extension
		)
		VALUES ($1, $2, $3, $4, 'FOLDER', '')
		RETURNING id
	`, publicID, e.userID, parentID, name).Scan(&id)

	if err != nil {
		t.Fatalf("failed to create folder: %v", err)
	}

	return id, publicID
}

func (e *testEnv) createFile(
	t *testing.T,
	parentID *int64,
	name string,
	size int64,
) (int64, string) {
	t.Helper()

	publicID := e.publicID()

	var id int64
	err := DB.QueryRow(context.Background(), `
		INSERT INTO entries (
			public_id,
			user_id,
			parent_id,
			name,
			type,
			content_type,
			extension,
			size,
			s3_key
		)
		VALUES ($1, $2, $3, $4, 'FILE', 'text/plain', 'txt', $5, $6)
		RETURNING id
	`,
		publicID,
		e.userID,
		parentID,
		name,
		size,
		"test/"+publicID,
	).Scan(&id)

	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	return id, publicID
}

func (e *testEnv) setUsedBytes(t *testing.T, size int64) {
	t.Helper()

	_, err := DB.Exec(context.Background(), `
		UPDATE user_storage
		SET used_bytes = $1
		WHERE user_id = $2
	`, size, e.userID)

	if err != nil {
		t.Fatalf("failed to update used bytes: %v", err)
	}
}

func (e *testEnv) assertStorage(
	t *testing.T,
	expectedUsed int64,
	expectedReserved int64,
) {
	t.Helper()

	var used, reserved int64

	err := DB.QueryRow(context.Background(), `
		SELECT used_bytes, reserved_bytes
		FROM user_storage
		WHERE user_id = $1
	`, e.userID).Scan(&used, &reserved)

	if err != nil {
		t.Fatalf("failed to read storage: %v", err)
	}

	if used != expectedUsed || reserved != expectedReserved {
		t.Fatalf(
			"storage mismatch: used=%d reserved=%d; expected used=%d reserved=%d",
			used,
			reserved,
			expectedUsed,
			expectedReserved,
		)
	}
}

func (e *testEnv) assertEntryDeleted(
	t *testing.T,
	publicID string,
	expectedDeleted bool,
) {
	t.Helper()

	var deleted bool

	err := DB.QueryRow(context.Background(), `
		SELECT deleted_at IS NOT NULL
		FROM entries
		WHERE public_id = $1 AND user_id = $2
	`, publicID, e.userID).Scan(&deleted)

	if err != nil {
		t.Fatalf("failed to read entry deletion state: %v", err)
	}

	if deleted != expectedDeleted {
		t.Fatalf(
			"entry %s deleted=%v; expected %v",
			publicID,
			deleted,
			expectedDeleted,
		)
	}
}

func (e *testEnv) assertEventCount(
	t *testing.T,
	eventType string,
	expected int,
) {
	t.Helper()

	var count int

	err := DB.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM outbox_events
		WHERE aggregate_type = 'ENTRY'
		  AND event_type = $1
		  AND aggregate_id IN (
			  SELECT public_id
			  FROM entries
			  WHERE user_id = $2
		  )
	`, eventType, e.userID).Scan(&count)

	if err != nil {
		t.Fatalf("failed to count outbox events: %v", err)
	}

	if count != expected {
		t.Fatalf(
			"event count for %s = %d; expected %d",
			eventType,
			count,
			expected,
		)
	}
}

// ------------------------------------------------------------
// TEST 1: File Trash -> Restore
// ------------------------------------------------------------

func TestIntegration_FileTrashAndRestore(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	_, fileID := env.createFile(t, nil, "hello", 100)
	env.setUsedBytes(t, 100)

	if err := SoftDeleteFile(ctx, fileID, env.userID); err != nil {
		t.Fatalf("SoftDeleteFile failed: %v", err)
	}

	env.assertEntryDeleted(t, fileID, true)

	// Trash must not release quota.
	env.assertStorage(t, 100, 0)

	if err := RestoreFile(ctx, fileID, env.userID); err != nil {
		t.Fatalf("RestoreFile failed: %v", err)
	}

	env.assertEntryDeleted(t, fileID, false)

	// Restore must not modify quota either.
	env.assertStorage(t, 100, 0)

	env.assertEventCount(t, "FILE_TRASHED", 1)
	env.assertEventCount(t, "FILE_RESTORED", 1)

	// Restoring an already active file must fail.
	err := RestoreFile(ctx, fileID, env.userID)
	if err == nil {
		t.Fatal("expected second restore to fail")
	}
}

// ------------------------------------------------------------
// TEST 2: Folder Restore preserves independently trashed child
// ------------------------------------------------------------

func TestIntegration_FolderRestorePreservesIndependentTrash(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	parentID, parentPublicID := env.createFolder(t, nil, "parent")
	childID, _ := env.createFolder(t, &parentID, "child")

	independentFileID, independentFilePublicID :=
		env.createFile(t, &childID, "independent", 100)

	env.setUsedBytes(t, 100)

	// Independently trash a file inside the nested folder.
	if err := SoftDeleteFile(ctx, independentFilePublicID, env.userID); err != nil {
		t.Fatalf("failed to trash independent file: %v", err)
	}

	// Now trash the parent folder.
	if err := SoftDeleteFolder(ctx, parentPublicID, env.userID); err != nil {
		t.Fatalf("failed to trash parent folder: %v", err)
	}

	// Restore the parent.
	if err := RestoreFolder(ctx, parentPublicID, env.userID); err != nil {
		t.Fatalf("failed to restore parent folder: %v", err)
	}

	// Parent and child folder should be active.
	env.assertEntryDeleted(t, parentPublicID, false)

	var childPublicID string
	err := DB.QueryRow(ctx, `
		SELECT public_id FROM entries WHERE id = $1
	`, childID).Scan(&childPublicID)

	if err != nil {
		t.Fatal(err)
	}

	env.assertEntryDeleted(t, childPublicID, false)

	// Independently trashed file must remain in Trash.
	env.assertEntryDeleted(t, independentFilePublicID, true)

	// Original parent relationship must be preserved.
	var actualParentID int64
	err = DB.QueryRow(ctx, `
		SELECT parent_id
		FROM entries
		WHERE id = $1
	`, independentFileID).Scan(&actualParentID)

	if err != nil {
		t.Fatal(err)
	}

	if actualParentID != childID {
		t.Fatalf(
			"file parent changed: got %d, expected %d",
			actualParentID,
			childID,
		)
	}

	// Trash/restore operations must not change quota.
	env.assertStorage(t, 100, 0)

	env.assertEventCount(t, "FOLDER_TRASHED", 1)
	env.assertEventCount(t, "FOLDER_RESTORED", 1)
}

// ------------------------------------------------------------
// TEST 3: Concurrent Restore vs Restore
// ------------------------------------------------------------

func TestIntegration_ConcurrentRestoreSameFile(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	_, filePublicID := env.createFile(t, nil, "race", 100)

	if err := SoftDeleteFile(ctx, filePublicID, env.userID); err != nil {
		t.Fatalf("failed to trash file: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup

	var successCount atomic.Int64
	var failureCount atomic.Int64

	for i := 0; i < 2; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			err := RestoreFile(ctx, filePublicID, env.userID)
			if err == nil {
				successCount.Add(1)
			} else {
				failureCount.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf(
			"expected exactly one successful restore; got %d",
			successCount.Load(),
		)
	}

	if failureCount.Load() != 1 {
		t.Fatalf(
			"expected exactly one failed restore; got %d",
			failureCount.Load(),
		)
	}

	env.assertEntryDeleted(t, filePublicID, false)

	// Exactly one restore event should be written.
	env.assertEventCount(t, "FILE_RESTORED", 1)
}

// ------------------------------------------------------------
// TEST 4: Concurrent Upload Finalization vs Parent Trash
// ------------------------------------------------------------

func TestIntegration_UploadFinalizeVsParentTrash(t *testing.T) {
	env := setupIntegrationTest(t)
	ctx := context.Background()

	parentID, parentPublicID := env.createFolder(t, nil, "upload-parent")

	const fileSize int64 = 500

	reservationID, err := CreateStorageReservation(
		ctx,
		env.userID,
		fileSize,
	)
	if err != nil {
		t.Fatalf("failed to create storage reservation: %v", err)
	}

	now := time.Now().UTC()
	filePublicID := env.publicID()

	entry := &models.EntryData{
		PublicId:    filePublicID,
		UserId:      env.userID,
		ParentId:    &parentID,
		Name:        "uploaded-file",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   "txt",
		Size:        fileSize,
		S3Key: sql.NullString{
			String: "test/" + filePublicID,
			Valid:  true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	start := make(chan struct{})
	var wg sync.WaitGroup

	var uploadErr error
	var trashErr error
	var uploadedEntryID int64

	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start

		uploadedEntryID, uploadErr = InsertEntryAndFinalizeStorage(
			ctx,
			entry,
			reservationID,
		)
	}()

	go func() {
		defer wg.Done()
		<-start

		trashErr = SoftDeleteFolder(
			ctx,
			parentPublicID,
			env.userID,
		)
	}()

	close(start)
	wg.Wait()

	if trashErr != nil {
		t.Fatalf("parent trash failed: %v", trashErr)
	}

	// Both operations must produce a valid serial outcome:
	//
	// A. Upload wins:
	//    - File gets inserted.
	//    - Parent is subsequently trashed.
	//    - File is also trashed.
	//    - Reservation becomes FINALIZED.
	//
	// B. Trash wins:
	//    - Parent gets trashed.
	//    - Upload finalization fails because parent is no longer active.
	//    - Reservation must be released.

	if uploadErr == nil {
		if uploadedEntryID == 0 {
			t.Fatal("upload succeeded but returned an invalid entry ID")
		}

		env.assertEntryDeleted(t, filePublicID, true)

		var status string
		err := DB.QueryRow(ctx, `
			SELECT status
			FROM storage_reservations
			WHERE id = $1
		`, reservationID).Scan(&status)

		if err != nil {
			t.Fatal(err)
		}

		if status != "FINALIZED" {
			t.Fatalf("reservation status=%s; expected FINALIZED", status)
		}

		env.assertStorage(t, fileSize, 0)

	} else {
		// Simulate the upload handler's reservation cleanup.
		if err := ReleaseStorageReservationByID(ctx, reservationID); err != nil {
			t.Fatalf("failed to release pending reservation: %v", err)
		}

		// No file should have been inserted.
		var count int

		err := DB.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM entries
			WHERE public_id = $1 AND user_id = $2
		`, filePublicID, env.userID).Scan(&count)

		if err != nil {
			t.Fatal(err)
		}

		if count != 0 {
			t.Fatalf("expected no uploaded entry, found %d", count)
		}

		var status string
		err = DB.QueryRow(ctx, `
			SELECT status
			FROM storage_reservations
			WHERE id = $1
		`, reservationID).Scan(&status)

		if err != nil {
			t.Fatal(err)
		}

		if status != "RELEASED" {
			t.Fatalf("reservation status=%s; expected RELEASED", status)
		}

		env.assertStorage(t, 0, 0)
	}

	env.assertEntryDeleted(t, parentPublicID, true)
}

// Helper for constructing sql.NullString without importing database/sql
// throughout the test code.
