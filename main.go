package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
)

// --- Messages ---
type scanDoneMsg []string
type pullResultMsg struct {
	path    string
	changes bool
	err     error
}

// --- Model ---
type model struct {
	repos    []string
	results  []pullResultMsg
	index    int
	spinner  spinner.Model
	scanning bool
	done     bool
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

// pullRepoCmd runs `git pull` in the specified directory.
func pullRepoCmd(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("git", "pull")
		cmd.Dir = path
		
		// Prevent git/ssh from hanging while waiting for user input
		cmd.Env = append(os.Environ(),
			"GIT_TERMINAL_PROMPT=0",
			"GIT_SSH_COMMAND=ssh -o BatchMode=yes",
		)

		out, err := cmd.CombinedOutput()
		outStr := string(out)

		if err != nil {
			cleanErr := strings.TrimSpace(outStr)
			
			// Detect authentication failures
			if strings.Contains(cleanErr, "could not read Username") || 
			   strings.Contains(cleanErr, "Permission denied") || 
			   strings.Contains(cleanErr, "Authentication failed") || 
			   strings.Contains(cleanErr, "Host key verification failed") {
				cleanErr = "Authentication required (skipped)"
			} else if cleanErr == "" {
				cleanErr = err.Error()
			} else {
				// Convert multiline errors into a single line
				cleanErr = strings.ReplaceAll(cleanErr, "\r\n", " | ")
				cleanErr = strings.ReplaceAll(cleanErr, "\n", " | ")
			}

			// Truncate to ensure it fits nicely on a standard terminal line
			if len(cleanErr) > 65 {
				cleanErr = cleanErr[:62] + "..."
			}

			return pullResultMsg{
				path:    path,
				changes: false,
				err:     fmt.Errorf(cleanErr),
			}
		}

		// Detect if changes were actually pulled
		changes := !strings.Contains(outStr, "Already up to date")

		return pullResultMsg{
			path:    path,
			changes: changes,
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
		return m, pullRepoCmd(m.repos[m.index])

	case pullResultMsg:
		m.results = append(m.results, msg)
		m.index++

		if m.index < len(m.repos) {
			return m, pullRepoCmd(m.repos[m.index])
		}

		m.done = true
		return m, tea.Quit

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

	for _, res := range m.results {
		name := pathStyle.Render(res.path)
		if res.err != nil {
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", errorStyle.Render("✗"), name, errorStyle.Render(res.err.Error())))
		} else if res.changes {
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", successStyle.Render("✓"), name, successStyle.Render("Changes pulled")))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s: %s\n", upToDateStyle.Render("-"), name, upToDateStyle.Render("Already up to date")))
		}
	}

	if m.done {
		if len(m.repos) == 0 {
			sb.WriteString("\nNo git repositories found in subdirectories.\n")
		} else {
			sb.WriteString(progressStyle.Render(fmt.Sprintf("\nFinished updating %d repositories.\n", len(m.repos))))
		}
	} else if m.scanning {
		sb.WriteString(fmt.Sprintf("%s Scanning subdirectories for git repositories...\n", m.spinner.View()))
	} else {
		progressText := fmt.Sprintf("(%d/%d)", m.index+1, len(m.repos))
		currentRepo := m.repos[m.index]
		sb.WriteString(fmt.Sprintf("%s Updating %s %s...\n", m.spinner.View(), pathStyle.Render(currentRepo), progressStyle.Render(progressText)))
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
