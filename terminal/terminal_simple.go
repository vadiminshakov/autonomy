package terminal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/config"
	"github.com/vadiminshakov/autonomy/core/task"
)

func RunHeadlessSimple() error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "exit" || input == "quit" {
			break
		}
	}
	return nil
}

func RunHeadlessWithInit() error {
	var client ai.AIClient
	var initialized = false
	var initError error

	// Send ready signal immediately for VS Code
	fmt.Println("Autonomy agent is ready! Enter your programming tasks or commands.")
	fmt.Fprintf(os.Stderr, "Autonomy agent is ready! Enter your programming tasks or commands.\n")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "exit" || input == "quit" {
			break
		}

		if input == "" {
			continue
		}

		// Initialize AI client on first real task
		if !initialized {
			cfg, err := config.LoadConfigFile()
			if err != nil {
				fmt.Printf("❌ Configuration error: %v\n", err)
				initError = err
				continue
			}

			// Create AI client with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			initChan := make(chan error, 1)

			go func() {
				c, err := ai.ProvideAiClient(cfg)
				if err != nil {
					initChan <- err
				} else {
					client = c
					initChan <- nil
				}
			}()

			select {
			case err := <-initChan:
				cancel()
				if err != nil {
					fmt.Printf("❌ Failed to initialize AI client: %v\n", err)
					initError = err
					continue
				}
				initialized = true
				initError = nil
			case <-ctx.Done():
				cancel()
				fmt.Println("❌ Agent initialization timeout - check your API configuration")
				initError = fmt.Errorf("initialization timeout")
				continue
			}
		}

		// If we had init errors but user keeps trying, show the error
		if initError != nil {
			fmt.Printf("❌ Agent not initialized: %v\n", initError)
			continue
		}

		// Process the task
		t := task.NewTask(client)
		t.SetOriginalTask(input)
		t.AddUserMessage(input)

		err := t.ProcessTask()
		t.Close()

		if err != nil {
			fmt.Printf("❌ Task failed: %v\n", err)
		}
	}

	return nil
}
