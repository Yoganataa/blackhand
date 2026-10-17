package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDisabledModelsRoundTrip(t *testing.T) {
	t.Setenv("NINEROUTER_API_KEY", "")
	s, err := Open(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	empty, err := s.GetDisabledModelsFor(ctx, "oc")
	if err != nil || len(empty) != 0 {
		t.Fatalf("fresh alias = %v, err = %v", empty, err)
	}

	if err := s.DisableModels(ctx, "oc", []string{"a", "b", "a"}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.GetDisabledModelsFor(ctx, "oc")
	if err != nil || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("after disable = %v, err = %v", ids, err)
	}

	// A second disable adds to the set instead of replacing it.
	if err := s.DisableModels(ctx, "oc", []string{"c"}); err != nil {
		t.Fatal(err)
	}
	if ids, _ = s.GetDisabledModelsFor(ctx, "oc"); len(ids) != 3 {
		t.Fatalf("after second disable = %v, want 3 ids", ids)
	}

	// Aliases are independent, and the aggregate read sees both.
	if err := s.DisableModels(ctx, "zed", []string{"z1"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.GetDisabledModels(ctx)
	if err != nil || len(all["oc"]) != 3 || len(all["zed"]) != 1 {
		t.Fatalf("aggregate = %v, err = %v", all, err)
	}

	// Enabling one id keeps the others; enabling with no ids clears the alias.
	if err := s.EnableModels(ctx, "oc", []string{"b"}); err != nil {
		t.Fatal(err)
	}
	if ids, _ = s.GetDisabledModelsFor(ctx, "oc"); len(ids) != 2 || ids[0] != "a" || ids[1] != "c" {
		t.Fatalf("after enable one = %v", ids)
	}
	if err := s.EnableModels(ctx, "oc", nil); err != nil {
		t.Fatal(err)
	}
	if ids, _ = s.GetDisabledModelsFor(ctx, "oc"); len(ids) != 0 {
		t.Fatalf("after clear = %v, want empty", ids)
	}
	if all, _ = s.GetDisabledModels(ctx); len(all["oc"]) != 0 || len(all["zed"]) != 1 {
		t.Fatalf("aggregate after clear = %v", all)
	}
}
