#!/bin/bash

# ============================================
# EVA-Code-Brain - Project Reorganization Script
# ============================================

echo "🔧 Reorganizing EVA-Code-Brain project structure..."
echo ""

# Move main application
echo "📦 Moving main application..."
mv main.go cmd/eva-code-brain/ 2>/dev/null || echo "  main.go already in place"

# Move internal packages
echo "📦 Moving internal packages..."
mv agents.go internal/agents/ 2>/dev/null || echo "  agents.go already in place"
mv chunker.go internal/chunker/ 2>/dev/null || echo "  chunker.go already in place"
mv graph/neo4j_client.go internal/graph/ 2>/dev/null || echo "  neo4j_client.go already in place"
mv indexer.go internal/indexer/ 2>/dev/null || echo "  indexer.go already in place"
mv memory.go internal/memory/ 2>/dev/null || echo "  memory.go already in place"
mv code_quality.go internal/quality/ 2>/dev/null || echo "  code_quality.go already in place"
mv search.go reranking.go internal/search/ 2>/dev/null || echo "  search files already in place"
mv tools.go git_tools.go quality_tools.go health_tools.go test_tools.go internal/tools/ 2>/dev/null || echo "  tools already in place"

# Move LLM clients
echo "📦 Moving LLM clients..."
mv ollama_client.go openai.go anthropic.go internal/llm/ 2>/dev/null || echo "  LLM clients already in place"

# Move migrations
echo "📦 Moving migrations..."
mv schema.sql v*.sql fix_memory_schema.sql migrations/ 2>/dev/null || echo "  migrations already in place"

# Move scripts
echo "📦 Moving scripts..."
mv deploy.sh index_all.sh migrate.sh setup.sh install.sh log.sh test_models.sh scripts/ 2>/dev/null || echo "  scripts already in place"

# Move web files
echo "📦 Moving web files..."
mv index.html pcm-processor.js web/ 2>/dev/null || echo "  web files already in place"

# Move test files
echo "📦 Moving test files..."
mv *_test.go test_*.go cmd/eva-code-brain/ 2>/dev/null || echo "  test files already in place"

# Create .gitignore if not exists
if [ ! -f .gitignore ]; then
    echo "📝 Creating .gitignore..."
    cat > .gitignore << 'EOF'
# Binaries
*.exe
*.dll
*.so
*.dylib
eva-code-brain
codebrain

# Test binary
*.test

# Output
*.out

# Dependency directories
vendor/

# Go workspace file
go.work

# Environment
.env
.env.local

# IDE
.vscode/
.idea/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Logs
*.log

# Database
*.db
*.sqlite

# Images (keep in assets/)
*.png
*.jpg
*.jpeg
!assets/*.png
!assets/*.jpg
EOF
fi

# Create assets directory
echo "📁 Creating assets directory..."
mkdir -p assets
mv eva.png jumento.png assets/ 2>/dev/null || echo "  images already in assets"

# Make scripts executable
echo "🔐 Making scripts executable..."
chmod +x scripts/*.sh

echo ""
echo "✅ Project reorganization complete!"
echo ""
echo "📁 New structure:"
echo "  cmd/eva-code-brain/    - Main application"
echo "  internal/              - Internal packages"
echo "  migrations/            - Database migrations"
echo "  scripts/               - Utility scripts"
echo "  web/                   - Web interface"
echo "  assets/                - Images and resources"
echo ""
echo "🚀 Next steps:"
echo "  1. Review the new structure"
echo "  2. Update import paths in Go files"
echo "  3. Run: go mod tidy"
echo "  4. Test: go build ./cmd/eva-code-brain"
echo ""
