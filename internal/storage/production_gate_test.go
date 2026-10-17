package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const upstreamFixtureKey = "sk-fixture-redacted"

func tableRowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// assertPrivateBackupMode checks POSIX permission bits where Go and the OS
// expose them. Windows file mode bits do not represent NTFS ACLs; on Windows
// the migration tests still verify that the backup exists and can be opened.
func assertPrivateBackupMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Log("Windows backup access is governed by NTFS ACLs; FileMode.Perm is not a POSIX permission check")
		return
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("backup permissions = %04o, want 0600", got)
	}
}

func verifyFixtureRows(t *testing.T, db *sql.DB) {
	t.Helper()
	for table, want := range map[string]int{
		"providerConnections": 1,
		"providerNodes": 1,
		"proxyPools": 1,
		"apiKeys": 1,
		"combos": 1,
		"kv": 2,
		"usageHistory": 1,
		"usageDaily": 1,
		"requestDetails": 1,
		"extension_fixture": 1,
	} {
		if got := tableRowCount(t, db, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}

	var settingData string
	if err := db.QueryRow("SELECT data FROM settings WHERE id=1").Scan(&settingData); err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(settingData), &settings); err != nil {
		t.Fatal(err)
	}
	if settings["fixtureMarker"] != "preserve-settings" {
		t.Errorf("settings marker changed: %#v", settings["fixtureMarker"])
	}

	var connectionData, usageMeta, requestData, extensionPayload string
	if err := db.QueryRow("SELECT data FROM providerConnections WHERE id='fixture-connection'").Scan(&connectionData); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT meta FROM usageHistory WHERE id=1").Scan(&usageMeta); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT data FROM requestDetails WHERE id='fixture-request'").Scan(&requestData); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT payload FROM extension_fixture WHERE id='extension-row'").Scan(&extensionPayload); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"provider connection": connectionData,
		"usage metadata": usageMeta,
		"request detail": requestData,
	} {
		if !strings.Contains(value, "preserve-") {
			t.Errorf("%s payload was not preserved: %s", name, value)
		}
	}
	if extensionPayload != "keep-me" {
		t.Errorf("unrelated extension data changed: %q", extensionPayload)
	}
	var apiKey string
	if err := db.QueryRow("SELECT key FROM apiKeys WHERE id='fixture-key-id'").Scan(&apiKey); err != nil {
		t.Fatal(err)
	}
	if apiKey != upstreamFixtureKey {
		t.Errorf("API key row changed: %q", apiKey)
	}
}

