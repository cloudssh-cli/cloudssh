//go:build integration

package aws

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/smithy-go"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

// Runs against a real account using the ambient AWS credentials (CI: OIDC role).
// Every top-level resource must list without error and return rows that match
// its columns.
func TestIntegrationListResources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := New(ctx, "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if ae := p.AuthError(); ae != nil { // New starts even when signed out
		t.Fatalf("not signed in: %v", ae)
	}
	for _, res := range p.Resources() {
		t.Run(res.Key(), func(t *testing.T) {
			page, err := res.List(ctx, provider.Query{Filters: res.DefaultFilters()})
			var apiErr smithy.APIError
			if err != nil && res.Key() != "ec2" && errors.As(err, &apiErr) && permissionDenied(apiErr.ErrorCode()) {
				t.Skipf("role may not read %s (grant its List/Describe actions to enable this check): %v", res.Key(), err)
			}
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			for _, r := range page.Rows {
				if len(r.Cells) != len(res.Columns()) {
					t.Fatalf("row/column mismatch: %v", r.Cells)
				}
			}
		})
	}
}

func permissionDenied(code string) bool {
	return strings.Contains(code, "Denied") || strings.Contains(code, "Unauthorized")
}
