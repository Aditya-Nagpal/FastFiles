package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/jackc/pgx/v5"
)

// CreateS3CleanupTask persists an S3 object that needs deletion.
// ON CONFLICT is not used: separate cleanup attempts are separate records.
func CreateS3CleanupTask(
	ctx context.Context,
	bucketName string,
	s3Key string,
) (int64, error) {
	if bucketName == "" {
		return 0, fmt.Errorf("bucket name cannot be empty")
	}
	if s3Key == "" {
		return 0, fmt.Errorf("S3 key cannot be empty")
	}

	query := `
		INSERT INTO s3_cleanup_tasks (
			bucket_name,
			s3_key,
			status,
			next_retry_at
		)
		VALUES ($1, $2, 'PENDING', CURRENT_TIMESTAMP)
		RETURNING id
	`

	var taskID int64
	err := DB.QueryRow(ctx, query, bucketName, s3Key).Scan(&taskID)
	if err != nil {
		return 0, fmt.Errorf("failed to create S3 cleanup task: %w", err)
	}

	return taskID, nil
}

// ClaimNextS3CleanupTask atomically claims one eligible cleanup task.
// PROCESSING tasks can be reclaimed after a worker crash using updated_at.
func ClaimNextS3CleanupTask(ctx context.Context) (*models.S3CleanupTask, error) {
	query := `
		WITH candidate AS (
			SELECT id
			FROM s3_cleanup_tasks
			WHERE (
				status = 'PENDING'
				AND next_retry_at <= CURRENT_TIMESTAMP
			)
			OR (
				status = 'PROCESSING'
				AND updated_at <= CURRENT_TIMESTAMP - INTERVAL '2 minutes'
			)
			ORDER BY next_retry_at, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE s3_cleanup_tasks AS task
		SET status = 'PROCESSING',
			attempts = task.attempts + 1,
			updated_at = CURRENT_TIMESTAMP
		FROM candidate
		WHERE task.id = candidate.id
		RETURNING
			task.id,
			task.bucket_name,
			task.s3_key,
			task.status,
			task.attempts,
			task.max_attempts,
			task.next_retry_at,
			task.last_error
	`

	var task models.S3CleanupTask

	err := DB.QueryRow(ctx, query).Scan(
		&task.ID,
		&task.BucketName,
		&task.S3Key,
		&task.Status,
		&task.Attempts,
		&task.MaxAttempts,
		&task.NextRetryAt,
		&task.LastError,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim S3 cleanup task: %w", err)
	}

	return &task, nil
}

// MarkS3CleanupCompleted marks a successfully deleted object as completed.
func MarkS3CleanupCompleted(ctx context.Context, taskID int64) error {
	query := `
		UPDATE s3_cleanup_tasks
		SET status = 'COMPLETED',
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status = 'PROCESSING'
	`

	tag, err := DB.Exec(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("failed to complete S3 cleanup task %d: %w", taskID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("S3 cleanup task %d not found or not processing", taskID)
	}

	return nil
}

// MarkS3CleanupFailed schedules a retry or permanently fails the task.
func MarkS3CleanupFailed(
	ctx context.Context,
	taskID int64,
	cleanupErr error,
) error {
	if cleanupErr == nil {
		return fmt.Errorf("cleanup error cannot be nil")
	}

	query := `
		UPDATE s3_cleanup_tasks
		SET status = CASE
				WHEN attempts >= max_attempts THEN 'FAILED'
				ELSE 'PENDING'
			END,
			next_retry_at = CURRENT_TIMESTAMP
				+ LEAST(POWER(2, LEAST(attempts - 1, 8)), 300)
				  * INTERVAL '1 second',
			last_error = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status = 'PROCESSING'
	`

	tag, err := DB.Exec(ctx, query, taskID, cleanupErr.Error())
	if err != nil {
		return fmt.Errorf("failed to record S3 cleanup failure for task %d: %w", taskID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("S3 cleanup task %d not found or not processing", taskID)
	}

	return nil
}
