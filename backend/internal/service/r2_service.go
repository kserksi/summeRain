// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/kserksi/summerain/internal/repository"
)

type R2Service struct {
	client     *s3.Client
	endpoint   string
	bucket     string
	publicURL  string
	mu         sync.RWMutex
	configured bool
	enabled    bool
	cfgRepo    *repository.SystemConfigRepo
	transport  *http.Transport
}

type r2UploadSnapshot struct {
	client   *s3.Client
	endpoint string
	bucket   string
}

func NewR2Service(cfgRepo *repository.SystemConfigRepo) *R2Service {
	svc := &R2Service{cfgRepo: cfgRepo}
	svc.reload()
	return svc
}

func (s *R2Service) reload() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetLocked()

	if s.cfgRepo == nil {
		return
	}

	enabled, _ := s.cfgRepo.FindByKey("r2_enabled")
	requestedEnabled := enabled != nil && enabled.ConfigValue == "true"

	endpoint, _ := s.cfgRepo.FindByKey("r2_endpoint")
	accessKey, _ := s.cfgRepo.FindByKey("r2_access_key")
	secretKey, _ := s.cfgRepo.FindByKey("r2_secret_key")
	bucket, _ := s.cfgRepo.FindByKey("r2_bucket")
	publicURL, _ := s.cfgRepo.FindByKey("r2_public_url")

	if endpoint == nil || accessKey == nil || secretKey == nil || bucket == nil {
		return
	}
	endpointValue, endpointErr := normalizeR2BaseURL(endpoint.ConfigValue)
	accessKeyValue := strings.TrimSpace(accessKey.ConfigValue)
	secretKeyValue := strings.TrimSpace(secretKey.ConfigValue)
	bucketValue := strings.TrimSpace(bucket.ConfigValue)
	if endpointErr != nil || accessKeyValue == "" || secretKeyValue == "" || bucketValue == "" {
		return
	}

	publicURLValue := endpointValue + "/" + bucketValue
	if publicURL != nil && strings.TrimSpace(publicURL.ConfigValue) != "" {
		var publicURLErr error
		publicURLValue, publicURLErr = normalizeR2BaseURL(publicURL.ConfigValue)
		if publicURLErr != nil {
			return
		}
	}

	httpClient, transport := newR2HTTPClient()
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion("auto"),
		awsconfig.WithHTTPClient(httpClient),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKeyValue, secretKeyValue, "",
		)),
	)
	if err != nil {
		transport.CloseIdleConnections()
		log.Printf("[R2] failed to load config: %v", err)
		return
	}

	s.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpointValue
		o.UsePathStyle = true
	})
	s.transport = transport
	s.endpoint = endpointValue
	s.bucket = bucketValue
	s.publicURL = publicURLValue
	s.configured = true
	s.enabled = requestedEnabled
	if s.enabled {
		log.Printf("[R2] enabled: bucket=%s public=%s", s.bucket, s.publicURL)
	}
}

func (s *R2Service) resetLocked() {
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	s.client = nil
	s.endpoint = ""
	s.bucket = ""
	s.publicURL = ""
	s.configured = false
	s.enabled = false
	s.transport = nil
}

func newR2HTTPClient() (*http.Client, *http.Transport) {
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	transport := baseTransport.Clone()
	transport.MaxConnsPerHost = 16
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.MaxResponseHeaderBytes = 1 << 20
	return &http.Client{Transport: transport}, transport
}

func normalizeR2BaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || value == "" || parsed.Hostname() == "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("R2 URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return value, nil
}

func (s *R2Service) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *R2Service) uploadSnapshot() (*r2UploadSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.enabled || !s.configured {
		return nil, false
	}
	return &r2UploadSnapshot{client: s.client, endpoint: s.endpoint, bucket: s.bucket}, true
}

func (t *r2UploadSnapshot) uploadBytes(ctx context.Context, data []byte, key, contentType string) error {
	reader := bytes.NewReader(data)
	_, err := t.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &t.bucket,
		Key:         &key,
		Body:        reader,
		ContentType: &contentType,
	})
	return err
}

func (t *r2UploadSnapshot) delete(ctx context.Context, key string) error {
	return deleteR2Object(ctx, t.client, t.bucket, key)
}

