package workers

import (
	"context"
	"fmt"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/events"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"
)

type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

func HandleS3CleanupEvent(ctx context.Context, message []byte) error {
	return handleS3CleanupEvent(ctx, message, utils.DeleteS3Object)
}

type deleteS3ObjectFunc func(context.Context, string) error

func handleS3CleanupEvent(
	ctx context.Context,
	message []byte,
	deleteObject deleteS3ObjectFunc,
) error {
	decoded, err := events.DecodeEvent(message)
	if err != nil {
		return &PermanentError{
			Err: fmt.Errorf("invalid S3 cleanup event: %w", err),
		}
	}

	event, ok := decoded.(events.S3CleanupEvent)
	if !ok {
		return &PermanentError{
			Err: fmt.Errorf("unexpected event schema for S3 cleanup worker"),
		}
	}

	if event.UserID <= 0 {
		return &PermanentError{
			Err: fmt.Errorf("invalid S3 cleanup event userId: %d", event.UserID),
		}
	}

	if len(event.Objects) == 0 {
		return &PermanentError{
			Err: fmt.Errorf("S3 cleanup event contains no objects"),
		}
	}

	for _, object := range event.Objects {
		if object.BucketName == "" || object.S3Key == "" {
			return &PermanentError{
				Err: fmt.Errorf(
					"invalid S3 cleanup object: bucketName=%q s3Key=%q",
					object.BucketName,
					object.S3Key,
				),
			}
		}

		if err := deleteObject(ctx, object.S3Key); err != nil {
			return fmt.Errorf(
				"S3 cleanup failed for key %s: %w",
				object.S3Key,
				err,
			)
		}
	}

	return nil
}
