package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/core/ai"
	"github.com/vadiminshakov/autonomy/core/decomposition"
	"github.com/vadiminshakov/autonomy/core/entity"
	"github.com/vadiminshakov/autonomy/core/tools"
	"github.com/vadiminshakov/autonomy/ui"
)

// Config holds task execution configuration
type Config struct {
	MaxIterations        int
	MaxHistorySize       int
	AICallTimeout        time.Duration
	ToolTimeout          time.Duration
	MinAPIInterval       time.Duration
	MaxNoToolAttempts    int
	EnableReflection     bool
	EnableFileValidation bool
}

func defaultConfig() Config {
	return Config{
		MaxIterations:        100,
		MaxHistorySize:       80,
		AICallTimeout:        300 * time.Second,
		ToolTimeout:          30 * time.Second,
		MinAPIInterval:       1 * time.Second,
		MaxNoToolAttempts:    5,
		EnableReflection:     true,
		EnableFileValidation: true,
	}
}

// Task manages AI-driven task execution
type Task struct {
	client     ai.AIClient
	promptData *entity.PromptData
	config     Config

	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	noToolCount int

	originalTask string
}

// NewTask creates a new task with default configuration
func NewTask(client ai.AIClient) *Task {
	return NewTaskWithConfig(client, defaultConfig())
}

