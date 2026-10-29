package utils

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

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
		return fmt.Errorf("load AWS configuration: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	s3Uploader = &S3Uploader{
		Client:     client,
		BucketName: ConfigEnv.AppConfig.BucketName,
		Region:     ConfigEnv.AppConfig.AWSRegion,
	}

	return nil
}

func FetchRawBytesFromS3(
	ctx context.Context,
	key string,
) ([]byte, error) {
	if s3Uploader == nil {
		return nil, fmt.Errorf("S3 uploader is not initialized")
	}

	result, err := s3Uploader.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s3Uploader.BucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch S3 object %q: %w", key, err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object %q: %w", key, err)
	}

	return data, nil
}

func DeleteS3Object(ctx context.Context, key string) error {
	if s3Uploader == nil {
		return fmt.Errorf("S3 uploader is not initialized")
	}

	_, err := s3Uploader.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s3Uploader.BucketName,
		Key:    &key,
	})
	if err != nil {
		return fmt.Errorf("failed to delete S3 object %q: %w", key, err)
	}

	return nil
}

// IsS3ConfigurationError identifies AWS errors that usually require
// correcting bucket configuration or credentials/permissions.
//
// These errors are not sent to the DLQ automatically. The caller should
// log them and use a slower retry schedule so the event is not lost.
func IsS3ConfigurationError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	switch apiErr.ErrorCode() {
	case "NoSuchBucket",
		"AccessDenied",
		"InvalidAccessKeyId",
		"SignatureDoesNotMatch",
		"AuthorizationHeaderMalformed",
		"ExpiredToken",
		"InvalidToken":
		return true
	default:
		return false
	}
}
