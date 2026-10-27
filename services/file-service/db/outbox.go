package db

import (
	"context"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/jackc/pgx/v5"
)

func InsertOutboxEvent(
	tx pgx.Tx,
	ctx context.Context,
	eventID string,
	eventPayload []byte,
	aggregateType string,
	aggregateId string,
	eventType string,
) error {
	query := `
		INSERT INTO outbox_events (
			event_id,
			aggregate_type,
			aggregate_id,
			event_type,
			payload
		)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`
	_, err := tx.Exec(ctx, query, eventID, aggregateType, aggregateId, eventType, eventPayload)
	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}
	return nil
}

func ClaimNextEvent(ctx context.Context) (*models.PendingEvent, error) {
	query := `
		WITH candidate AS (
			SELECT id
			FROM outbox_events
			WHERE status = 'PENDING'
			  AND next_attempt_at <= CURRENT_TIMESTAMP
			  AND (
				  locked_until IS NULL
				  OR locked_until < CURRENT_TIMESTAMP
			  )
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events AS o
		SET locked_until = CURRENT_TIMESTAMP + INTERVAL '30 seconds'
		FROM candidate AS c
		WHERE o.id = c.id
		RETURNING o.id, o.event_id, o.aggregate_id, o.payload
	`

	var event models.PendingEvent

	err := DB.QueryRow(ctx, query).Scan(
		&event.ID,
		&event.EventID,
		&event.AggregateID,
		&event.Payload,
	)
	if err != nil {
		return nil, err
	}

	return &event, nil
}

func MarkEventAsPublished(ctx context.Context, eventID int64) error {
	query := `
		UPDATE outbox_events
		SET status = 'PUBLISHED',
			published_at = CURRENT_TIMESTAMP,
			locked_until = NULL,
			last_error = NULL
		WHERE id = $1
	`
	_, err := DB.Exec(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as published: %w", err)
	}
	return nil
}

func RecordFailure(ctx context.Context, eventID int64, publishErr error) error {
	query := `
		UPDATE outbox_events
		SET attempts = attempts + 1,
		     next_attempt_at = CURRENT_TIMESTAMP
		         + LEAST(POWER(2, LEAST(attempts, 8)), 300)
		           * INTERVAL '1 second',
		     locked_until = NULL,
		     last_error = $2
		WHERE id = $1 AND status = 'PENDING'
	`
	tag, err := DB.Exec(ctx, query, eventID, publishErr.Error())
	if err != nil {
		return fmt.Errorf("failed to record retry for event %d: %w", eventID, err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("could not record retry for event %d: event not pending or missing", eventID)
	}

	return nil
}
