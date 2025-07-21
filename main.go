package main

import (
	"fmt"
	"log"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/index"
	"github.com/vadiminshakov/autonomy/core/task"
	"github.com/vadiminshakov/autonomy/terminal"
	"github.com/vadiminshakov/autonomy/ui"
)

func main() {
	var client task.AIClient

	// try to load configuration from file, otherwise start interactive setup
	cfg, err := config.LoadConfigFile()
	if err != nil {
		cfg, err = config.InteractiveSetup()
		if err != nil {
			log.Fatal(ui.Error("failed to set up configuration: " + err.Error()))
		}
	}

	fmt.Printf("Using provider: %s\n", cfg.Provider)

	switch cfg.Provider {
	case "openai":
		client, err = ai.NewOpenai(cfg)
	case "anthropic":
		client, err = ai.NewAnthropic(cfg)

	case "openrouter":
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

	if err := terminal.RunTerminal(client); err != nil {
		log.Fatal(err)
	}
}
