import * as vscode from 'vscode';

export class TaskItem extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public status: 'pending' | 'running' | 'completed' | 'failed',
        public readonly collapsibleState: vscode.TreeItemCollapsibleState,
        public readonly command?: vscode.Command
    ) {
        super(label, collapsibleState);
        
        this.tooltip = `${label} - ${status}`;
        this.description = status;
        
        switch (status) {
            case 'pending':
                this.iconPath = new vscode.ThemeIcon('clock');
                break;
            case 'running':
                this.iconPath = new vscode.ThemeIcon('loading~spin');
                break;
            case 'completed':
                this.iconPath = new vscode.ThemeIcon('check', new vscode.ThemeColor('terminal.ansiGreen'));
                break;
            case 'failed':
                this.iconPath = new vscode.ThemeIcon('error', new vscode.ThemeColor('terminal.ansiRed'));
                break;
        }
    }
}

export class AutonomyTaskProvider implements vscode.TreeDataProvider<TaskItem> {
    private _onDidChangeTreeData: vscode.EventEmitter<TaskItem | undefined | null | void> = new vscode.EventEmitter<TaskItem | undefined | null | void>();
    readonly onDidChangeTreeData: vscode.Event<TaskItem | undefined | null | void> = this._onDidChangeTreeData.event;

    private tasks: TaskItem[] = [];
    private maxTasks = 50;

    constructor() {}

    refresh(): void {
        this._onDidChangeTreeData.fire();
    }

    getTreeItem(element: TaskItem): vscode.TreeItem {
        return element;
    }

    getChildren(element?: TaskItem): Thenable<TaskItem[]> {
        if (!element) {
            return Promise.resolve(this.tasks);
        }
        
        return Promise.resolve([]);
    }

    addTask(task: TaskItem): void {
        this.tasks.unshift(task);
        
        if (this.tasks.length > this.maxTasks) {
            this.tasks = this.tasks.slice(0, this.maxTasks);
        }
        
        this.refresh();
    }

    updateTaskStatus(taskLabel: string, status: 'pending' | 'running' | 'completed' | 'failed'): void {
        const task = this.tasks.find(t => t.label === taskLabel);
        if (task) {
            task.status = status;
            task.description = status;
            
            switch (status) {
                case 'pending':
                    task.iconPath = new vscode.ThemeIcon('clock');
                    break;
                case 'running':
                    task.iconPath = new vscode.ThemeIcon('loading~spin');
                    break;
                case 'completed':
                    task.iconPath = new vscode.ThemeIcon('check', new vscode.ThemeColor('terminal.ansiGreen'));
                    break;
                case 'failed':
                    task.iconPath = new vscode.ThemeIcon('error', new vscode.ThemeColor('terminal.ansiRed'));
                    break;
            }
            
            this.refresh();
        }
    }

    clearTasks(): void {
        this.tasks = [];
        this.refresh();
    }

    getRunningTasks(): TaskItem[] {
        return this.tasks.filter(task => task.status === 'running');
    }

    getCompletedTasks(): TaskItem[] {
        return this.tasks.filter(task => task.status === 'completed');
    }

    getFailedTasks(): TaskItem[] {
        return this.tasks.filter(task => task.status === 'failed');
    }

    getTaskHistory(): TaskItem[] {
        return [...this.tasks];
    }
}