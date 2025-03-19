package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alesr/chachacha/internal/config"
	"github.com/alesr/chachacha/internal/events"
	"github.com/alesr/chachacha/internal/sessionrepo"
	pubevts "github.com/alesr/chachacha/pkg/events"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/rabbitmq/amqp091-go"
)

var (
	appStyle          = lipgloss.NewStyle().Margin(1, 2)
	titleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#D3D3D3")).Padding(0, 1).Bold(true)
	subtitleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#666666")).Padding(0, 1)
	statusBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#333333")).Padding(0, 1)
	infoStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#CCCCCC")).Padding(0, 1)
	warningStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#FFCC00")).Padding(0, 1)
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#CC0000")).Padding(0, 1)
	eventHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFCC00"))
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("#CC0000"))
)

// Event holds parsed event information for display.
type Event struct {
	Type      string
	Message   string
	Data      any
	Timestamp time.Time
}

type item struct {
	title       string
	description string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.title }

type mainMenuState int

const (
	mainMenu mainMenuState = iota
	hostRegistration
	playerRegistration
	statusView
	removeHostView
	removePlayerView
	eventView
)

type Host struct {
	HostID         string           `json:"host_id"`
	Mode           pubevts.GameMode `json:"mode"`
	AvailableSlots uint16           `json:"available_slots"`
}

type Player struct {
	PlayerID string            `json:"player_id"`
	HostID   *string           `json:"host_id,omitempty"`
	Mode     *pubevts.GameMode `json:"mode,omitempty"`
}

type GameSession struct {
	ID             string           `json:"session_id"`
	HostID         string           `json:"host_id"`
	Mode           pubevts.GameMode `json:"mode"`
	CreatedAt      time.Time        `json:"created_at"`
	Players        []string         `json:"players"`
	AvailableSlots uint16           `json:"available_slots"`
	State          string           `json:"state"`
}

type model struct {
	// global state
	cfg    *config.Config
	state  mainMenuState
	err    error
	conn   *amqp091.Connection
	ch     *amqp091.Channel
	repo   *sessionrepo.Redis
	events []Event
	timer  *time.Timer

	// UI components
	mainMenu   list.Model
	spinner    spinner.Model
	inputs     []textinput.Model
	focusIndex int
	waiting    bool

	// data
	hosts       []Host
	players     []Player
	sessions    []GameSession
	hostID      string
	playerID    string
	selectedID  string
	gameMode    string
	slots       uint16
	joinMessage string
	viewMessage string

	// channel for events
	eventChan chan Event
}

var mainProgram *tea.Program

func main() {
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	mainProgram = p

	go func() {
		for evt := range m.eventChan {
			mainProgram.Send(eventMsg{event: evt})
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}

func initialModel() model {
	cfg, err := config.Load()
	if err != nil {
		return model{err: fmt.Errorf("could not load config: %w", err)}
	}

	conn, ch, repo, err := setupConnections(cfg)
	if err != nil {
		return model{err: err}
	}

	// set up the main menu
	items := []list.Item{
		item{title: "Register a Host", description: "Create a new game host that players can join"},
		item{title: "Register a Player", description: "Register a player looking for a match"},
		item{title: "View Status", description: "Show current matchmaking state"},
		item{title: "Remove a Host", description: "Remove a host from matchmaking"},
		item{title: "Remove a Player", description: "Remove a player from matchmaking"},
		item{title: "View Events", description: "Show all received events"},
		item{title: "Quit", description: "Exit the application"},
	}

	delegate := list.NewDefaultDelegate()
	mainMenu := list.New(items, delegate, 0, 0)
	mainMenu.Title = "ChaChaChá Matchmaking Test"

	m := model{
		cfg:       cfg,
		state:     mainMenuState(0),
		conn:      conn,
		ch:        ch,
		repo:      repo,
		events:    []Event{},
		spinner:   spinner.New(),
		waiting:   false,
		mainMenu:  mainMenu,
		eventChan: make(chan Event, 100),
	}

	m.spinner.Spinner = spinner.Dot

	// set up the text inputs
	m.inputs = make([]textinput.Model, 3)
	for i := range m.inputs {
		t := textinput.New()
		t.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
		t.CharLimit = 32
		m.inputs[i] = t
	}

	// set up the event monitoring
	go setupEventObservers(ch, m.eventChan)
	return m
}

func setupConnections(cfg *config.Config) (*amqp091.Connection, *amqp091.Channel, *sessionrepo.Redis, error) {
	redisCli, err := sessionrepo.NewRedisClient(cfg.RedisAddr)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not connect to Redis: %w", err)
	}

	repo, err := sessionrepo.NewRedisRepo(redisCli)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not create Redis repo: %w", err)
	}

	conn, err := amqp091.Dial(cfg.RabbitMQURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, nil, fmt.Errorf("could not open a channel: %w", err)
	}

	if _, err := ch.QueueDeclare(cfg.QueueName, false, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, nil, fmt.Errorf("could not declare a queue: %w", err)
	}

	if _, err := events.NewPublisher(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, nil, fmt.Errorf("could not create event publisher: %w", err)
	}

	if err := events.SetupInputExchangeBindings(ch, cfg.QueueName); err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, nil, fmt.Errorf("could not set up input exchange queue bindings: %w", err)
	}

	if err := events.SetupOutputExchangeQueueBindings(ch); err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, nil, fmt.Errorf("could not set up monitoring binding queues: %w", err)
	}
	return conn, ch, repo, nil
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchInitialState(m),
	)
}

