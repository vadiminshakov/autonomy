
const vscode = acquireVsCodeApi();


let clearHistoryBtn, sendButton;
let messageInput, messagesContainer;
let statusIndicator, statusText;
let configForm, providerSelect, apiKeyInput, modelInput, modelSelect, toggleModelInputBtn, executablePathInput, baseUrlInput;
let saveConfigBtn, loadConfigBtn;


let agentRunning = false;
let currentTab = 'chat';


document.addEventListener('DOMContentLoaded', function() {
    initializeElements();
    setupEventListeners();
    loadConfig();
    
    
    console.log('webview JS: DOM loaded, setting initial state');
    if (sendButton && messageInput) {
        sendButton.disabled = false;
        messageInput.disabled = false;
        console.log('webview JS: Input field enabled on load');
    }
});

function initializeElements() {
    
    clearHistoryBtn = document.getElementById('clear-history');
    sendButton = document.getElementById('send-button');
    messageInput = document.getElementById('message-input');
    messagesContainer = document.getElementById('messages');
    statusIndicator = document.getElementById('status-indicator');
    statusText = document.getElementById('status-text');

    
    providerSelect = document.getElementById('provider');
    apiKeyInput = document.getElementById('api-key');
    modelInput = document.getElementById('model');
    modelSelect = document.getElementById('model-select');
    toggleModelInputBtn = document.getElementById('toggle-model-input');
    executablePathInput = document.getElementById('executable-path');
    baseUrlInput = document.getElementById('base-url');
    saveConfigBtn = document.getElementById('save-config');
    loadConfigBtn = document.getElementById('load-config');
}

function setupEventListeners() {
    
    clearHistoryBtn.addEventListener('click', () => {
        vscode.postMessage({ type: 'clearHistory' });
    });

    
    sendButton.addEventListener('click', sendMessage);
    messageInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    });

    
    messageInput.addEventListener('input', function() {
        this.style.height = 'auto';
        this.style.height = Math.min(this.scrollHeight, 120) + 'px';
    });

    
    saveConfigBtn.addEventListener('click', saveConfig);
    loadConfigBtn.addEventListener('click', loadConfig);

    
    providerSelect.addEventListener('change', onProviderChange);
    
    
    modelSelect.addEventListener('change', function() {
        if (this.value) {
            modelInput.value = this.value;
        }
    });
    
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
    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${type}`;
    
    const contentDiv = document.createElement('div');
    contentDiv.textContent = content;
    messageDiv.appendChild(contentDiv);

    if (timestamp) {
        const timestampDiv = document.createElement('div');
        timestampDiv.className = 'message-timestamp';
        timestampDiv.textContent = new Date(timestamp).toLocaleTimeString();
        messageDiv.appendChild(timestampDiv);
    }

    messagesContainer.appendChild(messageDiv);
    messagesContainer.scrollTop = messagesContainer.scrollHeight;
}

function clearMessages() {
    messagesContainer.innerHTML = '';
}

function updateAgentStatus(running) {
    console.log('webview JS: Updating agent status to:', running);
    agentRunning = running;
    
    if (running) {
        statusIndicator.className = 'status-dot online';
        statusText.textContent = 'Online';
        sendButton.disabled = false;
        messageInput.disabled = false;
        console.log('webview JS: Input field enabled');
    } else {
        statusIndicator.className = 'status-dot offline';
        statusText.textContent = 'Offline';
        sendButton.disabled = true;
        messageInput.disabled = true;
        console.log('webview JS: Input field disabled');
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
    apiKeyInput.value = config.hasApiKey ? '••••••••••••••••' : '';
    modelInput.value = config.model || '';
    executablePathInput.value = config.executablePath || 'autonomy';
    baseUrlInput.value = config.baseURL || '';

    
    onProviderChange();
    
    setLoading(loadConfigBtn, false);
    setLoading(saveConfigBtn, false);
}

function onProviderChange() {
    const provider = providerSelect.value;
    
    
    updateModelOptions(provider);
    
    
    updateBaseUrl(provider);
}

function updateModelOptions(provider) {
    const modelConfigs = {
        'openai': {
            placeholder: 'e.g., o3, gpt-4o, gpt-4o-mini',
            default: 'o3',
            models: ['o3', 'gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo']
        },
        'anthropic': {
            placeholder: 'e.g., claude-3-5-sonnet-20241022',
            default: 'claude-3-5-sonnet-20241022',
            models: ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022', 'claude-3-opus-20240229']
        },
        'openrouter': {
            placeholder: 'e.g., openai/o3, anthropic/claude-3.5-sonnet',
            default: 'openai/o3',
            models: ['openai/o3', 'anthropic/claude-3.5-sonnet', 'google/gemini-pro-1.5', 'meta-llama/llama-3.1-405b']
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
        'openai': 'https:
        'anthropic': 'https:
        'openrouter': 'https:
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
            updateAgentStatus(message.state.agentRunning);
            break;
        
        case 'configData':
            populateConfigForm(message.config);
            break;
        
        case 'configSaved':
            setLoading(saveConfigBtn, false);
            break;
    }
});


window.showTab = showTab;