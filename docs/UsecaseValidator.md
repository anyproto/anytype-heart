## Use Case archives validation tool

To use Use Case archives validation tool head to [cmd/usecasevalidator](../cmd/usecasevalidator), build the program
`go build`
and run it using
`./usecasevalidator -path <path_to_archive>`
command.

Program accepts one obligatory flag - path to the archive containing exported objects from space.
Other flags could be specified to validate/list/process files in archive:

```
Usage of ./usecasevalidator:
-a	Insert analytics context and original id
-creator    Set Anytype profile to LastModifiedDate and Creator
-exclude    Exclude objects that did not pass validation
-list       List all objects in archive
-path <string>   Path to zip archive
-r          Remove account related relations
-rules <string>  Path to file with processing rules
-validate   Perform validation upon all objects
-c          Collect usage information about custom types and relations 
```

If all protobuf objects contain correct information and were changed (e.g. `a`, `creator`, `r`, `rules` or `exclude` flags were specified), resulting archive would be written in **<path_to_archive>_new.zip** file in same directory.

If objects in archive have some incorrect information, e.g.:
- links to objects that are not presented in the archive
- relation links or object types not presented among objects

then program provides you with error messages in the output along with the list of all objects processed by the tool.

### Resolving incorrect data

Incorrect data found by tool could be resolved two different ways:
1. Via editor of the client and repetitive export of desired account
2. Using rules engine
3. Using Export Archive unpacker tool (see its [docs](ExportArchiveUnpacker.md))

Rules are the actions that could be done upon such entities of smartblock as:
- relation links
- object types
- details
- target object of dataView blocks
- target object of link blocks

Other entities will be added to the list on demand.

These entities could be added to the object, modified or deleted.
To choose the desired action **action** field should be provided to the JSON object.

**rules.json** file provides all rules to be processed upon archive.

#### Examples:

1. To change the target object of dataView block **"block1"** of object **"object1"**
   to some object **"object2"** these json object should be defined:

```json
{
    "action": "change",
    "entity": "dataViewTarget",
    "objectID": "object1",
    "targetID": "object2",
    "blockID": "block1"
}
```

2. To add relation link **grannyName** with format shorttext (=1)
   to some object **"lovelygrandson"** define this rule:

```json
{
    "action": "add",
    "entity": "relationLink",
    "objectID": "lovelygrandson",
    "relationLink": {
        "key": "grannyName",
        "format": 1
    }
}
```

3. To delete detail with key **awfulDetail** from all objects in the archive define:
```json
{
    "action": "remove",
    "entity": "detail",
    "detailKey": "awfulDetail"
}
```

More examples could be found in **rules.json** file
