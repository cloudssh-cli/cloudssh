package ui

import (
	"context"
	"strings"

	"github.com/cloudssh-cli/cloudssh/internal/provider"
)

// profilesResource lists the sign-in profiles configured on this machine as
// an ordinary table screen, so search, sorting and refresh work as everywhere
// else. Enter on a row switches the whole app to that profile.
type profilesResource struct{ auth provider.Authenticator }

type profileRef struct{ Name string }

func (profilesResource) Key() string                       { return "profiles" }
func (profilesResource) Label() string                     { return "Profiles" }
func (profilesResource) Name() string                      { return "Profiles" }
func (profilesResource) Crumb() string                     { return "profiles" }
func (profilesResource) Filters() []provider.FilterDef     { return nil }
func (profilesResource) DefaultFilters() []provider.Filter { return nil }

func (profilesResource) Columns() []provider.Column {
	return []provider.Column{
		{Title: "Active"}, {Title: "Profile", DefaultSort: "asc"}, {Title: "Auth"}, {Title: "Region"}, {Title: "Details"},
	}
}

func (r profilesResource) List(_ context.Context, _ provider.Query) (provider.Page, error) {
	current := ""
	if r.auth.AuthError() == nil {
		current = r.auth.Profile()
	}
	var rows []provider.Row
	for _, p := range r.auth.Profiles() {
		mark := ""
		if p.Name == current {
			mark = "●"
		}
		rows = append(rows, provider.Row{Cells: []string{mark, p.Name, p.Kind, p.Region, p.Detail}, Ref: profileRef{Name: p.Name}})
	}
	return provider.Page{Rows: rows}, nil
}

func isProfiles(s *screen) bool {
	_, ok := s.res.(*profilesResource)
	return ok
}

// authHeadline is the first line of the friendly auth explanation.
func authHeadline(err error) string {
	if err == nil {
		return ""
	}
	first, _, _ := strings.Cut(err.Error(), "\n")
	return first
}
