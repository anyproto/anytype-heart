package files

/*
We have two types of files: images and ordinary files

Each file consists of:
- Root node with its CID represented as domain.FileId type
- One file variant for ordinary files and multiple file variants for images
- File variant represented as pair node which consists of Content and Metadata nodes
- CID of content node represented as domain.FileContentId type
*/
