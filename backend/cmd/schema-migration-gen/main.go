// Copyright 2026 The summeRain Authors
// SPDX-License-Identifier: Apache-2.0

// Command schema-migration-gen renders the MySQL schema produced by the GORM
// models plus the checksummed migrations into explicit SQL, and verifies that a
// generated file reproduces that schema without drift.
//
// The tool never touches an existing database: every run creates a disposable
// database on the server named by SUMMERAIN_SCHEMA_DSN and drops it again.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/kserksi/summerain/internal/repository"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// dsnEnv names the MySQL server used for disposable schema generation. The
// database name inside the DSN is ignored.
const dsnEnv = "SUMMERAIN_SCHEMA_DSN"

var autoIncrementPattern = regexp.MustCompile(` AUTO_INCREMENT=\d+`)

func main() {
	log.SetFlags(0)
	dsn := strings.TrimSpace(os.Getenv(dsnEnv))
	if dsn == "" {
		log.Fatalf("%s must point at a MySQL server", dsnEnv)
	}

	ctx := context.Background()
	args := os.Args[1:]
	mode := "dump"
	if len(args) > 0 {
		mode = args[0]
	}

	switch mode {
	case "dump":
		if err := runDump(ctx, dsn); err != nil {
			log.Fatalf("dump schema: %v", err)
		}
	case "verify":
		if len(args) < 2 {
			log.Fatalf("usage: schema-migration-gen verify <file>")
		}
		if err := verifySchemaFile(ctx, dsn, args[1]); err != nil {
			log.Fatalf("verify schema: %v", err)
		}
		log.Printf("%s reproduces the bootstrap schema", args[1])
	default:
		log.Fatalf("unknown mode %q (want dump or verify)", mode)
	}
}

func runDump(ctx context.Context, dsn string) error {
	database, cleanup, err := scratchDatabase(ctx, dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := repository.BootstrapDatabase(ctx, database); err != nil {
		return fmt.Errorf("bootstrap scratch schema: %w", err)
	}
	dump, err := dumpSchema(ctx, database)
	if err != nil {
		return err
	}
	// Tables are emitted in name order, so a referencing table can precede
	// the table it points at. Disabling foreign key checks makes the file
	// runnable with the mysql client exactly as stored.
	if _, err := os.Stdout.WriteString("SET FOREIGN_KEY_CHECKS=0;\n\n"); err != nil {
		return err
	}
	if _, err := os.Stdout.WriteString(dump); err != nil {
		return err
	}
	_, err = os.Stdout.WriteString("SET FOREIGN_KEY_CHECKS=1;\n")
	return err
}

// scratchDatabase creates a disposable database on the DSN server and returns a
// handle to it. The returned cleanup drops the database again.
func scratchDatabase(ctx context.Context, dsn string) (*gorm.DB, func(), error) {
	config, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", dsnEnv, err)
	}
	config.DBName = ""
	admin, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		return nil, nil, fmt.Errorf("open admin connection: %w", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		admin.Close()
		return nil, nil, fmt.Errorf("ping %s: %w", dsnEnv, err)
	}

	name := fmt.Sprintf("summerain_schema_gen_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx,
		"CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
	); err != nil {
		admin.Close()
		return nil, nil, fmt.Errorf("create scratch database: %w", err)
	}
	cleanup := func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`"); err != nil {
			log.Printf("drop scratch database %s: %v", name, err)
		}
		admin.Close()
	}

	config.DBName = name
	database, err := gorm.Open(gormmysql.Open(config.FormatDSN()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("open scratch database: %w", err)
	}
	return database, cleanup, nil
}

// dumpSchema renders every base table of the current database in name order.
func dumpSchema(ctx context.Context, database *gorm.DB) (string, error) {
	var tables []string
	if err := database.WithContext(ctx).Raw(
		"SELECT `table_name` FROM `information_schema`.`tables` WHERE `table_schema` = DATABASE() AND `table_type` = 'BASE TABLE' ORDER BY `table_name`",
	).Scan(&tables).Error; err != nil {
		return "", fmt.Errorf("list tables: %w", err)
	}
	if len(tables) == 0 {
		return "", errors.New("no tables found")
	}
	var builder strings.Builder
	for _, table := range tables {
		var name, statement string
		row := database.WithContext(ctx).Raw("SHOW CREATE TABLE `" + table + "`").Row()
		if err := row.Scan(&name, &statement); err != nil {
			return "", fmt.Errorf("show create table %s: %w", table, err)
		}
		builder.WriteString(normalizeCreateTable(statement))
		builder.WriteString(";\n\n")
	}
	return builder.String(), nil
}

// normalizeCreateTable removes volatile table options so that two dumps of the
// same schema compare equal regardless of inserted rows.
func normalizeCreateTable(statement string) string {
	return strings.TrimSpace(autoIncrementPattern.ReplaceAllString(statement, ""))
}

// verifySchemaFile applies a generated file to a disposable database and proves
// that the running bootstrap leaves the schema unchanged afterwards. Equality
// means the file is complete and matches the models plus migrations.
func verifySchemaFile(ctx context.Context, dsn, file string) error {
	script, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	statements, err := splitStatements(string(script))
	if err != nil {
		return fmt.Errorf("parse %s: %w", file, err)
	}
	if len(statements) == 0 {
		return fmt.Errorf("%s contains no SQL statements", file)
	}

	database, cleanup, err := scratchDatabase(ctx, dsn)
	if err != nil {
		return err
	}
	defer cleanup()

	sqlDB, err := database.DB()
	if err != nil {
		return fmt.Errorf("sql handle: %w", err)
	}
	// One pinned connection keeps script-scoped settings such as
	// FOREIGN_KEY_CHECKS in effect for every statement.
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("pin connection: %w", err)
	}
	defer conn.Close()
	for index, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply statement %d of %s: %w", index+1, file, err)
		}
	}

	before, err := dumpSchema(ctx, database)
	if err != nil {
		return err
	}
	if err := repository.BootstrapDatabase(ctx, database); err != nil {
		return fmt.Errorf("bootstrap after applying %s: %w", file, err)
	}
	after, err := dumpSchema(ctx, database)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("%s does not reproduce the bootstrap schema: %s", file, firstDifference(before, after))
	}
	return nil
}

// splitStatements splits a generated script into statements terminated by a
// line-ending semicolon. Generated files contain only CREATE TABLE statements,
// so this deliberately simple parser is sufficient.
func splitStatements(script string) ([]string, error) {
	var statements []string
	var current strings.Builder
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		current.WriteString(line)
		current.WriteString("\n")
		if strings.HasSuffix(trimmed, ";") {
			statements = append(statements, strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(current.String()), ";")))
			current.Reset()
		}
	}
	if strings.TrimSpace(current.String()) != "" {
		return nil, errors.New("unterminated statement")
	}
	return statements, nil
}

// firstDifference reports the first differing line between two schema dumps.
func firstDifference(expected, actual string) string {
	expectedLines := strings.Split(expected, "\n")
	actualLines := strings.Split(actual, "\n")
	for index := 0; index < len(expectedLines) || index < len(actualLines); index++ {
		left, right := "", ""
		if index < len(expectedLines) {
			left = expectedLines[index]
		}
		if index < len(actualLines) {
			right = actualLines[index]
		}
		if left != right {
			return fmt.Sprintf("line %d:\n  file:      %s\n  bootstrap: %s", index+1, left, right)
		}
	}
	return "schemas differ"
}
