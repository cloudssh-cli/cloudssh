package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/SVMadhavaReddy/cloudssh/internal/config"
	"github.com/SVMadhavaReddy/cloudssh/internal/provider"
	"github.com/SVMadhavaReddy/cloudssh/internal/sshx"
)

// sshModal is the Python app's SshScreen: pick user and IP, preview the
// command, then hand the terminal to ssh.
type sshModal struct {
	host   provider.Host
	user   textinput.Model
	public bool // true = use public IP
	both   bool // both IPs available -> user can switch
	pemDir string
}

func newSSHModal(h provider.Host, cfg config.Config) (*sshModal, tea.Cmd) {
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
		pemDir: config.ExpandHome(cfg.PemFilesPath),
	}
	switch {
	case sm.both:
		sm.public = cfg.DefaultIP == "public"
	default:
		sm.public = h.PrivateIP == ""
	}
	return sm, sm.user.Focus()
}

func (sm *sshModal) target() (sshx.Target, error) {
	pref := "private"
	if sm.public {
		pref = "public"
	}
	return sshx.Resolve(sm.host, strings.TrimSpace(sm.user.Value()), pref, sm.pemDir)
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
	case "enter":
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

func radio(on bool, label, ip string) string {
	mark := "( )"
	if on {
		mark = hintKeyStyle.Render("(•)")
	}
	return mark + " " + label + " " + hintDescStyle.Render(ip)
}

func (sm *sshModal) view(w, h int) string {
	label := lipgloss.NewStyle().Foreground(colAccent)
	body := []string{"", label.Render(" Connect using user:"), " " + sm.user.View(), ""}
	if sm.both {
		body = append(body, label.Render(" Connect using IP:"),
			" "+radio(!sm.public, "Private", sm.host.PrivateIP)+"   "+radio(sm.public, "Public", sm.host.PublicIP), "")
	}
	var preview string
	if t, err := sm.target(); err != nil {
		preview = errorStyle.Render(err.Error())
	} else {
		preview = statusStyle.Render("ssh " + strings.Join(t.Args(), " "))
	}
	hints := "enter connect · esc cancel"
	if sm.both {
		hints += " · tab switch IP"
	}
	body = append(body, label.Render(" Final ssh command preview:"), " "+preview, "", hintDescStyle.Render(" "+hints))
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
