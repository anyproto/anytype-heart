## Use Case archives generating tool

To use Use Case archives processing tool head to [cmd/usecasegenerator](../cmd/usecasegenerator), build the program
`go build`
and run it using
`./archiveprocessor <path_to_archive>`
command.

Program accepts only one parameter - path to the archive containing exported objects from space.

If all protobuf objects contain correct information, resulting archive would be written in **<path_to_archive>_new.zip** file in same directory.

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