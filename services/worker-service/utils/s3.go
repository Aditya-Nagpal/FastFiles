package utils

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	ConfigEnv "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/worker-service/config"
)

type S3Uploader struct {
	Client     *s3.Client
	BucketName string
	Region     string
}

var s3Uploader *S3Uploader

func NewS3Uploader() error {
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(ConfigEnv.AppConfig.AWSRegion),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				ConfigEnv.AppConfig.AWSAccessKeyId,
				ConfigEnv.AppConfig.AWSSecretAccessKey,
				"",
			),
		),
	)

	if err != nil {
		return err
	}

	client := s3.NewFromConfig(cfg)

	s3Uploader = &S3Uploader{
		Client:     client,
		BucketName: ConfigEnv.AppConfig.BucketName,
		Region:     ConfigEnv.AppConfig.AWSRegion,
	}
	return nil
}

func FetchRawBytesFromS3(ctx context.Context, key string) ([]byte, error) {
	result, err := s3Uploader.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s3Uploader.BucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}
