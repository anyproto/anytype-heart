# core/block/import - Import System

## Overview
The import package handles importing content from various external sources into Anytype. It supports multiple formats and provides a unified import pipeline with progress tracking and error handling.

## Supported Formats

### Document Formats
- **Markdown** (.md) - Full markdown syntax support
- **HTML** (.html) - Web page import with styling
- **TXT** (.txt) - Plain text files
- **CSV** (.csv) - Tabular data import

### Platform Imports
- **Notion** - Full workspace export
- **Protobuf** (.pb) - Anytype export format
- **Web** - Direct URL import

## Architecture

### Import Pipeline
```
Source File/URL
    ↓
Format Detection
    ↓
Parser Selection
    ↓
Content Extraction
    ↓
Block Conversion
    ↓
Object Creation
    ↓
Relation Mapping
    ↓
File Processing
    ↓
Final Object
```

## Core Components

### Main Importer
- `importer.go` - Central import orchestrator
- Handles format detection
- Manages import sessions
- Progress tracking
- Error collection

### Common Components (`/common`)
- `common.go` - Shared functionality
- `collection.go` - Collection import
- `error.go` - Error handling
- `filenameprovider.go` - Name generation
- `types.go` - Common types

### Format-Specific Importers

#### Markdown (`/markdown`)
```go
type MarkdownConverter struct {
    // Converts markdown to Anytype blocks
    processImages   bool
    targetPath      string
}
```
- Full CommonMark support
- Image handling
- Code block preservation
- Link conversion

#### Notion (`/notion`)
```go
type NotionConverter struct {
    // Handles Notion export
    workspace    *NotionWorkspace
    databases    map[string]*Database
    pageMapping  map[string]string
}
```
- Database import
- Relation mapping
- File attachment handling
- Page hierarchy preservation

#### CSV (`/csv`)
```go
type CSVConverter struct {
    // CSV to database import
    strategy    ImportStrategy
    headers     []string
    delimiter   rune
}
```
- Table strategy - Import as table blocks
- Collection strategy - Import as database
- Type inference
- Custom delimiters

#### HTML (`/html`)
- DOM parsing
- Style extraction
- Image downloading
- Link preservation

#### Web (`/web`)
- URL fetching
- Readability extraction
- Metadata parsing
- Image caching

## Import Process

### 1. Initialization
```go
func (i *Import) Init(reader io.Reader, options ImportOptions) error {
    // Detect format
    format := i.detectFormat(reader)
    
    // Create converter
    i.converter = i.createConverter(format)
    
    // Initialize progress
    i.progress = newProgress()
    
    return nil
}
```

### 2. Conversion
```go
func (i *Import) Process(ctx context.Context) (*ImportResult, error) {
    // Parse source
    blocks, err := i.converter.Convert(ctx)
    
    // Create objects
    for _, block := range blocks {
        obj := i.createObject(block)
        i.result.Objects = append(i.result.Objects, obj)
    }
    
    return i.result, nil
}
```

### 3. File Handling
```go
func (i *Import) processFiles(files []*File) error {
    for _, file := range files {
        // Upload to file storage
        hash, err := i.fileStore.Add(file.Data)
        
        // Update references
        i.updateFileRefs(file.ID, hash)
    }
}
```

## Progress Tracking

### Progress Events
```go
type Progress struct {
    Total      int
    Current    int
    Message    string
    ObjectName string
}

// Send progress updates
i.progress.Update(Progress{
    Total:   100,
    Current: 50,
    Message: "Importing pages...",
})
```

### Error Collection
```go
type ImportError struct {
    ObjectName string
    Message    string
    Code       ErrorCode
}

// Collect non-fatal errors
i.errors.Add(ImportError{
    ObjectName: "Page 1",
    Message:    "Unsupported block type",
    Code:       ErrUnsupportedBlock,
})
```

## Converter Implementation

### Base Converter Pattern
```go
type Converter interface {
    Convert(ctx context.Context, reader io.Reader) (*ConvertResult, error)
    GetType() string
    GetFileExtensions() []string
}
```

### Block Creation
```go
func (c *converter) createTextBlock(content string) *model.Block {
    return &model.Block{
        Id: generateID(),
        Content: &model.BlockContentText{
            Text:  content,
            Style: model.BlockText_Paragraph,
        },
    }
}
```

### Relation Handling
```go
func (c *converter) mapRelations(source map[string]any) map[string]any {
    details := make(map[string]any)
    
    for key, value := range source {
        // Map to Anytype relations
        if relation, ok := c.relationMap[key]; ok {
            details[relation] = c.convertValue(value)
        }
    }
    
    return details
}
```

## Import Strategies

### Collection Import
- Creates database with entries
- Preserves structure
- Maps columns to relations
- Maintains relationships

### Page Import
- Creates individual pages
- Preserves formatting
- Handles nested content
- Maintains links

### Archive Import
- Handles .zip files
- Preserves folder structure
- Batch imports
- Progress per file

## Error Handling

### Error Types
- `ErrUnsupportedFormat` - Unknown file type
- `ErrCorruptedFile` - Can't parse file
- `ErrSizeLimitExceeded` - File too large
- `ErrUnsupportedBlock` - Unknown block type

### Recovery Strategies
```go
func (i *Import) handleError(err error) {
    switch {
    case IsRecoverable(err):
        // Log and continue
        i.errors.Add(err)
    case IsCritical(err):
        // Stop import
        return err
    default:
        // Try fallback
        i.tryFallback()
    }
}
```

## Performance Optimization

### Streaming Processing
- Process large files in chunks
- Avoid loading entire file in memory
- Stream parsing for CSV/JSON

### Batch Operations
```go
// Batch object creation
objects := make([]*Object, 0, 1000)
for _, item := range items {
    objects = append(objects, convert(item))
    if len(objects) >= 1000 {
        i.createObjects(objects)
        objects = objects[:0]
    }
}
```

### Parallel Processing
- Concurrent file uploads
- Parallel page conversion
- Worker pool for images

## Testing

### Test Fixtures
```
testdata/
├── notion/
│   ├── simple.zip
│   └── complex_workspace.zip
├── markdown/
│   ├── basic.md
│   └── with_images.md
└── csv/
    ├── simple.csv
    └── large_dataset.csv
```

### Integration Tests
```go
func TestNotionImport(t *testing.T) {
    // Load test export
    data := loadTestData("notion/simple.zip")
    
    // Run import
    result, err := importer.Import(data)
    
    // Verify results
    assert.NoError(t, err)
    assert.Len(t, result.Objects, 10)
}
```

## Common Issues

### Memory Usage
- Large imports can consume significant memory
- Use streaming where possible
- Monitor progress and cancel if needed

### Format Variations
- Different Notion export versions
- Markdown flavors
- CSV encodings

### Performance
- Image processing bottlenecks
- Large file uploads
- Database imports with many rows