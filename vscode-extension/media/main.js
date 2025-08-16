
const vscode = acquireVsCodeApi();


let clearHistoryBtn, newTaskBtn, sendButton;
let messageInput, messagesContainer;
let configForm, providerSelect, apiKeyInput, modelInput, modelSelect, toggleModelInputBtn, executablePathInput, baseUrlInput;
let saveConfigBtn, loadConfigBtn;


let agentRunning = false;
let currentTab = 'chat';
let originalConfig = {}; // Store original config to track changes
let configChanged = false;


document.addEventListener('DOMContentLoaded', function () {
    initializeElements();
    setupEventListeners();
    loadConfig();

    // не активируем элементы принудительно, ждём информации о состоянии агента
    // запрашиваем текущее состояние агента
    vscode.postMessage({ type: 'getAgentStatus' });

    // Request to load message history
    vscode.postMessage({ type: 'loadHistory' });
});

function initializeElements() {

    clearHistoryBtn = document.getElementById('clear-history');
    newTaskBtn = document.getElementById('new-task');
    sendButton = document.getElementById('send-button');
    messageInput = document.getElementById('message-input');
    messagesContainer = document.getElementById('messages');


    providerSelect = document.getElementById('provider');
    apiKeyInput = document.getElementById('api-key');
    modelInput = document.getElementById('model');
    modelSelect = document.getElementById('model-select');
    toggleModelInputBtn = document.getElementById('toggle-model-input');
    executablePathInput = document.getElementById('executable-path');
    baseUrlInput = document.getElementById('base-url');
    saveConfigBtn = document.getElementById('save-config');
    loadConfigBtn = document.getElementById('load-config');

    // по умолчанию элементы ввода отключены до получения информации о состоянии агента
    if (sendButton && messageInput) {
        sendButton.disabled = true;
        messageInput.disabled = true;
        messageInput.placeholder = 'Starting agent...';
    }
}

function setupEventListeners() {

    clearHistoryBtn.addEventListener('click', () => {
        vscode.postMessage({ type: 'clearHistory' });
    });

    newTaskBtn.addEventListener('click', () => {
        vscode.postMessage({ type: 'newTask' });
    });


    sendButton.addEventListener('click', sendMessage);
    messageInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    });


    messageInput.addEventListener('input', function () {
        this.style.height = 'auto';
        this.style.height = Math.min(this.scrollHeight, 120) + 'px';
    });


    saveConfigBtn.addEventListener('click', saveConfig);
    loadConfigBtn.addEventListener('click', loadConfig);


    providerSelect.addEventListener('change', function () {
        onProviderChange();
        checkConfigChanges();
    });


    modelSelect.addEventListener('change', function () {
        if (this.value) {
            modelInput.value = this.value;
        }
        checkConfigChanges();
    });

    // Add change listeners to all config inputs
    apiKeyInput.addEventListener('input', checkConfigChanges);
    modelInput.addEventListener('input', checkConfigChanges);
    executablePathInput.addEventListener('input', checkConfigChanges);
    baseUrlInput.addEventListener('input', checkConfigChanges);

    toggleModelInputBtn.addEventListener('click', toggleModelInput);
}

function sendMessage() {
    const message = messageInput.value.trim();
    if (!message) return;

    vscode.postMessage({
        type: 'sendMessage',
        value: message
    });

    messageInput.value = '';
    messageInput.style.height = 'auto';
    setLoading(sendButton, true);
}

