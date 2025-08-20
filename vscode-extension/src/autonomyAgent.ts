import * as vscode from 'vscode';
import { spawn, ChildProcess } from 'child_process';
import { AutonomyTaskProvider, TaskItem } from './taskProvider';

export interface AutonomyConfig {
    executablePath: string;
    provider: string;
    model: string;
    apiKey: string;
    baseURL?: string;
    maxIterations: number;
    enableReflection: boolean;
    maxTokens?: number;
    temperature?: number;
}

export class AutonomyAgent {
    private process: ChildProcess | undefined;
    private outputChannel: vscode.OutputChannel;
    private isRunningFlag = false;
    private currentTask: TaskItem | undefined;
    private outputCallback?: (output: string, type: 'stdout' | 'stderr' | 'task_status') => void;
    private isWebviewMode = false;

    constructor(
        private config: AutonomyConfig,
        private taskProvider: AutonomyTaskProvider
    ) {
        this.outputChannel = vscode.window.createOutputChannel('Autonomy Agent');
    }

    setOutputCallback(callback: (output: string, type: 'stdout' | 'stderr' | 'task_status') => void) {
        this.outputCallback = callback;
    }

    setWebviewMode(enabled: boolean) {
        this.isWebviewMode = enabled;
        if (enabled) {
            this.outputChannel.hide();
        }
    }

    async start(): Promise<void> {
        if (this.isRunningFlag) {
            throw new Error('Agent is already running');
        }

        try {
            await this.validateExecutable();

            await this.createConfigFile();

            if (!this.isWebviewMode) {
                this.outputChannel.show();
                this.outputChannel.appendLine('Starting Autonomy agent...');
            } else {
                this.outputChannel.hide();
            }

            const workingDir = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || process.cwd();
            
            const env = {
                ...process.env,
                VSCODE_PID: process.pid.toString(),
                VSCODE_WORKSPACE: workingDir,
                AUTONOMY_PARENT: 'vscode'
            };
            
            
            try {
                this.process = spawn(this.config.executablePath, ['--headless'], {
                    cwd: workingDir,
                    stdio: ['pipe', 'pipe', 'pipe'],
                    env: env
                });
            } catch (spawnError) {
                throw new Error(`Failed to spawn autonomy process: ${spawnError}`);
            }

            this.isRunningFlag = true;

            if (this.process.stdin) {
                this.process.stdin.write('\n');
            }

            const readyPromise = new Promise<void>((resolve, reject) => {
                let outputReceived = false;
                
                const statusInterval = setInterval(() => {
                    if (this.process) {
                    }
                }, 500);
                
                const timeout = setTimeout(() => {
                    clearInterval(statusInterval);
                    reject(new Error(`Timeout waiting for agent ready signal after 5 seconds. OutputReceived: ${outputReceived}. Please check your configuration and ensure the autonomy binary is working correctly.`));
                }, 5000);

                const fallbackTimeout = setTimeout(() => {
                    if (outputReceived && this.process && !this.process.killed) {
                        clearTimeout(timeout);
                        clearInterval(statusInterval);
                        this.process?.stdout?.removeListener('data', onStdout);
                        this.process?.stderr?.removeListener('data', onStderr);
                        resolve();
                    }
                }, 3000);

                const onStdout = (data: Buffer) => {
                    const output = data.toString();
                    outputReceived = true;
                    
                    const lowerOutput = output.toLowerCase();
                    
                    if (lowerOutput.includes('autonomy agent is ready') || 
                        lowerOutput.includes('ready') || 
                        lowerOutput.includes('listening') ||
                        lowerOutput.includes('started') ||
                        lowerOutput.includes('enter your task') ||
                        lowerOutput.includes('what would you like me to help with')) {
                        clearTimeout(timeout);
                        clearTimeout(fallbackTimeout);
                        clearInterval(statusInterval);
                        this.process?.stdout?.removeListener('data', onStdout);
                        this.process?.stderr?.removeListener('data', onStderr);
                        resolve();
                    }
                };

                const onStderr = (data: Buffer) => {
                    const error = data.toString();
                    outputReceived = true;
                    
                    const lowerError = error.toLowerCase();
                    
                    if (lowerError.includes('fatal') || 
                        lowerError.includes('panic') ||
                        lowerError.includes('error: invalid api key') ||
                        lowerError.includes('invalid api key') ||
                        lowerError.includes('config not found') ||
                        lowerError.includes('failed to connect') ||
                        lowerError.includes('no such file') ||
                        lowerError.includes('permission denied')) {
                        clearTimeout(timeout);
                        clearTimeout(fallbackTimeout);
                        clearInterval(statusInterval);
                        this.process?.stdout?.removeListener('data', onStdout);
                        this.process?.stderr?.removeListener('data', onStderr);
                        reject(new Error(`Agent startup failed: ${error.trim()}`));
                    }
                };

                this.process?.stdout?.on('data', onStdout);
                this.process?.stderr?.on('data', onStderr);
            });

            try {
                await readyPromise;
                
                this.setupProcessHandlers();
            } catch (error) {
                throw error;
            }

            if (!this.process || this.process.killed) {
                throw new Error('Failed to start autonomy process');
            }

        } catch (error) {
            this.isRunningFlag = false;
            throw error;
        }
    }

