package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL            string
	OpenAiApiKey           string
	BucketName             string
	AWSAccessKeyId         string
	AWSSecretAccessKey     string
	AWSRegion              string
	KafkaBrokers           string
	KafkaTopic             string
	KafkaS3CleanupDLQTopic string
}

var AppConfig *Config

func LoadConfig() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file", err.Error())
	}

	AppConfig = &Config{
		DatabaseURL:            getEnv("DATABASE_URL"),
		OpenAiApiKey:           getEnv("OPENAI_API_KEY"),
		BucketName:             getEnv("AWS_S3_BUCKET_NAME"),
		AWSAccessKeyId:         getEnv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey:     getEnv("AWS_SECRET_ACCESS_KEY"),
		AWSRegion:              getEnv("AWS_REGION"),
		KafkaBrokers:           getEnv("KAFKA_BROKERS"),
		KafkaTopic:             getEnv("KAFKA_TOPIC"),
		KafkaS3CleanupDLQTopic: getEnv("KAFKA_S3_CLEANUP_DLQ_TOPIC"),
	}
}

func getEnv(key string) string {
	value := os.Getenv(key)
	return value
}
