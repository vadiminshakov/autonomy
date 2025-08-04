package tools

// LSP protocol types for text editing operations

// Position represents a position in a text document
type Position struct {
	Line      int `json:"line"`      // 0-based line number
	Character int `json:"character"` // 0-based character offset
}

// Range represents a text range in a document
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextEdit represents a text edit operation
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// WorkspaceEdit represents multiple text edits across files
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"` // file URI -> edits
}

// LanguageServerConfig defines configuration for a language server
type LanguageServerConfig struct {
	Command    string   `json:"command"`    // executable name
	Args       []string `json:"args"`       // command arguments
	Extensions []string `json:"extensions"` // file extensions this server handles
}

// LSPRequest represents a JSON-RPC request to language server
type LSPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// LSPResponse represents a JSON-RPC response from language server
type LSPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *LSPError   `json:"error,omitempty"`
}

// LSPError represents an error in LSP response
type LSPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeParams for LSP initialize request
type InitializeParams struct {
	ProcessID    int                `json:"processId"`
	RootPath     string             `json:"rootPath"`
	RootURI      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
}

// ClientCapabilities defines what the client supports
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument"`
	Workspace    WorkspaceClientCapabilities    `json:"workspace"`
}

// TextDocumentClientCapabilities defines text document capabilities
type TextDocumentClientCapabilities struct {
	Synchronization TextDocumentSyncClientCapabilities `json:"synchronization"`
}

// TextDocumentSyncClientCapabilities defines sync capabilities
type TextDocumentSyncClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
	WillSave            bool `json:"willSave"`
	WillSaveWaitUntil   bool `json:"willSaveWaitUntil"`
	DidSave             bool `json:"didSave"`
}

// WorkspaceClientCapabilities defines workspace capabilities
type WorkspaceClientCapabilities struct {
	ApplyEdit              bool                        `json:"applyEdit"`
	WorkspaceEdit          WorkspaceEditClientCapabilities `json:"workspaceEdit"`
	DidChangeConfiguration DidChangeConfigurationClientCapabilities `json:"didChangeConfiguration"`
}

// WorkspaceEditClientCapabilities defines workspace edit capabilities
type WorkspaceEditClientCapabilities struct {
	DocumentChanges bool `json:"documentChanges"`
}

// DidChangeConfigurationClientCapabilities defines configuration change capabilities
type DidChangeConfigurationClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration"`
}

// DidOpenTextDocumentParams for textDocument/didOpen notification
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// TextDocumentItem represents a text document
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// ApplyWorkspaceEditParams for workspace/applyEdit request
type ApplyWorkspaceEditParams struct {
	Label string        `json:"label,omitempty"`
	Edit  WorkspaceEdit `json:"edit"`
}

// ApplyWorkspaceEditResponse for workspace/applyEdit response
type ApplyWorkspaceEditResponse struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"failureReason,omitempty"`
}