package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenUsesCompatibleSchemaAndDefaults(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	s, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil { t.Fatal(err) }
	defer s.Close()
	settings, err := s.GetSettings(context.Background())
	if err != nil { t.Fatal(err) }
	if settings["requireApiKey"] != true { t.Fatalf("requireApiKey default = %#v", settings["requireApiKey"]) }
	if settings["rtkEnabled"] != true { t.Fatalf("rtkEnabled default = %#v", settings["rtkEnabled"]) }
	if _, err := s.DB.Exec("INSERT INTO providerConnections(id,provider,authType,data,createdAt,updatedAt) VALUES('x','opencode','api','{}','now','now')"); err != nil { t.Fatalf("providerConnections schema incompatible: %v", err) }
	if _, err := s.DB.Exec("INSERT INTO requestDetails(id,timestamp,data) VALUES('r','now','{}')"); err != nil { t.Fatalf("requestDetails schema incompatible: %v", err) }
}

func TestSettingsUpdatePreservesUnknownKeysAndDoesNotMassAssignPassword(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	s, err := Open(filepath.Join(t.TempDir(), "data")); if err != nil { t.Fatal(err) }; defer s.Close()
	_, err = s.UpdateSettings(context.Background(), map[string]any{"customFutureSetting":map[string]any{"value":1},"password":"should-not-be-saved"})
	if err != nil { t.Fatal(err) }
	got, err := s.GetSettings(context.Background()); if err != nil { t.Fatal(err) }
	if got["customFutureSetting"] == nil { t.Fatal("unknown setting was not retained") }
	if _, ok := got["password"]; ok { t.Fatal("password was mass-assigned by settings update") }
}

func TestAPIKeyCRUDAndAccessPolicy(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	s, err := Open(filepath.Join(t.TempDir(), "data")); if err != nil { t.Fatal(err) }; defer s.Close()
	key, err := s.CreateAPIKey(context.Background(),"test client"); if err != nil { t.Fatal(err) }
	if key.ID==""||key.Key==""||!key.IsActive { t.Fatalf("invalid created key: %#v",key) }
	access:=KeyAccess{Restricted:true,Allow:[]string{"oc/example-model"}}
	active:=true
	updated,err:=s.UpdateAPIKey(context.Background(),key.ID,&active,&access);if err!=nil{t.Fatal(err)}
	if updated==nil||!updated.Access.Restricted{t.Fatalf("access update failed: %#v",updated)}
	if !s.AllowedForModel(updated,"oc/example-model")||!s.AllowedForModel(updated,"opencode/example-model"){t.Fatal("canonical model ID should be allowed")}
	if s.AllowedForModel(updated,"oc/other-model"){t.Fatal("restricted key allowed an unlisted model")}
	active=false
	updated,err=s.UpdateAPIKey(context.Background(),key.ID,&active,nil);if err!=nil{t.Fatal(err)}
	if updated==nil||updated.IsActive{t.Fatal("key should be inactive")}
	found,err:=s.GetAPIKeyByKey(context.Background(),key.Key);if err!=nil{t.Fatal(err)}
	if found==nil||found.ID!=key.ID{t.Fatal("key lookup failed")}
	removed,err:=s.DeleteAPIKey(context.Background(),key.ID);if err!=nil||!removed{t.Fatalf("delete failed: %v / %v",removed,err)}
}

func TestProxyPoolCRUD(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	s,err:=Open(filepath.Join(t.TempDir(),"data"));if err!=nil{t.Fatal(err)};defer s.Close()
	pool,err:=s.CreateProxyPool(context.Background(),map[string]any{"name":"test proxy","proxyUrl":"http://127.0.0.1:8888","type":"http","isActive":true})
	if err!=nil{t.Fatal(err)}
	if pool["id"]==""||pool["testStatus"]!="unknown"{t.Fatalf("unexpected pool %#v",pool)}
	updated,err:=s.UpdateProxyPool(context.Background(),pool["id"].(string),map[string]any{"strictProxy":true,"noProxy":"localhost,127.0.0.1"})
	if err!=nil{t.Fatal(err)}
	if updated["strictProxy"]!=true{t.Fatalf("strictProxy not persisted: %#v",updated)}
	count,err:=s.CountBoundConnections(context.Background(),pool["id"].(string));if err!=nil||count!=0{t.Fatalf("bound connection count = %d, %v",count,err)}
	deleted,err:=s.DeleteProxyPool(context.Background(),pool["id"].(string));if err!=nil||deleted==nil{t.Fatalf("pool delete = %#v, %v",deleted,err)}
}