// NewTaskWithConfig creates a new task with custom configuration
func NewTaskWithConfig(client ai.AIClient, config Config) *Task {
	ctx, cancel := context.WithCancel(context.Background())

	return &Task{
		client:     client,
		promptData: NewPromptData(),
		config:     config,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// SetOriginalTask sets the original task description for reflection
func (t *Task) SetOriginalTask(task string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.originalTask = task
}

// Close releases task resources
func (t *Task) Close() {
	if t.cancel != nil {
		t.cancel()
	}
}

// AddUserMessage adds a user message to history
func (t *Task) AddUserMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("user", message)
	t.trimHistoryIfNeeded()
}

// ProcessTask executes the main task loop
func (t *Task) ProcessTask() error {
	defer t.Close()

	// основной цикл выполнения задач
	for iter := 0; iter < t.config.MaxIterations; iter++ {
		if err := t.checkCancellation(); err != nil {
			return err
		}

		response, err := t.callAi()
		if err != nil {
			return fmt.Errorf("AI call failed: %v", err)
		}

		if response.Content != "" {
			fmt.Printf("%s\n", NormalizeOutput(response.Content))
		}

		if len(response.ToolCalls) == 0 {
			t.addAssistantMessage(response.Content)
			if shouldAbort := t.handleNoTools(); shouldAbort {
				return fmt.Errorf("task execution timed out")
			}
			continue
		}

		t.promptData.AddAssistantMessageWithTools(response.Content, response.ToolCalls)
		t.resetNoToolCount()

		completed, err := t.executeTools(response.ToolCalls)
		if err != nil {
			return fmt.Errorf("tool execution failed: %v", err)
		}

		if completed {
			return nil
		}
	}

	return fmt.Errorf("task execution timed out after %d iterations", t.config.MaxIterations)
}

func (t *Task) callAi() (*entity.AIResponse, error) {
	ctx, cancel := context.WithTimeout(t.ctx, t.config.AICallTimeout)
	defer cancel()

	t.mu.RLock()
	promptCopy := t.copyPromptData()
	t.mu.RUnlock()

	spinner := ui.ShowThinking()
	defer spinner.Stop()

	response, err := t.client.GenerateCode(ctx, promptCopy)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (t *Task) executeTools(calls []entity.ToolCall) (bool, error) {
	// проверяем, есть ли декомпозированная задача
	if hasDecomposedTask() {
		return t.executeDecomposition()
	}

	// иначе выполняем tool calls обычным способом
	return t.executeSequential(calls)
}

// executeDecomposition выполняет декомпозированную задачу напрямую
func (t *Task) executeDecomposition() (bool, error) {
	decomposedTask, err := getDecomposedTask()
	if err != nil {
		return false, fmt.Errorf("failed to get decomposed task: %v", err)
	}

	clearDecomposedTask()

	fmt.Print(ui.Dim(decomposedTask.GetStepSummary()))
	fmt.Printf("Executing decomposed task with %d steps...\n", len(decomposedTask.Steps))

	ctx, cancel := context.WithTimeout(t.ctx, 10*time.Minute)
	defer cancel()

	for i, step := range decomposedTask.Steps {
		if err := t.checkContext(ctx); err != nil {
			return false, err
		}

		fmt.Printf("Step %d/%d: %s\n", i+1, len(decomposedTask.Steps), step.Description)

		// выполняем логический шаг
		result, err := t.executeLogicalTaskStep(ctx, step)
		if err != nil {
			fmt.Printf("Step execution failed: %v\n", err)
			return false, err
		}

		fmt.Printf("Step %d completed: %s\n", i+1, result)
	}

	return false, nil
}

// executeLogicalTaskStep выполняет логический шаг задачи, отправляя описание модели
func (t *Task) executeLogicalTaskStep(ctx context.Context, step decomposition.TaskStep) (string, error) {
	// создаем сообщение с описанием шага
	stepMessage := entity.Message{
		Role:    "user",
		Content: fmt.Sprintf("Execute this step: %s", step.Description),
	}

	// добавляем сообщение в историю
	t.promptData.Messages = append(t.promptData.Messages, stepMessage)

	// получаем ответ от модели используя существующий метод
	response, err := t.client.GenerateCode(ctx, *t.promptData)
	if err != nil {
		return "", fmt.Errorf("failed to get AI response for logical step: %v", err)
	}

	// добавляем ответ модели в историю
	t.promptData.Messages = append(t.promptData.Messages, entity.Message{
		Role:    "assistant",
		Content: response.Content,
	})

	// если модель вернула tool calls, выполняем их
	if len(response.ToolCalls) > 0 {
		completed, err := t.executeTools(response.ToolCalls)
		if err != nil {
			return "", err
		}
		if completed {
			return "Step completed successfully", nil
		}
	}

	return response.Content, nil
}

func (t *Task) executeSequential(calls []entity.ToolCall) (bool, error) {
	if len(calls) > 1 {
		fmt.Printf("executing %d tools sequentially...\n", len(calls))
	}

	ctx, cancel := context.WithTimeout(t.ctx, 5*time.Minute)
	defer cancel()

	for _, call := range calls {
		if err := t.checkContext(ctx); err != nil {
			return false, err
		}

		// выполняем инструмент
		result, err := t.exec(ctx, call)

		// обрабатываем результат
		t.handleToolResult(call, result, err)

		// проверяем завершение
		if call.Name == "attempt_completion" && err == nil {
			return true, nil
		}
	}

	return false, nil
}

func (t *Task) exec(ctx context.Context, call entity.ToolCall) (string, error) {
	// Check if tool is available before execution
	availableTools := tools.List()
	toolExists := false
	for _, tool := range availableTools {
		if tool == call.Name {
			toolExists = true
			break
		}
	}

	if !toolExists {
		return "", fmt.Errorf("tool %s is not available", call.Name)
	}

	timeout := t.getToolTimeout(call.Name)
	toolCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultChan := make(chan struct {
		res string
		err error
	}, 1)

	go func() {
		res, err := tools.Execute(call.Name, call.Args)
		resultChan <- struct {
			res string
			err error
		}{res, err}
	}()

	select {
	case result := <-resultChan:
		return result.res, result.err
	case <-toolCtx.Done():
		return "", fmt.Errorf("tool %s timed out after %v", call.Name, timeout)
	}
}

func (t *Task) getToolTimeout(toolName string) time.Duration {
	longRunningTools := map[string]time.Duration{
		"decompose_task":  9 * time.Minute,
		"go_test":         2 * time.Minute,
		"search_index":    2 * time.Minute,
		"analyze_code_go": 1 * time.Minute,
	}

	if timeout, ok := longRunningTools[toolName]; ok {
		return timeout
	}

	return t.config.ToolTimeout
}

func (t *Task) handleToolResult(call entity.ToolCall, result string, err error) {
	if err != nil {
		fmt.Println(ui.Error(fmt.Sprintf("Error running %s: %v", call.Name, err)))
		if result != "" && !isSilentTool(call.Name) {
			fmt.Printf("%s\n", ui.Dim("Result: "+result))
		}
		t.promptData.AddToolResponse(call.ID, fmt.Sprintf("Error: %v. Result: %s", err, result))
		return
	}

	if isSilentTool(call.Name) {
		summary := silentToolSummary(call.Name, call.Args, result)
		fmt.Println(ui.Success("✓ "+call.Name) + summary)
	} else {
		fmt.Println(ui.Success("✓ " + call.Name))

		if call.Name == "find_files" {
			argsInfo := formatFindFilesArgs(call.Args)
			if argsInfo != "" {
				fmt.Printf("%s\n", ui.Info("Arguments: "+argsInfo))
			}
		}

		if result != "" {
			if call.Name == "attempt_completion" {
				fmt.Println(ui.Info(result))
			} else if isFileOperation(call.Name) {
				// специальная обработка для файловых операций
				displayResult := limitFileToolOutput(result)
				normalizedResult := NormalizeOutput(displayResult)
				fmt.Printf("```\n%s\n```\n", normalizedResult)
			} else {
				result = limitToolOutput(result)
				// нормализуем вывод для корректного отображения Unicode
				normalizedResult := NormalizeOutput(result)
				fmt.Printf("Tool result: %s\n", normalizedResult)
			}
		}
	}

	// ограничиваем результат для истории AI (не более 2000 символов)
	historyResult := limitToolOutput(result)
	t.promptData.AddToolResponse(call.ID, historyResult)
}

func limitToolOutput(result string) string {
	maxLines := 20
	maxChars := 2000

	if len(result) > maxChars {
		result = result[:maxChars] + "\n... [truncated]"
	}

	lines := strings.Split(result, "\n")
	if len(lines) > maxLines {
		truncated := strings.Join(lines[:maxLines], "\n")
		return truncated + "\n... [truncated]"
	}

	return result
}

// isFileOperation проверяет является ли инструмент файловой операцией
func isFileOperation(toolName string) bool {
	switch toolName {
	case "read_file", "write_file", "lsp_edit":
		return true
	default:
		return false
	}
}

// limitFileToolOutput ограничивает вывод файловых операций до 13 строк и 1500 символов
func limitFileToolOutput(result string) string {
	maxLines := 13
	maxChars := 1500

	// сначала ограничиваем по символам
	if len(result) > maxChars {
		result = result[:maxChars] + "\n... [показаны первые 1500 символов]"
	}

	// затем по строкам
	lines := strings.Split(result, "\n")
	if len(lines) > maxLines {
		truncated := strings.Join(lines[:maxLines], "\n")
		return truncated + "\n... [показаны первые 13 строк]"
	}

	return result
}

// NormalizeOutput нормализует Unicode и убирает проблемные символы
func NormalizeOutput(s string) string {
	// заменяем типографские кавычки и апострофы на обычные
	replacements := map[string]string{
		"\u2018": "'",   // Left single quotation mark
		"\u2019": "'",   // Right single quotation mark
		"\u201C": "\"",  // Left double quotation mark
		"\u201D": "\"",  // Right double quotation mark
		"\u2013": "-",   // En dash
		"\u2014": "-",   // Em dash
		"\u2026": "...", // Ellipsis
	}

	result := s
	for old, new := range replacements {
		result = strings.ReplaceAll(result, old, new)
	}

	// убираем только действительно проблемные управляющие символы
	var clean strings.Builder
	for _, r := range result {
		// разрешаем: переводы строк, табы, обычные печатные символы и Unicode символы
		if r == '\n' || r == '\t' || r == '\r' || (r >= 32 && r != 127) || r > 127 {
			clean.WriteRune(r)
		}
	}

	return strings.TrimSpace(clean.String())
}

func (t *Task) checkCancellation() error {
	select {
	case <-t.ctx.Done():
		return fmt.Errorf("task canceled: %v", t.ctx.Err())
	default:
		return nil
	}
}

func (t *Task) checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("operation timed out or was canceled")
	default:
		return nil
	}
}

