package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/index"
	"github.com/vadiminshakov/autonomy/terminal"
	"github.com/vadiminshakov/autonomy/ui"
)

func main() {
	var headless = flag.Bool("headless", false, "Run in headless mode (for VS Code extension)")
	var version = flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *version {
		fmt.Println("Autonomy v0.1.0")
		os.Exit(0)
	}

	runProgram(*headless)
}

func runProgram(headless bool) {
	if headless {
		if vscodePID := os.Getenv("VSCODE_PID"); vscodePID != "" {
			go monitorVSCodeProcess(vscodePID)
		}

		if err := terminal.RunHeadlessWithInit(); err != nil {
			log.Fatal(err)
		}

		return
	}

	cfg, err := config.LoadConfigFile()
	if err != nil {
		cfg, err = config.InteractiveSetup()
		if err != nil {
			log.Fatal(ui.Error("failed to set up configuration: " + err.Error()))
		}
	}

	client, err := ai.ProvideAiClient(cfg)
	if err != nil {
		log.Fatal(ui.Error("failed to create AI client: " + err.Error()))
	}

	if !headless {
		indexManager := index.GetIndexManager()
		if err := indexManager.Initialize(); err != nil {
			ui.ShowIndexWarning(fmt.Sprintf("Failed to initialize index manager: %v", err))
		}
		indexManager.StartAutoRebuild()
		defer indexManager.StopAutoRebuild()
	}

	if err := terminal.RunTerminal(client); err != nil {
		log.Fatal(err)
	}
}

func monitorVSCodeProcess(vscodePIDStr string) {
	vscodePID, err := strconv.Atoi(vscodePIDStr)
	if err != nil {

		return
	}

	for {
		time.Sleep(10 * time.Second)

		// сheck if VSCode process is still running
		if !isProcessRunning(vscodePID) {
			os.Exit(0)
		}
	}
}

func isProcessRunning(pid int) bool {
	// on Unix systems, sending signal 0 checks if process exists
	err := syscall.Kill(pid, 0)
	return err == nil
}
