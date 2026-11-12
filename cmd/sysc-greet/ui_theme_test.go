package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nomadcxx/sysc-greet/internal/themes"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

func TestInputThemeIncludesBlurredStyles(t *testing.T) {
	applyTheme("nord", true)
	t.Cleanup(func() { applyTheme("dracula", true) })
	m := model{usernameInput: textinput.New(), passwordInput: textinput.New()}
	m.applyInputTheme()

	for name, input := range map[string]textinput.Model{
		"username": m.usernameInput,
		"password": m.passwordInput,
	} {
		for state, styles := range map[string]textinput.StyleState{
			"focused": input.Styles.Focused,
			"blurred": input.Styles.Blurred,
		} {
			t.Run(name+"/"+state, func(t *testing.T) {
				if !sameColor(styles.Prompt.GetForeground(), Primary) {
					t.Error("prompt does not use the theme primary color")
				}
				if !sameColor(styles.Text.GetForeground(), FgPrimary) {
					t.Error("text does not use the theme foreground color")
				}
				if !sameColor(styles.Placeholder.GetForeground(), FgMuted) {
					t.Error("placeholder does not use the theme muted color")
				}
			})
		}
	}
}

func TestBorderedHelpUsesThemeForeground(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SYSC_BG", "")
	t.Cleanup(func() { applyTheme("dracula", true) })
	m := initialModel(Config{TestMode: true, ThemeName: "Nord"}, false)
	m.width, m.height = 132, 40

	for _, mode := range []ViewMode{ModeLogin, ModePassword, ModeLoading} {
		t.Run(string(mode), func(t *testing.T) {
			m.mode = mode
			canvas, ok := m.borderedLoginCanvas(m.width, m.height)
			if !ok {
				t.Fatal("bordered layout did not render")
			}
			screen := uv.NewScreenBuffer(m.width, m.height)
			canvas.Draw(screen, screen.Bounds())
			text := "F1 Menu"
			if mode == ModeLoading {
				text = "Please wait..."
			}
			for y, line := range strings.Split(screen.Buffer.String(), "\n") {
				if index := strings.Index(line, text); index >= 0 {
					x := lipgloss.Width(line[:index])
					for offset := range len(text) {
						if !sameColor(screen.CellAt(x+offset, y).Style.Fg, FgMuted) {
							t.Fatalf("help text %q does not use the theme muted color", text)
						}
					}
					return
				}
			}
			t.Fatalf("help text %q missing", text)
		})
	}
}

func TestClosingMenuRestoresUsernameFocus(t *testing.T) {
	for _, passwordMode := range []bool{false, true} {
		for _, closeKey := range []struct {
			name string
			code rune
		}{
			{"escape", tea.KeyEscape},
			{"enter", tea.KeyEnter},
		} {
			mode := "username"
			if passwordMode {
				mode = "password"
			}
			t.Run(mode+"/"+closeKey.name, func(t *testing.T) {
				m := model{
					mode: ModeLogin, focusState: FocusUsername,
					usernameInput: textinput.New(), passwordInput: textinput.New(),
				}
				m.usernameInput.SetValue("sample")
				if passwordMode {
					m.mode, m.focusState = ModePassword, FocusPassword
					m.passwordInput.Focus()
				} else {
					m.usernameInput.Focus()
				}
				m, _ = m.handleKeyInput(tea.KeyPressMsg{Code: tea.KeyF1})
				m, cmd := m.handleKeyInput(tea.KeyPressMsg{Code: closeKey.code})
				if m.mode != ModeLogin || m.focusState != FocusUsername || !m.usernameInput.Focused() || m.passwordInput.Focused() {
					t.Fatal("closing the menu must return to a focused username input")
				}
				if cmd == nil {
					t.Error("closing the menu must restart the input cursor blink")
				}
				if m.usernameInput.Value() != "sample" {
					t.Error("closing the menu changed the username")
				}
			})
		}
	}
}

func TestFocusMarkerFollowsFocus(t *testing.T) {
	const width = 72
	for _, tc := range []struct {
		name     string
		mode     ViewMode
		focus    FocusState
		marked   string
		unmarked string
	}{
		{"username", ModeLogin, FocusUsername, "▸ Username:", "  Session:"},
		{"session over username", ModeLogin, FocusSession, "▸ Session:", "  Username:"},
		{"password", ModePassword, FocusPassword, "▸ Password:", "  Session:"},
		{"session over password", ModePassword, FocusSession, "▸ Session:", "  Password:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := layoutModel(80, 24)
			m.mode, m.focusState = tc.mode, tc.focus
			plain := stripAnsi(m.renderMainForm(width))
			if !strings.Contains(plain, tc.marked) {
				t.Errorf("focused label %q is not marked", tc.marked)
			}
			if !strings.Contains(plain, tc.unmarked) {
				t.Errorf("unfocused label %q does not keep the marker gutter", tc.unmarked)
			}
			if got := strings.Count(plain, strings.TrimSpace(focusMarker)); got != 1 {
				t.Errorf("form shows %d focus markers, want 1", got)
			}
			for _, line := range strings.Split(plain, "\n") {
				if lipgloss.Width(line) > width {
					t.Errorf("line is wider than the form: %q", line)
				}
			}
		})
	}
}

