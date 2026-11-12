package ui

import (
	"context"
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

// sshModal is the Python app's SshScreen: pick user, IP and method, preview
// the command, then hand the terminal to ssh. When the provider can push a
// temporary key (EC2 Instance Connect) the method can be switched with ctrl+t.
type sshModal struct {
	host   provider.Host
	user   textinput.Model
	public bool // true = use public IP
	both   bool // both IPs available -> user can switch
	pemDir string

	auth    provider.Authorizer // nil when the provider has no temporary-key service
	viaAuth bool                // connect with a temporary key pushed through auth
}

func newSSHModal(h provider.Host, cfg config.Config, auth provider.Authorizer) (*sshModal, tea.Cmd) {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 30
	in.Placeholder = "Username"
	user := h.DefaultUser
	if user == "" {
		user = "ec2-user"
	}
	in.SetValue(user)
	in.CursorEnd()
	sm := &sshModal{
		host: h, user: in, both: h.PrivateIP != "" && h.PublicIP != "",
		pemDir:  config.ExpandHome(cfg.PemFilesPath),
		auth:    auth,
		viaAuth: auth != nil && cfg.SSHMethod == "instance-connect",
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

// target resolves the address, and the key file for the plain ssh method.
func (sm *sshModal) target() (sshx.Target, error) {
	return sshx.Resolve(sm.host, sm.username(), sm.pref(), sm.pemDir)
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
	case "enter":
		if sm.viaAuth {
			return true, tea.Batch(m.setStatus("pushing a temporary key via "+sm.auth.AuthLabel()+"..."), sm.authorize())
		}
		t, err := sm.target()
		if err != nil {
			return true, m.setStatus("error: " + err.Error())
		}
		// ExecProcess suspends the TUI and gives ssh the real terminal.
		return true, tea.ExecProcess(t.Command(), func(err error) tea.Msg { return sshDoneMsg{err} })
	}
	var cmd tea.Cmd
	sm.user, cmd = sm.user.Update(msg)
	return false, cmd
}

// authorize generates a throwaway key, has the provider install it, and
// returns the target to connect with.
func (sm *sshModal) authorize() tea.Cmd {
	host, user, pref, auth := sm.host, sm.username(), sm.pref(), sm.auth
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
		defer cancel()
		key, err := sshx.NewTempKey(ctx)
		if err != nil {
			return eicMsg{err: err}
		}
		if err := auth.AuthorizeSSH(ctx, host, user, key.Public); err != nil {
			return eicMsg{key: key, err: err}
		}
		host.KeyName = "" // the temporary key replaces any pem file
		t, err := sshx.Resolve(host, user, pref, "")
		if err != nil {
			return eicMsg{key: key, err: err}
		}
		t.KeyFile, t.OnlyKey = key.Path, true
		return eicMsg{target: t, key: key}
	}
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
		return statusStyle.Render("ssh " + strings.Join(t.Args(), " "))
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
	hints := "enter connect · esc cancel"
	if sm.both {
		hints += " · tab switch IP"
	}
	if sm.auth != nil {
		hints += " · ctrl+t switch method"
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