func fetchInitialState(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		hostEvents, err := m.repo.GetHosts(ctx)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not get hosts: %w", err)}
		}

		hosts := make([]Host, len(hostEvents))
		for i, h := range hostEvents {
			hosts[i] = Host{
				HostID:         h.HostID,
				Mode:           h.Mode,
				AvailableSlots: h.AvailableSlots,
			}
		}

		playerEvents, err := m.repo.GetPlayers(ctx)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not get players: %w", err)}
		}

		players := make([]Player, len(playerEvents))
		for i, p := range playerEvents {
			players[i] = Player{
				PlayerID: p.PlayerID,
				HostID:   p.HostID,
				Mode:     p.Mode,
			}
		}

		sessionEvents, err := m.repo.GetActiveGameSessions(ctx)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not get sessions: %w", err)}
		}

		sessions := make([]GameSession, len(sessionEvents))
		for i, s := range sessionEvents {
			sessions[i] = GameSession{
				ID:             s.ID,
				HostID:         s.HostID,
				Mode:           s.Mode,
				CreatedAt:      s.CreatedAt,
				Players:        s.Players,
				AvailableSlots: s.AvailableSlots,
				State:          s.State,
			}
		}
		return stateUpdateMsg{
			hosts:    hosts,
			players:  players,
			sessions: sessions,
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == mainMenu {
				return m, tea.Quit
			} else {
				m.state = mainMenu
				m.clearInputs()
				return m, fetchInitialState(m)
			}

		case "enter":
			switch m.state {
			case mainMenu:
				selected := m.mainMenu.SelectedItem()
				if i, ok := selected.(item); ok {
					switch i.title {
					case "Register a Host":
						m.state = hostRegistration
						m.setupHostRegistrationInputs()

					case "Register a Player":
						m.state = playerRegistration
						m.setupPlayerRegistrationInputs()

					case "View Status":
						m.state = statusView
						return m, fetchInitialState(m)

					case "Remove a Host":
						m.state = removeHostView
						return m, fetchInitialState(m)

					case "Remove a Player":
						m.state = removePlayerView
						return m, fetchInitialState(m)

					case "View Events":
						m.state = eventView

					case "Quit":
						return m, tea.Quit
					}
				}

			case hostRegistration:
				if m.focusIndex == len(m.inputs)-1 {
					m.hostID = m.inputs[0].Value()
					if m.hostID == "" {
						m.hostID = fmt.Sprintf("host-%d", time.Now().UnixNano())
					}

					m.gameMode = m.inputs[1].Value()
					if m.gameMode == "" {
						m.gameMode = "default"
					}

					slotsStr := m.inputs[2].Value()
					slots, err := strconv.ParseInt(slotsStr, 10, 16)
					if err != nil || slots <= 0 {
						slots = 1
					}
					m.slots = uint16(slots)

					m.waiting = true
					return m, tea.Batch(
						m.spinner.Tick,
						registerHostCmd(m),
					)
				} else {
					m.focusIndex++
					m.inputs[m.focusIndex].Focus()
					return m, nil
				}

			case playerRegistration:
				if m.focusIndex == len(m.inputs)-1 {
					m.playerID = m.inputs[0].Value()
					if m.playerID == "" {
						m.playerID = fmt.Sprintf("player-%d", time.Now().UnixNano())
					}

					m.hostID = m.inputs[1].Value()
					m.gameMode = m.inputs[2].Value()

					m.waiting = true
					return m, tea.Batch(
						m.spinner.Tick,
						registerPlayerCmd(m),
					)
				} else {
					m.focusIndex++
					m.inputs[m.focusIndex].Focus()
					return m, nil
				}

			case removeHostView:
				if m.selectedID != "" {
					m.waiting = true
					return m, tea.Batch(
						m.spinner.Tick,
						removeHostCmd(m, m.selectedID),
					)
				}

			case removePlayerView:
				if m.selectedID != "" {
					m.waiting = true
					return m, tea.Batch(
						m.spinner.Tick,
						removePlayerCmd(m, m.selectedID),
					)
				}
			}

		case "tab", "shift+tab":
			// handle tab navigation between inputs
			if m.state == hostRegistration || m.state == playerRegistration {
				if msg.String() == "tab" {
					m.focusIndex = (m.focusIndex + 1) % len(m.inputs)
				} else {
					m.focusIndex = (m.focusIndex - 1 + len(m.inputs)) % len(m.inputs)
				}
				for i := range m.inputs {
					if i == m.focusIndex {
						m.inputs[i].Focus()
					} else {
						m.inputs[i].Blur()
					}
				}
				return m, nil
			}

		// handle list selection for host/player removal
		case "up", "down":
			if m.state == removeHostView {
				if len(m.hosts) > 0 {
					index := 0
					for i, host := range m.hosts {
						if host.HostID == m.selectedID {
							index = i
							break
						}
					}

					if msg.String() == "up" {
						index = (index - 1 + len(m.hosts)) % len(m.hosts)
					} else {
						index = (index + 1) % len(m.hosts)
					}
					m.selectedID = m.hosts[index].HostID
				}
			} else if m.state == removePlayerView {
				if len(m.players) > 0 {
					index := 0
					for i, player := range m.players {
						if player.PlayerID == m.selectedID {
							index = i
							break
						}
					}

					if msg.String() == "up" {
						index = (index - 1 + len(m.players)) % len(m.players)
					} else {
						index = (index + 1) % len(m.players)
					}
					m.selectedID = m.players[index].PlayerID
				}
			} else if m.state == mainMenu {
				var cmd tea.Cmd
				if msg.String() == "up" {
					m.mainMenu, cmd = m.mainMenu.Update(msg)
				} else {
					m.mainMenu, cmd = m.mainMenu.Update(msg)
				}
				return m, cmd
			}
		}

	case stateUpdateMsg:
		m.hosts = msg.hosts
		m.players = msg.players
		m.sessions = msg.sessions

		// init selected ID for remove views
		if m.state == removeHostView && len(m.hosts) > 0 && m.selectedID == "" {
			m.selectedID = m.hosts[0].HostID
		} else if m.state == removePlayerView && len(m.players) > 0 && m.selectedID == "" {
			m.selectedID = m.players[0].PlayerID
		}

	case hostRegisteredMsg:
		m.viewMessage = fmt.Sprintf("✅ Host %s registered successfully!\nGame Mode: %s\nSlots: %d",
			msg.hostID, msg.gameMode, msg.slots)
		m.waiting = false
		cmds = append(cmds, waitForEventsCmd(5*time.Second))

	case playerRegisteredMsg:
		hostInfo := "any available host"
		if msg.hostID != "" {
			hostInfo = "host '" + msg.hostID + "'"
		}

		modeInfo := "any game mode"
		if msg.gameMode != "" {
			modeInfo = msg.gameMode
		}

		m.viewMessage = fmt.Sprintf("✅ Player %s registered successfully!\nRequested Host: %s\nGame Mode: %s",
			msg.playerID, hostInfo, modeInfo)
		m.waiting = false
		cmds = append(cmds, waitForEventsCmd(5*time.Second))

	case hostRemovedMsg:
		m.viewMessage = fmt.Sprintf("✅ Host %s removed successfully!", msg.hostID)
		m.waiting = false
		cmds = append(cmds, waitForEventsCmd(5*time.Second))

	case playerRemovedMsg:
		m.viewMessage = fmt.Sprintf("✅ Player %s removed successfully!", msg.playerID)
		m.waiting = false
		cmds = append(cmds, waitForEventsCmd(5*time.Second))

	case waitTimeoutMsg:
		m.viewMessage += "\n\nEvent waiting completed. Press any key to continue."
		return m, fetchInitialState(m)

	case eventMsg:
		m.events = append(m.events, msg.event)

	case spinner.TickMsg:
		if m.waiting {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case errorMsg:
		m.err = msg.err
		m.waiting = false

	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.mainMenu.SetSize(msg.Width-h, msg.Height-v)
	}

	// handle input updates
	if m.state == hostRegistration || m.state == playerRegistration {
		for i := range m.inputs {
			if i == m.focusIndex {
				var cmd tea.Cmd
				m.inputs[i], cmd = m.inputs[i].Update(msg)
				cmds = append(cmds, cmd)
			}
		}
	}

	// handle main menu updates
	if m.state == mainMenu {
		var cmd tea.Cmd
		m.mainMenu, cmd = m.mainMenu.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	var s string

	switch m.state {
	case mainMenu:
		s = m.mainMenu.View()

	case hostRegistration:
		s = titleStyle.Render("Register a Host") + "\n\n"
		s += "Please enter the host details:\n\n"

		for i, input := range m.inputs {
			var label string
			switch i {
			case 0:
				label = "Host ID (leave empty for random):"
			case 1:
				label = "Game mode (e.g., 1v1, 2v2):"
			case 2:
				label = "Available player slots:"
			}
			s += fmt.Sprintf("%s\n%s\n\n", label, input.View())
		}

		if m.waiting {
			s += m.spinner.View() + " Registering host..."
		} else if m.viewMessage != "" {
			s += infoStyle.Render(m.viewMessage)
		}

		s += "\n\nPress Enter to submit, Tab to cycle fields, or q to return to menu."

	case playerRegistration:
		s = titleStyle.Render("Register a Player") + "\n\n"
		s += "Please enter the player details:\n\n"

		for i, input := range m.inputs {
			var label string
			switch i {
			case 0:
				label = "Player ID (leave empty for random):"
			case 1:
				label = "Specific host ID to join (optional):"
			case 2:
				label = "Preferred game mode (optional):"
			}

			s += fmt.Sprintf("%s\n%s\n\n", label, input.View())
		}

		if m.waiting {
			s += m.spinner.View() + " Registering player..."
		} else if m.viewMessage != "" {
			s += infoStyle.Render(m.viewMessage)
		}
		s += "\n\nPress Enter to submit, Tab to cycle fields, or q to return to menu."

	case statusView:
		s = titleStyle.Render("Current Matchmaking Status") + "\n\n"

		// hosts
		s += subtitleStyle.Render(fmt.Sprintf("🎮 Available Hosts (%d)", len(m.hosts))) + "\n"
		if len(m.hosts) == 0 {
			s += "   No hosts available\n"
		} else {
			for i, host := range m.hosts {
				s += fmt.Sprintf("   %d. Host ID: %s\n", i+1, host.HostID)
				s += fmt.Sprintf("      Game Mode: %s\n", host.Mode)
				s += fmt.Sprintf("      Available Slots: %d\n\n", host.AvailableSlots)
			}
		}

		// players
		s += subtitleStyle.Render(fmt.Sprintf("👤 Players Looking for Matches (%d)", len(m.players))) + "\n"
		if len(m.players) == 0 {
			s += "   No players waiting\n"
		} else {
			for i, player := range m.players {
				s += fmt.Sprintf("   %d. Player ID: %s\n", i+1, player.PlayerID)
				if player.HostID != nil && *player.HostID != "" {
					s += fmt.Sprintf("      Preferred Host: %s\n", *player.HostID)
				} else {
					s += "      Preferred Host: Any\n"
				}
				if player.Mode != nil && *player.Mode != "" {
					s += fmt.Sprintf("      Preferred Game Mode: %s\n\n", *player.Mode)
				} else {
					s += "      Preferred Game Mode: Any\n\n"
				}
			}
		}

		// sessions
		s += subtitleStyle.Render(fmt.Sprintf("🎲 Active Game Sessions (%d)", len(m.sessions))) + "\n"
		if len(m.sessions) == 0 {
			s += "   No active sessions\n"
		} else {
			for i, session := range m.sessions {
				s += fmt.Sprintf("   %d. Session ID: %s\n", i+1, session.ID)
				s += fmt.Sprintf("      Host ID: %s\n", session.HostID)
				s += fmt.Sprintf("      Game Mode: %s\n", session.Mode)
				s += fmt.Sprintf("      State: %s\n", session.State)
				s += fmt.Sprintf("      Available Slots: %d\n", session.AvailableSlots)
				s += fmt.Sprintf("      Players (%d): %s\n", len(session.Players), strings.Join(session.Players, ", "))
				s += fmt.Sprintf("      Created: %s\n\n", session.CreatedAt.Format(time.RFC3339))
			}
		}
		s += "\nPress q to return to menu."

	case removeHostView:
		s = titleStyle.Render("Remove a Host") + "\n\n"

		if len(m.hosts) == 0 {
			s += "No hosts available to remove.\n\n"
			s += "Press q to return to menu."
		} else {
			s += "Select a host to remove (use up/down keys, Enter to confirm):\n\n"

			for _, host := range m.hosts {
				if host.HostID == m.selectedID {
					s += selectedItemStyle.Render("➤ "+host.HostID+" (Mode: "+string(host.Mode)+", Slots: "+strconv.Itoa(int(host.AvailableSlots))+")") + "\n"
				} else {
					s += itemStyle.Render(host.HostID+" (Mode: "+string(host.Mode)+", Slots: "+strconv.Itoa(int(host.AvailableSlots))+")") + "\n"
				}
			}

			if m.waiting {
				s += "\n" + m.spinner.View() + " Removing host..."
			} else if m.viewMessage != "" {
				s += "\n" + infoStyle.Render(m.viewMessage)
			}

			s += "\n\nPress Enter to confirm, or q to return to menu."
		}

	case removePlayerView:
		s = titleStyle.Render("Remove a Player") + "\n\n"

		if len(m.players) == 0 {
			s += "No players available to remove.\n\n"
			s += "Press q to return to menu."
		} else {
			s += "Select a player to remove (use up/down keys, Enter to confirm):\n\n"

			for _, player := range m.players {
				hostInfo := "Any"
				if player.HostID != nil && *player.HostID != "" {
					hostInfo = *player.HostID
				}

				modeInfo := "Any"
				if player.Mode != nil && *player.Mode != "" {
					modeInfo = string(*player.Mode)
				}

				if player.PlayerID == m.selectedID {
					s += selectedItemStyle.Render(fmt.Sprintf("➤ %s (Host: %s, Mode: %s)", player.PlayerID, hostInfo, modeInfo)) + "\n"
				} else {
					s += itemStyle.Render(fmt.Sprintf("%s (Host: %s, Mode: %s)", player.PlayerID, hostInfo, modeInfo)) + "\n"
				}
			}

			if m.waiting {
				s += "\n" + m.spinner.View() + " Removing player..."
			} else if m.viewMessage != "" {
				s += "\n" + infoStyle.Render(m.viewMessage)
			}
			s += "\n\nPress Enter to confirm, or q to return to menu."
		}

	case eventView:
		s = titleStyle.Render("Event Log") + "\n\n"

		if len(m.events) == 0 {
			s += "No events received yet.\n"
		} else {
			for i, evt := range m.events {
				timeStr := evt.Timestamp.Format("15:04:05")
				switch evt.Type {
				case "game_created":
					s += eventHeaderStyle.Render(fmt.Sprintf("[%s] 🎮 GAME CREATED (#%d)", timeStr, i+1)) + "\n"
				case "player_join_requested":
					s += eventHeaderStyle.Render(fmt.Sprintf("[%s] 👤 PLAYER JOIN REQUESTED (#%d)", timeStr, i+1)) + "\n"
				case "player_joined":
					s += eventHeaderStyle.Render(fmt.Sprintf("[%s] ✅ PLAYER JOINED (#%d)", timeStr, i+1)) + "\n"
				default:
					s += eventHeaderStyle.Render(fmt.Sprintf("[%s] 🔔 EVENT (%s) (#%d)", timeStr, evt.Type, i+1)) + "\n"
				}
				s += itemStyle.Render(evt.Message) + "\n\n"
			}
		}
		s += "\nPress q to return to menu."
	}

	statusBar := fmt.Sprintf("Hosts: %d | Players: %d | Sessions: %d", len(m.hosts), len(m.players), len(m.sessions))
	footerHeight := 1
	return appStyle.Render(s + "\n\n" + strings.Repeat(" ", footerHeight) + "\n" + statusBarStyle.Render(statusBar))
}

