# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [pb/protos/service/service.proto](#pb_protos_service_service-proto)
    - [ClientCommands](#anytype-ClientCommands)
  
- [pb/protos/changes.proto](#pb_protos_changes-proto)
    - [Change](#anytype-Change)
    - [Change.BlockCreate](#anytype-Change-BlockCreate)
    - [Change.BlockDuplicate](#anytype-Change-BlockDuplicate)
    - [Change.BlockMove](#anytype-Change-BlockMove)
    - [Change.BlockRemove](#anytype-Change-BlockRemove)
    - [Change.BlockUpdate](#anytype-Change-BlockUpdate)
    - [Change.Content](#anytype-Change-Content)
    - [Change.DetailsSet](#anytype-Change-DetailsSet)
    - [Change.DetailsUnset](#anytype-Change-DetailsUnset)
    - [Change.FileKeys](#anytype-Change-FileKeys)
    - [Change.FileKeys.KeysEntry](#anytype-Change-FileKeys-KeysEntry)
    - [Change.ObjectTypeAdd](#anytype-Change-ObjectTypeAdd)
    - [Change.ObjectTypeRemove](#anytype-Change-ObjectTypeRemove)
    - [Change.RelationAdd](#anytype-Change-RelationAdd)
    - [Change.RelationRemove](#anytype-Change-RelationRemove)
    - [Change.Snapshot](#anytype-Change-Snapshot)
    - [Change.Snapshot.LogHeadsEntry](#anytype-Change-Snapshot-LogHeadsEntry)
    - [Change.StoreKeySet](#anytype-Change-StoreKeySet)
    - [Change.StoreKeyUnset](#anytype-Change-StoreKeyUnset)
    - [Change.StoreSliceUpdate](#anytype-Change-StoreSliceUpdate)
    - [Change.StoreSliceUpdate.Add](#anytype-Change-StoreSliceUpdate-Add)
    - [Change.StoreSliceUpdate.Move](#anytype-Change-StoreSliceUpdate-Move)
    - [Change.StoreSliceUpdate.Remove](#anytype-Change-StoreSliceUpdate-Remove)
    - [Change._RelationAdd](#anytype-Change-_RelationAdd)
    - [Change._RelationRemove](#anytype-Change-_RelationRemove)
    - [Change._RelationUpdate](#anytype-Change-_RelationUpdate)
    - [Change._RelationUpdate.Dict](#anytype-Change-_RelationUpdate-Dict)
    - [Change._RelationUpdate.ObjectTypes](#anytype-Change-_RelationUpdate-ObjectTypes)
  
- [pb/protos/commands.proto](#pb_protos_commands-proto)
    - [Empty](#anytype-Empty)
    - [Rpc](#anytype-Rpc)
    - [Rpc.Account](#anytype-Rpc-Account)
    - [Rpc.Account.Config](#anytype-Rpc-Account-Config)
    - [Rpc.Account.ConfigUpdate](#anytype-Rpc-Account-ConfigUpdate)
    - [Rpc.Account.ConfigUpdate.Request](#anytype-Rpc-Account-ConfigUpdate-Request)
    - [Rpc.Account.ConfigUpdate.Response](#anytype-Rpc-Account-ConfigUpdate-Response)
    - [Rpc.Account.ConfigUpdate.Response.Error](#anytype-Rpc-Account-ConfigUpdate-Response-Error)
    - [Rpc.Account.Create](#anytype-Rpc-Account-Create)
    - [Rpc.Account.Create.Request](#anytype-Rpc-Account-Create-Request)
    - [Rpc.Account.Create.Response](#anytype-Rpc-Account-Create-Response)
    - [Rpc.Account.Create.Response.Error](#anytype-Rpc-Account-Create-Response-Error)
    - [Rpc.Account.Delete](#anytype-Rpc-Account-Delete)
    - [Rpc.Account.Delete.Request](#anytype-Rpc-Account-Delete-Request)
    - [Rpc.Account.Delete.Response](#anytype-Rpc-Account-Delete-Response)
    - [Rpc.Account.Delete.Response.Error](#anytype-Rpc-Account-Delete-Response-Error)
    - [Rpc.Account.GetConfig](#anytype-Rpc-Account-GetConfig)
    - [Rpc.Account.GetConfig.Get](#anytype-Rpc-Account-GetConfig-Get)
    - [Rpc.Account.GetConfig.Get.Request](#anytype-Rpc-Account-GetConfig-Get-Request)
    - [Rpc.Account.Move](#anytype-Rpc-Account-Move)
    - [Rpc.Account.Move.Request](#anytype-Rpc-Account-Move-Request)
    - [Rpc.Account.Move.Response](#anytype-Rpc-Account-Move-Response)
    - [Rpc.Account.Move.Response.Error](#anytype-Rpc-Account-Move-Response-Error)
    - [Rpc.Account.Recover](#anytype-Rpc-Account-Recover)
    - [Rpc.Account.Recover.Request](#anytype-Rpc-Account-Recover-Request)
    - [Rpc.Account.Recover.Response](#anytype-Rpc-Account-Recover-Response)
    - [Rpc.Account.Recover.Response.Error](#anytype-Rpc-Account-Recover-Response-Error)
    - [Rpc.Account.RecoverFromLegacyExport](#anytype-Rpc-Account-RecoverFromLegacyExport)
    - [Rpc.Account.RecoverFromLegacyExport.Request](#anytype-Rpc-Account-RecoverFromLegacyExport-Request)
    - [Rpc.Account.RecoverFromLegacyExport.Response](#anytype-Rpc-Account-RecoverFromLegacyExport-Response)
    - [Rpc.Account.RecoverFromLegacyExport.Response.Error](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error)
    - [Rpc.Account.Select](#anytype-Rpc-Account-Select)
    - [Rpc.Account.Select.Request](#anytype-Rpc-Account-Select-Request)
    - [Rpc.Account.Select.Response](#anytype-Rpc-Account-Select-Response)
    - [Rpc.Account.Select.Response.Error](#anytype-Rpc-Account-Select-Response-Error)
    - [Rpc.Account.Stop](#anytype-Rpc-Account-Stop)
    - [Rpc.Account.Stop.Request](#anytype-Rpc-Account-Stop-Request)
    - [Rpc.Account.Stop.Response](#anytype-Rpc-Account-Stop-Response)
    - [Rpc.Account.Stop.Response.Error](#anytype-Rpc-Account-Stop-Response-Error)
    - [Rpc.App](#anytype-Rpc-App)
    - [Rpc.App.GetVersion](#anytype-Rpc-App-GetVersion)
    - [Rpc.App.GetVersion.Request](#anytype-Rpc-App-GetVersion-Request)
    - [Rpc.App.GetVersion.Response](#anytype-Rpc-App-GetVersion-Response)
    - [Rpc.App.GetVersion.Response.Error](#anytype-Rpc-App-GetVersion-Response-Error)
    - [Rpc.App.SetDeviceState](#anytype-Rpc-App-SetDeviceState)
    - [Rpc.App.SetDeviceState.Request](#anytype-Rpc-App-SetDeviceState-Request)
    - [Rpc.App.SetDeviceState.Response](#anytype-Rpc-App-SetDeviceState-Response)
    - [Rpc.App.SetDeviceState.Response.Error](#anytype-Rpc-App-SetDeviceState-Response-Error)
    - [Rpc.App.Shutdown](#anytype-Rpc-App-Shutdown)
    - [Rpc.App.Shutdown.Request](#anytype-Rpc-App-Shutdown-Request)
    - [Rpc.App.Shutdown.Response](#anytype-Rpc-App-Shutdown-Response)
    - [Rpc.App.Shutdown.Response.Error](#anytype-Rpc-App-Shutdown-Response-Error)
    - [Rpc.Block](#anytype-Rpc-Block)
    - [Rpc.Block.Copy](#anytype-Rpc-Block-Copy)
    - [Rpc.Block.Copy.Request](#anytype-Rpc-Block-Copy-Request)
    - [Rpc.Block.Copy.Response](#anytype-Rpc-Block-Copy-Response)
    - [Rpc.Block.Copy.Response.Error](#anytype-Rpc-Block-Copy-Response-Error)
    - [Rpc.Block.Create](#anytype-Rpc-Block-Create)
    - [Rpc.Block.Create.Request](#anytype-Rpc-Block-Create-Request)
    - [Rpc.Block.Create.Response](#anytype-Rpc-Block-Create-Response)
    - [Rpc.Block.Create.Response.Error](#anytype-Rpc-Block-Create-Response-Error)
    - [Rpc.Block.CreateWidget](#anytype-Rpc-Block-CreateWidget)
    - [Rpc.Block.CreateWidget.Request](#anytype-Rpc-Block-CreateWidget-Request)
    - [Rpc.Block.CreateWidget.Response](#anytype-Rpc-Block-CreateWidget-Response)
    - [Rpc.Block.CreateWidget.Response.Error](#anytype-Rpc-Block-CreateWidget-Response-Error)
    - [Rpc.Block.Cut](#anytype-Rpc-Block-Cut)
    - [Rpc.Block.Cut.Request](#anytype-Rpc-Block-Cut-Request)
    - [Rpc.Block.Cut.Response](#anytype-Rpc-Block-Cut-Response)
    - [Rpc.Block.Cut.Response.Error](#anytype-Rpc-Block-Cut-Response-Error)
    - [Rpc.Block.Download](#anytype-Rpc-Block-Download)
    - [Rpc.Block.Download.Request](#anytype-Rpc-Block-Download-Request)
    - [Rpc.Block.Download.Response](#anytype-Rpc-Block-Download-Response)
    - [Rpc.Block.Download.Response.Error](#anytype-Rpc-Block-Download-Response-Error)
    - [Rpc.Block.Export](#anytype-Rpc-Block-Export)
    - [Rpc.Block.Export.Request](#anytype-Rpc-Block-Export-Request)
    - [Rpc.Block.Export.Response](#anytype-Rpc-Block-Export-Response)
    - [Rpc.Block.Export.Response.Error](#anytype-Rpc-Block-Export-Response-Error)
    - [Rpc.Block.ListConvertToObjects](#anytype-Rpc-Block-ListConvertToObjects)
    - [Rpc.Block.ListConvertToObjects.Request](#anytype-Rpc-Block-ListConvertToObjects-Request)
    - [Rpc.Block.ListConvertToObjects.Response](#anytype-Rpc-Block-ListConvertToObjects-Response)
    - [Rpc.Block.ListConvertToObjects.Response.Error](#anytype-Rpc-Block-ListConvertToObjects-Response-Error)
    - [Rpc.Block.ListDelete](#anytype-Rpc-Block-ListDelete)
    - [Rpc.Block.ListDelete.Request](#anytype-Rpc-Block-ListDelete-Request)
    - [Rpc.Block.ListDelete.Response](#anytype-Rpc-Block-ListDelete-Response)
    - [Rpc.Block.ListDelete.Response.Error](#anytype-Rpc-Block-ListDelete-Response-Error)
    - [Rpc.Block.ListDuplicate](#anytype-Rpc-Block-ListDuplicate)
    - [Rpc.Block.ListDuplicate.Request](#anytype-Rpc-Block-ListDuplicate-Request)
    - [Rpc.Block.ListDuplicate.Response](#anytype-Rpc-Block-ListDuplicate-Response)
    - [Rpc.Block.ListDuplicate.Response.Error](#anytype-Rpc-Block-ListDuplicate-Response-Error)
    - [Rpc.Block.ListMoveToExistingObject](#anytype-Rpc-Block-ListMoveToExistingObject)
    - [Rpc.Block.ListMoveToExistingObject.Request](#anytype-Rpc-Block-ListMoveToExistingObject-Request)
    - [Rpc.Block.ListMoveToExistingObject.Response](#anytype-Rpc-Block-ListMoveToExistingObject-Response)
    - [Rpc.Block.ListMoveToExistingObject.Response.Error](#anytype-Rpc-Block-ListMoveToExistingObject-Response-Error)
    - [Rpc.Block.ListMoveToNewObject](#anytype-Rpc-Block-ListMoveToNewObject)
    - [Rpc.Block.ListMoveToNewObject.Request](#anytype-Rpc-Block-ListMoveToNewObject-Request)
    - [Rpc.Block.ListMoveToNewObject.Response](#anytype-Rpc-Block-ListMoveToNewObject-Response)
    - [Rpc.Block.ListMoveToNewObject.Response.Error](#anytype-Rpc-Block-ListMoveToNewObject-Response-Error)
    - [Rpc.Block.ListSetAlign](#anytype-Rpc-Block-ListSetAlign)
    - [Rpc.Block.ListSetAlign.Request](#anytype-Rpc-Block-ListSetAlign-Request)
    - [Rpc.Block.ListSetAlign.Response](#anytype-Rpc-Block-ListSetAlign-Response)
    - [Rpc.Block.ListSetAlign.Response.Error](#anytype-Rpc-Block-ListSetAlign-Response-Error)
    - [Rpc.Block.ListSetBackgroundColor](#anytype-Rpc-Block-ListSetBackgroundColor)
    - [Rpc.Block.ListSetBackgroundColor.Request](#anytype-Rpc-Block-ListSetBackgroundColor-Request)
    - [Rpc.Block.ListSetBackgroundColor.Response](#anytype-Rpc-Block-ListSetBackgroundColor-Response)
    - [Rpc.Block.ListSetBackgroundColor.Response.Error](#anytype-Rpc-Block-ListSetBackgroundColor-Response-Error)
    - [Rpc.Block.ListSetFields](#anytype-Rpc-Block-ListSetFields)
    - [Rpc.Block.ListSetFields.Request](#anytype-Rpc-Block-ListSetFields-Request)
    - [Rpc.Block.ListSetFields.Request.BlockField](#anytype-Rpc-Block-ListSetFields-Request-BlockField)
    - [Rpc.Block.ListSetFields.Response](#anytype-Rpc-Block-ListSetFields-Response)
    - [Rpc.Block.ListSetFields.Response.Error](#anytype-Rpc-Block-ListSetFields-Response-Error)
    - [Rpc.Block.ListSetVerticalAlign](#anytype-Rpc-Block-ListSetVerticalAlign)
    - [Rpc.Block.ListSetVerticalAlign.Request](#anytype-Rpc-Block-ListSetVerticalAlign-Request)
    - [Rpc.Block.ListSetVerticalAlign.Response](#anytype-Rpc-Block-ListSetVerticalAlign-Response)
    - [Rpc.Block.ListSetVerticalAlign.Response.Error](#anytype-Rpc-Block-ListSetVerticalAlign-Response-Error)
    - [Rpc.Block.ListTurnInto](#anytype-Rpc-Block-ListTurnInto)
    - [Rpc.Block.ListTurnInto.Request](#anytype-Rpc-Block-ListTurnInto-Request)
    - [Rpc.Block.ListTurnInto.Response](#anytype-Rpc-Block-ListTurnInto-Response)
    - [Rpc.Block.ListTurnInto.Response.Error](#anytype-Rpc-Block-ListTurnInto-Response-Error)
    - [Rpc.Block.ListUpdate](#anytype-Rpc-Block-ListUpdate)
    - [Rpc.Block.ListUpdate.Request](#anytype-Rpc-Block-ListUpdate-Request)
    - [Rpc.Block.ListUpdate.Request.Text](#anytype-Rpc-Block-ListUpdate-Request-Text)
    - [Rpc.Block.Merge](#anytype-Rpc-Block-Merge)
    - [Rpc.Block.Merge.Request](#anytype-Rpc-Block-Merge-Request)
    - [Rpc.Block.Merge.Response](#anytype-Rpc-Block-Merge-Response)
    - [Rpc.Block.Merge.Response.Error](#anytype-Rpc-Block-Merge-Response-Error)
    - [Rpc.Block.Paste](#anytype-Rpc-Block-Paste)
    - [Rpc.Block.Paste.Request](#anytype-Rpc-Block-Paste-Request)
    - [Rpc.Block.Paste.Request.File](#anytype-Rpc-Block-Paste-Request-File)
    - [Rpc.Block.Paste.Response](#anytype-Rpc-Block-Paste-Response)
    - [Rpc.Block.Paste.Response.Error](#anytype-Rpc-Block-Paste-Response-Error)
    - [Rpc.Block.Replace](#anytype-Rpc-Block-Replace)
    - [Rpc.Block.Replace.Request](#anytype-Rpc-Block-Replace-Request)
    - [Rpc.Block.Replace.Response](#anytype-Rpc-Block-Replace-Response)
    - [Rpc.Block.Replace.Response.Error](#anytype-Rpc-Block-Replace-Response-Error)
    - [Rpc.Block.SetFields](#anytype-Rpc-Block-SetFields)
    - [Rpc.Block.SetFields.Request](#anytype-Rpc-Block-SetFields-Request)
    - [Rpc.Block.SetFields.Response](#anytype-Rpc-Block-SetFields-Response)
    - [Rpc.Block.SetFields.Response.Error](#anytype-Rpc-Block-SetFields-Response-Error)
    - [Rpc.Block.Split](#anytype-Rpc-Block-Split)
    - [Rpc.Block.Split.Request](#anytype-Rpc-Block-Split-Request)
    - [Rpc.Block.Split.Response](#anytype-Rpc-Block-Split-Response)
    - [Rpc.Block.Split.Response.Error](#anytype-Rpc-Block-Split-Response-Error)
    - [Rpc.Block.Upload](#anytype-Rpc-Block-Upload)
    - [Rpc.Block.Upload.Request](#anytype-Rpc-Block-Upload-Request)
    - [Rpc.Block.Upload.Response](#anytype-Rpc-Block-Upload-Response)
    - [Rpc.Block.Upload.Response.Error](#anytype-Rpc-Block-Upload-Response-Error)
    - [Rpc.BlockBookmark](#anytype-Rpc-BlockBookmark)
    - [Rpc.BlockBookmark.CreateAndFetch](#anytype-Rpc-BlockBookmark-CreateAndFetch)
    - [Rpc.BlockBookmark.CreateAndFetch.Request](#anytype-Rpc-BlockBookmark-CreateAndFetch-Request)
    - [Rpc.BlockBookmark.CreateAndFetch.Response](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response)
    - [Rpc.BlockBookmark.CreateAndFetch.Response.Error](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error)
    - [Rpc.BlockBookmark.Fetch](#anytype-Rpc-BlockBookmark-Fetch)
    - [Rpc.BlockBookmark.Fetch.Request](#anytype-Rpc-BlockBookmark-Fetch-Request)
    - [Rpc.BlockBookmark.Fetch.Response](#anytype-Rpc-BlockBookmark-Fetch-Response)
    - [Rpc.BlockBookmark.Fetch.Response.Error](#anytype-Rpc-BlockBookmark-Fetch-Response-Error)
    - [Rpc.BlockDataview](#anytype-Rpc-BlockDataview)
    - [Rpc.BlockDataview.CreateBookmark](#anytype-Rpc-BlockDataview-CreateBookmark)
    - [Rpc.BlockDataview.CreateBookmark.Request](#anytype-Rpc-BlockDataview-CreateBookmark-Request)
    - [Rpc.BlockDataview.CreateBookmark.Response](#anytype-Rpc-BlockDataview-CreateBookmark-Response)
    - [Rpc.BlockDataview.CreateBookmark.Response.Error](#anytype-Rpc-BlockDataview-CreateBookmark-Response-Error)
    - [Rpc.BlockDataview.CreateFromExistingObject](#anytype-Rpc-BlockDataview-CreateFromExistingObject)
    - [Rpc.BlockDataview.CreateFromExistingObject.Request](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Request)
    - [Rpc.BlockDataview.CreateFromExistingObject.Response](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Response)
    - [Rpc.BlockDataview.CreateFromExistingObject.Response.Error](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Response-Error)
    - [Rpc.BlockDataview.Filter](#anytype-Rpc-BlockDataview-Filter)
    - [Rpc.BlockDataview.Filter.Add](#anytype-Rpc-BlockDataview-Filter-Add)
    - [Rpc.BlockDataview.Filter.Add.Request](#anytype-Rpc-BlockDataview-Filter-Add-Request)
    - [Rpc.BlockDataview.Filter.Add.Response](#anytype-Rpc-BlockDataview-Filter-Add-Response)
    - [Rpc.BlockDataview.Filter.Add.Response.Error](#anytype-Rpc-BlockDataview-Filter-Add-Response-Error)
    - [Rpc.BlockDataview.Filter.Remove](#anytype-Rpc-BlockDataview-Filter-Remove)
    - [Rpc.BlockDataview.Filter.Remove.Request](#anytype-Rpc-BlockDataview-Filter-Remove-Request)
    - [Rpc.BlockDataview.Filter.Remove.Response](#anytype-Rpc-BlockDataview-Filter-Remove-Response)
    - [Rpc.BlockDataview.Filter.Remove.Response.Error](#anytype-Rpc-BlockDataview-Filter-Remove-Response-Error)
    - [Rpc.BlockDataview.Filter.Replace](#anytype-Rpc-BlockDataview-Filter-Replace)
    - [Rpc.BlockDataview.Filter.Replace.Request](#anytype-Rpc-BlockDataview-Filter-Replace-Request)
    - [Rpc.BlockDataview.Filter.Replace.Response](#anytype-Rpc-BlockDataview-Filter-Replace-Response)
    - [Rpc.BlockDataview.Filter.Replace.Response.Error](#anytype-Rpc-BlockDataview-Filter-Replace-Response-Error)
    - [Rpc.BlockDataview.Filter.Sort](#anytype-Rpc-BlockDataview-Filter-Sort)
    - [Rpc.BlockDataview.Filter.Sort.Request](#anytype-Rpc-BlockDataview-Filter-Sort-Request)
    - [Rpc.BlockDataview.Filter.Sort.Response](#anytype-Rpc-BlockDataview-Filter-Sort-Response)
    - [Rpc.BlockDataview.Filter.Sort.Response.Error](#anytype-Rpc-BlockDataview-Filter-Sort-Response-Error)
    - [Rpc.BlockDataview.GroupOrder](#anytype-Rpc-BlockDataview-GroupOrder)
    - [Rpc.BlockDataview.GroupOrder.Update](#anytype-Rpc-BlockDataview-GroupOrder-Update)
    - [Rpc.BlockDataview.GroupOrder.Update.Request](#anytype-Rpc-BlockDataview-GroupOrder-Update-Request)
    - [Rpc.BlockDataview.GroupOrder.Update.Response](#anytype-Rpc-BlockDataview-GroupOrder-Update-Response)
    - [Rpc.BlockDataview.GroupOrder.Update.Response.Error](#anytype-Rpc-BlockDataview-GroupOrder-Update-Response-Error)
    - [Rpc.BlockDataview.ObjectOrder](#anytype-Rpc-BlockDataview-ObjectOrder)
    - [Rpc.BlockDataview.ObjectOrder.Move](#anytype-Rpc-BlockDataview-ObjectOrder-Move)
    - [Rpc.BlockDataview.ObjectOrder.Move.Request](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Request)
    - [Rpc.BlockDataview.ObjectOrder.Move.Response](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Response)
    - [Rpc.BlockDataview.ObjectOrder.Move.Response.Error](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Response-Error)
    - [Rpc.BlockDataview.ObjectOrder.Update](#anytype-Rpc-BlockDataview-ObjectOrder-Update)
    - [Rpc.BlockDataview.ObjectOrder.Update.Request](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Request)
    - [Rpc.BlockDataview.ObjectOrder.Update.Response](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Response)
    - [Rpc.BlockDataview.ObjectOrder.Update.Response.Error](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Response-Error)
    - [Rpc.BlockDataview.Relation](#anytype-Rpc-BlockDataview-Relation)
    - [Rpc.BlockDataview.Relation.Add](#anytype-Rpc-BlockDataview-Relation-Add)
    - [Rpc.BlockDataview.Relation.Add.Request](#anytype-Rpc-BlockDataview-Relation-Add-Request)
    - [Rpc.BlockDataview.Relation.Add.Response](#anytype-Rpc-BlockDataview-Relation-Add-Response)
    - [Rpc.BlockDataview.Relation.Add.Response.Error](#anytype-Rpc-BlockDataview-Relation-Add-Response-Error)
    - [Rpc.BlockDataview.Relation.Delete](#anytype-Rpc-BlockDataview-Relation-Delete)
    - [Rpc.BlockDataview.Relation.Delete.Request](#anytype-Rpc-BlockDataview-Relation-Delete-Request)
    - [Rpc.BlockDataview.Relation.Delete.Response](#anytype-Rpc-BlockDataview-Relation-Delete-Response)
    - [Rpc.BlockDataview.Relation.Delete.Response.Error](#anytype-Rpc-BlockDataview-Relation-Delete-Response-Error)
    - [Rpc.BlockDataview.Relation.ListAvailable](#anytype-Rpc-BlockDataview-Relation-ListAvailable)
    - [Rpc.BlockDataview.Relation.ListAvailable.Request](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Request)
    - [Rpc.BlockDataview.Relation.ListAvailable.Response](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Response)
    - [Rpc.BlockDataview.Relation.ListAvailable.Response.Error](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Response-Error)
    - [Rpc.BlockDataview.SetSource](#anytype-Rpc-BlockDataview-SetSource)
    - [Rpc.BlockDataview.SetSource.Request](#anytype-Rpc-BlockDataview-SetSource-Request)
    - [Rpc.BlockDataview.SetSource.Response](#anytype-Rpc-BlockDataview-SetSource-Response)
    - [Rpc.BlockDataview.SetSource.Response.Error](#anytype-Rpc-BlockDataview-SetSource-Response-Error)
    - [Rpc.BlockDataview.Sort](#anytype-Rpc-BlockDataview-Sort)
    - [Rpc.BlockDataview.Sort.Add](#anytype-Rpc-BlockDataview-Sort-Add)
    - [Rpc.BlockDataview.Sort.Add.Request](#anytype-Rpc-BlockDataview-Sort-Add-Request)
    - [Rpc.BlockDataview.Sort.Add.Response](#anytype-Rpc-BlockDataview-Sort-Add-Response)
    - [Rpc.BlockDataview.Sort.Add.Response.Error](#anytype-Rpc-BlockDataview-Sort-Add-Response-Error)
    - [Rpc.BlockDataview.Sort.Remove](#anytype-Rpc-BlockDataview-Sort-Remove)
    - [Rpc.BlockDataview.Sort.Remove.Request](#anytype-Rpc-BlockDataview-Sort-Remove-Request)
    - [Rpc.BlockDataview.Sort.Remove.Response](#anytype-Rpc-BlockDataview-Sort-Remove-Response)
    - [Rpc.BlockDataview.Sort.Remove.Response.Error](#anytype-Rpc-BlockDataview-Sort-Remove-Response-Error)
    - [Rpc.BlockDataview.Sort.Replace](#anytype-Rpc-BlockDataview-Sort-Replace)
    - [Rpc.BlockDataview.Sort.Replace.Request](#anytype-Rpc-BlockDataview-Sort-Replace-Request)
    - [Rpc.BlockDataview.Sort.Replace.Response](#anytype-Rpc-BlockDataview-Sort-Replace-Response)
    - [Rpc.BlockDataview.Sort.Replace.Response.Error](#anytype-Rpc-BlockDataview-Sort-Replace-Response-Error)
    - [Rpc.BlockDataview.Sort.Sort](#anytype-Rpc-BlockDataview-Sort-Sort)
    - [Rpc.BlockDataview.Sort.Sort.Request](#anytype-Rpc-BlockDataview-Sort-Sort-Request)
    - [Rpc.BlockDataview.Sort.Sort.Response](#anytype-Rpc-BlockDataview-Sort-Sort-Response)
    - [Rpc.BlockDataview.Sort.Sort.Response.Error](#anytype-Rpc-BlockDataview-Sort-Sort-Response-Error)
    - [Rpc.BlockDataview.View](#anytype-Rpc-BlockDataview-View)
    - [Rpc.BlockDataview.View.Create](#anytype-Rpc-BlockDataview-View-Create)
    - [Rpc.BlockDataview.View.Create.Request](#anytype-Rpc-BlockDataview-View-Create-Request)
    - [Rpc.BlockDataview.View.Create.Response](#anytype-Rpc-BlockDataview-View-Create-Response)
    - [Rpc.BlockDataview.View.Create.Response.Error](#anytype-Rpc-BlockDataview-View-Create-Response-Error)
    - [Rpc.BlockDataview.View.Delete](#anytype-Rpc-BlockDataview-View-Delete)
    - [Rpc.BlockDataview.View.Delete.Request](#anytype-Rpc-BlockDataview-View-Delete-Request)
    - [Rpc.BlockDataview.View.Delete.Response](#anytype-Rpc-BlockDataview-View-Delete-Response)
    - [Rpc.BlockDataview.View.Delete.Response.Error](#anytype-Rpc-BlockDataview-View-Delete-Response-Error)
    - [Rpc.BlockDataview.View.SetActive](#anytype-Rpc-BlockDataview-View-SetActive)
    - [Rpc.BlockDataview.View.SetActive.Request](#anytype-Rpc-BlockDataview-View-SetActive-Request)
    - [Rpc.BlockDataview.View.SetActive.Response](#anytype-Rpc-BlockDataview-View-SetActive-Response)
    - [Rpc.BlockDataview.View.SetActive.Response.Error](#anytype-Rpc-BlockDataview-View-SetActive-Response-Error)
    - [Rpc.BlockDataview.View.SetPosition](#anytype-Rpc-BlockDataview-View-SetPosition)
    - [Rpc.BlockDataview.View.SetPosition.Request](#anytype-Rpc-BlockDataview-View-SetPosition-Request)
    - [Rpc.BlockDataview.View.SetPosition.Response](#anytype-Rpc-BlockDataview-View-SetPosition-Response)
    - [Rpc.BlockDataview.View.SetPosition.Response.Error](#anytype-Rpc-BlockDataview-View-SetPosition-Response-Error)
    - [Rpc.BlockDataview.View.Update](#anytype-Rpc-BlockDataview-View-Update)
    - [Rpc.BlockDataview.View.Update.Request](#anytype-Rpc-BlockDataview-View-Update-Request)
    - [Rpc.BlockDataview.View.Update.Response](#anytype-Rpc-BlockDataview-View-Update-Response)
    - [Rpc.BlockDataview.View.Update.Response.Error](#anytype-Rpc-BlockDataview-View-Update-Response-Error)
    - [Rpc.BlockDataview.ViewRelation](#anytype-Rpc-BlockDataview-ViewRelation)
    - [Rpc.BlockDataview.ViewRelation.Add](#anytype-Rpc-BlockDataview-ViewRelation-Add)
    - [Rpc.BlockDataview.ViewRelation.Add.Request](#anytype-Rpc-BlockDataview-ViewRelation-Add-Request)
    - [Rpc.BlockDataview.ViewRelation.Add.Response](#anytype-Rpc-BlockDataview-ViewRelation-Add-Response)
    - [Rpc.BlockDataview.ViewRelation.Add.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Add-Response-Error)
    - [Rpc.BlockDataview.ViewRelation.Remove](#anytype-Rpc-BlockDataview-ViewRelation-Remove)
    - [Rpc.BlockDataview.ViewRelation.Remove.Request](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Request)
    - [Rpc.BlockDataview.ViewRelation.Remove.Response](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Response)
    - [Rpc.BlockDataview.ViewRelation.Remove.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Response-Error)
    - [Rpc.BlockDataview.ViewRelation.Replace](#anytype-Rpc-BlockDataview-ViewRelation-Replace)
    - [Rpc.BlockDataview.ViewRelation.Replace.Request](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Request)
    - [Rpc.BlockDataview.ViewRelation.Replace.Response](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Response)
    - [Rpc.BlockDataview.ViewRelation.Replace.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Response-Error)
    - [Rpc.BlockDataview.ViewRelation.Sort](#anytype-Rpc-BlockDataview-ViewRelation-Sort)
    - [Rpc.BlockDataview.ViewRelation.Sort.Request](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Request)
    - [Rpc.BlockDataview.ViewRelation.Sort.Response](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Response)
    - [Rpc.BlockDataview.ViewRelation.Sort.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Response-Error)
    - [Rpc.BlockDiv](#anytype-Rpc-BlockDiv)
    - [Rpc.BlockDiv.ListSetStyle](#anytype-Rpc-BlockDiv-ListSetStyle)
    - [Rpc.BlockDiv.ListSetStyle.Request](#anytype-Rpc-BlockDiv-ListSetStyle-Request)
    - [Rpc.BlockDiv.ListSetStyle.Response](#anytype-Rpc-BlockDiv-ListSetStyle-Response)
    - [Rpc.BlockDiv.ListSetStyle.Response.Error](#anytype-Rpc-BlockDiv-ListSetStyle-Response-Error)
    - [Rpc.BlockFile](#anytype-Rpc-BlockFile)
    - [Rpc.BlockFile.CreateAndUpload](#anytype-Rpc-BlockFile-CreateAndUpload)
    - [Rpc.BlockFile.CreateAndUpload.Request](#anytype-Rpc-BlockFile-CreateAndUpload-Request)
    - [Rpc.BlockFile.CreateAndUpload.Response](#anytype-Rpc-BlockFile-CreateAndUpload-Response)
    - [Rpc.BlockFile.CreateAndUpload.Response.Error](#anytype-Rpc-BlockFile-CreateAndUpload-Response-Error)
    - [Rpc.BlockFile.ListSetStyle](#anytype-Rpc-BlockFile-ListSetStyle)
    - [Rpc.BlockFile.ListSetStyle.Request](#anytype-Rpc-BlockFile-ListSetStyle-Request)
    - [Rpc.BlockFile.ListSetStyle.Response](#anytype-Rpc-BlockFile-ListSetStyle-Response)
    - [Rpc.BlockFile.ListSetStyle.Response.Error](#anytype-Rpc-BlockFile-ListSetStyle-Response-Error)
    - [Rpc.BlockFile.SetName](#anytype-Rpc-BlockFile-SetName)
    - [Rpc.BlockFile.SetName.Request](#anytype-Rpc-BlockFile-SetName-Request)
    - [Rpc.BlockFile.SetName.Response](#anytype-Rpc-BlockFile-SetName-Response)
    - [Rpc.BlockFile.SetName.Response.Error](#anytype-Rpc-BlockFile-SetName-Response-Error)
    - [Rpc.BlockImage](#anytype-Rpc-BlockImage)
    - [Rpc.BlockImage.SetName](#anytype-Rpc-BlockImage-SetName)
    - [Rpc.BlockImage.SetName.Request](#anytype-Rpc-BlockImage-SetName-Request)
    - [Rpc.BlockImage.SetName.Response](#anytype-Rpc-BlockImage-SetName-Response)
    - [Rpc.BlockImage.SetName.Response.Error](#anytype-Rpc-BlockImage-SetName-Response-Error)
    - [Rpc.BlockImage.SetWidth](#anytype-Rpc-BlockImage-SetWidth)
    - [Rpc.BlockImage.SetWidth.Request](#anytype-Rpc-BlockImage-SetWidth-Request)
    - [Rpc.BlockImage.SetWidth.Response](#anytype-Rpc-BlockImage-SetWidth-Response)
    - [Rpc.BlockImage.SetWidth.Response.Error](#anytype-Rpc-BlockImage-SetWidth-Response-Error)
    - [Rpc.BlockLatex](#anytype-Rpc-BlockLatex)
    - [Rpc.BlockLatex.SetText](#anytype-Rpc-BlockLatex-SetText)
    - [Rpc.BlockLatex.SetText.Request](#anytype-Rpc-BlockLatex-SetText-Request)
    - [Rpc.BlockLatex.SetText.Response](#anytype-Rpc-BlockLatex-SetText-Response)
    - [Rpc.BlockLatex.SetText.Response.Error](#anytype-Rpc-BlockLatex-SetText-Response-Error)
    - [Rpc.BlockLink](#anytype-Rpc-BlockLink)
    - [Rpc.BlockLink.CreateWithObject](#anytype-Rpc-BlockLink-CreateWithObject)
    - [Rpc.BlockLink.CreateWithObject.Request](#anytype-Rpc-BlockLink-CreateWithObject-Request)
    - [Rpc.BlockLink.CreateWithObject.Response](#anytype-Rpc-BlockLink-CreateWithObject-Response)
    - [Rpc.BlockLink.CreateWithObject.Response.Error](#anytype-Rpc-BlockLink-CreateWithObject-Response-Error)
    - [Rpc.BlockLink.ListSetAppearance](#anytype-Rpc-BlockLink-ListSetAppearance)
    - [Rpc.BlockLink.ListSetAppearance.Request](#anytype-Rpc-BlockLink-ListSetAppearance-Request)
    - [Rpc.BlockLink.ListSetAppearance.Response](#anytype-Rpc-BlockLink-ListSetAppearance-Response)
    - [Rpc.BlockLink.ListSetAppearance.Response.Error](#anytype-Rpc-BlockLink-ListSetAppearance-Response-Error)
    - [Rpc.BlockRelation](#anytype-Rpc-BlockRelation)
    - [Rpc.BlockRelation.Add](#anytype-Rpc-BlockRelation-Add)
    - [Rpc.BlockRelation.Add.Request](#anytype-Rpc-BlockRelation-Add-Request)
    - [Rpc.BlockRelation.Add.Response](#anytype-Rpc-BlockRelation-Add-Response)
    - [Rpc.BlockRelation.Add.Response.Error](#anytype-Rpc-BlockRelation-Add-Response-Error)
    - [Rpc.BlockRelation.SetKey](#anytype-Rpc-BlockRelation-SetKey)
    - [Rpc.BlockRelation.SetKey.Request](#anytype-Rpc-BlockRelation-SetKey-Request)
    - [Rpc.BlockRelation.SetKey.Response](#anytype-Rpc-BlockRelation-SetKey-Response)
    - [Rpc.BlockRelation.SetKey.Response.Error](#anytype-Rpc-BlockRelation-SetKey-Response-Error)
    - [Rpc.BlockTable](#anytype-Rpc-BlockTable)
    - [Rpc.BlockTable.ColumnCreate](#anytype-Rpc-BlockTable-ColumnCreate)
    - [Rpc.BlockTable.ColumnCreate.Request](#anytype-Rpc-BlockTable-ColumnCreate-Request)
    - [Rpc.BlockTable.ColumnCreate.Response](#anytype-Rpc-BlockTable-ColumnCreate-Response)
    - [Rpc.BlockTable.ColumnCreate.Response.Error](#anytype-Rpc-BlockTable-ColumnCreate-Response-Error)
    - [Rpc.BlockTable.ColumnDelete](#anytype-Rpc-BlockTable-ColumnDelete)
    - [Rpc.BlockTable.ColumnDelete.Request](#anytype-Rpc-BlockTable-ColumnDelete-Request)
    - [Rpc.BlockTable.ColumnDelete.Response](#anytype-Rpc-BlockTable-ColumnDelete-Response)
    - [Rpc.BlockTable.ColumnDelete.Response.Error](#anytype-Rpc-BlockTable-ColumnDelete-Response-Error)
    - [Rpc.BlockTable.ColumnDuplicate](#anytype-Rpc-BlockTable-ColumnDuplicate)
    - [Rpc.BlockTable.ColumnDuplicate.Request](#anytype-Rpc-BlockTable-ColumnDuplicate-Request)
    - [Rpc.BlockTable.ColumnDuplicate.Response](#anytype-Rpc-BlockTable-ColumnDuplicate-Response)
    - [Rpc.BlockTable.ColumnDuplicate.Response.Error](#anytype-Rpc-BlockTable-ColumnDuplicate-Response-Error)
    - [Rpc.BlockTable.ColumnListFill](#anytype-Rpc-BlockTable-ColumnListFill)
    - [Rpc.BlockTable.ColumnListFill.Request](#anytype-Rpc-BlockTable-ColumnListFill-Request)
    - [Rpc.BlockTable.ColumnListFill.Response](#anytype-Rpc-BlockTable-ColumnListFill-Response)
    - [Rpc.BlockTable.ColumnListFill.Response.Error](#anytype-Rpc-BlockTable-ColumnListFill-Response-Error)
    - [Rpc.BlockTable.ColumnMove](#anytype-Rpc-BlockTable-ColumnMove)
    - [Rpc.BlockTable.ColumnMove.Request](#anytype-Rpc-BlockTable-ColumnMove-Request)
    - [Rpc.BlockTable.ColumnMove.Response](#anytype-Rpc-BlockTable-ColumnMove-Response)
    - [Rpc.BlockTable.ColumnMove.Response.Error](#anytype-Rpc-BlockTable-ColumnMove-Response-Error)
    - [Rpc.BlockTable.Create](#anytype-Rpc-BlockTable-Create)
    - [Rpc.BlockTable.Create.Request](#anytype-Rpc-BlockTable-Create-Request)
    - [Rpc.BlockTable.Create.Response](#anytype-Rpc-BlockTable-Create-Response)
    - [Rpc.BlockTable.Create.Response.Error](#anytype-Rpc-BlockTable-Create-Response-Error)
    - [Rpc.BlockTable.Expand](#anytype-Rpc-BlockTable-Expand)
    - [Rpc.BlockTable.Expand.Request](#anytype-Rpc-BlockTable-Expand-Request)
    - [Rpc.BlockTable.Expand.Response](#anytype-Rpc-BlockTable-Expand-Response)
    - [Rpc.BlockTable.Expand.Response.Error](#anytype-Rpc-BlockTable-Expand-Response-Error)
    - [Rpc.BlockTable.RowCreate](#anytype-Rpc-BlockTable-RowCreate)
    - [Rpc.BlockTable.RowCreate.Request](#anytype-Rpc-BlockTable-RowCreate-Request)
    - [Rpc.BlockTable.RowCreate.Response](#anytype-Rpc-BlockTable-RowCreate-Response)
    - [Rpc.BlockTable.RowCreate.Response.Error](#anytype-Rpc-BlockTable-RowCreate-Response-Error)
    - [Rpc.BlockTable.RowDelete](#anytype-Rpc-BlockTable-RowDelete)
    - [Rpc.BlockTable.RowDelete.Request](#anytype-Rpc-BlockTable-RowDelete-Request)
    - [Rpc.BlockTable.RowDelete.Response](#anytype-Rpc-BlockTable-RowDelete-Response)
    - [Rpc.BlockTable.RowDelete.Response.Error](#anytype-Rpc-BlockTable-RowDelete-Response-Error)
    - [Rpc.BlockTable.RowDuplicate](#anytype-Rpc-BlockTable-RowDuplicate)
    - [Rpc.BlockTable.RowDuplicate.Request](#anytype-Rpc-BlockTable-RowDuplicate-Request)
    - [Rpc.BlockTable.RowDuplicate.Response](#anytype-Rpc-BlockTable-RowDuplicate-Response)
    - [Rpc.BlockTable.RowDuplicate.Response.Error](#anytype-Rpc-BlockTable-RowDuplicate-Response-Error)
    - [Rpc.BlockTable.RowListClean](#anytype-Rpc-BlockTable-RowListClean)
    - [Rpc.BlockTable.RowListClean.Request](#anytype-Rpc-BlockTable-RowListClean-Request)
    - [Rpc.BlockTable.RowListClean.Response](#anytype-Rpc-BlockTable-RowListClean-Response)
    - [Rpc.BlockTable.RowListClean.Response.Error](#anytype-Rpc-BlockTable-RowListClean-Response-Error)
    - [Rpc.BlockTable.RowListFill](#anytype-Rpc-BlockTable-RowListFill)
    - [Rpc.BlockTable.RowListFill.Request](#anytype-Rpc-BlockTable-RowListFill-Request)
    - [Rpc.BlockTable.RowListFill.Response](#anytype-Rpc-BlockTable-RowListFill-Response)
    - [Rpc.BlockTable.RowListFill.Response.Error](#anytype-Rpc-BlockTable-RowListFill-Response-Error)
    - [Rpc.BlockTable.RowSetHeader](#anytype-Rpc-BlockTable-RowSetHeader)
    - [Rpc.BlockTable.RowSetHeader.Request](#anytype-Rpc-BlockTable-RowSetHeader-Request)
    - [Rpc.BlockTable.RowSetHeader.Response](#anytype-Rpc-BlockTable-RowSetHeader-Response)
    - [Rpc.BlockTable.RowSetHeader.Response.Error](#anytype-Rpc-BlockTable-RowSetHeader-Response-Error)
    - [Rpc.BlockTable.Sort](#anytype-Rpc-BlockTable-Sort)
    - [Rpc.BlockTable.Sort.Request](#anytype-Rpc-BlockTable-Sort-Request)
    - [Rpc.BlockTable.Sort.Response](#anytype-Rpc-BlockTable-Sort-Response)
    - [Rpc.BlockTable.Sort.Response.Error](#anytype-Rpc-BlockTable-Sort-Response-Error)
    - [Rpc.BlockText](#anytype-Rpc-BlockText)
    - [Rpc.BlockText.ListClearContent](#anytype-Rpc-BlockText-ListClearContent)
    - [Rpc.BlockText.ListClearContent.Request](#anytype-Rpc-BlockText-ListClearContent-Request)
    - [Rpc.BlockText.ListClearContent.Response](#anytype-Rpc-BlockText-ListClearContent-Response)
    - [Rpc.BlockText.ListClearContent.Response.Error](#anytype-Rpc-BlockText-ListClearContent-Response-Error)
    - [Rpc.BlockText.ListClearStyle](#anytype-Rpc-BlockText-ListClearStyle)
    - [Rpc.BlockText.ListClearStyle.Request](#anytype-Rpc-BlockText-ListClearStyle-Request)
    - [Rpc.BlockText.ListClearStyle.Response](#anytype-Rpc-BlockText-ListClearStyle-Response)
    - [Rpc.BlockText.ListClearStyle.Response.Error](#anytype-Rpc-BlockText-ListClearStyle-Response-Error)
    - [Rpc.BlockText.ListSetColor](#anytype-Rpc-BlockText-ListSetColor)
    - [Rpc.BlockText.ListSetColor.Request](#anytype-Rpc-BlockText-ListSetColor-Request)
    - [Rpc.BlockText.ListSetColor.Response](#anytype-Rpc-BlockText-ListSetColor-Response)
    - [Rpc.BlockText.ListSetColor.Response.Error](#anytype-Rpc-BlockText-ListSetColor-Response-Error)
    - [Rpc.BlockText.ListSetMark](#anytype-Rpc-BlockText-ListSetMark)
    - [Rpc.BlockText.ListSetMark.Request](#anytype-Rpc-BlockText-ListSetMark-Request)
    - [Rpc.BlockText.ListSetMark.Response](#anytype-Rpc-BlockText-ListSetMark-Response)
    - [Rpc.BlockText.ListSetMark.Response.Error](#anytype-Rpc-BlockText-ListSetMark-Response-Error)
    - [Rpc.BlockText.ListSetStyle](#anytype-Rpc-BlockText-ListSetStyle)
    - [Rpc.BlockText.ListSetStyle.Request](#anytype-Rpc-BlockText-ListSetStyle-Request)
    - [Rpc.BlockText.ListSetStyle.Response](#anytype-Rpc-BlockText-ListSetStyle-Response)
    - [Rpc.BlockText.ListSetStyle.Response.Error](#anytype-Rpc-BlockText-ListSetStyle-Response-Error)
    - [Rpc.BlockText.SetChecked](#anytype-Rpc-BlockText-SetChecked)
    - [Rpc.BlockText.SetChecked.Request](#anytype-Rpc-BlockText-SetChecked-Request)
    - [Rpc.BlockText.SetChecked.Response](#anytype-Rpc-BlockText-SetChecked-Response)
    - [Rpc.BlockText.SetChecked.Response.Error](#anytype-Rpc-BlockText-SetChecked-Response-Error)
    - [Rpc.BlockText.SetColor](#anytype-Rpc-BlockText-SetColor)
    - [Rpc.BlockText.SetColor.Request](#anytype-Rpc-BlockText-SetColor-Request)
    - [Rpc.BlockText.SetColor.Response](#anytype-Rpc-BlockText-SetColor-Response)
    - [Rpc.BlockText.SetColor.Response.Error](#anytype-Rpc-BlockText-SetColor-Response-Error)
    - [Rpc.BlockText.SetIcon](#anytype-Rpc-BlockText-SetIcon)
    - [Rpc.BlockText.SetIcon.Request](#anytype-Rpc-BlockText-SetIcon-Request)
    - [Rpc.BlockText.SetIcon.Response](#anytype-Rpc-BlockText-SetIcon-Response)
    - [Rpc.BlockText.SetIcon.Response.Error](#anytype-Rpc-BlockText-SetIcon-Response-Error)
    - [Rpc.BlockText.SetMarks](#anytype-Rpc-BlockText-SetMarks)
    - [Rpc.BlockText.SetMarks.Get](#anytype-Rpc-BlockText-SetMarks-Get)
    - [Rpc.BlockText.SetMarks.Get.Request](#anytype-Rpc-BlockText-SetMarks-Get-Request)
    - [Rpc.BlockText.SetMarks.Get.Response](#anytype-Rpc-BlockText-SetMarks-Get-Response)
    - [Rpc.BlockText.SetMarks.Get.Response.Error](#anytype-Rpc-BlockText-SetMarks-Get-Response-Error)
    - [Rpc.BlockText.SetStyle](#anytype-Rpc-BlockText-SetStyle)
    - [Rpc.BlockText.SetStyle.Request](#anytype-Rpc-BlockText-SetStyle-Request)
    - [Rpc.BlockText.SetStyle.Response](#anytype-Rpc-BlockText-SetStyle-Response)
    - [Rpc.BlockText.SetStyle.Response.Error](#anytype-Rpc-BlockText-SetStyle-Response-Error)
    - [Rpc.BlockText.SetText](#anytype-Rpc-BlockText-SetText)
    - [Rpc.BlockText.SetText.Request](#anytype-Rpc-BlockText-SetText-Request)
    - [Rpc.BlockText.SetText.Response](#anytype-Rpc-BlockText-SetText-Response)
    - [Rpc.BlockText.SetText.Response.Error](#anytype-Rpc-BlockText-SetText-Response-Error)
    - [Rpc.BlockVideo](#anytype-Rpc-BlockVideo)
    - [Rpc.BlockVideo.SetName](#anytype-Rpc-BlockVideo-SetName)
    - [Rpc.BlockVideo.SetName.Request](#anytype-Rpc-BlockVideo-SetName-Request)
    - [Rpc.BlockVideo.SetName.Response](#anytype-Rpc-BlockVideo-SetName-Response)
    - [Rpc.BlockVideo.SetName.Response.Error](#anytype-Rpc-BlockVideo-SetName-Response-Error)
    - [Rpc.BlockVideo.SetWidth](#anytype-Rpc-BlockVideo-SetWidth)
    - [Rpc.BlockVideo.SetWidth.Request](#anytype-Rpc-BlockVideo-SetWidth-Request)
    - [Rpc.BlockVideo.SetWidth.Response](#anytype-Rpc-BlockVideo-SetWidth-Response)
    - [Rpc.BlockVideo.SetWidth.Response.Error](#anytype-Rpc-BlockVideo-SetWidth-Response-Error)
    - [Rpc.BlockWidget](#anytype-Rpc-BlockWidget)
    - [Rpc.BlockWidget.SetLayout](#anytype-Rpc-BlockWidget-SetLayout)
    - [Rpc.BlockWidget.SetLayout.Request](#anytype-Rpc-BlockWidget-SetLayout-Request)
    - [Rpc.BlockWidget.SetLayout.Response](#anytype-Rpc-BlockWidget-SetLayout-Response)
    - [Rpc.BlockWidget.SetLayout.Response.Error](#anytype-Rpc-BlockWidget-SetLayout-Response-Error)
    - [Rpc.BlockWidget.SetLimit](#anytype-Rpc-BlockWidget-SetLimit)
    - [Rpc.BlockWidget.SetLimit.Request](#anytype-Rpc-BlockWidget-SetLimit-Request)
    - [Rpc.BlockWidget.SetLimit.Response](#anytype-Rpc-BlockWidget-SetLimit-Response)
    - [Rpc.BlockWidget.SetLimit.Response.Error](#anytype-Rpc-BlockWidget-SetLimit-Response-Error)
    - [Rpc.BlockWidget.SetTargetId](#anytype-Rpc-BlockWidget-SetTargetId)
    - [Rpc.BlockWidget.SetTargetId.Request](#anytype-Rpc-BlockWidget-SetTargetId-Request)
    - [Rpc.BlockWidget.SetTargetId.Response](#anytype-Rpc-BlockWidget-SetTargetId-Response)
    - [Rpc.BlockWidget.SetTargetId.Response.Error](#anytype-Rpc-BlockWidget-SetTargetId-Response-Error)
    - [Rpc.Debug](#anytype-Rpc-Debug)
    - [Rpc.Debug.ExportLocalstore](#anytype-Rpc-Debug-ExportLocalstore)
    - [Rpc.Debug.ExportLocalstore.Request](#anytype-Rpc-Debug-ExportLocalstore-Request)
    - [Rpc.Debug.ExportLocalstore.Response](#anytype-Rpc-Debug-ExportLocalstore-Response)
    - [Rpc.Debug.ExportLocalstore.Response.Error](#anytype-Rpc-Debug-ExportLocalstore-Response-Error)
    - [Rpc.Debug.Ping](#anytype-Rpc-Debug-Ping)
    - [Rpc.Debug.Ping.Request](#anytype-Rpc-Debug-Ping-Request)
    - [Rpc.Debug.Ping.Response](#anytype-Rpc-Debug-Ping-Response)
    - [Rpc.Debug.Ping.Response.Error](#anytype-Rpc-Debug-Ping-Response-Error)
    - [Rpc.Debug.SpaceSummary](#anytype-Rpc-Debug-SpaceSummary)
    - [Rpc.Debug.SpaceSummary.Request](#anytype-Rpc-Debug-SpaceSummary-Request)
    - [Rpc.Debug.SpaceSummary.Response](#anytype-Rpc-Debug-SpaceSummary-Response)
    - [Rpc.Debug.SpaceSummary.Response.Error](#anytype-Rpc-Debug-SpaceSummary-Response-Error)
    - [Rpc.Debug.Tree](#anytype-Rpc-Debug-Tree)
    - [Rpc.Debug.Tree.Request](#anytype-Rpc-Debug-Tree-Request)
    - [Rpc.Debug.Tree.Response](#anytype-Rpc-Debug-Tree-Response)
    - [Rpc.Debug.Tree.Response.Error](#anytype-Rpc-Debug-Tree-Response-Error)
    - [Rpc.Debug.TreeHeads](#anytype-Rpc-Debug-TreeHeads)
    - [Rpc.Debug.TreeHeads.Request](#anytype-Rpc-Debug-TreeHeads-Request)
    - [Rpc.Debug.TreeHeads.Response](#anytype-Rpc-Debug-TreeHeads-Response)
    - [Rpc.Debug.TreeHeads.Response.Error](#anytype-Rpc-Debug-TreeHeads-Response-Error)
    - [Rpc.Debug.TreeInfo](#anytype-Rpc-Debug-TreeInfo)
    - [Rpc.File](#anytype-Rpc-File)
    - [Rpc.File.Download](#anytype-Rpc-File-Download)
    - [Rpc.File.Download.Request](#anytype-Rpc-File-Download-Request)
    - [Rpc.File.Download.Response](#anytype-Rpc-File-Download-Response)
    - [Rpc.File.Download.Response.Error](#anytype-Rpc-File-Download-Response-Error)
    - [Rpc.File.Drop](#anytype-Rpc-File-Drop)
    - [Rpc.File.Drop.Request](#anytype-Rpc-File-Drop-Request)
    - [Rpc.File.Drop.Response](#anytype-Rpc-File-Drop-Response)
    - [Rpc.File.Drop.Response.Error](#anytype-Rpc-File-Drop-Response-Error)
    - [Rpc.File.ListOffload](#anytype-Rpc-File-ListOffload)
    - [Rpc.File.ListOffload.Request](#anytype-Rpc-File-ListOffload-Request)
    - [Rpc.File.ListOffload.Response](#anytype-Rpc-File-ListOffload-Response)
    - [Rpc.File.ListOffload.Response.Error](#anytype-Rpc-File-ListOffload-Response-Error)
    - [Rpc.File.Offload](#anytype-Rpc-File-Offload)
    - [Rpc.File.Offload.Request](#anytype-Rpc-File-Offload-Request)
    - [Rpc.File.Offload.Response](#anytype-Rpc-File-Offload-Response)
    - [Rpc.File.Offload.Response.Error](#anytype-Rpc-File-Offload-Response-Error)
    - [Rpc.File.SpaceUsage](#anytype-Rpc-File-SpaceUsage)
    - [Rpc.File.SpaceUsage.Request](#anytype-Rpc-File-SpaceUsage-Request)
    - [Rpc.File.SpaceUsage.Response](#anytype-Rpc-File-SpaceUsage-Response)
    - [Rpc.File.SpaceUsage.Response.Error](#anytype-Rpc-File-SpaceUsage-Response-Error)
    - [Rpc.File.SpaceUsage.Response.Usage](#anytype-Rpc-File-SpaceUsage-Response-Usage)
    - [Rpc.File.Upload](#anytype-Rpc-File-Upload)
    - [Rpc.File.Upload.Request](#anytype-Rpc-File-Upload-Request)
    - [Rpc.File.Upload.Response](#anytype-Rpc-File-Upload-Response)
    - [Rpc.File.Upload.Response.Error](#anytype-Rpc-File-Upload-Response-Error)
    - [Rpc.GenericErrorResponse](#anytype-Rpc-GenericErrorResponse)
    - [Rpc.GenericErrorResponse.Error](#anytype-Rpc-GenericErrorResponse-Error)
    - [Rpc.History](#anytype-Rpc-History)
    - [Rpc.History.GetVersions](#anytype-Rpc-History-GetVersions)
    - [Rpc.History.GetVersions.Request](#anytype-Rpc-History-GetVersions-Request)
    - [Rpc.History.GetVersions.Response](#anytype-Rpc-History-GetVersions-Response)
    - [Rpc.History.GetVersions.Response.Error](#anytype-Rpc-History-GetVersions-Response-Error)
    - [Rpc.History.SetVersion](#anytype-Rpc-History-SetVersion)
    - [Rpc.History.SetVersion.Request](#anytype-Rpc-History-SetVersion-Request)
    - [Rpc.History.SetVersion.Response](#anytype-Rpc-History-SetVersion-Response)
    - [Rpc.History.SetVersion.Response.Error](#anytype-Rpc-History-SetVersion-Response-Error)
    - [Rpc.History.ShowVersion](#anytype-Rpc-History-ShowVersion)
    - [Rpc.History.ShowVersion.Request](#anytype-Rpc-History-ShowVersion-Request)
    - [Rpc.History.ShowVersion.Response](#anytype-Rpc-History-ShowVersion-Response)
    - [Rpc.History.ShowVersion.Response.Error](#anytype-Rpc-History-ShowVersion-Response-Error)
    - [Rpc.History.Version](#anytype-Rpc-History-Version)
    - [Rpc.LinkPreview](#anytype-Rpc-LinkPreview)
    - [Rpc.LinkPreview.Request](#anytype-Rpc-LinkPreview-Request)
    - [Rpc.LinkPreview.Response](#anytype-Rpc-LinkPreview-Response)
    - [Rpc.LinkPreview.Response.Error](#anytype-Rpc-LinkPreview-Response-Error)
    - [Rpc.Log](#anytype-Rpc-Log)
    - [Rpc.Log.Send](#anytype-Rpc-Log-Send)
    - [Rpc.Log.Send.Request](#anytype-Rpc-Log-Send-Request)
    - [Rpc.Log.Send.Response](#anytype-Rpc-Log-Send-Response)
    - [Rpc.Log.Send.Response.Error](#anytype-Rpc-Log-Send-Response-Error)
    - [Rpc.Metrics](#anytype-Rpc-Metrics)
    - [Rpc.Metrics.SetParameters](#anytype-Rpc-Metrics-SetParameters)
    - [Rpc.Metrics.SetParameters.Request](#anytype-Rpc-Metrics-SetParameters-Request)
    - [Rpc.Metrics.SetParameters.Response](#anytype-Rpc-Metrics-SetParameters-Response)
    - [Rpc.Metrics.SetParameters.Response.Error](#anytype-Rpc-Metrics-SetParameters-Response-Error)
    - [Rpc.Navigation](#anytype-Rpc-Navigation)
    - [Rpc.Navigation.GetObjectInfoWithLinks](#anytype-Rpc-Navigation-GetObjectInfoWithLinks)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Request)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error)
    - [Rpc.Navigation.ListObjects](#anytype-Rpc-Navigation-ListObjects)
    - [Rpc.Navigation.ListObjects.Request](#anytype-Rpc-Navigation-ListObjects-Request)
    - [Rpc.Navigation.ListObjects.Response](#anytype-Rpc-Navigation-ListObjects-Response)
    - [Rpc.Navigation.ListObjects.Response.Error](#anytype-Rpc-Navigation-ListObjects-Response-Error)
    - [Rpc.Object](#anytype-Rpc-Object)
    - [Rpc.Object.ApplyTemplate](#anytype-Rpc-Object-ApplyTemplate)
    - [Rpc.Object.ApplyTemplate.Request](#anytype-Rpc-Object-ApplyTemplate-Request)
    - [Rpc.Object.ApplyTemplate.Response](#anytype-Rpc-Object-ApplyTemplate-Response)
    - [Rpc.Object.ApplyTemplate.Response.Error](#anytype-Rpc-Object-ApplyTemplate-Response-Error)
    - [Rpc.Object.BookmarkFetch](#anytype-Rpc-Object-BookmarkFetch)
    - [Rpc.Object.BookmarkFetch.Request](#anytype-Rpc-Object-BookmarkFetch-Request)
    - [Rpc.Object.BookmarkFetch.Response](#anytype-Rpc-Object-BookmarkFetch-Response)
    - [Rpc.Object.BookmarkFetch.Response.Error](#anytype-Rpc-Object-BookmarkFetch-Response-Error)
    - [Rpc.Object.Close](#anytype-Rpc-Object-Close)
    - [Rpc.Object.Close.Request](#anytype-Rpc-Object-Close-Request)
    - [Rpc.Object.Close.Response](#anytype-Rpc-Object-Close-Response)
    - [Rpc.Object.Close.Response.Error](#anytype-Rpc-Object-Close-Response-Error)
    - [Rpc.Object.Create](#anytype-Rpc-Object-Create)
    - [Rpc.Object.Create.Request](#anytype-Rpc-Object-Create-Request)
    - [Rpc.Object.Create.Response](#anytype-Rpc-Object-Create-Response)
    - [Rpc.Object.Create.Response.Error](#anytype-Rpc-Object-Create-Response-Error)
    - [Rpc.Object.CreateBookmark](#anytype-Rpc-Object-CreateBookmark)
    - [Rpc.Object.CreateBookmark.Request](#anytype-Rpc-Object-CreateBookmark-Request)
    - [Rpc.Object.CreateBookmark.Response](#anytype-Rpc-Object-CreateBookmark-Response)
    - [Rpc.Object.CreateBookmark.Response.Error](#anytype-Rpc-Object-CreateBookmark-Response-Error)
    - [Rpc.Object.CreateObjectType](#anytype-Rpc-Object-CreateObjectType)
    - [Rpc.Object.CreateObjectType.Request](#anytype-Rpc-Object-CreateObjectType-Request)
    - [Rpc.Object.CreateObjectType.Response](#anytype-Rpc-Object-CreateObjectType-Response)
    - [Rpc.Object.CreateObjectType.Response.Error](#anytype-Rpc-Object-CreateObjectType-Response-Error)
    - [Rpc.Object.CreateRelation](#anytype-Rpc-Object-CreateRelation)
    - [Rpc.Object.CreateRelation.Request](#anytype-Rpc-Object-CreateRelation-Request)
    - [Rpc.Object.CreateRelation.Response](#anytype-Rpc-Object-CreateRelation-Response)
    - [Rpc.Object.CreateRelation.Response.Error](#anytype-Rpc-Object-CreateRelation-Response-Error)
    - [Rpc.Object.CreateRelationOption](#anytype-Rpc-Object-CreateRelationOption)
    - [Rpc.Object.CreateRelationOption.Request](#anytype-Rpc-Object-CreateRelationOption-Request)
    - [Rpc.Object.CreateRelationOption.Response](#anytype-Rpc-Object-CreateRelationOption-Response)
    - [Rpc.Object.CreateRelationOption.Response.Error](#anytype-Rpc-Object-CreateRelationOption-Response-Error)
    - [Rpc.Object.CreateSet](#anytype-Rpc-Object-CreateSet)
    - [Rpc.Object.CreateSet.Request](#anytype-Rpc-Object-CreateSet-Request)
    - [Rpc.Object.CreateSet.Response](#anytype-Rpc-Object-CreateSet-Response)
    - [Rpc.Object.CreateSet.Response.Error](#anytype-Rpc-Object-CreateSet-Response-Error)
    - [Rpc.Object.Duplicate](#anytype-Rpc-Object-Duplicate)
    - [Rpc.Object.Duplicate.Request](#anytype-Rpc-Object-Duplicate-Request)
    - [Rpc.Object.Duplicate.Response](#anytype-Rpc-Object-Duplicate-Response)
    - [Rpc.Object.Duplicate.Response.Error](#anytype-Rpc-Object-Duplicate-Response-Error)
    - [Rpc.Object.Graph](#anytype-Rpc-Object-Graph)
    - [Rpc.Object.Graph.Edge](#anytype-Rpc-Object-Graph-Edge)
    - [Rpc.Object.Graph.Request](#anytype-Rpc-Object-Graph-Request)
    - [Rpc.Object.Graph.Response](#anytype-Rpc-Object-Graph-Response)
    - [Rpc.Object.Graph.Response.Error](#anytype-Rpc-Object-Graph-Response-Error)
    - [Rpc.Object.GroupsSubscribe](#anytype-Rpc-Object-GroupsSubscribe)
    - [Rpc.Object.GroupsSubscribe.Request](#anytype-Rpc-Object-GroupsSubscribe-Request)
    - [Rpc.Object.GroupsSubscribe.Response](#anytype-Rpc-Object-GroupsSubscribe-Response)
    - [Rpc.Object.GroupsSubscribe.Response.Error](#anytype-Rpc-Object-GroupsSubscribe-Response-Error)
    - [Rpc.Object.Import](#anytype-Rpc-Object-Import)
    - [Rpc.Object.Import.Notion](#anytype-Rpc-Object-Import-Notion)
    - [Rpc.Object.Import.Notion.ValidateToken](#anytype-Rpc-Object-Import-Notion-ValidateToken)
    - [Rpc.Object.Import.Notion.ValidateToken.Request](#anytype-Rpc-Object-Import-Notion-ValidateToken-Request)
    - [Rpc.Object.Import.Notion.ValidateToken.Response](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response)
    - [Rpc.Object.Import.Notion.ValidateToken.Response.Error](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error)
    - [Rpc.Object.Import.Request](#anytype-Rpc-Object-Import-Request)
    - [Rpc.Object.Import.Request.BookmarksParams](#anytype-Rpc-Object-Import-Request-BookmarksParams)
    - [Rpc.Object.Import.Request.CsvParams](#anytype-Rpc-Object-Import-Request-CsvParams)
    - [Rpc.Object.Import.Request.HtmlParams](#anytype-Rpc-Object-Import-Request-HtmlParams)
    - [Rpc.Object.Import.Request.MarkdownParams](#anytype-Rpc-Object-Import-Request-MarkdownParams)
    - [Rpc.Object.Import.Request.NotionParams](#anytype-Rpc-Object-Import-Request-NotionParams)
    - [Rpc.Object.Import.Request.PbParams](#anytype-Rpc-Object-Import-Request-PbParams)
    - [Rpc.Object.Import.Request.Snapshot](#anytype-Rpc-Object-Import-Request-Snapshot)
    - [Rpc.Object.Import.Request.TxtParams](#anytype-Rpc-Object-Import-Request-TxtParams)
    - [Rpc.Object.Import.Response](#anytype-Rpc-Object-Import-Response)
    - [Rpc.Object.Import.Response.Error](#anytype-Rpc-Object-Import-Response-Error)
    - [Rpc.Object.ImportList](#anytype-Rpc-Object-ImportList)
    - [Rpc.Object.ImportList.ImportResponse](#anytype-Rpc-Object-ImportList-ImportResponse)
    - [Rpc.Object.ImportList.Request](#anytype-Rpc-Object-ImportList-Request)
    - [Rpc.Object.ImportList.Response](#anytype-Rpc-Object-ImportList-Response)
    - [Rpc.Object.ImportList.Response.Error](#anytype-Rpc-Object-ImportList-Response-Error)
    - [Rpc.Object.ListDelete](#anytype-Rpc-Object-ListDelete)
    - [Rpc.Object.ListDelete.Request](#anytype-Rpc-Object-ListDelete-Request)
    - [Rpc.Object.ListDelete.Response](#anytype-Rpc-Object-ListDelete-Response)
    - [Rpc.Object.ListDelete.Response.Error](#anytype-Rpc-Object-ListDelete-Response-Error)
    - [Rpc.Object.ListDuplicate](#anytype-Rpc-Object-ListDuplicate)
    - [Rpc.Object.ListDuplicate.Request](#anytype-Rpc-Object-ListDuplicate-Request)
    - [Rpc.Object.ListDuplicate.Response](#anytype-Rpc-Object-ListDuplicate-Response)
    - [Rpc.Object.ListDuplicate.Response.Error](#anytype-Rpc-Object-ListDuplicate-Response-Error)
    - [Rpc.Object.ListExport](#anytype-Rpc-Object-ListExport)
    - [Rpc.Object.ListExport.Request](#anytype-Rpc-Object-ListExport-Request)
    - [Rpc.Object.ListExport.Response](#anytype-Rpc-Object-ListExport-Response)
    - [Rpc.Object.ListExport.Response.Error](#anytype-Rpc-Object-ListExport-Response-Error)
    - [Rpc.Object.ListSetIsArchived](#anytype-Rpc-Object-ListSetIsArchived)
    - [Rpc.Object.ListSetIsArchived.Request](#anytype-Rpc-Object-ListSetIsArchived-Request)
    - [Rpc.Object.ListSetIsArchived.Response](#anytype-Rpc-Object-ListSetIsArchived-Response)
    - [Rpc.Object.ListSetIsArchived.Response.Error](#anytype-Rpc-Object-ListSetIsArchived-Response-Error)
    - [Rpc.Object.ListSetIsFavorite](#anytype-Rpc-Object-ListSetIsFavorite)
    - [Rpc.Object.ListSetIsFavorite.Request](#anytype-Rpc-Object-ListSetIsFavorite-Request)
    - [Rpc.Object.ListSetIsFavorite.Response](#anytype-Rpc-Object-ListSetIsFavorite-Response)
    - [Rpc.Object.ListSetIsFavorite.Response.Error](#anytype-Rpc-Object-ListSetIsFavorite-Response-Error)
    - [Rpc.Object.Open](#anytype-Rpc-Object-Open)
    - [Rpc.Object.Open.Request](#anytype-Rpc-Object-Open-Request)
    - [Rpc.Object.Open.Response](#anytype-Rpc-Object-Open-Response)
    - [Rpc.Object.Open.Response.Error](#anytype-Rpc-Object-Open-Response-Error)
    - [Rpc.Object.OpenBreadcrumbs](#anytype-Rpc-Object-OpenBreadcrumbs)
    - [Rpc.Object.OpenBreadcrumbs.Request](#anytype-Rpc-Object-OpenBreadcrumbs-Request)
    - [Rpc.Object.OpenBreadcrumbs.Response](#anytype-Rpc-Object-OpenBreadcrumbs-Response)
    - [Rpc.Object.OpenBreadcrumbs.Response.Error](#anytype-Rpc-Object-OpenBreadcrumbs-Response-Error)
    - [Rpc.Object.Redo](#anytype-Rpc-Object-Redo)
    - [Rpc.Object.Redo.Request](#anytype-Rpc-Object-Redo-Request)
    - [Rpc.Object.Redo.Response](#anytype-Rpc-Object-Redo-Response)
    - [Rpc.Object.Redo.Response.Error](#anytype-Rpc-Object-Redo-Response-Error)
    - [Rpc.Object.Search](#anytype-Rpc-Object-Search)
    - [Rpc.Object.Search.Request](#anytype-Rpc-Object-Search-Request)
    - [Rpc.Object.Search.Response](#anytype-Rpc-Object-Search-Response)
    - [Rpc.Object.Search.Response.Error](#anytype-Rpc-Object-Search-Response-Error)
    - [Rpc.Object.SearchSubscribe](#anytype-Rpc-Object-SearchSubscribe)
    - [Rpc.Object.SearchSubscribe.Request](#anytype-Rpc-Object-SearchSubscribe-Request)
    - [Rpc.Object.SearchSubscribe.Response](#anytype-Rpc-Object-SearchSubscribe-Response)
    - [Rpc.Object.SearchSubscribe.Response.Error](#anytype-Rpc-Object-SearchSubscribe-Response-Error)
    - [Rpc.Object.SearchUnsubscribe](#anytype-Rpc-Object-SearchUnsubscribe)
    - [Rpc.Object.SearchUnsubscribe.Request](#anytype-Rpc-Object-SearchUnsubscribe-Request)
    - [Rpc.Object.SearchUnsubscribe.Response](#anytype-Rpc-Object-SearchUnsubscribe-Response)
    - [Rpc.Object.SearchUnsubscribe.Response.Error](#anytype-Rpc-Object-SearchUnsubscribe-Response-Error)
    - [Rpc.Object.SetBreadcrumbs](#anytype-Rpc-Object-SetBreadcrumbs)
    - [Rpc.Object.SetBreadcrumbs.Request](#anytype-Rpc-Object-SetBreadcrumbs-Request)
    - [Rpc.Object.SetBreadcrumbs.Response](#anytype-Rpc-Object-SetBreadcrumbs-Response)
    - [Rpc.Object.SetBreadcrumbs.Response.Error](#anytype-Rpc-Object-SetBreadcrumbs-Response-Error)
    - [Rpc.Object.SetDetails](#anytype-Rpc-Object-SetDetails)
    - [Rpc.Object.SetDetails.Detail](#anytype-Rpc-Object-SetDetails-Detail)
    - [Rpc.Object.SetDetails.Request](#anytype-Rpc-Object-SetDetails-Request)
    - [Rpc.Object.SetDetails.Response](#anytype-Rpc-Object-SetDetails-Response)
    - [Rpc.Object.SetDetails.Response.Error](#anytype-Rpc-Object-SetDetails-Response-Error)
    - [Rpc.Object.SetInternalFlags](#anytype-Rpc-Object-SetInternalFlags)
    - [Rpc.Object.SetInternalFlags.Request](#anytype-Rpc-Object-SetInternalFlags-Request)
    - [Rpc.Object.SetInternalFlags.Response](#anytype-Rpc-Object-SetInternalFlags-Response)
    - [Rpc.Object.SetInternalFlags.Response.Error](#anytype-Rpc-Object-SetInternalFlags-Response-Error)
    - [Rpc.Object.SetIsArchived](#anytype-Rpc-Object-SetIsArchived)
    - [Rpc.Object.SetIsArchived.Request](#anytype-Rpc-Object-SetIsArchived-Request)
    - [Rpc.Object.SetIsArchived.Response](#anytype-Rpc-Object-SetIsArchived-Response)
    - [Rpc.Object.SetIsArchived.Response.Error](#anytype-Rpc-Object-SetIsArchived-Response-Error)
    - [Rpc.Object.SetIsFavorite](#anytype-Rpc-Object-SetIsFavorite)
    - [Rpc.Object.SetIsFavorite.Request](#anytype-Rpc-Object-SetIsFavorite-Request)
    - [Rpc.Object.SetIsFavorite.Response](#anytype-Rpc-Object-SetIsFavorite-Response)
    - [Rpc.Object.SetIsFavorite.Response.Error](#anytype-Rpc-Object-SetIsFavorite-Response-Error)
    - [Rpc.Object.SetLayout](#anytype-Rpc-Object-SetLayout)
    - [Rpc.Object.SetLayout.Request](#anytype-Rpc-Object-SetLayout-Request)
    - [Rpc.Object.SetLayout.Response](#anytype-Rpc-Object-SetLayout-Response)
    - [Rpc.Object.SetLayout.Response.Error](#anytype-Rpc-Object-SetLayout-Response-Error)
    - [Rpc.Object.SetObjectType](#anytype-Rpc-Object-SetObjectType)
    - [Rpc.Object.SetObjectType.Request](#anytype-Rpc-Object-SetObjectType-Request)
    - [Rpc.Object.SetObjectType.Response](#anytype-Rpc-Object-SetObjectType-Response)
    - [Rpc.Object.SetObjectType.Response.Error](#anytype-Rpc-Object-SetObjectType-Response-Error)
    - [Rpc.Object.SetSource](#anytype-Rpc-Object-SetSource)
    - [Rpc.Object.SetSource.Request](#anytype-Rpc-Object-SetSource-Request)
    - [Rpc.Object.SetSource.Response](#anytype-Rpc-Object-SetSource-Response)
    - [Rpc.Object.SetSource.Response.Error](#anytype-Rpc-Object-SetSource-Response-Error)
    - [Rpc.Object.ShareByLink](#anytype-Rpc-Object-ShareByLink)
    - [Rpc.Object.ShareByLink.Request](#anytype-Rpc-Object-ShareByLink-Request)
    - [Rpc.Object.ShareByLink.Response](#anytype-Rpc-Object-ShareByLink-Response)
    - [Rpc.Object.ShareByLink.Response.Error](#anytype-Rpc-Object-ShareByLink-Response-Error)
    - [Rpc.Object.Show](#anytype-Rpc-Object-Show)
    - [Rpc.Object.Show.Request](#anytype-Rpc-Object-Show-Request)
    - [Rpc.Object.Show.Response](#anytype-Rpc-Object-Show-Response)
    - [Rpc.Object.Show.Response.Error](#anytype-Rpc-Object-Show-Response-Error)
    - [Rpc.Object.SubscribeIds](#anytype-Rpc-Object-SubscribeIds)
    - [Rpc.Object.SubscribeIds.Request](#anytype-Rpc-Object-SubscribeIds-Request)
    - [Rpc.Object.SubscribeIds.Response](#anytype-Rpc-Object-SubscribeIds-Response)
    - [Rpc.Object.SubscribeIds.Response.Error](#anytype-Rpc-Object-SubscribeIds-Response-Error)
    - [Rpc.Object.ToBookmark](#anytype-Rpc-Object-ToBookmark)
    - [Rpc.Object.ToBookmark.Request](#anytype-Rpc-Object-ToBookmark-Request)
    - [Rpc.Object.ToBookmark.Response](#anytype-Rpc-Object-ToBookmark-Response)
    - [Rpc.Object.ToBookmark.Response.Error](#anytype-Rpc-Object-ToBookmark-Response-Error)
    - [Rpc.Object.ToCollection](#anytype-Rpc-Object-ToCollection)
    - [Rpc.Object.ToCollection.Request](#anytype-Rpc-Object-ToCollection-Request)
    - [Rpc.Object.ToCollection.Response](#anytype-Rpc-Object-ToCollection-Response)
    - [Rpc.Object.ToCollection.Response.Error](#anytype-Rpc-Object-ToCollection-Response-Error)
    - [Rpc.Object.ToSet](#anytype-Rpc-Object-ToSet)
    - [Rpc.Object.ToSet.Request](#anytype-Rpc-Object-ToSet-Request)
    - [Rpc.Object.ToSet.Response](#anytype-Rpc-Object-ToSet-Response)
    - [Rpc.Object.ToSet.Response.Error](#anytype-Rpc-Object-ToSet-Response-Error)
    - [Rpc.Object.Undo](#anytype-Rpc-Object-Undo)
    - [Rpc.Object.Undo.Request](#anytype-Rpc-Object-Undo-Request)
    - [Rpc.Object.Undo.Response](#anytype-Rpc-Object-Undo-Response)
    - [Rpc.Object.Undo.Response.Error](#anytype-Rpc-Object-Undo-Response-Error)
    - [Rpc.Object.UndoRedoCounter](#anytype-Rpc-Object-UndoRedoCounter)
    - [Rpc.Object.WorkspaceSetDashboard](#anytype-Rpc-Object-WorkspaceSetDashboard)
    - [Rpc.Object.WorkspaceSetDashboard.Request](#anytype-Rpc-Object-WorkspaceSetDashboard-Request)
    - [Rpc.Object.WorkspaceSetDashboard.Response](#anytype-Rpc-Object-WorkspaceSetDashboard-Response)
    - [Rpc.Object.WorkspaceSetDashboard.Response.Error](#anytype-Rpc-Object-WorkspaceSetDashboard-Response-Error)
    - [Rpc.ObjectCollection](#anytype-Rpc-ObjectCollection)
    - [Rpc.ObjectCollection.Add](#anytype-Rpc-ObjectCollection-Add)
    - [Rpc.ObjectCollection.Add.Request](#anytype-Rpc-ObjectCollection-Add-Request)
    - [Rpc.ObjectCollection.Add.Response](#anytype-Rpc-ObjectCollection-Add-Response)
    - [Rpc.ObjectCollection.Add.Response.Error](#anytype-Rpc-ObjectCollection-Add-Response-Error)
    - [Rpc.ObjectCollection.Remove](#anytype-Rpc-ObjectCollection-Remove)
    - [Rpc.ObjectCollection.Remove.Request](#anytype-Rpc-ObjectCollection-Remove-Request)
    - [Rpc.ObjectCollection.Remove.Response](#anytype-Rpc-ObjectCollection-Remove-Response)
    - [Rpc.ObjectCollection.Remove.Response.Error](#anytype-Rpc-ObjectCollection-Remove-Response-Error)
    - [Rpc.ObjectCollection.Sort](#anytype-Rpc-ObjectCollection-Sort)
    - [Rpc.ObjectCollection.Sort.Request](#anytype-Rpc-ObjectCollection-Sort-Request)
    - [Rpc.ObjectCollection.Sort.Response](#anytype-Rpc-ObjectCollection-Sort-Response)
    - [Rpc.ObjectCollection.Sort.Response.Error](#anytype-Rpc-ObjectCollection-Sort-Response-Error)
    - [Rpc.ObjectRelation](#anytype-Rpc-ObjectRelation)
    - [Rpc.ObjectRelation.Add](#anytype-Rpc-ObjectRelation-Add)
    - [Rpc.ObjectRelation.Add.Request](#anytype-Rpc-ObjectRelation-Add-Request)
    - [Rpc.ObjectRelation.Add.Response](#anytype-Rpc-ObjectRelation-Add-Response)
    - [Rpc.ObjectRelation.Add.Response.Error](#anytype-Rpc-ObjectRelation-Add-Response-Error)
    - [Rpc.ObjectRelation.AddFeatured](#anytype-Rpc-ObjectRelation-AddFeatured)
    - [Rpc.ObjectRelation.AddFeatured.Request](#anytype-Rpc-ObjectRelation-AddFeatured-Request)
    - [Rpc.ObjectRelation.AddFeatured.Response](#anytype-Rpc-ObjectRelation-AddFeatured-Response)
    - [Rpc.ObjectRelation.AddFeatured.Response.Error](#anytype-Rpc-ObjectRelation-AddFeatured-Response-Error)
    - [Rpc.ObjectRelation.Delete](#anytype-Rpc-ObjectRelation-Delete)
    - [Rpc.ObjectRelation.Delete.Request](#anytype-Rpc-ObjectRelation-Delete-Request)
    - [Rpc.ObjectRelation.Delete.Response](#anytype-Rpc-ObjectRelation-Delete-Response)
    - [Rpc.ObjectRelation.Delete.Response.Error](#anytype-Rpc-ObjectRelation-Delete-Response-Error)
    - [Rpc.ObjectRelation.ListAvailable](#anytype-Rpc-ObjectRelation-ListAvailable)
    - [Rpc.ObjectRelation.ListAvailable.Request](#anytype-Rpc-ObjectRelation-ListAvailable-Request)
    - [Rpc.ObjectRelation.ListAvailable.Response](#anytype-Rpc-ObjectRelation-ListAvailable-Response)
    - [Rpc.ObjectRelation.ListAvailable.Response.Error](#anytype-Rpc-ObjectRelation-ListAvailable-Response-Error)
    - [Rpc.ObjectRelation.RemoveFeatured](#anytype-Rpc-ObjectRelation-RemoveFeatured)
    - [Rpc.ObjectRelation.RemoveFeatured.Request](#anytype-Rpc-ObjectRelation-RemoveFeatured-Request)
    - [Rpc.ObjectRelation.RemoveFeatured.Response](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response)
    - [Rpc.ObjectRelation.RemoveFeatured.Response.Error](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response-Error)
    - [Rpc.ObjectType](#anytype-Rpc-ObjectType)
    - [Rpc.ObjectType.Relation](#anytype-Rpc-ObjectType-Relation)
    - [Rpc.ObjectType.Relation.Add](#anytype-Rpc-ObjectType-Relation-Add)
    - [Rpc.ObjectType.Relation.Add.Request](#anytype-Rpc-ObjectType-Relation-Add-Request)
    - [Rpc.ObjectType.Relation.Add.Response](#anytype-Rpc-ObjectType-Relation-Add-Response)
    - [Rpc.ObjectType.Relation.Add.Response.Error](#anytype-Rpc-ObjectType-Relation-Add-Response-Error)
    - [Rpc.ObjectType.Relation.List](#anytype-Rpc-ObjectType-Relation-List)
    - [Rpc.ObjectType.Relation.List.Request](#anytype-Rpc-ObjectType-Relation-List-Request)
    - [Rpc.ObjectType.Relation.List.Response](#anytype-Rpc-ObjectType-Relation-List-Response)
    - [Rpc.ObjectType.Relation.List.Response.Error](#anytype-Rpc-ObjectType-Relation-List-Response-Error)
    - [Rpc.ObjectType.Relation.Remove](#anytype-Rpc-ObjectType-Relation-Remove)
    - [Rpc.ObjectType.Relation.Remove.Request](#anytype-Rpc-ObjectType-Relation-Remove-Request)
    - [Rpc.ObjectType.Relation.Remove.Response](#anytype-Rpc-ObjectType-Relation-Remove-Response)
    - [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype-Rpc-ObjectType-Relation-Remove-Response-Error)
    - [Rpc.Process](#anytype-Rpc-Process)
    - [Rpc.Process.Cancel](#anytype-Rpc-Process-Cancel)
    - [Rpc.Process.Cancel.Request](#anytype-Rpc-Process-Cancel-Request)
    - [Rpc.Process.Cancel.Response](#anytype-Rpc-Process-Cancel-Response)
    - [Rpc.Process.Cancel.Response.Error](#anytype-Rpc-Process-Cancel-Response-Error)
    - [Rpc.Relation](#anytype-Rpc-Relation)
    - [Rpc.Relation.ListRemoveOption](#anytype-Rpc-Relation-ListRemoveOption)
    - [Rpc.Relation.ListRemoveOption.Request](#anytype-Rpc-Relation-ListRemoveOption-Request)
    - [Rpc.Relation.ListRemoveOption.Response](#anytype-Rpc-Relation-ListRemoveOption-Response)
    - [Rpc.Relation.ListRemoveOption.Response.Error](#anytype-Rpc-Relation-ListRemoveOption-Response-Error)
    - [Rpc.Relation.Options](#anytype-Rpc-Relation-Options)
    - [Rpc.Relation.Options.Request](#anytype-Rpc-Relation-Options-Request)
    - [Rpc.Relation.Options.Response](#anytype-Rpc-Relation-Options-Response)
    - [Rpc.Relation.Options.Response.Error](#anytype-Rpc-Relation-Options-Response-Error)
    - [Rpc.Template](#anytype-Rpc-Template)
    - [Rpc.Template.Clone](#anytype-Rpc-Template-Clone)
    - [Rpc.Template.Clone.Request](#anytype-Rpc-Template-Clone-Request)
    - [Rpc.Template.Clone.Response](#anytype-Rpc-Template-Clone-Response)
    - [Rpc.Template.Clone.Response.Error](#anytype-Rpc-Template-Clone-Response-Error)
    - [Rpc.Template.CreateFromObject](#anytype-Rpc-Template-CreateFromObject)
    - [Rpc.Template.CreateFromObject.Request](#anytype-Rpc-Template-CreateFromObject-Request)
    - [Rpc.Template.CreateFromObject.Response](#anytype-Rpc-Template-CreateFromObject-Response)
    - [Rpc.Template.CreateFromObject.Response.Error](#anytype-Rpc-Template-CreateFromObject-Response-Error)
    - [Rpc.Template.CreateFromObjectType](#anytype-Rpc-Template-CreateFromObjectType)
    - [Rpc.Template.CreateFromObjectType.Request](#anytype-Rpc-Template-CreateFromObjectType-Request)
    - [Rpc.Template.CreateFromObjectType.Response](#anytype-Rpc-Template-CreateFromObjectType-Response)
    - [Rpc.Template.CreateFromObjectType.Response.Error](#anytype-Rpc-Template-CreateFromObjectType-Response-Error)
    - [Rpc.Template.ExportAll](#anytype-Rpc-Template-ExportAll)
    - [Rpc.Template.ExportAll.Request](#anytype-Rpc-Template-ExportAll-Request)
    - [Rpc.Template.ExportAll.Response](#anytype-Rpc-Template-ExportAll-Response)
    - [Rpc.Template.ExportAll.Response.Error](#anytype-Rpc-Template-ExportAll-Response-Error)
    - [Rpc.Unsplash](#anytype-Rpc-Unsplash)
    - [Rpc.Unsplash.Download](#anytype-Rpc-Unsplash-Download)
    - [Rpc.Unsplash.Download.Request](#anytype-Rpc-Unsplash-Download-Request)
    - [Rpc.Unsplash.Download.Response](#anytype-Rpc-Unsplash-Download-Response)
    - [Rpc.Unsplash.Download.Response.Error](#anytype-Rpc-Unsplash-Download-Response-Error)
    - [Rpc.Unsplash.Search](#anytype-Rpc-Unsplash-Search)
    - [Rpc.Unsplash.Search.Request](#anytype-Rpc-Unsplash-Search-Request)
    - [Rpc.Unsplash.Search.Response](#anytype-Rpc-Unsplash-Search-Response)
    - [Rpc.Unsplash.Search.Response.Error](#anytype-Rpc-Unsplash-Search-Response-Error)
    - [Rpc.Unsplash.Search.Response.Picture](#anytype-Rpc-Unsplash-Search-Response-Picture)
    - [Rpc.UserData](#anytype-Rpc-UserData)
    - [Rpc.UserData.Dump](#anytype-Rpc-UserData-Dump)
    - [Rpc.UserData.Dump.Request](#anytype-Rpc-UserData-Dump-Request)
    - [Rpc.UserData.Dump.Response](#anytype-Rpc-UserData-Dump-Response)
    - [Rpc.UserData.Dump.Response.Error](#anytype-Rpc-UserData-Dump-Response-Error)
    - [Rpc.Wallet](#anytype-Rpc-Wallet)
    - [Rpc.Wallet.CloseSession](#anytype-Rpc-Wallet-CloseSession)
    - [Rpc.Wallet.CloseSession.Request](#anytype-Rpc-Wallet-CloseSession-Request)
    - [Rpc.Wallet.CloseSession.Response](#anytype-Rpc-Wallet-CloseSession-Response)
    - [Rpc.Wallet.CloseSession.Response.Error](#anytype-Rpc-Wallet-CloseSession-Response-Error)
    - [Rpc.Wallet.Convert](#anytype-Rpc-Wallet-Convert)
    - [Rpc.Wallet.Convert.Request](#anytype-Rpc-Wallet-Convert-Request)
    - [Rpc.Wallet.Convert.Response](#anytype-Rpc-Wallet-Convert-Response)
    - [Rpc.Wallet.Convert.Response.Error](#anytype-Rpc-Wallet-Convert-Response-Error)
    - [Rpc.Wallet.Create](#anytype-Rpc-Wallet-Create)
    - [Rpc.Wallet.Create.Request](#anytype-Rpc-Wallet-Create-Request)
    - [Rpc.Wallet.Create.Response](#anytype-Rpc-Wallet-Create-Response)
    - [Rpc.Wallet.Create.Response.Error](#anytype-Rpc-Wallet-Create-Response-Error)
    - [Rpc.Wallet.CreateSession](#anytype-Rpc-Wallet-CreateSession)
    - [Rpc.Wallet.CreateSession.Request](#anytype-Rpc-Wallet-CreateSession-Request)
    - [Rpc.Wallet.CreateSession.Response](#anytype-Rpc-Wallet-CreateSession-Response)
    - [Rpc.Wallet.CreateSession.Response.Error](#anytype-Rpc-Wallet-CreateSession-Response-Error)
    - [Rpc.Wallet.Recover](#anytype-Rpc-Wallet-Recover)
    - [Rpc.Wallet.Recover.Request](#anytype-Rpc-Wallet-Recover-Request)
    - [Rpc.Wallet.Recover.Response](#anytype-Rpc-Wallet-Recover-Response)
    - [Rpc.Wallet.Recover.Response.Error](#anytype-Rpc-Wallet-Recover-Response-Error)
    - [Rpc.Workspace](#anytype-Rpc-Workspace)
    - [Rpc.Workspace.Create](#anytype-Rpc-Workspace-Create)
    - [Rpc.Workspace.Create.Request](#anytype-Rpc-Workspace-Create-Request)
    - [Rpc.Workspace.Create.Response](#anytype-Rpc-Workspace-Create-Response)
    - [Rpc.Workspace.Create.Response.Error](#anytype-Rpc-Workspace-Create-Response-Error)
    - [Rpc.Workspace.Export](#anytype-Rpc-Workspace-Export)
    - [Rpc.Workspace.Export.Request](#anytype-Rpc-Workspace-Export-Request)
    - [Rpc.Workspace.Export.Response](#anytype-Rpc-Workspace-Export-Response)
    - [Rpc.Workspace.Export.Response.Error](#anytype-Rpc-Workspace-Export-Response-Error)
    - [Rpc.Workspace.GetAll](#anytype-Rpc-Workspace-GetAll)
    - [Rpc.Workspace.GetAll.Request](#anytype-Rpc-Workspace-GetAll-Request)
    - [Rpc.Workspace.GetAll.Response](#anytype-Rpc-Workspace-GetAll-Response)
    - [Rpc.Workspace.GetAll.Response.Error](#anytype-Rpc-Workspace-GetAll-Response-Error)
    - [Rpc.Workspace.GetCurrent](#anytype-Rpc-Workspace-GetCurrent)
    - [Rpc.Workspace.GetCurrent.Request](#anytype-Rpc-Workspace-GetCurrent-Request)
    - [Rpc.Workspace.GetCurrent.Response](#anytype-Rpc-Workspace-GetCurrent-Response)
    - [Rpc.Workspace.GetCurrent.Response.Error](#anytype-Rpc-Workspace-GetCurrent-Response-Error)
    - [Rpc.Workspace.Object](#anytype-Rpc-Workspace-Object)
    - [Rpc.Workspace.Object.Add](#anytype-Rpc-Workspace-Object-Add)
    - [Rpc.Workspace.Object.Add.Request](#anytype-Rpc-Workspace-Object-Add-Request)
    - [Rpc.Workspace.Object.Add.Response](#anytype-Rpc-Workspace-Object-Add-Response)
    - [Rpc.Workspace.Object.Add.Response.Error](#anytype-Rpc-Workspace-Object-Add-Response-Error)
    - [Rpc.Workspace.Object.ListAdd](#anytype-Rpc-Workspace-Object-ListAdd)
    - [Rpc.Workspace.Object.ListAdd.Request](#anytype-Rpc-Workspace-Object-ListAdd-Request)
    - [Rpc.Workspace.Object.ListAdd.Response](#anytype-Rpc-Workspace-Object-ListAdd-Response)
    - [Rpc.Workspace.Object.ListAdd.Response.Error](#anytype-Rpc-Workspace-Object-ListAdd-Response-Error)
    - [Rpc.Workspace.Object.ListRemove](#anytype-Rpc-Workspace-Object-ListRemove)
    - [Rpc.Workspace.Object.ListRemove.Request](#anytype-Rpc-Workspace-Object-ListRemove-Request)
    - [Rpc.Workspace.Object.ListRemove.Response](#anytype-Rpc-Workspace-Object-ListRemove-Response)
    - [Rpc.Workspace.Object.ListRemove.Response.Error](#anytype-Rpc-Workspace-Object-ListRemove-Response-Error)
    - [Rpc.Workspace.Select](#anytype-Rpc-Workspace-Select)
    - [Rpc.Workspace.Select.Request](#anytype-Rpc-Workspace-Select-Request)
    - [Rpc.Workspace.Select.Response](#anytype-Rpc-Workspace-Select-Response)
    - [Rpc.Workspace.Select.Response.Error](#anytype-Rpc-Workspace-Select-Response-Error)
    - [Rpc.Workspace.SetIsHighlighted](#anytype-Rpc-Workspace-SetIsHighlighted)
    - [Rpc.Workspace.SetIsHighlighted.Request](#anytype-Rpc-Workspace-SetIsHighlighted-Request)
    - [Rpc.Workspace.SetIsHighlighted.Response](#anytype-Rpc-Workspace-SetIsHighlighted-Response)
    - [Rpc.Workspace.SetIsHighlighted.Response.Error](#anytype-Rpc-Workspace-SetIsHighlighted-Response-Error)
    - [StreamRequest](#anytype-StreamRequest)
  
    - [Rpc.Account.ConfigUpdate.Response.Error.Code](#anytype-Rpc-Account-ConfigUpdate-Response-Error-Code)
    - [Rpc.Account.ConfigUpdate.Timezones](#anytype-Rpc-Account-ConfigUpdate-Timezones)
    - [Rpc.Account.Create.Response.Error.Code](#anytype-Rpc-Account-Create-Response-Error-Code)
    - [Rpc.Account.Delete.Response.Error.Code](#anytype-Rpc-Account-Delete-Response-Error-Code)
    - [Rpc.Account.Move.Response.Error.Code](#anytype-Rpc-Account-Move-Response-Error-Code)
    - [Rpc.Account.Recover.Response.Error.Code](#anytype-Rpc-Account-Recover-Response-Error-Code)
    - [Rpc.Account.RecoverFromLegacyExport.Response.Error.Code](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error-Code)
    - [Rpc.Account.Select.Response.Error.Code](#anytype-Rpc-Account-Select-Response-Error-Code)
    - [Rpc.Account.Stop.Response.Error.Code](#anytype-Rpc-Account-Stop-Response-Error-Code)
    - [Rpc.App.GetVersion.Response.Error.Code](#anytype-Rpc-App-GetVersion-Response-Error-Code)
    - [Rpc.App.SetDeviceState.Request.DeviceState](#anytype-Rpc-App-SetDeviceState-Request-DeviceState)
    - [Rpc.App.SetDeviceState.Response.Error.Code](#anytype-Rpc-App-SetDeviceState-Response-Error-Code)
    - [Rpc.App.Shutdown.Response.Error.Code](#anytype-Rpc-App-Shutdown-Response-Error-Code)
    - [Rpc.Block.Copy.Response.Error.Code](#anytype-Rpc-Block-Copy-Response-Error-Code)
    - [Rpc.Block.Create.Response.Error.Code](#anytype-Rpc-Block-Create-Response-Error-Code)
    - [Rpc.Block.CreateWidget.Response.Error.Code](#anytype-Rpc-Block-CreateWidget-Response-Error-Code)
    - [Rpc.Block.Cut.Response.Error.Code](#anytype-Rpc-Block-Cut-Response-Error-Code)
    - [Rpc.Block.Download.Response.Error.Code](#anytype-Rpc-Block-Download-Response-Error-Code)
    - [Rpc.Block.Export.Response.Error.Code](#anytype-Rpc-Block-Export-Response-Error-Code)
    - [Rpc.Block.ListConvertToObjects.Response.Error.Code](#anytype-Rpc-Block-ListConvertToObjects-Response-Error-Code)
    - [Rpc.Block.ListDelete.Response.Error.Code](#anytype-Rpc-Block-ListDelete-Response-Error-Code)
    - [Rpc.Block.ListDuplicate.Response.Error.Code](#anytype-Rpc-Block-ListDuplicate-Response-Error-Code)
    - [Rpc.Block.ListMoveToExistingObject.Response.Error.Code](#anytype-Rpc-Block-ListMoveToExistingObject-Response-Error-Code)
    - [Rpc.Block.ListMoveToNewObject.Response.Error.Code](#anytype-Rpc-Block-ListMoveToNewObject-Response-Error-Code)
    - [Rpc.Block.ListSetAlign.Response.Error.Code](#anytype-Rpc-Block-ListSetAlign-Response-Error-Code)
    - [Rpc.Block.ListSetBackgroundColor.Response.Error.Code](#anytype-Rpc-Block-ListSetBackgroundColor-Response-Error-Code)
    - [Rpc.Block.ListSetFields.Response.Error.Code](#anytype-Rpc-Block-ListSetFields-Response-Error-Code)
    - [Rpc.Block.ListSetVerticalAlign.Response.Error.Code](#anytype-Rpc-Block-ListSetVerticalAlign-Response-Error-Code)
    - [Rpc.Block.ListTurnInto.Response.Error.Code](#anytype-Rpc-Block-ListTurnInto-Response-Error-Code)
    - [Rpc.Block.Merge.Response.Error.Code](#anytype-Rpc-Block-Merge-Response-Error-Code)
    - [Rpc.Block.Paste.Response.Error.Code](#anytype-Rpc-Block-Paste-Response-Error-Code)
    - [Rpc.Block.Replace.Response.Error.Code](#anytype-Rpc-Block-Replace-Response-Error-Code)
    - [Rpc.Block.SetFields.Response.Error.Code](#anytype-Rpc-Block-SetFields-Response-Error-Code)
    - [Rpc.Block.Split.Request.Mode](#anytype-Rpc-Block-Split-Request-Mode)
    - [Rpc.Block.Split.Response.Error.Code](#anytype-Rpc-Block-Split-Response-Error-Code)
    - [Rpc.Block.Upload.Response.Error.Code](#anytype-Rpc-Block-Upload-Response-Error-Code)
    - [Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error-Code)
    - [Rpc.BlockBookmark.Fetch.Response.Error.Code](#anytype-Rpc-BlockBookmark-Fetch-Response-Error-Code)
    - [Rpc.BlockDataview.CreateBookmark.Response.Error.Code](#anytype-Rpc-BlockDataview-CreateBookmark-Response-Error-Code)
    - [Rpc.BlockDataview.CreateFromExistingObject.Response.Error.Code](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Response-Error-Code)
    - [Rpc.BlockDataview.Filter.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Add-Response-Error-Code)
    - [Rpc.BlockDataview.Filter.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Remove-Response-Error-Code)
    - [Rpc.BlockDataview.Filter.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Replace-Response-Error-Code)
    - [Rpc.BlockDataview.Filter.Sort.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Sort-Response-Error-Code)
    - [Rpc.BlockDataview.GroupOrder.Update.Response.Error.Code](#anytype-Rpc-BlockDataview-GroupOrder-Update-Response-Error-Code)
    - [Rpc.BlockDataview.ObjectOrder.Move.Response.Error.Code](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Response-Error-Code)
    - [Rpc.BlockDataview.ObjectOrder.Update.Response.Error.Code](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Response-Error-Code)
    - [Rpc.BlockDataview.Relation.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-Add-Response-Error-Code)
    - [Rpc.BlockDataview.Relation.Delete.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-Delete-Response-Error-Code)
    - [Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Response-Error-Code)
    - [Rpc.BlockDataview.SetSource.Response.Error.Code](#anytype-Rpc-BlockDataview-SetSource-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Add-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Remove-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Replace-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Sort.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Sort-Response-Error-Code)
    - [Rpc.BlockDataview.View.Create.Response.Error.Code](#anytype-Rpc-BlockDataview-View-Create-Response-Error-Code)
    - [Rpc.BlockDataview.View.Delete.Response.Error.Code](#anytype-Rpc-BlockDataview-View-Delete-Response-Error-Code)
    - [Rpc.BlockDataview.View.SetActive.Response.Error.Code](#anytype-Rpc-BlockDataview-View-SetActive-Response-Error-Code)
    - [Rpc.BlockDataview.View.SetPosition.Response.Error.Code](#anytype-Rpc-BlockDataview-View-SetPosition-Response-Error-Code)
    - [Rpc.BlockDataview.View.Update.Response.Error.Code](#anytype-Rpc-BlockDataview-View-Update-Response-Error-Code)
    - [Rpc.BlockDataview.ViewRelation.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Add-Response-Error-Code)
    - [Rpc.BlockDataview.ViewRelation.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Response-Error-Code)
    - [Rpc.BlockDataview.ViewRelation.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Response-Error-Code)
    - [Rpc.BlockDataview.ViewRelation.Sort.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Response-Error-Code)
    - [Rpc.BlockDiv.ListSetStyle.Response.Error.Code](#anytype-Rpc-BlockDiv-ListSetStyle-Response-Error-Code)
    - [Rpc.BlockFile.CreateAndUpload.Response.Error.Code](#anytype-Rpc-BlockFile-CreateAndUpload-Response-Error-Code)
    - [Rpc.BlockFile.ListSetStyle.Response.Error.Code](#anytype-Rpc-BlockFile-ListSetStyle-Response-Error-Code)
    - [Rpc.BlockFile.SetName.Response.Error.Code](#anytype-Rpc-BlockFile-SetName-Response-Error-Code)
    - [Rpc.BlockImage.SetName.Response.Error.Code](#anytype-Rpc-BlockImage-SetName-Response-Error-Code)
    - [Rpc.BlockImage.SetWidth.Response.Error.Code](#anytype-Rpc-BlockImage-SetWidth-Response-Error-Code)
    - [Rpc.BlockLatex.SetText.Response.Error.Code](#anytype-Rpc-BlockLatex-SetText-Response-Error-Code)
    - [Rpc.BlockLink.CreateWithObject.Response.Error.Code](#anytype-Rpc-BlockLink-CreateWithObject-Response-Error-Code)
    - [Rpc.BlockLink.ListSetAppearance.Response.Error.Code](#anytype-Rpc-BlockLink-ListSetAppearance-Response-Error-Code)
    - [Rpc.BlockRelation.Add.Response.Error.Code](#anytype-Rpc-BlockRelation-Add-Response-Error-Code)
    - [Rpc.BlockRelation.SetKey.Response.Error.Code](#anytype-Rpc-BlockRelation-SetKey-Response-Error-Code)
    - [Rpc.BlockTable.ColumnCreate.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnCreate-Response-Error-Code)
    - [Rpc.BlockTable.ColumnDelete.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnDelete-Response-Error-Code)
    - [Rpc.BlockTable.ColumnDuplicate.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnDuplicate-Response-Error-Code)
    - [Rpc.BlockTable.ColumnListFill.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnListFill-Response-Error-Code)
    - [Rpc.BlockTable.ColumnMove.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnMove-Response-Error-Code)
    - [Rpc.BlockTable.Create.Response.Error.Code](#anytype-Rpc-BlockTable-Create-Response-Error-Code)
    - [Rpc.BlockTable.Expand.Response.Error.Code](#anytype-Rpc-BlockTable-Expand-Response-Error-Code)
    - [Rpc.BlockTable.RowCreate.Response.Error.Code](#anytype-Rpc-BlockTable-RowCreate-Response-Error-Code)
    - [Rpc.BlockTable.RowDelete.Response.Error.Code](#anytype-Rpc-BlockTable-RowDelete-Response-Error-Code)
    - [Rpc.BlockTable.RowDuplicate.Response.Error.Code](#anytype-Rpc-BlockTable-RowDuplicate-Response-Error-Code)
    - [Rpc.BlockTable.RowListClean.Response.Error.Code](#anytype-Rpc-BlockTable-RowListClean-Response-Error-Code)
    - [Rpc.BlockTable.RowListFill.Response.Error.Code](#anytype-Rpc-BlockTable-RowListFill-Response-Error-Code)
    - [Rpc.BlockTable.RowSetHeader.Response.Error.Code](#anytype-Rpc-BlockTable-RowSetHeader-Response-Error-Code)
    - [Rpc.BlockTable.Sort.Response.Error.Code](#anytype-Rpc-BlockTable-Sort-Response-Error-Code)
    - [Rpc.BlockText.ListClearContent.Response.Error.Code](#anytype-Rpc-BlockText-ListClearContent-Response-Error-Code)
    - [Rpc.BlockText.ListClearStyle.Response.Error.Code](#anytype-Rpc-BlockText-ListClearStyle-Response-Error-Code)
    - [Rpc.BlockText.ListSetColor.Response.Error.Code](#anytype-Rpc-BlockText-ListSetColor-Response-Error-Code)
    - [Rpc.BlockText.ListSetMark.Response.Error.Code](#anytype-Rpc-BlockText-ListSetMark-Response-Error-Code)
    - [Rpc.BlockText.ListSetStyle.Response.Error.Code](#anytype-Rpc-BlockText-ListSetStyle-Response-Error-Code)
    - [Rpc.BlockText.SetChecked.Response.Error.Code](#anytype-Rpc-BlockText-SetChecked-Response-Error-Code)
    - [Rpc.BlockText.SetColor.Response.Error.Code](#anytype-Rpc-BlockText-SetColor-Response-Error-Code)
    - [Rpc.BlockText.SetIcon.Response.Error.Code](#anytype-Rpc-BlockText-SetIcon-Response-Error-Code)
    - [Rpc.BlockText.SetMarks.Get.Response.Error.Code](#anytype-Rpc-BlockText-SetMarks-Get-Response-Error-Code)
    - [Rpc.BlockText.SetStyle.Response.Error.Code](#anytype-Rpc-BlockText-SetStyle-Response-Error-Code)
    - [Rpc.BlockText.SetText.Response.Error.Code](#anytype-Rpc-BlockText-SetText-Response-Error-Code)
    - [Rpc.BlockVideo.SetName.Response.Error.Code](#anytype-Rpc-BlockVideo-SetName-Response-Error-Code)
    - [Rpc.BlockVideo.SetWidth.Response.Error.Code](#anytype-Rpc-BlockVideo-SetWidth-Response-Error-Code)
    - [Rpc.BlockWidget.SetLayout.Response.Error.Code](#anytype-Rpc-BlockWidget-SetLayout-Response-Error-Code)
    - [Rpc.BlockWidget.SetLimit.Response.Error.Code](#anytype-Rpc-BlockWidget-SetLimit-Response-Error-Code)
    - [Rpc.BlockWidget.SetTargetId.Response.Error.Code](#anytype-Rpc-BlockWidget-SetTargetId-Response-Error-Code)
    - [Rpc.Debug.ExportLocalstore.Response.Error.Code](#anytype-Rpc-Debug-ExportLocalstore-Response-Error-Code)
    - [Rpc.Debug.Ping.Response.Error.Code](#anytype-Rpc-Debug-Ping-Response-Error-Code)
    - [Rpc.Debug.SpaceSummary.Response.Error.Code](#anytype-Rpc-Debug-SpaceSummary-Response-Error-Code)
    - [Rpc.Debug.Tree.Response.Error.Code](#anytype-Rpc-Debug-Tree-Response-Error-Code)
    - [Rpc.Debug.TreeHeads.Response.Error.Code](#anytype-Rpc-Debug-TreeHeads-Response-Error-Code)
    - [Rpc.File.Download.Response.Error.Code](#anytype-Rpc-File-Download-Response-Error-Code)
    - [Rpc.File.Drop.Response.Error.Code](#anytype-Rpc-File-Drop-Response-Error-Code)
    - [Rpc.File.ListOffload.Response.Error.Code](#anytype-Rpc-File-ListOffload-Response-Error-Code)
    - [Rpc.File.Offload.Response.Error.Code](#anytype-Rpc-File-Offload-Response-Error-Code)
    - [Rpc.File.SpaceUsage.Response.Error.Code](#anytype-Rpc-File-SpaceUsage-Response-Error-Code)
    - [Rpc.File.Upload.Response.Error.Code](#anytype-Rpc-File-Upload-Response-Error-Code)
    - [Rpc.GenericErrorResponse.Error.Code](#anytype-Rpc-GenericErrorResponse-Error-Code)
    - [Rpc.History.GetVersions.Response.Error.Code](#anytype-Rpc-History-GetVersions-Response-Error-Code)
    - [Rpc.History.SetVersion.Response.Error.Code](#anytype-Rpc-History-SetVersion-Response-Error-Code)
    - [Rpc.History.ShowVersion.Response.Error.Code](#anytype-Rpc-History-ShowVersion-Response-Error-Code)
    - [Rpc.LinkPreview.Response.Error.Code](#anytype-Rpc-LinkPreview-Response-Error-Code)
    - [Rpc.Log.Send.Request.Level](#anytype-Rpc-Log-Send-Request-Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype-Rpc-Log-Send-Response-Error-Code)
    - [Rpc.Metrics.SetParameters.Response.Error.Code](#anytype-Rpc-Metrics-SetParameters-Response-Error-Code)
    - [Rpc.Navigation.Context](#anytype-Rpc-Navigation-Context)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error-Code)
    - [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype-Rpc-Navigation-ListObjects-Response-Error-Code)
    - [Rpc.Object.ApplyTemplate.Response.Error.Code](#anytype-Rpc-Object-ApplyTemplate-Response-Error-Code)
    - [Rpc.Object.BookmarkFetch.Response.Error.Code](#anytype-Rpc-Object-BookmarkFetch-Response-Error-Code)
    - [Rpc.Object.Close.Response.Error.Code](#anytype-Rpc-Object-Close-Response-Error-Code)
    - [Rpc.Object.Create.Response.Error.Code](#anytype-Rpc-Object-Create-Response-Error-Code)
    - [Rpc.Object.CreateBookmark.Response.Error.Code](#anytype-Rpc-Object-CreateBookmark-Response-Error-Code)
    - [Rpc.Object.CreateObjectType.Response.Error.Code](#anytype-Rpc-Object-CreateObjectType-Response-Error-Code)
    - [Rpc.Object.CreateRelation.Response.Error.Code](#anytype-Rpc-Object-CreateRelation-Response-Error-Code)
    - [Rpc.Object.CreateRelationOption.Response.Error.Code](#anytype-Rpc-Object-CreateRelationOption-Response-Error-Code)
    - [Rpc.Object.CreateSet.Response.Error.Code](#anytype-Rpc-Object-CreateSet-Response-Error-Code)
    - [Rpc.Object.Duplicate.Response.Error.Code](#anytype-Rpc-Object-Duplicate-Response-Error-Code)
    - [Rpc.Object.Graph.Edge.Type](#anytype-Rpc-Object-Graph-Edge-Type)
    - [Rpc.Object.Graph.Response.Error.Code](#anytype-Rpc-Object-Graph-Response-Error-Code)
    - [Rpc.Object.GroupsSubscribe.Response.Error.Code](#anytype-Rpc-Object-GroupsSubscribe-Response-Error-Code)
    - [Rpc.Object.Import.Notion.ValidateToken.Response.Error.Code](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error-Code)
    - [Rpc.Object.Import.Request.CsvParams.Mode](#anytype-Rpc-Object-Import-Request-CsvParams-Mode)
    - [Rpc.Object.Import.Request.Mode](#anytype-Rpc-Object-Import-Request-Mode)
    - [Rpc.Object.Import.Request.Type](#anytype-Rpc-Object-Import-Request-Type)
    - [Rpc.Object.Import.Response.Error.Code](#anytype-Rpc-Object-Import-Response-Error-Code)
    - [Rpc.Object.ImportList.ImportResponse.Type](#anytype-Rpc-Object-ImportList-ImportResponse-Type)
    - [Rpc.Object.ImportList.Response.Error.Code](#anytype-Rpc-Object-ImportList-Response-Error-Code)
    - [Rpc.Object.ListDelete.Response.Error.Code](#anytype-Rpc-Object-ListDelete-Response-Error-Code)
    - [Rpc.Object.ListDuplicate.Response.Error.Code](#anytype-Rpc-Object-ListDuplicate-Response-Error-Code)
    - [Rpc.Object.ListExport.Format](#anytype-Rpc-Object-ListExport-Format)
    - [Rpc.Object.ListExport.Response.Error.Code](#anytype-Rpc-Object-ListExport-Response-Error-Code)
    - [Rpc.Object.ListSetIsArchived.Response.Error.Code](#anytype-Rpc-Object-ListSetIsArchived-Response-Error-Code)
    - [Rpc.Object.ListSetIsFavorite.Response.Error.Code](#anytype-Rpc-Object-ListSetIsFavorite-Response-Error-Code)
    - [Rpc.Object.Open.Response.Error.Code](#anytype-Rpc-Object-Open-Response-Error-Code)
    - [Rpc.Object.OpenBreadcrumbs.Response.Error.Code](#anytype-Rpc-Object-OpenBreadcrumbs-Response-Error-Code)
    - [Rpc.Object.Redo.Response.Error.Code](#anytype-Rpc-Object-Redo-Response-Error-Code)
    - [Rpc.Object.Search.Response.Error.Code](#anytype-Rpc-Object-Search-Response-Error-Code)
    - [Rpc.Object.SearchSubscribe.Response.Error.Code](#anytype-Rpc-Object-SearchSubscribe-Response-Error-Code)
    - [Rpc.Object.SearchUnsubscribe.Response.Error.Code](#anytype-Rpc-Object-SearchUnsubscribe-Response-Error-Code)
    - [Rpc.Object.SetBreadcrumbs.Response.Error.Code](#anytype-Rpc-Object-SetBreadcrumbs-Response-Error-Code)
    - [Rpc.Object.SetDetails.Response.Error.Code](#anytype-Rpc-Object-SetDetails-Response-Error-Code)
    - [Rpc.Object.SetInternalFlags.Response.Error.Code](#anytype-Rpc-Object-SetInternalFlags-Response-Error-Code)
    - [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype-Rpc-Object-SetIsArchived-Response-Error-Code)
    - [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype-Rpc-Object-SetIsFavorite-Response-Error-Code)
    - [Rpc.Object.SetLayout.Response.Error.Code](#anytype-Rpc-Object-SetLayout-Response-Error-Code)
    - [Rpc.Object.SetObjectType.Response.Error.Code](#anytype-Rpc-Object-SetObjectType-Response-Error-Code)
    - [Rpc.Object.SetSource.Response.Error.Code](#anytype-Rpc-Object-SetSource-Response-Error-Code)
    - [Rpc.Object.ShareByLink.Response.Error.Code](#anytype-Rpc-Object-ShareByLink-Response-Error-Code)
    - [Rpc.Object.Show.Response.Error.Code](#anytype-Rpc-Object-Show-Response-Error-Code)
    - [Rpc.Object.SubscribeIds.Response.Error.Code](#anytype-Rpc-Object-SubscribeIds-Response-Error-Code)
    - [Rpc.Object.ToBookmark.Response.Error.Code](#anytype-Rpc-Object-ToBookmark-Response-Error-Code)
    - [Rpc.Object.ToCollection.Response.Error.Code](#anytype-Rpc-Object-ToCollection-Response-Error-Code)
    - [Rpc.Object.ToSet.Response.Error.Code](#anytype-Rpc-Object-ToSet-Response-Error-Code)
    - [Rpc.Object.Undo.Response.Error.Code](#anytype-Rpc-Object-Undo-Response-Error-Code)
    - [Rpc.Object.WorkspaceSetDashboard.Response.Error.Code](#anytype-Rpc-Object-WorkspaceSetDashboard-Response-Error-Code)
    - [Rpc.ObjectCollection.Add.Response.Error.Code](#anytype-Rpc-ObjectCollection-Add-Response-Error-Code)
    - [Rpc.ObjectCollection.Remove.Response.Error.Code](#anytype-Rpc-ObjectCollection-Remove-Response-Error-Code)
    - [Rpc.ObjectCollection.Sort.Response.Error.Code](#anytype-Rpc-ObjectCollection-Sort-Response-Error-Code)
    - [Rpc.ObjectRelation.Add.Response.Error.Code](#anytype-Rpc-ObjectRelation-Add-Response-Error-Code)
    - [Rpc.ObjectRelation.AddFeatured.Response.Error.Code](#anytype-Rpc-ObjectRelation-AddFeatured-Response-Error-Code)
    - [Rpc.ObjectRelation.Delete.Response.Error.Code](#anytype-Rpc-ObjectRelation-Delete-Response-Error-Code)
    - [Rpc.ObjectRelation.ListAvailable.Response.Error.Code](#anytype-Rpc-ObjectRelation-ListAvailable-Response-Error-Code)
    - [Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response-Error-Code)
    - [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-Add-Response-Error-Code)
    - [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-List-Response-Error-Code)
    - [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-Remove-Response-Error-Code)
    - [Rpc.Process.Cancel.Response.Error.Code](#anytype-Rpc-Process-Cancel-Response-Error-Code)
    - [Rpc.Relation.ListRemoveOption.Response.Error.Code](#anytype-Rpc-Relation-ListRemoveOption-Response-Error-Code)
    - [Rpc.Relation.Options.Response.Error.Code](#anytype-Rpc-Relation-Options-Response-Error-Code)
    - [Rpc.Template.Clone.Response.Error.Code](#anytype-Rpc-Template-Clone-Response-Error-Code)
    - [Rpc.Template.CreateFromObject.Response.Error.Code](#anytype-Rpc-Template-CreateFromObject-Response-Error-Code)
    - [Rpc.Template.CreateFromObjectType.Response.Error.Code](#anytype-Rpc-Template-CreateFromObjectType-Response-Error-Code)
    - [Rpc.Template.ExportAll.Response.Error.Code](#anytype-Rpc-Template-ExportAll-Response-Error-Code)
    - [Rpc.Unsplash.Download.Response.Error.Code](#anytype-Rpc-Unsplash-Download-Response-Error-Code)
    - [Rpc.Unsplash.Search.Response.Error.Code](#anytype-Rpc-Unsplash-Search-Response-Error-Code)
    - [Rpc.UserData.Dump.Response.Error.Code](#anytype-Rpc-UserData-Dump-Response-Error-Code)
    - [Rpc.Wallet.CloseSession.Response.Error.Code](#anytype-Rpc-Wallet-CloseSession-Response-Error-Code)
    - [Rpc.Wallet.Convert.Response.Error.Code](#anytype-Rpc-Wallet-Convert-Response-Error-Code)
    - [Rpc.Wallet.Create.Response.Error.Code](#anytype-Rpc-Wallet-Create-Response-Error-Code)
    - [Rpc.Wallet.CreateSession.Response.Error.Code](#anytype-Rpc-Wallet-CreateSession-Response-Error-Code)
    - [Rpc.Wallet.Recover.Response.Error.Code](#anytype-Rpc-Wallet-Recover-Response-Error-Code)
    - [Rpc.Workspace.Create.Response.Error.Code](#anytype-Rpc-Workspace-Create-Response-Error-Code)
    - [Rpc.Workspace.Export.Response.Error.Code](#anytype-Rpc-Workspace-Export-Response-Error-Code)
    - [Rpc.Workspace.GetAll.Response.Error.Code](#anytype-Rpc-Workspace-GetAll-Response-Error-Code)
    - [Rpc.Workspace.GetCurrent.Response.Error.Code](#anytype-Rpc-Workspace-GetCurrent-Response-Error-Code)
    - [Rpc.Workspace.Object.Add.Response.Error.Code](#anytype-Rpc-Workspace-Object-Add-Response-Error-Code)
    - [Rpc.Workspace.Object.ListAdd.Response.Error.Code](#anytype-Rpc-Workspace-Object-ListAdd-Response-Error-Code)
    - [Rpc.Workspace.Object.ListRemove.Response.Error.Code](#anytype-Rpc-Workspace-Object-ListRemove-Response-Error-Code)
    - [Rpc.Workspace.Select.Response.Error.Code](#anytype-Rpc-Workspace-Select-Response-Error-Code)
    - [Rpc.Workspace.SetIsHighlighted.Response.Error.Code](#anytype-Rpc-Workspace-SetIsHighlighted-Response-Error-Code)
  
    - [File-level Extensions](#pb_protos_commands-proto-extensions)
  
- [pb/protos/events.proto](#pb_protos_events-proto)
    - [Event](#anytype-Event)
    - [Event.Account](#anytype-Event-Account)
    - [Event.Account.Config](#anytype-Event-Account-Config)
    - [Event.Account.Config.Update](#anytype-Event-Account-Config-Update)
    - [Event.Account.Details](#anytype-Event-Account-Details)
    - [Event.Account.Show](#anytype-Event-Account-Show)
    - [Event.Account.Update](#anytype-Event-Account-Update)
    - [Event.Block](#anytype-Event-Block)
    - [Event.Block.Add](#anytype-Event-Block-Add)
    - [Event.Block.Dataview](#anytype-Event-Block-Dataview)
    - [Event.Block.Dataview.GroupOrderUpdate](#anytype-Event-Block-Dataview-GroupOrderUpdate)
    - [Event.Block.Dataview.IsCollectionSet](#anytype-Event-Block-Dataview-IsCollectionSet)
    - [Event.Block.Dataview.ObjectOrderUpdate](#anytype-Event-Block-Dataview-ObjectOrderUpdate)
    - [Event.Block.Dataview.OldRelationDelete](#anytype-Event-Block-Dataview-OldRelationDelete)
    - [Event.Block.Dataview.OldRelationSet](#anytype-Event-Block-Dataview-OldRelationSet)
    - [Event.Block.Dataview.RelationDelete](#anytype-Event-Block-Dataview-RelationDelete)
    - [Event.Block.Dataview.RelationSet](#anytype-Event-Block-Dataview-RelationSet)
    - [Event.Block.Dataview.SliceChange](#anytype-Event-Block-Dataview-SliceChange)
    - [Event.Block.Dataview.SourceSet](#anytype-Event-Block-Dataview-SourceSet)
    - [Event.Block.Dataview.TargetObjectIdSet](#anytype-Event-Block-Dataview-TargetObjectIdSet)
    - [Event.Block.Dataview.ViewDelete](#anytype-Event-Block-Dataview-ViewDelete)
    - [Event.Block.Dataview.ViewOrder](#anytype-Event-Block-Dataview-ViewOrder)
    - [Event.Block.Dataview.ViewSet](#anytype-Event-Block-Dataview-ViewSet)
    - [Event.Block.Dataview.ViewUpdate](#anytype-Event-Block-Dataview-ViewUpdate)
    - [Event.Block.Dataview.ViewUpdate.Fields](#anytype-Event-Block-Dataview-ViewUpdate-Fields)
    - [Event.Block.Dataview.ViewUpdate.Filter](#anytype-Event-Block-Dataview-ViewUpdate-Filter)
    - [Event.Block.Dataview.ViewUpdate.Filter.Add](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Add)
    - [Event.Block.Dataview.ViewUpdate.Filter.Move](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Move)
    - [Event.Block.Dataview.ViewUpdate.Filter.Remove](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Remove)
    - [Event.Block.Dataview.ViewUpdate.Filter.Update](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Update)
    - [Event.Block.Dataview.ViewUpdate.Relation](#anytype-Event-Block-Dataview-ViewUpdate-Relation)
    - [Event.Block.Dataview.ViewUpdate.Relation.Add](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Add)
    - [Event.Block.Dataview.ViewUpdate.Relation.Move](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Move)
    - [Event.Block.Dataview.ViewUpdate.Relation.Remove](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Remove)
    - [Event.Block.Dataview.ViewUpdate.Relation.Update](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Update)
    - [Event.Block.Dataview.ViewUpdate.Sort](#anytype-Event-Block-Dataview-ViewUpdate-Sort)
    - [Event.Block.Dataview.ViewUpdate.Sort.Add](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Add)
    - [Event.Block.Dataview.ViewUpdate.Sort.Move](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Move)
    - [Event.Block.Dataview.ViewUpdate.Sort.Remove](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Remove)
    - [Event.Block.Dataview.ViewUpdate.Sort.Update](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Update)
    - [Event.Block.Delete](#anytype-Event-Block-Delete)
    - [Event.Block.FilesUpload](#anytype-Event-Block-FilesUpload)
    - [Event.Block.Fill](#anytype-Event-Block-Fill)
    - [Event.Block.Fill.Align](#anytype-Event-Block-Fill-Align)
    - [Event.Block.Fill.BackgroundColor](#anytype-Event-Block-Fill-BackgroundColor)
    - [Event.Block.Fill.Bookmark](#anytype-Event-Block-Fill-Bookmark)
    - [Event.Block.Fill.Bookmark.Description](#anytype-Event-Block-Fill-Bookmark-Description)
    - [Event.Block.Fill.Bookmark.FaviconHash](#anytype-Event-Block-Fill-Bookmark-FaviconHash)
    - [Event.Block.Fill.Bookmark.ImageHash](#anytype-Event-Block-Fill-Bookmark-ImageHash)
    - [Event.Block.Fill.Bookmark.TargetObjectId](#anytype-Event-Block-Fill-Bookmark-TargetObjectId)
    - [Event.Block.Fill.Bookmark.Title](#anytype-Event-Block-Fill-Bookmark-Title)
    - [Event.Block.Fill.Bookmark.Type](#anytype-Event-Block-Fill-Bookmark-Type)
    - [Event.Block.Fill.Bookmark.Url](#anytype-Event-Block-Fill-Bookmark-Url)
    - [Event.Block.Fill.ChildrenIds](#anytype-Event-Block-Fill-ChildrenIds)
    - [Event.Block.Fill.DatabaseRecords](#anytype-Event-Block-Fill-DatabaseRecords)
    - [Event.Block.Fill.Details](#anytype-Event-Block-Fill-Details)
    - [Event.Block.Fill.Div](#anytype-Event-Block-Fill-Div)
    - [Event.Block.Fill.Div.Style](#anytype-Event-Block-Fill-Div-Style)
    - [Event.Block.Fill.Fields](#anytype-Event-Block-Fill-Fields)
    - [Event.Block.Fill.File](#anytype-Event-Block-Fill-File)
    - [Event.Block.Fill.File.Hash](#anytype-Event-Block-Fill-File-Hash)
    - [Event.Block.Fill.File.Mime](#anytype-Event-Block-Fill-File-Mime)
    - [Event.Block.Fill.File.Name](#anytype-Event-Block-Fill-File-Name)
    - [Event.Block.Fill.File.Size](#anytype-Event-Block-Fill-File-Size)
    - [Event.Block.Fill.File.State](#anytype-Event-Block-Fill-File-State)
    - [Event.Block.Fill.File.Style](#anytype-Event-Block-Fill-File-Style)
    - [Event.Block.Fill.File.Type](#anytype-Event-Block-Fill-File-Type)
    - [Event.Block.Fill.File.Width](#anytype-Event-Block-Fill-File-Width)
    - [Event.Block.Fill.Link](#anytype-Event-Block-Fill-Link)
    - [Event.Block.Fill.Link.Fields](#anytype-Event-Block-Fill-Link-Fields)
    - [Event.Block.Fill.Link.Style](#anytype-Event-Block-Fill-Link-Style)
    - [Event.Block.Fill.Link.TargetBlockId](#anytype-Event-Block-Fill-Link-TargetBlockId)
    - [Event.Block.Fill.Restrictions](#anytype-Event-Block-Fill-Restrictions)
    - [Event.Block.Fill.Text](#anytype-Event-Block-Fill-Text)
    - [Event.Block.Fill.Text.Checked](#anytype-Event-Block-Fill-Text-Checked)
    - [Event.Block.Fill.Text.Color](#anytype-Event-Block-Fill-Text-Color)
    - [Event.Block.Fill.Text.Marks](#anytype-Event-Block-Fill-Text-Marks)
    - [Event.Block.Fill.Text.Style](#anytype-Event-Block-Fill-Text-Style)
    - [Event.Block.Fill.Text.Text](#anytype-Event-Block-Fill-Text-Text)
    - [Event.Block.MarksInfo](#anytype-Event-Block-MarksInfo)
    - [Event.Block.Set](#anytype-Event-Block-Set)
    - [Event.Block.Set.Align](#anytype-Event-Block-Set-Align)
    - [Event.Block.Set.BackgroundColor](#anytype-Event-Block-Set-BackgroundColor)
    - [Event.Block.Set.Bookmark](#anytype-Event-Block-Set-Bookmark)
    - [Event.Block.Set.Bookmark.Description](#anytype-Event-Block-Set-Bookmark-Description)
    - [Event.Block.Set.Bookmark.FaviconHash](#anytype-Event-Block-Set-Bookmark-FaviconHash)
    - [Event.Block.Set.Bookmark.ImageHash](#anytype-Event-Block-Set-Bookmark-ImageHash)
    - [Event.Block.Set.Bookmark.State](#anytype-Event-Block-Set-Bookmark-State)
    - [Event.Block.Set.Bookmark.TargetObjectId](#anytype-Event-Block-Set-Bookmark-TargetObjectId)
    - [Event.Block.Set.Bookmark.Title](#anytype-Event-Block-Set-Bookmark-Title)
    - [Event.Block.Set.Bookmark.Type](#anytype-Event-Block-Set-Bookmark-Type)
    - [Event.Block.Set.Bookmark.Url](#anytype-Event-Block-Set-Bookmark-Url)
    - [Event.Block.Set.ChildrenIds](#anytype-Event-Block-Set-ChildrenIds)
    - [Event.Block.Set.Div](#anytype-Event-Block-Set-Div)
    - [Event.Block.Set.Div.Style](#anytype-Event-Block-Set-Div-Style)
    - [Event.Block.Set.Fields](#anytype-Event-Block-Set-Fields)
    - [Event.Block.Set.File](#anytype-Event-Block-Set-File)
    - [Event.Block.Set.File.Hash](#anytype-Event-Block-Set-File-Hash)
    - [Event.Block.Set.File.Mime](#anytype-Event-Block-Set-File-Mime)
    - [Event.Block.Set.File.Name](#anytype-Event-Block-Set-File-Name)
    - [Event.Block.Set.File.Size](#anytype-Event-Block-Set-File-Size)
    - [Event.Block.Set.File.State](#anytype-Event-Block-Set-File-State)
    - [Event.Block.Set.File.Style](#anytype-Event-Block-Set-File-Style)
    - [Event.Block.Set.File.Type](#anytype-Event-Block-Set-File-Type)
    - [Event.Block.Set.File.Width](#anytype-Event-Block-Set-File-Width)
    - [Event.Block.Set.Latex](#anytype-Event-Block-Set-Latex)
    - [Event.Block.Set.Latex.Text](#anytype-Event-Block-Set-Latex-Text)
    - [Event.Block.Set.Link](#anytype-Event-Block-Set-Link)
    - [Event.Block.Set.Link.CardStyle](#anytype-Event-Block-Set-Link-CardStyle)
    - [Event.Block.Set.Link.Description](#anytype-Event-Block-Set-Link-Description)
    - [Event.Block.Set.Link.Fields](#anytype-Event-Block-Set-Link-Fields)
    - [Event.Block.Set.Link.IconSize](#anytype-Event-Block-Set-Link-IconSize)
    - [Event.Block.Set.Link.Relations](#anytype-Event-Block-Set-Link-Relations)
    - [Event.Block.Set.Link.Style](#anytype-Event-Block-Set-Link-Style)
    - [Event.Block.Set.Link.TargetBlockId](#anytype-Event-Block-Set-Link-TargetBlockId)
    - [Event.Block.Set.Relation](#anytype-Event-Block-Set-Relation)
    - [Event.Block.Set.Relation.Key](#anytype-Event-Block-Set-Relation-Key)
    - [Event.Block.Set.Restrictions](#anytype-Event-Block-Set-Restrictions)
    - [Event.Block.Set.TableRow](#anytype-Event-Block-Set-TableRow)
    - [Event.Block.Set.TableRow.IsHeader](#anytype-Event-Block-Set-TableRow-IsHeader)
    - [Event.Block.Set.Text](#anytype-Event-Block-Set-Text)
    - [Event.Block.Set.Text.Checked](#anytype-Event-Block-Set-Text-Checked)
    - [Event.Block.Set.Text.Color](#anytype-Event-Block-Set-Text-Color)
    - [Event.Block.Set.Text.IconEmoji](#anytype-Event-Block-Set-Text-IconEmoji)
    - [Event.Block.Set.Text.IconImage](#anytype-Event-Block-Set-Text-IconImage)
    - [Event.Block.Set.Text.Marks](#anytype-Event-Block-Set-Text-Marks)
    - [Event.Block.Set.Text.Style](#anytype-Event-Block-Set-Text-Style)
    - [Event.Block.Set.Text.Text](#anytype-Event-Block-Set-Text-Text)
    - [Event.Block.Set.VerticalAlign](#anytype-Event-Block-Set-VerticalAlign)
    - [Event.Block.Set.Widget](#anytype-Event-Block-Set-Widget)
    - [Event.Block.Set.Widget.Layout](#anytype-Event-Block-Set-Widget-Layout)
    - [Event.Block.Set.Widget.Limit](#anytype-Event-Block-Set-Widget-Limit)
    - [Event.File](#anytype-Event-File)
    - [Event.File.LimitReached](#anytype-Event-File-LimitReached)
    - [Event.File.LocalUsage](#anytype-Event-File-LocalUsage)
    - [Event.File.SpaceUsage](#anytype-Event-File-SpaceUsage)
    - [Event.Message](#anytype-Event-Message)
    - [Event.Object](#anytype-Event-Object)
    - [Event.Object.Details](#anytype-Event-Object-Details)
    - [Event.Object.Details.Amend](#anytype-Event-Object-Details-Amend)
    - [Event.Object.Details.Amend.KeyValue](#anytype-Event-Object-Details-Amend-KeyValue)
    - [Event.Object.Details.Set](#anytype-Event-Object-Details-Set)
    - [Event.Object.Details.Unset](#anytype-Event-Object-Details-Unset)
    - [Event.Object.Relations](#anytype-Event-Object-Relations)
    - [Event.Object.Relations.Amend](#anytype-Event-Object-Relations-Amend)
    - [Event.Object.Relations.Remove](#anytype-Event-Object-Relations-Remove)
    - [Event.Object.Remove](#anytype-Event-Object-Remove)
    - [Event.Object.Restrictions](#anytype-Event-Object-Restrictions)
    - [Event.Object.Restrictions.Set](#anytype-Event-Object-Restrictions-Set)
    - [Event.Object.Subscription](#anytype-Event-Object-Subscription)
    - [Event.Object.Subscription.Add](#anytype-Event-Object-Subscription-Add)
    - [Event.Object.Subscription.Counters](#anytype-Event-Object-Subscription-Counters)
    - [Event.Object.Subscription.Groups](#anytype-Event-Object-Subscription-Groups)
    - [Event.Object.Subscription.Position](#anytype-Event-Object-Subscription-Position)
    - [Event.Object.Subscription.Remove](#anytype-Event-Object-Subscription-Remove)
    - [Event.Ping](#anytype-Event-Ping)
    - [Event.Process](#anytype-Event-Process)
    - [Event.Process.Done](#anytype-Event-Process-Done)
    - [Event.Process.New](#anytype-Event-Process-New)
    - [Event.Process.Update](#anytype-Event-Process-Update)
    - [Event.Status](#anytype-Event-Status)
    - [Event.Status.Thread](#anytype-Event-Status-Thread)
    - [Event.Status.Thread.Account](#anytype-Event-Status-Thread-Account)
    - [Event.Status.Thread.Cafe](#anytype-Event-Status-Thread-Cafe)
    - [Event.Status.Thread.Cafe.PinStatus](#anytype-Event-Status-Thread-Cafe-PinStatus)
    - [Event.Status.Thread.Device](#anytype-Event-Status-Thread-Device)
    - [Event.Status.Thread.Summary](#anytype-Event-Status-Thread-Summary)
    - [Event.User](#anytype-Event-User)
    - [Event.User.Block](#anytype-Event-User-Block)
    - [Event.User.Block.Join](#anytype-Event-User-Block-Join)
    - [Event.User.Block.Left](#anytype-Event-User-Block-Left)
    - [Event.User.Block.SelectRange](#anytype-Event-User-Block-SelectRange)
    - [Event.User.Block.TextRange](#anytype-Event-User-Block-TextRange)
    - [Model](#anytype-Model)
    - [Model.Process](#anytype-Model-Process)
    - [Model.Process.Progress](#anytype-Model-Process-Progress)
    - [ResponseEvent](#anytype-ResponseEvent)
  
    - [Event.Block.Dataview.SliceOperation](#anytype-Event-Block-Dataview-SliceOperation)
    - [Event.Status.Thread.SyncStatus](#anytype-Event-Status-Thread-SyncStatus)
    - [Model.Process.State](#anytype-Model-Process-State)
    - [Model.Process.Type](#anytype-Model-Process-Type)
  
- [pb/protos/snapshot.proto](#pb_protos_snapshot-proto)
    - [Profile](#anytype-Profile)
    - [SnapshotWithType](#anytype-SnapshotWithType)
  
- [pkg/lib/pb/model/protos/localstore.proto](#pkg_lib_pb_model_protos_localstore-proto)
    - [ObjectDetails](#anytype-model-ObjectDetails)
    - [ObjectInfo](#anytype-model-ObjectInfo)
    - [ObjectInfoWithLinks](#anytype-model-ObjectInfoWithLinks)
    - [ObjectInfoWithOutboundLinks](#anytype-model-ObjectInfoWithOutboundLinks)
    - [ObjectInfoWithOutboundLinksIDs](#anytype-model-ObjectInfoWithOutboundLinksIDs)
    - [ObjectLinks](#anytype-model-ObjectLinks)
    - [ObjectLinksInfo](#anytype-model-ObjectLinksInfo)
    - [ObjectStoreChecksums](#anytype-model-ObjectStoreChecksums)
  
- [pkg/lib/pb/model/protos/models.proto](#pkg_lib_pb_model_protos_models-proto)
    - [Account](#anytype-model-Account)
    - [Account.Avatar](#anytype-model-Account-Avatar)
    - [Account.Config](#anytype-model-Account-Config)
    - [Account.Info](#anytype-model-Account-Info)
    - [Account.Status](#anytype-model-Account-Status)
    - [Block](#anytype-model-Block)
    - [Block.Content](#anytype-model-Block-Content)
    - [Block.Content.Bookmark](#anytype-model-Block-Content-Bookmark)
    - [Block.Content.Dataview](#anytype-model-Block-Content-Dataview)
    - [Block.Content.Dataview.Checkbox](#anytype-model-Block-Content-Dataview-Checkbox)
    - [Block.Content.Dataview.Date](#anytype-model-Block-Content-Dataview-Date)
    - [Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter)
    - [Block.Content.Dataview.Group](#anytype-model-Block-Content-Dataview-Group)
    - [Block.Content.Dataview.GroupOrder](#anytype-model-Block-Content-Dataview-GroupOrder)
    - [Block.Content.Dataview.ObjectOrder](#anytype-model-Block-Content-Dataview-ObjectOrder)
    - [Block.Content.Dataview.Relation](#anytype-model-Block-Content-Dataview-Relation)
    - [Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort)
    - [Block.Content.Dataview.Status](#anytype-model-Block-Content-Dataview-Status)
    - [Block.Content.Dataview.Tag](#anytype-model-Block-Content-Dataview-Tag)
    - [Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View)
    - [Block.Content.Dataview.ViewGroup](#anytype-model-Block-Content-Dataview-ViewGroup)
    - [Block.Content.Div](#anytype-model-Block-Content-Div)
    - [Block.Content.FeaturedRelations](#anytype-model-Block-Content-FeaturedRelations)
    - [Block.Content.File](#anytype-model-Block-Content-File)
    - [Block.Content.Icon](#anytype-model-Block-Content-Icon)
    - [Block.Content.Latex](#anytype-model-Block-Content-Latex)
    - [Block.Content.Layout](#anytype-model-Block-Content-Layout)
    - [Block.Content.Link](#anytype-model-Block-Content-Link)
    - [Block.Content.Relation](#anytype-model-Block-Content-Relation)
    - [Block.Content.Smartblock](#anytype-model-Block-Content-Smartblock)
    - [Block.Content.Table](#anytype-model-Block-Content-Table)
    - [Block.Content.TableColumn](#anytype-model-Block-Content-TableColumn)
    - [Block.Content.TableOfContents](#anytype-model-Block-Content-TableOfContents)
    - [Block.Content.TableRow](#anytype-model-Block-Content-TableRow)
    - [Block.Content.Text](#anytype-model-Block-Content-Text)
    - [Block.Content.Text.Mark](#anytype-model-Block-Content-Text-Mark)
    - [Block.Content.Text.Marks](#anytype-model-Block-Content-Text-Marks)
    - [Block.Content.Widget](#anytype-model-Block-Content-Widget)
    - [Block.Restrictions](#anytype-model-Block-Restrictions)
    - [BlockMetaOnly](#anytype-model-BlockMetaOnly)
    - [InternalFlag](#anytype-model-InternalFlag)
    - [Layout](#anytype-model-Layout)
    - [LinkPreview](#anytype-model-LinkPreview)
    - [Object](#anytype-model-Object)
    - [Object.ChangePayload](#anytype-model-Object-ChangePayload)
    - [ObjectType](#anytype-model-ObjectType)
    - [ObjectView](#anytype-model-ObjectView)
    - [ObjectView.DetailsSet](#anytype-model-ObjectView-DetailsSet)
    - [ObjectView.HistorySize](#anytype-model-ObjectView-HistorySize)
    - [ObjectView.RelationWithValuePerObject](#anytype-model-ObjectView-RelationWithValuePerObject)
    - [Range](#anytype-model-Range)
    - [Relation](#anytype-model-Relation)
    - [Relation.Option](#anytype-model-Relation-Option)
    - [RelationLink](#anytype-model-RelationLink)
    - [RelationOptions](#anytype-model-RelationOptions)
    - [RelationWithValue](#anytype-model-RelationWithValue)
    - [Relations](#anytype-model-Relations)
    - [Restrictions](#anytype-model-Restrictions)
    - [Restrictions.DataviewRestrictions](#anytype-model-Restrictions-DataviewRestrictions)
    - [SmartBlockSnapshotBase](#anytype-model-SmartBlockSnapshotBase)
    - [ThreadCreateQueueEntry](#anytype-model-ThreadCreateQueueEntry)
    - [ThreadDeeplinkPayload](#anytype-model-ThreadDeeplinkPayload)
  
    - [Account.StatusType](#anytype-model-Account-StatusType)
    - [Block.Align](#anytype-model-Block-Align)
    - [Block.Content.Bookmark.State](#anytype-model-Block-Content-Bookmark-State)
    - [Block.Content.Dataview.Filter.Condition](#anytype-model-Block-Content-Dataview-Filter-Condition)
    - [Block.Content.Dataview.Filter.Operator](#anytype-model-Block-Content-Dataview-Filter-Operator)
    - [Block.Content.Dataview.Filter.QuickOption](#anytype-model-Block-Content-Dataview-Filter-QuickOption)
    - [Block.Content.Dataview.Relation.DateFormat](#anytype-model-Block-Content-Dataview-Relation-DateFormat)
    - [Block.Content.Dataview.Relation.TimeFormat](#anytype-model-Block-Content-Dataview-Relation-TimeFormat)
    - [Block.Content.Dataview.Sort.Type](#anytype-model-Block-Content-Dataview-Sort-Type)
    - [Block.Content.Dataview.View.Size](#anytype-model-Block-Content-Dataview-View-Size)
    - [Block.Content.Dataview.View.Type](#anytype-model-Block-Content-Dataview-View-Type)
    - [Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style)
    - [Block.Content.File.State](#anytype-model-Block-Content-File-State)
    - [Block.Content.File.Style](#anytype-model-Block-Content-File-Style)
    - [Block.Content.File.Type](#anytype-model-Block-Content-File-Type)
    - [Block.Content.Layout.Style](#anytype-model-Block-Content-Layout-Style)
    - [Block.Content.Link.CardStyle](#anytype-model-Block-Content-Link-CardStyle)
    - [Block.Content.Link.Description](#anytype-model-Block-Content-Link-Description)
    - [Block.Content.Link.IconSize](#anytype-model-Block-Content-Link-IconSize)
    - [Block.Content.Link.Style](#anytype-model-Block-Content-Link-Style)
    - [Block.Content.Text.Mark.Type](#anytype-model-Block-Content-Text-Mark-Type)
    - [Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style)
    - [Block.Content.Widget.Layout](#anytype-model-Block-Content-Widget-Layout)
    - [Block.Position](#anytype-model-Block-Position)
    - [Block.VerticalAlign](#anytype-model-Block-VerticalAlign)
    - [InternalFlag.Value](#anytype-model-InternalFlag-Value)
    - [LinkPreview.Type](#anytype-model-LinkPreview-Type)
    - [ObjectType.Layout](#anytype-model-ObjectType-Layout)
    - [Relation.DataSource](#anytype-model-Relation-DataSource)
    - [Relation.Scope](#anytype-model-Relation-Scope)
    - [RelationFormat](#anytype-model-RelationFormat)
    - [Restrictions.DataviewRestriction](#anytype-model-Restrictions-DataviewRestriction)
    - [Restrictions.ObjectRestriction](#anytype-model-Restrictions-ObjectRestriction)
    - [SmartBlockType](#anytype-model-SmartBlockType)
  
- [Scalar Value Types](#scalar-value-types)



<a name="pb_protos_service_service-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/service/service.proto


 

 

 


<a name="anytype-ClientCommands"></a>

### ClientCommands


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| AppGetVersion | [Rpc.App.GetVersion.Request](#anytype-Rpc-App-GetVersion-Request) | [Rpc.App.GetVersion.Response](#anytype-Rpc-App-GetVersion-Response) |  |
| AppSetDeviceState | [Rpc.App.SetDeviceState.Request](#anytype-Rpc-App-SetDeviceState-Request) | [Rpc.App.SetDeviceState.Response](#anytype-Rpc-App-SetDeviceState-Response) |  |
| AppShutdown | [Rpc.App.Shutdown.Request](#anytype-Rpc-App-Shutdown-Request) | [Rpc.App.Shutdown.Response](#anytype-Rpc-App-Shutdown-Response) |  |
| WalletCreate | [Rpc.Wallet.Create.Request](#anytype-Rpc-Wallet-Create-Request) | [Rpc.Wallet.Create.Response](#anytype-Rpc-Wallet-Create-Response) | Wallet *** |
| WalletRecover | [Rpc.Wallet.Recover.Request](#anytype-Rpc-Wallet-Recover-Request) | [Rpc.Wallet.Recover.Response](#anytype-Rpc-Wallet-Recover-Response) |  |
| WalletConvert | [Rpc.Wallet.Convert.Request](#anytype-Rpc-Wallet-Convert-Request) | [Rpc.Wallet.Convert.Response](#anytype-Rpc-Wallet-Convert-Response) |  |
| WalletCreateSession | [Rpc.Wallet.CreateSession.Request](#anytype-Rpc-Wallet-CreateSession-Request) | [Rpc.Wallet.CreateSession.Response](#anytype-Rpc-Wallet-CreateSession-Response) |  |
| WalletCloseSession | [Rpc.Wallet.CloseSession.Request](#anytype-Rpc-Wallet-CloseSession-Request) | [Rpc.Wallet.CloseSession.Response](#anytype-Rpc-Wallet-CloseSession-Response) |  |
| WorkspaceCreate | [Rpc.Workspace.Create.Request](#anytype-Rpc-Workspace-Create-Request) | [Rpc.Workspace.Create.Response](#anytype-Rpc-Workspace-Create-Response) | Workspace *** |
| WorkspaceObjectAdd | [Rpc.Workspace.Object.Add.Request](#anytype-Rpc-Workspace-Object-Add-Request) | [Rpc.Workspace.Object.Add.Response](#anytype-Rpc-Workspace-Object-Add-Response) |  |
| WorkspaceObjectListAdd | [Rpc.Workspace.Object.ListAdd.Request](#anytype-Rpc-Workspace-Object-ListAdd-Request) | [Rpc.Workspace.Object.ListAdd.Response](#anytype-Rpc-Workspace-Object-ListAdd-Response) |  |
| WorkspaceObjectListRemove | [Rpc.Workspace.Object.ListRemove.Request](#anytype-Rpc-Workspace-Object-ListRemove-Request) | [Rpc.Workspace.Object.ListRemove.Response](#anytype-Rpc-Workspace-Object-ListRemove-Response) |  |
| WorkspaceSelect | [Rpc.Workspace.Select.Request](#anytype-Rpc-Workspace-Select-Request) | [Rpc.Workspace.Select.Response](#anytype-Rpc-Workspace-Select-Response) |  |
| WorkspaceGetCurrent | [Rpc.Workspace.GetCurrent.Request](#anytype-Rpc-Workspace-GetCurrent-Request) | [Rpc.Workspace.GetCurrent.Response](#anytype-Rpc-Workspace-GetCurrent-Response) |  |
| WorkspaceGetAll | [Rpc.Workspace.GetAll.Request](#anytype-Rpc-Workspace-GetAll-Request) | [Rpc.Workspace.GetAll.Response](#anytype-Rpc-Workspace-GetAll-Response) |  |
| WorkspaceSetIsHighlighted | [Rpc.Workspace.SetIsHighlighted.Request](#anytype-Rpc-Workspace-SetIsHighlighted-Request) | [Rpc.Workspace.SetIsHighlighted.Response](#anytype-Rpc-Workspace-SetIsHighlighted-Response) |  |
| WorkspaceExport | [Rpc.Workspace.Export.Request](#anytype-Rpc-Workspace-Export-Request) | [Rpc.Workspace.Export.Response](#anytype-Rpc-Workspace-Export-Response) |  |
| AccountRecover | [Rpc.Account.Recover.Request](#anytype-Rpc-Account-Recover-Request) | [Rpc.Account.Recover.Response](#anytype-Rpc-Account-Recover-Response) | Account *** |
| AccountCreate | [Rpc.Account.Create.Request](#anytype-Rpc-Account-Create-Request) | [Rpc.Account.Create.Response](#anytype-Rpc-Account-Create-Response) |  |
| AccountDelete | [Rpc.Account.Delete.Request](#anytype-Rpc-Account-Delete-Request) | [Rpc.Account.Delete.Response](#anytype-Rpc-Account-Delete-Response) |  |
| AccountSelect | [Rpc.Account.Select.Request](#anytype-Rpc-Account-Select-Request) | [Rpc.Account.Select.Response](#anytype-Rpc-Account-Select-Response) |  |
| AccountStop | [Rpc.Account.Stop.Request](#anytype-Rpc-Account-Stop-Request) | [Rpc.Account.Stop.Response](#anytype-Rpc-Account-Stop-Response) |  |
| AccountMove | [Rpc.Account.Move.Request](#anytype-Rpc-Account-Move-Request) | [Rpc.Account.Move.Response](#anytype-Rpc-Account-Move-Response) |  |
| AccountConfigUpdate | [Rpc.Account.ConfigUpdate.Request](#anytype-Rpc-Account-ConfigUpdate-Request) | [Rpc.Account.ConfigUpdate.Response](#anytype-Rpc-Account-ConfigUpdate-Response) |  |
| AccountRecoverFromLegacyExport | [Rpc.Account.RecoverFromLegacyExport.Request](#anytype-Rpc-Account-RecoverFromLegacyExport-Request) | [Rpc.Account.RecoverFromLegacyExport.Response](#anytype-Rpc-Account-RecoverFromLegacyExport-Response) |  |
| ObjectOpen | [Rpc.Object.Open.Request](#anytype-Rpc-Object-Open-Request) | [Rpc.Object.Open.Response](#anytype-Rpc-Object-Open-Response) | Object *** |
| ObjectClose | [Rpc.Object.Close.Request](#anytype-Rpc-Object-Close-Request) | [Rpc.Object.Close.Response](#anytype-Rpc-Object-Close-Response) |  |
| ObjectShow | [Rpc.Object.Show.Request](#anytype-Rpc-Object-Show-Request) | [Rpc.Object.Show.Response](#anytype-Rpc-Object-Show-Response) |  |
| ObjectCreate | [Rpc.Object.Create.Request](#anytype-Rpc-Object-Create-Request) | [Rpc.Object.Create.Response](#anytype-Rpc-Object-Create-Response) | ObjectCreate just creates the new page, without adding the link to it from some other page |
| ObjectCreateBookmark | [Rpc.Object.CreateBookmark.Request](#anytype-Rpc-Object-CreateBookmark-Request) | [Rpc.Object.CreateBookmark.Response](#anytype-Rpc-Object-CreateBookmark-Response) |  |
| ObjectCreateSet | [Rpc.Object.CreateSet.Request](#anytype-Rpc-Object-CreateSet-Request) | [Rpc.Object.CreateSet.Response](#anytype-Rpc-Object-CreateSet-Response) | ObjectCreateSet just creates the new set, without adding the link to it from some other page |
| ObjectGraph | [Rpc.Object.Graph.Request](#anytype-Rpc-Object-Graph-Request) | [Rpc.Object.Graph.Response](#anytype-Rpc-Object-Graph-Response) |  |
| ObjectSearch | [Rpc.Object.Search.Request](#anytype-Rpc-Object-Search-Request) | [Rpc.Object.Search.Response](#anytype-Rpc-Object-Search-Response) |  |
| ObjectSearchSubscribe | [Rpc.Object.SearchSubscribe.Request](#anytype-Rpc-Object-SearchSubscribe-Request) | [Rpc.Object.SearchSubscribe.Response](#anytype-Rpc-Object-SearchSubscribe-Response) |  |
| ObjectSubscribeIds | [Rpc.Object.SubscribeIds.Request](#anytype-Rpc-Object-SubscribeIds-Request) | [Rpc.Object.SubscribeIds.Response](#anytype-Rpc-Object-SubscribeIds-Response) |  |
| ObjectGroupsSubscribe | [Rpc.Object.GroupsSubscribe.Request](#anytype-Rpc-Object-GroupsSubscribe-Request) | [Rpc.Object.GroupsSubscribe.Response](#anytype-Rpc-Object-GroupsSubscribe-Response) |  |
| ObjectSearchUnsubscribe | [Rpc.Object.SearchUnsubscribe.Request](#anytype-Rpc-Object-SearchUnsubscribe-Request) | [Rpc.Object.SearchUnsubscribe.Response](#anytype-Rpc-Object-SearchUnsubscribe-Response) |  |
| ObjectSetDetails | [Rpc.Object.SetDetails.Request](#anytype-Rpc-Object-SetDetails-Request) | [Rpc.Object.SetDetails.Response](#anytype-Rpc-Object-SetDetails-Response) |  |
| ObjectDuplicate | [Rpc.Object.Duplicate.Request](#anytype-Rpc-Object-Duplicate-Request) | [Rpc.Object.Duplicate.Response](#anytype-Rpc-Object-Duplicate-Response) |  |
| ObjectSetObjectType | [Rpc.Object.SetObjectType.Request](#anytype-Rpc-Object-SetObjectType-Request) | [Rpc.Object.SetObjectType.Response](#anytype-Rpc-Object-SetObjectType-Response) | ObjectSetObjectType sets an existing object type to the object so it will appear in sets and suggests relations from this type |
| ObjectSetLayout | [Rpc.Object.SetLayout.Request](#anytype-Rpc-Object-SetLayout-Request) | [Rpc.Object.SetLayout.Response](#anytype-Rpc-Object-SetLayout-Response) |  |
| ObjectSetInternalFlags | [Rpc.Object.SetInternalFlags.Request](#anytype-Rpc-Object-SetInternalFlags-Request) | [Rpc.Object.SetInternalFlags.Response](#anytype-Rpc-Object-SetInternalFlags-Response) |  |
| ObjectSetIsFavorite | [Rpc.Object.SetIsFavorite.Request](#anytype-Rpc-Object-SetIsFavorite-Request) | [Rpc.Object.SetIsFavorite.Response](#anytype-Rpc-Object-SetIsFavorite-Response) |  |
| ObjectSetIsArchived | [Rpc.Object.SetIsArchived.Request](#anytype-Rpc-Object-SetIsArchived-Request) | [Rpc.Object.SetIsArchived.Response](#anytype-Rpc-Object-SetIsArchived-Response) |  |
| ObjectSetSource | [Rpc.Object.SetSource.Request](#anytype-Rpc-Object-SetSource-Request) | [Rpc.Object.SetSource.Response](#anytype-Rpc-Object-SetSource-Response) |  |
| ObjectWorkspaceSetDashboard | [Rpc.Object.WorkspaceSetDashboard.Request](#anytype-Rpc-Object-WorkspaceSetDashboard-Request) | [Rpc.Object.WorkspaceSetDashboard.Response](#anytype-Rpc-Object-WorkspaceSetDashboard-Response) |  |
| ObjectListDuplicate | [Rpc.Object.ListDuplicate.Request](#anytype-Rpc-Object-ListDuplicate-Request) | [Rpc.Object.ListDuplicate.Response](#anytype-Rpc-Object-ListDuplicate-Response) |  |
| ObjectListDelete | [Rpc.Object.ListDelete.Request](#anytype-Rpc-Object-ListDelete-Request) | [Rpc.Object.ListDelete.Response](#anytype-Rpc-Object-ListDelete-Response) |  |
| ObjectListSetIsArchived | [Rpc.Object.ListSetIsArchived.Request](#anytype-Rpc-Object-ListSetIsArchived-Request) | [Rpc.Object.ListSetIsArchived.Response](#anytype-Rpc-Object-ListSetIsArchived-Response) |  |
| ObjectListSetIsFavorite | [Rpc.Object.ListSetIsFavorite.Request](#anytype-Rpc-Object-ListSetIsFavorite-Request) | [Rpc.Object.ListSetIsFavorite.Response](#anytype-Rpc-Object-ListSetIsFavorite-Response) |  |
| ObjectApplyTemplate | [Rpc.Object.ApplyTemplate.Request](#anytype-Rpc-Object-ApplyTemplate-Request) | [Rpc.Object.ApplyTemplate.Response](#anytype-Rpc-Object-ApplyTemplate-Response) |  |
| ObjectToSet | [Rpc.Object.ToSet.Request](#anytype-Rpc-Object-ToSet-Request) | [Rpc.Object.ToSet.Response](#anytype-Rpc-Object-ToSet-Response) | ObjectToSet creates new set from given object and removes object |
| ObjectToCollection | [Rpc.Object.ToCollection.Request](#anytype-Rpc-Object-ToCollection-Request) | [Rpc.Object.ToCollection.Response](#anytype-Rpc-Object-ToCollection-Response) |  |
| ObjectShareByLink | [Rpc.Object.ShareByLink.Request](#anytype-Rpc-Object-ShareByLink-Request) | [Rpc.Object.ShareByLink.Response](#anytype-Rpc-Object-ShareByLink-Response) |  |
| ObjectUndo | [Rpc.Object.Undo.Request](#anytype-Rpc-Object-Undo-Request) | [Rpc.Object.Undo.Response](#anytype-Rpc-Object-Undo-Response) |  |
| ObjectRedo | [Rpc.Object.Redo.Request](#anytype-Rpc-Object-Redo-Request) | [Rpc.Object.Redo.Response](#anytype-Rpc-Object-Redo-Response) |  |
| ObjectListExport | [Rpc.Object.ListExport.Request](#anytype-Rpc-Object-ListExport-Request) | [Rpc.Object.ListExport.Response](#anytype-Rpc-Object-ListExport-Response) |  |
| ObjectBookmarkFetch | [Rpc.Object.BookmarkFetch.Request](#anytype-Rpc-Object-BookmarkFetch-Request) | [Rpc.Object.BookmarkFetch.Response](#anytype-Rpc-Object-BookmarkFetch-Response) |  |
| ObjectToBookmark | [Rpc.Object.ToBookmark.Request](#anytype-Rpc-Object-ToBookmark-Request) | [Rpc.Object.ToBookmark.Response](#anytype-Rpc-Object-ToBookmark-Response) |  |
| ObjectImport | [Rpc.Object.Import.Request](#anytype-Rpc-Object-Import-Request) | [Rpc.Object.Import.Response](#anytype-Rpc-Object-Import-Response) |  |
| ObjectImportList | [Rpc.Object.ImportList.Request](#anytype-Rpc-Object-ImportList-Request) | [Rpc.Object.ImportList.Response](#anytype-Rpc-Object-ImportList-Response) |  |
| ObjectImportNotionValidateToken | [Rpc.Object.Import.Notion.ValidateToken.Request](#anytype-Rpc-Object-Import-Notion-ValidateToken-Request) | [Rpc.Object.Import.Notion.ValidateToken.Response](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response) |  |
| ObjectCollectionAdd | [Rpc.ObjectCollection.Add.Request](#anytype-Rpc-ObjectCollection-Add-Request) | [Rpc.ObjectCollection.Add.Response](#anytype-Rpc-ObjectCollection-Add-Response) | Collections *** |
| ObjectCollectionRemove | [Rpc.ObjectCollection.Remove.Request](#anytype-Rpc-ObjectCollection-Remove-Request) | [Rpc.ObjectCollection.Remove.Response](#anytype-Rpc-ObjectCollection-Remove-Response) |  |
| ObjectCollectionSort | [Rpc.ObjectCollection.Sort.Request](#anytype-Rpc-ObjectCollection-Sort-Request) | [Rpc.ObjectCollection.Sort.Response](#anytype-Rpc-ObjectCollection-Sort-Response) |  |
| ObjectCreateRelation | [Rpc.Object.CreateRelation.Request](#anytype-Rpc-Object-CreateRelation-Request) | [Rpc.Object.CreateRelation.Response](#anytype-Rpc-Object-CreateRelation-Response) | Relations *** |
| ObjectCreateRelationOption | [Rpc.Object.CreateRelationOption.Request](#anytype-Rpc-Object-CreateRelationOption-Request) | [Rpc.Object.CreateRelationOption.Response](#anytype-Rpc-Object-CreateRelationOption-Response) |  |
| RelationListRemoveOption | [Rpc.Relation.ListRemoveOption.Request](#anytype-Rpc-Relation-ListRemoveOption-Request) | [Rpc.Relation.ListRemoveOption.Response](#anytype-Rpc-Relation-ListRemoveOption-Response) |  |
| RelationOptions | [Rpc.Relation.Options.Request](#anytype-Rpc-Relation-Options-Request) | [Rpc.Relation.Options.Response](#anytype-Rpc-Relation-Options-Response) |  |
| ObjectRelationAdd | [Rpc.ObjectRelation.Add.Request](#anytype-Rpc-ObjectRelation-Add-Request) | [Rpc.ObjectRelation.Add.Response](#anytype-Rpc-ObjectRelation-Add-Response) | Object Relations *** |
| ObjectRelationDelete | [Rpc.ObjectRelation.Delete.Request](#anytype-Rpc-ObjectRelation-Delete-Request) | [Rpc.ObjectRelation.Delete.Response](#anytype-Rpc-ObjectRelation-Delete-Response) |  |
| ObjectRelationAddFeatured | [Rpc.ObjectRelation.AddFeatured.Request](#anytype-Rpc-ObjectRelation-AddFeatured-Request) | [Rpc.ObjectRelation.AddFeatured.Response](#anytype-Rpc-ObjectRelation-AddFeatured-Response) |  |
| ObjectRelationRemoveFeatured | [Rpc.ObjectRelation.RemoveFeatured.Request](#anytype-Rpc-ObjectRelation-RemoveFeatured-Request) | [Rpc.ObjectRelation.RemoveFeatured.Response](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response) |  |
| ObjectRelationListAvailable | [Rpc.ObjectRelation.ListAvailable.Request](#anytype-Rpc-ObjectRelation-ListAvailable-Request) | [Rpc.ObjectRelation.ListAvailable.Response](#anytype-Rpc-ObjectRelation-ListAvailable-Response) |  |
| ObjectCreateObjectType | [Rpc.Object.CreateObjectType.Request](#anytype-Rpc-Object-CreateObjectType-Request) | [Rpc.Object.CreateObjectType.Response](#anytype-Rpc-Object-CreateObjectType-Response) | ObjectType commands *** |
| ObjectTypeRelationList | [Rpc.ObjectType.Relation.List.Request](#anytype-Rpc-ObjectType-Relation-List-Request) | [Rpc.ObjectType.Relation.List.Response](#anytype-Rpc-ObjectType-Relation-List-Response) |  |
| ObjectTypeRelationAdd | [Rpc.ObjectType.Relation.Add.Request](#anytype-Rpc-ObjectType-Relation-Add-Request) | [Rpc.ObjectType.Relation.Add.Response](#anytype-Rpc-ObjectType-Relation-Add-Response) |  |
| ObjectTypeRelationRemove | [Rpc.ObjectType.Relation.Remove.Request](#anytype-Rpc-ObjectType-Relation-Remove-Request) | [Rpc.ObjectType.Relation.Remove.Response](#anytype-Rpc-ObjectType-Relation-Remove-Response) |  |
| HistoryShowVersion | [Rpc.History.ShowVersion.Request](#anytype-Rpc-History-ShowVersion-Request) | [Rpc.History.ShowVersion.Response](#anytype-Rpc-History-ShowVersion-Response) |  |
| HistoryGetVersions | [Rpc.History.GetVersions.Request](#anytype-Rpc-History-GetVersions-Request) | [Rpc.History.GetVersions.Response](#anytype-Rpc-History-GetVersions-Response) |  |
| HistorySetVersion | [Rpc.History.SetVersion.Request](#anytype-Rpc-History-SetVersion-Request) | [Rpc.History.SetVersion.Response](#anytype-Rpc-History-SetVersion-Response) |  |
| FileOffload | [Rpc.File.Offload.Request](#anytype-Rpc-File-Offload-Request) | [Rpc.File.Offload.Response](#anytype-Rpc-File-Offload-Response) | Files *** |
| FileListOffload | [Rpc.File.ListOffload.Request](#anytype-Rpc-File-ListOffload-Request) | [Rpc.File.ListOffload.Response](#anytype-Rpc-File-ListOffload-Response) |  |
| FileUpload | [Rpc.File.Upload.Request](#anytype-Rpc-File-Upload-Request) | [Rpc.File.Upload.Response](#anytype-Rpc-File-Upload-Response) |  |
| FileDownload | [Rpc.File.Download.Request](#anytype-Rpc-File-Download-Request) | [Rpc.File.Download.Response](#anytype-Rpc-File-Download-Response) |  |
| FileDrop | [Rpc.File.Drop.Request](#anytype-Rpc-File-Drop-Request) | [Rpc.File.Drop.Response](#anytype-Rpc-File-Drop-Response) |  |
| FileSpaceUsage | [Rpc.File.SpaceUsage.Request](#anytype-Rpc-File-SpaceUsage-Request) | [Rpc.File.SpaceUsage.Response](#anytype-Rpc-File-SpaceUsage-Response) |  |
| NavigationListObjects | [Rpc.Navigation.ListObjects.Request](#anytype-Rpc-Navigation-ListObjects-Request) | [Rpc.Navigation.ListObjects.Response](#anytype-Rpc-Navigation-ListObjects-Response) |  |
| NavigationGetObjectInfoWithLinks | [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Request) | [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response) |  |
| TemplateCreateFromObject | [Rpc.Template.CreateFromObject.Request](#anytype-Rpc-Template-CreateFromObject-Request) | [Rpc.Template.CreateFromObject.Response](#anytype-Rpc-Template-CreateFromObject-Response) |  |
| TemplateCreateFromObjectType | [Rpc.Template.CreateFromObjectType.Request](#anytype-Rpc-Template-CreateFromObjectType-Request) | [Rpc.Template.CreateFromObjectType.Response](#anytype-Rpc-Template-CreateFromObjectType-Response) | to be renamed to ObjectCreateTemplate |
| TemplateClone | [Rpc.Template.Clone.Request](#anytype-Rpc-Template-Clone-Request) | [Rpc.Template.Clone.Response](#anytype-Rpc-Template-Clone-Response) |  |
| TemplateExportAll | [Rpc.Template.ExportAll.Request](#anytype-Rpc-Template-ExportAll-Request) | [Rpc.Template.ExportAll.Response](#anytype-Rpc-Template-ExportAll-Response) |  |
| LinkPreview | [Rpc.LinkPreview.Request](#anytype-Rpc-LinkPreview-Request) | [Rpc.LinkPreview.Response](#anytype-Rpc-LinkPreview-Response) |  |
| UnsplashSearch | [Rpc.Unsplash.Search.Request](#anytype-Rpc-Unsplash-Search-Request) | [Rpc.Unsplash.Search.Response](#anytype-Rpc-Unsplash-Search-Response) |  |
| UnsplashDownload | [Rpc.Unsplash.Download.Request](#anytype-Rpc-Unsplash-Download-Request) | [Rpc.Unsplash.Download.Response](#anytype-Rpc-Unsplash-Download-Response) | UnsplashDownload downloads picture from unsplash by ID, put it to the IPFS and returns the hash. The artist info is available in the object details |
| BlockUpload | [Rpc.Block.Upload.Request](#anytype-Rpc-Block-Upload-Request) | [Rpc.Block.Upload.Response](#anytype-Rpc-Block-Upload-Response) | General Block commands *** |
| BlockReplace | [Rpc.Block.Replace.Request](#anytype-Rpc-Block-Replace-Request) | [Rpc.Block.Replace.Response](#anytype-Rpc-Block-Replace-Response) |  |
| BlockCreate | [Rpc.Block.Create.Request](#anytype-Rpc-Block-Create-Request) | [Rpc.Block.Create.Response](#anytype-Rpc-Block-Create-Response) |  |
| BlockSplit | [Rpc.Block.Split.Request](#anytype-Rpc-Block-Split-Request) | [Rpc.Block.Split.Response](#anytype-Rpc-Block-Split-Response) |  |
| BlockMerge | [Rpc.Block.Merge.Request](#anytype-Rpc-Block-Merge-Request) | [Rpc.Block.Merge.Response](#anytype-Rpc-Block-Merge-Response) |  |
| BlockCopy | [Rpc.Block.Copy.Request](#anytype-Rpc-Block-Copy-Request) | [Rpc.Block.Copy.Response](#anytype-Rpc-Block-Copy-Response) |  |
| BlockPaste | [Rpc.Block.Paste.Request](#anytype-Rpc-Block-Paste-Request) | [Rpc.Block.Paste.Response](#anytype-Rpc-Block-Paste-Response) |  |
| BlockCut | [Rpc.Block.Cut.Request](#anytype-Rpc-Block-Cut-Request) | [Rpc.Block.Cut.Response](#anytype-Rpc-Block-Cut-Response) |  |
| BlockSetFields | [Rpc.Block.SetFields.Request](#anytype-Rpc-Block-SetFields-Request) | [Rpc.Block.SetFields.Response](#anytype-Rpc-Block-SetFields-Response) |  |
| BlockExport | [Rpc.Block.Export.Request](#anytype-Rpc-Block-Export-Request) | [Rpc.Block.Export.Response](#anytype-Rpc-Block-Export-Response) |  |
| BlockListDelete | [Rpc.Block.ListDelete.Request](#anytype-Rpc-Block-ListDelete-Request) | [Rpc.Block.ListDelete.Response](#anytype-Rpc-Block-ListDelete-Response) |  |
| BlockListMoveToExistingObject | [Rpc.Block.ListMoveToExistingObject.Request](#anytype-Rpc-Block-ListMoveToExistingObject-Request) | [Rpc.Block.ListMoveToExistingObject.Response](#anytype-Rpc-Block-ListMoveToExistingObject-Response) |  |
| BlockListMoveToNewObject | [Rpc.Block.ListMoveToNewObject.Request](#anytype-Rpc-Block-ListMoveToNewObject-Request) | [Rpc.Block.ListMoveToNewObject.Response](#anytype-Rpc-Block-ListMoveToNewObject-Response) |  |
| BlockListConvertToObjects | [Rpc.Block.ListConvertToObjects.Request](#anytype-Rpc-Block-ListConvertToObjects-Request) | [Rpc.Block.ListConvertToObjects.Response](#anytype-Rpc-Block-ListConvertToObjects-Response) |  |
| BlockListSetFields | [Rpc.Block.ListSetFields.Request](#anytype-Rpc-Block-ListSetFields-Request) | [Rpc.Block.ListSetFields.Response](#anytype-Rpc-Block-ListSetFields-Response) |  |
| BlockListDuplicate | [Rpc.Block.ListDuplicate.Request](#anytype-Rpc-Block-ListDuplicate-Request) | [Rpc.Block.ListDuplicate.Response](#anytype-Rpc-Block-ListDuplicate-Response) |  |
| BlockListSetBackgroundColor | [Rpc.Block.ListSetBackgroundColor.Request](#anytype-Rpc-Block-ListSetBackgroundColor-Request) | [Rpc.Block.ListSetBackgroundColor.Response](#anytype-Rpc-Block-ListSetBackgroundColor-Response) |  |
| BlockListSetAlign | [Rpc.Block.ListSetAlign.Request](#anytype-Rpc-Block-ListSetAlign-Request) | [Rpc.Block.ListSetAlign.Response](#anytype-Rpc-Block-ListSetAlign-Response) |  |
| BlockListSetVerticalAlign | [Rpc.Block.ListSetVerticalAlign.Request](#anytype-Rpc-Block-ListSetVerticalAlign-Request) | [Rpc.Block.ListSetVerticalAlign.Response](#anytype-Rpc-Block-ListSetVerticalAlign-Response) |  |
| BlockListTurnInto | [Rpc.Block.ListTurnInto.Request](#anytype-Rpc-Block-ListTurnInto-Request) | [Rpc.Block.ListTurnInto.Response](#anytype-Rpc-Block-ListTurnInto-Response) |  |
| BlockTextSetText | [Rpc.BlockText.SetText.Request](#anytype-Rpc-BlockText-SetText-Request) | [Rpc.BlockText.SetText.Response](#anytype-Rpc-BlockText-SetText-Response) | Text Block commands *** |
| BlockTextSetColor | [Rpc.BlockText.SetColor.Request](#anytype-Rpc-BlockText-SetColor-Request) | [Rpc.BlockText.SetColor.Response](#anytype-Rpc-BlockText-SetColor-Response) |  |
| BlockTextSetStyle | [Rpc.BlockText.SetStyle.Request](#anytype-Rpc-BlockText-SetStyle-Request) | [Rpc.BlockText.SetStyle.Response](#anytype-Rpc-BlockText-SetStyle-Response) |  |
| BlockTextSetChecked | [Rpc.BlockText.SetChecked.Request](#anytype-Rpc-BlockText-SetChecked-Request) | [Rpc.BlockText.SetChecked.Response](#anytype-Rpc-BlockText-SetChecked-Response) |  |
| BlockTextSetIcon | [Rpc.BlockText.SetIcon.Request](#anytype-Rpc-BlockText-SetIcon-Request) | [Rpc.BlockText.SetIcon.Response](#anytype-Rpc-BlockText-SetIcon-Response) |  |
| BlockTextListSetColor | [Rpc.BlockText.ListSetColor.Request](#anytype-Rpc-BlockText-ListSetColor-Request) | [Rpc.BlockText.ListSetColor.Response](#anytype-Rpc-BlockText-ListSetColor-Response) |  |
| BlockTextListSetMark | [Rpc.BlockText.ListSetMark.Request](#anytype-Rpc-BlockText-ListSetMark-Request) | [Rpc.BlockText.ListSetMark.Response](#anytype-Rpc-BlockText-ListSetMark-Response) |  |
| BlockTextListSetStyle | [Rpc.BlockText.ListSetStyle.Request](#anytype-Rpc-BlockText-ListSetStyle-Request) | [Rpc.BlockText.ListSetStyle.Response](#anytype-Rpc-BlockText-ListSetStyle-Response) |  |
| BlockTextListClearStyle | [Rpc.BlockText.ListClearStyle.Request](#anytype-Rpc-BlockText-ListClearStyle-Request) | [Rpc.BlockText.ListClearStyle.Response](#anytype-Rpc-BlockText-ListClearStyle-Response) |  |
| BlockTextListClearContent | [Rpc.BlockText.ListClearContent.Request](#anytype-Rpc-BlockText-ListClearContent-Request) | [Rpc.BlockText.ListClearContent.Response](#anytype-Rpc-BlockText-ListClearContent-Response) |  |
| BlockFileSetName | [Rpc.BlockFile.SetName.Request](#anytype-Rpc-BlockFile-SetName-Request) | [Rpc.BlockFile.SetName.Response](#anytype-Rpc-BlockFile-SetName-Response) | File block commands *** |
| BlockImageSetName | [Rpc.BlockImage.SetName.Request](#anytype-Rpc-BlockImage-SetName-Request) | [Rpc.BlockImage.SetName.Response](#anytype-Rpc-BlockImage-SetName-Response) |  |
| BlockVideoSetName | [Rpc.BlockVideo.SetName.Request](#anytype-Rpc-BlockVideo-SetName-Request) | [Rpc.BlockVideo.SetName.Response](#anytype-Rpc-BlockVideo-SetName-Response) |  |
| BlockFileCreateAndUpload | [Rpc.BlockFile.CreateAndUpload.Request](#anytype-Rpc-BlockFile-CreateAndUpload-Request) | [Rpc.BlockFile.CreateAndUpload.Response](#anytype-Rpc-BlockFile-CreateAndUpload-Response) |  |
| BlockFileListSetStyle | [Rpc.BlockFile.ListSetStyle.Request](#anytype-Rpc-BlockFile-ListSetStyle-Request) | [Rpc.BlockFile.ListSetStyle.Response](#anytype-Rpc-BlockFile-ListSetStyle-Response) |  |
| BlockDataviewViewCreate | [Rpc.BlockDataview.View.Create.Request](#anytype-Rpc-BlockDataview-View-Create-Request) | [Rpc.BlockDataview.View.Create.Response](#anytype-Rpc-BlockDataview-View-Create-Response) | Dataview block commands *** |
| BlockDataviewViewDelete | [Rpc.BlockDataview.View.Delete.Request](#anytype-Rpc-BlockDataview-View-Delete-Request) | [Rpc.BlockDataview.View.Delete.Response](#anytype-Rpc-BlockDataview-View-Delete-Response) |  |
| BlockDataviewViewUpdate | [Rpc.BlockDataview.View.Update.Request](#anytype-Rpc-BlockDataview-View-Update-Request) | [Rpc.BlockDataview.View.Update.Response](#anytype-Rpc-BlockDataview-View-Update-Response) |  |
| BlockDataviewViewSetActive | [Rpc.BlockDataview.View.SetActive.Request](#anytype-Rpc-BlockDataview-View-SetActive-Request) | [Rpc.BlockDataview.View.SetActive.Response](#anytype-Rpc-BlockDataview-View-SetActive-Response) |  |
| BlockDataviewViewSetPosition | [Rpc.BlockDataview.View.SetPosition.Request](#anytype-Rpc-BlockDataview-View-SetPosition-Request) | [Rpc.BlockDataview.View.SetPosition.Response](#anytype-Rpc-BlockDataview-View-SetPosition-Response) |  |
| BlockDataviewSetSource | [Rpc.BlockDataview.SetSource.Request](#anytype-Rpc-BlockDataview-SetSource-Request) | [Rpc.BlockDataview.SetSource.Response](#anytype-Rpc-BlockDataview-SetSource-Response) |  |
| BlockDataviewRelationAdd | [Rpc.BlockDataview.Relation.Add.Request](#anytype-Rpc-BlockDataview-Relation-Add-Request) | [Rpc.BlockDataview.Relation.Add.Response](#anytype-Rpc-BlockDataview-Relation-Add-Response) |  |
| BlockDataviewRelationDelete | [Rpc.BlockDataview.Relation.Delete.Request](#anytype-Rpc-BlockDataview-Relation-Delete-Request) | [Rpc.BlockDataview.Relation.Delete.Response](#anytype-Rpc-BlockDataview-Relation-Delete-Response) |  |
| BlockDataviewRelationListAvailable | [Rpc.BlockDataview.Relation.ListAvailable.Request](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Request) | [Rpc.BlockDataview.Relation.ListAvailable.Response](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Response) |  |
| BlockDataviewGroupOrderUpdate | [Rpc.BlockDataview.GroupOrder.Update.Request](#anytype-Rpc-BlockDataview-GroupOrder-Update-Request) | [Rpc.BlockDataview.GroupOrder.Update.Response](#anytype-Rpc-BlockDataview-GroupOrder-Update-Response) |  |
| BlockDataviewObjectOrderUpdate | [Rpc.BlockDataview.ObjectOrder.Update.Request](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Request) | [Rpc.BlockDataview.ObjectOrder.Update.Response](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Response) |  |
| BlockDataviewObjectOrderMove | [Rpc.BlockDataview.ObjectOrder.Move.Request](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Request) | [Rpc.BlockDataview.ObjectOrder.Move.Response](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Response) |  |
| BlockDataviewCreateFromExistingObject | [Rpc.BlockDataview.CreateFromExistingObject.Request](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Request) | [Rpc.BlockDataview.CreateFromExistingObject.Response](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Response) |  |
| BlockDataviewFilterAdd | [Rpc.BlockDataview.Filter.Add.Request](#anytype-Rpc-BlockDataview-Filter-Add-Request) | [Rpc.BlockDataview.Filter.Add.Response](#anytype-Rpc-BlockDataview-Filter-Add-Response) |  |
| BlockDataviewFilterRemove | [Rpc.BlockDataview.Filter.Remove.Request](#anytype-Rpc-BlockDataview-Filter-Remove-Request) | [Rpc.BlockDataview.Filter.Remove.Response](#anytype-Rpc-BlockDataview-Filter-Remove-Response) |  |
| BlockDataviewFilterReplace | [Rpc.BlockDataview.Filter.Replace.Request](#anytype-Rpc-BlockDataview-Filter-Replace-Request) | [Rpc.BlockDataview.Filter.Replace.Response](#anytype-Rpc-BlockDataview-Filter-Replace-Response) |  |
| BlockDataviewFilterSort | [Rpc.BlockDataview.Filter.Sort.Request](#anytype-Rpc-BlockDataview-Filter-Sort-Request) | [Rpc.BlockDataview.Filter.Sort.Response](#anytype-Rpc-BlockDataview-Filter-Sort-Response) |  |
| BlockDataviewSortAdd | [Rpc.BlockDataview.Sort.Add.Request](#anytype-Rpc-BlockDataview-Sort-Add-Request) | [Rpc.BlockDataview.Sort.Add.Response](#anytype-Rpc-BlockDataview-Sort-Add-Response) |  |
| BlockDataviewSortRemove | [Rpc.BlockDataview.Sort.Remove.Request](#anytype-Rpc-BlockDataview-Sort-Remove-Request) | [Rpc.BlockDataview.Sort.Remove.Response](#anytype-Rpc-BlockDataview-Sort-Remove-Response) |  |
| BlockDataviewSortReplace | [Rpc.BlockDataview.Sort.Replace.Request](#anytype-Rpc-BlockDataview-Sort-Replace-Request) | [Rpc.BlockDataview.Sort.Replace.Response](#anytype-Rpc-BlockDataview-Sort-Replace-Response) |  |
| BlockDataviewSortSort | [Rpc.BlockDataview.Sort.Sort.Request](#anytype-Rpc-BlockDataview-Sort-Sort-Request) | [Rpc.BlockDataview.Sort.Sort.Response](#anytype-Rpc-BlockDataview-Sort-Sort-Response) |  |
| BlockDataviewViewRelationAdd | [Rpc.BlockDataview.ViewRelation.Add.Request](#anytype-Rpc-BlockDataview-ViewRelation-Add-Request) | [Rpc.BlockDataview.ViewRelation.Add.Response](#anytype-Rpc-BlockDataview-ViewRelation-Add-Response) |  |
| BlockDataviewViewRelationRemove | [Rpc.BlockDataview.ViewRelation.Remove.Request](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Request) | [Rpc.BlockDataview.ViewRelation.Remove.Response](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Response) |  |
| BlockDataviewViewRelationReplace | [Rpc.BlockDataview.ViewRelation.Replace.Request](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Request) | [Rpc.BlockDataview.ViewRelation.Replace.Response](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Response) |  |
| BlockDataviewViewRelationSort | [Rpc.BlockDataview.ViewRelation.Sort.Request](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Request) | [Rpc.BlockDataview.ViewRelation.Sort.Response](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Response) |  |
| BlockTableCreate | [Rpc.BlockTable.Create.Request](#anytype-Rpc-BlockTable-Create-Request) | [Rpc.BlockTable.Create.Response](#anytype-Rpc-BlockTable-Create-Response) | Simple table block commands *** |
| BlockTableExpand | [Rpc.BlockTable.Expand.Request](#anytype-Rpc-BlockTable-Expand-Request) | [Rpc.BlockTable.Expand.Response](#anytype-Rpc-BlockTable-Expand-Response) |  |
| BlockTableRowCreate | [Rpc.BlockTable.RowCreate.Request](#anytype-Rpc-BlockTable-RowCreate-Request) | [Rpc.BlockTable.RowCreate.Response](#anytype-Rpc-BlockTable-RowCreate-Response) |  |
| BlockTableRowDelete | [Rpc.BlockTable.RowDelete.Request](#anytype-Rpc-BlockTable-RowDelete-Request) | [Rpc.BlockTable.RowDelete.Response](#anytype-Rpc-BlockTable-RowDelete-Response) |  |
| BlockTableRowDuplicate | [Rpc.BlockTable.RowDuplicate.Request](#anytype-Rpc-BlockTable-RowDuplicate-Request) | [Rpc.BlockTable.RowDuplicate.Response](#anytype-Rpc-BlockTable-RowDuplicate-Response) |  |
| BlockTableRowSetHeader | [Rpc.BlockTable.RowSetHeader.Request](#anytype-Rpc-BlockTable-RowSetHeader-Request) | [Rpc.BlockTable.RowSetHeader.Response](#anytype-Rpc-BlockTable-RowSetHeader-Response) |  |
| BlockTableColumnCreate | [Rpc.BlockTable.ColumnCreate.Request](#anytype-Rpc-BlockTable-ColumnCreate-Request) | [Rpc.BlockTable.ColumnCreate.Response](#anytype-Rpc-BlockTable-ColumnCreate-Response) |  |
| BlockTableColumnMove | [Rpc.BlockTable.ColumnMove.Request](#anytype-Rpc-BlockTable-ColumnMove-Request) | [Rpc.BlockTable.ColumnMove.Response](#anytype-Rpc-BlockTable-ColumnMove-Response) |  |
| BlockTableColumnDelete | [Rpc.BlockTable.ColumnDelete.Request](#anytype-Rpc-BlockTable-ColumnDelete-Request) | [Rpc.BlockTable.ColumnDelete.Response](#anytype-Rpc-BlockTable-ColumnDelete-Response) |  |
| BlockTableColumnDuplicate | [Rpc.BlockTable.ColumnDuplicate.Request](#anytype-Rpc-BlockTable-ColumnDuplicate-Request) | [Rpc.BlockTable.ColumnDuplicate.Response](#anytype-Rpc-BlockTable-ColumnDuplicate-Response) |  |
| BlockTableRowListFill | [Rpc.BlockTable.RowListFill.Request](#anytype-Rpc-BlockTable-RowListFill-Request) | [Rpc.BlockTable.RowListFill.Response](#anytype-Rpc-BlockTable-RowListFill-Response) |  |
| BlockTableRowListClean | [Rpc.BlockTable.RowListClean.Request](#anytype-Rpc-BlockTable-RowListClean-Request) | [Rpc.BlockTable.RowListClean.Response](#anytype-Rpc-BlockTable-RowListClean-Response) |  |
| BlockTableColumnListFill | [Rpc.BlockTable.ColumnListFill.Request](#anytype-Rpc-BlockTable-ColumnListFill-Request) | [Rpc.BlockTable.ColumnListFill.Response](#anytype-Rpc-BlockTable-ColumnListFill-Response) |  |
| BlockTableSort | [Rpc.BlockTable.Sort.Request](#anytype-Rpc-BlockTable-Sort-Request) | [Rpc.BlockTable.Sort.Response](#anytype-Rpc-BlockTable-Sort-Response) |  |
| BlockCreateWidget | [Rpc.Block.CreateWidget.Request](#anytype-Rpc-Block-CreateWidget-Request) | [Rpc.Block.CreateWidget.Response](#anytype-Rpc-Block-CreateWidget-Response) | Widget commands *** |
| BlockWidgetSetTargetId | [Rpc.BlockWidget.SetTargetId.Request](#anytype-Rpc-BlockWidget-SetTargetId-Request) | [Rpc.BlockWidget.SetTargetId.Response](#anytype-Rpc-BlockWidget-SetTargetId-Response) |  |
| BlockWidgetSetLayout | [Rpc.BlockWidget.SetLayout.Request](#anytype-Rpc-BlockWidget-SetLayout-Request) | [Rpc.BlockWidget.SetLayout.Response](#anytype-Rpc-BlockWidget-SetLayout-Response) |  |
| BlockWidgetSetLimit | [Rpc.BlockWidget.SetLimit.Request](#anytype-Rpc-BlockWidget-SetLimit-Request) | [Rpc.BlockWidget.SetLimit.Response](#anytype-Rpc-BlockWidget-SetLimit-Response) |  |
| BlockLinkCreateWithObject | [Rpc.BlockLink.CreateWithObject.Request](#anytype-Rpc-BlockLink-CreateWithObject-Request) | [Rpc.BlockLink.CreateWithObject.Response](#anytype-Rpc-BlockLink-CreateWithObject-Response) | Other specific block commands *** |
| BlockLinkListSetAppearance | [Rpc.BlockLink.ListSetAppearance.Request](#anytype-Rpc-BlockLink-ListSetAppearance-Request) | [Rpc.BlockLink.ListSetAppearance.Response](#anytype-Rpc-BlockLink-ListSetAppearance-Response) |  |
| BlockBookmarkFetch | [Rpc.BlockBookmark.Fetch.Request](#anytype-Rpc-BlockBookmark-Fetch-Request) | [Rpc.BlockBookmark.Fetch.Response](#anytype-Rpc-BlockBookmark-Fetch-Response) |  |
| BlockBookmarkCreateAndFetch | [Rpc.BlockBookmark.CreateAndFetch.Request](#anytype-Rpc-BlockBookmark-CreateAndFetch-Request) | [Rpc.BlockBookmark.CreateAndFetch.Response](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response) |  |
| BlockRelationSetKey | [Rpc.BlockRelation.SetKey.Request](#anytype-Rpc-BlockRelation-SetKey-Request) | [Rpc.BlockRelation.SetKey.Response](#anytype-Rpc-BlockRelation-SetKey-Response) |  |
| BlockRelationAdd | [Rpc.BlockRelation.Add.Request](#anytype-Rpc-BlockRelation-Add-Request) | [Rpc.BlockRelation.Add.Response](#anytype-Rpc-BlockRelation-Add-Response) |  |
| BlockDivListSetStyle | [Rpc.BlockDiv.ListSetStyle.Request](#anytype-Rpc-BlockDiv-ListSetStyle-Request) | [Rpc.BlockDiv.ListSetStyle.Response](#anytype-Rpc-BlockDiv-ListSetStyle-Response) |  |
| BlockLatexSetText | [Rpc.BlockLatex.SetText.Request](#anytype-Rpc-BlockLatex-SetText-Request) | [Rpc.BlockLatex.SetText.Response](#anytype-Rpc-BlockLatex-SetText-Response) |  |
| ProcessCancel | [Rpc.Process.Cancel.Request](#anytype-Rpc-Process-Cancel-Request) | [Rpc.Process.Cancel.Response](#anytype-Rpc-Process-Cancel-Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype-Rpc-Log-Send-Request) | [Rpc.Log.Send.Response](#anytype-Rpc-Log-Send-Response) |  |
| DebugTree | [Rpc.Debug.Tree.Request](#anytype-Rpc-Debug-Tree-Request) | [Rpc.Debug.Tree.Response](#anytype-Rpc-Debug-Tree-Response) |  |
| DebugTreeHeads | [Rpc.Debug.TreeHeads.Request](#anytype-Rpc-Debug-TreeHeads-Request) | [Rpc.Debug.TreeHeads.Response](#anytype-Rpc-Debug-TreeHeads-Response) |  |
| DebugSpaceSummary | [Rpc.Debug.SpaceSummary.Request](#anytype-Rpc-Debug-SpaceSummary-Request) | [Rpc.Debug.SpaceSummary.Response](#anytype-Rpc-Debug-SpaceSummary-Response) |  |
| DebugExportLocalstore | [Rpc.Debug.ExportLocalstore.Request](#anytype-Rpc-Debug-ExportLocalstore-Request) | [Rpc.Debug.ExportLocalstore.Response](#anytype-Rpc-Debug-ExportLocalstore-Response) |  |
| DebugPing | [Rpc.Debug.Ping.Request](#anytype-Rpc-Debug-Ping-Request) | [Rpc.Debug.Ping.Response](#anytype-Rpc-Debug-Ping-Response) |  |
| MetricsSetParameters | [Rpc.Metrics.SetParameters.Request](#anytype-Rpc-Metrics-SetParameters-Request) | [Rpc.Metrics.SetParameters.Response](#anytype-Rpc-Metrics-SetParameters-Response) |  |
| ListenSessionEvents | [StreamRequest](#anytype-StreamRequest) | [Event](#anytype-Event) stream | used only for lib-server via grpc |

 



<a name="pb_protos_changes-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/changes.proto



<a name="anytype-Change"></a>

### Change
the element of change tree used to store and internal apply smartBlock history


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| previous_ids | [string](#string) | repeated | ids of previous changes |
| last_snapshot_id | [string](#string) |  | id of the last snapshot |
| previous_meta_ids | [string](#string) | repeated | ids of the last changes with details/relations content |
| content | [Change.Content](#anytype-Change-Content) | repeated | set of actions to apply |
| snapshot | [Change.Snapshot](#anytype-Change-Snapshot) |  | snapshot - when not null, the Content will be ignored |
| fileKeys | [Change.FileKeys](#anytype-Change-FileKeys) | repeated | file keys related to changes content |
| timestamp | [int64](#int64) |  | creation timestamp |
| version | [uint32](#uint32) |  | version of business logic |






<a name="anytype-Change-BlockCreate"></a>

### Change.BlockCreate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| blocks | [model.Block](#anytype-model-Block) | repeated |  |






<a name="anytype-Change-BlockDuplicate"></a>

### Change.BlockDuplicate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Change-BlockMove"></a>

### Change.BlockMove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Change-BlockRemove"></a>

### Change.BlockRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype-Change-BlockUpdate"></a>

### Change.BlockUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| events | [Event.Message](#anytype-Event-Message) | repeated |  |






<a name="anytype-Change-Content"></a>

### Change.Content



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockCreate | [Change.BlockCreate](#anytype-Change-BlockCreate) |  |  |
| blockUpdate | [Change.BlockUpdate](#anytype-Change-BlockUpdate) |  |  |
| blockRemove | [Change.BlockRemove](#anytype-Change-BlockRemove) |  |  |
| blockMove | [Change.BlockMove](#anytype-Change-BlockMove) |  |  |
| blockDuplicate | [Change.BlockDuplicate](#anytype-Change-BlockDuplicate) |  |  |
| relationAdd | [Change.RelationAdd](#anytype-Change-RelationAdd) |  |  |
| relationRemove | [Change.RelationRemove](#anytype-Change-RelationRemove) |  |  |
| detailsSet | [Change.DetailsSet](#anytype-Change-DetailsSet) |  |  |
| detailsUnset | [Change.DetailsUnset](#anytype-Change-DetailsUnset) |  |  |
| old_relationAdd | [Change._RelationAdd](#anytype-Change-_RelationAdd) |  | deprecated |
| old_relationRemove | [Change._RelationRemove](#anytype-Change-_RelationRemove) |  |  |
| old_relationUpdate | [Change._RelationUpdate](#anytype-Change-_RelationUpdate) |  |  |
| objectTypeAdd | [Change.ObjectTypeAdd](#anytype-Change-ObjectTypeAdd) |  |  |
| objectTypeRemove | [Change.ObjectTypeRemove](#anytype-Change-ObjectTypeRemove) |  |  |
| storeKeySet | [Change.StoreKeySet](#anytype-Change-StoreKeySet) |  |  |
| storeKeyUnset | [Change.StoreKeyUnset](#anytype-Change-StoreKeyUnset) |  |  |
| storeSliceUpdate | [Change.StoreSliceUpdate](#anytype-Change-StoreSliceUpdate) |  |  |






<a name="anytype-Change-DetailsSet"></a>

### Change.DetailsSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  |  |






<a name="anytype-Change-DetailsUnset"></a>

### Change.DetailsUnset



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |






<a name="anytype-Change-FileKeys"></a>

### Change.FileKeys



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| keys | [Change.FileKeys.KeysEntry](#anytype-Change-FileKeys-KeysEntry) | repeated |  |






<a name="anytype-Change-FileKeys-KeysEntry"></a>

### Change.FileKeys.KeysEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="anytype-Change-ObjectTypeAdd"></a>

### Change.ObjectTypeAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype-Change-ObjectTypeRemove"></a>

### Change.ObjectTypeRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype-Change-RelationAdd"></a>

### Change.RelationAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relationLinks | [model.RelationLink](#anytype-model-RelationLink) | repeated |  |






<a name="anytype-Change-RelationRemove"></a>

### Change.RelationRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relationKey | [string](#string) | repeated |  |






<a name="anytype-Change-Snapshot"></a>

### Change.Snapshot



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| logHeads | [Change.Snapshot.LogHeadsEntry](#anytype-Change-Snapshot-LogHeadsEntry) | repeated | logId -&gt; lastChangeId |
| data | [model.SmartBlockSnapshotBase](#anytype-model-SmartBlockSnapshotBase) |  | snapshot data |
| fileKeys | [Change.FileKeys](#anytype-Change-FileKeys) | repeated | all file keys related to doc |






<a name="anytype-Change-Snapshot-LogHeadsEntry"></a>

### Change.Snapshot.LogHeadsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="anytype-Change-StoreKeySet"></a>

### Change.StoreKeySet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  |  |






<a name="anytype-Change-StoreKeyUnset"></a>

### Change.StoreKeyUnset



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |






<a name="anytype-Change-StoreSliceUpdate"></a>

### Change.StoreSliceUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| add | [Change.StoreSliceUpdate.Add](#anytype-Change-StoreSliceUpdate-Add) |  |  |
| remove | [Change.StoreSliceUpdate.Remove](#anytype-Change-StoreSliceUpdate-Remove) |  |  |
| move | [Change.StoreSliceUpdate.Move](#anytype-Change-StoreSliceUpdate-Move) |  |  |






<a name="anytype-Change-StoreSliceUpdate-Add"></a>

### Change.StoreSliceUpdate.Add



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Change-StoreSliceUpdate-Move"></a>

### Change.StoreSliceUpdate.Move



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Change-StoreSliceUpdate-Remove"></a>

### Change.StoreSliceUpdate.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype-Change-_RelationAdd"></a>

### Change._RelationAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relation | [model.Relation](#anytype-model-Relation) |  |  |






<a name="anytype-Change-_RelationRemove"></a>

### Change._RelationRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |






<a name="anytype-Change-_RelationUpdate"></a>

### Change._RelationUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| format | [model.RelationFormat](#anytype-model-RelationFormat) |  |  |
| name | [string](#string) |  |  |
| defaultValue | [google.protobuf.Value](#google-protobuf-Value) |  |  |
| objectTypes | [Change._RelationUpdate.ObjectTypes](#anytype-Change-_RelationUpdate-ObjectTypes) |  |  |
| multi | [bool](#bool) |  |  |
| selectDict | [Change._RelationUpdate.Dict](#anytype-Change-_RelationUpdate-Dict) |  |  |






<a name="anytype-Change-_RelationUpdate-Dict"></a>

### Change._RelationUpdate.Dict



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dict | [model.Relation.Option](#anytype-model-Relation-Option) | repeated |  |






<a name="anytype-Change-_RelationUpdate-ObjectTypes"></a>

### Change._RelationUpdate.ObjectTypes



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypes | [string](#string) | repeated |  |





 

 

 

 



<a name="pb_protos_commands-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/commands.proto



<a name="anytype-Empty"></a>

### Empty







<a name="anytype-Rpc"></a>

### Rpc
Rpc is a namespace, that agregates all of the service commands between client and middleware.
Structure: Topic &gt; Subtopic &gt; Subsub... &gt; Action &gt; (Request, Response).
Request – message from a client.
Response – message from a middleware.






<a name="anytype-Rpc-Account"></a>

### Rpc.Account







<a name="anytype-Rpc-Account-Config"></a>

### Rpc.Account.Config



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enableDataview | [bool](#bool) |  |  |
| enableDebug | [bool](#bool) |  |  |
| enablePrereleaseChannel | [bool](#bool) |  |  |
| enableSpaces | [bool](#bool) |  |  |
| extra | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Account-ConfigUpdate"></a>

### Rpc.Account.ConfigUpdate







<a name="anytype-Rpc-Account-ConfigUpdate-Request"></a>

### Rpc.Account.ConfigUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| timeZone | [string](#string) |  |  |
| IPFSStorageAddr | [string](#string) |  |  |






<a name="anytype-Rpc-Account-ConfigUpdate-Response"></a>

### Rpc.Account.ConfigUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.ConfigUpdate.Response.Error](#anytype-Rpc-Account-ConfigUpdate-Response-Error) |  |  |






<a name="anytype-Rpc-Account-ConfigUpdate-Response-Error"></a>

### Rpc.Account.ConfigUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.ConfigUpdate.Response.Error.Code](#anytype-Rpc-Account-ConfigUpdate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Create"></a>

### Rpc.Account.Create







<a name="anytype-Rpc-Account-Create-Request"></a>

### Rpc.Account.Create.Request
Front end to middleware request-to-create-an account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Account name |
| avatarLocalPath | [string](#string) |  | Path to an image, that will be used as an avatar of this account |
| storePath | [string](#string) |  | Path to local storage |
| icon | [int64](#int64) |  | Option of pre-installed icon |
| alphaInviteCode | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Create-Response"></a>

### Rpc.Account.Create.Response
Middleware-to-front-end response for an account creation request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Create.Response.Error](#anytype-Rpc-Account-Create-Response-Error) |  | Error while trying to create an account |
| account | [model.Account](#anytype-model-Account) |  | A newly created account; In case of a failure, i.e. error is non-NULL, the account model should contain empty/default-value fields |
| config | [Rpc.Account.Config](#anytype-Rpc-Account-Config) |  | deprecated, use account |






<a name="anytype-Rpc-Account-Create-Response-Error"></a>

### Rpc.Account.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Create.Response.Error.Code](#anytype-Rpc-Account-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Delete"></a>

### Rpc.Account.Delete







<a name="anytype-Rpc-Account-Delete-Request"></a>

### Rpc.Account.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| revert | [bool](#bool) |  |  |






<a name="anytype-Rpc-Account-Delete-Response"></a>

### Rpc.Account.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Delete.Response.Error](#anytype-Rpc-Account-Delete-Response-Error) |  | Error while trying to recover an account |
| status | [model.Account.Status](#anytype-model-Account-Status) |  |  |






<a name="anytype-Rpc-Account-Delete-Response-Error"></a>

### Rpc.Account.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Delete.Response.Error.Code](#anytype-Rpc-Account-Delete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-GetConfig"></a>

### Rpc.Account.GetConfig







<a name="anytype-Rpc-Account-GetConfig-Get"></a>

### Rpc.Account.GetConfig.Get







<a name="anytype-Rpc-Account-GetConfig-Get-Request"></a>

### Rpc.Account.GetConfig.Get.Request







<a name="anytype-Rpc-Account-Move"></a>

### Rpc.Account.Move







<a name="anytype-Rpc-Account-Move-Request"></a>

### Rpc.Account.Move.Request
Front-end-to-middleware request to move a account to a new disk location


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| newPath | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Move-Response"></a>

### Rpc.Account.Move.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Move.Response.Error](#anytype-Rpc-Account-Move-Response-Error) |  |  |






<a name="anytype-Rpc-Account-Move-Response-Error"></a>

### Rpc.Account.Move.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Move.Response.Error.Code](#anytype-Rpc-Account-Move-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Recover"></a>

### Rpc.Account.Recover







<a name="anytype-Rpc-Account-Recover-Request"></a>

### Rpc.Account.Recover.Request
Front end to middleware request-to-start-search of an accounts for a recovered mnemonic.
Each of an account that would be found will come with an AccountAdd event






<a name="anytype-Rpc-Account-Recover-Response"></a>

### Rpc.Account.Recover.Response
Middleware-to-front-end response to an account recover request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Recover.Response.Error](#anytype-Rpc-Account-Recover-Response-Error) |  | Error while trying to recover an account |






<a name="anytype-Rpc-Account-Recover-Response-Error"></a>

### Rpc.Account.Recover.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Recover.Response.Error.Code](#anytype-Rpc-Account-Recover-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-RecoverFromLegacyExport"></a>

### Rpc.Account.RecoverFromLegacyExport







<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Request"></a>

### Rpc.Account.RecoverFromLegacyExport.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| rootPath | [string](#string) |  |  |
| icon | [int64](#int64) |  |  |






<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Response"></a>

### Rpc.Account.RecoverFromLegacyExport.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountId | [string](#string) |  |  |
| error | [Rpc.Account.RecoverFromLegacyExport.Response.Error](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error) |  |  |






<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error"></a>

### Rpc.Account.RecoverFromLegacyExport.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.RecoverFromLegacyExport.Response.Error.Code](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Select"></a>

### Rpc.Account.Select







<a name="anytype-Rpc-Account-Select-Request"></a>

### Rpc.Account.Select.Request
Front end to middleware request-to-launch-a specific account using account id and a root path
User can select an account from those, that came with an AccountAdd events


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Id of a selected account |
| rootPath | [string](#string) |  | Root path is optional, set if this is a first request |






<a name="anytype-Rpc-Account-Select-Response"></a>

### Rpc.Account.Select.Response
Middleware-to-front-end response for an account select request, that can contain a NULL error and selected account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Select.Response.Error](#anytype-Rpc-Account-Select-Response-Error) |  | Error while trying to launch/select an account |
| account | [model.Account](#anytype-model-Account) |  | Selected account |
| config | [Rpc.Account.Config](#anytype-Rpc-Account-Config) |  | deprecated, use account |






<a name="anytype-Rpc-Account-Select-Response-Error"></a>

### Rpc.Account.Select.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Select.Response.Error.Code](#anytype-Rpc-Account-Select-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Stop"></a>

### Rpc.Account.Stop







<a name="anytype-Rpc-Account-Stop-Request"></a>

### Rpc.Account.Stop.Request
Front end to middleware request to stop currently running account node and optionally remove the locally stored data


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| removeData | [bool](#bool) |  |  |






<a name="anytype-Rpc-Account-Stop-Response"></a>

### Rpc.Account.Stop.Response
Middleware-to-front-end response for an account stop request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Stop.Response.Error](#anytype-Rpc-Account-Stop-Response-Error) |  | Error while trying to launch/select an account |






<a name="anytype-Rpc-Account-Stop-Response-Error"></a>

### Rpc.Account.Stop.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Stop.Response.Error.Code](#anytype-Rpc-Account-Stop-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-App"></a>

### Rpc.App







<a name="anytype-Rpc-App-GetVersion"></a>

### Rpc.App.GetVersion







<a name="anytype-Rpc-App-GetVersion-Request"></a>

### Rpc.App.GetVersion.Request







<a name="anytype-Rpc-App-GetVersion-Response"></a>

### Rpc.App.GetVersion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.App.GetVersion.Response.Error](#anytype-Rpc-App-GetVersion-Response-Error) |  |  |
| version | [string](#string) |  |  |
| details | [string](#string) |  | build date, branch and commit |






<a name="anytype-Rpc-App-GetVersion-Response-Error"></a>

### Rpc.App.GetVersion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.App.GetVersion.Response.Error.Code](#anytype-Rpc-App-GetVersion-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-App-SetDeviceState"></a>

### Rpc.App.SetDeviceState







<a name="anytype-Rpc-App-SetDeviceState-Request"></a>

### Rpc.App.SetDeviceState.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| deviceState | [Rpc.App.SetDeviceState.Request.DeviceState](#anytype-Rpc-App-SetDeviceState-Request-DeviceState) |  |  |






<a name="anytype-Rpc-App-SetDeviceState-Response"></a>

### Rpc.App.SetDeviceState.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.App.SetDeviceState.Response.Error](#anytype-Rpc-App-SetDeviceState-Response-Error) |  |  |






<a name="anytype-Rpc-App-SetDeviceState-Response-Error"></a>

### Rpc.App.SetDeviceState.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.App.SetDeviceState.Response.Error.Code](#anytype-Rpc-App-SetDeviceState-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-App-Shutdown"></a>

### Rpc.App.Shutdown







<a name="anytype-Rpc-App-Shutdown-Request"></a>

### Rpc.App.Shutdown.Request







<a name="anytype-Rpc-App-Shutdown-Response"></a>

### Rpc.App.Shutdown.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.App.Shutdown.Response.Error](#anytype-Rpc-App-Shutdown-Response-Error) |  |  |






<a name="anytype-Rpc-App-Shutdown-Response-Error"></a>

### Rpc.App.Shutdown.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.App.Shutdown.Response.Error.Code](#anytype-Rpc-App-Shutdown-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block"></a>

### Rpc.Block
Block commands






<a name="anytype-Rpc-Block-Copy"></a>

### Rpc.Block.Copy







<a name="anytype-Rpc-Block-Copy-Request"></a>

### Rpc.Block.Copy.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype-model-Block) | repeated |  |
| selectedTextRange | [model.Range](#anytype-model-Range) |  |  |






<a name="anytype-Rpc-Block-Copy-Response"></a>

### Rpc.Block.Copy.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Copy.Response.Error](#anytype-Rpc-Block-Copy-Response-Error) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype-model-Block) | repeated |  |






<a name="anytype-Rpc-Block-Copy-Response-Error"></a>

### Rpc.Block.Copy.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Copy.Response.Error.Code](#anytype-Rpc-Block-Copy-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Create"></a>

### Rpc.Block.Create
Create a Smart/Internal block. Request can contain a block with a content, or it can be an empty block with a specific block.content.
**Example scenario**
1A. Create Page on a dashboard
    1. Front -&gt; MW: Rpc.Block.Create.Request(blockId:dashboard.id, position:bottom, block: emtpy block with page content and id = &#34;&#34;)
    2. Front -&gt; MW: Rpc.Block.Close.Request(block: dashboard.id)
    3. Front &lt;- MW: Rpc.Block.Close.Response(err)
    4. Front &lt;- MW: Rpc.Block.Create.Response(page.id)
    5. Front &lt;- MW: Rpc.Block.Open.Response(err)
    6. Front &lt;- MW: Event.Block.Show(page)
1B. Create Page on a Page
    1. Front -&gt; MW: Rpc.Block.Create.Request(blockId:dashboard.id, position:bottom, block: emtpy block with page content and id = &#34;&#34;)
    2. Front &lt;- MW: Rpc.Block.Create.Response(newPage.id)
    3. Front &lt;- MW: Event.Block.Show(newPage)






<a name="anytype-Rpc-Block-Create-Request"></a>

### Rpc.Block.Create.Request
common simple block command


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest block |
| block | [model.Block](#anytype-model-Block) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |






<a name="anytype-Rpc-Block-Create-Response"></a>

### Rpc.Block.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Create.Response.Error](#anytype-Rpc-Block-Create-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Create-Response-Error"></a>

### Rpc.Block.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Create.Response.Error.Code](#anytype-Rpc-Block-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-CreateWidget"></a>

### Rpc.Block.CreateWidget







<a name="anytype-Rpc-Block-CreateWidget-Request"></a>

### Rpc.Block.CreateWidget.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest block |
| block | [model.Block](#anytype-model-Block) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| widgetLayout | [model.Block.Content.Widget.Layout](#anytype-model-Block-Content-Widget-Layout) |  |  |
| objectLimit | [int32](#int32) |  |  |






<a name="anytype-Rpc-Block-CreateWidget-Response"></a>

### Rpc.Block.CreateWidget.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CreateWidget.Response.Error](#anytype-Rpc-Block-CreateWidget-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-CreateWidget-Response-Error"></a>

### Rpc.Block.CreateWidget.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CreateWidget.Response.Error.Code](#anytype-Rpc-Block-CreateWidget-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Cut"></a>

### Rpc.Block.Cut







<a name="anytype-Rpc-Block-Cut-Request"></a>

### Rpc.Block.Cut.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype-model-Block) | repeated |  |
| selectedTextRange | [model.Range](#anytype-model-Range) |  |  |






<a name="anytype-Rpc-Block-Cut-Response"></a>

### Rpc.Block.Cut.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Cut.Response.Error](#anytype-Rpc-Block-Cut-Response-Error) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype-model-Block) | repeated |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Cut-Response-Error"></a>

### Rpc.Block.Cut.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Cut.Response.Error.Code](#anytype-Rpc-Block-Cut-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Download"></a>

### Rpc.Block.Download







<a name="anytype-Rpc-Block-Download-Request"></a>

### Rpc.Block.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Download-Response"></a>

### Rpc.Block.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Download.Response.Error](#anytype-Rpc-Block-Download-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Download-Response-Error"></a>

### Rpc.Block.Download.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Download.Response.Error.Code](#anytype-Rpc-Block-Download-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Export"></a>

### Rpc.Block.Export







<a name="anytype-Rpc-Block-Export-Request"></a>

### Rpc.Block.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype-model-Block) | repeated |  |






<a name="anytype-Rpc-Block-Export-Response"></a>

### Rpc.Block.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Export.Response.Error](#anytype-Rpc-Block-Export-Response-Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Export-Response-Error"></a>

### Rpc.Block.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Export.Response.Error.Code](#anytype-Rpc-Block-Export-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListConvertToObjects"></a>

### Rpc.Block.ListConvertToObjects







<a name="anytype-Rpc-Block-ListConvertToObjects-Request"></a>

### Rpc.Block.ListConvertToObjects.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| objectType | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListConvertToObjects-Response"></a>

### Rpc.Block.ListConvertToObjects.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListConvertToObjects.Response.Error](#anytype-Rpc-Block-ListConvertToObjects-Response-Error) |  |  |
| linkIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListConvertToObjects-Response-Error"></a>

### Rpc.Block.ListConvertToObjects.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListConvertToObjects.Response.Error.Code](#anytype-Rpc-Block-ListConvertToObjects-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListDelete"></a>

### Rpc.Block.ListDelete
Remove blocks from the childrenIds of its parents






<a name="anytype-Rpc-Block-ListDelete-Request"></a>

### Rpc.Block.ListDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockIds | [string](#string) | repeated | targets to remove |






<a name="anytype-Rpc-Block-ListDelete-Response"></a>

### Rpc.Block.ListDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListDelete.Response.Error](#anytype-Rpc-Block-ListDelete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListDelete-Response-Error"></a>

### Rpc.Block.ListDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListDelete.Response.Error.Code](#anytype-Rpc-Block-ListDelete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListDuplicate"></a>

### Rpc.Block.ListDuplicate
Makes blocks copy by given ids and paste it to shown place






<a name="anytype-Rpc-Block-ListDuplicate-Request"></a>

### Rpc.Block.ListDuplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest block |
| blockIds | [string](#string) | repeated | id of block for duplicate |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| targetContextId | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListDuplicate-Response"></a>

### Rpc.Block.ListDuplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListDuplicate.Response.Error](#anytype-Rpc-Block-ListDuplicate-Response-Error) |  |  |
| blockIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListDuplicate-Response-Error"></a>

### Rpc.Block.ListDuplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListDuplicate.Response.Error.Code](#anytype-Rpc-Block-ListDuplicate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListMoveToExistingObject"></a>

### Rpc.Block.ListMoveToExistingObject







<a name="anytype-Rpc-Block-ListMoveToExistingObject-Request"></a>

### Rpc.Block.ListMoveToExistingObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| targetContextId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype-model-Block-Position) |  | position relatively to the dropTargetId simple block |






<a name="anytype-Rpc-Block-ListMoveToExistingObject-Response"></a>

### Rpc.Block.ListMoveToExistingObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListMoveToExistingObject.Response.Error](#anytype-Rpc-Block-ListMoveToExistingObject-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListMoveToExistingObject-Response-Error"></a>

### Rpc.Block.ListMoveToExistingObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListMoveToExistingObject.Response.Error.Code](#anytype-Rpc-Block-ListMoveToExistingObject-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListMoveToNewObject"></a>

### Rpc.Block.ListMoveToNewObject







<a name="anytype-Rpc-Block-ListMoveToNewObject-Request"></a>

### Rpc.Block.ListMoveToNewObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | new object details |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype-model-Block-Position) |  | position relatively to the dropTargetId simple block |






<a name="anytype-Rpc-Block-ListMoveToNewObject-Response"></a>

### Rpc.Block.ListMoveToNewObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListMoveToNewObject.Response.Error](#anytype-Rpc-Block-ListMoveToNewObject-Response-Error) |  |  |
| linkId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListMoveToNewObject-Response-Error"></a>

### Rpc.Block.ListMoveToNewObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListMoveToNewObject.Response.Error.Code](#anytype-Rpc-Block-ListMoveToNewObject-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListSetAlign"></a>

### Rpc.Block.ListSetAlign







<a name="anytype-Rpc-Block-ListSetAlign-Request"></a>

### Rpc.Block.ListSetAlign.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated | when empty - align will be applied as layoutAlign |
| align | [model.Block.Align](#anytype-model-Block-Align) |  |  |






<a name="anytype-Rpc-Block-ListSetAlign-Response"></a>

### Rpc.Block.ListSetAlign.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetAlign.Response.Error](#anytype-Rpc-Block-ListSetAlign-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListSetAlign-Response-Error"></a>

### Rpc.Block.ListSetAlign.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetAlign.Response.Error.Code](#anytype-Rpc-Block-ListSetAlign-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListSetBackgroundColor"></a>

### Rpc.Block.ListSetBackgroundColor







<a name="anytype-Rpc-Block-ListSetBackgroundColor-Request"></a>

### Rpc.Block.ListSetBackgroundColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListSetBackgroundColor-Response"></a>

### Rpc.Block.ListSetBackgroundColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetBackgroundColor.Response.Error](#anytype-Rpc-Block-ListSetBackgroundColor-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListSetBackgroundColor-Response-Error"></a>

### Rpc.Block.ListSetBackgroundColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetBackgroundColor.Response.Error.Code](#anytype-Rpc-Block-ListSetBackgroundColor-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListSetFields"></a>

### Rpc.Block.ListSetFields







<a name="anytype-Rpc-Block-ListSetFields-Request"></a>

### Rpc.Block.ListSetFields.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockFields | [Rpc.Block.ListSetFields.Request.BlockField](#anytype-Rpc-Block-ListSetFields-Request-BlockField) | repeated |  |






<a name="anytype-Rpc-Block-ListSetFields-Request-BlockField"></a>

### Rpc.Block.ListSetFields.Request.BlockField



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Block-ListSetFields-Response"></a>

### Rpc.Block.ListSetFields.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetFields.Response.Error](#anytype-Rpc-Block-ListSetFields-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListSetFields-Response-Error"></a>

### Rpc.Block.ListSetFields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetFields.Response.Error.Code](#anytype-Rpc-Block-ListSetFields-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListSetVerticalAlign"></a>

### Rpc.Block.ListSetVerticalAlign







<a name="anytype-Rpc-Block-ListSetVerticalAlign-Request"></a>

### Rpc.Block.ListSetVerticalAlign.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockIds | [string](#string) | repeated |  |
| verticalAlign | [model.Block.VerticalAlign](#anytype-model-Block-VerticalAlign) |  |  |






<a name="anytype-Rpc-Block-ListSetVerticalAlign-Response"></a>

### Rpc.Block.ListSetVerticalAlign.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetVerticalAlign.Response.Error](#anytype-Rpc-Block-ListSetVerticalAlign-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListSetVerticalAlign-Response-Error"></a>

### Rpc.Block.ListSetVerticalAlign.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetVerticalAlign.Response.Error.Code](#anytype-Rpc-Block-ListSetVerticalAlign-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListTurnInto"></a>

### Rpc.Block.ListTurnInto







<a name="anytype-Rpc-Block-ListTurnInto-Request"></a>

### Rpc.Block.ListTurnInto.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |






<a name="anytype-Rpc-Block-ListTurnInto-Response"></a>

### Rpc.Block.ListTurnInto.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListTurnInto.Response.Error](#anytype-Rpc-Block-ListTurnInto-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-ListTurnInto-Response-Error"></a>

### Rpc.Block.ListTurnInto.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListTurnInto.Response.Error.Code](#anytype-Rpc-Block-ListTurnInto-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-ListUpdate"></a>

### Rpc.Block.ListUpdate







<a name="anytype-Rpc-Block-ListUpdate-Request"></a>

### Rpc.Block.ListUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| text | [Rpc.Block.ListUpdate.Request.Text](#anytype-Rpc-Block-ListUpdate-Request-Text) |  |  |
| backgroundColor | [string](#string) |  |  |
| align | [model.Block.Align](#anytype-model-Block-Align) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| divStyle | [model.Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style) |  |  |
| fileStyle | [model.Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |






<a name="anytype-Rpc-Block-ListUpdate-Request-Text"></a>

### Rpc.Block.ListUpdate.Request.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |
| color | [string](#string) |  |  |
| mark | [model.Block.Content.Text.Mark](#anytype-model-Block-Content-Text-Mark) |  |  |






<a name="anytype-Rpc-Block-Merge"></a>

### Rpc.Block.Merge







<a name="anytype-Rpc-Block-Merge-Request"></a>

### Rpc.Block.Merge.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| firstBlockId | [string](#string) |  |  |
| secondBlockId | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Merge-Response"></a>

### Rpc.Block.Merge.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Merge.Response.Error](#anytype-Rpc-Block-Merge-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Merge-Response-Error"></a>

### Rpc.Block.Merge.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Merge.Response.Error.Code](#anytype-Rpc-Block-Merge-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Paste"></a>

### Rpc.Block.Paste







<a name="anytype-Rpc-Block-Paste-Request"></a>

### Rpc.Block.Paste.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| focusedBlockId | [string](#string) |  |  |
| selectedTextRange | [model.Range](#anytype-model-Range) |  |  |
| selectedBlockIds | [string](#string) | repeated |  |
| isPartOfBlock | [bool](#bool) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype-model-Block) | repeated |  |
| fileSlot | [Rpc.Block.Paste.Request.File](#anytype-Rpc-Block-Paste-Request-File) | repeated |  |






<a name="anytype-Rpc-Block-Paste-Request-File"></a>

### Rpc.Block.Paste.Request.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| data | [bytes](#bytes) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Paste-Response"></a>

### Rpc.Block.Paste.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Paste.Response.Error](#anytype-Rpc-Block-Paste-Response-Error) |  |  |
| blockIds | [string](#string) | repeated |  |
| caretPosition | [int32](#int32) |  |  |
| isSameBlockCaret | [bool](#bool) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Paste-Response-Error"></a>

### Rpc.Block.Paste.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Paste.Response.Error.Code](#anytype-Rpc-Block-Paste-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Replace"></a>

### Rpc.Block.Replace







<a name="anytype-Rpc-Block-Replace-Request"></a>

### Rpc.Block.Replace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| block | [model.Block](#anytype-model-Block) |  |  |






<a name="anytype-Rpc-Block-Replace-Response"></a>

### Rpc.Block.Replace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Replace.Response.Error](#anytype-Rpc-Block-Replace-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Replace-Response-Error"></a>

### Rpc.Block.Replace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Replace.Response.Error.Code](#anytype-Rpc-Block-Replace-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-SetFields"></a>

### Rpc.Block.SetFields







<a name="anytype-Rpc-Block-SetFields-Request"></a>

### Rpc.Block.SetFields.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Block-SetFields-Response"></a>

### Rpc.Block.SetFields.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.SetFields.Response.Error](#anytype-Rpc-Block-SetFields-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-SetFields-Response-Error"></a>

### Rpc.Block.SetFields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.SetFields.Response.Error.Code](#anytype-Rpc-Block-SetFields-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Split"></a>

### Rpc.Block.Split







<a name="anytype-Rpc-Block-Split-Request"></a>

### Rpc.Block.Split.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype-model-Range) |  |  |
| style | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |
| mode | [Rpc.Block.Split.Request.Mode](#anytype-Rpc-Block-Split-Request-Mode) |  |  |






<a name="anytype-Rpc-Block-Split-Response"></a>

### Rpc.Block.Split.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Split.Response.Error](#anytype-Rpc-Block-Split-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Split-Response-Error"></a>

### Rpc.Block.Split.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Split.Response.Error.Code](#anytype-Rpc-Block-Split-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Upload"></a>

### Rpc.Block.Upload







<a name="anytype-Rpc-Block-Upload-Request"></a>

### Rpc.Block.Upload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| filePath | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Upload-Response"></a>

### Rpc.Block.Upload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Upload.Response.Error](#anytype-Rpc-Block-Upload-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Block-Upload-Response-Error"></a>

### Rpc.Block.Upload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Upload.Response.Error.Code](#anytype-Rpc-Block-Upload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockBookmark"></a>

### Rpc.BlockBookmark







<a name="anytype-Rpc-BlockBookmark-CreateAndFetch"></a>

### Rpc.BlockBookmark.CreateAndFetch







<a name="anytype-Rpc-BlockBookmark-CreateAndFetch-Request"></a>

### Rpc.BlockBookmark.CreateAndFetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-BlockBookmark-CreateAndFetch-Response"></a>

### Rpc.BlockBookmark.CreateAndFetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockBookmark.CreateAndFetch.Response.Error](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error"></a>

### Rpc.BlockBookmark.CreateAndFetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockBookmark-Fetch"></a>

### Rpc.BlockBookmark.Fetch







<a name="anytype-Rpc-BlockBookmark-Fetch-Request"></a>

### Rpc.BlockBookmark.Fetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-BlockBookmark-Fetch-Response"></a>

### Rpc.BlockBookmark.Fetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockBookmark.Fetch.Response.Error](#anytype-Rpc-BlockBookmark-Fetch-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockBookmark-Fetch-Response-Error"></a>

### Rpc.BlockBookmark.Fetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockBookmark.Fetch.Response.Error.Code](#anytype-Rpc-BlockBookmark-Fetch-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview"></a>

### Rpc.BlockDataview







<a name="anytype-Rpc-BlockDataview-CreateBookmark"></a>

### Rpc.BlockDataview.CreateBookmark







<a name="anytype-Rpc-BlockDataview-CreateBookmark-Request"></a>

### Rpc.BlockDataview.CreateBookmark.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-CreateBookmark-Response"></a>

### Rpc.BlockDataview.CreateBookmark.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.CreateBookmark.Response.Error](#anytype-Rpc-BlockDataview-CreateBookmark-Response-Error) |  |  |
| id | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-CreateBookmark-Response-Error"></a>

### Rpc.BlockDataview.CreateBookmark.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.CreateBookmark.Response.Error.Code](#anytype-Rpc-BlockDataview-CreateBookmark-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-CreateFromExistingObject"></a>

### Rpc.BlockDataview.CreateFromExistingObject







<a name="anytype-Rpc-BlockDataview-CreateFromExistingObject-Request"></a>

### Rpc.BlockDataview.CreateFromExistingObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| targetObjectId | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-CreateFromExistingObject-Response"></a>

### Rpc.BlockDataview.CreateFromExistingObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.CreateFromExistingObject.Response.Error](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| targetObjectId | [string](#string) |  |  |
| view | [model.Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) | repeated |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-CreateFromExistingObject-Response-Error"></a>

### Rpc.BlockDataview.CreateFromExistingObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.CreateFromExistingObject.Response.Error.Code](#anytype-Rpc-BlockDataview-CreateFromExistingObject-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter"></a>

### Rpc.BlockDataview.Filter







<a name="anytype-Rpc-BlockDataview-Filter-Add"></a>

### Rpc.BlockDataview.Filter.Add







<a name="anytype-Rpc-BlockDataview-Filter-Add-Request"></a>

### Rpc.BlockDataview.Filter.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| filter | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Add-Response"></a>

### Rpc.BlockDataview.Filter.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Filter.Add.Response.Error](#anytype-Rpc-BlockDataview-Filter-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Add-Response-Error"></a>

### Rpc.BlockDataview.Filter.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Filter.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Remove"></a>

### Rpc.BlockDataview.Filter.Remove







<a name="anytype-Rpc-BlockDataview-Filter-Remove-Request"></a>

### Rpc.BlockDataview.Filter.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| ids | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-Filter-Remove-Response"></a>

### Rpc.BlockDataview.Filter.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Filter.Remove.Response.Error](#anytype-Rpc-BlockDataview-Filter-Remove-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Remove-Response-Error"></a>

### Rpc.BlockDataview.Filter.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Filter.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Remove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Replace"></a>

### Rpc.BlockDataview.Filter.Replace







<a name="anytype-Rpc-BlockDataview-Filter-Replace-Request"></a>

### Rpc.BlockDataview.Filter.Replace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| id | [string](#string) |  |  |
| filter | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Replace-Response"></a>

### Rpc.BlockDataview.Filter.Replace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Filter.Replace.Response.Error](#anytype-Rpc-BlockDataview-Filter-Replace-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Replace-Response-Error"></a>

### Rpc.BlockDataview.Filter.Replace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Filter.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Replace-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Sort"></a>

### Rpc.BlockDataview.Filter.Sort







<a name="anytype-Rpc-BlockDataview-Filter-Sort-Request"></a>

### Rpc.BlockDataview.Filter.Sort.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| ids | [string](#string) | repeated | new order of filters |






<a name="anytype-Rpc-BlockDataview-Filter-Sort-Response"></a>

### Rpc.BlockDataview.Filter.Sort.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Filter.Sort.Response.Error](#anytype-Rpc-BlockDataview-Filter-Sort-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Filter-Sort-Response-Error"></a>

### Rpc.BlockDataview.Filter.Sort.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Filter.Sort.Response.Error.Code](#anytype-Rpc-BlockDataview-Filter-Sort-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-GroupOrder"></a>

### Rpc.BlockDataview.GroupOrder







<a name="anytype-Rpc-BlockDataview-GroupOrder-Update"></a>

### Rpc.BlockDataview.GroupOrder.Update







<a name="anytype-Rpc-BlockDataview-GroupOrder-Update-Request"></a>

### Rpc.BlockDataview.GroupOrder.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| groupOrder | [model.Block.Content.Dataview.GroupOrder](#anytype-model-Block-Content-Dataview-GroupOrder) |  |  |






<a name="anytype-Rpc-BlockDataview-GroupOrder-Update-Response"></a>

### Rpc.BlockDataview.GroupOrder.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.GroupOrder.Update.Response.Error](#anytype-Rpc-BlockDataview-GroupOrder-Update-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-GroupOrder-Update-Response-Error"></a>

### Rpc.BlockDataview.GroupOrder.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.GroupOrder.Update.Response.Error.Code](#anytype-Rpc-BlockDataview-GroupOrder-Update-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-ObjectOrder"></a>

### Rpc.BlockDataview.ObjectOrder







<a name="anytype-Rpc-BlockDataview-ObjectOrder-Move"></a>

### Rpc.BlockDataview.ObjectOrder.Move







<a name="anytype-Rpc-BlockDataview-ObjectOrder-Move-Request"></a>

### Rpc.BlockDataview.ObjectOrder.Move.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| viewId | [string](#string) |  |  |
| groupId | [string](#string) |  |  |
| afterId | [string](#string) |  |  |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-ObjectOrder-Move-Response"></a>

### Rpc.BlockDataview.ObjectOrder.Move.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.ObjectOrder.Move.Response.Error](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-ObjectOrder-Move-Response-Error"></a>

### Rpc.BlockDataview.ObjectOrder.Move.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.ObjectOrder.Move.Response.Error.Code](#anytype-Rpc-BlockDataview-ObjectOrder-Move-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-ObjectOrder-Update"></a>

### Rpc.BlockDataview.ObjectOrder.Update







<a name="anytype-Rpc-BlockDataview-ObjectOrder-Update-Request"></a>

### Rpc.BlockDataview.ObjectOrder.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| objectOrders | [model.Block.Content.Dataview.ObjectOrder](#anytype-model-Block-Content-Dataview-ObjectOrder) | repeated |  |






<a name="anytype-Rpc-BlockDataview-ObjectOrder-Update-Response"></a>

### Rpc.BlockDataview.ObjectOrder.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.ObjectOrder.Update.Response.Error](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-ObjectOrder-Update-Response-Error"></a>

### Rpc.BlockDataview.ObjectOrder.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.ObjectOrder.Update.Response.Error.Code](#anytype-Rpc-BlockDataview-ObjectOrder-Update-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation"></a>

### Rpc.BlockDataview.Relation







<a name="anytype-Rpc-BlockDataview-Relation-Add"></a>

### Rpc.BlockDataview.Relation.Add







<a name="anytype-Rpc-BlockDataview-Relation-Add-Request"></a>

### Rpc.BlockDataview.Relation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-Relation-Add-Response"></a>

### Rpc.BlockDataview.Relation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.Add.Response.Error](#anytype-Rpc-BlockDataview-Relation-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation-Add-Response-Error"></a>

### Rpc.BlockDataview.Relation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation-Delete"></a>

### Rpc.BlockDataview.Relation.Delete







<a name="anytype-Rpc-BlockDataview-Relation-Delete-Request"></a>

### Rpc.BlockDataview.Relation.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-Relation-Delete-Response"></a>

### Rpc.BlockDataview.Relation.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.Delete.Response.Error](#anytype-Rpc-BlockDataview-Relation-Delete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation-Delete-Response-Error"></a>

### Rpc.BlockDataview.Relation.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.Delete.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-Delete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation-ListAvailable"></a>

### Rpc.BlockDataview.Relation.ListAvailable







<a name="anytype-Rpc-BlockDataview-Relation-ListAvailable-Request"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation-ListAvailable-Response"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.ListAvailable.Response.Error](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Response-Error) |  |  |
| relations | [model.Relation](#anytype-model-Relation) | repeated |  |






<a name="anytype-Rpc-BlockDataview-Relation-ListAvailable-Response-Error"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-ListAvailable-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-SetSource"></a>

### Rpc.BlockDataview.SetSource







<a name="anytype-Rpc-BlockDataview-SetSource-Request"></a>

### Rpc.BlockDataview.SetSource.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-SetSource-Response"></a>

### Rpc.BlockDataview.SetSource.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.SetSource.Response.Error](#anytype-Rpc-BlockDataview-SetSource-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-SetSource-Response-Error"></a>

### Rpc.BlockDataview.SetSource.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.SetSource.Response.Error.Code](#anytype-Rpc-BlockDataview-SetSource-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort"></a>

### Rpc.BlockDataview.Sort







<a name="anytype-Rpc-BlockDataview-Sort-Add"></a>

### Rpc.BlockDataview.Sort.Add







<a name="anytype-Rpc-BlockDataview-Sort-Add-Request"></a>

### Rpc.BlockDataview.Sort.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| sort | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Add-Response"></a>

### Rpc.BlockDataview.Sort.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Sort.Add.Response.Error](#anytype-Rpc-BlockDataview-Sort-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Add-Response-Error"></a>

### Rpc.BlockDataview.Sort.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Sort.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Remove"></a>

### Rpc.BlockDataview.Sort.Remove







<a name="anytype-Rpc-BlockDataview-Sort-Remove-Request"></a>

### Rpc.BlockDataview.Sort.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| ids | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-Sort-Remove-Response"></a>

### Rpc.BlockDataview.Sort.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Sort.Remove.Response.Error](#anytype-Rpc-BlockDataview-Sort-Remove-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Remove-Response-Error"></a>

### Rpc.BlockDataview.Sort.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Sort.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Remove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Replace"></a>

### Rpc.BlockDataview.Sort.Replace







<a name="anytype-Rpc-BlockDataview-Sort-Replace-Request"></a>

### Rpc.BlockDataview.Sort.Replace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| id | [string](#string) |  |  |
| sort | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Replace-Response"></a>

### Rpc.BlockDataview.Sort.Replace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Sort.Replace.Response.Error](#anytype-Rpc-BlockDataview-Sort-Replace-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Replace-Response-Error"></a>

### Rpc.BlockDataview.Sort.Replace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Sort.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Replace-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Sort"></a>

### Rpc.BlockDataview.Sort.Sort







<a name="anytype-Rpc-BlockDataview-Sort-Sort-Request"></a>

### Rpc.BlockDataview.Sort.Sort.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| ids | [string](#string) | repeated | new order of sorts |






<a name="anytype-Rpc-BlockDataview-Sort-Sort-Response"></a>

### Rpc.BlockDataview.Sort.Sort.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Sort.Sort.Response.Error](#anytype-Rpc-BlockDataview-Sort-Sort-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-Sort-Response-Error"></a>

### Rpc.BlockDataview.Sort.Sort.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Sort.Sort.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Sort-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-View"></a>

### Rpc.BlockDataview.View







<a name="anytype-Rpc-BlockDataview-View-Create"></a>

### Rpc.BlockDataview.View.Create







<a name="anytype-Rpc-BlockDataview-View-Create-Request"></a>

### Rpc.BlockDataview.View.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to insert the new block |
| view | [model.Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-View-Create-Response"></a>

### Rpc.BlockDataview.View.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.Create.Response.Error](#anytype-Rpc-BlockDataview-View-Create-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| viewId | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-View-Create-Response-Error"></a>

### Rpc.BlockDataview.View.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.Create.Response.Error.Code](#anytype-Rpc-BlockDataview-View-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-View-Delete"></a>

### Rpc.BlockDataview.View.Delete







<a name="anytype-Rpc-BlockDataview-View-Delete-Request"></a>

### Rpc.BlockDataview.View.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockId | [string](#string) |  | id of the dataview |
| viewId | [string](#string) |  | id of the view to remove |






<a name="anytype-Rpc-BlockDataview-View-Delete-Response"></a>

### Rpc.BlockDataview.View.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.Delete.Response.Error](#anytype-Rpc-BlockDataview-View-Delete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-View-Delete-Response-Error"></a>

### Rpc.BlockDataview.View.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.Delete.Response.Error.Code](#anytype-Rpc-BlockDataview-View-Delete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-View-SetActive"></a>

### Rpc.BlockDataview.View.SetActive
set the current active view (persisted only within a session)






<a name="anytype-Rpc-BlockDataview-View-SetActive-Request"></a>

### Rpc.BlockDataview.View.SetActive.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block |
| viewId | [string](#string) |  | id of active view |
| offset | [uint32](#uint32) |  |  |
| limit | [uint32](#uint32) |  |  |






<a name="anytype-Rpc-BlockDataview-View-SetActive-Response"></a>

### Rpc.BlockDataview.View.SetActive.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.SetActive.Response.Error](#anytype-Rpc-BlockDataview-View-SetActive-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-View-SetActive-Response-Error"></a>

### Rpc.BlockDataview.View.SetActive.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.SetActive.Response.Error.Code](#anytype-Rpc-BlockDataview-View-SetActive-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-View-SetPosition"></a>

### Rpc.BlockDataview.View.SetPosition







<a name="anytype-Rpc-BlockDataview-View-SetPosition-Request"></a>

### Rpc.BlockDataview.View.SetPosition.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockId | [string](#string) |  | id of the dataview |
| viewId | [string](#string) |  | id of the view to remove |
| position | [uint32](#uint32) |  | index of view position (0 - means first) |






<a name="anytype-Rpc-BlockDataview-View-SetPosition-Response"></a>

### Rpc.BlockDataview.View.SetPosition.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.SetPosition.Response.Error](#anytype-Rpc-BlockDataview-View-SetPosition-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-View-SetPosition-Response-Error"></a>

### Rpc.BlockDataview.View.SetPosition.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.SetPosition.Response.Error.Code](#anytype-Rpc-BlockDataview-View-SetPosition-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-View-Update"></a>

### Rpc.BlockDataview.View.Update







<a name="anytype-Rpc-BlockDataview-View-Update-Request"></a>

### Rpc.BlockDataview.View.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| view | [model.Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) |  |  |






<a name="anytype-Rpc-BlockDataview-View-Update-Response"></a>

### Rpc.BlockDataview.View.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.Update.Response.Error](#anytype-Rpc-BlockDataview-View-Update-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-View-Update-Response-Error"></a>

### Rpc.BlockDataview.View.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.Update.Response.Error.Code](#anytype-Rpc-BlockDataview-View-Update-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation"></a>

### Rpc.BlockDataview.ViewRelation







<a name="anytype-Rpc-BlockDataview-ViewRelation-Add"></a>

### Rpc.BlockDataview.ViewRelation.Add







<a name="anytype-Rpc-BlockDataview-ViewRelation-Add-Request"></a>

### Rpc.BlockDataview.ViewRelation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| relation | [model.Block.Content.Dataview.Relation](#anytype-model-Block-Content-Dataview-Relation) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Add-Response"></a>

### Rpc.BlockDataview.ViewRelation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.ViewRelation.Add.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Add-Response-Error"></a>

### Rpc.BlockDataview.ViewRelation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.ViewRelation.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Remove"></a>

### Rpc.BlockDataview.ViewRelation.Remove







<a name="anytype-Rpc-BlockDataview-ViewRelation-Remove-Request"></a>

### Rpc.BlockDataview.ViewRelation.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Remove-Response"></a>

### Rpc.BlockDataview.ViewRelation.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.ViewRelation.Remove.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Remove-Response-Error"></a>

### Rpc.BlockDataview.ViewRelation.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.ViewRelation.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Remove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Replace"></a>

### Rpc.BlockDataview.ViewRelation.Replace







<a name="anytype-Rpc-BlockDataview-ViewRelation-Replace-Request"></a>

### Rpc.BlockDataview.ViewRelation.Replace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| relationKey | [string](#string) |  |  |
| relation | [model.Block.Content.Dataview.Relation](#anytype-model-Block-Content-Dataview-Relation) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Replace-Response"></a>

### Rpc.BlockDataview.ViewRelation.Replace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.ViewRelation.Replace.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Replace-Response-Error"></a>

### Rpc.BlockDataview.ViewRelation.Replace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.ViewRelation.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Replace-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Sort"></a>

### Rpc.BlockDataview.ViewRelation.Sort







<a name="anytype-Rpc-BlockDataview-ViewRelation-Sort-Request"></a>

### Rpc.BlockDataview.ViewRelation.Sort.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| relationKeys | [string](#string) | repeated | new order of relations |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Sort-Response"></a>

### Rpc.BlockDataview.ViewRelation.Sort.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.ViewRelation.Sort.Response.Error](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-ViewRelation-Sort-Response-Error"></a>

### Rpc.BlockDataview.ViewRelation.Sort.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.ViewRelation.Sort.Response.Error.Code](#anytype-Rpc-BlockDataview-ViewRelation-Sort-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockDiv"></a>

### Rpc.BlockDiv







<a name="anytype-Rpc-BlockDiv-ListSetStyle"></a>

### Rpc.BlockDiv.ListSetStyle







<a name="anytype-Rpc-BlockDiv-ListSetStyle-Request"></a>

### Rpc.BlockDiv.ListSetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style) |  |  |






<a name="anytype-Rpc-BlockDiv-ListSetStyle-Response"></a>

### Rpc.BlockDiv.ListSetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDiv.ListSetStyle.Response.Error](#anytype-Rpc-BlockDiv-ListSetStyle-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDiv-ListSetStyle-Response-Error"></a>

### Rpc.BlockDiv.ListSetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDiv.ListSetStyle.Response.Error.Code](#anytype-Rpc-BlockDiv-ListSetStyle-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockFile"></a>

### Rpc.BlockFile







<a name="anytype-Rpc-BlockFile-CreateAndUpload"></a>

### Rpc.BlockFile.CreateAndUpload







<a name="anytype-Rpc-BlockFile-CreateAndUpload-Request"></a>

### Rpc.BlockFile.CreateAndUpload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| fileType | [model.Block.Content.File.Type](#anytype-model-Block-Content-File-Type) |  |  |






<a name="anytype-Rpc-BlockFile-CreateAndUpload-Response"></a>

### Rpc.BlockFile.CreateAndUpload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.CreateAndUpload.Response.Error](#anytype-Rpc-BlockFile-CreateAndUpload-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockFile-CreateAndUpload-Response-Error"></a>

### Rpc.BlockFile.CreateAndUpload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.CreateAndUpload.Response.Error.Code](#anytype-Rpc-BlockFile-CreateAndUpload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockFile-ListSetStyle"></a>

### Rpc.BlockFile.ListSetStyle







<a name="anytype-Rpc-BlockFile-ListSetStyle-Request"></a>

### Rpc.BlockFile.ListSetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |






<a name="anytype-Rpc-BlockFile-ListSetStyle-Response"></a>

### Rpc.BlockFile.ListSetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.ListSetStyle.Response.Error](#anytype-Rpc-BlockFile-ListSetStyle-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockFile-ListSetStyle-Response-Error"></a>

### Rpc.BlockFile.ListSetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.ListSetStyle.Response.Error.Code](#anytype-Rpc-BlockFile-ListSetStyle-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockFile-SetName"></a>

### Rpc.BlockFile.SetName







<a name="anytype-Rpc-BlockFile-SetName-Request"></a>

### Rpc.BlockFile.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype-Rpc-BlockFile-SetName-Response"></a>

### Rpc.BlockFile.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.SetName.Response.Error](#anytype-Rpc-BlockFile-SetName-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockFile-SetName-Response-Error"></a>

### Rpc.BlockFile.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.SetName.Response.Error.Code](#anytype-Rpc-BlockFile-SetName-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockImage"></a>

### Rpc.BlockImage







<a name="anytype-Rpc-BlockImage-SetName"></a>

### Rpc.BlockImage.SetName







<a name="anytype-Rpc-BlockImage-SetName-Request"></a>

### Rpc.BlockImage.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype-Rpc-BlockImage-SetName-Response"></a>

### Rpc.BlockImage.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockImage.SetName.Response.Error](#anytype-Rpc-BlockImage-SetName-Response-Error) |  |  |






<a name="anytype-Rpc-BlockImage-SetName-Response-Error"></a>

### Rpc.BlockImage.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockImage.SetName.Response.Error.Code](#anytype-Rpc-BlockImage-SetName-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockImage-SetWidth"></a>

### Rpc.BlockImage.SetWidth







<a name="anytype-Rpc-BlockImage-SetWidth-Request"></a>

### Rpc.BlockImage.SetWidth.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype-Rpc-BlockImage-SetWidth-Response"></a>

### Rpc.BlockImage.SetWidth.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockImage.SetWidth.Response.Error](#anytype-Rpc-BlockImage-SetWidth-Response-Error) |  |  |






<a name="anytype-Rpc-BlockImage-SetWidth-Response-Error"></a>

### Rpc.BlockImage.SetWidth.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockImage.SetWidth.Response.Error.Code](#anytype-Rpc-BlockImage-SetWidth-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockLatex"></a>

### Rpc.BlockLatex







<a name="anytype-Rpc-BlockLatex-SetText"></a>

### Rpc.BlockLatex.SetText







<a name="anytype-Rpc-BlockLatex-SetText-Request"></a>

### Rpc.BlockLatex.SetText.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |






<a name="anytype-Rpc-BlockLatex-SetText-Response"></a>

### Rpc.BlockLatex.SetText.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLatex.SetText.Response.Error](#anytype-Rpc-BlockLatex-SetText-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockLatex-SetText-Response-Error"></a>

### Rpc.BlockLatex.SetText.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLatex.SetText.Response.Error.Code](#anytype-Rpc-BlockLatex-SetText-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockLink"></a>

### Rpc.BlockLink







<a name="anytype-Rpc-BlockLink-CreateWithObject"></a>

### Rpc.BlockLink.CreateWithObject







<a name="anytype-Rpc-BlockLink-CreateWithObject-Request"></a>

### Rpc.BlockLink.CreateWithObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | new object details |
| templateId | [string](#string) |  | optional template id for creating from template |
| internalFlags | [model.InternalFlag](#anytype-model-InternalFlag) | repeated |  |
| targetId | [string](#string) |  | link block params

id of the closest simple block |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  | link block fields |






<a name="anytype-Rpc-BlockLink-CreateWithObject-Response"></a>

### Rpc.BlockLink.CreateWithObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLink.CreateWithObject.Response.Error](#anytype-Rpc-BlockLink-CreateWithObject-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockLink-CreateWithObject-Response-Error"></a>

### Rpc.BlockLink.CreateWithObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLink.CreateWithObject.Response.Error.Code](#anytype-Rpc-BlockLink-CreateWithObject-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockLink-ListSetAppearance"></a>

### Rpc.BlockLink.ListSetAppearance







<a name="anytype-Rpc-BlockLink-ListSetAppearance-Request"></a>

### Rpc.BlockLink.ListSetAppearance.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| iconSize | [model.Block.Content.Link.IconSize](#anytype-model-Block-Content-Link-IconSize) |  |  |
| cardStyle | [model.Block.Content.Link.CardStyle](#anytype-model-Block-Content-Link-CardStyle) |  |  |
| description | [model.Block.Content.Link.Description](#anytype-model-Block-Content-Link-Description) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockLink-ListSetAppearance-Response"></a>

### Rpc.BlockLink.ListSetAppearance.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLink.ListSetAppearance.Response.Error](#anytype-Rpc-BlockLink-ListSetAppearance-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockLink-ListSetAppearance-Response-Error"></a>

### Rpc.BlockLink.ListSetAppearance.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLink.ListSetAppearance.Response.Error.Code](#anytype-Rpc-BlockLink-ListSetAppearance-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockRelation"></a>

### Rpc.BlockRelation







<a name="anytype-Rpc-BlockRelation-Add"></a>

### Rpc.BlockRelation.Add







<a name="anytype-Rpc-BlockRelation-Add-Request"></a>

### Rpc.BlockRelation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| relationKey | [string](#string) |  |  |






<a name="anytype-Rpc-BlockRelation-Add-Response"></a>

### Rpc.BlockRelation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockRelation.Add.Response.Error](#anytype-Rpc-BlockRelation-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockRelation-Add-Response-Error"></a>

### Rpc.BlockRelation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockRelation.Add.Response.Error.Code](#anytype-Rpc-BlockRelation-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockRelation-SetKey"></a>

### Rpc.BlockRelation.SetKey







<a name="anytype-Rpc-BlockRelation-SetKey-Request"></a>

### Rpc.BlockRelation.SetKey.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| key | [string](#string) |  |  |






<a name="anytype-Rpc-BlockRelation-SetKey-Response"></a>

### Rpc.BlockRelation.SetKey.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockRelation.SetKey.Response.Error](#anytype-Rpc-BlockRelation-SetKey-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockRelation-SetKey-Response-Error"></a>

### Rpc.BlockRelation.SetKey.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockRelation.SetKey.Response.Error.Code](#anytype-Rpc-BlockRelation-SetKey-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable"></a>

### Rpc.BlockTable







<a name="anytype-Rpc-BlockTable-ColumnCreate"></a>

### Rpc.BlockTable.ColumnCreate







<a name="anytype-Rpc-BlockTable-ColumnCreate-Request"></a>

### Rpc.BlockTable.ColumnCreate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest column |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnCreate-Response"></a>

### Rpc.BlockTable.ColumnCreate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.ColumnCreate.Response.Error](#anytype-Rpc-BlockTable-ColumnCreate-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnCreate-Response-Error"></a>

### Rpc.BlockTable.ColumnCreate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.ColumnCreate.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnCreate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnDelete"></a>

### Rpc.BlockTable.ColumnDelete







<a name="anytype-Rpc-BlockTable-ColumnDelete-Request"></a>

### Rpc.BlockTable.ColumnDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest column |






<a name="anytype-Rpc-BlockTable-ColumnDelete-Response"></a>

### Rpc.BlockTable.ColumnDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.ColumnDelete.Response.Error](#anytype-Rpc-BlockTable-ColumnDelete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnDelete-Response-Error"></a>

### Rpc.BlockTable.ColumnDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.ColumnDelete.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnDelete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnDuplicate"></a>

### Rpc.BlockTable.ColumnDuplicate







<a name="anytype-Rpc-BlockTable-ColumnDuplicate-Request"></a>

### Rpc.BlockTable.ColumnDuplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  |  |
| blockId | [string](#string) |  | block to duplicate |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnDuplicate-Response"></a>

### Rpc.BlockTable.ColumnDuplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.ColumnDuplicate.Response.Error](#anytype-Rpc-BlockTable-ColumnDuplicate-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnDuplicate-Response-Error"></a>

### Rpc.BlockTable.ColumnDuplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.ColumnDuplicate.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnDuplicate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnListFill"></a>

### Rpc.BlockTable.ColumnListFill







<a name="anytype-Rpc-BlockTable-ColumnListFill-Request"></a>

### Rpc.BlockTable.ColumnListFill.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockTable-ColumnListFill-Response"></a>

### Rpc.BlockTable.ColumnListFill.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.ColumnListFill.Response.Error](#anytype-Rpc-BlockTable-ColumnListFill-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnListFill-Response-Error"></a>

### Rpc.BlockTable.ColumnListFill.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.ColumnListFill.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnListFill-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnMove"></a>

### Rpc.BlockTable.ColumnMove







<a name="anytype-Rpc-BlockTable-ColumnMove-Request"></a>

### Rpc.BlockTable.ColumnMove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnMove-Response"></a>

### Rpc.BlockTable.ColumnMove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.ColumnMove.Response.Error](#anytype-Rpc-BlockTable-ColumnMove-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-ColumnMove-Response-Error"></a>

### Rpc.BlockTable.ColumnMove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.ColumnMove.Response.Error.Code](#anytype-Rpc-BlockTable-ColumnMove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-Create"></a>

### Rpc.BlockTable.Create







<a name="anytype-Rpc-BlockTable-Create-Request"></a>

### Rpc.BlockTable.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest block |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| rows | [uint32](#uint32) |  |  |
| columns | [uint32](#uint32) |  |  |
| withHeaderRow | [bool](#bool) |  |  |






<a name="anytype-Rpc-BlockTable-Create-Response"></a>

### Rpc.BlockTable.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.Create.Response.Error](#anytype-Rpc-BlockTable-Create-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-Create-Response-Error"></a>

### Rpc.BlockTable.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.Create.Response.Error.Code](#anytype-Rpc-BlockTable-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-Expand"></a>

### Rpc.BlockTable.Expand







<a name="anytype-Rpc-BlockTable-Expand-Request"></a>

### Rpc.BlockTable.Expand.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  |  |
| columns | [uint32](#uint32) |  | number of columns to append |
| rows | [uint32](#uint32) |  | number of rows to append |






<a name="anytype-Rpc-BlockTable-Expand-Response"></a>

### Rpc.BlockTable.Expand.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.Expand.Response.Error](#anytype-Rpc-BlockTable-Expand-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-Expand-Response-Error"></a>

### Rpc.BlockTable.Expand.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.Expand.Response.Error.Code](#anytype-Rpc-BlockTable-Expand-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-RowCreate"></a>

### Rpc.BlockTable.RowCreate







<a name="anytype-Rpc-BlockTable-RowCreate-Request"></a>

### Rpc.BlockTable.RowCreate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest row |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |






<a name="anytype-Rpc-BlockTable-RowCreate-Response"></a>

### Rpc.BlockTable.RowCreate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.RowCreate.Response.Error](#anytype-Rpc-BlockTable-RowCreate-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-RowCreate-Response-Error"></a>

### Rpc.BlockTable.RowCreate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.RowCreate.Response.Error.Code](#anytype-Rpc-BlockTable-RowCreate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-RowDelete"></a>

### Rpc.BlockTable.RowDelete







<a name="anytype-Rpc-BlockTable-RowDelete-Request"></a>

### Rpc.BlockTable.RowDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  | id of the closest row |






<a name="anytype-Rpc-BlockTable-RowDelete-Response"></a>

### Rpc.BlockTable.RowDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.RowDelete.Response.Error](#anytype-Rpc-BlockTable-RowDelete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-RowDelete-Response-Error"></a>

### Rpc.BlockTable.RowDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.RowDelete.Response.Error.Code](#anytype-Rpc-BlockTable-RowDelete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-RowDuplicate"></a>

### Rpc.BlockTable.RowDuplicate







<a name="anytype-Rpc-BlockTable-RowDuplicate-Request"></a>

### Rpc.BlockTable.RowDuplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  |  |
| blockId | [string](#string) |  | block to duplicate |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |






<a name="anytype-Rpc-BlockTable-RowDuplicate-Response"></a>

### Rpc.BlockTable.RowDuplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.RowDuplicate.Response.Error](#anytype-Rpc-BlockTable-RowDuplicate-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-RowDuplicate-Response-Error"></a>

### Rpc.BlockTable.RowDuplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.RowDuplicate.Response.Error.Code](#anytype-Rpc-BlockTable-RowDuplicate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-RowListClean"></a>

### Rpc.BlockTable.RowListClean







<a name="anytype-Rpc-BlockTable-RowListClean-Request"></a>

### Rpc.BlockTable.RowListClean.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockTable-RowListClean-Response"></a>

### Rpc.BlockTable.RowListClean.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.RowListClean.Response.Error](#anytype-Rpc-BlockTable-RowListClean-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-RowListClean-Response-Error"></a>

### Rpc.BlockTable.RowListClean.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.RowListClean.Response.Error.Code](#anytype-Rpc-BlockTable-RowListClean-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-RowListFill"></a>

### Rpc.BlockTable.RowListFill







<a name="anytype-Rpc-BlockTable-RowListFill-Request"></a>

### Rpc.BlockTable.RowListFill.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| blockIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockTable-RowListFill-Response"></a>

### Rpc.BlockTable.RowListFill.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.RowListFill.Response.Error](#anytype-Rpc-BlockTable-RowListFill-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-RowListFill-Response-Error"></a>

### Rpc.BlockTable.RowListFill.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.RowListFill.Response.Error.Code](#anytype-Rpc-BlockTable-RowListFill-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-RowSetHeader"></a>

### Rpc.BlockTable.RowSetHeader







<a name="anytype-Rpc-BlockTable-RowSetHeader-Request"></a>

### Rpc.BlockTable.RowSetHeader.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| targetId | [string](#string) |  |  |
| isHeader | [bool](#bool) |  |  |






<a name="anytype-Rpc-BlockTable-RowSetHeader-Response"></a>

### Rpc.BlockTable.RowSetHeader.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.RowSetHeader.Response.Error](#anytype-Rpc-BlockTable-RowSetHeader-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-RowSetHeader-Response-Error"></a>

### Rpc.BlockTable.RowSetHeader.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.RowSetHeader.Response.Error.Code](#anytype-Rpc-BlockTable-RowSetHeader-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockTable-Sort"></a>

### Rpc.BlockTable.Sort







<a name="anytype-Rpc-BlockTable-Sort-Request"></a>

### Rpc.BlockTable.Sort.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |
| columnId | [string](#string) |  |  |
| type | [model.Block.Content.Dataview.Sort.Type](#anytype-model-Block-Content-Dataview-Sort-Type) |  |  |






<a name="anytype-Rpc-BlockTable-Sort-Response"></a>

### Rpc.BlockTable.Sort.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockTable.Sort.Response.Error](#anytype-Rpc-BlockTable-Sort-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockTable-Sort-Response-Error"></a>

### Rpc.BlockTable.Sort.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockTable.Sort.Response.Error.Code](#anytype-Rpc-BlockTable-Sort-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText"></a>

### Rpc.BlockText







<a name="anytype-Rpc-BlockText-ListClearContent"></a>

### Rpc.BlockText.ListClearContent







<a name="anytype-Rpc-BlockText-ListClearContent-Request"></a>

### Rpc.BlockText.ListClearContent.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockText-ListClearContent-Response"></a>

### Rpc.BlockText.ListClearContent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListClearContent.Response.Error](#anytype-Rpc-BlockText-ListClearContent-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-ListClearContent-Response-Error"></a>

### Rpc.BlockText.ListClearContent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListClearContent.Response.Error.Code](#anytype-Rpc-BlockText-ListClearContent-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-ListClearStyle"></a>

### Rpc.BlockText.ListClearStyle







<a name="anytype-Rpc-BlockText-ListClearStyle-Request"></a>

### Rpc.BlockText.ListClearStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockText-ListClearStyle-Response"></a>

### Rpc.BlockText.ListClearStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListClearStyle.Response.Error](#anytype-Rpc-BlockText-ListClearStyle-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-ListClearStyle-Response-Error"></a>

### Rpc.BlockText.ListClearStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListClearStyle.Response.Error.Code](#anytype-Rpc-BlockText-ListClearStyle-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-ListSetColor"></a>

### Rpc.BlockText.ListSetColor







<a name="anytype-Rpc-BlockText-ListSetColor-Request"></a>

### Rpc.BlockText.ListSetColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-ListSetColor-Response"></a>

### Rpc.BlockText.ListSetColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListSetColor.Response.Error](#anytype-Rpc-BlockText-ListSetColor-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-ListSetColor-Response-Error"></a>

### Rpc.BlockText.ListSetColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListSetColor.Response.Error.Code](#anytype-Rpc-BlockText-ListSetColor-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-ListSetMark"></a>

### Rpc.BlockText.ListSetMark







<a name="anytype-Rpc-BlockText-ListSetMark-Request"></a>

### Rpc.BlockText.ListSetMark.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| mark | [model.Block.Content.Text.Mark](#anytype-model-Block-Content-Text-Mark) |  |  |






<a name="anytype-Rpc-BlockText-ListSetMark-Response"></a>

### Rpc.BlockText.ListSetMark.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListSetMark.Response.Error](#anytype-Rpc-BlockText-ListSetMark-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-ListSetMark-Response-Error"></a>

### Rpc.BlockText.ListSetMark.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListSetMark.Response.Error.Code](#anytype-Rpc-BlockText-ListSetMark-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-ListSetStyle"></a>

### Rpc.BlockText.ListSetStyle







<a name="anytype-Rpc-BlockText-ListSetStyle-Request"></a>

### Rpc.BlockText.ListSetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |






<a name="anytype-Rpc-BlockText-ListSetStyle-Response"></a>

### Rpc.BlockText.ListSetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListSetStyle.Response.Error](#anytype-Rpc-BlockText-ListSetStyle-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-ListSetStyle-Response-Error"></a>

### Rpc.BlockText.ListSetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListSetStyle.Response.Error.Code](#anytype-Rpc-BlockText-ListSetStyle-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetChecked"></a>

### Rpc.BlockText.SetChecked







<a name="anytype-Rpc-BlockText-SetChecked-Request"></a>

### Rpc.BlockText.SetChecked.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype-Rpc-BlockText-SetChecked-Response"></a>

### Rpc.BlockText.SetChecked.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetChecked.Response.Error](#anytype-Rpc-BlockText-SetChecked-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-SetChecked-Response-Error"></a>

### Rpc.BlockText.SetChecked.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetChecked.Response.Error.Code](#anytype-Rpc-BlockText-SetChecked-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetColor"></a>

### Rpc.BlockText.SetColor







<a name="anytype-Rpc-BlockText-SetColor-Request"></a>

### Rpc.BlockText.SetColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| color | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetColor-Response"></a>

### Rpc.BlockText.SetColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetColor.Response.Error](#anytype-Rpc-BlockText-SetColor-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-SetColor-Response-Error"></a>

### Rpc.BlockText.SetColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetColor.Response.Error.Code](#anytype-Rpc-BlockText-SetColor-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetIcon"></a>

### Rpc.BlockText.SetIcon







<a name="anytype-Rpc-BlockText-SetIcon-Request"></a>

### Rpc.BlockText.SetIcon.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| iconImage | [string](#string) |  | in case both image and emoji are set, image has a priority to show |
| iconEmoji | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetIcon-Response"></a>

### Rpc.BlockText.SetIcon.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetIcon.Response.Error](#anytype-Rpc-BlockText-SetIcon-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-SetIcon-Response-Error"></a>

### Rpc.BlockText.SetIcon.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetIcon.Response.Error.Code](#anytype-Rpc-BlockText-SetIcon-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetMarks"></a>

### Rpc.BlockText.SetMarks







<a name="anytype-Rpc-BlockText-SetMarks-Get"></a>

### Rpc.BlockText.SetMarks.Get
Get marks list in the selected range in text block.






<a name="anytype-Rpc-BlockText-SetMarks-Get-Request"></a>

### Rpc.BlockText.SetMarks.Get.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype-model-Range) |  |  |






<a name="anytype-Rpc-BlockText-SetMarks-Get-Response"></a>

### Rpc.BlockText.SetMarks.Get.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetMarks.Get.Response.Error](#anytype-Rpc-BlockText-SetMarks-Get-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-SetMarks-Get-Response-Error"></a>

### Rpc.BlockText.SetMarks.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetMarks.Get.Response.Error.Code](#anytype-Rpc-BlockText-SetMarks-Get-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetStyle"></a>

### Rpc.BlockText.SetStyle







<a name="anytype-Rpc-BlockText-SetStyle-Request"></a>

### Rpc.BlockText.SetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| style | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |






<a name="anytype-Rpc-BlockText-SetStyle-Response"></a>

### Rpc.BlockText.SetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetStyle.Response.Error](#anytype-Rpc-BlockText-SetStyle-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-SetStyle-Response-Error"></a>

### Rpc.BlockText.SetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetStyle.Response.Error.Code](#anytype-Rpc-BlockText-SetStyle-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockText-SetText"></a>

### Rpc.BlockText.SetText







<a name="anytype-Rpc-BlockText-SetText-Request"></a>

### Rpc.BlockText.SetText.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |
| marks | [model.Block.Content.Text.Marks](#anytype-model-Block-Content-Text-Marks) |  |  |






<a name="anytype-Rpc-BlockText-SetText-Response"></a>

### Rpc.BlockText.SetText.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetText.Response.Error](#anytype-Rpc-BlockText-SetText-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockText-SetText-Response-Error"></a>

### Rpc.BlockText.SetText.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetText.Response.Error.Code](#anytype-Rpc-BlockText-SetText-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockVideo"></a>

### Rpc.BlockVideo







<a name="anytype-Rpc-BlockVideo-SetName"></a>

### Rpc.BlockVideo.SetName







<a name="anytype-Rpc-BlockVideo-SetName-Request"></a>

### Rpc.BlockVideo.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype-Rpc-BlockVideo-SetName-Response"></a>

### Rpc.BlockVideo.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockVideo.SetName.Response.Error](#anytype-Rpc-BlockVideo-SetName-Response-Error) |  |  |






<a name="anytype-Rpc-BlockVideo-SetName-Response-Error"></a>

### Rpc.BlockVideo.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockVideo.SetName.Response.Error.Code](#anytype-Rpc-BlockVideo-SetName-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockVideo-SetWidth"></a>

### Rpc.BlockVideo.SetWidth







<a name="anytype-Rpc-BlockVideo-SetWidth-Request"></a>

### Rpc.BlockVideo.SetWidth.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype-Rpc-BlockVideo-SetWidth-Response"></a>

### Rpc.BlockVideo.SetWidth.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockVideo.SetWidth.Response.Error](#anytype-Rpc-BlockVideo-SetWidth-Response-Error) |  |  |






<a name="anytype-Rpc-BlockVideo-SetWidth-Response-Error"></a>

### Rpc.BlockVideo.SetWidth.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockVideo.SetWidth.Response.Error.Code](#anytype-Rpc-BlockVideo-SetWidth-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockWidget"></a>

### Rpc.BlockWidget







<a name="anytype-Rpc-BlockWidget-SetLayout"></a>

### Rpc.BlockWidget.SetLayout







<a name="anytype-Rpc-BlockWidget-SetLayout-Request"></a>

### Rpc.BlockWidget.SetLayout.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| layout | [model.Block.Content.Widget.Layout](#anytype-model-Block-Content-Widget-Layout) |  |  |






<a name="anytype-Rpc-BlockWidget-SetLayout-Response"></a>

### Rpc.BlockWidget.SetLayout.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockWidget.SetLayout.Response.Error](#anytype-Rpc-BlockWidget-SetLayout-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockWidget-SetLayout-Response-Error"></a>

### Rpc.BlockWidget.SetLayout.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockWidget.SetLayout.Response.Error.Code](#anytype-Rpc-BlockWidget-SetLayout-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockWidget-SetLimit"></a>

### Rpc.BlockWidget.SetLimit







<a name="anytype-Rpc-BlockWidget-SetLimit-Request"></a>

### Rpc.BlockWidget.SetLimit.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| limit | [int32](#int32) |  |  |






<a name="anytype-Rpc-BlockWidget-SetLimit-Response"></a>

### Rpc.BlockWidget.SetLimit.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockWidget.SetLimit.Response.Error](#anytype-Rpc-BlockWidget-SetLimit-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockWidget-SetLimit-Response-Error"></a>

### Rpc.BlockWidget.SetLimit.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockWidget.SetLimit.Response.Error.Code](#anytype-Rpc-BlockWidget-SetLimit-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockWidget-SetTargetId"></a>

### Rpc.BlockWidget.SetTargetId







<a name="anytype-Rpc-BlockWidget-SetTargetId-Request"></a>

### Rpc.BlockWidget.SetTargetId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |






<a name="anytype-Rpc-BlockWidget-SetTargetId-Response"></a>

### Rpc.BlockWidget.SetTargetId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockWidget.SetTargetId.Response.Error](#anytype-Rpc-BlockWidget-SetTargetId-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockWidget-SetTargetId-Response-Error"></a>

### Rpc.BlockWidget.SetTargetId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockWidget.SetTargetId.Response.Error.Code](#anytype-Rpc-BlockWidget-SetTargetId-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug"></a>

### Rpc.Debug







<a name="anytype-Rpc-Debug-ExportLocalstore"></a>

### Rpc.Debug.ExportLocalstore







<a name="anytype-Rpc-Debug-ExportLocalstore-Request"></a>

### Rpc.Debug.ExportLocalstore.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| docIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |






<a name="anytype-Rpc-Debug-ExportLocalstore-Response"></a>

### Rpc.Debug.ExportLocalstore.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.ExportLocalstore.Response.Error](#anytype-Rpc-Debug-ExportLocalstore-Response-Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Debug-ExportLocalstore-Response-Error"></a>

### Rpc.Debug.ExportLocalstore.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.ExportLocalstore.Response.Error.Code](#anytype-Rpc-Debug-ExportLocalstore-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-Ping"></a>

### Rpc.Debug.Ping







<a name="anytype-Rpc-Debug-Ping-Request"></a>

### Rpc.Debug.Ping.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  |  |
| numberOfEventsToSend | [int32](#int32) |  |  |






<a name="anytype-Rpc-Debug-Ping-Response"></a>

### Rpc.Debug.Ping.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Ping.Response.Error](#anytype-Rpc-Debug-Ping-Response-Error) |  |  |
| index | [int32](#int32) |  |  |






<a name="anytype-Rpc-Debug-Ping-Response-Error"></a>

### Rpc.Debug.Ping.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Ping.Response.Error.Code](#anytype-Rpc-Debug-Ping-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-SpaceSummary"></a>

### Rpc.Debug.SpaceSummary







<a name="anytype-Rpc-Debug-SpaceSummary-Request"></a>

### Rpc.Debug.SpaceSummary.Request







<a name="anytype-Rpc-Debug-SpaceSummary-Response"></a>

### Rpc.Debug.SpaceSummary.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.SpaceSummary.Response.Error](#anytype-Rpc-Debug-SpaceSummary-Response-Error) |  |  |
| spaceId | [string](#string) |  |  |
| infos | [Rpc.Debug.TreeInfo](#anytype-Rpc-Debug-TreeInfo) | repeated |  |






<a name="anytype-Rpc-Debug-SpaceSummary-Response-Error"></a>

### Rpc.Debug.SpaceSummary.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.SpaceSummary.Response.Error.Code](#anytype-Rpc-Debug-SpaceSummary-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-Tree"></a>

### Rpc.Debug.Tree







<a name="anytype-Rpc-Debug-Tree-Request"></a>

### Rpc.Debug.Tree.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| treeId | [string](#string) |  |  |
| path | [string](#string) |  |  |
| unanonymized | [bool](#bool) |  | set to true to disable mocking of the actual data inside changes |
| generateSvg | [bool](#bool) |  | set to true to write both ZIP and SVG files |






<a name="anytype-Rpc-Debug-Tree-Response"></a>

### Rpc.Debug.Tree.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Tree.Response.Error](#anytype-Rpc-Debug-Tree-Response-Error) |  |  |
| filename | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-Tree-Response-Error"></a>

### Rpc.Debug.Tree.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Tree.Response.Error.Code](#anytype-Rpc-Debug-Tree-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-TreeHeads"></a>

### Rpc.Debug.TreeHeads







<a name="anytype-Rpc-Debug-TreeHeads-Request"></a>

### Rpc.Debug.TreeHeads.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| treeId | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-TreeHeads-Response"></a>

### Rpc.Debug.TreeHeads.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.TreeHeads.Response.Error](#anytype-Rpc-Debug-TreeHeads-Response-Error) |  |  |
| spaceId | [string](#string) |  |  |
| info | [Rpc.Debug.TreeInfo](#anytype-Rpc-Debug-TreeInfo) |  |  |






<a name="anytype-Rpc-Debug-TreeHeads-Response-Error"></a>

### Rpc.Debug.TreeHeads.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.TreeHeads.Response.Error.Code](#anytype-Rpc-Debug-TreeHeads-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-TreeInfo"></a>

### Rpc.Debug.TreeInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| treeId | [string](#string) |  |  |
| headIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-File"></a>

### Rpc.File







<a name="anytype-Rpc-File-Download"></a>

### Rpc.File.Download







<a name="anytype-Rpc-File-Download-Request"></a>

### Rpc.File.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| path | [string](#string) |  | path to save file. Temp directory is used if empty |






<a name="anytype-Rpc-File-Download-Response"></a>

### Rpc.File.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Download.Response.Error](#anytype-Rpc-File-Download-Response-Error) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype-Rpc-File-Download-Response-Error"></a>

### Rpc.File.Download.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Download.Response.Error.Code](#anytype-Rpc-File-Download-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-Drop"></a>

### Rpc.File.Drop







<a name="anytype-Rpc-File-Drop-Request"></a>

### Rpc.File.Drop.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype-model-Block-Position) |  | position relatively to the dropTargetId simple block |
| localFilePaths | [string](#string) | repeated |  |






<a name="anytype-Rpc-File-Drop-Response"></a>

### Rpc.File.Drop.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Drop.Response.Error](#anytype-Rpc-File-Drop-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-File-Drop-Response-Error"></a>

### Rpc.File.Drop.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Drop.Response.Error.Code](#anytype-Rpc-File-Drop-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-ListOffload"></a>

### Rpc.File.ListOffload







<a name="anytype-Rpc-File-ListOffload-Request"></a>

### Rpc.File.ListOffload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| onlyIds | [string](#string) | repeated | empty means all |
| includeNotPinned | [bool](#bool) |  | false mean not-yet-pinned files will be not |






<a name="anytype-Rpc-File-ListOffload-Response"></a>

### Rpc.File.ListOffload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.ListOffload.Response.Error](#anytype-Rpc-File-ListOffload-Response-Error) |  |  |
| filesOffloaded | [int32](#int32) |  |  |
| bytesOffloaded | [uint64](#uint64) |  |  |






<a name="anytype-Rpc-File-ListOffload-Response-Error"></a>

### Rpc.File.ListOffload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.ListOffload.Response.Error.Code](#anytype-Rpc-File-ListOffload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-Offload"></a>

### Rpc.File.Offload







<a name="anytype-Rpc-File-Offload-Request"></a>

### Rpc.File.Offload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| includeNotPinned | [bool](#bool) |  |  |






<a name="anytype-Rpc-File-Offload-Response"></a>

### Rpc.File.Offload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Offload.Response.Error](#anytype-Rpc-File-Offload-Response-Error) |  |  |
| bytesOffloaded | [uint64](#uint64) |  |  |






<a name="anytype-Rpc-File-Offload-Response-Error"></a>

### Rpc.File.Offload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Offload.Response.Error.Code](#anytype-Rpc-File-Offload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-SpaceUsage"></a>

### Rpc.File.SpaceUsage







<a name="anytype-Rpc-File-SpaceUsage-Request"></a>

### Rpc.File.SpaceUsage.Request







<a name="anytype-Rpc-File-SpaceUsage-Response"></a>

### Rpc.File.SpaceUsage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.SpaceUsage.Response.Error](#anytype-Rpc-File-SpaceUsage-Response-Error) |  |  |
| usage | [Rpc.File.SpaceUsage.Response.Usage](#anytype-Rpc-File-SpaceUsage-Response-Usage) |  |  |






<a name="anytype-Rpc-File-SpaceUsage-Response-Error"></a>

### Rpc.File.SpaceUsage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.SpaceUsage.Response.Error.Code](#anytype-Rpc-File-SpaceUsage-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-SpaceUsage-Response-Usage"></a>

### Rpc.File.SpaceUsage.Response.Usage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filesCount | [uint64](#uint64) |  |  |
| cidsCount | [uint64](#uint64) |  |  |
| bytesUsage | [uint64](#uint64) |  |  |
| bytesLeft | [uint64](#uint64) |  |  |
| bytesLimit | [uint64](#uint64) |  |  |
| localBytesUsage | [uint64](#uint64) |  |  |






<a name="anytype-Rpc-File-Upload"></a>

### Rpc.File.Upload







<a name="anytype-Rpc-File-Upload-Request"></a>

### Rpc.File.Upload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| type | [model.Block.Content.File.Type](#anytype-model-Block-Content-File-Type) |  |  |
| disableEncryption | [bool](#bool) |  | deprecated, has no affect |
| style | [model.Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |






<a name="anytype-Rpc-File-Upload-Response"></a>

### Rpc.File.Upload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Upload.Response.Error](#anytype-Rpc-File-Upload-Response-Error) |  |  |
| hash | [string](#string) |  |  |






<a name="anytype-Rpc-File-Upload-Response-Error"></a>

### Rpc.File.Upload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Upload.Response.Error.Code](#anytype-Rpc-File-Upload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-GenericErrorResponse"></a>

### Rpc.GenericErrorResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.GenericErrorResponse.Error](#anytype-Rpc-GenericErrorResponse-Error) |  |  |






<a name="anytype-Rpc-GenericErrorResponse-Error"></a>

### Rpc.GenericErrorResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.GenericErrorResponse.Error.Code](#anytype-Rpc-GenericErrorResponse-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-History"></a>

### Rpc.History







<a name="anytype-Rpc-History-GetVersions"></a>

### Rpc.History.GetVersions
returns list of versions (changes)






<a name="anytype-Rpc-History-GetVersions-Request"></a>

### Rpc.History.GetVersions.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| lastVersionId | [string](#string) |  | when indicated, results will include versions before given id |
| limit | [int32](#int32) |  | desired count of versions |






<a name="anytype-Rpc-History-GetVersions-Response"></a>

### Rpc.History.GetVersions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.GetVersions.Response.Error](#anytype-Rpc-History-GetVersions-Response-Error) |  |  |
| versions | [Rpc.History.Version](#anytype-Rpc-History-Version) | repeated |  |






<a name="anytype-Rpc-History-GetVersions-Response-Error"></a>

### Rpc.History.GetVersions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.GetVersions.Response.Error.Code](#anytype-Rpc-History-GetVersions-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-History-SetVersion"></a>

### Rpc.History.SetVersion







<a name="anytype-Rpc-History-SetVersion-Request"></a>

### Rpc.History.SetVersion.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| versionId | [string](#string) |  |  |






<a name="anytype-Rpc-History-SetVersion-Response"></a>

### Rpc.History.SetVersion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.SetVersion.Response.Error](#anytype-Rpc-History-SetVersion-Response-Error) |  |  |






<a name="anytype-Rpc-History-SetVersion-Response-Error"></a>

### Rpc.History.SetVersion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.SetVersion.Response.Error.Code](#anytype-Rpc-History-SetVersion-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-History-ShowVersion"></a>

### Rpc.History.ShowVersion
returns blockShow event for given version






<a name="anytype-Rpc-History-ShowVersion-Request"></a>

### Rpc.History.ShowVersion.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| versionId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype-Rpc-History-ShowVersion-Response"></a>

### Rpc.History.ShowVersion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.ShowVersion.Response.Error](#anytype-Rpc-History-ShowVersion-Response-Error) |  |  |
| objectView | [model.ObjectView](#anytype-model-ObjectView) |  |  |
| version | [Rpc.History.Version](#anytype-Rpc-History-Version) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype-Rpc-History-ShowVersion-Response-Error"></a>

### Rpc.History.ShowVersion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.ShowVersion.Response.Error.Code](#anytype-Rpc-History-ShowVersion-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-History-Version"></a>

### Rpc.History.Version



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| previousIds | [string](#string) | repeated |  |
| authorId | [string](#string) |  |  |
| authorName | [string](#string) |  |  |
| time | [int64](#int64) |  |  |
| groupId | [int64](#int64) |  |  |






<a name="anytype-Rpc-LinkPreview"></a>

### Rpc.LinkPreview







<a name="anytype-Rpc-LinkPreview-Request"></a>

### Rpc.LinkPreview.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-LinkPreview-Response"></a>

### Rpc.LinkPreview.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.LinkPreview.Response.Error](#anytype-Rpc-LinkPreview-Response-Error) |  |  |
| linkPreview | [model.LinkPreview](#anytype-model-LinkPreview) |  |  |






<a name="anytype-Rpc-LinkPreview-Response-Error"></a>

### Rpc.LinkPreview.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.LinkPreview.Response.Error.Code](#anytype-Rpc-LinkPreview-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Log"></a>

### Rpc.Log







<a name="anytype-Rpc-Log-Send"></a>

### Rpc.Log.Send







<a name="anytype-Rpc-Log-Send-Request"></a>

### Rpc.Log.Send.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| message | [string](#string) |  |  |
| level | [Rpc.Log.Send.Request.Level](#anytype-Rpc-Log-Send-Request-Level) |  |  |






<a name="anytype-Rpc-Log-Send-Response"></a>

### Rpc.Log.Send.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Log.Send.Response.Error](#anytype-Rpc-Log-Send-Response-Error) |  |  |






<a name="anytype-Rpc-Log-Send-Response-Error"></a>

### Rpc.Log.Send.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Log.Send.Response.Error.Code](#anytype-Rpc-Log-Send-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Metrics"></a>

### Rpc.Metrics







<a name="anytype-Rpc-Metrics-SetParameters"></a>

### Rpc.Metrics.SetParameters







<a name="anytype-Rpc-Metrics-SetParameters-Request"></a>

### Rpc.Metrics.SetParameters.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| platform | [string](#string) |  |  |






<a name="anytype-Rpc-Metrics-SetParameters-Response"></a>

### Rpc.Metrics.SetParameters.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Metrics.SetParameters.Response.Error](#anytype-Rpc-Metrics-SetParameters-Response-Error) |  |  |






<a name="anytype-Rpc-Metrics-SetParameters-Response-Error"></a>

### Rpc.Metrics.SetParameters.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Metrics.SetParameters.Response.Error.Code](#anytype-Rpc-Metrics-SetParameters-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Navigation"></a>

### Rpc.Navigation







<a name="anytype-Rpc-Navigation-GetObjectInfoWithLinks"></a>

### Rpc.Navigation.GetObjectInfoWithLinks
Get the info for page alongside with info for all inbound and outbound links from/to this page






<a name="anytype-Rpc-Navigation-GetObjectInfoWithLinks-Request"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| context | [Rpc.Navigation.Context](#anytype-Rpc-Navigation-Context) |  |  |






<a name="anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error) |  |  |
| object | [model.ObjectInfoWithLinks](#anytype-model-ObjectInfoWithLinks) |  |  |






<a name="anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Navigation-ListObjects"></a>

### Rpc.Navigation.ListObjects







<a name="anytype-Rpc-Navigation-ListObjects-Request"></a>

### Rpc.Navigation.ListObjects.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| context | [Rpc.Navigation.Context](#anytype-Rpc-Navigation-Context) |  |  |
| fullText | [string](#string) |  |  |
| limit | [int32](#int32) |  |  |
| offset | [int32](#int32) |  |  |






<a name="anytype-Rpc-Navigation-ListObjects-Response"></a>

### Rpc.Navigation.ListObjects.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Navigation.ListObjects.Response.Error](#anytype-Rpc-Navigation-ListObjects-Response-Error) |  |  |
| objects | [model.ObjectInfo](#anytype-model-ObjectInfo) | repeated |  |






<a name="anytype-Rpc-Navigation-ListObjects-Response-Error"></a>

### Rpc.Navigation.ListObjects.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype-Rpc-Navigation-ListObjects-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object"></a>

### Rpc.Object







<a name="anytype-Rpc-Object-ApplyTemplate"></a>

### Rpc.Object.ApplyTemplate







<a name="anytype-Rpc-Object-ApplyTemplate-Request"></a>

### Rpc.Object.ApplyTemplate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| templateId | [string](#string) |  | id of template |






<a name="anytype-Rpc-Object-ApplyTemplate-Response"></a>

### Rpc.Object.ApplyTemplate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ApplyTemplate.Response.Error](#anytype-Rpc-Object-ApplyTemplate-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ApplyTemplate-Response-Error"></a>

### Rpc.Object.ApplyTemplate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ApplyTemplate.Response.Error.Code](#anytype-Rpc-Object-ApplyTemplate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-BookmarkFetch"></a>

### Rpc.Object.BookmarkFetch







<a name="anytype-Rpc-Object-BookmarkFetch-Request"></a>

### Rpc.Object.BookmarkFetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-Object-BookmarkFetch-Response"></a>

### Rpc.Object.BookmarkFetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.BookmarkFetch.Response.Error](#anytype-Rpc-Object-BookmarkFetch-Response-Error) |  |  |






<a name="anytype-Rpc-Object-BookmarkFetch-Response-Error"></a>

### Rpc.Object.BookmarkFetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.BookmarkFetch.Response.Error.Code](#anytype-Rpc-Object-BookmarkFetch-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Close"></a>

### Rpc.Object.Close







<a name="anytype-Rpc-Object-Close-Request"></a>

### Rpc.Object.Close.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | deprecated |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Close-Response"></a>

### Rpc.Object.Close.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Close.Response.Error](#anytype-Rpc-Object-Close-Response-Error) |  |  |






<a name="anytype-Rpc-Object-Close-Response-Error"></a>

### Rpc.Object.Close.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Close.Response.Error.Code](#anytype-Rpc-Object-Close-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Create"></a>

### Rpc.Object.Create







<a name="anytype-Rpc-Object-Create-Request"></a>

### Rpc.Object.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | object details |
| internalFlags | [model.InternalFlag](#anytype-model-InternalFlag) | repeated |  |
| templateId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Create-Response"></a>

### Rpc.Object.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Create.Response.Error](#anytype-Rpc-Object-Create-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-Create-Response-Error"></a>

### Rpc.Object.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Create.Response.Error.Code](#anytype-Rpc-Object-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateBookmark"></a>

### Rpc.Object.CreateBookmark







<a name="anytype-Rpc-Object-CreateBookmark-Request"></a>

### Rpc.Object.CreateBookmark.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateBookmark-Response"></a>

### Rpc.Object.CreateBookmark.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateBookmark.Response.Error](#anytype-Rpc-Object-CreateBookmark-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateBookmark-Response-Error"></a>

### Rpc.Object.CreateBookmark.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateBookmark.Response.Error.Code](#anytype-Rpc-Object-CreateBookmark-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateObjectType"></a>

### Rpc.Object.CreateObjectType







<a name="anytype-Rpc-Object-CreateObjectType-Request"></a>

### Rpc.Object.CreateObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| internalFlags | [model.InternalFlag](#anytype-model-InternalFlag) | repeated |  |






<a name="anytype-Rpc-Object-CreateObjectType-Response"></a>

### Rpc.Object.CreateObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateObjectType.Response.Error](#anytype-Rpc-Object-CreateObjectType-Response-Error) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateObjectType-Response-Error"></a>

### Rpc.Object.CreateObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateObjectType.Response.Error.Code](#anytype-Rpc-Object-CreateObjectType-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateRelation"></a>

### Rpc.Object.CreateRelation







<a name="anytype-Rpc-Object-CreateRelation-Request"></a>

### Rpc.Object.CreateRelation.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateRelation-Response"></a>

### Rpc.Object.CreateRelation.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateRelation.Response.Error](#anytype-Rpc-Object-CreateRelation-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| key | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateRelation-Response-Error"></a>

### Rpc.Object.CreateRelation.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateRelation.Response.Error.Code](#anytype-Rpc-Object-CreateRelation-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateRelationOption"></a>

### Rpc.Object.CreateRelationOption







<a name="anytype-Rpc-Object-CreateRelationOption-Request"></a>

### Rpc.Object.CreateRelationOption.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateRelationOption-Response"></a>

### Rpc.Object.CreateRelationOption.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateRelationOption.Response.Error](#anytype-Rpc-Object-CreateRelationOption-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateRelationOption-Response-Error"></a>

### Rpc.Object.CreateRelationOption.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateRelationOption.Response.Error.Code](#anytype-Rpc-Object-CreateRelationOption-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateSet"></a>

### Rpc.Object.CreateSet







<a name="anytype-Rpc-Object-CreateSet-Request"></a>

### Rpc.Object.CreateSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | if omitted the name of page will be the same with object type |
| templateId | [string](#string) |  | optional template id for creating from template |
| internalFlags | [model.InternalFlag](#anytype-model-InternalFlag) | repeated |  |






<a name="anytype-Rpc-Object-CreateSet-Response"></a>

### Rpc.Object.CreateSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateSet.Response.Error](#anytype-Rpc-Object-CreateSet-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-CreateSet-Response-Error"></a>

### Rpc.Object.CreateSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateSet.Response.Error.Code](#anytype-Rpc-Object-CreateSet-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Duplicate"></a>

### Rpc.Object.Duplicate







<a name="anytype-Rpc-Object-Duplicate-Request"></a>

### Rpc.Object.Duplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Duplicate-Response"></a>

### Rpc.Object.Duplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Duplicate.Response.Error](#anytype-Rpc-Object-Duplicate-Response-Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype-Rpc-Object-Duplicate-Response-Error"></a>

### Rpc.Object.Duplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Duplicate.Response.Error.Code](#anytype-Rpc-Object-Duplicate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Graph"></a>

### Rpc.Object.Graph







<a name="anytype-Rpc-Object-Graph-Edge"></a>

### Rpc.Object.Graph.Edge



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) |  |  |
| target | [string](#string) |  |  |
| name | [string](#string) |  |  |
| type | [Rpc.Object.Graph.Edge.Type](#anytype-Rpc-Object-Graph-Edge-Type) |  |  |
| description | [string](#string) |  |  |
| iconImage | [string](#string) |  |  |
| iconEmoji | [string](#string) |  |  |
| hidden | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-Graph-Request"></a>

### Rpc.Object.Graph.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |
| limit | [int32](#int32) |  |  |
| objectTypeFilter | [string](#string) | repeated | additional filter by objectTypes

DEPRECATED |
| keys | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-Graph-Response"></a>

### Rpc.Object.Graph.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Graph.Response.Error](#anytype-Rpc-Object-Graph-Response-Error) |  |  |
| nodes | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| edges | [Rpc.Object.Graph.Edge](#anytype-Rpc-Object-Graph-Edge) | repeated |  |






<a name="anytype-Rpc-Object-Graph-Response-Error"></a>

### Rpc.Object.Graph.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Graph.Response.Error.Code](#anytype-Rpc-Object-Graph-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-GroupsSubscribe"></a>

### Rpc.Object.GroupsSubscribe







<a name="anytype-Rpc-Object-GroupsSubscribe-Request"></a>

### Rpc.Object.GroupsSubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  |  |
| relationKey | [string](#string) |  |  |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |
| source | [string](#string) | repeated |  |
| collectionId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-GroupsSubscribe-Response"></a>

### Rpc.Object.GroupsSubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.GroupsSubscribe.Response.Error](#anytype-Rpc-Object-GroupsSubscribe-Response-Error) |  |  |
| groups | [model.Block.Content.Dataview.Group](#anytype-model-Block-Content-Dataview-Group) | repeated |  |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-GroupsSubscribe-Response-Error"></a>

### Rpc.Object.GroupsSubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.GroupsSubscribe.Response.Error.Code](#anytype-Rpc-Object-GroupsSubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Import"></a>

### Rpc.Object.Import







<a name="anytype-Rpc-Object-Import-Notion"></a>

### Rpc.Object.Import.Notion







<a name="anytype-Rpc-Object-Import-Notion-ValidateToken"></a>

### Rpc.Object.Import.Notion.ValidateToken







<a name="anytype-Rpc-Object-Import-Notion-ValidateToken-Request"></a>

### Rpc.Object.Import.Notion.ValidateToken.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Import-Notion-ValidateToken-Response"></a>

### Rpc.Object.Import.Notion.ValidateToken.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Import.Notion.ValidateToken.Response.Error](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error) |  |  |






<a name="anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error"></a>

### Rpc.Object.Import.Notion.ValidateToken.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Import.Notion.ValidateToken.Response.Error.Code](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Import-Request"></a>

### Rpc.Object.Import.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| notionParams | [Rpc.Object.Import.Request.NotionParams](#anytype-Rpc-Object-Import-Request-NotionParams) |  |  |
| bookmarksParams | [Rpc.Object.Import.Request.BookmarksParams](#anytype-Rpc-Object-Import-Request-BookmarksParams) |  | for internal use |
| markdownParams | [Rpc.Object.Import.Request.MarkdownParams](#anytype-Rpc-Object-Import-Request-MarkdownParams) |  |  |
| htmlParams | [Rpc.Object.Import.Request.HtmlParams](#anytype-Rpc-Object-Import-Request-HtmlParams) |  |  |
| txtParams | [Rpc.Object.Import.Request.TxtParams](#anytype-Rpc-Object-Import-Request-TxtParams) |  |  |
| pbParams | [Rpc.Object.Import.Request.PbParams](#anytype-Rpc-Object-Import-Request-PbParams) |  |  |
| csvParams | [Rpc.Object.Import.Request.CsvParams](#anytype-Rpc-Object-Import-Request-CsvParams) |  |  |
| snapshots | [Rpc.Object.Import.Request.Snapshot](#anytype-Rpc-Object-Import-Request-Snapshot) | repeated | optional, for external developers usage |
| updateExistingObjects | [bool](#bool) |  |  |
| type | [Rpc.Object.Import.Request.Type](#anytype-Rpc-Object-Import-Request-Type) |  |  |
| mode | [Rpc.Object.Import.Request.Mode](#anytype-Rpc-Object-Import-Request-Mode) |  |  |
| noProgress | [bool](#bool) |  |  |
| isMigration | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-Import-Request-BookmarksParams"></a>

### Rpc.Object.Import.Request.BookmarksParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Import-Request-CsvParams"></a>

### Rpc.Object.Import.Request.CsvParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |
| mode | [Rpc.Object.Import.Request.CsvParams.Mode](#anytype-Rpc-Object-Import-Request-CsvParams-Mode) |  |  |
| useFirstRowForRelations | [bool](#bool) |  |  |
| delimiter | [string](#string) |  |  |
| transposeRowsAndColumns | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-Import-Request-HtmlParams"></a>

### Rpc.Object.Import.Request.HtmlParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-Import-Request-MarkdownParams"></a>

### Rpc.Object.Import.Request.MarkdownParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-Import-Request-NotionParams"></a>

### Rpc.Object.Import.Request.NotionParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| apiKey | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Import-Request-PbParams"></a>

### Rpc.Object.Import.Request.PbParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |
| noCollection | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-Import-Request-Snapshot"></a>

### Rpc.Object.Import.Request.Snapshot



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| snapshot | [model.SmartBlockSnapshotBase](#anytype-model-SmartBlockSnapshotBase) |  |  |






<a name="anytype-Rpc-Object-Import-Request-TxtParams"></a>

### Rpc.Object.Import.Request.TxtParams



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-Import-Response"></a>

### Rpc.Object.Import.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Import.Response.Error](#anytype-Rpc-Object-Import-Response-Error) |  |  |






<a name="anytype-Rpc-Object-Import-Response-Error"></a>

### Rpc.Object.Import.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Import.Response.Error.Code](#anytype-Rpc-Object-Import-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ImportList"></a>

### Rpc.Object.ImportList







<a name="anytype-Rpc-Object-ImportList-ImportResponse"></a>

### Rpc.Object.ImportList.ImportResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [Rpc.Object.ImportList.ImportResponse.Type](#anytype-Rpc-Object-ImportList-ImportResponse-Type) |  |  |






<a name="anytype-Rpc-Object-ImportList-Request"></a>

### Rpc.Object.ImportList.Request







<a name="anytype-Rpc-Object-ImportList-Response"></a>

### Rpc.Object.ImportList.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ImportList.Response.Error](#anytype-Rpc-Object-ImportList-Response-Error) |  |  |
| response | [Rpc.Object.ImportList.ImportResponse](#anytype-Rpc-Object-ImportList-ImportResponse) | repeated |  |






<a name="anytype-Rpc-Object-ImportList-Response-Error"></a>

### Rpc.Object.ImportList.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ImportList.Response.Error.Code](#anytype-Rpc-Object-ImportList-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListDelete"></a>

### Rpc.Object.ListDelete







<a name="anytype-Rpc-Object-ListDelete-Request"></a>

### Rpc.Object.ListDelete.Request
Deletes the object, keys from the local store and unsubscribe from remote changes. Also offloads all orphan files


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated | objects to remove |






<a name="anytype-Rpc-Object-ListDelete-Response"></a>

### Rpc.Object.ListDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListDelete.Response.Error](#anytype-Rpc-Object-ListDelete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-ListDelete-Response-Error"></a>

### Rpc.Object.ListDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListDelete.Response.Error.Code](#anytype-Rpc-Object-ListDelete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListDuplicate"></a>

### Rpc.Object.ListDuplicate







<a name="anytype-Rpc-Object-ListDuplicate-Request"></a>

### Rpc.Object.ListDuplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-ListDuplicate-Response"></a>

### Rpc.Object.ListDuplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListDuplicate.Response.Error](#anytype-Rpc-Object-ListDuplicate-Response-Error) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-ListDuplicate-Response-Error"></a>

### Rpc.Object.ListDuplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListDuplicate.Response.Error.Code](#anytype-Rpc-Object-ListDuplicate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListExport"></a>

### Rpc.Object.ListExport







<a name="anytype-Rpc-Object-ListExport-Request"></a>

### Rpc.Object.ListExport.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| objectIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |
| format | [Rpc.Object.ListExport.Format](#anytype-Rpc-Object-ListExport-Format) |  | export format |
| zip | [bool](#bool) |  | save as zip file |
| includeNested | [bool](#bool) |  | include all nested |
| includeFiles | [bool](#bool) |  | include all files |
| isJson | [bool](#bool) |  | for protobuf export |
| includeArchived | [bool](#bool) |  | for migration |






<a name="anytype-Rpc-Object-ListExport-Response"></a>

### Rpc.Object.ListExport.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListExport.Response.Error](#anytype-Rpc-Object-ListExport-Response-Error) |  |  |
| path | [string](#string) |  |  |
| succeed | [int32](#int32) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-ListExport-Response-Error"></a>

### Rpc.Object.ListExport.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListExport.Response.Error.Code](#anytype-Rpc-Object-ListExport-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListSetIsArchived"></a>

### Rpc.Object.ListSetIsArchived







<a name="anytype-Rpc-Object-ListSetIsArchived-Request"></a>

### Rpc.Object.ListSetIsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isArchived | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-ListSetIsArchived-Response"></a>

### Rpc.Object.ListSetIsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListSetIsArchived.Response.Error](#anytype-Rpc-Object-ListSetIsArchived-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ListSetIsArchived-Response-Error"></a>

### Rpc.Object.ListSetIsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListSetIsArchived.Response.Error.Code](#anytype-Rpc-Object-ListSetIsArchived-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListSetIsFavorite"></a>

### Rpc.Object.ListSetIsFavorite







<a name="anytype-Rpc-Object-ListSetIsFavorite-Request"></a>

### Rpc.Object.ListSetIsFavorite.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isFavorite | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-ListSetIsFavorite-Response"></a>

### Rpc.Object.ListSetIsFavorite.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListSetIsFavorite.Response.Error](#anytype-Rpc-Object-ListSetIsFavorite-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ListSetIsFavorite-Response-Error"></a>

### Rpc.Object.ListSetIsFavorite.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListSetIsFavorite.Response.Error.Code](#anytype-Rpc-Object-ListSetIsFavorite-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Open"></a>

### Rpc.Object.Open







<a name="anytype-Rpc-Object-Open-Request"></a>

### Rpc.Object.Open.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| objectId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |
| includeRelationsAsDependentObjects | [bool](#bool) |  | some clients may set this option instead if having the single subscription to all relations |






<a name="anytype-Rpc-Object-Open-Response"></a>

### Rpc.Object.Open.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Open.Response.Error](#anytype-Rpc-Object-Open-Response-Error) |  |  |
| objectView | [model.ObjectView](#anytype-model-ObjectView) |  |  |






<a name="anytype-Rpc-Object-Open-Response-Error"></a>

### Rpc.Object.Open.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Open.Response.Error.Code](#anytype-Rpc-Object-Open-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-OpenBreadcrumbs"></a>

### Rpc.Object.OpenBreadcrumbs







<a name="anytype-Rpc-Object-OpenBreadcrumbs-Request"></a>

### Rpc.Object.OpenBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | deprecated |
| traceId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-OpenBreadcrumbs-Response"></a>

### Rpc.Object.OpenBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.OpenBreadcrumbs.Response.Error](#anytype-Rpc-Object-OpenBreadcrumbs-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| objectView | [model.ObjectView](#anytype-model-ObjectView) |  |  |






<a name="anytype-Rpc-Object-OpenBreadcrumbs-Response-Error"></a>

### Rpc.Object.OpenBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.OpenBreadcrumbs.Response.Error.Code](#anytype-Rpc-Object-OpenBreadcrumbs-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Redo"></a>

### Rpc.Object.Redo







<a name="anytype-Rpc-Object-Redo-Request"></a>

### Rpc.Object.Redo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |






<a name="anytype-Rpc-Object-Redo-Response"></a>

### Rpc.Object.Redo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Redo.Response.Error](#anytype-Rpc-Object-Redo-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| counters | [Rpc.Object.UndoRedoCounter](#anytype-Rpc-Object-UndoRedoCounter) |  |  |






<a name="anytype-Rpc-Object-Redo-Response-Error"></a>

### Rpc.Object.Redo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Redo.Response.Error.Code](#anytype-Rpc-Object-Redo-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Search"></a>

### Rpc.Object.Search







<a name="anytype-Rpc-Object-Search-Request"></a>

### Rpc.Object.Search.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |
| sorts | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated |  |
| fullText | [string](#string) |  |  |
| offset | [int32](#int32) |  |  |
| limit | [int32](#int32) |  |  |
| objectTypeFilter | [string](#string) | repeated | additional filter by objectTypes

DEPRECATED |
| keys | [string](#string) | repeated | needed keys in details for return, when empty - will return all |






<a name="anytype-Rpc-Object-Search-Response"></a>

### Rpc.Object.Search.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Search.Response.Error](#anytype-Rpc-Object-Search-Response-Error) |  |  |
| records | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |






<a name="anytype-Rpc-Object-Search-Response-Error"></a>

### Rpc.Object.Search.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Search.Response.Error.Code](#anytype-Rpc-Object-Search-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SearchSubscribe"></a>

### Rpc.Object.SearchSubscribe







<a name="anytype-Rpc-Object-SearchSubscribe-Request"></a>

### Rpc.Object.SearchSubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated | filters |
| sorts | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated | sorts |
| limit | [int64](#int64) |  | results limit |
| offset | [int64](#int64) |  | initial offset; middleware will find afterId |
| keys | [string](#string) | repeated | (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| afterId | [string](#string) |  | (optional) pagination: middleware will return results after given id |
| beforeId | [string](#string) |  | (optional) pagination: middleware will return results before given id |
| source | [string](#string) | repeated |  |
| ignoreWorkspace | [string](#string) |  |  |
| noDepSubscription | [bool](#bool) |  | disable dependent subscription |
| collectionId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SearchSubscribe-Response"></a>

### Rpc.Object.SearchSubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SearchSubscribe.Response.Error](#anytype-Rpc-Object-SearchSubscribe-Response-Error) |  |  |
| records | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| dependencies | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| subId | [string](#string) |  |  |
| counters | [Event.Object.Subscription.Counters](#anytype-Event-Object-Subscription-Counters) |  |  |






<a name="anytype-Rpc-Object-SearchSubscribe-Response-Error"></a>

### Rpc.Object.SearchSubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SearchSubscribe.Response.Error.Code](#anytype-Rpc-Object-SearchSubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SearchUnsubscribe"></a>

### Rpc.Object.SearchUnsubscribe







<a name="anytype-Rpc-Object-SearchUnsubscribe-Request"></a>

### Rpc.Object.SearchUnsubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-SearchUnsubscribe-Response"></a>

### Rpc.Object.SearchUnsubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SearchUnsubscribe.Response.Error](#anytype-Rpc-Object-SearchUnsubscribe-Response-Error) |  |  |






<a name="anytype-Rpc-Object-SearchUnsubscribe-Response-Error"></a>

### Rpc.Object.SearchUnsubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SearchUnsubscribe.Response.Error.Code](#anytype-Rpc-Object-SearchUnsubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetBreadcrumbs"></a>

### Rpc.Object.SetBreadcrumbs







<a name="anytype-Rpc-Object-SetBreadcrumbs-Request"></a>

### Rpc.Object.SetBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| breadcrumbsId | [string](#string) |  |  |
| ids | [string](#string) | repeated | page ids |






<a name="anytype-Rpc-Object-SetBreadcrumbs-Response"></a>

### Rpc.Object.SetBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetBreadcrumbs.Response.Error](#anytype-Rpc-Object-SetBreadcrumbs-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetBreadcrumbs-Response-Error"></a>

### Rpc.Object.SetBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetBreadcrumbs.Response.Error.Code](#anytype-Rpc-Object-SetBreadcrumbs-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetDetails"></a>

### Rpc.Object.SetDetails







<a name="anytype-Rpc-Object-SetDetails-Detail"></a>

### Rpc.Object.SetDetails.Detail



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  | NUll - removes key |






<a name="anytype-Rpc-Object-SetDetails-Request"></a>

### Rpc.Object.SetDetails.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| details | [Rpc.Object.SetDetails.Detail](#anytype-Rpc-Object-SetDetails-Detail) | repeated |  |






<a name="anytype-Rpc-Object-SetDetails-Response"></a>

### Rpc.Object.SetDetails.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetDetails.Response.Error](#anytype-Rpc-Object-SetDetails-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetDetails-Response-Error"></a>

### Rpc.Object.SetDetails.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetDetails.Response.Error.Code](#anytype-Rpc-Object-SetDetails-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetInternalFlags"></a>

### Rpc.Object.SetInternalFlags







<a name="anytype-Rpc-Object-SetInternalFlags-Request"></a>

### Rpc.Object.SetInternalFlags.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| internalFlags | [model.InternalFlag](#anytype-model-InternalFlag) | repeated |  |






<a name="anytype-Rpc-Object-SetInternalFlags-Response"></a>

### Rpc.Object.SetInternalFlags.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetInternalFlags.Response.Error](#anytype-Rpc-Object-SetInternalFlags-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetInternalFlags-Response-Error"></a>

### Rpc.Object.SetInternalFlags.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetInternalFlags.Response.Error.Code](#anytype-Rpc-Object-SetInternalFlags-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetIsArchived"></a>

### Rpc.Object.SetIsArchived







<a name="anytype-Rpc-Object-SetIsArchived-Request"></a>

### Rpc.Object.SetIsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| isArchived | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-SetIsArchived-Response"></a>

### Rpc.Object.SetIsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetIsArchived.Response.Error](#anytype-Rpc-Object-SetIsArchived-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetIsArchived-Response-Error"></a>

### Rpc.Object.SetIsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype-Rpc-Object-SetIsArchived-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetIsFavorite"></a>

### Rpc.Object.SetIsFavorite







<a name="anytype-Rpc-Object-SetIsFavorite-Request"></a>

### Rpc.Object.SetIsFavorite.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| isFavorite | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-SetIsFavorite-Response"></a>

### Rpc.Object.SetIsFavorite.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetIsFavorite.Response.Error](#anytype-Rpc-Object-SetIsFavorite-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetIsFavorite-Response-Error"></a>

### Rpc.Object.SetIsFavorite.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype-Rpc-Object-SetIsFavorite-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetLayout"></a>

### Rpc.Object.SetLayout







<a name="anytype-Rpc-Object-SetLayout-Request"></a>

### Rpc.Object.SetLayout.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| layout | [model.ObjectType.Layout](#anytype-model-ObjectType-Layout) |  |  |






<a name="anytype-Rpc-Object-SetLayout-Response"></a>

### Rpc.Object.SetLayout.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetLayout.Response.Error](#anytype-Rpc-Object-SetLayout-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetLayout-Response-Error"></a>

### Rpc.Object.SetLayout.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetLayout.Response.Error.Code](#anytype-Rpc-Object-SetLayout-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetObjectType"></a>

### Rpc.Object.SetObjectType







<a name="anytype-Rpc-Object-SetObjectType-Request"></a>

### Rpc.Object.SetObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| objectTypeUrl | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetObjectType-Response"></a>

### Rpc.Object.SetObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetObjectType.Response.Error](#anytype-Rpc-Object-SetObjectType-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetObjectType-Response-Error"></a>

### Rpc.Object.SetObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetObjectType.Response.Error.Code](#anytype-Rpc-Object-SetObjectType-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SetSource"></a>

### Rpc.Object.SetSource







<a name="anytype-Rpc-Object-SetSource-Request"></a>

### Rpc.Object.SetSource.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-SetSource-Response"></a>

### Rpc.Object.SetSource.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetSource.Response.Error](#anytype-Rpc-Object-SetSource-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-SetSource-Response-Error"></a>

### Rpc.Object.SetSource.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetSource.Response.Error.Code](#anytype-Rpc-Object-SetSource-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ShareByLink"></a>

### Rpc.Object.ShareByLink







<a name="anytype-Rpc-Object-ShareByLink-Request"></a>

### Rpc.Object.ShareByLink.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ShareByLink-Response"></a>

### Rpc.Object.ShareByLink.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| link | [string](#string) |  |  |
| error | [Rpc.Object.ShareByLink.Response.Error](#anytype-Rpc-Object-ShareByLink-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ShareByLink-Response-Error"></a>

### Rpc.Object.ShareByLink.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ShareByLink.Response.Error.Code](#anytype-Rpc-Object-ShareByLink-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Show"></a>

### Rpc.Object.Show







<a name="anytype-Rpc-Object-Show-Request"></a>

### Rpc.Object.Show.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | deprecated |
| objectId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |
| includeRelationsAsDependentObjects | [bool](#bool) |  | some clients may set this option instead if having the single subscription to all relations |






<a name="anytype-Rpc-Object-Show-Response"></a>

### Rpc.Object.Show.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Show.Response.Error](#anytype-Rpc-Object-Show-Response-Error) |  |  |
| objectView | [model.ObjectView](#anytype-model-ObjectView) |  |  |






<a name="anytype-Rpc-Object-Show-Response-Error"></a>

### Rpc.Object.Show.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Show.Response.Error.Code](#anytype-Rpc-Object-Show-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SubscribeIds"></a>

### Rpc.Object.SubscribeIds







<a name="anytype-Rpc-Object-SubscribeIds-Request"></a>

### Rpc.Object.SubscribeIds.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| ids | [string](#string) | repeated | ids for subscribe |
| keys | [string](#string) | repeated | sorts (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| ignoreWorkspace | [string](#string) |  |  |
| noDepSubscription | [bool](#bool) |  | disable dependent subscription |






<a name="anytype-Rpc-Object-SubscribeIds-Response"></a>

### Rpc.Object.SubscribeIds.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SubscribeIds.Response.Error](#anytype-Rpc-Object-SubscribeIds-Response-Error) |  |  |
| records | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| dependencies | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-SubscribeIds-Response-Error"></a>

### Rpc.Object.SubscribeIds.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SubscribeIds.Response.Error.Code](#anytype-Rpc-Object-SubscribeIds-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ToBookmark"></a>

### Rpc.Object.ToBookmark







<a name="anytype-Rpc-Object-ToBookmark-Request"></a>

### Rpc.Object.ToBookmark.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ToBookmark-Response"></a>

### Rpc.Object.ToBookmark.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ToBookmark.Response.Error](#anytype-Rpc-Object-ToBookmark-Response-Error) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ToBookmark-Response-Error"></a>

### Rpc.Object.ToBookmark.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ToBookmark.Response.Error.Code](#anytype-Rpc-Object-ToBookmark-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ToCollection"></a>

### Rpc.Object.ToCollection







<a name="anytype-Rpc-Object-ToCollection-Request"></a>

### Rpc.Object.ToCollection.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ToCollection-Response"></a>

### Rpc.Object.ToCollection.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ToCollection.Response.Error](#anytype-Rpc-Object-ToCollection-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ToCollection-Response-Error"></a>

### Rpc.Object.ToCollection.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ToCollection.Response.Error.Code](#anytype-Rpc-Object-ToCollection-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ToSet"></a>

### Rpc.Object.ToSet







<a name="anytype-Rpc-Object-ToSet-Request"></a>

### Rpc.Object.ToSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-ToSet-Response"></a>

### Rpc.Object.ToSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ToSet.Response.Error](#anytype-Rpc-Object-ToSet-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ToSet-Response-Error"></a>

### Rpc.Object.ToSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ToSet.Response.Error.Code](#anytype-Rpc-Object-ToSet-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Undo"></a>

### Rpc.Object.Undo







<a name="anytype-Rpc-Object-Undo-Request"></a>

### Rpc.Object.Undo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context object |






<a name="anytype-Rpc-Object-Undo-Response"></a>

### Rpc.Object.Undo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Undo.Response.Error](#anytype-Rpc-Object-Undo-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| counters | [Rpc.Object.UndoRedoCounter](#anytype-Rpc-Object-UndoRedoCounter) |  |  |






<a name="anytype-Rpc-Object-Undo-Response-Error"></a>

### Rpc.Object.Undo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Undo.Response.Error.Code](#anytype-Rpc-Object-Undo-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-UndoRedoCounter"></a>

### Rpc.Object.UndoRedoCounter
Available undo/redo operations


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| undo | [int32](#int32) |  |  |
| redo | [int32](#int32) |  |  |






<a name="anytype-Rpc-Object-WorkspaceSetDashboard"></a>

### Rpc.Object.WorkspaceSetDashboard







<a name="anytype-Rpc-Object-WorkspaceSetDashboard-Request"></a>

### Rpc.Object.WorkspaceSetDashboard.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-WorkspaceSetDashboard-Response"></a>

### Rpc.Object.WorkspaceSetDashboard.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.WorkspaceSetDashboard.Response.Error](#anytype-Rpc-Object-WorkspaceSetDashboard-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-WorkspaceSetDashboard-Response-Error"></a>

### Rpc.Object.WorkspaceSetDashboard.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.WorkspaceSetDashboard.Response.Error.Code](#anytype-Rpc-Object-WorkspaceSetDashboard-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectCollection"></a>

### Rpc.ObjectCollection







<a name="anytype-Rpc-ObjectCollection-Add"></a>

### Rpc.ObjectCollection.Add







<a name="anytype-Rpc-ObjectCollection-Add-Request"></a>

### Rpc.ObjectCollection.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| afterId | [string](#string) |  |  |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectCollection-Add-Response"></a>

### Rpc.ObjectCollection.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectCollection.Add.Response.Error](#anytype-Rpc-ObjectCollection-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectCollection-Add-Response-Error"></a>

### Rpc.ObjectCollection.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectCollection.Add.Response.Error.Code](#anytype-Rpc-ObjectCollection-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectCollection-Remove"></a>

### Rpc.ObjectCollection.Remove







<a name="anytype-Rpc-ObjectCollection-Remove-Request"></a>

### Rpc.ObjectCollection.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectCollection-Remove-Response"></a>

### Rpc.ObjectCollection.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectCollection.Remove.Response.Error](#anytype-Rpc-ObjectCollection-Remove-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectCollection-Remove-Response-Error"></a>

### Rpc.ObjectCollection.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectCollection.Remove.Response.Error.Code](#anytype-Rpc-ObjectCollection-Remove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectCollection-Sort"></a>

### Rpc.ObjectCollection.Sort







<a name="anytype-Rpc-ObjectCollection-Sort-Request"></a>

### Rpc.ObjectCollection.Sort.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectCollection-Sort-Response"></a>

### Rpc.ObjectCollection.Sort.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectCollection.Sort.Response.Error](#anytype-Rpc-ObjectCollection-Sort-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectCollection-Sort-Response-Error"></a>

### Rpc.ObjectCollection.Sort.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectCollection.Sort.Response.Error.Code](#anytype-Rpc-ObjectCollection-Sort-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectRelation"></a>

### Rpc.ObjectRelation







<a name="anytype-Rpc-ObjectRelation-Add"></a>

### Rpc.ObjectRelation.Add







<a name="anytype-Rpc-ObjectRelation-Add-Request"></a>

### Rpc.ObjectRelation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectRelation-Add-Response"></a>

### Rpc.ObjectRelation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.Add.Response.Error](#anytype-Rpc-ObjectRelation-Add-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectRelation-Add-Response-Error"></a>

### Rpc.ObjectRelation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.Add.Response.Error.Code](#anytype-Rpc-ObjectRelation-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectRelation-AddFeatured"></a>

### Rpc.ObjectRelation.AddFeatured







<a name="anytype-Rpc-ObjectRelation-AddFeatured-Request"></a>

### Rpc.ObjectRelation.AddFeatured.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectRelation-AddFeatured-Response"></a>

### Rpc.ObjectRelation.AddFeatured.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.AddFeatured.Response.Error](#anytype-Rpc-ObjectRelation-AddFeatured-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectRelation-AddFeatured-Response-Error"></a>

### Rpc.ObjectRelation.AddFeatured.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.AddFeatured.Response.Error.Code](#anytype-Rpc-ObjectRelation-AddFeatured-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectRelation-Delete"></a>

### Rpc.ObjectRelation.Delete







<a name="anytype-Rpc-ObjectRelation-Delete-Request"></a>

### Rpc.ObjectRelation.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectRelation-Delete-Response"></a>

### Rpc.ObjectRelation.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.Delete.Response.Error](#anytype-Rpc-ObjectRelation-Delete-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectRelation-Delete-Response-Error"></a>

### Rpc.ObjectRelation.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.Delete.Response.Error.Code](#anytype-Rpc-ObjectRelation-Delete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectRelation-ListAvailable"></a>

### Rpc.ObjectRelation.ListAvailable







<a name="anytype-Rpc-ObjectRelation-ListAvailable-Request"></a>

### Rpc.ObjectRelation.ListAvailable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectRelation-ListAvailable-Response"></a>

### Rpc.ObjectRelation.ListAvailable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.ListAvailable.Response.Error](#anytype-Rpc-ObjectRelation-ListAvailable-Response-Error) |  |  |
| relations | [model.Relation](#anytype-model-Relation) | repeated |  |






<a name="anytype-Rpc-ObjectRelation-ListAvailable-Response-Error"></a>

### Rpc.ObjectRelation.ListAvailable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.ListAvailable.Response.Error.Code](#anytype-Rpc-ObjectRelation-ListAvailable-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectRelation-RemoveFeatured"></a>

### Rpc.ObjectRelation.RemoveFeatured







<a name="anytype-Rpc-ObjectRelation-RemoveFeatured-Request"></a>

### Rpc.ObjectRelation.RemoveFeatured.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectRelation-RemoveFeatured-Response"></a>

### Rpc.ObjectRelation.RemoveFeatured.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.RemoveFeatured.Response.Error](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-ObjectRelation-RemoveFeatured-Response-Error"></a>

### Rpc.ObjectRelation.RemoveFeatured.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType"></a>

### Rpc.ObjectType







<a name="anytype-Rpc-ObjectType-Relation"></a>

### Rpc.ObjectType.Relation







<a name="anytype-Rpc-ObjectType-Relation-Add"></a>

### Rpc.ObjectType.Relation.Add







<a name="anytype-Rpc-ObjectType-Relation-Add-Request"></a>

### Rpc.ObjectType.Relation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectType-Relation-Add-Response"></a>

### Rpc.ObjectType.Relation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Add.Response.Error](#anytype-Rpc-ObjectType-Relation-Add-Response-Error) |  |  |
| relations | [model.Relation](#anytype-model-Relation) | repeated |  |






<a name="anytype-Rpc-ObjectType-Relation-Add-Response-Error"></a>

### Rpc.ObjectType.Relation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-Relation-List"></a>

### Rpc.ObjectType.Relation.List







<a name="anytype-Rpc-ObjectType-Relation-List-Request"></a>

### Rpc.ObjectType.Relation.List.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| appendRelationsFromOtherTypes | [bool](#bool) |  | add relations from other object types in the end |






<a name="anytype-Rpc-ObjectType-Relation-List-Response"></a>

### Rpc.ObjectType.Relation.List.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.List.Response.Error](#anytype-Rpc-ObjectType-Relation-List-Response-Error) |  |  |
| relations | [model.RelationLink](#anytype-model-RelationLink) | repeated |  |






<a name="anytype-Rpc-ObjectType-Relation-List-Response-Error"></a>

### Rpc.ObjectType.Relation.List.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-List-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-Relation-Remove"></a>

### Rpc.ObjectType.Relation.Remove







<a name="anytype-Rpc-ObjectType-Relation-Remove-Request"></a>

### Rpc.ObjectType.Relation.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectType-Relation-Remove-Response"></a>

### Rpc.ObjectType.Relation.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype-Rpc-ObjectType-Relation-Remove-Response-Error) |  |  |






<a name="anytype-Rpc-ObjectType-Relation-Remove-Response-Error"></a>

### Rpc.ObjectType.Relation.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-Remove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Process"></a>

### Rpc.Process







<a name="anytype-Rpc-Process-Cancel"></a>

### Rpc.Process.Cancel







<a name="anytype-Rpc-Process-Cancel-Request"></a>

### Rpc.Process.Cancel.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype-Rpc-Process-Cancel-Response"></a>

### Rpc.Process.Cancel.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Process.Cancel.Response.Error](#anytype-Rpc-Process-Cancel-Response-Error) |  |  |






<a name="anytype-Rpc-Process-Cancel-Response-Error"></a>

### Rpc.Process.Cancel.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Process.Cancel.Response.Error.Code](#anytype-Rpc-Process-Cancel-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Relation"></a>

### Rpc.Relation







<a name="anytype-Rpc-Relation-ListRemoveOption"></a>

### Rpc.Relation.ListRemoveOption







<a name="anytype-Rpc-Relation-ListRemoveOption-Request"></a>

### Rpc.Relation.ListRemoveOption.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| optionIds | [string](#string) | repeated |  |
| checkInObjects | [bool](#bool) |  |  |






<a name="anytype-Rpc-Relation-ListRemoveOption-Response"></a>

### Rpc.Relation.ListRemoveOption.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Relation.ListRemoveOption.Response.Error](#anytype-Rpc-Relation-ListRemoveOption-Response-Error) |  |  |






<a name="anytype-Rpc-Relation-ListRemoveOption-Response-Error"></a>

### Rpc.Relation.ListRemoveOption.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Relation.ListRemoveOption.Response.Error.Code](#anytype-Rpc-Relation-ListRemoveOption-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Relation-Options"></a>

### Rpc.Relation.Options







<a name="anytype-Rpc-Relation-Options-Request"></a>

### Rpc.Relation.Options.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relationKey | [string](#string) |  |  |






<a name="anytype-Rpc-Relation-Options-Response"></a>

### Rpc.Relation.Options.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Relation.Options.Response.Error](#anytype-Rpc-Relation-Options-Response-Error) |  |  |
| options | [model.RelationOptions](#anytype-model-RelationOptions) |  |  |






<a name="anytype-Rpc-Relation-Options-Response-Error"></a>

### Rpc.Relation.Options.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Relation.Options.Response.Error.Code](#anytype-Rpc-Relation-Options-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Template"></a>

### Rpc.Template







<a name="anytype-Rpc-Template-Clone"></a>

### Rpc.Template.Clone







<a name="anytype-Rpc-Template-Clone-Request"></a>

### Rpc.Template.Clone.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of template block for cloning |






<a name="anytype-Rpc-Template-Clone-Response"></a>

### Rpc.Template.Clone.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.Clone.Response.Error](#anytype-Rpc-Template-Clone-Response-Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype-Rpc-Template-Clone-Response-Error"></a>

### Rpc.Template.Clone.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.Clone.Response.Error.Code](#anytype-Rpc-Template-Clone-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Template-CreateFromObject"></a>

### Rpc.Template.CreateFromObject







<a name="anytype-Rpc-Template-CreateFromObject-Request"></a>

### Rpc.Template.CreateFromObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of block for making them template |






<a name="anytype-Rpc-Template-CreateFromObject-Response"></a>

### Rpc.Template.CreateFromObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.CreateFromObject.Response.Error](#anytype-Rpc-Template-CreateFromObject-Response-Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype-Rpc-Template-CreateFromObject-Response-Error"></a>

### Rpc.Template.CreateFromObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.CreateFromObject.Response.Error.Code](#anytype-Rpc-Template-CreateFromObject-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Template-CreateFromObjectType"></a>

### Rpc.Template.CreateFromObjectType







<a name="anytype-Rpc-Template-CreateFromObjectType-Request"></a>

### Rpc.Template.CreateFromObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectType | [string](#string) |  | id of desired object type |






<a name="anytype-Rpc-Template-CreateFromObjectType-Response"></a>

### Rpc.Template.CreateFromObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.CreateFromObjectType.Response.Error](#anytype-Rpc-Template-CreateFromObjectType-Response-Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype-Rpc-Template-CreateFromObjectType-Response-Error"></a>

### Rpc.Template.CreateFromObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.CreateFromObjectType.Response.Error.Code](#anytype-Rpc-Template-CreateFromObjectType-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Template-ExportAll"></a>

### Rpc.Template.ExportAll







<a name="anytype-Rpc-Template-ExportAll-Request"></a>

### Rpc.Template.ExportAll.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |






<a name="anytype-Rpc-Template-ExportAll-Response"></a>

### Rpc.Template.ExportAll.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.ExportAll.Response.Error](#anytype-Rpc-Template-ExportAll-Response-Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Template-ExportAll-Response-Error"></a>

### Rpc.Template.ExportAll.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.ExportAll.Response.Error.Code](#anytype-Rpc-Template-ExportAll-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Unsplash"></a>

### Rpc.Unsplash







<a name="anytype-Rpc-Unsplash-Download"></a>

### Rpc.Unsplash.Download







<a name="anytype-Rpc-Unsplash-Download-Request"></a>

### Rpc.Unsplash.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pictureId | [string](#string) |  |  |






<a name="anytype-Rpc-Unsplash-Download-Response"></a>

### Rpc.Unsplash.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Unsplash.Download.Response.Error](#anytype-Rpc-Unsplash-Download-Response-Error) |  |  |
| hash | [string](#string) |  |  |






<a name="anytype-Rpc-Unsplash-Download-Response-Error"></a>

### Rpc.Unsplash.Download.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Unsplash.Download.Response.Error.Code](#anytype-Rpc-Unsplash-Download-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Unsplash-Search"></a>

### Rpc.Unsplash.Search







<a name="anytype-Rpc-Unsplash-Search-Request"></a>

### Rpc.Unsplash.Search.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| query | [string](#string) |  | empty means random images |
| limit | [int32](#int32) |  | may be omitted if the request was cached previously with another limit |






<a name="anytype-Rpc-Unsplash-Search-Response"></a>

### Rpc.Unsplash.Search.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Unsplash.Search.Response.Error](#anytype-Rpc-Unsplash-Search-Response-Error) |  |  |
| pictures | [Rpc.Unsplash.Search.Response.Picture](#anytype-Rpc-Unsplash-Search-Response-Picture) | repeated |  |






<a name="anytype-Rpc-Unsplash-Search-Response-Error"></a>

### Rpc.Unsplash.Search.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Unsplash.Search.Response.Error.Code](#anytype-Rpc-Unsplash-Search-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Unsplash-Search-Response-Picture"></a>

### Rpc.Unsplash.Search.Response.Picture



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [string](#string) |  |  |
| artist | [string](#string) |  |  |
| artistUrl | [string](#string) |  |  |






<a name="anytype-Rpc-UserData"></a>

### Rpc.UserData







<a name="anytype-Rpc-UserData-Dump"></a>

### Rpc.UserData.Dump







<a name="anytype-Rpc-UserData-Dump-Request"></a>

### Rpc.UserData.Dump.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |






<a name="anytype-Rpc-UserData-Dump-Response"></a>

### Rpc.UserData.Dump.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.UserData.Dump.Response.Error](#anytype-Rpc-UserData-Dump-Response-Error) |  |  |






<a name="anytype-Rpc-UserData-Dump-Response-Error"></a>

### Rpc.UserData.Dump.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.UserData.Dump.Response.Error.Code](#anytype-Rpc-UserData-Dump-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet"></a>

### Rpc.Wallet







<a name="anytype-Rpc-Wallet-CloseSession"></a>

### Rpc.Wallet.CloseSession







<a name="anytype-Rpc-Wallet-CloseSession-Request"></a>

### Rpc.Wallet.CloseSession.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-CloseSession-Response"></a>

### Rpc.Wallet.CloseSession.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.CloseSession.Response.Error](#anytype-Rpc-Wallet-CloseSession-Response-Error) |  |  |






<a name="anytype-Rpc-Wallet-CloseSession-Response-Error"></a>

### Rpc.Wallet.CloseSession.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.CloseSession.Response.Error.Code](#anytype-Rpc-Wallet-CloseSession-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-Convert"></a>

### Rpc.Wallet.Convert







<a name="anytype-Rpc-Wallet-Convert-Request"></a>

### Rpc.Wallet.Convert.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| mnemonic | [string](#string) |  | Mnemonic of a wallet to convert |
| entropy | [string](#string) |  | entropy of a wallet to convert |






<a name="anytype-Rpc-Wallet-Convert-Response"></a>

### Rpc.Wallet.Convert.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Convert.Response.Error](#anytype-Rpc-Wallet-Convert-Response-Error) |  | Error while trying to recover a wallet |
| entropy | [string](#string) |  |  |
| mnemonic | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-Convert-Response-Error"></a>

### Rpc.Wallet.Convert.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Convert.Response.Error.Code](#anytype-Rpc-Wallet-Convert-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-Create"></a>

### Rpc.Wallet.Create







<a name="anytype-Rpc-Wallet-Create-Request"></a>

### Rpc.Wallet.Create.Request
Front-end-to-middleware request to create a new wallet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) |  | Path to a wallet directory |






<a name="anytype-Rpc-Wallet-Create-Response"></a>

### Rpc.Wallet.Create.Response
Middleware-to-front-end response, that can contain mnemonic of a created account and a NULL error or an empty mnemonic and a non-NULL error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Create.Response.Error](#anytype-Rpc-Wallet-Create-Response-Error) |  |  |
| mnemonic | [string](#string) |  | Mnemonic of a new account (sequence of words, divided by spaces) |






<a name="anytype-Rpc-Wallet-Create-Response-Error"></a>

### Rpc.Wallet.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Create.Response.Error.Code](#anytype-Rpc-Wallet-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-CreateSession"></a>

### Rpc.Wallet.CreateSession







<a name="anytype-Rpc-Wallet-CreateSession-Request"></a>

### Rpc.Wallet.CreateSession.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| mnemonic | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-CreateSession-Response"></a>

### Rpc.Wallet.CreateSession.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.CreateSession.Response.Error](#anytype-Rpc-Wallet-CreateSession-Response-Error) |  |  |
| token | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-CreateSession-Response-Error"></a>

### Rpc.Wallet.CreateSession.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.CreateSession.Response.Error.Code](#anytype-Rpc-Wallet-CreateSession-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Wallet-Recover"></a>

### Rpc.Wallet.Recover







<a name="anytype-Rpc-Wallet-Recover-Request"></a>

### Rpc.Wallet.Recover.Request
Front end to middleware request-to-recover-a wallet with this mnemonic and a rootPath


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) |  | Path to a wallet directory |
| mnemonic | [string](#string) |  | Mnemonic of a wallet to recover |






<a name="anytype-Rpc-Wallet-Recover-Response"></a>

### Rpc.Wallet.Recover.Response
Middleware-to-front-end response, that can contain a NULL error or a non-NULL error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Recover.Response.Error](#anytype-Rpc-Wallet-Recover-Response-Error) |  | Error while trying to recover a wallet |






<a name="anytype-Rpc-Wallet-Recover-Response-Error"></a>

### Rpc.Wallet.Recover.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Recover.Response.Error.Code](#anytype-Rpc-Wallet-Recover-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace"></a>

### Rpc.Workspace







<a name="anytype-Rpc-Workspace-Create"></a>

### Rpc.Workspace.Create







<a name="anytype-Rpc-Workspace-Create-Request"></a>

### Rpc.Workspace.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Create-Response"></a>

### Rpc.Workspace.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Create.Response.Error](#anytype-Rpc-Workspace-Create-Response-Error) |  |  |
| workspaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Create-Response-Error"></a>

### Rpc.Workspace.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Create.Response.Error.Code](#anytype-Rpc-Workspace-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Export"></a>

### Rpc.Workspace.Export







<a name="anytype-Rpc-Workspace-Export-Request"></a>

### Rpc.Workspace.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| workspaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Export-Response"></a>

### Rpc.Workspace.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Export.Response.Error](#anytype-Rpc-Workspace-Export-Response-Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Workspace-Export-Response-Error"></a>

### Rpc.Workspace.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Export.Response.Error.Code](#anytype-Rpc-Workspace-Export-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-GetAll"></a>

### Rpc.Workspace.GetAll







<a name="anytype-Rpc-Workspace-GetAll-Request"></a>

### Rpc.Workspace.GetAll.Request







<a name="anytype-Rpc-Workspace-GetAll-Response"></a>

### Rpc.Workspace.GetAll.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.GetAll.Response.Error](#anytype-Rpc-Workspace-GetAll-Response-Error) |  |  |
| workspaceIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Workspace-GetAll-Response-Error"></a>

### Rpc.Workspace.GetAll.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.GetAll.Response.Error.Code](#anytype-Rpc-Workspace-GetAll-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-GetCurrent"></a>

### Rpc.Workspace.GetCurrent







<a name="anytype-Rpc-Workspace-GetCurrent-Request"></a>

### Rpc.Workspace.GetCurrent.Request







<a name="anytype-Rpc-Workspace-GetCurrent-Response"></a>

### Rpc.Workspace.GetCurrent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.GetCurrent.Response.Error](#anytype-Rpc-Workspace-GetCurrent-Response-Error) |  |  |
| workspaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-GetCurrent-Response-Error"></a>

### Rpc.Workspace.GetCurrent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.GetCurrent.Response.Error.Code](#anytype-Rpc-Workspace-GetCurrent-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Object"></a>

### Rpc.Workspace.Object







<a name="anytype-Rpc-Workspace-Object-Add"></a>

### Rpc.Workspace.Object.Add







<a name="anytype-Rpc-Workspace-Object-Add-Request"></a>

### Rpc.Workspace.Object.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Object-Add-Response"></a>

### Rpc.Workspace.Object.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Object.Add.Response.Error](#anytype-Rpc-Workspace-Object-Add-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Workspace-Object-Add-Response-Error"></a>

### Rpc.Workspace.Object.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Object.Add.Response.Error.Code](#anytype-Rpc-Workspace-Object-Add-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Object-ListAdd"></a>

### Rpc.Workspace.Object.ListAdd







<a name="anytype-Rpc-Workspace-Object-ListAdd-Request"></a>

### Rpc.Workspace.Object.ListAdd.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Workspace-Object-ListAdd-Response"></a>

### Rpc.Workspace.Object.ListAdd.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Object.ListAdd.Response.Error](#anytype-Rpc-Workspace-Object-ListAdd-Response-Error) |  |  |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Workspace-Object-ListAdd-Response-Error"></a>

### Rpc.Workspace.Object.ListAdd.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Object.ListAdd.Response.Error.Code](#anytype-Rpc-Workspace-Object-ListAdd-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Object-ListRemove"></a>

### Rpc.Workspace.Object.ListRemove







<a name="anytype-Rpc-Workspace-Object-ListRemove-Request"></a>

### Rpc.Workspace.Object.ListRemove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Workspace-Object-ListRemove-Response"></a>

### Rpc.Workspace.Object.ListRemove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Object.ListRemove.Response.Error](#anytype-Rpc-Workspace-Object-ListRemove-Response-Error) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Rpc-Workspace-Object-ListRemove-Response-Error"></a>

### Rpc.Workspace.Object.ListRemove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Object.ListRemove.Response.Error.Code](#anytype-Rpc-Workspace-Object-ListRemove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Select"></a>

### Rpc.Workspace.Select







<a name="anytype-Rpc-Workspace-Select-Request"></a>

### Rpc.Workspace.Select.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| workspaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-Select-Response"></a>

### Rpc.Workspace.Select.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Select.Response.Error](#anytype-Rpc-Workspace-Select-Response-Error) |  |  |






<a name="anytype-Rpc-Workspace-Select-Response-Error"></a>

### Rpc.Workspace.Select.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Select.Response.Error.Code](#anytype-Rpc-Workspace-Select-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Workspace-SetIsHighlighted"></a>

### Rpc.Workspace.SetIsHighlighted







<a name="anytype-Rpc-Workspace-SetIsHighlighted-Request"></a>

### Rpc.Workspace.SetIsHighlighted.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| isHighlighted | [bool](#bool) |  |  |






<a name="anytype-Rpc-Workspace-SetIsHighlighted-Response"></a>

### Rpc.Workspace.SetIsHighlighted.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.SetIsHighlighted.Response.Error](#anytype-Rpc-Workspace-SetIsHighlighted-Response-Error) |  |  |






<a name="anytype-Rpc-Workspace-SetIsHighlighted-Response-Error"></a>

### Rpc.Workspace.SetIsHighlighted.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.SetIsHighlighted.Response.Error.Code](#anytype-Rpc-Workspace-SetIsHighlighted-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-StreamRequest"></a>

### StreamRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |





 


<a name="anytype-Rpc-Account-ConfigUpdate-Response-Error-Code"></a>

### Rpc.Account.ConfigUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| FAILED_TO_WRITE_CONFIG | 102 |  |
| FAILED_TO_GET_CONFIG | 103 |  |



<a name="anytype-Rpc-Account-ConfigUpdate-Timezones"></a>

### Rpc.Account.ConfigUpdate.Timezones


| Name | Number | Description |
| ---- | ------ | ----------- |
| GMT | 0 |  |
| ECT | 1 |  |
| EET | 2 |  |
| EAT | 3 |  |
| MET | 4 |  |
| NET | 5 |  |
| PLT | 6 |  |
| IST | 7 |  |
| BST | 8 |  |
| VST | 9 |  |
| CTT | 10 |  |
| JST | 11 |  |
| ACT | 12 |  |
| AET | 13 |  |
| SST | 14 |  |
| NST | 15 |  |
| MIT | 16 |  |
| HST | 17 |  |
| AST | 18 |  |
| PST | 19 |  |
| MST | 20 |  |
| CST | 21 |  |
| IET | 22 |  |
| PRT | 23 |  |
| CNT | 24 |  |
| BET | 25 |  |
| BRT | 26 |  |
| CAT | 27 |  |



<a name="anytype-Rpc-Account-Create-Response-Error-Code"></a>

### Rpc.Account.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; Account should be non-empty |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Avatar or name is not correct |
| ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE | 101 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME | 102 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR | 103 |  |
| FAILED_TO_STOP_RUNNING_NODE | 104 |  |
| FAILED_TO_WRITE_CONFIG | 105 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 106 |  |
| BAD_INVITE_CODE | 900 |  |
| NET_ERROR | 901 | means general network error |
| NET_CONNECTION_REFUSED | 902 | means we wasn&#39;t able to connect to the cafe server |
| NET_OFFLINE | 903 | client can additionally support this error code to notify user that device is offline |



<a name="anytype-Rpc-Account-Delete-Response-Error-Code"></a>

### Rpc.Account.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_ALREADY_DELETED | 101 |  |
| ACCOUNT_IS_ACTIVE | 102 |  |



<a name="anytype-Rpc-Account-Move-Response-Error-Code"></a>

### Rpc.Account.Move.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_STOP_NODE | 101 |  |
| FAILED_TO_IDENTIFY_ACCOUNT_DIR | 102 |  |
| FAILED_TO_REMOVE_ACCOUNT_DATA | 103 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 104 |  |
| FAILED_TO_WRITE_CONFIG | 105 |  |
| FAILED_TO_GET_CONFIG | 106 |  |



<a name="anytype-Rpc-Account-Recover-Response-Error-Code"></a>

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
| FAILED_TO_STOP_RUNNING_NODE | 107 |  |
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |
| ACCOUNT_IS_DELETED | 109 |  |



<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error-Code"></a>

### Rpc.Account.RecoverFromLegacyExport.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| DIFFERENT_ACCOUNT | 3 |  |



<a name="anytype-Rpc-Account-Select-Response-Error-Code"></a>

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
| FAILED_TO_STOP_SEARCHER_NODE | 106 |  |
| FAILED_TO_RECOVER_PREDEFINED_BLOCKS | 107 |  |
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |



<a name="anytype-Rpc-Account-Stop-Response-Error-Code"></a>

### Rpc.Account.Stop.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Id or root path is wrong |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| FAILED_TO_STOP_NODE | 102 |  |
| FAILED_TO_REMOVE_ACCOUNT_DATA | 103 |  |



<a name="anytype-Rpc-App-GetVersion-Response-Error-Code"></a>

### Rpc.App.GetVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| VERSION_IS_EMPTY | 3 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype-Rpc-App-SetDeviceState-Request-DeviceState"></a>

### Rpc.App.SetDeviceState.Request.DeviceState


| Name | Number | Description |
| ---- | ------ | ----------- |
| BACKGROUND | 0 |  |
| FOREGROUND | 1 |  |



<a name="anytype-Rpc-App-SetDeviceState-Response-Error-Code"></a>

### Rpc.App.SetDeviceState.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |



<a name="anytype-Rpc-App-Shutdown-Response-Error-Code"></a>

### Rpc.App.Shutdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |



<a name="anytype-Rpc-Block-Copy-Response-Error-Code"></a>

### Rpc.Block.Copy.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Create-Response-Error-Code"></a>

### Rpc.Block.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-CreateWidget-Response-Error-Code"></a>

### Rpc.Block.CreateWidget.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Cut-Response-Error-Code"></a>

### Rpc.Block.Cut.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Download-Response-Error-Code"></a>

### Rpc.Block.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Export-Response-Error-Code"></a>

### Rpc.Block.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListConvertToObjects-Response-Error-Code"></a>

### Rpc.Block.ListConvertToObjects.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListDelete-Response-Error-Code"></a>

### Rpc.Block.ListDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListDuplicate-Response-Error-Code"></a>

### Rpc.Block.ListDuplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListMoveToExistingObject-Response-Error-Code"></a>

### Rpc.Block.ListMoveToExistingObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListMoveToNewObject-Response-Error-Code"></a>

### Rpc.Block.ListMoveToNewObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListSetAlign-Response-Error-Code"></a>

### Rpc.Block.ListSetAlign.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListSetBackgroundColor-Response-Error-Code"></a>

### Rpc.Block.ListSetBackgroundColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListSetFields-Response-Error-Code"></a>

### Rpc.Block.ListSetFields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListSetVerticalAlign-Response-Error-Code"></a>

### Rpc.Block.ListSetVerticalAlign.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-ListTurnInto-Response-Error-Code"></a>

### Rpc.Block.ListTurnInto.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Merge-Response-Error-Code"></a>

### Rpc.Block.Merge.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Paste-Response-Error-Code"></a>

### Rpc.Block.Paste.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Replace-Response-Error-Code"></a>

### Rpc.Block.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-SetFields-Response-Error-Code"></a>

### Rpc.Block.SetFields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Split-Request-Mode"></a>

### Rpc.Block.Split.Request.Mode


| Name | Number | Description |
| ---- | ------ | ----------- |
| BOTTOM | 0 | new block will be created under existing |
| TOP | 1 | new block will be created above existing |
| INNER | 2 | new block will be created as the first children of existing |
| TITLE | 3 | new block will be created after header (not required for set at client side, will auto set for title block) |



<a name="anytype-Rpc-Block-Split-Response-Error-Code"></a>

### Rpc.Block.Split.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-Upload-Response-Error-Code"></a>

### Rpc.Block.Upload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error-Code"></a>

### Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockBookmark-Fetch-Response-Error-Code"></a>

### Rpc.BlockBookmark.Fetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-CreateBookmark-Response-Error-Code"></a>

### Rpc.BlockDataview.CreateBookmark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-CreateFromExistingObject-Response-Error-Code"></a>

### Rpc.BlockDataview.CreateFromExistingObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-Filter-Add-Response-Error-Code"></a>

### Rpc.BlockDataview.Filter.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Filter-Remove-Response-Error-Code"></a>

### Rpc.BlockDataview.Filter.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Filter-Replace-Response-Error-Code"></a>

### Rpc.BlockDataview.Filter.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Filter-Sort-Response-Error-Code"></a>

### Rpc.BlockDataview.Filter.Sort.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-GroupOrder-Update-Response-Error-Code"></a>

### Rpc.BlockDataview.GroupOrder.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-ObjectOrder-Move-Response-Error-Code"></a>

### Rpc.BlockDataview.ObjectOrder.Move.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-ObjectOrder-Update-Response-Error-Code"></a>

### Rpc.BlockDataview.ObjectOrder.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Relation-Add-Response-Error-Code"></a>

### Rpc.BlockDataview.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Relation-Delete-Response-Error-Code"></a>

### Rpc.BlockDataview.Relation.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Relation-ListAvailable-Response-Error-Code"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_A_DATAVIEW_BLOCK | 3 | ... |



<a name="anytype-Rpc-BlockDataview-SetSource-Response-Error-Code"></a>

### Rpc.BlockDataview.SetSource.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Sort-Add-Response-Error-Code"></a>

### Rpc.BlockDataview.Sort.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Sort-Remove-Response-Error-Code"></a>

### Rpc.BlockDataview.Sort.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Sort-Replace-Response-Error-Code"></a>

### Rpc.BlockDataview.Sort.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-Sort-Sort-Response-Error-Code"></a>

### Rpc.BlockDataview.Sort.Sort.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-View-Create-Response-Error-Code"></a>

### Rpc.BlockDataview.View.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-View-Delete-Response-Error-Code"></a>

### Rpc.BlockDataview.View.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-View-SetActive-Response-Error-Code"></a>

### Rpc.BlockDataview.View.SetActive.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-View-SetPosition-Response-Error-Code"></a>

### Rpc.BlockDataview.View.SetPosition.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-View-Update-Response-Error-Code"></a>

### Rpc.BlockDataview.View.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockDataview-ViewRelation-Add-Response-Error-Code"></a>

### Rpc.BlockDataview.ViewRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-ViewRelation-Remove-Response-Error-Code"></a>

### Rpc.BlockDataview.ViewRelation.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-ViewRelation-Replace-Response-Error-Code"></a>

### Rpc.BlockDataview.ViewRelation.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDataview-ViewRelation-Sort-Response-Error-Code"></a>

### Rpc.BlockDataview.ViewRelation.Sort.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockDiv-ListSetStyle-Response-Error-Code"></a>

### Rpc.BlockDiv.ListSetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockFile-CreateAndUpload-Response-Error-Code"></a>

### Rpc.BlockFile.CreateAndUpload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockFile-ListSetStyle-Response-Error-Code"></a>

### Rpc.BlockFile.ListSetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockFile-SetName-Response-Error-Code"></a>

### Rpc.BlockFile.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockImage-SetName-Response-Error-Code"></a>

### Rpc.BlockImage.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockImage-SetWidth-Response-Error-Code"></a>

### Rpc.BlockImage.SetWidth.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockLatex-SetText-Response-Error-Code"></a>

### Rpc.BlockLatex.SetText.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockLink-CreateWithObject-Response-Error-Code"></a>

### Rpc.BlockLink.CreateWithObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockLink-ListSetAppearance-Response-Error-Code"></a>

### Rpc.BlockLink.ListSetAppearance.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockRelation-Add-Response-Error-Code"></a>

### Rpc.BlockRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockRelation-SetKey-Response-Error-Code"></a>

### Rpc.BlockRelation.SetKey.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockTable-ColumnCreate-Response-Error-Code"></a>

### Rpc.BlockTable.ColumnCreate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-ColumnDelete-Response-Error-Code"></a>

### Rpc.BlockTable.ColumnDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-ColumnDuplicate-Response-Error-Code"></a>

### Rpc.BlockTable.ColumnDuplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-ColumnListFill-Response-Error-Code"></a>

### Rpc.BlockTable.ColumnListFill.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-ColumnMove-Response-Error-Code"></a>

### Rpc.BlockTable.ColumnMove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-Create-Response-Error-Code"></a>

### Rpc.BlockTable.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-Expand-Response-Error-Code"></a>

### Rpc.BlockTable.Expand.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-RowCreate-Response-Error-Code"></a>

### Rpc.BlockTable.RowCreate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-RowDelete-Response-Error-Code"></a>

### Rpc.BlockTable.RowDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-RowDuplicate-Response-Error-Code"></a>

### Rpc.BlockTable.RowDuplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-RowListClean-Response-Error-Code"></a>

### Rpc.BlockTable.RowListClean.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-RowListFill-Response-Error-Code"></a>

### Rpc.BlockTable.RowListFill.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-RowSetHeader-Response-Error-Code"></a>

### Rpc.BlockTable.RowSetHeader.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockTable-Sort-Response-Error-Code"></a>

### Rpc.BlockTable.Sort.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-ListClearContent-Response-Error-Code"></a>

### Rpc.BlockText.ListClearContent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-ListClearStyle-Response-Error-Code"></a>

### Rpc.BlockText.ListClearStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-ListSetColor-Response-Error-Code"></a>

### Rpc.BlockText.ListSetColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-ListSetMark-Response-Error-Code"></a>

### Rpc.BlockText.ListSetMark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-ListSetStyle-Response-Error-Code"></a>

### Rpc.BlockText.ListSetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-SetChecked-Response-Error-Code"></a>

### Rpc.BlockText.SetChecked.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-SetColor-Response-Error-Code"></a>

### Rpc.BlockText.SetColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-SetIcon-Response-Error-Code"></a>

### Rpc.BlockText.SetIcon.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-SetMarks-Get-Response-Error-Code"></a>

### Rpc.BlockText.SetMarks.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-SetStyle-Response-Error-Code"></a>

### Rpc.BlockText.SetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockText-SetText-Response-Error-Code"></a>

### Rpc.BlockText.SetText.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockVideo-SetName-Response-Error-Code"></a>

### Rpc.BlockVideo.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockVideo-SetWidth-Response-Error-Code"></a>

### Rpc.BlockVideo.SetWidth.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-BlockWidget-SetLayout-Response-Error-Code"></a>

### Rpc.BlockWidget.SetLayout.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockWidget-SetLimit-Response-Error-Code"></a>

### Rpc.BlockWidget.SetLimit.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-BlockWidget-SetTargetId-Response-Error-Code"></a>

### Rpc.BlockWidget.SetTargetId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Debug-ExportLocalstore-Response-Error-Code"></a>

### Rpc.Debug.ExportLocalstore.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Debug-Ping-Response-Error-Code"></a>

### Rpc.Debug.Ping.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Debug-SpaceSummary-Response-Error-Code"></a>

### Rpc.Debug.SpaceSummary.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Debug-Tree-Response-Error-Code"></a>

### Rpc.Debug.Tree.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Debug-TreeHeads-Response-Error-Code"></a>

### Rpc.Debug.TreeHeads.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-File-Download-Response-Error-Code"></a>

### Rpc.File.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |



<a name="anytype-Rpc-File-Drop-Response-Error-Code"></a>

### Rpc.File.Drop.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-File-ListOffload-Response-Error-Code"></a>

### Rpc.File.ListOffload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |



<a name="anytype-Rpc-File-Offload-Response-Error-Code"></a>

### Rpc.File.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |
| FILE_NOT_YET_PINNED | 104 |  |



<a name="anytype-Rpc-File-SpaceUsage-Response-Error-Code"></a>

### Rpc.File.SpaceUsage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-File-Upload-Response-Error-Code"></a>

### Rpc.File.Upload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-GenericErrorResponse-Error-Code"></a>

### Rpc.GenericErrorResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-History-GetVersions-Response-Error-Code"></a>

### Rpc.History.GetVersions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-History-SetVersion-Response-Error-Code"></a>

### Rpc.History.SetVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-History-ShowVersion-Response-Error-Code"></a>

### Rpc.History.ShowVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-LinkPreview-Response-Error-Code"></a>

### Rpc.LinkPreview.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Log-Send-Request-Level"></a>

### Rpc.Log.Send.Request.Level


| Name | Number | Description |
| ---- | ------ | ----------- |
| DEBUG | 0 |  |
| ERROR | 1 |  |
| FATAL | 2 |  |
| INFO | 3 |  |
| PANIC | 4 |  |
| WARNING | 5 |  |



<a name="anytype-Rpc-Log-Send-Response-Error-Code"></a>

### Rpc.Log.Send.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype-Rpc-Metrics-SetParameters-Response-Error-Code"></a>

### Rpc.Metrics.SetParameters.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Navigation-Context"></a>

### Rpc.Navigation.Context


| Name | Number | Description |
| ---- | ------ | ----------- |
| Navigation | 0 |  |
| MoveTo | 1 | do not show sets/archive |
| LinkTo | 2 | same for mention, do not show sets/archive |



<a name="anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error-Code"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Navigation-ListObjects-Response-Error-Code"></a>

### Rpc.Navigation.ListObjects.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ApplyTemplate-Response-Error-Code"></a>

### Rpc.Object.ApplyTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-BookmarkFetch-Response-Error-Code"></a>

### Rpc.Object.BookmarkFetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Close-Response-Error-Code"></a>

### Rpc.Object.Close.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Create-Response-Error-Code"></a>

### Rpc.Object.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-CreateBookmark-Response-Error-Code"></a>

### Rpc.Object.CreateBookmark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-CreateObjectType-Response-Error-Code"></a>

### Rpc.Object.CreateObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 | ... |



<a name="anytype-Rpc-Object-CreateRelation-Response-Error-Code"></a>

### Rpc.Object.CreateRelation.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-CreateRelationOption-Response-Error-Code"></a>

### Rpc.Object.CreateRelationOption.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-CreateSet-Response-Error-Code"></a>

### Rpc.Object.CreateSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype-Rpc-Object-Duplicate-Response-Error-Code"></a>

### Rpc.Object.Duplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Graph-Edge-Type"></a>

### Rpc.Object.Graph.Edge.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Link | 0 |  |
| Relation | 1 |  |



<a name="anytype-Rpc-Object-Graph-Response-Error-Code"></a>

### Rpc.Object.Graph.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-GroupsSubscribe-Response-Error-Code"></a>

### Rpc.Object.GroupsSubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error-Code"></a>

### Rpc.Object.Import.Notion.ValidateToken.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| INTERNAL_ERROR | 1 |  |
| UNAUTHORIZED | 2 |  |
| UNKNOWN_ERROR | 3 |  |
| BAD_INPUT | 4 |  |
| FORBIDDEN | 5 |  |
| SERVICE_UNAVAILABLE | 6 |  |
| ACCOUNT_IS_NOT_RUNNING | 7 |  |



<a name="anytype-Rpc-Object-Import-Request-CsvParams-Mode"></a>

### Rpc.Object.Import.Request.CsvParams.Mode


| Name | Number | Description |
| ---- | ------ | ----------- |
| COLLECTION | 0 |  |
| TABLE | 1 |  |



<a name="anytype-Rpc-Object-Import-Request-Mode"></a>

### Rpc.Object.Import.Request.Mode


| Name | Number | Description |
| ---- | ------ | ----------- |
| ALL_OR_NOTHING | 0 |  |
| IGNORE_ERRORS | 1 |  |



<a name="anytype-Rpc-Object-Import-Request-Type"></a>

### Rpc.Object.Import.Request.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Notion | 0 |  |
| Markdown | 1 |  |
| External | 2 | external developers use it |
| Pb | 3 |  |
| Html | 4 |  |
| Txt | 5 |  |
| Csv | 6 |  |



<a name="anytype-Rpc-Object-Import-Response-Error-Code"></a>

### Rpc.Object.Import.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| INTERNAL_ERROR | 1 |  |
| UNKNOWN_ERROR | 2 |  |
| BAD_INPUT | 3 |  |
| ACCOUNT_IS_NOT_RUNNING | 4 |  |
| NO_OBJECTS_TO_IMPORT | 5 |  |
| IMPORT_IS_CANCELED | 6 |  |



<a name="anytype-Rpc-Object-ImportList-ImportResponse-Type"></a>

### Rpc.Object.ImportList.ImportResponse.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Notion | 0 |  |
| Markdown | 1 |  |
| Html | 2 |  |
| Txt | 3 |  |



<a name="anytype-Rpc-Object-ImportList-Response-Error-Code"></a>

### Rpc.Object.ImportList.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| INTERNAL_ERROR | 1 |  |
| UNKNOWN_ERROR | 2 |  |
| BAD_INPUT | 3 |  |



<a name="anytype-Rpc-Object-ListDelete-Response-Error-Code"></a>

### Rpc.Object.ListDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ListDuplicate-Response-Error-Code"></a>

### Rpc.Object.ListDuplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ListExport-Format"></a>

### Rpc.Object.ListExport.Format


| Name | Number | Description |
| ---- | ------ | ----------- |
| Markdown | 0 |  |
| Protobuf | 1 |  |
| JSON | 2 |  |
| DOT | 3 |  |
| SVG | 4 |  |
| GRAPH_JSON | 5 |  |



<a name="anytype-Rpc-Object-ListExport-Response-Error-Code"></a>

### Rpc.Object.ListExport.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ListSetIsArchived-Response-Error-Code"></a>

### Rpc.Object.ListSetIsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ListSetIsFavorite-Response-Error-Code"></a>

### Rpc.Object.ListSetIsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Open-Response-Error-Code"></a>

### Rpc.Object.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



<a name="anytype-Rpc-Object-OpenBreadcrumbs-Response-Error-Code"></a>

### Rpc.Object.OpenBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Redo-Response-Error-Code"></a>

### Rpc.Object.Redo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype-Rpc-Object-Search-Response-Error-Code"></a>

### Rpc.Object.Search.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SearchSubscribe-Response-Error-Code"></a>

### Rpc.Object.SearchSubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SearchUnsubscribe-Response-Error-Code"></a>

### Rpc.Object.SearchUnsubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-SetBreadcrumbs-Response-Error-Code"></a>

### Rpc.Object.SetBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SetDetails-Response-Error-Code"></a>

### Rpc.Object.SetDetails.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SetInternalFlags-Response-Error-Code"></a>

### Rpc.Object.SetInternalFlags.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype-Rpc-Object-SetIsArchived-Response-Error-Code"></a>

### Rpc.Object.SetIsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SetIsFavorite-Response-Error-Code"></a>

### Rpc.Object.SetIsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SetLayout-Response-Error-Code"></a>

### Rpc.Object.SetLayout.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-SetObjectType-Response-Error-Code"></a>

### Rpc.Object.SetObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype-Rpc-Object-SetSource-Response-Error-Code"></a>

### Rpc.Object.SetSource.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-ShareByLink-Response-Error-Code"></a>

### Rpc.Object.ShareByLink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Show-Response-Error-Code"></a>

### Rpc.Object.Show.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



<a name="anytype-Rpc-Object-SubscribeIds-Response-Error-Code"></a>

### Rpc.Object.SubscribeIds.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ToBookmark-Response-Error-Code"></a>

### Rpc.Object.ToBookmark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ToCollection-Response-Error-Code"></a>

### Rpc.Object.ToCollection.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ToSet-Response-Error-Code"></a>

### Rpc.Object.ToSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Undo-Response-Error-Code"></a>

### Rpc.Object.Undo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype-Rpc-Object-WorkspaceSetDashboard-Response-Error-Code"></a>

### Rpc.Object.WorkspaceSetDashboard.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectCollection-Add-Response-Error-Code"></a>

### Rpc.ObjectCollection.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectCollection-Remove-Response-Error-Code"></a>

### Rpc.ObjectCollection.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectCollection-Sort-Response-Error-Code"></a>

### Rpc.ObjectCollection.Sort.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectRelation-Add-Response-Error-Code"></a>

### Rpc.ObjectRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectRelation-AddFeatured-Response-Error-Code"></a>

### Rpc.ObjectRelation.AddFeatured.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-ObjectRelation-Delete-Response-Error-Code"></a>

### Rpc.ObjectRelation.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectRelation-ListAvailable-Response-Error-Code"></a>

### Rpc.ObjectRelation.ListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-ObjectRelation-RemoveFeatured-Response-Error-Code"></a>

### Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-ObjectType-Relation-Add-Response-Error-Code"></a>

### Rpc.ObjectType.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 | ... |



<a name="anytype-Rpc-ObjectType-Relation-List-Response-Error-Code"></a>

### Rpc.ObjectType.Relation.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 | ... |



<a name="anytype-Rpc-ObjectType-Relation-Remove-Response-Error-Code"></a>

### Rpc.ObjectType.Relation.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 | ... |



<a name="anytype-Rpc-Process-Cancel-Response-Error-Code"></a>

### Rpc.Process.Cancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Relation-ListRemoveOption-Response-Error-Code"></a>

### Rpc.Relation.ListRemoveOption.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| OPTION_USED_BY_OBJECTS | 3 |  |



<a name="anytype-Rpc-Relation-Options-Response-Error-Code"></a>

### Rpc.Relation.Options.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Template-Clone-Response-Error-Code"></a>

### Rpc.Template.Clone.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Template-CreateFromObject-Response-Error-Code"></a>

### Rpc.Template.CreateFromObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Template-CreateFromObjectType-Response-Error-Code"></a>

### Rpc.Template.CreateFromObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Template-ExportAll-Response-Error-Code"></a>

### Rpc.Template.ExportAll.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Unsplash-Download-Response-Error-Code"></a>

### Rpc.Unsplash.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 | ... |



<a name="anytype-Rpc-Unsplash-Search-Response-Error-Code"></a>

### Rpc.Unsplash.Search.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 | ... |



<a name="anytype-Rpc-UserData-Dump-Response-Error-Code"></a>

### Rpc.UserData.Dump.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Wallet-CloseSession-Response-Error-Code"></a>

### Rpc.Wallet.CloseSession.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Wallet-Convert-Response-Error-Code"></a>

### Rpc.Wallet.Convert.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; wallet successfully recovered |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | mnemonic is wrong |



<a name="anytype-Rpc-Wallet-Create-Response-Error-Code"></a>

### Rpc.Wallet.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; mnemonic should be non-empty |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Root path is wrong |
| FAILED_TO_CREATE_LOCAL_REPO | 101 | ... |



<a name="anytype-Rpc-Wallet-CreateSession-Response-Error-Code"></a>

### Rpc.Wallet.CreateSession.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Wallet-Recover-Response-Error-Code"></a>

### Rpc.Wallet.Recover.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; wallet successfully recovered |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Root path or mnemonic is wrong |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |



<a name="anytype-Rpc-Workspace-Create-Response-Error-Code"></a>

### Rpc.Workspace.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-Export-Response-Error-Code"></a>

### Rpc.Workspace.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-GetAll-Response-Error-Code"></a>

### Rpc.Workspace.GetAll.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-GetCurrent-Response-Error-Code"></a>

### Rpc.Workspace.GetCurrent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-Object-Add-Response-Error-Code"></a>

### Rpc.Workspace.Object.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-Object-ListAdd-Response-Error-Code"></a>

### Rpc.Workspace.Object.ListAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-Object-ListRemove-Response-Error-Code"></a>

### Rpc.Workspace.Object.ListRemove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-Select-Response-Error-Code"></a>

### Rpc.Workspace.Select.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-SetIsHighlighted-Response-Error-Code"></a>

### Rpc.Workspace.SetIsHighlighted.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |


 


<a name="pb_protos_commands-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| no_auth | bool | .google.protobuf.MessageOptions | 7777 |  |

 

 



<a name="pb_protos_events-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/events.proto



<a name="anytype-Event"></a>

### Event
Event – type of message, that could be sent from a middleware to the corresponding front-end.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype-Event-Message) | repeated |  |
| contextId | [string](#string) |  |  |
| initiator | [model.Account](#anytype-model-Account) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype-Event-Account"></a>

### Event.Account







<a name="anytype-Event-Account-Config"></a>

### Event.Account.Config







<a name="anytype-Event-Account-Config-Update"></a>

### Event.Account.Config.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [model.Account.Config](#anytype-model-Account-Config) |  |  |
| status | [model.Account.Status](#anytype-model-Account-Status) |  |  |






<a name="anytype-Event-Account-Details"></a>

### Event.Account.Details



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| profileId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Event-Account-Show"></a>

### Event.Account.Show
Message, that will be sent to the front on each account found after an AccountRecoverRequest


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  | Number of an account in an all found accounts list |
| account | [model.Account](#anytype-model-Account) |  | An Account, that has been found for the mnemonic |






<a name="anytype-Event-Account-Update"></a>

### Event.Account.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [model.Account.Config](#anytype-model-Account-Config) |  |  |
| status | [model.Account.Status](#anytype-model-Account-Status) |  |  |






<a name="anytype-Event-Block"></a>

### Event.Block







<a name="anytype-Event-Block-Add"></a>

### Event.Block.Add
Event to show internal blocks on a client.
Example Scenarios
A. Block Creation
1. Block A have been created on a client C1
2. Client C2 receives Event.Block.Add(Block A), Event.Block.Update(Page.children)
B. Partial block load
1. Client C1 opens Page1, that contains, for example, 133 blocks.
2. M -&gt; F: ShowFullScreen(Root, blocks1-50)
3. M -&gt; F: Block.Add(blocks51-100)
3. M -&gt; F: Block.Add(blocks101-133)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [model.Block](#anytype-model-Block) | repeated | id -&gt; block |






<a name="anytype-Event-Block-Dataview"></a>

### Event.Block.Dataview







<a name="anytype-Event-Block-Dataview-GroupOrderUpdate"></a>

### Event.Block.Dataview.GroupOrderUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| groupOrder | [model.Block.Content.Dataview.GroupOrder](#anytype-model-Block-Content-Dataview-GroupOrder) |  |  |






<a name="anytype-Event-Block-Dataview-IsCollectionSet"></a>

### Event.Block.Dataview.IsCollectionSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| value | [bool](#bool) |  |  |






<a name="anytype-Event-Block-Dataview-ObjectOrderUpdate"></a>

### Event.Block.Dataview.ObjectOrderUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  |  |
| groupId | [string](#string) |  |  |
| sliceChanges | [Event.Block.Dataview.SliceChange](#anytype-Event-Block-Dataview-SliceChange) | repeated |  |






<a name="anytype-Event-Block-Dataview-OldRelationDelete"></a>

### Event.Block.Dataview.OldRelationDelete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| relationKey | [string](#string) |  | relation key to remove |






<a name="anytype-Event-Block-Dataview-OldRelationSet"></a>

### Event.Block.Dataview.OldRelationSet
sent when the dataview relation has been changed or added


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| relationKey | [string](#string) |  | relation key to update |
| relation | [model.Relation](#anytype-model-Relation) |  |  |






<a name="anytype-Event-Block-Dataview-RelationDelete"></a>

### Event.Block.Dataview.RelationDelete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| relationKeys | [string](#string) | repeated | relation key to remove |






<a name="anytype-Event-Block-Dataview-RelationSet"></a>

### Event.Block.Dataview.RelationSet
sent when the dataview relation has been changed or added


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| relationLinks | [model.RelationLink](#anytype-model-RelationLink) | repeated | relation id to update |






<a name="anytype-Event-Block-Dataview-SliceChange"></a>

### Event.Block.Dataview.SliceChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| op | [Event.Block.Dataview.SliceOperation](#anytype-Event-Block-Dataview-SliceOperation) |  |  |
| ids | [string](#string) | repeated |  |
| afterId | [string](#string) |  |  |






<a name="anytype-Event-Block-Dataview-SourceSet"></a>

### Event.Block.Dataview.SourceSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| source | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-TargetObjectIdSet"></a>

### Event.Block.Dataview.TargetObjectIdSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| targetObjectId | [string](#string) |  |  |






<a name="anytype-Event-Block-Dataview-ViewDelete"></a>

### Event.Block.Dataview.ViewDelete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id to remove |






<a name="anytype-Event-Block-Dataview-ViewOrder"></a>

### Event.Block.Dataview.ViewOrder



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewIds | [string](#string) | repeated | view ids in new order |






<a name="anytype-Event-Block-Dataview-ViewSet"></a>

### Event.Block.Dataview.ViewSet
sent when the view have been changed or added


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id, client should double check this to make sure client doesn&#39;t switch the active view in the middle |
| view | [model.Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate"></a>

### Event.Block.Dataview.ViewUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| viewId | [string](#string) |  |  |
| filter | [Event.Block.Dataview.ViewUpdate.Filter](#anytype-Event-Block-Dataview-ViewUpdate-Filter) | repeated |  |
| relation | [Event.Block.Dataview.ViewUpdate.Relation](#anytype-Event-Block-Dataview-ViewUpdate-Relation) | repeated |  |
| sort | [Event.Block.Dataview.ViewUpdate.Sort](#anytype-Event-Block-Dataview-ViewUpdate-Sort) | repeated |  |
| fields | [Event.Block.Dataview.ViewUpdate.Fields](#anytype-Event-Block-Dataview-ViewUpdate-Fields) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Fields"></a>

### Event.Block.Dataview.ViewUpdate.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [model.Block.Content.Dataview.View.Type](#anytype-model-Block-Content-Dataview-View-Type) |  |  |
| name | [string](#string) |  |  |
| coverRelationKey | [string](#string) |  | Relation used for cover in gallery |
| hideIcon | [bool](#bool) |  | Hide icon near name |
| cardSize | [model.Block.Content.Dataview.View.Size](#anytype-model-Block-Content-Dataview-View-Size) |  | Gallery card size |
| coverFit | [bool](#bool) |  | Image fits container |
| groupRelationKey | [string](#string) |  | Group view by this relationKey |
| groupBackgroundColors | [bool](#bool) |  | Enable backgrounds in groups |
| pageLimit | [int32](#int32) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Filter"></a>

### Event.Block.Dataview.ViewUpdate.Filter



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| add | [Event.Block.Dataview.ViewUpdate.Filter.Add](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Add) |  |  |
| remove | [Event.Block.Dataview.ViewUpdate.Filter.Remove](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Remove) |  |  |
| update | [Event.Block.Dataview.ViewUpdate.Filter.Update](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Update) |  |  |
| move | [Event.Block.Dataview.ViewUpdate.Filter.Move](#anytype-Event-Block-Dataview-ViewUpdate-Filter-Move) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Filter-Add"></a>

### Event.Block.Dataview.ViewUpdate.Filter.Add



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| items | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Filter-Move"></a>

### Event.Block.Dataview.ViewUpdate.Filter.Move



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Filter-Remove"></a>

### Event.Block.Dataview.ViewUpdate.Filter.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Filter-Update"></a>

### Event.Block.Dataview.ViewUpdate.Filter.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| item | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Relation"></a>

### Event.Block.Dataview.ViewUpdate.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| add | [Event.Block.Dataview.ViewUpdate.Relation.Add](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Add) |  |  |
| remove | [Event.Block.Dataview.ViewUpdate.Relation.Remove](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Remove) |  |  |
| update | [Event.Block.Dataview.ViewUpdate.Relation.Update](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Update) |  |  |
| move | [Event.Block.Dataview.ViewUpdate.Relation.Move](#anytype-Event-Block-Dataview-ViewUpdate-Relation-Move) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Relation-Add"></a>

### Event.Block.Dataview.ViewUpdate.Relation.Add



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| items | [model.Block.Content.Dataview.Relation](#anytype-model-Block-Content-Dataview-Relation) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Relation-Move"></a>

### Event.Block.Dataview.ViewUpdate.Relation.Move



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Relation-Remove"></a>

### Event.Block.Dataview.ViewUpdate.Relation.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Relation-Update"></a>

### Event.Block.Dataview.ViewUpdate.Relation.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| item | [model.Block.Content.Dataview.Relation](#anytype-model-Block-Content-Dataview-Relation) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Sort"></a>

### Event.Block.Dataview.ViewUpdate.Sort



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| add | [Event.Block.Dataview.ViewUpdate.Sort.Add](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Add) |  |  |
| remove | [Event.Block.Dataview.ViewUpdate.Sort.Remove](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Remove) |  |  |
| update | [Event.Block.Dataview.ViewUpdate.Sort.Update](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Update) |  |  |
| move | [Event.Block.Dataview.ViewUpdate.Sort.Move](#anytype-Event-Block-Dataview-ViewUpdate-Sort-Move) |  |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Sort-Add"></a>

### Event.Block.Dataview.ViewUpdate.Sort.Add



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| items | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Sort-Move"></a>

### Event.Block.Dataview.ViewUpdate.Sort.Move



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| afterId | [string](#string) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Sort-Remove"></a>

### Event.Block.Dataview.ViewUpdate.Sort.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Dataview-ViewUpdate-Sort-Update"></a>

### Event.Block.Dataview.ViewUpdate.Sort.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| item | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) |  |  |






<a name="anytype-Event-Block-Delete"></a>

### Event.Block.Delete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockIds | [string](#string) | repeated |  |






<a name="anytype-Event-Block-FilesUpload"></a>

### Event.Block.FilesUpload
Middleware to front end event message, that will be sent on one of this scenarios:
Precondition: user A opened a block
1. User A drops a set of files/pictures/videos
2. User A creates a MediaBlock and drops a single media, that corresponds to its type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  | if empty =&gt; create new blocks |
| filePath | [string](#string) | repeated | filepaths to the files |






<a name="anytype-Event-Block-Fill"></a>

### Event.Block.Fill







<a name="anytype-Event-Block-Fill-Align"></a>

### Event.Block.Fill.Align



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| align | [model.Block.Align](#anytype-model-Block-Align) |  |  |






<a name="anytype-Event-Block-Fill-BackgroundColor"></a>

### Event.Block.Fill.BackgroundColor



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| backgroundColor | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark"></a>

### Event.Block.Fill.Bookmark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [Event.Block.Fill.Bookmark.Url](#anytype-Event-Block-Fill-Bookmark-Url) |  |  |
| title | [Event.Block.Fill.Bookmark.Title](#anytype-Event-Block-Fill-Bookmark-Title) |  |  |
| description | [Event.Block.Fill.Bookmark.Description](#anytype-Event-Block-Fill-Bookmark-Description) |  |  |
| imageHash | [Event.Block.Fill.Bookmark.ImageHash](#anytype-Event-Block-Fill-Bookmark-ImageHash) |  |  |
| faviconHash | [Event.Block.Fill.Bookmark.FaviconHash](#anytype-Event-Block-Fill-Bookmark-FaviconHash) |  |  |
| type | [Event.Block.Fill.Bookmark.Type](#anytype-Event-Block-Fill-Bookmark-Type) |  |  |
| targetObjectId | [Event.Block.Fill.Bookmark.TargetObjectId](#anytype-Event-Block-Fill-Bookmark-TargetObjectId) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-Description"></a>

### Event.Block.Fill.Bookmark.Description



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-FaviconHash"></a>

### Event.Block.Fill.Bookmark.FaviconHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-ImageHash"></a>

### Event.Block.Fill.Bookmark.ImageHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-TargetObjectId"></a>

### Event.Block.Fill.Bookmark.TargetObjectId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-Title"></a>

### Event.Block.Fill.Bookmark.Title



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-Type"></a>

### Event.Block.Fill.Bookmark.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.LinkPreview.Type](#anytype-model-LinkPreview-Type) |  |  |






<a name="anytype-Event-Block-Fill-Bookmark-Url"></a>

### Event.Block.Fill.Bookmark.Url



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-ChildrenIds"></a>

### Event.Block.Fill.ChildrenIds



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| childrenIds | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Fill-DatabaseRecords"></a>

### Event.Block.Fill.DatabaseRecords



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| records | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |






<a name="anytype-Event-Block-Fill-Details"></a>

### Event.Block.Fill.Details



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Event-Block-Fill-Div"></a>

### Event.Block.Fill.Div



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| style | [Event.Block.Fill.Div.Style](#anytype-Event-Block-Fill-Div-Style) |  |  |






<a name="anytype-Event-Block-Fill-Div-Style"></a>

### Event.Block.Fill.Div.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style) |  |  |






<a name="anytype-Event-Block-Fill-Fields"></a>

### Event.Block.Fill.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Event-Block-Fill-File"></a>

### Event.Block.Fill.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Event.Block.Fill.File.Type](#anytype-Event-Block-Fill-File-Type) |  |  |
| state | [Event.Block.Fill.File.State](#anytype-Event-Block-Fill-File-State) |  |  |
| mime | [Event.Block.Fill.File.Mime](#anytype-Event-Block-Fill-File-Mime) |  |  |
| hash | [Event.Block.Fill.File.Hash](#anytype-Event-Block-Fill-File-Hash) |  |  |
| name | [Event.Block.Fill.File.Name](#anytype-Event-Block-Fill-File-Name) |  |  |
| size | [Event.Block.Fill.File.Size](#anytype-Event-Block-Fill-File-Size) |  |  |
| style | [Event.Block.Fill.File.Style](#anytype-Event-Block-Fill-File-Style) |  |  |






<a name="anytype-Event-Block-Fill-File-Hash"></a>

### Event.Block.Fill.File.Hash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-File-Mime"></a>

### Event.Block.Fill.File.Mime



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-File-Name"></a>

### Event.Block.Fill.File.Name



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-File-Size"></a>

### Event.Block.Fill.File.Size



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int64](#int64) |  |  |






<a name="anytype-Event-Block-Fill-File-State"></a>

### Event.Block.Fill.File.State



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.State](#anytype-model-Block-Content-File-State) |  |  |






<a name="anytype-Event-Block-Fill-File-Style"></a>

### Event.Block.Fill.File.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |






<a name="anytype-Event-Block-Fill-File-Type"></a>

### Event.Block.Fill.File.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Type](#anytype-model-Block-Content-File-Type) |  |  |






<a name="anytype-Event-Block-Fill-File-Width"></a>

### Event.Block.Fill.File.Width



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) |  |  |






<a name="anytype-Event-Block-Fill-Link"></a>

### Event.Block.Fill.Link



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| targetBlockId | [Event.Block.Fill.Link.TargetBlockId](#anytype-Event-Block-Fill-Link-TargetBlockId) |  |  |
| style | [Event.Block.Fill.Link.Style](#anytype-Event-Block-Fill-Link-Style) |  |  |
| fields | [Event.Block.Fill.Link.Fields](#anytype-Event-Block-Fill-Link-Fields) |  |  |






<a name="anytype-Event-Block-Fill-Link-Fields"></a>

### Event.Block.Fill.Link.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Event-Block-Fill-Link-Style"></a>

### Event.Block.Fill.Link.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.Style](#anytype-model-Block-Content-Link-Style) |  |  |






<a name="anytype-Event-Block-Fill-Link-TargetBlockId"></a>

### Event.Block.Fill.Link.TargetBlockId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Restrictions"></a>

### Event.Block.Fill.Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| restrictions | [model.Block.Restrictions](#anytype-model-Block-Restrictions) |  |  |






<a name="anytype-Event-Block-Fill-Text"></a>

### Event.Block.Fill.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [Event.Block.Fill.Text.Text](#anytype-Event-Block-Fill-Text-Text) |  |  |
| style | [Event.Block.Fill.Text.Style](#anytype-Event-Block-Fill-Text-Style) |  |  |
| marks | [Event.Block.Fill.Text.Marks](#anytype-Event-Block-Fill-Text-Marks) |  |  |
| checked | [Event.Block.Fill.Text.Checked](#anytype-Event-Block-Fill-Text-Checked) |  |  |
| color | [Event.Block.Fill.Text.Color](#anytype-Event-Block-Fill-Text-Color) |  |  |






<a name="anytype-Event-Block-Fill-Text-Checked"></a>

### Event.Block.Fill.Text.Checked



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) |  |  |






<a name="anytype-Event-Block-Fill-Text-Color"></a>

### Event.Block.Fill.Text.Color



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Fill-Text-Marks"></a>

### Event.Block.Fill.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Marks](#anytype-model-Block-Content-Text-Marks) |  |  |






<a name="anytype-Event-Block-Fill-Text-Style"></a>

### Event.Block.Fill.Text.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |






<a name="anytype-Event-Block-Fill-Text-Text"></a>

### Event.Block.Fill.Text.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-MarksInfo"></a>

### Event.Block.MarksInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marksInRange | [model.Block.Content.Text.Mark.Type](#anytype-model-Block-Content-Text-Mark-Type) | repeated |  |






<a name="anytype-Event-Block-Set"></a>

### Event.Block.Set







<a name="anytype-Event-Block-Set-Align"></a>

### Event.Block.Set.Align



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| align | [model.Block.Align](#anytype-model-Block-Align) |  |  |






<a name="anytype-Event-Block-Set-BackgroundColor"></a>

### Event.Block.Set.BackgroundColor



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| backgroundColor | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Bookmark"></a>

### Event.Block.Set.Bookmark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [Event.Block.Set.Bookmark.Url](#anytype-Event-Block-Set-Bookmark-Url) |  |  |
| title | [Event.Block.Set.Bookmark.Title](#anytype-Event-Block-Set-Bookmark-Title) |  |  |
| description | [Event.Block.Set.Bookmark.Description](#anytype-Event-Block-Set-Bookmark-Description) |  |  |
| imageHash | [Event.Block.Set.Bookmark.ImageHash](#anytype-Event-Block-Set-Bookmark-ImageHash) |  |  |
| faviconHash | [Event.Block.Set.Bookmark.FaviconHash](#anytype-Event-Block-Set-Bookmark-FaviconHash) |  |  |
| type | [Event.Block.Set.Bookmark.Type](#anytype-Event-Block-Set-Bookmark-Type) |  |  |
| targetObjectId | [Event.Block.Set.Bookmark.TargetObjectId](#anytype-Event-Block-Set-Bookmark-TargetObjectId) |  |  |
| state | [Event.Block.Set.Bookmark.State](#anytype-Event-Block-Set-Bookmark-State) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-Description"></a>

### Event.Block.Set.Bookmark.Description



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-FaviconHash"></a>

### Event.Block.Set.Bookmark.FaviconHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-ImageHash"></a>

### Event.Block.Set.Bookmark.ImageHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-State"></a>

### Event.Block.Set.Bookmark.State



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Bookmark.State](#anytype-model-Block-Content-Bookmark-State) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-TargetObjectId"></a>

### Event.Block.Set.Bookmark.TargetObjectId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-Title"></a>

### Event.Block.Set.Bookmark.Title



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-Type"></a>

### Event.Block.Set.Bookmark.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.LinkPreview.Type](#anytype-model-LinkPreview-Type) |  |  |






<a name="anytype-Event-Block-Set-Bookmark-Url"></a>

### Event.Block.Set.Bookmark.Url



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-ChildrenIds"></a>

### Event.Block.Set.ChildrenIds



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| childrenIds | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Set-Div"></a>

### Event.Block.Set.Div



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| style | [Event.Block.Set.Div.Style](#anytype-Event-Block-Set-Div-Style) |  |  |






<a name="anytype-Event-Block-Set-Div-Style"></a>

### Event.Block.Set.Div.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style) |  |  |






<a name="anytype-Event-Block-Set-Fields"></a>

### Event.Block.Set.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Event-Block-Set-File"></a>

### Event.Block.Set.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Event.Block.Set.File.Type](#anytype-Event-Block-Set-File-Type) |  |  |
| state | [Event.Block.Set.File.State](#anytype-Event-Block-Set-File-State) |  |  |
| mime | [Event.Block.Set.File.Mime](#anytype-Event-Block-Set-File-Mime) |  |  |
| hash | [Event.Block.Set.File.Hash](#anytype-Event-Block-Set-File-Hash) |  |  |
| name | [Event.Block.Set.File.Name](#anytype-Event-Block-Set-File-Name) |  |  |
| size | [Event.Block.Set.File.Size](#anytype-Event-Block-Set-File-Size) |  |  |
| style | [Event.Block.Set.File.Style](#anytype-Event-Block-Set-File-Style) |  |  |






<a name="anytype-Event-Block-Set-File-Hash"></a>

### Event.Block.Set.File.Hash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-File-Mime"></a>

### Event.Block.Set.File.Mime



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-File-Name"></a>

### Event.Block.Set.File.Name



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-File-Size"></a>

### Event.Block.Set.File.Size



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int64](#int64) |  |  |






<a name="anytype-Event-Block-Set-File-State"></a>

### Event.Block.Set.File.State



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.State](#anytype-model-Block-Content-File-State) |  |  |






<a name="anytype-Event-Block-Set-File-Style"></a>

### Event.Block.Set.File.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |






<a name="anytype-Event-Block-Set-File-Type"></a>

### Event.Block.Set.File.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Type](#anytype-model-Block-Content-File-Type) |  |  |






<a name="anytype-Event-Block-Set-File-Width"></a>

### Event.Block.Set.File.Width



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) |  |  |






<a name="anytype-Event-Block-Set-Latex"></a>

### Event.Block.Set.Latex



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [Event.Block.Set.Latex.Text](#anytype-Event-Block-Set-Latex-Text) |  |  |






<a name="anytype-Event-Block-Set-Latex-Text"></a>

### Event.Block.Set.Latex.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Link"></a>

### Event.Block.Set.Link



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| targetBlockId | [Event.Block.Set.Link.TargetBlockId](#anytype-Event-Block-Set-Link-TargetBlockId) |  |  |
| style | [Event.Block.Set.Link.Style](#anytype-Event-Block-Set-Link-Style) |  |  |
| fields | [Event.Block.Set.Link.Fields](#anytype-Event-Block-Set-Link-Fields) |  |  |
| iconSize | [Event.Block.Set.Link.IconSize](#anytype-Event-Block-Set-Link-IconSize) |  |  |
| cardStyle | [Event.Block.Set.Link.CardStyle](#anytype-Event-Block-Set-Link-CardStyle) |  |  |
| description | [Event.Block.Set.Link.Description](#anytype-Event-Block-Set-Link-Description) |  |  |
| relations | [Event.Block.Set.Link.Relations](#anytype-Event-Block-Set-Link-Relations) |  |  |






<a name="anytype-Event-Block-Set-Link-CardStyle"></a>

### Event.Block.Set.Link.CardStyle



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.CardStyle](#anytype-model-Block-Content-Link-CardStyle) |  |  |






<a name="anytype-Event-Block-Set-Link-Description"></a>

### Event.Block.Set.Link.Description



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.Description](#anytype-model-Block-Content-Link-Description) |  |  |






<a name="anytype-Event-Block-Set-Link-Fields"></a>

### Event.Block.Set.Link.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Event-Block-Set-Link-IconSize"></a>

### Event.Block.Set.Link.IconSize



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.IconSize](#anytype-model-Block-Content-Link-IconSize) |  |  |






<a name="anytype-Event-Block-Set-Link-Relations"></a>

### Event.Block.Set.Link.Relations



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | repeated |  |






<a name="anytype-Event-Block-Set-Link-Style"></a>

### Event.Block.Set.Link.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.Style](#anytype-model-Block-Content-Link-Style) |  |  |






<a name="anytype-Event-Block-Set-Link-TargetBlockId"></a>

### Event.Block.Set.Link.TargetBlockId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Relation"></a>

### Event.Block.Set.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| key | [Event.Block.Set.Relation.Key](#anytype-Event-Block-Set-Relation-Key) |  |  |






<a name="anytype-Event-Block-Set-Relation-Key"></a>

### Event.Block.Set.Relation.Key



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Restrictions"></a>

### Event.Block.Set.Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| restrictions | [model.Block.Restrictions](#anytype-model-Block-Restrictions) |  |  |






<a name="anytype-Event-Block-Set-TableRow"></a>

### Event.Block.Set.TableRow



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| isHeader | [Event.Block.Set.TableRow.IsHeader](#anytype-Event-Block-Set-TableRow-IsHeader) |  |  |






<a name="anytype-Event-Block-Set-TableRow-IsHeader"></a>

### Event.Block.Set.TableRow.IsHeader



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) |  |  |






<a name="anytype-Event-Block-Set-Text"></a>

### Event.Block.Set.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [Event.Block.Set.Text.Text](#anytype-Event-Block-Set-Text-Text) |  |  |
| style | [Event.Block.Set.Text.Style](#anytype-Event-Block-Set-Text-Style) |  |  |
| marks | [Event.Block.Set.Text.Marks](#anytype-Event-Block-Set-Text-Marks) |  |  |
| checked | [Event.Block.Set.Text.Checked](#anytype-Event-Block-Set-Text-Checked) |  |  |
| color | [Event.Block.Set.Text.Color](#anytype-Event-Block-Set-Text-Color) |  |  |
| iconEmoji | [Event.Block.Set.Text.IconEmoji](#anytype-Event-Block-Set-Text-IconEmoji) |  |  |
| iconImage | [Event.Block.Set.Text.IconImage](#anytype-Event-Block-Set-Text-IconImage) |  |  |






<a name="anytype-Event-Block-Set-Text-Checked"></a>

### Event.Block.Set.Text.Checked



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) |  |  |






<a name="anytype-Event-Block-Set-Text-Color"></a>

### Event.Block.Set.Text.Color



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Text-IconEmoji"></a>

### Event.Block.Set.Text.IconEmoji



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Text-IconImage"></a>

### Event.Block.Set.Text.IconImage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-Text-Marks"></a>

### Event.Block.Set.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Marks](#anytype-model-Block-Content-Text-Marks) |  |  |






<a name="anytype-Event-Block-Set-Text-Style"></a>

### Event.Block.Set.Text.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |






<a name="anytype-Event-Block-Set-Text-Text"></a>

### Event.Block.Set.Text.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Block-Set-VerticalAlign"></a>

### Event.Block.Set.VerticalAlign



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| verticalAlign | [model.Block.VerticalAlign](#anytype-model-Block-VerticalAlign) |  |  |






<a name="anytype-Event-Block-Set-Widget"></a>

### Event.Block.Set.Widget



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| layout | [Event.Block.Set.Widget.Layout](#anytype-Event-Block-Set-Widget-Layout) |  |  |
| limit | [Event.Block.Set.Widget.Limit](#anytype-Event-Block-Set-Widget-Limit) |  |  |






<a name="anytype-Event-Block-Set-Widget-Layout"></a>

### Event.Block.Set.Widget.Layout



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Widget.Layout](#anytype-model-Block-Content-Widget-Layout) |  |  |






<a name="anytype-Event-Block-Set-Widget-Limit"></a>

### Event.Block.Set.Widget.Limit



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) |  |  |






<a name="anytype-Event-File"></a>

### Event.File







<a name="anytype-Event-File-LimitReached"></a>

### Event.File.LimitReached



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| fileId | [string](#string) |  |  |






<a name="anytype-Event-File-LocalUsage"></a>

### Event.File.LocalUsage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| localBytesUsage | [uint64](#uint64) |  |  |






<a name="anytype-Event-File-SpaceUsage"></a>

### Event.File.SpaceUsage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| bytesUsage | [uint64](#uint64) |  |  |






<a name="anytype-Event-Message"></a>

### Event.Message



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountShow | [Event.Account.Show](#anytype-Event-Account-Show) |  |  |
| accountDetails | [Event.Account.Details](#anytype-Event-Account-Details) |  |  |
| accountConfigUpdate | [Event.Account.Config.Update](#anytype-Event-Account-Config-Update) |  |  |
| accountUpdate | [Event.Account.Update](#anytype-Event-Account-Update) |  |  |
| objectDetailsSet | [Event.Object.Details.Set](#anytype-Event-Object-Details-Set) |  |  |
| objectDetailsAmend | [Event.Object.Details.Amend](#anytype-Event-Object-Details-Amend) |  |  |
| objectDetailsUnset | [Event.Object.Details.Unset](#anytype-Event-Object-Details-Unset) |  |  |
| objectRelationsAmend | [Event.Object.Relations.Amend](#anytype-Event-Object-Relations-Amend) |  |  |
| objectRelationsRemove | [Event.Object.Relations.Remove](#anytype-Event-Object-Relations-Remove) |  |  |
| objectRemove | [Event.Object.Remove](#anytype-Event-Object-Remove) |  |  |
| objectRestrictionsSet | [Event.Object.Restrictions.Set](#anytype-Event-Object-Restrictions-Set) |  |  |
| subscriptionAdd | [Event.Object.Subscription.Add](#anytype-Event-Object-Subscription-Add) |  |  |
| subscriptionRemove | [Event.Object.Subscription.Remove](#anytype-Event-Object-Subscription-Remove) |  |  |
| subscriptionPosition | [Event.Object.Subscription.Position](#anytype-Event-Object-Subscription-Position) |  |  |
| subscriptionCounters | [Event.Object.Subscription.Counters](#anytype-Event-Object-Subscription-Counters) |  |  |
| subscriptionGroups | [Event.Object.Subscription.Groups](#anytype-Event-Object-Subscription-Groups) |  |  |
| blockAdd | [Event.Block.Add](#anytype-Event-Block-Add) |  |  |
| blockDelete | [Event.Block.Delete](#anytype-Event-Block-Delete) |  |  |
| filesUpload | [Event.Block.FilesUpload](#anytype-Event-Block-FilesUpload) |  |  |
| marksInfo | [Event.Block.MarksInfo](#anytype-Event-Block-MarksInfo) |  |  |
| blockSetFields | [Event.Block.Set.Fields](#anytype-Event-Block-Set-Fields) |  |  |
| blockSetChildrenIds | [Event.Block.Set.ChildrenIds](#anytype-Event-Block-Set-ChildrenIds) |  |  |
| blockSetRestrictions | [Event.Block.Set.Restrictions](#anytype-Event-Block-Set-Restrictions) |  |  |
| blockSetBackgroundColor | [Event.Block.Set.BackgroundColor](#anytype-Event-Block-Set-BackgroundColor) |  |  |
| blockSetText | [Event.Block.Set.Text](#anytype-Event-Block-Set-Text) |  |  |
| blockSetFile | [Event.Block.Set.File](#anytype-Event-Block-Set-File) |  |  |
| blockSetLink | [Event.Block.Set.Link](#anytype-Event-Block-Set-Link) |  |  |
| blockSetBookmark | [Event.Block.Set.Bookmark](#anytype-Event-Block-Set-Bookmark) |  |  |
| blockSetAlign | [Event.Block.Set.Align](#anytype-Event-Block-Set-Align) |  |  |
| blockSetDiv | [Event.Block.Set.Div](#anytype-Event-Block-Set-Div) |  |  |
| blockSetRelation | [Event.Block.Set.Relation](#anytype-Event-Block-Set-Relation) |  |  |
| blockSetLatex | [Event.Block.Set.Latex](#anytype-Event-Block-Set-Latex) |  |  |
| blockSetVerticalAlign | [Event.Block.Set.VerticalAlign](#anytype-Event-Block-Set-VerticalAlign) |  |  |
| blockSetTableRow | [Event.Block.Set.TableRow](#anytype-Event-Block-Set-TableRow) |  |  |
| blockSetWidget | [Event.Block.Set.Widget](#anytype-Event-Block-Set-Widget) |  |  |
| blockDataviewViewSet | [Event.Block.Dataview.ViewSet](#anytype-Event-Block-Dataview-ViewSet) |  |  |
| blockDataviewViewDelete | [Event.Block.Dataview.ViewDelete](#anytype-Event-Block-Dataview-ViewDelete) |  |  |
| blockDataviewViewOrder | [Event.Block.Dataview.ViewOrder](#anytype-Event-Block-Dataview-ViewOrder) |  |  |
| blockDataviewSourceSet | [Event.Block.Dataview.SourceSet](#anytype-Event-Block-Dataview-SourceSet) |  |  |
| blockDataViewGroupOrderUpdate | [Event.Block.Dataview.GroupOrderUpdate](#anytype-Event-Block-Dataview-GroupOrderUpdate) |  |  |
| blockDataViewObjectOrderUpdate | [Event.Block.Dataview.ObjectOrderUpdate](#anytype-Event-Block-Dataview-ObjectOrderUpdate) |  |  |
| blockDataviewRelationDelete | [Event.Block.Dataview.RelationDelete](#anytype-Event-Block-Dataview-RelationDelete) |  |  |
| blockDataviewRelationSet | [Event.Block.Dataview.RelationSet](#anytype-Event-Block-Dataview-RelationSet) |  |  |
| blockDataviewViewUpdate | [Event.Block.Dataview.ViewUpdate](#anytype-Event-Block-Dataview-ViewUpdate) |  |  |
| blockDataviewTargetObjectIdSet | [Event.Block.Dataview.TargetObjectIdSet](#anytype-Event-Block-Dataview-TargetObjectIdSet) |  |  |
| blockDataviewIsCollectionSet | [Event.Block.Dataview.IsCollectionSet](#anytype-Event-Block-Dataview-IsCollectionSet) |  |  |
| blockDataviewOldRelationDelete | [Event.Block.Dataview.OldRelationDelete](#anytype-Event-Block-Dataview-OldRelationDelete) |  | deprecated |
| blockDataviewOldRelationSet | [Event.Block.Dataview.OldRelationSet](#anytype-Event-Block-Dataview-OldRelationSet) |  | deprecated |
| userBlockJoin | [Event.User.Block.Join](#anytype-Event-User-Block-Join) |  |  |
| userBlockLeft | [Event.User.Block.Left](#anytype-Event-User-Block-Left) |  |  |
| userBlockSelectRange | [Event.User.Block.SelectRange](#anytype-Event-User-Block-SelectRange) |  |  |
| userBlockTextRange | [Event.User.Block.TextRange](#anytype-Event-User-Block-TextRange) |  |  |
| ping | [Event.Ping](#anytype-Event-Ping) |  |  |
| processNew | [Event.Process.New](#anytype-Event-Process-New) |  |  |
| processUpdate | [Event.Process.Update](#anytype-Event-Process-Update) |  |  |
| processDone | [Event.Process.Done](#anytype-Event-Process-Done) |  |  |
| threadStatus | [Event.Status.Thread](#anytype-Event-Status-Thread) |  |  |
| fileLimitReached | [Event.File.LimitReached](#anytype-Event-File-LimitReached) |  |  |
| fileSpaceUsage | [Event.File.SpaceUsage](#anytype-Event-File-SpaceUsage) |  |  |
| fileLocalUsage | [Event.File.LocalUsage](#anytype-Event-File-LocalUsage) |  |  |






<a name="anytype-Event-Object"></a>

### Event.Object







<a name="anytype-Event-Object-Details"></a>

### Event.Object.Details







<a name="anytype-Event-Object-Details-Amend"></a>

### Event.Object.Details.Amend
Amend (i.e. add a new key-value pair or update an existing key-value pair) existing state


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| details | [Event.Object.Details.Amend.KeyValue](#anytype-Event-Object-Details-Amend-KeyValue) | repeated | slice of changed key-values |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Object-Details-Amend-KeyValue"></a>

### Event.Object.Details.Amend.KeyValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  | should not be null |






<a name="anytype-Event-Object-Details-Set"></a>

### Event.Object.Details.Set
Overwrite current state


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | can not be a partial state. Should replace client details state |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Object-Details-Unset"></a>

### Event.Object.Details.Unset
Unset existing detail keys


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| keys | [string](#string) | repeated |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Object-Relations"></a>

### Event.Object.Relations







<a name="anytype-Event-Object-Relations-Amend"></a>

### Event.Object.Relations.Amend



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| relationLinks | [model.RelationLink](#anytype-model-RelationLink) | repeated |  |






<a name="anytype-Event-Object-Relations-Remove"></a>

### Event.Object.Relations.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Event-Object-Remove"></a>

### Event.Object.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated | notifies that objects were removed |






<a name="anytype-Event-Object-Restrictions"></a>

### Event.Object.Restrictions







<a name="anytype-Event-Object-Restrictions-Set"></a>

### Event.Object.Restrictions.Set



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| restrictions | [model.Restrictions](#anytype-model-Restrictions) |  |  |






<a name="anytype-Event-Object-Subscription"></a>

### Event.Object.Subscription







<a name="anytype-Event-Object-Subscription-Add"></a>

### Event.Object.Subscription.Add
Adds new document to subscriptions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | object id |
| afterId | [string](#string) |  | id of previous doc in order, empty means first |
| subId | [string](#string) |  | subscription id |






<a name="anytype-Event-Object-Subscription-Counters"></a>

### Event.Object.Subscription.Counters



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total | [int64](#int64) |  | total available records |
| nextCount | [int64](#int64) |  | how many records available after |
| prevCount | [int64](#int64) |  | how many records available before |
| subId | [string](#string) |  | subscription id |






<a name="anytype-Event-Object-Subscription-Groups"></a>

### Event.Object.Subscription.Groups



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  |  |
| group | [model.Block.Content.Dataview.Group](#anytype-model-Block-Content-Dataview-Group) |  |  |
| remove | [bool](#bool) |  |  |






<a name="anytype-Event-Object-Subscription-Position"></a>

### Event.Object.Subscription.Position
Indicates new position of document


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | object id |
| afterId | [string](#string) |  | id of previous doc in order, empty means first |
| subId | [string](#string) |  | subscription id |






<a name="anytype-Event-Object-Subscription-Remove"></a>

### Event.Object.Subscription.Remove
Removes document from subscription


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | object id |
| subId | [string](#string) |  | subscription id |






<a name="anytype-Event-Ping"></a>

### Event.Ping



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  |  |






<a name="anytype-Event-Process"></a>

### Event.Process







<a name="anytype-Event-Process-Done"></a>

### Event.Process.Done



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype-Model-Process) |  |  |






<a name="anytype-Event-Process-New"></a>

### Event.Process.New



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype-Model-Process) |  |  |






<a name="anytype-Event-Process-Update"></a>

### Event.Process.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype-Model-Process) |  |  |






<a name="anytype-Event-Status"></a>

### Event.Status







<a name="anytype-Event-Status-Thread"></a>

### Event.Status.Thread



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| summary | [Event.Status.Thread.Summary](#anytype-Event-Status-Thread-Summary) |  |  |
| cafe | [Event.Status.Thread.Cafe](#anytype-Event-Status-Thread-Cafe) |  |  |
| accounts | [Event.Status.Thread.Account](#anytype-Event-Status-Thread-Account) | repeated |  |






<a name="anytype-Event-Status-Thread-Account"></a>

### Event.Status.Thread.Account



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| imageHash | [string](#string) |  |  |
| online | [bool](#bool) |  |  |
| lastPulled | [int64](#int64) |  |  |
| lastEdited | [int64](#int64) |  |  |
| devices | [Event.Status.Thread.Device](#anytype-Event-Status-Thread-Device) | repeated |  |






<a name="anytype-Event-Status-Thread-Cafe"></a>

### Event.Status.Thread.Cafe



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Event.Status.Thread.SyncStatus](#anytype-Event-Status-Thread-SyncStatus) |  |  |
| lastPulled | [int64](#int64) |  |  |
| lastPushSucceed | [bool](#bool) |  |  |
| files | [Event.Status.Thread.Cafe.PinStatus](#anytype-Event-Status-Thread-Cafe-PinStatus) |  |  |






<a name="anytype-Event-Status-Thread-Cafe-PinStatus"></a>

### Event.Status.Thread.Cafe.PinStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pinning | [int32](#int32) |  |  |
| pinned | [int32](#int32) |  |  |
| failed | [int32](#int32) |  |  |
| updated | [int64](#int64) |  |  |






<a name="anytype-Event-Status-Thread-Device"></a>

### Event.Status.Thread.Device



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| online | [bool](#bool) |  |  |
| lastPulled | [int64](#int64) |  |  |
| lastEdited | [int64](#int64) |  |  |






<a name="anytype-Event-Status-Thread-Summary"></a>

### Event.Status.Thread.Summary



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Event.Status.Thread.SyncStatus](#anytype-Event-Status-Thread-SyncStatus) |  |  |






<a name="anytype-Event-User"></a>

### Event.User







<a name="anytype-Event-User-Block"></a>

### Event.User.Block







<a name="anytype-Event-User-Block-Join"></a>

### Event.User.Block.Join
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A opened a block
1. User B opens the same block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that opened a block |






<a name="anytype-Event-User-Block-Left"></a>

### Event.User.Block.Left
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B closes the block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that left the block |






<a name="anytype-Event-User-Block-SelectRange"></a>

### Event.User.Block.SelectRange
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B selects some inner blocks
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that selected blocks |
| blockIdsArray | [string](#string) | repeated | Ids of selected blocks. |






<a name="anytype-Event-User-Block-TextRange"></a>

### Event.User.Block.TextRange
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B sets cursor or selects a text region into a text block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that selected a text |
| blockId | [string](#string) |  | Id of the text block, that have a selection |
| range | [model.Range](#anytype-model-Range) |  | Range of the selection |






<a name="anytype-Model"></a>

### Model







<a name="anytype-Model-Process"></a>

### Model.Process



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Model.Process.Type](#anytype-Model-Process-Type) |  |  |
| state | [Model.Process.State](#anytype-Model-Process-State) |  |  |
| progress | [Model.Process.Progress](#anytype-Model-Process-Progress) |  |  |






<a name="anytype-Model-Process-Progress"></a>

### Model.Process.Progress



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total | [int64](#int64) |  |  |
| done | [int64](#int64) |  |  |
| message | [string](#string) |  |  |






<a name="anytype-ResponseEvent"></a>

### ResponseEvent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype-Event-Message) | repeated |  |
| contextId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |





 


<a name="anytype-Event-Block-Dataview-SliceOperation"></a>

### Event.Block.Dataview.SliceOperation


| Name | Number | Description |
| ---- | ------ | ----------- |
| SliceOperationNone | 0 | not used |
| SliceOperationAdd | 1 |  |
| SliceOperationMove | 2 |  |
| SliceOperationRemove | 3 |  |
| SliceOperationReplace | 4 |  |



<a name="anytype-Event-Status-Thread-SyncStatus"></a>

### Event.Status.Thread.SyncStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Offline | 1 |  |
| Syncing | 2 |  |
| Synced | 3 |  |
| Failed | 4 |  |



<a name="anytype-Model-Process-State"></a>

### Model.Process.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Running | 1 |  |
| Done | 2 |  |
| Canceled | 3 |  |
| Error | 4 |  |



<a name="anytype-Model-Process-Type"></a>

### Model.Process.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| DropFiles | 0 |  |
| Import | 1 |  |
| Export | 2 |  |
| SaveFile | 3 |  |
| RecoverAccount | 4 |  |
| Migration | 5 |  |


 

 

 



<a name="pb_protos_snapshot-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/snapshot.proto



<a name="anytype-Profile"></a>

### Profile



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| avatar | [string](#string) |  |  |
| address | [string](#string) |  |  |
| spaceDashboardId | [string](#string) |  |  |
| profileId | [string](#string) |  |  |
| analyticsId | [string](#string) |  |  |






<a name="anytype-SnapshotWithType"></a>

### SnapshotWithType



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sbType | [model.SmartBlockType](#anytype-model-SmartBlockType) |  |  |
| snapshot | [Change.Snapshot](#anytype-Change-Snapshot) |  |  |





 

 

 

 



<a name="pkg_lib_pb_model_protos_localstore-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pkg/lib/pb/model/protos/localstore.proto



<a name="anytype-model-ObjectDetails"></a>

### ObjectDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-model-ObjectInfo"></a>

### ObjectInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| objectTypeUrls | [string](#string) | repeated | deprecated |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| relations | [Relation](#anytype-model-Relation) | repeated |  |
| snippet | [string](#string) |  |  |
| hasInboundLinks | [bool](#bool) |  |  |
| objectType | [SmartBlockType](#anytype-model-SmartBlockType) |  |  |






<a name="anytype-model-ObjectInfoWithLinks"></a>

### ObjectInfoWithLinks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| info | [ObjectInfo](#anytype-model-ObjectInfo) |  |  |
| links | [ObjectLinksInfo](#anytype-model-ObjectLinksInfo) |  |  |






<a name="anytype-model-ObjectInfoWithOutboundLinks"></a>

### ObjectInfoWithOutboundLinks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| info | [ObjectInfo](#anytype-model-ObjectInfo) |  |  |
| outboundLinks | [ObjectInfo](#anytype-model-ObjectInfo) | repeated |  |






<a name="anytype-model-ObjectInfoWithOutboundLinksIDs"></a>

### ObjectInfoWithOutboundLinksIDs



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| info | [ObjectInfo](#anytype-model-ObjectInfo) |  |  |
| outboundLinks | [string](#string) | repeated |  |






<a name="anytype-model-ObjectLinks"></a>

### ObjectLinks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inboundIDs | [string](#string) | repeated |  |
| outboundIDs | [string](#string) | repeated |  |






<a name="anytype-model-ObjectLinksInfo"></a>

### ObjectLinksInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inbound | [ObjectInfo](#anytype-model-ObjectInfo) | repeated |  |
| outbound | [ObjectInfo](#anytype-model-ObjectInfo) | repeated |  |






<a name="anytype-model-ObjectStoreChecksums"></a>

### ObjectStoreChecksums



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| bundledObjectTypes | [string](#string) |  |  |
| bundledRelations | [string](#string) |  |  |
| bundledLayouts | [string](#string) |  |  |
| objectsForceReindexCounter | [int32](#int32) |  | increased in order to trigger all objects reindex |
| filesForceReindexCounter | [int32](#int32) |  | increased in order to fully reindex all objects |
| idxRebuildCounter | [int32](#int32) |  | increased in order to remove indexes and reindex everything. Automatically triggers objects and files reindex(one time only) |
| fulltextRebuild | [int32](#int32) |  | increased in order to perform fulltext indexing for all type of objects (useful when we change fulltext config) |
| bundledTemplates | [string](#string) |  |  |
| bundledObjects | [int32](#int32) |  | anytypeProfile and maybe some others in the feature |
| filestoreKeysForceReindexCounter | [int32](#int32) |  |  |





 

 

 

 



<a name="pkg_lib_pb_model_protos_models-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pkg/lib/pb/model/protos/models.proto



<a name="anytype-model-Account"></a>

### Account
Contains basic information about a user account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | User&#39;s thread id |
| name | [string](#string) |  | User name, that associated with this account |
| avatar | [Account.Avatar](#anytype-model-Account-Avatar) |  | Avatar of a user&#39;s account |
| config | [Account.Config](#anytype-model-Account-Config) |  |  |
| status | [Account.Status](#anytype-model-Account-Status) |  |  |
| info | [Account.Info](#anytype-model-Account-Info) |  |  |






<a name="anytype-model-Account-Avatar"></a>

### Account.Avatar
Avatar of a user&#39;s account. It could be an image or color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| image | [Block.Content.File](#anytype-model-Block-Content-File) |  | Image of the avatar. Contains the hash to retrieve the image. |
| color | [string](#string) |  | Color of the avatar, used if image not set. |






<a name="anytype-model-Account-Config"></a>

### Account.Config



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enableDataview | [bool](#bool) |  |  |
| enableDebug | [bool](#bool) |  |  |
| enablePrereleaseChannel | [bool](#bool) |  |  |
| enableSpaces | [bool](#bool) |  |  |
| extra | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-model-Account-Info"></a>

### Account.Info



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| homeObjectId | [string](#string) |  | home dashboard block id |
| archiveObjectId | [string](#string) |  | archive block id |
| profileObjectId | [string](#string) |  | profile block id |
| marketplaceWorkspaceId | [string](#string) |  | marketplace workspace id |
| deviceId | [string](#string) |  |  |
| accountSpaceId | [string](#string) |  | marketplace template id |
| widgetsId | [string](#string) |  |  |
| gatewayUrl | [string](#string) |  | gateway url for fetching static files |
| localStoragePath | [string](#string) |  | path to local storage |
| timeZone | [string](#string) |  | time zone from config |
| analyticsId | [string](#string) |  |  |






<a name="anytype-model-Account-Status"></a>

### Account.Status



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| statusType | [Account.StatusType](#anytype-model-Account-StatusType) |  |  |
| deletionDate | [int64](#int64) |  |  |






<a name="anytype-model-Block"></a>

### Block



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| restrictions | [Block.Restrictions](#anytype-model-Block-Restrictions) |  |  |
| childrenIds | [string](#string) | repeated |  |
| backgroundColor | [string](#string) |  |  |
| align | [Block.Align](#anytype-model-Block-Align) |  |  |
| verticalAlign | [Block.VerticalAlign](#anytype-model-Block-VerticalAlign) |  |  |
| smartblock | [Block.Content.Smartblock](#anytype-model-Block-Content-Smartblock) |  |  |
| text | [Block.Content.Text](#anytype-model-Block-Content-Text) |  |  |
| file | [Block.Content.File](#anytype-model-Block-Content-File) |  |  |
| layout | [Block.Content.Layout](#anytype-model-Block-Content-Layout) |  |  |
| div | [Block.Content.Div](#anytype-model-Block-Content-Div) |  |  |
| bookmark | [Block.Content.Bookmark](#anytype-model-Block-Content-Bookmark) |  |  |
| icon | [Block.Content.Icon](#anytype-model-Block-Content-Icon) |  |  |
| link | [Block.Content.Link](#anytype-model-Block-Content-Link) |  |  |
| dataview | [Block.Content.Dataview](#anytype-model-Block-Content-Dataview) |  |  |
| relation | [Block.Content.Relation](#anytype-model-Block-Content-Relation) |  |  |
| featuredRelations | [Block.Content.FeaturedRelations](#anytype-model-Block-Content-FeaturedRelations) |  |  |
| latex | [Block.Content.Latex](#anytype-model-Block-Content-Latex) |  |  |
| tableOfContents | [Block.Content.TableOfContents](#anytype-model-Block-Content-TableOfContents) |  |  |
| table | [Block.Content.Table](#anytype-model-Block-Content-Table) |  |  |
| tableColumn | [Block.Content.TableColumn](#anytype-model-Block-Content-TableColumn) |  |  |
| tableRow | [Block.Content.TableRow](#anytype-model-Block-Content-TableRow) |  |  |
| widget | [Block.Content.Widget](#anytype-model-Block-Content-Widget) |  |  |






<a name="anytype-model-Block-Content"></a>

### Block.Content







<a name="anytype-model-Block-Content-Bookmark"></a>

### Block.Content.Bookmark
Bookmark is to keep a web-link and to preview a content.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| title | [string](#string) |  | Deprecated. Get this data from the target object. |
| description | [string](#string) |  | Deprecated. Get this data from the target object. |
| imageHash | [string](#string) |  | Deprecated. Get this data from the target object. |
| faviconHash | [string](#string) |  | Deprecated. Get this data from the target object. |
| type | [LinkPreview.Type](#anytype-model-LinkPreview-Type) |  |  |
| targetObjectId | [string](#string) |  |  |
| state | [Block.Content.Bookmark.State](#anytype-model-Block-Content-Bookmark-State) |  |  |






<a name="anytype-model-Block-Content-Dataview"></a>

### Block.Content.Dataview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| views | [Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) | repeated |  |
| relations | [Relation](#anytype-model-Relation) | repeated | deprecated |
| activeView | [string](#string) |  | saved within a session |
| groupOrders | [Block.Content.Dataview.GroupOrder](#anytype-model-Block-Content-Dataview-GroupOrder) | repeated |  |
| objectOrders | [Block.Content.Dataview.ObjectOrder](#anytype-model-Block-Content-Dataview-ObjectOrder) | repeated |  |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated |  |
| TargetObjectId | [string](#string) |  |  |
| isCollection | [bool](#bool) |  |  |






<a name="anytype-model-Block-Content-Dataview-Checkbox"></a>

### Block.Content.Dataview.Checkbox



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| checked | [bool](#bool) |  |  |






<a name="anytype-model-Block-Content-Dataview-Date"></a>

### Block.Content.Dataview.Date







<a name="anytype-model-Block-Content-Dataview-Filter"></a>

### Block.Content.Dataview.Filter



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| operator | [Block.Content.Dataview.Filter.Operator](#anytype-model-Block-Content-Dataview-Filter-Operator) |  | looks not applicable? |
| RelationKey | [string](#string) |  |  |
| relationProperty | [string](#string) |  |  |
| condition | [Block.Content.Dataview.Filter.Condition](#anytype-model-Block-Content-Dataview-Filter-Condition) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  |  |
| quickOption | [Block.Content.Dataview.Filter.QuickOption](#anytype-model-Block-Content-Dataview-Filter-QuickOption) |  |  |
| format | [RelationFormat](#anytype-model-RelationFormat) |  |  |
| includeTime | [bool](#bool) |  |  |






<a name="anytype-model-Block-Content-Dataview-Group"></a>

### Block.Content.Dataview.Group



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| status | [Block.Content.Dataview.Status](#anytype-model-Block-Content-Dataview-Status) |  |  |
| tag | [Block.Content.Dataview.Tag](#anytype-model-Block-Content-Dataview-Tag) |  |  |
| checkbox | [Block.Content.Dataview.Checkbox](#anytype-model-Block-Content-Dataview-Checkbox) |  |  |
| date | [Block.Content.Dataview.Date](#anytype-model-Block-Content-Dataview-Date) |  |  |






<a name="anytype-model-Block-Content-Dataview-GroupOrder"></a>

### Block.Content.Dataview.GroupOrder



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| viewId | [string](#string) |  |  |
| viewGroups | [Block.Content.Dataview.ViewGroup](#anytype-model-Block-Content-Dataview-ViewGroup) | repeated |  |






<a name="anytype-model-Block-Content-Dataview-ObjectOrder"></a>

### Block.Content.Dataview.ObjectOrder



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| viewId | [string](#string) |  |  |
| groupId | [string](#string) |  |  |
| objectIds | [string](#string) | repeated |  |






<a name="anytype-model-Block-Content-Dataview-Relation"></a>

### Block.Content.Dataview.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| isVisible | [bool](#bool) |  |  |
| width | [int32](#int32) |  | the displayed column % calculated based on other visible relations |
| dateIncludeTime | [bool](#bool) |  |  |
| timeFormat | [Block.Content.Dataview.Relation.TimeFormat](#anytype-model-Block-Content-Dataview-Relation-TimeFormat) |  |  |
| dateFormat | [Block.Content.Dataview.Relation.DateFormat](#anytype-model-Block-Content-Dataview-Relation-DateFormat) |  |  |






<a name="anytype-model-Block-Content-Dataview-Sort"></a>

### Block.Content.Dataview.Sort



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| RelationKey | [string](#string) |  |  |
| type | [Block.Content.Dataview.Sort.Type](#anytype-model-Block-Content-Dataview-Sort-Type) |  |  |
| customOrder | [google.protobuf.Value](#google-protobuf-Value) | repeated |  |
| format | [RelationFormat](#anytype-model-RelationFormat) |  |  |
| includeTime | [bool](#bool) |  |  |






<a name="anytype-model-Block-Content-Dataview-Status"></a>

### Block.Content.Dataview.Status



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype-model-Block-Content-Dataview-Tag"></a>

### Block.Content.Dataview.Tag



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype-model-Block-Content-Dataview-View"></a>

### Block.Content.Dataview.View



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Block.Content.Dataview.View.Type](#anytype-model-Block-Content-Dataview-View-Type) |  |  |
| name | [string](#string) |  |  |
| sorts | [Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated |  |
| filters | [Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |
| relations | [Block.Content.Dataview.Relation](#anytype-model-Block-Content-Dataview-Relation) | repeated | relations fields/columns options, also used to provide the order |
| coverRelationKey | [string](#string) |  | Relation used for cover in gallery |
| hideIcon | [bool](#bool) |  | Hide icon near name |
| cardSize | [Block.Content.Dataview.View.Size](#anytype-model-Block-Content-Dataview-View-Size) |  | Gallery card size |
| coverFit | [bool](#bool) |  | Image fits container |
| groupRelationKey | [string](#string) |  | Group view by this relationKey |
| groupBackgroundColors | [bool](#bool) |  | Enable backgrounds in groups |
| pageLimit | [int32](#int32) |  |  |






<a name="anytype-model-Block-Content-Dataview-ViewGroup"></a>

### Block.Content.Dataview.ViewGroup



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| groupId | [string](#string) |  |  |
| index | [int32](#int32) |  |  |
| hidden | [bool](#bool) |  |  |
| backgroundColor | [string](#string) |  |  |






<a name="anytype-model-Block-Content-Div"></a>

### Block.Content.Div
Divider: block, that contains only one horizontal thin line


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style) |  |  |






<a name="anytype-model-Block-Content-FeaturedRelations"></a>

### Block.Content.FeaturedRelations







<a name="anytype-model-Block-Content-File"></a>

### Block.Content.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| name | [string](#string) |  |  |
| type | [Block.Content.File.Type](#anytype-model-Block-Content-File-Type) |  |  |
| mime | [string](#string) |  |  |
| size | [int64](#int64) |  |  |
| addedAt | [int64](#int64) |  |  |
| state | [Block.Content.File.State](#anytype-model-Block-Content-File-State) |  |  |
| style | [Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |






<a name="anytype-model-Block-Content-Icon"></a>

### Block.Content.Icon



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |






<a name="anytype-model-Block-Content-Latex"></a>

### Block.Content.Latex



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |






<a name="anytype-model-Block-Content-Layout"></a>

### Block.Content.Layout
Layout have no visual representation, but affects on blocks, that it contains.
Row/Column layout blocks creates only automatically, after some of a D&amp;D operations, for example


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Layout.Style](#anytype-model-Block-Content-Layout-Style) |  |  |






<a name="anytype-model-Block-Content-Link"></a>

### Block.Content.Link
Link: block to link some content from an external sources.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetBlockId | [string](#string) |  | id of the target block |
| style | [Block.Content.Link.Style](#anytype-model-Block-Content-Link-Style) |  | deprecated |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| iconSize | [Block.Content.Link.IconSize](#anytype-model-Block-Content-Link-IconSize) |  |  |
| cardStyle | [Block.Content.Link.CardStyle](#anytype-model-Block-Content-Link-CardStyle) |  |  |
| description | [Block.Content.Link.Description](#anytype-model-Block-Content-Link-Description) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype-model-Block-Content-Relation"></a>

### Block.Content.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |






<a name="anytype-model-Block-Content-Smartblock"></a>

### Block.Content.Smartblock







<a name="anytype-model-Block-Content-Table"></a>

### Block.Content.Table







<a name="anytype-model-Block-Content-TableColumn"></a>

### Block.Content.TableColumn







<a name="anytype-model-Block-Content-TableOfContents"></a>

### Block.Content.TableOfContents







<a name="anytype-model-Block-Content-TableRow"></a>

### Block.Content.TableRow



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| isHeader | [bool](#bool) |  |  |






<a name="anytype-model-Block-Content-Text"></a>

### Block.Content.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  |  |
| marks | [Block.Content.Text.Marks](#anytype-model-Block-Content-Text-Marks) |  | list of marks to apply to the text |
| checked | [bool](#bool) |  |  |
| color | [string](#string) |  |  |
| iconEmoji | [string](#string) |  | used with style Callout |
| iconImage | [string](#string) |  | in case both image and emoji are set, image should has a priority in the UI |






<a name="anytype-model-Block-Content-Text-Mark"></a>

### Block.Content.Text.Mark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Range](#anytype-model-Range) |  | range of symbols to apply this mark. From(symbol) To(symbol) |
| type | [Block.Content.Text.Mark.Type](#anytype-model-Block-Content-Text-Mark-Type) |  |  |
| param | [string](#string) |  | link, color, etc |






<a name="anytype-model-Block-Content-Text-Marks"></a>

### Block.Content.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marks | [Block.Content.Text.Mark](#anytype-model-Block-Content-Text-Mark) | repeated |  |






<a name="anytype-model-Block-Content-Widget"></a>

### Block.Content.Widget



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| layout | [Block.Content.Widget.Layout](#anytype-model-Block-Content-Widget-Layout) |  |  |
| limit | [int32](#int32) |  |  |






<a name="anytype-model-Block-Restrictions"></a>

### Block.Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| read | [bool](#bool) |  |  |
| edit | [bool](#bool) |  |  |
| remove | [bool](#bool) |  |  |
| drag | [bool](#bool) |  |  |
| dropOn | [bool](#bool) |  |  |






<a name="anytype-model-BlockMetaOnly"></a>

### BlockMetaOnly
Used to decode block meta only, without the content itself


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-model-InternalFlag"></a>

### InternalFlag



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [InternalFlag.Value](#anytype-model-InternalFlag-Value) |  |  |






<a name="anytype-model-Layout"></a>

### Layout



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [ObjectType.Layout](#anytype-model-ObjectType-Layout) |  |  |
| name | [string](#string) |  |  |
| requiredRelations | [Relation](#anytype-model-Relation) | repeated | relations required for this object type |






<a name="anytype-model-LinkPreview"></a>

### LinkPreview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| title | [string](#string) |  |  |
| description | [string](#string) |  |  |
| imageUrl | [string](#string) |  |  |
| faviconUrl | [string](#string) |  |  |
| type | [LinkPreview.Type](#anytype-model-LinkPreview-Type) |  |  |






<a name="anytype-model-Object"></a>

### Object







<a name="anytype-model-Object-ChangePayload"></a>

### Object.ChangePayload



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| smartBlockType | [SmartBlockType](#anytype-model-SmartBlockType) |  |  |






<a name="anytype-model-ObjectType"></a>

### ObjectType



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  | leave empty in case you want to create the new one |
| name | [string](#string) |  | name of objectType (can be localized for bundled types) |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated | cannot contain more than one Relation with the same RelationType |
| layout | [ObjectType.Layout](#anytype-model-ObjectType-Layout) |  |  |
| iconEmoji | [string](#string) |  | emoji symbol |
| description | [string](#string) |  |  |
| hidden | [bool](#bool) |  |  |
| readonly | [bool](#bool) |  |  |
| types | [SmartBlockType](#anytype-model-SmartBlockType) | repeated |  |
| isArchived | [bool](#bool) |  | sets locally to hide object type from set and some other places |
| installedByDefault | [bool](#bool) |  |  |






<a name="anytype-model-ObjectView"></a>

### ObjectView
Works with a smart blocks: Page, Dashboard
Dashboard opened, click on a page, Rpc.Block.open, Block.ShowFullscreen(PageBlock)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootId | [string](#string) |  | Root block id |
| blocks | [Block](#anytype-model-Block) | repeated | dependent simple blocks (descendants) |
| details | [ObjectView.DetailsSet](#anytype-model-ObjectView-DetailsSet) | repeated | details for the current and dependent objects |
| type | [SmartBlockType](#anytype-model-SmartBlockType) |  |  |
| relations | [Relation](#anytype-model-Relation) | repeated | DEPRECATED, use relationLinks instead |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated |  |
| restrictions | [Restrictions](#anytype-model-Restrictions) |  | object restrictions |
| history | [ObjectView.HistorySize](#anytype-model-ObjectView-HistorySize) |  |  |






<a name="anytype-model-ObjectView-DetailsSet"></a>

### ObjectView.DetailsSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | can not be a partial state. Should replace client details state |
| subIds | [string](#string) | repeated |  |






<a name="anytype-model-ObjectView-HistorySize"></a>

### ObjectView.HistorySize



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| undo | [int32](#int32) |  |  |
| redo | [int32](#int32) |  |  |






<a name="anytype-model-ObjectView-RelationWithValuePerObject"></a>

### ObjectView.RelationWithValuePerObject



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| relations | [RelationWithValue](#anytype-model-RelationWithValue) | repeated |  |






<a name="anytype-model-Range"></a>

### Range
General purpose structure, uses in Mark.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) |  |  |
| to | [int32](#int32) |  |  |






<a name="anytype-model-Relation"></a>

### Relation
Relation describe the human-interpreted relation type. It may be something like &#34;Date of creation, format=date&#34; or &#34;Assignee, format=objectId, objectType=person&#34;


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| key | [string](#string) |  | Key under which the value is stored in the map. Must be unique for the object type. It usually auto-generated bsonid, but also may be something human-readable in case of prebuilt types. |
| format | [RelationFormat](#anytype-model-RelationFormat) |  | format of the underlying data |
| name | [string](#string) |  | name to show (can be localized for bundled types) |
| defaultValue | [google.protobuf.Value](#google-protobuf-Value) |  |  |
| dataSource | [Relation.DataSource](#anytype-model-Relation-DataSource) |  | where the data is stored |
| hidden | [bool](#bool) |  | internal, not displayed to user (e.g. coverX, coverY) |
| readOnly | [bool](#bool) |  | value not editable by user tobe renamed to readonlyValue |
| readOnlyRelation | [bool](#bool) |  | relation metadata, eg name and format is not editable by user |
| multi | [bool](#bool) |  | allow multiple values (stored in pb list) |
| objectTypes | [string](#string) | repeated | URL of object type, empty to allow link to any object |
| selectDict | [Relation.Option](#anytype-model-Relation-Option) | repeated | index 10, 11 was used in internal-only builds. Can be reused, but may break some test accounts

default dictionary with unique values to choose for select/multiSelect format |
| maxCount | [int32](#int32) |  | max number of values can be set for this relation. 0 means no limit. 1 means the value can be stored in non-repeated field |
| description | [string](#string) |  |  |
| scope | [Relation.Scope](#anytype-model-Relation-Scope) |  | on-store fields, injected only locally

scope from which this relation have been aggregated |
| creator | [string](#string) |  | creator profile id |






<a name="anytype-model-Relation-Option"></a>

### Relation.Option



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | id generated automatically if omitted |
| text | [string](#string) |  |  |
| color | [string](#string) |  | stored |
| relationKey | [string](#string) |  | 4 is reserved for old relation format

stored |






<a name="anytype-model-RelationLink"></a>

### RelationLink



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| format | [RelationFormat](#anytype-model-RelationFormat) |  |  |






<a name="anytype-model-RelationOptions"></a>

### RelationOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| options | [Relation.Option](#anytype-model-Relation-Option) | repeated |  |






<a name="anytype-model-RelationWithValue"></a>

### RelationWithValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relation | [Relation](#anytype-model-Relation) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  |  |






<a name="anytype-model-Relations"></a>

### Relations



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relations | [Relation](#anytype-model-Relation) | repeated |  |






<a name="anytype-model-Restrictions"></a>

### Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| object | [Restrictions.ObjectRestriction](#anytype-model-Restrictions-ObjectRestriction) | repeated |  |
| dataview | [Restrictions.DataviewRestrictions](#anytype-model-Restrictions-DataviewRestrictions) | repeated |  |






<a name="anytype-model-Restrictions-DataviewRestrictions"></a>

### Restrictions.DataviewRestrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| restrictions | [Restrictions.DataviewRestriction](#anytype-model-Restrictions-DataviewRestriction) | repeated |  |






<a name="anytype-model-SmartBlockSnapshotBase"></a>

### SmartBlockSnapshotBase



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [Block](#anytype-model-Block) | repeated |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| fileKeys | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| extraRelations | [Relation](#anytype-model-Relation) | repeated | deprecated |
| objectTypes | [string](#string) | repeated |  |
| collections | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| removedCollectionKeys | [string](#string) | repeated |  |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated |  |






<a name="anytype-model-ThreadCreateQueueEntry"></a>

### ThreadCreateQueueEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| collectionThread | [string](#string) |  |  |
| threadId | [string](#string) |  |  |






<a name="anytype-model-ThreadDeeplinkPayload"></a>

### ThreadDeeplinkPayload



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| addrs | [string](#string) | repeated |  |





 


<a name="anytype-model-Account-StatusType"></a>

### Account.StatusType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Active | 0 |  |
| PendingDeletion | 1 |  |
| StartedDeletion | 2 |  |
| Deleted | 3 |  |



<a name="anytype-model-Block-Align"></a>

### Block.Align


| Name | Number | Description |
| ---- | ------ | ----------- |
| AlignLeft | 0 |  |
| AlignCenter | 1 |  |
| AlignRight | 2 |  |



<a name="anytype-model-Block-Content-Bookmark-State"></a>

### Block.Content.Bookmark.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 |  |
| Fetching | 1 |  |
| Done | 2 |  |
| Error | 3 |  |



<a name="anytype-model-Block-Content-Dataview-Filter-Condition"></a>

### Block.Content.Dataview.Filter.Condition


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Equal | 1 |  |
| NotEqual | 2 |  |
| Greater | 3 |  |
| Less | 4 |  |
| GreaterOrEqual | 5 |  |
| LessOrEqual | 6 |  |
| Like | 7 |  |
| NotLike | 8 |  |
| In | 9 | &#34;at least one value(from the provided list) is IN&#34; |
| NotIn | 10 | &#34;none of provided values are IN&#34; |
| Empty | 11 |  |
| NotEmpty | 12 |  |
| AllIn | 13 |  |
| NotAllIn | 14 |  |
| ExactIn | 15 |  |
| NotExactIn | 16 |  |
| Exists | 17 |  |



<a name="anytype-model-Block-Content-Dataview-Filter-Operator"></a>

### Block.Content.Dataview.Filter.Operator


| Name | Number | Description |
| ---- | ------ | ----------- |
| And | 0 |  |
| Or | 1 |  |



<a name="anytype-model-Block-Content-Dataview-Filter-QuickOption"></a>

### Block.Content.Dataview.Filter.QuickOption


| Name | Number | Description |
| ---- | ------ | ----------- |
| ExactDate | 0 |  |
| Yesterday | 1 |  |
| Today | 2 |  |
| Tomorrow | 3 |  |
| LastWeek | 4 |  |
| CurrentWeek | 5 |  |
| NextWeek | 6 |  |
| LastMonth | 7 |  |
| CurrentMonth | 8 |  |
| NextMonth | 9 |  |
| NumberOfDaysAgo | 10 |  |
| NumberOfDaysNow | 11 |  |



<a name="anytype-model-Block-Content-Dataview-Relation-DateFormat"></a>

### Block.Content.Dataview.Relation.DateFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| MonthAbbrBeforeDay | 0 | Jul 30, 2020 |
| MonthAbbrAfterDay | 1 | 30 Jul 2020 |
| Short | 2 | 30/07/2020 |
| ShortUS | 3 | 07/30/2020 |
| ISO | 4 | 2020-07-30 |



<a name="anytype-model-Block-Content-Dataview-Relation-TimeFormat"></a>

### Block.Content.Dataview.Relation.TimeFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| Format12 | 0 |  |
| Format24 | 1 |  |



<a name="anytype-model-Block-Content-Dataview-Sort-Type"></a>

### Block.Content.Dataview.Sort.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Asc | 0 |  |
| Desc | 1 |  |
| Custom | 2 |  |



<a name="anytype-model-Block-Content-Dataview-View-Size"></a>

### Block.Content.Dataview.View.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| Small | 0 |  |
| Medium | 1 |  |
| Large | 2 |  |



<a name="anytype-model-Block-Content-Dataview-View-Type"></a>

### Block.Content.Dataview.View.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Table | 0 |  |
| List | 1 |  |
| Gallery | 2 |  |
| Kanban | 3 |  |



<a name="anytype-model-Block-Content-Div-Style"></a>

### Block.Content.Div.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Line | 0 |  |
| Dots | 1 |  |



<a name="anytype-model-Block-Content-File-State"></a>

### Block.Content.File.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 | There is no file and preview, it&#39;s an empty block, that waits files. |
| Uploading | 1 | There is still no file/preview, but file already uploading |
| Done | 2 | File and preview downloaded |
| Error | 3 | Error while uploading |



<a name="anytype-model-Block-Content-File-Style"></a>

### Block.Content.File.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Auto | 0 | all types expect File and None has Embed style by default |
| Link | 1 |  |
| Embed | 2 |  |



<a name="anytype-model-Block-Content-File-Type"></a>

### Block.Content.File.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| File | 1 |  |
| Image | 2 |  |
| Video | 3 |  |
| Audio | 4 |  |
| PDF | 5 |  |



<a name="anytype-model-Block-Content-Layout-Style"></a>

### Block.Content.Layout.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Row | 0 |  |
| Column | 1 |  |
| Div | 2 |  |
| Header | 3 |  |
| TableRows | 4 |  |
| TableColumns | 5 |  |



<a name="anytype-model-Block-Content-Link-CardStyle"></a>

### Block.Content.Link.CardStyle


| Name | Number | Description |
| ---- | ------ | ----------- |
| Text | 0 |  |
| Card | 1 |  |
| Inline | 2 |  |



<a name="anytype-model-Block-Content-Link-Description"></a>

### Block.Content.Link.Description


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Added | 1 |  |
| Content | 2 |  |



<a name="anytype-model-Block-Content-Link-IconSize"></a>

### Block.Content.Link.IconSize


| Name | Number | Description |
| ---- | ------ | ----------- |
| SizeNone | 0 |  |
| SizeSmall | 1 |  |
| SizeMedium | 2 |  |



<a name="anytype-model-Block-Content-Link-Style"></a>

### Block.Content.Link.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Page | 0 |  |
| Dataview | 1 |  |
| Dashboard | 2 |  |
| Archive | 3 | ... |



<a name="anytype-model-Block-Content-Text-Mark-Type"></a>

### Block.Content.Text.Mark.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Strikethrough | 0 |  |
| Keyboard | 1 |  |
| Italic | 2 |  |
| Bold | 3 |  |
| Underscored | 4 |  |
| Link | 5 |  |
| TextColor | 6 |  |
| BackgroundColor | 7 |  |
| Mention | 8 |  |
| Emoji | 9 |  |
| Object | 10 |  |



<a name="anytype-model-Block-Content-Text-Style"></a>

### Block.Content.Text.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Paragraph | 0 |  |
| Header1 | 1 |  |
| Header2 | 2 |  |
| Header3 | 3 |  |
| Header4 | 4 | deprecated |
| Quote | 5 |  |
| Code | 6 |  |
| Title | 7 | currently only one block of this style can exists on a page |
| Checkbox | 8 |  |
| Marked | 9 |  |
| Numbered | 10 |  |
| Toggle | 11 |  |
| Description | 12 | currently only one block of this style can exists on a page |
| Callout | 13 |  |



<a name="anytype-model-Block-Content-Widget-Layout"></a>

### Block.Content.Widget.Layout


| Name | Number | Description |
| ---- | ------ | ----------- |
| Link | 0 |  |
| Tree | 1 |  |
| List | 2 |  |
| CompactList | 3 |  |



<a name="anytype-model-Block-Position"></a>

### Block.Position


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Top | 1 | above target block |
| Bottom | 2 | under target block |
| Left | 3 | to left of target block |
| Right | 4 | to right of target block |
| Inner | 5 | inside target block, as last block |
| Replace | 6 | replace target block |
| InnerFirst | 7 | inside target block, as first block |



<a name="anytype-model-Block-VerticalAlign"></a>

### Block.VerticalAlign


| Name | Number | Description |
| ---- | ------ | ----------- |
| VerticalAlignTop | 0 |  |
| VerticalAlignMiddle | 1 |  |
| VerticalAlignBottom | 2 |  |



<a name="anytype-model-InternalFlag-Value"></a>

### InternalFlag.Value
Use such a weird construction due to the issue with imported repeated enum type
Look https://github.com/golang/protobuf/issues/1135 for more information.

| Name | Number | Description |
| ---- | ------ | ----------- |
| editorDeleteEmpty | 0 |  |
| editorSelectType | 1 |  |
| editorSelectTemplate | 2 |  |
| collectionDontIndexLinks | 3 |  |



<a name="anytype-model-LinkPreview-Type"></a>

### LinkPreview.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Page | 1 |  |
| Image | 2 |  |
| Text | 3 |  |



<a name="anytype-model-ObjectType-Layout"></a>

### ObjectType.Layout


| Name | Number | Description |
| ---- | ------ | ----------- |
| basic | 0 |  |
| profile | 1 |  |
| todo | 2 |  |
| set | 3 |  |
| objectType | 4 |  |
| relation | 5 |  |
| file | 6 |  |
| dashboard | 7 |  |
| image | 8 |  |
| note | 9 |  |
| space | 10 |  |
| bookmark | 11 |  |
| relationOptionsList | 12 |  |
| relationOption | 13 |  |
| collection | 14 |  |
| database | 20 | to be released later |



<a name="anytype-model-Relation-DataSource"></a>

### Relation.DataSource


| Name | Number | Description |
| ---- | ------ | ----------- |
| details | 0 | default, stored inside the object&#39;s details |
| derived | 1 | stored locally, e.g. in badger or generated on the fly |
| account | 2 | stored in the account DB. means existing only for specific anytype account |
| local | 3 | stored locally |



<a name="anytype-model-Relation-Scope"></a>

### Relation.Scope


| Name | Number | Description |
| ---- | ------ | ----------- |
| object | 0 | stored within the object |
| type | 1 | stored within the object type |
| setOfTheSameType | 2 | aggregated from the dataview of sets of the same object type |
| objectsOfTheSameType | 3 | aggregated from the dataview of sets of the same object type |
| library | 4 | aggregated from relations library |



<a name="anytype-model-RelationFormat"></a>

### RelationFormat
RelationFormat describes how the underlying data is stored in the google.protobuf.Value and how it should be validated/sanitized

| Name | Number | Description |
| ---- | ------ | ----------- |
| longtext | 0 | string |
| shorttext | 1 | string, usually short enough. May be truncated in the future |
| number | 2 | double |
| status | 3 | string or list of string(len==1) |
| tag | 11 | list of string (choose multiple from a list) |
| date | 4 | float64(pb.Value doesn&#39;t have int64) or the string |
| file | 5 | relation can has objects of specific types: file, image, audio, video |
| checkbox | 6 | boolean |
| url | 7 | string with sanity check |
| email | 8 | string with sanity check |
| phone | 9 | string with sanity check |
| emoji | 10 | one emoji, can contains multiple utf-8 symbols |
| object | 100 | relation can has objectType to specify objectType |
| relations | 101 | base64-encoded relation pb model |



<a name="anytype-model-Restrictions-DataviewRestriction"></a>

### Restrictions.DataviewRestriction


| Name | Number | Description |
| ---- | ------ | ----------- |
| DVNone | 0 |  |
| DVRelation | 1 |  |
| DVCreateObject | 2 |  |
| DVViews | 3 |  |



<a name="anytype-model-Restrictions-ObjectRestriction"></a>

### Restrictions.ObjectRestriction


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Delete | 1 | restricts delete |
| Relations | 2 | restricts work with relations |
| Blocks | 3 | restricts work with blocks |
| Details | 4 | restricts work with details |
| TypeChange | 5 | restricts type changing |
| LayoutChange | 6 | restricts layout changing |
| Template | 7 | restricts template creation from this object |
| Duplicate | 8 | restricts duplicate object |



<a name="anytype-model-SmartBlockType"></a>

### SmartBlockType


| Name | Number | Description |
| ---- | ------ | ----------- |
| AccountOld | 0 | deprecated |
| Page | 16 |  |
| ProfilePage | 17 |  |
| Home | 32 |  |
| Archive | 48 |  |
| Widget | 112 |  |
| File | 256 |  |
| Template | 288 |  |
| BundledTemplate | 289 |  |
| BundledRelation | 512 | DEPRECATED |
| SubObject | 513 |  |
| BundledObjectType | 514 |  |
| AnytypeProfile | 515 |  |
| Date | 516 |  |
| Workspace | 518 |  |
| MissingObject | 519 |  |


 

 

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

