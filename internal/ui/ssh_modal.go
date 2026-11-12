package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/sshx"
)

const authTimeout = 20 * time.Second

// sshModal lets the user pick the user, IP and method, preview
// the command, then hand the terminal to ssh. When the provider can push a
// temporary key (e.g. EC2 Instance Connect on AWS) or a tunnel is running, those
// choices appear as extra fields. tab moves between fields, left/right change a choice.
type sshModal struct {
	host   provider.Host
	user   textinput.Model
	public bool // true = use public IP
	both   bool // both IPs available -> user can switch
	pemDir string

	auth    provider.Authorizer // nil when the provider has no temporary-key service
	viaAuth bool                // connect with a temporary key pushed through auth

	tunnels   []sshx.LocalTunnel // tunnels the user already runs that lead to this host
	viaTunnel bool               // connect through tunnels[0] instead of directly

	focus int // index into fields()
}

// sshField is one row of the dialog.
type sshField int

const (
	fieldUser sshField = iota
	fieldAddress
	fieldMethod
	fieldRoute
)

// fields lists the rows that apply to this host, in tab order.
func (sm *sshModal) fields() []sshField {
	out := []sshField{fieldUser}
	if sm.both {
		out = append(out, fieldAddress)
	}
	if sm.auth != nil {
		out = append(out, fieldMethod)
	}
	if len(sm.tunnels) > 0 {
		out = append(out, fieldRoute)
	}
	return out
}

func (sm *sshModal) current() sshField {
	f := sm.fields()
	return f[min(sm.focus, len(f)-1)]
}

func (sm *sshModal) move(d int) tea.Cmd {
	n := len(sm.fields())
	sm.focus = (sm.focus + d + n) % n
	if sm.current() == fieldUser {
		return sm.user.Focus()
	}
	sm.user.Blur()
	return nil
}

func (sm *sshModal) toggle(f sshField) {
	switch f {
	case fieldAddress:
		sm.public = !sm.public
	case fieldMethod:
		sm.viaAuth = !sm.viaAuth
	case fieldRoute:
		sm.viaTunnel = !sm.viaTunnel
	}
}

func newSSHModal(h provider.Host, cfg config.Config, auth provider.Authorizer, tunnels []sshx.LocalTunnel) (*sshModal, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 30
	in.Placeholder = "(ssh default)"
	in.SetValue(h.DefaultUser) // the provider fills this in from the machine image; blank = ssh default
	in.CursorEnd()
	sm := &sshModal{
		host: h, user: in, both: h.PrivateIP != "" && h.PublicIP != "",
		pemDir:  config.ExpandHome(cfg.PemFilesPath),
		auth:    auth,
		viaAuth: auth != nil && cfg.SSHMethod == config.SSHMethodTemporaryKey,
		tunnels: tunnels,
	}
	switch {
	case sm.both:
		sm.public = cfg.DefaultIP == "public"
	default:
		sm.public = h.PrivateIP == ""
	}
	return sm, sm.user.Focus()
}

func (sm *sshModal) pref() string {
	if sm.public {
		return "public"
	}
	return "private"
}

func (sm *sshModal) username() string { return strings.TrimSpace(sm.user.Value()) }

// withTunnel points t at the running tunnel when the user chose it. The host
// key is stored under the instance, not under 127.0.0.1:<port>, so known_hosts
// stays valid across tunnels on different ports.
func (sm *sshModal) withTunnel(t sshx.Target) sshx.Target {
	if !sm.viaTunnel || len(sm.tunnels) == 0 {
		return t
	}
	alias := sm.host.ID
	if alias == "" {
		alias = sm.host.PrivateIP
	}
	t.Address, t.Port = "127.0.0.1", sm.tunnels[0].Port
	t.Options = append(t.Options, "HostKeyAlias="+alias, "ServerAliveInterval=5")
	return t
}

// target resolves the address, and the key file for the plain ssh method.
func (sm *sshModal) target() (sshx.Target, error) {
	t, err := sshx.Resolve(sm.host, sm.username(), sm.pref(), sm.pemDir)
	return sm.withTunnel(t), err
}

