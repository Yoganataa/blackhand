package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var validProxyTypes = map[string]bool{
	"http": true, "vercel": true, "cloudflare": true, "deno": true, "netlify": true,
}

func (s *Store) GetProxyPools(ctx context.Context, active *bool) ([]map[string]any, error) {
	query := "SELECT id,isActive,testStatus,data,createdAt,updatedAt FROM proxyPools"
	args := []any{}
	if active != nil { query += " WHERE isActive=?"; if *active { args = append(args,1) } else { args=append(args,0) } }
	query += " ORDER BY updatedAt DESC"
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		pool, err := scanProxyPool(rows)
		if err != nil { return nil, err }
		out = append(out, pool)
	}
	return out, rows.Err()
}

func (s *Store) GetProxyPool(ctx context.Context, id string) (map[string]any, error) {
	pool, err := scanProxyPool(s.DB.QueryRowContext(ctx, "SELECT id,isActive,testStatus,data,createdAt,updatedAt FROM proxyPools WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) { return nil, nil }
	if err != nil { return nil, err }
	return pool, nil
}

type proxyPoolScanner interface { Scan(dest ...any) error }
func scanProxyPool(row proxyPoolScanner) (map[string]any, error) {
	var id, status, raw, createdAt, updatedAt sql.NullString
	var active int
	if err := row.Scan(&id,&active,&status,&raw,&createdAt,&updatedAt); err != nil { return nil, err }
	extra := map[string]any{}
	if raw.Valid && raw.String != "" {
		if err := json.Unmarshal([]byte(raw.String), &extra); err != nil { return nil, fmt.Errorf("decode proxy pool %s: %w", id.String, err) }
	}
	extra["id"] = id.String
	extra["isActive"] = active == 1
	extra["testStatus"] = defaultString(status.String, "unknown")
	extra["createdAt"] = createdAt.String
	extra["updatedAt"] = updatedAt.String
	if _, ok := extra["name"]; !ok { extra["name"] = "" }
	if _, ok := extra["proxyUrl"]; !ok { extra["proxyUrl"] = "" }
	if _, ok := extra["noProxy"]; !ok { extra["noProxy"] = "" }
	if _, ok := extra["type"]; !ok { extra["type"] = "http" }
	if _, ok := extra["strictProxy"]; !ok { extra["strictProxy"] = false }
	if _, ok := extra["lastTestedAt"]; !ok { extra["lastTestedAt"] = nil }
	if _, ok := extra["lastError"]; !ok { extra["lastError"] = nil }
	return extra, nil
}

func (s *Store) CreateProxyPool(ctx context.Context, input map[string]any) (map[string]any, error) {
	name, _ := input["name"].(string)
	proxyURL, _ := input["proxyUrl"].(string)
	noProxy, _ := input["noProxy"].(string)
	kind, _ := input["type"].(string)
	name, proxyURL, noProxy = strings.TrimSpace(name), strings.TrimSpace(proxyURL), strings.TrimSpace(noProxy)
	if name == "" { return nil, fmt.Errorf("Name is required") }
	if proxyURL == "" { return nil, fmt.Errorf("Proxy URL is required") }
	if !validProxyTypes[kind] { kind = "http" }
	active := true
	if v, ok := input["isActive"]; ok { active = v == true }
	strict := input["strictProxy"] == true
	now := nowISO()
	id, err := randomUUID()
	if err != nil { return nil, err }
	data := map[string]any{
		"name":name, "proxyUrl":proxyURL, "noProxy":noProxy, "type":kind,
		"strictProxy":strict, "lastTestedAt":nil, "lastError":nil,
	}
	encoded, err := json.Marshal(data)
	if err != nil { return nil, err }
	status := "unknown"
	if value, ok := input["testStatus"].(string); ok && value != "" { status = value }
	isActive := 0; if active { isActive = 1 }
	_, err = s.DB.ExecContext(ctx, `INSERT INTO proxyPools(id,isActive,testStatus,data,createdAt,updatedAt) VALUES(?,?,?,?,?,?)`,
		id,isActive,status,string(encoded),now,now)
	if err != nil { return nil, err }
	return s.GetProxyPool(ctx, id)
}

func (s *Store) UpdateProxyPool(ctx context.Context, id string, updates map[string]any) (map[string]any, error) {
	current, err := s.GetProxyPool(ctx, id)
	if err != nil || current == nil { return current, err }
	merged := make(map[string]any, len(current)+len(updates))
	for k,v := range current { merged[k]=v }
	for k,v := range updates { merged[k]=v }
	if value, exists := updates["name"]; exists {
		name, ok := value.(string); if !ok || strings.TrimSpace(name)=="" { return nil, fmt.Errorf("Name is required") }
		merged["name"] = strings.TrimSpace(name)
	}
	if value, exists := updates["proxyUrl"]; exists {
		url, ok := value.(string); if !ok || strings.TrimSpace(url)=="" { return nil, fmt.Errorf("Proxy URL is required") }
		merged["proxyUrl"] = strings.TrimSpace(url)
	}
	if value, exists := updates["noProxy"]; exists {
		url, ok := value.(string); if !ok { return nil, fmt.Errorf("noProxy must be a string") }; merged["noProxy"] = strings.TrimSpace(url)
	}
	if value, exists := updates["type"]; exists {
		kind, ok := value.(string); if !ok || !validProxyTypes[kind] { kind = "http" }; merged["type"] = kind
	}
	if value, exists := updates["isActive"]; exists { merged["isActive"] = value == true }
	if value, exists := updates["strictProxy"]; exists { merged["strictProxy"] = value == true }

	data := map[string]any{}
	for k,v := range merged {
		switch k {
		case "id","isActive","testStatus","createdAt","updatedAt","boundConnectionCount": continue
		default: data[k] = v
		}
	}
	encoded, err := json.Marshal(data)
	if err != nil { return nil, err }
	isActive := 0; if merged["isActive"] == true { isActive = 1 }
	testStatus, _ := merged["testStatus"].(string)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.DB.ExecContext(ctx, "UPDATE proxyPools SET isActive=?,testStatus=?,data=?,updatedAt=? WHERE id=?",
		isActive, defaultString(testStatus,"unknown"), string(encoded), now, id)
	if err != nil { return nil, err }
	changed, err := result.RowsAffected()
	if err != nil { return nil, err }
	if changed == 0 { return nil, nil }
	return s.GetProxyPool(ctx, id)
}

func (s *Store) DeleteProxyPool(ctx context.Context, id string) (map[string]any, error) {
	current, err := s.GetProxyPool(ctx, id)
	if err != nil || current == nil { return current, err }
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM proxyPools WHERE id=?", id); err != nil { return nil, err }
	return current, nil
}

func (s *Store) CountBoundConnections(ctx context.Context, poolID string) (int, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT data FROM providerConnections")
	if err != nil { return 0, err }
	defer rows.Close()
	count := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil { return 0, err }
		var data map[string]any
		if json.Unmarshal([]byte(raw), &data) != nil { continue }
		providerData, _ := data["providerSpecificData"].(map[string]any)
		if providerData["proxyPoolId"] == poolID { count++ }
	}
	return count, rows.Err()
}