    async stop(): Promise<void> {
        if (this.process && !this.process.killed) {
            this.process.kill('SIGTERM');

            await new Promise(resolve => setTimeout(resolve, 2000));

            if (!this.process.killed) {
                this.process.kill('SIGKILL');
            }
        }

        this.isRunningFlag = false;
        this.process = undefined;

        if (!this.isWebviewMode) {
            this.outputChannel.appendLine('Autonomy agent stopped');
        }
    }

    isRunning(): boolean {
        return this.isRunningFlag && this.process !== undefined && !this.process.killed;
    }

    async runTask(taskDescription: string): Promise<void> {
        if (!this.isRunning()) {
            throw new Error('Agent is not running');
        }

        this.currentTask = new TaskItem(
            taskDescription,
            'running',
            vscode.TreeItemCollapsibleState.None
        );

        this.taskProvider.addTask(this.currentTask);

        if (!this.isWebviewMode) {
            this.outputChannel.appendLine(`\n--- Running task: ${taskDescription} ---`);
        }

        try {
            if (this.process && this.process.stdin) {
                this.process.stdin.write(taskDescription + '\n');

                this.currentTask.status = 'running';
                this.taskProvider.refresh();
            }
        } catch (error) {
            if (!this.isWebviewMode) {
                this.outputChannel.appendLine(`Error running task: ${error}`);
            }
            if (this.currentTask) {
                this.currentTask.status = 'failed';
                this.taskProvider.refresh();
            }
            throw error;
        }
    }

    private async validateExecutable(): Promise<void> {
        const fs = require('fs');
        const autonomyPath = `${process.env.HOME}/.local/bin/autonomy`;

        try {
            if (fs.existsSync(autonomyPath) && fs.statSync(autonomyPath).isFile()) {
                this.config.executablePath = autonomyPath;
                return Promise.resolve();
            } else {
                throw new Error(`Cannot find autonomy executable at ${autonomyPath}. Please run 'make install' in the autonomy project directory.`);
            }
        } catch (error) {
            return Promise.reject(error);
        }
    }

    private async createConfigFile(): Promise<void> {
        const globalConfigPath = require('path').join(require('os').homedir(), '.autonomy', 'config.json');

        if (require('fs').existsSync(globalConfigPath)) {
            if (!this.isWebviewMode) {
                this.outputChannel.appendLine('Using global configuration from ~/.autonomy/config.json');
            }
            return;
        }

        const workspaceFolder = vscode.workspace.workspaceFolders?.[0];
        if (!workspaceFolder) {
            throw new Error('No workspace folder found');
        }

        const configPath = vscode.Uri.joinPath(workspaceFolder.uri, '.autonomy', 'config.json');

        const config = {
            provider: this.config.provider,
            model: this.config.model,
            api_key: this.config.apiKey,
            base_url: this.config.baseURL || "",
            max_iterations: this.config.maxIterations,
            enable_reflection: this.config.enableReflection
        };

        try {
            const autonomyDir = vscode.Uri.joinPath(workspaceFolder.uri, '.autonomy');
            await vscode.workspace.fs.createDirectory(autonomyDir);

            const configContent = JSON.stringify(config, null, 2);
            await vscode.workspace.fs.writeFile(configPath, Buffer.from(configContent, 'utf8'));

            if (!this.isWebviewMode) {
                this.outputChannel.appendLine(`Configuration written to ${configPath.fsPath}`);
            }
        } catch (error) {
            throw new Error(`Failed to create config file: ${error}`);
        }
    }

