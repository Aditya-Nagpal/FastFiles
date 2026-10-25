package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	SharedTasks "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/shared/tasks"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/consumers"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/tasks"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/utils"
	"github.com/hibiken/asynq"
)

func main() {
	config.LoadConfig()
	db.ConnectDatabase()

	if err := utils.NewS3Uploader(); err != nil {
		log.Fatalf("Failed to initialize S3 uploader: %v", err)
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	brokersEnv := config.AppConfig.KafkaBrokers
	if brokersEnv == "" {
		brokersEnv = "localhost:9092"
	}

	brokers := strings.Split(brokersEnv, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	topic := config.AppConfig.KafkaTopic
	if topic == "" {
		topic = "fastfiles.entry-events"
	}

	folderSizeDone := make(chan struct{})
	go func() {
		defer close(folderSizeDone)

		if err := consumers.RunFolderSizeConsumer(ctx, brokers, topic); err != nil {
			log.Printf("Folder-size consumer stopped: %v", err)
			cancel()
		}
	}()

	s3CleanupDone := make(chan struct{})

	go func() {
		defer close(s3CleanupDone)

		if err := consumers.RunS3CleanupConsumer(ctx, brokers, topic); err != nil {
			log.Printf("S3 cleanup consumer stopped: %v", err)
			cancel()
		}
	}()

	srv := asynq.NewServer(
		asynq.RedisClientOpt{Addr: "localhost:6379"},
		asynq.Config{Concurrency: 10},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(
		SharedTasks.TypeGenerateEmbedding,
		tasks.HandleGenerateEmbedding,
	)

	asynqDone := make(chan error, 1)
	go func() {
		asynqDone <- srv.Run(mux)
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down worker service...")
	case err := <-asynqDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Asynq server stopped: %v", err)
		}
		cancel()
	}

	srv.Shutdown()
	cancel()
	<-folderSizeDone
	<-s3CleanupDone
	log.Println("Worker service stopped")
}
