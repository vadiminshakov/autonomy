import * as vscode from 'vscode';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { AutonomyConfig } from './autonomyAgent';

export class ConfigurationManager {
    getConfiguration(): AutonomyConfig {
        const globalConfig = this.readGlobalConfig();
        
        if (!globalConfig) {
            throw new Error('Configuration file not found at ~/.autonomy/config.json. Please create the configuration file.');
        }
        
        const provider = globalConfig.provider || 'openai';
        
        const baseUrlDefaults = {
            'openai': 'https://api.openai.com/v1',
            'anthropic': 'https://api.anthropic.com', 
            'openrouter': 'https://openrouter.ai/api/v1'
        };
        
        return {
            executablePath: globalConfig.executable_path || 'autonomy',
            provider: provider,
            model: globalConfig.model || 'o3',
            apiKey: globalConfig.api_key || '',
            baseURL: globalConfig.base_url || baseUrlDefaults[provider as keyof typeof baseUrlDefaults],
            maxIterations: globalConfig.max_iterations || 100,
            enableReflection: globalConfig.enable_reflection !== undefined ? globalConfig.enable_reflection : true,
            skipExecutableValidation: false
        };
    }

    private readGlobalConfig(): any {
        try {
            const configPath = path.join(os.homedir(), '.autonomy', 'config.json');
            console.log('configManager: Attempting to read global config from:', configPath);
            
            if (fs.existsSync(configPath)) {
                const configContent = fs.readFileSync(configPath, 'utf8');
                const config = JSON.parse(configContent);
                console.log('configManager: Successfully loaded global config');
                return config;
            } else {
                console.log('configManager: Global config file not found');
                return null;
            }
        } catch (error) {
            console.error('configManager: Error reading global config:', error);
            return null;
        }
    }

    private async writeGlobalConfig(config: any): Promise<void> {
        try {
            const configDir = path.join(os.homedir(), '.autonomy');
            const configPath = path.join(configDir, 'config.json');
            
            if (!fs.existsSync(configDir)) {
                fs.mkdirSync(configDir, { recursive: true });
            }
            
            fs.writeFileSync(configPath, JSON.stringify(config, null, 2));
            console.log('configManager: Configuration saved to:', configPath);
        } catch (error) {
            console.error('configManager: Error writing global config:', error);
            throw error;
        }
    }

    async configure(): Promise<void> {
        const currentConfig = this.readGlobalConfig() || {};
        
        const providers = ['openai', 'anthropic', 'openrouter'];
        const selectedProvider = await vscode.window.showQuickPick(providers, {
            placeHolder: 'Select AI provider',
            canPickMany: false
        });

        if (selectedProvider) {
            currentConfig.provider = selectedProvider;
        }

        const apiKey = await vscode.window.showInputBox({
            prompt: `Enter API key for ${selectedProvider}`,
            password: true,
            ignoreFocusOut: true
        });

        if (apiKey) {
            currentConfig.api_key = apiKey;
        }

        let models: string[] = [];
        switch (selectedProvider) {
            case 'openai':
                models = ['o4', 'o3', 'gpt-4.1', 'gpt-4o'];
                break;
            case 'anthropic':
                models = ['claude-4-opus', 'claude-4-sonnet-20250514', 'claude-3-7-sonnet'];
                break;
            case 'openrouter':
                models = ['google/gemini-2.5-pro', 'x-ai/grok-4', 'moonshotai/kimi-k2', 'qwen/qwen3-coder', 'deepseek/deepseek-chat-v3-0324'];
                break;
        }

        if (models.length > 0) {
            const selectedModel = await vscode.window.showQuickPick(models, {
                placeHolder: 'Select model',
                canPickMany: false
            });

            if (selectedModel) {
                currentConfig.model = selectedModel;
            }
        }

        const baseUrlDefaults = {
            'openai': 'https://api.openai.com/v1',
            'anthropic': 'https://api.anthropic.com', 
            'openrouter': 'https://openrouter.ai/api/v1'
        };

        const defaultProvider = selectedProvider || 'openai';
        const baseURL = await vscode.window.showInputBox({
            prompt: 'Enter base URL (optional)',
            placeHolder: baseUrlDefaults[defaultProvider as keyof typeof baseUrlDefaults],
            value: baseUrlDefaults[defaultProvider as keyof typeof baseUrlDefaults],
            ignoreFocusOut: true
        });

        if (baseURL) {
            currentConfig.base_url = baseURL;
        }

        const executablePath = await vscode.window.showInputBox({
            prompt: 'Enter path to autonomy executable',
            value: currentConfig.executable_path || 'autonomy',
            ignoreFocusOut: true
        });

        if (executablePath) {
            currentConfig.executable_path = executablePath;
        }

        const advancedSettings = await vscode.window.showQuickPick(
            ['Yes', 'No'],
            {
                placeHolder: 'Configure advanced settings?',
                canPickMany: false
            }
        );

        if (advancedSettings === 'Yes') {
            const maxIterations = await vscode.window.showInputBox({
                prompt: 'Maximum task iterations',
                value: currentConfig.max_iterations?.toString() || '100',
                validateInput: (value) => {
                    const num = parseInt(value);
                    if (isNaN(num) || num <= 0) {
                        return 'Please enter a positive number';
                    }
                    return null;
                }
            });

            if (maxIterations) {
                currentConfig.max_iterations = parseInt(maxIterations);
            }

            const enableReflection = await vscode.window.showQuickPick(
                ['Yes', 'No'],
                {
                    placeHolder: 'Enable reflection system?',
                    canPickMany: false
                }
            );

            if (enableReflection) {
                currentConfig.enable_reflection = enableReflection === 'Yes';
            }
        }

        await this.writeGlobalConfig(currentConfig);
        vscode.window.showInformationMessage('Autonomy configuration updated successfully!');
    }

    async validateConfiguration(): Promise<boolean> {
        const config = this.getConfiguration();
        
        if (!config.apiKey) {
            const configure = await vscode.window.showWarningMessage(
                'Autonomy API key is not configured',
                'Configure Now',
                'Cancel'
            );
            
            if (configure === 'Configure Now') {
                await this.configure();
                return this.validateConfiguration();
            }
            return false;
        }

        return true;
    }
}