    private setupProcessHandlers(): void {
        if (!this.process) {
            return;
        }

        this.process.stdout?.on('data', (data: Buffer) => {
            const output = data.toString();

            if (!this.isWebviewMode) {
                this.outputChannel.append(output);
            }

            this.parseOutput(output);

            if (this.outputCallback) {
                this.outputCallback(output, 'stdout');
            }
        });

        this.process.stderr?.on('data', (data: Buffer) => {
            const error = data.toString();

            if (!this.isWebviewMode) {
                this.outputChannel.append(`ERROR: ${error}`);
            }

            if (this.outputCallback && this.isActualError(error)) {
                this.outputCallback(`ERROR: ${error}`, 'stderr');
            }
        });

        this.process.on('close', (code, signal) => {
            this.isRunningFlag = false;

            if (!this.isWebviewMode) {
                this.outputChannel.appendLine(`\nAutonomy process exited with code ${code}, signal ${signal}`);
            }
        });

        this.process.on('error', (error) => {
            this.isRunningFlag = false;

            if (!this.isWebviewMode) {
                this.outputChannel.appendLine(`Process error: ${error.message}`);
            } else {
                if (this.outputCallback) {
                    this.outputCallback(`process error: ${error.message}`, 'stderr');
                }
            }

            if (this.currentTask && this.currentTask.status === 'running') {
                this.currentTask.status = 'failed';
                this.taskProvider.refresh();
            }

            this.cleanup();
        });
    }

    private parseOutput(output: string): void {
        const completionPatterns = [
            /Task\s+completed\s+successfully/i,
            /✅\s*Task\s+completed/i,
            /✅\s*All\s+done/i,
            /^✅/m,
            /Done\s+attempt_completion/i,
            /🎉\s*Task\s+completed/i
        ];

        const failurePatterns = [
            /Task\s+failed/i,
            /❌\s*Task/i,
            /Error:\s+Task/i
        ];

        const isTaskCompleted = completionPatterns.some(pattern => pattern.test(output));
        const isTaskFailed = failurePatterns.some(pattern => pattern.test(output));

        if (isTaskCompleted && this.currentTask && this.currentTask.status === 'running') {
            this.currentTask.status = 'completed';
            this.taskProvider.refresh();
            // Notify webview to hide thinking indicator on task completion
            if (this.outputCallback) {
                this.outputCallback('TASK_COMPLETED', 'task_status');
            }
        } else if (isTaskFailed && this.currentTask && this.currentTask.status === 'running') {
            this.currentTask.status = 'failed';
            this.taskProvider.refresh();
            // Notify webview to hide thinking indicator on task failure
            if (this.outputCallback) {
                this.outputCallback('TASK_FAILED', 'task_status');
            }
        } else if (output.includes('Tool')) {
            const cleanOutput = output.replace(/[\u00A0-\u9999<>\&]/gim, '').replace(/\uFFFD/g, '');
            const toolMatch = cleanOutput.match(/Tool (\w+)/);
            if (toolMatch && this.currentTask) {
                this.currentTask.tooltip = `Running tool: ${toolMatch[1]}`;
                this.taskProvider.refresh();
            }
        }
    }

    private isActualError(stderr: string): boolean {
        const lowerStderr = stderr.toLowerCase();

        if (lowerStderr.includes('task iteration')) return false;
        if (lowerStderr.includes('=== task iteration')) return false;
        if (lowerStderr.includes('ai requested tools')) return false;
        if (lowerStderr.includes('info:')) return false;
        if (lowerStderr.includes('debug:')) return false;
        if (lowerStderr.includes('log:')) return false;

        return lowerStderr.includes('error:') ||
            lowerStderr.includes('failed') ||
            lowerStderr.includes('panic') ||
            lowerStderr.includes('fatal') ||
            lowerStderr.includes('warning:') ||
            lowerStderr.includes('warn:');
    }

    getOutputChannel(): vscode.OutputChannel {
        return this.outputChannel;
    }

    private cleanup(): void {
        if (this.process) {
            try {
                if (!this.process.killed) {
                    this.process.kill('SIGTERM');

                    setTimeout(() => {
                        if (this.process && !this.process.killed) {
                            this.process.kill('SIGKILL');
                        }
                    }, 5000);
                }
            } catch (error) {
            }
        }

        this.isRunningFlag = false;
        this.process = undefined;
    }
}