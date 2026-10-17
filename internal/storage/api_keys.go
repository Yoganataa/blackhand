package storage

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const configuredAPIKeyID = "00000000-0000-4000-8000-000000000001"

type KeyAccess struct {
	Restricted bool     `json:"restricted"`
	Allow      []string `json:"allow"`
}

type APIKey struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	MachineID string    `json:"machineId,omitempty"`
	IsActive  bool      `json:"isActive"`
	CreatedAt string    `json:"createdAt"`
	Access    KeyAccess `json:"access"`
}

func (s *Store) EnsureConfiguredAPIKey(ctx context.Context) error {
	key := strings.TrimSpace(os.Getenv("NINEROUTER_API_KEY"))
	if key == "" { return nil }
	name := strings.TrimSpace(os.Getenv("NINEROUTER_API_KEY_NAME"))
	if name == "" { name = "Environment API Key" }
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	var existingID string
	err = tx.QueryRowContext(ctx, "SELECT id FROM apiKeys WHERE key=? LIMIT 1", key).Scan(&existingID)
	if err == nil {
		if _, err := tx.ExecContext(ctx, "UPDATE apiKeys SET isActive=1 WHERE id=?", existingID); err != nil { return err }
		if existingID != configuredAPIKeyID {
			if _, err := tx.ExecContext(ctx, "UPDATE apiKeys SET isActive=0 WHERE id=?", configuredAPIKeyID); err != nil { return err }
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) { return err }

	var managedKey string
	err = tx.QueryRowContext(ctx, "SELECT key FROM apiKeys WHERE id=?", configuredAPIKeyID).Scan(&managedKey)
	if err == nil {
		if _, err := tx.ExecContext(ctx, "UPDATE apiKeys SET key=?,name=?,machineId=NULL,isActive=1,accessRestricted=0,accessAllow=NULL WHERE id=?", key, name, configuredAPIKeyID); err != nil { return err }
	} else if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO apiKeys(id,key,name,machineId,isActive,createdAt,accessRestricted,accessAllow) VALUES(?,?,?,NULL,1,?,0,NULL)`, configuredAPIKeyID, key, name, nowISO()); err != nil { return err }
	} else { return err }
	return tx.Commit()
}

func (s *Store) GetAPIKeys(ctx context.Context) ([]APIKey, error) {
	if err := s.EnsureConfiguredAPIKey(ctx); err != nil { return nil, err }
	rows, err := s.DB.QueryContext(ctx, `SELECT id,key,name,machineId,isActive,createdAt,accessRestricted,accessAllow FROM apiKeys ORDER BY createdAt ASC`)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil { return nil, err }
		out = append(out, key)
	}
	return out, rows.Err()
}

func (s *Store) GetAPIKeyByID(ctx context.Context, id string) (*APIKey, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT id,key,name,machineId,isActive,createdAt,accessRestricted,accessAllow FROM apiKeys WHERE id=?`, id)
	key, err := scanAPIKey(row)
	if errors.Is(err, sql.ErrNoRows) { return nil, nil }
	if err != nil { return nil, err }
	return &key, nil
}

func (s *Store) GetAPIKeyByKey(ctx context.Context, raw string) (*APIKey, error) {
	if strings.TrimSpace(raw) == "" { return nil, nil }
	if err := s.EnsureConfiguredAPIKey(ctx); err != nil { return nil, err }
	row := s.DB.QueryRowContext(ctx, `SELECT id,key,name,machineId,isActive,createdAt,accessRestricted,accessAllow FROM apiKeys WHERE key=? LIMIT 1`, raw)
	key, err := scanAPIKey(row)
	if errors.Is(err, sql.ErrNoRows) { return nil, nil }
	if err != nil { return nil, err }
	return &key, nil
}

type apiKeyScanner interface { Scan(dest ...any) error }
func scanAPIKey(row apiKeyScanner) (APIKey, error) {
	var key APIKey
	var machine, name, accessAllow sql.NullString
	var active, restricted int
	if err := row.Scan(&key.ID, &key.Key, &name, &machine, &active, &key.CreatedAt, &restricted, &accessAllow); err != nil {
		return APIKey{}, err
	}
	if name.Valid { key.Name = name.String }
	if machine.Valid { key.MachineID = machine.String }
	key.IsActive = active == 1
	key.Access = KeyAccess{Restricted: restricted == 1, Allow: []string{}}
	if key.Access.Restricted && accessAllow.Valid && accessAllow.String != "" {
		if err := json.Unmarshal([]byte(accessAllow.String), &key.Access.Allow); err != nil {
			// Restricted key with corrupted allow-list fails closed.
			key.Access.Allow = []string{}
		}
		key.Access.Allow = normalizeAllowList(key.Access.Allow)
	}
	return key, nil
}