function addMessage(type, content, timestamp = null) {
    if (!messagesContainer) {
        return;
    }

    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${type}`;

    const contentDiv = document.createElement('div');
    contentDiv.className = 'message-content';

    // Parse markdown for agent messages, plain text for others
    if (type === 'agent' && typeof marked !== 'undefined') {
        try {
            // Process escaped newlines and other escape sequences
            let processedContent = content
                .replace(/\\n/g, '\n')           // Convert \n to actual newlines
                .replace(/\\t/g, '\t')           // Convert \t to actual tabs
                .replace(/\\r/g, '\r')           // Convert \r to actual carriage returns
                .replace(/\\\\/g, '\\');         // Convert \\ to single backslash

            // Special handling for tool calls
            processedContent = processedContent.replace(/🔧 Tool: ([^\n]+)/g, '<div class="tool-call">🔧 Tool: $1</div>');
            processedContent = processedContent.replace(/📋 Result: ([^\n]+)/g, '<div class="tool-call">📋 Result: $1</div>');

            // Configure marked with safe options
            marked.setOptions({
                breaks: true,
                gfm: true,
                sanitize: false,
                highlight: function (code, lang) {
                    // Simple syntax highlighting for common languages
                    if (lang && ['javascript', 'js', 'python', 'py', 'bash', 'sh', 'json', 'xml', 'html', 'css', 'go', 'rust'].includes(lang.toLowerCase())) {
                        return `<code class="language-${lang.toLowerCase()}">${escapeHtml(code)}</code>`;
                    }
                    return `<code>${escapeHtml(code)}</code>`;
                }
            });
            contentDiv.innerHTML = marked.parse(processedContent);
        } catch (e) {
            // Fallback to plain text if markdown parsing fails
            contentDiv.textContent = content;
        }
    } else {
        contentDiv.textContent = content;
    }

    messageDiv.appendChild(contentDiv);

    if (timestamp) {
        const timestampDiv = document.createElement('div');
        timestampDiv.className = 'message-timestamp';
        timestampDiv.textContent = new Date(timestamp).toLocaleTimeString();
        messageDiv.appendChild(timestampDiv);
    }

    messagesContainer.appendChild(messageDiv);
    messagesContainer.scrollTop = messagesContainer.scrollHeight;

    // Disable spinner when we receive response from agent
    if (type === 'agent' || type === 'system') {
        setLoading(sendButton, false);
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function addThinkingIndicator(messageId) {
    // Remove any existing thinking indicator
    removeThinkingIndicator();

    const thinkingDiv = document.createElement('div');
    thinkingDiv.className = 'message thinking';
    thinkingDiv.id = messageId;

    const contentDiv = document.createElement('div');
    contentDiv.className = 'thinking-content';
    contentDiv.innerHTML = '<span class="thinking-dots">thinking</span><span class="dots">...</span>';
    thinkingDiv.appendChild(contentDiv);

    messagesContainer.appendChild(thinkingDiv);
    messagesContainer.scrollTop = messagesContainer.scrollHeight;
}

function removeThinkingIndicator(messageId = null) {
    if (messageId) {
        const thinkingElement = document.getElementById(messageId);
        if (thinkingElement) {
            thinkingElement.remove();
        }
    } else {
        // Remove all thinking indicators
        const thinkingElements = document.querySelectorAll('.message.thinking');
        thinkingElements.forEach(el => el.remove());
    }
}

function clearMessages() {
    messagesContainer.innerHTML = '';
}

function updateAgentStatus(running) {
    console.log('webview: updateAgentStatus called with running:', running);
    agentRunning = running;

    if (running) {
        sendButton.disabled = false;
        messageInput.disabled = false;
        messageInput.placeholder = "Type your task here... (e.g., 'Add error handling to the getUserData function')";
        console.log('webview: Agent running - enabled input elements');
    } else {
        sendButton.disabled = true;
        messageInput.disabled = true;
        messageInput.placeholder = 'Agent not running. Check configuration or restart.';
        console.log('webview: Agent not running - disabled input elements');
    }

    setLoading(sendButton, false);
}

function setLoading(button, loading) {
    if (loading) {
        button.classList.add('loading');
        button.disabled = true;
    } else {
        button.classList.remove('loading');

        if (button === sendButton) {
            button.disabled = !agentRunning;
        } else {
            button.disabled = false;
        }
    }
}

function showTab(tabName) {

    document.querySelectorAll('.tab-content').forEach(tab => {
        tab.classList.remove('active');
    });


    document.querySelectorAll('.tab-button').forEach(btn => {
        btn.classList.remove('active');
    });


    document.getElementById(`${tabName}-tab`).classList.add('active');


    event.target.classList.add('active');

    currentTab = tabName;


    if (tabName === 'config') {
        loadConfig();
    }
}

function saveConfig() {
    // Add pressed animation
    saveConfigBtn.classList.add('btn-pressed');
    setTimeout(() => {
        saveConfigBtn.classList.remove('btn-pressed');
    }, 150);

    const config = {
        provider: providerSelect.value,
        apiKey: apiKeyInput.value,
        model: modelInput.value,
        executablePath: executablePathInput.value,
        baseURL: baseUrlInput.value
    };

    vscode.postMessage({
        type: 'configure',
        config: config
    });

    setLoading(saveConfigBtn, true);
}

function loadConfig() {
    vscode.postMessage({ type: 'getConfig' });
    setLoading(loadConfigBtn, true);
}

function populateConfigForm(config) {
    providerSelect.value = config.provider || 'openai';
    apiKeyInput.value = config.apiKey || '';
    modelInput.value = config.model || '';
    executablePathInput.value = config.executablePath || 'autonomy';
    baseUrlInput.value = config.baseURL || '';

    // Store original config for change detection
    originalConfig = {
        provider: config.provider || 'openai',
        apiKey: config.apiKey || '',
        model: config.model || '',
        executablePath: config.executablePath || 'autonomy',
        baseURL: config.baseURL || ''
    };

    onProviderChange();

    setLoading(loadConfigBtn, false);
    setLoading(saveConfigBtn, false);

    // Reset config changed state and update button
    configChanged = false;
    updateSaveButtonState();
}

function onProviderChange() {
    const provider = providerSelect.value;


    updateModelOptions(provider);


    updateBaseUrl(provider);
}

function updateModelOptions(provider) {
    const modelConfigs = {
        'openai': {
            placeholder: 'e.g., o4, o3, gpt-4.1',
            default: 'o3',
            models: ['o4', 'o3', 'gpt-4.1', 'gpt-4o']
        },
        'anthropic': {
            placeholder: 'e.g., claude-4-opus, claude-4-sonnet-20250514',
            default: 'claude-4-opus',
            models: ['claude-4-opus', 'claude-4-sonnet-20250514', 'claude-3-7-sonnet']
        },
        'openrouter': {
            placeholder: 'e.g., google/gemini-2.5-pro, x-ai/grok-4',
            default: 'google/gemini-2.5-pro',
            models: ['google/gemini-2.5-pro', 'x-ai/grok-4', 'moonshotai/kimi-k2', 'qwen/qwen3-coder', 'deepseek/deepseek-chat-v3-0324']
        },
        'local': {
            placeholder: 'e.g., deepseek-coder-v2:16b',
            default: 'deepseek-coder-v2:16b',
            models: ['deepseek-coder-v2:16b', 'llama4', 'llama3.2:latest', 'qwen2.5-coder:7b-instruct',]
        }
    };

    const config = modelConfigs[provider] || modelConfigs['openai'];
    modelInput.placeholder = config.placeholder;


    modelSelect.innerHTML = '<option value="">Select a model...</option>';
    config.models.forEach(model => {
        const option = document.createElement('option');
        option.value = model;
        option.textContent = model;
        modelSelect.appendChild(option);
    });


    if (!modelInput.value || shouldUpdateModel(provider, modelInput.value)) {
        modelInput.value = config.default;
        modelSelect.value = config.default;
    }
}

function shouldUpdateModel(provider, currentModel) {

    if (provider === 'openrouter' && !currentModel.includes('/')) {
        return true;
    }
    if (provider !== 'openrouter' && currentModel.includes('/')) {
        return true;
    }
    return false;
}

function updateBaseUrl(provider) {
    const baseUrls = {
        'openai': 'https://api.openai.com/v1',
        'anthropic': 'https://api.anthropic.com',
        'openrouter': 'https://openrouter.ai/api/v1',
        'local': 'http://localhost:11434/v1'
    };


    const currentUrl = baseUrlInput.value;
    const isDefaultUrl = Object.values(baseUrls).includes(currentUrl);

    if (!currentUrl || isDefaultUrl) {
        baseUrlInput.value = baseUrls[provider] || '';
    }
}

function toggleModelInput() {
    const isSelectVisible = modelSelect.style.display !== 'none';

    if (isSelectVisible) {

        modelSelect.style.display = 'none';
        modelInput.style.display = 'block';
        toggleModelInputBtn.textContent = '📋';
        toggleModelInputBtn.title = 'Switch to dropdown';
    } else {

        modelSelect.style.display = 'block';
        modelInput.style.display = 'none';
        toggleModelInputBtn.textContent = '✏️';
        toggleModelInputBtn.title = 'Switch to text input';


        if (modelInput.value) {
            modelSelect.value = modelInput.value;
        }
    }
}


window.addEventListener('message', event => {
    const message = event.data;

    switch (message.type) {
        case 'addMessage':
            addMessage(message.message.type, message.message.content, message.message.timestamp);
            break;

        case 'clearMessages':
            clearMessages();
            break;

        case 'updateState':
            console.log('webview: Received updateState message:', message.state);
            updateAgentStatus(message.state.agentRunning);
            break;

        case 'configData':
            populateConfigForm(message.config);
            break;

        case 'configSaved':
            setLoading(saveConfigBtn, false);
            // Reset config changed state after successful save
            configChanged = false;
            updateSaveButtonState();
            // Update original config with current values
            originalConfig = {
                provider: providerSelect.value,
                apiKey: apiKeyInput.value,
                model: modelInput.value,
                executablePath: executablePathInput.value,
                baseURL: baseUrlInput.value
            };
            break;

        case 'addThinking':
            addThinkingIndicator(message.messageId);
            break;

        case 'removeThinking':
            removeThinkingIndicator(message.messageId);
            break;
    }
});


// Check if config has changed
function checkConfigChanges() {
    const currentConfig = {
        provider: providerSelect.value,
        apiKey: apiKeyInput.value,
        model: modelInput.value,
        executablePath: executablePathInput.value,
        baseURL: baseUrlInput.value
    };

    configChanged = JSON.stringify(currentConfig) !== JSON.stringify(originalConfig);
    updateSaveButtonState();
}

// Update save button state based on config changes
function updateSaveButtonState() {
    if (configChanged) {
        saveConfigBtn.classList.remove('btn-save-disabled');
        saveConfigBtn.disabled = false;
    } else {
        saveConfigBtn.classList.add('btn-save-disabled');
        saveConfigBtn.disabled = true;
    }
}

window.showTab = showTab;