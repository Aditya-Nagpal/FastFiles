package utils

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"

	ConfigEnv "github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/file-service/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Uploader struct {
	Client     *s3.Client
	Presigner  *s3.PresignClient
	BucketName string
	Region     string
}

func NewS3Uploader() (*S3Uploader, error) {
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
		panic("Failed to load AWS config: " + err.Error())
	}

	client := s3.NewFromConfig(cfg)

	presignClient := s3.NewPresignClient(client)

	return &S3Uploader{
		Client:     client,
		Presigner:  presignClient,
		BucketName: ConfigEnv.AppConfig.BucketName,
		Region:     ConfigEnv.AppConfig.AWSRegion,
	}, nil
}

func (u *S3Uploader) UploadFile(file multipart.File, fileHeader *multipart.FileHeader, key string) error {
	_, err := u.Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(u.BucketName),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(fileHeader.Header.Get("Content-Type")),
		ACL:         types.ObjectCannedACLPublicRead, // For public read access
	})
	return err
}

func (u *S3Uploader) DeleteObject(prefix string, isFolder bool) error {
	if isFolder {
		listInput := &s3.ListObjectsV2Input{
			Bucket: aws.String(u.BucketName),
			Prefix: aws.String(prefix),
		}

		output, err := u.Client.ListObjectsV2(context.TODO(), listInput)
		if err != nil {
			return err
		}

		var objects []types.ObjectIdentifier
		for _, obj := range output.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}

		if len(objects) == 0 {
			return fmt.Errorf("no objects found in the folder")
		}

		_, err = u.Client.DeleteObjects(context.TODO(), &s3.DeleteObjectsInput{
			Bucket: aws.String(u.BucketName),
			Delete: &types.Delete{Objects: objects},
		})
		return err
	}
	_, err := u.Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(u.BucketName),
		Key:    aws.String(prefix),
	})
	return err
}

func (u *S3Uploader) GetS3URL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", u.BucketName, u.Region, key)
}

func (u *S3Uploader) GeneratePresignedURL(key string, expiry time.Duration, filename string) (string, error) {
	presignResult, err := u.Presigner.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket:                     aws.String(u.BucketName),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(`attachment; filename="` + filename + `"`),
	}, func(options *s3.PresignOptions) {
		options.Expires = expiry
	})
	if err != nil {
		return "", err
	}

	return presignResult.URL, nil
}
