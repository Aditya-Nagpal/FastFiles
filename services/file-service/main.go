package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/db"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/outbox"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/routes"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/services/storage_recovery"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/services/tasks"
	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/utils"
	"github.com/gin-gonic/gin"
)

func main() {
	config.LoadConfig()
	db.ConnectDatabase()

	// Graceful shutdown on Ctrl+C or termination.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Kafka configuration.
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

	// Start the outbox publisher in the background.
	go outbox.Run(ctx, brokers, topic)

	// Start storage reservation recovery worker.
	recoveryDone := make(chan struct{})

	go func() {
		defer close(recoveryDone)
		storage_recovery.Run(ctx)
	}()

	// Initialize S3 uploader.
	s3Uploader, err := utils.NewS3Uploader()
	if err != nil {
		log.Fatalf("Failed to initialize S3 uploader: %v", err)
	}

	// Initialize task service.
	taskService := tasks.NewTaskService("localhost" + config.AppConfig.RedisPort)

	// Initialize Gin router.
	r := gin.Default()
	routes.SetupRoutes(r, s3Uploader, taskService)

	server := &http.Server{
		Addr:    config.AppConfig.Port,
		Handler: r,
	}

	// Start HTTP server in the background.
	go func() {
		log.Printf("File service listening on %s", server.Addr)

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server failed: %v", err)
			stop()
		}
	}()

	// Wait for shutdown signal.
	<-ctx.Done()
	log.Println("Shutting down file service...")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	// Stop accepting new requests and wait for active requests.
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Wait for recovery worker to stop.
	select {
	case <-recoveryDone:
		log.Println("Storage recovery worker shut down successfully")

	case <-shutdownCtx.Done():
		log.Println("Timed out waiting for storage recovery worker")
	}
}
