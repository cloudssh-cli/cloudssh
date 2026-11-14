// Package cli wires flags, config and the TUI together. It is cloud-agnostic:
// the cloud is picked by name from the registry in internal/cloud, whose
// backends register themselves (see clouds.go).
package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/cloudssh-cli/cloudssh/internal/cloud"
	"github.com/cloudssh-cli/cloudssh/internal/config"
	"github.com/cloudssh-cli/cloudssh/internal/logx"
	"github.com/cloudssh-cli/cloudssh/internal/provider"
	"github.com/cloudssh-cli/cloudssh/internal/ui"
	"github.com/cloudssh-cli/cloudssh/internal/version"
)

// demoPageSize: with 100+ sample machines this is five pages, so even after the
// background pages load there is more to ask for with n, and search has more to widen.
const demoPageSize = 25

func NewRootCmd() *cobra.Command {
	var (
		cfgPath                                                     string
		cloudName, profile, region, view, pems, ssh, theme, logFile string
		pageSize                                                    int
		demo, logDebug                                              bool
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
			if theme != "" {
				cfg.Theme = theme
			}
			if logFile == "" {
				logFile = os.Getenv("CLOUDSSH_LOG")
			}
			if logFile != "" {
				cfg.LogFile = logFile
			}
			cfg.LogDebug = cfg.LogDebug || logDebug
			closeLog, err := logx.Init(config.ExpandHome(cfg.LogFile), cfg.LogDebug)
			if err != nil {
				return fmt.Errorf("log file: %w", err)
			}
			defer closeLog()
			logx.L().Info("start", "version", version.Version, "cloud", cfg.Cloud, "demo", demo, "pem_files_path", cfg.PemFilesPath)
			if dir, err := config.ThemesDir(); err == nil {
				ui.LoadThemes(dir)
			}
			if err := ui.SetTheme(cfg.Theme); err != nil {
				return err
			}

			if !slices.Contains(cloud.Names(), cfg.Cloud) { // fail before the TUI takes over the terminal
				return fmt.Errorf("cloud %q is not supported (available: %s)", cfg.Cloud, strings.Join(cloud.Names(), ", "))
			}
			open := func() (provider.Provider, error) {
				began := time.Now()
				prov, err := cloud.Open(context.Background(), cfg.Cloud, provider.Options{
					Profile: cfg.Profile, Region: cfg.Region, PageSize: cfg.PageSize, Demo: demo,
				})
				if err == nil {
					err = validateView(cfg.View, prov)
				}
				logx.L().Info("provider opened", "took", time.Since(began).Round(time.Millisecond), "err", err)
				return prov, err
			}
			// The TUI starts first and signs in behind the splash: opening a
			// cloud can take seconds (SSO, token refresh) and must not leave
			// a blank terminal.
			final, err := tea.NewProgram(ui.NewBoot(cfg, open, demo), tea.WithAltScreen()).Run()
			if err != nil {
				return err
			}
			if b, ok := final.(ui.Boot); ok {
				return b.Err()
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&cfgPath, "config", "c", "", "path to config file (default: $XDG_CONFIG_HOME/cloudssh/config.yaml)")
	f.StringVar(&cloudName, "cloud", "", "cloud to use: "+strings.Join(cloud.Names(), ", ")+" (default aws)")
	f.StringVar(&profile, "profile", "", "sign-in profile (what a profile means depends on the cloud)")
	f.StringVar(&region, "region", "", "region (cloud specific)")
	f.BoolVar(&demo, "demo", false, "run on built-in sample data: no cloud account or network needed, ssh opens a harmless local shell")
	f.StringVar(&ssh, "ssh-method", "", fmt.Sprintf("default connection method: %q (your keys) or %q (a short-lived key pushed through the cloud, when offered)", config.SSHMethodSSH, config.SSHMethodTemporaryKey))
	f.StringVar(&logFile, "log-file", "", "write a debug log (sign-in timing, errors, ssh commands) to this file; also $CLOUDSSH_LOG or log_file in the config")
	f.BoolVar(&logDebug, "debug", false, "log more detail (with --log-file)")
	f.StringVar(&pems, "pem-files-path", "", "directory holding <key-pair-name>.pem files")
	f.IntVar(&pageSize, "page-size", 0, "items per page (minimum 5)")
	f.StringVar(&theme, "theme", "", "colour theme: "+strings.Join(ui.ThemeNames(), ", ")+" (default auto: follows the terminal background, dark if unknown); your own: <config dir>/themes/<name>.yaml")
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
