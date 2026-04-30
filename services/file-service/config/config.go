package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	RedisPort          string
	DatabaseURL        string
	BucketName         string
	AWSAccessKeyId     string
	AWSSecretAccessKey string
	AWSRegion          string
	OpenAiApiKey       string
}

var AppConfig *Config

func LoadConfig() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	AppConfig = &Config{
		Port:               getEnv("PORT"),
		RedisPort:          getEnv("REDIS_PORT"),
		DatabaseURL:        getEnv("DATABASE_URL"),
		BucketName:         getEnv("AWS_S3_BUCKET_NAME"),
		AWSAccessKeyId:     getEnv("AWS_ACCESS_KEY_ID"),
		AWSSecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY"),
		AWSRegion:          getEnv("AWS_REGION"),
		OpenAiApiKey:       getEnv("OPENAI_API_KEY"),
	}
}

func getEnv(key string) string {
	value := os.Getenv(key)
	return value
}
