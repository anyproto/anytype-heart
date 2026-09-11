package wrapper

// preamble.go — the workspace facts a host puts in front of the model
// before the user's first word (APIV2.md §8.59). The first on-device run
// started with nothing about the workspace in context: no date, no space,
// no type names — and a 3B model refused to answer, then guessed a space
// and a type. This is ONE global text, never a per-space one: an account
// can hold a hundred spaces, and a small model forgets fast, so the
// preamble carries the few most useful facts — today's date, the space the
// user is in, how many spaces there are, and what this conversation has
// recently touched — and nothing else. It is rendered by heart so every
// delivery puts the same words in front of the model; the embedding host
// decides where they go (instructions or the user turn).

import (
	"context"
	"fmt"
	"strings"
)

// Bounds on the recents the preamble names: a handful, most recent first.
const (
	preambleRecentSpaces = 3
	preambleRecentTypes  = 5
)

// Preamble renders the preamble from the run context, the space list and
// the session's recents. Best-effort: a space listing that fails drops the
// space facts and keeps the date; the session is read, never written.
func (r *Runner) Preamble(ctx context.Context) (string, error) {
	var lines []string
	now := r.nowLocal()
	lines = append(lines, fmt.Sprintf("Today is %s.", now.Format("Monday, 2 January 2006")))

	session, err := r.store.Load()
	if err != nil {
		return "", fmt.Errorf("load session: %w", err)
	}
	runCtx := r.Context()
	rows := r.spaceRows(ctx)
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[row.Id] = row.Name
	}
	spell := func(id string) string {
		if name := names[id]; name != "" {
			return name + " (" + id + ")"
		}
		return id
	}

	// the current space: the one the user is looking at (the host said),
	// else the working space a find set
	current := runCtx.Space
	if current == "" {
		current = session.Space
	}
	if current != "" {
		lines = append(lines, "Current space: "+spell(current)+" — describe, create and create_type use it when given no space.")
	}
	if rows != nil {
		lines = append(lines, fmt.Sprintf("%d spaces in the account; find with no space searches all of them, spaces lists them.", len(rows)))
	}

	var recentSpaces []string
	for _, id := range session.RecentSpaces {
		if id == current || len(recentSpaces) >= preambleRecentSpaces {
			continue
		}
		recentSpaces = append(recentSpaces, spell(id))
	}
	if len(recentSpaces) > 0 {
		lines = append(lines, "Recently used spaces: "+strings.Join(recentSpaces, ", ")+".")
	}
	recentTypes := session.RecentTypes
	if len(recentTypes) > preambleRecentTypes {
		recentTypes = recentTypes[:preambleRecentTypes]
	}
	if len(recentTypes) > 0 {
		lines = append(lines, "Recently used types: "+strings.Join(recentTypes, ", ")+".")
	}
	return strings.Join(lines, "\n"), nil
}
