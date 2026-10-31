package handlers

import (
	"context"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
)

func CleanupUploadedS3Object(
	uploader *utils.S3Uploader,
	s3Key string,
) {
	// Persist cleanup intent before attempting S3 deletion.
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	taskID, err := db.CreateS3CleanupTask(
		ctx,
		uploader.BucketName,
		s3Key,
	)
	if err != nil {
		log.Printf(
			"CRITICAL: Failed to persist S3 cleanup task for key %s: %v",
			s3Key,
			err,
		)
		return
	}

	// Attempt immediate cleanup.
	deleteCtx, deleteCancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer deleteCancel()

	err = uploader.DeleteObjectWithContext(
		deleteCtx,
		uploader.BucketName,
		s3Key,
	)

	if err != nil {
		log.Printf(
			"S3 immediate cleanup failed for key %s, task %d will be retried: %v",
			s3Key,
			taskID,
			err,
		)

		// Schedule retry using the existing retry mechanism.
		retryCtx, retryCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer retryCancel()

		if markErr := db.MarkS3CleanupFailed(
			retryCtx,
			taskID,
			err,
		); markErr != nil {
			log.Printf(
				"CRITICAL: Failed to schedule retry for S3 cleanup task %d: %v",
				taskID,
				markErr,
			)
		}

		return
	}

	// S3 deletion succeeded. Mark the durable task completed.
	completeCtx, completeCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer completeCancel()

	if err := db.MarkS3CleanupCompleted(completeCtx, taskID); err != nil {
		log.Printf(
			"S3 object %s deleted, but failed to mark cleanup task %d completed: %v",
			s3Key,
			taskID,
			err,
		)
	}
}
