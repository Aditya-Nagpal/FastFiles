package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func InsertOutboxEvent(
	tx pgx.Tx,
	ctx context.Context,
	eventPayload []byte,
	aggregateType,
	aggregateId,
	eventType string,
) error {
	query := `
		INSERT INTO outbox_events (
			aggregate_type,
			aggregate_id,
			event_type,
			payload
		)
		VALUES ($1, $2, $3, $4::jsonb)
	`
	_, err := tx.Exec(ctx, query, aggregateType, aggregateId, eventType, eventPayload)
	if err != nil {
		return fmt.Errorf("failed to insert outbox event: %w", err)
	}
	return nil
}
