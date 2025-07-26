import * as vscode from 'vscode';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { AutonomyConfig } from './autonomyAgent';

export class ConfigurationManager {
    getConfiguration(): AutonomyConfig {
        const globalConfig = this.readGlobalConfig();
        
        const vscodeConfig = vscode.workspace.getConfiguration('autonomy');
        
        return {
            executablePath: globalConfig?.executable_path || vscodeConfig.get<string>('executablePath') || 'autonomy',
            provider: globalConfig?.provider || vscodeConfig.get<string>('provider') || 'openai',
            model: globalConfig?.model || vscodeConfig.get<string>('model') || 'o3',
            apiKey: globalConfig?.api_key || vscodeConfig.get<string>('apiKey') || '',
            baseURL: globalConfig?.base_url || vscodeConfig.get<string>('baseURL'),
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
                models = ['o3', 'o1', 'gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo'];
                break;
            case 'anthropic':
                models = ['claude-3-5-sonnet-20241022', 'claude-3-5-haiku-20241022', 'claude-3-opus-20240229'];
                break;
            case 'openrouter':
                models = ['openai/o3', 'anthropic/claude-3.5-sonnet', 'google/gemini-pro'];
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

        if (selectedProvider === 'openrouter') {
            const baseURL = await vscode.window.showInputBox({
                prompt: 'Enter base URL (optional)',
                placeHolder: 'https://openrouter.ai/api/v1',
                ignoreFocusOut: true
            });

            if (baseURL) {
                await config.update('baseURL', baseURL, vscode.ConfigurationTarget.Workspace);
            }
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