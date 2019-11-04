# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [service/service.proto](#service/service.proto)
  
  
  
    - [ClientCommands](#anytype.ClientCommands)
  

- [account.proto](#account.proto)
    - [Account](#anytype.Account)
    - [AccountCreateRequest](#anytype.AccountCreateRequest)
    - [AccountCreateResponse](#anytype.AccountCreateResponse)
    - [AccountCreateResponse.Error](#anytype.AccountCreateResponse.Error)
    - [AccountRecoverRequest](#anytype.AccountRecoverRequest)
    - [AccountRecoverResponse](#anytype.AccountRecoverResponse)
    - [AccountRecoverResponse.Error](#anytype.AccountRecoverResponse.Error)
    - [AccountSelectRequest](#anytype.AccountSelectRequest)
    - [AccountSelectResponse](#anytype.AccountSelectResponse)
    - [AccountSelectResponse.Error](#anytype.AccountSelectResponse.Error)
    - [AccountShow](#anytype.AccountShow)
    - [AccountStartRequest](#anytype.AccountStartRequest)
    - [AccountStartResponse](#anytype.AccountStartResponse)
    - [AccountStartResponse.Error](#anytype.AccountStartResponse.Error)
    - [Avatar](#anytype.Avatar)
    - [WalletCreateRequest](#anytype.WalletCreateRequest)
    - [WalletCreateResponse](#anytype.WalletCreateResponse)
    - [WalletCreateResponse.Error](#anytype.WalletCreateResponse.Error)
    - [WalletRecoverRequest](#anytype.WalletRecoverRequest)
    - [WalletRecoverResponse](#anytype.WalletRecoverResponse)
    - [WalletRecoverResponse.Error](#anytype.WalletRecoverResponse.Error)
  
    - [AccountCreateResponse.Error.Code](#anytype.AccountCreateResponse.Error.Code)
    - [AccountRecoverResponse.Error.Code](#anytype.AccountRecoverResponse.Error.Code)
    - [AccountSelectResponse.Error.Code](#anytype.AccountSelectResponse.Error.Code)
    - [AccountStartResponse.Error.Code](#anytype.AccountStartResponse.Error.Code)
    - [WalletCreateResponse.Error.Code](#anytype.WalletCreateResponse.Error.Code)
    - [WalletRecoverResponse.Error.Code](#anytype.WalletRecoverResponse.Error.Code)
  
  
  

- [block.proto](#block.proto)
    - [Block](#anytype.Block)
    - [BlockAtomicChange](#anytype.BlockAtomicChange)
    - [BlockChanges](#anytype.BlockChanges)
    - [BlockConnections](#anytype.BlockConnections)
    - [BlockConnectionsList](#anytype.BlockConnectionsList)
    - [BlockContentDashboard](#anytype.BlockContentDashboard)
    - [BlockContentDashboardChange](#anytype.BlockContentDashboardChange)
    - [BlockContentDataview](#anytype.BlockContentDataview)
    - [BlockContentMedia](#anytype.BlockContentMedia)
    - [BlockContentPage](#anytype.BlockContentPage)
    - [BlockContentPageChange](#anytype.BlockContentPageChange)
    - [BlockContentText](#anytype.BlockContentText)
    - [BlockContentText.Mark](#anytype.BlockContentText.Mark)
    - [BlockContentText.Marks](#anytype.BlockContentText.Marks)
    - [BlockContentTextChange](#anytype.BlockContentTextChange)
    - [BlockCreate](#anytype.BlockCreate)
    - [BlockCreateRequest](#anytype.BlockCreateRequest)
    - [BlockCreateResponse](#anytype.BlockCreateResponse)
    - [BlockCreateResponse.Error](#anytype.BlockCreateResponse.Error)
    - [BlockHeader](#anytype.BlockHeader)
    - [BlockHeaderChange](#anytype.BlockHeaderChange)
    - [BlockHeadersList](#anytype.BlockHeadersList)
    - [BlockOpenRequest](#anytype.BlockOpenRequest)
    - [BlockOpenResponse](#anytype.BlockOpenResponse)
    - [BlockOpenResponse.Error](#anytype.BlockOpenResponse.Error)
    - [BlockPermissions](#anytype.BlockPermissions)
    - [BlockShow](#anytype.BlockShow)
    - [BlockUpdate](#anytype.BlockUpdate)
    - [BlockUpdateRequest](#anytype.BlockUpdateRequest)
    - [BlockUpdateResponse](#anytype.BlockUpdateResponse)
    - [BlockUpdateResponse.Error](#anytype.BlockUpdateResponse.Error)
    - [BlocksList](#anytype.BlocksList)
    - [Range](#anytype.Range)
  
    - [BlockContentDashboard.Style](#anytype.BlockContentDashboard.Style)
    - [BlockContentPage.Style](#anytype.BlockContentPage.Style)
    - [BlockContentText.Mark.Type](#anytype.BlockContentText.Mark.Type)
    - [BlockContentText.MarkerType](#anytype.BlockContentText.MarkerType)
    - [BlockContentText.Style](#anytype.BlockContentText.Style)
    - [BlockCreateResponse.Error.Code](#anytype.BlockCreateResponse.Error.Code)
    - [BlockOpenResponse.Error.Code](#anytype.BlockOpenResponse.Error.Code)
    - [BlockType](#anytype.BlockType)
    - [BlockUpdateResponse.Error.Code](#anytype.BlockUpdateResponse.Error.Code)
  
  
  

- [edit.proto](#edit.proto)
    - [UserBlockFocus](#anytype.UserBlockFocus)
    - [UserBlockJoin](#anytype.UserBlockJoin)
    - [UserBlockLeft](#anytype.UserBlockLeft)
    - [UserBlockSelectRange](#anytype.UserBlockSelectRange)
    - [UserBlockTextRange](#anytype.UserBlockTextRange)
  
  
  
  

- [event.proto](#event.proto)
    - [Event](#anytype.Event)
  
  
  
  

- [file.proto](#file.proto)
    - [Image](#anytype.Image)
    - [ImageGetBlobRequest](#anytype.ImageGetBlobRequest)
    - [ImageGetBlobResponse](#anytype.ImageGetBlobResponse)
    - [ImageGetBlobResponse.Error](#anytype.ImageGetBlobResponse.Error)
    - [ImageGetFileRequest](#anytype.ImageGetFileRequest)
    - [ImageGetFileResponse](#anytype.ImageGetFileResponse)
    - [ImageGetFileResponse.Error](#anytype.ImageGetFileResponse.Error)
    - [IpfsGetFileRequest](#anytype.IpfsGetFileRequest)
    - [IpfsGetFileResponse](#anytype.IpfsGetFileResponse)
    - [IpfsGetFileResponse.Error](#anytype.IpfsGetFileResponse.Error)
    - [Video](#anytype.Video)
  
    - [ImageGetBlobResponse.Error.Code](#anytype.ImageGetBlobResponse.Error.Code)
    - [ImageGetFileResponse.Error.Code](#anytype.ImageGetFileResponse.Error.Code)
    - [ImageSize](#anytype.ImageSize)
    - [IpfsGetFileResponse.Error.Code](#anytype.IpfsGetFileResponse.Error.Code)
    - [VideoSize](#anytype.VideoSize)
  
  
  

- [misc.proto](#misc.proto)
    - [LogSendRequest](#anytype.LogSendRequest)
    - [LogSendResponse](#anytype.LogSendResponse)
    - [LogSendResponse.Error](#anytype.LogSendResponse.Error)
    - [VersionGetRequest](#anytype.VersionGetRequest)
    - [VersionGetResponse](#anytype.VersionGetResponse)
    - [VersionGetResponse.Error](#anytype.VersionGetResponse.Error)
  
    - [LogSendRequest.Level](#anytype.LogSendRequest.Level)
    - [LogSendResponse.Error.Code](#anytype.LogSendResponse.Error.Code)
    - [VersionGetResponse.Error.Code](#anytype.VersionGetResponse.Error.Code)
  
  
  

- [struct.proto](#struct.proto)
    - [ListValue](#anytype.ListValue)
    - [Struct](#anytype.Struct)
    - [Struct.FieldsEntry](#anytype.Struct.FieldsEntry)
    - [Value](#anytype.Value)
  
    - [NullValue](#anytype.NullValue)
  
  
  

- [Scalar Value Types](#scalar-value-types)



<a name="service/service.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## service/service.proto


 

 

 


<a name="anytype.ClientCommands"></a>

### ClientCommands


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| WalletCreate | [WalletCreateRequest](#anytype.WalletCreateRequest) | [WalletCreateResponse](#anytype.WalletCreateResponse) |  |
| WalletRecover | [WalletRecoverRequest](#anytype.WalletRecoverRequest) | [WalletRecoverResponse](#anytype.WalletRecoverResponse) |  |
| AccountRecover | [AccountRecoverRequest](#anytype.AccountRecoverRequest) | [AccountRecoverResponse](#anytype.AccountRecoverResponse) |  |
| AccountCreate | [AccountCreateRequest](#anytype.AccountCreateRequest) | [AccountCreateResponse](#anytype.AccountCreateResponse) |  |
| AccountSelect | [AccountSelectRequest](#anytype.AccountSelectRequest) | [AccountSelectResponse](#anytype.AccountSelectResponse) |  |
| ImageGetBlob | [ImageGetBlobRequest](#anytype.ImageGetBlobRequest) | [ImageGetBlobResponse](#anytype.ImageGetBlobResponse) |  |
| VersionGet | [VersionGetRequest](#anytype.VersionGetRequest) | [VersionGetResponse](#anytype.VersionGetResponse) |  |
| LogSend | [LogSendRequest](#anytype.LogSendRequest) | [LogSendResponse](#anytype.LogSendResponse) |  |
| BlockOpen | [BlockOpenRequest](#anytype.BlockOpenRequest) | [BlockOpenResponse](#anytype.BlockOpenResponse) |  |
| BlockCreate | [BlockCreateRequest](#anytype.BlockCreateRequest) | [BlockCreateResponse](#anytype.BlockCreateResponse) |  |
| BlockUpdate | [BlockUpdateRequest](#anytype.BlockUpdateRequest) | [BlockUpdateResponse](#anytype.BlockUpdateResponse) |  |

 



<a name="account.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## account.proto



<a name="anytype.Account"></a>

### Account



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| avatar | [Avatar](#anytype.Avatar) |  |  |






<a name="anytype.AccountCreateRequest"></a>

### AccountCreateRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| avatarLocalPath | [string](#string) |  |  |
| avatarColor | [string](#string) |  |  |






<a name="anytype.AccountCreateResponse"></a>

### AccountCreateResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [AccountCreateResponse.Error](#anytype.AccountCreateResponse.Error) |  |  |
| account | [Account](#anytype.Account) |  |  |






<a name="anytype.AccountCreateResponse.Error"></a>

### AccountCreateResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [AccountCreateResponse.Error.Code](#anytype.AccountCreateResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.AccountRecoverRequest"></a>

### AccountRecoverRequest
Start accounts search for recovered mnemonic






<a name="anytype.AccountRecoverResponse"></a>

### AccountRecoverResponse
Found accounts will come in event AccountAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [AccountRecoverResponse.Error](#anytype.AccountRecoverResponse.Error) |  |  |






<a name="anytype.AccountRecoverResponse.Error"></a>

### AccountRecoverResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [AccountRecoverResponse.Error.Code](#anytype.AccountRecoverResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.AccountSelectRequest"></a>

### AccountSelectRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| rootPath | [string](#string) |  | optional, set if this is the first request |






<a name="anytype.AccountSelectResponse"></a>

### AccountSelectResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [AccountSelectResponse.Error](#anytype.AccountSelectResponse.Error) |  |  |
| account | [Account](#anytype.Account) |  |  |






<a name="anytype.AccountSelectResponse.Error"></a>

### AccountSelectResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [AccountSelectResponse.Error.Code](#anytype.AccountSelectResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.AccountShow"></a>

### AccountShow



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int64](#int64) |  |  |
| account | [Account](#anytype.Account) |  |  |






<a name="anytype.AccountStartRequest"></a>

### AccountStartRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.AccountStartResponse"></a>

### AccountStartResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [AccountStartResponse.Error](#anytype.AccountStartResponse.Error) |  |  |
| account | [Account](#anytype.Account) |  |  |






<a name="anytype.AccountStartResponse.Error"></a>

### AccountStartResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [AccountStartResponse.Error.Code](#anytype.AccountStartResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Avatar"></a>

### Avatar



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| image | [Image](#anytype.Image) |  |  |
| color | [string](#string) |  |  |






<a name="anytype.WalletCreateRequest"></a>

### WalletCreateRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) |  |  |






<a name="anytype.WalletCreateResponse"></a>

### WalletCreateResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [WalletCreateResponse.Error](#anytype.WalletCreateResponse.Error) |  |  |
| mnemonic | [string](#string) |  |  |






<a name="anytype.WalletCreateResponse.Error"></a>

### WalletCreateResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [WalletCreateResponse.Error.Code](#anytype.WalletCreateResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.WalletRecoverRequest"></a>

### WalletRecoverRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) |  |  |
| mnemonic | [string](#string) |  |  |






<a name="anytype.WalletRecoverResponse"></a>

### WalletRecoverResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [WalletRecoverResponse.Error](#anytype.WalletRecoverResponse.Error) |  |  |






<a name="anytype.WalletRecoverResponse.Error"></a>

### WalletRecoverResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [WalletRecoverResponse.Error.Code](#anytype.WalletRecoverResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |





 


<a name="anytype.AccountCreateResponse.Error.Code"></a>

### AccountCreateResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE | 101 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME | 102 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR | 103 |  |



<a name="anytype.AccountRecoverResponse.Error.Code"></a>

### AccountRecoverResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_ACCOUNTS_FOUND | 101 | ... |
| NEED_TO_RECOVER_WALLET_FIRST | 102 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 103 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 104 |  |
| FAILED_TO_RUN_NODE | 105 |  |



<a name="anytype.AccountSelectResponse.Error.Code"></a>

### AccountSelectResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 102 |  |
| FAILED_TO_RUN_NODE | 103 |  |
| FAILED_TO_FIND_ACCOUNT_INFO | 104 |  |
| LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET | 105 |  |



<a name="anytype.AccountStartResponse.Error.Code"></a>

### AccountStartResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| LOCAL_REPO_DOESNT_EXIST | 101 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 102 |  |
| FAILED_TO_RUN_NODE | 103 |  |
| FAILED_TO_FIND_ACCOUNT_INFO | 104 |  |



<a name="anytype.WalletCreateResponse.Error.Code"></a>

### WalletCreateResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 101 | ... |



<a name="anytype.WalletRecoverResponse.Error.Code"></a>

### WalletRecoverResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |


 

 

 



<a name="block.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## block.proto



<a name="anytype.Block"></a>

### Block



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| header | [BlockHeader](#anytype.BlockHeader) |  |  |
| dashboard | [BlockContentDashboard](#anytype.BlockContentDashboard) |  |  |
| page | [BlockContentPage](#anytype.BlockContentPage) |  |  |
| dataview | [BlockContentDataview](#anytype.BlockContentDataview) |  |  |
| text | [BlockContentText](#anytype.BlockContentText) |  |  |
| media | [BlockContentMedia](#anytype.BlockContentMedia) |  |  |






<a name="anytype.BlockAtomicChange"></a>

### BlockAtomicChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [BlockContentTextChange](#anytype.BlockContentTextChange) |  |  |
| blockHeader | [BlockHeaderChange](#anytype.BlockHeaderChange) |  |  |
| page | [BlockContentPageChange](#anytype.BlockContentPageChange) |  |  |
| dashboard | [BlockContentDashboardChange](#anytype.BlockContentDashboardChange) |  |  |






<a name="anytype.BlockChanges"></a>

### BlockChanges



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changes | [BlockAtomicChange](#anytype.BlockAtomicChange) | repeated |  |






<a name="anytype.BlockConnections"></a>

### BlockConnections



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| next | [string](#string) |  |  |
| columnBottom | [string](#string) |  |  |
| rowRight | [string](#string) |  |  |
| inner | [string](#string) |  |  |






<a name="anytype.BlockConnectionsList"></a>

### BlockConnectionsList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| structure | [BlockConnections](#anytype.BlockConnections) | repeated |  |






<a name="anytype.BlockContentDashboard"></a>

### BlockContentDashboard



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [BlockContentDashboard.Style](#anytype.BlockContentDashboard.Style) |  |  |
| structure | [BlockConnectionsList](#anytype.BlockConnectionsList) |  |  |
| headers | [BlockHeadersList](#anytype.BlockHeadersList) |  |  |






<a name="anytype.BlockContentDashboardChange"></a>

### BlockContentDashboardChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [BlockContentDashboard.Style](#anytype.BlockContentDashboard.Style) |  |  |
| structure | [BlockConnectionsList](#anytype.BlockConnectionsList) |  |  |
| headers | [BlockHeadersList](#anytype.BlockHeadersList) |  |  |






<a name="anytype.BlockContentDataview"></a>

### BlockContentDataview
...






<a name="anytype.BlockContentMedia"></a>

### BlockContentMedia



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| link | [string](#string) |  |  |






<a name="anytype.BlockContentPage"></a>

### BlockContentPage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [BlockContentPage.Style](#anytype.BlockContentPage.Style) |  |  |
| structure | [BlockConnectionsList](#anytype.BlockConnectionsList) |  |  |
| blocks | [BlocksList](#anytype.BlocksList) |  |  |






<a name="anytype.BlockContentPageChange"></a>

### BlockContentPageChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [BlockContentPage.Style](#anytype.BlockContentPage.Style) |  |  |
| structure | [BlockConnectionsList](#anytype.BlockConnectionsList) |  |  |
| blocks | [BlocksList](#anytype.BlocksList) |  |  |






<a name="anytype.BlockContentText"></a>

### BlockContentText



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [BlockContentText.Style](#anytype.BlockContentText.Style) |  |  |
| marksList | [BlockContentText.Marks](#anytype.BlockContentText.Marks) |  |  |
| toggleable | [bool](#bool) |  |  |
| markerType | [BlockContentText.MarkerType](#anytype.BlockContentText.MarkerType) |  |  |
| checkable | [bool](#bool) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype.BlockContentText.Mark"></a>

### BlockContentText.Mark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Range](#anytype.Range) |  |  |
| type | [BlockContentText.Mark.Type](#anytype.BlockContentText.Mark.Type) |  |  |
| param | [string](#string) |  | link, color, etc |






<a name="anytype.BlockContentText.Marks"></a>

### BlockContentText.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marks | [BlockContentText.Mark](#anytype.BlockContentText.Mark) | repeated |  |






<a name="anytype.BlockContentTextChange"></a>

### BlockContentTextChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [BlockContentText.Style](#anytype.BlockContentText.Style) |  |  |
| marks | [BlockContentText.Marks](#anytype.BlockContentText.Marks) |  |  |
| toggleable | [bool](#bool) |  |  |
| markerType | [BlockContentText.MarkerType](#anytype.BlockContentText.MarkerType) |  |  |
| checkable | [bool](#bool) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype.BlockCreate"></a>

### BlockCreate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| block | [Block](#anytype.Block) |  |  |






<a name="anytype.BlockCreateRequest"></a>

### BlockCreateRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [BlockType](#anytype.BlockType) |  |  |






<a name="anytype.BlockCreateResponse"></a>

### BlockCreateResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [BlockCreateResponse.Error](#anytype.BlockCreateResponse.Error) |  |  |






<a name="anytype.BlockCreateResponse.Error"></a>

### BlockCreateResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [BlockCreateResponse.Error.Code](#anytype.BlockCreateResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.BlockHeader"></a>

### BlockHeader



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [BlockType](#anytype.BlockType) |  |  |
| fields | [Struct](#anytype.Struct) |  |  |
| permissions | [BlockPermissions](#anytype.BlockPermissions) |  |  |






<a name="anytype.BlockHeaderChange"></a>

### BlockHeaderChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [BlockType](#anytype.BlockType) |  |  |
| name | [string](#string) |  |  |
| icon | [string](#string) |  |  |
| permissions | [BlockPermissions](#anytype.BlockPermissions) |  |  |






<a name="anytype.BlockHeadersList"></a>

### BlockHeadersList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| headers | [BlockHeader](#anytype.BlockHeader) | repeated |  |






<a name="anytype.BlockOpenRequest"></a>

### BlockOpenRequest
commands


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.BlockOpenResponse"></a>

### BlockOpenResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [BlockOpenResponse.Error](#anytype.BlockOpenResponse.Error) |  |  |
| blockHeader | [BlockHeader](#anytype.BlockHeader) |  |  |






<a name="anytype.BlockOpenResponse.Error"></a>

### BlockOpenResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [BlockOpenResponse.Error.Code](#anytype.BlockOpenResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.BlockPermissions"></a>

### BlockPermissions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| read | [bool](#bool) |  |  |
| edit | [bool](#bool) |  |  |
| remove | [bool](#bool) |  |  |
| drag | [bool](#bool) |  |  |
| dropOn | [bool](#bool) |  |  |






<a name="anytype.BlockShow"></a>

### BlockShow
call


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| block | [Block](#anytype.Block) |  |  |






<a name="anytype.BlockUpdate"></a>

### BlockUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changes | [BlockChanges](#anytype.BlockChanges) |  |  |






<a name="anytype.BlockUpdateRequest"></a>

### BlockUpdateRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changes | [BlockChanges](#anytype.BlockChanges) |  |  |






<a name="anytype.BlockUpdateResponse"></a>

### BlockUpdateResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [BlockUpdateResponse.Error](#anytype.BlockUpdateResponse.Error) |  |  |






<a name="anytype.BlockUpdateResponse.Error"></a>

### BlockUpdateResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [BlockUpdateResponse.Error.Code](#anytype.BlockUpdateResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.BlocksList"></a>

### BlocksList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [Block](#anytype.Block) | repeated |  |






<a name="anytype.Range"></a>

### Range



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) |  |  |
| to | [int32](#int32) |  |  |





 


<a name="anytype.BlockContentDashboard.Style"></a>

### BlockContentDashboard.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| MAIN_SCREEN | 0 | ... |



<a name="anytype.BlockContentPage.Style"></a>

### BlockContentPage.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| EMPTY | 0 |  |
| TASK | 1 |  |
| BOOKMARK | 2 |  |
| SET | 3 | ... |



<a name="anytype.BlockContentText.Mark.Type"></a>

### BlockContentText.Mark.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| STRIKETHROUGH | 0 |  |
| KEYBOARD | 1 |  |
| ITALIC | 2 |  |
| BOLD | 3 |  |
| LINK | 4 |  |



<a name="anytype.BlockContentText.MarkerType"></a>

### BlockContentText.MarkerType


| Name | Number | Description |
| ---- | ------ | ----------- |
| none | 0 |  |
| number | 1 |  |
| bullet | 2 |  |



<a name="anytype.BlockContentText.Style"></a>

### BlockContentText.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| p | 0 |  |
| h1 | 1 |  |
| h2 | 2 |  |
| h3 | 3 |  |
| h4 | 4 |  |
| quote | 5 |  |



<a name="anytype.BlockCreateResponse.Error.Code"></a>

### BlockCreateResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.BlockOpenResponse.Error.Code"></a>

### BlockOpenResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.BlockType"></a>

### BlockType


| Name | Number | Description |
| ---- | ------ | ----------- |
| DASHBOARD | 0 |  |
| PAGE | 1 |  |
| DATAVIEW | 2 |  |
| TEXT | 3 |  |
| FILE | 4 |  |
| PICTURE | 5 |  |
| VIDEO | 6 |  |



<a name="anytype.BlockUpdateResponse.Error.Code"></a>

### BlockUpdateResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |


 

 

 



<a name="edit.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## edit.proto



<a name="anytype.UserBlockFocus"></a>

### UserBlockFocus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Account](#anytype.Account) |  |  |
| blockId | [string](#string) |  |  |






<a name="anytype.UserBlockJoin"></a>

### UserBlockJoin



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Account](#anytype.Account) |  |  |






<a name="anytype.UserBlockLeft"></a>

### UserBlockLeft



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Account](#anytype.Account) |  |  |






<a name="anytype.UserBlockSelectRange"></a>

### UserBlockSelectRange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Account](#anytype.Account) |  |  |
| blockIdsArray | [string](#string) | repeated |  |






<a name="anytype.UserBlockTextRange"></a>

### UserBlockTextRange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Account](#anytype.Account) |  |  |
| blockId | [string](#string) |  |  |
| range | [Range](#anytype.Range) |  |  |





 

 

 

 



<a name="event.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## event.proto



<a name="anytype.Event"></a>

### Event



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountShow | [AccountShow](#anytype.AccountShow) |  | show wallet&#39;s accounts that were loaded from local or remote source |
| blockShow | [BlockShow](#anytype.BlockShow) |  |  |
| blockUpdate | [BlockUpdate](#anytype.BlockUpdate) |  |  |
| blockCreate | [BlockCreate](#anytype.BlockCreate) |  |  |





 

 

 

 



<a name="file.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## file.proto



<a name="anytype.Image"></a>

### Image



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| sizes | [ImageSize](#anytype.ImageSize) | repeated |  |






<a name="anytype.ImageGetBlobRequest"></a>

### ImageGetBlobRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| size | [ImageSize](#anytype.ImageSize) |  |  |






<a name="anytype.ImageGetBlobResponse"></a>

### ImageGetBlobResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [ImageGetBlobResponse.Error](#anytype.ImageGetBlobResponse.Error) |  |  |
| blob | [bytes](#bytes) |  |  |






<a name="anytype.ImageGetBlobResponse.Error"></a>

### ImageGetBlobResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [ImageGetBlobResponse.Error.Code](#anytype.ImageGetBlobResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.ImageGetFileRequest"></a>

### ImageGetFileRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| size | [ImageSize](#anytype.ImageSize) |  |  |






<a name="anytype.ImageGetFileResponse"></a>

### ImageGetFileResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [ImageGetFileResponse.Error](#anytype.ImageGetFileResponse.Error) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype.ImageGetFileResponse.Error"></a>

### ImageGetFileResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [ImageGetFileResponse.Error.Code](#anytype.ImageGetFileResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.IpfsGetFileRequest"></a>

### IpfsGetFileRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.IpfsGetFileResponse"></a>

### IpfsGetFileResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [IpfsGetFileResponse.Error](#anytype.IpfsGetFileResponse.Error) |  |  |
| data | [bytes](#bytes) |  |  |
| media | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.IpfsGetFileResponse.Error"></a>

### IpfsGetFileResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [IpfsGetFileResponse.Error.Code](#anytype.IpfsGetFileResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Video"></a>

### Video



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| sizes | [VideoSize](#anytype.VideoSize) | repeated |  |





 


<a name="anytype.ImageGetBlobResponse.Error.Code"></a>

### ImageGetBlobResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.ImageGetFileResponse.Error.Code"></a>

### ImageGetFileResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.ImageSize"></a>

### ImageSize


| Name | Number | Description |
| ---- | ------ | ----------- |
| LARGE | 0 |  |
| SMALL | 1 |  |
| THUMB | 2 |  |



<a name="anytype.IpfsGetFileResponse.Error.Code"></a>

### IpfsGetFileResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.VideoSize"></a>

### VideoSize


| Name | Number | Description |
| ---- | ------ | ----------- |
| SD_360p | 0 |  |
| SD_480p | 1 |  |
| HD_720p | 2 |  |
| HD_1080p | 3 |  |
| UHD_1440p | 4 |  |
| UHD_2160p | 5 |  |


 

 

 



<a name="misc.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## misc.proto



<a name="anytype.LogSendRequest"></a>

### LogSendRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| message | [string](#string) |  |  |
| level | [LogSendRequest.Level](#anytype.LogSendRequest.Level) |  |  |






<a name="anytype.LogSendResponse"></a>

### LogSendResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [LogSendResponse.Error](#anytype.LogSendResponse.Error) |  |  |






<a name="anytype.LogSendResponse.Error"></a>

### LogSendResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [LogSendResponse.Error.Code](#anytype.LogSendResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.VersionGetRequest"></a>

### VersionGetRequest







<a name="anytype.VersionGetResponse"></a>

### VersionGetResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [VersionGetResponse.Error](#anytype.VersionGetResponse.Error) |  |  |
| version | [string](#string) |  | version is generate by git describe |
| details | [string](#string) |  |  |






<a name="anytype.VersionGetResponse.Error"></a>

### VersionGetResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [VersionGetResponse.Error.Code](#anytype.VersionGetResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |





 


<a name="anytype.LogSendRequest.Level"></a>

### LogSendRequest.Level


| Name | Number | Description |
| ---- | ------ | ----------- |
| DEBUG | 0 |  |
| ERROR | 1 |  |
| FATAL | 2 |  |
| INFO | 3 |  |
| PANIC | 4 |  |
| WARNING | 5 |  |



<a name="anytype.LogSendResponse.Error.Code"></a>

### LogSendResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.VersionGetResponse.Error.Code"></a>

### VersionGetResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| VERSION_IS_EMPTY | 3 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |


 

 

 



<a name="struct.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## struct.proto



<a name="anytype.ListValue"></a>

### ListValue
`ListValue` is a wrapper around a repeated field of values.

The JSON representation for `ListValue` is JSON array.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| values | [Value](#anytype.Value) | repeated | Repeated field of dynamically typed values. |






<a name="anytype.Struct"></a>

### Struct



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fields | [Struct.FieldsEntry](#anytype.Struct.FieldsEntry) | repeated | Unordered map of dynamically typed values. |






<a name="anytype.Struct.FieldsEntry"></a>

### Struct.FieldsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [Value](#anytype.Value) |  |  |






<a name="anytype.Value"></a>

### Value
`Value` represents a dynamically typed value which can be either
null, a number, a string, a boolean, a recursive struct value, or a
list of values. A producer of value is expected to set one of that
variants, absence of any variant indicates an error.

The JSON representation for `Value` is JSON value.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| null_value | [NullValue](#anytype.NullValue) |  | Represents a null value. |
| number_value | [double](#double) |  | Represents a double value. |
| string_value | [string](#string) |  | Represents a string value. |
| bool_value | [bool](#bool) |  | Represents a boolean value. |
| struct_value | [Struct](#anytype.Struct) |  | Represents a structured value. |
| list_value | [ListValue](#anytype.ListValue) |  | Represents a repeated `Value`. |





 


<a name="anytype.NullValue"></a>

### NullValue
`NullValue` is a singleton enumeration to represent the null value for the
`Value` type union.

 The JSON representation for `NullValue` is JSON `null`.

| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL_VALUE | 0 | Null value. |


 

 

 



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

