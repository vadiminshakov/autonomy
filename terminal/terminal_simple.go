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
		fmt.Printf("Task: %s\n", input)
	}
	return nil
}

func RunHeadlessWithInit() error {
	var client ai.AIClient
	var initialized = false

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
				fmt.Printf("Error: No configuration found. %v\n", err)
				continue
			}

			// Create AI client with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
					fmt.Printf("Error: Failed to create AI client: %v\n", err)
					continue
				}
				initialized = true
			case <-ctx.Done():
				cancel()
				fmt.Printf("Error: AI client initialization timed out\n")
				continue
			}
		}

		// Process the task
		t := task.NewTask(client)
		t.SetOriginalTask(input)
		t.AddUserMessage(input)

		err := t.ProcessTask()
		t.Close()

		if err != nil {
			fmt.Printf("Task failed: %v\n", err)
		} else {
			fmt.Printf("Task completed\n")
		}
	}

	return nil
}