// CanDelete remains true when delivery is disabled but credentials are kept,
// allowing durable cleanup of objects uploaded before R2 was turned off.
func (s *R2Service) CurrentTarget() (endpoint, bucket string, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpoint, s.bucket, s.configured

}

func (s *R2Service) CanDelete(endpoint, bucket string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configured && s.endpoint == strings.TrimRight(strings.TrimSpace(endpoint), "/") && s.bucket == strings.TrimSpace(bucket)
}

func (s *R2Service) PublicURL(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.enabled {
		return ""
	}
	return s.publicURL + "/" + key
}

func (s *R2Service) PublicURLForTarget(endpoint, bucket, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.configured || s.endpoint != strings.TrimRight(strings.TrimSpace(endpoint), "/") || s.bucket != strings.TrimSpace(bucket) {
		return "", fmt.Errorf("R2 public target is not configured")
	}
	return s.publicURL + "/" + key, nil
}

func (s *R2Service) Upload(localPath, key string) error {
	target, ok := s.uploadSnapshot()
	if !ok {
		return fmt.Errorf("R2 upload is not enabled")
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("R2 upload: open local: %w", err)
	}
	defer file.Close()

	_, err = target.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: &target.bucket,
		Key:    &key,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("R2 upload: put object: %w", err)
	}
	return nil
}

func (s *R2Service) UploadBytes(data []byte, key string, contentType string) error {
	target, ok := s.uploadSnapshot()
	if !ok {
		return fmt.Errorf("R2 upload is not enabled")
	}
	return target.uploadBytes(context.Background(), data, key, contentType)
}

func (s *R2Service) Delete(key string) error {
	endpoint, bucket, ok := s.CurrentTarget()
	if !ok {
		return fmt.Errorf("R2 delete is not configured")
	}
	return s.DeleteContext(context.Background(), endpoint, bucket, key)
}

func (s *R2Service) DeleteContext(ctx context.Context, endpoint, bucket, key string) error {
	s.mu.RLock()
	if !s.configured || s.endpoint != strings.TrimRight(strings.TrimSpace(endpoint), "/") || s.bucket != strings.TrimSpace(bucket) {
		s.mu.RUnlock()
		return fmt.Errorf("R2 delete target is not configured")
	}
	client := s.client
	configuredBucket := s.bucket
	s.mu.RUnlock()

	return deleteR2Object(ctx, client, configuredBucket, key)
}

func deleteR2Object(ctx context.Context, client *s3.Client, bucket, key string) error {
	_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: &key})
	if err == nil {
		return nil
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusNotFound {
		return nil
	}
	return err
}

func (s *R2Service) Download(key string) (io.ReadCloser, error) {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil, fmt.Errorf("R2 not enabled")
	}
	client := s.client
	bucket := s.bucket
	s.mu.RUnlock()

	resp, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *R2Service) DownloadForTarget(ctx context.Context, endpoint, bucket, key string) (io.ReadCloser, error) {
	s.mu.RLock()
	if !s.configured || s.endpoint != strings.TrimRight(strings.TrimSpace(endpoint), "/") || s.bucket != strings.TrimSpace(bucket) {
		s.mu.RUnlock()
		return nil, fmt.Errorf("R2 download target is not configured")
	}
	client := s.client
	configuredBucket := s.bucket
	s.mu.RUnlock()

	resp, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: &configuredBucket, Key: &key})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *R2Service) Exists(key string) bool {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return false
	}
	client := s.client
	bucket := s.bucket
	s.mu.RUnlock()

	_, err := client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	return err == nil
}

func TestR2Connection(endpoint, accessKey, secretKey, bucket string) error {
	endpoint, err := normalizeR2BaseURL(endpoint)
	if err != nil {
		return fmt.Errorf("R2 endpoint invalid: %w", err)
	}
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)
	bucket = strings.TrimSpace(bucket)
	if accessKey == "" || secretKey == "" || bucket == "" {
		return fmt.Errorf("R2 credentials and bucket must not be empty")
	}

	httpClient, transport := newR2HTTPClient()
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithHTTPClient(httpClient),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true
	})

	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &bucket,
	})
	if err != nil {
		return fmt.Errorf("连接测试失败: %w", err)
	}

	return nil
}
