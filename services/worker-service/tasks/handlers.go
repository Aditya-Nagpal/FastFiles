package tasks

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hibiken/asynq"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/models"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"

	SharedTasks "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/tasks"
)

func HandleGenerateEmbedding(ctx context.Context, t *asynq.Task) error {
	var payload models.Payload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		log.Printf("Failed to unmarshal payload: ", err.Error())
		return err
	}

	s3Key := payload.S3Key

	rawBytes, err := utils.FetchRawBytesFromS3(ctx, s3Key)
	if err != nil {
		log.Printf("Failed to fetch raw bytes from S3: ", err.Error())
		return err
	}

	content, err := utils.ExtractTextFromPDF(rawBytes)
	if err != nil {
		log.Printf("Failed to extract text from PDF: ", err.Error())
		return err
	}

	cleanText := utils.SanitizeUTF8(content)
	chunks := chunkText(cleanText, 4000, 500)

	for i, chunk := range chunks {
		vector, err := SharedTasks.GenerateEmbedding(chunk, config.AppConfig.OpenAiApiKey)
		if err != nil {
			log.Printf("Failed to generate embedding for chunk %d: %v", i, err.Error())
			continue
		}

		err = db.InsertChunkVector(ctx, payload.InternalID, i, chunk, vector)
		if err != nil {
			log.Printf("Failed to insert embedding in postgreSQL for chunk %d: %v", i, err.Error())
			return err
		}
	}
	return nil
}
