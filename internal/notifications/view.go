package notifications

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/cli/go-gh/v2/pkg/text"

	"github.com/nobe4/gh-not/internal/colors"
)

// truncate truncates a string to a maximum length, adding an ellipsis if truncated.
// It respects UTF-8 rune boundaries to avoid splitting multi-byte characters.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	const ellipsisLen = 3
	// Reserve space for ellipsis
	if maxLen <= ellipsisLen {
		return string(runes[:maxLen])
	}

	return string(runes[:maxLen-ellipsisLen]) + "..."
}

//nolint:gochecknoglobals // This map is used a lot.
var prettyRead = map[bool]string{
	false: colors.Red("RD"),
	true:  colors.Green("UR"),
}

//nolint:gochecknoglobals // This map is used a lot.
var prettyTypes = map[string]string{
	"Issue":       colors.Blue("IS"),
	"PullRequest": colors.Cyan("PR"),
	"Discussion":  colors.Green("DS"),
}

//nolint:gochecknoglobals // This map is used a lot.
var prettyState = map[string]string{
	"open":   colors.Green("OP"),
	"closed": colors.Red("CL"),
	"merged": colors.Magenta("MG"),
}

func (n Notification) String() string {
	if n.rendered != "" {
		return n.rendered
	}

	return fmt.Sprintf("[%s] %s at %s: %s", n.ID, n.Author.Login, n.UpdatedAt, n.Subject.Title)
}

func (n Notification) prettyRead() string {
	if p, ok := prettyRead[n.Unread]; ok {
		return p
	}

	return colors.Yellow("R?")
}

func (n Notification) prettyType() string {
	if p, ok := prettyTypes[n.Subject.Type]; ok {
		return p
	}

	return colors.Yellow("T?")
}

func (n Notification) prettyState() string {
	if p, ok := prettyState[n.Subject.State]; ok {
		return p
	}

	return colors.Yellow("S?")
}

func (n Notifications) String() string {
	var sb strings.Builder
	for _, notif := range n {
		_, _ = sb.WriteString(fmt.Sprintf("%s\n", notif))
	}

	return sb.String()
}

func (n Notifications) Visible() Notifications {
	visible := Notifications{}

	for _, n := range n {
		if n.Visible() {
			visible = append(visible, n)
		}
	}

	return visible
}

func (n Notifications) TagsMap() map[string]int {
	tags := map[string]int{}

	for _, n := range n {
		for _, t := range n.Meta.Tags {
			tags[t]++
		}
	}

	return tags
}

func (n Notification) Visible() bool {
	return !n.Meta.Done && !n.Meta.Hidden
}

// Render the notifications in a human readable format.
// If possible, render a table, otherwise render a simple string.
//
//revive:disable:cognitive-complexity // TODO: simplify.
//revive:disable:function-length // TODO: refactor.
//revive:disable:cyclomatic // TODO: simplify.
//nolint:cyclop // TODO: simplify.
func (n Notifications) Render() error {
	if len(n) == 0 {
		return nil
	}

	// Default to a simple string
	for _, notif := range n {
		notif.rendered = fmt.Sprintf(
			"%s %s %s %s by %s at %s: '%s'",
			notif.prettyRead(),
			notif.prettyType(),
			notif.prettyState(),
			notif.Repository.FullName,
			notif.Author.Login,
			text.RelativeTimeAgo(time.Now(), notif.UpdatedAt),
			notif.Subject.Title)
	}

	// Try to render a table
	out := bytes.Buffer{}

	t := term.FromEnv()

	w, _, err := t.Size()
	if err != nil {
		return fmt.Errorf("failed to get terminal size: %w", err)
	}

	// If terminal is too narrow, skip table rendering and use fallback string format
	const minTerminalWidth = 60
	if w < minTerminalWidth {
		slog.Debug("terminal too narrow for table rendering", "width", w, "min", minTerminalWidth)

		return nil
	}

	// Calculate dynamic column widths to prevent wrapping
	const (
		minRepoWidth  = 10
		minTitleWidth = 20
		// Fixed width columns: RD(2) + IS(2) + OP(2) + spacing(~6) + time column(~25 including "by user")
		fixedWidth     = 37
		repoPercent    = 30
		percentDivisor = 100
		maxAuthorWidth = 25
	)

	availableWidth := w - fixedWidth
	if availableWidth < (minRepoWidth + minTitleWidth) {
		slog.Debug("insufficient width for table columns",
			"available", availableWidth,
			"required", minRepoWidth+minTitleWidth)

		return nil
	}

	// Allocate 30% to repository name, 70% to title
	repoWidth := availableWidth * repoPercent / percentDivisor
	repoWidth = max(repoWidth, minRepoWidth)

	titleWidth := availableWidth - repoWidth
	if titleWidth < minTitleWidth {
		titleWidth = minTitleWidth
		repoWidth = availableWidth - titleWidth
	}

	slog.Debug("calculated column widths",
		"terminal", w,
		"available", availableWidth,
		"repo", repoWidth,
		"title", titleWidth)

	printer := tableprinter.New(&out, t.IsTerminalOutput(), w)

	for _, notif := range n {
		printer.AddField(notif.prettyRead())
		printer.AddField(notif.prettyType())
		printer.AddField(notif.prettyState())

		// Truncate repository full name
		repoField := truncate(notif.Repository.FullName, repoWidth)
		if repoField != notif.Repository.FullName {
			slog.Debug("truncated repository name",
				"original", notif.Repository.FullName,
				"truncated", repoField,
				"maxWidth", repoWidth)
		}

		printer.AddField(repoField)

		// Author login - cap at maxAuthorWidth characters to be safe
		authorField := truncate(notif.Author.Login, maxAuthorWidth)

		printer.AddField(authorField)

		// Truncate subject title
		titleField := truncate(notif.Subject.Title, titleWidth)
		if titleField != notif.Subject.Title {
			slog.Debug("truncated title",
				"original", notif.Subject.Title,
				"truncated", titleField,
				"maxWidth", titleWidth)
		}

		printer.AddField(titleField)

		relativeTime := text.RelativeTimeAgo(time.Now(), notif.UpdatedAt)
		if notif.LatestCommentor.Login != "" {
			relativeTime += " by " + notif.LatestCommentor.Login
		}

		printer.AddField(relativeTime)
		printer.EndRow()
	}

	if err := printer.Render(); err != nil {
		return fmt.Errorf("failed to render table: %w", err)
	}

	// Safety guard: prevent index out of range panic
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	for i, l := range lines {
		if i >= len(n) {
			slog.Warn("more output lines than notifications",
				"lines", len(lines),
				"notifications", len(n))

			break
		}

		n[i].rendered = l
	}

	return nil
}
