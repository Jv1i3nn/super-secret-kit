package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"ssk/internal/config"
)

func jiraOutputFormat(cmd *cobra.Command, defaultFormat string) (string, error) {
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return "", err
	}
	if format != defaultFormat && format != "json" {
		return "", fmt.Errorf("--format must be %s or json", defaultFormat)
	}
	return format, nil
}

func effectiveJiraWorkspace() string {
	if jiraWorkspace == "" {
		return config.DefaultJiraWorkspace
	}
	return jiraWorkspace
}

// Marshal before writing so normalization/encoding errors cannot emit partial JSON.
func writeJiraJSON(output io.Writer, result interface{}) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Jira JSON: %w", err)
	}
	data = append(data, '\n')
	_, err = output.Write(data)
	return err
}