func TestUsageHistoryIsIdempotentAndChartable(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	s,err:=Open(filepath.Join(t.TempDir(),"data"));if err!=nil{t.Fatal(err)};defer s.Close()
	now:=time.Now().UTC().Format(time.RFC3339Nano)
	entry:=UsageEntry{Timestamp:now,Provider:"opencode",Model:"oc/example",APIKey:"secret-key-for-test",Endpoint:"/v1/chat/completions",Status:"200",Tokens:map[string]any{"prompt_tokens":float64(100),"completion_tokens":float64(20)}}
	if err:=s.SaveRequestUsage(context.Background(),entry);err!=nil{t.Fatal(err)}
	if err:=s.SaveRequestUsage(context.Background(),entry);err!=nil{t.Fatal(err)}
	var count int
	if err:=s.DB.QueryRow("SELECT COUNT(*) FROM usageHistory").Scan(&count);err!=nil{t.Fatal(err)}
	if count!=1{t.Fatalf("duplicate usage inserted %d rows",count)}
	stats,err:=s.GetUsageStats(context.Background(),"all");if err!=nil{t.Fatal(err)}
	if numberValue(stats["totalRequests"])!=1{t.Fatalf("totalRequests = %#v",stats["totalRequests"])}
	chart,err:=s.GetChartData(context.Background(),"7d");if err!=nil{t.Fatal(err)}
	if len(chart)!=7{t.Fatalf("expected 7 chart buckets, got %d",len(chart))}
	history,err:=s.GetUsageHistory(context.Background(),UsageFilter{});if err!=nil{t.Fatal(err)}
	if len(history)!=1{t.Fatalf("history rows = %d",len(history))}
	if history[0]["apiKeyMasked"]=="secret-key-for-test"{t.Fatal("raw API key leaked in usage history")}
}

func TestExistingScaffoldDatabaseIsBackedUpAndImported(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	dataDir:=filepath.Join(t.TempDir(),"data")
	dbPath:=filepath.Join(dataDir,"db","data.sqlite")
	if err:=os.MkdirAll(filepath.Dir(dbPath),0o700);err!=nil{t.Fatal(err)}
	db,err:=sql.Open("sqlite",dbPath);if err!=nil{t.Fatal(err)}
	_,err=db.Exec(`CREATE TABLE settings(key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL);
INSERT INTO settings VALUES('legacyCustom','123','now');
CREATE TABLE proxy_pools(id TEXT PRIMARY KEY,name TEXT NOT NULL,proxy_url TEXT NOT NULL,no_proxy TEXT NOT NULL DEFAULT '',type TEXT NOT NULL DEFAULT 'http',active INTEGER NOT NULL DEFAULT 1,strict_proxy INTEGER NOT NULL DEFAULT 0,test_status TEXT NOT NULL DEFAULT 'unknown',last_tested_at TEXT,last_error TEXT,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
INSERT INTO proxy_pools(id,name,proxy_url,type,active,strict_proxy,test_status,created_at,updated_at) VALUES('legacy-pool','old pool','http://127.0.0.1:8888','http',1,1,'unknown','now','now');
CREATE TABLE usage_events(id INTEGER PRIMARY KEY AUTOINCREMENT,created_at TEXT,provider TEXT,model TEXT,status INTEGER,prompt_tokens INTEGER,completion_tokens INTEGER,cost REAL);
INSERT INTO usage_events(created_at,provider,model,status,prompt_tokens,completion_tokens,cost) VALUES('2025-01-02T00:00:00Z','opencode','oc/old-model',200,20,10,0);`)
	if err!=nil{t.Fatal(err)};if err:=db.Close();err!=nil{t.Fatal(err)}
	s,err:=Open(dataDir);if err!=nil{t.Fatal(err)};defer s.Close()
	settings,err:=s.GetSettings(context.Background());if err!=nil{t.Fatal(err)}
	if settings["legacyCustom"]==nil{raw,_:=json.Marshal(settings);t.Fatalf("legacy setting not imported: %s",raw)}
	pools,err:=s.GetProxyPools(context.Background(),nil);if err!=nil{t.Fatal(err)}
	if len(pools)!=1||pools[0]["id"]!="legacy-pool"{t.Fatalf("legacy pool not imported: %#v",pools)}
	rows,err:=s.GetUsageHistory(context.Background(),UsageFilter{});if err!=nil{t.Fatal(err)}
	if len(rows)!=1||rows[0]["model"]!="oc/old-model"{t.Fatalf("legacy usage not imported: %#v",rows)}
	var rollupCount int
	if err:=s.DB.QueryRow("SELECT COUNT(*) FROM usageDaily").Scan(&rollupCount);err!=nil{t.Fatal(err)}
	if rollupCount!=1{t.Fatalf("legacy usage daily rollup count = %d, want 1",rollupCount)}
	stats,err:=s.GetUsageStats(context.Background(),"all");if err!=nil{t.Fatal(err)}
	if numberValue(stats["totalRequests"])!=1{t.Fatalf("legacy rollup totalRequests = %#v",stats["totalRequests"])}
	backups,err:=os.ReadDir(filepath.Join(dataDir,"db","backups"));if err!=nil{t.Fatal(err)}
	if len(backups)==0{t.Fatal("missing pre-migration database backup")}
}
