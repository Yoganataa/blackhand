-- Sanitized fixture modeled on the supplied data.sqlite's schema and row-count shape.
-- Values are synthetic; no credentials or private settings are copied from the supplied database.
PRAGMA foreign_keys = ON;

CREATE TABLE _meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO _meta(key,value) VALUES
  ('appVersion','fixture-v1'),
  ('backupSchemaVersion','1'),
  ('schemaVersion','1'),
  ('totalRequestsLifetime','3');

CREATE TABLE apiKeys (
  id TEXT PRIMARY KEY,
  key TEXT UNIQUE NOT NULL,
  name TEXT,
  machineId TEXT,
  isActive INTEGER DEFAULT 1,
  createdAt TEXT NOT NULL,
  accessRestricted INTEGER DEFAULT 0,
  accessAllow TEXT
);
INSERT INTO apiKeys(id,key,name,machineId,isActive,createdAt,accessRestricted,accessAllow)
VALUES ('fixture-key-id','sk-blackhand-fixture-key-never-use','Sanitized fixture key',NULL,1,
        '2026-10-17T10:00:00.000Z',0,NULL);

CREATE TABLE combos (
  id TEXT PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  kind TEXT,
  models TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
CREATE TABLE kv (
  scope TEXT NOT NULL,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY(scope,key)
);
CREATE TABLE providerConnections (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  authType TEXT NOT NULL,
  name TEXT,
  email TEXT,
  priority INTEGER,
  isActive INTEGER DEFAULT 1,
  data TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
CREATE TABLE providerNodes (
  id TEXT PRIMARY KEY,
  type TEXT,
  name TEXT,
  data TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
CREATE TABLE proxyPools (
  id TEXT PRIMARY KEY,
  isActive INTEGER DEFAULT 1,
  testStatus TEXT,
  data TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
CREATE TABLE requestDetails (
  id TEXT PRIMARY KEY,
  timestamp TEXT NOT NULL,
  provider TEXT,
  model TEXT,
  connectionId TEXT,
  status TEXT,
  data TEXT NOT NULL
);
CREATE TABLE settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);
INSERT INTO settings(id,data) VALUES
  (1,'{"rtkEnabled":true,"headroomEnabled":true,"fixtureMarker":"preserve-sanitized-settings"}');

CREATE TABLE usageDaily (dateKey TEXT PRIMARY KEY, data TEXT NOT NULL);
INSERT INTO usageDaily(dateKey,data) VALUES
  ('2026-10-17','{"requests":3,"promptTokens":1496603,"completionTokens":3104,"cachedTokens":588416,"cost":0.1302587048,"byProvider":{},"byModel":{},"byAccount":{},"byApiKey":{},"byEndpoint":{}}');

CREATE TABLE usageHistory (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp TEXT NOT NULL,
  provider TEXT,
  model TEXT,
  connectionId TEXT,
  apiKey TEXT,
  endpoint TEXT,
  promptTokens INTEGER DEFAULT 0,
  completionTokens INTEGER DEFAULT 0,
  cost REAL DEFAULT 0,
  status TEXT,
  tokens TEXT,
  meta TEXT
);
INSERT INTO usageHistory(id,timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta) VALUES
 (1,'2026-10-17T10:00:00.000Z','opencode','oc/fixture-model',NULL,'sk-blackhand-fixture-key-never-use','/v1/chat/completions',500000,1000,0.05,'200','{"prompt_tokens":500000,"completion_tokens":1000,"cached_tokens":200000}','{"fixtureMarker":"preserve-usage-1"}'),
 (2,'2026-10-17T10:01:00.000Z','opencode','oc/fixture-model',NULL,'sk-blackhand-fixture-key-never-use','/v1/chat/completions',500000,1000,0.04,'200','{"prompt_tokens":500000,"completion_tokens":1000,"cached_tokens":200000}','{"fixtureMarker":"preserve-usage-2"}'),
 (3,'2026-10-17T10:02:00.000Z','opencode','oc/fixture-model',NULL,'sk-blackhand-fixture-key-never-use','/v1/chat/completions',496603,1104,0.0402587048,'200','{"prompt_tokens":496603,"completion_tokens":1104,"cached_tokens":188416}','{"fixtureMarker":"preserve-usage-3"}');

CREATE INDEX idx_ak_key ON apiKeys(key);
CREATE INDEX idx_combo_name ON combos(name);
CREATE INDEX idx_kv_scope ON kv(scope);
CREATE INDEX idx_pc_priority ON providerConnections(provider, priority);
CREATE INDEX idx_pc_provider ON providerConnections(provider);
CREATE INDEX idx_pc_provider_active ON providerConnections(provider, isActive);
CREATE INDEX idx_pn_type ON providerNodes(type);
CREATE INDEX idx_pp_active ON proxyPools(isActive);
CREATE INDEX idx_pp_status ON proxyPools(testStatus);
CREATE INDEX idx_rd_conn ON requestDetails(connectionId);
CREATE INDEX idx_rd_model ON requestDetails(model);
CREATE INDEX idx_rd_provider ON requestDetails(provider);
CREATE INDEX idx_rd_ts ON requestDetails(timestamp DESC);
CREATE INDEX idx_uh_conn ON usageHistory(connectionId);
CREATE INDEX idx_uh_model ON usageHistory(model);
CREATE INDEX idx_uh_provider ON usageHistory(provider);
CREATE INDEX idx_uh_ts ON usageHistory(timestamp DESC);
