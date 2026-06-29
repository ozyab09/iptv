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

// Package-level logger with sanitized output.
var logger = utils.NewSanitizedLoggerWithPrefix("[s3]")

// metadata is shared across all S3 uploads for consistent tracking.
var defaultMetadata = map[string]string{
	"uploaded-by": "iptv-m3u-filter",
}

// buildS3Metadata creates a metadata map with a timestamp.
func buildS3Metadata(extra ...map[string]string) map[string]string {
	m := make(map[string]string, len(defaultMetadata)+1+len(extra))
	for k, v := range defaultMetadata {
		m[k] = v
	}
	m["upload-timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	for _, e := range extra {
		for k, v := range e {
			m[k] = v
		}
	}
	return m
}

// newS3Client creates an S3 client configured for the given endpoint and region.
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

	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &s3Endpoint
		o.UsePathStyle = true
	}), nil
}

// validateEndpoint checks that the S3 endpoint URL is a valid HTTP/HTTPS URL.
func validateEndpoint(s3Endpoint string) error {
	if s3Endpoint == "" || (!strings.HasPrefix(s3Endpoint, "http://") && !strings.HasPrefix(s3Endpoint, "https://")) {
		return fmt.Errorf("invalid S3 endpoint URL: %s. Must be a valid HTTP/HTTPS URL", s3Endpoint)
	}
	return nil
}

// getContentType returns the provided contentType or defaults to "application/x-mpegurl".
func getContentType(contentType string) string {
	if contentType == "" {
		return "application/x-mpegurl"
	}
	return contentType
}

// UploadToS3 uploads a string as an S3 object.
func UploadToS3(content, bucket, key, s3Endpoint, region, contentType string) error {
	if err := validateEndpoint(s3Endpoint); err != nil {
		return err
	}
	logger.Info("Uploading to S3-compatible storage: s3://%s/%s", bucket, key)

	client, err := newS3Client(s3Endpoint, region)
	if err != nil {
		return err
	}

	ct := getContentType(contentType)
	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        strings.NewReader(content),
		ContentType: &ct,
		Metadata:    buildS3Metadata(),
	})
	if err != nil {
		logger.Error("Error uploading to S3-compatible storage: %v", err)
		return err
	}

	logger.Info("Upload to S3-compatible storage completed successfully")
	return nil
}

// UploadFileToS3 uploads a local file to S3.
func UploadFileToS3(filePath, bucket, key, outputDir, s3Endpoint, region, contentType string) error {
	if err := validateEndpoint(s3Endpoint); err != nil {
		return err
	}

	// If file not found at filePath, try outputDir + filename.
	fullPath := filePath
	if outputDir != "" {
		altPath := path.Join(outputDir, path.Base(filePath))
		if _, err := os.Stat(altPath); err == nil {
			fullPath = altPath
		}
	}

	logger.Info("Uploading file to S3-compatible storage: s3://%s/%s from %s", bucket, key, fullPath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", fullPath, err)
	}

	client, err := newS3Client(s3Endpoint, region)
	if err != nil {
		return err
	}

	ct := getContentType(contentType)
	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: &ct,
		Metadata:    buildS3Metadata(map[string]string{"source-file": fullPath}),
	})
	if err != nil {
		logger.Error("Error uploading file to S3-compatible storage: %v", err)
		return err
	}

	logger.Info("File upload to S3-compatible storage completed successfully")
	return nil
}

// UploadArchiveToS3 gzip-compresses content and uploads it to S3 under archive/YYYY-MM-DD/.
func UploadArchiveToS3(content, bucket, baseKey, s3Endpoint, region string) (string, error) {
	if err := validateEndpoint(s3Endpoint); err != nil {
		return "", err
	}

	now := time.Now().UTC()
	dateStr := now.Format("2006-01-02")
	timeStr := now.Format("15-04-05")
	uniqueID := uuid.New().String()[:8]

	baseName := strings.TrimSuffix(path.Base(baseKey), path.Ext(baseKey))
	archiveKey := fmt.Sprintf("archive/%s/%s-%s_%s.gz", dateStr, timeStr, uniqueID, baseName)

	logger.Info("Uploading archive to S3: s3://%s/%s", bucket, archiveKey)

	// Gzip compress the content.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(content)); err != nil {
		return "", fmt.Errorf("failed to compress content: %w", err)
	}
	gw.Close()

	client, err := newS3Client(s3Endpoint, region)
	if err != nil {
		return "", err
	}

	originalSizeKB := float64(len(content)) / 1024
	compressedSizeKB := float64(buf.Len()) / 1024
	contentType := "application/gzip"

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &archiveKey,
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: &contentType,
		Metadata: buildS3Metadata(map[string]string{
			"original-size-kb":   fmt.Sprintf("%.2f", originalSizeKB),
			"compressed-size-kb": fmt.Sprintf("%.2f", compressedSizeKB),
		}),
	})
	if err != nil {
		logger.Error("Error uploading archive to S3: %v", err)
		return "", err
	}

	logger.Info("Archive upload completed: %s (%.2f KB, original: %.2f KB)", archiveKey, compressedSizeKB, originalSizeKB)
	return archiveKey, nil
}