func (m *model) clearInputs() {
	for i := range m.inputs {
		m.inputs[i].Reset()
		m.inputs[i].Blur()
	}
	m.focusIndex = 0
	m.viewMessage = ""
}
func (m *model) setupHostRegistrationInputs() {
	m.clearInputs()
	m.inputs[0].Placeholder = "host-123 (leave empty for random)"
	m.inputs[1].Placeholder = "1v1"
	m.inputs[2].Placeholder = "1"
	m.inputs[0].Focus()
}

func (m *model) setupPlayerRegistrationInputs() {
	m.clearInputs()
	m.inputs[0].Placeholder = "player-123 (leave empty for random)"
	m.inputs[1].Placeholder = "Optional: specific host ID"
	m.inputs[2].Placeholder = "Optional: preferred game mode"
	m.inputs[0].Focus()
}

// Messages

type stateUpdateMsg struct {
	hosts    []Host
	players  []Player
	sessions []GameSession
}

type hostRegisteredMsg struct {
	hostID   string
	gameMode string
	slots    uint16
}

type playerRegisteredMsg struct {
	playerID string
	hostID   string
	gameMode string
}

type hostRemovedMsg struct {
	hostID string
}

type playerRemovedMsg struct {
	playerID string
}

type eventMsg struct {
	event Event
}