func (sm *sshModal) key(m *Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return true, nil
	case "tab", "down":
		return false, sm.move(1)
	case "shift+tab", "up":
		return false, sm.move(-1)
	case "left", "right", " ":
		if sm.current() != fieldUser { // on a choice these change it; in the user box they edit
			sm.toggle(sm.current())
			return false, nil
		}
	case "enter":
		if sm.viaAuth {
			return true, tea.Batch(m.setStatus("pushing a temporary key via "+sm.auth.AuthLabel()+"..."), sm.authorize())
		}
		t, err := sm.target()
		if err != nil {
			return true, m.setStatus("error: " + err.Error())
		}
		// the terminal is handed to ssh on a cleared screen
		return true, execSession(m.command(t), func(err error) tea.Msg { return sshDoneMsg{err} })
	}
	if sm.current() != fieldUser {
		return false, nil // typing only goes into the user box
	}
	var cmd tea.Cmd
	sm.user, cmd = sm.user.Update(msg)
	return false, cmd
}

// authorize generates a throwaway key, has the provider install it, and
// returns the target to connect with.
func (sm *sshModal) authorize() tea.Cmd {
	host, user, pref, auth := sm.host, sm.username(), sm.pref(), sm.auth
	snapshot := *sm // withTunnel reads the toggles; the dialog is gone by the time this runs
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
		defer cancel()
		key, err := sshx.NewTempKey(ctx)
		if err != nil {
			return tempKeyMsg{err: err}
		}
		if err := auth.AuthorizeSSH(ctx, host, user, key.Public); err != nil {
			return tempKeyMsg{key: key, err: err}
		}
		host.KeyName = "" // the temporary key replaces any pem file
		t, err := sshx.Resolve(host, user, pref, "")
		if err != nil {
			return tempKeyMsg{key: key, err: err}
		}
		t.KeyFile, t.OnlyKey = key.Path, true
		return tempKeyMsg{target: snapshot.withTunnel(t), key: key}
	}
}

// addr is the address a direct connection would use, for display.
func (sm *sshModal) addr() string {
	if sm.public {
		return sm.host.PublicIP
	}
	return sm.host.PrivateIP
}

func radio(on bool, label, detail string) string {
	mark := "( )"
	if on {
		mark = hintKeyStyle.Render("(•)")
	}
	return mark + " " + label + " " + hintDescStyle.Render(detail)
}

func (sm *sshModal) preview() string {
	if sm.viaAuth {
		h := sm.host
		h.KeyName = ""
		t, err := sshx.Resolve(h, sm.username(), sm.pref(), "")
		if err != nil {
			return errorStyle.Render(err.Error())
		}
		t.KeyFile, t.OnlyKey = "<temporary key>", true
		return statusStyle.Render("ssh " + strings.Join(sm.withTunnel(t).Args(), " "))
	}
	t, err := sm.target()
	if err != nil {
		return errorStyle.Render(err.Error())
	}
	return statusStyle.Render("ssh " + strings.Join(t.Args(), " "))
}

func (sm *sshModal) view(w, h int) string {
	label := func(f sshField, text string) string {
		if sm.current() == f {
			return hintKeyStyle.Render("▶ ") + headSortStyle.Render(text)
		}
		return "  " + lipgloss.NewStyle().Foreground(colAccent).Render(text)
	}
	body := []string{"", label(fieldUser, "User"), "  " + sm.user.View(), ""}
	if sm.both {
		body = append(body, label(fieldAddress, "Address"),
			"  "+radio(!sm.public, "Private", sm.host.PrivateIP)+"   "+radio(sm.public, "Public", sm.host.PublicIP), "")
	}
	if sm.auth != nil {
		body = append(body, label(fieldMethod, "Method"),
			"  "+radio(!sm.viaAuth, "SSH key", "your keys / agent")+"   "+radio(sm.viaAuth, sm.auth.AuthLabel(), "temporary key"), "")
	}
	if len(sm.tunnels) > 0 {
		body = append(body, label(fieldRoute, "Route"),
			"  "+radio(!sm.viaTunnel, "Direct", sm.addr())+"   "+radio(sm.viaTunnel, "Running tunnel", "127.0.0.1:"+strconv.Itoa(sm.tunnels[0].Port)), "")
	}
	hints := "enter connect · esc cancel"
	if len(sm.fields()) > 1 {
		hints = "tab next · ← → change · " + hints
	}
	body = append(body, "  "+lipgloss.NewStyle().Foreground(colAccent).Render("Command"), "  "+sm.preview(), "", hintDescStyle.Render("  "+hints))
	bw := 0
	for _, l := range body {
		bw = max(bw, lipgloss.Width(l))
	}
	bw = min(max(bw+4, 50), w)
	name := sm.host.Name
	if name == "" {
		name = sm.host.ID
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(colPrimary).Render("SSH => " + name)
	return placeCenter(panel(title, body, bw, len(body)+2), w, h)
}
