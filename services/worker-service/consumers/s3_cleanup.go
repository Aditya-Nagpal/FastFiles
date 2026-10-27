package consumers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/models"
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

			if dlqErr := PublishToS3CleanupDLQ(
				ctx, brokers, message, "unknown", err,
			); dlqErr != nil {
				log.Printf("S3 cleanup: failed to publish malformed event to DLQ: %v", dlqErr)
				if !wait(ctx, 2*time.Second) {
					return nil
				}
				continue
			}

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

		sentToDLQ := false

		for {
			err = workers.HandleS3CleanupEvent(ctx, message.Value)
			if err == nil {
				break
			}

			var permanentErr *workers.PermanentError
			if errors.As(err, &permanentErr) {
				if dlqErr := PublishToS3CleanupDLQ(
					ctx, brokers, message, event.EventID, permanentErr,
				); dlqErr != nil {
					log.Printf("S3 cleanup: DLQ publish failed for event %s: %v",
						event.EventID, dlqErr)

					if !wait(ctx, 2*time.Second) {
						return nil
					}
					continue
				}

				if err := commitWithRetry(ctx, reader, message); err != nil {
					return err
				}

				log.Printf("S3 cleanup: permanent failure sent to DLQ; event=%s",
					event.EventID)
				sentToDLQ = true
				break
			}

			log.Printf("S3 cleanup failed for event %s: %v", event.EventID, err)

			if !wait(ctx, 2*time.Second) {
				return nil
			}
		}

		if sentToDLQ {
			continue
		}

		if err := commitWithRetry(ctx, reader, message); err != nil {
			return err
		}

		log.Printf("S3 cleanup completed: event=%s type=%s",
			event.EventID, event.EventType)
	}
}

func PublishToS3CleanupDLQ(
	ctx context.Context,
	brokers []string,
	original kafka.Message,
	eventID string,
	cause error,
) error {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        "s3-cleanup-dlq",
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
	defer writer.Close()

	payload, err := json.Marshal(models.S3CleanupDLQMessage{
		EventID:         eventID,
		Error:           cause.Error(),
		OriginalPayload: string(original.Value),
		FailedAt:        time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal DLQ message: %w", err)
	}

	return writer.WriteMessages(ctx, kafka.Message{
		Key:   original.Key,
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(eventID)},
			{Key: "failure_reason", Value: []byte(cause.Error())},
		},
	})

}
