## Exported archives converter tool

Anytype allows user to export desired objects whether in _JSON_ or _PROTOBUF_ formats. 

Most of exported archives contain objects stored in _PROTOBUF_ format, as it is encoded and lightweight. However, if you need to convert an exported archive to see all objects in _JSON_ format you can use **archiveconverter** tool.

CLI can run in two modes.

### Unpack
```bash
go run main.go -unpack <path_to_zip>
```
`unpack` parameter accepts path to the _ZIP_ archive containing _PROTOBUF_ files. Each file should be one of following model:

- ChangeSnapshot
- SnapshotWithType
- Profile

As a result program generates a directory with files in _JSON_ format corresponding to content of input archive.
You can edit it freely.

### Pack
```bash
go run main.go -pack <path_to_directory>
```

`pack` parameter accepts path to the directory containing _JSON_ files, that also should be one of models presented in previous paragraph.

In packing mode program does reverse operation and creates an archive with _PROTOBUF_ files corresponding to _JSON_ objects presented in input directory.