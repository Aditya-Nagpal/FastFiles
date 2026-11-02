package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func TestOutboxPublisher_PublishesPendingEvent(t *testing.T) {
	// 1. Load environment.
	envPath, err := findEnvFile()
	if err != nil {
		t.Fatalf("failed to locate .env: %v", err)
	}

	if err := godotenv.Load(envPath); err != nil {
		t.Fatalf("failed to load .env: %v", err)
	}

	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run integration tests")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("DATABASE_URL is required")
	}

	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
		if brokers[i] == "" {
			t.Fatal("KAFKA_BROKERS contains an empty broker address")
		}
	}

	// 2. Setup context and database.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create database pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("database ping: %v", err)
	}

	previousDB := db.DB
	db.DB = pool
	defer func() {
		db.DB = previousDB
	}()

	// 3. Create a unique test event.
	eventID := uuid.NewString()
	publicID := "integration-file-" + uuid.NewString()

	payload, err := json.Marshal(events.EntryEvent{
		EventID:     eventID,
		EventType:   events.EventFileDeleted,
		AggregateID: publicID,
		UserID:      987654321,
		EntryID:     987654321,
		PublicID:    publicID,
		EntryType:   "FILE",
		Size:        10,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO outbox_events (
			event_id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload
		)
		VALUES ($1, 'ENTRY', $2, $3, $4::jsonb)
	`, eventID, publicID, events.EventFileDeleted, payload)
	if err != nil {
		t.Fatalf("insert pending outbox event: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_, cleanupErr := pool.Exec(
			cleanupCtx,
			`DELETE FROM outbox_events WHERE event_id = $1`,
			eventID,
		)
		if cleanupErr != nil {
			t.Logf("outbox test cleanup failed: %v", cleanupErr)
		}
	})

	// 4. Create a dedicated Kafka topic for this test.
	testTopic := "fastfiles-integration-" + uuid.NewString()

	createKafkaTopic(t, brokers, testTopic)

	// 5. Create reader before starting the publisher.
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		Topic:       testTopic,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer reader.Close()

	// 6. Start the actual outbox publisher.
	publisherCtx, stopPublisher := context.WithCancel(ctx)
	publisherDone := make(chan struct{})

	go func() {
		defer close(publisherDone)
		Run(publisherCtx, brokers, testTopic)
	}()

	publisherStopped := false

	stop := func() {
		if publisherStopped {
			return
		}

		publisherStopped = true
		stopPublisher()
		<-publisherDone
	}
	defer stop()

	// 7. Read the published Kafka message.
	message, err := reader.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read published Kafka message: %v", err)
	}

	var received events.EntryEvent
	if err := json.Unmarshal(message.Value, &received); err != nil {
		t.Fatalf("unmarshal Kafka message: %v", err)
	}

	// 8. Validate the received event.
	if received.EventID != eventID {
		t.Fatalf(
			"event ID mismatch: got %q, want %q",
			received.EventID,
			eventID,
		)
	}

	if received.EventType != events.EventFileDeleted {
		t.Fatalf(
			"event type mismatch: got %q, want %q",
			received.EventType,
			events.EventFileDeleted,
		)
	}

	if received.PublicID != publicID {
		t.Fatalf(
			"public ID mismatch: got %q, want %q",
			received.PublicID,
			publicID,
		)
	}

	// 9. Wait until the publisher marks the event as PUBLISHED.
	// Do this BEFORE cancelling the publisher context.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		var status string

		err := pool.QueryRow(ctx, `
			SELECT status
			FROM outbox_events
			WHERE event_id = $1
		`, eventID).Scan(&status)

		if err == nil && status == "PUBLISHED" {
			break
		}

		if err != nil {
			t.Logf("waiting for outbox status update: %v", err)
		}

		select {
		case <-ctx.Done():
			t.Fatalf(
				"outbox event was not marked PUBLISHED before timeout; last status=%q, error=%v",
				status,
				err,
			)
		case <-ticker.C:
		}
	}

	// 10. Stop publisher only after successful DB status update.
	stop()

	t.Logf(
		"SUCCESS: event %s published to Kafka and marked PUBLISHED",
		eventID,
	)
}

func findEnvFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(dir, ".env")

		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return "", fmt.Errorf(".env not found in current or parent directories")
}

func createKafkaTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()

	if len(brokers) == 0 {
		t.Fatal("no Kafka brokers configured")
	}

	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		t.Fatalf("dial Kafka broker: %v", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		t.Fatalf("get Kafka controller: %v", err)
	}

	controllerConn, err := kafka.Dial(
		"tcp",
		fmt.Sprintf("%s:%d", controller.Host, controller.Port),
	)
	if err != nil {
		t.Fatalf("dial Kafka controller: %v", err)
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil && !strings.Contains(
		strings.ToLower(err.Error()),
		"already exists",
	) {
		t.Fatalf("create Kafka topic: %v", err)
	}
}
