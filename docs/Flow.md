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