type waitTimeoutMsg struct{}

type errorMsg struct {
	err error
}

// Commands

func registerHostCmd(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		hostMsg := pubevts.HostRegistrationEvent{
			HostID:         m.hostID,
			Mode:           pubevts.GameMode(m.gameMode),
			AvailableSlots: m.slots,
		}

		msgBody, err := json.Marshal(hostMsg)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not marshal host message: %w", err)}
		}

		if err := m.ch.PublishWithContext(
			ctx,
			pubevts.ExchangeMatchRequest,
			pubevts.RoutingKeyHostRegistration,
			false,
			false,
			amqp091.Publishing{
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypeHostRegistration,
				Body:        msgBody,
			},
		); err != nil {
			return errorMsg{err: fmt.Errorf("could not publish host registration: %w", err)}
		}
		return hostRegisteredMsg{
			hostID:   m.hostID,
			gameMode: m.gameMode,
			slots:    m.slots,
		}
	}
}

func registerPlayerCmd(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		playerMsg := pubevts.PlayerMatchRequestEvent{
			PlayerID: m.playerID,
		}

		if m.hostID != "" {
			playerMsg.HostID = &m.hostID
		}

		if m.gameMode != "" {
			mode := pubevts.GameMode(m.gameMode)
			playerMsg.Mode = &mode
		}

		msgBody, err := json.Marshal(playerMsg)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not marshal player message: %w", err)}
		}

		if err := m.ch.PublishWithContext(
			ctx,
			pubevts.ExchangeMatchRequest,
			pubevts.RoutingKeyPlayerMatchRequest,
			false,
			false,
			amqp091.Publishing{
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypePlayerMatchRequest,
				Body:        msgBody,
			},
		); err != nil {
			return errorMsg{err: fmt.Errorf("could not publish player registration: %w", err)}
		}

		hostID := ""
		if playerMsg.HostID != nil {
			hostID = *playerMsg.HostID
		}

		gameMode := ""
		if playerMsg.Mode != nil {
			gameMode = string(*playerMsg.Mode)
		}
		return playerRegisteredMsg{
			playerID: m.playerID,
			hostID:   hostID,
			gameMode: gameMode,
		}
	}
}