func (t *Task) addAssistantMessage(content string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("assistant", content)
	t.trimHistoryIfNeeded()
}

func (t *Task) addUserMessage(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.promptData.AddMessage("user", message)
	t.trimHistoryIfNeeded()
}

func (t *Task) trimHistoryIfNeeded() {
	if len(t.promptData.Messages) > t.config.MaxHistorySize {
		cut := len(t.promptData.Messages) / 2

		if cut < 1 {
			cut = 1
		}

		contextMsg := t.contextCompaction(cut)

		if contextMsg != "" {
			summaryMsg := entity.Message{
				Role:    "system",
				Content: contextMsg,
			}

			t.promptData.Messages = append(
				[]entity.Message{t.promptData.Messages[0], summaryMsg},
				t.promptData.Messages[cut+1:]...,
			)
		} else {
			t.promptData.Messages = append(
				t.promptData.Messages[:1],
				t.promptData.Messages[cut+1:]...,
			)
		}
	}
}

func (t *Task) contextCompaction(messagesToTrim int) string {
	var toolsUsed []string
	var filesModified []string
	seenTools := make(map[string]bool)
	seenFiles := make(map[string]bool)

	for i := 1; i <= messagesToTrim && i < len(t.promptData.Messages); i++ {
		msg := t.promptData.Messages[i]
		content := strings.ToLower(msg.Content)

		if strings.Contains(content, "result of ") {
			parts := strings.Split(content, ":")
			if len(parts) > 0 {
				toolName := strings.TrimSpace(strings.TrimPrefix(parts[0], "result of "))
				if !seenTools[toolName] {
					toolsUsed = append(toolsUsed, toolName)
					seenTools[toolName] = true
				}
			}
		}

		if strings.Contains(content, "write_file") || strings.Contains(content, "lsp_edit") {
			if idx := strings.Index(content, "path:"); idx != -1 {
				pathPart := content[idx+5:]
				if endIdx := strings.IndexAny(pathPart, " \n,}"); endIdx != -1 {
					filename := strings.TrimSpace(pathPart[:endIdx])
					if !seenFiles[filename] {
						filesModified = append(filesModified, filename)
						seenFiles[filename] = true
					}
				}
			}
		}
	}

	if len(toolsUsed) == 0 && len(filesModified) == 0 {
		return ""
	}

	var contextParts []string
	contextParts = append(contextParts, "CONTEXT FROM PREVIOUS MESSAGES:")

	if len(toolsUsed) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("Tools already used: %s", strings.Join(toolsUsed, ", ")))
	}

	if len(filesModified) > 0 {
		contextParts = append(contextParts, fmt.Sprintf("Files modified: %s", strings.Join(filesModified, ", ")))
	}

	contextParts = append(contextParts, "Continue building on this work.")

	return strings.Join(contextParts, "\n")
}

