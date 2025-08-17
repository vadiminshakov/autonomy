import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
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

        // Добавляем обработчик закрытия webview
        webviewView.onDidDispose(() => {
            console.log('webviewProvider: Webview disposed, stopping autonomy agent');
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

        // автозапуск агента при открытии webview
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
                console.log('webviewProvider: Auto-start failed:', error);
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
        // Останавливаем текущий агент, если он есть
        if (this.autonomyAgent) {
            await this.stopAutonomyAgent();
        }

        try {
            const config = this.configManager.getConfiguration();
            if (!config.apiKey) {
                this.sendMessage('system', 'Welcome to Autonomy! Please configure your API key in the Settings tab to get started.');
                return;
            }

            // Создаем новый агент
            const { AutonomyAgent } = require('./autonomyAgent');
            const { AutonomyTaskProvider } = require('./taskProvider');

            const taskProvider = new AutonomyTaskProvider();
            this.autonomyAgent = new AutonomyAgent(config, taskProvider);

            // Настраиваем агент для webview
            this.autonomyAgent!.setOutputCallback((output: string, type: 'stdout' | 'stderr' | 'task_status') => {
                this.sendAgentOutput(output, type);
            });
            this.autonomyAgent!.setWebviewMode(true);

            await this.autonomyAgent!.start();

            this.sendMessage('system', '🤖 Autonomy agent is ready! You can now send your coding tasks.');
            this.updateWebviewState();

        } catch (error) {
            console.error('webviewProvider: Failed to start agent:', error);
            this.sendMessage('system', `❌ Failed to start agent: ${error}. Please check your configuration.`);
            throw error;
        }
    }

    private async stopAutonomyAgent() {
        if (this.autonomyAgent) {
            try {
                await this.autonomyAgent.stop();
            } catch (error) {
                console.error('webviewProvider: Error stopping agent:', error);
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
            console.log('webviewProvider: Auto-start failed:', error);
            this.sendMessage('system', '⚠️ Could not start agent automatically. Please check your configuration in the Settings tab.');
            this.updateWebviewState();
        }
    }

    private async autoStartAgent() {
        // Переадресуем на новый метод
        return this.startFreshAgent();
    }

    public async cleanup() {
        console.log('webviewProvider: Cleaning up resources');
        await this.stopAutonomyAgent();
    }

    public handleTaskFromCommand(task: string) {
        console.log('webviewProvider: Received task from command:', task);
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
        // Защита от множественных вызовов
        if (this.isProcessingMessage) {
            console.log('webviewProvider: Already processing message, ignoring duplicate');
            return;
        }

        this.isProcessingMessage = true;

        try {
            // Handle /clear command
            if (message.trim() === '/clear') {
                this.handleClearHistory();
                return;
            }

            // Добавляем пользовательское сообщение в историю и отправляем в webview
            this.addToHistory('user', message);
            this.sendMessage('user', message);

            // проверяем состояние агента и перезапускаем если нужно
            if (!this.autonomyAgent || !this.autonomyAgent.isRunning()) {
                this.sendMessage('system', '🔄 Starting agent...');
                await this.autoStartAgent();

                // даем время для запуска
                if (this.autonomyAgent && this.autonomyAgent.isRunning()) {
                    await new Promise(resolve => setTimeout(resolve, 500));
                } else {
                    throw new Error('Failed to start agent. Please check your configuration.');
                }
            }

            // показываем thinking индикатор
            this.showThinkingIndicator();

            // выполняем задачу с таймаутом - НО НЕ ЧЕРЕЗ КОМАНДУ!
            // Команда autonomy.runTask может вызывать handleTaskFromCommand -> handleSendMessage рекурсивно
            if (this.autonomyAgent) {
                await this.autonomyAgent.runTask(message);
            }

        } catch (error) {
            this.hideThinkingIndicator();
            const errorMsg = `❌ Error: ${error}`;
            this.addToHistory('system', errorMsg);
            this.sendMessage('system', errorMsg);

            // если произошла ошибка, проверяем состояние агента
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

            // Write to global config
            await this.configManager.writeGlobalConfig(currentConfig);

            this.sendMessage('system', 'Configuration saved successfully. Restarting Autonomy with new settings...');

            // Restart autonomy agent with new configuration
            try {
                if (this.autonomyAgent && this.autonomyAgent.isRunning()) {
                    await this.autonomyAgent.stop();
                    this.clearMessagesFile(); // Clear messages when restarting
                }

                // Перезапускаем агент с новой конфигурацией
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
                    enableReflection: config.enableReflection
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
                    enableReflection: true
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
        // Обновляем состояние UI после очистки
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
        console.log(`webviewProvider: Updating webview state - agentRunning: ${isRunning}, hasAgent: ${!!this.autonomyAgent}`);

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