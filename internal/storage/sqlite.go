package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database shared by API handlers and the request pipeline.
// Its schema intentionally mirrors 9Router's src/lib/db/schema.js, so an existing
// DATA_DIR/db/data.sqlite can be reused without a destructive conversion.
type Store struct {
	DB      *sql.DB
	DataDir string
}

const schemaVersion = "2"

func Open(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "./data"
	}
	for _, dir := range []string{
		dataDir,
		filepath.Join(dataDir, "db"),
		filepath.Join(dataDir, "db", "backups"),
		filepath.Join(dataDir, "auth"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create data directory %q: %w", dir, err)
		}
	}

	dbPath := filepath.Join(dataDir, "db", "data.sqlite")
	existed := fileExists(dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(0)

	closeOnError := func(err error) (*Store, error) {
		_ = db.Close()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return closeOnError(fmt.Errorf("ping SQLite: %w", err))
	}

	// These settings match 9Router's SQLite defaults. WAL permits concurrent
	// readers while write transactions remain short and explicit.
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 30000000",
		"PRAGMA cache_size = -64000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return closeOnError(fmt.Errorf("configure SQLite (%s): %w", pragma, err))
		}
	}

	// Backup once before the first schema reconciliation that may rename or add
	// objects. A healthy database with schemaVersion=2 should not create a backup
	// on every startup.
	if existed {
		needsBackup, err := schemaNeedsRepair(ctx, db)
		if err != nil { return closeOnError(fmt.Errorf("inspect existing schema: %w", err)) }
		if needsBackup {
			if err := backupBeforeSchemaRepair(ctx, db, dataDir); err != nil {
				return closeOnError(fmt.Errorf("backup before schema repair: %w", err))
			}
		}
	}
	if err := initSchema(ctx, db); err != nil {
		return closeOnError(err)
	}
	store := &Store{DB: db, DataDir: dataDir}
	if err := store.migrateLegacyScaffold(ctx); err != nil {
		return closeOnError(err)
	}
	if err := store.EnsureConfiguredAPIKey(ctx); err != nil {
		return closeOnError(fmt.Errorf("reconcile environment API key: %w", err))
	}
	return store, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func tableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
	return count > 0, err
}

func columnExists(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil { return false, err }
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil { return false, err }
		if name == column { return true, nil }
	}
	return false, rows.Err()
}


func schemaNeedsRepair(ctx context.Context, db *sql.DB) (bool, error) {
	var version string
	err := db.QueryRowContext(ctx, "SELECT value FROM _meta WHERE key='schemaVersion'").Scan(&version)
	if err == nil && version != schemaVersion { return true, nil }
	if err != nil && err != sql.ErrNoRows {
		exists, tableErr := tableExists(ctx, db, "_meta")
		if tableErr != nil { return false, tableErr }
		if exists { return false, err }
	}
	required := map[string][]string{
		"_meta":{"key","value"},
		"settings":{"id","data"},
		"providerConnections":{"id","provider","authType","isActive","data","createdAt","updatedAt"},
		"providerNodes":{"id","type","name","data","createdAt","updatedAt"},
		"proxyPools":{"id","isActive","testStatus","data","createdAt","updatedAt"},
		"apiKeys":{"id","key","name","machineId","isActive","createdAt","accessRestricted","accessAllow"},
		"combos":{"id","name","kind","models","createdAt","updatedAt"},
		"kv":{"scope","key","value"},
		"usageHistory":{"id","timestamp","provider","model","connectionId","apiKey","endpoint","promptTokens","completionTokens","cost","status","tokens","meta"},
		"usageDaily":{"dateKey","data"},
		"requestDetails":{"id","timestamp","provider","model","connectionId","status","data"},
	}
	for table, columns := range required {
		exists, err := tableExists(ctx, db, table)
		if err != nil { return false, err }
		if !exists { return true, nil }
		for _, column := range columns {
			exists, err := columnExists(ctx, db, table, column)
			if err != nil { return false, err }
			if !exists { return true, nil }
		}
	}
	return false, nil
}

