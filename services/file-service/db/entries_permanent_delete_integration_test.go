package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"database/sql"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
)

const pdTestBucket = "fastfiles-permanent-delete-test"

type pdTestDB struct {
	pool   *pgxpool.Pool
	userID int64
}

func pdSetupTestDB(t *testing.T) *pdTestDB {
	t.Helper()

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	if os.Getenv("DATABASE_URL") == "" {
		_ = godotenv.Load("../.env", "../../.env")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("failed to create database pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping database: %v", err)
	}

	previousDB := DB
	DB = pool
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	userID := time.Now().UnixNano()

	testDB := &pdTestDB{
		pool:   pool,
		userID: userID,
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		// Remove test outbox events first.
		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM outbox_events
			WHERE payload->>'userId' = $1
		`, fmt.Sprint(userID))

		// ON DELETE CASCADE removes child entries.
		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM entries
			WHERE user_id = $1
		`, userID)

		_, _ = pool.Exec(cleanupCtx, `
			DELETE FROM user_storage
			WHERE user_id = $1
		`, userID)

		DB = previousDB
		pool.Close()
	})

	return testDB
}

func pdPublicID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	return "p" + id[:20]
}

func (tdb *pdTestDB) createStorage(
	t *testing.T,
	quotaBytes int64,
	usedBytes int64,
) {
	t.Helper()

	_, err := tdb.pool.Exec(context.Background(), `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes
		)
		VALUES ($1, $2, $3, 0)
	`, tdb.userID, quotaBytes, usedBytes)

	if err != nil {
		t.Fatalf("failed to create user storage: %v", err)
	}
}

func (tdb *pdTestDB) createEntry(
	t *testing.T,
	parentID *int64,
	name string,
	entryType string,
	size int64,
	s3Key string,
	isDeleted bool,
	explicitlyDeleted bool,
) (int64, string) {
	t.Helper()

	publicID := pdPublicID()

	var parent any
	if parentID != nil {
		parent = *parentID
	}

	var deletedAt any
	if isDeleted {
		deletedAt = time.Now().UTC()
	}

	var key any
	if s3Key != "" {
		key = s3Key
	}

	var entryID int64

	err := tdb.pool.QueryRow(context.Background(), `
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
			deleted_at,
			explicitly_deleted
		)
		VALUES (
			$1, $2, $3, $4, $5,
			'application/octet-stream',
			'', $6, $7, $8, $9
		)
		RETURNING id
	`,
		publicID,
		tdb.userID,
		parent,
		name,
		entryType,
		size,
		key,
		deletedAt,
		explicitlyDeleted,
	).Scan(&entryID)

	if err != nil {
		t.Fatalf("failed to create %s entry: %v", entryType, err)
	}

	return entryID, publicID
}

func (tdb *pdTestDB) getUsedBytes(t *testing.T) int64 {
	t.Helper()

	var usedBytes int64

	err := tdb.pool.QueryRow(context.Background(), `
		SELECT used_bytes
		FROM user_storage
		WHERE user_id = $1
	`, tdb.userID).Scan(&usedBytes)

	if err != nil {
		t.Fatalf("failed to read used storage: %v", err)
	}

	return usedBytes
}

