package scaffold

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// manualEntryOption is the sentinel choice offered alongside discovered
// project directories that falls through to a free-text path prompt.
const manualEntryOption = "Enter a path manually..."

// ErrCancelled is returned when the user backs out of the picker (Ctrl+C or
// Esc) instead of choosing a target.
var ErrCancelled = errors.New("cancelled")

// PickProjectDir shows a fuzzy-filterable select over candidates, plus a
// manual-entry fallback for anything not discovered. It returns
// ErrCancelled if the user aborts.
func PickProjectDir(candidates []string) (string, error) {
	options := make([]huh.Option[string], 0, len(candidates)+1)
	for _, c := range candidates {
		options = append(options, huh.NewOption(c, c))
	}
	options = append(options, huh.NewOption(manualEntryOption, manualEntryOption))

	var choice string
	sel := huh.NewSelect[string]().
		Title("Where should figma-map be installed?").
		Options(options...).
		Filtering(true).
		Value(&choice)

	if err := huh.Run(sel); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("project picker: %w", err)
	}

	if choice != manualEntryOption {
		return choice, nil
	}

	var path string
	input := huh.NewInput().
		Title("Project path").
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("path can't be empty")
			}
			return nil
		}).
		Value(&path)

	if err := huh.Run(input); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("manual path entry: %w", err)
	}
	return strings.TrimSpace(path), nil
}

// Confirm shows a yes/no prompt with the given message, returning
// ErrCancelled if the user aborts rather than answering.
func Confirm(title string) (bool, error) {
	var ok bool
	c := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&ok)

	if err := huh.Run(c); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, ErrCancelled
		}
		return false, fmt.Errorf("confirm: %w", err)
	}
	return ok, nil
}
