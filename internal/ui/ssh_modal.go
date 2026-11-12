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
// temporary key (e.g. EC2 Instance Connect on AWS) the method can be switched with ctrl+t.
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
	case "tab":
		if sm.both {
			sm.public = !sm.public
		}
		return false, nil
	case "ctrl+t":
		if sm.auth != nil {
			sm.viaAuth = !sm.viaAuth
		}
		return false, nil
	case "ctrl+o":
		if len(sm.tunnels) > 0 {
			sm.viaTunnel = !sm.viaTunnel
		}
		return false, nil
	case "enter":
		if sm.viaAuth {
			return true, tea.Batch(m.setStatus("pushing a temporary key via "+sm.auth.AuthLabel()+"..."), sm.authorize())
		}
		t, err := sm.target()
		if err != nil {
			return true, m.setStatus("error: " + err.Error())
		}
		// ExecProcess suspends the TUI and gives ssh the real terminal.
		return true, execSession(m.command(t), func(err error) tea.Msg { return sshDoneMsg{err} })
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
	label := lipgloss.NewStyle().Foreground(colAccent)
	body := []string{"", label.Render(" Connect using user:"), " " + sm.user.View(), ""}
	if sm.both {
		body = append(body, label.Render(" Connect using IP:"),
			" "+radio(!sm.public, "Private", sm.host.PrivateIP)+"   "+radio(sm.public, "Public", sm.host.PublicIP), "")
	}
	if sm.auth != nil {
		body = append(body, label.Render(" Connect using:"),
			" "+radio(!sm.viaAuth, "SSH key", "your keys / agent")+"   "+radio(sm.viaAuth, sm.auth.AuthLabel(), "temporary key"), "")
	}
	if len(sm.tunnels) > 0 {
		tun := sm.tunnels[0]
		body = append(body, label.Render(" Connect via:"),
			" "+radio(!sm.viaTunnel, "Direct", sm.addr())+"   "+radio(sm.viaTunnel, "Running tunnel", "127.0.0.1:"+strconv.Itoa(tun.Port)), "")
	}
	hints := "enter connect · esc cancel"
	if sm.both {
		hints += " · tab switch IP"
	}
	if sm.auth != nil {
		hints += " · ctrl+t switch method"
	}
	if len(sm.tunnels) > 0 {
		hints += " · ctrl+o use tunnel"
	}
	body = append(body, label.Render(" Final ssh command preview:"), " "+sm.preview(), "", hintDescStyle.Render(" "+hints))
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
