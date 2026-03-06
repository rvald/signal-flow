#!/bin/sh
# Wait for Ollama to be available
echo "Waiting for Ollama service to start..."
while ! curl -s http://ollama:11434/api/tags > /dev/null; do
  sleep 2
done

echo "Ollama is up. Pulling required models..."
echo "Pulling gemma3:4b (for Intelligence Flash tier)..."
curl -s -X POST http://ollama:11434/api/pull -d '{"name": "gemma3:4b"}'
echo "\nPulling deepseek-r1:8b (for Intelligence Reasoning tier and Agent)..."
curl -s -X POST http://ollama:11434/api/pull -d '{"name": "deepseek-r1:8b"}'

echo "\nModel initialization complete."