func TestUpstreamShapedSchemaFixtureMigrationBackupAndRestore(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	dataDir := filepath.Join(t.TempDir(), "source")
	dbPath := filepath.Join(dataDir, "db", "data.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("testdata", "9router-schema-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(fixture)); err != nil {
		_ = db.Close()
		t.Fatalf("load sanitized schema fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Opening a v1 fixture should back up before adding the v2 key-access columns.
	store, err := Open(dataDir)
	if err != nil {
		t.Fatalf("migrate upstream-shaped fixture: %v", err)
	}
	verifyFixtureRows(t, store.DB)
	var schemaVersion string
	if err := store.DB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != schemaVersionExpectedForTest {
		t.Fatalf("schemaVersion after migration = %q", schemaVersion)
	}
	var restricted int
	if err := store.DB.QueryRow("SELECT accessRestricted FROM apiKeys WHERE id='fixture-key-id'").Scan(&restricted); err != nil {
		t.Fatal(err)
	}
	if restricted != 0 {
		t.Fatalf("existing API key should default to unrestricted after additive migration, got %d", restricted)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(dataDir, "db", "backups")
	backups, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Fatal("migration did not create a pre-change backup")
	}
	backupPath := filepath.Join(backupDir, backups[0].Name())
	assertPrivateBackupMode(t, backupPath)

	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	verifyFixtureRows(t, backupDB)
	if hasColumn, err := columnExists(context.Background(), backupDB, "apiKeys", "accessRestricted"); err != nil {
		t.Fatal(err)
	} else if hasColumn {
		t.Fatal("pre-migration backup unexpectedly contains the new access column")
	}
	if err := backupDB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != "1" {
		t.Fatalf("backup schemaVersion = %q, want original version 1", schemaVersion)
	}
	if err := backupDB.Close(); err != nil {
		t.Fatal(err)
	}

	// Restore the snapshot into an isolated DATA_DIR and repeat startup migration.
	restoreDir := filepath.Join(t.TempDir(), "restore")
	restorePath := filepath.Join(restoreDir, "db", "data.sqlite")
	if err := os.MkdirAll(filepath.Dir(restorePath), 0o700); err != nil {
		t.Fatal(err)
	}
	backupBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(restorePath, backupBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(restoreDir)
	if err != nil {
		t.Fatalf("open restored backup: %v", err)
	}
	defer restored.Close()
	verifyFixtureRows(t, restored.DB)
	if err := restored.DB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != schemaVersionExpectedForTest {
		t.Fatalf("restored schemaVersion = %q, want %s", schemaVersion, schemaVersionExpectedForTest)
	}
}

const schemaVersionExpectedForTest = "2"

func TestSchemaV1WithAccessColumnsStillBacksUpAndPreservesData(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	dataDir := filepath.Join(t.TempDir(), "source")
	dbPath := filepath.Join(dataDir, "db", "data.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("testdata", "9router-schema-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(fixture)); err != nil {
		_ = db.Close()
		t.Fatalf("load sanitized schema fixture: %v", err)
	}

	// Match a real 9Router database where additive API-key columns already
	// exist even though _meta.schemaVersion has not yet been advanced.
	if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN accessRestricted INTEGER DEFAULT 0"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE apiKeys ADD COLUMN accessAllow TEXT"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE _meta SET value='1' WHERE key='schemaVersion'"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dataDir)
	if err != nil {
		t.Fatalf("migrate schema-v1 database with current columns: %v", err)
	}
	verifyFixtureRows(t, store.DB)
	var schemaVersion string
	if err := store.DB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != schemaVersionExpectedForTest {
		t.Fatalf("schemaVersion after migration = %q, want %s", schemaVersion, schemaVersionExpectedForTest)
	}
	var restricted int
	var allow sql.NullString
	if err := store.DB.QueryRow("SELECT accessRestricted, accessAllow FROM apiKeys WHERE id='fixture-key-id'").Scan(&restricted, &allow); err != nil {
		t.Fatal(err)
	}
	if restricted != 0 || allow.Valid {
		t.Fatalf("existing key access settings changed: restricted=%d allowPresent=%v", restricted, allow.Valid)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	backupDir := filepath.Join(dataDir, "db", "backups")
	backups, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) == 0 {
		t.Fatal("schema-version reconciliation did not create a pre-change backup")
	}
	backupDB, err := sql.Open("sqlite", filepath.Join(backupDir, backups[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	verifyFixtureRows(t, backupDB)
	for _, column := range []string{"accessRestricted", "accessAllow"} {
		if exists, err := columnExists(context.Background(), backupDB, "apiKeys", column); err != nil {
			t.Fatal(err)
		} else if !exists {
			t.Fatalf("pre-migration backup lost existing column %s", column)
		}
	}
	if err := backupDB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != "1" {
		t.Fatalf("backup schemaVersion = %q, want original version 1", schemaVersion)
	}
}

func TestConcurrentUsageWritesKeepHistoryAndDailyRollupsConsistent(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	store, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const workers = 8
	const writesPerWorker = 24
	const total = workers * writesPerWorker
	base := time.Now().UTC().Truncate(time.Minute)
	errCh := make(chan error, total)
	var wg sync.WaitGroup

	// A reader concurrently checks bounded recent history while writers commit.
	readerDone := make(chan struct{})
	readerErr := make(chan error, 1)
	go func() {
		defer close(readerDone)
		for i := 0; i < 25; i++ {
			if _, err := store.GetUsageHistory(context.Background(), UsageFilter{Limit: 25}); err != nil {
				select { case readerErr <- err: default: }
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < writesPerWorker; i++ {
				n := worker*writesPerWorker + i
				entry := UsageEntry{
					Timestamp: base.Add(time.Duration(n)*time.Millisecond).Format(time.RFC3339Nano),
					Provider: "opencode",
					Model: fmt.Sprintf("oc/stress-%02d", worker),
					APIKey: "stress-key",
					Endpoint: "/v1/chat/completions",
					Status: "200",
					Cost: 0.001,
					Tokens: map[string]any{"prompt_tokens": int64(10), "completion_tokens": int64(3)},
				}
				if err := store.SaveRequestUsage(context.Background(), entry); err != nil {
					errCh <- fmt.Errorf("writer %d item %d: %w", worker, i, err)
				}
			}
		}(worker)
	}
	wg.Wait()
	<-readerDone
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	select {
	case err := <-readerErr:
		t.Fatalf("concurrent history read failed: %v", err)
	default:
	}

	if got := tableRowCount(t, store.DB, "usageHistory"); got != total {
		t.Fatalf("history rows = %d, want %d", got, total)
	}
	var totalLifetime string
	if err := store.DB.QueryRow("SELECT value FROM _meta WHERE key='totalRequestsLifetime'").Scan(&totalLifetime); err != nil {
		t.Fatal(err)
	}
	if totalLifetime != fmt.Sprint(total) {
		t.Fatalf("lifetime counter = %q, want %d", totalLifetime, total)
	}
	var dailyJSON string
	if err := store.DB.QueryRow("SELECT data FROM usageDaily WHERE dateKey=?", localDateKey(base.Format(time.RFC3339Nano))).Scan(&dailyJSON); err != nil {
		t.Fatal(err)
	}
	var daily map[string]any
	if err := json.Unmarshal([]byte(dailyJSON), &daily); err != nil {
		t.Fatal(err)
	}
	if got := numberValue(daily["requests"]); got != total {
		t.Fatalf("daily rollup requests = %v, want %d", got, total)
	}
	stats, err := store.GetUsageStats(context.Background(), "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := numberValue(stats["totalRequests"]); got != total {
		t.Fatalf("usage stats totalRequests = %v, want %d", got, total)
	}
	if _, err := store.DB.Exec("PRAGMA integrity_check"); err != nil {
		t.Fatalf("SQLite integrity check query: %v", err)
	}
}

func TestUsageHistoryHighCardinalityHonorsLimitAndTimestampIndex(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	store, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const rowsToInsert = 5000
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tx, err := store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO usageHistory(timestamp,provider,model,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	for i := 0; i < rowsToInsert; i++ {
		stamp := dbTimestamp(base.Add(time.Duration(i) * time.Second))
		tokens, _ := json.Marshal(map[string]int{"prompt_tokens": 1, "completion_tokens": 2})
		if _, err := stmt.Exec(stamp, "opencode", fmt.Sprintf("oc/model-%04d", i%100), fmt.Sprintf("synthetic-key-%04d", i), "/v1/chat/completions", 1, 2, 0.001, "200", string(tokens), "{}"); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert history row %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	history, err := store.GetUsageHistory(context.Background(), UsageFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 100 {
		t.Fatalf("bounded history returned %d rows, want 100", len(history))
	}
	if got, want := history[0]["timestamp"], dbTimestamp(base.Add(time.Duration(rowsToInsert-1)*time.Second)); got != want {
		t.Fatalf("latest history timestamp = %v, want %s", got, want)
	}
	if got, want := history[len(history)-1]["timestamp"], dbTimestamp(base.Add(time.Duration(rowsToInsert-100)*time.Second)); got != want {
		t.Fatalf("oldest bounded history timestamp = %v, want %s", got, want)
	}

	planRows, err := store.DB.Query("EXPLAIN QUERY PLAN SELECT timestamp,provider,model FROM usageHistory ORDER BY timestamp DESC LIMIT 100")
	if err != nil {
		t.Fatal(err)
	}
	defer planRows.Close()
	usesIndex := false
	for planRows.Next() {
		var id, parent, notused int
		var detail string
		if err := planRows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "idx_uh_ts") {
			usesIndex = true
		}
	}
	if err := planRows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usesIndex {
		t.Fatal("high-cardinality recent-history query did not use idx_uh_ts")
	}
	var integrity string
	if err := store.DB.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("SQLite integrity_check = %q", integrity)
	}
}


func TestSanitizedSuppliedDatabaseShapeMigrationPreservesSettingsUsageAndKey(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	dataDir := filepath.Join(t.TempDir(), "data")
	dbPath := filepath.Join(dataDir, "db", "data.sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("testdata", "supplied-db-shape-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rawDB.Exec(string(fixture)); err != nil {
		_ = rawDB.Close()
		t.Fatalf("load sanitized supplied-database shape: %v", err)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dataDir)
	if err != nil {
		t.Fatalf("open and migrate sanitized database shape: %v", err)
	}
	defer store.Close()

	for table, want := range map[string]int{
		"settings": 1, "apiKeys": 1, "usageHistory": 3, "usageDaily": 1,
		"providerConnections": 0, "providerNodes": 0, "proxyPools": 0,
		"combos": 0, "kv": 0, "requestDetails": 0,
	} {
		if got := tableRowCount(t, store.DB, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}

	var version string
	if err := store.DB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersionExpectedForTest {
		t.Fatalf("schemaVersion after migration = %q, want %s", version, schemaVersionExpectedForTest)
	}

	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings["fixtureMarker"] != "preserve-sanitized-settings" || settings["rtkEnabled"] != true {
		t.Fatalf("settings were not preserved through migration: marker=%v rtkEnabled=%v", settings["fixtureMarker"], settings["rtkEnabled"])
	}

	const fixtureKey = "sk-blackhand-fixture-key-never-use"
	key, err := store.GetAPIKeyByKey(context.Background(), fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	if key == nil || !key.IsActive || key.Key != fixtureKey || key.Access.Restricted {
		t.Fatalf("active unrestricted fixture API key did not survive migration: %#v", key)
	}

	history, err := store.GetUsageHistory(context.Background(), UsageFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("usage history returned %d rows, want 3", len(history))
	}
	for _, row := range history {
		if strings.Contains(fmt.Sprint(row["apiKeyMasked"]), fixtureKey) {
			t.Fatal("usage history leaked the raw fixture key")
		}
	}
	stats, err := store.GetUsageStats(context.Background(), "all")
	if err != nil {
		t.Fatal(err)
	}
	if got := numberValue(stats["totalRequests"]); got != 3 {
		t.Fatalf("usage stats totalRequests = %v, want 3", got)
	}
	if got := numberValue(stats["totalPromptTokens"]); got != 1496603 {
		t.Fatalf("usage stats prompt tokens = %v, want 1496603", got)
	}
	if got := numberValue(stats["totalCompletionTokens"]); got != 3104 {
		t.Fatalf("usage stats completion tokens = %v, want 3104", got)
	}
	if got := numberValue(stats["totalCachedTokens"]); got != 588416 {
		t.Fatalf("usage stats cached tokens = %v, want 588416", got)
	}
	if got := numberValue(stats["totalCost"]); got < 0.1302586 || got > 0.1302588 {
		t.Fatalf("usage stats cost = %.10f, want approximately 0.1302587048", got)
	}

	backupDir := filepath.Join(dataDir, "db", "backups")
	backups, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("backup count = %d, want exactly one pre-migration backup", len(backups))
	}
	backupPath := filepath.Join(backupDir, backups[0].Name())
	assertPrivateBackupMode(t, backupPath)
	backupDB, err := sql.Open("sqlite", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	if err := backupDB.QueryRow("SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "1" {
		t.Fatalf("backup schemaVersion = %q, want original version 1", version)
	}
	for table, want := range map[string]int{"settings": 1, "apiKeys": 1, "usageHistory": 3, "usageDaily": 1} {
		if got := tableRowCount(t, backupDB, table); got != want {
			t.Errorf("pre-migration backup %s rows = %d, want %d", table, got, want)
		}
	}
	var rawSettings string
	if err := backupDB.QueryRow("SELECT data FROM settings WHERE id=1").Scan(&rawSettings); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawSettings, "preserve-sanitized-settings") {
		t.Fatal("pre-migration backup lost the settings fixture marker")
	}
	if got := tableRowCount(t, backupDB, "usageHistory"); got != 3 {
		t.Fatalf("pre-migration backup usage history = %d, want 3", got)
	}
}
