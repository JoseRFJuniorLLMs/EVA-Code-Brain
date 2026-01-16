package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// Chunk represents a semantic unit of code
type Chunk struct {
	Type      string // "function", "method", "type", "import", "generic"
	Name      string // Function/type name
	Content   string // Full code
	Context   string // Surrounding context (imports, package)
	StartLine int
	EndLine   int
}

// SemanticChunker handles splitting code into semantic chunks
type SemanticChunker struct{}

func NewSemanticChunker() *SemanticChunker {
	return &SemanticChunker{}
}

// Chunk splits content based on file extension
func (sc *SemanticChunker) Chunk(filePath string, content string) ([]Chunk, error) {
	ext := strings.ToLower(filePath)
	if strings.HasSuffix(ext, ".go") {
		return sc.chunkGo(content)
	}
	if strings.HasSuffix(ext, ".py") {
		return sc.chunkPython(content)
	}
	if strings.HasSuffix(ext, ".sql") {
		return sc.chunkSQL(content)
	}
	return sc.chunkGeneric(content)
}

// chunkGo parses Go code using AST
func (sc *SemanticChunker) chunkGo(content string) ([]Chunk, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "", content, parser.ParseComments)
	if err != nil {
		// Fallback to generic if parse fails (snippet might be incomplete)
		return sc.chunkGeneric(content)
	}

	var chunks []Chunk

	// Extract package and imports for context
	pkgName := node.Name.Name
	var imports []string
	for _, imp := range node.Imports {
		imports = append(imports, fmt.Sprintf("%s", imp.Path.Value))
	}
	contextStr := fmt.Sprintf("package %s\nimport (%s)", pkgName, strings.Join(imports, ", "))

	// Traverse declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Function or Method
			start := fset.Position(d.Pos()).Line
			end := fset.Position(d.End()).Line

			// Extract content exactly including comments
			code := extractLines(content, start, end)

			name := d.Name.Name
			chunkType := "function"
			if d.Recv != nil && len(d.Recv.List) > 0 {
				chunkType = "method"
				// Capture receiver name
				if t, ok := d.Recv.List[0].Type.(*ast.StarExpr); ok {
					if id, ok := t.X.(*ast.Ident); ok {
						name = id.Name + "." + name
					}
				} else if id, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}

			chunks = append(chunks, Chunk{
				Type:      chunkType,
				Name:      name,
				Content:   code,
				Context:   contextStr,
				StartLine: start,
				EndLine:   end,
			})

		case *ast.GenDecl:
			// Types, Consts, Vars
			if d.Tok == token.TYPE {
				start := fset.Position(d.Pos()).Line
				end := fset.Position(d.End()).Line
				code := extractLines(content, start, end)

				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						chunks = append(chunks, Chunk{
							Type:      "type",
							Name:      ts.Name.Name,
							Content:   code,
							Context:   contextStr,
							StartLine: start,
							EndLine:   end,
						})
					}
				}
			}
		}
	}

	// If no significant chunks found (e.g. only main with no funcs or just vars),
	// or if the file was parsed but yielded 0 chunks, fallback to generic
	if len(chunks) == 0 {
		return sc.chunkGeneric(content)
	}

	return chunks, nil
}

// chunkPython extracts classes and functions using indentation/regex
func (sc *SemanticChunker) chunkPython(content string) ([]Chunk, error) {
	lines := strings.Split(content, "\n")
	var chunks []Chunk

	var currentName string
	var currentType string
	var startLine int
	var chunkLines []string
	indentBase := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if len(chunkLines) > 0 {
				chunkLines = append(chunkLines, line)
			}
			continue
		}

		// Detect Start (Class or Def)
		if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") {
			// Check indentation
			currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))

			// If we were already in a chunk, decide if we should finish it
			// Higher level or equal level indent usually means new block
			if len(chunkLines) > 0 && currentIndent <= indentBase {
				chunks = append(chunks, Chunk{
					Type:      currentType,
					Name:      currentName,
					Content:   strings.Join(chunkLines, "\n"),
					StartLine: startLine,
					EndLine:   i,
				})
				chunkLines = nil
				indentBase = -1
			}

			if indentBase == -1 {
				startLine = i + 1
				indentBase = currentIndent
				if strings.HasPrefix(trimmed, "def ") {
					currentType = "function"
					currentName = strings.TrimPrefix(trimmed, "def ")
				} else {
					currentType = "class"
					currentName = strings.TrimPrefix(trimmed, "class ")
				}
				currentName = strings.Split(currentName, "(")[0]
				currentName = strings.TrimSuffix(currentName, ":")
			}
		}

		if indentBase != -1 {
			chunkLines = append(chunkLines, line)
		} else {
			// Top-level code not inside class/func goes to generic later?
			// For now, let's just use generic for small files or top-level scripts
		}
	}

	// Final chunk
	if len(chunkLines) > 0 {
		chunks = append(chunks, Chunk{
			Type:      currentType,
			Name:      currentName,
			Content:   strings.Join(chunkLines, "\n"),
			StartLine: startLine,
			EndLine:   len(lines),
		})
	}

	if len(chunks) == 0 {
		return sc.chunkGeneric(content)
	}
	return chunks, nil
}

