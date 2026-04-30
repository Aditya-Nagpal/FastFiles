package tasks

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hibiken/asynq"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"

	SharedTasks "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/tasks"

	"testing"
)

func HandleGenerateEmbedding(ctx context.Context, t *asynq.Task) error {
	var payload models.Payload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		log.Printf("Failed to unmarshal payload: ", err.Error())
		return err
	}

	s3Key := payload.S3Key

	content, err := utils.FetchContentFromS3(ctx, s3Key)
	if err != nil {
		log.Printf("Failed to fetch content from S3: ", err.Error())
		return err
	}

	// vector, err := GenerateEmbedding(content, config.AppConfig.OpenAiApiKey)
	// if err != nil {
	// 	return err
	// }

	var tt *testing.T
	vector, err := SharedTasks.TestGenerateEmbedding(content, tt)
	if err != nil {
		log.Printf("Failed to generate embedding: ", err.Error())
		return err
	}

	err = db.UpdateEmbedding(ctx, payload.InternalID, vector)
	if err != nil {
		log.Printf("Failed to update embedding in postgreSQL: ", err.Error())
		return err
	}
	return nil
}
