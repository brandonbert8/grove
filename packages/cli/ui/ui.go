// Package ui is Grove's terminal design system: one sober palette,
// one set of symbols, zero ANSI scattered through generators.
//
// Philosophy: minimal, recognizable and useful. Colors render only on
// capable terminals; pipes, CI, NO_COLOR and --no-color get clean text.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Palette — darker variants chosen to read on light AND dark terminals.
const (
	groveViolet = "#7C3AED" // brand
	okGreen     = "#16A34A" // success
	errRed      = "#DC2626" // error
	warnAmber   = "#B45309" // warning
	mutedGray   = "#6B7280" // muted / verbose
)

// Action is a filesystem operation kind for file listings.
type Action int

const (
	// Create writes a new file.
	Create Action = iota
	// Update modifies an existing file.
	Update
	// Skip leaves an existing file untouched.
	Skip
	// Delete removes a file.
	Delete
)

// Label renders the aligned operation tag: CREATE, UPDATE, SKIP, DELETE.
func (a Action) Label() string {
	switch a {
	case Create:
		return "CREATE"
	case Update:
		return "UPDATE"
	case Skip:
		return "SKIP"
	case Delete:
		return "DELETE"
	default:
		return "CREATE"
	}
}

// FileChange is one filesystem operation reported by generators. Logic
// returns these; only the Renderer prints them.
type FileChange struct {
	Action Action
	Path   string
}

// Renderer prints the Grove CLI experience. All color flows through
// lipgloss styles owned here; generators never touch ANSI.
type Renderer struct {
	out     io.Writer
	verbose bool
	color   bool

	brand   lipgloss.Style
	success lipgloss.Style
	failure lipgloss.Style
	warning lipgloss.Style
	muted   lipgloss.Style
	create  lipgloss.Style
	update  lipgloss.Style
	skip    lipgloss.Style
	del     lipgloss.Style
}

// Option tunes a Renderer.
type Option func(*Renderer)

// WithColor forces colored (true) or plain (false) output.
func WithColor(enabled bool) Option {
	return func(r *Renderer) { r.color = enabled }
}

// WithVerbose enables debug lines.
func WithVerbose(enabled bool) Option {
	return func(r *Renderer) { r.verbose = enabled }
}

// New returns a Renderer writing to out. The color profile is set
// explicitly (TrueColor when enabled, ASCII otherwise) so output is
// deterministic: real terminals get the palette, pipes/tests get clean
// text. DetectColor decides up front; the renderer never sniffs.
func New(out io.Writer, opts ...Option) *Renderer {
	r := &Renderer{out: out, color: true}
	for _, opt := range opts {
		opt(r)
	}
	lr := lipgloss.NewRenderer(out)
	if r.color {
		lr.SetColorProfile(termenv.TrueColor)
	} else {
		lr.SetColorProfile(termenv.Ascii)
	}
	mk := func(c string, bold bool) lipgloss.Style {
		s := lr.NewStyle().Foreground(lipgloss.Color(c))
		if bold {
			s = s.Bold(true)
		}
		return s
	}
	r.brand = mk(groveViolet, true)
	r.success = mk(okGreen, true)
	r.failure = mk(errRed, true)
	r.warning = mk(warnAmber, true)
	r.muted = mk(mutedGray, false)
	r.create = mk(okGreen, true)
	r.update = mk("#2563EB", true)
	r.skip = mk(mutedGray, true)
	r.del = mk(errRed, true)
	return r
}

// DetectColor reports whether out deserves color: TTY, no --no-color,
// no NO_COLOR in the environment.
func DetectColor(out io.Writer, forceNoColor bool) bool {
	_, noColorEnv := os.LookupEnv("NO_COLOR")
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return colorEnabled(forceNoColor, noColorEnv, st.Mode()&os.ModeCharDevice != 0)
}

// colorEnabled is the pure decision behind DetectColor, kept separate
// for tests (real TTYs are scarce in CI).
func colorEnabled(forceNoColor, noColorEnv, isTTY bool) bool {
	if forceNoColor || noColorEnv {
		return false
	}
	return isTTY
}

// paint renders s through style, or raw when color is off.
func (r *Renderer) paint(s lipgloss.Style, text string) string {
	if !r.color {
		return text
	}
	return s.Render(text)
}

// Header prints the brand line: ◆ Grove [version].
func (r *Renderer) Header(version string) {
	line := "◆ Grove"
	if version != "" {
		line += " " + version
	}
	fmt.Fprintln(r.out, r.paint(r.brand, line))
	fmt.Fprintln(r.out)
}

// Success prints: ✔ message.
func (r *Renderer) Success(msg string) {
	fmt.Fprintln(r.out, r.paint(r.success, "✔")+" "+msg)
}

// Failure prints an actionable error block:
//
//	✖ Title
//
//	Detail.
//
//	Hint.
func (r *Renderer) Failure(title, detail, hint string) {
	fmt.Fprintln(r.out, r.paint(r.failure, "✖")+" "+title)
	if detail != "" {
		fmt.Fprintln(r.out)
		for _, line := range strings.Split(detail, "\n") {
			fmt.Fprintln(r.out, line)
		}
	}
	if hint != "" {
		fmt.Fprintln(r.out)
		fmt.Fprintln(r.out, r.paint(r.muted, hint))
	}
}

// Warning prints: ⚠ message.
func (r *Renderer) Warning(msg string) {
	fmt.Fprintln(r.out, r.paint(r.warning, "⚠")+" "+msg)
}

// Info prints: → message.
func (r *Renderer) Info(msg string) {
	fmt.Fprintln(r.out, "→ "+msg)
}

// Muted prints dim secondary text.
func (r *Renderer) Muted(msg string) {
	fmt.Fprintln(r.out, r.paint(r.muted, msg))
}

// Verbosef prints dim debug lines only in verbose mode.
func (r *Renderer) Verbosef(format string, args ...any) {
	if !r.verbose {
		return
	}
	fmt.Fprintln(r.out, r.paint(r.muted, "[verbose] "+fmt.Sprintf(format, args...)))
}

// FileChanges prints aligned operations:
//
//	CREATE  modules/users/module.go
//	UPDATE  modules/users/module.go
func (r *Renderer) FileChanges(changes []FileChange) {
	for _, c := range changes {
		style := r.create
		switch c.Action {
		case Update:
			style = r.update
		case Skip:
			style = r.skip
		case Delete:
			style = r.del
		}
		fmt.Fprintln(r.out, r.paint(style, fmt.Sprintf("%-6s", c.Action.Label()))+"  "+c.Path)
	}
}

// Section prints a titled block (e.g. next steps): title, blank line,
// then one line per item. Items are preformatted by the caller with
// Command/URL helpers.
func (r *Renderer) Section(title string, lines []string) {
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, title)
	fmt.Fprintln(r.out)
	for _, line := range lines {
		fmt.Fprintln(r.out, line)
	}
}

// URL highlights a URL for next-steps output.
func (r *Renderer) URL(url string) string {
	return r.paint(r.update, url)
}

// Command highlights a shell command for next-steps output.
func (r *Renderer) Command(cmd string) string {
	if !r.color {
		return "  " + cmd
	}
	return "  " + lipgloss.NewStyle().Bold(true).Render(cmd)
}
