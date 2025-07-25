---
# yaml-language-server: $schema=./schemas/document.schema.json
Object Type: Document
name: Project Documentation
# File format relation - direct file paths
attachments:
  - ./files/report.pdf
  - ./files/budget.xlsx
cover_image: ./files/logo.png
# Object format relation - references to other exported objects
related_documents:
  - Name: Technical Specification
    File: ./technical_spec.md
  - Name: Architecture Overview
    File: ./architecture.md
references:
  Name: API Documentation
  File: ./api_docs.md
# Tag format - just names
tags:
  - documentation
  - important
  - technical
status: Published
---

# Project Documentation

This document demonstrates the different relation formats:
- **File format**: Direct file paths for attachments
- **Object format**: References to other exported objects with Name and File properties
- **Tag/Status format**: Simple string values