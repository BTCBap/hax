// Package alert provides toast-style notifications for bubbletea apps.
// Based on bubbleup (go.dalton.dog/bubbleup) with added fade-out support.
package alert

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/reflow/ansi"
)

// Alert type keys
const (
	InfoKey  = "Info"
	WarnKey  = "Warn"
	ErrorKey = "Error"
	DebugKey = "Debug"
)

// Animation constants
const (
	FadeOutDuration = 500 * time.Millisecond
)

// Colors
var (
	backColor, _ = colorful.Hex("#000000")
	baseStyle    = lipgloss.NewStyle()
)

// AlertDefinition defines a custom alert type
type AlertDefinition struct {
	Key       string
	ForeColor string
	Style     lipgloss.Style
	Prefix    string
}

// alert represents an active alert instance
type alert struct {
	message      string
	deathTime    time.Time
	fadeOutStart time.Time
	prefix       string
	foreColor    colorful.Color
	style        lipgloss.Style
	width        int
	curLerpStep  float64
	lerpVel      float64 // spring velocity for physics-based interpolation
	fadingOut    bool
}

func (n *alert) render() string {
	newColor := backColor.BlendLab(n.foreColor, n.curLerpStep)
	lipColor := lipgloss.Color(newColor.Hex())
	newStyle := baseStyle.Foreground(lipColor)
	return newStyle.Render(fmt.Sprintf("%v %v", n.prefix, n.message))
}

// AlertModel manages alert display
type AlertModel struct {
	alertTypes  map[string]AlertDefinition
	activeAlert *alert
	width       int
	duration    time.Duration
	lerpSpring  harmonica.Spring // spring for physics-based color interpolation
}

// tickMsg triggers alert updates
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*33, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// alertMsg triggers a new alert
type alertMsg struct {
	alertKey string
	msg      string
	dur      time.Duration
}

// NewAlertModel creates a new alert model
// duration is in seconds (will be multiplied by time.Second internally)
func NewAlertModel(width int, duration int) *AlertModel {
	model := &AlertModel{
		activeAlert: nil,
		width:       width,
		alertTypes:  make(map[string]AlertDefinition),
		duration:    time.Duration(duration) * time.Second,
		// Spring with moderate stiffness and low damping for a natural feel
		// with subtle overshoot (brief flash) before settling
		lerpSpring: harmonica.NewSpring(harmonica.FPS(30), 7.0, 0.6),
	}
	model.registerDefaults()
	return model
}

// Init starts the tick loop
func (m AlertModel) Init() tea.Cmd {
	return tickCmd()
}

// Update handles messages
func (m AlertModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w := msg.Width - 4
		if w > 80 {
			w = 80
		}
		if w < 40 {
			w = 40
		}
		m.width = w

	case tickMsg:
		now := time.Time(msg)
		if m.activeAlert != nil {
			// Check if we should start fading out
			if !m.activeAlert.fadingOut && now.After(m.activeAlert.fadeOutStart) {
				m.activeAlert.fadingOut = true
			}

			// Use spring physics for natural acceleration/deceleration
			target := 1.0
			if m.activeAlert.fadingOut {
				target = 0.0
			}
			m.activeAlert.curLerpStep, m.activeAlert.lerpVel = m.lerpSpring.Update(
				m.activeAlert.curLerpStep, m.activeAlert.lerpVel, target,
			)
			// Clamp to valid range (spring can overshoot slightly for a flash effect)
			m.activeAlert.curLerpStep = math.Max(0, math.Min(1.1, m.activeAlert.curLerpStep))

			// Remove alert when faded out completely
			if m.activeAlert.fadingOut && m.activeAlert.curLerpStep < 0.01 {
				m.activeAlert = nil
			}
		}
		return m, tickCmd()

	case alertMsg:
		m.activeAlert = m.newAlert(msg.alertKey, msg.msg, msg.dur)
	}

	return m, nil
}

