package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/workers"
	"github.com/segmentio/kafka-go"
)

func RunFolderSizeConsumer(
	ctx context.Context,
	brokers []string,
	topic string,
) error {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  "folder-size-worker",
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	defer reader.Close()

	log.Printf("Folder-size consumer started; topic=%s", topic)

	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}

			log.Printf("Kafka fetch failed: %v", err)
			if !wait(ctx, 2*time.Second) {
				return nil
			}
			continue
		}

		// Invalid JSON cannot succeed on retry. Log and commit it
		// to avoid blocking this partition indefinitely.
		if !json.Valid(message.Value) {
			log.Printf(
				"Skipping malformed Kafka message at partition=%d offset=%d",
				message.Partition,
				message.Offset,
			)
			if err := commitWithRetry(ctx, reader, message); err != nil {
				return err
			}
			continue
		}

		var event events.EntryEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			log.Printf("Skipping invalid event at offset=%d: %v", message.Offset, err)
			if err := commitWithRetry(ctx, reader, message); err != nil {
				return err
			}
			continue
		}

		// Retry the same message until processing succeeds or shutdown begins.
		for {
			err = workers.HandleFolderSizeEvent(ctx, message.Value)
			if err == nil {
				break
			}

			log.Printf(
				"Folder-size processing failed for event %s: %v",
				event.EventID,
				err,
			)

			if !wait(ctx, 2*time.Second) {
				return nil
			}
		}

		// Commit only after successful processing.
		if err := commitWithRetry(ctx, reader, message); err != nil {
			return err
		}
	}
}

func commitWithRetry(
	ctx context.Context,
	reader *kafka.Reader,
	message kafka.Message,
) error {
	for {
		err := reader.CommitMessages(ctx, message)
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return nil
		}

		log.Printf("Kafka offset commit failed: %v", err)

		if !wait(ctx, time.Second) {
			return nil
		}
	}
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
