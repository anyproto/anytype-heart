# Flow
## Back links
To handle editor navigation and support object links consistency `backlinks` built-in relation is provided by Middleware. This relation handles a list of objects that have a link to the object the relation is set to.

Relation is updated on every `smartBlock.updateBackLinks` invocation that is called from `Show` and from `injectLocalDetails` methods of `smartBlock` object, so from the user perspective back links are updated on following object actions:

- Object initialization
- Object opening, as ObjectOpen gRPC request triggers `smartBlock.Show` method
- State Rebuild of a particular object that is triggered from the any-sync side
- Resetting of Object version that needs repetitive injection of local details

### TODO:
The drawback of this behavior is that back links update is not triggered on every actual change of back links content.

Need to redesign this approach the way to handle every possible change of object's incoming links:

- Creation of link block
- Creation of inline set or collection
- Creation of widget (?)
- Creation of bookmark
- ...

As now backlinks are gotten from the object store, all these changes need to be done after store redesign, so mechanism of getting links of object and saving it both to store and to backlinks relation has same enter point.

## Internal Flags
All objects can have relation `InternalFlags` set to its details. This relation handles list of flags used mainly by client:

- `DeleteEmpty` - stands for deletion of empty objects. Middleware performs object deletion on ObjectClose in case object has this internal flag.
- `SelectType` - stands for showing Type Picker on client-side.
- `SelectTemplate` - stands for showing Template Picker on client-side.

Client passes `internalFlags` value on object creation, and then the content of its value could be modified only by Middleware. However, it only deletes flags, and it deletes them on **ALL** commands where new state of object is applied **EXCEPT** these cases:

1. ObjectApplyTemplate
2. ObjectSetObjectType - deletes only `DeleteEmpty` and `SelectType`
3. ObjectSetDetails - if one of details is `Name`
4. ObjectOpen
5. ObjectClose
6. BlockTextSetText - if no changes in block were made OR it is **title** or **description** block

In all other cases Middleware performs state Non-Emptiness check. And if state is not empty, it wipes `internalFlags`.

### Empty state definition used in check
A state that either has no blocks or all blocks have no text inside. This definition is awful, because these kinds of objects would be deleted on closing:

- Object has no blocks filled, but a lot of details pre-filled by user
- Object has a lot of custom blocks that could not handle text

### TODO:

1. Redesign `state.IsEmpty` check. Reasons are mentioned above
2. Think about moving `SelectType` and `SelectTemplate` flags controlling logic to clients, because Middleware does not use these flags, but have very complex logic on its maintaining

## Blank template
Blank template is a virtual template for all kinds of objects, that could be chosen as default on client-side. Middleware does not have built-in template object to apply it on state on desired object, but understands literal `blank` as a value of `templateId` field of following commands:

- BlockLinkCreateWithObject
- ObjectCreate
- ObjectCreateSet
- ObjectApplyTemplate

If `blank` is chosen as `templateId` in one of first three commands, Middleware acts as no template was chosen at all and creates new object regarding remaining parameters of request.

If `blank` is chosen as `templateId` in ObjectApplyTemplate, Middleware creates applies new state, leaving only **Type** and **Layout** of previous state.

## System Objects Update
Some Object types and Relations are recognized as **System** ones, because application business logic depends on their content.

Examples of system types are **Note**, **Task** and **Bookmark**. Non-system - **Contact**, **Goal** and **Feature**.

Examples of system relations are **Id**, **Name** and **Done**. Non-system - **FocalRatio**, **Instagram** and **HowToReproduce**.

System types and relations could not be modified by the users or deleted from spaces.
However, sometimes developers need to modify system objects to support some new features.

To handle system object update and save backward compatibility each system object type and relation has its own `Revision`.
Anytype will update system objects only if `Revision` of object from marketplace is higher than `Revision` of object from user's space.

### How to update system objects

1. Update description of system object, that is stored in `pkg/lib/bundle`
2. Increase `revision` field of system type/relation or put `"revision":1` if it was empty
3. Generate go-level variables for new version of types and relations using `pkg/lib/bundle/generator`
4. Make sure that new fields are taken into account in [System Object Reviser](../core/block/object/objectcreator/systemobjectreviser.go).
   (Right now only these fields are checked: **Revision**, **Name**, **Description**, **IsHidden**, **IsReadonly**)
5. Build and run Anytype. All system objects with lower `Revision` should be updated according your changes in all spaces