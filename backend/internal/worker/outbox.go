// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	cdnPurgeEventType              = "image.cdn.purge"
	storageDeleteMaxPaths          = 100
	storageDeleteMaxR2Paths        = 5
	storageDeleteR2Timeout         = 10 * time.Second
	outboxDefaultMaxAttempts       = uint(10)
	outboxUnconfiguredRetryDelay   = 5 * time.Minute
	outboxMaximumRetryDelay        = 10 * time.Minute
	outboxMaximumResponseBodyBytes = 64 << 10
)

var (
	errCDNPurgeNotConfigured = errors.New("CDN purge delivery is not configured")
	errR2DeleteNotConfigured = errors.New("R2 deletion is not configured")
	errStorageDeleteInvalid  = errors.New("storage deletion event is invalid")
)

type outboxStore interface {
	Claim(context.Context, string, time.Time, int, time.Duration) ([]model.OutboxEvent, error)
	MarkPublished(context.Context, model.OutboxEvent, time.Time) error
	DeferUnconfigured(context.Context, model.OutboxEvent, time.Time, error) error
	MarkFailed(context.Context, model.OutboxEvent, time.Time, time.Duration, error) error
}

type outboxDeliverer interface {
	Deliver(context.Context, model.OutboxEvent) error
}

type gormOutboxStore struct {
	db *gorm.DB
}

func (s *gormOutboxStore) Claim(ctx context.Context, owner string, now time.Time, limit int, lease time.Duration) ([]model.OutboxEvent, error) {
	if limit < 1 {
		return nil, nil
	}
	var events []model.OutboxEvent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A crashed process never owns an event forever. Expired leases become
		// eligible before the next locked claim.
		if err := tx.Model(&model.OutboxEvent{}).
			Where("status = ? AND lease_expires_at <= ?", model.OutboxEventStatusPublishing, now).
			Updates(map[string]interface{}{
				"status":           model.OutboxEventStatusPending,
				"available_at":     now,
				"lease_owner":      nil,
				"lease_token":      nil,
				"lease_expires_at": nil,
			}).Error; err != nil {
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND available_at <= ?", model.OutboxEventStatusPending, now).
			Order("id ASC").Limit(limit).Find(&events).Error; err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		leaseToken, err := secureOutboxToken()
		if err != nil {
			return err
		}
		leaseExpiresAt := now.Add(lease)
		ids := make([]uint64, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		result := tx.Model(&model.OutboxEvent{}).
			Where("id IN ? AND status = ?", ids, model.OutboxEventStatusPending).
			Updates(map[string]interface{}{
				"status":           model.OutboxEventStatusPublishing,
				"attempts":         gorm.Expr("attempts + 1"),
				"lease_owner":      owner,
				"lease_token":      leaseToken,
				"lease_expires_at": leaseExpiresAt,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(events)) {
			return fmt.Errorf("claimed %d of %d outbox events", result.RowsAffected, len(events))
		}
		for i := range events {
			events[i].Status = model.OutboxEventStatusPublishing
			events[i].Attempts++
			events[i].LeaseOwner = &owner
			events[i].LeaseToken = &leaseToken
			events[i].LeaseExpiresAt = &leaseExpiresAt
		}
		return nil
	})
	return events, err
}

func (s *gormOutboxStore) MarkPublished(ctx context.Context, event model.OutboxEvent, now time.Time) error {
	result := s.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_token = ?", event.ID, model.OutboxEventStatusPublishing, leaseToken(event)).
		Updates(map[string]interface{}{
			"status":           model.OutboxEventStatusPublished,
			"published_at":     now,
			"lease_owner":      nil,
			"lease_token":      nil,
			"lease_expires_at": nil,
			"last_error":       "",
		})
	return requireOneOutboxRow(result, event.ID)
}

func (s *gormOutboxStore) DeferUnconfigured(ctx context.Context, event model.OutboxEvent, now time.Time, cause error) error {
	result := s.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_token = ?", event.ID, model.OutboxEventStatusPublishing, leaseToken(event)).
		Updates(map[string]interface{}{
			"status":           model.OutboxEventStatusPending,
			"attempts":         gorm.Expr("GREATEST(attempts - 1, 0)"),
			"available_at":     now.Add(outboxUnconfiguredRetryDelay),
			"lease_owner":      nil,
			"lease_token":      nil,
			"lease_expires_at": nil,
			"last_error":       truncateOutboxError(cause),
		})
	return requireOneOutboxRow(result, event.ID)
}

func (s *gormOutboxStore) MarkFailed(ctx context.Context, event model.OutboxEvent, now time.Time, delay time.Duration, cause error) error {
	maxAttempts := event.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = outboxDefaultMaxAttempts
	}
	status := model.OutboxEventStatusPending
	if event.Attempts >= maxAttempts {
		status = model.OutboxEventStatusDead
	}
	result := s.db.WithContext(ctx).Model(&model.OutboxEvent{}).
		Where("id = ? AND status = ? AND lease_token = ?", event.ID, model.OutboxEventStatusPublishing, leaseToken(event)).
		Updates(map[string]interface{}{
			"status":           status,
			"available_at":     now.Add(delay),
			"lease_owner":      nil,
			"lease_token":      nil,
			"lease_expires_at": nil,
			"last_error":       truncateOutboxError(cause),
		})
	return requireOneOutboxRow(result, event.ID)
}

