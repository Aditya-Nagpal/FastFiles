package workers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"
)

func HandleS3CleanupEvent(ctx context.Context, message []byte) error {
	var event events.EntryEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("invalid entry event JSON: %w", err)
	}

	if event.UserID <= 0 || event.EntryID <= 0 {
		return fmt.Errorf(
			"invalid event identity: userId=%d entryId=%d",
			event.UserID,
			event.EntryID,
		)
	}

	var keys []string

	switch event.EventType {
	case events.EventFileDeleted:
		key, err := db.GetSingleFileKey(ctx, event.UserID, event.EntryID)
		if err != nil {
			return fmt.Errorf("failed to fetch deleted file S3 key: %w", err)
		}

		if key != "" {
			keys = append(keys, key)
		}

	case events.EventFolderDeleted:
		descendantKeys, err := db.GetDescendantFileKeys(ctx, event.UserID, event.EntryID)
		if err != nil {
			return fmt.Errorf("failed to fetch descendant file S3 keys: %w", err)
		}
		keys = append(keys, descendantKeys...)

	default:
		// This consumer only handles deletion events.
		return nil
	}

	for _, key := range keys {
		if err := utils.DeleteS3Object(ctx, key); err != nil {
			return fmt.Errorf("S3 cleanup failed for entry %d: %w", event.EntryID, err)
		}
	}

	return nil
}
