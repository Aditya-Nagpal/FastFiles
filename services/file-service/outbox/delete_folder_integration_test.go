package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/segmentio/kafka-go"
)

func TestDeleteFolder_PublishesOutboxEvent(t *testing.T) {
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
	var fileAID int64

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
		VALUES ($1, $2, $3, 'file-a', 'FILE', 'text/plain', 'txt', $4, $5)
		RETURNING id
	`,
		filePublicIDs[0],
		userID,
		rootID,
		fileSize,
		fmt.Sprintf("integration-tests/%s/file-a.txt", testID),
	).Scan(&fileAID)

	if err != nil {
		t.Fatalf("failed to create root file: %v", err)
	}

	// 5. Create two files under nested folder.
	var nestedFileIDs []int64

	for i := 1; i < 3; i++ {
		var entryID int64

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
			VALUES ($1, $2, $3, $4, 'FILE', 'text/plain', 'txt', $5, $6)
			RETURNING id
		`,
			filePublicIDs[i],
			userID,
			nestedID,
			fmt.Sprintf("file-%d", i+1),
			fileSize,
			fmt.Sprintf("integration-tests/%s/nested/file-%d.txt", testID, i+1),
		).Scan(&entryID)

		if err != nil {
			t.Fatalf("failed to create nested file %d: %v", i+1, err)
		}

		nestedFileIDs = append(nestedFileIDs, entryID)
	}

	// Cleanup only this test's data.
	// Registered after pool cleanup so it executes first.
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

	// 6. Call the actual production DeleteFolder function.
	if err := db.DeleteFolder(ctx, rootPublicID, userID); err != nil {
		t.Fatalf("DeleteFolder failed: %v", err)
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
		t.Fatalf("failed to verify deleted entries: %v", err)
	}

	if totalEntries != 5 {
		t.Fatalf("expected 5 entries, found %d", totalEntries)
	}

	if deletedEntries != 5 {
		t.Fatalf("expected all 5 entries soft-deleted, found %d", deletedEntries)
	}

	// 8. Verify aggregate quota release.
	var usedBytes int64

	err = pool.QueryRow(ctx, `
		SELECT used_bytes
		FROM user_storage
		WHERE user_id = $1
	`, userID).Scan(&usedBytes)

	if err != nil {
		t.Fatalf("failed to read storage usage: %v", err)
	}

	if usedBytes != 0 {
		t.Fatalf("expected used_bytes=0, got %d", usedBytes)
	}

	// 9. Fetch the actual outbox event created by DeleteFolder.
	var (
		eventID string
		payload []byte
		status  string
	)

	err = pool.QueryRow(ctx, `
		SELECT event_id, payload, status
		FROM outbox_events
		WHERE aggregate_id = $1
		  AND event_type = 'FOLDER_DELETED'
	`, rootPublicID).Scan(&eventID, &payload, &status)

	if err != nil {
		t.Fatalf("FOLDER_DELETED outbox event not found: %v", err)
	}

	var event events.EntryEvent

	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("failed to decode outbox payload: %v", err)
	}

	if event.EventID != eventID {
		t.Fatalf("event ID mismatch: payload=%s database=%s", event.EventID, eventID)
	}

	if event.EventType != events.EventFolderDeleted {
		t.Fatalf("expected FOLDER_DELETED, got %s", event.EventType)
	}

	if event.EntryID != rootID {
		t.Fatalf("expected root entry ID %d, got %d", rootID, event.EntryID)
	}

	if event.UserID != userID {
		t.Fatalf("expected user ID %d, got %d", userID, event.UserID)
	}

	if event.Size != totalSize {
		t.Fatalf("expected event size %d, got %d", totalSize, event.Size)
	}

	if status != "PENDING" {
		t.Fatalf("expected initial outbox status PENDING, got %s", status)
	}

	// 10. Create an isolated Kafka topic.
	testTopic := "fastfiles-delete-folder-" + testID
	createKafkaTopic(t, brokers, testTopic)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       testTopic,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer reader.Close()

	// 11. Start the actual publisher.
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

	// 12. Read until our specific folder event arrives.
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

		if publishedEvent.EventType != events.EventFolderDeleted {
			t.Fatalf(
				"expected FOLDER_DELETED, got %s",
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

		if publishedEvent.Size != totalSize {
			t.Fatalf(
				"expected event size %d, got %d",
				totalSize,
				publishedEvent.Size,
			)
		}

		found = true
	}

	// 13. Verify publisher marked the event as PUBLISHED.
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
		"SUCCESS: DeleteFolder soft-deleted all descendants, released %d bytes, created FOLDER_DELETED event %s, and publisher delivered it to Kafka",
		totalSize,
		eventID,
	)
}
