package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/index"
	"github.com/vadiminshakov/autonomy/core/task"
	"github.com/vadiminshakov/autonomy/terminal"
	"github.com/vadiminshakov/autonomy/ui"
)

func main() {
	provider := flag.String("provider", "auto", "AI provider: openai|anthropic|auto (default auto)")
	flag.Parse()

	var client task.AIClient
	var err error

	switch *provider {
	case "auto":
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			*provider = "anthropic"
		} else if os.Getenv("OPENAI_API_KEY") != "" {
			*provider = "openai"
		} else {
			log.Fatal("environment variable OPENAI_API_KEY or ANTHROPIC_API_KEY is not set")
		}
	}

	fmt.Printf("Using provider: %s\n", *provider)

	switch *provider {
	case "openai":
		if os.Getenv("OPENAI_API_KEY") == "" {
			log.Fatal(ui.Error("OPENAI_API_KEY is not set"))
		}
		client, err = ai.NewOpenai()

	case "anthropic":
		if os.Getenv("ANTHROPIC_API_KEY") == "" {
			log.Fatal(ui.Error("ANTHROPIC_API_KEY is not set"))
		}
		client, err = ai.NewAnthropic()

	default:
		log.Fatalf("you need to specify provider with -provider anthropic|openai flag, got %s", *provider)
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
