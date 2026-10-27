package workers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"
)

func HandleFolderSizeEvent(ctx context.Context, message []byte) error {
	var event events.EntryEvent

	if err := json.Unmarshal(message, &event); err != nil {
		return &PermanentError{
			Err: fmt.Errorf("invalid entry event JSON: %w", err),
		}
	}

	switch event.EventType {
	case events.EventFileCreated,
		events.EventFileDeleted,
		events.EventFolderDeleted:
		// These events can change ancestor folder sizes.
	default:
		return nil
	}

	if event.UserID <= 0 || event.EntryID <= 0 {
		return &PermanentError{
			Err: fmt.Errorf(
				"invalid event identity: userId=%d entryId=%d",
				event.UserID,
				event.EntryID,
			),
		}
	}

	if err := db.RecalculateAncestorFolderSizes(
		ctx,
		event.UserID,
		event.EntryID,
	); err != nil {
		return fmt.Errorf(
			"failed to recalculate ancestor folder sizes: %w",
			err,
		)
	}

	return nil
}