func (tdb *pdTestDB) countEntries(t *testing.T) int {
	t.Helper()

	var count int

	err := tdb.pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM entries
		WHERE user_id = $1
	`, tdb.userID).Scan(&count)

	if err != nil {
		t.Fatalf("failed to count entries: %v", err)
	}

	return count
}

func (tdb *pdTestDB) getCleanupEvents(t *testing.T, aggregateID string) []events.S3CleanupEvent {
	t.Helper()

	rows, err := tdb.pool.Query(context.Background(), `
		SELECT payload
		FROM outbox_events
		WHERE aggregate_type = 'S3_CLEANUP'
		  AND aggregate_id = $1
		  AND event_type = $2
	`, aggregateID, events.EventS3Cleanup)

	if err != nil {
		t.Fatalf("failed to query cleanup outbox events: %v", err)
	}
	defer rows.Close()

	var result []events.S3CleanupEvent

	for rows.Next() {
		var payload []byte

		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("failed to scan outbox payload: %v", err)
		}

		var event events.S3CleanupEvent

		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("failed to decode cleanup event: %v", err)
		}

		result = append(result, event)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate outbox events: %v", err)
	}

	return result
}

func pdSetBucket(t *testing.T) {
	t.Helper()

	previousConfig := config.AppConfig

	config.AppConfig = &config.Config{
		BucketName: pdTestBucket,
	}

	t.Cleanup(func() {
		config.AppConfig = previousConfig
	})
}

// ------------------------------------------------------------
// File permanent deletion
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_File(t *testing.T) {
	tdb := pdSetupTestDB(t)
	pdSetBucket(t)

	tdb.createStorage(t, 10_000, 1_000)

	_, publicID := tdb.createEntry(
		t,
		nil,
		"file.txt",
		"FILE",
		100,
		"users/test/file.txt",
		true,
		true,
	)

	err := PermanentlyDeleteEntry(
		context.Background(),
		publicID,
		tdb.userID,
	)
	if err != nil {
		t.Fatalf("permanent delete failed: %v", err)
	}

	if got := tdb.countEntries(t); got != 0 {
		t.Fatalf("remaining entries = %d, want 0", got)
	}

	if got := tdb.getUsedBytes(t); got != 900 {
		t.Fatalf("used bytes = %d, want 900", got)
	}

	eventsFound := tdb.getCleanupEvents(t, publicID)

	if len(eventsFound) != 1 {
		t.Fatalf("cleanup events = %d, want 1", len(eventsFound))
	}

	event := eventsFound[0]

	if event.EventType != events.EventS3Cleanup {
		t.Errorf("event type = %s, want %s", event.EventType, events.EventS3Cleanup)
	}

	if event.UserID != tdb.userID {
		t.Errorf("event user ID = %d, want %d", event.UserID, tdb.userID)
	}

	if len(event.Objects) != 1 {
		t.Fatalf("cleanup objects = %d, want 1", len(event.Objects))
	}

	if event.Objects[0].BucketName != pdTestBucket {
		t.Errorf("bucket = %s, want %s", event.Objects[0].BucketName, pdTestBucket)
	}

	if event.Objects[0].S3Key != "users/test/file.txt" {
		t.Errorf("S3 key = %s, want users/test/file.txt", event.Objects[0].S3Key)
	}

	t.Log("PASS: trashed file permanently deleted, quota released and cleanup event created")
}

// ------------------------------------------------------------
// Recursive folder deletion, including independently trashed
// descendants
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_FolderWithDescendants(t *testing.T) {
	tdb := pdSetupTestDB(t)
	pdSetBucket(t)

	tdb.createStorage(t, 10_000, 1_000)

	rootID, rootPublicID := tdb.createEntry(
		t,
		nil,
		"root",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	nestedID, _ := tdb.createEntry(
		t,
		&rootID,
		"nested",
		"FOLDER",
		0,
		"",
		true,
		false,
	)

	independentFolderID, _ := tdb.createEntry(
		t,
		&rootID,
		"independently-trashed",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	tdb.createEntry(
		t,
		&rootID,
		"root-file.txt",
		"FILE",
		100,
		"root/file.txt",
		true,
		false,
	)

	tdb.createEntry(
		t,
		&nestedID,
		"nested-file.txt",
		"FILE",
		200,
		"root/nested/file.txt",
		true,
		false,
	)

	tdb.createEntry(
		t,
		&independentFolderID,
		"independent-file.txt",
		"FILE",
		300,
		"root/independent/file.txt",
		true,
		false,
	)

	err := PermanentlyDeleteEntry(
		context.Background(),
		rootPublicID,
		tdb.userID,
	)
	if err != nil {
		t.Fatalf("folder permanent delete failed: %v", err)
	}

	if got := tdb.countEntries(t); got != 0 {
		t.Fatalf("remaining entries = %d, want 0", got)
	}

	if got := tdb.getUsedBytes(t); got != 400 {
		t.Fatalf("used bytes = %d, want 400", got)
	}

	eventsFound := tdb.getCleanupEvents(t, rootPublicID)

	if len(eventsFound) != 1 {
		t.Fatalf("cleanup events = %d, want 1", len(eventsFound))
	}

	event := eventsFound[0]

	expectedKeys := map[string]bool{
		"root/file.txt":             false,
		"root/nested/file.txt":      false,
		"root/independent/file.txt": false,
	}

	if len(event.Objects) != len(expectedKeys) {
		t.Fatalf(
			"cleanup objects = %d, want %d",
			len(event.Objects),
			len(expectedKeys),
		)
	}

	for _, object := range event.Objects {
		if object.BucketName != pdTestBucket {
			t.Errorf("unexpected bucket: %s", object.BucketName)
		}

		if _, exists := expectedKeys[object.S3Key]; !exists {
			t.Errorf("unexpected S3 key: %s", object.S3Key)
			continue
		}

		expectedKeys[object.S3Key] = true
	}

	for key, found := range expectedKeys {
		if !found {
			t.Errorf("S3 key missing from cleanup event: %s", key)
		}
	}

	t.Log("PASS: folder and all descendants, including independently trashed subtree, permanently deleted")
}

// ------------------------------------------------------------
// Empty folder
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_EmptyFolder(t *testing.T) {
	tdb := pdSetupTestDB(t)

	tdb.createStorage(t, 10_000, 500)

	_, publicID := tdb.createEntry(
		t,
		nil,
		"empty-folder",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	err := PermanentlyDeleteEntry(
		context.Background(),
		publicID,
		tdb.userID,
	)
	if err != nil {
		t.Fatalf("empty folder permanent delete failed: %v", err)
	}

	if got := tdb.countEntries(t); got != 0 {
		t.Fatalf("remaining entries = %d, want 0", got)
	}

	if got := tdb.getUsedBytes(t); got != 500 {
		t.Fatalf("used bytes = %d, want 500", got)
	}

	eventsFound := tdb.getCleanupEvents(t, publicID)

	if len(eventsFound) != 0 {
		t.Fatalf("cleanup events = %d, want 0 for empty folder", len(eventsFound))
	}

	t.Log("PASS: empty folder deleted without creating an empty S3 cleanup event")
}

// ------------------------------------------------------------
// Active entry and wrong user
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_RejectsActiveEntry(t *testing.T) {
	tdb := pdSetupTestDB(t)

	tdb.createStorage(t, 10_000, 100)

	_, publicID := tdb.createEntry(
		t,
		nil,
		"active-file.txt",
		"FILE",
		100,
		"active/file.txt",
		false,
		false,
	)

	err := PermanentlyDeleteEntry(
		context.Background(),
		publicID,
		tdb.userID,
	)

	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("error = %v, want ErrEntryNotFound", err)
	}

	if got := tdb.countEntries(t); got != 1 {
		t.Fatalf("remaining entries = %d, want 1", got)
	}

	if got := tdb.getUsedBytes(t); got != 100 {
		t.Fatalf("used bytes = %d, want 100", got)
	}

	t.Log("PASS: active entry cannot be permanently deleted")
}

func TestPermanentlyDeleteEntry_RejectsWrongUser(t *testing.T) {
	tdb := pdSetupTestDB(t)

	tdb.createStorage(t, 10_000, 100)

	_, publicID := tdb.createEntry(
		t,
		nil,
		"private-file.txt",
		"FILE",
		100,
		"private/file.txt",
		true,
		true,
	)

	err := PermanentlyDeleteEntry(
		context.Background(),
		publicID,
		tdb.userID+1,
	)

	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("error = %v, want ErrEntryNotFound", err)
	}

	if got := tdb.countEntries(t); got != 1 {
		t.Fatalf("remaining entries = %d, want 1", got)
	}

	t.Log("PASS: another user's entry cannot be permanently deleted")
}

// ------------------------------------------------------------
// Quota inconsistency must rollback the complete transaction
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_QuotaFailureRollsBack(t *testing.T) {
	tdb := pdSetupTestDB(t)
	pdSetBucket(t)

	// File size is 100, but used_bytes is only 50.
	tdb.createStorage(t, 10_000, 50)

	_, publicID := tdb.createEntry(
		t,
		nil,
		"quota-file.txt",
		"FILE",
		100,
		"quota/file.txt",
		true,
		true,
	)

	err := PermanentlyDeleteEntry(
		context.Background(),
		publicID,
		tdb.userID,
	)

	if err == nil {
		t.Fatal("expected quota inconsistency error, got nil")
	}

	if got := tdb.countEntries(t); got != 1 {
		t.Fatalf("remaining entries = %d, want 1 after rollback", got)
	}

	if got := tdb.getUsedBytes(t); got != 50 {
		t.Fatalf("used bytes = %d, want 50 after rollback", got)
	}

	eventsFound := tdb.getCleanupEvents(t, publicID)

	if len(eventsFound) != 0 {
		t.Fatalf("cleanup events = %d, want 0 after rollback", len(eventsFound))
	}

	t.Logf("PASS: quota failure rolled back permanent deletion and outbox insertion: %v", err)
}

// ------------------------------------------------------------
// Concurrent parent and child permanent deletion
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_ConcurrentParentChildDelete(t *testing.T) {
	tdb := pdSetupTestDB(t)
	pdSetBucket(t)

	tdb.createStorage(t, 10_000, 300)

	rootID, rootPublicID := tdb.createEntry(
		t,
		nil,
		"root",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	childID, childPublicID := tdb.createEntry(
		t,
		&rootID,
		"child",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	tdb.createEntry(
		t,
		&rootID,
		"root-file.txt",
		"FILE",
		200,
		"root/file.txt",
		true,
		false,
	)

	tdb.createEntry(
		t,
		&childID,
		"child-file.txt",
		"FILE",
		100,
		"root/child/file.txt",
		true,
		false,
	)

	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	errs := make(chan error, 2)

	go func() {
		defer wg.Done()
		<-start

		errs <- PermanentlyDeleteEntry(
			context.Background(),
			rootPublicID,
			tdb.userID,
		)
	}()

	go func() {
		defer wg.Done()
		<-start

		errs <- PermanentlyDeleteEntry(
			context.Background(),
			childPublicID,
			tdb.userID,
		)
	}()

	close(start)
	wg.Wait()
	close(errs)

	var childNotFound bool

	for err := range errs {
		if err == nil {
			continue
		}

		if errors.Is(err, ErrEntryNotFound) {
			childNotFound = true
			continue
		}

		t.Fatalf("unexpected concurrent delete error: %v", err)
	}

	// If parent deletion wins, child deletion may return not found.
	// If child deletion wins, parent deletion should still succeed.
	_ = childNotFound

	if got := tdb.countEntries(t); got != 0 {
		t.Fatalf("remaining entries = %d, want 0", got)
	}

	if got := tdb.getUsedBytes(t); got != 0 {
		t.Fatalf("used bytes = %d, want 0", got)
	}

	// Both possible execution orders must account for exactly two files.
	rows, err := tdb.pool.Query(context.Background(), `
		SELECT payload
		FROM outbox_events
		WHERE payload->>'userId' = $1
		  AND event_type = $2
	`, fmt.Sprint(tdb.userID), events.EventS3Cleanup)

	if err != nil {
		t.Fatalf("failed to query cleanup events: %v", err)
	}
	defer rows.Close()

	totalObjects := 0

	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("failed to scan cleanup event: %v", err)
		}

		var event events.S3CleanupEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("failed to decode cleanup event: %v", err)
		}

		totalObjects += len(event.Objects)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate cleanup events: %v", err)
	}

	if totalObjects != 2 {
		t.Fatalf("total cleanup objects = %d, want 2", totalObjects)
	}

	t.Log("PASS: concurrent parent-child permanent deletion preserved entries, quota and cleanup event consistency")
}

// ------------------------------------------------------------
// Missing entry
// ------------------------------------------------------------

func TestPermanentlyDeleteEntry_NotFound(t *testing.T) {
	tdb := pdSetupTestDB(t)

	tdb.createStorage(t, 10_000, 0)

	err := PermanentlyDeleteEntry(
		context.Background(),
		pdPublicID(),
		tdb.userID,
	)

	if !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("error = %v, want ErrEntryNotFound", err)
	}

	t.Log("PASS: missing entry returns ErrEntryNotFound")
}

// ------------------------------------------------------------
// Hierarchy validation
// ------------------------------------------------------------

func TestLockPermanentDeleteHierarchy_RejectsFileAsParent(t *testing.T) {
	tdb := pdSetupTestDB(t)

	tdb.createStorage(t, 10_000, 0)

	fileID, _ := tdb.createEntry(
		t,
		nil,
		"parent-file.txt",
		"FILE",
		0,
		"",
		true,
		true,
	)

	childID, _ := tdb.createEntry(
		t,
		&fileID,
		"child",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	tx, err := tdb.pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(context.Background())

	err = LockPermanentDeleteHierarchy(
		tx,
		context.Background(),
		tdb.userID,
		childID,
		"FOLDER",
	)

	if !errors.Is(err, ErrInvalidEntryHierarchy) {
		t.Fatalf("error = %v, want ErrInvalidEntryHierarchy", err)
	}

	t.Log("PASS: invalid hierarchy with a file parent is rejected")
}

func TestPermanentlyDeleteEntry_ConcurrentRestore(t *testing.T) {
	tdb := pdSetupTestDB(t)
	pdSetBucket(t)

	tdb.createStorage(t, 10_000, 300)

	rootID, rootPublicID := tdb.createEntry(
		t,
		nil,
		"root",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	childID, childPublicID := tdb.createEntry(
		t,
		&rootID,
		"child",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	tdb.createEntry(
		t,
		&childID,
		"file.txt",
		"FILE",
		300,
		"root/child/file.txt",
		true,
		false,
	)

	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	errs := make(chan error, 2)

	go func() {
		defer wg.Done()
		<-start

		errs <- PermanentlyDeleteEntry(
			context.Background(),
			rootPublicID,
			tdb.userID,
		)
	}()

	go func() {
		defer wg.Done()
		<-start

		errs <- RestoreFolder(
			context.Background(),
			childPublicID,
			tdb.userID,
		)
	}()

	close(start)
	wg.Wait()
	close(errs)

	var (
		deleteSuccess  bool
		restoreSuccess bool
	)

	for err := range errs {
		if err == nil {
			// Both operations cannot succeed on the same subtree.
			continue
		}

		if !errors.Is(err, ErrEntryNotFound) {
			t.Fatalf("unexpected concurrent operation error: %v", err)
		}
	}

	// Inspect the final database state.
	var rootExists bool
	err := tdb.pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM entries
			WHERE public_id = $1 AND user_id = $2
		)
	`, rootPublicID, tdb.userID).Scan(&rootExists)

	if err != nil {
		t.Fatal(err)
	}

	var childDeletedAt *time.Time
	err = tdb.pool.QueryRow(context.Background(), `
		SELECT deleted_at
		FROM entries
		WHERE public_id = $1 AND user_id = $2
	`, childPublicID, tdb.userID).Scan(&childDeletedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		// Parent permanent deletion cascaded to child.
		if rootExists {
			t.Fatal("root exists but child was deleted")
		}

		if got := tdb.getUsedBytes(t); got != 0 {
			t.Fatalf("used bytes = %d, want 0", got)
		}

		t.Log("PASS: permanent delete won; restore did not resurrect deleted subtree")
		return
	}

	if err != nil {
		t.Fatal(err)
	}

	// If restore won, the child must be active and the parent must
	// remain present (but trashed).
	if !rootExists {
		t.Fatal("child exists but parent does not")
	}

	if childDeletedAt != nil {
		t.Fatal("child should be restored when permanent delete did not win")
	}

	if got := tdb.getUsedBytes(t); got != 300 {
		t.Fatalf("used bytes = %d, want 300", got)
	}

	_ = deleteSuccess
	_ = restoreSuccess

	t.Log("PASS: restore won; permanent delete did not remove restored subtree")
}

