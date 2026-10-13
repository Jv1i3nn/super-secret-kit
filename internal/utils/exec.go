package utils

import (
	"os/exec"
)

// ExecuteCommand executes a system command with arguments
func ExecuteCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}
