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

    console.log('Starting checkAndInstallAutonomy...');
    checkAndInstallAutonomy(context).then(() => {
        console.log('Autonomy installation check completed, webview can now auto-start');
        webviewProvider.enableAutoStart();
    }).catch(error => {
        console.error('Error in checkAndInstallAutonomy:', error);
    });

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
            
            // Always set webview mode when using extension, and set output callback
            autonomyAgent.setOutputCallback((output: string, type: 'stdout' | 'stderr') => {
                webviewProvider.sendAgentOutput(output, type);
            });
            
            autonomyAgent.setWebviewMode(true);
            
            console.log('extension: Starting agent...');
            await autonomyAgent.start();
            
            console.log('extension: Agent started, updating webview...');
            webviewProvider.setAutonomyAgent(autonomyAgent);
            
            if (fromWebview) {
                webviewProvider.forceUpdateWebviewState();
            }
            
            vscode.commands.executeCommand('setContext', 'autonomy:active', true);
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

    const installCommand = vscode.commands.registerCommand('autonomy.installCli', async () => {
        vscode.window.showInformationMessage('Starting Autonomy CLI installation check...');
        await checkAndInstallAutonomy(context);
    });

    context.subscriptions.push(
        startCommand,
        runTaskCommand,
        configureCommand,
        stopCommand,
        openWebviewCommand,
        installCommand
    );

    // Auto-start removed since we only use global config now

    // Config change listener removed since we only use global config now
}