func removeHostCmd(m model, hostID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		hostMsg := pubevts.HostRegistrationRemovalEvent{
			HostID: hostID,
		}

		msgBody, err := json.Marshal(hostMsg)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not marshal host removal message: %w", err)}
		}

		if err := m.ch.PublishWithContext(
			ctx,
			pubevts.ExchangeMatchRequest,
			pubevts.RoutingKeyHostRegistrationRemoval,
			false,
			false,
			amqp091.Publishing{
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypeHostRegistrationRemoval,
				Body:        msgBody,
			},
		); err != nil {
			return errorMsg{err: fmt.Errorf("could not publish host removal: %w", err)}
		}
		return hostRemovedMsg{
			hostID: hostID,
		}
	}
}

func removePlayerCmd(m model, playerID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		playerMsg := pubevts.PlayerMatchRequestRemovalEvent{
			PlayerID: playerID,
		}

		msgBody, err := json.Marshal(playerMsg)
		if err != nil {
			return errorMsg{err: fmt.Errorf("could not marshal player removal message: %w", err)}
		}

		if err := m.ch.PublishWithContext(
			ctx,
			pubevts.ExchangeMatchRequest,
			pubevts.RoutingKeyPlayerMatchRequestRemoval,
			false,
			false,
			amqp091.Publishing{
				ContentType: pubevts.ContentType,
				Type:        pubevts.MsgTypePlayerMatchRequestRemoval,
				Body:        msgBody,
			},
		); err != nil {
			return errorMsg{err: fmt.Errorf("could not publish player removal: %w", err)}
		}
		return playerRemovedMsg{
			playerID: playerID,
		}
	}
}