func requireOneOutboxRow(result *gorm.DB, eventID uint64) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("outbox event %d lease was lost", eventID)
	}
	return nil
}

func leaseToken(event model.OutboxEvent) string {
	if event.LeaseToken == nil {
		return ""
	}
	return *event.LeaseToken
}

type outboxDelivery struct {
	cfg                 config.CDNConfig
	db                  *gorm.DB
	storageRoot         string
	r2                  remoteObjectDeleter
	client              *http.Client
	pacer               *requestPacer
	unconfiguredWarning sync.Once
}

type remoteObjectDeleter interface {
	CanDelete(string, string) bool
	DeleteContext(context.Context, string, string, string) error
}

func newOutboxDelivery(cfg config.CDNConfig) *outboxDelivery {
	return &outboxDelivery{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.PurgeRequestTimeout},
		pacer:  newRequestPacer(cfg.PurgeRequestsPerSecond),
	}
}

func (d *outboxDelivery) Deliver(ctx context.Context, event model.OutboxEvent) error {
	if event.EventType == model.OutboxEventTypeStorageDelete {
		return d.deleteStorageFiles(ctx, event)
	}
	if event.EventType != cdnPurgeEventType {
		return nil
	}
	if !d.cloudflareConfigured() && d.cfg.PurgeWebhookURL == "" {
		d.unconfiguredWarning.Do(func() {
			log.Printf("[outbox] CDN purge is not configured; purge events will remain pending")
		})
		return errCDNPurgeNotConfigured
	}
	files, err := cdnPurgeURLs(d.cfg.PublicBaseURL, event.Payload)
	if err != nil {
		return err
	}
	if err := d.pacer.Wait(ctx); err != nil {
		return err
	}
	if d.cloudflareConfigured() {
		return d.purgeCloudflare(ctx, files)
	}
	return d.purgeWebhook(ctx, event, files)
}

type storageDeletionCapacityLock struct {
	ID uint8
}

