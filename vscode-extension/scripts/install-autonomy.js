#!/usr/bin/env node

const { exec } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

console.log('🤖 Autonomy VS Code Extension - Auto Installation');
console.log('================================================');

function checkIfAutonomyExists() {
    return new Promise((resolve) => {
        exec('autonomy --version', (error, stdout, stderr) => {
            if (error) {
                resolve(false);
            } else {
                console.log('✅ Autonomy already installed:', stdout.trim());
                resolve(true);
            }
        });
    });
}

function installAutonomy() {
    return new Promise((resolve, reject) => {
        console.log('📦 Installing Autonomy CLI...');
        
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
                console.log('❌ Unsupported platform:', platform);
                console.log('Please install Autonomy manually from: https://github.com/vadiminshakov/autonomy');
                resolve(false);
                return;
        }
        
        console.log('🔄 Running installation command...');
        console.log('Command:', installCommand);
        
        exec(installCommand, { timeout: 60000 }, (error, stdout, stderr) => {
            if (error) {
                console.log('❌ Auto-installation failed:', error.message);
                console.log('📖 Please install manually using instructions below:');
                printManualInstructions();
                resolve(false);
            } else {
                console.log('✅ Autonomy installed successfully!');
                console.log(stdout);
                resolve(true);
            }
        });
    });
}

function printManualInstructions() {
    console.log('');
    console.log('📋 Manual Installation Instructions:');
    console.log('===================================');
    
    const platform = os.platform();
    
    switch (platform) {
        case 'darwin':
            console.log('macOS:');
            console.log('  curl -sSL https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.sh | bash');
            console.log('');
            console.log('Or with Homebrew (if available):');
            console.log('  brew install vadiminshakov/autonomy/autonomy');
            break;
            
        case 'linux':
            console.log('Linux:');
            console.log('  curl -sSL https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.sh | bash');
            console.log('');
            console.log('Or download binary from:');
            console.log('  https://github.com/vadiminshakov/autonomy/releases');
            break;
            
        case 'win32':
            console.log('Windows:');
            console.log('  powershell -Command "iwr -Uri https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.ps1 | iex"');
            console.log('');
            console.log('Or download .exe from:');
            console.log('  https://github.com/vadiminshakov/autonomy/releases');
            break;
    }
    
    console.log('');
    console.log('📖 For more details visit: https://github.com/vadiminshakov/autonomy');
}

function createConfigExample() {
    const configDir = path.join(os.homedir(), '.autonomy');
    const configFile = path.join(configDir, 'config.json');
    
    if (!fs.existsSync(configFile)) {
        console.log('📝 Creating example configuration...');
        
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
            console.log('✅ Example config created at:', configFile);
            console.log('💡 Don\'t forget to add your API key!');
        } catch (error) {
            console.log('⚠️  Could not create config file:', error.message);
        }
    }
}

async function main() {
    try {
        const autonomyExists = await checkIfAutonomyExists();
        
        if (!autonomyExists) {
            console.log('⚠️  Autonomy CLI not found, attempting installation...');
            const installSuccess = await installAutonomy();
            
            if (!installSuccess) {
                console.log('');
                console.log('⚠️  Auto-installation was not successful.');
                console.log('📋 Please follow the manual installation steps above.');
                console.log('');
                console.log('🔧 After installation, restart VS Code and configure the extension.');
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