// chunkSQL extracts tables, functions and triggers
func (sc *SemanticChunker) chunkSQL(content string) ([]Chunk, error) {
	// Simple SQL splitter by semicolon and common keywords
	var chunks []Chunk
	parts := strings.Split(content, ";")
	lineOffset := 1

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		numLines := strings.Count(part, "\n")

		if trimmed == "" {
			lineOffset += numLines
			continue
		}

		upper := strings.ToUpper(trimmed)

		// OPTIMIZATION: Ignore Data Dumps to prevent context window overflow
		// We only want schema definitions (CREATE, ALTER, logic)
		if strings.HasPrefix(upper, "INSERT INTO") ||
			strings.HasPrefix(upper, "COPY ") ||
			strings.HasPrefix(upper, "UPDATE ") ||
			strings.HasPrefix(upper, "VALUES") {
			lineOffset += numLines
			continue
		}

		chunkType := "sql_statement"
		name := "query"

		if strings.Contains(upper, "CREATE TABLE") {
			chunkType = "table"
			// Extract name (simple)
			words := strings.Fields(trimmed)
			for i, w := range words {
				if strings.ToUpper(w) == "TABLE" && i+1 < len(words) {
					name = strings.Trim(words[i+1], "(\"`")
					break
				}
			}
		} else if strings.Contains(upper, "CREATE FUNCTION") || strings.Contains(upper, "CREATE PROCEDURE") {
			chunkType = "function"
		} else if strings.Contains(upper, "CREATE TRIGGER") {
			chunkType = "trigger"
		} else if strings.Contains(upper, "ALTER TABLE") {
			chunkType = "alter"
		}

		chunks = append(chunks, Chunk{
			Type:      chunkType,
			Name:      name,
			Content:   trimmed + ";",
			StartLine: lineOffset,
			EndLine:   lineOffset + numLines,
		})
		lineOffset += numLines + 1
	}

	if len(chunks) == 0 {
		return sc.chunkGeneric(content)
	}
	return chunks, nil
}

// chunkGeneric splits by paragraphs/blocks as fallback
func (sc *SemanticChunker) chunkGeneric(content string) ([]Chunk, error) {
	var chunks []Chunk
	lines := strings.Split(content, "\n")

	const maxChunkSize = 800
	const overlap = 100

	var currentChunk strings.Builder
	startLine := 1
	currentLines := 0

	for i, line := range lines {
		if currentChunk.Len()+len(line) > maxChunkSize && currentChunk.Len() > 0 {
			// Save current chunk
			chunks = append(chunks, Chunk{
				Type:      "generic",
				Name:      "block",
				Content:   currentChunk.String(),
				Context:   "",
				StartLine: startLine,
				EndLine:   startLine + currentLines,
			})

			// Reset with overlap
			// For simplicity in this generic version, just hard reset or keep last few lines
			// Implementing proper overlap requires keeping a buffer of lines
			currentChunk.Reset()
			startLine = i + 1
			currentLines = 0
		}

		currentChunk.WriteString(line + "\n")
		currentLines++
	}

	// Add remaining
	if currentChunk.Len() > 0 {
		chunks = append(chunks, Chunk{
			Type:      "generic",
			Name:      "block",
			Content:   currentChunk.String(),
			Context:   "",
			StartLine: startLine,
			EndLine:   startLine + currentLines,
		})
	}

	return chunks, nil
}

// Helper to extract exact lines from content
func extractLines(content string, start, end int) string {
	lines := strings.Split(content, "\n")
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}

	// zero-based index
	return strings.Join(lines[start-1:end], "\n")
}
