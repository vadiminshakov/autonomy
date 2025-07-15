package ui

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// REPLCommands stores command history and provides REPL functionality
type REPLCommands struct {
	history []string
	reader  *bufio.Reader
}

// NewREPL creates a new REPL interface
func NewREPL() *REPLCommands {
	setupTerminal()

	return &REPLCommands{
		history: make([]string, 0),
		reader:  bufio.NewReader(os.Stdin),
	}
}

// setupTerminal configures the terminal mode
func setupTerminal() {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		cmd := exec.Command("stty", "icanon", "echo")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
}

// Close releases REPL resources (no-op)
func (r *REPLCommands) Close() {}

// ShowWelcome prints the welcome message
func (r *REPLCommands) ShowWelcome() {
	fmt.Print("\033[2J\033[H")

	fmt.Println()
	fmt.Println(BrightCyan("🤖 AI programming assistant"))
	fmt.Println()
	fmt.Println(Info("Enter your programming tasks or commands"))
	fmt.Println(Dim("Available commands: help, clear, history, exit"))
	fmt.Println()
}

// GetPrompt returns a styled prompt for user input
func (r *REPLCommands) GetPrompt() string {
	timestamp := time.Now().Format("15:04")
	return fmt.Sprintf("%s [%s] %s ",
		BrightBlue("autonomy"),
		Dim(timestamp),
		BrightGreen("❯"))
}

// ReadInput reads user input and handles built-in commands
func (r *REPLCommands) ReadInput() (string, bool) {
	fmt.Print(r.GetPrompt())

	line, err := r.reader.ReadString('\n')
	if err != nil {
		return "", true
	}

	// remove trailing newlines and control characters
	inputStr := strings.TrimRight(line, "\r\n")
	inputStr = strings.TrimSpace(inputStr)

	// filter out control characters (including ^M)
	var filtered []rune
	for _, r := range inputStr {
		if r >= 32 || r == '\t' { // keep printable chars and tabs
			filtered = append(filtered, r)
		}
	}
	inputStr = string(filtered)

	if inputStr == "" {
		return "", false
	}

	// add to history
	r.history = append(r.history, inputStr)

	// handle built-in commands
	switch inputStr {
	case "exit":
		return "", true

	case "help":
		r.showHelp()
		return "", false

	case "clear":
		r.clear()
		return "", false

	case "history":
		r.showHistory()
		return "", false

	default:
		return inputStr, false
	}
}

// showHelp prints built-in command help
func (r *REPLCommands) showHelp() {
	helpText := `Available commands:

General:
  help     – show this help
  clear    – clear the screen
  history  – show command history
  exit     – quit the program`

	fmt.Println(helpText)
}

// clear clears the terminal screen
func (r *REPLCommands) clear() {
	fmt.Print("\033[2J\033[H")
	fmt.Println(BrightCyan("🧹 Screen cleared"))
	fmt.Println()
}

// showHistory prints command history
func (r *REPLCommands) showHistory() {
	fmt.Println()
	if len(r.history) == 0 {
		fmt.Println(Info("Command history is empty"))
		fmt.Println()
		return
	}

	fmt.Println(BrightCyan("📜 Command history:"))
	fmt.Println()

	start := 0
	if len(r.history) > 10 {
		start = len(r.history) - 10
		fmt.Println(Dim("... (showing last 10 commands)"))
	}

	for i := start; i < len(r.history); i++ {
		cmd := r.history[i]
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		fmt.Printf("%s %s %s\n",
			Dim(fmt.Sprintf("%2d.", i+1)),
			BrightWhite(cmd),
			Dim(time.Now().Format("15:04")))
	}
	fmt.Println()
}

// ShowError prints the error in a formatted style
func ShowError(err error) {
	fmt.Println()
	fmt.Println(Error(err.Error()))
	fmt.Println()
}

// ShowTaskStart prints a start-task banner
func ShowTaskStart(task string) {
	fmt.Println()
	fmt.Println(AI("Running task: " + task))
	fmt.Println(Dim(strings.Repeat("─", 50)))
}

// ShowTaskComplete prints a task-completed banner
func ShowTaskComplete() {
	fmt.Println(Dim(strings.Repeat("─", 50)))
	fmt.Println(Success("Task completed!"))
	fmt.Println()
}
