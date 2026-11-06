package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/segmentio/kafka-go"
)

func TestSoftDeleteFolder_PublishesOutboxEvent(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	envPath, err := findEnvFile()
	if err != nil {
		t.Fatalf("failed to find .env: %v", err)
	}

	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	brokersValue := os.Getenv("KAFKA_BROKERS")

	if databaseURL == "" || brokersValue == "" {
		t.Fatal("DATABASE_URL and KAFKA_BROKERS must be configured")
	}

	brokers := strings.Split(brokersValue, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	if brokers[0] == "" {
		t.Fatal("KAFKA_BROKERS is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	previousDB := db.DB
	db.DB = pool

	t.Cleanup(func() {
		db.DB = previousDB
		pool.Close()
	})

	testID := uuid.NewString()
	userID := time.Now().UnixNano()

	rootPublicID := strings.ReplaceAll(uuid.NewString(), "-", "")[:21]
	nestedPublicID := strings.ReplaceAll(uuid.NewString(), "-", "")[:21]

	filePublicIDs := []string{
		strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
		strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
		strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
	}

	const fileSize int64 = 128
	const totalSize int64 = 384

	// 1. Create storage fixture with committed usage.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes
		)
		VALUES ($1, 10485760, $2, 0)
	`, userID, totalSize)
	if err != nil {
		t.Fatalf("failed to create storage fixture: %v", err)
	}

	// 2. Create root folder.
	var rootID int64

	err = pool.QueryRow(ctx, `
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
		VALUES ($1, $2, NULL, 'root-folder', 'FOLDER', '', '', 0, NULL)
		RETURNING id
	`, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatalf("failed to create root folder: %v", err)
	}

	// 3. Create nested folder.
	var nestedID int64

	err = pool.QueryRow(ctx, `
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
		VALUES ($1, $2, $3, 'nested-folder', 'FOLDER', '', '', 0, NULL)
		RETURNING id
	`, nestedPublicID, userID, rootID).Scan(&nestedID)
	if err != nil {
		t.Fatalf("failed to create nested folder: %v", err)
	}

	// 4. Create one file under root.
	_, err = pool.Exec(ctx, `
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
		VALUES ($1, $2, $3, 'file-a', 'FILE', 'text/plain', 'txt', $4, $5)
	`,
		filePublicIDs[0],
		userID,
		rootID,
		fileSize,
		fmt.Sprintf("integration-tests/%s/file-a.txt", testID),
	)
	if err != nil {
		t.Fatalf("failed to create root file: %v", err)
	}

	// 5. Create two files under nested folder.
	for i := 1; i < 3; i++ {
		_, err = pool.Exec(ctx, `
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
		`,
			filePublicIDs[i],
			userID,
			nestedID,
			fmt.Sprintf("file-%d", i+1),
			fileSize,
			fmt.Sprintf("integration-tests/%s/nested/file-%d.txt", testID, i+1),
		)
		if err != nil {
			t.Fatalf("failed to create nested file %d: %v", i+1, err)
		}
	}

	// Cleanup only this test's data.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, err := pool.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE aggregate_id = $1
		`, rootPublicID)
		if err != nil {
			t.Errorf("failed to clean outbox fixture: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean entry fixtures: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, userID)
		if err != nil {
			t.Errorf("failed to clean storage fixture: %v", err)
		}
	})

	// 6. Call the actual production SoftDeleteFolder function.
	if err := db.SoftDeleteFolder(ctx, rootPublicID, userID); err != nil {
		t.Fatalf("SoftDeleteFolder failed: %v", err)
	}

	// 7. Verify root and all descendants are soft-deleted.
	var totalEntries int
	var deletedEntries int

	err = pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE deleted_at IS NOT NULL)
		FROM entries
		WHERE user_id = $1
	`, userID).Scan(&totalEntries, &deletedEntries)
	if err != nil {
		t.Fatalf("failed to verify trashed entries: %v", err)
	}

	if totalEntries != 5 {
		t.Fatalf("expected 5 entries, found %d", totalEntries)
	}

	if deletedEntries != 5 {
		t.Fatalf("expected all 5 entries trashed, found %d", deletedEntries)
	}

	// 8. Verify explicitly_deleted flags.
	var explicitlyDeletedCount int

	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1
		  AND explicitly_deleted = TRUE
	`, userID).Scan(&explicitlyDeletedCount)
	if err != nil {
		t.Fatalf("failed to verify explicitly_deleted flags: %v", err)
	}

	if explicitlyDeletedCount != 1 {
		t.Fatalf("expected only root folder to be explicitly deleted, found %d", explicitlyDeletedCount)
	}

	// 9. Verify quota is preserved because entries are in Trash, not permanently deleted.
	var usedBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes)
	if err != nil {
		t.Fatalf("failed to read storage usage: %v", err)
	}

	if usedBytes != totalSize {
		t.Fatalf("expected used_bytes=%d, got %d", totalSize, usedBytes)
	}

	// 10. Fetch the actual outbox event created by SoftDeleteFolder.
	var (
		eventID string
		payload []byte
		status  string
	)

	err = pool.QueryRow(ctx, `
		SELECT event_id, payload, status
		FROM outbox_events
		WHERE aggregate_id = $1
		  AND event_type = 'FOLDER_TRASHED'
	`, rootPublicID).Scan(&eventID, &payload, &status)
	if err != nil {
		t.Fatalf("FOLDER_TRASHED outbox event not found: %v", err)
	}

	var event events.EntryEvent

	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}

	if event.EventID != eventID {
		t.Fatalf("event ID mismatch: payload=%s database=%s", event.EventID, eventID)
	}

	if event.EventType != events.EventFolderTrashed {
		t.Fatalf("expected FOLDER_TRASHED, got %s", event.EventType)
	}

	if event.EntryID != rootID {
		t.Fatalf("expected root entry ID %d, got %d", rootID, event.EntryID)
	}

	if event.UserID != userID {
		t.Fatalf("expected user ID %d, got %d", userID, event.UserID)
	}

	if status != "PENDING" {
		t.Fatalf("expected initial outbox status PENDING, got %s", status)
	}

	// 11. Create an isolated Kafka topic.
	testTopic := "fastfiles-trash-folder-" + testID
	createKafkaTopic(t, brokers, testTopic)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       testTopic,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer reader.Close()

	// 12. Start the actual publisher.
	publisherCtx, stopPublisher := context.WithCancel(ctx)
	publisherDone := make(chan struct{})

	go func() {
		defer close(publisherDone)
		Run(publisherCtx, brokers, testTopic)
	}()

	defer func() {
		stopPublisher()

		select {
		case <-publisherDone:
		case <-time.After(5 * time.Second):
			t.Error("publisher did not stop within 5 seconds")
		}
	}()

	// 13. Read until our specific folder event arrives.
	found := false

	for !found {
		message, err := reader.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("failed to read Kafka message: %v", err)
		}

		var publishedEvent events.EntryEvent

		if err := json.Unmarshal(message.Value, &publishedEvent); err != nil {
			t.Fatalf("failed to decode Kafka message: %v", err)
		}

		if publishedEvent.EventID != eventID {
			continue
		}

		if publishedEvent.EventType != events.EventFolderTrashed {
			t.Fatalf(
				"expected FOLDER_TRASHED, got %s",
				publishedEvent.EventType,
			)
		}

		if publishedEvent.EntryID != rootID {
			t.Fatalf(
				"expected root entry ID %d, got %d",
				rootID,
				publishedEvent.EntryID,
			)
		}

		found = true
	}

	// 14. Verify publisher marked the event as PUBLISHED.
	for {
		err := pool.QueryRow(ctx, `
			SELECT status
			FROM outbox_events
			WHERE event_id = $1
		`, eventID).Scan(&status)
		if err != nil {
			t.Fatalf("failed to read outbox status: %v", err)
		}

		if status == "PUBLISHED" {
			break
		}

		select {
		case <-ctx.Done():
			t.Fatalf(
				"event reached Kafka but outbox status did not become PUBLISHED; last status=%s",
				status,
			)
		case <-time.After(200 * time.Millisecond):
		}
	}

	t.Logf(
		"SUCCESS: SoftDeleteFolder soft-deleted all descendants, preserved %d bytes of quota, created FOLDER_TRASHED event %s, and publisher delivered it to Kafka",
		totalSize,
		eventID,
	)
}

