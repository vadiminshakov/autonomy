package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vadiminshakov/autonomy/ui"
)

// LSPClient manages communication with a language server
type LSPClient struct {
	config      LanguageServerConfig
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	stderr      io.ReadCloser
	requestID   int
	responses   map[int]chan LSPResponse
	responsesMu sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	initialized bool
	rootPath    string
}

// LSPClientManager manages multiple LSP clients
type LSPClientManager struct {
	clients map[string]*LSPClient
	mu      sync.RWMutex
}

var globalLSPManager = &LSPClientManager{
	clients: make(map[string]*LSPClient),
}

// GetLSPClient returns or creates an LSP client for the given file
func GetLSPClient(filePath string, rootPath string) (*LSPClient, error) {
	config, found := GetLanguageServerForFile(filePath)
	if !found {
		return nil, fmt.Errorf("no language server found for file: %s", filePath)
	}

	return globalLSPManager.GetOrCreateClient(*config, rootPath)
}

// GetOrCreateClient returns existing client or creates a new one
func (m *LSPClientManager) GetOrCreateClient(config LanguageServerConfig, rootPath string) (*LSPClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clientKey := fmt.Sprintf("%s:%s", config.Command, rootPath)
	
	if client, exists := m.clients[clientKey]; exists && client.IsAlive() {
		return client, nil
	}

	// create new client
	client, err := NewLSPClient(config, rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create LSP client: %v", err)
	}

	m.clients[clientKey] = client
	return client, nil
}

// NewLSPClient creates a new LSP client
func NewLSPClient(config LanguageServerConfig, rootPath string) (*LSPClient, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	client := &LSPClient{
		config:    config,
		responses: make(map[int]chan LSPResponse),
		ctx:       ctx,
		cancel:    cancel,
		rootPath:  rootPath,
	}

	if err := client.start(); err != nil {
		cancel()
		return nil, err
	}

	return client, nil
}

// start launches the language server process
func (c *LSPClient) start() error {
	// check if command exists
	_, err := exec.LookPath(c.config.Command)
	if err != nil {
		return fmt.Errorf("language server '%s' not found in PATH: %v", c.config.Command, err)
	}

	fmt.Printf("%s Starting LSP server: %s %v\n", 
		ui.Info("LSP"), c.config.Command, c.config.Args)

	// start the language server process
	c.cmd = exec.CommandContext(c.ctx, c.config.Command, c.config.Args...)
	c.cmd.Dir = c.rootPath

	// set up pipes
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	c.stdin = stdin

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}
	c.stdout = stdout

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}
	c.stderr = stderr

	// start the process
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start language server: %v", err)
	}

	// start response handler
	go c.handleResponses()
	go c.handleErrors()

	// initialize the language server
	if err := c.initialize(); err != nil {
		c.Close()
		return fmt.Errorf("failed to initialize language server: %v", err)
	}

	c.initialized = true
	fmt.Printf("%s LSP server initialized successfully\n", ui.Success("LSP"))
	return nil
}

// initialize sends the initialize request to the language server
func (c *LSPClient) initialize() error {
	initParams := InitializeParams{
		ProcessID: os.Getpid(),
		RootPath:  c.rootPath,
		RootURI:   "file://" + c.rootPath,
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Synchronization: TextDocumentSyncClientCapabilities{
					DynamicRegistration: true,
					WillSave:            true,
					WillSaveWaitUntil:   true,
					DidSave:             true,
				},
			},
			Workspace: WorkspaceClientCapabilities{
				ApplyEdit: true,
				WorkspaceEdit: WorkspaceEditClientCapabilities{
					DocumentChanges: true,
				},
				DidChangeConfiguration: DidChangeConfigurationClientCapabilities{
					DynamicRegistration: true,
				},
			},
		},
	}

	response, err := c.sendRequest("initialize", initParams)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return fmt.Errorf("initialize failed: %s", response.Error.Message)
	}

	// send initialized notification
	return c.sendNotification("initialized", map[string]interface{}{})
}

