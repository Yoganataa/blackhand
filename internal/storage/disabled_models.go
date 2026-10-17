package storage

import (
	"context"
	"database/sql"
	"encoding/json"
)

// Disabled models are stored one kv row per provider alias: value = JSON array of model ids.
// Scope name and shape follow 9Router's disabledModelsDb (`{ providerAlias: [ids] }`).
const disabledModelsScope = "disabledModels"

// GetDisabledModels returns every provider alias with its disabled model ids.
func (s *Store) GetDisabledModels(ctx context.Context) (map[string][]string, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT key,value FROM kv WHERE scope=?", disabledModelsScope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[string][]string{}
	for rows.Next() {
		var alias, raw string
		if err := rows.Scan(&alias, &raw); err != nil {
			return nil, err
		}
		ids := []string{}
		if json.Unmarshal([]byte(raw), &ids) != nil {
			continue
		}
		result[alias] = ids
	}
	return result, rows.Err()
}

// GetDisabledModelsFor returns the disabled model ids of one provider alias.
func (s *Store) GetDisabledModelsFor(ctx context.Context, alias string) ([]string, error) {
	var raw string
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM kv WHERE scope=? AND key=?", disabledModelsScope, alias).Scan(&raw)
	if err == sql.ErrNoRows {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []string{}
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return []string{}, nil
	}
	return ids, nil
}

func (s *Store) saveDisabledModels(ctx context.Context, alias string, ids []string) error {
	if len(ids) == 0 {
		_, err := s.DB.ExecContext(ctx, "DELETE FROM kv WHERE scope=? AND key=?", disabledModelsScope, alias)
		return err
	}
	encoded, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx,
		"INSERT INTO kv(scope,key,value) VALUES(?,?,?) ON CONFLICT(scope,key) DO UPDATE SET value=excluded.value",
		disabledModelsScope, alias, string(encoded))
	return err
}

// DisableModels adds ids to the disabled set of a provider alias (idempotent).
func (s *Store) DisableModels(ctx context.Context, alias string, ids []string) error {
	current, err := s.GetDisabledModelsFor(ctx, alias)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	merged := make([]string, 0, len(current)+len(ids))
	for _, id := range append(current, ids...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		merged = append(merged, id)
	}
	return s.saveDisabledModels(ctx, alias, merged)
}

// EnableModels removes ids from the disabled set; an empty list clears the whole alias.
func (s *Store) EnableModels(ctx context.Context, alias string, ids []string) error {
	if len(ids) == 0 {
		return s.saveDisabledModels(ctx, alias, nil)
	}
	current, err := s.GetDisabledModelsFor(ctx, alias)
	if err != nil {
		return err
	}
	remove := map[string]bool{}
	for _, id := range ids {
		remove[id] = true
	}
	kept := make([]string, 0, len(current))
	for _, id := range current {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	return s.saveDisabledModels(ctx, alias, kept)
}
