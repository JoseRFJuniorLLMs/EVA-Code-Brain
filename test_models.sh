#!/bin/bash
# Test script to verify model selection

echo "Testing model selection..."
echo ""

echo "1. Testing with model=gpt4"
curl -X POST http://localhost:666/chat \
  -H "Content-Type: application/json" \
  -d '{"question":"teste","model":"gpt4"}' \
  2>/dev/null | head -c 200
echo ""
echo ""

echo "2. Testing with model=grok"
curl -X POST http://localhost:666/chat \
  -H "Content-Type: application/json" \
  -d '{"question":"teste","model":"grok"}' \
  2>/dev/null | head -c 200
echo ""
echo ""

echo "3. Testing with model=auto"
curl -X POST http://localhost:666/chat \
  -H "Content-Type: application/json" \
  -d '{"question":"teste","model":"auto"}' \
  2>/dev/null | head -c 200
echo ""
