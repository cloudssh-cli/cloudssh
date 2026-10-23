//go:build integration

package aws

import (
	"context"
	"testing"
	"time"
)

// Runs against a real account using the ambient AWS credentials (CI: OIDC role).
func TestIntegrationListInstances(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	p, err := New(ctx, "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	page, err := p.Resources()[0].List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range page.Rows {
		if len(r.Cells) != len(p.Resources()[0].Columns()) {
			t.Fatalf("row/column mismatch: %v", r.Cells)
		}
	}
}
