-- Synthetic, sanitized 9Router-shaped schema fixture.
-- Source schema: Yoganataa/9router/src/lib/db/schema.js at dev commit
-- db6112897399ac981ca73ed573d14102e84ca0f1. This fixture uses schemaVersion=1
-- and omits the later apiKeys access-control columns to exercise additive migration.
-- Every provider, key, proxy and account value below is fictitious.
PRAGMA foreign_keys = ON;

CREATE TABLE _meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO _meta(key,value) VALUES
  ('schemaVersion','1'),
  ('migrationVersion','1'),
  ('totalRequestsLifetime','1');

CREATE TABLE settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);
INSERT INTO settings(id,data) VALUES
  (1,'{"requireApiKey":true,"rtkEnabled":true,"fixtureMarker":"preserve-settings"}');

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
INSERT INTO providerConnections(id,provider,authType,name,email,priority,isActive,data,createdAt,updatedAt)
VALUES ('fixture-connection','opencode','apikey','Synthetic account','fixture@example.invalid',1,1,
  '{"apiKey":"REDACTED_FIXTURE_VALUE","baseUrl":"https://example.invalid/v1","fixtureMarker":"preserve-connection-data"}',
  '2026-10-10T11:00:00.000Z','2026-10-10T11:00:00.000Z');

CREATE TABLE providerNodes (
  id TEXT PRIMARY KEY,
  type TEXT,
  name TEXT,
  data TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
INSERT INTO providerNodes(id,type,name,data,createdAt,updatedAt)
VALUES ('fixture-node','provider','Synthetic node','{"fixtureMarker":"preserve-node"}',
  '2026-10-10T11:00:00.000Z','2026-10-10T11:00:00.000Z');

CREATE TABLE proxyPools (
  id TEXT PRIMARY KEY,
  isActive INTEGER DEFAULT 1,
  testStatus TEXT,
  data TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
INSERT INTO proxyPools(id,isActive,testStatus,data,createdAt,updatedAt)
VALUES ('fixture-pool',1,'success',
  '{"name":"Synthetic proxy","proxyUrl":"http://proxy.example.invalid:3128","type":"http","noProxy":"localhost","strictProxy":true,"fixtureMarker":"preserve-pool"}',
  '2026-10-10T11:00:00.000Z','2026-10-10T11:00:00.000Z');

-- SchemaVersion 1 fixture predates the additive per-key access columns.
CREATE TABLE apiKeys (
  id TEXT PRIMARY KEY,
  key TEXT UNIQUE NOT NULL,
  name TEXT,
  machineId TEXT,
  isActive INTEGER DEFAULT 1,
  createdAt TEXT NOT NULL
);
INSERT INTO apiKeys(id,key,name,machineId,isActive,createdAt)
VALUES ('fixture-key-id','sk-fixture-redacted','Synthetic client','fixture-machine',1,'2026-10-10T11:00:00.000Z');

CREATE TABLE combos (
  id TEXT PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  kind TEXT,
  models TEXT NOT NULL,
  createdAt TEXT NOT NULL,
  updatedAt TEXT NOT NULL
);
INSERT INTO combos(id,name,kind,models,createdAt,updatedAt)
VALUES ('fixture-combo','Synthetic combo','custom','["oc/fixture-model"]',
  '2026-10-10T11:00:00.000Z','2026-10-10T11:00:00.000Z');

CREATE TABLE kv (
  scope TEXT NOT NULL,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  PRIMARY KEY (scope,key)
);
INSERT INTO kv(scope,key,value) VALUES
  ('modelAliases','fixture-alias','{"provider":"opencode","model":"fixture-model"}'),
  ('pricing','opencode','{"fixture-model":{"input":1.0,"output":2.0}}');

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
INSERT INTO usageHistory(id,timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta)
VALUES (1,'2026-10-10T11:22:33.456Z','opencode','oc/fixture-model','fixture-connection','sk-fixture-redacted',
  '/v1/chat/completions',12,7,0.25,'200','{"prompt_tokens":12,"completion_tokens":7}','{"fixtureMarker":"preserve-usage"}');

CREATE TABLE usageDaily (dateKey TEXT PRIMARY KEY, data TEXT NOT NULL);
INSERT INTO usageDaily(dateKey,data)
VALUES ('2026-10-10','{"requests":1,"promptTokens":12,"completionTokens":7,"cachedTokens":0,"cost":0.25,"byProvider":{},"byModel":{},"byAccount":{},"byApiKey":{},"byEndpoint":{}}');

CREATE TABLE requestDetails (
  id TEXT PRIMARY KEY,
  timestamp TEXT NOT NULL,
  provider TEXT,
  model TEXT,
  connectionId TEXT,
  status TEXT,
  data TEXT NOT NULL
);
INSERT INTO requestDetails(id,timestamp,provider,model,connectionId,status,data)
VALUES ('fixture-request','2026-10-10T11:22:33.456Z','opencode','oc/fixture-model','fixture-connection','200',
  '{"fixtureMarker":"preserve-request-detail"}');

-- 9Router may have unrelated extension tables; additive schema sync must leave them alone.
CREATE TABLE extension_fixture (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
INSERT INTO extension_fixture(id,payload) VALUES ('extension-row','keep-me');
