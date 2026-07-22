// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/kserksi/summerain/internal/model"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	bootstrapMySQLDSNEnv      = "SUMMERAIN_BOOTSTRAP_TEST_MYSQL_DSN"
	bootstrapMySQLRequiredEnv = "SUMMERAIN_REQUIRE_MYSQL_TESTS"
)

type bootstrapMySQLFixture struct {
	DatabaseName string
	DSN          string
	DB           *gorm.DB
	SQL          *sql.DB
}

func TestBootstrapDatabaseMySQLEmptyDatabaseIsIdempotentWithOneConnection(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	fixture.SQL.SetMaxOpenConns(1)
	fixture.SQL.SetMaxIdleConns(1)

	runBootstrapDatabase(t, fixture.DB)

	var migrationCount int64
	if err := fixture.DB.Table("schema_migrations").Count(&migrationCount).Error; err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	if migrationCount != int64(len(schemaMigrations)) {
		t.Fatalf("schema migration count = %d, want %d", migrationCount, len(schemaMigrations))
	}

	if err := fixture.DB.Where("config_key = ?", "private_token_ttl_default_ms").Delete(&model.SystemConfig{}).Error; err != nil {
		t.Fatalf("delete canonical token TTL config: %v", err)
	}
	legacyTTL := model.SystemConfig{
		ConfigKey:   "image_token_default_ttl",
		ConfigValue: "1800000",
		ConfigType:  "int",
		Description: "legacy admin key",
	}
	if err := fixture.DB.Create(&legacyTTL).Error; err != nil {
		t.Fatalf("create legacy token TTL config: %v", err)
	}
	runBootstrapDatabase(t, fixture.DB)
	assertSystemConfigValue(t, fixture.DB, "private_token_ttl_default_ms", "1800000")

	setSystemConfigValue(t, fixture.DB, "private_token_ttl_default_ms", "7200000")
	setSystemConfigValue(t, fixture.DB, "site_language", "ja-JP")
	runBootstrapDatabase(t, fixture.DB)

	assertSystemConfigValue(t, fixture.DB, "private_token_ttl_default_ms", "7200000")
	assertSystemConfigValue(t, fixture.DB, "site_language", "ja-JP")
	if err := fixture.DB.Table("schema_migrations").Count(&migrationCount).Error; err != nil {
		t.Fatalf("count schema migrations after repeated bootstrap: %v", err)
	}
	if migrationCount != int64(len(schemaMigrations)) {
		t.Fatalf("schema migration count after repeated bootstrap = %d, want %d", migrationCount, len(schemaMigrations))
	}
}

