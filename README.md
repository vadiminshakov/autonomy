<img src="https://raw.githubusercontent.com/vadiminshakov/autonomy/main/vscode-extension/media/icon.png" alt="Autonomy Logo" width="128" height="128"> 

# Autonomy – Go AI Coding Agent

![Tests](https://github.com/vadiminshakov/autonomy/actions/workflows/test.yml/badge.svg)
![Build](https://github.com/vadiminshakov/autonomy/actions/workflows/release.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/vadiminshakov/autonomy)](https://goreportcard.com/report/github.com/vadiminshakov/autonomy)

An experimental AI coding agent written in Go. 

One of the key goals of this project is to support local models, providing developers with privacy-focused AI assistance that runs entirely on their own hardware.

![Demo](https://github.com/vadiminshakov/autonomy/releases/download/v0.0.0/demo.gif)

## Install as VSCode extension

[VSCode extension](https://marketplace.visualstudio.com/items?itemName=Autonomy.autonomy-vscode)

## Install as terminal agent

```bash
curl -sSL https://raw.githubusercontent.com/vadiminshakov/autonomy/main/install.sh | bash
```

Or with Go:

```bash
go install github.com/vadiminshakov/autonomy@latest
```

## Usage

Just run:

```bash
autonomy
```

On first launch the interactive wizard will guide you to:

1. Choose provider
2. Enter API key or use local mode
3. Choose model

## Contributing

Pull requests welcome.
