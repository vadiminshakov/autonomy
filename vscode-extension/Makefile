.PHONY: build install build-only help version

# Get version from package.json
VERSION := $(shell node -p "require('./package.json').version")
EXTENSION_NAME := autonomy-vscode-$(VERSION).vsix

# Default target
all: build

# Build and package VS Code extension
build:
	npm install
	npm run compile
	echo "y" | npm run package
	code --install-extension $(EXTENSION_NAME) --force
	@echo "✅ Extension built and installed successfully!"

# Just build without installing
build-only:
	npm install
	npm run compile
	echo "y" | npm run package

# Install existing package
install:
	code --install-extension $(EXTENSION_NAME) --force
	@echo "✅ Extension installed successfully!"

# Show version
version:
	@echo "Current version: $(VERSION)"
	@echo "Extension file: $(EXTENSION_NAME)"

# Show help
help:
	@echo "Available targets:"
	@echo "  build      - Build, package and install VS Code extension (default)"
	@echo "  build-only - Build and package without installing"
	@echo "  install    - Install existing .vsix package"
	@echo "  version    - Show current version from package.json"
	@echo "  help       - Show this help"
	@echo ""
	@echo "Current version: $(VERSION)"