func (d *outboxDelivery) deleteStorageFiles(ctx context.Context, event model.OutboxEvent) error {
	if d.db == nil || strings.TrimSpace(d.storageRoot) == "" {
		return errors.New("storage deletion worker is not configured")
	}
	var payload model.StorageDeletePayload
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		return fmt.Errorf("%w: decode payload: %v", errStorageDeleteInvalid, err)
	}
	if len(payload.Paths) == 0 || len(payload.Paths) > storageDeleteMaxPaths {
		return fmt.Errorf("%w: payload has %d paths", errStorageDeleteInvalid, len(payload.Paths))
	}
	if len(payload.RemoteObjects) > storageDeleteMaxR2Paths {
		return fmt.Errorf("%w: payload has %d remote objects", errStorageDeleteInvalid, len(payload.RemoteObjects))
	}
	pathSet := make(map[string]struct{}, len(payload.Paths))
	for _, storedPath := range payload.Paths {
		storedPath = strings.TrimSpace(storedPath)
		if storedPath == "" || len(storedPath) > 500 {
			return fmt.Errorf("%w: payload contains an invalid path", errStorageDeleteInvalid)
		}
		pathSet[storedPath] = struct{}{}
	}
	remoteByPath := make(map[string]model.StorageDeleteRemoteObject, len(payload.RemoteObjects))
	for _, object := range payload.RemoteObjects {
		object.Path = strings.TrimSpace(object.Path)
		object.Backend = strings.TrimSpace(object.Backend)
		object.Endpoint = strings.TrimRight(strings.TrimSpace(object.Endpoint), "/")
		object.Bucket = strings.TrimSpace(object.Bucket)
		if object.Path == "" || len(object.Path) > 500 || object.Backend != "r2" || object.Endpoint == "" || object.Bucket == "" {
			return fmt.Errorf("%w: payload contains an invalid remote object", errStorageDeleteInvalid)
		}
		if _, exists := pathSet[object.Path]; !exists {
			return fmt.Errorf("%w: remote path has no local target", errStorageDeleteInvalid)
		}
		if existing, duplicate := remoteByPath[object.Path]; duplicate && existing != object {
			return fmt.Errorf("%w: payload contains conflicting remote targets", errStorageDeleteInvalid)
		}
		remoteByPath[object.Path] = object
	}
	remoteDeletes := make([]model.StorageDeleteRemoteObject, 0, len(remoteByPath))
	if err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var capacityLock storageDeletionCapacityLock
		if err := tx.Table("v2_capacity_locks").Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", 1).Take(&capacityLock).Error; err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(payload.Paths))
		deletionDirectories := make([]string, 0, len(payload.Paths))
		for _, storedPath := range payload.Paths {
			storedPath = strings.TrimSpace(storedPath)
			if storedPath == "" || len(storedPath) > 500 {
				return fmt.Errorf("%w: payload contains an invalid path", errStorageDeleteInvalid)
			}
			if _, exists := seen[storedPath]; exists {
				continue
			}
			seen[storedPath] = struct{}{}
			var references int64
			if err := tx.Model(&model.ImageVariant{}).Where("storage_path = ?", storedPath).Count(&references).Error; err != nil {
				return err
			}
			if references > 0 {
				continue
			}
			if err := tx.Model(&model.ImageFile{}).
				Where("original_path = ? OR thumbnail_path = ? OR processed_path = ?", storedPath, storedPath, storedPath).
				Count(&references).Error; err != nil {
				return err
			}
			if references > 0 {
				continue
			}
			fullPath, err := deletionPathInside(d.storageRoot, storedPath)
			if err != nil {
				return fmt.Errorf("%w: resolve path %q: %v", errStorageDeleteInvalid, storedPath, err)
			}
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove storage file %q: %w", storedPath, err)
			}
			deletionDirectories = append(deletionDirectories, filepath.Dir(fullPath))
			if object, deleteRemote := remoteByPath[storedPath]; deleteRemote {
				remoteDeletes = append(remoteDeletes, object)
			}
		}
		return syncStorageDeletionDirectories(deletionDirectories)
	}); err != nil {
		return err
	}
	if len(remoteDeletes) == 0 {
		return nil
	}
	for _, object := range remoteDeletes {
		if d.r2 == nil || !d.r2.CanDelete(object.Endpoint, object.Bucket) {
			return errR2DeleteNotConfigured
		}
		deleteCtx, cancel := context.WithTimeout(ctx, storageDeleteR2Timeout)
		err := d.r2.DeleteContext(deleteCtx, object.Endpoint, object.Bucket, object.Path)
		cancel()
		if err != nil {
			return fmt.Errorf("remove R2 object %q: %w", object.Path, err)
		}
	}
	return nil
}

func syncStorageDeletionDirectories(directories []string) error {
	return syncStorageDeletionDirectoriesWith(directories, syncStorageDeletionDirectory)
}

func syncStorageDeletionDirectoriesWith(directories []string, syncDirectory func(string) error) error {
	unique := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		if directory == "" {
			continue
		}
		directory = filepath.Clean(directory)
		unique[directory] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for directory := range unique {
		ordered = append(ordered, directory)
	}
	sort.Strings(ordered)
	for _, directory := range ordered {
		if err := syncDirectory(directory); err != nil && !ignorableStorageDirectorySyncError(err) {
			return fmt.Errorf("sync storage directory %q: %w", directory, err)
		}
	}
	return nil
}

func syncStorageDeletionDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil && !ignorableStorageDirectorySyncError(syncErr) {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return syncErr
}

func ignorableStorageDirectorySyncError(err error) bool {
	return errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, unix.EINVAL) ||
		errors.Is(err, unix.ENOTSUP) ||
		errors.Is(err, unix.EROFS)
}

func (d *outboxDelivery) cloudflareConfigured() bool {
	return d.cfg.CloudflareZoneID != "" && d.cfg.CloudflareAPIToken != ""
}

