import * as vscode from 'vscode';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { AutonomyConfig } from './autonomyAgent';

export class ConfigurationManager {
    getConfiguration(): AutonomyConfig {
        const globalConfig = this.readGlobalConfig();
        
        const vscodeConfig = vscode.workspace.getConfiguration('autonomy');
        
        const provider = globalConfig?.provider || vscodeConfig.get<string>('provider') || 'openai';
        
        const baseUrlDefaults = {
            'openai': 'https://api.openai.com/v1',
            'anthropic': 'https://api.anthropic.com', 
            'openrouter': 'https://openrouter.ai/api/v1'
        };
        
        return {
            executablePath: globalConfig?.executable_path || vscodeConfig.get<string>('executablePath') || 'autonomy',
            provider: provider,
            model: globalConfig?.model || vscodeConfig.get<string>('model') || 'o3',
            apiKey: globalConfig?.api_key || vscodeConfig.get<string>('apiKey') || '',
            baseURL: globalConfig?.base_url || vscodeConfig.get<string>('baseURL') || baseUrlDefaults[provider as keyof typeof baseUrlDefaults],
            maxIterations: globalConfig?.max_iterations || vscodeConfig.get<number>('maxIterations') || 100,
            enableReflection: globalConfig?.enable_reflection !== undefined ? globalConfig.enable_reflection : vscodeConfig.get<boolean>('enableReflection') || true,
            skipExecutableValidation: vscodeConfig.get<boolean>('skipExecutableValidation') || false
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
                console.log('configManager: Global config file not found, using VS Code settings');
                return null;
            }
        } catch (error) {
            console.error('configManager: Error reading global config:', error);
            return null;
        }
    }

    async configure(): Promise<void> {
        const config = vscode.workspace.getConfiguration('autonomy');
        
        const providers = ['openai', 'anthropic', 'openrouter'];
        const selectedProvider = await vscode.window.showQuickPick(providers, {
            placeHolder: 'Select AI provider',
            canPickMany: false
        });

        if (selectedProvider) {
            await config.update('provider', selectedProvider, vscode.ConfigurationTarget.Workspace);
        }

        const apiKey = await vscode.window.showInputBox({
            prompt: `Enter API key for ${selectedProvider}`,
            password: true,
            ignoreFocusOut: true
        });

        if (apiKey) {
            await config.update('apiKey', apiKey, vscode.ConfigurationTarget.Workspace);
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
                await config.update('model', selectedModel, vscode.ConfigurationTarget.Workspace);
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
            await config.update('baseURL', baseURL, vscode.ConfigurationTarget.Workspace);
        }

        const executablePath = await vscode.window.showInputBox({
            prompt: 'Enter path to autonomy executable',
            value: config.get<string>('executablePath') || 'autonomy',
            ignoreFocusOut: true
        });

        if (executablePath) {
            await config.update('executablePath', executablePath, vscode.ConfigurationTarget.Workspace);
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
                value: config.get<number>('maxIterations')?.toString() || '100',
                validateInput: (value) => {
                    const num = parseInt(value);
                    if (isNaN(num) || num <= 0) {
                        return 'Please enter a positive number';
                    }
                    return null;
                }
            });

            if (maxIterations) {
                await config.update('maxIterations', parseInt(maxIterations), vscode.ConfigurationTarget.Workspace);
            }

            const enableReflection = await vscode.window.showQuickPick(
                ['Yes', 'No'],
                {
                    placeHolder: 'Enable reflection system?',
                    canPickMany: false
                }
            );

            if (enableReflection) {
                await config.update('enableReflection', enableReflection === 'Yes', vscode.ConfigurationTarget.Workspace);
            }

            const autoStart = await vscode.window.showQuickPick(
                ['Yes', 'No'],
                {
                    placeHolder: 'Auto-start agent when VS Code opens?',
                    canPickMany: false
                }
            );

            if (autoStart) {
                await config.update('autoStart', autoStart === 'Yes', vscode.ConfigurationTarget.Workspace);
            }
        }

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