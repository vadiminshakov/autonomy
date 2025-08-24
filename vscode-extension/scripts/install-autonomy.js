#!/usr/bin/env node

const { exec } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

function checkIfAutonomyExists() {
    return new Promise((resolve) => {
        exec('autonomy --version', (error, stdout, stderr) => {
            if (error) {
                resolve(false);
            } else {
                resolve(true);
            }
        });
    });
}

function installAutonomy() {
    return new Promise((resolve, reject) => {

        const platform = os.platform();
        let installCommand;

        switch (platform) {
            case 'darwin': // macOS
                installCommand = 'curl -sSL https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.sh | bash';
                break;
            case 'linux':
                installCommand = 'curl -sSL https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.sh | bash';
                break;
            case 'win32': // Windows
                installCommand = 'powershell -Command "iwr -Uri https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.ps1 | iex"';
                break;
            default:
                resolve(false);
                return;
        }

        exec(installCommand, { timeout: 60000 }, (error, stdout, stderr) => {
            if (error) {
                printManualInstructions();
                resolve(false);
            } else {
                resolve(true);
            }
        });
    });
}

function printManualInstructions() {

    // Manual installation instructions removed
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

async function main() {
    try {
        const autonomyExists = await checkIfAutonomyExists();

        if (!autonomyExists) {
            const installSuccess = await installAutonomy();

            if (!installSuccess) {
                return;
            }
        }

        createConfigExample();

        console.log('');
        console.log('🎉 Setup complete! You can now use the Autonomy VS Code extension.');
        console.log('🔧 Configure your API key in the extension settings or ~/.autonomy/config.json');
        console.log('');

    } catch (error) {
        console.error('❌ Installation script failed:', error);
        printManualInstructions();
    }
}

if (require.main === module) {
    main();
}