func (t *Task) handleNoTools() bool {
	t.mu.Lock()
	t.noToolCount++
	count := t.noToolCount
	t.mu.Unlock()

	if count >= t.config.MaxNoToolAttempts {
		fmt.Println(ui.Warning("AI failed to use tools after multiple attempts. Aborting."))
		return true
	}

	t.forceToolUsage()

	return false
}

func (t *Task) resetNoToolCount() {
	t.mu.Lock()
	t.noToolCount = 0
	t.mu.Unlock()
}

func (t *Task) forceToolUsage() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.promptData.AddMessage("user", t.promptData.GetForceToolsMessage())
}

func (t *Task) copyPromptData() entity.PromptData {
	promptCopy := *t.promptData
	promptCopy.Messages = make([]entity.Message, len(t.promptData.Messages))
	copy(promptCopy.Messages, t.promptData.Messages)
	return promptCopy
}

func (t *Task) handleAIError(err error) error {
	fmt.Println(ui.Error("AI error: " + err.Error()))

	if strings.Contains(err.Error(), "400 Bad Request") {
		fmt.Println(ui.Warning("This usually means:"))
		fmt.Println(ui.Dim("   • The model doesn't support function calling"))
		fmt.Println(ui.Dim("   • The request format is incompatible"))
		fmt.Println(ui.Dim("   • Try using a different model or provider"))
	} else if strings.Contains(err.Error(), "401") {
		fmt.Println(ui.Warning("This usually means:"))
		fmt.Println(ui.Dim("   • API key is invalid or expired"))
		fmt.Println(ui.Dim("   • Check your configuration"))
	} else if strings.Contains(err.Error(), "429") {
		fmt.Println(ui.Warning("This usually means:"))
		fmt.Println(ui.Dim("   • Rate limit exceeded"))
		fmt.Println(ui.Dim("   • Wait a moment and try again"))
	}

	fmt.Println(ui.Info("Please try again or rephrase the request"))

	return err
}

func formatFindFilesArgs(args map[string]any) string {
	var parts []string

	if path, ok := args["path"].(string); ok && path != "" {
		if path == "." {
			parts = append(parts, "path: current directory")
		} else {
			parts = append(parts, fmt.Sprintf("path: %s", path))
		}
	}

	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		parts = append(parts, fmt.Sprintf("pattern: %s", pattern))
	}

	if caseInsensitive, ok := args["case_insensitive"]; ok {
		var isInsensitive bool
		switch v := caseInsensitive.(type) {
		case bool:
			isInsensitive = v
		case string:
			isInsensitive = (v == "true" || v == "1")
		}
		if isInsensitive {
			parts = append(parts, "case_insensitive: true")
		}
	}

	return strings.Join(parts, ", ")
}

func hasDecomposedTask() bool {
	state := tools.GetTaskState()
	hasTask, exists := state.GetContext("has_decomposed_task")
	return exists && hasTask == true
}

func getDecomposedTask() (*decomposition.DecompositionResult, error) {
	state := tools.GetTaskState()

	hasTask, exists := state.GetContext("has_decomposed_task")
	if !exists || hasTask != true {
		return nil, fmt.Errorf("no decomposed task found")
	}

	taskData, exists := state.GetContext("decomposed_task")
	if !exists {
		return nil, fmt.Errorf("decomposed task flag set but no task data found")
	}

	result, ok := taskData.(*decomposition.DecompositionResult)
	if !ok {
		return nil, fmt.Errorf("invalid decomposed task data type")
	}

	return result, nil
}

func clearDecomposedTask() {
	state := tools.GetTaskState()
	state.SetContext("has_decomposed_task", false)
	state.SetContext("decomposed_task", nil)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractTaskFromHistory извлекает задачу из истории сообщений
func (t *Task) extractTaskFromHistory() string {
	for _, msg := range t.promptData.Messages {
		if msg.Role == "user" && len(msg.Content) > 10 {
			return msg.Content
		}
	}
	return "Complete the assigned task"
}
