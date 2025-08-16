package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chzyer/readline"

	"github.com/vadiminshakov/autonomy/core/config"
)

type REPLCommands struct {
	history  []string
	readline *readline.Instance
}

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

var completer = readline.NewPrefixCompleter(
	readline.PcItem("help"),
	readline.PcItem("clear"),
	readline.PcItem("history"),
	readline.PcItem("reconfig"),
	readline.PcItem("exit"),
)

func (r *REPLCommands) Close() {
	if r.readline != nil {
		r.readline.Close()
	}
}

func (r *REPLCommands) ShowWelcome() {
	fmt.Print("\033[2J\033[H")

	fmt.Println()
	fmt.Println(BrightCyan("AI programming assistant"))
	fmt.Println()
	fmt.Println(BrightBlue("Enter your programming tasks or commands"))
	fmt.Println(Dim("Available commands: help, clear, history, reconfig, exit"))
	fmt.Println()
}

func (r *REPLCommands) GetPrompt() string {
	timestamp := time.Now().Format("15:04")
	return fmt.Sprintf("%s [%s] %s ",
		BrightBlue("autonomy"),
		Dim(timestamp),
		BrightGreen("❯"))
}

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

	inputStr := strings.TrimSpace(line)

	if inputStr == "" {
		return "", false, false
	}

	r.history = append(r.history, inputStr)

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

func (r *REPLCommands) clear() {
	fmt.Print("\033[2J\033[H")
	fmt.Println(BrightCyan("Screen cleared"))
	fmt.Println()
}

func (r *REPLCommands) showHistory() {
	fmt.Println()
	if len(r.history) == 0 {
		fmt.Println(BrightBlue("Command history is empty"))
		fmt.Println()
		return
	}

	fmt.Println(BrightCyan("Command history:"))
	fmt.Println()

	start := len(r.history) - 10
	if start < 0 {
		start = 0
	}

	for i, cmd := range r.history[start:] {
		fmt.Printf("%s %s %s\n",
			Dim(fmt.Sprintf("%d.", i+start+1)),
			BrightBlue("→"),
			cmd)
	}
	fmt.Println()
}

func ShowError(err error) {
	fmt.Println()
	fmt.Println(BrightRed(err.Error()))
	fmt.Println()
}

func ShowTaskStart(task string) {
	fmt.Println()
	fmt.Println(BrightPurple("Running task: " + task))
	fmt.Println(Dim(strings.Repeat("─", 50)))
}

func ShowTaskComplete() {
	fmt.Println(Dim(strings.Repeat("─", 50)))
	fmt.Println(BrightGreen("Task completed!"))
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
		fmt.Println(BrightRed("failed to reinitialize readline: " + reinitErr.Error()))
		return false
	}
	r.readline = rl

	if err != nil {
		fmt.Println(BrightRed("failed to reconfigure: " + err.Error()))
		fmt.Println()
		return false
	}
	fmt.Println(BrightGreen("configuration updated."))
	fmt.Println()
	return true
}
