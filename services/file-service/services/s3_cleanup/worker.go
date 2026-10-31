package s3_cleanup

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
)

const (
	cleanupPollInterval = 10 * time.Second
	cleanupTimeout      = 30 * time.Second
	dbOperationTimeout  = 10 * time.Second
)

// cleanupDeleteFunc makes S3 deletion injectable for tests.
type cleanupDeleteFunc func(
	ctx context.Context,
	bucketName string,
	key string,
) error

type cleanupWorker struct {
	claimTask  func(context.Context) (*models.S3CleanupTask, error)
	markDone   func(context.Context, int64) error
	markFailed func(context.Context, int64, error) error
	deleteS3   cleanupDeleteFunc
}

func Run(ctx context.Context, uploader *utils.S3Uploader) {
	if uploader == nil {
		log.Println("S3 cleanup worker: uploader is nil; worker will not start")
		return
	}

	worker := cleanupWorker{
		claimTask:  db.ClaimNextS3CleanupTask,
		markDone:   db.MarkS3CleanupCompleted,
		markFailed: db.MarkS3CleanupFailed,
		deleteS3:   uploader.DeleteObjectWithContext,
	}

	runWith(ctx, cleanupPollInterval, worker)
}

func runWith(
	ctx context.Context,
	pollInterval time.Duration,
	worker cleanupWorker,
) {
	if pollInterval <= 0 {
		log.Println("S3 cleanup worker: invalid polling interval")
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	log.Println("S3 cleanup worker started")

	for {
		if ctx.Err() != nil {
			log.Println("S3 cleanup worker stopped")
			return
		}

		// Process all currently available tasks before waiting.
		processed := processAvailableTasks(ctx, worker)

		if ctx.Err() != nil {
			log.Println("S3 cleanup worker stopped")
			return
		}

		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			log.Println("S3 cleanup worker stopped")
			return

		case <-ticker.C:
		}
	}
}

// Returns true if at least one task was claimed.
func processAvailableTasks(
	ctx context.Context,
	worker cleanupWorker,
) bool {
	processed := false

	for ctx.Err() == nil {
		claimCtx, cancel := context.WithTimeout(ctx, dbOperationTimeout)
		task, err := worker.claimTask(claimCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return processed
			}

			log.Printf("S3 cleanup worker: failed to claim task: %v", err)
			return processed
		}

		if task == nil {
			return processed
		}

		processed = true
		processTask(ctx, worker, task)
	}

	return processed
}

func processTask(
	ctx context.Context,
	worker cleanupWorker,
	task *models.S3CleanupTask,
) {
	log.Printf(
		"S3 cleanup worker: processing task_id=%d bucket=%s key=%s attempt=%d/%d",
		task.ID,
		task.BucketName,
		task.S3Key,
		task.Attempts,
		task.MaxAttempts,
	)

	// Bound the duration of an individual S3 delete request.
	deleteCtx, cancelDelete := context.WithTimeout(ctx, cleanupTimeout)
	deleteErr := worker.deleteS3(deleteCtx, task.BucketName, task.S3Key)
	cancelDelete()

	if deleteErr != nil {
		handleTaskFailure(ctx, worker, task.ID, deleteErr)
		return
	}

	// Use a fresh bounded context for DB status update.
	// The worker's parent context may have been cancelled after S3 deletion.
	doneCtx, cancelDone := context.WithTimeout(
		context.Background(),
		dbOperationTimeout,
	)
	defer cancelDone()

	if err := worker.markDone(doneCtx, task.ID); err != nil {
		log.Printf(
			"S3 cleanup worker: object deleted but failed to mark task %d completed: %v",
			task.ID,
			err,
		)
		return
	}

	log.Printf("S3 cleanup worker: task %d completed", task.ID)
}

func handleTaskFailure(
	ctx context.Context,
	worker cleanupWorker,
	taskID int64,
	deleteErr error,
) {
	log.Printf(
		"S3 cleanup worker: deletion failed for task %d: %v",
		taskID,
		deleteErr,
	)

	// Don't attempt another DB operation using a cancelled worker context.
	updateCtx, cancel := context.WithTimeout(
		context.Background(),
		dbOperationTimeout,
	)
	defer cancel()

	if err := worker.markFailed(updateCtx, taskID, deleteErr); err != nil {
		log.Printf(
			"S3 cleanup worker: failed to record failure for task %d: %v",
			taskID,
			fmt.Errorf("original deletion error: %w; status update error: %v", deleteErr, err),
		)
		return
	}

	if ctx.Err() == nil {
		log.Printf("S3 cleanup worker: retry scheduled for task %d", taskID)
	}
}
