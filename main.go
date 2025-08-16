package main

import (
	"flag"
	"fmt"
	"log"
	"os"

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
	cfg, err := config.LoadConfigFile()
	if err != nil {
		if headless {
			log.Fatal(ui.Error("failed to load configuration in headless mode: " + err.Error()))
		}
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

	if headless {
		if err := terminal.RunHeadless(client); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := terminal.RunTerminal(client); err != nil {
			log.Fatal(err)
		}
	}
}
