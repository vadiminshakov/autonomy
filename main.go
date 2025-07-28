package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/index"
	"github.com/vadiminshakov/autonomy/core/task"
	"github.com/vadiminshakov/autonomy/terminal"
	"github.com/vadiminshakov/autonomy/ui"
)

func main() {
	var client task.AIClient

	// Parse command line flags
	var headless = flag.Bool("headless", false, "Run in headless mode (for VS Code extension)")
	var version = flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *version {
		fmt.Println("Autonomy v0.1.0")
		os.Exit(0)
	}

	// try to load configuration from file, otherwise start interactive setup
	cfg, err := config.LoadConfigFile()
	if err != nil {
		if *headless {
			log.Fatal(ui.Error("failed to load configuration in headless mode: " + err.Error()))
		}
		cfg, err = config.InteractiveSetup()
		if err != nil {
			log.Fatal(ui.Error("failed to set up configuration: " + err.Error()))
		}
	}

	switch cfg.Provider {
	case "openai":
		client, err = ai.NewOpenai(cfg)
	case "anthropic":
		client, err = ai.NewAnthropic(cfg)

	case "openrouter":
		client, err = ai.NewOpenai(cfg)
	case "local":
		client, err = ai.NewOpenai(cfg)
	default:
		log.Fatalf("unknown provider %s in config", cfg.Provider)
	}

	if err != nil {
		log.Fatal(ui.Error("failed to create AI client: " + err.Error()))
	}

	indexManager := index.GetIndexManager()
	if err := indexManager.Initialize(); err != nil {
		ui.ShowIndexWarning(fmt.Sprintf("Failed to initialize index manager: %v", err))
	}
	indexManager.StartAutoRebuild()
	defer indexManager.StopAutoRebuild()

	if *headless {
		if err := terminal.RunHeadless(client); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := terminal.RunTerminal(client); err != nil {
			log.Fatal(err)
		}
	}
}
