---
title: Project Documentation
author: Jane Smith
status: in-progress
priority: high
due_date: 2024-12-31
Start Date: 2023-06-01
End Date: 2023-06-01T14:30:00
tags: [documentation, important, technical]
completion: 75
reviewed: false
website: https://anytype.io
email: contact@anytype.io
description: This is a longer description that contains more details about the project documentation. It should be imported as a longtext relation.
Object Type: Task
---

# Project Overview

This is an example markdown file with YAML front matter that demonstrates how properties are imported as relations in Anytype.

## Features

- YAML properties are converted to appropriate relation formats
- Existing relations are reused when available
- Various data types are supported:
  - Text (short and long)
  - Numbers
  - Booleans (checkboxes)
  - Arrays (tags)
  - URLs
  - Emails

## Implementation Details

The YAML front matter is extracted before markdown parsing and properties are created as relations with appropriate formats based on their values.