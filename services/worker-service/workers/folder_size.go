package workers

import (
	"context"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"
)

func HandleFolderSizeEvent(ctx context.Context, message []byte) error {
	decoded, err := events.DecodeEvent(message)
	if err != nil {
		return &PermanentError{
			Err: fmt.Errorf("invalid folder-size event: %w", err),
		}
	}

	event, ok := decoded.(events.EntryEvent)
	if !ok {
		return &PermanentError{
			Err: fmt.Errorf("unexpected event schema for folder-size worker"),
		}
	}

	switch event.EventType {
	case events.EventFileCreated,
		events.EventFileTrashed,
		events.EventFolderTrashed,
		events.EventFileRestored,
		events.EventFolderRestored:
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
