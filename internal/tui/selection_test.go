package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestStripANSIKeepsOSC8VisibleText(t *testing.T) {
	input := "open \x1b]8;;https://example.com/a\x1b\\https://example.com/a\x1b]8;;\x1b\\ now"
	got := stripANSI(input)
	want := "open https://example.com/a now"
	if got != want {
		t.Fatalf("stripANSI() = %q, want %q", got, want)
	}
}

func TestSelectionTextFromRenderedSkipsBordersAndPadding(t *testing.T) {
	rendered := shell("Open:\nhttps://example.com/a\nNext line", 80)
	got := selectionTextFromRendered(rendered, point{x: 0, y: 0}, point{x: 79, y: 20})
	want := "posters\nOpen:\nhttps://example.com/a\nNext line"
	if got != want {
		t.Fatalf("selection text = %q, want %q", got, want)
	}
}

func TestSelectionTextKeepsEdgeCharacters(t *testing.T) {
	rendered := shell("ABCDEFGHIJ\n0123456789", 80)
	got := selectionTextFromRendered(rendered, point{x: 0, y: 0}, point{x: 79, y: 20})
	want := "posters\nABCDEFGHIJ\n0123456789"
	if got != want {
		t.Fatalf("selection text = %q, want %q", got, want)
	}
}

func TestSelectionTextSupportsReverseDrag(t *testing.T) {
	rendered := shell("Open:\nhttps://example.com/a\nNext line", 80)
	forward := selectionTextFromRendered(rendered, point{x: 0, y: 0}, point{x: 79, y: 20})
	reverse := selectionTextFromRendered(rendered, point{x: 79, y: 20}, point{x: 0, y: 0})
	if reverse != forward {
		t.Fatalf("reverse selection = %q, want %q", reverse, forward)
	}
}

func TestHighlightSelectionDoesNotIncludeBorders(t *testing.T) {
	rendered := shell("Open:\nhttps://example.com/a\nNext line", 80)
	bounds, ok := detectSelectionBounds(stripANSI(rendered))
	if !ok {
		t.Fatal("selection bounds not detected")
	}
	highlighted := highlightRenderedSelection(rendered, point{x: 0, y: 0}, point{x: 79, y: 20})
	lines := strings.Split(highlighted, "\n")
	if strings.Contains(lines[bounds.outerTop], "\x1b[7m") {
		t.Fatalf("top border highlighted: %q", lines[bounds.outerTop])
	}
	if strings.Contains(lines[bounds.outerBot], "\x1b[7m") {
		t.Fatalf("bottom border highlighted: %q", lines[bounds.outerBot])
	}
}

func TestCenteredSelectionDoesNotHighlightOuterWhitespace(t *testing.T) {
	rendered := centerView(shell("Open:\nhttps://example.com/a\nNext line", 80), 100, 24)
	bounds, ok := detectSelectionBounds(stripANSI(rendered))
	if !ok {
		t.Fatal("selection bounds not detected")
	}
	highlighted := highlightRenderedSelection(rendered, point{x: 0, y: 0}, point{x: 99, y: 23})
	lines := strings.Split(highlighted, "\n")
	for y, line := range lines {
		if !strings.Contains(line, "\x1b[7m") {
			continue
		}
		prefix := line[:strings.Index(line, "\x1b[7m")]
		if lipgloss.Width(stripANSI(prefix)) < bounds.left {
			t.Fatalf("highlight starts before card interior on line %d: %q", y, line)
		}
		reset := strings.LastIndex(line, "\x1b[27m")
		if reset < 0 {
			t.Fatalf("highlight reset missing on line %d: %q", y, line)
		}
		throughHighlight := line[:reset]
		if lipgloss.Width(stripANSI(throughHighlight)) > bounds.right+1 {
			t.Fatalf("highlight extends past card interior on line %d: %q", y, line)
		}
	}
}

func TestClampSelectionPointAllowsBorderPressButStartsInside(t *testing.T) {
	rendered := shell("Open:\nNext line", 80)
	got, ok := clampSelectionPoint(rendered, point{x: 0, y: 0}, true)
	if !ok {
		t.Fatal("border press rejected")
	}
	if got != (point{x: 1 + lipglossHorizontalPadding, y: 1 + verticalContentPadding}) {
		t.Fatalf("clamped point = %#v, want x=%d y=%d", got, 1+lipglossHorizontalPadding, 1+verticalContentPadding)
	}
}

func TestClampSelectionPointRejectsOutsideCard(t *testing.T) {
	rendered := centerView(shell("Open:\nNext line", 80), 100, 24)
	if _, ok := clampSelectionPoint(rendered, point{x: 0, y: 0}, true); ok {
		t.Fatal("outside-card press accepted")
	}
}

func TestBaseViewCentersCard(t *testing.T) {
	m := Model{screen: screenLogin, width: 100, height: 24}
	bounds, ok := detectSelectionBounds(stripANSI(m.baseView()))
	if !ok {
		t.Fatal("centered card bounds not detected")
	}
	if bounds.outerTop == 0 || bounds.outerLeft == 0 {
		t.Fatalf("card not centered: bounds=%#v", bounds)
	}
}

func TestHighlightPlainSelectionKeepsVisibleText(t *testing.T) {
	plain := "alpha\nbeta\ngamma"
	highlighted := highlightPlainSelection(plain, point{x: 1, y: 0}, point{x: 2, y: 1})
	if !strings.Contains(highlighted, "\x1b[7m") {
		t.Fatalf("highlight missing reverse-video sequence: %q", highlighted)
	}
	if got := stripANSI(highlighted); got != plain {
		t.Fatalf("visible text changed: %q, want %q", got, plain)
	}
}

func TestUpdateMouseTracksSelection(t *testing.T) {
	m := Model{width: 80}
	model, cmd := m.updateMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 3})
	if cmd != nil {
		t.Fatal("press returned command")
	}
	m = model.(Model)
	if !m.selection.active || m.selection.start != (point{x: 1 + lipglossHorizontalPadding, y: 1 + verticalContentPadding}) {
		t.Fatalf("selection not started: %#v", m.selection)
	}
	model, cmd = m.updateMouse(tea.MouseMsg{Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, X: 8, Y: 4})
	if cmd != nil {
		t.Fatal("motion returned command")
	}
	m = model.(Model)
	if m.selection.end != (point{x: 1 + lipglossHorizontalPadding, y: 4}) {
		t.Fatalf("selection end not updated: %#v", m.selection)
	}
}
