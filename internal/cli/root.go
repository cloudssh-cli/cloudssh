// Package cli wires flags, config and the TUI together. It is cloud-agnostic:
// the cloud is picked by name from the registry in internal/cloud, whose
// backends register themselves (see clouds.go).
package cli

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/SVMadhavaReddy/cloudssh/internal/cloud"
	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/ui"
	"github.com/SVMadhavaReddy/cloudssh/internal/version"
)

// demoPageSize: the sample fleet then spans two pages, enough to try paging and search.
const demoPageSize = 25

func NewRootCmd() *cobra.Command {
	var (
		cfgPath                                     string
		cloudName, profile, region, view, pems, ssh string
		pageSize                                    int
		demo                                        bool
	)
	cmd := &cobra.Command{
		Use:           "cloudssh",
		Short:         "Browse cloud resources and SSH into them from a terminal UI",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Version,
		RunE: func(_ *cobra.Command, _ []string) error {
			explicit := cfgPath != ""
			if !explicit {
				p, err := config.DefaultPath()
				if err != nil {
					return err
				}
				cfgPath = p
			}
			cfg, err := config.Load(cfgPath, explicit)
			if err != nil {
				return err
			}
			if cloudName != "" {
				cfg.Cloud = cloudName
			}
			if profile != "" {
				cfg.Profile = profile
			}
			if region != "" {
				cfg.Region = region
			}
			if view != "" {
				cfg.View = view
			}
			if ssh != "" {
				if !config.ValidSSHMethod(ssh) {
					return fmt.Errorf("--ssh-method must be %q or %q, got %q", config.SSHMethodSSH, config.SSHMethodTemporaryKey, ssh)
				}
				cfg.SSHMethod = ssh
			}
			if pems != "" {
				cfg.PemFilesPath = pems
			}
			cfg.PageSize = effectivePageSize(pageSize, demo, cfg.PageSize)

			prov, err := cloud.Open(context.Background(), cfg.Cloud, provider.Options{
				Profile: cfg.Profile, Region: cfg.Region, PageSize: cfg.PageSize, Demo: demo,
			})
			if err != nil {
				return err
			}
			if err := validateView(cfg.View, prov); err != nil {
				return err
			}
			_, err = tea.NewProgram(ui.New(cfg, prov), tea.WithAltScreen()).Run()
			return err
		},
	}
	f := cmd.Flags()
	f.StringVarP(&cfgPath, "config", "c", "", "path to config file (default: $XDG_CONFIG_HOME/cloudssh/config.yaml)")
	f.StringVar(&cloudName, "cloud", "", "cloud to use: "+strings.Join(cloud.Names(), ", ")+" (default aws)")
	f.StringVar(&profile, "profile", "", "sign-in profile (what a profile means depends on the cloud)")
	f.StringVar(&region, "region", "", "region (cloud specific)")
	f.BoolVar(&demo, "demo", false, "run on built-in sample data: no cloud account or network needed, ssh opens a harmless local shell")
	f.StringVar(&ssh, "ssh-method", "", fmt.Sprintf("default connection method: %q (your keys) or %q (a short-lived key pushed through the cloud, when offered)", config.SSHMethodSSH, config.SSHMethodTemporaryKey))
	f.StringVar(&pems, "pem-files-path", "", "directory holding <key-pair-name>.pem files")
	f.IntVar(&pageSize, "page-size", 0, "items per page (minimum 5)")
	f.StringVar(&view, "view", "", "screen to open first: dashboard or a resource key of the chosen cloud (default: its first resource)")
	cmd.SetVersionTemplate(fmt.Sprintf("cloudssh %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date))
	return cmd
}

// validateView checks that view is empty, "dashboard" or one of the provider's resources.
func validateView(view string, p provider.Provider) error {
	if view == "" || view == "dashboard" {
		return nil
	}
	keys := []string{"dashboard"}
	for _, r := range p.Resources() {
		if r.Key() == view {
			return nil
		}
		keys = append(keys, r.Key())
	}
	return fmt.Errorf("view %q not found for %s (choose one of: %s)", view, p.Name(), strings.Join(keys, ", "))
}

// effectivePageSize: an explicit --page-size wins (minimum 5), demo mode
// defaults to demoPageSize, otherwise the configured value stands.
func effectivePageSize(flag int, demo bool, configured int32) int32 {
	switch {
	case flag > 0:
		return int32(max(flag, 5)) //nolint:gosec // bounded by a CLI int, API caps apply later
	case demo:
		return demoPageSize
	}
	return configured
}
