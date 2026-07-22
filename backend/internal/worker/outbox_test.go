// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kserksi/summerain/internal/config"
	"github.com/kserksi/summerain/internal/model"
)

func TestCDNPurgeURLsIncludesFixedAndCompatibilityRoutes(t *testing.T) {
	files, err := cdnPurgeURLs("https://cdn.example.com/root/", `{"old_asset_link":"public123","asset_link":"deleted456"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 20 {
		t.Fatalf("files count = %d, want 20", len(files))
	}
	want := []string{
		"https://cdn.example.com/root/i/public123.webp",
		"https://cdn.example.com/root/i/public123/publish.webp",
		"https://cdn.example.com/root/i/public123/master.webp",
		"https://cdn.example.com/root/i/public123/gallery.webp",
		"https://cdn.example.com/root/i/public123/admin.webp",
		"https://cdn.example.com/root/i/public123",
		"https://cdn.example.com/root/i/public123?type=thumbnail",
	}
	for _, expected := range want {
		if !containsString(files, expected) {
			t.Errorf("missing purge URL %q", expected)
		}
	}
}

func TestCDNPurgeURLsIncludesNewVisibilityAlias(t *testing.T) {
	files, err := cdnPurgeURLs("https://cdn.example.com", `{"old_asset_link":"public123","new_asset_link":"private123S"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 20 {
		t.Fatalf("files count = %d, want 20", len(files))
	}
	for _, expected := range []string{
		"https://cdn.example.com/i/public123/publish.webp",
		"https://cdn.example.com/i/private123S/publish.webp",
	} {
		if !containsString(files, expected) {
			t.Errorf("missing purge URL %q", expected)
		}
	}
}

func TestCDNPurgeURLsRejectsUnsafeOrMissingAliases(t *testing.T) {
	for _, payload := range []string{
		`{}`,
		`{"old_asset_link":"../private"}`,
		`{"asset_link":"public?token=secret"}`,
		`not-json`,
	} {
		if _, err := cdnPurgeURLs("https://cdn.example.com", payload); err == nil {
			t.Errorf("payload %q was accepted", payload)
		}
	}
}

func TestOutboxDeliveryUsesCloudflareAPIAndBearerToken(t *testing.T) {
	var requestPath string
	var authorization string
	var files []string
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		requestPath = r.URL.Path
		authorization = r.Header.Get("Authorization")
		var body struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		files = body.Files
		return jsonHTTPResponse(http.StatusOK, `{"success":true,"errors":[]}`), nil
	})

	delivery := newOutboxDelivery(config.CDNConfig{
		PublicBaseURL:          "https://cdn.example.com",
		CloudflareZoneID:       "zone-id",
		CloudflareAPIToken:     "api-token",
		CloudflareAPIBaseURL:   "https://api.cloudflare.test",
		PurgeRequestsPerSecond: 20,
		PurgeRequestTimeout:    2 * time.Second,
	})
	delivery.client = &http.Client{Transport: transport}
	err := delivery.Deliver(context.Background(), model.OutboxEvent{
		ID: 7, EventType: cdnPurgeEventType, Payload: `{"old_asset_link":"public123"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestPath != "/zones/zone-id/purge_cache" {
		t.Fatalf("request path = %q", requestPath)
	}
	if authorization != "Bearer api-token" {
		t.Fatalf("Authorization = %q", authorization)
	}
	if len(files) != 10 || !containsString(files, "https://cdn.example.com/i/public123/gallery.webp") {
		t.Fatalf("unexpected purge files: %#v", files)
	}
}

func TestOutboxDeliveryPrefersCloudflareOverWebhook(t *testing.T) {
	cloudflareCalls := 0
	webhookCalls := 0
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.cloudflare.test" {
			cloudflareCalls++
			return jsonHTTPResponse(http.StatusOK, `{"success":true}`), nil
		}
		webhookCalls++
		return jsonHTTPResponse(http.StatusNoContent, ""), nil
	})

	delivery := newOutboxDelivery(config.CDNConfig{
		PublicBaseURL: "https://cdn.example.com", CloudflareZoneID: "zone",
		CloudflareAPIToken: "token", CloudflareAPIBaseURL: "https://api.cloudflare.test",
		PurgeWebhookURL: "https://webhook.test/purge", PurgeRequestsPerSecond: 20, PurgeRequestTimeout: 2 * time.Second,
	})
	delivery.client = &http.Client{Transport: transport}
	if err := delivery.Deliver(context.Background(), model.OutboxEvent{EventType: cdnPurgeEventType, Payload: `{"asset_link":"abc123"}`}); err != nil {
		t.Fatal(err)
	}
	if cloudflareCalls != 1 || webhookCalls != 0 {
		t.Fatalf("cloudflare calls=%d webhook calls=%d", cloudflareCalls, webhookCalls)
	}
}

func TestOutboxDeliveryRejectsUnsuccessfulCloudflareEnvelope(t *testing.T) {
	delivery := newOutboxDelivery(config.CDNConfig{
		PublicBaseURL: "https://cdn.example.com", CloudflareZoneID: "zone",
		CloudflareAPIToken: "token", CloudflareAPIBaseURL: "https://api.cloudflare.test",
		PurgeRequestsPerSecond: 20, PurgeRequestTimeout: 2 * time.Second,
	})
	delivery.client = &http.Client{Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonHTTPResponse(http.StatusOK, `{"success":false,"errors":[{"code":1000,"message":"rejected"}]}`), nil
	})}
	err := delivery.Deliver(context.Background(), model.OutboxEvent{EventType: cdnPurgeEventType, Payload: `{"asset_link":"abc123"}`})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("error=%v, want Cloudflare rejection", err)
	}
}

func TestOutboxDeliverySupportsAuthenticatedWebhook(t *testing.T) {
	var gotAuthorization, gotIdempotency string
	var gotFiles []string
	transport := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		gotAuthorization = r.Header.Get("Authorization")
		gotIdempotency = r.Header.Get("Idempotency-Key")
		var body struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode webhook: %v", err)
		}
		gotFiles = body.Files
		return jsonHTTPResponse(http.StatusNoContent, ""), nil
	})

	delivery := newOutboxDelivery(config.CDNConfig{
		PublicBaseURL: "https://cdn.example.com", PurgeWebhookURL: "https://webhook.test/purge",
		PurgeWebhookToken: "webhook-token", PurgeRequestsPerSecond: 20, PurgeRequestTimeout: 2 * time.Second,
	})
	delivery.client = &http.Client{Transport: transport}
	event := model.OutboxEvent{ID: 9, EventType: cdnPurgeEventType, DedupeKey: "image:9:private", Payload: `{"old_asset_link":"abc123"}`}
	if err := delivery.Deliver(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if gotAuthorization != "Bearer webhook-token" || gotIdempotency != event.DedupeKey {
		t.Fatalf("unexpected webhook headers auth=%q idempotency=%q", gotAuthorization, gotIdempotency)
	}
	if len(gotFiles) != 10 {
		t.Fatalf("webhook files count=%d, want 10", len(gotFiles))
	}
}

func TestOutboxDeliveryDoesNotAcknowledgeUnconfiguredPurge(t *testing.T) {
	delivery := newOutboxDelivery(config.CDNConfig{PurgeRequestsPerSecond: 1})
	err := delivery.Deliver(context.Background(), model.OutboxEvent{EventType: cdnPurgeEventType, Payload: `{}`})
	if !errors.Is(err, errCDNPurgeNotConfigured) {
		t.Fatalf("error = %v, want errCDNPurgeNotConfigured", err)
	}
	if err := delivery.Deliver(context.Background(), model.OutboxEvent{EventType: "image.processing.completed"}); err != nil {
		t.Fatalf("non-CDN event failed: %v", err)
	}
}

func TestProcessOutboxBatchDefersUnconfiguredWithoutPublishing(t *testing.T) {
	store := &fakeOutboxStore{events: []model.OutboxEvent{
		{ID: 1, EventType: cdnPurgeEventType, Payload: `{"asset_link":"abc"}`},
		{ID: 2, EventType: "image.processing.completed"},
	}}
	delivery := newOutboxDelivery(config.CDNConfig{PurgeRequestsPerSecond: 1})
	processed, err := processOutboxBatch(context.Background(), store, delivery, "test", time.Now(), 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d, want 2", processed)
	}
	if !reflect.DeepEqual(store.deferred, []uint64{1}) {
		t.Fatalf("deferred=%v, want [1]", store.deferred)
	}
	if !reflect.DeepEqual(store.published, []uint64{2}) {
		t.Fatalf("published=%v, want [2]", store.published)
	}
}

func TestProcessOutboxBatchRetriesDeliveryFailureWithoutPublishing(t *testing.T) {
	store := &fakeOutboxStore{events: []model.OutboxEvent{{ID: 3, EventType: cdnPurgeEventType, Attempts: 1}}}
	processed, err := processOutboxBatch(
		context.Background(), store, failingOutboxDelivery{err: errors.New("temporary outage")},
		"test", time.Now(), 10, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || !reflect.DeepEqual(store.failed, []uint64{3}) {
		t.Fatalf("processed=%d failed=%v", processed, store.failed)
	}
	if len(store.published) != 0 {
		t.Fatalf("failed event was published: %v", store.published)
	}
}

func TestProcessOutboxBatchDefersUnconfiguredR2WithoutConsumingAttempts(t *testing.T) {
	store := &fakeOutboxStore{events: []model.OutboxEvent{{ID: 4, EventType: model.OutboxEventTypeStorageDelete}}}
	processed, err := processOutboxBatch(
		context.Background(), store, failingOutboxDelivery{err: errR2DeleteNotConfigured},
		"test", time.Now(), 10, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || !reflect.DeepEqual(store.deferred, []uint64{4}) {
		t.Fatalf("processed=%d deferred=%v, want one deferred event", processed, store.deferred)
	}
	if len(store.failed) != 0 || len(store.published) != 0 {
		t.Fatalf("unconfigured R2 event failed=%v published=%v", store.failed, store.published)
	}
}

func TestProcessOutboxBatchKeepsTransientStorageDeletionPending(t *testing.T) {
	store := &fakeOutboxStore{events: []model.OutboxEvent{{ID: 5, EventType: model.OutboxEventTypeStorageDelete}}}
	processed, err := processOutboxBatch(
		context.Background(), store, failingOutboxDelivery{err: errors.New("R2 timeout")},
		"test", time.Now(), 10, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 || !reflect.DeepEqual(store.deferred, []uint64{5}) || len(store.failed) != 0 {
		t.Fatalf("processed=%d deferred=%v failed=%v", processed, store.deferred, store.failed)
	}
}

func TestProcessOutboxBatchReleasesFencedClaimsAfterCancellation(t *testing.T) {
	owner := "cancelled-worker"
	token := "cancelled-lease"
	store := &fakeOutboxStore{events: []model.OutboxEvent{
		{ID: 6, Status: model.OutboxEventStatusPublishing, Attempts: 1, LeaseOwner: &owner, LeaseToken: &token},
		{ID: 7, Status: model.OutboxEventStatusPublishing, Attempts: 1, LeaseOwner: &owner, LeaseToken: &token},
	}, trackLeaseState: true}
	ctx, cancel := context.WithCancel(context.Background())
	delivery := outboxDelivererFunc(func(context.Context, model.OutboxEvent) error {
		cancel()
		return context.Canceled
	})

	processed, err := processOutboxBatch(ctx, store, delivery, owner, time.Now(), 10, time.Minute)
	if processed != len(store.events) || !errors.Is(err, context.Canceled) {
		t.Fatalf("process result=(%d, %v), want (%d, context canceled)", processed, err, len(store.events))
	}
	if !reflect.DeepEqual(store.released, []uint64{6, 7}) {
		t.Fatalf("released=%v, want [6 7]", store.released)
	}
	if store.releaseOwner != owner {
		t.Fatalf("release owner=%q, want %q", store.releaseOwner, owner)
	}
	if store.releaseContextErr != nil {
		t.Fatalf("release used canceled execution context: %v", store.releaseContextErr)
	}
	if !store.releaseRefundAttempt || store.events[0].Attempts != 0 || store.events[1].Attempts != 0 {
		t.Fatalf("cancellation did not refund claims: refund=%t attempts=(%d, %d)", store.releaseRefundAttempt, store.events[0].Attempts, store.events[1].Attempts)
	}
}

func TestProcessOutboxBatchReleasesClaimsAndRepanics(t *testing.T) {
	owner := "panic-worker"
	token := "panic-lease"
	store := &fakeOutboxStore{events: []model.OutboxEvent{
		{ID: 8, Status: model.OutboxEventStatusPublishing, Attempts: 1, MaxAttempts: 10, LeaseOwner: &owner, LeaseToken: &token},
		{ID: 9, Status: model.OutboxEventStatusPublishing, Attempts: 1, MaxAttempts: 10, LeaseOwner: &owner, LeaseToken: &token},
	}, trackLeaseState: true}
	panicValue := "delivery panic"

	defer func() {
		if recovered := recover(); recovered != panicValue {
			t.Fatalf("panic=%v, want %q", recovered, panicValue)
		}
		if !reflect.DeepEqual(store.failed, []uint64{8}) {
			t.Fatalf("failed=%v, want active event [8]", store.failed)
		}
		if !reflect.DeepEqual(store.released, []uint64{9}) {
			t.Fatalf("released=%v, want only unprocessed event [9]", store.released)
		}
		if store.releaseContextErr != nil {
			t.Fatalf("panic cleanup context error=%v", store.releaseContextErr)
		}
		if !store.releaseRefundAttempt || store.events[0].Attempts != 1 || store.events[1].Attempts != 0 {
			t.Fatalf("panic cleanup attempts=(%d, %d), want active=1 remaining=0", store.events[0].Attempts, store.events[1].Attempts)
		}
	}()

	_, _ = processOutboxBatch(
		context.Background(),
		store,
		outboxDelivererFunc(func(context.Context, model.OutboxEvent) error { panic(panicValue) }),
		owner,
		time.Now(),
		10,
		time.Minute,
	)
}

func TestProcessOutboxBatchPanicPreservesAttemptWhenMarkFailedErrors(t *testing.T) {
	owner := "panic-worker"
	token := "panic-lease"
	store := &fakeOutboxStore{events: []model.OutboxEvent{
		{ID: 10, Status: model.OutboxEventStatusPublishing, Attempts: 1, MaxAttempts: 10, LeaseOwner: &owner, LeaseToken: &token},
		{ID: 11, Status: model.OutboxEventStatusPublishing, Attempts: 1, MaxAttempts: 10, LeaseOwner: &owner, LeaseToken: &token},
	}, trackLeaseState: true, markFailedErr: errors.New("database unavailable")}
	panicValue := "delivery panic"

	defer func() {
		if recovered := recover(); recovered != panicValue {
			t.Fatalf("panic=%v, want %q", recovered, panicValue)
		}
		if store.events[0].Attempts != 1 || store.events[1].Attempts != 0 {
			t.Fatalf("panic cleanup attempts=(%d, %d), want active=1 remaining=0", store.events[0].Attempts, store.events[1].Attempts)
		}
		if !reflect.DeepEqual(store.released, []uint64{10, 11}) {
			t.Fatalf("released=%v, want active and remaining events", store.released)
		}
	}()

	_, _ = processOutboxBatch(
		context.Background(),
		store,
		outboxDelivererFunc(func(context.Context, model.OutboxEvent) error { panic(panicValue) }),
		owner,
		time.Now(),
		10,
		time.Minute,
	)
}

func TestOutboxRetryAndLeaseBound(t *testing.T) {
	if got := outboxRetryDelay(1); got != 2*time.Second {
		t.Fatalf("retry delay=%s, want 2s", got)
	}
	if got := outboxRetryDelay(100); got != 256*time.Second {
		t.Fatalf("capped retry delay=%s, want 256s", got)
	}
	batch := effectiveOutboxBatchSize(config.CDNConfig{
		OutboxBatchSize: 50, OutboxLease: 30 * time.Second,
		PurgeRequestTimeout: 10 * time.Second, PurgeRequestsPerSecond: 1,
	})
	if batch != 2 {
		t.Fatalf("effective batch=%d, want 2", batch)
	}
	now := time.Now()
	if got := parseRetryAfter("120", now); got != 2*time.Minute {
		t.Fatalf("Retry-After=%s, want 2m", got)
	}
}

type fakeOutboxStore struct {
	events               []model.OutboxEvent
	published            []uint64
	deferred             []uint64
	failed               []uint64
	released             []uint64
	releaseOwner         string
	releaseContextErr    error
	releaseRefundAttempt bool
	markFailedErr        error
	claimLimit           int
	trackLeaseState      bool
}

type failingOutboxDelivery struct{ err error }

type outboxDelivererFunc func(context.Context, model.OutboxEvent) error

func (d failingOutboxDelivery) Deliver(context.Context, model.OutboxEvent) error { return d.err }

func (d outboxDelivererFunc) Deliver(ctx context.Context, event model.OutboxEvent) error {
	return d(ctx, event)
}

func (s *fakeOutboxStore) Claim(_ context.Context, _ string, _ time.Time, limit int, _ time.Duration) ([]model.OutboxEvent, error) {
	s.claimLimit = limit
	if len(s.events) > limit {
		return s.events[:limit], nil
	}
	return s.events, nil
}

func (s *fakeOutboxStore) MarkPublished(_ context.Context, event model.OutboxEvent, _ time.Time) error {
	s.published = append(s.published, event.ID)
	if s.trackLeaseState {
		s.setEventStatus(event.ID, model.OutboxEventStatusPublished)
	}
	return nil
}

func (s *fakeOutboxStore) DeferUnconfigured(_ context.Context, event model.OutboxEvent, _ time.Time, _ error) error {
	s.deferred = append(s.deferred, event.ID)
	if s.trackLeaseState {
		s.setEventStatus(event.ID, model.OutboxEventStatusPending)
	}
	return nil
}

func (s *fakeOutboxStore) MarkFailed(_ context.Context, event model.OutboxEvent, _ time.Time, _ time.Duration, _ error) error {
	s.failed = append(s.failed, event.ID)
	if s.markFailedErr != nil {
		return s.markFailedErr
	}
	if s.trackLeaseState {
		status := model.OutboxEventStatusPending
		if event.MaxAttempts > 0 && event.Attempts >= event.MaxAttempts {
			status = model.OutboxEventStatusDead
		}
		s.setEventStatus(event.ID, status)
	}
	return nil
}

func (s *fakeOutboxStore) ReleaseClaims(ctx context.Context, owner string, events []model.OutboxEvent, _ time.Time, refundAttempt bool) error {
	s.releaseContextErr = ctx.Err()
	s.releaseOwner = owner
	s.releaseRefundAttempt = refundAttempt
	for _, event := range events {
		if s.trackLeaseState {
			current := s.eventByID(event.ID)
			if current == nil || current.Status != model.OutboxEventStatusPublishing {
				continue
			}
			current.Status = model.OutboxEventStatusPending
			current.LeaseOwner = nil
			current.LeaseToken = nil
			current.LeaseExpiresAt = nil
			if refundAttempt && current.Attempts > 0 {
				current.Attempts--
			}
		}
		s.released = append(s.released, event.ID)
	}
	return nil
}

func (s *fakeOutboxStore) setEventStatus(id uint64, status string) {
	if event := s.eventByID(id); event != nil {
		event.Status = status
		event.LeaseOwner = nil
		event.LeaseToken = nil
		event.LeaseExpiresAt = nil
	}
}

func (s *fakeOutboxStore) eventByID(id uint64) *model.OutboxEvent {
	for i := range s.events {
		if s.events[i].ID == id {
			return &s.events[i]
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	values = append([]string(nil), values...)
	sort.Strings(values)
	index := sort.SearchStrings(values, target)
	return index < len(values) && strings.EqualFold(values[index], target)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
