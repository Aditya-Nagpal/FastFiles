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

func RunS3CleanupConsumer(
	ctx context.Context,
	brokers []string,
	topic string,
) error {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  "s3-cleanup-worker",
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer reader.Close()

	log.Printf("S3 cleanup consumer started; topic=%s", topic)

	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return nil
			}

			log.Printf("S3 cleanup: Kafka fetch failed: %v", err)
			if !wait(ctx, 2*time.Second) {
				return nil
			}
			continue
		}

		var event events.EntryEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			log.Printf("S3 cleanup: malformed event at offset=%d: %v",
				message.Offset, err)
			if err := commitWithRetry(ctx, reader, message); err != nil {
				return err
			}
			continue
		}

		switch event.EventType {
		case events.EventFileDeleted, events.EventFolderDeleted:
		default:
			// This group also receives FILE_CREATED and ignores it.
			if err := commitWithRetry(ctx, reader, message); err != nil {
				return err
			}
			continue
		}

		for {
			err = workers.HandleS3CleanupEvent(ctx, message.Value)
			if err == nil {
				break
			}

			log.Printf("S3 cleanup failed for event %s: %v",
				event.EventID, err)

			if !wait(ctx, 2*time.Second) {
				return nil
			}
		}

		if err := commitWithRetry(ctx, reader, message); err != nil {
			return err
		}

		log.Printf("S3 cleanup completed: event=%s type=%s",
			event.EventID, event.EventType)
	}
}
