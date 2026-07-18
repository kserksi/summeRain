// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"strings"
	"testing"
)

func TestSchemaMigrationPlanIsValid(t *testing.T) {
	if err := validateSchemaMigrationPlan(schemaMigrations); err != nil {
		t.Fatalf("validateSchemaMigrationPlan() error = %v", err)
	}
}

func TestSchemaMigrationChecksumsAreStable(t *testing.T) {
	want := map[uint64]string{
		2026071501: "fa6af6dbe0662431b032d1db73b6ed7651a828d44a7736d90eb87aff1150f420",
		2026071502: "00a41ae54649613e3ac78a520ffb3d2838bf10245a7689406efcd7e571ab3575",
		2026071601: "e7fa3b27d3571f4b48d6d2477c943ad3183bb13eb5171521a1c997e0e3c7b359",
		2026071602: "40f84d93d4fcd49ec1352b7f618ca8cd7a63f325be6baa44457eec56025b801b",
		2026071603: "c9ad96f0ca384ff007e5e11e7e5e12d6aaad68a3c6caafed04578f7f84a527f0",
	}
	for _, migration := range schemaMigrations {
		got := schemaMigrationChecksum(migration)
		expected, ok := want[migration.Version]
		if !ok {
			t.Fatalf("migration %d does not have a checksum fixture", migration.Version)
		}
		if got != expected {
			t.Fatalf("migration %d checksum = %s, want %s; append a new migration instead of editing an applied one", migration.Version, got, expected)
		}
	}
	if len(want) != len(schemaMigrations) {
		t.Fatalf("checksum fixture count = %d, migration count = %d", len(want), len(schemaMigrations))
	}
}

func TestRemoteLineageMigrationDoesNotInferHistoricalStorage(t *testing.T) {
	for _, migration := range schemaMigrations {
		if migration.Version != 2026071603 {
			continue
		}
		for _, operation := range migration.Operations {
			if operation.Kind == schemaMigrationSQL {
				t.Fatalf("remote lineage migration contains data inference operation %q", operation.Name)
			}
		}
		return
	}
	t.Fatal("remote lineage migration is missing")
}

func TestSchemaMigrationChecksumCoversOperations(t *testing.T) {
	original := schemaMigrations[0]
	mutated := original
	mutated.Operations = append([]schemaMigrationOperation(nil), original.Operations...)
	mutated.Operations[0].Statement += " "
	if schemaMigrationChecksum(original) == schemaMigrationChecksum(mutated) {
		t.Fatal("checksum did not change after operation mutation")
	}
}

func TestValidateAppliedSchemaMigrations(t *testing.T) {
	migration := schemaMigrations[0]
	valid := map[uint64]appliedSchemaMigration{
		migration.Version: {
			Version:  migration.Version,
			Name:     migration.Name,
			Checksum: schemaMigrationChecksum(migration),
		},
	}
	if err := validateAppliedSchemaMigrations(schemaMigrations, valid); err != nil {
		t.Fatalf("valid applied migrations rejected: %v", err)
	}

	wrongChecksum := map[uint64]appliedSchemaMigration{
		migration.Version: {Version: migration.Version, Name: migration.Name, Checksum: strings.Repeat("0", 64)},
	}
	if err := validateAppliedSchemaMigrations(schemaMigrations, wrongChecksum); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}

	unknown := map[uint64]appliedSchemaMigration{
		9999999999: {Version: 9999999999, Name: "future", Checksum: strings.Repeat("a", 64)},
	}
	if err := validateAppliedSchemaMigrations(schemaMigrations, unknown); err == nil {
		t.Fatal("unknown database migration was accepted")
	}

	later := schemaMigrations[1]
	gap := map[uint64]appliedSchemaMigration{
		later.Version: {Version: later.Version, Name: later.Name, Checksum: schemaMigrationChecksum(later)},
	}
	if err := validateAppliedSchemaMigrations(schemaMigrations, gap); err == nil {
		t.Fatal("applied migration after a version gap was accepted")
	}
}

func TestV2MigrationContainsRequiredObjects(t *testing.T) {
	required := []string{
		"`pipeline_version`",
		"`processing_status`",
		"`origin_alias`",
		"`idx_images_processing_status`",
		"`idx_images_origin_alias`",
		"`image_variants`",
		"`upload_sessions`",
		"`upload_parts`",
		"`processing_jobs`",
		"`outbox_events`",
		"`v2_capacity_locks`",
	}
	var plan strings.Builder
	for _, migration := range schemaMigrations {
		for _, operation := range migration.Operations {
			plan.WriteString(operation.Statement)
			plan.WriteByte('\n')
		}
	}
	for _, object := range required {
		if !strings.Contains(plan.String(), object) {
			t.Errorf("migration plan does not contain %s", object)
		}
	}
}

func TestSchemaMigrationLockNameFitsMySQLLimit(t *testing.T) {
	short := schemaMigrationLockName("summerain")
	if short != schemaMigrationLockPrefix+"summerain" {
		t.Fatalf("short lock name = %q", short)
	}
	long := schemaMigrationLockName(strings.Repeat("database", 20))
	if len(long) > 64 {
		t.Fatalf("lock name length = %d, want <= 64", len(long))
	}
	if long != schemaMigrationLockName(strings.Repeat("database", 20)) {
		t.Fatal("lock name is not deterministic")
	}
}

func TestValidateSchemaMigrationPlanRejectsInvalidOrder(t *testing.T) {
	invalid := []schemaMigration{
		{Version: 2, Name: "second", Operations: []schemaMigrationOperation{{Kind: schemaMigrationSQL, Name: "x", Statement: "SELECT 1"}}},
		{Version: 1, Name: "first", Operations: []schemaMigrationOperation{{Kind: schemaMigrationSQL, Name: "y", Statement: "SELECT 1"}}},
	}
	if err := validateSchemaMigrationPlan(invalid); err == nil {
		t.Fatal("out-of-order plan was accepted")
	}
}
