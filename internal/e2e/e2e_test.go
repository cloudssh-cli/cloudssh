// Package e2e drives the real UI over the demo provider like a user would:
// keystrokes in, screen text out. The UI package itself is cloud-agnostic and
// tested with fakes; these tests prove the real cloud code, the demo data and
// the UI work together. Add a test per user-visible flow; when you add a cloud
// with demo data, add the same flows for it (see .claude/skills/demo-data).
package e2e

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudssh-cli/cloudssh/internal/config"
	"github.com/cloudssh-cli/cloudssh/internal/provider"
	"github.com/cloudssh-cli/cloudssh/internal/provider/aws"
	"github.com/cloudssh-cli/cloudssh/internal/ui"
)

// ---- tiny harness (black box: only View() is inspected) ----

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// run executes cmd, flattening batches and skipping slow timer commands.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, run(c)...)
			}
			return out
		}
		return []tea.Msg{msg}
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func settle(m ui.Model, cmd tea.Cmd) ui.Model {
	queue := []tea.Cmd{cmd}
	for i := 0; i < 30 && len(queue) > 0; i++ {
		c := queue[0]
		queue = queue[1:]
		for _, msg := range run(c) {
			next, nc := m.Update(msg)
			m = next.(ui.Model)
			queue = append(queue, nc)
		}
	}
	return m
}

// press sends each key and lets every resulting command (API calls) finish.
func press(m ui.Model, keys ...string) ui.Model {
	for _, k := range keys {
		next, cmd := m.Update(keyMsg(k))
		m = settle(next.(ui.Model), cmd)
	}
	return m
}

// typeText types literal text (e.g. into the search box).
func typeText(m ui.Model, s string) ui.Model {
	for _, r := range s {
		next, _ := m.Update(keyMsg(string(r)))
		m = next.(ui.Model)
	}
	return m
}

func start(t *testing.T, p provider.Provider, w, h int) ui.Model {
	t.Helper()
	return startWith(t, p, w, h, func(c *config.Config) { c.Prefetch, c.SearchPrefetch = 0, 0 })
}

// startWith builds the UI over p with the default test config adjusted by mut.
func startWith(t *testing.T, p provider.Provider, w, h int, mut func(*config.Config)) ui.Model {
	t.Helper()
	cfg := config.Default()
	cfg.SkipSplash, cfg.PageSize, cfg.PrefetchDelayMs = true, 8, 0
	mut(&cfg)
	m := ui.New(cfg, p)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = next.(ui.Model)
	return settle(m, m.Init())
}

func demo(t *testing.T) ui.Model { t.Helper(); return start(t, aws.NewDemo(8), 150, 40) }

func mustContain(t *testing.T, m ui.Model, wants ...string) {
	t.Helper()
	v := m.View()
	for _, w := range wants {
		if !strings.Contains(v, w) {
			t.Errorf("screen lacks %q:\n%s", w, v)
		}
	}
}

func mustNotContain(t *testing.T, m ui.Model, nots ...string) {
	t.Helper()
	v := m.View()
	for _, n := range nots {
		if strings.Contains(v, n) {
			t.Errorf("screen should not show %q:\n%s", n, v)
		}
	}
}

// ---- flows ----

func TestDemoOpensOnRunningInstancesAndSaysItIsADemo(t *testing.T) {
	m := demo(t)
	mustContain(t, m, "Cloud: aws · us-east-1 · demo", "Account: cloudssh-production (123456789012)", "Auth: SSO (cloudssh-prod) · role AdministratorAccess", "aws · us-east-1",
		"Instances[", "api-prod-1", "bastion-01", "running", "Press n to load more...", "<ec2_instances>")
	mustNotContain(t, m, "postgres-replica", "old-build-agent") // stopped and terminated hide behind the default state filter
}

func TestPagingWalksEveryRunningInstance(t *testing.T) {
	m := demo(t)
	pages := 1
	for strings.Contains(m.View(), "to load more...") && pages < 20 {
		m = press(m, "n")
		pages++
	}
	if pages < 5 { // ~50 running instances at 8 per page
		t.Errorf("the demo should need several pages, got %d:\n%s", pages, m.View())
	}
	mustContain(t, m, "all loaded") // nothing left to load
	mustNotContain(t, m, "to load more...")
}

func TestSearchOnlySeesLoadedPagesAndSaysSo(t *testing.T) {
	m := demo(t)
	m = typeText(press(m, "/"), "batch-worker-2")
	m = press(m, "enter")
	mustContain(t, m, "Searched 8 rows", "n to load more")
	mustNotContain(t, m, "batch-worker-25") // not loaded yet, so a search cannot find it
	for i := 0; i < 6; i++ {
		m = press(m, "n")
	}
	mustContain(t, m, "batch-worker-25", "Searched ") // the fleet is bigger than seven pages: the note stays until everything is loaded
}

