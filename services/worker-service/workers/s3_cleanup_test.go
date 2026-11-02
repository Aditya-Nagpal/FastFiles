package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type fakeS3DeleteClient struct {
	deletedKeys []string
	err         error
}

func (f *fakeS3DeleteClient) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	f.deletedKeys = append(f.deletedKeys, *params.Key)

	if f.err != nil {
		return nil, f.err
	}

	return &s3.DeleteObjectOutput{}, nil
}

func TestHandleS3CleanupEvent_FileDeleted(t *testing.T) {
	ctx := context.Background()

	// Fake S3 client
	fakeClient := &fakeS3DeleteClient{}

	// Event payload
	event := events.EntryEvent{
		EventID:   "test-event-1",
		EventType: events.EventFileDeleted,
		UserID:    1,
		EntryID:   100,
	}

	message, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	// THIS IS WHERE THE LINE GOES.
	err = handleS3CleanupEvent(
		ctx,
		message,
		fakeClient.deleteObject,
	)

	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}

func (f *fakeS3DeleteClient) deleteObject(ctx context.Context, key string) error {
	return utils.DeleteS3ObjectWithClient(ctx, f, "test-bucket", key)
}

func TestHandleS3CleanupEvent_FolderDeleted(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
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

	previousDB := db.DB
	db.DB = pool
	defer func() {
		db.DB = previousDB
	}()

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

		_, _ = pool.Exec(
			cleanupCtx,
			`DELETE FROM entries WHERE user_id = $1`,
			userID,
		)
	})

	// Create root folder.
	var rootID int64
	err = pool.QueryRow(ctx, `
        INSERT INTO entries (
            public_id, user_id, name, type, extension, size
        )
        VALUES ($1, $2, 'root', 'FOLDER', '', 0)
        RETURNING id
    `, rootPublicID, userID).Scan(&rootID)
	if err != nil {
		t.Fatal(err)
	}

	// Create nested folder.
	var childID int64
	err = pool.QueryRow(ctx, `
        INSERT INTO entries (
            public_id, user_id, parent_id, name, type, extension, size
        )
        VALUES ($1, $2, $3, 'nested', 'FOLDER', '', 0)
        RETURNING id
    `, childPublicID, userID, rootID).Scan(&childID)
	if err != nil {
		t.Fatal(err)
	}

	expectedKeys := map[string]bool{
		"users/test/files/file-a.txt": false,
		"users/test/files/file-b.txt": false,
	}

	// Create files inside nested folder.
	_, err = pool.Exec(ctx, `
        INSERT INTO entries (
            public_id, user_id, parent_id, name,
            type, extension, size, s3_key
        )
        VALUES
            ($1, $2, $3, 'file-a', 'FILE', 'txt', 100, $4),
            ($5, $2, $3, 'file-b', 'FILE', 'txt', 200, $6)
    `,
		filePublicID1, userID, childID,
		"users/test/files/file-a.txt",
		filePublicID2, "users/test/files/file-b.txt",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate DeleteFolder: all descendants are soft-deleted.
	_, err = pool.Exec(ctx, `
        UPDATE entries
        SET deleted_at = CURRENT_TIMESTAMP,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = $1
    `, userID)
	if err != nil {
		t.Fatal(err)
	}

	// Build FOLDER_DELETED event.
	event := events.EntryEvent{
		EventID:   "test-folder-delete-event",
		EventType: events.EventFolderDeleted,
		UserID:    userID,
		EntryID:   rootID,
	}

	message, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	// Fake S3 client.
	fakeClient := &fakeS3DeleteClient{}

	// Execute actual worker handler with fake S3 deletion.
	err = handleS3CleanupEvent(
		ctx,
		message,
		fakeClient.deleteObject,
	)
	if err != nil {
		t.Fatalf("folder S3 cleanup failed: %v", err)
	}

	if len(fakeClient.deletedKeys) != len(expectedKeys) {
		t.Fatalf(
			"deleted keys count = %d, want %d; keys=%v",
			len(fakeClient.deletedKeys),
			len(expectedKeys),
			fakeClient.deletedKeys,
		)
	}

	for _, key := range fakeClient.deletedKeys {
		if _, exists := expectedKeys[key]; !exists {
			t.Errorf("unexpected S3 key deleted: %s", key)
			continue
		}

		expectedKeys[key] = true
	}

	for key, deleted := range expectedKeys {
		if !deleted {
			t.Errorf("S3 key was not deleted: %s", key)
		}
	}

	t.Logf(
		"PASS: folder cleanup deleted all descendant S3 objects: %v",
		fakeClient.deletedKeys,
	)
}
