package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

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

func ClaimNextS3CleanupTask(ctx context.Context) (*models.S3CleanupTask, error) {
	query := `
		WITH exhausted AS (
			UPDATE s3_cleanup_tasks
			SET status = 'FAILED',
				lease_token = NULL,
				lease_expires_at = NULL,
				last_error = COALESCE(
					last_error,
					'Maximum cleanup attempts exhausted'
				),
				updated_at = CURRENT_TIMESTAMP
			WHERE attempts >= max_attempts
			  AND (
				status = 'PENDING'
				OR (
					status = 'PROCESSING'
					AND (lease_expires_at IS NULL OR lease_expires_at <= CURRENT_TIMESTAMP)
				)
			  )
			RETURNING id
		),
		candidate AS (
			SELECT id
			FROM s3_cleanup_tasks
			WHERE attempts < max_attempts
			  AND (
				(
					status = 'PENDING'
					AND next_retry_at <= CURRENT_TIMESTAMP
				)
				OR (
					status = 'PROCESSING'
					AND (lease_expires_at IS NULL OR lease_expires_at <= CURRENT_TIMESTAMP)
				)
			  )
			ORDER BY next_retry_at, id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE s3_cleanup_tasks AS task
		SET status = 'PROCESSING',
			attempts = task.attempts + 1,
			lease_token = gen_random_uuid(),
			lease_expires_at = CURRENT_TIMESTAMP + INTERVAL '5 minutes',
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
			task.last_error,
			task.lease_token,
			task.lease_expires_at
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
		&task.LeaseToken,
		&task.LeaseExpiresAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim S3 cleanup task: %w", err)
	}

	return &task, nil
}

func ClaimS3CleanupTaskByID(ctx context.Context, taskID int64) (*models.S3CleanupTask, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid S3 cleanup task ID")
	}

	query := `
		UPDATE s3_cleanup_tasks
		SET status = 'PROCESSING',
			attempts = attempts + 1,
			lease_token = gen_random_uuid(),
			lease_expires_at = CURRENT_TIMESTAMP + INTERVAL '5 minutes',
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status = 'PENDING'
		  AND next_retry_at <= CURRENT_TIMESTAMP
		  AND attempts < max_attempts
		RETURNING
			id,
			bucket_name,
			s3_key,
			status,
			attempts,
			max_attempts,
			next_retry_at,
			last_error,
			lease_token,
			lease_expires_at
	`

	var task models.S3CleanupTask

	err := DB.QueryRow(ctx, query, taskID).Scan(
		&task.ID,
		&task.BucketName,
		&task.S3Key,
		&task.Status,
		&task.Attempts,
		&task.MaxAttempts,
		&task.NextRetryAt,
		&task.LastError,
		&task.LeaseToken,
		&task.LeaseExpiresAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		// Another worker may already have claimed this task.
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf(
			"failed to claim S3 cleanup task %d: %w",
			taskID,
			err,
		)
	}

	return &task, nil
}

func MarkS3CleanupCompleted(ctx context.Context, taskID int64, leaseToken uuid.UUID) error {
	query := `
		UPDATE s3_cleanup_tasks
		SET status = 'COMPLETED',
			last_error = NULL,
			lease_token = NULL,
			lease_expires_at = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status = 'PROCESSING'
		  AND lease_token = $2
		  AND lease_expires_at > CURRENT_TIMESTAMP
	`

	tag, err := DB.Exec(ctx, query, taskID, leaseToken)
	if err != nil {
		return fmt.Errorf(
			"failed to complete S3 cleanup task %d: %w",
			taskID,
			err,
		)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"S3 cleanup task %d: lease ownership lost or expired",
			taskID,
		)
	}

	return nil
}

func MarkS3CleanupFailed(ctx context.Context, taskID int64, leaseToken uuid.UUID, cleanupErr error) error {
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
				+ LEAST(
					POWER(2, GREATEST(attempts, 1) - 1),
					300
				) * INTERVAL '1 second',
			last_error = $3,
			lease_token = NULL,
			lease_expires_at = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		  AND status = 'PROCESSING'
		  AND lease_token = $2
		  AND lease_expires_at > CURRENT_TIMESTAMP
	`

	tag, err := DB.Exec(ctx, query, taskID, leaseToken, cleanupErr.Error())
	if err != nil {
		return fmt.Errorf(
			"failed to record S3 cleanup failure for task %d: %w",
			taskID,
			err,
		)
	}

	if tag.RowsAffected() != 1 {
		return fmt.Errorf(
			"S3 cleanup task %d: lease ownership lost or expired",
			taskID,
		)
	}

	return nil
}