func TestSearchNarrowsTheLoadedRows(t *testing.T) {
	m := demo(t)
	m = press(m, "n", "n")
	m = typeText(press(m, "/"), "api-prod")
	m = press(m, "enter")
	mustContain(t, m, "api-prod-1", "api-prod-2", "api-prod-3", "</api-prod>")
	mustNotContain(t, m, "bastion-01", "web-prod-1")
}

func TestFiltersDialogQueriesTheServer(t *testing.T) {
	m := demo(t)
	m = press(m, "F")
	mustContain(t, m, "Update data filters", "Instance Name", "Instance State", "Availability Zone", "running")
	m = press(m, "up", "up", "up", "up", "up", "up") // from Instance State (7th row) up to Instance Name
	m = typeText(m, "ecs-prod")
	m = press(m, "enter")
	mustContain(t, m, "ecs-prod-apps-1", "ecs-prod-apps-2", "ecs-prod-apps-3", "name=ecs-prod")
	mustNotContain(t, m, "web-prod-1", "bastion-01")
}

func TestSSHDialogUsesTheDefaultUserFromTheImageAndOffersTheTemporaryKey(t *testing.T) {
	m := demo(t)
	m = press(m, "S") // first row: api-prod-1, an Ubuntu machine
	mustContain(t, m, "SSH => api-prod-1", "ubuntu@10.0.10.", "Method", "EC2 Instance Connect", "tab next")
	m = press(m, "tab", "right") // to the Method field, then change it
	mustContain(t, m, "-i <EC2 Instance Connect>", "IdentitiesOnly=yes")
	m = press(m, "esc")
	mustNotContain(t, m, "SSH =>")
}

func TestAutoScalingGroupDrillsIntoItsInstances(t *testing.T) {
	m := demo(t)
	m = press(m, "4")
	mustContain(t, m, "Auto Scaling · Groups[6]", "batch-workers-asg", "web-prod-asg", "api-prod-asg", "ecs-prod-apps-asg", "Desired Capacity")
	m = press(typeText(press(m, "/"), "web-prod"), "enter", "enter")
	mustContain(t, m, "EC2 · asg web-prod-asg[4]", "web-prod-1", "web-prod-4", "<autoscaling_groups>", "<ec2_instances>")
	mustNotContain(t, m, "api-prod-1")
	m = press(m, "esc")
	mustContain(t, m, "Auto Scaling · Groups[")
}

func TestVPCToSubnetsToInstances(t *testing.T) {
	m := demo(t)
	m = press(m, "5")
	mustContain(t, m, "VPC · Networks[3]", "prod-main", "prod-dr", "172.31.0.0/16")
	m = press(typeText(press(m, "/"), "prod-main"), "enter")
	m = press(m, "enter")
	mustContain(t, m, "[8]", "prod-public-a", "prod-private-a", "prod-data-b", "Available Ips", "<vpcs>", "<subnets>")
	m = press(m, "enter") // the subnet with the fewest free addresses has the most machines
	mustContain(t, m, "<vpcs> <subnets> <ec2_instances>", "EC2 · vpc subnet prod-")
}

func TestECSClusterToServicesToTasksToInstance(t *testing.T) {
	m := demo(t)
	m = press(m, "2")
	mustContain(t, m, "ECS · Clusters[3]", "prod-apps", "prod-fargate", "batch")
	m = press(m, "enter") // prod-apps runs the most tasks
	mustContain(t, m, "ECS · prod-apps[5]", "api", "worker", "log-agent", "scheduler", "migrations", "3 running / 3 desired", "<ecs_clusters> <services>")
	m = press(m, "enter") // api
	mustContain(t, m, "ECS · prod-apps › api[3]", "RUNNING/RUNNING", "1", "2 GB", "<tasks>")
	m = press(m, "enter") // an EC2-launched task resolves to its container instance
	mustContain(t, m, "<ecs_clusters> <services> <tasks> <ec2_instances>", "ecs-prod-apps-")
}

func TestDrillGuardsExplainThemselvesInsteadOfFailing(t *testing.T) {
	m := demo(t)
	m = press(m, "2", "j", "j", "enter") // "batch" has no services
	mustContain(t, m, "cluster batch has no services")
	m = press(m, "k", "k", "enter") // prod-apps services, sorted by name: api, log-agent, migrations, scheduler, worker
	m = press(m, "j", "j", "enter") // migrations: desired 0
	mustContain(t, m, "service migrations has no running / pending tasks")
	m = press(m, "j", "enter") // scheduler runs on Fargate
	m = press(m, "enter")      // its task has no EC2 host
	mustContain(t, m, "Fargate")
}

func TestEKSClusterToNodeGroupsToNodes(t *testing.T) {
	m := demo(t)
	m = press(m, "3")
	mustContain(t, m, "EKS · Clusters[3]", "data-platform", "legacy-eks", "prod-eks", "1.30", "UPDATING")
	m = press(m, "j", "j", "enter") // prod-eks
	mustContain(t, m, "EKS · prod-eks[2]", "general", "spot", "1.30.2-20240729", "3/6", "<eks_clusters> <node_groups>")
	m = press(m, "enter") // general
	mustContain(t, m, "prod-eks-general-node", "<node_groups> <ec2_instances>")
}

