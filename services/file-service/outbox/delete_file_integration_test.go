package outbox

import (
	"context"
	"database/sql"
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

func TestDeleteFile_PublishesOutboxEvent(t *testing.T) {
	envPath, err := findEnvFile()
	if err != nil {
		t.Fatalf("failed to find .env file: %v", err)
	}

	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is not configured")
	}

	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	if len(brokers) == 0 || brokers[0] == "" {
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

	// Register pool cleanup before fixture cleanup.
	// Cleanup callbacks execute in reverse registration order.
	t.Cleanup(func() {
		db.DB = previousDB
		pool.Close()
	})

	testID := uuid.NewString()
	publicID := strings.ReplaceAll(testID, "-", "")[:21]
	userID := time.Now().UnixNano()
	fileSize := int64(1024)

	s3Key := fmt.Sprintf(
		"integration-tests/%s/test-file.txt",
		testID,
	)

	// Create storage fixture.
	_, err = pool.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes
		)
		VALUES ($1, 10485760, $2, 0)
	`, userID, fileSize)
	if err != nil {
		t.Fatalf("failed to create storage fixture: %v", err)
	}

	// Create a real FILE entry.
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
		VALUES ($1, $2, NULL, $3, 'FILE', $4, $5, $6, $7)
	`, publicID, userID, "test-file", "text/plain", "txt", fileSize, s3Key)
	if err != nil {
		t.Fatalf("failed to create file fixture: %v", err)
	}

	// Clean up only this test's data.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cleanupCancel()

		_, err := pool.Exec(cleanupCtx,
			`DELETE FROM outbox_events WHERE aggregate_id = $1`,
			publicID,
		)
		if err != nil {
			t.Errorf("failed to clean outbox fixture: %v", err)
		}

		_, err = pool.Exec(cleanupCtx,
			`DELETE FROM entries WHERE public_id = $1 AND user_id = $2`,
			publicID,
			userID,
		)
		if err != nil {
			t.Errorf("failed to clean entry fixture: %v", err)
		}

		_, err = pool.Exec(cleanupCtx,
			`DELETE FROM user_storage WHERE user_id = $1`,
			userID,
		)
		if err != nil {
			t.Errorf("failed to clean storage fixture: %v", err)
		}
	})

	// Call the actual production deletion function.
	if err := db.DeleteFile(ctx, publicID, userID); err != nil {
		t.Fatalf("DeleteFile failed: %v", err)
	}

	// Verify that the entry was soft-deleted.
	var deletedAt sql.NullTime
	err = pool.QueryRow(ctx, `
		SELECT deleted_at
		FROM entries
		WHERE public_id = $1 AND user_id = $2
	`, publicID, userID).Scan(&deletedAt)

	if err != nil {
		t.Fatalf("failed to read deleted entry: %v", err)
	}
	if !deletedAt.Valid {
		t.Fatal("expected entry to be soft-deleted")
	}

	// Verify quota release.
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

	// Get the event created by DeleteFile's transaction.
	var eventID string
	err = pool.QueryRow(ctx, `
		SELECT event_id
		FROM outbox_events
		WHERE aggregate_id = $1
		  AND event_type = 'FILE_DELETED'
	`, publicID).Scan(&eventID)

	if err != nil {
		t.Fatalf("FILE_DELETED outbox event not found: %v", err)
	}

	// Create an isolated Kafka topic.
	testTopic := "fastfiles-delete-integration-" + testID
	createKafkaTopic(t, brokers, testTopic)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       testTopic,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer reader.Close()

	// Start the actual outbox publisher.
	publisherCtx, stopPublisher := context.WithCancel(ctx)
	publisherDone := make(chan struct{})

	go func() {
		defer close(publisherDone)
		Run(publisherCtx, brokers, testTopic)
	}()

	stop := func() {
		stopPublisher()
		select {
		case <-publisherDone:
		case <-time.After(5 * time.Second):
			t.Error("publisher did not stop within 5 seconds")
		}
	}
	defer stop()

	// Read until our specific event arrives.
	found := false

	for !found {
		message, err := reader.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("failed to read Kafka message: %v", err)
		}

		var event events.EntryEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			t.Fatalf("failed to decode Kafka event: %v", err)
		}

		if event.EventID != eventID {
			continue
		}

		if event.EventType != events.EventFileDeleted {
			t.Fatalf("expected FILE_DELETED, got %s", event.EventType)
		}
		if event.PublicID != publicID {
			t.Fatalf("expected public ID %s, got %s", publicID, event.PublicID)
		}
		if event.UserID != userID {
			t.Fatalf("expected user ID %d, got %d", userID, event.UserID)
		}
		if event.EntryType != "FILE" {
			t.Fatalf("expected entry type FILE, got %s", event.EntryType)
		}
		if event.Size != fileSize {
			t.Fatalf("expected size %d, got %d", fileSize, event.Size)
		}

		found = true
	}

	// Verify that the publisher marked the event as PUBLISHED.
	for {
		var status string
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
			t.Fatalf("event was received but status did not become PUBLISHED; last status: %s", status)
		case <-time.After(200 * time.Millisecond):
		}
	}

	t.Logf(
		"SUCCESS: DeleteFile soft-deleted file, released quota, created outbox event %s, and publisher delivered it to Kafka",
		eventID,
	)
}
