package models

import (
	"time"
)

type Payload struct {
	InternalID int64  `json:"internal_id"`
	S3Key      string `json:"s3_key"`
}

type S3CleanupDLQMessage struct {
	EventID         string    `json:"event_id"`
	Error           string    `json:"error"`
	OriginalPayload string    `json:"original_payload"`
	FailedAt        time.Time `json:"failed_at"`
}

type FolderSizeDLQMessage struct {
	EventID         string    `json:"event_id"`
	Error           string    `json:"error"`
	OriginalPayload string    `json:"original_payload"`
	FailedAt        time.Time `json:"failed_at"`
}
