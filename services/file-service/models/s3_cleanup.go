package models

import "time"

type S3CleanupTask struct {
	ID          int64
	BucketName  string
	S3Key       string
	Status      string
	Attempts    int
	MaxAttempts int
	NextRetryAt time.Time
	LastError   *string
}
