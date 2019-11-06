# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [service/service.proto](#service/service.proto)
  
  
  
    - [ClientCommands](#anytype.ClientCommands)
  

- [changes.proto](#changes.proto)
    - [Change](#anytype.Change)
    - [Change.Block](#anytype.Change.Block)
    - [Change.Block.Children](#anytype.Change.Block.Children)
    - [Change.Block.Content](#anytype.Change.Block.Content)
    - [Change.Block.Content.Dashboard](#anytype.Change.Block.Content.Dashboard)
    - [Change.Block.Content.File](#anytype.Change.Block.Content.File)
    - [Change.Block.Content.Image](#anytype.Change.Block.Content.Image)
    - [Change.Block.Content.Page](#anytype.Change.Block.Content.Page)
    - [Change.Block.Content.Text](#anytype.Change.Block.Content.Text)
    - [Change.Block.Content.Video](#anytype.Change.Block.Content.Video)
    - [Change.Block.Fields](#anytype.Change.Block.Fields)
    - [Change.Block.Permissions](#anytype.Change.Block.Permissions)
    - [Change.Multiple](#anytype.Change.Multiple)
    - [Change.Multiple.BlocksList](#anytype.Change.Multiple.BlocksList)
    - [Change.Single](#anytype.Change.Single)
    - [Change.Single.BlocksList](#anytype.Change.Single.BlocksList)
  
  
  
  

- [commands.proto](#commands.proto)
    - [Rpc](#anytype.Rpc)
    - [Rpc.Account](#anytype.Rpc.Account)
    - [Rpc.Account.Create](#anytype.Rpc.Account.Create)
    - [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request)
    - [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response)
    - [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error)
    - [Rpc.Account.Recover](#anytype.Rpc.Account.Recover)
    - [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request)
    - [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response)
    - [Rpc.Account.Recover.Response.Error](#anytype.Rpc.Account.Recover.Response.Error)
    - [Rpc.Account.Select](#anytype.Rpc.Account.Select)
    - [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request)
    - [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response)
    - [Rpc.Account.Select.Response.Error](#anytype.Rpc.Account.Select.Response.Error)
    - [Rpc.Block](#anytype.Rpc.Block)
    - [Rpc.Block.Create](#anytype.Rpc.Block.Create)
    - [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request)
    - [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response)
    - [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error)
    - [Rpc.Block.History](#anytype.Rpc.Block.History)
    - [Rpc.Block.History.Move](#anytype.Rpc.Block.History.Move)
    - [Rpc.Block.History.Move.Request](#anytype.Rpc.Block.History.Move.Request)
    - [Rpc.Block.History.Move.Response](#anytype.Rpc.Block.History.Move.Response)
    - [Rpc.Block.History.Move.Response.Error](#anytype.Rpc.Block.History.Move.Response.Error)
    - [Rpc.Block.Open](#anytype.Rpc.Block.Open)
    - [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request)
    - [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response)
    - [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error)
    - [Rpc.Block.Update](#anytype.Rpc.Block.Update)
    - [Rpc.Block.Update.Request](#anytype.Rpc.Block.Update.Request)
    - [Rpc.Block.Update.Response](#anytype.Rpc.Block.Update.Response)
    - [Rpc.Block.Update.Response.Error](#anytype.Rpc.Block.Update.Response.Error)
    - [Rpc.Image](#anytype.Rpc.Image)
    - [Rpc.Image.Get](#anytype.Rpc.Image.Get)
    - [Rpc.Image.Get.Blob](#anytype.Rpc.Image.Get.Blob)
    - [Rpc.Image.Get.Blob.Request](#anytype.Rpc.Image.Get.Blob.Request)
    - [Rpc.Image.Get.Blob.Response](#anytype.Rpc.Image.Get.Blob.Response)
    - [Rpc.Image.Get.Blob.Response.Error](#anytype.Rpc.Image.Get.Blob.Response.Error)
    - [Rpc.Image.Get.File](#anytype.Rpc.Image.Get.File)
    - [Rpc.Image.Get.File.Request](#anytype.Rpc.Image.Get.File.Request)
    - [Rpc.Image.Get.File.Response](#anytype.Rpc.Image.Get.File.Response)
    - [Rpc.Image.Get.File.Response.Error](#anytype.Rpc.Image.Get.File.Response.Error)
    - [Rpc.Ipfs](#anytype.Rpc.Ipfs)
    - [Rpc.Ipfs.Get](#anytype.Rpc.Ipfs.Get)
    - [Rpc.Ipfs.Get.File](#anytype.Rpc.Ipfs.Get.File)
    - [Rpc.Ipfs.Get.File.Request](#anytype.Rpc.Ipfs.Get.File.Request)
    - [Rpc.Ipfs.Get.File.Response](#anytype.Rpc.Ipfs.Get.File.Response)
    - [Rpc.Ipfs.Get.File.Response.Error](#anytype.Rpc.Ipfs.Get.File.Response.Error)
    - [Rpc.Log](#anytype.Rpc.Log)
    - [Rpc.Log.Send](#anytype.Rpc.Log.Send)
    - [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request)
    - [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response)
    - [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error)
    - [Rpc.Version](#anytype.Rpc.Version)
    - [Rpc.Version.Get](#anytype.Rpc.Version.Get)
    - [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request)
    - [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response)
    - [Rpc.Version.Get.Response.Error](#anytype.Rpc.Version.Get.Response.Error)
    - [Rpc.Wallet](#anytype.Rpc.Wallet)
    - [Rpc.Wallet.Create](#anytype.Rpc.Wallet.Create)
    - [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request)
    - [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response)
    - [Rpc.Wallet.Create.Response.Error](#anytype.Rpc.Wallet.Create.Response.Error)
    - [Rpc.Wallet.Recover](#anytype.Rpc.Wallet.Recover)
    - [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request)
    - [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response)
    - [Rpc.Wallet.Recover.Response.Error](#anytype.Rpc.Wallet.Recover.Response.Error)
  
    - [Rpc.Account.Create.Response.Error.Code](#anytype.Rpc.Account.Create.Response.Error.Code)
    - [Rpc.Account.Recover.Response.Error.Code](#anytype.Rpc.Account.Recover.Response.Error.Code)
    - [Rpc.Account.Select.Response.Error.Code](#anytype.Rpc.Account.Select.Response.Error.Code)
    - [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code)
    - [Rpc.Block.History.Move.Response.Error.Code](#anytype.Rpc.Block.History.Move.Response.Error.Code)
    - [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code)
    - [Rpc.Block.Update.Response.Error.Code](#anytype.Rpc.Block.Update.Response.Error.Code)
    - [Rpc.Image.Get.Blob.Response.Error.Code](#anytype.Rpc.Image.Get.Blob.Response.Error.Code)
    - [Rpc.Image.Get.File.Response.Error.Code](#anytype.Rpc.Image.Get.File.Response.Error.Code)
    - [Rpc.Ipfs.Get.File.Response.Error.Code](#anytype.Rpc.Ipfs.Get.File.Response.Error.Code)
    - [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code)
    - [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code)
    - [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code)
    - [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code)
  
  
  

- [events.proto](#events.proto)
    - [Event](#anytype.Event)
    - [Event.Account](#anytype.Event.Account)
    - [Event.Account.Show](#anytype.Event.Account.Show)
    - [Event.Block](#anytype.Event.Block)
    - [Event.Block.Create](#anytype.Event.Block.Create)
    - [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload)
    - [Event.Block.Show](#anytype.Event.Block.Show)
    - [Event.Block.Update](#anytype.Event.Block.Update)
    - [Event.User](#anytype.Event.User)
    - [Event.User.Block](#anytype.Event.User.Block)
    - [Event.User.Block.Join](#anytype.Event.User.Block.Join)
    - [Event.User.Block.Left](#anytype.Event.User.Block.Left)
    - [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange)
    - [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange)
  
  
  
  

- [models.proto](#models.proto)
    - [Model](#anytype.Model)
    - [Model.Account](#anytype.Model.Account)
    - [Model.Account.Avatar](#anytype.Model.Account.Avatar)
    - [Model.Block](#anytype.Model.Block)
    - [Model.Block.Content](#anytype.Model.Block.Content)
    - [Model.Block.Content.Dashboard](#anytype.Model.Block.Content.Dashboard)
    - [Model.Block.Content.Dataview](#anytype.Model.Block.Content.Dataview)
    - [Model.Block.Content.Div](#anytype.Model.Block.Content.Div)
    - [Model.Block.Content.File](#anytype.Model.Block.Content.File)
    - [Model.Block.Content.File.Preview](#anytype.Model.Block.Content.File.Preview)
    - [Model.Block.Content.Image](#anytype.Model.Block.Content.Image)
    - [Model.Block.Content.Image.Preview](#anytype.Model.Block.Content.Image.Preview)
    - [Model.Block.Content.Layout](#anytype.Model.Block.Content.Layout)
    - [Model.Block.Content.Page](#anytype.Model.Block.Content.Page)
    - [Model.Block.Content.Text](#anytype.Model.Block.Content.Text)
    - [Model.Block.Content.Text.Mark](#anytype.Model.Block.Content.Text.Mark)
    - [Model.Block.Content.Text.Marks](#anytype.Model.Block.Content.Text.Marks)
    - [Model.Block.Content.Video](#anytype.Model.Block.Content.Video)
    - [Model.Block.Content.Video.Preview](#anytype.Model.Block.Content.Video.Preview)
    - [Model.Block.Permissions](#anytype.Model.Block.Permissions)
    - [Model.Image](#anytype.Model.Image)
    - [Model.Range](#anytype.Model.Range)
    - [Model.Struct](#anytype.Model.Struct)
    - [Model.Struct.FieldsEntry](#anytype.Model.Struct.FieldsEntry)
    - [Model.Struct.ListValue](#anytype.Model.Struct.ListValue)
    - [Model.Struct.Value](#anytype.Model.Struct.Value)
    - [Model.Video](#anytype.Model.Video)
  
    - [Model.Block.Content.Dashboard.Style](#anytype.Model.Block.Content.Dashboard.Style)
    - [Model.Block.Content.File.State](#anytype.Model.Block.Content.File.State)
    - [Model.Block.Content.Image.State](#anytype.Model.Block.Content.Image.State)
    - [Model.Block.Content.Layout.Style](#anytype.Model.Block.Content.Layout.Style)
    - [Model.Block.Content.Page.Style](#anytype.Model.Block.Content.Page.Style)
    - [Model.Block.Content.Text.Mark.Type](#anytype.Model.Block.Content.Text.Mark.Type)
    - [Model.Block.Content.Text.MarkerType](#anytype.Model.Block.Content.Text.MarkerType)
    - [Model.Block.Content.Text.Style](#anytype.Model.Block.Content.Text.Style)
    - [Model.Block.Content.Video.State](#anytype.Model.Block.Content.Video.State)
    - [Model.Block.Type](#anytype.Model.Block.Type)
    - [Model.Image.Size](#anytype.Model.Image.Size)
    - [Model.Struct.NullValue](#anytype.Model.Struct.NullValue)
    - [Model.Video.Size](#anytype.Model.Video.Size)
  
  
  

- [Scalar Value Types](#scalar-value-types)



<a name="service/service.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## service/service.proto


 

 

 


<a name="anytype.ClientCommands"></a>

### ClientCommands


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| WalletCreate | [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request) | [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response) |  |
| WalletRecover | [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request) | [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response) |  |
| AccountRecover | [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request) | [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response) |  |
| AccountCreate | [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request) | [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response) |  |
| AccountSelect | [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request) | [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response) |  |
| ImageGetBlob | [Rpc.Image.Get.Blob.Request](#anytype.Rpc.Image.Get.Blob.Request) | [Rpc.Image.Get.Blob.Response](#anytype.Rpc.Image.Get.Blob.Response) |  |
| VersionGet | [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request) | [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request) | [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response) |  |
| BlockOpen | [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request) | [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response) |  |
| BlockCreate | [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request) | [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response) |  |
| BlockUpdate | [Rpc.Block.Update.Request](#anytype.Rpc.Block.Update.Request) | [Rpc.Block.Update.Response](#anytype.Rpc.Block.Update.Response) |  |
| BlockHistoryMove | [Rpc.Block.History.Move.Request](#anytype.Rpc.Block.History.Move.Request) | [Rpc.Block.History.Move.Response](#anytype.Rpc.Block.History.Move.Response) | rpc BlockFilesUpload (Block Rpc.History.Move.Request) returns (BlockRpc..History Move.Response); |

 



<a name="changes.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## changes.proto



<a name="anytype.Change"></a>

### Change







<a name="anytype.Change.Block"></a>

### Change.Block







<a name="anytype.Change.Block.Children"></a>

### Change.Block.Children



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| children | [string](#string) | repeated |  |






<a name="anytype.Change.Block.Content"></a>

### Change.Block.Content







<a name="anytype.Change.Block.Content.Dashboard"></a>

### Change.Block.Content.Dashboard



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Model.Block.Content.Dashboard.Style](#anytype.Model.Block.Content.Dashboard.Style) |  |  |
| block | [Model.Block](#anytype.Model.Block) |  |  |






<a name="anytype.Change.Block.Content.File"></a>

### Change.Block.Content.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| content | [string](#string) |  |  |
| state | [Model.Block.Content.File.State](#anytype.Model.Block.Content.File.State) |  |  |
| preview | [Model.Block.Content.File.Preview](#anytype.Model.Block.Content.File.Preview) |  |  |






<a name="anytype.Change.Block.Content.Image"></a>

### Change.Block.Content.Image



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| content | [string](#string) |  |  |
| state | [Model.Block.Content.Image.State](#anytype.Model.Block.Content.Image.State) |  |  |
| preview | [Model.Block.Content.Image.Preview](#anytype.Model.Block.Content.Image.Preview) |  |  |






<a name="anytype.Change.Block.Content.Page"></a>

### Change.Block.Content.Page



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Model.Block.Content.Page.Style](#anytype.Model.Block.Content.Page.Style) |  |  |
| block | [Model.Block](#anytype.Model.Block) |  |  |






<a name="anytype.Change.Block.Content.Text"></a>

### Change.Block.Content.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [Model.Block.Content.Text.Style](#anytype.Model.Block.Content.Text.Style) |  |  |
| marks | [Model.Block.Content.Text.Marks](#anytype.Model.Block.Content.Text.Marks) |  |  |
| toggleable | [bool](#bool) |  |  |
| markerType | [Model.Block.Content.Text.MarkerType](#anytype.Model.Block.Content.Text.MarkerType) |  |  |
| checkable | [bool](#bool) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype.Change.Block.Content.Video"></a>

### Change.Block.Content.Video



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| content | [string](#string) |  |  |
| state | [Model.Block.Content.Video.State](#anytype.Model.Block.Content.Video.State) |  |  |
| preview | [Model.Block.Content.Video.Preview](#anytype.Model.Block.Content.Video.Preview) |  |  |






<a name="anytype.Change.Block.Fields"></a>

### Change.Block.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fields | [Model.Struct](#anytype.Model.Struct) |  |  |






<a name="anytype.Change.Block.Permissions"></a>

### Change.Block.Permissions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| permissions | [Model.Block.Permissions](#anytype.Model.Block.Permissions) |  |  |






<a name="anytype.Change.Multiple"></a>

### Change.Multiple







<a name="anytype.Change.Multiple.BlocksList"></a>

### Change.Multiple.BlocksList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changes | [Change.Single.BlocksList](#anytype.Change.Single.BlocksList) | repeated |  |






<a name="anytype.Change.Single"></a>

### Change.Single







<a name="anytype.Change.Single.BlocksList"></a>

### Change.Single.BlocksList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | repeated |  |
| text | [Change.Block.Content.Text](#anytype.Change.Block.Content.Text) |  |  |
| fields | [Change.Block.Fields](#anytype.Change.Block.Fields) |  |  |
| premissions | [Change.Block.Permissions](#anytype.Change.Block.Permissions) |  |  |
| children | [Change.Block.Children](#anytype.Change.Block.Children) |  |  |
| page | [Change.Block.Content.Page](#anytype.Change.Block.Content.Page) |  |  |
| dashboard | [Change.Block.Content.Dashboard](#anytype.Change.Block.Content.Dashboard) |  |  |
| video | [Change.Block.Content.Video](#anytype.Change.Block.Content.Video) |  |  |
| image | [Change.Block.Content.Image](#anytype.Change.Block.Content.Image) |  |  |
| file | [Change.Block.Content.File](#anytype.Change.Block.Content.File) |  |  |





 

 

 

 



<a name="commands.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## commands.proto



<a name="anytype.Rpc"></a>

### Rpc







<a name="anytype.Rpc.Account"></a>

### Rpc.Account







<a name="anytype.Rpc.Account.Create"></a>

### Rpc.Account.Create







<a name="anytype.Rpc.Account.Create.Request"></a>

### Rpc.Account.Create.Request
Front end to middleware request-to-create-an account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Account name |
| avatarLocalPath | [string](#string) |  | Path to an image, that will be used as an avatar of this account |
| avatarColor | [string](#string) |  | Avatar color as an alternative for avatar image |






<a name="anytype.Rpc.Account.Create.Response"></a>

### Rpc.Account.Create.Response
Middleware-to-front-end response for an account creation request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error) |  | Error while trying to create an account |
| account | [Model.Account](#anytype.Model.Account) |  | A newly created account; In case of a failure, i.e. error is non-NULL, the account model should contain empty/default-value fields |






<a name="anytype.Rpc.Account.Create.Response.Error"></a>

### Rpc.Account.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Create.Response.Error.Code](#anytype.Rpc.Account.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Account.Recover"></a>

### Rpc.Account.Recover







<a name="anytype.Rpc.Account.Recover.Request"></a>

### Rpc.Account.Recover.Request
Front end to middleware request-to-start-search of an accounts for a recovered mnemonic. 
Each of an account that would be found will come with an AccountAdd event






<a name="anytype.Rpc.Account.Recover.Response"></a>

### Rpc.Account.Recover.Response
Middleware-to-front-end response to an account recover request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Recover.Response.Error](#anytype.Rpc.Account.Recover.Response.Error) |  | Error while trying to recover an account |






<a name="anytype.Rpc.Account.Recover.Response.Error"></a>

### Rpc.Account.Recover.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Recover.Response.Error.Code](#anytype.Rpc.Account.Recover.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Account.Select"></a>

### Rpc.Account.Select







<a name="anytype.Rpc.Account.Select.Request"></a>

### Rpc.Account.Select.Request
Front end to middleware request-to-launch-a specific account using account id and a root path
User can select an account from those, that came with an AccountAdd events


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Id of a selected account |
| rootPath | [string](#string) |  | Root path is optional, set if this is a first request |






<a name="anytype.Rpc.Account.Select.Response"></a>

### Rpc.Account.Select.Response
Middleware-to-front-end response for an account select request, that can contain a NULL error and selected account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Select.Response.Error](#anytype.Rpc.Account.Select.Response.Error) |  | Error while trying to launch/select an account |
| account | [Model.Account](#anytype.Model.Account) |  | Selected account |






<a name="anytype.Rpc.Account.Select.Response.Error"></a>

### Rpc.Account.Select.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Select.Response.Error.Code](#anytype.Rpc.Account.Select.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block"></a>

### Rpc.Block







<a name="anytype.Rpc.Block.Create"></a>

### Rpc.Block.Create







<a name="anytype.Rpc.Block.Create.Request"></a>

### Rpc.Block.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| block | [Model.Block](#anytype.Model.Block) |  |  |
| contextBlockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Create.Response"></a>

### Rpc.Block.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Create.Response.Error"></a>

### Rpc.Block.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.History"></a>

### Rpc.Block.History







<a name="anytype.Rpc.Block.History.Move"></a>

### Rpc.Block.History.Move







<a name="anytype.Rpc.Block.History.Move.Request"></a>

### Rpc.Block.History.Move.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| contextBlockId | [string](#string) |  |  |
| moveForward | [bool](#bool) |  |  |






<a name="anytype.Rpc.Block.History.Move.Response"></a>

### Rpc.Block.History.Move.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.History.Move.Response.Error](#anytype.Rpc.Block.History.Move.Response.Error) |  |  |






<a name="anytype.Rpc.Block.History.Move.Response.Error"></a>

### Rpc.Block.History.Move.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.History.Move.Response.Error.Code](#anytype.Rpc.Block.History.Move.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Open"></a>

### Rpc.Block.Open







<a name="anytype.Rpc.Block.Open.Request"></a>

### Rpc.Block.Open.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Open.Response"></a>

### Rpc.Block.Open.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Open.Response.Error"></a>

### Rpc.Block.Open.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Update"></a>

### Rpc.Block.Update







<a name="anytype.Rpc.Block.Update.Request"></a>

### Rpc.Block.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changes | [Change.Multiple.BlocksList](#anytype.Change.Multiple.BlocksList) |  |  |






<a name="anytype.Rpc.Block.Update.Response"></a>

### Rpc.Block.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Update.Response.Error](#anytype.Rpc.Block.Update.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Update.Response.Error"></a>

### Rpc.Block.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Update.Response.Error.Code](#anytype.Rpc.Block.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Image"></a>

### Rpc.Image







<a name="anytype.Rpc.Image.Get"></a>

### Rpc.Image.Get







<a name="anytype.Rpc.Image.Get.Blob"></a>

### Rpc.Image.Get.Blob







<a name="anytype.Rpc.Image.Get.Blob.Request"></a>

### Rpc.Image.Get.Blob.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| size | [Model.Image.Size](#anytype.Model.Image.Size) |  |  |






<a name="anytype.Rpc.Image.Get.Blob.Response"></a>

### Rpc.Image.Get.Blob.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Image.Get.Blob.Response.Error](#anytype.Rpc.Image.Get.Blob.Response.Error) |  |  |
| blob | [bytes](#bytes) |  |  |






<a name="anytype.Rpc.Image.Get.Blob.Response.Error"></a>

### Rpc.Image.Get.Blob.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Image.Get.Blob.Response.Error.Code](#anytype.Rpc.Image.Get.Blob.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Image.Get.File"></a>

### Rpc.Image.Get.File







<a name="anytype.Rpc.Image.Get.File.Request"></a>

### Rpc.Image.Get.File.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| size | [Model.Image.Size](#anytype.Model.Image.Size) |  |  |






<a name="anytype.Rpc.Image.Get.File.Response"></a>

### Rpc.Image.Get.File.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Image.Get.File.Response.Error](#anytype.Rpc.Image.Get.File.Response.Error) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype.Rpc.Image.Get.File.Response.Error"></a>

### Rpc.Image.Get.File.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Image.Get.File.Response.Error.Code](#anytype.Rpc.Image.Get.File.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs"></a>

### Rpc.Ipfs







<a name="anytype.Rpc.Ipfs.Get"></a>

### Rpc.Ipfs.Get







<a name="anytype.Rpc.Ipfs.Get.File"></a>

### Rpc.Ipfs.Get.File







<a name="anytype.Rpc.Ipfs.Get.File.Request"></a>

### Rpc.Ipfs.Get.File.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.Get.File.Response"></a>

### Rpc.Ipfs.Get.File.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Ipfs.Get.File.Response.Error](#anytype.Rpc.Ipfs.Get.File.Response.Error) |  |  |
| data | [bytes](#bytes) |  |  |
| media | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.Get.File.Response.Error"></a>

### Rpc.Ipfs.Get.File.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Ipfs.Get.File.Response.Error.Code](#anytype.Rpc.Ipfs.Get.File.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Log"></a>

### Rpc.Log







<a name="anytype.Rpc.Log.Send"></a>

### Rpc.Log.Send







<a name="anytype.Rpc.Log.Send.Request"></a>

### Rpc.Log.Send.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| message | [string](#string) |  |  |
| level | [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level) |  |  |






<a name="anytype.Rpc.Log.Send.Response"></a>

### Rpc.Log.Send.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error) |  |  |






<a name="anytype.Rpc.Log.Send.Response.Error"></a>

### Rpc.Log.Send.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Version"></a>

### Rpc.Version







<a name="anytype.Rpc.Version.Get"></a>

### Rpc.Version.Get







<a name="anytype.Rpc.Version.Get.Request"></a>

### Rpc.Version.Get.Request







<a name="anytype.Rpc.Version.Get.Response"></a>

### Rpc.Version.Get.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Version.Get.Response.Error](#anytype.Rpc.Version.Get.Response.Error) |  |  |
| version | [string](#string) |  |  |






<a name="anytype.Rpc.Version.Get.Response.Error"></a>

### Rpc.Version.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Wallet"></a>

### Rpc.Wallet







<a name="anytype.Rpc.Wallet.Create"></a>

### Rpc.Wallet.Create







<a name="anytype.Rpc.Wallet.Create.Request"></a>

### Rpc.Wallet.Create.Request
Front-end-to-middleware request to create a new wallet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) |  | Path to a wallet directory |






<a name="anytype.Rpc.Wallet.Create.Response"></a>

### Rpc.Wallet.Create.Response
Middleware-to-front-end response, that can contain mnemonic of a created account and a NULL error or an empty mnemonic and a non-NULL error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Create.Response.Error](#anytype.Rpc.Wallet.Create.Response.Error) |  |  |
| mnemonic | [string](#string) |  | Mnemonic of a new account (sequence of words, divided by spaces) |






<a name="anytype.Rpc.Wallet.Create.Response.Error"></a>

### Rpc.Wallet.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Wallet.Recover"></a>

### Rpc.Wallet.Recover







<a name="anytype.Rpc.Wallet.Recover.Request"></a>

### Rpc.Wallet.Recover.Request
Front end to middleware request-to-recover-a wallet with this mnemonic and a rootPath


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) |  | Path to a wallet directory |
| mnemonic | [string](#string) |  | Mnemonic of a wallet to recover |






<a name="anytype.Rpc.Wallet.Recover.Response"></a>

### Rpc.Wallet.Recover.Response
Middleware-to-front-end response, that can contain a NULL error or a non-NULL error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Recover.Response.Error](#anytype.Rpc.Wallet.Recover.Response.Error) |  | Error while trying to recover a wallet |






<a name="anytype.Rpc.Wallet.Recover.Response.Error"></a>

### Rpc.Wallet.Recover.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |





 


<a name="anytype.Rpc.Account.Create.Response.Error.Code"></a>

### Rpc.Account.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; Account should be non-empty |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Avatar or name is not correct |
| ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE | 101 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME | 102 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR | 103 |  |



<a name="anytype.Rpc.Account.Recover.Response.Error.Code"></a>

### Rpc.Account.Recover.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |
| NO_ACCOUNTS_FOUND | 101 |  |
| NEED_TO_RECOVER_WALLET_FIRST | 102 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 103 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 104 |  |
| FAILED_TO_RUN_NODE | 105 |  |
| WALLET_RECOVER_NOT_PERFORMED | 106 |  |



<a name="anytype.Rpc.Account.Select.Response.Error.Code"></a>

### Rpc.Account.Select.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Id or root path is wrong |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 102 |  |
| FAILED_TO_RUN_NODE | 103 |  |
| FAILED_TO_FIND_ACCOUNT_INFO | 104 |  |
| LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET | 105 |  |



<a name="anytype.Rpc.Block.Create.Response.Error.Code"></a>

### Rpc.Block.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.History.Move.Response.Error.Code"></a>

### Rpc.Block.History.Move.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype.Rpc.Block.Open.Response.Error.Code"></a>

### Rpc.Block.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Update.Response.Error.Code"></a>

### Rpc.Block.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Image.Get.Blob.Response.Error.Code"></a>

### Rpc.Image.Get.Blob.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.Image.Get.File.Response.Error.Code"></a>

### Rpc.Image.Get.File.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.Ipfs.Get.File.Response.Error.Code"></a>

### Rpc.Ipfs.Get.File.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.Log.Send.Request.Level"></a>

### Rpc.Log.Send.Request.Level


| Name | Number | Description |
| ---- | ------ | ----------- |
| DEBUG | 0 |  |
| ERROR | 1 |  |
| FATAL | 2 |  |
| INFO | 3 |  |
| PANIC | 4 |  |
| WARNING | 5 |  |



<a name="anytype.Rpc.Log.Send.Response.Error.Code"></a>

### Rpc.Log.Send.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.Version.Get.Response.Error.Code"></a>

### Rpc.Version.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| VERSION_IS_EMPTY | 3 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.Wallet.Create.Response.Error.Code"></a>

### Rpc.Wallet.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; mnemonic should be non-empty |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Root path is wrong |
| FAILED_TO_CREATE_LOCAL_REPO | 101 | ... |



<a name="anytype.Rpc.Wallet.Recover.Response.Error.Code"></a>

### Rpc.Wallet.Recover.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; wallet successfully recovered |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Root path or mnemonic is wrong |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |


 

 

 



<a name="events.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## events.proto



<a name="anytype.Event"></a>

### Event



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountShow | [Event.Account.Show](#anytype.Event.Account.Show) |  | show wallet&#39;s accounts that were loaded from local or remote source |
| blockShow | [Event.Block.Show](#anytype.Event.Block.Show) |  |  |
| blockUpdate | [Event.Block.Update](#anytype.Event.Block.Update) |  |  |
| blockCreate | [Event.Block.Create](#anytype.Event.Block.Create) |  |  |
| userBlockTextRange | [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange) |  |  |
| userBlockJoin | [Event.User.Block.Join](#anytype.Event.User.Block.Join) |  |  |
| userBlockLeft | [Event.User.Block.Left](#anytype.Event.User.Block.Left) |  |  |
| userBlockSelectRange | [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange) |  |  |
| filesUpload | [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload) |  |  |






<a name="anytype.Event.Account"></a>

### Event.Account







<a name="anytype.Event.Account.Show"></a>

### Event.Account.Show
Message, that will be sent to the front on each account found after an AccountRecoverRequest


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int64](#int64) |  | Number of an account in an all found accounts list |
| account | [Model.Account](#anytype.Model.Account) |  | An Account, that has been found for the mnemonic |






<a name="anytype.Event.Block"></a>

### Event.Block







<a name="anytype.Event.Block.Create"></a>

### Event.Block.Create



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| block | [Model.Block](#anytype.Model.Block) |  |  |






<a name="anytype.Event.Block.FilesUpload"></a>

### Event.Block.FilesUpload
Middleware to front end event message, that will be sent on one of this scenarios:
Precondition: user A opened a block
1. User A drops a set of files/pictures/videos
2. User A creates a MediaBlock and drops a single media, that corresponds to its type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filePath | [string](#string) | repeated | filepaths to the files |
| blockId | [string](#string) |  | if empty =&gt; create new blocks |






<a name="anytype.Event.Block.Show"></a>

### Event.Block.Show



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| block | [Model.Block](#anytype.Model.Block) |  |  |






<a name="anytype.Event.Block.Update"></a>

### Event.Block.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changes | [Change.Multiple.BlocksList](#anytype.Change.Multiple.BlocksList) |  |  |






<a name="anytype.Event.User"></a>

### Event.User







<a name="anytype.Event.User.Block"></a>

### Event.User.Block







<a name="anytype.Event.User.Block.Join"></a>

### Event.User.Block.Join
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A opened a block
1. User B opens the same block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) |  | Account of the user, that opened a block |






<a name="anytype.Event.User.Block.Left"></a>

### Event.User.Block.Left
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B closes the block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) |  | Account of the user, that left the block |






<a name="anytype.Event.User.Block.SelectRange"></a>

### Event.User.Block.SelectRange
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B selects some inner blocks
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) |  | Account of the user, that selected blocks |
| blockIdsArray | [string](#string) | repeated | Ids of selected blocks. |






<a name="anytype.Event.User.Block.TextRange"></a>

### Event.User.Block.TextRange
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B sets cursor or selects a text region into a text block 
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) |  | Account of the user, that selected a text |
| blockId | [string](#string) |  | Id of the text block, that have a selection |
| range | [Model.Range](#anytype.Model.Range) |  | Range of the selection |





 

 

 

 



<a name="models.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## models.proto



<a name="anytype.Model"></a>

### Model







<a name="anytype.Model.Account"></a>

### Model.Account
Contains basic information about user account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | User&#39;s thread id |
| name | [string](#string) |  | User name, that associated with this account |
| avatar | [Model.Account.Avatar](#anytype.Model.Account.Avatar) |  | Avatar of a user&#39;s account |






<a name="anytype.Model.Account.Avatar"></a>

### Model.Account.Avatar
Avatar of a user&#39;s account. It could be an image or color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| image | [Model.Image](#anytype.Model.Image) |  | Image of the avatar. Contains hash and size |
| color | [string](#string) |  | Color of the avatar, if no image |






<a name="anytype.Model.Block"></a>

### Model.Block



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [Model.Struct](#anytype.Model.Struct) |  |  |
| permissions | [Model.Block.Permissions](#anytype.Model.Block.Permissions) |  |  |
| children | [string](#string) | repeated |  |
| dashboard | [Model.Block.Content.Dashboard](#anytype.Model.Block.Content.Dashboard) |  |  |
| page | [Model.Block.Content.Page](#anytype.Model.Block.Content.Page) |  |  |
| dataview | [Model.Block.Content.Dataview](#anytype.Model.Block.Content.Dataview) |  |  |
| text | [Model.Block.Content.Text](#anytype.Model.Block.Content.Text) |  |  |
| video | [Model.Block.Content.Video](#anytype.Model.Block.Content.Video) |  |  |
| image | [Model.Block.Content.Image](#anytype.Model.Block.Content.Image) |  |  |
| file | [Model.Block.Content.File](#anytype.Model.Block.Content.File) |  |  |
| layout | [Model.Block.Content.Layout](#anytype.Model.Block.Content.Layout) |  |  |
| div | [Model.Block.Content.Div](#anytype.Model.Block.Content.Div) |  |  |






<a name="anytype.Model.Block.Content"></a>

### Model.Block.Content







<a name="anytype.Model.Block.Content.Dashboard"></a>

### Model.Block.Content.Dashboard



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Model.Block.Content.Dashboard.Style](#anytype.Model.Block.Content.Dashboard.Style) |  |  |






<a name="anytype.Model.Block.Content.Dataview"></a>

### Model.Block.Content.Dataview
...






<a name="anytype.Model.Block.Content.Div"></a>

### Model.Block.Content.Div







<a name="anytype.Model.Block.Content.File"></a>

### Model.Block.Content.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| localFilePath | [string](#string) |  |  |
| state | [Model.Block.Content.File.State](#anytype.Model.Block.Content.File.State) |  |  |
| preview | [Model.Block.Content.File.Preview](#anytype.Model.Block.Content.File.Preview) |  |  |






<a name="anytype.Model.Block.Content.File.Preview"></a>

### Model.Block.Content.File.Preview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| icon | [string](#string) |  |  |






<a name="anytype.Model.Block.Content.Image"></a>

### Model.Block.Content.Image



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| localFilePath | [string](#string) |  |  |
| state | [Model.Block.Content.Image.State](#anytype.Model.Block.Content.Image.State) |  |  |
| preview | [Model.Block.Content.Image.Preview](#anytype.Model.Block.Content.Image.Preview) |  |  |






<a name="anytype.Model.Block.Content.Image.Preview"></a>

### Model.Block.Content.Image.Preview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| thumbnail | [bytes](#bytes) |  |  |
| name | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype.Model.Block.Content.Layout"></a>

### Model.Block.Content.Layout



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Model.Block.Content.Layout.Style](#anytype.Model.Block.Content.Layout.Style) |  |  |






<a name="anytype.Model.Block.Content.Page"></a>

### Model.Block.Content.Page



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Model.Block.Content.Page.Style](#anytype.Model.Block.Content.Page.Style) |  |  |






<a name="anytype.Model.Block.Content.Text"></a>

### Model.Block.Content.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [Model.Block.Content.Text.Style](#anytype.Model.Block.Content.Text.Style) |  |  |
| marksList | [Model.Block.Content.Text.Marks](#anytype.Model.Block.Content.Text.Marks) |  |  |
| toggleable | [bool](#bool) |  |  |
| markerType | [Model.Block.Content.Text.MarkerType](#anytype.Model.Block.Content.Text.MarkerType) |  |  |
| checkable | [bool](#bool) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype.Model.Block.Content.Text.Mark"></a>

### Model.Block.Content.Text.Mark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Model.Range](#anytype.Model.Range) |  |  |
| type | [Model.Block.Content.Text.Mark.Type](#anytype.Model.Block.Content.Text.Mark.Type) |  |  |
| param | [string](#string) |  | link, color, etc |






<a name="anytype.Model.Block.Content.Text.Marks"></a>

### Model.Block.Content.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marks | [Model.Block.Content.Text.Mark](#anytype.Model.Block.Content.Text.Mark) | repeated |  |






<a name="anytype.Model.Block.Content.Video"></a>

### Model.Block.Content.Video



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| localFilePath | [string](#string) |  |  |
| state | [Model.Block.Content.Video.State](#anytype.Model.Block.Content.Video.State) |  |  |
| preview | [Model.Block.Content.Video.Preview](#anytype.Model.Block.Content.Video.Preview) |  |  |






<a name="anytype.Model.Block.Content.Video.Preview"></a>

### Model.Block.Content.Video.Preview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| thumbnail | [bytes](#bytes) |  |  |
| name | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype.Model.Block.Permissions"></a>

### Model.Block.Permissions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| read | [bool](#bool) |  |  |
| edit | [bool](#bool) |  |  |
| remove | [bool](#bool) |  |  |
| drag | [bool](#bool) |  |  |
| dropOn | [bool](#bool) |  |  |






<a name="anytype.Model.Image"></a>

### Model.Image



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| sizes | [Model.Image.Size](#anytype.Model.Image.Size) | repeated |  |






<a name="anytype.Model.Range"></a>

### Model.Range



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) |  |  |
| to | [int32](#int32) |  |  |






<a name="anytype.Model.Struct"></a>

### Model.Struct



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fields | [Model.Struct.FieldsEntry](#anytype.Model.Struct.FieldsEntry) | repeated | Unordered map of dynamically typed values. |






<a name="anytype.Model.Struct.FieldsEntry"></a>

### Model.Struct.FieldsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [Model.Struct.Value](#anytype.Model.Struct.Value) |  |  |






<a name="anytype.Model.Struct.ListValue"></a>

### Model.Struct.ListValue
`ListValue` is a wrapper around a repeated field of values.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| values | [Model.Struct.Value](#anytype.Model.Struct.Value) | repeated |  |






<a name="anytype.Model.Struct.Value"></a>

### Model.Struct.Value
`Value` represents a dynamically typed value which can be either
null, a number, a string, a boolean, a recursive struct value, or a
list of values. A producer of value is expected to set one of that
variants, absence of any variant indicates an error.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| null_value | [Model.Struct.NullValue](#anytype.Model.Struct.NullValue) |  | Represents a null value. |
| number_value | [double](#double) |  | Represents a double value. |
| string_value | [string](#string) |  | Represents a string value. |
| bool_value | [bool](#bool) |  | Represents a boolean value. |
| struct_value | [Model.Struct](#anytype.Model.Struct) |  | Represents a structured value. |
| list_value | [Model.Struct.ListValue](#anytype.Model.Struct.ListValue) |  | Represents a repeated `Value`. |






<a name="anytype.Model.Video"></a>

### Model.Video



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| sizes | [Model.Video.Size](#anytype.Model.Video.Size) | repeated |  |





 


<a name="anytype.Model.Block.Content.Dashboard.Style"></a>

### Model.Block.Content.Dashboard.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| MAIN_SCREEN | 0 | ... |



<a name="anytype.Model.Block.Content.File.State"></a>

### Model.Block.Content.File.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| EMPTY | 0 |  |
| UPLOADING | 1 |  |
| PREVIEW | 2 |  |
| DOWNLOADING | 3 |  |
| DONE | 4 |  |



<a name="anytype.Model.Block.Content.Image.State"></a>

### Model.Block.Content.Image.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| EMPTY | 0 |  |
| UPLOADING | 1 |  |
| PREVIEW | 2 |  |
| DOWNLOADING | 3 |  |
| DONE | 4 |  |



<a name="anytype.Model.Block.Content.Layout.Style"></a>

### Model.Block.Content.Layout.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| ROW | 0 |  |
| COLUMN | 1 |  |



<a name="anytype.Model.Block.Content.Page.Style"></a>

### Model.Block.Content.Page.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| EMPTY | 0 |  |
| TASK | 1 |  |
| BOOKMARK | 2 |  |
| SET | 3 | ... |



<a name="anytype.Model.Block.Content.Text.Mark.Type"></a>

### Model.Block.Content.Text.Mark.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| STRIKETHROUGH | 0 |  |
| KEYBOARD | 1 |  |
| ITALIC | 2 |  |
| BOLD | 3 |  |
| LINK | 4 |  |



<a name="anytype.Model.Block.Content.Text.MarkerType"></a>

### Model.Block.Content.Text.MarkerType


| Name | Number | Description |
| ---- | ------ | ----------- |
| none | 0 |  |
| number | 1 |  |
| bullet | 2 |  |



<a name="anytype.Model.Block.Content.Text.Style"></a>

### Model.Block.Content.Text.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| p | 0 |  |
| h1 | 1 |  |
| h2 | 2 |  |
| h3 | 3 |  |
| h4 | 4 |  |
| quote | 5 |  |
| code | 6 |  |



<a name="anytype.Model.Block.Content.Video.State"></a>

### Model.Block.Content.Video.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| EMPTY | 0 |  |
| UPLOADING | 1 |  |
| PREVIEW | 2 |  |
| DOWNLOADING | 3 |  |
| DONE | 4 |  |



<a name="anytype.Model.Block.Type"></a>

### Model.Block.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| DASHBOARD | 0 |  |
| PAGE | 1 |  |
| DATAVIEW | 2 |  |
| TEXT | 3 |  |
| FILE | 4 |  |
| PICTURE | 5 |  |
| VIDEO | 6 |  |
| BOOKMARK | 7 |  |
| LAYOUT | 8 |  |
| DIV | 9 |  |



<a name="anytype.Model.Image.Size"></a>

### Model.Image.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| LARGE | 0 |  |
| SMALL | 1 |  |
| THUMB | 2 |  |



<a name="anytype.Model.Struct.NullValue"></a>

### Model.Struct.NullValue
`NullValue` is a singleton enumeration to represent the null value for the

| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL_VALUE | 0 |  |



<a name="anytype.Model.Video.Size"></a>

### Model.Video.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| SD_360p | 0 |  |
| SD_480p | 1 |  |
| HD_720p | 2 |  |
| HD_1080p | 3 |  |
| UHD_1440p | 4 |  |
| UHD_2160p | 5 |  |


 

 

 



## Scalar Value Types

| .proto Type | Notes | C++ Type | Java Type | Python Type |
| ----------- | ----- | -------- | --------- | ----------- |
| <a name="double" /> double |  | double | double | float |
| <a name="float" /> float |  | float | float | float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long |
| <a name="bool" /> bool |  | bool | boolean | boolean |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str |