func TestBootstrapDatabaseMySQLMigratesCompleteLegacyTokens(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	createLegacyAccessTokenTable(t, fixture.SQL, true)
	if _, err := fixture.SQL.Exec(`INSERT INTO image_access_tokens
  (image_id, user_id, token_hash, token_prefix, token_suffix, description, expires_at)
VALUES (1, 1, ?, 'prefix', 'suffix', 'legacy token', DATE_ADD(NOW(3), INTERVAL 1 HOUR))`, strings.Repeat("a", 64)); err != nil {
		t.Fatalf("insert complete legacy access token: %v", err)
	}

	runBootstrapDatabase(t, fixture.DB)

	var tokenCount int64
	if err := fixture.DB.Table("image_access_tokens").Count(&tokenCount).Error; err != nil {
		t.Fatalf("count migrated access tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("migrated access token count = %d, want 0", tokenCount)
	}
	for _, column := range legacyAccessTokenColumns {
		assertColumnExists(t, fixture.SQL, "image_access_tokens", column, false)
	}
	assertColumnExists(t, fixture.SQL, "image_access_tokens", "token", true)
}

func TestBootstrapDatabaseMySQLPreservesModernTokensWithPartialLegacyColumns(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	runBootstrapDatabase(t, fixture.DB)

	user := model.User{
		Username:     "partial-legacy-user",
		Email:        "partial-legacy@example.test",
		PasswordHash: strings.Repeat("p", 60),
	}
	if err := fixture.DB.Create(&user).Error; err != nil {
		t.Fatalf("create token owner: %v", err)
	}
	imageFile := model.ImageFile{
		FileHash:     strings.Repeat("b", 64),
		FileSize:     1024,
		MimeType:     "image/webp",
		Width:        400,
		Height:       400,
		OriginalPath: "users/partial/master.webp",
	}
	if err := fixture.DB.Create(&imageFile).Error; err != nil {
		t.Fatalf("create image file: %v", err)
	}
	image := model.Image{
		UserID:      user.ID,
		ImageFileID: imageFile.ID,
		UniqueLink:  "partial-legacy-image",
		Filename:    "partial.webp",
		Visibility:  "private",
		FileSize:    imageFile.FileSize,
	}
	if err := fixture.DB.Create(&image).Error; err != nil {
		t.Fatalf("create image: %v", err)
	}
	token := model.ImageAccessToken{
		ImageID:   image.ID,
		UserID:    user.ID,
		Token:     strings.Repeat("c", 64),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := fixture.DB.Create(&token).Error; err != nil {
		t.Fatalf("create modern access token: %v", err)
	}
	if _, err := fixture.SQL.Exec(`ALTER TABLE image_access_tokens
  ADD COLUMN token_prefix VARCHAR(16) NULL,
  ADD COLUMN token_suffix VARCHAR(16) NULL,
  ADD COLUMN description VARCHAR(255) NULL`); err != nil {
		t.Fatalf("add partial legacy access token columns: %v", err)
	}

	runBootstrapDatabase(t, fixture.DB)

	var persisted model.ImageAccessToken
	if err := fixture.DB.First(&persisted, token.ID).Error; err != nil {
		t.Fatalf("load modern access token after partial migration: %v", err)
	}
	if persisted.Token != token.Token {
		t.Fatalf("modern access token changed: got %q, want %q", persisted.Token, token.Token)
	}
	for _, column := range []string{"token_prefix", "token_suffix", "description"} {
		assertColumnExists(t, fixture.SQL, "image_access_tokens", column, false)
	}
}

func TestBootstrapDatabaseMySQLRejectsBadLedgerBeforeLegacyMutationAndReleasesLock(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	createLegacyAccessTokenTable(t, fixture.SQL, true)
	if _, err := fixture.SQL.Exec(`INSERT INTO image_access_tokens
  (image_id, user_id, token_hash, token_prefix, token_suffix, description, expires_at)
VALUES (1, 1, ?, 'prefix', 'suffix', 'must survive', DATE_ADD(NOW(3), INTERVAL 1 HOUR))`, strings.Repeat("d", 64)); err != nil {
		t.Fatalf("insert legacy access token: %v", err)
	}
	if _, err := fixture.SQL.Exec(createSchemaMigrationsTableSQL); err != nil {
		t.Fatalf("create schema migration ledger: %v", err)
	}
	firstMigration := schemaMigrations[0]
	if _, err := fixture.SQL.Exec(
		"INSERT INTO schema_migrations (version, name, checksum) VALUES (?, ?, ?)",
		firstMigration.Version,
		firstMigration.Name,
		strings.Repeat("0", 64),
	); err != nil {
		t.Fatalf("insert invalid schema migration record: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := BootstrapDatabase(ctx, fixture.DB)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("BootstrapDatabase() error = %v, want checksum mismatch", err)
	}

	var tokenCount int64
	if err := fixture.DB.Table("image_access_tokens").Count(&tokenCount).Error; err != nil {
		t.Fatalf("count legacy access tokens after rejected ledger: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("legacy access token count after rejected ledger = %d, want 1", tokenCount)
	}
	assertColumnExists(t, fixture.SQL, "image_access_tokens", "token_hash", true)
	assertTableExists(t, fixture.SQL, "users", false)
	assertAdvisoryLockAvailable(t, fixture.DSN, schemaMigrationLockName(fixture.DatabaseName))
}

func TestBootstrapDatabaseMySQLRejectsMissingRecordedObjectBeforeLegacyMutation(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	runBootstrapDatabase(t, fixture.DB)

	if _, err := fixture.SQL.Exec(`ALTER TABLE image_access_tokens
  ADD COLUMN token_hash VARCHAR(64) NULL,
  ADD COLUMN token_prefix VARCHAR(16) NULL`); err != nil {
		t.Fatalf("add legacy access token columns: %v", err)
	}
	if _, err := fixture.SQL.Exec("DROP INDEX idx_images_processing_status ON images"); err != nil {
		t.Fatalf("remove recorded schema object: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := BootstrapDatabase(ctx, fixture.DB)
	if err == nil || !strings.Contains(err.Error(), "recorded but object") {
		t.Fatalf("BootstrapDatabase() error = %v, want missing recorded object", err)
	}
	assertColumnExists(t, fixture.SQL, "image_access_tokens", "token_hash", true)
	assertColumnExists(t, fixture.SQL, "image_access_tokens", "token_prefix", true)
	assertAdvisoryLockAvailable(t, fixture.DSN, schemaMigrationLockName(fixture.DatabaseName))
}

func TestBootstrapDatabaseMySQLHonorsContextWhileWaitingForLock(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	createLegacyAccessTokenTable(t, fixture.SQL, true)
	if _, err := fixture.SQL.Exec(`INSERT INTO image_access_tokens
  (image_id, user_id, token_hash, token_prefix, token_suffix, description, expires_at)
VALUES (1, 1, ?, 'prefix', 'suffix', 'must survive lock wait', DATE_ADD(NOW(3), INTERVAL 1 HOUR))`, strings.Repeat("e", 64)); err != nil {
		t.Fatalf("insert legacy access token: %v", err)
	}

	releaseLock := holdAdvisoryLock(t, fixture.DSN, schemaMigrationLockName(fixture.DatabaseName))
	defer releaseLock()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	start := time.Now()
	err := BootstrapDatabase(ctx, fixture.DB)
	cancel()
	if err == nil {
		t.Fatal("BootstrapDatabase() succeeded while another connection held the advisory lock")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("BootstrapDatabase() error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("BootstrapDatabase() returned after %s, want prompt context cancellation", elapsed)
	}

	var tokenCount int64
	if err := fixture.DB.Table("image_access_tokens").Count(&tokenCount).Error; err != nil {
		t.Fatalf("count legacy access tokens after lock timeout: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("legacy access token count after lock timeout = %d, want 1", tokenCount)
	}
	assertColumnExists(t, fixture.SQL, "image_access_tokens", "token_hash", true)
	assertTableExists(t, fixture.SQL, "schema_migrations", false)
}

func TestBootstrapDatabaseMySQLSerializesIndependentGORMHandles(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	secondDB, secondSQL := openBootstrapGORM(t, fixture.DSN)
	t.Cleanup(func() {
		if err := secondSQL.Close(); err != nil {
			t.Errorf("close second bootstrap connection pool: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	start := make(chan struct{})
	errorsByHandle := make(chan error, 2)
	for _, db := range []*gorm.DB{fixture.DB, secondDB} {
		go func(handle *gorm.DB) {
			<-start
			errorsByHandle <- BootstrapDatabase(ctx, handle)
		}(db)
	}
	close(start)
	var bootstrapErrors []error
	for range 2 {
		if err := <-errorsByHandle; err != nil {
			bootstrapErrors = append(bootstrapErrors, err)
		}
	}
	if len(bootstrapErrors) > 0 {
		t.Fatalf("concurrent BootstrapDatabase() failed: %v", errors.Join(bootstrapErrors...))
	}

	var migrationCount int64
	if err := fixture.DB.Table("schema_migrations").Count(&migrationCount).Error; err != nil {
		t.Fatalf("count migrations after concurrent bootstrap: %v", err)
	}
	if migrationCount != int64(len(schemaMigrations)) {
		t.Fatalf("migration count after concurrent bootstrap = %d, want %d", migrationCount, len(schemaMigrations))
	}
	for _, key := range []string{"private_token_ttl_default_ms", "site_language"} {
		var count int64
		if err := fixture.DB.Model(&model.SystemConfig{}).Where("config_key = ?", key).Count(&count).Error; err != nil {
			t.Fatalf("count default %q after concurrent bootstrap: %v", key, err)
		}
		if count != 1 {
			t.Fatalf("default %q count after concurrent bootstrap = %d, want 1", key, count)
		}
	}
}

func TestBootstrapDatabaseMySQLRecoversWhenDDLExistsWithoutLedgerEntry(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	if err := fixture.DB.AutoMigrate(databaseBootstrapModels()...); err != nil {
		t.Fatalf("create baseline schema: %v", err)
	}

	firstOperation := schemaMigrations[0].Operations[0]
	conn, err := fixture.SQL.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve connection for partial migration setup: %v", err)
	}
	exists, err := schemaMigrationOperationExists(context.Background(), conn, firstOperation)
	if err != nil {
		conn.Close()
		t.Fatalf("inspect partial migration operation: %v", err)
	}
	if !exists {
		if _, err := conn.ExecContext(context.Background(), firstOperation.Statement); err != nil {
			conn.Close()
			t.Fatalf("apply unrecorded migration operation: %v", err)
		}
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("release partial migration setup connection: %v", err)
	}
	assertColumnExists(t, fixture.SQL, firstOperation.TableName, firstOperation.Name, true)
	assertTableExists(t, fixture.SQL, "schema_migrations", false)

	runBootstrapDatabase(t, fixture.DB)

	var migrationCount int64
	if err := fixture.DB.Table("schema_migrations").Count(&migrationCount).Error; err != nil {
		t.Fatalf("count migrations after partial DDL recovery: %v", err)
	}
	if migrationCount != int64(len(schemaMigrations)) {
		t.Fatalf("migration count after partial DDL recovery = %d, want %d", migrationCount, len(schemaMigrations))
	}
	assertColumnExists(t, fixture.SQL, firstOperation.TableName, firstOperation.Name, true)
}

func TestBootstrapDatabaseMySQLPropagatesSeedFailureAndReleasesLock(t *testing.T) {
	fixture := newBootstrapMySQLFixture(t)
	runBootstrapDatabase(t, fixture.DB)

	if _, err := fixture.SQL.Exec("DELETE FROM system_configs"); err != nil {
		t.Fatalf("remove default configs: %v", err)
	}
	if _, err := fixture.SQL.Exec(`CREATE TRIGGER reject_bootstrap_config
BEFORE INSERT ON system_configs
FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced bootstrap seed failure'`); err != nil {
		t.Fatalf("create seed failure trigger: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := BootstrapDatabase(ctx, fixture.DB)
	if err == nil || !strings.Contains(err.Error(), "seed default configuration") {
		t.Fatalf("BootstrapDatabase() error = %v, want seed failure", err)
	}
	var configCount int64
	if err := fixture.DB.Model(&model.SystemConfig{}).Count(&configCount).Error; err != nil {
		t.Fatalf("count configs after seed failure: %v", err)
	}
	if configCount != 0 {
		t.Fatalf("config count after seed failure = %d, want 0", configCount)
	}
	assertAdvisoryLockAvailable(t, fixture.DSN, schemaMigrationLockName(fixture.DatabaseName))
}

func newBootstrapMySQLFixture(t *testing.T) *bootstrapMySQLFixture {
	t.Helper()
	baseDSN := strings.TrimSpace(os.Getenv(bootstrapMySQLDSNEnv))
	if baseDSN == "" {
		if os.Getenv(bootstrapMySQLRequiredEnv) == "1" {
			t.Fatalf("%s must be configured when %s=1", bootstrapMySQLDSNEnv, bootstrapMySQLRequiredEnv)
		}
		t.Skipf("%s is not configured", bootstrapMySQLDSNEnv)
	}

	baseConfig, err := drivermysql.ParseDSN(baseDSN)
	if err != nil {
		t.Fatalf("parse %s: %v", bootstrapMySQLDSNEnv, err)
	}
	baseConfig.DBName = ""
	adminDB, err := sql.Open("mysql", baseConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL administration connection: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		adminDB.Close()
		t.Fatalf("ping MySQL administration connection: %v", err)
	}

	databaseName := randomBootstrapDatabaseName(t)
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci"); err != nil {
		adminDB.Close()
		t.Fatalf("create isolated MySQL database %q: %v", databaseName, err)
	}

	var applicationSQL *sql.DB
	t.Cleanup(func() {
		if applicationSQL != nil {
			if err := applicationSQL.Close(); err != nil {
				t.Errorf("close bootstrap connection pool: %v", err)
			}
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := adminDB.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS `"+databaseName+"`"); err != nil {
			t.Errorf("drop isolated MySQL database %q: %v", databaseName, err)
		}
		if err := adminDB.Close(); err != nil {
			t.Errorf("close MySQL administration connection: %v", err)
		}
	})

	applicationConfig := *baseConfig
	applicationConfig.DBName = databaseName
	applicationConfig.ParseTime = true
	dsn := applicationConfig.FormatDSN()
	db, sqlDB := openBootstrapGORM(t, dsn)
	applicationSQL = sqlDB

	return &bootstrapMySQLFixture{
		DatabaseName: databaseName,
		DSN:          dsn,
		DB:           db,
		SQL:          sqlDB,
	}
}

func openBootstrapGORM(t *testing.T, dsn string) (*gorm.DB, *sql.DB) {
	t.Helper()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open GORM MySQL connection: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("resolve GORM SQL connection pool: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		t.Fatalf("ping GORM MySQL connection: %v", err)
	}
	return db, sqlDB
}

func randomBootstrapDatabaseName(t *testing.T) string {
	t.Helper()
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		t.Fatalf("generate isolated database name: %v", err)
	}
	return "summerain_bootstrap_" + hex.EncodeToString(randomBytes)
}

func runBootstrapDatabase(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := BootstrapDatabase(ctx, db); err != nil {
		t.Fatalf("BootstrapDatabase() failed: %v", err)
	}
}

func createLegacyAccessTokenTable(t *testing.T, db *sql.DB, includeTokenHash bool) {
	t.Helper()
	tokenHashColumn := ""
	if includeTokenHash {
		tokenHashColumn = "token_hash VARCHAR(64) NOT NULL,"
	}
	statement := fmt.Sprintf(`CREATE TABLE image_access_tokens (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  image_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  %s
  token_prefix VARCHAR(16) NULL,
  token_suffix VARCHAR(16) NULL,
  description VARCHAR(255) NULL,
  expires_at DATETIME(3) NOT NULL,
  revoked_at DATETIME(3) NULL,
  usage_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci`, tokenHashColumn)
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("create legacy access token table: %v", err)
	}
}

func setSystemConfigValue(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	result := db.Model(&model.SystemConfig{}).Where("config_key = ?", key).Update("config_value", value)
	if result.Error != nil {
		t.Fatalf("update system config %q: %v", key, result.Error)
	}
	if result.RowsAffected != 1 {
		t.Fatalf("updated system config %q rows = %d, want 1", key, result.RowsAffected)
	}
}

func assertSystemConfigValue(t *testing.T, db *gorm.DB, key, want string) {
	t.Helper()
	var config model.SystemConfig
	if err := db.Where("config_key = ?", key).First(&config).Error; err != nil {
		t.Fatalf("load system config %q: %v", key, err)
	}
	if config.ConfigValue != want {
		t.Fatalf("system config %q = %q, want %q", key, config.ConfigValue, want)
	}
}

func assertColumnExists(t *testing.T, db *sql.DB, tableName, columnName string, want bool) {
	t.Helper()
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, tableName, columnName).Scan(&count); err != nil {
		t.Fatalf("inspect column %s.%s: %v", tableName, columnName, err)
	}
	if got := count > 0; got != want {
		t.Fatalf("column %s.%s exists = %t, want %t", tableName, columnName, got, want)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, tableName string, want bool) {
	t.Helper()
	var count int64
	if err := db.QueryRow(`SELECT COUNT(*)
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`, tableName).Scan(&count); err != nil {
		t.Fatalf("inspect table %s: %v", tableName, err)
	}
	if got := count > 0; got != want {
		t.Fatalf("table %s exists = %t, want %t", tableName, got, want)
	}
}

func assertAdvisoryLockAvailable(t *testing.T, dsn, lockName string) {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open independent lock assertion pool: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve independent lock assertion connection: %v", err)
	}
	defer conn.Close()
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(context.Background(), "SELECT GET_LOCK(?, 0)", lockName).Scan(&acquired); err != nil {
		t.Fatalf("acquire advisory lock after failed bootstrap: %v", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		t.Fatalf("advisory lock %q remained held after failed bootstrap", lockName)
	}
	var released sql.NullInt64
	if err := conn.QueryRowContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
		t.Fatalf("release asserted advisory lock: %v", err)
	}
	if !released.Valid || released.Int64 != 1 {
		t.Fatalf("release asserted advisory lock %q = %v, want 1", lockName, released)
	}
}

func holdAdvisoryLock(t *testing.T, dsn, lockName string) func() {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open advisory lock holder pool: %v", err)
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		t.Fatalf("reserve advisory lock holder connection: %v", err)
	}
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(context.Background(), "SELECT GET_LOCK(?, 0)", lockName).Scan(&acquired); err != nil {
		conn.Close()
		db.Close()
		t.Fatalf("acquire advisory lock for timeout test: %v", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		conn.Close()
		db.Close()
		t.Fatalf("advisory lock %q could not be held for timeout test", lockName)
	}

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released sql.NullInt64
		if err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
			t.Errorf("release timeout-test advisory lock: %v", err)
		} else if !released.Valid || released.Int64 != 1 {
			t.Errorf("release timeout-test advisory lock %q = %v, want 1", lockName, released)
		}
		if err := conn.Close(); err != nil {
			t.Errorf("close advisory lock holder connection: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Errorf("close advisory lock holder pool: %v", err)
		}
	}
}