async function checkAndInstallAutonomy(context: vscode.ExtensionContext) {
    console.log('checkAndInstallAutonomy: Starting installation check...');
    const { exec } = require('child_process');
    const fs = require('fs');
    const path = require('path');
    const os = require('os');
    
    function checkIfAutonomyExists(): Promise<boolean> {
        console.log('checkIfAutonomyExists: Checking autonomy --version...');
        return new Promise((resolve) => {
            exec('autonomy --version', (error: any, stdout: string, stderr: string) => {
                if (error) {
                    console.log('checkIfAutonomyExists: Command failed:', error.message);
                    resolve(false);
                } else {
                    console.log('checkIfAutonomyExists: Autonomy already installed:', stdout.trim());
                    resolve(true);
                }
            });
        });
    }
    
    function installAutonomy(): Promise<boolean> {
        return new Promise((resolve) => {
            const platform = os.platform();
            let installCommand: string;
            
            switch (platform) {
                case 'darwin':
                    const arch = os.arch() === 'arm64' ? 'arm64' : 'amd64';
                    installCommand = `mkdir -p ~/.local/bin && cd ~/.local/bin && curl -L -o autonomy.tar.gz https://github.com/vadiminshakov/autonomy/releases/latest/download/autonomy-darwin-${arch}.tar.gz && tar -xzf autonomy.tar.gz && rm autonomy.tar.gz && chmod +x autonomy`;
                    break;
                case 'linux':
                    const linuxArch = os.arch() === 'arm64' ? 'arm64' : 'amd64';
                    installCommand = `mkdir -p ~/.local/bin && cd ~/.local/bin && curl -L -o autonomy.tar.gz https://github.com/vadiminshakov/autonomy/releases/latest/download/autonomy-linux-${linuxArch}.tar.gz && tar -xzf autonomy.tar.gz && rm autonomy.tar.gz && chmod +x autonomy`;
                    break;
                case 'win32':
                    installCommand = 'powershell -Command "iwr -Uri https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.ps1 | iex"';
                    break;
                default:
                    console.log('Unsupported platform:', platform);
                    vscode.window.showWarningMessage(
                        'Autonomy CLI auto-installation not supported on this platform. Please install manually from: https://github.com/vadiminshakov/autonomy'
                    );
                    resolve(false);
                    return;
            }
            
            console.log('Installing Autonomy CLI...');
            vscode.window.showInformationMessage('Installing Autonomy CLI... This may take a moment.');
            
            exec(installCommand, { timeout: 60000 }, async (error: any, stdout: string, stderr: string) => {
                if (error) {
                    console.error('Auto-installation failed:', error.message);
                    vscode.window.showErrorMessage(
                        'Autonomy CLI auto-installation failed. Please install manually.',
                        'Open Instructions'
                    ).then(selection => {
                        if (selection === 'Open Instructions') {
                            vscode.env.openExternal(vscode.Uri.parse('https://github.com/vadiminshakov/autonomy#installation'));
                        }
                    });
                    resolve(false);
                } else {
                    console.log('Autonomy installed successfully!');
                    
                    // Update global config to use local bin path
                    const homePath = os.homedir();
                    const configPath = path.join(homePath, '.autonomy', 'config.json');
                    try {
                        let globalConfig = {};
                        if (fs.existsSync(configPath)) {
                            globalConfig = JSON.parse(fs.readFileSync(configPath, 'utf8'));
                        }
                        globalConfig.executable_path = `${homePath}/.local/bin/autonomy`;
                        if (!fs.existsSync(path.dirname(configPath))) {
                            fs.mkdirSync(path.dirname(configPath), { recursive: true });
                        }
                        fs.writeFileSync(configPath, JSON.stringify(globalConfig, null, 2));
                    } catch (error) {
                        console.error('Failed to update global config:', error);
                    }
                    
                    vscode.window.showInformationMessage('Autonomy CLI installed successfully! Please restart VS Code to update PATH.');
                    resolve(true);
                }
            });
        });
    }
    
    function createConfigExample() {
        const configDir = path.join(os.homedir(), '.autonomy');
        const configFile = path.join(configDir, 'config.json');
        
        if (!fs.existsSync(configFile)) {
            try {
                if (!fs.existsSync(configDir)) {
                    fs.mkdirSync(configDir, { recursive: true });
                }
                
                const exampleConfig = {
                    provider: "openai",
                    model: "o3",
                    api_key: "your-api-key-here",
                    base_url: "https://api.openai.com/v1",
                    max_iterations: 100,
                    enable_reflection: true
                };
                
                fs.writeFileSync(configFile, JSON.stringify(exampleConfig, null, 2));
                console.log('Example config created at:', configFile);
            } catch (error) {
                console.log('Could not create config file:', error);
            }
        }
    }
    
    try {
        console.log('checkAndInstallAutonomy: Checking if Autonomy exists...');
        const autonomyExists = await checkIfAutonomyExists();
        console.log('checkAndInstallAutonomy: Autonomy exists:', autonomyExists);
        
        if (!autonomyExists) {
            const autoInstall = true; // Always auto-install since we don't use VSCode settings
            
            if (autoInstall) {
                console.log('checkAndInstallAutonomy: Autonomy not found, installing automatically...');
                vscode.window.showInformationMessage('Installing Autonomy CLI automatically...');
                
                const installSuccess = await installAutonomy();
                if (installSuccess) {
                    createConfigExample();
                    vscode.window.showInformationMessage('Autonomy CLI installed successfully! Extension is ready to use.');
                } else {
                    vscode.window.showErrorMessage(
                        'Failed to install Autonomy CLI automatically. Please install manually.',
                        'Open Instructions'
                    ).then(selection => {
                        if (selection === 'Open Instructions') {
                            vscode.env.openExternal(vscode.Uri.parse('https://github.com/vadiminshakov/autonomy#installation'));
                        }
                    });
                }
            } else {
                console.log('checkAndInstallAutonomy: Auto-install disabled, showing manual instructions');
                vscode.window.showWarningMessage(
                    'Autonomy CLI is not installed. Auto-install is disabled.',
                    'Install Manually',
                    'Enable Auto-install'
                ).then(selection => {
                    if (selection === 'Install Manually') {
                        vscode.env.openExternal(vscode.Uri.parse('https://github.com/vadiminshakov/autonomy#installation'));
                    } else if (selection === 'Enable Auto-install') {
                        checkAndInstallAutonomy(context);
                    }
                });
            }
        } else {
            console.log('checkAndInstallAutonomy: Autonomy already installed');
            createConfigExample();
        }
    } catch (error) {
        console.error('Installation check failed:', error);
    }
}

export function deactivate() {
    if (autonomyAgent) {
        autonomyAgent.stop();
    }
}