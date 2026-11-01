package handlers

import (
	"context"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
)

func CleanupUploadedS3Object(uploader *utils.S3Uploader, s3Key string) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	// Persist cleanup intent first.
	taskID, err := db.CreateS3CleanupTask(
		ctx,
		uploader.BucketName,
		s3Key,
	)
	if err != nil {
		log.Printf(
			"CRITICAL: Failed to persist S3 cleanup task: %v",
			err,
		)
		return
	}

	// Claim this specific task before attempting deletion.
	task, err := db.ClaimS3CleanupTaskByID(ctx, taskID)
	if err != nil {
		log.Printf(
			"Failed to claim immediate S3 cleanup task %d: %v",
			taskID,
			err,
		)
		return
	}

	// Another worker may already have claimed it.
	if task == nil {
		log.Printf(
			"S3 cleanup task %d was not available for immediate processing",
			taskID,
		)
		return
	}

	if task.LeaseToken == nil {
		log.Printf(
			"CRITICAL: S3 cleanup task %d has no lease token",
			taskID,
		)
		return
	}

	leaseToken := *task.LeaseToken

	// Attempt immediate S3 deletion.
	deleteCtx, deleteCancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)

	err = uploader.DeleteObjectWithContext(
		deleteCtx,
		task.BucketName,
		task.S3Key,
	)
	deleteCancel()

	if err != nil {
		log.Printf(
			"S3 immediate cleanup failed for task %d: %v",
			taskID,
			err,
		)

		retryCtx, retryCancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)

		if markErr := db.MarkS3CleanupFailed(
			retryCtx,
			taskID,
			leaseToken,
			err,
		); markErr != nil {
			log.Printf(
				"CRITICAL: Failed to schedule retry for task %d: %v",
				taskID,
				markErr,
			)
		}

		retryCancel()
		return
	}

	// Mark completed using the same lease token.
	completeCtx, completeCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer completeCancel()

	if err := db.MarkS3CleanupCompleted(
		completeCtx,
		taskID,
		leaseToken,
	); err != nil {
		log.Printf(
			"S3 object deleted but failed to complete cleanup task %d: %v",
			taskID,
			err,
		)
	}
}