// sendRequest sends a request and waits for response
func (c *LSPClient) sendRequest(method string, params interface{}) (*LSPResponse, error) {
	c.requestID++
	id := c.requestID

	request := LSPRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// create response channel
	respChan := make(chan LSPResponse, 1)
	c.responsesMu.Lock()
	c.responses[id] = respChan
	c.responsesMu.Unlock()

	// send request
	if err := c.writeMessage(request); err != nil {
		c.responsesMu.Lock()
		delete(c.responses, id)
		c.responsesMu.Unlock()
		return nil, err
	}

	// wait for response with timeout
	select {
	case response := <-respChan:
		return &response, nil
	case <-time.After(30 * time.Second):
		c.responsesMu.Lock()
		delete(c.responses, id)
		c.responsesMu.Unlock()
		return nil, fmt.Errorf("request timeout")
	case <-c.ctx.Done():
		return nil, fmt.Errorf("client closed")
	}
}

// sendNotification sends a notification (no response expected)
func (c *LSPClient) sendNotification(method string, params interface{}) error {
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}

	return c.writeMessage(notification)
}

// writeMessage writes a JSON-RPC message with LSP headers
func (c *LSPClient) writeMessage(message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return err
	}
	
	if _, err := c.stdin.Write(data); err != nil {
		return err
	}

	return nil
}

// handleResponses processes incoming responses from the language server
func (c *LSPClient) handleResponses() {
	scanner := bufio.NewScanner(c.stdout)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if strings.HasPrefix(line, "Content-Length:") {
			lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			length, err := strconv.Atoi(lengthStr)
			if err != nil {
				continue
			}

			// skip empty line
			scanner.Scan()

			// read message content
			content := make([]byte, length)
			n, err := io.ReadFull(c.stdout, content)
			if err != nil || n != length {
				continue
			}

			var response LSPResponse
			if err := json.Unmarshal(content, &response); err != nil {
				continue
			}

			// route response to waiting request
			if response.ID != 0 {
				c.responsesMu.RLock()
				if respChan, exists := c.responses[response.ID]; exists {
					select {
					case respChan <- response:
					default:
					}
					delete(c.responses, response.ID)
				}
				c.responsesMu.RUnlock()
			}
		}
	}
}

// handleErrors processes stderr from the language server
func (c *LSPClient) handleErrors() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		fmt.Printf("%s LSP stderr: %s\n", ui.Warning("LSP"), scanner.Text())
	}
}

// IsAlive checks if the client is still running
func (c *LSPClient) IsAlive() bool {
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	
	// check if process is still running
	return c.cmd.ProcessState == nil
}

// OpenDocument notifies the language server that a document is opened
func (c *LSPClient) OpenDocument(filePath string, content string) error {
	if !c.initialized {
		return fmt.Errorf("client not initialized")
	}

	languageID := GetLanguageIDForFile(filePath)
	uri := "file://" + filepath.ToSlash(filePath)

	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       content,
		},
	}

	return c.sendNotification("textDocument/didOpen", params)
}

// ApplyEdit applies a workspace edit using the language server
func (c *LSPClient) ApplyEdit(edit WorkspaceEdit) error {
	if !c.initialized {
		return fmt.Errorf("client not initialized")
	}

	params := ApplyWorkspaceEditParams{
		Edit: edit,
	}

	response, err := c.sendRequest("workspace/applyEdit", params)
	if err != nil {
		return err
	}

	if response.Error != nil {
		return fmt.Errorf("apply edit failed: %s", response.Error.Message)
	}

	// parse response
	var result ApplyWorkspaceEditResponse
	if err := json.Unmarshal(response.Result.([]byte), &result); err != nil {
		return fmt.Errorf("failed to parse apply edit response: %v", err)
	}

	if !result.Applied {
		return fmt.Errorf("edit was not applied: %s", result.Reason)
	}

	return nil
}

// Close shuts down the LSP client
func (c *LSPClient) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	
	if c.stdin != nil {
		c.stdin.Close()
	}
	
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	
	return nil
}

// CloseAll closes all LSP clients
func (m *LSPClientManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for _, client := range m.clients {
		client.Close()
	}
	
	m.clients = make(map[string]*LSPClient)
}