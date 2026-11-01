package models

import (
	"time"

	"github.com/google/uuid"
)

type S3CleanupTask struct {
	ID             int64
	BucketName     string
	S3Key          string
	Status         string
	Attempts       int
	MaxAttempts    int
	NextRetryAt    time.Time
	LastError      *string
	LeaseToken     *uuid.UUID
	LeaseExpiresAt *time.Time
}
