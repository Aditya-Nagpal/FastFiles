package events

import "time"

const (
	EventFileCreated   = "FILE_CREATED"
	EventFileDeleted   = "FILE_DELETED"
	EventFolderDeleted = "FOLDER_DELETED"
)

type EntryEvent struct {
	EventID     string    `json:"eventId"`
	EventType   string    `json:"eventType"`
	AggregateID string    `json:"aggregateId"`
	UserID      int64     `json:"userId"`
	EntryID     int64     `json:"entryId"`
	PublicID    string    `json:"publicId"`
	ParentID    *int64    `json:"parentId,omitempty"`
	Name        string    `json:"name,omitempty"`
	EntryType   string    `json:"entryType"`
	Size        int64     `json:"size"`
	S3Key       string    `json:"s3Key,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}
