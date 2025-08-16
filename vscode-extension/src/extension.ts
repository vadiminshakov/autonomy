import * as vscode from 'vscode';
import { AutonomyAgent } from './autonomyAgent';
import { AutonomyTaskProvider } from './taskProvider';
import { ConfigurationManager } from './configManager';
import { AutonomyWebviewProvider } from './webviewProvider';

let autonomyAgent: AutonomyAgent | undefined;
let taskProvider: AutonomyTaskProvider;
let webviewProvider: AutonomyWebviewProvider;

export function activate(context: vscode.ExtensionContext) {
    console.log('🚀 Autonomy extension is now active');

    const configManager = new ConfigurationManager();
    taskProvider = new AutonomyTaskProvider();
    webviewProvider = new AutonomyWebviewProvider(context.extensionUri, configManager);

    console.log('Starting quick autonomy check...');
    // Запускаем проверку в фоне без блокировки расширения
    quickCheckAutonomy(context).then((available) => {
        if (available) {
            console.log('Autonomy is available, webview can auto-start');
            webviewProvider.enableAutoStart();
        } else {
            console.log('Autonomy not immediately available, but webview will still try to auto-start');
            // всё равно включаем автостарт, пусть webview сам разбирается
            webviewProvider.enableAutoStart();
        }
    }).catch(error => {
        console.error('Error in quickCheckAutonomy:', error);
        // не блокируем автостарт даже если проверка не удалась
        webviewProvider.enableAutoStart();
    });

    vscode.window.registerTreeDataProvider('autonomyTaskView', taskProvider);

    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(AutonomyWebviewProvider.viewType, webviewProvider)
    );
    console.log('✅ Webview provider registered');

    const startCommand = vscode.commands.registerCommand('autonomy.start', async (fromWebview?: boolean) => {
        console.log('extension: autonomy.start command called - delegating to webview');

        // Теперь все управление агентами происходит через webview
        // Эта команда остается для обратной совместимости
        if (fromWebview) {
            console.log('extension: Start request from webview - handled internally');
            return;
        }

        // Для команд не из webview показываем сообщение
        vscode.window.showInformationMessage('Autonomy agent is managed through the Autonomy panel. Please use the webview interface.');
    });

    const runTaskCommand = vscode.commands.registerCommand('autonomy.runTask', async (taskMessage?: string) => {
        // Теперь все задачи выполняются через webview
        let task = taskMessage;
        if (!task) {
            task = await vscode.window.showInputBox({
                prompt: 'Enter task for Autonomy agent',
                placeHolder: 'e.g., Add error handling to the getUserData function',
                ignoreFocusOut: true
            });
        }

        if (task) {
            // Передаем задачу в webview
            webviewProvider.handleTaskFromCommand(task);

            // Показываем webview
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
        console.log('Global config changed, restarting autonomy agent...');
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
                console.error('Failed to restart autonomy agent:', error);
                vscode.window.showErrorMessage(`Failed to restart Autonomy agent: ${error}`);
            }
        }
    });

    context.subscriptions.push(configWatcher);

    console.log('✅ Autonomy extension activation completed');
}

// быстрая проверка наличия autonomy без установки
async function quickCheckAutonomy(context: vscode.ExtensionContext): Promise<boolean> {
    console.log('quickCheckAutonomy: Quick check for autonomy...');
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
                console.log('quickCheckAutonomy: Autonomy not immediately available');
                resolve(false);
            } else {
                console.log('quickCheckAutonomy: Autonomy found:', stdout.trim());
                resolve(true);
            }
        });

        // Таймаут 3 секунды
        setTimeout(() => {
            if (!child.killed) {
                child.kill('SIGTERM');
                resolve(false);
            }
        }, 3000);
    });
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
            const checkOptions = {
                timeout: 5000, // 5 секунд таймаут
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
                    if (error.code === 'ENOENT' || error.signal === 'SIGTERM') {
                        console.log('checkIfAutonomyExists: Autonomy not found in PATH');
                    } else {
                        console.log('checkIfAutonomyExists: Command failed:', error.message);
                    }
                    resolve(false);
                } else {
                    console.log('checkIfAutonomyExists: Autonomy already installed:', stdout.trim());
                    resolve(true);
                }
            });

            // Принудительно завершаем через 5 секунд если команда зависла
            setTimeout(() => {
                if (!child.killed) {
                    console.log('checkIfAutonomyExists: Timeout reached, killing process');
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

                console.log(`Installing Autonomy CLI... (attempt ${attempt})`);

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
                        console.error(errorMsg);
                        console.error('stdout:', stdout);
                        console.error('stderr:', stderr);

                        // Send error to webview chat
                        if (webviewProvider) {
                            webviewProvider.sendAgentOutput(`❌ ${errorMsg}`, 'stderr');
                            if (stderr) {
                                webviewProvider.sendAgentOutput(`Error details: ${stderr}`, 'stderr');
                            }
                        }

                        if (attempt < maxAttempts) {
                            const retryMsg = `Retrying in ${backoffMs / 1000} seconds... (attempt ${attempt + 1})`;
                            console.log(retryMsg);
                            if (webviewProvider) {
                                webviewProvider.sendAgentOutput(retryMsg, 'stdout');
                            }
                            setTimeout(attemptInstall, backoffMs);
                        } else {
                            const failMsg = 'Autonomy CLI auto-installation failed after maximum attempts. Please install manually.';
                            console.error(failMsg);
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
                                console.error(verifyMsg);
                                if (webviewProvider) {
                                    webviewProvider.sendAgentOutput(`⚠️ ${verifyMsg}`, 'stderr');
                                }

                                if (attempt < maxAttempts) {
                                    const retryMsg = `Retrying verification in ${backoffMs / 1000} seconds...`;
                                    console.log(retryMsg);
                                    if (webviewProvider) {
                                        webviewProvider.sendAgentOutput(retryMsg, 'stdout');
                                    }
                                    setTimeout(attemptInstall, backoffMs);
                                } else {
                                    resolve(false);
                                }
                            } else {
                                const successMsg = `✅ Autonomy CLI installed successfully! Version: ${verifyStdout.trim()}`;
                                console.log(successMsg);
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
    console.log('extension: Deactivating Autonomy extension');

    // Останавливаем глобальный агент
    if (autonomyAgent) {
        console.log('extension: Stopping global autonomy agent');
        autonomyAgent.stop().catch(error => {
            console.error('extension: Error stopping global agent:', error);
        });
    }

    // Останавливаем агент в webview
    if (webviewProvider) {
        console.log('extension: Stopping webview autonomy agent');
        webviewProvider.cleanup().catch(error => {
            console.error('extension: Error cleaning up webview:', error);
        });
    }
}