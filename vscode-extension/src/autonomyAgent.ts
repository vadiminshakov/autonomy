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
    skipExecutableValidation?: boolean;
}

export class AutonomyAgent {
    private process: ChildProcess | undefined;
    private outputChannel: vscode.OutputChannel;
    private isRunningFlag = false;
    private currentTask: TaskItem | undefined;
    private outputCallback?: (output: string, type: 'stdout' | 'stderr') => void;
    private isWebviewMode = false;

    constructor(
        private config: AutonomyConfig,
        private taskProvider: AutonomyTaskProvider
    ) {
        this.outputChannel = vscode.window.createOutputChannel('Autonomy Agent');
    }

    setOutputCallback(callback: (output: string, type: 'stdout' | 'stderr') => void) {
        this.outputCallback = callback;
    }

    setWebviewMode(enabled: boolean) {
        this.isWebviewMode = enabled;
        if (enabled) {
            console.log('autonomyAgent: Webview mode enabled - terminal output disabled');
            // Hide the output channel when in webview mode
            this.outputChannel.hide();
        } else {
            console.log('autonomyAgent: Webview mode disabled - terminal output enabled');
        }
    }

    async start(): Promise<void> {
        if (this.isRunningFlag) {
            throw new Error('Agent is already running');
        }

        try {
            if (!this.config.skipExecutableValidation) {
                await this.validateExecutable();
            } else {
                console.log('autonomyAgent: Skipping executable validation (as configured)');
                if (!this.isWebviewMode) {
                    this.outputChannel.appendLine('Skipping executable validation (as configured in settings)');
                }
            }

            await this.createConfigFile();

            if (!this.isWebviewMode) {
                this.outputChannel.show();
                this.outputChannel.appendLine('Starting Autonomy agent...');
            } else {
                // Ensure output channel is hidden in webview mode
                this.outputChannel.hide();
            }

            this.process = spawn(this.config.executablePath, ['--headless'], {
                cwd: vscode.workspace.workspaceFolders?.[0]?.uri.fsPath || process.cwd(),
                stdio: ['pipe', 'pipe', 'pipe']
            });

            this.setupProcessHandlers();
            this.isRunningFlag = true;

            await new Promise(resolve => setTimeout(resolve, 1000));

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
        console.log(`autonomyAgent: Validating executable: ${this.config.executablePath}`);
        
        return new Promise((resolve, reject) => {
            const testProcess = spawn(this.config.executablePath, ['--version'], {
                stdio: 'pipe'
            });

            const timeout = setTimeout(() => {
                testProcess.kill();
                reject(new Error(`Timeout validating autonomy executable. Please ensure the 'autonomy' executable is installed and available in your PATH, or configure the correct path in settings.`));
            }, 3000);

            testProcess.on('error', (error) => {
                clearTimeout(timeout);
                console.error(`autonomyAgent: Executable validation error:`, error);
                
                let errorMsg = `Cannot find autonomy executable at "${this.config.executablePath}". `;
                if (error.message.includes('ENOENT')) {
                    errorMsg += 'Please install autonomy or configure the correct path in extension settings.';
                } else {
                    errorMsg += `Error: ${error.message}`;
                }
                
                reject(new Error(errorMsg));
            });

            testProcess.on('close', (code) => {
                clearTimeout(timeout);
                console.log(`autonomyAgent: Executable validation completed with code: ${code}`);
                if (code === 0) {
                    resolve();
                } else if (code === null) {
                    reject(new Error(`Autonomy executable validation timed out. Please check if the executable is working correctly.`));
                } else {
                    reject(new Error(`Autonomy executable test failed with exit code ${code}. Please check your installation.`));
                }
            });
        });
    }

    private async createConfigFile(): Promise<void> {
        console.log('autonomyAgent: Checking for global config...');
        
        const globalConfigPath = require('path').join(require('os').homedir(), '.autonomy', 'config.json');
        
        if (require('fs').existsSync(globalConfigPath)) {
            console.log('autonomyAgent: Using global config from ~/.autonomy/config.json');
            if (!this.isWebviewMode) {
                this.outputChannel.appendLine('Using global configuration from ~/.autonomy/config.json');
            }
            return;
        }
        
        console.log('autonomyAgent: No global config found, creating local config...');
        
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
            console.log('autonomyAgent: Created .autonomy directory');

            const configContent = JSON.stringify(config, null, 2);
            await vscode.workspace.fs.writeFile(configPath, Buffer.from(configContent, 'utf8'));
            
            console.log(`autonomyAgent: Configuration written to ${configPath.fsPath}`);
            if (!this.isWebviewMode) {
                this.outputChannel.appendLine(`Configuration written to ${configPath.fsPath}`);
            }
        } catch (error) {
            console.error('autonomyAgent: Error creating config file:', error);
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
            
            if (this.outputCallback) {
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
            }
            
            if (this.currentTask && this.currentTask.status === 'running') {
                this.currentTask.status = 'failed';
                this.taskProvider.refresh();
            }
        });
    }

    private parseOutput(output: string): void {
        const completionPatterns = [
            /Task\s+completed\s+successfully\.?\s*$/i,
            /✅\s*Task\s+completed/i,
            /✅\s*All\s+done/i,
            /^✅/m
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
        } else if (isTaskFailed && this.currentTask && this.currentTask.status === 'running') {
            this.currentTask.status = 'failed';
            this.taskProvider.refresh();
        } else if (output.includes('📋 Tool')) {
            const toolMatch = output.match(/📋 Tool (\w+)/);
            if (toolMatch && this.currentTask) {
                this.currentTask.tooltip = `Running tool: ${toolMatch[1]}`;
                this.taskProvider.refresh();
            }
        }
    }

    getOutputChannel(): vscode.OutputChannel {
        return this.outputChannel;
    }
}