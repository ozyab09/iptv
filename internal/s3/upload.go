package s3

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/ozyab/iptv/internal/utils"
)

var logger = utils.NewSanitizedLoggerWithPrefix("[s3]")

func newS3Client(s3Endpoint, region string) (*s3.Client, error) {
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	awsCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &s3Endpoint
		o.UsePathStyle = true
	})

	return client, nil
}

func UploadToS3(content, bucket, key, s3Endpoint, region, contentType string) error {
	if contentType == "" {
		contentType = "application/x-mpegurl"
	}

	logger.Info("Uploading to S3-compatible storage: s3://%s/%s", bucket, key)

	if s3Endpoint == "" || (!strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://")) {
		return fmt.Errorf("invalid S3 endpoint URL: %s. Must be a valid HTTP/HTTPS URL", s3Endpoint)
	}

	client, err := newS3Client(s3Endpoint, region)
	if err != nil {
		return err
	}

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        strings.NewReader(content),
		ContentType: &contentType,
		Metadata: map[string]string{
			"uploaded-by":      "m3u-simple-filter-script",
			"upload-timestamp": fmt.Sprintf("%d", time.Now().Unix()),
		},
	})
	if err != nil {
		logger.Error("Error uploading to S3-compatible storage: %v", err)
		return err
	}

	logger.Info("Upload to S3-compatible storage completed successfully")
	return nil
}

func UploadFileToS3(filePath, bucket, key, outputDir, s3Endpoint, region, contentType string) error {
	if contentType == "" {
		contentType = "application/x-mpegurl"
	}

	fullPath := filePath
	if outputDir != "" {
		altPath := path.Join(outputDir, path.Base(filePath))
		if _, err := os.Stat(altPath); err == nil {
			fullPath = altPath
		}
	}

	logger.Info("Uploading file to S3-compatible storage: s3://%s/%s from %s", bucket, key, fullPath)

	if s3Endpoint == "" || (!strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://")) {
		return fmt.Errorf("invalid S3 endpoint URL: %s. Must be a valid HTTP/HTTPS URL", s3Endpoint)
	}

	client, err := newS3Client(s3Endpoint, region)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", fullPath, err)
	}

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &contentType,
		Metadata: map[string]string{
			"uploaded-by":      "m3u-simple-filter-script",
			"upload-timestamp": fmt.Sprintf("%d", time.Now().Unix()),
			"source-file":      fullPath,
		},
	})
	if err != nil {
		logger.Error("Error uploading file to S3-compatible storage: %v", err)
		return err
	}

	logger.Info("File upload to S3-compatible storage completed successfully")
	return nil
}

func UploadArchiveToS3(content, bucket, baseKey, s3Endpoint, region string) (string, error) {
	now := time.Now().UTC()
	dateStr := now.Format("2006-01-02")
	timeStr := now.Format("15-04-05")
	uniqueID := uuid.New().String()[:8]

	baseName := strings.TrimSuffix(path.Base(baseKey), path.Ext(baseKey))
	archiveKey := fmt.Sprintf("archive/%s/%s-%s_%s.gz", dateStr, timeStr, uniqueID, baseName)

	logger.Info("Uploading archive to S3: s3://%s/%s", bucket, archiveKey)

	if s3Endpoint == "" || (!strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://")) {
		return "", fmt.Errorf("invalid S3 endpoint URL: %s. Must be a valid HTTP/HTTPS URL", s3Endpoint)
	}

	client, err := newS3Client(s3Endpoint, region)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(content)); err != nil {
		return "", fmt.Errorf("failed to compress content: %w", err)
	}
	gw.Close()

	originalSizeKB := float64(len(content)) / 1024
	compressedSizeKB := float64(buf.Len()) / 1024

	contentType := "application/gzip"
	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &archiveKey,
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: &contentType,
		Metadata: map[string]string{
			"uploaded-by":        "m3u-simple-filter-script",
			"upload-timestamp":   fmt.Sprintf("%d", time.Now().Unix()),
			"original-size-kb":   fmt.Sprintf("%.2f", originalSizeKB),
			"compressed-size-kb": fmt.Sprintf("%.2f", compressedSizeKB),
		},
	})
	if err != nil {
		logger.Error("Error uploading archive to S3: %v", err)
		return "", err
	}

	logger.Info("Archive upload completed: %s (%.2f KB, original: %.2f KB)", archiveKey, compressedSizeKB, originalSizeKB)
	return archiveKey, nil
}
