import * as vscode from 'vscode';
import { AutonomyAgent, AutonomyConfig } from './autonomyAgent';
import { ConfigurationManager } from './configManager';

export class AutonomyWebviewProvider implements vscode.WebviewViewProvider {
    public static readonly viewType = 'autonomyWebview';
    private _view?: vscode.WebviewView;
    private autonomyAgent?: AutonomyAgent;
    private configManager: ConfigurationManager;
    private messageHistory: Array<{type: 'user' | 'agent' | 'system', content: string, timestamp: Date}> = [];
    private autoStartEnabled = false;
    private thinkingMessageId: string | null = null;

    constructor(
        private readonly _extensionUri: vscode.Uri,
        configManager: ConfigurationManager
    ) {
        this.configManager = configManager;
    }

    public resolveWebviewView(
        webviewView: vscode.WebviewView,
        context: vscode.WebviewViewResolveContext,
        _token: vscode.CancellationToken,
    ) {
        this._view = webviewView;

        webviewView.webview.options = {
            enableScripts: true,
            localResourceRoots: [
                this._extensionUri
            ]
        };

        webviewView.webview.html = this._getHtmlForWebview(webviewView.webview);

        webviewView.webview.onDidReceiveMessage(
            message => {
                switch (message.type) {
                    case 'sendMessage':
                        this.handleSendMessage(message.value);
                        break;
                    case 'configure':
                        this.handleConfigure(message.config);
                        break;
                    case 'getConfig':
                        this.handleGetConfig();
                        break;
                    case 'clearHistory':
                        this.handleClearHistory();
                        break;
                }
            },
            undefined,
        );

        this.updateWebviewState();
        
        if (this.autoStartEnabled) {
            this.autoStartAgent();
        }
    }

    public setAutonomyAgent(agent: AutonomyAgent | undefined) {
        this.autonomyAgent = agent;
        this.updateWebviewState();
    }

    public forceUpdateWebviewState() {
        this.updateWebviewState();
    }

    public enableAutoStart() {
        this.autoStartEnabled = true;
        if (this._view) {
            this.autoStartAgent();
        }
    }

    public sendAgentOutput(output: string, type: 'stdout' | 'stderr' | 'task_status') {
        // Handle special task status messages
        if (type === 'task_status') {
            if (output === 'TASK_COMPLETED' || output === 'TASK_FAILED') {
                this.hideThinkingIndicator();
            }
            return;
        }
        
        // Send output immediately without buffering
        if (output.trim()) {
            const filteredOutput = this.filterOutput(output);
            
            if (filteredOutput.trim()) {
                const messageType = type === 'stderr' ? 'system' : 'agent';
                
                // Hide thinking indicator before showing new message
                this.hideThinkingIndicator();
                
                this.addToHistory(messageType, filteredOutput);
                this.sendMessage(messageType, filteredOutput);
                
                // Show thinking indicator after agent messages, but only if task is still running
                if (messageType === 'agent' && !this.isTaskCompletionMessage(filteredOutput)) {
                    this.showThinkingIndicator();
                }
            }
        }
    }

    private isTaskCompletionMessage(output: string): boolean {
        const completionPatterns = [
            /Task\s+completed\s+successfully/i,
            /✅\s*Task\s+completed/i,
            /✅\s*All\s+done/i,
            /^✅/m,
            /Done\s+attempt_completion/i,
            /🎉\s*Task\s+completed/i,
            /Task\s+failed/i,
            /❌\s*Task/i,
            /Error:\s+Task/i
        ];
        
        return completionPatterns.some(pattern => pattern.test(output));
    }


    private async autoStartAgent() {
        if (this.autonomyAgent && this.autonomyAgent.isRunning()) {
            return;
        }

        try {
            const config = this.configManager.getConfiguration();
            if (!config.apiKey) {
                this.sendMessage('system', 'Please configure your API key in the Settings tab to start using Autonomy.');
                return;
            }

            console.log('webviewProvider: Auto-starting agent...');
            this.sendMessage('system', 'Checking Autonomy installation...');
            
            // First try to run install check command to ensure Autonomy is installed
            try {
                await vscode.commands.executeCommand('autonomy.installCli');
                // Wait a bit for installation to complete
                await new Promise(resolve => setTimeout(resolve, 2000));
            } catch (installError) {
                console.log('webviewProvider: Install check failed, continuing with agent start...');
            }
            
            this.sendMessage('system', 'Starting Autonomy agent...');
            await vscode.commands.executeCommand('autonomy.start', true);
            
            // Wait for agent to be fully running
            let attempts = 0;
            while (attempts < 10 && (!this.autonomyAgent || !this.autonomyAgent.isRunning())) {
                console.log(`webviewProvider: Waiting for agent, attempt ${attempts + 1}, agent exists: ${!!this.autonomyAgent}, running: ${this.autonomyAgent?.isRunning()}`);
                await new Promise(resolve => setTimeout(resolve, 500));
                attempts++;
            }
            
            console.log(`webviewProvider: After waiting, agent exists: ${!!this.autonomyAgent}, running: ${this.autonomyAgent?.isRunning()}`);
            
            if (this.autonomyAgent && this.autonomyAgent.isRunning()) {
                this.sendMessage('system', 'Autonomy agent is ready! You can now send tasks.');
                this.updateWebviewState();
            } else {
                // Force update anyway in case the agent is actually running
                this.updateWebviewState();
                this.sendMessage('system', 'Agent may be ready. If input is still disabled, please check the console or restart.');
            }
            
        } catch (error) {
            console.error('webviewProvider: Error auto-starting agent:', error);
            this.sendMessage('system', `Failed to start agent: ${error}. Please run "Autonomy: Install Autonomy CLI" command or check your configuration in the Settings tab.`);
        }
    }

