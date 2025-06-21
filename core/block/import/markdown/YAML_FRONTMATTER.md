# YAML Front Matter in Markdown Import

## What is YAML Front Matter?

YAML front matter is metadata placed at the beginning of a markdown file, enclosed between two lines of three dashes (`---`). This metadata is parsed and converted into Anytype object properties during import.

## Format

```markdown
---
key: value
another_key: another value
list_key: [item1, item2, item3]
---

# Markdown content starts here
```

## Supported Data Types

### Text
```yaml
title: My Document
description: A longer text that will be imported as longtext if >100 characters
```

### Numbers
```yaml
count: 42
rating: 4.5
```

### Dates
```yaml
# Date without time
created: 2023-06-01

# Date with time
modified: 2023-06-01T14:30:00
```

### Booleans
```yaml
published: true
draft: false
```

### Lists (Tags)
```yaml
tags: [important, urgent, review]
categories: ["Work", "Personal"]
```

### URLs and Emails
```yaml
website: https://anytype.io
contact: support@anytype.io
```

## Special Properties

### Object Type
The `Type` or `Object type` property sets the object type:

```yaml
Type: Task
# or
Object type: Note
```

This will search for an existing type with that name and apply it to the imported object.

## Example

```markdown
---
title: Project Planning
Type: Task
Start Date: 2023-06-01
End Date: 2023-06-15T17:00:00
status: in-progress
priority: high
assigned_to: John Doe
tags: [planning, q2-2023, important]
completed: false
budget: 50000
website: https://project.example.com
---

# Project Planning

## Overview
This document outlines the project planning for Q2 2023...
```

This will create a Task object with:
- Title as short text
- Start Date as date (without time)
- End Date as date (with time)
- Status, priority, assigned_to as short text
- Tags as a tag relation
- Completed as checkbox
- Budget as number
- Website as URL