// Copyright 2026 kserks
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/summerain/image-gallery/internal/repository"
)

type R2Service struct {
	client    *s3.Client
	bucket    string
	publicURL string
	mu        sync.RWMutex
	enabled   bool
	cfgRepo   *repository.SystemConfigRepo
}

func NewR2Service(cfgRepo *repository.SystemConfigRepo) *R2Service {
	svc := &R2Service{cfgRepo: cfgRepo}
	svc.reload()
	return svc
}

func (s *R2Service) reload() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfgRepo == nil {
		s.enabled = false
		return
	}

	enabled, _ := s.cfgRepo.FindByKey("r2_enabled")
	if enabled == nil || enabled.ConfigValue != "true" {
		s.enabled = false
		return
	}

	endpoint, _ := s.cfgRepo.FindByKey("r2_endpoint")
	accessKey, _ := s.cfgRepo.FindByKey("r2_access_key")
	secretKey, _ := s.cfgRepo.FindByKey("r2_secret_key")
	bucket, _ := s.cfgRepo.FindByKey("r2_bucket")
	publicURL, _ := s.cfgRepo.FindByKey("r2_public_url")

	if endpoint == nil || accessKey == nil || secretKey == nil || bucket == nil {
		s.enabled = false
		return
	}

	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			accessKey.ConfigValue, secretKey.ConfigValue, "",
		)),
	)
	if err != nil {
		log.Printf("[R2] failed to load config: %v", err)
		s.enabled = false
		return
	}

	s.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint.ConfigValue
		o.UsePathStyle = true
	})
	s.bucket = bucket.ConfigValue
	if publicURL != nil {
		s.publicURL = strings.TrimSuffix(publicURL.ConfigValue, "/")
	} else {
		s.publicURL = endpoint.ConfigValue + "/" + bucket.ConfigValue
	}
	s.enabled = true
	log.Printf("[R2] enabled: bucket=%s public=%s", s.bucket, s.publicURL)
}

func (s *R2Service) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *R2Service) PublicURL(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.enabled {
		return ""
	}
	return s.publicURL + "/" + key
}

func (s *R2Service) Upload(localPath, key string) error {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil
	}
	client := s.client
	bucket := s.bucket
	s.mu.RUnlock()

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("R2 upload: open local: %w", err)
	}
	defer file.Close()

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("R2 upload: put object: %w", err)
	}
	return nil
}

func (s *R2Service) UploadBytes(data []byte, key string, contentType string) error {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil
	}
	client := s.client
	bucket := s.bucket
	s.mu.RUnlock()

	reader := strings.NewReader(string(data))
	_, err := client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        reader,
		ContentType: &contentType,
	})
	return err
}

func (s *R2Service) Delete(key string) error {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil
	}
	client := s.client
	bucket := s.bucket
	s.mu.RUnlock()

	_, err := client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	return err
}

func (s *R2Service) MigrateLocalDir(basePath, subdir string) (int, error) {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return 0, fmt.Errorf("R2 not enabled")
	}
	s.mu.RUnlock()

	dir := filepath.Join(basePath, subdir)
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(basePath, path)
		relPath = filepath.ToSlash(relPath)
		if e := s.Upload(path, relPath); e != nil {
			log.Printf("[R2] migrate failed: %s: %v", relPath, e)
			return nil
		}
		count++
		return nil
	})
	return count, err
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
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true
	})

	_, err = client.HeadBucket(context.TODO(), &s3.HeadBucketInput{
		Bucket: &bucket,
	})
	if err != nil {
		return fmt.Errorf("连接测试失败: %w", err)
	}

	return nil
}
