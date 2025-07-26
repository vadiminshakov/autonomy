import * as vscode from 'vscode';
import { AutonomyAgent } from './autonomyAgent';
import { AutonomyTaskProvider } from './taskProvider';
import { ConfigurationManager } from './configManager';
import { AutonomyWebviewProvider } from './webviewProvider';

let autonomyAgent: AutonomyAgent | undefined;
let taskProvider: AutonomyTaskProvider;
let webviewProvider: AutonomyWebviewProvider;

export function activate(context: vscode.ExtensionContext) {
    console.log('Autonomy extension is now active');

    const configManager = new ConfigurationManager();
    taskProvider = new AutonomyTaskProvider();
    webviewProvider = new AutonomyWebviewProvider(context.extensionUri, configManager);

    vscode.window.registerTreeDataProvider('autonomyTaskView', taskProvider);

    console.log('Registering webview provider with viewType:', AutonomyWebviewProvider.viewType);
    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(AutonomyWebviewProvider.viewType, webviewProvider)
    );
    console.log('Webview provider registered successfully');

    const startCommand = vscode.commands.registerCommand('autonomy.start', async (fromWebview?: boolean) => {
        console.log('extension: autonomy.start command called');
        
        if (autonomyAgent && autonomyAgent.isRunning()) {
            console.log('extension: Agent already running');
            vscode.window.showInformationMessage('Autonomy agent is already running');
            return;
        }

        try {
            console.log('extension: Getting configuration...');
            const config = configManager.getConfiguration();
            console.log('extension: Creating AutonomyAgent...');
            autonomyAgent = new AutonomyAgent(config, taskProvider);
            
            if (fromWebview && webviewProvider) {
                autonomyAgent.setOutputCallback((output: string, type: 'stdout' | 'stderr') => {
                    webviewProvider.sendAgentOutput(output, type);
                });
                
                autonomyAgent.setWebviewMode(true);
            }
            
            console.log('extension: Starting agent...');
            await autonomyAgent.start();
            
            console.log('extension: Agent started, updating webview...');
            webviewProvider.setAutonomyAgent(autonomyAgent);
            
            if (fromWebview) {
                webviewProvider.forceUpdateWebviewState();
            }
            
            vscode.commands.executeCommand('setContext', 'autonomy:active', true);
            if (!fromWebview) {
                vscode.window.showInformationMessage('Autonomy agent started successfully');
            }
            console.log('extension: autonomy.start command completed successfully');
        } catch (error) {
            console.error('extension: Error starting agent:', error);
            vscode.window.showErrorMessage(`Failed to start Autonomy agent: ${error}`);
            throw error;
        }
    });

    const runTaskCommand = vscode.commands.registerCommand('autonomy.runTask', async (taskMessage?: string) => {
        if (!autonomyAgent || !autonomyAgent.isRunning()) {
            const start = await vscode.window.showInformationMessage(
                'Autonomy agent is not running. Start it now?',
                'Start',
                'Cancel'
            );
            if (start === 'Start') {
                await vscode.commands.executeCommand('autonomy.start');
                if (!autonomyAgent || !autonomyAgent.isRunning()) {
                    return;
                }
            } else {
                return;
            }
        }

        let task = taskMessage;
        if (!task) {
            task = await vscode.window.showInputBox({
                prompt: 'Enter task for Autonomy agent',
                placeHolder: 'e.g., Add error handling to the getUserData function',
                ignoreFocusOut: true
            });
        }

        if (task) {
            autonomyAgent!.runTask(task);
        }
    });

    const configureCommand = vscode.commands.registerCommand('autonomy.configure', async () => {
        await configManager.configure();
    });

    const stopCommand = vscode.commands.registerCommand('autonomy.stop', async () => {
        if (autonomyAgent) {
            await autonomyAgent.stop();
            autonomyAgent = undefined;
            
            webviewProvider.setAutonomyAgent(undefined);
            
            vscode.commands.executeCommand('setContext', 'autonomy:active', false);
            vscode.window.showInformationMessage('Autonomy agent stopped');
        }
    });

    const openWebviewCommand = vscode.commands.registerCommand('autonomy.openWebview', async () => {
        await vscode.commands.executeCommand('autonomy.focus');
    });

    context.subscriptions.push(
        startCommand,
        runTaskCommand,
        configureCommand,
        stopCommand,
        openWebviewCommand
    );

    const autoStart = vscode.workspace.getConfiguration('autonomy').get<boolean>('autoStart');
    if (autoStart) {
        vscode.commands.executeCommand('autonomy.start');
    }

    const configChangeListener = vscode.workspace.onDidChangeConfiguration(event => {
        if (event.affectsConfiguration('autonomy')) {
            if (autonomyAgent) {
                vscode.window.showInformationMessage(
                    'Autonomy configuration changed. Restart the agent to apply changes.',
                    'Restart'
                ).then(selection => {
                    if (selection === 'Restart') {
                        vscode.commands.executeCommand('autonomy.stop').then(() => {
                            vscode.commands.executeCommand('autonomy.start');
                        });
                    }
                });
            }
        }
    });

    context.subscriptions.push(configChangeListener);
}

export function deactivate() {
    if (autonomyAgent) {
        autonomyAgent.stop();
    }
}