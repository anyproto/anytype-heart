### Standard codes 

1. NULL - import finished without errors
2. INTERNAL_ERROR - error in internal logic of Anytype. For example, problems with infrastructure layer

### Common codes
1. FILE_LOAD_ERROR - when we have problems loading file to our infrastructure
2. IMPORT_IS_CANCELED - user cancelled import

### Notion specific codes
1. NOTION_NO_OBJECTS_IN_INTEGRATION - user didn't add any object to Notion token, so we couldn't import anything
2. NOTION_SERVER_IS_UNAVAILABLE - when Notion request returns >500 error codes, which means problem with their servers
3. NOTION_RATE_LIMIT_EXCEEDED - when Notion requests returns 429 error. That means, we exceeded a limit of request to Notion server and exceeded attempts amount to reach this server

### Files import specific codes
1. FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE - user imported empty archive or archive without targeted import type (for example user chose to import HTML file, but archive contains only MD files)
2. FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY - user imported empty directory or directory without targeted import type

### HTML import specific codes
1. HTML_WRONG_HTML_STRUCTURE - there was error with rendering HTML file

### Any Block import specific codes
1. PB_NOT_ANYBLOCK_FORMAT - when we failed to unmarshal given pb/json file or file contain invalid snapshot

### CSV import specific codes
1. CSV_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED - user tried to import CSV file, where amount of rows or columns exceeded 1000