// View returns empty string (use Render instead)
func (m AlertModel) View() tea.View {
	return tea.NewView("")
}

// Render overlays the alert on content with slide-reveal animation.
// As the alert fades in, lines are revealed top-down. On fade-out, lines
// disappear bottom-up, creating a natural slide effect.
func (m AlertModel) Render(content string) string {
	if m.activeAlert == nil {
		return content
	}

	notifString := m.activeAlert.render()
	notifLines := strings.Split(notifString, "\n")
	contentLines := strings.Split(content, "\n")

	notifHeight := len(notifLines)
	contentHeight := len(contentLines)

	// Calculate how many notification lines to reveal based on spring progress
	visibleLines := int(math.Ceil(m.activeAlert.curLerpStep * float64(notifHeight)))
	if visibleLines > notifHeight {
		visibleLines = notifHeight
	}
	if visibleLines < 0 {
		visibleLines = 0
	}

	var builder strings.Builder

	for i := 0; i < contentHeight; i++ {
		if i > 0 {
			builder.WriteByte('\n')
		}

		if i >= visibleLines {
			// Beyond revealed notification lines — show content
			builder.WriteString(contentLines[i])
		} else {
			notifLine := notifLines[i]
			builder.WriteString(notifLine)

			notifLen := ansi.PrintableRuneWidth(notifLine)
			if notifLen < len(contentLines[i]) {
				// Add remaining content after notification
				remaining := cutLeft(contentLines[i], notifLen)
				builder.WriteString(remaining)
			}
		}
	}

	return builder.String()
}

// NewAlertCmd creates a command to show an alert
func (m AlertModel) NewAlertCmd(alertType, message string) tea.Cmd {
	return func() tea.Msg {
		return alertMsg{alertKey: alertType, msg: message, dur: m.duration}
	}
}

// RegisterNewAlertType registers a custom alert type
func (m *AlertModel) RegisterNewAlertType(definition AlertDefinition) {
	if m.alertTypes == nil {
		m.alertTypes = make(map[string]AlertDefinition)
	}
	m.alertTypes[definition.Key] = definition
}

func (m AlertModel) newAlert(key, msg string, dur time.Duration) *alert {
	if msg == "" || key == "" {
		return nil
	}

	alertDef, ok := m.alertTypes[key]
	if !ok {
		return nil
	}

	color, err := colorful.Hex(alertDef.ForeColor)
	if err != nil {
		return nil
	}

	now := time.Now()
	return &alert{
		message:      msg,
		deathTime:    now.Add(dur),
		fadeOutStart: now.Add(dur - FadeOutDuration),
		prefix:       alertDef.Prefix,
		foreColor:    color,
		style:        alertDef.Style,
		width:        m.width,
		curLerpStep:  0.3,
		fadingOut:    false,
	}
}

func (m *AlertModel) registerDefaults() {
	m.RegisterNewAlertType(AlertDefinition{
		Key:       InfoKey,
		Prefix:    "(i)",
		ForeColor: "#00FF00",
	})
	m.RegisterNewAlertType(AlertDefinition{
		Key:       WarnKey,
		Prefix:    "(!)",
		ForeColor: "#FFFF00",
	})
	m.RegisterNewAlertType(AlertDefinition{
		Key:       ErrorKey,
		Prefix:    "[!!]",
		ForeColor: "#FF0000",
	})
	m.RegisterNewAlertType(AlertDefinition{
		Key:       DebugKey,
		Prefix:    "(?)",
		ForeColor: "#FF00FF",
	})
}

// cutLeft removes n characters from the left of a string, handling ANSI codes
func cutLeft(s string, n int) string {
	if n <= 0 {
		return s
	}

	var result strings.Builder
	width := 0
	inEscape := false

	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			result.WriteRune(r)
			continue
		}
		if inEscape {
			result.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		width++
		if width > n {
			result.WriteRune(r)
		}
	}

	return result.String()
}