func TestSwitchingProfilesChangesTheWholeApp(t *testing.T) {
	m := demo(t)
	m = press(m, "p")
	mustContain(t, m, "Profiles[3]", "cloudssh-prod", "cloudssh-staging", "cloudssh-sandbox", "●", "SSO", "access keys", "123456789012 AdministratorAccess")
	m = press(m, "j", "enter") // cloudssh-sandbox (profiles sort by name: prod, sandbox, staging)
	mustContain(t, m, "Auth: access keys (cloudssh-sandbox)", "aws · us-west-2", "playground")
	mustNotContain(t, m, "bastion-01")
	m = press(m, "4")
	mustContain(t, m, "No auto scaling groups found!")             // the cloudssh-sandbox account is tiny: the empty state
	m = press(m, "p", "k", "k", "enter")                           // the cursor rests on the active profile (cloudssh-sandbox); two up is cloudssh-prod
	mustContain(t, m, "Auth: SSO (cloudssh-prod)", "web-prod-asg") // back on the screen we came from, now for cloudssh-prod
}

func TestUnknownDemoProfileStartsSignedOutAndRecovers(t *testing.T) {
	p, err := aws.Open(context.Background(), provider.Options{Demo: true, Profile: "nope", PageSize: 8})
	if err != nil {
		t.Fatal(err)
	}
	m := start(t, p, 150, 40)
	mustContain(t, m, "Auth: not signed in", `There is no demo profile called "nope".`, "cloudssh-prod", "Profiles[3]", "Use profile")
	m = press(m, "enter") // cloudssh-prod
	mustContain(t, m, "Auth: SSO (cloudssh-prod)", "bastion-01")
}

func TestEveryDemoScreenFitsTheTerminal(t *testing.T) {
	for _, size := range [][2]int{{150, 40}, {120, 30}, {80, 24}} {
		m := start(t, aws.NewDemo(8), size[0], size[1])
		screens := map[string]ui.Model{"ec2": m}
		for _, k := range []string{"2", "3", "4", "5", "d", "p"} {
			screens[k] = press(m, k)
		}
		screens["help"] = press(m, "?")
		screens["filters"] = press(m, "F")
		screens["ssh"] = press(m, "S")
		for name, sm := range screens {
			lines := strings.Split(sm.View(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s at %v: %d lines, want %d", name, size, len(lines), size[1])
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w > size[0] {
					t.Errorf("%s at %v: line %d is %d wide", name, size, i, w)
				}
			}
		}
	}
}

// With the default demo page size the 100+ sample machines span five pages.
func TestDemoHasOverAHundredMachinesInFivePagesOfTwentyFive(t *testing.T) {
	m := start(t, aws.NewDemo(25), 150, 40)
	mustContain(t, m, "Press n to load more...")
	m = press(m, "n", "n", "n", "n")
	mustContain(t, m, "all loaded")
	mustNotContain(t, m, "to load more...")
	if !regexp.MustCompile(`Instances\[1\d\d\]`).MatchString(m.View()) { // 100+ running machines, counted exactly now
		t.Errorf("the demo should hold 100+ running machines:\n%s", m.View())
	}
}

func TestFilterDialogAcceptsOrAndNot(t *testing.T) {
	m := demo(t)
	m = press(m, "F", "up", "up", "up", "up", "up", "up") // Instance Name
	m = typeText(m, "(web-prod|api-prod) !web-prod-4")
	m = press(m, "enter")
	mustContain(t, m, "web-prod-1", "web-prod-3", "api-prod-1", "name=(web-prod|api-prod) !web-prod-4")
	if n := strings.Count(m.View(), "web-prod-4"); n != 1 { // only inside the filter chip, never as a row
		t.Errorf("web-prod-4 must be excluded from the rows (seen %d times):\n%s", n, m.View())
	}
	mustNotContain(t, m, "bastion-01")
}

// The first page shows at once, the next few arrive by themselves, and opening
// the search box brings in the rest so a search sees (nearly) everything.
func TestBackgroundPagesThenSearchLoadsTheRest(t *testing.T) {
	m := startWith(t, aws.NewDemo(10), 150, 40, func(c *config.Config) { c.Prefetch, c.SearchPrefetch = 3, 10 })
	mustContain(t, m, "Press n to load more...", "Instances[40+]") // page 1 plus three background pages; the rest still missing
	m = press(m, "/")
	if strings.Contains(m.View(), "to load more...") || strings.Contains(m.View(), "+]") {
		t.Errorf("opening search should have loaded everything:\n%s", m.View())
	}
	m = typeText(m, "batch-worker-2")
	mustContain(t, m, "batch-worker-25") // it lived on a page that only the search brought in
}
