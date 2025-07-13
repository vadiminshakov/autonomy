# Autonomy – AI Coding Agent in Go

An experimental AI coding assistant written in Go.

## Prerequisites

* Either an OpenAI or Anthropic API key

## Running

1. Clone the repository and change into it:

   ```bash
   git clone https://github.com/vadiminshakov/autonomy.git
   cd autonomy
   ```

2. Export the API key for the provider you want to use:

   ```bash
   # OpenAI
   export OPENAI_API_KEY="your_openai_api_key"
   
   # —- or —-
   
   # Anthropic
   export ANTHROPIC_API_KEY="your_anthropic_api_key"
   ```

3. Start the agent (provider is detected automatically, or you can pass it explicitly):

   ```bash
   # automatic provider detection
   go run .

   # explicit provider selection
   go run . -provider openai   # or -provider anthropic
   ```

The application will launch an interactive REPL where you can give the agent tasks and observe its reasoning and code edits.

## Contributing

We welcome contributions of all kinds! If you have ideas, find a bug, or want to add a new feature, feel free to open an issue or submit a pull request.