type folderConcurrencyFixture struct {
	userID       int64
	rootID       int64
	rootPublicID string
	filePublicID string
	testID       string
}

const (
	folderTestInitialUsedBytes int64 = 512
	folderTestUploadSize       int64 = 128
)

func newFolderConcurrencyTestPool(
	t *testing.T,
	ctx context.Context,
) (*pgxpool.Pool, string) {
	t.Helper()

	envPath, err := findEnvFile()
	if err != nil {
		t.Fatalf("failed to find .env: %v", err)
	}

	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not configured")
	}

	appName := "fastfiles-lock-test-" + uuid.NewString()

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("failed to parse database config: %v", err)
	}

	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}

	config.ConnConfig.RuntimeParams["application_name"] = appName
	config.MaxConns = 8

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("database ping failed: %v", err)
	}

	previousDB := db.DB
	db.DB = pool

	t.Cleanup(func() {
		db.DB = previousDB
		pool.Close()
	})

	return pool, appName
}

func createFolderConcurrencyFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) folderConcurrencyFixture {
	t.Helper()

	fixture := folderConcurrencyFixture{
		userID:       time.Now().UnixNano(),
		rootPublicID: strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
		filePublicID: strings.ReplaceAll(uuid.NewString(), "-", "")[:21],
		testID:       uuid.NewString(),
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO user_storage (
			user_id, quota_bytes, used_bytes, reserved_bytes
		)
		VALUES ($1, 10485760, $2, 0)
	`, fixture.userID, folderTestInitialUsedBytes)
	if err != nil {
		t.Fatalf("failed to create storage fixture: %v", err)
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO entries (
			public_id, user_id, parent_id, name,
			type, content_type, extension, size, s3_key
		)
		VALUES ($1, $2, NULL, 'root', 'FOLDER', '', '', 0, NULL)
		RETURNING id
	`, fixture.rootPublicID, fixture.userID).Scan(&fixture.rootID)
	if err != nil {
		t.Fatalf("failed to create folder fixture: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, err := pool.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE aggregate_id = $1
		`, fixture.rootPublicID)
		if err != nil {
			t.Errorf("failed to clean outbox fixture: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
		`, fixture.userID)
		if err != nil {
			t.Errorf("failed to clean entry fixtures: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM storage_reservations
			WHERE user_id = $1
		`, fixture.userID)
		if err != nil {
			t.Errorf("failed to clean reservations: %v", err)
		}

		_, err = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, fixture.userID)
		if err != nil {
			t.Errorf("failed to clean storage fixture: %v", err)
		}
	})

	return fixture
}

func createFolderConcurrencyEntry(
	fixture folderConcurrencyFixture,
) *models.EntryData {
	now := time.Now().UTC()

	return &models.EntryData{
		PublicId:    fixture.filePublicID,
		UserId:      fixture.userID,
		ParentId:    &fixture.rootID,
		Name:        "concurrent-upload",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   "txt",
		Size:        folderTestUploadSize,
		S3Key: sql.NullString{
			String: fmt.Sprintf(
				"integration-tests/%s.txt",
				fixture.testID,
			),
			Valid: true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Wait until the expected number of connections from this test
// are actually waiting for PostgreSQL locks.
func waitForFolderLockWaiters(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	appName string,
	blockerPID int,
	expectedWaiters int,
) {
	t.Helper()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	for {
		var waitingCount int

		err := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM pg_stat_activity
			WHERE application_name = $1
			  AND pid <> $2
			  AND wait_event_type = 'Lock'
		`, appName, blockerPID).Scan(&waitingCount)
		if err != nil {
			t.Fatalf("failed to inspect PostgreSQL lock waits: %v", err)
		}

		if waitingCount >= expectedWaiters {
			t.Logf(
				"Confirmed %d PostgreSQL lock waiter(s)",
				waitingCount,
			)
			return
		}

		select {
		case <-ctx.Done():
			t.Fatalf(
				"context ended while waiting for %d lock waiters; found %d",
				expectedWaiters,
				waitingCount,
			)
		case <-timeout.C:
			t.Fatalf(
				"timed out waiting for %d PostgreSQL lock waiters; found %d",
				expectedWaiters,
				waitingCount,
			)
		case <-ticker.C:
		}
	}
}

func TestSoftDeleteFolder_UploadConcurrency(t *testing.T) {
	t.Run("upload_wins", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			30*time.Second,
		)
		defer cancel()

		if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
			t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
		}

		pool, appName := newFolderConcurrencyTestPool(t, ctx)
		fixture := createFolderConcurrencyFixture(t, ctx, pool)

		reservationID, err := db.CreateStorageReservation(
			ctx,
			fixture.userID,
			folderTestUploadSize,
		)
		if err != nil {
			t.Fatalf("failed to create storage reservation: %v", err)
		}

		// Test transaction holds a SHARE lock on the parent folder.
		// This gives us deterministic control over the lock ordering.
		lockTx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin lock transaction: %v", err)
		}
		defer lockTx.Rollback(ctx)

		err = db.LockEntryHierarchy(
			lockTx,
			ctx,
			fixture.userID,
			fixture.rootID,
			"FOLDER",
			false,
		)
		if err != nil {
			t.Fatalf("failed to acquire upload SHARE lock: %v", err)
		}

		var blockerPID int
		if err := lockTx.QueryRow(ctx, `SELECT pg_backend_pid()`).
			Scan(&blockerPID); err != nil {
			t.Fatalf("failed to get blocker PID: %v", err)
		}

		// Run the actual production upload finalization while the
		// SHARE lock is held. Its own SHARE lock is compatible.
		entryData := createFolderConcurrencyEntry(fixture)

		_, err = db.InsertEntryAndFinalizeStorage(
			ctx,
			entryData,
			reservationID,
		)
		if err != nil {
			t.Fatalf("InsertEntryAndFinalizeStorage failed: %v", err)
		}

		// Start actual production Trash.
		// It must wait because our transaction still holds SHARE.
		trashDone := make(chan error, 1)

		go func() {
			trashDone <- db.SoftDeleteFolder(
				ctx,
				fixture.rootPublicID,
				fixture.userID,
			)
		}()

		waitForFolderLockWaiters(
			t,
			ctx,
			pool,
			appName,
			blockerPID,
			1,
		)

		// Release SHARE lock. Trash can now proceed.
		if err := lockTx.Commit(ctx); err != nil {
			t.Fatalf("failed to commit upload-side lock transaction: %v", err)
		}

		if err := <-trashDone; err != nil {
			t.Fatalf("SoftDeleteFolder failed: %v", err)
		}

		// Verify both folder and uploaded file are trashed.
		var deletedCount int

		err = pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM entries
			WHERE user_id = $1
			  AND deleted_at IS NOT NULL
		`, fixture.userID).Scan(&deletedCount)
		if err != nil {
			t.Fatalf("failed to verify trashed entries: %v", err)
		}

		if deletedCount != 2 {
			t.Fatalf(
				"expected folder and uploaded file trashed; got %d",
				deletedCount,
			)
		}

		var usedBytes, reservedBytes int64

		err = pool.QueryRow(ctx, `
			SELECT used_bytes, reserved_bytes
			FROM user_storage
			WHERE user_id = $1
		`, fixture.userID).Scan(&usedBytes, &reservedBytes)
		if err != nil {
			t.Fatalf("failed to read quota: %v", err)
		}

		if usedBytes != folderTestInitialUsedBytes+folderTestUploadSize {
			t.Fatalf(
				"expected used_bytes=%d, got %d",
				folderTestInitialUsedBytes+folderTestUploadSize,
				usedBytes,
			)
		}

		if reservedBytes != 0 {
			t.Fatalf("expected reserved_bytes=0, got %d", reservedBytes)
		}

		var reservationStatus string

		err = pool.QueryRow(ctx, `
			SELECT status
			FROM storage_reservations
			WHERE id = $1
		`, reservationID).Scan(&reservationStatus)
		if err != nil {
			t.Fatalf("failed to read reservation status: %v", err)
		}

		if reservationStatus != "FINALIZED" {
			t.Fatalf(
				"expected FINALIZED reservation, got %s",
				reservationStatus,
			)
		}

		t.Log(
			"SUCCESS: upload finalized while SHARE lock was held; " +
				"Trash genuinely waited for the lock and then trashed the uploaded file",
		)
	})

	t.Run("trash_wins", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			30*time.Second,
		)
		defer cancel()

		if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
			t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
		}

		pool, appName := newFolderConcurrencyTestPool(t, ctx)
		fixture := createFolderConcurrencyFixture(t, ctx, pool)

		reservationID, err := db.CreateStorageReservation(
			ctx,
			fixture.userID,
			folderTestUploadSize,
		)
		if err != nil {
			t.Fatalf("failed to create storage reservation: %v", err)
		}

		// Test transaction holds an exclusive lock on the parent folder.
		lockTx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("failed to begin lock transaction: %v", err)
		}
		defer lockTx.Rollback(ctx)

		err = db.LockEntryHierarchy(
			lockTx,
			ctx,
			fixture.userID,
			fixture.rootID,
			"FOLDER",
			true,
		)
		if err != nil {
			t.Fatalf("failed to acquire Trash exclusive lock: %v", err)
		}

		var blockerPID int
		if err := lockTx.QueryRow(ctx, `SELECT pg_backend_pid()`).
			Scan(&blockerPID); err != nil {
			t.Fatalf("failed to get blocker PID: %v", err)
		}

		// Queue actual production Trash first.
		trashDone := make(chan error, 1)

		go func() {
			trashDone <- db.SoftDeleteFolder(
				ctx,
				fixture.rootPublicID,
				fixture.userID,
			)
		}()

		// Confirm Trash is waiting for our exclusive lock.
		waitForFolderLockWaiters(
			t,
			ctx,
			pool,
			appName,
			blockerPID,
			1,
		)

		// Now queue actual production upload finalization behind Trash.
		uploadDone := make(chan error, 1)
		entryData := createFolderConcurrencyEntry(fixture)

		go func() {
			_, uploadErr := db.InsertEntryAndFinalizeStorage(
				ctx,
				entryData,
				reservationID,
			)
			uploadDone <- uploadErr
		}()

		// Both production operations must now be waiting for PostgreSQL locks.
		waitForFolderLockWaiters(
			t,
			ctx,
			pool,
			appName,
			blockerPID,
			2,
		)

		// Release the blocker. Trash was queued first, so it gets
		// the exclusive lock before upload can acquire its SHARE lock.
		if err := lockTx.Commit(ctx); err != nil {
			t.Fatalf("failed to commit Trash-side lock transaction: %v", err)
		}

		trashErr := <-trashDone
		if trashErr != nil {
			t.Fatalf("SoftDeleteFolder failed: %v", trashErr)
		}

		uploadErr := <-uploadDone
		if !errors.Is(uploadErr, db.ErrEntryNotFound) {
			t.Fatalf(
				"expected upload to fail with ErrEntryNotFound, got %v",
				uploadErr,
			)
		}

		// The upload finalization did not consume the reservation.
		// Release it as the upload failure cleanup would.
		if err := db.ReleaseStorageReservationByID(ctx, reservationID); err != nil {
			t.Fatalf("failed to release upload reservation: %v", err)
		}

		// Verify no file entry was created.
		var fileCount int

		err = pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM entries
			WHERE public_id = $1
			  AND user_id = $2
		`, fixture.filePublicID, fixture.userID).Scan(&fileCount)
		if err != nil {
			t.Fatalf("failed to verify uploaded file: %v", err)
		}

		if fileCount != 0 {
			t.Fatalf("expected no uploaded file entry, found %d", fileCount)
		}

		// Verify quota accounting.
		var usedBytes, reservedBytes int64

		err = pool.QueryRow(ctx, `
			SELECT used_bytes, reserved_bytes
			FROM user_storage
			WHERE user_id = $1
		`, fixture.userID).Scan(&usedBytes, &reservedBytes)
		if err != nil {
			t.Fatalf("failed to read quota: %v", err)
		}

		if usedBytes != folderTestInitialUsedBytes {
			t.Fatalf(
				"expected used_bytes=%d, got %d",
				folderTestInitialUsedBytes,
				usedBytes,
			)
		}

		if reservedBytes != 0 {
			t.Fatalf("expected reserved_bytes=0, got %d", reservedBytes)
		}

		var reservationStatus string

		err = pool.QueryRow(ctx, `
			SELECT status
			FROM storage_reservations
			WHERE id = $1
		`, reservationID).Scan(&reservationStatus)
		if err != nil {
			t.Fatalf("failed to read reservation status: %v", err)
		}

		if reservationStatus != "RELEASED" {
			t.Fatalf(
				"expected reservation status RELEASED, got %s",
				reservationStatus,
			)
		}

		t.Log(
			"SUCCESS: Trash was queued before upload; both waited on PostgreSQL locks; " +
				"Trash won and upload was rejected",
		)
	})
}
