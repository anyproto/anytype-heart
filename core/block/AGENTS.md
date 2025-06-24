# core/block - Block Management System

## Overview
The core/block package is the heart of Anytype's content management system. It handles all block operations, object management, and content transformations within the Anytype ecosystem.

## Key Concepts

### Blocks vs Objects
- **Objects** (formerly SmartBlocks): Top-level content units (pages, tasks, etc.)
- **Blocks**: Individual content elements within objects (text, images, etc.)
- Every object contains a tree of blocks

### Architecture
```
Object (e.g., Page)
├── Root Block
├── Title Block
├── Text Block
├── Image Block
└── Nested Blocks...
```

## Main Components

### Core Services
- `service.go` - Main block service orchestrating all operations
- `create.go` - Object creation logic
- `delete.go` - Object deletion and cleanup
- `undo.go` - Undo/redo functionality

### Sub-packages

#### `/editor` - Block editing and state management
- SmartBlock implementations
- State management
- Change processing

#### `/simple` - Basic block types
- Text, file, link blocks
- Block-specific operations

#### `/dataview` - Database views
- Collection and set management
- Query processing

#### `/import` - Import from external sources
- Notion, Markdown, CSV, HTML importers
- Format converters

#### `/export` - Export functionality
- Multiple format support
- Archive creation

#### `/cache` - Object caching layer
- In-memory object cache
- Performance optimization

#### `/source` - Data source abstraction
- Storage interface
- Object retrieval

#### `/restriction` - Access control
- Permission checking
- Operation restrictions

## Key Operations

### Object Lifecycle
1. **Create**: Via `ObjectCreate` - generates unique ID, initializes state
2. **Open**: Loads from storage, applies migrations
3. **Edit**: Applies changes via CRDT operations
4. **Save**: Persists to local storage and sync queue
5. **Close**: Cleanup and cache management

### Change Processing
- All edits are CRDT changes
- Changes are applied to state
- Synchronous indexing to ObjectStore
- Asynchronous fulltext indexing

### Storage Integration
- Objects stored in space-specific databases
- Properties indexed in ObjectStore (SQLite)
- Fulltext search via Tantivy
- File content in IPFS-like storage

## Important Patterns

### Service Registration
```go
func (s *service) Init(a *app.App) error {
    // Register dependencies
    s.picker = app.MustComponent[cache.ObjectGetter](a)
    s.store = app.MustComponent[objectstore.ObjectStore](a)
    return nil
}
```

### Error Handling
- Always wrap errors with context
- Use specific error types for different failures
- Handle space deletion gracefully

### Event Broadcasting
- Changes trigger events for UI updates
- Subscription system for real-time updates
- Cross-space event handling

## Performance Considerations

### Caching Strategy
- Hot objects kept in memory
- LRU eviction policy
- Preload related objects

### Indexing
- Synchronous property indexing
- Asynchronous fulltext indexing
- Batch operations when possible

### Concurrency
- Thread-safe object access
- Lock granularity at object level
- Avoid long-running operations in locks

## Common Tasks

### Creating a Page
```go
details := map[string]any{
    "type": "page",
    "name": "My Page",
}
resp, err := s.ObjectCreate(ctx, req)
```

### Adding a Block
```go
s.BlockCreate(ctx, &BlockCreateRequest{
    ObjectId: objectId,
    Block: &model.Block{...},
    Position: model.Block_Inner,
})
```

### Searching Objects
- Use ObjectStore queries
- Filter by properties
- Sort and paginate results

## Dependencies
- `objectstore` - Property storage
- `cache` - Object caching
- `filestorage` - File handling
- `syncstatus` - Sync state
- `subscription` - Real-time updates