func normalizeAllowList(input []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(input))
	for _, item := range input {
		value := strings.TrimSpace(item)
		if value == "" { continue }
		canonical := strings.ToLower(value)
		if seen[canonical] { continue }
		seen[canonical] = true
		out = append(out, value)
	}
	return out
}

func ValidateKeyAccess(access KeyAccess) (KeyAccess, error) {
	if len(access.Allow) > 200 { return KeyAccess{}, fmt.Errorf("access.allow has more than 200 entries") }
	for _, item := range access.Allow {
		if len([]rune(strings.TrimSpace(item))) > 256 { return KeyAccess{}, fmt.Errorf("access.allow entries must be at most 256 characters") }
	}
	access.Allow = normalizeAllowList(access.Allow)
	if !access.Restricted { access.Allow = []string{} }
	return access, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, name string) (*APIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" { return nil, errors.New("Name is required") }
	machineID, err := s.MachineID()
	if err != nil { return nil, err }
	key, err := generateAPIKey(machineID)
	if err != nil { return nil, err }
	id, err := randomUUID()
	if err != nil { return nil, err }
	now := nowISO()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO apiKeys(id,key,name,machineId,isActive,createdAt,accessRestricted,accessAllow) VALUES(?,?,?,?,1,?,0,?)`,
		id, key, name, machineID, now, "[]")
	if err != nil { return nil, err }
	return &APIKey{ID:id, Key:key, Name:name, MachineID:machineID, IsActive:true, CreatedAt:now, Access:KeyAccess{Restricted:false, Allow:[]string{}}}, nil
}

func (s *Store) UpdateAPIKey(ctx context.Context, id string, active *bool, access *KeyAccess) (*APIKey, error) {
	current, err := s.GetAPIKeyByID(ctx, id)
	if err != nil || current == nil { return current, err }
	if access != nil {
		validated, err := ValidateKeyAccess(*access)
		if err != nil { return nil, err }
		access = &validated
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil { return nil, err }
	defer tx.Rollback()
	if active != nil {
		value := 0; if *active { value = 1 }
		if _, err := tx.ExecContext(ctx, "UPDATE apiKeys SET isActive=? WHERE id=?", value, id); err != nil { return nil, err }
	}
	if access != nil {
		allow, err := json.Marshal(access.Allow)
		if err != nil { return nil, err }
		restricted := 0; if access.Restricted { restricted = 1 }
		if _, err := tx.ExecContext(ctx, "UPDATE apiKeys SET accessRestricted=?,accessAllow=? WHERE id=?", restricted, string(allow), id); err != nil { return nil, err }
	}
	if err := tx.Commit(); err != nil { return nil, err }
	return s.GetAPIKeyByID(ctx, id)
}

func (s *Store) DeleteAPIKey(ctx context.Context, id string) (bool, error) {
	result, err := s.DB.ExecContext(ctx, "DELETE FROM apiKeys WHERE id=?", id)
	if err != nil { return false, err }
	n, err := result.RowsAffected()
	return n > 0, err
}

func generateAPIKey(machineID string) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, 6)
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil { return "", err }
	for i := range raw { raw[i] = alphabet[int(random[i])%len(alphabet)] }
	keyID := string(raw)
	secret := envDefault("API_KEY_SECRET", "endpoint-proxy-api-key-secret")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(machineID + keyID))
	crc := hex.EncodeToString(mac.Sum(nil))[:8]
	return "sk-" + machineID + "-" + keyID + "-" + crc, nil
}

func randomUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil { return "", err }
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(b)
	return raw[:8]+"-"+raw[8:12]+"-"+raw[12:16]+"-"+raw[16:20]+"-"+raw[20:], nil
}

func (s *Store) IsConfiguredAPIKey(key string) bool {
	configured := strings.TrimSpace(os.Getenv("NINEROUTER_API_KEY"))
	return configured != "" && hmac.Equal([]byte(configured), []byte(key))
}

func canonicalOpenCodeModel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "oc/") { return "opencode/" + strings.TrimPrefix(value, "oc/") }
	return value
}

func (s *Store) AllowedForModel(key *APIKey, requestedModel string) bool {
	if key == nil || !key.Access.Restricted { return true }
	requested := canonicalOpenCodeModel(requestedModel)
	for _, allowed := range key.Access.Allow {
		candidate := canonicalOpenCodeModel(allowed)
		if candidate == requested { return true }
		if !strings.Contains(candidate, "/") && (candidate == "opencode" || candidate == "oc") && strings.HasPrefix(requested, "opencode/") { return true }
	}
	return false
}

func SortAPIKeys(keys []APIKey) {
	sort.SliceStable(keys, func(i,j int) bool { return keys[i].CreatedAt < keys[j].CreatedAt })
}
