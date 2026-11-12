// Package cli wires flags, config and the TUI together.
package cli

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider/aws"
	"github.com/SVMadhavaReddy/cloudssh/internal/ui"
	"github.com/SVMadhavaReddy/cloudssh/internal/version"
)

func NewRootCmd() *cobra.Command {
	var (
		cfgPath                            string
		cloud, profile, region, view, pems string
		pageSize                           int
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
			if cloud != "" {
				cfg.Cloud = cloud
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
			if pems != "" {
				cfg.PemFilesPath = pems
			}
			if pageSize > 0 {
				cfg.PageSize = int32(max(pageSize, 5)) //nolint:gosec // bounded by a CLI int, API caps apply later
			}

			var prov provider.Provider
			switch cfg.Cloud {
			case "aws":
				prov, err = aws.New(context.Background(), cfg.Profile, cfg.Region, cfg.PageSize)
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("cloud %q is not supported yet", cfg.Cloud)
			}
			if err := validateView(cfg.View, prov); err != nil {
				return err
			}
			_, err = tea.NewProgram(ui.New(cfg, prov), tea.WithAltScreen()).Run()
			return err
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "path to config file (default: $XDG_CONFIG_HOME/cloudssh/config.yaml)")
	cmd.Flags().StringVar(&cloud, "cloud", "", "cloud provider (aws)")
	cmd.Flags().StringVar(&profile, "profile", "", "AWS profile")
	cmd.Flags().StringVar(&region, "region", "", "AWS region")
	cmd.Flags().StringVar(&pems, "pem-files-path", "", "directory holding <key-pair-name>.pem files")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "items per page (minimum 5)")
	cmd.Flags().StringVar(&view, "view", "", "screen to open first: dashboard, ec2, ecs, eks, asg, vpc (default ec2)")
	cmd.SetVersionTemplate(fmt.Sprintf("cloudssh %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date))
	return cmd
}

// validateView checks that view is "dashboard" or one of the provider's resources.
func validateView(view string, p provider.Provider) error {
	keys := []string{"dashboard"}
	for _, r := range p.Resources() {
		if r.Key() == view {
			return nil
		}
		keys = append(keys, r.Key())
	}
	if view == "dashboard" {
		return nil
	}
	return fmt.Errorf("view %q not found for %s (choose one of: %s)", view, p.Name(), strings.Join(keys, ", "))
}
