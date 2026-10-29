package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ssk/internal/services"
)

func addJiraHistoryFlags(cmd *cobra.Command) {
	cmd.Flags().Int("description-lines", 0, "retain the first n normalized description lines (0: unlimited)")
	cmd.Flags().Int("comment-chars", 0, "retrieve comments and cap each retained body by Unicode characters (0: unlimited)")
	cmd.Flags().Bool("include-development", true, "fetch development-panel data")
	cmd.Flags().String("changes-since", "", "include changes since this RFC 3339 timestamp (inclusive)")
	cmd.Flags().String("changes-until", "", "include changes before this RFC 3339 timestamp (exclusive; default: command start)")
	cmd.Flags().String("comments-since", "", "include comments created or last edited since this RFC 3339 timestamp (inclusive)")
	cmd.Flags().String("comments-until", "", "include comments created or last edited before this RFC 3339 timestamp (exclusive; default: command start)")
	cmd.Flags().Int("comments-limit", 0, "retrieve all visible comments and retain the newest n qualifying comments (0: unlimited)")
}

func jiraHistoryOptions(cmd *cobra.Command, started time.Time) (services.TicketInspectionOptions, error) {
	var opts services.TicketInspectionOptions
	for _, source := range []string{"changes", "comments"} {
		// Flags may be absent for embedded users of the command helpers.
		if cmd.Flags().Lookup(source+"-since") == nil {
			continue
		}
		since, err := cmd.Flags().GetString(source + "-since")
		if err != nil {
			return opts, err
		}
		until, err := cmd.Flags().GetString(source + "-until")
		if err != nil {
			return opts, err
		}
		for _, bound := range []struct{ name, value string }{{source + "-since", since}, {source + "-until", until}} {
			if cmd.Flags().Changed(bound.name) && strings.TrimSpace(bound.value) == "" {
				return opts, fmt.Errorf("--%s cannot be empty", bound.name)
			}
		}
		period, err := services.ParseJiraPeriod(since, until, started)
		if err != nil {
			return opts, fmt.Errorf("--%s-since/--%s-until: %w", source, source, err)
		}
		if source == "changes" {
			opts.ChangesPeriod = period
		} else {
			opts.CommentsPeriod = period
		}
	}
	if cmd.Flags().Changed("comments-limit") {
		limit, err := cmd.Flags().GetInt("comments-limit")
		if err != nil {
			return opts, err
		}
		if limit < 0 {
			return opts, fmt.Errorf("--comments-limit cannot be negative")
		}
		opts.CommentsLimit = &limit
	}
	for _, name := range []string{"description-lines", "comment-chars"} {
		if cmd.Flags().Lookup(name) == nil {
			continue
		}
		value, err := cmd.Flags().GetInt(name)
		if err != nil {
			return opts, err
		}
		if value < 0 {
			return opts, fmt.Errorf("--%s cannot be negative", name)
		}
		if name == "description-lines" {
			opts.DescriptionLines = value
		} else {
			opts.CommentChars = value
			if cmd.Flags().Changed(name) && opts.CommentsLimit == nil {
				unlimited := 0
				opts.CommentsLimit = &unlimited
			}
		}
	}
	if cmd.Flags().Changed("include-development") {
		include, err := cmd.Flags().GetBool("include-development")
		if err != nil {
			return opts, err
		}
		opts.IncludeDevelopment = &include
	}
	return opts, nil
}
