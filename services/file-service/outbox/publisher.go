package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/jackc/pgx/v5"
	"github.com/segmentio/kafka-go"
)

func Run(ctx context.Context, brokers []string, topic string) {
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}
	defer writer.Close()

	log.Println("Outbox publisher started")

	for {
		if ctx.Err() != nil {
			return
		}

		event, err := db.ClaimNextEvent(ctx)

		if errors.Is(err, pgx.ErrNoRows) {
			if !wait(ctx, time.Second) {
				return
			}
			continue
		}

		if err != nil {
			log.Printf("Outbox: failed to claim event: %v", err)

			if !wait(ctx, 2*time.Second) {
				return
			}
			continue
		}

		// Decode according to event type.
		var (
			messagePayload any
			userID         int64
		)

		switch event.EventType {
		case events.EventS3Cleanup:
			var cleanupPayload events.S3CleanupEvent

			if err := json.Unmarshal(event.Payload, &cleanupPayload); err != nil {
				log.Printf(
					"Outbox: failed to decode S3 cleanup event %d: %v",
					event.ID, err,
				)

				if newErr := db.RecordFailure(ctx, event.ID, err); newErr != nil {
					log.Printf("Outbox: retry scheduling failed: %v", newErr)

					if !wait(ctx, 2*time.Second) {
						return
					}
				}
				continue
			}

			cleanupPayload.EventID = event.EventID
			cleanupPayload.EventType = event.EventType

			userID = cleanupPayload.UserID
			messagePayload = cleanupPayload

		default:
			var entryPayload events.EntryEvent

			if err := json.Unmarshal(event.Payload, &entryPayload); err != nil {
				log.Printf(
					"Outbox: failed to decode entry event %d: %v",
					event.ID, err,
				)

				if newErr := db.RecordFailure(ctx, event.ID, err); newErr != nil {
					log.Printf("Outbox: retry scheduling failed: %v", newErr)

					if !wait(ctx, 2*time.Second) {
						return
					}
				}
				continue
			}

			entryPayload.EventID = event.EventID
			entryPayload.EventType = event.EventType

			userID = entryPayload.UserID
			messagePayload = entryPayload
		}

		value, err := json.Marshal(messagePayload)
		if err != nil {
			log.Printf(
				"Outbox: failed to encode event %d: %v",
				event.ID, err,
			)

			if newErr := db.RecordFailure(ctx, event.ID, err); newErr != nil {
				log.Printf("Outbox: retry scheduling failed: %v", newErr)

				if !wait(ctx, 2*time.Second) {
					return
				}
			}
			continue
		}

		message := kafka.Message{
			Key:   fmt.Appendf(nil, "%d", userID),
			Value: value,
			Headers: []kafka.Header{
				{
					Key:   "event_id",
					Value: []byte(event.EventID),
				},
				{
					Key:   "event_type",
					Value: []byte(event.EventType),
				},
			},
		}

		publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = writer.WriteMessages(publishCtx, message)
		cancel()

		if err != nil {
			log.Printf(
				"Outbox: Kafka publish failed for event %d: %v",
				event.ID, err,
			)

			if newErr := db.RecordFailure(ctx, event.ID, err); newErr != nil {
				log.Printf("Outbox: retry scheduling failed: %v", newErr)

				if !wait(ctx, 2*time.Second) {
					return
				}
			}
			continue
		}

		err = db.MarkEventAsPublished(ctx, event.ID)
		if err != nil {
			log.Printf(
				"Outbox: published event %d, but failed to mark it published: %v",
				event.ID, err,
			)
			// It may be published again after its claim expires.
			// Consumers must be idempotent.
			continue
		}

		log.Printf(
			"Outbox: published event_id=%s type=%s",
			event.EventID,
			event.EventType,
		)
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