func TestFocusMarkerKeepsFieldsInPlace(t *testing.T) {
	column := func(focus FocusState, text string) int {
		m := layoutModel(80, 24)
		m.focusState = focus
		for _, line := range strings.Split(stripAnsi(m.renderMainForm(72)), "\n") {
			if index := strings.Index(line, text); index >= 0 {
				return lipgloss.Width(line[:index])
			}
		}
		t.Fatalf("%q missing from the form", text)
		return 0
	}
	for _, text := range []string{"Session:", "XFCE", "Username:", "nomadx"} {
		if focused, unfocused := column(FocusUsername, text), column(FocusSession, text); focused != unfocused {
			t.Errorf("%q moves from column %d to %d when focus changes", text, focused, unfocused)
		}
	}
}

// useBundledThemes loads only the themes shipped in the repository.
func useBundledThemes(t *testing.T) []string {
	t.Helper()
	original := themes.CustomThemes
	t.Cleanup(func() {
		themes.CustomThemes = original
		applyTheme("dracula", true)
	})
	themes.CustomThemes = make(map[string]themes.ThemeColors)
	return append(themes.GetAvailableThemes(), themes.ScanCustomThemes([]string{filepath.Join("..", "..", "themes")})...)
}

func TestLabelContrastInEveryTheme(t *testing.T) {
	names := useBundledThemes(t)
	if len(names) < 27 {
		t.Fatalf("found %d themes, want the 12 built-in and 15 bundled ones", len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			applyTheme(name, true)
			focused, unfocused := labelColors()
			got := contrastRatio(focused, unfocused)
			t.Logf("focused on unfocused = %.2f, was %.2f; unfocused on background = %.2f; focused lightened: %v",
				got, contrastRatio(Primary, FgSecondary), contrastRatio(unfocused, BgBase), !sameColor(focused, Primary))
			if got < labelFocusContrast {
				t.Errorf("focused label on unfocused label = %.2f, want >= %.1f", got, labelFocusContrast)
			}
			// A theme whose fg_muted is already below the limit is left as it is.
			legible := math.Min(labelMinContrast, contrastRatio(FgMuted, BgBase))
			if onBackground := contrastRatio(unfocused, BgBase); onBackground < legible {
				t.Errorf("unfocused label on background = %.2f, want >= %.2f", onBackground, legible)
			}
			if onBackground, primary := contrastRatio(focused, BgBase), contrastRatio(Primary, BgBase); onBackground < primary {
				t.Errorf("focused label on background = %.2f, harder to read than primary (%.2f)", onBackground, primary)
			}
		})
	}
}

func TestFocusedLabelStaysPrimaryWhereItCan(t *testing.T) {
	useBundledThemes(t)
	// Void and Rose-Pine have room for a dimmer label under Primary.
	// Solarized has not, but its text color is far enough above Primary.
	for _, name := range []string{"Void", "Rose-Pine", "Solarized"} {
		applyTheme(name, true)
		if focused, _ := labelColors(); !sameColor(focused, Primary) {
			t.Errorf("%s: focused label is not the theme primary", name)
		}
	}
	applyTheme("Solarized", true)
	if _, unfocused := labelColors(); !sameColor(unfocused, FgPrimary) {
		t.Error("Solarized: unfocused label is not the theme text color")
	}
}

func TestThemeLabelColorsOverrideDerivedOnes(t *testing.T) {
	useBundledThemes(t)
	dir := t.TempDir()
	palette := `[colors]
bg_base = "#282a36"
bg_active = "#44475a"
primary = "#bd93f9"
secondary = "#8be9fd"
accent = "#50fa7b"
warning = "#f1fa8c"
danger = "#ff5555"
fg_primary = "#f8f8f2"
fg_secondary = "#f1f2f6"
fg_muted = "#6272a4"
border_focus = "#bd93f9"
`
	for file, labels := range map[string]string{
		"both.toml":      "label_focused = \"#8be9fd\"\nlabel_unfocused = \"#44475a\"\n",
		"unfocused.toml": "label_unfocused = \"#44475a\"\n",
		"neither.toml":   "",
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(palette+labels), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	themes.ScanCustomThemes([]string{dir})

	applyTheme("both", true)
	focused, unfocused := labelColors()
	if !sameColor(focused, lipgloss.Color("#8be9fd")) || !sameColor(unfocused, lipgloss.Color("#44475a")) {
		t.Error("theme label colors were not used")
	}

	applyTheme("unfocused", true)
	focused, unfocused = labelColors()
	if !sameColor(unfocused, lipgloss.Color("#44475a")) {
		t.Error("label_unfocused was not used")
	}
	if focused == nil || contrastRatio(focused, unfocused) < labelFocusContrast {
		t.Error("the focused label was not derived against label_unfocused")
	}

	applyTheme("neither", true)
	derivedFocused, derivedUnfocused := labelColors()
	applyTheme("dracula", true)
	focused, unfocused = labelColors()
	if !sameColor(focused, derivedFocused) || !sameColor(unfocused, derivedUnfocused) {
		t.Error("a theme without label colors does not match the built-in theme with the same palette")
	}
}
