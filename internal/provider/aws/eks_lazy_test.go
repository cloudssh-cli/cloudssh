package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/eks"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

// countingEKS counts the per-item describe calls the provider makes.
type countingEKS struct {
	eksAPI
	clusters, groups int
}

func (c *countingEKS) DescribeCluster(ctx context.Context, in *eks.DescribeClusterInput, o ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	c.clusters++
	return c.eksAPI.DescribeCluster(ctx, in, o...)
}

func (c *countingEKS) DescribeNodegroup(ctx context.Context, in *eks.DescribeNodegroupInput, o ...func(*eks.Options)) (*eks.DescribeNodegroupOutput, error) {
	c.groups++
	return c.eksAPI.DescribeNodegroup(ctx, in, o...)
}

func TestEKSListsNamesFirstAndDescribesOnlyWhenAsked(t *testing.T) {
	p := NewDemo(25)
	counter := &countingEKS{eksAPI: p.cl.eks}
	p.cl.eks = counter
	ctx := context.Background()

	r := &eksClusters{p: p}
	pg, err := r.List(ctx, provider.Query{})
	if err != nil || len(pg.Rows) != 3 || counter.clusters != 0 {
		t.Fatalf("listing must not describe: rows=%d describes=%d err=%v", len(pg.Rows), counter.clusters, err)
	}
	for _, row := range pg.Rows {
		if !row.Partial || row.ID == "" || row.Cells[1] != "…" {
			t.Fatalf("expected a cheap partial row, got %+v", row)
		}
	}
	full, err := r.Enrich(ctx, pg.Rows[0])
	if err != nil || full.Partial || counter.clusters != 1 || full.Cells[1] == "…" || full.ID != pg.Rows[0].ID {
		t.Fatalf("enrich: %+v describes=%d err=%v", full, counter.clusters, err)
	}

	// drilling into a node group that has not been enriched yet fetches what it needs
	g := &eksNodeGroups{p: p, cluster: "prod-eks"}
	gp, err := g.List(ctx, provider.Query{})
	if err != nil || counter.groups != 0 || len(gp.Rows) != 2 || !gp.Rows[0].Partial {
		t.Fatalf("node groups list: rows=%d describes=%d err=%v", len(gp.Rows), counter.groups, err)
	}
	d, err := g.Drill(ctx, gp.Rows[0])
	if err != nil || d.Resource == nil || counter.groups != 1 {
		t.Fatalf("drill on a partial row: %+v describes=%d err=%v", d, counter.groups, err)
	}
}
