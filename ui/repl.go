package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chzyer/readline"

	"github.com/vadiminshakov/autonomy/core/config"
)

// REPLCommands stores command history and provides REPL functionality
type REPLCommands struct {
	history  []string
	readline *readline.Instance
}

// createReadline creates a new readline instance with standard configuration
func createReadline() (*readline.Instance, error) {
	return readline.NewEx(&readline.Config{
		Prompt:            "",
		HistoryFile:       "/tmp/autonomy_history",
		AutoComplete:      completer,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		HistorySearchFold: true,
	})
}

// NewREPL creates a new REPL interface
func NewREPL() *REPLCommands {
	rl, err := createReadline()
	if err != nil {
		panic(err)
	}

	return &REPLCommands{
		history:  make([]string, 0),
		readline: rl,
	}
}

// completer provides auto-completion for built-in commands
var completer = readline.NewPrefixCompleter(
	readline.PcItem("help"),
	readline.PcItem("clear"),
	readline.PcItem("history"),
	readline.PcItem("reconfig"),
	readline.PcItem("exit"),
)

// Close releases REPL resources
func (r *REPLCommands) Close() {
	if r.readline != nil {
		r.readline.Close()
	}
}

// ShowWelcome prints the welcome message
func (r *REPLCommands) ShowWelcome() {
	fmt.Print("\033[2J\033[H")

	fmt.Println()
	fmt.Println(BrightCyan("🤖 AI programming assistant"))
	fmt.Println()
	fmt.Println(Info("Enter your programming tasks or commands"))
	fmt.Println(Dim("Available commands: help, clear, history, reconfig, exit"))
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
func (r *REPLCommands) ReadInput() (string, bool, bool) {
	r.readline.SetPrompt(r.GetPrompt())

	line, err := r.readline.Readline()
	if err != nil {
		if err == readline.ErrInterrupt {
			return "", false, false
		} else if err == io.EOF {
			return "", true, false
		}
		return "", true, false
	}

	// trim whitespace
	inputStr := strings.TrimSpace(line)

	if inputStr == "" {
		return "", false, false
	}

	// add to history
	r.history = append(r.history, inputStr)

	// handle built-in commands
	switch inputStr {
	case "exit":
		return "", true, false

	case "help":
		r.showHelp()
		return "", false, false

	case "clear":
		r.clear()
		return "", false, false

	case "history":
		r.showHistory()
		return "", false, false

	case "reconfig":
		if r.reconfig() {
			return "", false, true
		}
		return "", false, false

	default:
		return inputStr, false, false
	}
}

// showHelp prints built-in command help
func (r *REPLCommands) showHelp() {
	helpText := `Available commands:

General:
  help     – show this help
  clear    – clear the screen
  history  – show command history
  reconfig – recreate configuration
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

func (r *REPLCommands) reconfig() bool {
	fmt.Println()

	if r.readline != nil {
		r.readline.Close()
	}

	_, err := config.InteractiveSetup()

	rl, reinitErr := createReadline()
	if reinitErr != nil {
		fmt.Println(Error("failed to reinitialize readline: " + reinitErr.Error()))
		return false
	}
	r.readline = rl

	if err != nil {
		fmt.Println(Error("failed to reconfigure: " + err.Error()))
		fmt.Println()
		return false
	}
	fmt.Println(Success("configuration updated."))
	fmt.Println()
	return true
}
