package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Styling ---
var (
	spinnerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	progressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	upToDateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	pathStyle     = lipgloss.NewStyle().Bold(true)
	statStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// --- Messages ---
type scanDoneMsg []string
type pullResultMsg struct {
	path    string
	changes bool
	commits int
	err     error
}

// --- Model ---
type model struct {
	repos        []string
	results      []pullResultMsg
	completed    int
	updatedCount int
	totalCommits int
	spinner      spinner.Model
	scanning     bool
	done         bool
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return model{
		spinner:  s,
		scanning: true,
	}
}

// --- Init ---
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		scanSubdirsCmd(),
	)
}

// --- Commands ---

// scanSubdirsCmd looks for subdirectories containing a .git folder.
func scanSubdirsCmd() tea.Cmd {
	return func() tea.Msg {
		entries, err := os.ReadDir(".")
		if err != nil {
			return scanDoneMsg{}
		}

		var gitDirs []string
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			gitPath := filepath.Join(entry.Name(), ".git")
			info, err := os.Stat(gitPath)
			if err == nil && info.IsDir() {
				gitDirs = append(gitDirs, entry.Name())
			}
		}
		return scanDoneMsg(gitDirs)
	}
}

// getHEAD is a helper to get the current commit hash
func getHEAD(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// pullRepoCmd runs `git pull --all` in the specified directory asynchronously.
func pullRepoCmd(path string) tea.Cmd {
	return func() tea.Msg {
		beforeHash := getHEAD(path)

		// git pull --all fetches all remotes and pulls the tracking branch
		cmd := exec.Command("git", "pull", "--all", "--tags")
		cmd.Dir = path

		cmd.Env = append(os.Environ(),
			"GIT_TERMINAL_PROMPT=0",
			"GIT_SSH_COMMAND=ssh -o BatchMode=yes",
		)

		out, err := cmd.CombinedOutput()
		outStr := string(out)

		if err != nil {
			cleanErr := strings.TrimSpace(outStr)

			if strings.Contains(cleanErr, "could not read Username") ||
				strings.Contains(cleanErr, "Permission denied") ||
				strings.Contains(cleanErr, "Authentication failed") ||
				strings.Contains(cleanErr, "Host key verification failed") {
				cleanErr = "Authentication required (skipped)"
			} else if cleanErr == "" {
				cleanErr = err.Error()
			} else {
				cleanErr = strings.ReplaceAll(cleanErr, "\r\n", " | ")
				cleanErr = strings.ReplaceAll(cleanErr, "\n", " | ")
			}

			if len(cleanErr) > 65 {
				cleanErr = cleanErr[:62] + "..."
			}

			return pullResultMsg{
				path:    path,
				changes: false,
				err:     errors.New(cleanErr),
			}
		}

		afterHash := getHEAD(path)
		commits := 0
		changes := false

		// Calculate if HEAD advanced to count commits
		if beforeHash != "" && afterHash != "" && beforeHash != afterHash {
			changes = true
			countCmd := exec.Command("git", "rev-list", "--count", beforeHash+".."+afterHash)
			countCmd.Dir = path
			countOut, _ := countCmd.Output()
			commits, _ = strconv.Atoi(strings.TrimSpace(string(countOut)))
		} else if !strings.Contains(outStr, "Already up to date") {
			// Catch-all for when other branches were fetched but HEAD didn't advance
			changes = true
		}

		return pullResultMsg{
			path:    path,
			changes: changes,
			commits: commits,
			err:     nil,
		}
	}
}

// --- Update ---
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}

	case scanDoneMsg:
		m.repos = msg
		m.scanning = false
		if len(m.repos) == 0 {
			m.done = true
			return m, tea.Quit
		}

		// Dispatch ALL git pull commands at once using tea.Batch
		var cmds []tea.Cmd
		for _, repo := range m.repos {
			cmds = append(cmds, pullRepoCmd(repo))
		}
		return m, tea.Batch(cmds...)

	case pullResultMsg:
		m.results = append(m.results, msg)
		m.completed++

		if msg.changes {
			m.updatedCount++
			m.totalCommits += msg.commits
		}

		// Check if all goroutines have returned
		if m.completed == len(m.repos) {
			m.done = true
			return m, tea.Quit
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// --- View ---
func (m model) View() string {
	var sb strings.Builder

	// 1. Render Completed Results (appearing as they finish)
	for _, res := range m.results {
		name := pathStyle.Render(res.path)
		if res.err != nil {
			fmt.Fprintf(&sb, "%s %s: %s\n", errorStyle.Render("✗"), name, errorStyle.Render(res.err.Error()))
		} else if res.changes {
			var commitText string
			if res.commits > 0 {
				commitText = fmt.Sprintf("Pulled %d commit(s)", res.commits)
			} else {
				commitText = "Fetched remote branches"
			}
			fmt.Fprintf(&sb, "%s %s: %s\n", successStyle.Render("✓"), name, successStyle.Render(commitText))
		} else {
			fmt.Fprintf(&sb, "%s %s: %s\n", upToDateStyle.Render("-"), name, upToDateStyle.Render("Already up to date"))
		}
	}

	// 2. Render Current State & Summary
	if m.done {
		if len(m.repos) == 0 {
			sb.WriteString("\nNo git repositories found in subdirectories.\n")
		} else {
			summary := fmt.Sprintf("\nFinished scanning %d repositories. %s updated ", len(m.repos), statStyle.Render(strconv.Itoa(m.updatedCount)))
			if m.updatedCount > 0 {
				summary += fmt.Sprintf("(%s total commits pulled).\n", statStyle.Render(strconv.Itoa(m.totalCommits)))
			} else {
				summary += ".\n"
			}
			sb.WriteString(progressStyle.Render(summary))
		}
	} else if m.scanning {
		fmt.Fprintf(&sb, "%s Scanning subdirectories for git repositories...\n", m.spinner.View())
	} else {
		// Active parallel progress
		progressText := fmt.Sprintf("(%d/%d)", m.completed, len(m.repos))
		fmt.Fprintf(&sb, "%s Updating repositories in parallel %s...\n", m.spinner.View(), progressStyle.Render(progressText))
	}

	if !m.done {
		sb.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Press q to quit"))
	}

	return sb.String()
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error starting app: %v\n", err)
		os.Exit(1)
	}
}
