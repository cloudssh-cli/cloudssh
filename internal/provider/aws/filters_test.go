package aws

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
)

func TestSplitValuesSeparatesOrFromNot(t *testing.T) {
	inc, exc := splitValues(" web , api ,!canary, ! test ,, !")
	if !reflect.DeepEqual(inc, []string{"web", "api"}) || !reflect.DeepEqual(exc, []string{"canary", "test"}) {
		t.Fatalf("include=%v exclude=%v", inc, exc)
	}
}

func TestBuildFiltersTurnsCommasIntoAlternativesAndSkipsNegations(t *testing.T) {
	got := buildFilters(ec2Specs, []provider.Filter{
		{Key: "instance-name", Value: "web,api"},     // two alternatives, partial match
		{Key: "instance-id", Value: "i-1,i-2"},       // two exact alternatives
		{Key: "state", Value: "!terminated"},         // not expressible in the API: applied afterwards
		{Key: "instance-type", Value: "t3,!t3.nano"}, // include part only
	})
	if len(got) != 3 {
		t.Fatalf("filters: %+v", got)
	}
	if !reflect.DeepEqual(got[0].Values, []string{"*web*", "*api*"}) || !reflect.DeepEqual(got[1].Values, []string{"i-1", "i-2"}) || !reflect.DeepEqual(got[2].Values, []string{"*t3*"}) {
		t.Fatalf("values: %+v", got)
	}
}

// The real list code, run over the demo data, must honour or / not.
func TestInstanceListUnderstandsOrAndNot(t *testing.T) {
	r := &instances{p: NewDemo(50)}
	names := func(fs ...provider.Filter) []string {
		pg, err := r.List(context.Background(), provider.Query{Filters: fs})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, row := range pg.Rows {
			out = append(out, row.Cells[1])
		}
		return out
	}
	or := names(provider.Filter{Key: "instance-name", Value: "web-prod-1,web-prod-2"})
	if len(or) != 2 || or[0] != "web-prod-1" || or[1] != "web-prod-2" {
		t.Fatalf("or: %v", or)
	}
	mixed := names(provider.Filter{Key: "instance-name", Value: "web-prod,!web-prod-4"})
	if len(mixed) != 3 {
		t.Fatalf("include web-prod but not -4: %v", mixed)
	}
	not := names(provider.Filter{Key: "state", Value: "running"}, provider.Filter{Key: "instance-name", Value: "!web,!api,!batch,!eks,!ecs"})
	for _, n := range not {
		if strings.Contains(n, "web") || strings.Contains(n, "api") || strings.Contains(n, "batch") || strings.Contains(n, "eks") || strings.Contains(n, "ecs") {
			t.Fatalf("%q should have been excluded: %v", n, not)
		}
	}
	if len(not) == 0 {
		t.Fatal("bastion and friends should remain")
	}
	notState := names(provider.Filter{Key: "state", Value: "!running"})
	if len(notState) == 0 {
		t.Fatal("stopped and pending machines remain when running is excluded")
	}
}
