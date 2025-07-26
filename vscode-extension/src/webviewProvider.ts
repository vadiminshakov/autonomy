import * as vscode from 'vscode';
import { AutonomyAgent, AutonomyConfig } from './autonomyAgent';
import { ConfigurationManager } from './configManager';

export class AutonomyWebviewProvider implements vscode.WebviewViewProvider {
    public static readonly viewType = 'autonomyWebview';
    private _view?: vscode.WebviewView;
    private autonomyAgent?: AutonomyAgent;
    private configManager: ConfigurationManager;
    private messageHistory: Array<{type: 'user' | 'agent' | 'system', content: string, timestamp: Date}> = [];
    private outputBuffer = '';
    private bufferTimer?: NodeJS.Timeout;

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
        
        this.autoStartAgent();
    }

    public setAutonomyAgent(agent: AutonomyAgent | undefined) {
        this.autonomyAgent = agent;
        this.updateWebviewState();
    }

    public forceUpdateWebviewState() {
        this.updateWebviewState();
    }

    public sendAgentOutput(output: string, type: 'stdout' | 'stderr') {
        this.outputBuffer += output;
        
        if (this.bufferTimer) {
            clearTimeout(this.bufferTimer);
        }
        
        this.bufferTimer = setTimeout(() => {
            this.flushOutputBuffer(type);
        }, 500);
    }

    private flushOutputBuffer(type: 'stdout' | 'stderr') {
        if (!this.outputBuffer.trim()) {
            this.outputBuffer = '';
            return;
        }
        
        const filteredOutput = this.filterOutput(this.outputBuffer);
        
        if (filteredOutput) {
            const messageType = type === 'stderr' ? 'system' : 'agent';
            this.addToHistory(messageType, filteredOutput);
            this.sendMessage(messageType, filteredOutput);
        }
        
        this.outputBuffer = '';
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
            this.sendMessage('system', 'Starting Autonomy agent...');
            
            await vscode.commands.executeCommand('autonomy.start', true);
            
            this.sendMessage('system', 'Autonomy agent is ready! You can now send tasks.');
            
        } catch (error) {
            console.error('webviewProvider: Error auto-starting agent:', error);
            this.sendMessage('system', `Failed to start agent: ${error}. Please check your configuration in the Settings tab.`);
        }
    }

    private filterOutput(output: string): string {
            let filtered = output.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, '');
        
        filtered = filtered.replace(/\r[^\n]/g, '');
        filtered = filtered.replace(/\r/g, '');
        
        filtered = filtered.replace(/[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]/g, '');
        filtered = filtered.replace(/[▁▂▃▄▅▆▇█]/g, '');
        filtered = filtered.replace(/[░▒▓]/g, '');
        filtered = filtered.replace(/[◐◓◑◒]/g, '');
        filtered = filtered.replace(/[◴◷◶◵]/g, '');
        filtered = filtered.replace(/[▏▎▍▌▋▊▉]/g, '');
        filtered = filtered.replace(/[┤┐└┴┬├─│]/g, '');
        filtered = filtered.replace(/[⋮⋯⋰⋱]/g, '');
        filtered = filtered.replace(/[◯●○◉]/g, '');
        
        filtered = filtered.replace(/\s*[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]\s*/g, '');
        filtered = filtered.replace(/\|\s*\/\s*-\s*\\\s*/g, '');
        
        filtered = filtered.replace(/\.{3,}/g, '');
        filtered = filtered.replace(/\s*\.\s*\.\s*\.\s*/g, '');
        
        filtered = filtered.replace(/\n{3,}/g, '\n\n');
        filtered = filtered.replace(/[ \t]{3,}/g, '  ');
        
        const lines = filtered.split('\n');
        const meaningfulLines = lines.filter(line => {
            const trimmed = line.trim();
            if (!trimmed) return false;
            
            if (/^thinking[.\s]*$/i.test(trimmed)) return false;
            if (/^(thinking|processing|working)[.\s]*$/i.test(trimmed)) return false;
            if (/thinking\s+thinking/i.test(trimmed)) return false;
            if (/^(\s*thinking\s*){2,}/i.test(trimmed)) return false;
            if (trimmed.split(/\s+/).filter(word => word.toLowerCase() === 'thinking').length > 1) return false;
            
            if (/^[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏▁▂▃▄▅▆▇█░▒▓◐◓◑◒◴◷◶◵▏▎▍▌▋▊▉┤┐└┴┬├─│⋮⋯⋰⋱◯●○◉\s\.\-\|\/\\]+$/.test(trimmed)) return false;
            
            if (trimmed.length < 3 && /^[\.\-\s\|\/\\]*$/.test(trimmed)) return false;
            
            if (/^(.)\1{5,}$/.test(trimmed)) return false;
            
            const words = trimmed.split(/\s+/);
            const thinkingCount = words.filter(word => word.toLowerCase() === 'thinking').length;
            if (thinkingCount > words.length / 2) return false;
            
            return true;
        });
        
        const result = meaningfulLines.join('\n').trim();
        
        return result.length > 5 ? result : '';
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
        try {
            const vscodeConfig = vscode.workspace.getConfiguration('autonomy');
            
            if (config.provider) {
                await vscodeConfig.update('provider', config.provider, vscode.ConfigurationTarget.Workspace);
            }
            if (config.apiKey) {
                await vscodeConfig.update('apiKey', config.apiKey, vscode.ConfigurationTarget.Workspace);
            }
            if (config.model) {
                await vscodeConfig.update('model', config.model, vscode.ConfigurationTarget.Workspace);
            }
            if (config.executablePath) {
                await vscodeConfig.update('executablePath', config.executablePath, vscode.ConfigurationTarget.Workspace);
            }
            if (config.baseURL) {
                await vscodeConfig.update('baseURL', config.baseURL, vscode.ConfigurationTarget.Workspace);
            }

            this.sendMessage('system', 'Configuration saved successfully');
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
        const config = this.configManager.getConfiguration();
        console.log('webviewProvider: Sending config to webview:', {
            provider: config.provider,
            model: config.model,
            hasApiKey: !!config.apiKey,
            baseURL: config.baseURL
        });
        
        this._view?.webview.postMessage({
            type: 'configData',
            config: {
                provider: config.provider,
                model: config.model,
                executablePath: config.executablePath,
                baseURL: config.baseURL,
                hasApiKey: !!config.apiKey,
                maxIterations: config.maxIterations,
                enableReflection: config.enableReflection
            }
        });
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