func waitForEventsCmd(duration time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(duration)
		return waitTimeoutMsg{}
	}
}

func setupEventObservers(ch *amqp091.Channel, eventChan chan Event) {
	monitorQueues := []string{
		"monitor." + pubevts.ExchangeGameCreated,
		"monitor." + pubevts.ExchangePlayerJoinRequested,
		"monitor." + pubevts.ExchangePlayerJoined,
	}

	typeMapping := map[string]string{
		"monitor." + pubevts.ExchangeGameCreated:         "game_created",
		"monitor." + pubevts.ExchangePlayerJoinRequested: "player_join_requested",
		"monitor." + pubevts.ExchangePlayerJoined:        "player_joined",
	}

	for _, queueName := range monitorQueues {
		msgs, err := ch.Consume(
			queueName, "", true, false, false, false, nil)
		if err != nil {
			log.Printf("Warning: Failed to set up consumer for %s: %v", queueName, err)
			continue
		}

		go func(queue string, eventType string) {
			for msg := range msgs {
				event := Event{
					Type:      eventType,
					Timestamp: time.Now(),
				}

				switch eventType {
				case "game_created":
					var data pubevts.GameCreatedEvent
					if err := json.Unmarshal(msg.Body, &data); err == nil {
						event.Message = fmt.Sprintf("Host '%s' created game with %d slots, mode: %s",
							data.HostID, data.MaxPlayers, data.GameMode)
						event.Data = data
					}
				case "player_join_requested":
					var data pubevts.PlayerJoinRequestedEvent
					if err := json.Unmarshal(msg.Body, &data); err == nil {
						hostInfo := "any available host"
						if data.HostID != nil && *data.HostID != "" {
							hostInfo = "host '" + *data.HostID + "'"
						}

						modeInfo := "any game mode"
						if data.GameMode != nil && *data.GameMode != "" {
							modeInfo = string(*data.GameMode)
						}

						event.Message = fmt.Sprintf("Player '%s' looking to join %s, mode: %s",
							data.PlayerID, hostInfo, modeInfo)
						event.Data = data
					}
				case "player_joined":
					var data pubevts.PlayerJoinedEvent
					if err := json.Unmarshal(msg.Body, &data); err == nil {
						event.Message = fmt.Sprintf("Player '%s' joined host '%s' (%d/%d players)",
							data.PlayerID, data.HostID, data.CurrentPlayerCount, data.MaxPlayers)
						event.Data = data
					}
				default:
					event.Message = string(msg.Body)
				}
				eventChan <- event
			}
		}(queueName, typeMapping[queueName])
	}
}