func TestPermanentlyDeleteEntry_ConcurrentUploadFinalization(t *testing.T) {
	tdb := pdSetupTestDB(t)
	pdSetBucket(t)

	const uploadSize int64 = 100

	tdb.createStorage(t, 10_000, 0)

	rootID, rootPublicID := tdb.createEntry(
		t,
		nil,
		"root",
		"FOLDER",
		0,
		"",
		true,
		true,
	)

	// Simulate the reservation created by UploadFile.
	_, err := tdb.pool.Exec(context.Background(), `
		UPDATE user_storage
		SET reserved_bytes = $1
		WHERE user_id = $2
	`, uploadSize, tdb.userID)
	if err != nil {
		t.Fatalf("failed to reserve storage: %v", err)
	}

	var reservationID int64

	err = tdb.pool.QueryRow(context.Background(), `
		INSERT INTO storage_reservations (
			user_id,
			size_bytes,
			status,
			lease_expires_at
		)
		VALUES (
			$1,
			$2,
			'PENDING',
			CURRENT_TIMESTAMP + INTERVAL '10 minutes'
		)
		RETURNING id
	`, tdb.userID, uploadSize).Scan(&reservationID)

	if err != nil {
		t.Fatalf("failed to create storage reservation: %v", err)
	}

	uploadPublicID := pdPublicID()

	uploadData := &models.EntryData{
		PublicId:    uploadPublicID,
		UserId:      tdb.userID,
		ParentId:    &rootID,
		Name:        "uploaded.txt",
		Type:        "FILE",
		ContentType: "text/plain",
		Extension:   "txt",
		Size:        uploadSize,
		S3Key: sql.NullString{
			String: "root/uploaded.txt",
			Valid:  true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	start := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	var (
		uploadErr error
		deleteErr error
	)

	go func() {
		defer wg.Done()
		<-start

		_, uploadErr = InsertEntryAndFinalizeStorage(
			context.Background(),
			uploadData,
			reservationID,
		)
	}()

	go func() {
		defer wg.Done()
		<-start

		deleteErr = PermanentlyDeleteEntry(
			context.Background(),
			rootPublicID,
			tdb.userID,
		)
	}()

	close(start)
	wg.Wait()

	if deleteErr != nil {
		t.Fatalf("permanent delete failed: %v", deleteErr)
	}

	// Upload can either:
	// 1. Win the hierarchy lock and commit. Parent deletion then
	//    deletes the uploaded file as part of its subtree.
	// 2. Lose the hierarchy lock. Finalization returns ErrEntryNotFound.
	if uploadErr != nil && !errors.Is(uploadErr, ErrEntryNotFound) {
		t.Fatalf("unexpected upload finalization error: %v", uploadErr)
	}

	// Parent deletion must leave no entries behind in either case.
	if got := tdb.countEntries(t); got != 0 {
		t.Fatalf("remaining entries = %d, want 0", got)
	}

	// Parent deletion must release used quota, including the uploaded
	// file if upload finalization won the race.
	if got := tdb.getUsedBytes(t); got != 0 {
		t.Fatalf("used bytes = %d, want 0", got)
	}

	var (
		reservedBytes     int64
		reservationStatus string
	)

	err = tdb.pool.QueryRow(context.Background(), `
		SELECT
			us.reserved_bytes,
			sr.status
		FROM user_storage us
		JOIN storage_reservations sr
		  ON sr.user_id = us.user_id
		WHERE us.user_id = $1
		  AND sr.id = $2
	`, tdb.userID, reservationID).Scan(
		&reservedBytes,
		&reservationStatus,
	)

	if err != nil {
		t.Fatalf("failed to inspect reservation: %v", err)
	}

	if uploadErr == nil {
		// Upload finalized first. Parent deletion subsequently released
		// its used quota.
		if reservationStatus != "FINALIZED" {
			t.Fatalf(
				"reservation status = %s, want FINALIZED",
				reservationStatus,
			)
		}

		if reservedBytes != 0 {
			t.Fatalf("reserved bytes = %d, want 0", reservedBytes)
		}

		eventsFound := tdb.getCleanupEvents(t, rootPublicID)
		if len(eventsFound) != 1 {
			t.Fatalf("cleanup events = %d, want 1", len(eventsFound))
		}

		if len(eventsFound[0].Objects) != 1 {
			t.Fatalf(
				"cleanup objects = %d, want 1",
				len(eventsFound[0].Objects),
			)
		}

		if eventsFound[0].Objects[0].S3Key != "root/uploaded.txt" {
			t.Errorf(
				"cleanup key = %s, want root/uploaded.txt",
				eventsFound[0].Objects[0].S3Key,
			)
		}

		t.Log("PASS: upload finalized first; permanent delete removed uploaded entry and released its quota")
	} else {
		// Permanent deletion won. Upload finalization was rejected.
		// The handler's deferred cleanup would release this reservation.
		if reservationStatus != "PENDING" {
			t.Fatalf(
				"reservation status = %s, want PENDING before cleanup",
				reservationStatus,
			)
		}

		if reservedBytes != uploadSize {
			t.Fatalf(
				"reserved bytes = %d, want %d before cleanup",
				reservedBytes,
				uploadSize,
			)
		}

		if err := ReleaseStorageReservationByID(
			context.Background(),
			reservationID,
		); err != nil {
			t.Fatalf("failed to release rejected upload reservation: %v", err)
		}

		if got := tdb.getUsedBytes(t); got != 0 {
			t.Fatalf("used bytes after reservation cleanup = %d, want 0", got)
		}

		var finalReservedBytes int64

		err = tdb.pool.QueryRow(context.Background(), `
			SELECT reserved_bytes
			FROM user_storage
			WHERE user_id = $1
		`, tdb.userID).Scan(&finalReservedBytes)

		if err != nil {
			t.Fatal(err)
		}

		if finalReservedBytes != 0 {
			t.Fatalf(
				"reserved bytes after cleanup = %d, want 0",
				finalReservedBytes,
			)
		}

		eventsFound := tdb.getCleanupEvents(t, rootPublicID)
		if len(eventsFound) != 0 {
			t.Fatalf(
				"cleanup events = %d, want 0 because upload did not finalize",
				len(eventsFound),
			)
		}

		t.Log("PASS: permanent delete won; upload finalization was rejected and reservation was released")
	}
}

// Compile-time assertion that the test uses the expected pgx transaction API.
var _ pgx.Tx
