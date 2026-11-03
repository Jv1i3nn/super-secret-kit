package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/viper"
)

// Run the real entrypoint in a child so a signal cannot terminate the test runner.
func TestCommandSignalProcess(t *testing.T) {
	if os.Getenv("SSK_TEST_SIGNAL_PROCESS") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			rootCmd.SetArgs(os.Args[index+1:])
			break
		}
	}
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, context.Canceled) {
			os.Exit(2)
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func signalTestProcess(t *testing.T, args ...string) (*exec.Cmd, context.Context) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	// Explicit paths and service URLs keep the child away from real account data.
	config := fmt.Sprintf("external_project_path: %q\ndatabase_url: %q\njira_api_url: %q\nhoroquartz_url: https://hq.example\n",
		viper.GetString("external_project_path"), viper.GetString("database_url"), viper.GetString("jira_api_url"))
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	childArgs := append([]string{"-test.run=^TestCommandSignalProcess$", "--", "--config", configPath}, args...)
	child := exec.CommandContext(ctx, executable, childArgs...)
	child.Env = append(os.Environ(), "SSK_TEST_SIGNAL_PROCESS=1")
	return child, ctx
}

func TestInteractivePromptRetainsSignalTermination(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			setupJiraOutputTest(t, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("credential prompt must not contact Jira")
			})
			child, ctx := signalTestProcess(t, "logtime", "--reset-credentials-jira")
			input, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = input.Close() }()
			output, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			child.Stderr = &stderr
			if startErr := child.Start(); startErr != nil {
				t.Fatal(startErr)
			}
			ready := make(chan error, 1)
			go func() {
				reader := bufio.NewReader(output)
				var text strings.Builder
				for {
					part, readErr := reader.ReadString(' ')
					text.WriteString(part)
					if strings.Contains(text.String(), "Please set your Jira username (email).") {
						ready <- nil
						return
					}
					if readErr != nil {
						ready <- readErr
						return
					}
				}
			}()
			select {
			case promptErr := <-ready:
				if promptErr != nil {
					_ = child.Wait()
					t.Fatalf("waiting for credential prompt: %v; stderr: %s", promptErr, stderr.String())
				}
			case <-ctx.Done():
				_ = child.Wait()
				t.Fatal("credential prompt did not appear")
			}
			if signalErr := child.Process.Signal(sig); signalErr != nil {
				t.Fatal(signalErr)
			}
			err = child.Wait()
			var exit *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exit) {
				t.Fatalf("prompt did not terminate on %s: %v (deadline: %v)", sig, err, ctx.Err())
			}
			status, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != sig {
				t.Fatalf("expected termination by %s, got %v", sig, err)
			}
		})
	}
}

func TestJiraSignalsCancelActiveRequests(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			reached, canceled := make(chan struct{}), make(chan struct{})
			setupJiraOutputTest(t, func(_ http.ResponseWriter, r *http.Request) {
				close(reached)
				<-r.Context().Done()
				close(canceled)
			})
			child, ctx := signalTestProcess(t, "jira", "query", "project = EX", "--format", "json")
			var stdout, stderr bytes.Buffer
			child.Stdout, child.Stderr = &stdout, &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-reached:
			case <-ctx.Done():
				_ = child.Wait()
				t.Fatalf("Jira request did not start: %s", stderr.String())
			}
			if err := child.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			err := child.Wait()
			var exit *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exit) || exit.ExitCode() != 2 {
				t.Fatalf("expected context cancellation on %s: %v; stderr: %s", sig, err, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("cancellation emitted partial JSON: %s", stdout.String())
			}
			select {
			case <-canceled:
			case <-ctx.Done():
				t.Fatal("Jira request was not canceled")
			}
		})
	}
}
