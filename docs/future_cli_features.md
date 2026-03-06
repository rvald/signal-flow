# Future CLI Features: Setup Wizard & Hydration Progress

This document outlines the planned implementation for improving the user experience around LLM provider selection and model hydration, specifically focusing on the first-time setup flow.

## 1. Interactive Setup Wizard (`signal-flow check` or `signal-flow enable`)
Currently, users are expected to manually edit `~/.config/signal-flow/pipeline.yaml` or provide CLI flags. We will implement an interactive setup wizard using a library like `charmbracelet/huh` or `AlecAivazis/survey`.

### Planned Flow:
1. **Provider Selection**: "Which LLM provider would you like to use for the intelligence pipeline?"
   - Choices: `[Ollama (Local), Gemini, Claude, OpenAI]`
2. **Model Selection (If Ollama)**: "Which models would you like to use?"
   - Flash Tier (Analysis): `[gemma3:4b (Default), llama3.2, qwen2.5:3b]`
   - Reasoning Tier (Distillation): `[deepseek-r1:8b (Default), mixtral:8x7b]`
3. **Configuration Generation**: Write these selections automatically to `~/.config/signal-flow/pipeline.yaml`.

## 2. Model Hydration & Progress Indicators
When Ollama is selected as the provider, the CLI should proactively check if the selected models exist locally before executing the pipeline or chat bot. If they do not exist, the CLI should pull them and display a progress bar.

### Implementation Strategy:
1. **API Integration**: Utilize the Ollama API directly from the Go CLI rather than relying solely on the background bash script (`init-ollama.sh`).
   - Endpoint: `GET /api/tags` (Check if model exists)
   - Endpoint: `POST /api/pull` (Trigger download streams)
2. **Progress UI**: Use a library like `charmbracelet/bubbles/progress` or `schollz/progressbar` to stream the JSON lines from the `/api/pull` endpoint.
   - The `/api/pull` endpoint returns `{"status": "downloading digestname", "total": 1000, "completed": 500}`.
   - Parse this streaming JSON response to update the CLI progress bar visually.
3. **Graceful Fallback**: If the `ollama` daemon is not running when the CLI attempts this, provide a friendly error instructing the user to start Docker or the Ollama app.

## 3. Implementation Steps
- [ ] Research and integrate a terminal UI library for progress bars.
- [ ] Implement `checkOllamaModels(models []string)` function to hit `/api/tags`.
- [ ] Implement `pullOllamaModel(model string)` function with streaming JSON parser.
- [ ] Hook the hydration check into the pre-run phase of the `pipeline run` and `bot start` commands.
- [ ] Build the interactive survey for `signal-flow setup`.