func (d *outboxDelivery) purgeCloudflare(ctx context.Context, files []string) error {
	body, err := json.Marshal(map[string]interface{}{"files": files})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(d.cfg.CloudflareAPIBaseURL, "/") + "/zones/" + url.PathEscape(d.cfg.CloudflareZoneID) + "/purge_cache"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.cfg.CloudflareAPIToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("Cloudflare purge request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, outboxMaximumResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("read Cloudflare purge response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return newOutboxHTTPError("Cloudflare", resp, responseBody)
	}
	var result struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return fmt.Errorf("decode Cloudflare purge response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("Cloudflare purge rejected: %s", compactResponse(responseBody))
	}
	return nil
}

func (d *outboxDelivery) purgeWebhook(ctx context.Context, event model.OutboxEvent, files []string) error {
	body, err := json.Marshal(struct {
		EventID     uint64   `json:"event_id"`
		EventType   string   `json:"event_type"`
		AggregateID string   `json:"aggregate_id"`
		DedupeKey   string   `json:"dedupe_key"`
		Files       []string `json:"files"`
	}{
		EventID: event.ID, EventType: event.EventType, AggregateID: event.AggregateID,
		DedupeKey: event.DedupeKey, Files: files,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.cfg.PurgeWebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", event.DedupeKey)
	req.Header.Set("X-SummeRain-Event-ID", strconv.FormatUint(event.ID, 10))
	if d.cfg.PurgeWebhookToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.cfg.PurgeWebhookToken)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("CDN purge webhook request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, outboxMaximumResponseBodyBytes))
	if err != nil {
		return fmt.Errorf("read CDN purge webhook response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return newOutboxHTTPError("CDN purge webhook", resp, responseBody)
	}
	return nil
}

type outboxHTTPError struct {
	message    string
	retryAfter time.Duration
}

func (e *outboxHTTPError) Error() string             { return e.message }
func (e *outboxHTTPError) RetryAfter() time.Duration { return e.retryAfter }

func newOutboxHTTPError(service string, resp *http.Response, responseBody []byte) error {
	return &outboxHTTPError{
		message:    fmt.Sprintf("%s returned HTTP %d: %s", service, resp.StatusCode, compactResponse(responseBody)),
		retryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
	}
}

func cdnPurgeURLs(publicBaseURL, payload string) ([]string, error) {
	var eventPayload struct {
		OldAssetLink string `json:"old_asset_link"`
		NewAssetLink string `json:"new_asset_link"`
		AssetLink    string `json:"asset_link"`
	}
	if err := json.Unmarshal([]byte(payload), &eventPayload); err != nil {
		return nil, fmt.Errorf("decode CDN purge payload: %w", err)
	}
	base, err := normalizePublicBaseURL(publicBaseURL)
	if err != nil {
		return nil, err
	}
	links := []string{eventPayload.OldAssetLink, eventPayload.NewAssetLink, eventPayload.AssetLink}
	seenLinks := make(map[string]struct{}, len(links))
	files := make([]string, 0, len(links)*10)
	for _, link := range links {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		if _, exists := seenLinks[link]; exists {
			continue
		}
		if !validAssetLink(link) {
			return nil, fmt.Errorf("invalid asset link in CDN purge payload")
		}
		seenLinks[link] = struct{}{}
		encoded := url.PathEscape(link)
		canonical := base + "/i/" + encoded
		files = append(files,
			canonical+".webp",
			canonical+"/publish.webp",
			canonical+"/master.webp",
			canonical+"/gallery.webp",
			canonical+"/admin.webp",
			canonical,
			canonical+"?type=thumbnail",
			canonical+".webp?w=400",
			canonical+".webp?w=80&h=60",
			canonical+".webp?q=85",
		)
	}
	if len(files) == 0 {
		return nil, errors.New("CDN purge payload has no old_asset_link or asset_link")
	}
	return files, nil
}

func normalizePublicBaseURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("CDN public base URL must be an absolute http(s) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("CDN public base URL must not contain credentials, query, or fragment")
	}
	return strings.TrimRight(value, "/"), nil
}

func validAssetLink(value string) bool {
	if len(value) == 0 || len(value) > 255 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

type requestPacer struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

func newRequestPacer(requestsPerSecond int) *requestPacer {
	if requestsPerSecond < 1 {
		requestsPerSecond = 1
	}
	return &requestPacer{interval: time.Second / time.Duration(requestsPerSecond)}
}

func (p *requestPacer) Wait(ctx context.Context) error {
	p.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if p.next.After(now) {
		wait = p.next.Sub(now)
		p.next = p.next.Add(p.interval)
	} else {
		p.next = now.Add(p.interval)
	}
	p.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (m *Manager) runOutbox(ctx context.Context) {
	if m.DB == nil || m.Config == nil {
		log.Printf("[outbox] disabled: database or configuration is unavailable")
		return
	}
	cfg := m.Config.CDN
	store := &gormOutboxStore{db: m.DB}
	delivery := newOutboxDelivery(cfg)
	delivery.db = m.DB
	delivery.storageRoot = m.Config.Storage.BasePath
	delivery.r2 = m.R2
	workerID := newOutboxWorkerID()
	batchSize := effectiveOutboxBatchSize(cfg)

	run := func() {
		for ctx.Err() == nil {
			processed, err := processOutboxBatch(ctx, store, delivery, workerID, time.Now(), batchSize, cfg.OutboxLease)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Printf("[outbox] batch failed: %v", err)
				}
				return
			}
			if processed < batchSize {
				return
			}
		}
	}

	run()
	ticker := time.NewTicker(cfg.OutboxPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

func processOutboxBatch(ctx context.Context, store outboxStore, delivery outboxDeliverer, workerID string, now time.Time, limit int, lease time.Duration) (int, error) {
	events, err := store.Claim(ctx, workerID, now, limit, lease)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return len(events), err
		}
		deliveryErr := delivery.Deliver(ctx, event)
		finishedAt := time.Now()
		switch {
		case deliveryErr == nil:
			err = store.MarkPublished(ctx, event, finishedAt)
		case ctx.Err() != nil && (errors.Is(deliveryErr, context.Canceled) || errors.Is(deliveryErr, context.DeadlineExceeded)):
			return len(events), ctx.Err()
		case errors.Is(deliveryErr, errCDNPurgeNotConfigured), errors.Is(deliveryErr, errR2DeleteNotConfigured):
			err = store.DeferUnconfigured(ctx, event, finishedAt, deliveryErr)
		case event.EventType == model.OutboxEventTypeStorageDelete && !errors.Is(deliveryErr, errStorageDeleteInvalid):
			err = store.DeferUnconfigured(ctx, event, finishedAt, deliveryErr)
		default:
			delay := outboxRetryDelay(event.Attempts)
			var retryable interface{ RetryAfter() time.Duration }
			if errors.As(deliveryErr, &retryable) && retryable.RetryAfter() > delay {
				delay = minDuration(retryable.RetryAfter(), outboxMaximumRetryDelay)
			}
			err = store.MarkFailed(ctx, event, finishedAt, delay, deliveryErr)
			log.Printf("[outbox] delivery failed id=%d type=%s attempt=%d: %v", event.ID, event.EventType, event.Attempts, deliveryErr)
		}
		if err != nil {
			return len(events), err
		}
	}
	return len(events), nil
}

func effectiveOutboxBatchSize(cfg config.CDNConfig) int {
	batch := cfg.OutboxBatchSize
	if batch < 1 {
		batch = 1
	}
	interval := time.Second / time.Duration(maxInt(cfg.PurgeRequestsPerSecond, 1))
	budget := cfg.OutboxLease - 5*time.Second
	perRequest := cfg.PurgeRequestTimeout + interval
	if budget > 0 && perRequest > 0 {
		leaseBound := int(budget / perRequest)
		if leaseBound < 1 {
			leaseBound = 1
		}
		if batch > leaseBound {
			batch = leaseBound
		}
	}
	return batch
}

func outboxRetryDelay(attempt uint) time.Duration {
	if attempt > 8 {
		attempt = 8
	}
	delay := time.Duration(uint64(1)<<attempt) * time.Second
	return minDuration(delay, outboxMaximumRetryDelay)
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil && when.After(now) {
		return when.Sub(now)
	}
	return 0
}

func compactResponse(body []byte) string {
	message := strings.TrimSpace(string(body))
	if message == "" {
		return "empty response"
	}
	if len(message) > 1024 {
		message = message[:1024]
	}
	return message
}

func truncateOutboxError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 4096 {
		message = message[:4096]
	}
	return message
}

func secureOutboxToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate outbox lease token: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func newOutboxWorkerID() string {
	hostname, _ := os.Hostname()
	token, err := secureOutboxToken()
	if err != nil {
		token = strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	workerID := fmt.Sprintf("%s:%d:%s", hostname, os.Getpid(), token[:minInt(len(token), 12)])
	if len(workerID) > 100 {
		workerID = workerID[len(workerID)-100:]
	}
	return workerID
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
