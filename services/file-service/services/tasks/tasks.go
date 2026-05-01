package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/tasks"
	"github.com/hibiken/asynq"
)

type TaskService struct {
	Client *asynq.Client
}

func NewTaskService(redisAddr string) *TaskService {
	return &TaskService{
		Client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
	}
}

func (s *TaskService) EnqueueGenerateEmbeddingTask(internalID int64, s3Key string) error {
	payload, err := json.Marshal(map[string]interface{}{
		"internal_id": internalID,
		"s3_key":      s3Key,
	})
	if err != nil {
		return fmt.Errorf("Failed to marshal payload: %v", err)
	}

	task := asynq.NewTask(tasks.TypeGenerateEmbedding, payload)

	_, err = s.Client.Enqueue(task, asynq.MaxRetry(0), asynq.Timeout(0), asynq.Retention(2*time.Hour))
	return err
}
