package events

import "time"

const EventS3Cleanup = "S3_CLEANUP"

type S3CleanupObject struct {
	BucketName string `json:"bucketName"`
	S3Key      string `json:"s3Key"`
}

type S3CleanupEvent struct {
	EventID   string            `json:"eventId"`
	EventType string            `json:"eventType"`
	UserID    int64             `json:"userId"`
	Objects   []S3CleanupObject `json:"objects"`
	CreatedAt time.Time         `json:"createdAt"`
}
