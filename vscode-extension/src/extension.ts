import * as vscode from 'vscode';
import { AutonomyAgent } from './autonomyAgent';
import { AutonomyTaskProvider } from './taskProvider';
import { ConfigurationManager } from './configManager';
import { AutonomyWebviewProvider } from './webviewProvider';

let autonomyAgent: AutonomyAgent | undefined;
let taskProvider: AutonomyTaskProvider;
let webviewProvider: AutonomyWebviewProvider;

export function activate(context: vscode.ExtensionContext) {

    const configManager = new ConfigurationManager();
    taskProvider = new AutonomyTaskProvider();
    webviewProvider = new AutonomyWebviewProvider(context.extensionUri, configManager);

    // Run check in background without blocking extension
    quickCheckAutonomy(context).then((available) => {
        if (available) {
            webviewProvider.enableAutoStart();
        } else {
            // enable auto-start anyway, let webview handle it itself
            webviewProvider.enableAutoStart();
        }
    }).catch(error => {
        // don't block auto-start even if check failed
        webviewProvider.enableAutoStart();
    });

    vscode.window.registerTreeDataProvider('autonomyTaskView', taskProvider);

    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(AutonomyWebviewProvider.viewType, webviewProvider)
    );

    const startCommand = vscode.commands.registerCommand('autonomy.start', async (fromWebview?: boolean) => {

        // Now all agent management happens through webview
        // This command remains for backward compatibility
        if (fromWebview) {
            return;
        }

        // For commands not from webview show message
        vscode.window.showInformationMessage('Autonomy agent is managed through the Autonomy panel. Please use the webview interface.');
    });

    const runTaskCommand = vscode.commands.registerCommand('autonomy.runTask', async (taskMessage?: string) => {
        // Now all tasks are executed through webview
        let task = taskMessage;
        if (!task) {
            task = await vscode.window.showInputBox({
                prompt: 'Enter task for Autonomy agent',
                placeHolder: 'e.g., Add error handling to the getUserData function',
                ignoreFocusOut: true
            });
        }

        if (task) {
            // Pass task to webview
            webviewProvider.handleTaskFromCommand(task);

            // Show webview
            vscode.commands.executeCommand('autonomyWebview.focus');
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
            webviewProvider.onAgentStopped(); // Clear messages file

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

    // Watch for global config changes and restart agent automatically
    const os = require('os');
    const path = require('path');
    const globalConfigPath = path.join(os.homedir(), '.autonomy', 'config.json');

    const configWatcher = vscode.workspace.createFileSystemWatcher(globalConfigPath);
    configWatcher.onDidChange(async () => {
        if (autonomyAgent && autonomyAgent.isRunning()) {
            await autonomyAgent.stop();
            webviewProvider.onAgentStopped(); // Clear messages file
            try {
                const config = configManager.getConfiguration();
                autonomyAgent = new AutonomyAgent(config, taskProvider);
                autonomyAgent.setOutputCallback((output: string, type: 'stdout' | 'stderr' | 'task_status') => {
                    webviewProvider.sendAgentOutput(output, type);
                });
                autonomyAgent.setWebviewMode(true);
                await autonomyAgent.start();
                webviewProvider.setAutonomyAgent(autonomyAgent);
                vscode.window.showInformationMessage('Autonomy agent restarted due to config changes');
            } catch (error) {
                vscode.window.showErrorMessage(`Failed to restart Autonomy agent: ${error}`);
            }
        }
    });

    context.subscriptions.push(configWatcher);
}

// quick check for autonomy presence without installation
async function quickCheckAutonomy(context: vscode.ExtensionContext): Promise<boolean> {
    const { exec } = require('child_process');

    return new Promise((resolve) => {
        const child = exec('autonomy --version', {
            timeout: 3000,
            env: {
                ...process.env,
                PATH: process.env.PATH || '/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin',
            },
            shell: true
        }, (error: any, stdout: string) => {
            if (error) {
                resolve(false);
            } else {
                resolve(true);
            }
        });

        // 3 second timeout
        setTimeout(() => {
            if (!child.killed) {
                child.kill('SIGTERM');
                resolve(false);
            }
        }, 3000);
    });
}

async function checkAndInstallAutonomy(context: vscode.ExtensionContext) {
    const { exec } = require('child_process');
    const fs = require('fs');
    const path = require('path');
    const os = require('os');

    function checkIfAutonomyExists(): Promise<boolean> {
        return new Promise((resolve) => {
            const checkOptions = {
                timeout: 5000, // 5 second timeout
                env: {
                    ...process.env,
                    PATH: process.env.PATH || '/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin',
                    HOME: process.env.HOME || require('os').homedir(),
                    SHELL: process.env.SHELL || '/bin/bash'
                },
                shell: true
            };

            const child = exec('autonomy --version', checkOptions, (error: any, stdout: string, stderr: string) => {
                if (error) {
                    resolve(false);
                } else {
                    resolve(true);
                }
            });

            // Force terminate after 5 seconds if command hangs
            setTimeout(() => {
                if (!child.killed) {
                    child.kill('SIGTERM');
                    resolve(false);
                }
            }, 5000);
        });
    }

    function installAutonomy(): Promise<boolean> {
        return new Promise((resolve) => {
            let attempt = 0;
            const maxAttempts = 10; // Unlimited with exponential backoff

            const attemptInstall = () => {
                attempt++;
                const backoffMs = Math.min(1000 * Math.pow(2, attempt - 1), 30000); // Max 30 seconds

                // Send installation status to webview
                if (webviewProvider) {
                    webviewProvider.sendAgentOutput(`Installing Autonomy CLI (attempt ${attempt})...`, 'stdout');
                }

                const installCommand = 'curl -sSL https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.sh | bash';

                // Set up proper environment for shell execution
                const execOptions = {
                    timeout: 120000,
                    env: {
                        ...process.env,
                        PATH: process.env.PATH || '/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin',
                        HOME: process.env.HOME || require('os').homedir(),
                        SHELL: process.env.SHELL || '/bin/bash'
                    },
                    shell: true
                };

                exec(installCommand, execOptions, async (error: any, stdout: string, stderr: string) => {
                    if (error) {
                        const errorMsg = `Installation attempt ${attempt} failed: ${error.message}`;

                        // Send error to webview chat
                        if (webviewProvider) {
                            webviewProvider.sendAgentOutput(`❌ ${errorMsg}`, 'stderr');
                            if (stderr) {
                                webviewProvider.sendAgentOutput(`Error details: ${stderr}`, 'stderr');
                            }
                        }

                        if (attempt < maxAttempts) {
                            const retryMsg = `Retrying in ${backoffMs / 1000} seconds... (attempt ${attempt + 1})`;
                            if (webviewProvider) {
                                webviewProvider.sendAgentOutput(retryMsg, 'stdout');
                            }
                            setTimeout(attemptInstall, backoffMs);
                        } else {
                            const failMsg = 'Autonomy CLI auto-installation failed after maximum attempts. Please install manually.';
                            if (webviewProvider) {
                                webviewProvider.sendAgentOutput(`❌ ${failMsg}`, 'stderr');
                                webviewProvider.sendAgentOutput('Installation instructions: https://github.com/vadiminshakov/autonomy#installation', 'stderr');
                            }
                            vscode.window.showErrorMessage(failMsg, 'Open Instructions').then(selection => {
                                if (selection === 'Open Instructions') {
                                    vscode.env.openExternal(vscode.Uri.parse('https://github.com/vadiminshakov/autonomy#installation'));
                                }
                            });
                            resolve(false);
                        }
                    } else {
                        // Verify installation by checking if autonomy command works
                        exec('autonomy --version', execOptions, (verifyError: any, verifyStdout: string) => {
                            if (verifyError) {
                                const verifyMsg = `Installation completed but verification failed: ${verifyError.message}`;
                                if (webviewProvider) {
                                    webviewProvider.sendAgentOutput(`⚠️ ${verifyMsg}`, 'stderr');
                                }

                                if (attempt < maxAttempts) {
                                    const retryMsg = `Retrying verification in ${backoffMs / 1000} seconds...`;
                                    if (webviewProvider) {
                                        webviewProvider.sendAgentOutput(retryMsg, 'stdout');
                                    }
                                    setTimeout(attemptInstall, backoffMs);
                                } else {
                                    resolve(false);
                                }
                            } else {
                                const successMsg = `✅ Autonomy CLI installed successfully! Version: ${verifyStdout.trim()}`;
                                if (webviewProvider) {
                                    webviewProvider.sendAgentOutput(successMsg, 'stdout');
                                }
                                vscode.window.showInformationMessage('Autonomy CLI installed successfully!');
                                resolve(true);
                            }
                        });
                    }
                });
            };

            attemptInstall();
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
            } catch (error) {
                // Could not create config file
            }
        }
    }

    try {
        const autonomyExists = await checkIfAutonomyExists();

        if (!autonomyExists) {
            const autoInstall = true; // Always auto-install since we don't use VSCode settings

            if (autoInstall) {
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
            createConfigExample();
        }
    } catch (error) {
        // Installation check failed
    }
}

export function deactivate() {

    // Stop global agent
    if (autonomyAgent) {
        autonomyAgent.stop().catch(error => {
            // Error stopping global agent
        });
    }

    // Stop webview agent
    if (webviewProvider) {
        webviewProvider.cleanup().catch(error => {
            // Error cleaning up webview
        });
    }
}