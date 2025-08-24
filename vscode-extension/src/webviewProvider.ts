import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { spawn, ChildProcess } from 'child_process';
import { AutonomyAgent, AutonomyConfig } from './autonomyAgent';
import { ConfigurationManager } from './configManager';

export class AutonomyWebviewProvider implements vscode.WebviewViewProvider {
    public static readonly viewType = 'autonomyWebview';
    private _view?: vscode.WebviewView;
    private autonomyAgent?: AutonomyAgent;
    private configManager: ConfigurationManager;
    private messageHistory: Array<{ type: 'user' | 'agent' | 'system', content: string, timestamp: Date }> = [];
    private autoStartEnabled = false;
    private thinkingMessageId: string | null = null;
    private messagesFilePath: string;
    private isProcessingMessage = false;

    constructor(
        private readonly _extensionUri: vscode.Uri,
        configManager: ConfigurationManager
    ) {
        this.configManager = configManager;
        this.messagesFilePath = path.join(os.homedir(), '.autonomy', 'task_messages.json');
        this.loadMessages();
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

        webviewView.onDidDispose(() => {
            this.stopAutonomyAgent();
        });

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
                    case 'newTask':
                        this.handleNewTask();
                        break;
                    case 'loadHistory':
                        this.loadAndDisplayMessages();
                        break;
                    case 'getAgentStatus':
                        this.updateWebviewState();
                        break;
                    case 'installLocalModels':
                        this.handleInstallLocalModels();
                        break;
                    case 'checkLocalStatus':
                        this.handleCheckLocalStatus();
                        break;
                    case 'getLocalModels':
                        this.handleGetLocalModels();
                        break;
                }
            },
            undefined,
        );

        this.updateWebviewState();

        // Load and display messages after a short delay to ensure webview is ready
        setTimeout(() => {
            this.loadAndDisplayMessages();
        }, 100);

        // Also try again after a longer delay in case the first attempt was too early
        setTimeout(() => {
            if (this.messageHistory.length > 0) {
                this.loadAndDisplayMessages();
            }
        }, 1000);

        // auto-start agent when opening webview
        setTimeout(() => {
            this.attemptAutoStart();
        }, 800);
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
            this.autoStartAgent().catch(error => {
            });
        }
    }

    public onAgentStopped() {
        this.clearMessagesFile();
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


    private async startFreshAgent() {
        // Stop current agent if it exists
        if (this.autonomyAgent) {
            await this.stopAutonomyAgent();
        }

        try {
            const config = this.configManager.getConfiguration();
            if (!config.apiKey) {
                this.sendMessage('system', 'Welcome to Autonomy! Please configure your API key in the Settings tab to get started.');
                return;
            }

            // Create new agent
            const { AutonomyAgent } = require('./autonomyAgent');
            const { AutonomyTaskProvider } = require('./taskProvider');

            const taskProvider = new AutonomyTaskProvider();
            this.autonomyAgent = new AutonomyAgent(config, taskProvider);

            // Configure agent for webview
            this.autonomyAgent!.setOutputCallback((output: string, type: 'stdout' | 'stderr' | 'task_status') => {
                this.sendAgentOutput(output, type);
            });
            this.autonomyAgent!.setWebviewMode(true);

            await this.autonomyAgent!.start();

            this.sendMessage('system', '🤖 Autonomy agent is ready! You can now send your coding tasks.');
            this.updateWebviewState();

        } catch (error) {
            let errorMessage = `❌ Failed to start agent: ${error}`;

            if (error instanceof Error && error.message.includes('Timeout waiting for agent ready signal')) {
                errorMessage = `❌ Failed to start agent: Timeout waiting for agent ready signal. This might be due to:\n• Invalid API key or configuration\n• Network connectivity issues\n• Missing dependencies\n\nPlease check your configuration in the Settings tab and ensure your API key is valid.`;
            }

            this.sendMessage('system', errorMessage);
            throw error;
        }
    }

    private async stopAutonomyAgent() {
        if (this.autonomyAgent) {
            try {
                await this.autonomyAgent.stop();
            } catch (error) {
                // Error stopping agent - continue silently
            }
            this.autonomyAgent = undefined;
            this.updateWebviewState();
        }
    }

    private async attemptAutoStart() {
        try {
            const config = this.configManager.getConfiguration();
            if (!config.apiKey) {
                this.sendMessage('system', '👋 Welcome to Autonomy! Please configure your API key in the Settings tab to get started.');
                this.updateWebviewState();
                return;
            }

            await this.startFreshAgent();
        } catch (error) {
            let errorMessage = '⚠️ Could not start agent automatically. Please check your configuration in the Settings tab.';

            if (error instanceof Error && error.message.includes('Timeout waiting for agent ready signal')) {
                errorMessage = '⚠️ Could not start agent automatically due to timeout. This might be due to:\n• Invalid API key or configuration\n• Network connectivity issues\n• Missing dependencies\n\nPlease check your configuration in the Settings tab and ensure your API key is valid.';
            }

            this.sendMessage('system', errorMessage);
            this.updateWebviewState();
        }
    }

    private async autoStartAgent() {
        // Redirect to new method
        return this.startFreshAgent();
    }

    public async cleanup() {
        await this.stopAutonomyAgent();
    }

    public handleTaskFromCommand(task: string) {
        this.handleSendMessage(task);
    }

    private filterOutput(output: string): string {
        // Remove ANSI escape codes
        let filtered = output.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '');
        filtered = filtered.replace(/\r[^\n]/g, '');
        filtered = filtered.replace(/\r/g, '');

        // Remove replacement character (�) and other problematic Unicode characters
        filtered = filtered.replace(/\uFFFD/g, '');  // Remove � character
        filtered = filtered.replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, ''); // Remove control characters

        // remove escaped characters if they accidentally got in
        filtered = filtered.replace(/\\n/g, '\n');
        filtered = filtered.replace(/\\t/g, '\t');
        filtered = filtered.replace(/\\r/g, '\r');
        filtered = filtered.replace(/\\\"/g, '"');
        filtered = filtered.replace(/\\\'/g, "'");

        // Filter out thinking text and repetitive messages
        const lines = filtered.split('\n');
        const meaningfulLines = lines.filter(line => {
            const trimmed = line.trim().toLowerCase();

            // Filter out any line containing "thinking"
            if (trimmed.includes('thinking')) return false;

            // Keep tool results
            if (line.trim().startsWith('Tool result:') || line.trim().startsWith('📋 Result:')) return true;

            // Keep tool completion messages
            if (line.trim().startsWith('✓')) return true;

            // Keep tool call messages
            if (line.trim().startsWith('🔧 Tool:')) return true;

            // Filter out task iteration messages (but keep other messages)
            if (trimmed.includes('=== task iteration')) return false;

            // Keep messages about AI requesting tools
            if (trimmed.includes('ai requested tools')) return true;

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

    private loadMessages() {
        try {
            if (fs.existsSync(this.messagesFilePath)) {
                const data = fs.readFileSync(this.messagesFilePath, 'utf8');
                const messages = JSON.parse(data);
                this.messageHistory = messages.map((msg: any) => ({
                    ...msg,
                    timestamp: new Date(msg.timestamp)
                }));
            } else {
                this.messageHistory = [];
            }
        } catch (error) {
            this.messageHistory = [];
        }
    }

    private saveMessages() {
        try {
            const autonomyDir = path.dirname(this.messagesFilePath);
            if (!fs.existsSync(autonomyDir)) {
                fs.mkdirSync(autonomyDir, { recursive: true });
            }

            const data = JSON.stringify(this.messageHistory, null, 2);
            fs.writeFileSync(this.messagesFilePath, data, 'utf8');
        } catch (error) {
            // Silently handle errors
        }
    }

    private clearMessagesFile() {
        try {
            if (fs.existsSync(this.messagesFilePath)) {
                fs.unlinkSync(this.messagesFilePath);
            }
        } catch (error) {
            // Silently handle errors
        }
    }

    private loadAndDisplayMessages() {
        if (!this._view) {
            return;
        }

        // Display all loaded messages in the webview
        for (const message of this.messageHistory) {
            this._view.webview.postMessage({
                type: 'addMessage',
                message: {
                    type: message.type,
                    content: message.content,
                    timestamp: message.timestamp.toISOString()
                }
            });
        }
    }

    private async handleSendMessage(message: string) {
        if (this.isProcessingMessage) {
            return;
        }

        this.isProcessingMessage = true;

        try {
            // Handle /clear command
            if (message.trim() === '/clear') {
                this.handleClearHistory();
                return;
            }

            // Add user message to history and send to webview
            this.addToHistory('user', message);
            this.sendMessage('user', message);

            // check agent status and restart if needed
            if (!this.autonomyAgent || !this.autonomyAgent.isRunning()) {
                this.sendMessage('system', '🔄 Starting agent...');
                await this.autoStartAgent();

                // give time for startup
                if (this.autonomyAgent && this.autonomyAgent.isRunning()) {
                    await new Promise(resolve => setTimeout(resolve, 500));
                } else {
                    throw new Error('Failed to start agent. Please check your configuration.');
                }
            }

            // show thinking indicator
            this.showThinkingIndicator();

            // execute task with timeout - BUT NOT THROUGH COMMAND!
            // Command autonomy.runTask can call handleTaskFromCommand -> handleSendMessage recursively
            if (this.autonomyAgent) {
                await this.autonomyAgent.runTask(message);
            }

        } catch (error) {
            this.hideThinkingIndicator();
            const errorMsg = `❌ Error: ${error}`;
            this.addToHistory('system', errorMsg);
            this.sendMessage('system', errorMsg);

            // if an error occurred, check agent status
            if (this.autonomyAgent && !this.autonomyAgent.isRunning()) {
                this.sendMessage('system', '🔄 Agent stopped. Please try sending your message again.');
                this.updateWebviewState();
            }
        } finally {
            this.isProcessingMessage = false;
        }
    }

    private async handleConfigure(config: any) {
        try {
            // Read current global config or create new one
            const currentConfig = this.configManager.readGlobalConfig() || {};

            // Update config with new values
            if (config.provider) {
                currentConfig.provider = config.provider;
            }
            if (config.apiKey) {
                currentConfig.api_key = config.apiKey;
            }
            if (config.model) {
                currentConfig.model = config.model;
            }
            if (config.executablePath) {
                currentConfig.executable_path = config.executablePath;
            }
            if (config.baseURL) {
                currentConfig.base_url = config.baseURL;
            }
            if (config.maxTokens !== undefined && config.maxTokens !== null && config.maxTokens !== '' && !isNaN(parseInt(config.maxTokens, 10))) {
                currentConfig.max_tokens = parseInt(config.maxTokens, 10);
            }
            if (config.temperature !== undefined && config.temperature !== null && config.temperature !== '' && !isNaN(parseFloat(config.temperature))) {
                currentConfig.temperature = parseFloat(config.temperature);
            }

            // Write to global config
            await this.configManager.writeGlobalConfig(currentConfig);

            this.sendMessage('system', 'Configuration saved successfully. Restarting Autonomy with new settings...');

            // Restart autonomy agent with new configuration
            try {
                if (this.autonomyAgent && this.autonomyAgent.isRunning()) {
                    await this.autonomyAgent.stop();
                    this.clearMessagesFile(); // Clear messages when restarting
                }

                // Restart agent with new configuration
                await this.startFreshAgent();

                this.sendMessage('system', 'Autonomy agent restarted successfully with new configuration!');
            } catch (restartError) {
                this.sendMessage('system', `Configuration saved but failed to restart agent: ${restartError}. Please restart manually.`);
            }

            this.updateWebviewState();

            this._view?.webview.postMessage({
                type: 'configSaved'
            });
        } catch (error) {
            this.sendMessage('system', `Failed to save configuration: ${error}`);

            this._view?.webview.postMessage({
                type: 'configSaved'
            });
        }
    }

    private handleGetConfig() {
        try {
            const config = this.configManager.getConfiguration();

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
                    enableReflection: config.enableReflection,
                    maxTokens: config.maxTokens,
                    temperature: config.temperature
                }
            });
        } catch (error) {
            // If no global config exists, send empty config
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
                    enableReflection: true,
                    maxTokens: undefined,
                    temperature: undefined
                }
            });
        }
    }

    private handleClearHistory() {
        this.messageHistory = [];
        this.clearMessagesFile();
        this._view?.webview.postMessage({
            type: 'clearMessages'
        });
    }

    private handleNewTask() {
        this.handleClearHistory();
        this.sendMessage('system', 'Starting new task. Previous conversation cleared.');
        // Update UI state after clearing
        this.updateWebviewState();
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

        // Save messages to file after each addition
        this.saveMessages();
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
            <script src="https://cdn.jsdelivr.net/npm/marked@9.1.6/marked.min.js"></script>
            <title>Autonomy Agent</title>
        </head>
        <body>
            <div class="container">
                <!-- Header -->
                <div class="header">
                    <h2><img src="${webview.asWebviewUri(vscode.Uri.joinPath(this._extensionUri, 'media', 'icon.png'))}" class="header-icon" alt="Autonomy"> Autonomy Agent</h2>
                </div>

                <!-- Tabs -->
                <div class="tabs">
                    <button class="tab-button active" onclick="showTab('chat')">Chat</button>
                    <button class="tab-button" onclick="showTab('config')">Settings</button>
                </div>

                <!-- Chat Tab -->
                <div id="chat-tab" class="tab-content active">
                    <div class="agent-controls">
                        <button id="new-task" class="btn btn-primary">New Task</button>
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

                        <div class="form-group">
                            <label for="max-tokens">Max Tokens:</label>
                            <input type="number" id="max-tokens" placeholder="16384" min="1" max="200000">
                            <small>Maximum number of tokens to generate (leave empty for default: 16384)</small>
                        </div>

                        <div class="form-group">
                            <label for="temperature">Temperature:</label>
                            <input type="number" id="temperature" placeholder="0.0" min="0" max="2" step="0.1">
                            <small>Controls randomness: 0.0 = deterministic, 1.0 = creative (leave empty for provider default)</small>
                        </div>

                        <!-- Local Models Section -->
                        <div class="form-group local-models-section">
                            <label>Local Models Setup:</label>
                            <div class="local-models-info">
                                <p style="font-size: 12px; color: var(--text-secondary); margin: 8px 0;">
                                    Install Ollama and recommended coding models for local AI development
                                </p>
                            </div>
                            <div style="display: flex; gap: 8px;">
                                <button id="install-local-models" class="btn btn-secondary" style="flex: 1;">
                                    🚀 Install Local Models
                                </button>
                                <button id="check-local-status" class="btn btn-tertiary" style="flex: 0 0 auto; padding: 8px 12px;">
                                    🔍 Check Status
                                </button>
                            </div>
                            <div id="installation-progress" class="installation-progress" style="display: none;">
                                <div class="progress-text"></div>
                                <div class="progress-bar">
                                    <div class="progress-fill"></div>
                                </div>
                            </div>
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

    private async handleInstallLocalModels() {
        try {
            this.sendInstallationProgress('Checking Ollama installation...', 10);

            // Check if Ollama is installed
            const isOllamaInstalled = await this.checkOllamaInstalled();
            
            if (!isOllamaInstalled) {
                // Check if we can install automatically or need manual intervention
                const canAutoInstall = await this.canAutoInstallOllama();
                
                if (canAutoInstall) {
                    this.sendInstallationProgress('Installing Ollama...', 20);
                    try {
                        await this.installOllama();
                        this.sendInstallationProgress('Ollama installed successfully', 40);
                    } catch (installError) {
                        // If automatic installation fails, offer manual installation
                        this.sendInstallationProgress('Automatic installation failed', 25);
                        await this.handleOllamaInstallationFailure();
                        return; // Exit early as we need manual intervention
                    }
                } else {
                    // Go directly to manual installation
                    await this.handleOllamaInstallationFailure();
                    return;
                }
            } else {
                this.sendInstallationProgress('Ollama is already installed', 30);
            }

            // Check if Ollama service is running
            this.sendInstallationProgress('Starting Ollama service...', 50);
            await this.ensureOllamaRunning();

            // Install models
            const models = ['gpt-oss:20b', 'gemma3:latest', 'qwen3:latest'];
            const progressPerModel = 40 / models.length; // 40% total for models
            let currentProgress = 60;

            for (let i = 0; i < models.length; i++) {
                const model = models[i];
                this.sendInstallationProgress(`Installing model ${model}... (${i + 1}/${models.length})`, currentProgress);
                
                try {
                    await this.installOllamaModel(model);
                    currentProgress += progressPerModel;
                    this.sendInstallationProgress(`Model ${model} installed successfully`, currentProgress);
                } catch (error) {
                    this.sendInstallationProgress(`Failed to install ${model}: ${error}`, currentProgress);
                    // Continue with other models
                }
            }

            // Final step - update configuration to use local
            this.sendInstallationProgress('Updating configuration...', 95);
            await this.updateConfigToLocal();

            this.sendInstallationComplete(true, 'All local models installed successfully! 🎉');
            
            // Update the models dropdown with newly installed models
            const finalInstalledModels = await this.getInstalledModels();
            if (finalInstalledModels.length > 0) {
                this.sendLocalModels(finalInstalledModels, true, '');
            }
            
        } catch (error) {
            this.sendInstallationComplete(false, `Installation failed: ${error}`);
        }
    }

    private async checkOllamaInstalled(): Promise<boolean> {
        return new Promise((resolve) => {
            const process = spawn('which', ['ollama'], { shell: true });
            
            process.on('close', (code) => {
                resolve(code === 0);
            });
            
            process.on('error', () => {
                resolve(false);
            });
        });
    }

    private async canAutoInstallOllama(): Promise<boolean> {
        const platform = os.platform();
        
        // Windows не поддерживает автоустановку
        if (platform === 'win32') {
            return false;
        }

        // Проверяем наличие необходимых инструментов
        try {
            // Проверяем curl
            await this.runCommand('which curl');
            
            // На macOS проверяем Homebrew
            if (platform === 'darwin') {
                try {
                    await this.runCommand('which brew');
                    return true; // Homebrew доступен
                } catch (error) {
                    // Homebrew нет, но curl есть - можем попробовать скрипт
                    return true;
                }
            }
            
            return true; // Linux с curl
        } catch (error) {
            return false; // Нет curl
        }
    }

    private async installOllama(): Promise<void> {
        const platform = os.platform();
        
        switch (platform) {
            case 'darwin':
                return this.installOllamaMacOS();
            case 'linux':
                return this.installOllamaLinux();
            case 'win32':
                return this.installOllamaWindows();
            default:
                throw new Error(`Unsupported platform: ${platform}`);
        }
    }

    private async installOllamaMacOS(): Promise<void> {
        // Try Homebrew first (most common on macOS)
        try {
            await this.runCommand('brew --version');
            this.sendInstallationProgress('Installing Ollama via Homebrew...', 25);
            await this.runCommand('brew install ollama');
            return;
        } catch (error) {
            // Homebrew not available, try direct download
        }

        // Fallback to direct installation script
        this.sendInstallationProgress('Installing Ollama via installation script...', 25);
        await this.runCommand('curl -fsSL https://ollama.com/install.sh | sh');
    }

    private async installOllamaLinux(): Promise<void> {
        // Try package managers first
        try {
            // Check for apt (Debian/Ubuntu)
            await this.runCommand('which apt-get');
            this.sendInstallationProgress('Installing Ollama via apt...', 25);
            await this.runCommand('sudo apt-get update && sudo apt-get install -y curl');
            await this.runCommand('curl -fsSL https://ollama.com/install.sh | sh');
            return;
        } catch (error) {
            // Try yum/dnf (RedHat/Fedora)
            try {
                await this.runCommand('which yum');
                this.sendInstallationProgress('Installing Ollama via yum...', 25);
                await this.runCommand('sudo yum install -y curl');
                await this.runCommand('curl -fsSL https://ollama.com/install.sh | sh');
                return;
            } catch (error) {
                // Fallback to direct script
                this.sendInstallationProgress('Installing Ollama via installation script...', 25);
                await this.runCommand('curl -fsSL https://ollama.com/install.sh | sh');
            }
        }
    }

    private async installOllamaWindows(): Promise<void> {
        // Show helpful message for Windows users
        vscode.window.showInformationMessage(
            'Automatic Ollama installation is not supported on Windows. Would you like to download it manually?',
            'Download Ollama',
            'Cancel'
        ).then(selection => {
            if (selection === 'Download Ollama') {
                vscode.env.openExternal(vscode.Uri.parse('https://ollama.com/download'));
            }
        });
        throw new Error('Please install Ollama manually from https://ollama.com');
    }

    private async runCommand(command: string): Promise<string> {
        return new Promise((resolve, reject) => {
            const process = spawn(command, { shell: true });
            
            let stdout = '';
            let stderr = '';
            
            process.stdout?.on('data', (data) => {
                stdout += data.toString();
            });
            
            process.stderr?.on('data', (data) => {
                stderr += data.toString();
            });
            
            process.on('close', (code) => {
                if (code === 0) {
                    resolve(stdout);
                } else {
                    reject(new Error(`Command failed with code ${code}. Stderr: ${stderr}`));
                }
            });
            
            process.on('error', (error) => {
                reject(new Error(`Command execution failed: ${error.message}`));
            });
        });
    }

    private async ensureOllamaRunning(): Promise<void> {
        // First check if Ollama is already running
        const isAlreadyRunning = await this.checkOllamaHealth();
        if (isAlreadyRunning) {
            return; // Already running, nothing to do
        }

        return new Promise((resolve, reject) => {
            // Try to start Ollama service
            const process = spawn('ollama', ['serve'], { 
                shell: true,
                detached: true,
                stdio: 'ignore'
            });
            
            process.unref();
            
            // Give it a moment to start
            setTimeout(() => {
                // Check if it's responding
                this.checkOllamaHealth().then(isHealthy => {
                    if (isHealthy) {
                        resolve();
                    } else {
                        reject(new Error('Ollama service failed to start. Please try starting it manually with "ollama serve"'));
                    }
                });
            }, 3000); // Увеличиваем время ожидания до 3 секунд
        });
    }

    private async checkOllamaHealth(): Promise<boolean> {
        return new Promise((resolve) => {
            const process = spawn('curl', ['-s', 'http://localhost:11434/api/tags'], { shell: true });
            
            process.on('close', (code) => {
                resolve(code === 0);
            });
            
            process.on('error', () => {
                resolve(false);
            });
        });
    }

    private async installOllamaModel(modelName: string): Promise<void> {
        return new Promise((resolve, reject) => {
            const process = spawn('ollama', ['pull', modelName], { shell: true });
            
            let output = '';
            
            process.stdout?.on('data', (data) => {
                output += data.toString();
                // Could parse download progress here if needed
            });
            
            process.stderr?.on('data', (data) => {
                output += data.toString();
            });
            
            process.on('close', (code) => {
                if (code === 0) {
                    resolve();
                } else {
                    reject(new Error(`Failed to install model ${modelName}. Exit code: ${code}`));
                }
            });
            
            process.on('error', (error) => {
                reject(new Error(`Error installing model ${modelName}: ${error.message}`));
            });
        });
    }

    private async updateConfigToLocal(): Promise<void> {
        try {
            const currentConfig = this.configManager.readGlobalConfig() || {};
            
            // Update to local configuration
            currentConfig.provider = 'local'; // Use the 'local' provider
            currentConfig.base_url = 'http://localhost:11434/v1';
            currentConfig.api_key = 'local-api-key';
            currentConfig.model = 'qwen3:latest'; // Default to qwen3 as it's good for coding
            
            await this.configManager.writeGlobalConfig(currentConfig);
            
            // Also update VS Code configuration for consistency
            await vscode.workspace.getConfiguration('autonomy').update('provider', 'local', vscode.ConfigurationTarget.Global);
            await vscode.workspace.getConfiguration('autonomy').update('baseURL', 'http://localhost:11434/v1', vscode.ConfigurationTarget.Global);
            await vscode.workspace.getConfiguration('autonomy').update('model', 'qwen3:latest', vscode.ConfigurationTarget.Global);
            
        } catch (error) {
            throw new Error(`Failed to update configuration: ${error}`);
        }
    }

    private sendInstallationProgress(status: string, progress: number) {
        this._view?.webview.postMessage({
            type: 'installationProgress',
            status: status,
            progress: progress
        });
    }

    private sendInstallationComplete(success: boolean, message: string) {
        this._view?.webview.postMessage({
            type: 'installationComplete',
            success: success,
            message: message
        });
    }

    private async handleOllamaInstallationFailure(): Promise<void> {
        const platform = os.platform();
        let instructions = '';
        let downloadUrl = 'https://ollama.com/download';

        switch (platform) {
            case 'darwin':
                instructions = 'Automatic installation failed. Please install Ollama manually:\n\n• Via Homebrew: brew install ollama\n• Or download the app from ollama.com\n\nAfter installation, click "Try Again"';
                break;
            case 'linux':
                instructions = 'Automatic installation failed. Please install Ollama manually:\n\n• Run in terminal: curl -fsSL https://ollama.com/install.sh | sh\n• Or download from ollama.com\n\nAfter installation, click "Try Again"';
                break;
            case 'win32':
                instructions = 'Please download and install Ollama from ollama.com, then click "Try Again"';
                break;
        }

        const selection = await vscode.window.showWarningMessage(
            instructions,
            { modal: true },
            'Open Download Page',
            'Try Again',
            'Skip Installation'
        );

        switch (selection) {
            case 'Open Download Page':
                vscode.env.openExternal(vscode.Uri.parse(downloadUrl));
                this.sendInstallationComplete(false, '🌐 Download page opened. Install Ollama and click "Install Local Models" again');
                break;
            case 'Try Again':
                // Restart the installation process
                setTimeout(() => this.handleInstallLocalModels(), 1000);
                break;
            case 'Skip Installation':
                this.sendInstallationComplete(false, '⏭️ Ollama installation skipped. You can install it manually later');
                break;
            default:
                this.sendInstallationComplete(false, '❌ Installation cancelled');
                break;
        }
    }

    private async handleCheckLocalStatus() {
        try {
            this.sendInstallationProgress('Checking local AI status...', 20);

            // Check Ollama installation
            const isOllamaInstalled = await this.checkOllamaInstalled();
            if (!isOllamaInstalled) {
                this.sendInstallationComplete(false, '❌ Ollama not installed. Click "Install Local Models" to get started');
                return;
            }

            // Check Ollama service
            this.sendInstallationProgress('Checking Ollama service...', 40);
            const isOllamaRunning = await this.checkOllamaHealth();
            if (!isOllamaRunning) {
                this.sendInstallationComplete(false, '⚠️ Ollama installed but not running. Try: ollama serve');
                return;
            }

            // Check installed models
            this.sendInstallationProgress('Checking installed models...', 60);
            const installedModels = await this.getInstalledModels();
            
            this.sendInstallationProgress('Getting model status...', 80);
            const requiredModels = ['gpt-oss:20b', 'gemma3:latest', 'qwen3:latest'];
            const missingModels = requiredModels.filter(model => !installedModels.includes(model));

            let statusMessage = '✅ Ollama is running';
            if (installedModels.length > 0) {
                statusMessage += `\n📦 Installed models: ${installedModels.join(', ')}`;
            }
            if (missingModels.length > 0) {
                statusMessage += `\n🔍 Missing models: ${missingModels.join(', ')}`;
            }

            this.sendInstallationComplete(true, statusMessage);

            // Also update the models dropdown if we're on local provider
            if (installedModels.length > 0) {
                this.sendLocalModels(installedModels, true, '');
            }

        } catch (error) {
            this.sendInstallationComplete(false, `Status check failed: ${error}`);
        }
    }

    private async getInstalledModels(): Promise<string[]> {
        try {
            const output = await this.runCommand('ollama list');
            // Parse output to get model names
            const lines = output.split('\n').slice(1); // Skip header
            const models = lines
                .filter(line => line.trim())
                .map(line => line.split(/\s+/)[0]) // First column is model name
                .filter(name => name && name !== 'NAME'); // Filter out header remnants
            
            return models;
        } catch (error) {
            return [];
        }
    }

    private async handleGetLocalModels() {
        try {
            // Check if Ollama is installed and running
            const isOllamaInstalled = await this.checkOllamaInstalled();
            if (!isOllamaInstalled) {
                this.sendLocalModels([], false, 'Ollama not installed');
                return;
            }

            const isOllamaRunning = await this.checkOllamaHealth();
            if (!isOllamaRunning) {
                this.sendLocalModels([], false, 'Ollama not running - try: ollama serve');
                return;
            }

            // Get installed models
            const installedModels = await this.getInstalledModels();
            this.sendLocalModels(installedModels, true, installedModels.length > 0 ? '' : 'No models installed');

        } catch (error) {
            this.sendLocalModels([], false, `Failed to get models: ${error}`);
        }
    }

    private sendLocalModels(models: string[], success: boolean, message: string) {
        this._view?.webview.postMessage({
            type: 'localModels',
            models: models,
            success: success,
            message: message
        });
    }
}