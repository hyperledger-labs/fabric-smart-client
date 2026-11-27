/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hyperledger-labs/fabric-smart-client/examples/chat"
	viewservice "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Run shows the chat of user me until the user quits. It logs in first and logs out last.
func Run(mgr *viewservice.Manager, inbox *chat.Inbox, me string) error {
	input := textinput.New()
	input.Placeholder = "type a message or /join <channel>"
	input.Focus()
	m := &model{mgr: mgr, inbox: inbox, me: me, input: input, viewport: viewport.New(80, 20)}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

type model struct {
	mgr      *viewservice.Manager
	inbox    *chat.Inbox
	me, org  string
	channels []string
	current  string
	lines    []string
	viewport viewport.Model
	input    textinput.Model
}

// The results of the commands the model runs in the background.
type (
	lineMsg    string
	errMsg     struct{ err error }
	loginMsg   []chat.User
	joinedMsg  string
	leftMsg    string
	inboxMsg   []chat.Message
	logoutDone struct{}
)

func (m *model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.waitInbox(), m.request(chat.OpLogin, "", "", func(r *chat.Reply) tea.Msg { return loginMsg(r.Users) }))
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width, m.viewport.Height = msg.Width, max(msg.Height-3, 1)
		m.input.Width = msg.Width - 4
		m.refresh()
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, m.logout()
		case tea.KeyEnter:
			line := m.input.Value()
			m.input.Reset()
			return m, m.run(line)
		}
	case lineMsg:
		m.add(string(msg))
	case errMsg:
		m.add("! " + msg.err.Error())
	case loginMsg:
		for _, u := range msg {
			if u.Name == m.me {
				m.org = u.Org
			}
		}
		m.add(fmt.Sprintf("logged in as %s; %s", m.me, usage))
	case joinedMsg:
		if !slices.Contains(m.channels, string(msg)) {
			m.channels = append(m.channels, string(msg))
		}
		m.current = string(msg)
		m.add("joined #" + string(msg))
	case leftMsg:
		m.channels = slices.DeleteFunc(m.channels, func(c string) bool { return c == string(msg) })
		if m.current == string(msg) {
			m.current = ""
			if len(m.channels) > 0 {
				m.current = m.channels[len(m.channels)-1]
			}
		}
		m.add("left #" + string(msg))
	case inboxMsg:
		for _, in := range msg {
			if in.Channel == "" {
				m.add(fmt.Sprintf("[dm] %s: %s", in.From, in.Text))
			} else {
				m.add(fmt.Sprintf("[#%s] %s: %s", in.Channel, in.From, in.Text))
			}
		}
		return m, m.waitInbox()
	case logoutDone:
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *model) View() string {
	var header strings.Builder
	for _, c := range m.channels {
		if c == m.current {
			fmt.Fprintf(&header, "[#%s] ", c)
		} else {
			fmt.Fprintf(&header, " #%s  ", c)
		}
	}
	fmt.Fprintf(&header, "  %s (%s)", m.me, m.org)
	return header.String() + "\n" + m.viewport.View() + "\n" + m.input.View()
}

// run turns an input line into the command that carries it out.
func (m *model) run(line string) tea.Cmd {
	c, err := Parse(line)
	if err != nil {
		return msgCmd(errMsg{err})
	}
	switch c.Kind {
	case KindSay:
		if m.current == "" {
			return msgCmd(lineMsg("! join a channel first: /join <channel>"))
		}
		channel := m.current
		echo := fmt.Sprintf("[#%s] %s: %s", channel, m.me, c.Text)
		return m.request(chat.OpPost, channel, c.Text, func(r *chat.Reply) tea.Msg {
			if len(r.Failed) > 0 {
				return lineMsg(echo + "  (not delivered to " + strings.Join(r.Failed, ", ") + ")")
			}
			return lineMsg(echo)
		})
	case KindJoin:
		return m.request(chat.OpJoin, c.Arg, "", func(*chat.Reply) tea.Msg { return joinedMsg(c.Arg) })
	case KindLeave:
		channel := c.Arg
		if channel == "" {
			channel = m.current
		}
		if channel == "" {
			return msgCmd(lineMsg("! not in a channel"))
		}
		return m.request(chat.OpLeave, channel, "", func(*chat.Reply) tea.Msg { return leftMsg(channel) })
	case KindMsg:
		return m.call(&chat.DM{To: c.Arg, Text: c.Text}, func(any) tea.Msg {
			return lineMsg(fmt.Sprintf("[dm to %s] %s: %s", c.Arg, m.me, c.Text))
		})
	case KindWho:
		return m.request(chat.OpWho, "", "", func(r *chat.Reply) tea.Msg {
			var users []string
			for _, u := range r.Users {
				users = append(users, fmt.Sprintf("%s (%s)", u.Name, u.Org))
			}
			return lineMsg("online: " + strings.Join(users, ", "))
		})
	default: // KindQuit
		return m.logout()
	}
}

// request sends a Request to the server in the background and turns the reply into a message.
func (m *model) request(op chat.Op, channel, text string, done func(*chat.Reply) tea.Msg) tea.Cmd {
	return m.call(&chat.Call{Op: op, Channel: channel, Text: text}, func(res any) tea.Msg {
		reply, _ := res.(*chat.Reply)
		if reply == nil {
			reply = &chat.Reply{}
		}
		return done(reply)
	})
}

// call runs v on the node in the background; errors become a line in the chat.
func (m *model) call(v view.View, done func(any) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		res, err := m.mgr.InitiateView(context.Background(), v)
		if err != nil {
			return errMsg{err}
		}
		return done(res)
	}
}

// logout logs out and quits; it quits even when the server cannot be reached.
func (m *model) logout() tea.Cmd {
	return func() tea.Msg {
		_, _ = m.mgr.InitiateView(context.Background(), &chat.Call{Op: chat.OpLogout})
		return logoutDone{}
	}
}

func (m *model) waitInbox() tea.Cmd {
	return func() tea.Msg {
		<-m.inbox.Notify()
		return inboxMsg(m.inbox.Drain())
	}
}

func (m *model) add(line string) {
	m.lines = append(m.lines, line)
	m.refresh()
}

func (m *model) refresh() {
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	m.viewport.GotoBottom()
}

func msgCmd(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