    private filterOutput(output: string): string {
        // Remove ANSI escape codes
        let filtered = output.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '');
        filtered = filtered.replace(/\r[^\n]/g, '');
        filtered = filtered.replace(/\r/g, '');
        
        // Remove replacement character (�) and other problematic Unicode characters
        filtered = filtered.replace(/\uFFFD/g, '');  // Remove � character
        filtered = filtered.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, ''); // Remove control characters
        
        // Filter out thinking text and repetitive messages
        const lines = filtered.split('\n');
        const meaningfulLines = lines.filter(line => {
            const trimmed = line.trim().toLowerCase();
            
            // Filter out any line containing "thinking"
            if (trimmed.includes('thinking')) return false;
            
            // Filter out task iteration messages
            if (trimmed.includes('=== task iteration')) return false;
            if (trimmed.includes('task iteration')) return false;
            
            // Filter out empty lines
            if (!trimmed) return false;
            
            return true;
        });
        
        const result = meaningfulLines.join('\n').trim();
        return result;
    }

    private showThinkingIndicator() {
        if (this.thinkingMessageId) {
            this.hideThinkingIndicator();
        }
        
        this.thinkingMessageId = 'thinking-' + Date.now();
        this._view?.webview.postMessage({
            type: 'addThinking',
            messageId: this.thinkingMessageId
        });
    }

    private hideThinkingIndicator() {
        if (this.thinkingMessageId) {
            this._view?.webview.postMessage({
                type: 'removeThinking',
                messageId: this.thinkingMessageId
            });
            this.thinkingMessageId = null;
        }
    }

    private async handleSendMessage(message: string) {
        this.addToHistory('user', message);
        this.sendMessage('user', message);

        try {
            if (!this.autonomyAgent || !this.autonomyAgent.isRunning()) {
                await this.autoStartAgent();
                await new Promise(resolve => setTimeout(resolve, 1000));
            }
            
            await vscode.commands.executeCommand('autonomy.runTask', message);
        } catch (error) {
            const errorMsg = `Error executing task: ${error}`;
            this.addToHistory('system', errorMsg);
            this.sendMessage('system', errorMsg);
        }
    }

    private async handleConfigure(config: any) {
        console.log('webviewProvider: handleConfigure called with config:', config);
        
        try {
            // Read current global config or create new one
            const currentConfig = this.configManager.readGlobalConfig() || {};
            console.log('webviewProvider: Current global config:', currentConfig);
            
            // Update config with new values
            if (config.provider) {
                console.log('webviewProvider: Updating provider to:', config.provider);
                currentConfig.provider = config.provider;
            }
            if (config.apiKey) {
                console.log('webviewProvider: Updating API key (masked)');
                currentConfig.api_key = config.apiKey;
            }
            if (config.model) {
                console.log('webviewProvider: Updating model to:', config.model);
                currentConfig.model = config.model;
            }
            if (config.executablePath) {
                console.log('webviewProvider: Updating executable path to:', config.executablePath);
                currentConfig.executable_path = config.executablePath;
            }
            if (config.baseURL) {
                console.log('webviewProvider: Updating base URL to:', config.baseURL);
                currentConfig.base_url = config.baseURL;
            }

            console.log('webviewProvider: Final config to write:', currentConfig);

            // Write to global config
            await this.configManager.writeGlobalConfig(currentConfig);
            console.log('webviewProvider: Global config written successfully');

            this.sendMessage('system', 'Configuration saved successfully to ~/.autonomy/config.json');
            this.updateWebviewState();
            
            this._view?.webview.postMessage({
                type: 'configSaved'
            });
        } catch (error) {
            console.error('webviewProvider: Error in handleConfigure:', error);
            this.sendMessage('system', `Failed to save configuration: ${error}`);
            
            this._view?.webview.postMessage({
                type: 'configSaved'
            });
        }
    }

    private handleGetConfig() {
        try {
            const config = this.configManager.getConfiguration();
            console.log('webviewProvider: Loaded config from global file:', {
                provider: config.provider,
                model: config.model,
                hasApiKey: !!config.apiKey,
                baseURL: config.baseURL,
                executablePath: config.executablePath
            });
            
            this._view?.webview.postMessage({
                type: 'configData',
                config: {
                    provider: config.provider,
                    model: config.model,
                    executablePath: config.executablePath,
                    baseURL: config.baseURL,
                    apiKey: config.apiKey, // Send actual API key so UI can show it
                    hasApiKey: !!config.apiKey,
                    maxIterations: config.maxIterations,
                    enableReflection: config.enableReflection
                }
            });
            
            console.log('webviewProvider: Sent config to webview with baseURL:', config.baseURL);
        } catch (error) {
            // If no global config exists, send empty config
            console.log('webviewProvider: No global config found, sending default config. Error:', error);
            this._view?.webview.postMessage({
                type: 'configData',
                config: {
                    provider: 'openai',
                    model: 'o3',
                    executablePath: 'autonomy',
                    baseURL: 'https://api.openai.com/v1',
                    apiKey: '',
                    hasApiKey: false,
                    maxIterations: 100,
                    enableReflection: true
                }
            });
        }
    }

    private handleClearHistory() {
        this.messageHistory = [];
        this._view?.webview.postMessage({
            type: 'clearMessages'
        });
    }

    private addToHistory(type: 'user' | 'agent' | 'system', content: string) {
        this.messageHistory.push({
            type,
            content,
            timestamp: new Date()
        });

        if (this.messageHistory.length > 100) {
            this.messageHistory = this.messageHistory.slice(-100);
        }
    }

    private sendMessage(type: 'user' | 'agent' | 'system', content: string) {
        this._view?.webview.postMessage({
            type: 'addMessage',
            message: {
                type,
                content,
                timestamp: new Date().toISOString()
            }
        });
    }

    private updateWebviewState() {
        const isRunning = this.autonomyAgent?.isRunning() || false;
        console.log('webviewProvider: Updating webview state, agent running:', isRunning);
        
        this._view?.webview.postMessage({
            type: 'updateState',
            state: {
                agentRunning: isRunning
            }
        });
    }

    private _getHtmlForWebview(webview: vscode.Webview) {
        const scriptUri = webview.asWebviewUri(vscode.Uri.joinPath(this._extensionUri, 'media', 'main.js'));
        const styleUri = webview.asWebviewUri(vscode.Uri.joinPath(this._extensionUri, 'media', 'main.css'));

        return `<!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <link href="${styleUri}" rel="stylesheet">
            <title>Autonomy Agent</title>
        </head>
        <body>
            <div class="container">
                <!-- Header -->
                <div class="header">
                    <h2><img src="${webview.asWebviewUri(vscode.Uri.joinPath(this._extensionUri, 'media', 'icon.png'))}" class="header-icon" alt="Autonomy"> Autonomy Agent</h2>
                    <div class="status">
                        <span id="status-indicator" class="status-dot offline"></span>
                        <span id="status-text">Offline</span>
                    </div>
                </div>

                <!-- Tabs -->
                <div class="tabs">
                    <button class="tab-button active" onclick="showTab('chat')">Chat</button>
                    <button class="tab-button" onclick="showTab('config')">Settings</button>
                </div>

                <!-- Chat Tab -->
                <div id="chat-tab" class="tab-content active">
                    <div class="agent-controls">
                        <button id="clear-history" class="btn btn-tertiary">Clear History</button>
                    </div>

                    <div id="messages" class="messages"></div>

                    <div class="input-area">
                        <textarea id="message-input" placeholder="Type your task here... (e.g., 'Add error handling to the getUserData function')" rows="3"></textarea>
                        <button id="send-button" class="btn btn-primary">Send</button>
                    </div>
                </div>

                <!-- Config Tab -->
                <div id="config-tab" class="tab-content">
                    <div class="config-form">
                        <h3>Configuration</h3>
                        
                        <div class="form-group">
                            <label for="provider">AI Provider:</label>
                            <select id="provider">
                                <option value="openai">OpenAI</option>
                                <option value="anthropic">Anthropic</option>
                                <option value="openrouter">OpenRouter</option>
                                <option value="local">Local</option>
                            </select>
                        </div>

                        <div class="form-group">
                            <label for="api-key">API Key:</label>
                            <input type="password" id="api-key" placeholder="Enter your API key">
                            <small>Your API key is stored securely in VS Code settings</small>
                        </div>

                        <div class="form-group">
                            <label for="model">Model:</label>
                            <div class="model-input-group">
                                <select id="model-select" style="display: none;">
                                    <option value="">Select a model...</option>
                                </select>
                                <input type="text" id="model" placeholder="e.g., o3, claude-3-5-sonnet-20241022">
                                <button type="button" id="toggle-model-input" class="btn btn-small">✏️</button>
                            </div>
                        </div>

                        <div class="form-group">
                            <label for="executable-path">Executable Path:</label>
                            <input type="text" id="executable-path" placeholder="autonomy">
                            <small>Path to the autonomy executable</small>
                        </div>

                        <div class="form-group">
                            <label for="base-url">Base URL (Optional):</label>
                            <input type="text" id="base-url" placeholder="https://api.openai.com/v1">
                        </div>

                        <div class="form-actions">
                            <button id="save-config" class="btn btn-primary">Save Configuration</button>
                            <button id="load-config" class="btn btn-secondary">Load Current</button>
                        </div>
                    </div>
                </div>
            </div>

            <script src="${scriptUri}"></script>
        </body>
        </html>`;
    }
}