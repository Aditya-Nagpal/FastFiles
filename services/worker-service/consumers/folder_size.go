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

const folderSizeDLQTopic = "folder-size-dlq"

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

		var event events.EntryEvent

		// Malformed JSON cannot succeed on retry.
		if err := json.Unmarshal(message.Value, &event); err != nil {
			permanentErr := fmt.Errorf("invalid entry event JSON: %w", err)

			if err := publishAndCommitFolderSizeDLQ(
				ctx, brokers, reader, message, "unknown", permanentErr,
			); err != nil {
				return err
			}
			continue
		}

		// Retry processing until success, permanent failure,
		// or shutdown.
		for {
			err := workers.HandleFolderSizeEvent(ctx, message.Value)
			if err == nil {
				break
			}

			var permanentErr *workers.PermanentError
			if errors.As(err, &permanentErr) {
				log.Printf(
					"Folder-size permanent failure at offset=%d: %v",
					message.Offset,
					err,
				)

				if err := publishAndCommitFolderSizeDLQ(
					ctx, brokers, reader, message, event.EventID, err,
				); err != nil {
					return err
				}

				// Already committed after successful DLQ publish.
				// Do not commit this message again.
				goto nextMessage
			}

			log.Printf(
				"Folder-size transient failure for event %s: %v",
				event.EventID,
				err,
			)

			if !wait(ctx, 2*time.Second) {
				return nil
			}
		}

		// Normal successful processing: commit once.
		if err := commitWithRetry(ctx, reader, message); err != nil {
			return err
		}

	nextMessage:
		continue
	}
}

func publishAndCommitFolderSizeDLQ(
	ctx context.Context,
	brokers []string,
	reader *kafka.Reader,
	original kafka.Message,
	eventID string,
	cause error,
) error {
	// Retry DLQ publishing. Never commit the original event
	// until the DLQ write succeeds.
	for {
		err := PublishToFolderSizeDLQ(
			ctx, brokers, original, eventID, cause,
		)
		if err == nil {
			break
		}

		log.Printf("Folder-size DLQ publish failed: %v", err)

		if !wait(ctx, 2*time.Second) {
			return ctx.Err()
		}
	}

	if err := commitWithRetry(ctx, reader, original); err != nil {
		return fmt.Errorf(
			"folder-size DLQ published but original offset commit failed: %w",
			err,
		)
	}

	log.Printf(
		"Folder-size permanent failure sent to DLQ; partition=%d offset=%d",
		original.Partition,
		original.Offset,
	)

	return nil
}

func PublishToFolderSizeDLQ(
	ctx context.Context,
	brokers []string,
	original kafka.Message,
	eventID string,
	cause error,
) error {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        folderSizeDLQTopic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
	defer writer.Close()

	payload, err := json.Marshal(models.FolderSizeDLQMessage{
		EventID:         eventID,
		Error:           cause.Error(),
		OriginalPayload: string(original.Value),
		FailedAt:        time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal folder-size DLQ message: %w", err)
	}

	if err := writer.WriteMessages(ctx, kafka.Message{
		Key:   original.Key,
		Value: payload,
		Headers: []kafka.Header{
			{Key: "event_id", Value: []byte(eventID)},
			{Key: "failure_reason", Value: []byte(cause.Error())},
		},
	}); err != nil {
		return fmt.Errorf("publish folder-size DLQ message: %w", err)
	}

	return nil
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
			return ctx.Err()
		}

		log.Printf("Kafka offset commit failed: %v", err)

		if !wait(ctx, time.Second) {
			return ctx.Err()
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