func backupBeforeSchemaRepair(ctx context.Context, db *sql.DB, dataDir string) error {
	// The backup is cheap and never overwritten. VACUUM INTO includes committed
	// WAL state, unlike copying data.sqlite while the WAL is active.
	backupPath := filepath.Join(dataDir, "db", "backups", "pre-schema-"+time.Now().UTC().Format("20060102T150405.000000000")+".sqlite")
	escaped := strings.ReplaceAll(backupPath, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return err
	}
	return os.Chmod(backupPath, 0o600)
}

type tableDefinition struct {
	name string
	create string
	columns map[string]string
	indexes []string
}

func initSchema(ctx context.Context, db *sql.DB) error {
	// Blackhand's first scaffold briefly used a key/value settings table. Keep
	// those rows intact under a legacy name before establishing the 9Router
	// compatible settings(id=1,data) table.
	settingsExists, err := tableExists(ctx, db, "settings")
	if err != nil { return err }
	if settingsExists {
		hasID, err := columnExists(ctx, db, "settings", "id")
		if err != nil { return err }
		hasData, err := columnExists(ctx, db, "settings", "data")
		if err != nil { return err }
		if !hasID || !hasData {
			legacyExists, err := tableExists(ctx, db, "blackhand_legacy_settings")
			if err != nil { return err }
			if legacyExists {
				return errors.New("both incompatible settings table and blackhand_legacy_settings exist; refusing to rename or discard either")
			}
			if _, err := db.ExecContext(ctx, "ALTER TABLE settings RENAME TO blackhand_legacy_settings"); err != nil {
				return fmt.Errorf("preserve early Blackhand settings table: %w", err)
			}
		}
	}

	tables := []tableDefinition{
		{name:"_meta", create:`CREATE TABLE IF NOT EXISTS _meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`},
		{name:"settings", create:`CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL)`},
		{name:"providerConnections", create:`CREATE TABLE IF NOT EXISTS providerConnections (id TEXT PRIMARY KEY, provider TEXT NOT NULL, authType TEXT NOT NULL, name TEXT, email TEXT, priority INTEGER, isActive INTEGER DEFAULT 1, data TEXT NOT NULL, createdAt TEXT NOT NULL, updatedAt TEXT NOT NULL)`, columns:map[string]string{"id":"TEXT","provider":"TEXT","authType":"TEXT","name":"TEXT","email":"TEXT","priority":"INTEGER","isActive":"INTEGER DEFAULT 1","data":"TEXT NOT NULL DEFAULT '{}'","createdAt":"TEXT NOT NULL DEFAULT ''","updatedAt":"TEXT NOT NULL DEFAULT ''"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_pc_provider ON providerConnections(provider)","CREATE INDEX IF NOT EXISTS idx_pc_provider_active ON providerConnections(provider, isActive)","CREATE INDEX IF NOT EXISTS idx_pc_priority ON providerConnections(provider, priority)"}},
		{name:"providerNodes", create:`CREATE TABLE IF NOT EXISTS providerNodes (id TEXT PRIMARY KEY, type TEXT, name TEXT, data TEXT NOT NULL, createdAt TEXT NOT NULL, updatedAt TEXT NOT NULL)`, columns:map[string]string{"id":"TEXT","type":"TEXT","name":"TEXT","data":"TEXT NOT NULL DEFAULT '{}'","createdAt":"TEXT NOT NULL DEFAULT ''","updatedAt":"TEXT NOT NULL DEFAULT ''"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_pn_type ON providerNodes(type)"}},
		{name:"proxyPools", create:`CREATE TABLE IF NOT EXISTS proxyPools (id TEXT PRIMARY KEY, isActive INTEGER DEFAULT 1, testStatus TEXT, data TEXT NOT NULL, createdAt TEXT NOT NULL, updatedAt TEXT NOT NULL)`, columns:map[string]string{"id":"TEXT","isActive":"INTEGER DEFAULT 1","testStatus":"TEXT","data":"TEXT NOT NULL DEFAULT '{}'","createdAt":"TEXT NOT NULL DEFAULT ''","updatedAt":"TEXT NOT NULL DEFAULT ''"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_pp_active ON proxyPools(isActive)","CREATE INDEX IF NOT EXISTS idx_pp_status ON proxyPools(testStatus)"}},
		{name:"apiKeys", create:`CREATE TABLE IF NOT EXISTS apiKeys (id TEXT PRIMARY KEY, key TEXT UNIQUE NOT NULL, name TEXT, machineId TEXT, isActive INTEGER DEFAULT 1, createdAt TEXT NOT NULL, accessRestricted INTEGER DEFAULT 0, accessAllow TEXT)`, columns:map[string]string{"id":"TEXT","key":"TEXT","name":"TEXT","machineId":"TEXT","isActive":"INTEGER DEFAULT 1","createdAt":"TEXT NOT NULL DEFAULT ''","accessRestricted":"INTEGER DEFAULT 0","accessAllow":"TEXT"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_ak_key ON apiKeys(key)"}},
		{name:"combos", create:`CREATE TABLE IF NOT EXISTS combos (id TEXT PRIMARY KEY, name TEXT UNIQUE NOT NULL, kind TEXT, models TEXT NOT NULL, createdAt TEXT NOT NULL, updatedAt TEXT NOT NULL)`, columns:map[string]string{"id":"TEXT","name":"TEXT","kind":"TEXT","models":"TEXT NOT NULL DEFAULT '[]'","createdAt":"TEXT NOT NULL DEFAULT ''","updatedAt":"TEXT NOT NULL DEFAULT ''"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_combo_name ON combos(name)"}},
		{name:"kv", create:`CREATE TABLE IF NOT EXISTS kv (scope TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(scope, key))`, columns:map[string]string{"scope":"TEXT NOT NULL DEFAULT ''","key":"TEXT NOT NULL DEFAULT ''","value":"TEXT NOT NULL DEFAULT 'null'"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_kv_scope ON kv(scope)"}},
		{name:"usageHistory", create:`CREATE TABLE IF NOT EXISTS usageHistory (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp TEXT NOT NULL, provider TEXT, model TEXT, connectionId TEXT, apiKey TEXT, endpoint TEXT, promptTokens INTEGER DEFAULT 0, completionTokens INTEGER DEFAULT 0, cost REAL DEFAULT 0, status TEXT, tokens TEXT, meta TEXT)`, columns:map[string]string{"timestamp":"TEXT NOT NULL DEFAULT ''","provider":"TEXT","model":"TEXT","connectionId":"TEXT","apiKey":"TEXT","endpoint":"TEXT","promptTokens":"INTEGER DEFAULT 0","completionTokens":"INTEGER DEFAULT 0","cost":"REAL DEFAULT 0","status":"TEXT","tokens":"TEXT","meta":"TEXT"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_uh_ts ON usageHistory(timestamp DESC)","CREATE INDEX IF NOT EXISTS idx_uh_provider ON usageHistory(provider)","CREATE INDEX IF NOT EXISTS idx_uh_model ON usageHistory(model)","CREATE INDEX IF NOT EXISTS idx_uh_conn ON usageHistory(connectionId)"}},
		{name:"usageDaily", create:`CREATE TABLE IF NOT EXISTS usageDaily (dateKey TEXT PRIMARY KEY, data TEXT NOT NULL)`, columns:map[string]string{"dateKey":"TEXT","data":"TEXT NOT NULL DEFAULT '{}'"}},
		{name:"requestDetails", create:`CREATE TABLE IF NOT EXISTS requestDetails (id TEXT PRIMARY KEY, timestamp TEXT NOT NULL, provider TEXT, model TEXT, connectionId TEXT, status TEXT, data TEXT NOT NULL)`, columns:map[string]string{"id":"TEXT","timestamp":"TEXT NOT NULL DEFAULT ''","provider":"TEXT","model":"TEXT","connectionId":"TEXT","status":"TEXT","data":"TEXT NOT NULL DEFAULT '{}'"}, indexes:[]string{"CREATE INDEX IF NOT EXISTS idx_rd_ts ON requestDetails(timestamp DESC)","CREATE INDEX IF NOT EXISTS idx_rd_provider ON requestDetails(provider)","CREATE INDEX IF NOT EXISTS idx_rd_model ON requestDetails(model)","CREATE INDEX IF NOT EXISTS idx_rd_conn ON requestDetails(connectionId)"}},
	}

	for _, table := range tables {
		if _, err := db.ExecContext(ctx, table.create); err != nil {
			return fmt.Errorf("create %s table: %w", table.name, err)
		}
		for col, definition := range table.columns {
			exists, err := columnExists(ctx, db, table.name, col)
			if err != nil { return fmt.Errorf("inspect %s.%s: %w", table.name, col, err) }
			if !exists {
				if _, err := db.ExecContext(ctx, "ALTER TABLE "+table.name+" ADD COLUMN "+col+" "+definition); err != nil {
					return fmt.Errorf("add %s.%s: %w", table.name, col, err)
				}
			}
		}
		for _, indexSQL := range table.indexes {
			if _, err := db.ExecContext(ctx, indexSQL); err != nil {
				return fmt.Errorf("create index for %s: %w", table.name, err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO _meta(key,value) VALUES('schemaVersion',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", schemaVersion); err != nil {
		return fmt.Errorf("stamp schema version: %w", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO _meta(key,value) VALUES('migrationVersion','1') ON CONFLICT(key) DO UPDATE SET value=excluded.value"); err != nil {
		return fmt.Errorf("stamp migration version: %w", err)
	}
	return nil
}

func (s *Store) migrateLegacyScaffold(ctx context.Context) error {
	if err := s.importLegacySettings(ctx); err != nil { return err }
	if err := s.importLegacyProxyPools(ctx); err != nil { return err }
	if err := s.importLegacyUsage(ctx); err != nil { return err }
	return nil
}

func (s *Store) legacyImported(ctx context.Context, key string) (bool, error) {
	var value string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM _meta WHERE key=?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) { return false, nil }
	return value == "1", err
}

func (s *Store) setLegacyImported(ctx context.Context, key string) error {
	_, err := s.DB.ExecContext(ctx, "INSERT INTO _meta(key,value) VALUES(?, '1') ON CONFLICT(key) DO UPDATE SET value='1'", key)
	return err
}

func (s *Store) importLegacySettings(ctx context.Context) error {
	done, err := s.legacyImported(ctx, "migratedBlackhandLegacySettings")
	if err != nil || done { return err }
	exists, err := tableExists(ctx, s.DB, "blackhand_legacy_settings")
	if err != nil || !exists { return err }
	hasKey, err := columnExists(ctx, s.DB, "blackhand_legacy_settings", "key")
	if err != nil || !hasKey { return err }
	rows, err := s.DB.QueryContext(ctx, "SELECT key,value_json FROM blackhand_legacy_settings")
	if err != nil { return err }
	defer rows.Close()
	updates := map[string]json.RawMessage{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil { return err }
		if !json.Valid([]byte(raw)) { encoded, _ := json.Marshal(raw); updates[key] = encoded } else { updates[key] = json.RawMessage(raw) }
	}
	if err := rows.Err(); err != nil { return err }
	if len(updates) > 0 {
		current, err := s.GetRawSettings(ctx)
		if err != nil { return err }
		for k, v := range updates { if _, exists := current[k]; !exists { current[k] = v } }
		if err := s.writeRawSettings(ctx, current); err != nil { return err }
	}
	return s.setLegacyImported(ctx, "migratedBlackhandLegacySettings")
}

func (s *Store) importLegacyProxyPools(ctx context.Context) error {
	done, err := s.legacyImported(ctx, "migratedBlackhandProxyPools")
	if err != nil || done { return err }
	exists, err := tableExists(ctx, s.DB, "proxy_pools")
	if err != nil || !exists { return err }
	rows, err := s.DB.QueryContext(ctx, "SELECT id,name,proxy_url,no_proxy,type,active,strict_proxy,test_status,last_tested_at,last_error,created_at,updated_at FROM proxy_pools")
	if err != nil { return err }
	defer rows.Close()
	for rows.Next() {
		var id, name, proxyURL, noProxy, kind, status, lastTested, lastError, createdAt, updatedAt sql.NullString
		var active, strict int
		if err := rows.Scan(&id,&name,&proxyURL,&noProxy,&kind,&active,&strict,&status,&lastTested,&lastError,&createdAt,&updatedAt); err != nil { return err }
		data, _ := json.Marshal(map[string]any{
			"name":name.String, "proxyUrl":proxyURL.String, "noProxy":noProxy.String,
			"type":defaultString(kind.String,"http"), "strictProxy":strict != 0,
			"lastTestedAt":nullString(lastTested), "lastError":nullString(lastError),
		})
		_, err := s.DB.ExecContext(ctx, `INSERT OR IGNORE INTO proxyPools(id,isActive,testStatus,data,createdAt,updatedAt) VALUES(?,?,?,?,?,?)`,
			id.String, active, defaultString(status.String,"unknown"), string(data), defaultString(createdAt.String,time.Now().UTC().Format(time.RFC3339Nano)), defaultString(updatedAt.String,time.Now().UTC().Format(time.RFC3339Nano)))
		if err != nil { return err }
	}
	if err := rows.Err(); err != nil { return err }
	return s.setLegacyImported(ctx, "migratedBlackhandProxyPools")
}

func (s *Store) importLegacyUsage(ctx context.Context) error {
	done, err := s.legacyImported(ctx, "migratedBlackhandUsageEvents")
	if err != nil || done { return err }
	exists, err := tableExists(ctx, s.DB, "usage_events")
	if err != nil || !exists { return err }
	rows, err := s.DB.QueryContext(ctx, "SELECT created_at,provider,model,status,prompt_tokens,completion_tokens,cost FROM usage_events ORDER BY id")
	if err != nil { return err }
	defer rows.Close()
	for rows.Next() {
		var createdAt, provider, model sql.NullString
		var status, prompt, completion int
		var cost float64
		if err := rows.Scan(&createdAt,&provider,&model,&status,&prompt,&completion,&cost); err != nil { return err }
		tokens, _ := json.Marshal(map[string]int{"prompt_tokens":prompt,"completion_tokens":completion})
		ts := defaultString(createdAt.String,time.Now().UTC().Format(time.RFC3339Nano))
		_, err := s.DB.ExecContext(ctx, `INSERT INTO usageHistory(timestamp,provider,model,promptTokens,completionTokens,cost,status,tokens,meta) VALUES(?,?,?,?,?,?,?,?,?)`,
			ts, nullString(provider), nullString(model), prompt, completion, cost, fmt.Sprint(status), string(tokens), "{}")
		if err != nil { return err }
	}
	if err := rows.Err(); err != nil { return err }
	return s.setLegacyImported(ctx, "migratedBlackhandUsageEvents")
}

func (s *Store) GetRawSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, "SELECT data FROM settings WHERE id=1").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) { return map[string]json.RawMessage{}, nil }
	if err != nil { return nil, err }
	result := map[string]json.RawMessage{}
	if raw == "" { return result, nil }
	if err := json.Unmarshal([]byte(raw), &result); err != nil { return nil, fmt.Errorf("decode settings JSON: %w", err) }
	return result, nil
}

func (s *Store) writeRawSettings(ctx context.Context, settings map[string]json.RawMessage) error {
	raw, err := json.Marshal(settings)
	if err != nil { return err }
	_, err = s.DB.ExecContext(ctx, "INSERT INTO settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(raw))
	return err
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" { return fallback }
	return value
}
func nullString(value sql.NullString) any {
	if !value.Valid { return nil }
	return value.String
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys:=make([]string,0,len(m)); for k:=range m { keys=append(keys,k) }; sort.Strings(keys); return keys
}
