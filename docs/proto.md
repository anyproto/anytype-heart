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
    - [Change.DeviceAdd](#anytype-Change-DeviceAdd)
    - [Change.DeviceUpdate](#anytype-Change-DeviceUpdate)
    - [Change.FileKeys](#anytype-Change-FileKeys)
    - [Change.FileKeys.KeysEntry](#anytype-Change-FileKeys-KeysEntry)
    - [Change.NotificationCreate](#anytype-Change-NotificationCreate)
    - [Change.NotificationUpdate](#anytype-Change-NotificationUpdate)
    - [Change.ObjectTypeAdd](#anytype-Change-ObjectTypeAdd)
    - [Change.ObjectTypeRemove](#anytype-Change-ObjectTypeRemove)
    - [Change.OriginalCreatedTimestampSet](#anytype-Change-OriginalCreatedTimestampSet)
    - [Change.RelationAdd](#anytype-Change-RelationAdd)
    - [Change.RelationRemove](#anytype-Change-RelationRemove)
    - [Change.SetFileInfo](#anytype-Change-SetFileInfo)
    - [Change.Snapshot](#anytype-Change-Snapshot)
    - [Change.Snapshot.LogHeadsEntry](#anytype-Change-Snapshot-LogHeadsEntry)
    - [Change.StoreKeySet](#anytype-Change-StoreKeySet)
    - [Change.StoreKeyUnset](#anytype-Change-StoreKeyUnset)
    - [Change.StoreSliceUpdate](#anytype-Change-StoreSliceUpdate)
    - [Change.StoreSliceUpdate.Add](#anytype-Change-StoreSliceUpdate-Add)
    - [Change.StoreSliceUpdate.Move](#anytype-Change-StoreSliceUpdate-Move)
    - [Change.StoreSliceUpdate.Remove](#anytype-Change-StoreSliceUpdate-Remove)
    - [ChangeNoSnapshot](#anytype-ChangeNoSnapshot)
    - [DocumentCreate](#anytype-DocumentCreate)
    - [DocumentDelete](#anytype-DocumentDelete)
    - [DocumentModify](#anytype-DocumentModify)
    - [KeyModify](#anytype-KeyModify)
    - [StoreChange](#anytype-StoreChange)
    - [StoreChangeContent](#anytype-StoreChangeContent)
  
    - [ModifyOp](#anytype-ModifyOp)
  
- [pb/protos/commands.proto](#pb_protos_commands-proto)
    - [Empty](#anytype-Empty)
    - [Rpc](#anytype-Rpc)
    - [Rpc.AI](#anytype-Rpc-AI)
    - [Rpc.AI.Autofill](#anytype-Rpc-AI-Autofill)
    - [Rpc.AI.Autofill.Request](#anytype-Rpc-AI-Autofill-Request)
    - [Rpc.AI.Autofill.Response](#anytype-Rpc-AI-Autofill-Response)
    - [Rpc.AI.Autofill.Response.Error](#anytype-Rpc-AI-Autofill-Response-Error)
    - [Rpc.AI.ListSummary](#anytype-Rpc-AI-ListSummary)
    - [Rpc.AI.ListSummary.Request](#anytype-Rpc-AI-ListSummary-Request)
    - [Rpc.AI.ListSummary.Response](#anytype-Rpc-AI-ListSummary-Response)
    - [Rpc.AI.ListSummary.Response.Error](#anytype-Rpc-AI-ListSummary-Response-Error)
    - [Rpc.AI.ObjectCreateFromUrl](#anytype-Rpc-AI-ObjectCreateFromUrl)
    - [Rpc.AI.ObjectCreateFromUrl.Request](#anytype-Rpc-AI-ObjectCreateFromUrl-Request)
    - [Rpc.AI.ObjectCreateFromUrl.Response](#anytype-Rpc-AI-ObjectCreateFromUrl-Response)
    - [Rpc.AI.ObjectCreateFromUrl.Response.Error](#anytype-Rpc-AI-ObjectCreateFromUrl-Response-Error)
    - [Rpc.AI.ProviderConfig](#anytype-Rpc-AI-ProviderConfig)
    - [Rpc.AI.WritingTools](#anytype-Rpc-AI-WritingTools)
    - [Rpc.AI.WritingTools.Request](#anytype-Rpc-AI-WritingTools-Request)
    - [Rpc.AI.WritingTools.Response](#anytype-Rpc-AI-WritingTools-Response)
    - [Rpc.AI.WritingTools.Response.Error](#anytype-Rpc-AI-WritingTools-Response-Error)
    - [Rpc.Account](#anytype-Rpc-Account)
    - [Rpc.Account.ChangeJsonApiAddr](#anytype-Rpc-Account-ChangeJsonApiAddr)
    - [Rpc.Account.ChangeJsonApiAddr.Request](#anytype-Rpc-Account-ChangeJsonApiAddr-Request)
    - [Rpc.Account.ChangeJsonApiAddr.Response](#anytype-Rpc-Account-ChangeJsonApiAddr-Response)
    - [Rpc.Account.ChangeJsonApiAddr.Response.Error](#anytype-Rpc-Account-ChangeJsonApiAddr-Response-Error)
    - [Rpc.Account.ChangeNetworkConfigAndRestart](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart)
    - [Rpc.Account.ChangeNetworkConfigAndRestart.Request](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Request)
    - [Rpc.Account.ChangeNetworkConfigAndRestart.Response](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response)
    - [Rpc.Account.ChangeNetworkConfigAndRestart.Response.Error](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response-Error)
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
    - [Rpc.Account.EnableLocalNetworkSync](#anytype-Rpc-Account-EnableLocalNetworkSync)
    - [Rpc.Account.EnableLocalNetworkSync.Request](#anytype-Rpc-Account-EnableLocalNetworkSync-Request)
    - [Rpc.Account.EnableLocalNetworkSync.Response](#anytype-Rpc-Account-EnableLocalNetworkSync-Response)
    - [Rpc.Account.EnableLocalNetworkSync.Response.Error](#anytype-Rpc-Account-EnableLocalNetworkSync-Response-Error)
    - [Rpc.Account.GetConfig](#anytype-Rpc-Account-GetConfig)
    - [Rpc.Account.GetConfig.Get](#anytype-Rpc-Account-GetConfig-Get)
    - [Rpc.Account.GetConfig.Get.Request](#anytype-Rpc-Account-GetConfig-Get-Request)
    - [Rpc.Account.LocalLink](#anytype-Rpc-Account-LocalLink)
    - [Rpc.Account.LocalLink.CreateApp](#anytype-Rpc-Account-LocalLink-CreateApp)
    - [Rpc.Account.LocalLink.CreateApp.Request](#anytype-Rpc-Account-LocalLink-CreateApp-Request)
    - [Rpc.Account.LocalLink.CreateApp.Response](#anytype-Rpc-Account-LocalLink-CreateApp-Response)
    - [Rpc.Account.LocalLink.CreateApp.Response.Error](#anytype-Rpc-Account-LocalLink-CreateApp-Response-Error)
    - [Rpc.Account.LocalLink.ListApps](#anytype-Rpc-Account-LocalLink-ListApps)
    - [Rpc.Account.LocalLink.ListApps.Request](#anytype-Rpc-Account-LocalLink-ListApps-Request)
    - [Rpc.Account.LocalLink.ListApps.Response](#anytype-Rpc-Account-LocalLink-ListApps-Response)
    - [Rpc.Account.LocalLink.ListApps.Response.Error](#anytype-Rpc-Account-LocalLink-ListApps-Response-Error)
    - [Rpc.Account.LocalLink.NewChallenge](#anytype-Rpc-Account-LocalLink-NewChallenge)
    - [Rpc.Account.LocalLink.NewChallenge.Request](#anytype-Rpc-Account-LocalLink-NewChallenge-Request)
    - [Rpc.Account.LocalLink.NewChallenge.Response](#anytype-Rpc-Account-LocalLink-NewChallenge-Response)
    - [Rpc.Account.LocalLink.NewChallenge.Response.Error](#anytype-Rpc-Account-LocalLink-NewChallenge-Response-Error)
    - [Rpc.Account.LocalLink.RevokeApp](#anytype-Rpc-Account-LocalLink-RevokeApp)
    - [Rpc.Account.LocalLink.RevokeApp.Request](#anytype-Rpc-Account-LocalLink-RevokeApp-Request)
    - [Rpc.Account.LocalLink.RevokeApp.Response](#anytype-Rpc-Account-LocalLink-RevokeApp-Response)
    - [Rpc.Account.LocalLink.RevokeApp.Response.Error](#anytype-Rpc-Account-LocalLink-RevokeApp-Response-Error)
    - [Rpc.Account.LocalLink.SolveChallenge](#anytype-Rpc-Account-LocalLink-SolveChallenge)
    - [Rpc.Account.LocalLink.SolveChallenge.Request](#anytype-Rpc-Account-LocalLink-SolveChallenge-Request)
    - [Rpc.Account.LocalLink.SolveChallenge.Response](#anytype-Rpc-Account-LocalLink-SolveChallenge-Response)
    - [Rpc.Account.LocalLink.SolveChallenge.Response.Error](#anytype-Rpc-Account-LocalLink-SolveChallenge-Response-Error)
    - [Rpc.Account.Migrate](#anytype-Rpc-Account-Migrate)
    - [Rpc.Account.Migrate.Request](#anytype-Rpc-Account-Migrate-Request)
    - [Rpc.Account.Migrate.Response](#anytype-Rpc-Account-Migrate-Response)
    - [Rpc.Account.Migrate.Response.Error](#anytype-Rpc-Account-Migrate-Response-Error)
    - [Rpc.Account.MigrateCancel](#anytype-Rpc-Account-MigrateCancel)
    - [Rpc.Account.MigrateCancel.Request](#anytype-Rpc-Account-MigrateCancel-Request)
    - [Rpc.Account.MigrateCancel.Response](#anytype-Rpc-Account-MigrateCancel-Response)
    - [Rpc.Account.MigrateCancel.Response.Error](#anytype-Rpc-Account-MigrateCancel-Response-Error)
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
    - [Rpc.Account.RevertDeletion](#anytype-Rpc-Account-RevertDeletion)
    - [Rpc.Account.RevertDeletion.Request](#anytype-Rpc-Account-RevertDeletion-Request)
    - [Rpc.Account.RevertDeletion.Response](#anytype-Rpc-Account-RevertDeletion-Response)
    - [Rpc.Account.RevertDeletion.Response.Error](#anytype-Rpc-Account-RevertDeletion-Response-Error)
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
    - [Rpc.Block.Preview](#anytype-Rpc-Block-Preview)
    - [Rpc.Block.Preview.Request](#anytype-Rpc-Block-Preview-Request)
    - [Rpc.Block.Preview.Response](#anytype-Rpc-Block-Preview-Response)
    - [Rpc.Block.Preview.Response.Error](#anytype-Rpc-Block-Preview-Response-Error)
    - [Rpc.Block.Replace](#anytype-Rpc-Block-Replace)
    - [Rpc.Block.Replace.Request](#anytype-Rpc-Block-Replace-Request)
    - [Rpc.Block.Replace.Response](#anytype-Rpc-Block-Replace-Response)
    - [Rpc.Block.Replace.Response.Error](#anytype-Rpc-Block-Replace-Response-Error)
    - [Rpc.Block.SetCarriage](#anytype-Rpc-Block-SetCarriage)
    - [Rpc.Block.SetCarriage.Request](#anytype-Rpc-Block-SetCarriage-Request)
    - [Rpc.Block.SetCarriage.Response](#anytype-Rpc-Block-SetCarriage-Response)
    - [Rpc.Block.SetCarriage.Response.Error](#anytype-Rpc-Block-SetCarriage-Response-Error)
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
    - [Rpc.BlockDataview.Relation.Set](#anytype-Rpc-BlockDataview-Relation-Set)
    - [Rpc.BlockDataview.Relation.Set.Request](#anytype-Rpc-BlockDataview-Relation-Set-Request)
    - [Rpc.BlockDataview.Relation.Set.Response](#anytype-Rpc-BlockDataview-Relation-Set-Response)
    - [Rpc.BlockDataview.Relation.Set.Response.Error](#anytype-Rpc-BlockDataview-Relation-Set-Response-Error)
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
    - [Rpc.BlockDataview.Sort.SSort](#anytype-Rpc-BlockDataview-Sort-SSort)
    - [Rpc.BlockDataview.Sort.SSort.Request](#anytype-Rpc-BlockDataview-Sort-SSort-Request)
    - [Rpc.BlockDataview.Sort.SSort.Response](#anytype-Rpc-BlockDataview-Sort-SSort-Response)
    - [Rpc.BlockDataview.Sort.SSort.Response.Error](#anytype-Rpc-BlockDataview-Sort-SSort-Response-Error)
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
    - [Rpc.BlockFile.SetTargetObjectId](#anytype-Rpc-BlockFile-SetTargetObjectId)
    - [Rpc.BlockFile.SetTargetObjectId.Request](#anytype-Rpc-BlockFile-SetTargetObjectId-Request)
    - [Rpc.BlockFile.SetTargetObjectId.Response](#anytype-Rpc-BlockFile-SetTargetObjectId-Response)
    - [Rpc.BlockFile.SetTargetObjectId.Response.Error](#anytype-Rpc-BlockFile-SetTargetObjectId-Response-Error)
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
    - [Rpc.BlockLatex.SetProcessor](#anytype-Rpc-BlockLatex-SetProcessor)
    - [Rpc.BlockLatex.SetProcessor.Request](#anytype-Rpc-BlockLatex-SetProcessor-Request)
    - [Rpc.BlockLatex.SetProcessor.Response](#anytype-Rpc-BlockLatex-SetProcessor-Response)
    - [Rpc.BlockLatex.SetProcessor.Response.Error](#anytype-Rpc-BlockLatex-SetProcessor-Response-Error)
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
    - [Rpc.BlockWidget.SetViewId](#anytype-Rpc-BlockWidget-SetViewId)
    - [Rpc.BlockWidget.SetViewId.Request](#anytype-Rpc-BlockWidget-SetViewId-Request)
    - [Rpc.BlockWidget.SetViewId.Response](#anytype-Rpc-BlockWidget-SetViewId-Response)
    - [Rpc.BlockWidget.SetViewId.Response.Error](#anytype-Rpc-BlockWidget-SetViewId-Response-Error)
    - [Rpc.Broadcast](#anytype-Rpc-Broadcast)
    - [Rpc.Broadcast.PayloadEvent](#anytype-Rpc-Broadcast-PayloadEvent)
    - [Rpc.Broadcast.PayloadEvent.Request](#anytype-Rpc-Broadcast-PayloadEvent-Request)
    - [Rpc.Broadcast.PayloadEvent.Response](#anytype-Rpc-Broadcast-PayloadEvent-Response)
    - [Rpc.Broadcast.PayloadEvent.Response.Error](#anytype-Rpc-Broadcast-PayloadEvent-Response-Error)
    - [Rpc.Chat](#anytype-Rpc-Chat)
    - [Rpc.Chat.AddMessage](#anytype-Rpc-Chat-AddMessage)
    - [Rpc.Chat.AddMessage.Request](#anytype-Rpc-Chat-AddMessage-Request)
    - [Rpc.Chat.AddMessage.Response](#anytype-Rpc-Chat-AddMessage-Response)
    - [Rpc.Chat.AddMessage.Response.Error](#anytype-Rpc-Chat-AddMessage-Response-Error)
    - [Rpc.Chat.DeleteMessage](#anytype-Rpc-Chat-DeleteMessage)
    - [Rpc.Chat.DeleteMessage.Request](#anytype-Rpc-Chat-DeleteMessage-Request)
    - [Rpc.Chat.DeleteMessage.Response](#anytype-Rpc-Chat-DeleteMessage-Response)
    - [Rpc.Chat.DeleteMessage.Response.Error](#anytype-Rpc-Chat-DeleteMessage-Response-Error)
    - [Rpc.Chat.EditMessageContent](#anytype-Rpc-Chat-EditMessageContent)
    - [Rpc.Chat.EditMessageContent.Request](#anytype-Rpc-Chat-EditMessageContent-Request)
    - [Rpc.Chat.EditMessageContent.Response](#anytype-Rpc-Chat-EditMessageContent-Response)
    - [Rpc.Chat.EditMessageContent.Response.Error](#anytype-Rpc-Chat-EditMessageContent-Response-Error)
    - [Rpc.Chat.GetMessages](#anytype-Rpc-Chat-GetMessages)
    - [Rpc.Chat.GetMessages.Request](#anytype-Rpc-Chat-GetMessages-Request)
    - [Rpc.Chat.GetMessages.Response](#anytype-Rpc-Chat-GetMessages-Response)
    - [Rpc.Chat.GetMessages.Response.Error](#anytype-Rpc-Chat-GetMessages-Response-Error)
    - [Rpc.Chat.GetMessagesByIds](#anytype-Rpc-Chat-GetMessagesByIds)
    - [Rpc.Chat.GetMessagesByIds.Request](#anytype-Rpc-Chat-GetMessagesByIds-Request)
    - [Rpc.Chat.GetMessagesByIds.Response](#anytype-Rpc-Chat-GetMessagesByIds-Response)
    - [Rpc.Chat.GetMessagesByIds.Response.Error](#anytype-Rpc-Chat-GetMessagesByIds-Response-Error)
    - [Rpc.Chat.ReadAll](#anytype-Rpc-Chat-ReadAll)
    - [Rpc.Chat.ReadAll.Request](#anytype-Rpc-Chat-ReadAll-Request)
    - [Rpc.Chat.ReadAll.Response](#anytype-Rpc-Chat-ReadAll-Response)
    - [Rpc.Chat.ReadAll.Response.Error](#anytype-Rpc-Chat-ReadAll-Response-Error)
    - [Rpc.Chat.ReadMessages](#anytype-Rpc-Chat-ReadMessages)
    - [Rpc.Chat.ReadMessages.Request](#anytype-Rpc-Chat-ReadMessages-Request)
    - [Rpc.Chat.ReadMessages.Response](#anytype-Rpc-Chat-ReadMessages-Response)
    - [Rpc.Chat.ReadMessages.Response.Error](#anytype-Rpc-Chat-ReadMessages-Response-Error)
    - [Rpc.Chat.SubscribeLastMessages](#anytype-Rpc-Chat-SubscribeLastMessages)
    - [Rpc.Chat.SubscribeLastMessages.Request](#anytype-Rpc-Chat-SubscribeLastMessages-Request)
    - [Rpc.Chat.SubscribeLastMessages.Response](#anytype-Rpc-Chat-SubscribeLastMessages-Response)
    - [Rpc.Chat.SubscribeLastMessages.Response.Error](#anytype-Rpc-Chat-SubscribeLastMessages-Response-Error)
    - [Rpc.Chat.SubscribeToMessagePreviews](#anytype-Rpc-Chat-SubscribeToMessagePreviews)
    - [Rpc.Chat.SubscribeToMessagePreviews.Request](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Request)
    - [Rpc.Chat.SubscribeToMessagePreviews.Response](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response)
    - [Rpc.Chat.SubscribeToMessagePreviews.Response.ChatPreview](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-ChatPreview)
    - [Rpc.Chat.SubscribeToMessagePreviews.Response.Error](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-Error)
    - [Rpc.Chat.ToggleMessageReaction](#anytype-Rpc-Chat-ToggleMessageReaction)
    - [Rpc.Chat.ToggleMessageReaction.Request](#anytype-Rpc-Chat-ToggleMessageReaction-Request)
    - [Rpc.Chat.ToggleMessageReaction.Response](#anytype-Rpc-Chat-ToggleMessageReaction-Response)
    - [Rpc.Chat.ToggleMessageReaction.Response.Error](#anytype-Rpc-Chat-ToggleMessageReaction-Response-Error)
    - [Rpc.Chat.Unread](#anytype-Rpc-Chat-Unread)
    - [Rpc.Chat.Unread.Request](#anytype-Rpc-Chat-Unread-Request)
    - [Rpc.Chat.Unread.Response](#anytype-Rpc-Chat-Unread-Response)
    - [Rpc.Chat.Unread.Response.Error](#anytype-Rpc-Chat-Unread-Response-Error)
    - [Rpc.Chat.Unsubscribe](#anytype-Rpc-Chat-Unsubscribe)
    - [Rpc.Chat.Unsubscribe.Request](#anytype-Rpc-Chat-Unsubscribe-Request)
    - [Rpc.Chat.Unsubscribe.Response](#anytype-Rpc-Chat-Unsubscribe-Response)
    - [Rpc.Chat.Unsubscribe.Response.Error](#anytype-Rpc-Chat-Unsubscribe-Response-Error)
    - [Rpc.Chat.UnsubscribeFromMessagePreviews](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews)
    - [Rpc.Chat.UnsubscribeFromMessagePreviews.Request](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Request)
    - [Rpc.Chat.UnsubscribeFromMessagePreviews.Response](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response)
    - [Rpc.Chat.UnsubscribeFromMessagePreviews.Response.Error](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response-Error)
    - [Rpc.Debug](#anytype-Rpc-Debug)
    - [Rpc.Debug.AccountSelectTrace](#anytype-Rpc-Debug-AccountSelectTrace)
    - [Rpc.Debug.AccountSelectTrace.Request](#anytype-Rpc-Debug-AccountSelectTrace-Request)
    - [Rpc.Debug.AccountSelectTrace.Response](#anytype-Rpc-Debug-AccountSelectTrace-Response)
    - [Rpc.Debug.AccountSelectTrace.Response.Error](#anytype-Rpc-Debug-AccountSelectTrace-Response-Error)
    - [Rpc.Debug.AnystoreObjectChanges](#anytype-Rpc-Debug-AnystoreObjectChanges)
    - [Rpc.Debug.AnystoreObjectChanges.Request](#anytype-Rpc-Debug-AnystoreObjectChanges-Request)
    - [Rpc.Debug.AnystoreObjectChanges.Response](#anytype-Rpc-Debug-AnystoreObjectChanges-Response)
    - [Rpc.Debug.AnystoreObjectChanges.Response.Change](#anytype-Rpc-Debug-AnystoreObjectChanges-Response-Change)
    - [Rpc.Debug.AnystoreObjectChanges.Response.Error](#anytype-Rpc-Debug-AnystoreObjectChanges-Response-Error)
    - [Rpc.Debug.ExportLocalstore](#anytype-Rpc-Debug-ExportLocalstore)
    - [Rpc.Debug.ExportLocalstore.Request](#anytype-Rpc-Debug-ExportLocalstore-Request)
    - [Rpc.Debug.ExportLocalstore.Response](#anytype-Rpc-Debug-ExportLocalstore-Response)
    - [Rpc.Debug.ExportLocalstore.Response.Error](#anytype-Rpc-Debug-ExportLocalstore-Response-Error)
    - [Rpc.Debug.ExportLog](#anytype-Rpc-Debug-ExportLog)
    - [Rpc.Debug.ExportLog.Request](#anytype-Rpc-Debug-ExportLog-Request)
    - [Rpc.Debug.ExportLog.Response](#anytype-Rpc-Debug-ExportLog-Response)
    - [Rpc.Debug.ExportLog.Response.Error](#anytype-Rpc-Debug-ExportLog-Response-Error)
    - [Rpc.Debug.NetCheck](#anytype-Rpc-Debug-NetCheck)
    - [Rpc.Debug.NetCheck.Request](#anytype-Rpc-Debug-NetCheck-Request)
    - [Rpc.Debug.NetCheck.Response](#anytype-Rpc-Debug-NetCheck-Response)
    - [Rpc.Debug.NetCheck.Response.Error](#anytype-Rpc-Debug-NetCheck-Response-Error)
    - [Rpc.Debug.OpenedObjects](#anytype-Rpc-Debug-OpenedObjects)
    - [Rpc.Debug.OpenedObjects.Request](#anytype-Rpc-Debug-OpenedObjects-Request)
    - [Rpc.Debug.OpenedObjects.Response](#anytype-Rpc-Debug-OpenedObjects-Response)
    - [Rpc.Debug.OpenedObjects.Response.Error](#anytype-Rpc-Debug-OpenedObjects-Response-Error)
    - [Rpc.Debug.Ping](#anytype-Rpc-Debug-Ping)
    - [Rpc.Debug.Ping.Request](#anytype-Rpc-Debug-Ping-Request)
    - [Rpc.Debug.Ping.Response](#anytype-Rpc-Debug-Ping-Response)
    - [Rpc.Debug.Ping.Response.Error](#anytype-Rpc-Debug-Ping-Response-Error)
    - [Rpc.Debug.RunProfiler](#anytype-Rpc-Debug-RunProfiler)
    - [Rpc.Debug.RunProfiler.Request](#anytype-Rpc-Debug-RunProfiler-Request)
    - [Rpc.Debug.RunProfiler.Response](#anytype-Rpc-Debug-RunProfiler-Response)
    - [Rpc.Debug.RunProfiler.Response.Error](#anytype-Rpc-Debug-RunProfiler-Response-Error)
    - [Rpc.Debug.SpaceSummary](#anytype-Rpc-Debug-SpaceSummary)
    - [Rpc.Debug.SpaceSummary.Request](#anytype-Rpc-Debug-SpaceSummary-Request)
    - [Rpc.Debug.SpaceSummary.Response](#anytype-Rpc-Debug-SpaceSummary-Response)
    - [Rpc.Debug.SpaceSummary.Response.Error](#anytype-Rpc-Debug-SpaceSummary-Response-Error)
    - [Rpc.Debug.StackGoroutines](#anytype-Rpc-Debug-StackGoroutines)
    - [Rpc.Debug.StackGoroutines.Request](#anytype-Rpc-Debug-StackGoroutines-Request)
    - [Rpc.Debug.StackGoroutines.Response](#anytype-Rpc-Debug-StackGoroutines-Response)
    - [Rpc.Debug.StackGoroutines.Response.Error](#anytype-Rpc-Debug-StackGoroutines-Response-Error)
    - [Rpc.Debug.Stat](#anytype-Rpc-Debug-Stat)
    - [Rpc.Debug.Stat.Request](#anytype-Rpc-Debug-Stat-Request)
    - [Rpc.Debug.Stat.Response](#anytype-Rpc-Debug-Stat-Response)
    - [Rpc.Debug.Stat.Response.Error](#anytype-Rpc-Debug-Stat-Response-Error)
    - [Rpc.Debug.Subscriptions](#anytype-Rpc-Debug-Subscriptions)
    - [Rpc.Debug.Subscriptions.Request](#anytype-Rpc-Debug-Subscriptions-Request)
    - [Rpc.Debug.Subscriptions.Response](#anytype-Rpc-Debug-Subscriptions-Response)
    - [Rpc.Debug.Subscriptions.Response.Error](#anytype-Rpc-Debug-Subscriptions-Response-Error)
    - [Rpc.Debug.Tree](#anytype-Rpc-Debug-Tree)
    - [Rpc.Debug.Tree.Request](#anytype-Rpc-Debug-Tree-Request)
    - [Rpc.Debug.Tree.Response](#anytype-Rpc-Debug-Tree-Response)
    - [Rpc.Debug.Tree.Response.Error](#anytype-Rpc-Debug-Tree-Response-Error)
    - [Rpc.Debug.TreeHeads](#anytype-Rpc-Debug-TreeHeads)
    - [Rpc.Debug.TreeHeads.Request](#anytype-Rpc-Debug-TreeHeads-Request)
    - [Rpc.Debug.TreeHeads.Response](#anytype-Rpc-Debug-TreeHeads-Response)
    - [Rpc.Debug.TreeHeads.Response.Error](#anytype-Rpc-Debug-TreeHeads-Response-Error)
    - [Rpc.Debug.TreeInfo](#anytype-Rpc-Debug-TreeInfo)
    - [Rpc.Device](#anytype-Rpc-Device)
    - [Rpc.Device.List](#anytype-Rpc-Device-List)
    - [Rpc.Device.List.Request](#anytype-Rpc-Device-List-Request)
    - [Rpc.Device.List.Response](#anytype-Rpc-Device-List-Response)
    - [Rpc.Device.List.Response.Error](#anytype-Rpc-Device-List-Response-Error)
    - [Rpc.Device.NetworkState](#anytype-Rpc-Device-NetworkState)
    - [Rpc.Device.NetworkState.Set](#anytype-Rpc-Device-NetworkState-Set)
    - [Rpc.Device.NetworkState.Set.Request](#anytype-Rpc-Device-NetworkState-Set-Request)
    - [Rpc.Device.NetworkState.Set.Response](#anytype-Rpc-Device-NetworkState-Set-Response)
    - [Rpc.Device.NetworkState.Set.Response.Error](#anytype-Rpc-Device-NetworkState-Set-Response-Error)
    - [Rpc.Device.SetName](#anytype-Rpc-Device-SetName)
    - [Rpc.Device.SetName.Request](#anytype-Rpc-Device-SetName-Request)
    - [Rpc.Device.SetName.Response](#anytype-Rpc-Device-SetName-Response)
    - [Rpc.Device.SetName.Response.Error](#anytype-Rpc-Device-SetName-Response-Error)
    - [Rpc.File](#anytype-Rpc-File)
    - [Rpc.File.CacheCancelDownload](#anytype-Rpc-File-CacheCancelDownload)
    - [Rpc.File.CacheCancelDownload.Request](#anytype-Rpc-File-CacheCancelDownload-Request)
    - [Rpc.File.CacheCancelDownload.Response](#anytype-Rpc-File-CacheCancelDownload-Response)
    - [Rpc.File.CacheCancelDownload.Response.Error](#anytype-Rpc-File-CacheCancelDownload-Response-Error)
    - [Rpc.File.CacheDownload](#anytype-Rpc-File-CacheDownload)
    - [Rpc.File.CacheDownload.Request](#anytype-Rpc-File-CacheDownload-Request)
    - [Rpc.File.CacheDownload.Response](#anytype-Rpc-File-CacheDownload-Response)
    - [Rpc.File.CacheDownload.Response.Error](#anytype-Rpc-File-CacheDownload-Response-Error)
    - [Rpc.File.DiscardPreload](#anytype-Rpc-File-DiscardPreload)
    - [Rpc.File.DiscardPreload.Request](#anytype-Rpc-File-DiscardPreload-Request)
    - [Rpc.File.DiscardPreload.Response](#anytype-Rpc-File-DiscardPreload-Response)
    - [Rpc.File.DiscardPreload.Response.Error](#anytype-Rpc-File-DiscardPreload-Response-Error)
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
    - [Rpc.File.NodeUsage](#anytype-Rpc-File-NodeUsage)
    - [Rpc.File.NodeUsage.Request](#anytype-Rpc-File-NodeUsage-Request)
    - [Rpc.File.NodeUsage.Response](#anytype-Rpc-File-NodeUsage-Response)
    - [Rpc.File.NodeUsage.Response.Error](#anytype-Rpc-File-NodeUsage-Response-Error)
    - [Rpc.File.NodeUsage.Response.Space](#anytype-Rpc-File-NodeUsage-Response-Space)
    - [Rpc.File.NodeUsage.Response.Usage](#anytype-Rpc-File-NodeUsage-Response-Usage)
    - [Rpc.File.Offload](#anytype-Rpc-File-Offload)
    - [Rpc.File.Offload.Request](#anytype-Rpc-File-Offload-Request)
    - [Rpc.File.Offload.Response](#anytype-Rpc-File-Offload-Response)
    - [Rpc.File.Offload.Response.Error](#anytype-Rpc-File-Offload-Response-Error)
    - [Rpc.File.Reconcile](#anytype-Rpc-File-Reconcile)
    - [Rpc.File.Reconcile.Request](#anytype-Rpc-File-Reconcile-Request)
    - [Rpc.File.Reconcile.Response](#anytype-Rpc-File-Reconcile-Response)
    - [Rpc.File.Reconcile.Response.Error](#anytype-Rpc-File-Reconcile-Response-Error)
    - [Rpc.File.SetAutoDownload](#anytype-Rpc-File-SetAutoDownload)
    - [Rpc.File.SetAutoDownload.Request](#anytype-Rpc-File-SetAutoDownload-Request)
    - [Rpc.File.SetAutoDownload.Response](#anytype-Rpc-File-SetAutoDownload-Response)
    - [Rpc.File.SetAutoDownload.Response.Error](#anytype-Rpc-File-SetAutoDownload-Response-Error)
    - [Rpc.File.SpaceOffload](#anytype-Rpc-File-SpaceOffload)
    - [Rpc.File.SpaceOffload.Request](#anytype-Rpc-File-SpaceOffload-Request)
    - [Rpc.File.SpaceOffload.Response](#anytype-Rpc-File-SpaceOffload-Response)
    - [Rpc.File.SpaceOffload.Response.Error](#anytype-Rpc-File-SpaceOffload-Response-Error)
    - [Rpc.File.SpaceUsage](#anytype-Rpc-File-SpaceUsage)
    - [Rpc.File.SpaceUsage.Request](#anytype-Rpc-File-SpaceUsage-Request)
    - [Rpc.File.SpaceUsage.Response](#anytype-Rpc-File-SpaceUsage-Response)
    - [Rpc.File.SpaceUsage.Response.Error](#anytype-Rpc-File-SpaceUsage-Response-Error)
    - [Rpc.File.SpaceUsage.Response.Usage](#anytype-Rpc-File-SpaceUsage-Response-Usage)
    - [Rpc.File.Upload](#anytype-Rpc-File-Upload)
    - [Rpc.File.Upload.Request](#anytype-Rpc-File-Upload-Request)
    - [Rpc.File.Upload.Response](#anytype-Rpc-File-Upload-Response)
    - [Rpc.File.Upload.Response.Error](#anytype-Rpc-File-Upload-Response-Error)
    - [Rpc.Gallery](#anytype-Rpc-Gallery)
    - [Rpc.Gallery.DownloadIndex](#anytype-Rpc-Gallery-DownloadIndex)
    - [Rpc.Gallery.DownloadIndex.Request](#anytype-Rpc-Gallery-DownloadIndex-Request)
    - [Rpc.Gallery.DownloadIndex.Response](#anytype-Rpc-Gallery-DownloadIndex-Response)
    - [Rpc.Gallery.DownloadIndex.Response.Category](#anytype-Rpc-Gallery-DownloadIndex-Response-Category)
    - [Rpc.Gallery.DownloadIndex.Response.Error](#anytype-Rpc-Gallery-DownloadIndex-Response-Error)
    - [Rpc.Gallery.DownloadManifest](#anytype-Rpc-Gallery-DownloadManifest)
    - [Rpc.Gallery.DownloadManifest.Request](#anytype-Rpc-Gallery-DownloadManifest-Request)
    - [Rpc.Gallery.DownloadManifest.Response](#anytype-Rpc-Gallery-DownloadManifest-Response)
    - [Rpc.Gallery.DownloadManifest.Response.Error](#anytype-Rpc-Gallery-DownloadManifest-Response-Error)
    - [Rpc.GenericErrorResponse](#anytype-Rpc-GenericErrorResponse)
    - [Rpc.GenericErrorResponse.Error](#anytype-Rpc-GenericErrorResponse-Error)
    - [Rpc.History](#anytype-Rpc-History)
    - [Rpc.History.DiffVersions](#anytype-Rpc-History-DiffVersions)
    - [Rpc.History.DiffVersions.Request](#anytype-Rpc-History-DiffVersions-Request)
    - [Rpc.History.DiffVersions.Response](#anytype-Rpc-History-DiffVersions-Response)
    - [Rpc.History.DiffVersions.Response.Error](#anytype-Rpc-History-DiffVersions-Response-Error)
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
    - [Rpc.Initial](#anytype-Rpc-Initial)
    - [Rpc.Initial.SetParameters](#anytype-Rpc-Initial-SetParameters)
    - [Rpc.Initial.SetParameters.Request](#anytype-Rpc-Initial-SetParameters-Request)
    - [Rpc.Initial.SetParameters.Response](#anytype-Rpc-Initial-SetParameters-Response)
    - [Rpc.Initial.SetParameters.Response.Error](#anytype-Rpc-Initial-SetParameters-Response-Error)
    - [Rpc.LinkPreview](#anytype-Rpc-LinkPreview)
    - [Rpc.LinkPreview.Request](#anytype-Rpc-LinkPreview-Request)
    - [Rpc.LinkPreview.Response](#anytype-Rpc-LinkPreview-Response)
    - [Rpc.LinkPreview.Response.Error](#anytype-Rpc-LinkPreview-Response-Error)
    - [Rpc.Log](#anytype-Rpc-Log)
    - [Rpc.Log.Send](#anytype-Rpc-Log-Send)
    - [Rpc.Log.Send.Request](#anytype-Rpc-Log-Send-Request)
    - [Rpc.Log.Send.Response](#anytype-Rpc-Log-Send-Response)
    - [Rpc.Log.Send.Response.Error](#anytype-Rpc-Log-Send-Response-Error)
    - [Rpc.Membership](#anytype-Rpc-Membership)
    - [Rpc.Membership.CodeGetInfo](#anytype-Rpc-Membership-CodeGetInfo)
    - [Rpc.Membership.CodeGetInfo.Request](#anytype-Rpc-Membership-CodeGetInfo-Request)
    - [Rpc.Membership.CodeGetInfo.Response](#anytype-Rpc-Membership-CodeGetInfo-Response)
    - [Rpc.Membership.CodeGetInfo.Response.Error](#anytype-Rpc-Membership-CodeGetInfo-Response-Error)
    - [Rpc.Membership.CodeRedeem](#anytype-Rpc-Membership-CodeRedeem)
    - [Rpc.Membership.CodeRedeem.Request](#anytype-Rpc-Membership-CodeRedeem-Request)
    - [Rpc.Membership.CodeRedeem.Response](#anytype-Rpc-Membership-CodeRedeem-Response)
    - [Rpc.Membership.CodeRedeem.Response.Error](#anytype-Rpc-Membership-CodeRedeem-Response-Error)
    - [Rpc.Membership.Finalize](#anytype-Rpc-Membership-Finalize)
    - [Rpc.Membership.Finalize.Request](#anytype-Rpc-Membership-Finalize-Request)
    - [Rpc.Membership.Finalize.Response](#anytype-Rpc-Membership-Finalize-Response)
    - [Rpc.Membership.Finalize.Response.Error](#anytype-Rpc-Membership-Finalize-Response-Error)
    - [Rpc.Membership.GetPortalLinkUrl](#anytype-Rpc-Membership-GetPortalLinkUrl)
    - [Rpc.Membership.GetPortalLinkUrl.Request](#anytype-Rpc-Membership-GetPortalLinkUrl-Request)
    - [Rpc.Membership.GetPortalLinkUrl.Response](#anytype-Rpc-Membership-GetPortalLinkUrl-Response)
    - [Rpc.Membership.GetPortalLinkUrl.Response.Error](#anytype-Rpc-Membership-GetPortalLinkUrl-Response-Error)
    - [Rpc.Membership.GetStatus](#anytype-Rpc-Membership-GetStatus)
    - [Rpc.Membership.GetStatus.Request](#anytype-Rpc-Membership-GetStatus-Request)
    - [Rpc.Membership.GetStatus.Response](#anytype-Rpc-Membership-GetStatus-Response)
    - [Rpc.Membership.GetStatus.Response.Error](#anytype-Rpc-Membership-GetStatus-Response-Error)
    - [Rpc.Membership.GetTiers](#anytype-Rpc-Membership-GetTiers)
    - [Rpc.Membership.GetTiers.Request](#anytype-Rpc-Membership-GetTiers-Request)
    - [Rpc.Membership.GetTiers.Response](#anytype-Rpc-Membership-GetTiers-Response)
    - [Rpc.Membership.GetTiers.Response.Error](#anytype-Rpc-Membership-GetTiers-Response-Error)
    - [Rpc.Membership.GetVerificationEmail](#anytype-Rpc-Membership-GetVerificationEmail)
    - [Rpc.Membership.GetVerificationEmail.Request](#anytype-Rpc-Membership-GetVerificationEmail-Request)
    - [Rpc.Membership.GetVerificationEmail.Response](#anytype-Rpc-Membership-GetVerificationEmail-Response)
    - [Rpc.Membership.GetVerificationEmail.Response.Error](#anytype-Rpc-Membership-GetVerificationEmail-Response-Error)
    - [Rpc.Membership.GetVerificationEmailStatus](#anytype-Rpc-Membership-GetVerificationEmailStatus)
    - [Rpc.Membership.GetVerificationEmailStatus.Request](#anytype-Rpc-Membership-GetVerificationEmailStatus-Request)
    - [Rpc.Membership.GetVerificationEmailStatus.Response](#anytype-Rpc-Membership-GetVerificationEmailStatus-Response)
    - [Rpc.Membership.GetVerificationEmailStatus.Response.Error](#anytype-Rpc-Membership-GetVerificationEmailStatus-Response-Error)
    - [Rpc.Membership.IsNameValid](#anytype-Rpc-Membership-IsNameValid)
    - [Rpc.Membership.IsNameValid.Request](#anytype-Rpc-Membership-IsNameValid-Request)
    - [Rpc.Membership.IsNameValid.Response](#anytype-Rpc-Membership-IsNameValid-Response)
    - [Rpc.Membership.IsNameValid.Response.Error](#anytype-Rpc-Membership-IsNameValid-Response-Error)
    - [Rpc.Membership.RegisterPaymentRequest](#anytype-Rpc-Membership-RegisterPaymentRequest)
    - [Rpc.Membership.RegisterPaymentRequest.Request](#anytype-Rpc-Membership-RegisterPaymentRequest-Request)
    - [Rpc.Membership.RegisterPaymentRequest.Response](#anytype-Rpc-Membership-RegisterPaymentRequest-Response)
    - [Rpc.Membership.RegisterPaymentRequest.Response.Error](#anytype-Rpc-Membership-RegisterPaymentRequest-Response-Error)
    - [Rpc.Membership.VerifyAppStoreReceipt](#anytype-Rpc-Membership-VerifyAppStoreReceipt)
    - [Rpc.Membership.VerifyAppStoreReceipt.Request](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Request)
    - [Rpc.Membership.VerifyAppStoreReceipt.Response](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Response)
    - [Rpc.Membership.VerifyAppStoreReceipt.Response.Error](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Response-Error)
    - [Rpc.Membership.VerifyEmailCode](#anytype-Rpc-Membership-VerifyEmailCode)
    - [Rpc.Membership.VerifyEmailCode.Request](#anytype-Rpc-Membership-VerifyEmailCode-Request)
    - [Rpc.Membership.VerifyEmailCode.Response](#anytype-Rpc-Membership-VerifyEmailCode-Response)
    - [Rpc.Membership.VerifyEmailCode.Response.Error](#anytype-Rpc-Membership-VerifyEmailCode-Response-Error)
    - [Rpc.MembershipV2](#anytype-Rpc-MembershipV2)
    - [Rpc.MembershipV2.AnyNameAllocate](#anytype-Rpc-MembershipV2-AnyNameAllocate)
    - [Rpc.MembershipV2.AnyNameAllocate.Request](#anytype-Rpc-MembershipV2-AnyNameAllocate-Request)
    - [Rpc.MembershipV2.AnyNameAllocate.Response](#anytype-Rpc-MembershipV2-AnyNameAllocate-Response)
    - [Rpc.MembershipV2.AnyNameAllocate.Response.Error](#anytype-Rpc-MembershipV2-AnyNameAllocate-Response-Error)
    - [Rpc.MembershipV2.AnyNameIsValid](#anytype-Rpc-MembershipV2-AnyNameIsValid)
    - [Rpc.MembershipV2.AnyNameIsValid.Request](#anytype-Rpc-MembershipV2-AnyNameIsValid-Request)
    - [Rpc.MembershipV2.AnyNameIsValid.Response](#anytype-Rpc-MembershipV2-AnyNameIsValid-Response)
    - [Rpc.MembershipV2.AnyNameIsValid.Response.Error](#anytype-Rpc-MembershipV2-AnyNameIsValid-Response-Error)
    - [Rpc.MembershipV2.CartGet](#anytype-Rpc-MembershipV2-CartGet)
    - [Rpc.MembershipV2.CartGet.Request](#anytype-Rpc-MembershipV2-CartGet-Request)
    - [Rpc.MembershipV2.CartGet.Response](#anytype-Rpc-MembershipV2-CartGet-Response)
    - [Rpc.MembershipV2.CartGet.Response.Error](#anytype-Rpc-MembershipV2-CartGet-Response-Error)
    - [Rpc.MembershipV2.CartUpdate](#anytype-Rpc-MembershipV2-CartUpdate)
    - [Rpc.MembershipV2.CartUpdate.Request](#anytype-Rpc-MembershipV2-CartUpdate-Request)
    - [Rpc.MembershipV2.CartUpdate.Response](#anytype-Rpc-MembershipV2-CartUpdate-Response)
    - [Rpc.MembershipV2.CartUpdate.Response.Error](#anytype-Rpc-MembershipV2-CartUpdate-Response-Error)
    - [Rpc.MembershipV2.GetPortalLink](#anytype-Rpc-MembershipV2-GetPortalLink)
    - [Rpc.MembershipV2.GetPortalLink.Request](#anytype-Rpc-MembershipV2-GetPortalLink-Request)
    - [Rpc.MembershipV2.GetPortalLink.Response](#anytype-Rpc-MembershipV2-GetPortalLink-Response)
    - [Rpc.MembershipV2.GetPortalLink.Response.Error](#anytype-Rpc-MembershipV2-GetPortalLink-Response-Error)
    - [Rpc.MembershipV2.GetProducts](#anytype-Rpc-MembershipV2-GetProducts)
    - [Rpc.MembershipV2.GetProducts.Request](#anytype-Rpc-MembershipV2-GetProducts-Request)
    - [Rpc.MembershipV2.GetProducts.Response](#anytype-Rpc-MembershipV2-GetProducts-Response)
    - [Rpc.MembershipV2.GetProducts.Response.Error](#anytype-Rpc-MembershipV2-GetProducts-Response-Error)
    - [Rpc.MembershipV2.GetStatus](#anytype-Rpc-MembershipV2-GetStatus)
    - [Rpc.MembershipV2.GetStatus.Request](#anytype-Rpc-MembershipV2-GetStatus-Request)
    - [Rpc.MembershipV2.GetStatus.Response](#anytype-Rpc-MembershipV2-GetStatus-Response)
    - [Rpc.MembershipV2.GetStatus.Response.Error](#anytype-Rpc-MembershipV2-GetStatus-Response-Error)
    - [Rpc.NameService](#anytype-Rpc-NameService)
    - [Rpc.NameService.ResolveAnyId](#anytype-Rpc-NameService-ResolveAnyId)
    - [Rpc.NameService.ResolveAnyId.Request](#anytype-Rpc-NameService-ResolveAnyId-Request)
    - [Rpc.NameService.ResolveAnyId.Response](#anytype-Rpc-NameService-ResolveAnyId-Response)
    - [Rpc.NameService.ResolveAnyId.Response.Error](#anytype-Rpc-NameService-ResolveAnyId-Response-Error)
    - [Rpc.NameService.ResolveName](#anytype-Rpc-NameService-ResolveName)
    - [Rpc.NameService.ResolveName.Request](#anytype-Rpc-NameService-ResolveName-Request)
    - [Rpc.NameService.ResolveName.Response](#anytype-Rpc-NameService-ResolveName-Response)
    - [Rpc.NameService.ResolveName.Response.Error](#anytype-Rpc-NameService-ResolveName-Response-Error)
    - [Rpc.NameService.ResolveSpaceId](#anytype-Rpc-NameService-ResolveSpaceId)
    - [Rpc.NameService.ResolveSpaceId.Request](#anytype-Rpc-NameService-ResolveSpaceId-Request)
    - [Rpc.NameService.ResolveSpaceId.Response](#anytype-Rpc-NameService-ResolveSpaceId-Response)
    - [Rpc.NameService.ResolveSpaceId.Response.Error](#anytype-Rpc-NameService-ResolveSpaceId-Response-Error)
    - [Rpc.NameService.UserAccount](#anytype-Rpc-NameService-UserAccount)
    - [Rpc.NameService.UserAccount.Get](#anytype-Rpc-NameService-UserAccount-Get)
    - [Rpc.NameService.UserAccount.Get.Request](#anytype-Rpc-NameService-UserAccount-Get-Request)
    - [Rpc.NameService.UserAccount.Get.Response](#anytype-Rpc-NameService-UserAccount-Get-Response)
    - [Rpc.NameService.UserAccount.Get.Response.Error](#anytype-Rpc-NameService-UserAccount-Get-Response-Error)
    - [Rpc.Navigation](#anytype-Rpc-Navigation)
    - [Rpc.Navigation.GetObjectInfoWithLinks](#anytype-Rpc-Navigation-GetObjectInfoWithLinks)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Request)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error)
    - [Rpc.Navigation.ListObjects](#anytype-Rpc-Navigation-ListObjects)
    - [Rpc.Navigation.ListObjects.Request](#anytype-Rpc-Navigation-ListObjects-Request)
    - [Rpc.Navigation.ListObjects.Response](#anytype-Rpc-Navigation-ListObjects-Response)
    - [Rpc.Navigation.ListObjects.Response.Error](#anytype-Rpc-Navigation-ListObjects-Response-Error)
    - [Rpc.Notification](#anytype-Rpc-Notification)
    - [Rpc.Notification.List](#anytype-Rpc-Notification-List)
    - [Rpc.Notification.List.Request](#anytype-Rpc-Notification-List-Request)
    - [Rpc.Notification.List.Response](#anytype-Rpc-Notification-List-Response)
    - [Rpc.Notification.List.Response.Error](#anytype-Rpc-Notification-List-Response-Error)
    - [Rpc.Notification.Reply](#anytype-Rpc-Notification-Reply)
    - [Rpc.Notification.Reply.Request](#anytype-Rpc-Notification-Reply-Request)
    - [Rpc.Notification.Reply.Response](#anytype-Rpc-Notification-Reply-Response)
    - [Rpc.Notification.Reply.Response.Error](#anytype-Rpc-Notification-Reply-Response-Error)
    - [Rpc.Notification.Test](#anytype-Rpc-Notification-Test)
    - [Rpc.Notification.Test.Request](#anytype-Rpc-Notification-Test-Request)
    - [Rpc.Notification.Test.Response](#anytype-Rpc-Notification-Test-Response)
    - [Rpc.Notification.Test.Response.Error](#anytype-Rpc-Notification-Test-Response-Error)
    - [Rpc.Object](#anytype-Rpc-Object)
    - [Rpc.Object.ApplyTemplate](#anytype-Rpc-Object-ApplyTemplate)
    - [Rpc.Object.ApplyTemplate.Request](#anytype-Rpc-Object-ApplyTemplate-Request)
    - [Rpc.Object.ApplyTemplate.Response](#anytype-Rpc-Object-ApplyTemplate-Response)
    - [Rpc.Object.ApplyTemplate.Response.Error](#anytype-Rpc-Object-ApplyTemplate-Response-Error)
    - [Rpc.Object.BookmarkFetch](#anytype-Rpc-Object-BookmarkFetch)
    - [Rpc.Object.BookmarkFetch.Request](#anytype-Rpc-Object-BookmarkFetch-Request)
    - [Rpc.Object.BookmarkFetch.Response](#anytype-Rpc-Object-BookmarkFetch-Response)
    - [Rpc.Object.BookmarkFetch.Response.Error](#anytype-Rpc-Object-BookmarkFetch-Response-Error)
    - [Rpc.Object.ChatAdd](#anytype-Rpc-Object-ChatAdd)
    - [Rpc.Object.ChatAdd.Request](#anytype-Rpc-Object-ChatAdd-Request)
    - [Rpc.Object.ChatAdd.Response](#anytype-Rpc-Object-ChatAdd-Response)
    - [Rpc.Object.ChatAdd.Response.Error](#anytype-Rpc-Object-ChatAdd-Response-Error)
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
    - [Rpc.Object.CreateFromUrl](#anytype-Rpc-Object-CreateFromUrl)
    - [Rpc.Object.CreateFromUrl.Request](#anytype-Rpc-Object-CreateFromUrl-Request)
    - [Rpc.Object.CreateFromUrl.Response](#anytype-Rpc-Object-CreateFromUrl-Response)
    - [Rpc.Object.CreateFromUrl.Response.Error](#anytype-Rpc-Object-CreateFromUrl-Response-Error)
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
    - [Rpc.Object.CrossSpaceSearchSubscribe](#anytype-Rpc-Object-CrossSpaceSearchSubscribe)
    - [Rpc.Object.CrossSpaceSearchSubscribe.Request](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Request)
    - [Rpc.Object.CrossSpaceSearchSubscribe.Response](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response)
    - [Rpc.Object.CrossSpaceSearchSubscribe.Response.Error](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response-Error)
    - [Rpc.Object.CrossSpaceSearchUnsubscribe](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe)
    - [Rpc.Object.CrossSpaceSearchUnsubscribe.Request](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Request)
    - [Rpc.Object.CrossSpaceSearchUnsubscribe.Response](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response)
    - [Rpc.Object.CrossSpaceSearchUnsubscribe.Response.Error](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response-Error)
    - [Rpc.Object.DateByTimestamp](#anytype-Rpc-Object-DateByTimestamp)
    - [Rpc.Object.DateByTimestamp.Request](#anytype-Rpc-Object-DateByTimestamp-Request)
    - [Rpc.Object.DateByTimestamp.Response](#anytype-Rpc-Object-DateByTimestamp-Response)
    - [Rpc.Object.DateByTimestamp.Response.Error](#anytype-Rpc-Object-DateByTimestamp-Response-Error)
    - [Rpc.Object.Duplicate](#anytype-Rpc-Object-Duplicate)
    - [Rpc.Object.Duplicate.Request](#anytype-Rpc-Object-Duplicate-Request)
    - [Rpc.Object.Duplicate.Response](#anytype-Rpc-Object-Duplicate-Response)
    - [Rpc.Object.Duplicate.Response.Error](#anytype-Rpc-Object-Duplicate-Response-Error)
    - [Rpc.Object.Export](#anytype-Rpc-Object-Export)
    - [Rpc.Object.Export.Request](#anytype-Rpc-Object-Export-Request)
    - [Rpc.Object.Export.Response](#anytype-Rpc-Object-Export-Response)
    - [Rpc.Object.Export.Response.Error](#anytype-Rpc-Object-Export-Response-Error)
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
    - [Rpc.Object.ImportExperience](#anytype-Rpc-Object-ImportExperience)
    - [Rpc.Object.ImportExperience.Request](#anytype-Rpc-Object-ImportExperience-Request)
    - [Rpc.Object.ImportExperience.Response](#anytype-Rpc-Object-ImportExperience-Response)
    - [Rpc.Object.ImportExperience.Response.Error](#anytype-Rpc-Object-ImportExperience-Response-Error)
    - [Rpc.Object.ImportList](#anytype-Rpc-Object-ImportList)
    - [Rpc.Object.ImportList.ImportResponse](#anytype-Rpc-Object-ImportList-ImportResponse)
    - [Rpc.Object.ImportList.Request](#anytype-Rpc-Object-ImportList-Request)
    - [Rpc.Object.ImportList.Response](#anytype-Rpc-Object-ImportList-Response)
    - [Rpc.Object.ImportList.Response.Error](#anytype-Rpc-Object-ImportList-Response-Error)
    - [Rpc.Object.ImportUseCase](#anytype-Rpc-Object-ImportUseCase)
    - [Rpc.Object.ImportUseCase.Request](#anytype-Rpc-Object-ImportUseCase-Request)
    - [Rpc.Object.ImportUseCase.Response](#anytype-Rpc-Object-ImportUseCase-Response)
    - [Rpc.Object.ImportUseCase.Response.Error](#anytype-Rpc-Object-ImportUseCase-Response-Error)
    - [Rpc.Object.ListDelete](#anytype-Rpc-Object-ListDelete)
    - [Rpc.Object.ListDelete.Request](#anytype-Rpc-Object-ListDelete-Request)
    - [Rpc.Object.ListDelete.Response](#anytype-Rpc-Object-ListDelete-Response)
    - [Rpc.Object.ListDelete.Response.Error](#anytype-Rpc-Object-ListDelete-Response-Error)
    - [Rpc.Object.ListDuplicate](#anytype-Rpc-Object-ListDuplicate)
    - [Rpc.Object.ListDuplicate.Request](#anytype-Rpc-Object-ListDuplicate-Request)
    - [Rpc.Object.ListDuplicate.Response](#anytype-Rpc-Object-ListDuplicate-Response)
    - [Rpc.Object.ListDuplicate.Response.Error](#anytype-Rpc-Object-ListDuplicate-Response-Error)
    - [Rpc.Object.ListExport](#anytype-Rpc-Object-ListExport)
    - [Rpc.Object.ListExport.RelationsWhiteList](#anytype-Rpc-Object-ListExport-RelationsWhiteList)
    - [Rpc.Object.ListExport.Request](#anytype-Rpc-Object-ListExport-Request)
    - [Rpc.Object.ListExport.Response](#anytype-Rpc-Object-ListExport-Response)
    - [Rpc.Object.ListExport.Response.Error](#anytype-Rpc-Object-ListExport-Response-Error)
    - [Rpc.Object.ListExport.StateFilters](#anytype-Rpc-Object-ListExport-StateFilters)
    - [Rpc.Object.ListModifyDetailValues](#anytype-Rpc-Object-ListModifyDetailValues)
    - [Rpc.Object.ListModifyDetailValues.Request](#anytype-Rpc-Object-ListModifyDetailValues-Request)
    - [Rpc.Object.ListModifyDetailValues.Request.Operation](#anytype-Rpc-Object-ListModifyDetailValues-Request-Operation)
    - [Rpc.Object.ListModifyDetailValues.Response](#anytype-Rpc-Object-ListModifyDetailValues-Response)
    - [Rpc.Object.ListModifyDetailValues.Response.Error](#anytype-Rpc-Object-ListModifyDetailValues-Response-Error)
    - [Rpc.Object.ListSetDetails](#anytype-Rpc-Object-ListSetDetails)
    - [Rpc.Object.ListSetDetails.Request](#anytype-Rpc-Object-ListSetDetails-Request)
    - [Rpc.Object.ListSetDetails.Response](#anytype-Rpc-Object-ListSetDetails-Response)
    - [Rpc.Object.ListSetDetails.Response.Error](#anytype-Rpc-Object-ListSetDetails-Response-Error)
    - [Rpc.Object.ListSetIsArchived](#anytype-Rpc-Object-ListSetIsArchived)
    - [Rpc.Object.ListSetIsArchived.Request](#anytype-Rpc-Object-ListSetIsArchived-Request)
    - [Rpc.Object.ListSetIsArchived.Response](#anytype-Rpc-Object-ListSetIsArchived-Response)
    - [Rpc.Object.ListSetIsArchived.Response.Error](#anytype-Rpc-Object-ListSetIsArchived-Response-Error)
    - [Rpc.Object.ListSetIsFavorite](#anytype-Rpc-Object-ListSetIsFavorite)
    - [Rpc.Object.ListSetIsFavorite.Request](#anytype-Rpc-Object-ListSetIsFavorite-Request)
    - [Rpc.Object.ListSetIsFavorite.Response](#anytype-Rpc-Object-ListSetIsFavorite-Response)
    - [Rpc.Object.ListSetIsFavorite.Response.Error](#anytype-Rpc-Object-ListSetIsFavorite-Response-Error)
    - [Rpc.Object.ListSetObjectType](#anytype-Rpc-Object-ListSetObjectType)
    - [Rpc.Object.ListSetObjectType.Request](#anytype-Rpc-Object-ListSetObjectType-Request)
    - [Rpc.Object.ListSetObjectType.Response](#anytype-Rpc-Object-ListSetObjectType-Response)
    - [Rpc.Object.ListSetObjectType.Response.Error](#anytype-Rpc-Object-ListSetObjectType-Response-Error)
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
    - [Rpc.Object.Refresh](#anytype-Rpc-Object-Refresh)
    - [Rpc.Object.Refresh.Request](#anytype-Rpc-Object-Refresh-Request)
    - [Rpc.Object.Refresh.Response](#anytype-Rpc-Object-Refresh-Response)
    - [Rpc.Object.Refresh.Response.Error](#anytype-Rpc-Object-Refresh-Response-Error)
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
    - [Rpc.Object.SearchWithMeta](#anytype-Rpc-Object-SearchWithMeta)
    - [Rpc.Object.SearchWithMeta.Request](#anytype-Rpc-Object-SearchWithMeta-Request)
    - [Rpc.Object.SearchWithMeta.Response](#anytype-Rpc-Object-SearchWithMeta-Response)
    - [Rpc.Object.SearchWithMeta.Response.Error](#anytype-Rpc-Object-SearchWithMeta-Response-Error)
    - [Rpc.Object.SetBreadcrumbs](#anytype-Rpc-Object-SetBreadcrumbs)
    - [Rpc.Object.SetBreadcrumbs.Request](#anytype-Rpc-Object-SetBreadcrumbs-Request)
    - [Rpc.Object.SetBreadcrumbs.Response](#anytype-Rpc-Object-SetBreadcrumbs-Response)
    - [Rpc.Object.SetBreadcrumbs.Response.Error](#anytype-Rpc-Object-SetBreadcrumbs-Response-Error)
    - [Rpc.Object.SetDetails](#anytype-Rpc-Object-SetDetails)
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
    - [Rpc.ObjectType.ListConflictingRelations](#anytype-Rpc-ObjectType-ListConflictingRelations)
    - [Rpc.ObjectType.ListConflictingRelations.Request](#anytype-Rpc-ObjectType-ListConflictingRelations-Request)
    - [Rpc.ObjectType.ListConflictingRelations.Response](#anytype-Rpc-ObjectType-ListConflictingRelations-Response)
    - [Rpc.ObjectType.ListConflictingRelations.Response.Error](#anytype-Rpc-ObjectType-ListConflictingRelations-Response-Error)
    - [Rpc.ObjectType.Recommended](#anytype-Rpc-ObjectType-Recommended)
    - [Rpc.ObjectType.Recommended.FeaturedRelationsSet](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet)
    - [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Request](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Request)
    - [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response)
    - [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response.Error](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response-Error)
    - [Rpc.ObjectType.Recommended.RelationsSet](#anytype-Rpc-ObjectType-Recommended-RelationsSet)
    - [Rpc.ObjectType.Recommended.RelationsSet.Request](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Request)
    - [Rpc.ObjectType.Recommended.RelationsSet.Response](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Response)
    - [Rpc.ObjectType.Recommended.RelationsSet.Response.Error](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Response-Error)
    - [Rpc.ObjectType.Relation](#anytype-Rpc-ObjectType-Relation)
    - [Rpc.ObjectType.Relation.Add](#anytype-Rpc-ObjectType-Relation-Add)
    - [Rpc.ObjectType.Relation.Add.Request](#anytype-Rpc-ObjectType-Relation-Add-Request)
    - [Rpc.ObjectType.Relation.Add.Response](#anytype-Rpc-ObjectType-Relation-Add-Response)
    - [Rpc.ObjectType.Relation.Add.Response.Error](#anytype-Rpc-ObjectType-Relation-Add-Response-Error)
    - [Rpc.ObjectType.Relation.Remove](#anytype-Rpc-ObjectType-Relation-Remove)
    - [Rpc.ObjectType.Relation.Remove.Request](#anytype-Rpc-ObjectType-Relation-Remove-Request)
    - [Rpc.ObjectType.Relation.Remove.Response](#anytype-Rpc-ObjectType-Relation-Remove-Response)
    - [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype-Rpc-ObjectType-Relation-Remove-Response-Error)
    - [Rpc.ObjectType.ResolveLayoutConflicts](#anytype-Rpc-ObjectType-ResolveLayoutConflicts)
    - [Rpc.ObjectType.ResolveLayoutConflicts.Request](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Request)
    - [Rpc.ObjectType.ResolveLayoutConflicts.Response](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response)
    - [Rpc.ObjectType.ResolveLayoutConflicts.Response.Error](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response-Error)
    - [Rpc.ObjectType.SetOrder](#anytype-Rpc-ObjectType-SetOrder)
    - [Rpc.ObjectType.SetOrder.Request](#anytype-Rpc-ObjectType-SetOrder-Request)
    - [Rpc.ObjectType.SetOrder.Response](#anytype-Rpc-ObjectType-SetOrder-Response)
    - [Rpc.ObjectType.SetOrder.Response.Error](#anytype-Rpc-ObjectType-SetOrder-Response-Error)
    - [Rpc.Process](#anytype-Rpc-Process)
    - [Rpc.Process.Cancel](#anytype-Rpc-Process-Cancel)
    - [Rpc.Process.Cancel.Request](#anytype-Rpc-Process-Cancel-Request)
    - [Rpc.Process.Cancel.Response](#anytype-Rpc-Process-Cancel-Response)
    - [Rpc.Process.Cancel.Response.Error](#anytype-Rpc-Process-Cancel-Response-Error)
    - [Rpc.Process.Subscribe](#anytype-Rpc-Process-Subscribe)
    - [Rpc.Process.Subscribe.Request](#anytype-Rpc-Process-Subscribe-Request)
    - [Rpc.Process.Subscribe.Response](#anytype-Rpc-Process-Subscribe-Response)
    - [Rpc.Process.Subscribe.Response.Error](#anytype-Rpc-Process-Subscribe-Response-Error)
    - [Rpc.Process.Unsubscribe](#anytype-Rpc-Process-Unsubscribe)
    - [Rpc.Process.Unsubscribe.Request](#anytype-Rpc-Process-Unsubscribe-Request)
    - [Rpc.Process.Unsubscribe.Response](#anytype-Rpc-Process-Unsubscribe-Response)
    - [Rpc.Process.Unsubscribe.Response.Error](#anytype-Rpc-Process-Unsubscribe-Response-Error)
    - [Rpc.Publishing](#anytype-Rpc-Publishing)
    - [Rpc.Publishing.Create](#anytype-Rpc-Publishing-Create)
    - [Rpc.Publishing.Create.Request](#anytype-Rpc-Publishing-Create-Request)
    - [Rpc.Publishing.Create.Response](#anytype-Rpc-Publishing-Create-Response)
    - [Rpc.Publishing.Create.Response.Error](#anytype-Rpc-Publishing-Create-Response-Error)
    - [Rpc.Publishing.GetStatus](#anytype-Rpc-Publishing-GetStatus)
    - [Rpc.Publishing.GetStatus.Request](#anytype-Rpc-Publishing-GetStatus-Request)
    - [Rpc.Publishing.GetStatus.Response](#anytype-Rpc-Publishing-GetStatus-Response)
    - [Rpc.Publishing.GetStatus.Response.Error](#anytype-Rpc-Publishing-GetStatus-Response-Error)
    - [Rpc.Publishing.List](#anytype-Rpc-Publishing-List)
    - [Rpc.Publishing.List.Request](#anytype-Rpc-Publishing-List-Request)
    - [Rpc.Publishing.List.Response](#anytype-Rpc-Publishing-List-Response)
    - [Rpc.Publishing.List.Response.Error](#anytype-Rpc-Publishing-List-Response-Error)
    - [Rpc.Publishing.PublishState](#anytype-Rpc-Publishing-PublishState)
    - [Rpc.Publishing.Remove](#anytype-Rpc-Publishing-Remove)
    - [Rpc.Publishing.Remove.Request](#anytype-Rpc-Publishing-Remove-Request)
    - [Rpc.Publishing.Remove.Response](#anytype-Rpc-Publishing-Remove-Response)
    - [Rpc.Publishing.Remove.Response.Error](#anytype-Rpc-Publishing-Remove-Response-Error)
    - [Rpc.Publishing.ResolveUri](#anytype-Rpc-Publishing-ResolveUri)
    - [Rpc.Publishing.ResolveUri.Request](#anytype-Rpc-Publishing-ResolveUri-Request)
    - [Rpc.Publishing.ResolveUri.Response](#anytype-Rpc-Publishing-ResolveUri-Response)
    - [Rpc.Publishing.ResolveUri.Response.Error](#anytype-Rpc-Publishing-ResolveUri-Response-Error)
    - [Rpc.PushNotification](#anytype-Rpc-PushNotification)
    - [Rpc.PushNotification.RegisterToken](#anytype-Rpc-PushNotification-RegisterToken)
    - [Rpc.PushNotification.RegisterToken.Request](#anytype-Rpc-PushNotification-RegisterToken-Request)
    - [Rpc.PushNotification.RegisterToken.Response](#anytype-Rpc-PushNotification-RegisterToken-Response)
    - [Rpc.PushNotification.RegisterToken.Response.Error](#anytype-Rpc-PushNotification-RegisterToken-Response-Error)
    - [Rpc.PushNotification.ResetIds](#anytype-Rpc-PushNotification-ResetIds)
    - [Rpc.PushNotification.ResetIds.Request](#anytype-Rpc-PushNotification-ResetIds-Request)
    - [Rpc.PushNotification.ResetIds.Response](#anytype-Rpc-PushNotification-ResetIds-Response)
    - [Rpc.PushNotification.ResetIds.Response.Error](#anytype-Rpc-PushNotification-ResetIds-Response-Error)
    - [Rpc.PushNotification.SetForceModeIds](#anytype-Rpc-PushNotification-SetForceModeIds)
    - [Rpc.PushNotification.SetForceModeIds.Request](#anytype-Rpc-PushNotification-SetForceModeIds-Request)
    - [Rpc.PushNotification.SetForceModeIds.Response](#anytype-Rpc-PushNotification-SetForceModeIds-Response)
    - [Rpc.PushNotification.SetForceModeIds.Response.Error](#anytype-Rpc-PushNotification-SetForceModeIds-Response-Error)
    - [Rpc.PushNotification.SetSpaceMode](#anytype-Rpc-PushNotification-SetSpaceMode)
    - [Rpc.PushNotification.SetSpaceMode.Request](#anytype-Rpc-PushNotification-SetSpaceMode-Request)
    - [Rpc.PushNotification.SetSpaceMode.Response](#anytype-Rpc-PushNotification-SetSpaceMode-Response)
    - [Rpc.PushNotification.SetSpaceMode.Response.Error](#anytype-Rpc-PushNotification-SetSpaceMode-Response-Error)
    - [Rpc.Relation](#anytype-Rpc-Relation)
    - [Rpc.Relation.ListRemoveOption](#anytype-Rpc-Relation-ListRemoveOption)
    - [Rpc.Relation.ListRemoveOption.Request](#anytype-Rpc-Relation-ListRemoveOption-Request)
    - [Rpc.Relation.ListRemoveOption.Response](#anytype-Rpc-Relation-ListRemoveOption-Response)
    - [Rpc.Relation.ListRemoveOption.Response.Error](#anytype-Rpc-Relation-ListRemoveOption-Response-Error)
    - [Rpc.Relation.ListWithValue](#anytype-Rpc-Relation-ListWithValue)
    - [Rpc.Relation.ListWithValue.Request](#anytype-Rpc-Relation-ListWithValue-Request)
    - [Rpc.Relation.ListWithValue.Response](#anytype-Rpc-Relation-ListWithValue-Response)
    - [Rpc.Relation.ListWithValue.Response.Error](#anytype-Rpc-Relation-ListWithValue-Response-Error)
    - [Rpc.Relation.ListWithValue.Response.ResponseItem](#anytype-Rpc-Relation-ListWithValue-Response-ResponseItem)
    - [Rpc.Relation.Option](#anytype-Rpc-Relation-Option)
    - [Rpc.Relation.Option.SetOrder](#anytype-Rpc-Relation-Option-SetOrder)
    - [Rpc.Relation.Option.SetOrder.Request](#anytype-Rpc-Relation-Option-SetOrder-Request)
    - [Rpc.Relation.Option.SetOrder.Response](#anytype-Rpc-Relation-Option-SetOrder-Response)
    - [Rpc.Relation.Option.SetOrder.Response.Error](#anytype-Rpc-Relation-Option-SetOrder-Response-Error)
    - [Rpc.Relation.Options](#anytype-Rpc-Relation-Options)
    - [Rpc.Relation.Options.Request](#anytype-Rpc-Relation-Options-Request)
    - [Rpc.Relation.Options.Response](#anytype-Rpc-Relation-Options-Response)
    - [Rpc.Relation.Options.Response.Error](#anytype-Rpc-Relation-Options-Response-Error)
    - [Rpc.Space](#anytype-Rpc-Space)
    - [Rpc.Space.Delete](#anytype-Rpc-Space-Delete)
    - [Rpc.Space.Delete.Request](#anytype-Rpc-Space-Delete-Request)
    - [Rpc.Space.Delete.Response](#anytype-Rpc-Space-Delete-Response)
    - [Rpc.Space.Delete.Response.Error](#anytype-Rpc-Space-Delete-Response-Error)
    - [Rpc.Space.InviteChange](#anytype-Rpc-Space-InviteChange)
    - [Rpc.Space.InviteChange.Request](#anytype-Rpc-Space-InviteChange-Request)
    - [Rpc.Space.InviteChange.Response](#anytype-Rpc-Space-InviteChange-Response)
    - [Rpc.Space.InviteChange.Response.Error](#anytype-Rpc-Space-InviteChange-Response-Error)
    - [Rpc.Space.InviteGenerate](#anytype-Rpc-Space-InviteGenerate)
    - [Rpc.Space.InviteGenerate.Request](#anytype-Rpc-Space-InviteGenerate-Request)
    - [Rpc.Space.InviteGenerate.Response](#anytype-Rpc-Space-InviteGenerate-Response)
    - [Rpc.Space.InviteGenerate.Response.Error](#anytype-Rpc-Space-InviteGenerate-Response-Error)
    - [Rpc.Space.InviteGetCurrent](#anytype-Rpc-Space-InviteGetCurrent)
    - [Rpc.Space.InviteGetCurrent.Request](#anytype-Rpc-Space-InviteGetCurrent-Request)
    - [Rpc.Space.InviteGetCurrent.Response](#anytype-Rpc-Space-InviteGetCurrent-Response)
    - [Rpc.Space.InviteGetCurrent.Response.Error](#anytype-Rpc-Space-InviteGetCurrent-Response-Error)
    - [Rpc.Space.InviteGetGuest](#anytype-Rpc-Space-InviteGetGuest)
    - [Rpc.Space.InviteGetGuest.Request](#anytype-Rpc-Space-InviteGetGuest-Request)
    - [Rpc.Space.InviteGetGuest.Response](#anytype-Rpc-Space-InviteGetGuest-Response)
    - [Rpc.Space.InviteGetGuest.Response.Error](#anytype-Rpc-Space-InviteGetGuest-Response-Error)
    - [Rpc.Space.InviteRevoke](#anytype-Rpc-Space-InviteRevoke)
    - [Rpc.Space.InviteRevoke.Request](#anytype-Rpc-Space-InviteRevoke-Request)
    - [Rpc.Space.InviteRevoke.Response](#anytype-Rpc-Space-InviteRevoke-Response)
    - [Rpc.Space.InviteRevoke.Response.Error](#anytype-Rpc-Space-InviteRevoke-Response-Error)
    - [Rpc.Space.InviteView](#anytype-Rpc-Space-InviteView)
    - [Rpc.Space.InviteView.Request](#anytype-Rpc-Space-InviteView-Request)
    - [Rpc.Space.InviteView.Response](#anytype-Rpc-Space-InviteView-Response)
    - [Rpc.Space.InviteView.Response.Error](#anytype-Rpc-Space-InviteView-Response-Error)
    - [Rpc.Space.Join](#anytype-Rpc-Space-Join)
    - [Rpc.Space.Join.Request](#anytype-Rpc-Space-Join-Request)
    - [Rpc.Space.Join.Response](#anytype-Rpc-Space-Join-Response)
    - [Rpc.Space.Join.Response.Error](#anytype-Rpc-Space-Join-Response-Error)
    - [Rpc.Space.JoinCancel](#anytype-Rpc-Space-JoinCancel)
    - [Rpc.Space.JoinCancel.Request](#anytype-Rpc-Space-JoinCancel-Request)
    - [Rpc.Space.JoinCancel.Response](#anytype-Rpc-Space-JoinCancel-Response)
    - [Rpc.Space.JoinCancel.Response.Error](#anytype-Rpc-Space-JoinCancel-Response-Error)
    - [Rpc.Space.LeaveApprove](#anytype-Rpc-Space-LeaveApprove)
    - [Rpc.Space.LeaveApprove.Request](#anytype-Rpc-Space-LeaveApprove-Request)
    - [Rpc.Space.LeaveApprove.Response](#anytype-Rpc-Space-LeaveApprove-Response)
    - [Rpc.Space.LeaveApprove.Response.Error](#anytype-Rpc-Space-LeaveApprove-Response-Error)
    - [Rpc.Space.MakeShareable](#anytype-Rpc-Space-MakeShareable)
    - [Rpc.Space.MakeShareable.Request](#anytype-Rpc-Space-MakeShareable-Request)
    - [Rpc.Space.MakeShareable.Response](#anytype-Rpc-Space-MakeShareable-Response)
    - [Rpc.Space.MakeShareable.Response.Error](#anytype-Rpc-Space-MakeShareable-Response-Error)
    - [Rpc.Space.ParticipantPermissionsChange](#anytype-Rpc-Space-ParticipantPermissionsChange)
    - [Rpc.Space.ParticipantPermissionsChange.Request](#anytype-Rpc-Space-ParticipantPermissionsChange-Request)
    - [Rpc.Space.ParticipantPermissionsChange.Response](#anytype-Rpc-Space-ParticipantPermissionsChange-Response)
    - [Rpc.Space.ParticipantPermissionsChange.Response.Error](#anytype-Rpc-Space-ParticipantPermissionsChange-Response-Error)
    - [Rpc.Space.ParticipantRemove](#anytype-Rpc-Space-ParticipantRemove)
    - [Rpc.Space.ParticipantRemove.Request](#anytype-Rpc-Space-ParticipantRemove-Request)
    - [Rpc.Space.ParticipantRemove.Response](#anytype-Rpc-Space-ParticipantRemove-Response)
    - [Rpc.Space.ParticipantRemove.Response.Error](#anytype-Rpc-Space-ParticipantRemove-Response-Error)
    - [Rpc.Space.RequestApprove](#anytype-Rpc-Space-RequestApprove)
    - [Rpc.Space.RequestApprove.Request](#anytype-Rpc-Space-RequestApprove-Request)
    - [Rpc.Space.RequestApprove.Response](#anytype-Rpc-Space-RequestApprove-Response)
    - [Rpc.Space.RequestApprove.Response.Error](#anytype-Rpc-Space-RequestApprove-Response-Error)
    - [Rpc.Space.RequestDecline](#anytype-Rpc-Space-RequestDecline)
    - [Rpc.Space.RequestDecline.Request](#anytype-Rpc-Space-RequestDecline-Request)
    - [Rpc.Space.RequestDecline.Response](#anytype-Rpc-Space-RequestDecline-Response)
    - [Rpc.Space.RequestDecline.Response.Error](#anytype-Rpc-Space-RequestDecline-Response-Error)
    - [Rpc.Space.SetOrder](#anytype-Rpc-Space-SetOrder)
    - [Rpc.Space.SetOrder.Request](#anytype-Rpc-Space-SetOrder-Request)
    - [Rpc.Space.SetOrder.Response](#anytype-Rpc-Space-SetOrder-Response)
    - [Rpc.Space.SetOrder.Response.Error](#anytype-Rpc-Space-SetOrder-Response-Error)
    - [Rpc.Space.StopSharing](#anytype-Rpc-Space-StopSharing)
    - [Rpc.Space.StopSharing.Request](#anytype-Rpc-Space-StopSharing-Request)
    - [Rpc.Space.StopSharing.Response](#anytype-Rpc-Space-StopSharing-Response)
    - [Rpc.Space.StopSharing.Response.Error](#anytype-Rpc-Space-StopSharing-Response-Error)
    - [Rpc.Space.UnsetOrder](#anytype-Rpc-Space-UnsetOrder)
    - [Rpc.Space.UnsetOrder.Request](#anytype-Rpc-Space-UnsetOrder-Request)
    - [Rpc.Space.UnsetOrder.Response](#anytype-Rpc-Space-UnsetOrder-Response)
    - [Rpc.Space.UnsetOrder.Response.Error](#anytype-Rpc-Space-UnsetOrder-Response-Error)
    - [Rpc.Template](#anytype-Rpc-Template)
    - [Rpc.Template.Clone](#anytype-Rpc-Template-Clone)
    - [Rpc.Template.Clone.Request](#anytype-Rpc-Template-Clone-Request)
    - [Rpc.Template.Clone.Response](#anytype-Rpc-Template-Clone-Response)
    - [Rpc.Template.Clone.Response.Error](#anytype-Rpc-Template-Clone-Response-Error)
    - [Rpc.Template.CreateFromObject](#anytype-Rpc-Template-CreateFromObject)
    - [Rpc.Template.CreateFromObject.Request](#anytype-Rpc-Template-CreateFromObject-Request)
    - [Rpc.Template.CreateFromObject.Response](#anytype-Rpc-Template-CreateFromObject-Response)
    - [Rpc.Template.CreateFromObject.Response.Error](#anytype-Rpc-Template-CreateFromObject-Response-Error)
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
    - [Rpc.Workspace.Open](#anytype-Rpc-Workspace-Open)
    - [Rpc.Workspace.Open.Request](#anytype-Rpc-Workspace-Open-Request)
    - [Rpc.Workspace.Open.Response](#anytype-Rpc-Workspace-Open-Response)
    - [Rpc.Workspace.Open.Response.Error](#anytype-Rpc-Workspace-Open-Response-Error)
    - [Rpc.Workspace.Select](#anytype-Rpc-Workspace-Select)
    - [Rpc.Workspace.Select.Request](#anytype-Rpc-Workspace-Select-Request)
    - [Rpc.Workspace.Select.Response](#anytype-Rpc-Workspace-Select-Response)
    - [Rpc.Workspace.Select.Response.Error](#anytype-Rpc-Workspace-Select-Response-Error)
    - [Rpc.Workspace.SetInfo](#anytype-Rpc-Workspace-SetInfo)
    - [Rpc.Workspace.SetInfo.Request](#anytype-Rpc-Workspace-SetInfo-Request)
    - [Rpc.Workspace.SetInfo.Response](#anytype-Rpc-Workspace-SetInfo-Response)
    - [Rpc.Workspace.SetInfo.Response.Error](#anytype-Rpc-Workspace-SetInfo-Response-Error)
    - [StreamRequest](#anytype-StreamRequest)
  
    - [Rpc.AI.Autofill.Request.AutofillMode](#anytype-Rpc-AI-Autofill-Request-AutofillMode)
    - [Rpc.AI.Autofill.Response.Error.Code](#anytype-Rpc-AI-Autofill-Response-Error-Code)
    - [Rpc.AI.ListSummary.Response.Error.Code](#anytype-Rpc-AI-ListSummary-Response-Error-Code)
    - [Rpc.AI.ObjectCreateFromUrl.Response.Error.Code](#anytype-Rpc-AI-ObjectCreateFromUrl-Response-Error-Code)
    - [Rpc.AI.Provider](#anytype-Rpc-AI-Provider)
    - [Rpc.AI.WritingTools.Request.Language](#anytype-Rpc-AI-WritingTools-Request-Language)
    - [Rpc.AI.WritingTools.Request.WritingMode](#anytype-Rpc-AI-WritingTools-Request-WritingMode)
    - [Rpc.AI.WritingTools.Response.Error.Code](#anytype-Rpc-AI-WritingTools-Response-Error-Code)
    - [Rpc.Account.ChangeJsonApiAddr.Response.Error.Code](#anytype-Rpc-Account-ChangeJsonApiAddr-Response-Error-Code)
    - [Rpc.Account.ChangeNetworkConfigAndRestart.Response.Error.Code](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response-Error-Code)
    - [Rpc.Account.ConfigUpdate.Response.Error.Code](#anytype-Rpc-Account-ConfigUpdate-Response-Error-Code)
    - [Rpc.Account.ConfigUpdate.Timezones](#anytype-Rpc-Account-ConfigUpdate-Timezones)
    - [Rpc.Account.Create.Response.Error.Code](#anytype-Rpc-Account-Create-Response-Error-Code)
    - [Rpc.Account.Delete.Response.Error.Code](#anytype-Rpc-Account-Delete-Response-Error-Code)
    - [Rpc.Account.EnableLocalNetworkSync.Response.Error.Code](#anytype-Rpc-Account-EnableLocalNetworkSync-Response-Error-Code)
    - [Rpc.Account.LocalLink.CreateApp.Response.Error.Code](#anytype-Rpc-Account-LocalLink-CreateApp-Response-Error-Code)
    - [Rpc.Account.LocalLink.ListApps.Response.Error.Code](#anytype-Rpc-Account-LocalLink-ListApps-Response-Error-Code)
    - [Rpc.Account.LocalLink.NewChallenge.Response.Error.Code](#anytype-Rpc-Account-LocalLink-NewChallenge-Response-Error-Code)
    - [Rpc.Account.LocalLink.RevokeApp.Response.Error.Code](#anytype-Rpc-Account-LocalLink-RevokeApp-Response-Error-Code)
    - [Rpc.Account.LocalLink.SolveChallenge.Response.Error.Code](#anytype-Rpc-Account-LocalLink-SolveChallenge-Response-Error-Code)
    - [Rpc.Account.Migrate.Response.Error.Code](#anytype-Rpc-Account-Migrate-Response-Error-Code)
    - [Rpc.Account.MigrateCancel.Response.Error.Code](#anytype-Rpc-Account-MigrateCancel-Response-Error-Code)
    - [Rpc.Account.Move.Response.Error.Code](#anytype-Rpc-Account-Move-Response-Error-Code)
    - [Rpc.Account.NetworkMode](#anytype-Rpc-Account-NetworkMode)
    - [Rpc.Account.Recover.Response.Error.Code](#anytype-Rpc-Account-Recover-Response-Error-Code)
    - [Rpc.Account.RecoverFromLegacyExport.Response.Error.Code](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error-Code)
    - [Rpc.Account.RevertDeletion.Response.Error.Code](#anytype-Rpc-Account-RevertDeletion-Response-Error-Code)
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
    - [Rpc.Block.Preview.Response.Error.Code](#anytype-Rpc-Block-Preview-Response-Error-Code)
    - [Rpc.Block.Replace.Response.Error.Code](#anytype-Rpc-Block-Replace-Response-Error-Code)
    - [Rpc.Block.SetCarriage.Response.Error.Code](#anytype-Rpc-Block-SetCarriage-Response-Error-Code)
    - [Rpc.Block.SetFields.Response.Error.Code](#anytype-Rpc-Block-SetFields-Response-Error-Code)
    - [Rpc.Block.Split.Request.Mode](#anytype-Rpc-Block-Split-Request-Mode)
    - [Rpc.Block.Split.Response.Error.Code](#anytype-Rpc-Block-Split-Response-Error-Code)
    - [Rpc.Block.Upload.Response.Error.Code](#anytype-Rpc-Block-Upload-Response-Error-Code)
    - [Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response-Error-Code)
    - [Rpc.BlockBookmark.Fetch.Response.Error.Code](#anytype-Rpc-BlockBookmark-Fetch-Response-Error-Code)
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
    - [Rpc.BlockDataview.Relation.Set.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-Set-Response-Error-Code)
    - [Rpc.BlockDataview.SetSource.Response.Error.Code](#anytype-Rpc-BlockDataview-SetSource-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Add.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Add-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Remove.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Remove-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.Replace.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-Replace-Response-Error-Code)
    - [Rpc.BlockDataview.Sort.SSort.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-SSort-Response-Error-Code)
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
    - [Rpc.BlockFile.SetTargetObjectId.Response.Error.Code](#anytype-Rpc-BlockFile-SetTargetObjectId-Response-Error-Code)
    - [Rpc.BlockImage.SetName.Response.Error.Code](#anytype-Rpc-BlockImage-SetName-Response-Error-Code)
    - [Rpc.BlockImage.SetWidth.Response.Error.Code](#anytype-Rpc-BlockImage-SetWidth-Response-Error-Code)
    - [Rpc.BlockLatex.SetProcessor.Response.Error.Code](#anytype-Rpc-BlockLatex-SetProcessor-Response-Error-Code)
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
    - [Rpc.BlockWidget.SetViewId.Response.Error.Code](#anytype-Rpc-BlockWidget-SetViewId-Response-Error-Code)
    - [Rpc.Broadcast.PayloadEvent.Response.Error.Code](#anytype-Rpc-Broadcast-PayloadEvent-Response-Error-Code)
    - [Rpc.Chat.AddMessage.Response.Error.Code](#anytype-Rpc-Chat-AddMessage-Response-Error-Code)
    - [Rpc.Chat.DeleteMessage.Response.Error.Code](#anytype-Rpc-Chat-DeleteMessage-Response-Error-Code)
    - [Rpc.Chat.EditMessageContent.Response.Error.Code](#anytype-Rpc-Chat-EditMessageContent-Response-Error-Code)
    - [Rpc.Chat.GetMessages.Response.Error.Code](#anytype-Rpc-Chat-GetMessages-Response-Error-Code)
    - [Rpc.Chat.GetMessagesByIds.Response.Error.Code](#anytype-Rpc-Chat-GetMessagesByIds-Response-Error-Code)
    - [Rpc.Chat.ReadAll.Response.Error.Code](#anytype-Rpc-Chat-ReadAll-Response-Error-Code)
    - [Rpc.Chat.ReadMessages.ReadType](#anytype-Rpc-Chat-ReadMessages-ReadType)
    - [Rpc.Chat.ReadMessages.Response.Error.Code](#anytype-Rpc-Chat-ReadMessages-Response-Error-Code)
    - [Rpc.Chat.SubscribeLastMessages.Response.Error.Code](#anytype-Rpc-Chat-SubscribeLastMessages-Response-Error-Code)
    - [Rpc.Chat.SubscribeToMessagePreviews.Response.Error.Code](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-Error-Code)
    - [Rpc.Chat.ToggleMessageReaction.Response.Error.Code](#anytype-Rpc-Chat-ToggleMessageReaction-Response-Error-Code)
    - [Rpc.Chat.Unread.ReadType](#anytype-Rpc-Chat-Unread-ReadType)
    - [Rpc.Chat.Unread.Response.Error.Code](#anytype-Rpc-Chat-Unread-Response-Error-Code)
    - [Rpc.Chat.Unsubscribe.Response.Error.Code](#anytype-Rpc-Chat-Unsubscribe-Response-Error-Code)
    - [Rpc.Chat.UnsubscribeFromMessagePreviews.Response.Error.Code](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response-Error-Code)
    - [Rpc.Debug.AccountSelectTrace.Response.Error.Code](#anytype-Rpc-Debug-AccountSelectTrace-Response-Error-Code)
    - [Rpc.Debug.AnystoreObjectChanges.Request.OrderBy](#anytype-Rpc-Debug-AnystoreObjectChanges-Request-OrderBy)
    - [Rpc.Debug.AnystoreObjectChanges.Response.Error.Code](#anytype-Rpc-Debug-AnystoreObjectChanges-Response-Error-Code)
    - [Rpc.Debug.ExportLocalstore.Response.Error.Code](#anytype-Rpc-Debug-ExportLocalstore-Response-Error-Code)
    - [Rpc.Debug.ExportLog.Response.Error.Code](#anytype-Rpc-Debug-ExportLog-Response-Error-Code)
    - [Rpc.Debug.NetCheck.Response.Error.Code](#anytype-Rpc-Debug-NetCheck-Response-Error-Code)
    - [Rpc.Debug.OpenedObjects.Response.Error.Code](#anytype-Rpc-Debug-OpenedObjects-Response-Error-Code)
    - [Rpc.Debug.Ping.Response.Error.Code](#anytype-Rpc-Debug-Ping-Response-Error-Code)
    - [Rpc.Debug.RunProfiler.Response.Error.Code](#anytype-Rpc-Debug-RunProfiler-Response-Error-Code)
    - [Rpc.Debug.SpaceSummary.Response.Error.Code](#anytype-Rpc-Debug-SpaceSummary-Response-Error-Code)
    - [Rpc.Debug.StackGoroutines.Response.Error.Code](#anytype-Rpc-Debug-StackGoroutines-Response-Error-Code)
    - [Rpc.Debug.Stat.Response.Error.Code](#anytype-Rpc-Debug-Stat-Response-Error-Code)
    - [Rpc.Debug.Subscriptions.Response.Error.Code](#anytype-Rpc-Debug-Subscriptions-Response-Error-Code)
    - [Rpc.Debug.Tree.Response.Error.Code](#anytype-Rpc-Debug-Tree-Response-Error-Code)
    - [Rpc.Debug.TreeHeads.Response.Error.Code](#anytype-Rpc-Debug-TreeHeads-Response-Error-Code)
    - [Rpc.Device.List.Response.Error.Code](#anytype-Rpc-Device-List-Response-Error-Code)
    - [Rpc.Device.NetworkState.Set.Response.Error.Code](#anytype-Rpc-Device-NetworkState-Set-Response-Error-Code)
    - [Rpc.Device.SetName.Response.Error.Code](#anytype-Rpc-Device-SetName-Response-Error-Code)
    - [Rpc.File.CacheCancelDownload.Response.Error.Code](#anytype-Rpc-File-CacheCancelDownload-Response-Error-Code)
    - [Rpc.File.CacheDownload.Response.Error.Code](#anytype-Rpc-File-CacheDownload-Response-Error-Code)
    - [Rpc.File.DiscardPreload.Response.Error.Code](#anytype-Rpc-File-DiscardPreload-Response-Error-Code)
    - [Rpc.File.Download.Response.Error.Code](#anytype-Rpc-File-Download-Response-Error-Code)
    - [Rpc.File.Drop.Response.Error.Code](#anytype-Rpc-File-Drop-Response-Error-Code)
    - [Rpc.File.ListOffload.Response.Error.Code](#anytype-Rpc-File-ListOffload-Response-Error-Code)
    - [Rpc.File.NodeUsage.Response.Error.Code](#anytype-Rpc-File-NodeUsage-Response-Error-Code)
    - [Rpc.File.Offload.Response.Error.Code](#anytype-Rpc-File-Offload-Response-Error-Code)
    - [Rpc.File.Reconcile.Response.Error.Code](#anytype-Rpc-File-Reconcile-Response-Error-Code)
    - [Rpc.File.SetAutoDownload.Response.Error.Code](#anytype-Rpc-File-SetAutoDownload-Response-Error-Code)
    - [Rpc.File.SpaceOffload.Response.Error.Code](#anytype-Rpc-File-SpaceOffload-Response-Error-Code)
    - [Rpc.File.SpaceUsage.Response.Error.Code](#anytype-Rpc-File-SpaceUsage-Response-Error-Code)
    - [Rpc.File.Upload.Response.Error.Code](#anytype-Rpc-File-Upload-Response-Error-Code)
    - [Rpc.Gallery.DownloadIndex.Response.Error.Code](#anytype-Rpc-Gallery-DownloadIndex-Response-Error-Code)
    - [Rpc.Gallery.DownloadManifest.Response.Error.Code](#anytype-Rpc-Gallery-DownloadManifest-Response-Error-Code)
    - [Rpc.GenericErrorResponse.Error.Code](#anytype-Rpc-GenericErrorResponse-Error-Code)
    - [Rpc.History.DiffVersions.Response.Error.Code](#anytype-Rpc-History-DiffVersions-Response-Error-Code)
    - [Rpc.History.GetVersions.Response.Error.Code](#anytype-Rpc-History-GetVersions-Response-Error-Code)
    - [Rpc.History.SetVersion.Response.Error.Code](#anytype-Rpc-History-SetVersion-Response-Error-Code)
    - [Rpc.History.ShowVersion.Response.Error.Code](#anytype-Rpc-History-ShowVersion-Response-Error-Code)
    - [Rpc.Initial.SetParameters.Response.Error.Code](#anytype-Rpc-Initial-SetParameters-Response-Error-Code)
    - [Rpc.LinkPreview.Response.Error.Code](#anytype-Rpc-LinkPreview-Response-Error-Code)
    - [Rpc.Log.Send.Request.Level](#anytype-Rpc-Log-Send-Request-Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype-Rpc-Log-Send-Response-Error-Code)
    - [Rpc.Membership.CodeGetInfo.Response.Error.Code](#anytype-Rpc-Membership-CodeGetInfo-Response-Error-Code)
    - [Rpc.Membership.CodeRedeem.Response.Error.Code](#anytype-Rpc-Membership-CodeRedeem-Response-Error-Code)
    - [Rpc.Membership.Finalize.Response.Error.Code](#anytype-Rpc-Membership-Finalize-Response-Error-Code)
    - [Rpc.Membership.GetPortalLinkUrl.Response.Error.Code](#anytype-Rpc-Membership-GetPortalLinkUrl-Response-Error-Code)
    - [Rpc.Membership.GetStatus.Response.Error.Code](#anytype-Rpc-Membership-GetStatus-Response-Error-Code)
    - [Rpc.Membership.GetTiers.Response.Error.Code](#anytype-Rpc-Membership-GetTiers-Response-Error-Code)
    - [Rpc.Membership.GetVerificationEmail.Response.Error.Code](#anytype-Rpc-Membership-GetVerificationEmail-Response-Error-Code)
    - [Rpc.Membership.GetVerificationEmailStatus.Response.Error.Code](#anytype-Rpc-Membership-GetVerificationEmailStatus-Response-Error-Code)
    - [Rpc.Membership.IsNameValid.Response.Error.Code](#anytype-Rpc-Membership-IsNameValid-Response-Error-Code)
    - [Rpc.Membership.RegisterPaymentRequest.Response.Error.Code](#anytype-Rpc-Membership-RegisterPaymentRequest-Response-Error-Code)
    - [Rpc.Membership.VerifyAppStoreReceipt.Response.Error.Code](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Response-Error-Code)
    - [Rpc.Membership.VerifyEmailCode.Response.Error.Code](#anytype-Rpc-Membership-VerifyEmailCode-Response-Error-Code)
    - [Rpc.MembershipV2.AnyNameAllocate.Response.Error.Code](#anytype-Rpc-MembershipV2-AnyNameAllocate-Response-Error-Code)
    - [Rpc.MembershipV2.AnyNameIsValid.Response.Error.Code](#anytype-Rpc-MembershipV2-AnyNameIsValid-Response-Error-Code)
    - [Rpc.MembershipV2.CartGet.Response.Error.Code](#anytype-Rpc-MembershipV2-CartGet-Response-Error-Code)
    - [Rpc.MembershipV2.CartUpdate.Response.Error.Code](#anytype-Rpc-MembershipV2-CartUpdate-Response-Error-Code)
    - [Rpc.MembershipV2.GetPortalLink.Response.Error.Code](#anytype-Rpc-MembershipV2-GetPortalLink-Response-Error-Code)
    - [Rpc.MembershipV2.GetProducts.Response.Error.Code](#anytype-Rpc-MembershipV2-GetProducts-Response-Error-Code)
    - [Rpc.MembershipV2.GetStatus.Response.Error.Code](#anytype-Rpc-MembershipV2-GetStatus-Response-Error-Code)
    - [Rpc.NameService.ResolveAnyId.Response.Error.Code](#anytype-Rpc-NameService-ResolveAnyId-Response-Error-Code)
    - [Rpc.NameService.ResolveName.Response.Error.Code](#anytype-Rpc-NameService-ResolveName-Response-Error-Code)
    - [Rpc.NameService.ResolveSpaceId.Response.Error.Code](#anytype-Rpc-NameService-ResolveSpaceId-Response-Error-Code)
    - [Rpc.NameService.UserAccount.Get.Response.Error.Code](#anytype-Rpc-NameService-UserAccount-Get-Response-Error-Code)
    - [Rpc.Navigation.Context](#anytype-Rpc-Navigation-Context)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response-Error-Code)
    - [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype-Rpc-Navigation-ListObjects-Response-Error-Code)
    - [Rpc.Notification.List.Response.Error.Code](#anytype-Rpc-Notification-List-Response-Error-Code)
    - [Rpc.Notification.Reply.Response.Error.Code](#anytype-Rpc-Notification-Reply-Response-Error-Code)
    - [Rpc.Notification.Test.Response.Error.Code](#anytype-Rpc-Notification-Test-Response-Error-Code)
    - [Rpc.Object.ApplyTemplate.Response.Error.Code](#anytype-Rpc-Object-ApplyTemplate-Response-Error-Code)
    - [Rpc.Object.BookmarkFetch.Response.Error.Code](#anytype-Rpc-Object-BookmarkFetch-Response-Error-Code)
    - [Rpc.Object.ChatAdd.Response.Error.Code](#anytype-Rpc-Object-ChatAdd-Response-Error-Code)
    - [Rpc.Object.Close.Response.Error.Code](#anytype-Rpc-Object-Close-Response-Error-Code)
    - [Rpc.Object.Create.Response.Error.Code](#anytype-Rpc-Object-Create-Response-Error-Code)
    - [Rpc.Object.CreateBookmark.Response.Error.Code](#anytype-Rpc-Object-CreateBookmark-Response-Error-Code)
    - [Rpc.Object.CreateFromUrl.Response.Error.Code](#anytype-Rpc-Object-CreateFromUrl-Response-Error-Code)
    - [Rpc.Object.CreateObjectType.Response.Error.Code](#anytype-Rpc-Object-CreateObjectType-Response-Error-Code)
    - [Rpc.Object.CreateRelation.Response.Error.Code](#anytype-Rpc-Object-CreateRelation-Response-Error-Code)
    - [Rpc.Object.CreateRelationOption.Response.Error.Code](#anytype-Rpc-Object-CreateRelationOption-Response-Error-Code)
    - [Rpc.Object.CreateSet.Response.Error.Code](#anytype-Rpc-Object-CreateSet-Response-Error-Code)
    - [Rpc.Object.CrossSpaceSearchSubscribe.Response.Error.Code](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response-Error-Code)
    - [Rpc.Object.CrossSpaceSearchUnsubscribe.Response.Error.Code](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response-Error-Code)
    - [Rpc.Object.DateByTimestamp.Response.Error.Code](#anytype-Rpc-Object-DateByTimestamp-Response-Error-Code)
    - [Rpc.Object.Duplicate.Response.Error.Code](#anytype-Rpc-Object-Duplicate-Response-Error-Code)
    - [Rpc.Object.Export.Response.Error.Code](#anytype-Rpc-Object-Export-Response-Error-Code)
    - [Rpc.Object.Graph.Edge.Type](#anytype-Rpc-Object-Graph-Edge-Type)
    - [Rpc.Object.Graph.Response.Error.Code](#anytype-Rpc-Object-Graph-Response-Error-Code)
    - [Rpc.Object.GroupsSubscribe.Response.Error.Code](#anytype-Rpc-Object-GroupsSubscribe-Response-Error-Code)
    - [Rpc.Object.Import.Notion.ValidateToken.Response.Error.Code](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response-Error-Code)
    - [Rpc.Object.Import.Request.CsvParams.Mode](#anytype-Rpc-Object-Import-Request-CsvParams-Mode)
    - [Rpc.Object.Import.Request.Mode](#anytype-Rpc-Object-Import-Request-Mode)
    - [Rpc.Object.Import.Request.PbParams.Type](#anytype-Rpc-Object-Import-Request-PbParams-Type)
    - [Rpc.Object.Import.Response.Error.Code](#anytype-Rpc-Object-Import-Response-Error-Code)
    - [Rpc.Object.ImportExperience.Response.Error.Code](#anytype-Rpc-Object-ImportExperience-Response-Error-Code)
    - [Rpc.Object.ImportList.ImportResponse.Type](#anytype-Rpc-Object-ImportList-ImportResponse-Type)
    - [Rpc.Object.ImportList.Response.Error.Code](#anytype-Rpc-Object-ImportList-Response-Error-Code)
    - [Rpc.Object.ImportUseCase.Request.UseCase](#anytype-Rpc-Object-ImportUseCase-Request-UseCase)
    - [Rpc.Object.ImportUseCase.Response.Error.Code](#anytype-Rpc-Object-ImportUseCase-Response-Error-Code)
    - [Rpc.Object.ListDelete.Response.Error.Code](#anytype-Rpc-Object-ListDelete-Response-Error-Code)
    - [Rpc.Object.ListDuplicate.Response.Error.Code](#anytype-Rpc-Object-ListDuplicate-Response-Error-Code)
    - [Rpc.Object.ListExport.Response.Error.Code](#anytype-Rpc-Object-ListExport-Response-Error-Code)
    - [Rpc.Object.ListModifyDetailValues.Response.Error.Code](#anytype-Rpc-Object-ListModifyDetailValues-Response-Error-Code)
    - [Rpc.Object.ListSetDetails.Response.Error.Code](#anytype-Rpc-Object-ListSetDetails-Response-Error-Code)
    - [Rpc.Object.ListSetIsArchived.Response.Error.Code](#anytype-Rpc-Object-ListSetIsArchived-Response-Error-Code)
    - [Rpc.Object.ListSetIsFavorite.Response.Error.Code](#anytype-Rpc-Object-ListSetIsFavorite-Response-Error-Code)
    - [Rpc.Object.ListSetObjectType.Response.Error.Code](#anytype-Rpc-Object-ListSetObjectType-Response-Error-Code)
    - [Rpc.Object.Open.Response.Error.Code](#anytype-Rpc-Object-Open-Response-Error-Code)
    - [Rpc.Object.OpenBreadcrumbs.Response.Error.Code](#anytype-Rpc-Object-OpenBreadcrumbs-Response-Error-Code)
    - [Rpc.Object.Redo.Response.Error.Code](#anytype-Rpc-Object-Redo-Response-Error-Code)
    - [Rpc.Object.Refresh.Response.Error.Code](#anytype-Rpc-Object-Refresh-Response-Error-Code)
    - [Rpc.Object.Search.Response.Error.Code](#anytype-Rpc-Object-Search-Response-Error-Code)
    - [Rpc.Object.SearchSubscribe.Response.Error.Code](#anytype-Rpc-Object-SearchSubscribe-Response-Error-Code)
    - [Rpc.Object.SearchUnsubscribe.Response.Error.Code](#anytype-Rpc-Object-SearchUnsubscribe-Response-Error-Code)
    - [Rpc.Object.SearchWithMeta.Response.Error.Code](#anytype-Rpc-Object-SearchWithMeta-Response-Error-Code)
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
    - [Rpc.ObjectType.ListConflictingRelations.Response.Error.Code](#anytype-Rpc-ObjectType-ListConflictingRelations-Response-Error-Code)
    - [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response.Error.Code](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response-Error-Code)
    - [Rpc.ObjectType.Recommended.RelationsSet.Response.Error.Code](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Response-Error-Code)
    - [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-Add-Response-Error-Code)
    - [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype-Rpc-ObjectType-Relation-Remove-Response-Error-Code)
    - [Rpc.ObjectType.ResolveLayoutConflicts.Response.Error.Code](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response-Error-Code)
    - [Rpc.ObjectType.SetOrder.Response.Error.Code](#anytype-Rpc-ObjectType-SetOrder-Response-Error-Code)
    - [Rpc.Process.Cancel.Response.Error.Code](#anytype-Rpc-Process-Cancel-Response-Error-Code)
    - [Rpc.Process.Subscribe.Response.Error.Code](#anytype-Rpc-Process-Subscribe-Response-Error-Code)
    - [Rpc.Process.Unsubscribe.Response.Error.Code](#anytype-Rpc-Process-Unsubscribe-Response-Error-Code)
    - [Rpc.Publishing.Create.Response.Error.Code](#anytype-Rpc-Publishing-Create-Response-Error-Code)
    - [Rpc.Publishing.GetStatus.Response.Error.Code](#anytype-Rpc-Publishing-GetStatus-Response-Error-Code)
    - [Rpc.Publishing.List.Response.Error.Code](#anytype-Rpc-Publishing-List-Response-Error-Code)
    - [Rpc.Publishing.PublishStatus](#anytype-Rpc-Publishing-PublishStatus)
    - [Rpc.Publishing.Remove.Response.Error.Code](#anytype-Rpc-Publishing-Remove-Response-Error-Code)
    - [Rpc.Publishing.ResolveUri.Response.Error.Code](#anytype-Rpc-Publishing-ResolveUri-Response-Error-Code)
    - [Rpc.PushNotification.Mode](#anytype-Rpc-PushNotification-Mode)
    - [Rpc.PushNotification.RegisterToken.Platform](#anytype-Rpc-PushNotification-RegisterToken-Platform)
    - [Rpc.PushNotification.RegisterToken.Response.Error.Code](#anytype-Rpc-PushNotification-RegisterToken-Response-Error-Code)
    - [Rpc.PushNotification.ResetIds.Response.Error.Code](#anytype-Rpc-PushNotification-ResetIds-Response-Error-Code)
    - [Rpc.PushNotification.SetForceModeIds.Response.Error.Code](#anytype-Rpc-PushNotification-SetForceModeIds-Response-Error-Code)
    - [Rpc.PushNotification.SetSpaceMode.Response.Error.Code](#anytype-Rpc-PushNotification-SetSpaceMode-Response-Error-Code)
    - [Rpc.Relation.ListRemoveOption.Response.Error.Code](#anytype-Rpc-Relation-ListRemoveOption-Response-Error-Code)
    - [Rpc.Relation.ListWithValue.Response.Error.Code](#anytype-Rpc-Relation-ListWithValue-Response-Error-Code)
    - [Rpc.Relation.Option.SetOrder.Response.Error.Code](#anytype-Rpc-Relation-Option-SetOrder-Response-Error-Code)
    - [Rpc.Relation.Options.Response.Error.Code](#anytype-Rpc-Relation-Options-Response-Error-Code)
    - [Rpc.Space.Delete.Response.Error.Code](#anytype-Rpc-Space-Delete-Response-Error-Code)
    - [Rpc.Space.InviteChange.Response.Error.Code](#anytype-Rpc-Space-InviteChange-Response-Error-Code)
    - [Rpc.Space.InviteGenerate.Response.Error.Code](#anytype-Rpc-Space-InviteGenerate-Response-Error-Code)
    - [Rpc.Space.InviteGetCurrent.Response.Error.Code](#anytype-Rpc-Space-InviteGetCurrent-Response-Error-Code)
    - [Rpc.Space.InviteGetGuest.Response.Error.Code](#anytype-Rpc-Space-InviteGetGuest-Response-Error-Code)
    - [Rpc.Space.InviteRevoke.Response.Error.Code](#anytype-Rpc-Space-InviteRevoke-Response-Error-Code)
    - [Rpc.Space.InviteView.Response.Error.Code](#anytype-Rpc-Space-InviteView-Response-Error-Code)
    - [Rpc.Space.Join.Response.Error.Code](#anytype-Rpc-Space-Join-Response-Error-Code)
    - [Rpc.Space.JoinCancel.Response.Error.Code](#anytype-Rpc-Space-JoinCancel-Response-Error-Code)
    - [Rpc.Space.LeaveApprove.Response.Error.Code](#anytype-Rpc-Space-LeaveApprove-Response-Error-Code)
    - [Rpc.Space.MakeShareable.Response.Error.Code](#anytype-Rpc-Space-MakeShareable-Response-Error-Code)
    - [Rpc.Space.ParticipantPermissionsChange.Response.Error.Code](#anytype-Rpc-Space-ParticipantPermissionsChange-Response-Error-Code)
    - [Rpc.Space.ParticipantRemove.Response.Error.Code](#anytype-Rpc-Space-ParticipantRemove-Response-Error-Code)
    - [Rpc.Space.RequestApprove.Response.Error.Code](#anytype-Rpc-Space-RequestApprove-Response-Error-Code)
    - [Rpc.Space.RequestDecline.Response.Error.Code](#anytype-Rpc-Space-RequestDecline-Response-Error-Code)
    - [Rpc.Space.SetOrder.Response.Error.Code](#anytype-Rpc-Space-SetOrder-Response-Error-Code)
    - [Rpc.Space.StopSharing.Response.Error.Code](#anytype-Rpc-Space-StopSharing-Response-Error-Code)
    - [Rpc.Space.UnsetOrder.Response.Error.Code](#anytype-Rpc-Space-UnsetOrder-Response-Error-Code)
    - [Rpc.Template.Clone.Response.Error.Code](#anytype-Rpc-Template-Clone-Response-Error-Code)
    - [Rpc.Template.CreateFromObject.Response.Error.Code](#anytype-Rpc-Template-CreateFromObject-Response-Error-Code)
    - [Rpc.Template.ExportAll.Response.Error.Code](#anytype-Rpc-Template-ExportAll-Response-Error-Code)
    - [Rpc.Unsplash.Download.Response.Error.Code](#anytype-Rpc-Unsplash-Download-Response-Error-Code)
    - [Rpc.Unsplash.Search.Response.Error.Code](#anytype-Rpc-Unsplash-Search-Response-Error-Code)
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
    - [Rpc.Workspace.Open.Response.Error.Code](#anytype-Rpc-Workspace-Open-Response-Error-Code)
    - [Rpc.Workspace.Select.Response.Error.Code](#anytype-Rpc-Workspace-Select-Response-Error-Code)
    - [Rpc.Workspace.SetInfo.Response.Error.Code](#anytype-Rpc-Workspace-SetInfo-Response-Error-Code)
  
- [pb/protos/events.proto](#pb_protos_events-proto)
    - [Event](#anytype-Event)
    - [Event.Account](#anytype-Event-Account)
    - [Event.Account.Config](#anytype-Event-Account-Config)
    - [Event.Account.Config.Update](#anytype-Event-Account-Config-Update)
    - [Event.Account.Details](#anytype-Event-Account-Details)
    - [Event.Account.LinkChallenge](#anytype-Event-Account-LinkChallenge)
    - [Event.Account.LinkChallenge.ClientInfo](#anytype-Event-Account-LinkChallenge-ClientInfo)
    - [Event.Account.LinkChallengeHide](#anytype-Event-Account-LinkChallengeHide)
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
    - [Event.Block.Set.File.TargetObjectId](#anytype-Event-Block-Set-File-TargetObjectId)
    - [Event.Block.Set.File.Type](#anytype-Event-Block-Set-File-Type)
    - [Event.Block.Set.File.Width](#anytype-Event-Block-Set-File-Width)
    - [Event.Block.Set.Latex](#anytype-Event-Block-Set-Latex)
    - [Event.Block.Set.Latex.Processor](#anytype-Event-Block-Set-Latex-Processor)
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
    - [Event.Block.Set.Widget.ViewId](#anytype-Event-Block-Set-Widget-ViewId)
    - [Event.Chat](#anytype-Event-Chat)
    - [Event.Chat.Add](#anytype-Event-Chat-Add)
    - [Event.Chat.Delete](#anytype-Event-Chat-Delete)
    - [Event.Chat.Update](#anytype-Event-Chat-Update)
    - [Event.Chat.UpdateMentionReadStatus](#anytype-Event-Chat-UpdateMentionReadStatus)
    - [Event.Chat.UpdateMessageReadStatus](#anytype-Event-Chat-UpdateMessageReadStatus)
    - [Event.Chat.UpdateMessageSyncStatus](#anytype-Event-Chat-UpdateMessageSyncStatus)
    - [Event.Chat.UpdateReactions](#anytype-Event-Chat-UpdateReactions)
    - [Event.Chat.UpdateState](#anytype-Event-Chat-UpdateState)
    - [Event.File](#anytype-Event-File)
    - [Event.File.LimitReached](#anytype-Event-File-LimitReached)
    - [Event.File.LimitUpdated](#anytype-Event-File-LimitUpdated)
    - [Event.File.LocalUsage](#anytype-Event-File-LocalUsage)
    - [Event.File.SpaceUsage](#anytype-Event-File-SpaceUsage)
    - [Event.Import](#anytype-Event-Import)
    - [Event.Import.Finish](#anytype-Event-Import-Finish)
    - [Event.Membership](#anytype-Event-Membership)
    - [Event.Membership.TiersUpdate](#anytype-Event-Membership-TiersUpdate)
    - [Event.Membership.Update](#anytype-Event-Membership-Update)
    - [Event.MembershipV2](#anytype-Event-MembershipV2)
    - [Event.MembershipV2.ProductsUpdate](#anytype-Event-MembershipV2-ProductsUpdate)
    - [Event.MembershipV2.Update](#anytype-Event-MembershipV2-Update)
    - [Event.Message](#anytype-Event-Message)
    - [Event.Notification](#anytype-Event-Notification)
    - [Event.Notification.Send](#anytype-Event-Notification-Send)
    - [Event.Notification.Update](#anytype-Event-Notification-Update)
    - [Event.Object](#anytype-Event-Object)
    - [Event.Object.Close](#anytype-Event-Object-Close)
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
    - [Event.P2PStatus](#anytype-Event-P2PStatus)
    - [Event.P2PStatus.Update](#anytype-Event-P2PStatus-Update)
    - [Event.Payload](#anytype-Event-Payload)
    - [Event.Payload.Broadcast](#anytype-Event-Payload-Broadcast)
    - [Event.Ping](#anytype-Event-Ping)
    - [Event.Process](#anytype-Event-Process)
    - [Event.Process.Done](#anytype-Event-Process-Done)
    - [Event.Process.New](#anytype-Event-Process-New)
    - [Event.Process.Update](#anytype-Event-Process-Update)
    - [Event.Space](#anytype-Event-Space)
    - [Event.Space.SyncStatus](#anytype-Event-Space-SyncStatus)
    - [Event.Space.SyncStatus.Update](#anytype-Event-Space-SyncStatus-Update)
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
    - [Model.Process.DropFiles](#anytype-Model-Process-DropFiles)
    - [Model.Process.Export](#anytype-Model-Process-Export)
    - [Model.Process.Import](#anytype-Model-Process-Import)
    - [Model.Process.Migration](#anytype-Model-Process-Migration)
    - [Model.Process.PreloadFile](#anytype-Model-Process-PreloadFile)
    - [Model.Process.Progress](#anytype-Model-Process-Progress)
    - [Model.Process.SaveFile](#anytype-Model-Process-SaveFile)
    - [ResponseEvent](#anytype-ResponseEvent)
  
    - [Event.Block.Dataview.SliceOperation](#anytype-Event-Block-Dataview-SliceOperation)
    - [Event.P2PStatus.Status](#anytype-Event-P2PStatus-Status)
    - [Event.Space.Network](#anytype-Event-Space-Network)
    - [Event.Space.Status](#anytype-Event-Space-Status)
    - [Event.Space.SyncError](#anytype-Event-Space-SyncError)
    - [Event.Status.Thread.SyncStatus](#anytype-Event-Status-Thread-SyncStatus)
    - [Model.Process.State](#anytype-Model-Process-State)
  
- [pb/protos/snapshot.proto](#pb_protos_snapshot-proto)
    - [Profile](#anytype-Profile)
    - [SnapshotWithType](#anytype-SnapshotWithType)
    - [WidgetBlock](#anytype-WidgetBlock)
  
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
    - [Account.Auth](#anytype-model-Account-Auth)
    - [Account.Auth.AppInfo](#anytype-model-Account-Auth-AppInfo)
    - [Account.Config](#anytype-model-Account-Config)
    - [Account.Info](#anytype-model-Account-Info)
    - [Account.Status](#anytype-model-Account-Status)
    - [Block](#anytype-model-Block)
    - [Block.Content](#anytype-model-Block-Content)
    - [Block.Content.Bookmark](#anytype-model-Block-Content-Bookmark)
    - [Block.Content.Chat](#anytype-model-Block-Content-Chat)
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
    - [ChatMessage](#anytype-model-ChatMessage)
    - [ChatMessage.Attachment](#anytype-model-ChatMessage-Attachment)
    - [ChatMessage.MessageContent](#anytype-model-ChatMessage-MessageContent)
    - [ChatMessage.Reactions](#anytype-model-ChatMessage-Reactions)
    - [ChatMessage.Reactions.IdentityList](#anytype-model-ChatMessage-Reactions-IdentityList)
    - [ChatMessage.Reactions.ReactionsEntry](#anytype-model-ChatMessage-Reactions-ReactionsEntry)
    - [ChatState](#anytype-model-ChatState)
    - [ChatState.UnreadState](#anytype-model-ChatState-UnreadState)
    - [Detail](#anytype-model-Detail)
    - [DeviceInfo](#anytype-model-DeviceInfo)
    - [Export](#anytype-model-Export)
    - [FileEncryptionKey](#anytype-model-FileEncryptionKey)
    - [FileInfo](#anytype-model-FileInfo)
    - [IdentityProfile](#anytype-model-IdentityProfile)
    - [IdentityProfileWithKey](#anytype-model-IdentityProfileWithKey)
    - [Import](#anytype-model-Import)
    - [InternalFlag](#anytype-model-InternalFlag)
    - [Invite](#anytype-model-Invite)
    - [InvitePayload](#anytype-model-InvitePayload)
    - [Layout](#anytype-model-Layout)
    - [LinkPreview](#anytype-model-LinkPreview)
    - [ManifestInfo](#anytype-model-ManifestInfo)
    - [Membership](#anytype-model-Membership)
    - [MembershipTierData](#anytype-model-MembershipTierData)
    - [MembershipV2](#anytype-model-MembershipV2)
    - [MembershipV2.Amount](#anytype-model-MembershipV2-Amount)
    - [MembershipV2.Cart](#anytype-model-MembershipV2-Cart)
    - [MembershipV2.CartProduct](#anytype-model-MembershipV2-CartProduct)
    - [MembershipV2.Data](#anytype-model-MembershipV2-Data)
    - [MembershipV2.Features](#anytype-model-MembershipV2-Features)
    - [MembershipV2.Invoice](#anytype-model-MembershipV2-Invoice)
    - [MembershipV2.Product](#anytype-model-MembershipV2-Product)
    - [MembershipV2.ProductStatus](#anytype-model-MembershipV2-ProductStatus)
    - [MembershipV2.PurchaseInfo](#anytype-model-MembershipV2-PurchaseInfo)
    - [MembershipV2.PurchasedProduct](#anytype-model-MembershipV2-PurchasedProduct)
    - [Metadata](#anytype-model-Metadata)
    - [Metadata.Payload](#anytype-model-Metadata-Payload)
    - [Metadata.Payload.IdentityPayload](#anytype-model-Metadata-Payload-IdentityPayload)
    - [Notification](#anytype-model-Notification)
    - [Notification.Export](#anytype-model-Notification-Export)
    - [Notification.GalleryImport](#anytype-model-Notification-GalleryImport)
    - [Notification.Import](#anytype-model-Notification-Import)
    - [Notification.ParticipantPermissionsChange](#anytype-model-Notification-ParticipantPermissionsChange)
    - [Notification.ParticipantRemove](#anytype-model-Notification-ParticipantRemove)
    - [Notification.ParticipantRequestApproved](#anytype-model-Notification-ParticipantRequestApproved)
    - [Notification.ParticipantRequestDecline](#anytype-model-Notification-ParticipantRequestDecline)
    - [Notification.RequestToJoin](#anytype-model-Notification-RequestToJoin)
    - [Notification.RequestToLeave](#anytype-model-Notification-RequestToLeave)
    - [Notification.Test](#anytype-model-Notification-Test)
    - [Object](#anytype-model-Object)
    - [Object.ChangePayload](#anytype-model-Object-ChangePayload)
    - [ObjectType](#anytype-model-ObjectType)
    - [ObjectView](#anytype-model-ObjectView)
    - [ObjectView.BlockParticipant](#anytype-model-ObjectView-BlockParticipant)
    - [ObjectView.DetailsSet](#anytype-model-ObjectView-DetailsSet)
    - [ObjectView.HistorySize](#anytype-model-ObjectView-HistorySize)
    - [ObjectView.RelationWithValuePerObject](#anytype-model-ObjectView-RelationWithValuePerObject)
    - [ParticipantPermissionChange](#anytype-model-ParticipantPermissionChange)
    - [Range](#anytype-model-Range)
    - [Relation](#anytype-model-Relation)
    - [Relation.Option](#anytype-model-Relation-Option)
    - [RelationLink](#anytype-model-RelationLink)
    - [RelationOptions](#anytype-model-RelationOptions)
    - [RelationWithValue](#anytype-model-RelationWithValue)
    - [Relations](#anytype-model-Relations)
    - [Restrictions](#anytype-model-Restrictions)
    - [Restrictions.DataviewRestrictions](#anytype-model-Restrictions-DataviewRestrictions)
    - [Search](#anytype-model-Search)
    - [Search.Meta](#anytype-model-Search-Meta)
    - [Search.Result](#anytype-model-Search-Result)
    - [SmartBlockSnapshotBase](#anytype-model-SmartBlockSnapshotBase)
    - [SpaceObjectHeader](#anytype-model-SpaceObjectHeader)
  
    - [Account.Auth.LocalApiScope](#anytype-model-Account-Auth-LocalApiScope)
    - [Account.StatusType](#anytype-model-Account-StatusType)
    - [Block.Align](#anytype-model-Block-Align)
    - [Block.Content.Bookmark.State](#anytype-model-Block-Content-Bookmark-State)
    - [Block.Content.Dataview.Filter.Condition](#anytype-model-Block-Content-Dataview-Filter-Condition)
    - [Block.Content.Dataview.Filter.Operator](#anytype-model-Block-Content-Dataview-Filter-Operator)
    - [Block.Content.Dataview.Filter.QuickOption](#anytype-model-Block-Content-Dataview-Filter-QuickOption)
    - [Block.Content.Dataview.Relation.DateFormat](#anytype-model-Block-Content-Dataview-Relation-DateFormat)
    - [Block.Content.Dataview.Relation.FormulaType](#anytype-model-Block-Content-Dataview-Relation-FormulaType)
    - [Block.Content.Dataview.Relation.TimeFormat](#anytype-model-Block-Content-Dataview-Relation-TimeFormat)
    - [Block.Content.Dataview.Sort.EmptyType](#anytype-model-Block-Content-Dataview-Sort-EmptyType)
    - [Block.Content.Dataview.Sort.Type](#anytype-model-Block-Content-Dataview-Sort-Type)
    - [Block.Content.Dataview.View.Size](#anytype-model-Block-Content-Dataview-View-Size)
    - [Block.Content.Dataview.View.Type](#anytype-model-Block-Content-Dataview-View-Type)
    - [Block.Content.Div.Style](#anytype-model-Block-Content-Div-Style)
    - [Block.Content.File.State](#anytype-model-Block-Content-File-State)
    - [Block.Content.File.Style](#anytype-model-Block-Content-File-Style)
    - [Block.Content.File.Type](#anytype-model-Block-Content-File-Type)
    - [Block.Content.Latex.Processor](#anytype-model-Block-Content-Latex-Processor)
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
    - [ChatMessage.Attachment.AttachmentType](#anytype-model-ChatMessage-Attachment-AttachmentType)
    - [DeviceNetworkType](#anytype-model-DeviceNetworkType)
    - [Export.Format](#anytype-model-Export-Format)
    - [FileIndexingStatus](#anytype-model-FileIndexingStatus)
    - [ImageKind](#anytype-model-ImageKind)
    - [Import.ErrorCode](#anytype-model-Import-ErrorCode)
    - [Import.Type](#anytype-model-Import-Type)
    - [InternalFlag.Value](#anytype-model-InternalFlag-Value)
    - [InviteType](#anytype-model-InviteType)
    - [LinkPreview.Type](#anytype-model-LinkPreview-Type)
    - [Membership.EmailVerificationStatus](#anytype-model-Membership-EmailVerificationStatus)
    - [Membership.PaymentMethod](#anytype-model-Membership-PaymentMethod)
    - [Membership.Status](#anytype-model-Membership-Status)
    - [MembershipTierData.PeriodType](#anytype-model-MembershipTierData-PeriodType)
    - [MembershipV2.PaymentProvider](#anytype-model-MembershipV2-PaymentProvider)
    - [MembershipV2.Period](#anytype-model-MembershipV2-Period)
    - [MembershipV2.ProductStatus.Status](#anytype-model-MembershipV2-ProductStatus-Status)
    - [NameserviceNameType](#anytype-model-NameserviceNameType)
    - [Notification.ActionType](#anytype-model-Notification-ActionType)
    - [Notification.Export.Code](#anytype-model-Notification-Export-Code)
    - [Notification.Status](#anytype-model-Notification-Status)
    - [ObjectOrigin](#anytype-model-ObjectOrigin)
    - [ObjectType.Layout](#anytype-model-ObjectType-Layout)
    - [ParticipantPermissions](#anytype-model-ParticipantPermissions)
    - [ParticipantStatus](#anytype-model-ParticipantStatus)
    - [Relation.DataSource](#anytype-model-Relation-DataSource)
    - [Relation.Scope](#anytype-model-Relation-Scope)
    - [RelationFormat](#anytype-model-RelationFormat)
    - [Restrictions.DataviewRestriction](#anytype-model-Restrictions-DataviewRestriction)
    - [Restrictions.ObjectRestriction](#anytype-model-Restrictions-ObjectRestriction)
    - [SmartBlockType](#anytype-model-SmartBlockType)
    - [SpaceAccessType](#anytype-model-SpaceAccessType)
    - [SpaceShareableStatus](#anytype-model-SpaceShareableStatus)
    - [SpaceStatus](#anytype-model-SpaceStatus)
    - [SpaceUxType](#anytype-model-SpaceUxType)
    - [SyncError](#anytype-model-SyncError)
    - [SyncStatus](#anytype-model-SyncStatus)
  
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
| AccountLocalLinkNewChallenge | [Rpc.Account.LocalLink.NewChallenge.Request](#anytype-Rpc-Account-LocalLink-NewChallenge-Request) | [Rpc.Account.LocalLink.NewChallenge.Response](#anytype-Rpc-Account-LocalLink-NewChallenge-Response) |  |
| AccountLocalLinkSolveChallenge | [Rpc.Account.LocalLink.SolveChallenge.Request](#anytype-Rpc-Account-LocalLink-SolveChallenge-Request) | [Rpc.Account.LocalLink.SolveChallenge.Response](#anytype-Rpc-Account-LocalLink-SolveChallenge-Response) |  |
| AccountLocalLinkCreateApp | [Rpc.Account.LocalLink.CreateApp.Request](#anytype-Rpc-Account-LocalLink-CreateApp-Request) | [Rpc.Account.LocalLink.CreateApp.Response](#anytype-Rpc-Account-LocalLink-CreateApp-Response) |  |
| AccountLocalLinkListApps | [Rpc.Account.LocalLink.ListApps.Request](#anytype-Rpc-Account-LocalLink-ListApps-Request) | [Rpc.Account.LocalLink.ListApps.Response](#anytype-Rpc-Account-LocalLink-ListApps-Response) |  |
| AccountLocalLinkRevokeApp | [Rpc.Account.LocalLink.RevokeApp.Request](#anytype-Rpc-Account-LocalLink-RevokeApp-Request) | [Rpc.Account.LocalLink.RevokeApp.Response](#anytype-Rpc-Account-LocalLink-RevokeApp-Response) |  |
| WalletCreateSession | [Rpc.Wallet.CreateSession.Request](#anytype-Rpc-Wallet-CreateSession-Request) | [Rpc.Wallet.CreateSession.Response](#anytype-Rpc-Wallet-CreateSession-Response) |  |
| WalletCloseSession | [Rpc.Wallet.CloseSession.Request](#anytype-Rpc-Wallet-CloseSession-Request) | [Rpc.Wallet.CloseSession.Response](#anytype-Rpc-Wallet-CloseSession-Response) |  |
| WorkspaceCreate | [Rpc.Workspace.Create.Request](#anytype-Rpc-Workspace-Create-Request) | [Rpc.Workspace.Create.Response](#anytype-Rpc-Workspace-Create-Response) | Workspace *** |
| WorkspaceOpen | [Rpc.Workspace.Open.Request](#anytype-Rpc-Workspace-Open-Request) | [Rpc.Workspace.Open.Response](#anytype-Rpc-Workspace-Open-Response) |  |
| WorkspaceObjectAdd | [Rpc.Workspace.Object.Add.Request](#anytype-Rpc-Workspace-Object-Add-Request) | [Rpc.Workspace.Object.Add.Response](#anytype-Rpc-Workspace-Object-Add-Response) |  |
| WorkspaceObjectListAdd | [Rpc.Workspace.Object.ListAdd.Request](#anytype-Rpc-Workspace-Object-ListAdd-Request) | [Rpc.Workspace.Object.ListAdd.Response](#anytype-Rpc-Workspace-Object-ListAdd-Response) |  |
| WorkspaceObjectListRemove | [Rpc.Workspace.Object.ListRemove.Request](#anytype-Rpc-Workspace-Object-ListRemove-Request) | [Rpc.Workspace.Object.ListRemove.Response](#anytype-Rpc-Workspace-Object-ListRemove-Response) |  |
| WorkspaceSelect | [Rpc.Workspace.Select.Request](#anytype-Rpc-Workspace-Select-Request) | [Rpc.Workspace.Select.Response](#anytype-Rpc-Workspace-Select-Response) |  |
| WorkspaceGetCurrent | [Rpc.Workspace.GetCurrent.Request](#anytype-Rpc-Workspace-GetCurrent-Request) | [Rpc.Workspace.GetCurrent.Response](#anytype-Rpc-Workspace-GetCurrent-Response) |  |
| WorkspaceGetAll | [Rpc.Workspace.GetAll.Request](#anytype-Rpc-Workspace-GetAll-Request) | [Rpc.Workspace.GetAll.Response](#anytype-Rpc-Workspace-GetAll-Response) |  |
| WorkspaceSetInfo | [Rpc.Workspace.SetInfo.Request](#anytype-Rpc-Workspace-SetInfo-Request) | [Rpc.Workspace.SetInfo.Response](#anytype-Rpc-Workspace-SetInfo-Response) |  |
| WorkspaceExport | [Rpc.Workspace.Export.Request](#anytype-Rpc-Workspace-Export-Request) | [Rpc.Workspace.Export.Response](#anytype-Rpc-Workspace-Export-Response) |  |
| AccountRecover | [Rpc.Account.Recover.Request](#anytype-Rpc-Account-Recover-Request) | [Rpc.Account.Recover.Response](#anytype-Rpc-Account-Recover-Response) | Account *** |
| AccountMigrate | [Rpc.Account.Migrate.Request](#anytype-Rpc-Account-Migrate-Request) | [Rpc.Account.Migrate.Response](#anytype-Rpc-Account-Migrate-Response) |  |
| AccountMigrateCancel | [Rpc.Account.MigrateCancel.Request](#anytype-Rpc-Account-MigrateCancel-Request) | [Rpc.Account.MigrateCancel.Response](#anytype-Rpc-Account-MigrateCancel-Response) |  |
| AccountCreate | [Rpc.Account.Create.Request](#anytype-Rpc-Account-Create-Request) | [Rpc.Account.Create.Response](#anytype-Rpc-Account-Create-Response) |  |
| AccountDelete | [Rpc.Account.Delete.Request](#anytype-Rpc-Account-Delete-Request) | [Rpc.Account.Delete.Response](#anytype-Rpc-Account-Delete-Response) |  |
| AccountRevertDeletion | [Rpc.Account.RevertDeletion.Request](#anytype-Rpc-Account-RevertDeletion-Request) | [Rpc.Account.RevertDeletion.Response](#anytype-Rpc-Account-RevertDeletion-Response) |  |
| AccountSelect | [Rpc.Account.Select.Request](#anytype-Rpc-Account-Select-Request) | [Rpc.Account.Select.Response](#anytype-Rpc-Account-Select-Response) |  |
| AccountEnableLocalNetworkSync | [Rpc.Account.EnableLocalNetworkSync.Request](#anytype-Rpc-Account-EnableLocalNetworkSync-Request) | [Rpc.Account.EnableLocalNetworkSync.Response](#anytype-Rpc-Account-EnableLocalNetworkSync-Response) |  |
| AccountChangeJsonApiAddr | [Rpc.Account.ChangeJsonApiAddr.Request](#anytype-Rpc-Account-ChangeJsonApiAddr-Request) | [Rpc.Account.ChangeJsonApiAddr.Response](#anytype-Rpc-Account-ChangeJsonApiAddr-Response) |  |
| AccountStop | [Rpc.Account.Stop.Request](#anytype-Rpc-Account-Stop-Request) | [Rpc.Account.Stop.Response](#anytype-Rpc-Account-Stop-Response) |  |
| AccountMove | [Rpc.Account.Move.Request](#anytype-Rpc-Account-Move-Request) | [Rpc.Account.Move.Response](#anytype-Rpc-Account-Move-Response) |  |
| AccountConfigUpdate | [Rpc.Account.ConfigUpdate.Request](#anytype-Rpc-Account-ConfigUpdate-Request) | [Rpc.Account.ConfigUpdate.Response](#anytype-Rpc-Account-ConfigUpdate-Response) |  |
| AccountRecoverFromLegacyExport | [Rpc.Account.RecoverFromLegacyExport.Request](#anytype-Rpc-Account-RecoverFromLegacyExport-Request) | [Rpc.Account.RecoverFromLegacyExport.Response](#anytype-Rpc-Account-RecoverFromLegacyExport-Response) |  |
| AccountChangeNetworkConfigAndRestart | [Rpc.Account.ChangeNetworkConfigAndRestart.Request](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Request) | [Rpc.Account.ChangeNetworkConfigAndRestart.Response](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response) |  |
| SpaceDelete | [Rpc.Space.Delete.Request](#anytype-Rpc-Space-Delete-Request) | [Rpc.Space.Delete.Response](#anytype-Rpc-Space-Delete-Response) | Space *** |
| SpaceInviteGenerate | [Rpc.Space.InviteGenerate.Request](#anytype-Rpc-Space-InviteGenerate-Request) | [Rpc.Space.InviteGenerate.Response](#anytype-Rpc-Space-InviteGenerate-Response) |  |
| SpaceInviteChange | [Rpc.Space.InviteChange.Request](#anytype-Rpc-Space-InviteChange-Request) | [Rpc.Space.InviteChange.Response](#anytype-Rpc-Space-InviteChange-Response) |  |
| SpaceInviteGetCurrent | [Rpc.Space.InviteGetCurrent.Request](#anytype-Rpc-Space-InviteGetCurrent-Request) | [Rpc.Space.InviteGetCurrent.Response](#anytype-Rpc-Space-InviteGetCurrent-Response) |  |
| SpaceInviteGetGuest | [Rpc.Space.InviteGetGuest.Request](#anytype-Rpc-Space-InviteGetGuest-Request) | [Rpc.Space.InviteGetGuest.Response](#anytype-Rpc-Space-InviteGetGuest-Response) |  |
| SpaceInviteRevoke | [Rpc.Space.InviteRevoke.Request](#anytype-Rpc-Space-InviteRevoke-Request) | [Rpc.Space.InviteRevoke.Response](#anytype-Rpc-Space-InviteRevoke-Response) |  |
| SpaceInviteView | [Rpc.Space.InviteView.Request](#anytype-Rpc-Space-InviteView-Request) | [Rpc.Space.InviteView.Response](#anytype-Rpc-Space-InviteView-Response) |  |
| SpaceJoin | [Rpc.Space.Join.Request](#anytype-Rpc-Space-Join-Request) | [Rpc.Space.Join.Response](#anytype-Rpc-Space-Join-Response) |  |
| SpaceJoinCancel | [Rpc.Space.JoinCancel.Request](#anytype-Rpc-Space-JoinCancel-Request) | [Rpc.Space.JoinCancel.Response](#anytype-Rpc-Space-JoinCancel-Response) |  |
| SpaceStopSharing | [Rpc.Space.StopSharing.Request](#anytype-Rpc-Space-StopSharing-Request) | [Rpc.Space.StopSharing.Response](#anytype-Rpc-Space-StopSharing-Response) |  |
| SpaceRequestApprove | [Rpc.Space.RequestApprove.Request](#anytype-Rpc-Space-RequestApprove-Request) | [Rpc.Space.RequestApprove.Response](#anytype-Rpc-Space-RequestApprove-Response) |  |
| SpaceRequestDecline | [Rpc.Space.RequestDecline.Request](#anytype-Rpc-Space-RequestDecline-Request) | [Rpc.Space.RequestDecline.Response](#anytype-Rpc-Space-RequestDecline-Response) |  |
| SpaceLeaveApprove | [Rpc.Space.LeaveApprove.Request](#anytype-Rpc-Space-LeaveApprove-Request) | [Rpc.Space.LeaveApprove.Response](#anytype-Rpc-Space-LeaveApprove-Response) |  |
| SpaceMakeShareable | [Rpc.Space.MakeShareable.Request](#anytype-Rpc-Space-MakeShareable-Request) | [Rpc.Space.MakeShareable.Response](#anytype-Rpc-Space-MakeShareable-Response) |  |
| SpaceParticipantRemove | [Rpc.Space.ParticipantRemove.Request](#anytype-Rpc-Space-ParticipantRemove-Request) | [Rpc.Space.ParticipantRemove.Response](#anytype-Rpc-Space-ParticipantRemove-Response) |  |
| SpaceParticipantPermissionsChange | [Rpc.Space.ParticipantPermissionsChange.Request](#anytype-Rpc-Space-ParticipantPermissionsChange-Request) | [Rpc.Space.ParticipantPermissionsChange.Response](#anytype-Rpc-Space-ParticipantPermissionsChange-Response) |  |
| SpaceSetOrder | [Rpc.Space.SetOrder.Request](#anytype-Rpc-Space-SetOrder-Request) | [Rpc.Space.SetOrder.Response](#anytype-Rpc-Space-SetOrder-Response) |  |
| SpaceUnsetOrder | [Rpc.Space.UnsetOrder.Request](#anytype-Rpc-Space-UnsetOrder-Request) | [Rpc.Space.UnsetOrder.Response](#anytype-Rpc-Space-UnsetOrder-Response) |  |
| PublishingCreate | [Rpc.Publishing.Create.Request](#anytype-Rpc-Publishing-Create-Request) | [Rpc.Publishing.Create.Response](#anytype-Rpc-Publishing-Create-Response) | Publishing *** |
| PublishingRemove | [Rpc.Publishing.Remove.Request](#anytype-Rpc-Publishing-Remove-Request) | [Rpc.Publishing.Remove.Response](#anytype-Rpc-Publishing-Remove-Response) |  |
| PublishingList | [Rpc.Publishing.List.Request](#anytype-Rpc-Publishing-List-Request) | [Rpc.Publishing.List.Response](#anytype-Rpc-Publishing-List-Response) |  |
| PublishingResolveUri | [Rpc.Publishing.ResolveUri.Request](#anytype-Rpc-Publishing-ResolveUri-Request) | [Rpc.Publishing.ResolveUri.Response](#anytype-Rpc-Publishing-ResolveUri-Response) |  |
| PublishingGetStatus | [Rpc.Publishing.GetStatus.Request](#anytype-Rpc-Publishing-GetStatus-Request) | [Rpc.Publishing.GetStatus.Response](#anytype-Rpc-Publishing-GetStatus-Response) |  |
| ObjectOpen | [Rpc.Object.Open.Request](#anytype-Rpc-Object-Open-Request) | [Rpc.Object.Open.Response](#anytype-Rpc-Object-Open-Response) | Object *** |
| ObjectRefresh | [Rpc.Object.Refresh.Request](#anytype-Rpc-Object-Refresh-Request) | [Rpc.Object.Refresh.Response](#anytype-Rpc-Object-Refresh-Response) |  |
| ObjectClose | [Rpc.Object.Close.Request](#anytype-Rpc-Object-Close-Request) | [Rpc.Object.Close.Response](#anytype-Rpc-Object-Close-Response) |  |
| ObjectShow | [Rpc.Object.Show.Request](#anytype-Rpc-Object-Show-Request) | [Rpc.Object.Show.Response](#anytype-Rpc-Object-Show-Response) |  |
| ObjectCreate | [Rpc.Object.Create.Request](#anytype-Rpc-Object-Create-Request) | [Rpc.Object.Create.Response](#anytype-Rpc-Object-Create-Response) | ObjectCreate just creates the new page, without adding the link to it from some other page |
| ObjectCreateBookmark | [Rpc.Object.CreateBookmark.Request](#anytype-Rpc-Object-CreateBookmark-Request) | [Rpc.Object.CreateBookmark.Response](#anytype-Rpc-Object-CreateBookmark-Response) |  |
| ObjectCreateFromUrl | [Rpc.Object.CreateFromUrl.Request](#anytype-Rpc-Object-CreateFromUrl-Request) | [Rpc.Object.CreateFromUrl.Response](#anytype-Rpc-Object-CreateFromUrl-Response) |  |
| ObjectCreateSet | [Rpc.Object.CreateSet.Request](#anytype-Rpc-Object-CreateSet-Request) | [Rpc.Object.CreateSet.Response](#anytype-Rpc-Object-CreateSet-Response) | ObjectCreateSet just creates the new set, without adding the link to it from some other page |
| ObjectGraph | [Rpc.Object.Graph.Request](#anytype-Rpc-Object-Graph-Request) | [Rpc.Object.Graph.Response](#anytype-Rpc-Object-Graph-Response) |  |
| ObjectSearch | [Rpc.Object.Search.Request](#anytype-Rpc-Object-Search-Request) | [Rpc.Object.Search.Response](#anytype-Rpc-Object-Search-Response) |  |
| ObjectSearchWithMeta | [Rpc.Object.SearchWithMeta.Request](#anytype-Rpc-Object-SearchWithMeta-Request) | [Rpc.Object.SearchWithMeta.Response](#anytype-Rpc-Object-SearchWithMeta-Response) |  |
| ObjectSearchSubscribe | [Rpc.Object.SearchSubscribe.Request](#anytype-Rpc-Object-SearchSubscribe-Request) | [Rpc.Object.SearchSubscribe.Response](#anytype-Rpc-Object-SearchSubscribe-Response) |  |
| ObjectCrossSpaceSearchSubscribe | [Rpc.Object.CrossSpaceSearchSubscribe.Request](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Request) | [Rpc.Object.CrossSpaceSearchSubscribe.Response](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response) |  |
| ObjectCrossSpaceSearchUnsubscribe | [Rpc.Object.CrossSpaceSearchUnsubscribe.Request](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Request) | [Rpc.Object.CrossSpaceSearchUnsubscribe.Response](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response) |  |
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
| ObjectListSetObjectType | [Rpc.Object.ListSetObjectType.Request](#anytype-Rpc-Object-ListSetObjectType-Request) | [Rpc.Object.ListSetObjectType.Response](#anytype-Rpc-Object-ListSetObjectType-Response) |  |
| ObjectListSetDetails | [Rpc.Object.ListSetDetails.Request](#anytype-Rpc-Object-ListSetDetails-Request) | [Rpc.Object.ListSetDetails.Response](#anytype-Rpc-Object-ListSetDetails-Response) |  |
| ObjectListModifyDetailValues | [Rpc.Object.ListModifyDetailValues.Request](#anytype-Rpc-Object-ListModifyDetailValues-Request) | [Rpc.Object.ListModifyDetailValues.Response](#anytype-Rpc-Object-ListModifyDetailValues-Response) |  |
| ObjectApplyTemplate | [Rpc.Object.ApplyTemplate.Request](#anytype-Rpc-Object-ApplyTemplate-Request) | [Rpc.Object.ApplyTemplate.Response](#anytype-Rpc-Object-ApplyTemplate-Response) |  |
| ObjectToSet | [Rpc.Object.ToSet.Request](#anytype-Rpc-Object-ToSet-Request) | [Rpc.Object.ToSet.Response](#anytype-Rpc-Object-ToSet-Response) | ObjectToSet creates new set from given object and removes object |
| ObjectToCollection | [Rpc.Object.ToCollection.Request](#anytype-Rpc-Object-ToCollection-Request) | [Rpc.Object.ToCollection.Response](#anytype-Rpc-Object-ToCollection-Response) |  |
| ObjectShareByLink | [Rpc.Object.ShareByLink.Request](#anytype-Rpc-Object-ShareByLink-Request) | [Rpc.Object.ShareByLink.Response](#anytype-Rpc-Object-ShareByLink-Response) |  |
| ObjectUndo | [Rpc.Object.Undo.Request](#anytype-Rpc-Object-Undo-Request) | [Rpc.Object.Undo.Response](#anytype-Rpc-Object-Undo-Response) |  |
| ObjectRedo | [Rpc.Object.Redo.Request](#anytype-Rpc-Object-Redo-Request) | [Rpc.Object.Redo.Response](#anytype-Rpc-Object-Redo-Response) |  |
| ObjectListExport | [Rpc.Object.ListExport.Request](#anytype-Rpc-Object-ListExport-Request) | [Rpc.Object.ListExport.Response](#anytype-Rpc-Object-ListExport-Response) |  |
| ObjectExport | [Rpc.Object.Export.Request](#anytype-Rpc-Object-Export-Request) | [Rpc.Object.Export.Response](#anytype-Rpc-Object-Export-Response) |  |
| ObjectBookmarkFetch | [Rpc.Object.BookmarkFetch.Request](#anytype-Rpc-Object-BookmarkFetch-Request) | [Rpc.Object.BookmarkFetch.Response](#anytype-Rpc-Object-BookmarkFetch-Response) |  |
| ObjectImport | [Rpc.Object.Import.Request](#anytype-Rpc-Object-Import-Request) | [Rpc.Object.Import.Response](#anytype-Rpc-Object-Import-Response) |  |
| ObjectImportList | [Rpc.Object.ImportList.Request](#anytype-Rpc-Object-ImportList-Request) | [Rpc.Object.ImportList.Response](#anytype-Rpc-Object-ImportList-Response) |  |
| ObjectImportNotionValidateToken | [Rpc.Object.Import.Notion.ValidateToken.Request](#anytype-Rpc-Object-Import-Notion-ValidateToken-Request) | [Rpc.Object.Import.Notion.ValidateToken.Response](#anytype-Rpc-Object-Import-Notion-ValidateToken-Response) |  |
| ObjectImportUseCase | [Rpc.Object.ImportUseCase.Request](#anytype-Rpc-Object-ImportUseCase-Request) | [Rpc.Object.ImportUseCase.Response](#anytype-Rpc-Object-ImportUseCase-Response) |  |
| ObjectImportExperience | [Rpc.Object.ImportExperience.Request](#anytype-Rpc-Object-ImportExperience-Request) | [Rpc.Object.ImportExperience.Response](#anytype-Rpc-Object-ImportExperience-Response) |  |
| ObjectDateByTimestamp | [Rpc.Object.DateByTimestamp.Request](#anytype-Rpc-Object-DateByTimestamp-Request) | [Rpc.Object.DateByTimestamp.Response](#anytype-Rpc-Object-DateByTimestamp-Response) |  |
| ObjectCollectionAdd | [Rpc.ObjectCollection.Add.Request](#anytype-Rpc-ObjectCollection-Add-Request) | [Rpc.ObjectCollection.Add.Response](#anytype-Rpc-ObjectCollection-Add-Response) | Collections *** |
| ObjectCollectionRemove | [Rpc.ObjectCollection.Remove.Request](#anytype-Rpc-ObjectCollection-Remove-Request) | [Rpc.ObjectCollection.Remove.Response](#anytype-Rpc-ObjectCollection-Remove-Response) |  |
| ObjectCollectionSort | [Rpc.ObjectCollection.Sort.Request](#anytype-Rpc-ObjectCollection-Sort-Request) | [Rpc.ObjectCollection.Sort.Response](#anytype-Rpc-ObjectCollection-Sort-Response) |  |
| ObjectCreateRelation | [Rpc.Object.CreateRelation.Request](#anytype-Rpc-Object-CreateRelation-Request) | [Rpc.Object.CreateRelation.Response](#anytype-Rpc-Object-CreateRelation-Response) | Relations *** |
| ObjectCreateRelationOption | [Rpc.Object.CreateRelationOption.Request](#anytype-Rpc-Object-CreateRelationOption-Request) | [Rpc.Object.CreateRelationOption.Response](#anytype-Rpc-Object-CreateRelationOption-Response) |  |
| RelationListRemoveOption | [Rpc.Relation.ListRemoveOption.Request](#anytype-Rpc-Relation-ListRemoveOption-Request) | [Rpc.Relation.ListRemoveOption.Response](#anytype-Rpc-Relation-ListRemoveOption-Response) |  |
| RelationOptions | [Rpc.Relation.Options.Request](#anytype-Rpc-Relation-Options-Request) | [Rpc.Relation.Options.Response](#anytype-Rpc-Relation-Options-Response) |  |
| RelationOptionSetOrder | [Rpc.Relation.Option.SetOrder.Request](#anytype-Rpc-Relation-Option-SetOrder-Request) | [Rpc.Relation.Option.SetOrder.Response](#anytype-Rpc-Relation-Option-SetOrder-Response) |  |
| RelationListWithValue | [Rpc.Relation.ListWithValue.Request](#anytype-Rpc-Relation-ListWithValue-Request) | [Rpc.Relation.ListWithValue.Response](#anytype-Rpc-Relation-ListWithValue-Response) |  |
| ObjectRelationAdd | [Rpc.ObjectRelation.Add.Request](#anytype-Rpc-ObjectRelation-Add-Request) | [Rpc.ObjectRelation.Add.Response](#anytype-Rpc-ObjectRelation-Add-Response) | Object Relations *** |
| ObjectRelationDelete | [Rpc.ObjectRelation.Delete.Request](#anytype-Rpc-ObjectRelation-Delete-Request) | [Rpc.ObjectRelation.Delete.Response](#anytype-Rpc-ObjectRelation-Delete-Response) |  |
| ObjectRelationAddFeatured | [Rpc.ObjectRelation.AddFeatured.Request](#anytype-Rpc-ObjectRelation-AddFeatured-Request) | [Rpc.ObjectRelation.AddFeatured.Response](#anytype-Rpc-ObjectRelation-AddFeatured-Response) |  |
| ObjectRelationRemoveFeatured | [Rpc.ObjectRelation.RemoveFeatured.Request](#anytype-Rpc-ObjectRelation-RemoveFeatured-Request) | [Rpc.ObjectRelation.RemoveFeatured.Response](#anytype-Rpc-ObjectRelation-RemoveFeatured-Response) |  |
| ObjectRelationListAvailable | [Rpc.ObjectRelation.ListAvailable.Request](#anytype-Rpc-ObjectRelation-ListAvailable-Request) | [Rpc.ObjectRelation.ListAvailable.Response](#anytype-Rpc-ObjectRelation-ListAvailable-Response) |  |
| ObjectCreateObjectType | [Rpc.Object.CreateObjectType.Request](#anytype-Rpc-Object-CreateObjectType-Request) | [Rpc.Object.CreateObjectType.Response](#anytype-Rpc-Object-CreateObjectType-Response) | ObjectType commands *** |
| ObjectTypeRelationAdd | [Rpc.ObjectType.Relation.Add.Request](#anytype-Rpc-ObjectType-Relation-Add-Request) | [Rpc.ObjectType.Relation.Add.Response](#anytype-Rpc-ObjectType-Relation-Add-Response) |  |
| ObjectTypeRelationRemove | [Rpc.ObjectType.Relation.Remove.Request](#anytype-Rpc-ObjectType-Relation-Remove-Request) | [Rpc.ObjectType.Relation.Remove.Response](#anytype-Rpc-ObjectType-Relation-Remove-Response) |  |
| ObjectTypeRecommendedRelationsSet | [Rpc.ObjectType.Recommended.RelationsSet.Request](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Request) | [Rpc.ObjectType.Recommended.RelationsSet.Response](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Response) |  |
| ObjectTypeRecommendedFeaturedRelationsSet | [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Request](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Request) | [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response) |  |
| ObjectTypeListConflictingRelations | [Rpc.ObjectType.ListConflictingRelations.Request](#anytype-Rpc-ObjectType-ListConflictingRelations-Request) | [Rpc.ObjectType.ListConflictingRelations.Response](#anytype-Rpc-ObjectType-ListConflictingRelations-Response) |  |
| ObjectTypeResolveLayoutConflicts | [Rpc.ObjectType.ResolveLayoutConflicts.Request](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Request) | [Rpc.ObjectType.ResolveLayoutConflicts.Response](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response) |  |
| ObjectTypeSetOrder | [Rpc.ObjectType.SetOrder.Request](#anytype-Rpc-ObjectType-SetOrder-Request) | [Rpc.ObjectType.SetOrder.Response](#anytype-Rpc-ObjectType-SetOrder-Response) |  |
| HistoryShowVersion | [Rpc.History.ShowVersion.Request](#anytype-Rpc-History-ShowVersion-Request) | [Rpc.History.ShowVersion.Response](#anytype-Rpc-History-ShowVersion-Response) |  |
| HistoryGetVersions | [Rpc.History.GetVersions.Request](#anytype-Rpc-History-GetVersions-Request) | [Rpc.History.GetVersions.Response](#anytype-Rpc-History-GetVersions-Response) |  |
| HistorySetVersion | [Rpc.History.SetVersion.Request](#anytype-Rpc-History-SetVersion-Request) | [Rpc.History.SetVersion.Response](#anytype-Rpc-History-SetVersion-Response) |  |
| HistoryDiffVersions | [Rpc.History.DiffVersions.Request](#anytype-Rpc-History-DiffVersions-Request) | [Rpc.History.DiffVersions.Response](#anytype-Rpc-History-DiffVersions-Response) |  |
| FileSpaceOffload | [Rpc.File.SpaceOffload.Request](#anytype-Rpc-File-SpaceOffload-Request) | [Rpc.File.SpaceOffload.Response](#anytype-Rpc-File-SpaceOffload-Response) | Files *** |
| FileReconcile | [Rpc.File.Reconcile.Request](#anytype-Rpc-File-Reconcile-Request) | [Rpc.File.Reconcile.Response](#anytype-Rpc-File-Reconcile-Response) |  |
| FileListOffload | [Rpc.File.ListOffload.Request](#anytype-Rpc-File-ListOffload-Request) | [Rpc.File.ListOffload.Response](#anytype-Rpc-File-ListOffload-Response) |  |
| FileUpload | [Rpc.File.Upload.Request](#anytype-Rpc-File-Upload-Request) | [Rpc.File.Upload.Response](#anytype-Rpc-File-Upload-Response) |  |
| FileDownload | [Rpc.File.Download.Request](#anytype-Rpc-File-Download-Request) | [Rpc.File.Download.Response](#anytype-Rpc-File-Download-Response) |  |
| FileDiscardPreload | [Rpc.File.DiscardPreload.Request](#anytype-Rpc-File-DiscardPreload-Request) | [Rpc.File.DiscardPreload.Response](#anytype-Rpc-File-DiscardPreload-Response) |  |
| FileDrop | [Rpc.File.Drop.Request](#anytype-Rpc-File-Drop-Request) | [Rpc.File.Drop.Response](#anytype-Rpc-File-Drop-Response) |  |
| FileSpaceUsage | [Rpc.File.SpaceUsage.Request](#anytype-Rpc-File-SpaceUsage-Request) | [Rpc.File.SpaceUsage.Response](#anytype-Rpc-File-SpaceUsage-Response) |  |
| FileNodeUsage | [Rpc.File.NodeUsage.Request](#anytype-Rpc-File-NodeUsage-Request) | [Rpc.File.NodeUsage.Response](#anytype-Rpc-File-NodeUsage-Response) |  |
| FileSetAutoDownload | [Rpc.File.SetAutoDownload.Request](#anytype-Rpc-File-SetAutoDownload-Request) | [Rpc.File.SetAutoDownload.Response](#anytype-Rpc-File-SetAutoDownload-Response) |  |
| FileCacheDownload | [Rpc.File.CacheDownload.Request](#anytype-Rpc-File-CacheDownload-Request) | [Rpc.File.CacheDownload.Response](#anytype-Rpc-File-CacheDownload-Response) |  |
| FileCacheCancelDownload | [Rpc.File.CacheCancelDownload.Request](#anytype-Rpc-File-CacheCancelDownload-Request) | [Rpc.File.CacheCancelDownload.Response](#anytype-Rpc-File-CacheCancelDownload-Response) |  |
| NavigationListObjects | [Rpc.Navigation.ListObjects.Request](#anytype-Rpc-Navigation-ListObjects-Request) | [Rpc.Navigation.ListObjects.Response](#anytype-Rpc-Navigation-ListObjects-Response) |  |
| NavigationGetObjectInfoWithLinks | [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Request) | [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype-Rpc-Navigation-GetObjectInfoWithLinks-Response) |  |
| TemplateCreateFromObject | [Rpc.Template.CreateFromObject.Request](#anytype-Rpc-Template-CreateFromObject-Request) | [Rpc.Template.CreateFromObject.Response](#anytype-Rpc-Template-CreateFromObject-Response) |  |
| TemplateClone | [Rpc.Template.Clone.Request](#anytype-Rpc-Template-Clone-Request) | [Rpc.Template.Clone.Response](#anytype-Rpc-Template-Clone-Response) |  |
| TemplateExportAll | [Rpc.Template.ExportAll.Request](#anytype-Rpc-Template-ExportAll-Request) | [Rpc.Template.ExportAll.Response](#anytype-Rpc-Template-ExportAll-Response) |  |
| LinkPreview | [Rpc.LinkPreview.Request](#anytype-Rpc-LinkPreview-Request) | [Rpc.LinkPreview.Response](#anytype-Rpc-LinkPreview-Response) |  |
| UnsplashSearch | [Rpc.Unsplash.Search.Request](#anytype-Rpc-Unsplash-Search-Request) | [Rpc.Unsplash.Search.Response](#anytype-Rpc-Unsplash-Search-Response) |  |
| UnsplashDownload | [Rpc.Unsplash.Download.Request](#anytype-Rpc-Unsplash-Download-Request) | [Rpc.Unsplash.Download.Response](#anytype-Rpc-Unsplash-Download-Response) | UnsplashDownload downloads picture from unsplash by ID, put it to the IPFS and returns the hash. The artist info is available in the object details |
| GalleryDownloadManifest | [Rpc.Gallery.DownloadManifest.Request](#anytype-Rpc-Gallery-DownloadManifest-Request) | [Rpc.Gallery.DownloadManifest.Response](#anytype-Rpc-Gallery-DownloadManifest-Response) |  |
| GalleryDownloadIndex | [Rpc.Gallery.DownloadIndex.Request](#anytype-Rpc-Gallery-DownloadIndex-Request) | [Rpc.Gallery.DownloadIndex.Response](#anytype-Rpc-Gallery-DownloadIndex-Response) |  |
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
| BlockSetCarriage | [Rpc.Block.SetCarriage.Request](#anytype-Rpc-Block-SetCarriage-Request) | [Rpc.Block.SetCarriage.Response](#anytype-Rpc-Block-SetCarriage-Response) |  |
| BlockPreview | [Rpc.Block.Preview.Request](#anytype-Rpc-Block-Preview-Request) | [Rpc.Block.Preview.Response](#anytype-Rpc-Block-Preview-Response) |  |
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
| BlockFileSetTargetObjectId | [Rpc.BlockFile.SetTargetObjectId.Request](#anytype-Rpc-BlockFile-SetTargetObjectId-Request) | [Rpc.BlockFile.SetTargetObjectId.Response](#anytype-Rpc-BlockFile-SetTargetObjectId-Response) |  |
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
| BlockDataviewRelationSet | [Rpc.BlockDataview.Relation.Set.Request](#anytype-Rpc-BlockDataview-Relation-Set-Request) | [Rpc.BlockDataview.Relation.Set.Response](#anytype-Rpc-BlockDataview-Relation-Set-Response) |  |
| BlockDataviewRelationAdd | [Rpc.BlockDataview.Relation.Add.Request](#anytype-Rpc-BlockDataview-Relation-Add-Request) | [Rpc.BlockDataview.Relation.Add.Response](#anytype-Rpc-BlockDataview-Relation-Add-Response) |  |
| BlockDataviewRelationDelete | [Rpc.BlockDataview.Relation.Delete.Request](#anytype-Rpc-BlockDataview-Relation-Delete-Request) | [Rpc.BlockDataview.Relation.Delete.Response](#anytype-Rpc-BlockDataview-Relation-Delete-Response) |  |
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
| BlockDataviewSortSort | [Rpc.BlockDataview.Sort.SSort.Request](#anytype-Rpc-BlockDataview-Sort-SSort-Request) | [Rpc.BlockDataview.Sort.SSort.Response](#anytype-Rpc-BlockDataview-Sort-SSort-Response) |  |
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
| BlockWidgetSetViewId | [Rpc.BlockWidget.SetViewId.Request](#anytype-Rpc-BlockWidget-SetViewId-Request) | [Rpc.BlockWidget.SetViewId.Response](#anytype-Rpc-BlockWidget-SetViewId-Response) |  |
| BlockLinkCreateWithObject | [Rpc.BlockLink.CreateWithObject.Request](#anytype-Rpc-BlockLink-CreateWithObject-Request) | [Rpc.BlockLink.CreateWithObject.Response](#anytype-Rpc-BlockLink-CreateWithObject-Response) | Other specific block commands *** |
| BlockLinkListSetAppearance | [Rpc.BlockLink.ListSetAppearance.Request](#anytype-Rpc-BlockLink-ListSetAppearance-Request) | [Rpc.BlockLink.ListSetAppearance.Response](#anytype-Rpc-BlockLink-ListSetAppearance-Response) |  |
| BlockBookmarkFetch | [Rpc.BlockBookmark.Fetch.Request](#anytype-Rpc-BlockBookmark-Fetch-Request) | [Rpc.BlockBookmark.Fetch.Response](#anytype-Rpc-BlockBookmark-Fetch-Response) |  |
| BlockBookmarkCreateAndFetch | [Rpc.BlockBookmark.CreateAndFetch.Request](#anytype-Rpc-BlockBookmark-CreateAndFetch-Request) | [Rpc.BlockBookmark.CreateAndFetch.Response](#anytype-Rpc-BlockBookmark-CreateAndFetch-Response) |  |
| BlockRelationSetKey | [Rpc.BlockRelation.SetKey.Request](#anytype-Rpc-BlockRelation-SetKey-Request) | [Rpc.BlockRelation.SetKey.Response](#anytype-Rpc-BlockRelation-SetKey-Response) |  |
| BlockRelationAdd | [Rpc.BlockRelation.Add.Request](#anytype-Rpc-BlockRelation-Add-Request) | [Rpc.BlockRelation.Add.Response](#anytype-Rpc-BlockRelation-Add-Response) |  |
| BlockDivListSetStyle | [Rpc.BlockDiv.ListSetStyle.Request](#anytype-Rpc-BlockDiv-ListSetStyle-Request) | [Rpc.BlockDiv.ListSetStyle.Response](#anytype-Rpc-BlockDiv-ListSetStyle-Response) |  |
| BlockLatexSetText | [Rpc.BlockLatex.SetText.Request](#anytype-Rpc-BlockLatex-SetText-Request) | [Rpc.BlockLatex.SetText.Response](#anytype-Rpc-BlockLatex-SetText-Response) |  |
| ProcessCancel | [Rpc.Process.Cancel.Request](#anytype-Rpc-Process-Cancel-Request) | [Rpc.Process.Cancel.Response](#anytype-Rpc-Process-Cancel-Response) |  |
| ProcessSubscribe | [Rpc.Process.Subscribe.Request](#anytype-Rpc-Process-Subscribe-Request) | [Rpc.Process.Subscribe.Response](#anytype-Rpc-Process-Subscribe-Response) |  |
| ProcessUnsubscribe | [Rpc.Process.Unsubscribe.Request](#anytype-Rpc-Process-Unsubscribe-Request) | [Rpc.Process.Unsubscribe.Response](#anytype-Rpc-Process-Unsubscribe-Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype-Rpc-Log-Send-Request) | [Rpc.Log.Send.Response](#anytype-Rpc-Log-Send-Response) |  |
| DebugStat | [Rpc.Debug.Stat.Request](#anytype-Rpc-Debug-Stat-Request) | [Rpc.Debug.Stat.Response](#anytype-Rpc-Debug-Stat-Response) |  |
| DebugTree | [Rpc.Debug.Tree.Request](#anytype-Rpc-Debug-Tree-Request) | [Rpc.Debug.Tree.Response](#anytype-Rpc-Debug-Tree-Response) |  |
| DebugTreeHeads | [Rpc.Debug.TreeHeads.Request](#anytype-Rpc-Debug-TreeHeads-Request) | [Rpc.Debug.TreeHeads.Response](#anytype-Rpc-Debug-TreeHeads-Response) |  |
| DebugSpaceSummary | [Rpc.Debug.SpaceSummary.Request](#anytype-Rpc-Debug-SpaceSummary-Request) | [Rpc.Debug.SpaceSummary.Response](#anytype-Rpc-Debug-SpaceSummary-Response) |  |
| DebugStackGoroutines | [Rpc.Debug.StackGoroutines.Request](#anytype-Rpc-Debug-StackGoroutines-Request) | [Rpc.Debug.StackGoroutines.Response](#anytype-Rpc-Debug-StackGoroutines-Response) |  |
| DebugExportLocalstore | [Rpc.Debug.ExportLocalstore.Request](#anytype-Rpc-Debug-ExportLocalstore-Request) | [Rpc.Debug.ExportLocalstore.Response](#anytype-Rpc-Debug-ExportLocalstore-Response) |  |
| DebugPing | [Rpc.Debug.Ping.Request](#anytype-Rpc-Debug-Ping-Request) | [Rpc.Debug.Ping.Response](#anytype-Rpc-Debug-Ping-Response) |  |
| DebugSubscriptions | [Rpc.Debug.Subscriptions.Request](#anytype-Rpc-Debug-Subscriptions-Request) | [Rpc.Debug.Subscriptions.Response](#anytype-Rpc-Debug-Subscriptions-Response) |  |
| DebugOpenedObjects | [Rpc.Debug.OpenedObjects.Request](#anytype-Rpc-Debug-OpenedObjects-Request) | [Rpc.Debug.OpenedObjects.Response](#anytype-Rpc-Debug-OpenedObjects-Response) |  |
| DebugRunProfiler | [Rpc.Debug.RunProfiler.Request](#anytype-Rpc-Debug-RunProfiler-Request) | [Rpc.Debug.RunProfiler.Response](#anytype-Rpc-Debug-RunProfiler-Response) |  |
| DebugAccountSelectTrace | [Rpc.Debug.AccountSelectTrace.Request](#anytype-Rpc-Debug-AccountSelectTrace-Request) | [Rpc.Debug.AccountSelectTrace.Response](#anytype-Rpc-Debug-AccountSelectTrace-Response) |  |
| DebugAnystoreObjectChanges | [Rpc.Debug.AnystoreObjectChanges.Request](#anytype-Rpc-Debug-AnystoreObjectChanges-Request) | [Rpc.Debug.AnystoreObjectChanges.Response](#anytype-Rpc-Debug-AnystoreObjectChanges-Response) |  |
| DebugNetCheck | [Rpc.Debug.NetCheck.Request](#anytype-Rpc-Debug-NetCheck-Request) | [Rpc.Debug.NetCheck.Response](#anytype-Rpc-Debug-NetCheck-Response) |  |
| DebugExportLog | [Rpc.Debug.ExportLog.Request](#anytype-Rpc-Debug-ExportLog-Request) | [Rpc.Debug.ExportLog.Response](#anytype-Rpc-Debug-ExportLog-Response) |  |
| InitialSetParameters | [Rpc.Initial.SetParameters.Request](#anytype-Rpc-Initial-SetParameters-Request) | [Rpc.Initial.SetParameters.Response](#anytype-Rpc-Initial-SetParameters-Response) |  |
| ListenSessionEvents | [StreamRequest](#anytype-StreamRequest) | [Event](#anytype-Event) stream | used only for lib-server via grpc |
| NotificationList | [Rpc.Notification.List.Request](#anytype-Rpc-Notification-List-Request) | [Rpc.Notification.List.Response](#anytype-Rpc-Notification-List-Response) |  |
| NotificationReply | [Rpc.Notification.Reply.Request](#anytype-Rpc-Notification-Reply-Request) | [Rpc.Notification.Reply.Response](#anytype-Rpc-Notification-Reply-Response) |  |
| NotificationTest | [Rpc.Notification.Test.Request](#anytype-Rpc-Notification-Test-Request) | [Rpc.Notification.Test.Response](#anytype-Rpc-Notification-Test-Response) |  |
| MembershipGetStatus | [Rpc.Membership.GetStatus.Request](#anytype-Rpc-Membership-GetStatus-Request) | [Rpc.Membership.GetStatus.Response](#anytype-Rpc-Membership-GetStatus-Response) | Membership *** Get current subscription status (tier, expiration date, etc.) WARNING: can be cached by Anytype Heart |
| MembershipIsNameValid | [Rpc.Membership.IsNameValid.Request](#anytype-Rpc-Membership-IsNameValid-Request) | [Rpc.Membership.IsNameValid.Response](#anytype-Rpc-Membership-IsNameValid-Response) | Check if the requested name is valid and vacant for the requested tier |
| MembershipRegisterPaymentRequest | [Rpc.Membership.RegisterPaymentRequest.Request](#anytype-Rpc-Membership-RegisterPaymentRequest-Request) | [Rpc.Membership.RegisterPaymentRequest.Response](#anytype-Rpc-Membership-RegisterPaymentRequest-Response) | Buy a subscription, will return a payment URL. The user should be redirected to this URL to complete the payment. |
| MembershipGetPortalLinkUrl | [Rpc.Membership.GetPortalLinkUrl.Request](#anytype-Rpc-Membership-GetPortalLinkUrl-Request) | [Rpc.Membership.GetPortalLinkUrl.Response](#anytype-Rpc-Membership-GetPortalLinkUrl-Response) | Get a link to the user&#39;s subscription management portal. The user should be redirected to this URL to manage their subscription: a) change his billing details b) see payment info, invoices, etc c) cancel the subscription |
| MembershipGetVerificationEmailStatus | [Rpc.Membership.GetVerificationEmailStatus.Request](#anytype-Rpc-Membership-GetVerificationEmailStatus-Request) | [Rpc.Membership.GetVerificationEmailStatus.Response](#anytype-Rpc-Membership-GetVerificationEmailStatus-Response) | Check the current status of the verification email |
| MembershipGetVerificationEmail | [Rpc.Membership.GetVerificationEmail.Request](#anytype-Rpc-Membership-GetVerificationEmail-Request) | [Rpc.Membership.GetVerificationEmail.Response](#anytype-Rpc-Membership-GetVerificationEmail-Response) | Send a verification code to the user&#39;s email. The user should enter this code to verify his email. |
| MembershipVerifyEmailCode | [Rpc.Membership.VerifyEmailCode.Request](#anytype-Rpc-Membership-VerifyEmailCode-Request) | [Rpc.Membership.VerifyEmailCode.Response](#anytype-Rpc-Membership-VerifyEmailCode-Response) | Verify the user&#39;s email with the code received in the previous step (MembershipGetVerificationEmail) |
| MembershipFinalize | [Rpc.Membership.Finalize.Request](#anytype-Rpc-Membership-Finalize-Request) | [Rpc.Membership.Finalize.Response](#anytype-Rpc-Membership-Finalize-Response) | If your subscription is in PendingRequiresFinalization: please call MembershipFinalize to finish the process |
| MembershipGetTiers | [Rpc.Membership.GetTiers.Request](#anytype-Rpc-Membership-GetTiers-Request) | [Rpc.Membership.GetTiers.Response](#anytype-Rpc-Membership-GetTiers-Response) |  |
| MembershipVerifyAppStoreReceipt | [Rpc.Membership.VerifyAppStoreReceipt.Request](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Request) | [Rpc.Membership.VerifyAppStoreReceipt.Response](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Response) |  |
| MembershipCodeGetInfo | [Rpc.Membership.CodeGetInfo.Request](#anytype-Rpc-Membership-CodeGetInfo-Request) | [Rpc.Membership.CodeGetInfo.Response](#anytype-Rpc-Membership-CodeGetInfo-Response) |  |
| MembershipCodeRedeem | [Rpc.Membership.CodeRedeem.Request](#anytype-Rpc-Membership-CodeRedeem-Request) | [Rpc.Membership.CodeRedeem.Response](#anytype-Rpc-Membership-CodeRedeem-Response) |  |
| MembershipV2GetProducts | [Rpc.MembershipV2.GetProducts.Request](#anytype-Rpc-MembershipV2-GetProducts-Request) | [Rpc.MembershipV2.GetProducts.Response](#anytype-Rpc-MembershipV2-GetProducts-Response) | enumerate all available for purchase products |
| MembershipV2GetStatus | [Rpc.MembershipV2.GetStatus.Request](#anytype-Rpc-MembershipV2-GetStatus-Request) | [Rpc.MembershipV2.GetStatus.Response](#anytype-Rpc-MembershipV2-GetStatus-Response) |  |
| MembershipV2GetPortalLink | [Rpc.MembershipV2.GetPortalLink.Request](#anytype-Rpc-MembershipV2-GetPortalLink-Request) | [Rpc.MembershipV2.GetPortalLink.Response](#anytype-Rpc-MembershipV2-GetPortalLink-Response) |  |
| MembershipV2AnyNameIsValid | [Rpc.MembershipV2.AnyNameIsValid.Request](#anytype-Rpc-MembershipV2-AnyNameIsValid-Request) | [Rpc.MembershipV2.AnyNameIsValid.Response](#anytype-Rpc-MembershipV2-AnyNameIsValid-Response) |  |
| MembershipV2AnyNameAllocate | [Rpc.MembershipV2.AnyNameAllocate.Request](#anytype-Rpc-MembershipV2-AnyNameAllocate-Request) | [Rpc.MembershipV2.AnyNameAllocate.Response](#anytype-Rpc-MembershipV2-AnyNameAllocate-Response) |  |
| MembershipV2CartGet | [Rpc.MembershipV2.CartGet.Request](#anytype-Rpc-MembershipV2-CartGet-Request) | [Rpc.MembershipV2.CartGet.Response](#anytype-Rpc-MembershipV2-CartGet-Response) |  |
| MembershipV2CartUpdate | [Rpc.MembershipV2.CartUpdate.Request](#anytype-Rpc-MembershipV2-CartUpdate-Request) | [Rpc.MembershipV2.CartUpdate.Response](#anytype-Rpc-MembershipV2-CartUpdate-Response) |  |
| NameServiceUserAccountGet | [Rpc.NameService.UserAccount.Get.Request](#anytype-Rpc-NameService-UserAccount-Get-Request) | [Rpc.NameService.UserAccount.Get.Response](#anytype-Rpc-NameService-UserAccount-Get-Response) | Name Service: *** hello.any -&gt; data |
| NameServiceResolveName | [Rpc.NameService.ResolveName.Request](#anytype-Rpc-NameService-ResolveName-Request) | [Rpc.NameService.ResolveName.Response](#anytype-Rpc-NameService-ResolveName-Response) |  |
| NameServiceResolveAnyId | [Rpc.NameService.ResolveAnyId.Request](#anytype-Rpc-NameService-ResolveAnyId-Request) | [Rpc.NameService.ResolveAnyId.Response](#anytype-Rpc-NameService-ResolveAnyId-Response) | 12D3KooWA8EXV3KjBxEU5EnsPfneLx84vMWAtTBQBeyooN82KSuS -&gt; hello.any |
| BroadcastPayloadEvent | [Rpc.Broadcast.PayloadEvent.Request](#anytype-Rpc-Broadcast-PayloadEvent-Request) | [Rpc.Broadcast.PayloadEvent.Response](#anytype-Rpc-Broadcast-PayloadEvent-Response) |  |
| DeviceSetName | [Rpc.Device.SetName.Request](#anytype-Rpc-Device-SetName-Request) | [Rpc.Device.SetName.Response](#anytype-Rpc-Device-SetName-Response) |  |
| DeviceList | [Rpc.Device.List.Request](#anytype-Rpc-Device-List-Request) | [Rpc.Device.List.Response](#anytype-Rpc-Device-List-Response) |  |
| DeviceNetworkStateSet | [Rpc.Device.NetworkState.Set.Request](#anytype-Rpc-Device-NetworkState-Set-Request) | [Rpc.Device.NetworkState.Set.Response](#anytype-Rpc-Device-NetworkState-Set-Response) |  |
| ChatAddMessage | [Rpc.Chat.AddMessage.Request](#anytype-Rpc-Chat-AddMessage-Request) | [Rpc.Chat.AddMessage.Response](#anytype-Rpc-Chat-AddMessage-Response) | Chats |
| ChatEditMessageContent | [Rpc.Chat.EditMessageContent.Request](#anytype-Rpc-Chat-EditMessageContent-Request) | [Rpc.Chat.EditMessageContent.Response](#anytype-Rpc-Chat-EditMessageContent-Response) |  |
| ChatToggleMessageReaction | [Rpc.Chat.ToggleMessageReaction.Request](#anytype-Rpc-Chat-ToggleMessageReaction-Request) | [Rpc.Chat.ToggleMessageReaction.Response](#anytype-Rpc-Chat-ToggleMessageReaction-Response) |  |
| ChatDeleteMessage | [Rpc.Chat.DeleteMessage.Request](#anytype-Rpc-Chat-DeleteMessage-Request) | [Rpc.Chat.DeleteMessage.Response](#anytype-Rpc-Chat-DeleteMessage-Response) |  |
| ChatGetMessages | [Rpc.Chat.GetMessages.Request](#anytype-Rpc-Chat-GetMessages-Request) | [Rpc.Chat.GetMessages.Response](#anytype-Rpc-Chat-GetMessages-Response) |  |
| ChatGetMessagesByIds | [Rpc.Chat.GetMessagesByIds.Request](#anytype-Rpc-Chat-GetMessagesByIds-Request) | [Rpc.Chat.GetMessagesByIds.Response](#anytype-Rpc-Chat-GetMessagesByIds-Response) |  |
| ChatSubscribeLastMessages | [Rpc.Chat.SubscribeLastMessages.Request](#anytype-Rpc-Chat-SubscribeLastMessages-Request) | [Rpc.Chat.SubscribeLastMessages.Response](#anytype-Rpc-Chat-SubscribeLastMessages-Response) |  |
| ChatUnsubscribe | [Rpc.Chat.Unsubscribe.Request](#anytype-Rpc-Chat-Unsubscribe-Request) | [Rpc.Chat.Unsubscribe.Response](#anytype-Rpc-Chat-Unsubscribe-Response) |  |
| ChatReadMessages | [Rpc.Chat.ReadMessages.Request](#anytype-Rpc-Chat-ReadMessages-Request) | [Rpc.Chat.ReadMessages.Response](#anytype-Rpc-Chat-ReadMessages-Response) |  |
| ChatUnreadMessages | [Rpc.Chat.Unread.Request](#anytype-Rpc-Chat-Unread-Request) | [Rpc.Chat.Unread.Response](#anytype-Rpc-Chat-Unread-Response) |  |
| ChatSubscribeToMessagePreviews | [Rpc.Chat.SubscribeToMessagePreviews.Request](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Request) | [Rpc.Chat.SubscribeToMessagePreviews.Response](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response) |  |
| ChatUnsubscribeFromMessagePreviews | [Rpc.Chat.UnsubscribeFromMessagePreviews.Request](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Request) | [Rpc.Chat.UnsubscribeFromMessagePreviews.Response](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response) |  |
| ObjectChatAdd | [Rpc.Object.ChatAdd.Request](#anytype-Rpc-Object-ChatAdd-Request) | [Rpc.Object.ChatAdd.Response](#anytype-Rpc-Object-ChatAdd-Response) |  |
| ChatReadAll | [Rpc.Chat.ReadAll.Request](#anytype-Rpc-Chat-ReadAll-Request) | [Rpc.Chat.ReadAll.Response](#anytype-Rpc-Chat-ReadAll-Response) |  |
| AIWritingTools | [Rpc.AI.WritingTools.Request](#anytype-Rpc-AI-WritingTools-Request) | [Rpc.AI.WritingTools.Response](#anytype-Rpc-AI-WritingTools-Response) | mock AI RPCs for compatibility between branches. Not implemented in main |
| AIAutofill | [Rpc.AI.Autofill.Request](#anytype-Rpc-AI-Autofill-Request) | [Rpc.AI.Autofill.Response](#anytype-Rpc-AI-Autofill-Response) |  |
| AIListSummary | [Rpc.AI.ListSummary.Request](#anytype-Rpc-AI-ListSummary-Request) | [Rpc.AI.ListSummary.Response](#anytype-Rpc-AI-ListSummary-Response) |  |
| AIObjectCreateFromUrl | [Rpc.AI.ObjectCreateFromUrl.Request](#anytype-Rpc-AI-ObjectCreateFromUrl-Request) | [Rpc.AI.ObjectCreateFromUrl.Response](#anytype-Rpc-AI-ObjectCreateFromUrl-Response) |  |
| PushNotificationRegisterToken | [Rpc.PushNotification.RegisterToken.Request](#anytype-Rpc-PushNotification-RegisterToken-Request) | [Rpc.PushNotification.RegisterToken.Response](#anytype-Rpc-PushNotification-RegisterToken-Response) | Push |
| PushNotificationSetSpaceMode | [Rpc.PushNotification.SetSpaceMode.Request](#anytype-Rpc-PushNotification-SetSpaceMode-Request) | [Rpc.PushNotification.SetSpaceMode.Response](#anytype-Rpc-PushNotification-SetSpaceMode-Response) |  |
| PushNotificationSetForceModeIds | [Rpc.PushNotification.SetForceModeIds.Request](#anytype-Rpc-PushNotification-SetForceModeIds-Request) | [Rpc.PushNotification.SetForceModeIds.Response](#anytype-Rpc-PushNotification-SetForceModeIds-Response) |  |
| PushNotificationResetIds | [Rpc.PushNotification.ResetIds.Request](#anytype-Rpc-PushNotification-ResetIds-Request) | [Rpc.PushNotification.ResetIds.Response](#anytype-Rpc-PushNotification-ResetIds-Response) |  |

 



<a name="pb_protos_changes-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/changes.proto



<a name="anytype-Change"></a>

### Change
the element of change tree used to store and internal apply smartBlock history


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| content | [Change.Content](#anytype-Change-Content) | repeated | set of actions to apply |
| snapshot | [Change.Snapshot](#anytype-Change-Snapshot) |  | snapshot - when not null, the Content will be ignored |
| fileKeys | [Change.FileKeys](#anytype-Change-FileKeys) | repeated | file keys related to changes content |
| timestamp | [int64](#int64) |  | creation timestamp |
| version | [uint32](#uint32) |  | version of business logic |
| changeType | [uint32](#uint32) |  | business-level type of change applied to object |






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
| objectTypeAdd | [Change.ObjectTypeAdd](#anytype-Change-ObjectTypeAdd) |  |  |
| objectTypeRemove | [Change.ObjectTypeRemove](#anytype-Change-ObjectTypeRemove) |  |  |
| storeKeySet | [Change.StoreKeySet](#anytype-Change-StoreKeySet) |  |  |
| storeKeyUnset | [Change.StoreKeyUnset](#anytype-Change-StoreKeyUnset) |  |  |
| storeSliceUpdate | [Change.StoreSliceUpdate](#anytype-Change-StoreSliceUpdate) |  |  |
| originalCreatedTimestampSet | [Change.OriginalCreatedTimestampSet](#anytype-Change-OriginalCreatedTimestampSet) |  |  |
| setFileInfo | [Change.SetFileInfo](#anytype-Change-SetFileInfo) |  |  |
| notificationCreate | [Change.NotificationCreate](#anytype-Change-NotificationCreate) |  |  |
| notificationUpdate | [Change.NotificationUpdate](#anytype-Change-NotificationUpdate) |  |  |
| deviceAdd | [Change.DeviceAdd](#anytype-Change-DeviceAdd) |  |  |
| deviceUpdate | [Change.DeviceUpdate](#anytype-Change-DeviceUpdate) |  |  |






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






<a name="anytype-Change-DeviceAdd"></a>

### Change.DeviceAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| device | [model.DeviceInfo](#anytype-model-DeviceInfo) |  |  |






<a name="anytype-Change-DeviceUpdate"></a>

### Change.DeviceUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |






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






<a name="anytype-Change-NotificationCreate"></a>

### Change.NotificationCreate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| notification | [model.Notification](#anytype-model-Notification) |  |  |






<a name="anytype-Change-NotificationUpdate"></a>

### Change.NotificationUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| status | [model.Notification.Status](#anytype-model-Notification-Status) |  |  |






<a name="anytype-Change-ObjectTypeAdd"></a>

### Change.ObjectTypeAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| key | [string](#string) |  |  |






<a name="anytype-Change-ObjectTypeRemove"></a>

### Change.ObjectTypeRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| key | [string](#string) |  |  |






<a name="anytype-Change-OriginalCreatedTimestampSet"></a>

### Change.OriginalCreatedTimestampSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ts | [int64](#int64) |  |  |






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






<a name="anytype-Change-SetFileInfo"></a>

### Change.SetFileInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fileInfo | [model.FileInfo](#anytype-model-FileInfo) |  |  |






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






<a name="anytype-ChangeNoSnapshot"></a>

### ChangeNoSnapshot



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| content | [Change.Content](#anytype-Change-Content) | repeated | set of actions to apply |
| fileKeys | [Change.FileKeys](#anytype-Change-FileKeys) | repeated | file keys related to changes content |
| timestamp | [int64](#int64) |  | creation timestamp |
| version | [uint32](#uint32) |  | version of business logic |
| changeType | [uint32](#uint32) |  | business-level type of change applied to object |






<a name="anytype-DocumentCreate"></a>

### DocumentCreate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| collection | [string](#string) |  |  |
| documentId | [string](#string) |  |  |
| value | [string](#string) |  | json |






<a name="anytype-DocumentDelete"></a>

### DocumentDelete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| collection | [string](#string) |  |  |
| documentId | [string](#string) |  |  |






<a name="anytype-DocumentModify"></a>

### DocumentModify



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| collection | [string](#string) |  |  |
| documentId | [string](#string) |  |  |
| keys | [KeyModify](#anytype-KeyModify) | repeated |  |






<a name="anytype-KeyModify"></a>

### KeyModify



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| keyPath | [string](#string) | repeated | key path; example: [user, email] |
| modifyOp | [ModifyOp](#anytype-ModifyOp) |  | modify op: set, unset, inc, etc. |
| modifyValue | [string](#string) |  | json value; example: &#39;&#34;new@email.com&#34;&#39; |






<a name="anytype-StoreChange"></a>

### StoreChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changeSet | [StoreChangeContent](#anytype-StoreChangeContent) | repeated |  |






<a name="anytype-StoreChangeContent"></a>

### StoreChangeContent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| create | [DocumentCreate](#anytype-DocumentCreate) |  |  |
| modify | [DocumentModify](#anytype-DocumentModify) |  |  |
| delete | [DocumentDelete](#anytype-DocumentDelete) |  |  |





 


<a name="anytype-ModifyOp"></a>

### ModifyOp


| Name | Number | Description |
| ---- | ------ | ----------- |
| Set | 0 |  |
| Unset | 1 |  |
| Inc | 2 |  |
| AddToSet | 3 |  |
| Pull | 4 |  |


 

 

 



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






<a name="anytype-Rpc-AI"></a>

### Rpc.AI







<a name="anytype-Rpc-AI-Autofill"></a>

### Rpc.AI.Autofill







<a name="anytype-Rpc-AI-Autofill-Request"></a>

### Rpc.AI.Autofill.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [Rpc.AI.ProviderConfig](#anytype-Rpc-AI-ProviderConfig) |  |  |
| mode | [Rpc.AI.Autofill.Request.AutofillMode](#anytype-Rpc-AI-Autofill-Request-AutofillMode) |  |  |
| options | [string](#string) | repeated |  |
| context | [string](#string) | repeated |  |






<a name="anytype-Rpc-AI-Autofill-Response"></a>

### Rpc.AI.Autofill.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.AI.Autofill.Response.Error](#anytype-Rpc-AI-Autofill-Response-Error) |  |  |
| text | [string](#string) |  |  |






<a name="anytype-Rpc-AI-Autofill-Response-Error"></a>

### Rpc.AI.Autofill.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.AI.Autofill.Response.Error.Code](#anytype-Rpc-AI-Autofill-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-AI-ListSummary"></a>

### Rpc.AI.ListSummary







<a name="anytype-Rpc-AI-ListSummary-Request"></a>

### Rpc.AI.ListSummary.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [Rpc.AI.ProviderConfig](#anytype-Rpc-AI-ProviderConfig) |  |  |
| spaceId | [string](#string) |  |  |
| objectIds | [string](#string) | repeated |  |
| prompt | [string](#string) |  |  |






<a name="anytype-Rpc-AI-ListSummary-Response"></a>

### Rpc.AI.ListSummary.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.AI.ListSummary.Response.Error](#anytype-Rpc-AI-ListSummary-Response-Error) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-AI-ListSummary-Response-Error"></a>

### Rpc.AI.ListSummary.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.AI.ListSummary.Response.Error.Code](#anytype-Rpc-AI-ListSummary-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-AI-ObjectCreateFromUrl"></a>

### Rpc.AI.ObjectCreateFromUrl







<a name="anytype-Rpc-AI-ObjectCreateFromUrl-Request"></a>

### Rpc.AI.ObjectCreateFromUrl.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [Rpc.AI.ProviderConfig](#anytype-Rpc-AI-ProviderConfig) |  |  |
| spaceId | [string](#string) |  |  |
| url | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-AI-ObjectCreateFromUrl-Response"></a>

### Rpc.AI.ObjectCreateFromUrl.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.AI.ObjectCreateFromUrl.Response.Error](#anytype-Rpc-AI-ObjectCreateFromUrl-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-AI-ObjectCreateFromUrl-Response-Error"></a>

### Rpc.AI.ObjectCreateFromUrl.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.AI.ObjectCreateFromUrl.Response.Error.Code](#anytype-Rpc-AI-ObjectCreateFromUrl-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-AI-ProviderConfig"></a>

### Rpc.AI.ProviderConfig



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| provider | [Rpc.AI.Provider](#anytype-Rpc-AI-Provider) |  |  |
| endpoint | [string](#string) |  |  |
| model | [string](#string) |  |  |
| token | [string](#string) |  |  |
| temperature | [float](#float) |  |  |






<a name="anytype-Rpc-AI-WritingTools"></a>

### Rpc.AI.WritingTools







<a name="anytype-Rpc-AI-WritingTools-Request"></a>

### Rpc.AI.WritingTools.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [Rpc.AI.ProviderConfig](#anytype-Rpc-AI-ProviderConfig) |  |  |
| mode | [Rpc.AI.WritingTools.Request.WritingMode](#anytype-Rpc-AI-WritingTools-Request-WritingMode) |  |  |
| language | [Rpc.AI.WritingTools.Request.Language](#anytype-Rpc-AI-WritingTools-Request-Language) |  |  |
| text | [string](#string) |  |  |






<a name="anytype-Rpc-AI-WritingTools-Response"></a>

### Rpc.AI.WritingTools.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.AI.WritingTools.Response.Error](#anytype-Rpc-AI-WritingTools-Response-Error) |  |  |
| text | [string](#string) |  |  |






<a name="anytype-Rpc-AI-WritingTools-Response-Error"></a>

### Rpc.AI.WritingTools.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.AI.WritingTools.Response.Error.Code](#anytype-Rpc-AI-WritingTools-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account"></a>

### Rpc.Account







<a name="anytype-Rpc-Account-ChangeJsonApiAddr"></a>

### Rpc.Account.ChangeJsonApiAddr







<a name="anytype-Rpc-Account-ChangeJsonApiAddr-Request"></a>

### Rpc.Account.ChangeJsonApiAddr.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| listenAddr | [string](#string) |  | make sure to use 127.0.0.1:x to not listen on all interfaces; recommended value is 127.0.0.1:31009 |






<a name="anytype-Rpc-Account-ChangeJsonApiAddr-Response"></a>

### Rpc.Account.ChangeJsonApiAddr.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.ChangeJsonApiAddr.Response.Error](#anytype-Rpc-Account-ChangeJsonApiAddr-Response-Error) |  |  |






<a name="anytype-Rpc-Account-ChangeJsonApiAddr-Response-Error"></a>

### Rpc.Account.ChangeJsonApiAddr.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.ChangeJsonApiAddr.Response.Error.Code](#anytype-Rpc-Account-ChangeJsonApiAddr-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-ChangeNetworkConfigAndRestart"></a>

### Rpc.Account.ChangeNetworkConfigAndRestart







<a name="anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Request"></a>

### Rpc.Account.ChangeNetworkConfigAndRestart.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| networkMode | [Rpc.Account.NetworkMode](#anytype-Rpc-Account-NetworkMode) |  |  |
| networkCustomConfigFilePath | [string](#string) |  |  |






<a name="anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response"></a>

### Rpc.Account.ChangeNetworkConfigAndRestart.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.ChangeNetworkConfigAndRestart.Response.Error](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response-Error) |  |  |






<a name="anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response-Error"></a>

### Rpc.Account.ChangeNetworkConfigAndRestart.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.ChangeNetworkConfigAndRestart.Response.Error.Code](#anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






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
| disableLocalNetworkSync | [bool](#bool) |  | Disable local network discovery |
| networkMode | [Rpc.Account.NetworkMode](#anytype-Rpc-Account-NetworkMode) |  | optional, default is DefaultConfig |
| networkCustomConfigFilePath | [string](#string) |  | config path for the custom network mode } |
| preferYamuxTransport | [bool](#bool) |  | optional, default is false, recommended in case of problems with QUIC transport |
| jsonApiListenAddr | [string](#string) |  | optional, if empty json api will not be started; 127.0.0.1:31009 should be the default one |
| joinStreamUrl | [string](#string) |  | anytype:// schema URL to join an embed stream |
| enableMembershipV2 | [bool](#bool) |  | if true - will run membership v2 polling loop, v2 methods will be available if false - will run membership v1 polling loop, v2 methods will return error

optional, default is false |






<a name="anytype-Rpc-Account-Create-Response"></a>

### Rpc.Account.Create.Response
Middleware-to-front-end response for an account creation request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Create.Response.Error](#anytype-Rpc-Account-Create-Response-Error) |  | Error while trying to create an account |
| account | [model.Account](#anytype-model-Account) |  | A newly created account; In case of a failure, i.e. error is non-NULL, the account model should contain empty/default-value fields |
| config | [Rpc.Account.Config](#anytype-Rpc-Account-Config) |  | deprecated, use account, GO-1926 |






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






<a name="anytype-Rpc-Account-EnableLocalNetworkSync"></a>

### Rpc.Account.EnableLocalNetworkSync







<a name="anytype-Rpc-Account-EnableLocalNetworkSync-Request"></a>

### Rpc.Account.EnableLocalNetworkSync.Request







<a name="anytype-Rpc-Account-EnableLocalNetworkSync-Response"></a>

### Rpc.Account.EnableLocalNetworkSync.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.EnableLocalNetworkSync.Response.Error](#anytype-Rpc-Account-EnableLocalNetworkSync-Response-Error) |  |  |






<a name="anytype-Rpc-Account-EnableLocalNetworkSync-Response-Error"></a>

### Rpc.Account.EnableLocalNetworkSync.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.EnableLocalNetworkSync.Response.Error.Code](#anytype-Rpc-Account-EnableLocalNetworkSync-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-GetConfig"></a>

### Rpc.Account.GetConfig
TODO: Remove this request if we do not need it, GO-1926






<a name="anytype-Rpc-Account-GetConfig-Get"></a>

### Rpc.Account.GetConfig.Get







<a name="anytype-Rpc-Account-GetConfig-Get-Request"></a>

### Rpc.Account.GetConfig.Get.Request







<a name="anytype-Rpc-Account-LocalLink"></a>

### Rpc.Account.LocalLink







<a name="anytype-Rpc-Account-LocalLink-CreateApp"></a>

### Rpc.Account.LocalLink.CreateApp







<a name="anytype-Rpc-Account-LocalLink-CreateApp-Request"></a>

### Rpc.Account.LocalLink.CreateApp.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| app | [model.Account.Auth.AppInfo](#anytype-model-Account-Auth-AppInfo) |  |  |






<a name="anytype-Rpc-Account-LocalLink-CreateApp-Response"></a>

### Rpc.Account.LocalLink.CreateApp.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.LocalLink.CreateApp.Response.Error](#anytype-Rpc-Account-LocalLink-CreateApp-Response-Error) |  |  |
| appKey | [string](#string) |  | persistent key, that can be used to restore session via CreateSession or for JSON API |






<a name="anytype-Rpc-Account-LocalLink-CreateApp-Response-Error"></a>

### Rpc.Account.LocalLink.CreateApp.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.LocalLink.CreateApp.Response.Error.Code](#anytype-Rpc-Account-LocalLink-CreateApp-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-ListApps"></a>

### Rpc.Account.LocalLink.ListApps







<a name="anytype-Rpc-Account-LocalLink-ListApps-Request"></a>

### Rpc.Account.LocalLink.ListApps.Request







<a name="anytype-Rpc-Account-LocalLink-ListApps-Response"></a>

### Rpc.Account.LocalLink.ListApps.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.LocalLink.ListApps.Response.Error](#anytype-Rpc-Account-LocalLink-ListApps-Response-Error) |  |  |
| app | [model.Account.Auth.AppInfo](#anytype-model-Account-Auth-AppInfo) | repeated |  |






<a name="anytype-Rpc-Account-LocalLink-ListApps-Response-Error"></a>

### Rpc.Account.LocalLink.ListApps.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.LocalLink.ListApps.Response.Error.Code](#anytype-Rpc-Account-LocalLink-ListApps-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-NewChallenge"></a>

### Rpc.Account.LocalLink.NewChallenge







<a name="anytype-Rpc-Account-LocalLink-NewChallenge-Request"></a>

### Rpc.Account.LocalLink.NewChallenge.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| appName | [string](#string) |  | just for info, not secure to rely on |
| scope | [model.Account.Auth.LocalApiScope](#anytype-model-Account-Auth-LocalApiScope) |  |  |






<a name="anytype-Rpc-Account-LocalLink-NewChallenge-Response"></a>

### Rpc.Account.LocalLink.NewChallenge.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.LocalLink.NewChallenge.Response.Error](#anytype-Rpc-Account-LocalLink-NewChallenge-Response-Error) |  |  |
| challengeId | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-NewChallenge-Response-Error"></a>

### Rpc.Account.LocalLink.NewChallenge.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.LocalLink.NewChallenge.Response.Error.Code](#anytype-Rpc-Account-LocalLink-NewChallenge-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-RevokeApp"></a>

### Rpc.Account.LocalLink.RevokeApp







<a name="anytype-Rpc-Account-LocalLink-RevokeApp-Request"></a>

### Rpc.Account.LocalLink.RevokeApp.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| appHash | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-RevokeApp-Response"></a>

### Rpc.Account.LocalLink.RevokeApp.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.LocalLink.RevokeApp.Response.Error](#anytype-Rpc-Account-LocalLink-RevokeApp-Response-Error) |  |  |






<a name="anytype-Rpc-Account-LocalLink-RevokeApp-Response-Error"></a>

### Rpc.Account.LocalLink.RevokeApp.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.LocalLink.RevokeApp.Response.Error.Code](#anytype-Rpc-Account-LocalLink-RevokeApp-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-SolveChallenge"></a>

### Rpc.Account.LocalLink.SolveChallenge







<a name="anytype-Rpc-Account-LocalLink-SolveChallenge-Request"></a>

### Rpc.Account.LocalLink.SolveChallenge.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| challengeId | [string](#string) |  |  |
| answer | [string](#string) |  |  |






<a name="anytype-Rpc-Account-LocalLink-SolveChallenge-Response"></a>

### Rpc.Account.LocalLink.SolveChallenge.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.LocalLink.SolveChallenge.Response.Error](#anytype-Rpc-Account-LocalLink-SolveChallenge-Response-Error) |  |  |
| sessionToken | [string](#string) |  | ephemeral token for the session |
| appKey | [string](#string) |  | persistent key, that can be used to restore session via CreateSession |






<a name="anytype-Rpc-Account-LocalLink-SolveChallenge-Response-Error"></a>

### Rpc.Account.LocalLink.SolveChallenge.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.LocalLink.SolveChallenge.Response.Error.Code](#anytype-Rpc-Account-LocalLink-SolveChallenge-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-Migrate"></a>

### Rpc.Account.Migrate







<a name="anytype-Rpc-Account-Migrate-Request"></a>

### Rpc.Account.Migrate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Id of a selected account |
| rootPath | [string](#string) |  |  |
| fulltextPrimaryLanguage | [string](#string) |  | optional, default fts language |






<a name="anytype-Rpc-Account-Migrate-Response"></a>

### Rpc.Account.Migrate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Migrate.Response.Error](#anytype-Rpc-Account-Migrate-Response-Error) |  |  |






<a name="anytype-Rpc-Account-Migrate-Response-Error"></a>

### Rpc.Account.Migrate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Migrate.Response.Error.Code](#anytype-Rpc-Account-Migrate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |
| requiredSpace | [int64](#int64) |  |  |






<a name="anytype-Rpc-Account-MigrateCancel"></a>

### Rpc.Account.MigrateCancel







<a name="anytype-Rpc-Account-MigrateCancel-Request"></a>

### Rpc.Account.MigrateCancel.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Id of a selected account |






<a name="anytype-Rpc-Account-MigrateCancel-Response"></a>

### Rpc.Account.MigrateCancel.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.MigrateCancel.Response.Error](#anytype-Rpc-Account-MigrateCancel-Response-Error) |  |  |






<a name="anytype-Rpc-Account-MigrateCancel-Response-Error"></a>

### Rpc.Account.MigrateCancel.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.MigrateCancel.Response.Error.Code](#anytype-Rpc-Account-MigrateCancel-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






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
| fulltextPrimaryLanguage | [string](#string) |  | optional, default fts language |






<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Response"></a>

### Rpc.Account.RecoverFromLegacyExport.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountId | [string](#string) |  |  |
| personalSpaceId | [string](#string) |  |  |
| error | [Rpc.Account.RecoverFromLegacyExport.Response.Error](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error) |  |  |






<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error"></a>

### Rpc.Account.RecoverFromLegacyExport.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.RecoverFromLegacyExport.Response.Error.Code](#anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Account-RevertDeletion"></a>

### Rpc.Account.RevertDeletion







<a name="anytype-Rpc-Account-RevertDeletion-Request"></a>

### Rpc.Account.RevertDeletion.Request







<a name="anytype-Rpc-Account-RevertDeletion-Response"></a>

### Rpc.Account.RevertDeletion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.RevertDeletion.Response.Error](#anytype-Rpc-Account-RevertDeletion-Response-Error) |  | Error while trying to recover an account |
| status | [model.Account.Status](#anytype-model-Account-Status) |  |  |






<a name="anytype-Rpc-Account-RevertDeletion-Response-Error"></a>

### Rpc.Account.RevertDeletion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.RevertDeletion.Response.Error.Code](#anytype-Rpc-Account-RevertDeletion-Response-Error-Code) |  |  |
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
| disableLocalNetworkSync | [bool](#bool) |  | Disable local network discovery |
| networkMode | [Rpc.Account.NetworkMode](#anytype-Rpc-Account-NetworkMode) |  | optional, default is DefaultConfig |
| networkCustomConfigFilePath | [string](#string) |  | config path for the custom network mode |
| preferYamuxTransport | [bool](#bool) |  | optional, default is false, recommended in case of problems with QUIC transport |
| jsonApiListenAddr | [string](#string) |  | optional, if empty json api will not be started; 127.0.0.1:31009 should be the default one |
| fulltextPrimaryLanguage | [string](#string) |  | optional, default fts language |
| joinStreamURL | [string](#string) |  | anytype:// schema URL to join an embed stream |
| enableMembershipV2 | [bool](#bool) |  | if true - will run membership v2 polling loop, v2 methods will be available if false - will run membership v1 polling loop, v2 methods will return error

optional, default is false |






<a name="anytype-Rpc-Account-Select-Response"></a>

### Rpc.Account.Select.Response
Middleware-to-front-end response for an account select request, that can contain a NULL error and selected account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Select.Response.Error](#anytype-Rpc-Account-Select-Response-Error) |  | Error while trying to launch/select an account |
| account | [model.Account](#anytype-model-Account) |  | Selected account |
| config | [Rpc.Account.Config](#anytype-Rpc-Account-Config) |  | deprecated, use account, GO-1926 |






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
| viewId | [string](#string) |  |  |






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
| objectTypeUniqueKey | [string](#string) |  |  |
| templateId | [string](#string) |  |  |
| block | [model.Block](#anytype-model-Block) |  |  |






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
| url | [string](#string) |  |  |






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






<a name="anytype-Rpc-Block-Preview"></a>

### Rpc.Block.Preview







<a name="anytype-Rpc-Block-Preview-Request"></a>

### Rpc.Block.Preview.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| html | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-Block-Preview-Response"></a>

### Rpc.Block.Preview.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Preview.Response.Error](#anytype-Rpc-Block-Preview-Response-Error) |  |  |
| blocks | [model.Block](#anytype-model-Block) | repeated |  |






<a name="anytype-Rpc-Block-Preview-Response-Error"></a>

### Rpc.Block.Preview.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Preview.Response.Error.Code](#anytype-Rpc-Block-Preview-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Block-SetCarriage"></a>

### Rpc.Block.SetCarriage







<a name="anytype-Rpc-Block-SetCarriage-Request"></a>

### Rpc.Block.SetCarriage.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype-model-Range) |  |  |






<a name="anytype-Rpc-Block-SetCarriage-Response"></a>

### Rpc.Block.SetCarriage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.SetCarriage.Response.Error](#anytype-Rpc-Block-SetCarriage-Response-Error) |  |  |






<a name="anytype-Rpc-Block-SetCarriage-Response-Error"></a>

### Rpc.Block.SetCarriage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.SetCarriage.Response.Error.Code](#anytype-Rpc-Block-SetCarriage-Response-Error-Code) |  |  |
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
| bytes | [bytes](#bytes) |  |  |






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
| templateId | [string](#string) |  |  |






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
| templateId | [string](#string) |  |  |






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






<a name="anytype-Rpc-BlockDataview-Relation-Set"></a>

### Rpc.BlockDataview.Relation.Set







<a name="anytype-Rpc-BlockDataview-Relation-Set-Request"></a>

### Rpc.BlockDataview.Relation.Set.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to set relation |
| relationKeys | [string](#string) | repeated |  |






<a name="anytype-Rpc-BlockDataview-Relation-Set-Response"></a>

### Rpc.BlockDataview.Relation.Set.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.Set.Response.Error](#anytype-Rpc-BlockDataview-Relation-Set-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Relation-Set-Response-Error"></a>

### Rpc.BlockDataview.Relation.Set.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.Set.Response.Error.Code](#anytype-Rpc-BlockDataview-Relation-Set-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-BlockDataview-Sort-SSort"></a>

### Rpc.BlockDataview.Sort.SSort







<a name="anytype-Rpc-BlockDataview-Sort-SSort-Request"></a>

### Rpc.BlockDataview.Sort.SSort.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| ids | [string](#string) | repeated | new order of sorts |






<a name="anytype-Rpc-BlockDataview-Sort-SSort-Response"></a>

### Rpc.BlockDataview.Sort.SSort.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Sort.SSort.Response.Error](#anytype-Rpc-BlockDataview-Sort-SSort-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockDataview-Sort-SSort-Response-Error"></a>

### Rpc.BlockDataview.Sort.SSort.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Sort.SSort.Response.Error.Code](#anytype-Rpc-BlockDataview-Sort-SSort-Response-Error-Code) |  |  |
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
set the current active view locally






<a name="anytype-Rpc-BlockDataview-View-SetActive-Request"></a>

### Rpc.BlockDataview.View.SetActive.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block |
| viewId | [string](#string) |  | id of active view |






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
| imageKind | [model.ImageKind](#anytype-model-ImageKind) |  |  |






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






<a name="anytype-Rpc-BlockFile-SetTargetObjectId"></a>

### Rpc.BlockFile.SetTargetObjectId







<a name="anytype-Rpc-BlockFile-SetTargetObjectId-Request"></a>

### Rpc.BlockFile.SetTargetObjectId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-BlockFile-SetTargetObjectId-Response"></a>

### Rpc.BlockFile.SetTargetObjectId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.SetTargetObjectId.Response.Error](#anytype-Rpc-BlockFile-SetTargetObjectId-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockFile-SetTargetObjectId-Response-Error"></a>

### Rpc.BlockFile.SetTargetObjectId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.SetTargetObjectId.Response.Error.Code](#anytype-Rpc-BlockFile-SetTargetObjectId-Response-Error-Code) |  |  |
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







<a name="anytype-Rpc-BlockLatex-SetProcessor"></a>

### Rpc.BlockLatex.SetProcessor







<a name="anytype-Rpc-BlockLatex-SetProcessor-Request"></a>

### Rpc.BlockLatex.SetProcessor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| processor | [model.Block.Content.Latex.Processor](#anytype-model-Block-Content-Latex-Processor) |  |  |






<a name="anytype-Rpc-BlockLatex-SetProcessor-Response"></a>

### Rpc.BlockLatex.SetProcessor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLatex.SetProcessor.Response.Error](#anytype-Rpc-BlockLatex-SetProcessor-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockLatex-SetProcessor-Response-Error"></a>

### Rpc.BlockLatex.SetProcessor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLatex.SetProcessor.Response.Error.Code](#anytype-Rpc-BlockLatex-SetProcessor-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-BlockLatex-SetText"></a>

### Rpc.BlockLatex.SetText







<a name="anytype-Rpc-BlockLatex-SetText-Request"></a>

### Rpc.BlockLatex.SetText.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |
| processor | [model.Block.Content.Latex.Processor](#anytype-model-Block-Content-Latex-Processor) |  |  |






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
| spaceId | [string](#string) |  |  |
| objectTypeUniqueKey | [string](#string) |  |  |
| block | [model.Block](#anytype-model-Block) |  |  |
| targetId | [string](#string) |  | link block params

id of the closest simple block |
| position | [model.Block.Position](#anytype-model-Block-Position) |  |  |
| fields | [google.protobuf.Struct](#google-protobuf-Struct) |  | deprecated link block fields |






<a name="anytype-Rpc-BlockLink-CreateWithObject-Response"></a>

### Rpc.BlockLink.CreateWithObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLink.CreateWithObject.Response.Error](#anytype-Rpc-BlockLink-CreateWithObject-Response-Error) |  |  |
| blockId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






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
| selectedTextRange | [model.Range](#anytype-model-Range) |  |  |






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






<a name="anytype-Rpc-BlockWidget-SetViewId"></a>

### Rpc.BlockWidget.SetViewId







<a name="anytype-Rpc-BlockWidget-SetViewId-Request"></a>

### Rpc.BlockWidget.SetViewId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| viewId | [string](#string) |  |  |






<a name="anytype-Rpc-BlockWidget-SetViewId-Response"></a>

### Rpc.BlockWidget.SetViewId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockWidget.SetViewId.Response.Error](#anytype-Rpc-BlockWidget-SetViewId-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-BlockWidget-SetViewId-Response-Error"></a>

### Rpc.BlockWidget.SetViewId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockWidget.SetViewId.Response.Error.Code](#anytype-Rpc-BlockWidget-SetViewId-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Broadcast"></a>

### Rpc.Broadcast







<a name="anytype-Rpc-Broadcast-PayloadEvent"></a>

### Rpc.Broadcast.PayloadEvent







<a name="anytype-Rpc-Broadcast-PayloadEvent-Request"></a>

### Rpc.Broadcast.PayloadEvent.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| payload | [string](#string) |  |  |






<a name="anytype-Rpc-Broadcast-PayloadEvent-Response"></a>

### Rpc.Broadcast.PayloadEvent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| error | [Rpc.Broadcast.PayloadEvent.Response.Error](#anytype-Rpc-Broadcast-PayloadEvent-Response-Error) |  |  |






<a name="anytype-Rpc-Broadcast-PayloadEvent-Response-Error"></a>

### Rpc.Broadcast.PayloadEvent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Broadcast.PayloadEvent.Response.Error.Code](#anytype-Rpc-Broadcast-PayloadEvent-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat"></a>

### Rpc.Chat







<a name="anytype-Rpc-Chat-AddMessage"></a>

### Rpc.Chat.AddMessage







<a name="anytype-Rpc-Chat-AddMessage-Request"></a>

### Rpc.Chat.AddMessage.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  |  |
| message | [model.ChatMessage](#anytype-model-ChatMessage) |  |  |






<a name="anytype-Rpc-Chat-AddMessage-Response"></a>

### Rpc.Chat.AddMessage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.AddMessage.Response.Error](#anytype-Rpc-Chat-AddMessage-Response-Error) |  |  |
| messageId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Chat-AddMessage-Response-Error"></a>

### Rpc.Chat.AddMessage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.AddMessage.Response.Error.Code](#anytype-Rpc-Chat-AddMessage-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-DeleteMessage"></a>

### Rpc.Chat.DeleteMessage







<a name="anytype-Rpc-Chat-DeleteMessage-Request"></a>

### Rpc.Chat.DeleteMessage.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  |  |
| messageId | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-DeleteMessage-Response"></a>

### Rpc.Chat.DeleteMessage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.DeleteMessage.Response.Error](#anytype-Rpc-Chat-DeleteMessage-Response-Error) |  |  |






<a name="anytype-Rpc-Chat-DeleteMessage-Response-Error"></a>

### Rpc.Chat.DeleteMessage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.DeleteMessage.Response.Error.Code](#anytype-Rpc-Chat-DeleteMessage-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-EditMessageContent"></a>

### Rpc.Chat.EditMessageContent







<a name="anytype-Rpc-Chat-EditMessageContent-Request"></a>

### Rpc.Chat.EditMessageContent.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  |  |
| messageId | [string](#string) |  |  |
| editedMessage | [model.ChatMessage](#anytype-model-ChatMessage) |  |  |






<a name="anytype-Rpc-Chat-EditMessageContent-Response"></a>

### Rpc.Chat.EditMessageContent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.EditMessageContent.Response.Error](#anytype-Rpc-Chat-EditMessageContent-Response-Error) |  |  |






<a name="anytype-Rpc-Chat-EditMessageContent-Response-Error"></a>

### Rpc.Chat.EditMessageContent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.EditMessageContent.Response.Error.Code](#anytype-Rpc-Chat-EditMessageContent-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-GetMessages"></a>

### Rpc.Chat.GetMessages







<a name="anytype-Rpc-Chat-GetMessages-Request"></a>

### Rpc.Chat.GetMessages.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  |  |
| afterOrderId | [string](#string) |  | OrderId of the message after which to get messages |
| beforeOrderId | [string](#string) |  | OrderId of the message before which to get messages |
| limit | [int32](#int32) |  |  |
| includeBoundary | [bool](#bool) |  | If true, include a message at the boundary (afterOrderId or beforeOrderId) |






<a name="anytype-Rpc-Chat-GetMessages-Response"></a>

### Rpc.Chat.GetMessages.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.GetMessages.Response.Error](#anytype-Rpc-Chat-GetMessages-Response-Error) |  |  |
| messages | [model.ChatMessage](#anytype-model-ChatMessage) | repeated |  |
| chatState | [model.ChatState](#anytype-model-ChatState) |  |  |






<a name="anytype-Rpc-Chat-GetMessages-Response-Error"></a>

### Rpc.Chat.GetMessages.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.GetMessages.Response.Error.Code](#anytype-Rpc-Chat-GetMessages-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-GetMessagesByIds"></a>

### Rpc.Chat.GetMessagesByIds







<a name="anytype-Rpc-Chat-GetMessagesByIds-Request"></a>

### Rpc.Chat.GetMessagesByIds.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  |  |
| messageIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-Chat-GetMessagesByIds-Response"></a>

### Rpc.Chat.GetMessagesByIds.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.GetMessagesByIds.Response.Error](#anytype-Rpc-Chat-GetMessagesByIds-Response-Error) |  |  |
| messages | [model.ChatMessage](#anytype-model-ChatMessage) | repeated |  |






<a name="anytype-Rpc-Chat-GetMessagesByIds-Response-Error"></a>

### Rpc.Chat.GetMessagesByIds.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.GetMessagesByIds.Response.Error.Code](#anytype-Rpc-Chat-GetMessagesByIds-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-ReadAll"></a>

### Rpc.Chat.ReadAll







<a name="anytype-Rpc-Chat-ReadAll-Request"></a>

### Rpc.Chat.ReadAll.Request







<a name="anytype-Rpc-Chat-ReadAll-Response"></a>

### Rpc.Chat.ReadAll.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.ReadAll.Response.Error](#anytype-Rpc-Chat-ReadAll-Response-Error) |  |  |






<a name="anytype-Rpc-Chat-ReadAll-Response-Error"></a>

### Rpc.Chat.ReadAll.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.ReadAll.Response.Error.Code](#anytype-Rpc-Chat-ReadAll-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-ReadMessages"></a>

### Rpc.Chat.ReadMessages







<a name="anytype-Rpc-Chat-ReadMessages-Request"></a>

### Rpc.Chat.ReadMessages.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [Rpc.Chat.ReadMessages.ReadType](#anytype-Rpc-Chat-ReadMessages-ReadType) |  |  |
| chatObjectId | [string](#string) |  | id of the chat object |
| afterOrderId | [string](#string) |  | read from this orderId; if empty - read from the beginning of the chat |
| beforeOrderId | [string](#string) |  | read til this orderId |
| lastStateId | [string](#string) |  | stateId from the last processed ChatState event(or GetMessages). Used to prevent race conditions |






<a name="anytype-Rpc-Chat-ReadMessages-Response"></a>

### Rpc.Chat.ReadMessages.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.ReadMessages.Response.Error](#anytype-Rpc-Chat-ReadMessages-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Chat-ReadMessages-Response-Error"></a>

### Rpc.Chat.ReadMessages.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.ReadMessages.Response.Error.Code](#anytype-Rpc-Chat-ReadMessages-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-SubscribeLastMessages"></a>

### Rpc.Chat.SubscribeLastMessages







<a name="anytype-Rpc-Chat-SubscribeLastMessages-Request"></a>

### Rpc.Chat.SubscribeLastMessages.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  | Identifier for the chat |
| limit | [int32](#int32) |  | Number of max last messages to return and subscribe |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-SubscribeLastMessages-Response"></a>

### Rpc.Chat.SubscribeLastMessages.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.SubscribeLastMessages.Response.Error](#anytype-Rpc-Chat-SubscribeLastMessages-Response-Error) |  |  |
| messages | [model.ChatMessage](#anytype-model-ChatMessage) | repeated | List of messages |
| numMessagesBefore | [int32](#int32) |  | Number of messages before the returned messages |
| chatState | [model.ChatState](#anytype-model-ChatState) |  | Chat state |






<a name="anytype-Rpc-Chat-SubscribeLastMessages-Response-Error"></a>

### Rpc.Chat.SubscribeLastMessages.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.SubscribeLastMessages.Response.Error.Code](#anytype-Rpc-Chat-SubscribeLastMessages-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-SubscribeToMessagePreviews"></a>

### Rpc.Chat.SubscribeToMessagePreviews







<a name="anytype-Rpc-Chat-SubscribeToMessagePreviews-Request"></a>

### Rpc.Chat.SubscribeToMessagePreviews.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-SubscribeToMessagePreviews-Response"></a>

### Rpc.Chat.SubscribeToMessagePreviews.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.SubscribeToMessagePreviews.Response.Error](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-Error) |  |  |
| previews | [Rpc.Chat.SubscribeToMessagePreviews.Response.ChatPreview](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-ChatPreview) | repeated |  |






<a name="anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-ChatPreview"></a>

### Rpc.Chat.SubscribeToMessagePreviews.Response.ChatPreview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| chatObjectId | [string](#string) |  |  |
| message | [model.ChatMessage](#anytype-model-ChatMessage) |  |  |
| state | [model.ChatState](#anytype-model-ChatState) |  |  |
| dependencies | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |






<a name="anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-Error"></a>

### Rpc.Chat.SubscribeToMessagePreviews.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.SubscribeToMessagePreviews.Response.Error.Code](#anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-ToggleMessageReaction"></a>

### Rpc.Chat.ToggleMessageReaction







<a name="anytype-Rpc-Chat-ToggleMessageReaction-Request"></a>

### Rpc.Chat.ToggleMessageReaction.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  |  |
| messageId | [string](#string) |  |  |
| emoji | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-ToggleMessageReaction-Response"></a>

### Rpc.Chat.ToggleMessageReaction.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.ToggleMessageReaction.Response.Error](#anytype-Rpc-Chat-ToggleMessageReaction-Response-Error) |  |  |
| added | [bool](#bool) |  | Added is true when reaction is added, false when removed |






<a name="anytype-Rpc-Chat-ToggleMessageReaction-Response-Error"></a>

### Rpc.Chat.ToggleMessageReaction.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.ToggleMessageReaction.Response.Error.Code](#anytype-Rpc-Chat-ToggleMessageReaction-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-Unread"></a>

### Rpc.Chat.Unread







<a name="anytype-Rpc-Chat-Unread-Request"></a>

### Rpc.Chat.Unread.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [Rpc.Chat.Unread.ReadType](#anytype-Rpc-Chat-Unread-ReadType) |  |  |
| chatObjectId | [string](#string) |  |  |
| afterOrderId | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-Unread-Response"></a>

### Rpc.Chat.Unread.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.Unread.Response.Error](#anytype-Rpc-Chat-Unread-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Chat-Unread-Response-Error"></a>

### Rpc.Chat.Unread.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.Unread.Response.Error.Code](#anytype-Rpc-Chat-Unread-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-Unsubscribe"></a>

### Rpc.Chat.Unsubscribe







<a name="anytype-Rpc-Chat-Unsubscribe-Request"></a>

### Rpc.Chat.Unsubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| chatObjectId | [string](#string) |  | Identifier for the chat |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-Unsubscribe-Response"></a>

### Rpc.Chat.Unsubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.Unsubscribe.Response.Error](#anytype-Rpc-Chat-Unsubscribe-Response-Error) |  |  |






<a name="anytype-Rpc-Chat-Unsubscribe-Response-Error"></a>

### Rpc.Chat.Unsubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.Unsubscribe.Response.Error.Code](#anytype-Rpc-Chat-Unsubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-UnsubscribeFromMessagePreviews"></a>

### Rpc.Chat.UnsubscribeFromMessagePreviews







<a name="anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Request"></a>

### Rpc.Chat.UnsubscribeFromMessagePreviews.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response"></a>

### Rpc.Chat.UnsubscribeFromMessagePreviews.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Chat.UnsubscribeFromMessagePreviews.Response.Error](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response-Error) |  |  |






<a name="anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response-Error"></a>

### Rpc.Chat.UnsubscribeFromMessagePreviews.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Chat.UnsubscribeFromMessagePreviews.Response.Error.Code](#anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug"></a>

### Rpc.Debug







<a name="anytype-Rpc-Debug-AccountSelectTrace"></a>

### Rpc.Debug.AccountSelectTrace







<a name="anytype-Rpc-Debug-AccountSelectTrace-Request"></a>

### Rpc.Debug.AccountSelectTrace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dir | [string](#string) |  | empty means using OS-provided temp dir |






<a name="anytype-Rpc-Debug-AccountSelectTrace-Response"></a>

### Rpc.Debug.AccountSelectTrace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.AccountSelectTrace.Response.Error](#anytype-Rpc-Debug-AccountSelectTrace-Response-Error) |  |  |
| path | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-AccountSelectTrace-Response-Error"></a>

### Rpc.Debug.AccountSelectTrace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.AccountSelectTrace.Response.Error.Code](#anytype-Rpc-Debug-AccountSelectTrace-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-AnystoreObjectChanges"></a>

### Rpc.Debug.AnystoreObjectChanges







<a name="anytype-Rpc-Debug-AnystoreObjectChanges-Request"></a>

### Rpc.Debug.AnystoreObjectChanges.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| orderBy | [Rpc.Debug.AnystoreObjectChanges.Request.OrderBy](#anytype-Rpc-Debug-AnystoreObjectChanges-Request-OrderBy) |  |  |






<a name="anytype-Rpc-Debug-AnystoreObjectChanges-Response"></a>

### Rpc.Debug.AnystoreObjectChanges.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.AnystoreObjectChanges.Response.Error](#anytype-Rpc-Debug-AnystoreObjectChanges-Response-Error) |  |  |
| changes | [Rpc.Debug.AnystoreObjectChanges.Response.Change](#anytype-Rpc-Debug-AnystoreObjectChanges-Response-Change) | repeated |  |
| wrongOrder | [bool](#bool) |  |  |






<a name="anytype-Rpc-Debug-AnystoreObjectChanges-Response-Change"></a>

### Rpc.Debug.AnystoreObjectChanges.Response.Change



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| changeId | [string](#string) |  |  |
| orderId | [string](#string) |  |  |
| error | [string](#string) |  |  |
| change | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Debug-AnystoreObjectChanges-Response-Error"></a>

### Rpc.Debug.AnystoreObjectChanges.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.AnystoreObjectChanges.Response.Error.Code](#anytype-Rpc-Debug-AnystoreObjectChanges-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-ExportLocalstore"></a>

### Rpc.Debug.ExportLocalstore







<a name="anytype-Rpc-Debug-ExportLocalstore-Request"></a>

### Rpc.Debug.ExportLocalstore.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| docIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |
| spaceId | [string](#string) |  |  |






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






<a name="anytype-Rpc-Debug-ExportLog"></a>

### Rpc.Debug.ExportLog







<a name="anytype-Rpc-Debug-ExportLog-Request"></a>

### Rpc.Debug.ExportLog.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dir | [string](#string) |  | empty means using OS-provided temp dir |






<a name="anytype-Rpc-Debug-ExportLog-Response"></a>

### Rpc.Debug.ExportLog.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.ExportLog.Response.Error](#anytype-Rpc-Debug-ExportLog-Response-Error) |  |  |
| path | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-ExportLog-Response-Error"></a>

### Rpc.Debug.ExportLog.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.ExportLog.Response.Error.Code](#anytype-Rpc-Debug-ExportLog-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-NetCheck"></a>

### Rpc.Debug.NetCheck







<a name="anytype-Rpc-Debug-NetCheck-Request"></a>

### Rpc.Debug.NetCheck.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| clientYml | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-NetCheck-Response"></a>

### Rpc.Debug.NetCheck.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.NetCheck.Response.Error](#anytype-Rpc-Debug-NetCheck-Response-Error) |  |  |
| result | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-NetCheck-Response-Error"></a>

### Rpc.Debug.NetCheck.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.NetCheck.Response.Error.Code](#anytype-Rpc-Debug-NetCheck-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-OpenedObjects"></a>

### Rpc.Debug.OpenedObjects







<a name="anytype-Rpc-Debug-OpenedObjects-Request"></a>

### Rpc.Debug.OpenedObjects.Request







<a name="anytype-Rpc-Debug-OpenedObjects-Response"></a>

### Rpc.Debug.OpenedObjects.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.OpenedObjects.Response.Error](#anytype-Rpc-Debug-OpenedObjects-Response-Error) |  |  |
| objectIDs | [string](#string) | repeated |  |






<a name="anytype-Rpc-Debug-OpenedObjects-Response-Error"></a>

### Rpc.Debug.OpenedObjects.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.OpenedObjects.Response.Error.Code](#anytype-Rpc-Debug-OpenedObjects-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Debug-RunProfiler"></a>

### Rpc.Debug.RunProfiler







<a name="anytype-Rpc-Debug-RunProfiler-Request"></a>

### Rpc.Debug.RunProfiler.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| durationInSeconds | [int32](#int32) |  |  |






<a name="anytype-Rpc-Debug-RunProfiler-Response"></a>

### Rpc.Debug.RunProfiler.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.RunProfiler.Response.Error](#anytype-Rpc-Debug-RunProfiler-Response-Error) |  |  |
| path | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-RunProfiler-Response-Error"></a>

### Rpc.Debug.RunProfiler.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.RunProfiler.Response.Error.Code](#anytype-Rpc-Debug-RunProfiler-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-SpaceSummary"></a>

### Rpc.Debug.SpaceSummary







<a name="anytype-Rpc-Debug-SpaceSummary-Request"></a>

### Rpc.Debug.SpaceSummary.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






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






<a name="anytype-Rpc-Debug-StackGoroutines"></a>

### Rpc.Debug.StackGoroutines







<a name="anytype-Rpc-Debug-StackGoroutines-Request"></a>

### Rpc.Debug.StackGoroutines.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-StackGoroutines-Response"></a>

### Rpc.Debug.StackGoroutines.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.StackGoroutines.Response.Error](#anytype-Rpc-Debug-StackGoroutines-Response-Error) |  |  |






<a name="anytype-Rpc-Debug-StackGoroutines-Response-Error"></a>

### Rpc.Debug.StackGoroutines.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.StackGoroutines.Response.Error.Code](#anytype-Rpc-Debug-StackGoroutines-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-Stat"></a>

### Rpc.Debug.Stat







<a name="anytype-Rpc-Debug-Stat-Request"></a>

### Rpc.Debug.Stat.Request







<a name="anytype-Rpc-Debug-Stat-Response"></a>

### Rpc.Debug.Stat.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Stat.Response.Error](#anytype-Rpc-Debug-Stat-Response-Error) |  |  |
| jsonStat | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-Stat-Response-Error"></a>

### Rpc.Debug.Stat.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Stat.Response.Error.Code](#anytype-Rpc-Debug-Stat-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Debug-Subscriptions"></a>

### Rpc.Debug.Subscriptions







<a name="anytype-Rpc-Debug-Subscriptions-Request"></a>

### Rpc.Debug.Subscriptions.Request







<a name="anytype-Rpc-Debug-Subscriptions-Response"></a>

### Rpc.Debug.Subscriptions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Subscriptions.Response.Error](#anytype-Rpc-Debug-Subscriptions-Response-Error) |  |  |
| subscriptions | [string](#string) | repeated |  |






<a name="anytype-Rpc-Debug-Subscriptions-Response-Error"></a>

### Rpc.Debug.Subscriptions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Subscriptions.Response.Error.Code](#anytype-Rpc-Debug-Subscriptions-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Device"></a>

### Rpc.Device







<a name="anytype-Rpc-Device-List"></a>

### Rpc.Device.List







<a name="anytype-Rpc-Device-List-Request"></a>

### Rpc.Device.List.Request







<a name="anytype-Rpc-Device-List-Response"></a>

### Rpc.Device.List.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Device.List.Response.Error](#anytype-Rpc-Device-List-Response-Error) |  |  |
| devices | [model.DeviceInfo](#anytype-model-DeviceInfo) | repeated |  |






<a name="anytype-Rpc-Device-List-Response-Error"></a>

### Rpc.Device.List.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Device.List.Response.Error.Code](#anytype-Rpc-Device-List-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Device-NetworkState"></a>

### Rpc.Device.NetworkState







<a name="anytype-Rpc-Device-NetworkState-Set"></a>

### Rpc.Device.NetworkState.Set







<a name="anytype-Rpc-Device-NetworkState-Set-Request"></a>

### Rpc.Device.NetworkState.Set.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| deviceNetworkType | [model.DeviceNetworkType](#anytype-model-DeviceNetworkType) |  |  |






<a name="anytype-Rpc-Device-NetworkState-Set-Response"></a>

### Rpc.Device.NetworkState.Set.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Device.NetworkState.Set.Response.Error](#anytype-Rpc-Device-NetworkState-Set-Response-Error) |  |  |






<a name="anytype-Rpc-Device-NetworkState-Set-Response-Error"></a>

### Rpc.Device.NetworkState.Set.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Device.NetworkState.Set.Response.Error.Code](#anytype-Rpc-Device-NetworkState-Set-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Device-SetName"></a>

### Rpc.Device.SetName







<a name="anytype-Rpc-Device-SetName-Request"></a>

### Rpc.Device.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| deviceId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype-Rpc-Device-SetName-Response"></a>

### Rpc.Device.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Device.SetName.Response.Error](#anytype-Rpc-Device-SetName-Response-Error) |  |  |






<a name="anytype-Rpc-Device-SetName-Response-Error"></a>

### Rpc.Device.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Device.SetName.Response.Error.Code](#anytype-Rpc-Device-SetName-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File"></a>

### Rpc.File







<a name="anytype-Rpc-File-CacheCancelDownload"></a>

### Rpc.File.CacheCancelDownload







<a name="anytype-Rpc-File-CacheCancelDownload-Request"></a>

### Rpc.File.CacheCancelDownload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fileObjectId | [string](#string) |  |  |






<a name="anytype-Rpc-File-CacheCancelDownload-Response"></a>

### Rpc.File.CacheCancelDownload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.CacheCancelDownload.Response.Error](#anytype-Rpc-File-CacheCancelDownload-Response-Error) |  |  |






<a name="anytype-Rpc-File-CacheCancelDownload-Response-Error"></a>

### Rpc.File.CacheCancelDownload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.CacheCancelDownload.Response.Error.Code](#anytype-Rpc-File-CacheCancelDownload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-CacheDownload"></a>

### Rpc.File.CacheDownload







<a name="anytype-Rpc-File-CacheDownload-Request"></a>

### Rpc.File.CacheDownload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fileObjectId | [string](#string) |  |  |






<a name="anytype-Rpc-File-CacheDownload-Response"></a>

### Rpc.File.CacheDownload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.CacheDownload.Response.Error](#anytype-Rpc-File-CacheDownload-Response-Error) |  |  |






<a name="anytype-Rpc-File-CacheDownload-Response-Error"></a>

### Rpc.File.CacheDownload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.CacheDownload.Response.Error.Code](#anytype-Rpc-File-CacheDownload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-DiscardPreload"></a>

### Rpc.File.DiscardPreload







<a name="anytype-Rpc-File-DiscardPreload-Request"></a>

### Rpc.File.DiscardPreload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fileId | [string](#string) |  |  |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-File-DiscardPreload-Response"></a>

### Rpc.File.DiscardPreload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.DiscardPreload.Response.Error](#anytype-Rpc-File-DiscardPreload-Response-Error) |  |  |






<a name="anytype-Rpc-File-DiscardPreload-Response-Error"></a>

### Rpc.File.DiscardPreload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.DiscardPreload.Response.Error.Code](#anytype-Rpc-File-DiscardPreload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-Download"></a>

### Rpc.File.Download







<a name="anytype-Rpc-File-Download-Request"></a>

### Rpc.File.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
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






<a name="anytype-Rpc-File-NodeUsage"></a>

### Rpc.File.NodeUsage







<a name="anytype-Rpc-File-NodeUsage-Request"></a>

### Rpc.File.NodeUsage.Request







<a name="anytype-Rpc-File-NodeUsage-Response"></a>

### Rpc.File.NodeUsage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.NodeUsage.Response.Error](#anytype-Rpc-File-NodeUsage-Response-Error) |  |  |
| usage | [Rpc.File.NodeUsage.Response.Usage](#anytype-Rpc-File-NodeUsage-Response-Usage) |  |  |
| spaces | [Rpc.File.NodeUsage.Response.Space](#anytype-Rpc-File-NodeUsage-Response-Space) | repeated |  |






<a name="anytype-Rpc-File-NodeUsage-Response-Error"></a>

### Rpc.File.NodeUsage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.NodeUsage.Response.Error.Code](#anytype-Rpc-File-NodeUsage-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-NodeUsage-Response-Space"></a>

### Rpc.File.NodeUsage.Response.Space



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| filesCount | [uint64](#uint64) |  |  |
| cidsCount | [uint64](#uint64) |  |  |
| bytesUsage | [uint64](#uint64) |  |  |






<a name="anytype-Rpc-File-NodeUsage-Response-Usage"></a>

### Rpc.File.NodeUsage.Response.Usage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filesCount | [uint64](#uint64) |  |  |
| cidsCount | [uint64](#uint64) |  |  |
| bytesUsage | [uint64](#uint64) |  |  |
| bytesLeft | [uint64](#uint64) |  |  |
| bytesLimit | [uint64](#uint64) |  |  |
| localBytesUsage | [uint64](#uint64) |  |  |






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






<a name="anytype-Rpc-File-Reconcile"></a>

### Rpc.File.Reconcile







<a name="anytype-Rpc-File-Reconcile-Request"></a>

### Rpc.File.Reconcile.Request







<a name="anytype-Rpc-File-Reconcile-Response"></a>

### Rpc.File.Reconcile.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Reconcile.Response.Error](#anytype-Rpc-File-Reconcile-Response-Error) |  |  |






<a name="anytype-Rpc-File-Reconcile-Response-Error"></a>

### Rpc.File.Reconcile.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Reconcile.Response.Error.Code](#anytype-Rpc-File-Reconcile-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-SetAutoDownload"></a>

### Rpc.File.SetAutoDownload







<a name="anytype-Rpc-File-SetAutoDownload-Request"></a>

### Rpc.File.SetAutoDownload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enabled | [bool](#bool) |  |  |
| wifi_only | [bool](#bool) |  |  |






<a name="anytype-Rpc-File-SetAutoDownload-Response"></a>

### Rpc.File.SetAutoDownload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.SetAutoDownload.Response.Error](#anytype-Rpc-File-SetAutoDownload-Response-Error) |  |  |






<a name="anytype-Rpc-File-SetAutoDownload-Response-Error"></a>

### Rpc.File.SetAutoDownload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.SetAutoDownload.Response.Error.Code](#anytype-Rpc-File-SetAutoDownload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-SpaceOffload"></a>

### Rpc.File.SpaceOffload







<a name="anytype-Rpc-File-SpaceOffload-Request"></a>

### Rpc.File.SpaceOffload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-File-SpaceOffload-Response"></a>

### Rpc.File.SpaceOffload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.SpaceOffload.Response.Error](#anytype-Rpc-File-SpaceOffload-Response-Error) |  |  |
| filesOffloaded | [int32](#int32) |  |  |
| bytesOffloaded | [uint64](#uint64) |  |  |






<a name="anytype-Rpc-File-SpaceOffload-Response-Error"></a>

### Rpc.File.SpaceOffload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.SpaceOffload.Response.Error.Code](#anytype-Rpc-File-SpaceOffload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-File-SpaceUsage"></a>

### Rpc.File.SpaceUsage







<a name="anytype-Rpc-File-SpaceUsage-Request"></a>

### Rpc.File.SpaceUsage.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






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
| spaceId | [string](#string) |  |  |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| type | [model.Block.Content.File.Type](#anytype-model-Block-Content-File-Type) |  |  |
| disableEncryption | [bool](#bool) |  | deprecated, has no affect, GO-1926 |
| style | [model.Block.Content.File.Style](#anytype-model-Block-Content-File-Style) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | additional details for file object |
| origin | [model.ObjectOrigin](#anytype-model-ObjectOrigin) |  |  |
| imageKind | [model.ImageKind](#anytype-model-ImageKind) |  |  |
| preloadOnly | [bool](#bool) |  | if true, only async preload the file without creating object |
| preloadFileId | [string](#string) |  | if set, reuse already preloaded file with this id. May block if async preload operation is not finished yet |






<a name="anytype-Rpc-File-Upload-Response"></a>

### Rpc.File.Upload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Upload.Response.Error](#anytype-Rpc-File-Upload-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| preloadFileId | [string](#string) |  | returned when preloadOnly is true, can be passed back in subsequent requests |






<a name="anytype-Rpc-File-Upload-Response-Error"></a>

### Rpc.File.Upload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Upload.Response.Error.Code](#anytype-Rpc-File-Upload-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Gallery"></a>

### Rpc.Gallery







<a name="anytype-Rpc-Gallery-DownloadIndex"></a>

### Rpc.Gallery.DownloadIndex







<a name="anytype-Rpc-Gallery-DownloadIndex-Request"></a>

### Rpc.Gallery.DownloadIndex.Request







<a name="anytype-Rpc-Gallery-DownloadIndex-Response"></a>

### Rpc.Gallery.DownloadIndex.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Gallery.DownloadIndex.Response.Error](#anytype-Rpc-Gallery-DownloadIndex-Response-Error) |  |  |
| categories | [Rpc.Gallery.DownloadIndex.Response.Category](#anytype-Rpc-Gallery-DownloadIndex-Response-Category) | repeated |  |
| experiences | [model.ManifestInfo](#anytype-model-ManifestInfo) | repeated |  |






<a name="anytype-Rpc-Gallery-DownloadIndex-Response-Category"></a>

### Rpc.Gallery.DownloadIndex.Response.Category



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| experiences | [string](#string) | repeated |  |
| icon | [string](#string) |  |  |






<a name="anytype-Rpc-Gallery-DownloadIndex-Response-Error"></a>

### Rpc.Gallery.DownloadIndex.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Gallery.DownloadIndex.Response.Error.Code](#anytype-Rpc-Gallery-DownloadIndex-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Gallery-DownloadManifest"></a>

### Rpc.Gallery.DownloadManifest







<a name="anytype-Rpc-Gallery-DownloadManifest-Request"></a>

### Rpc.Gallery.DownloadManifest.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-Gallery-DownloadManifest-Response"></a>

### Rpc.Gallery.DownloadManifest.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Gallery.DownloadManifest.Response.Error](#anytype-Rpc-Gallery-DownloadManifest-Response-Error) |  |  |
| info | [model.ManifestInfo](#anytype-model-ManifestInfo) |  |  |






<a name="anytype-Rpc-Gallery-DownloadManifest-Response-Error"></a>

### Rpc.Gallery.DownloadManifest.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Gallery.DownloadManifest.Response.Error.Code](#anytype-Rpc-Gallery-DownloadManifest-Response-Error-Code) |  |  |
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







<a name="anytype-Rpc-History-DiffVersions"></a>

### Rpc.History.DiffVersions







<a name="anytype-Rpc-History-DiffVersions-Request"></a>

### Rpc.History.DiffVersions.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| spaceId | [string](#string) |  |  |
| currentVersion | [string](#string) |  |  |
| previousVersion | [string](#string) |  |  |






<a name="anytype-Rpc-History-DiffVersions-Response"></a>

### Rpc.History.DiffVersions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.DiffVersions.Response.Error](#anytype-Rpc-History-DiffVersions-Response-Error) |  |  |
| historyEvents | [Event.Message](#anytype-Event-Message) | repeated |  |
| objectView | [model.ObjectView](#anytype-model-ObjectView) |  |  |






<a name="anytype-Rpc-History-DiffVersions-Response-Error"></a>

### Rpc.History.DiffVersions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.DiffVersions.Response.Error.Code](#anytype-Rpc-History-DiffVersions-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






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
| notIncludeVersion | [bool](#bool) |  |  |






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






<a name="anytype-Rpc-Initial"></a>

### Rpc.Initial







<a name="anytype-Rpc-Initial-SetParameters"></a>

### Rpc.Initial.SetParameters







<a name="anytype-Rpc-Initial-SetParameters-Request"></a>

### Rpc.Initial.SetParameters.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| platform | [string](#string) |  |  |
| version | [string](#string) |  |  |
| workdir | [string](#string) |  |  |
| logLevel | [string](#string) |  |  |
| doNotSendLogs | [bool](#bool) |  |  |
| doNotSaveLogs | [bool](#bool) |  |  |
| doNotSendTelemetry | [bool](#bool) |  |  |






<a name="anytype-Rpc-Initial-SetParameters-Response"></a>

### Rpc.Initial.SetParameters.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Initial.SetParameters.Response.Error](#anytype-Rpc-Initial-SetParameters-Response-Error) |  |  |






<a name="anytype-Rpc-Initial-SetParameters-Response-Error"></a>

### Rpc.Initial.SetParameters.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Initial.SetParameters.Response.Error.Code](#anytype-Rpc-Initial-SetParameters-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






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






<a name="anytype-Rpc-Membership"></a>

### Rpc.Membership
A Membership is a bundle of several &#34;Features&#34;
every user should have one and only one tier
users can not have N tiers (no combining)






<a name="anytype-Rpc-Membership-CodeGetInfo"></a>

### Rpc.Membership.CodeGetInfo







<a name="anytype-Rpc-Membership-CodeGetInfo-Request"></a>

### Rpc.Membership.CodeGetInfo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-CodeGetInfo-Response"></a>

### Rpc.Membership.CodeGetInfo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.CodeGetInfo.Response.Error](#anytype-Rpc-Membership-CodeGetInfo-Response-Error) |  |  |
| requestedTier | [uint32](#uint32) |  | which tier current code can unlock |






<a name="anytype-Rpc-Membership-CodeGetInfo-Response-Error"></a>

### Rpc.Membership.CodeGetInfo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.CodeGetInfo.Response.Error.Code](#anytype-Rpc-Membership-CodeGetInfo-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-CodeRedeem"></a>

### Rpc.Membership.CodeRedeem







<a name="anytype-Rpc-Membership-CodeRedeem-Request"></a>

### Rpc.Membership.CodeRedeem.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [string](#string) |  |  |
| nsName | [string](#string) |  |  |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-Membership-CodeRedeem-Response"></a>

### Rpc.Membership.CodeRedeem.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.CodeRedeem.Response.Error](#anytype-Rpc-Membership-CodeRedeem-Response-Error) |  |  |
| requestedTier | [uint32](#uint32) |  | which tier does the current code unlock |






<a name="anytype-Rpc-Membership-CodeRedeem-Response-Error"></a>

### Rpc.Membership.CodeRedeem.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.CodeRedeem.Response.Error.Code](#anytype-Rpc-Membership-CodeRedeem-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-Finalize"></a>

### Rpc.Membership.Finalize







<a name="anytype-Rpc-Membership-Finalize-Request"></a>

### Rpc.Membership.Finalize.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nsName | [string](#string) |  | if empty - then no name requested if non-empty - PP node will register that name on behalf of the user |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-Membership-Finalize-Response"></a>

### Rpc.Membership.Finalize.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.Finalize.Response.Error](#anytype-Rpc-Membership-Finalize-Response-Error) |  |  |






<a name="anytype-Rpc-Membership-Finalize-Response-Error"></a>

### Rpc.Membership.Finalize.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.Finalize.Response.Error.Code](#anytype-Rpc-Membership-Finalize-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetPortalLinkUrl"></a>

### Rpc.Membership.GetPortalLinkUrl
Generate a link to the portal where user can:
a) change his billing details
b) see payment info, invoices, etc
c) cancel membership






<a name="anytype-Rpc-Membership-GetPortalLinkUrl-Request"></a>

### Rpc.Membership.GetPortalLinkUrl.Request







<a name="anytype-Rpc-Membership-GetPortalLinkUrl-Response"></a>

### Rpc.Membership.GetPortalLinkUrl.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.GetPortalLinkUrl.Response.Error](#anytype-Rpc-Membership-GetPortalLinkUrl-Response-Error) |  |  |
| portalUrl | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetPortalLinkUrl-Response-Error"></a>

### Rpc.Membership.GetPortalLinkUrl.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.GetPortalLinkUrl.Response.Error.Code](#anytype-Rpc-Membership-GetPortalLinkUrl-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetStatus"></a>

### Rpc.Membership.GetStatus
Get the current status of the membership
including the tier, status, dates, etc
WARNING: this can be cached by Anytype heart






<a name="anytype-Rpc-Membership-GetStatus-Request"></a>

### Rpc.Membership.GetStatus.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| noCache | [bool](#bool) |  | pass true to force the cache update by default this is false |






<a name="anytype-Rpc-Membership-GetStatus-Response"></a>

### Rpc.Membership.GetStatus.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.GetStatus.Response.Error](#anytype-Rpc-Membership-GetStatus-Response-Error) |  |  |
| data | [model.Membership](#anytype-model-Membership) |  |  |






<a name="anytype-Rpc-Membership-GetStatus-Response-Error"></a>

### Rpc.Membership.GetStatus.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.GetStatus.Response.Error.Code](#anytype-Rpc-Membership-GetStatus-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetTiers"></a>

### Rpc.Membership.GetTiers
Tiers can change on the backend so if you want to show users the latest data
you can call this method to get the latest tiers






<a name="anytype-Rpc-Membership-GetTiers-Request"></a>

### Rpc.Membership.GetTiers.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| noCache | [bool](#bool) |  | pass true to force the cache update by default this is false |
| locale | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetTiers-Response"></a>

### Rpc.Membership.GetTiers.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.GetTiers.Response.Error](#anytype-Rpc-Membership-GetTiers-Response-Error) |  |  |
| tiers | [model.MembershipTierData](#anytype-model-MembershipTierData) | repeated |  |






<a name="anytype-Rpc-Membership-GetTiers-Response-Error"></a>

### Rpc.Membership.GetTiers.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.GetTiers.Response.Error.Code](#anytype-Rpc-Membership-GetTiers-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetVerificationEmail"></a>

### Rpc.Membership.GetVerificationEmail
Send an e-mail with verification code to the user
can be called multiple times but with some timeout (N seconds) between calls






<a name="anytype-Rpc-Membership-GetVerificationEmail-Request"></a>

### Rpc.Membership.GetVerificationEmail.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| email | [string](#string) |  |  |
| subscribeToNewsletter | [bool](#bool) |  |  |
| insiderTipsAndTutorials | [bool](#bool) |  |  |
| isOnboardingList | [bool](#bool) |  | if we are coming from the onboarding list |






<a name="anytype-Rpc-Membership-GetVerificationEmail-Response"></a>

### Rpc.Membership.GetVerificationEmail.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.GetVerificationEmail.Response.Error](#anytype-Rpc-Membership-GetVerificationEmail-Response-Error) |  |  |






<a name="anytype-Rpc-Membership-GetVerificationEmail-Response-Error"></a>

### Rpc.Membership.GetVerificationEmail.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.GetVerificationEmail.Response.Error.Code](#anytype-Rpc-Membership-GetVerificationEmail-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-GetVerificationEmailStatus"></a>

### Rpc.Membership.GetVerificationEmailStatus
Get the current status of the e-mail verification.
Status can change if you call GetVerificationEmail or VerifyEmailCode






<a name="anytype-Rpc-Membership-GetVerificationEmailStatus-Request"></a>

### Rpc.Membership.GetVerificationEmailStatus.Request







<a name="anytype-Rpc-Membership-GetVerificationEmailStatus-Response"></a>

### Rpc.Membership.GetVerificationEmailStatus.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.GetVerificationEmailStatus.Response.Error](#anytype-Rpc-Membership-GetVerificationEmailStatus-Response-Error) |  |  |
| status | [model.Membership.EmailVerificationStatus](#anytype-model-Membership-EmailVerificationStatus) |  |  |






<a name="anytype-Rpc-Membership-GetVerificationEmailStatus-Response-Error"></a>

### Rpc.Membership.GetVerificationEmailStatus.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.GetVerificationEmailStatus.Response.Error.Code](#anytype-Rpc-Membership-GetVerificationEmailStatus-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-IsNameValid"></a>

### Rpc.Membership.IsNameValid
Check if the requested name is valid and vacant for the requested tier
before requesting a payment link and paying






<a name="anytype-Rpc-Membership-IsNameValid-Request"></a>

### Rpc.Membership.IsNameValid.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| requestedTier | [uint32](#uint32) |  |  |
| nsName | [string](#string) |  |  |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-Membership-IsNameValid-Response"></a>

### Rpc.Membership.IsNameValid.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.IsNameValid.Response.Error](#anytype-Rpc-Membership-IsNameValid-Response-Error) |  |  |






<a name="anytype-Rpc-Membership-IsNameValid-Response-Error"></a>

### Rpc.Membership.IsNameValid.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.IsNameValid.Response.Error.Code](#anytype-Rpc-Membership-IsNameValid-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-RegisterPaymentRequest"></a>

### Rpc.Membership.RegisterPaymentRequest
Generate a unique id for payment request (for mobile clients)
Generate a link to Stripe/Crypto where user can pay for the membership (for desktop client)






<a name="anytype-Rpc-Membership-RegisterPaymentRequest-Request"></a>

### Rpc.Membership.RegisterPaymentRequest.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| requestedTier | [uint32](#uint32) |  |  |
| paymentMethod | [model.Membership.PaymentMethod](#anytype-model-Membership-PaymentMethod) |  |  |
| nsName | [string](#string) |  | if empty - then no name requested if non-empty - PP node will register that name on behalf of the user |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |
| userEmail | [string](#string) |  | for some tiers and payment methods (like crypto) we need an e-mail please get if either from: 1. Membership.GetStatus() -&gt; anytype.model.Membership.userEmail field 2. Ask user from the UI |






<a name="anytype-Rpc-Membership-RegisterPaymentRequest-Response"></a>

### Rpc.Membership.RegisterPaymentRequest.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.RegisterPaymentRequest.Response.Error](#anytype-Rpc-Membership-RegisterPaymentRequest-Response-Error) |  |  |
| paymentUrl | [string](#string) |  | will feature current billing ID stripe.com/?client_reference_id=1234 |
| billingId | [string](#string) |  | billingID is only needed for mobile clients |






<a name="anytype-Rpc-Membership-RegisterPaymentRequest-Response-Error"></a>

### Rpc.Membership.RegisterPaymentRequest.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.RegisterPaymentRequest.Response.Error.Code](#anytype-Rpc-Membership-RegisterPaymentRequest-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-VerifyAppStoreReceipt"></a>

### Rpc.Membership.VerifyAppStoreReceipt







<a name="anytype-Rpc-Membership-VerifyAppStoreReceipt-Request"></a>

### Rpc.Membership.VerifyAppStoreReceipt.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| receipt | [string](#string) |  | receipt is a JWT-encoded string including info about subscription purchase |






<a name="anytype-Rpc-Membership-VerifyAppStoreReceipt-Response"></a>

### Rpc.Membership.VerifyAppStoreReceipt.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.VerifyAppStoreReceipt.Response.Error](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Response-Error) |  |  |






<a name="anytype-Rpc-Membership-VerifyAppStoreReceipt-Response-Error"></a>

### Rpc.Membership.VerifyAppStoreReceipt.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.VerifyAppStoreReceipt.Response.Error.Code](#anytype-Rpc-Membership-VerifyAppStoreReceipt-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-VerifyEmailCode"></a>

### Rpc.Membership.VerifyEmailCode
Verify the e-mail address of the user
need a correct code that was sent to the user when calling GetVerificationEmail






<a name="anytype-Rpc-Membership-VerifyEmailCode-Request"></a>

### Rpc.Membership.VerifyEmailCode.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [string](#string) |  |  |






<a name="anytype-Rpc-Membership-VerifyEmailCode-Response"></a>

### Rpc.Membership.VerifyEmailCode.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Membership.VerifyEmailCode.Response.Error](#anytype-Rpc-Membership-VerifyEmailCode-Response-Error) |  |  |






<a name="anytype-Rpc-Membership-VerifyEmailCode-Response-Error"></a>

### Rpc.Membership.VerifyEmailCode.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Membership.VerifyEmailCode.Response.Error.Code](#anytype-Rpc-Membership-VerifyEmailCode-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2"></a>

### Rpc.MembershipV2







<a name="anytype-Rpc-MembershipV2-AnyNameAllocate"></a>

### Rpc.MembershipV2.AnyNameAllocate







<a name="anytype-Rpc-MembershipV2-AnyNameAllocate-Request"></a>

### Rpc.MembershipV2.AnyNameAllocate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nsName | [string](#string) |  | PP node will register that name on behalf of the user |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-MembershipV2-AnyNameAllocate-Response"></a>

### Rpc.MembershipV2.AnyNameAllocate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.AnyNameAllocate.Response.Error](#anytype-Rpc-MembershipV2-AnyNameAllocate-Response-Error) |  |  |






<a name="anytype-Rpc-MembershipV2-AnyNameAllocate-Response-Error"></a>

### Rpc.MembershipV2.AnyNameAllocate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.AnyNameAllocate.Response.Error.Code](#anytype-Rpc-MembershipV2-AnyNameAllocate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-AnyNameIsValid"></a>

### Rpc.MembershipV2.AnyNameIsValid
Check if the requested name is valid and vacant for the requested tier
before requesting a payment link and paying






<a name="anytype-Rpc-MembershipV2-AnyNameIsValid-Request"></a>

### Rpc.MembershipV2.AnyNameIsValid.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nsName | [string](#string) |  |  |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-MembershipV2-AnyNameIsValid-Response"></a>

### Rpc.MembershipV2.AnyNameIsValid.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.AnyNameIsValid.Response.Error](#anytype-Rpc-MembershipV2-AnyNameIsValid-Response-Error) |  |  |






<a name="anytype-Rpc-MembershipV2-AnyNameIsValid-Response-Error"></a>

### Rpc.MembershipV2.AnyNameIsValid.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.AnyNameIsValid.Response.Error.Code](#anytype-Rpc-MembershipV2-AnyNameIsValid-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-CartGet"></a>

### Rpc.MembershipV2.CartGet







<a name="anytype-Rpc-MembershipV2-CartGet-Request"></a>

### Rpc.MembershipV2.CartGet.Request







<a name="anytype-Rpc-MembershipV2-CartGet-Response"></a>

### Rpc.MembershipV2.CartGet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.CartGet.Response.Error](#anytype-Rpc-MembershipV2-CartGet-Response-Error) |  |  |
| cart | [model.MembershipV2.Cart](#anytype-model-MembershipV2-Cart) |  |  |






<a name="anytype-Rpc-MembershipV2-CartGet-Response-Error"></a>

### Rpc.MembershipV2.CartGet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.CartGet.Response.Error.Code](#anytype-Rpc-MembershipV2-CartGet-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-CartUpdate"></a>

### Rpc.MembershipV2.CartUpdate







<a name="anytype-Rpc-MembershipV2-CartUpdate-Request"></a>

### Rpc.MembershipV2.CartUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| productIds | [string](#string) | repeated |  |
| isYearly | [bool](#bool) |  |  |






<a name="anytype-Rpc-MembershipV2-CartUpdate-Response"></a>

### Rpc.MembershipV2.CartUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.CartUpdate.Response.Error](#anytype-Rpc-MembershipV2-CartUpdate-Response-Error) |  |  |
| cart | [model.MembershipV2.Cart](#anytype-model-MembershipV2-Cart) |  |  |






<a name="anytype-Rpc-MembershipV2-CartUpdate-Response-Error"></a>

### Rpc.MembershipV2.CartUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.CartUpdate.Response.Error.Code](#anytype-Rpc-MembershipV2-CartUpdate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-GetPortalLink"></a>

### Rpc.MembershipV2.GetPortalLink







<a name="anytype-Rpc-MembershipV2-GetPortalLink-Request"></a>

### Rpc.MembershipV2.GetPortalLink.Request







<a name="anytype-Rpc-MembershipV2-GetPortalLink-Response"></a>

### Rpc.MembershipV2.GetPortalLink.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.GetPortalLink.Response.Error](#anytype-Rpc-MembershipV2-GetPortalLink-Response-Error) |  |  |
| url | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-GetPortalLink-Response-Error"></a>

### Rpc.MembershipV2.GetPortalLink.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.GetPortalLink.Response.Error.Code](#anytype-Rpc-MembershipV2-GetPortalLink-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-GetProducts"></a>

### Rpc.MembershipV2.GetProducts







<a name="anytype-Rpc-MembershipV2-GetProducts-Request"></a>

### Rpc.MembershipV2.GetProducts.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| noCache | [bool](#bool) |  | pass true to force the cache update by default this is false |






<a name="anytype-Rpc-MembershipV2-GetProducts-Response"></a>

### Rpc.MembershipV2.GetProducts.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.GetProducts.Response.Error](#anytype-Rpc-MembershipV2-GetProducts-Response-Error) |  |  |
| products | [model.MembershipV2.Product](#anytype-model-MembershipV2-Product) | repeated |  |






<a name="anytype-Rpc-MembershipV2-GetProducts-Response-Error"></a>

### Rpc.MembershipV2.GetProducts.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.GetProducts.Response.Error.Code](#anytype-Rpc-MembershipV2-GetProducts-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-MembershipV2-GetStatus"></a>

### Rpc.MembershipV2.GetStatus







<a name="anytype-Rpc-MembershipV2-GetStatus-Request"></a>

### Rpc.MembershipV2.GetStatus.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| noCache | [bool](#bool) |  | pass true to force the cache update by default this is false |






<a name="anytype-Rpc-MembershipV2-GetStatus-Response"></a>

### Rpc.MembershipV2.GetStatus.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MembershipV2.GetStatus.Response.Error](#anytype-Rpc-MembershipV2-GetStatus-Response-Error) |  |  |
| data | [model.MembershipV2.Data](#anytype-model-MembershipV2-Data) |  |  |






<a name="anytype-Rpc-MembershipV2-GetStatus-Response-Error"></a>

### Rpc.MembershipV2.GetStatus.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MembershipV2.GetStatus.Response.Error.Code](#anytype-Rpc-MembershipV2-GetStatus-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-NameService"></a>

### Rpc.NameService







<a name="anytype-Rpc-NameService-ResolveAnyId"></a>

### Rpc.NameService.ResolveAnyId







<a name="anytype-Rpc-NameService-ResolveAnyId-Request"></a>

### Rpc.NameService.ResolveAnyId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| anyId | [string](#string) |  |  |






<a name="anytype-Rpc-NameService-ResolveAnyId-Response"></a>

### Rpc.NameService.ResolveAnyId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.NameService.ResolveAnyId.Response.Error](#anytype-Rpc-NameService-ResolveAnyId-Response-Error) |  |  |
| found | [bool](#bool) |  |  |
| nsName | [string](#string) |  | not including suffix |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-NameService-ResolveAnyId-Response-Error"></a>

### Rpc.NameService.ResolveAnyId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.NameService.ResolveAnyId.Response.Error.Code](#anytype-Rpc-NameService-ResolveAnyId-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-NameService-ResolveName"></a>

### Rpc.NameService.ResolveName







<a name="anytype-Rpc-NameService-ResolveName-Request"></a>

### Rpc.NameService.ResolveName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| nsName | [string](#string) |  |  |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-NameService-ResolveName-Response"></a>

### Rpc.NameService.ResolveName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.NameService.ResolveName.Response.Error](#anytype-Rpc-NameService-ResolveName-Response-Error) |  |  |
| available | [bool](#bool) |  |  |
| ownerScwEthAddress | [string](#string) |  | EOA -&gt; SCW -&gt; name This field is non-empty only if name is &#34;already registered&#34; |
| ownerEthAddress | [string](#string) |  | This field is non-empty only if name is &#34;already registered&#34; |
| ownerAnyAddress | [string](#string) |  | A content hash attached to this name This field is non-empty only if name is &#34;already registered&#34; |
| spaceId | [string](#string) |  | A SpaceId attached to this name This field is non-empty only if name is &#34;already registered&#34; |
| nameExpires | [int64](#int64) |  | A timestamp when this name expires |






<a name="anytype-Rpc-NameService-ResolveName-Response-Error"></a>

### Rpc.NameService.ResolveName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.NameService.ResolveName.Response.Error.Code](#anytype-Rpc-NameService-ResolveName-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-NameService-ResolveSpaceId"></a>

### Rpc.NameService.ResolveSpaceId







<a name="anytype-Rpc-NameService-ResolveSpaceId-Request"></a>

### Rpc.NameService.ResolveSpaceId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-NameService-ResolveSpaceId-Response"></a>

### Rpc.NameService.ResolveSpaceId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.NameService.ResolveSpaceId.Response.Error](#anytype-Rpc-NameService-ResolveSpaceId-Response-Error) |  |  |
| found | [bool](#bool) |  |  |
| nsName | [string](#string) |  | not including suffix |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |






<a name="anytype-Rpc-NameService-ResolveSpaceId-Response-Error"></a>

### Rpc.NameService.ResolveSpaceId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.NameService.ResolveSpaceId.Response.Error.Code](#anytype-Rpc-NameService-ResolveSpaceId-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-NameService-UserAccount"></a>

### Rpc.NameService.UserAccount







<a name="anytype-Rpc-NameService-UserAccount-Get"></a>

### Rpc.NameService.UserAccount.Get







<a name="anytype-Rpc-NameService-UserAccount-Get-Request"></a>

### Rpc.NameService.UserAccount.Get.Request







<a name="anytype-Rpc-NameService-UserAccount-Get-Response"></a>

### Rpc.NameService.UserAccount.Get.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.NameService.UserAccount.Get.Response.Error](#anytype-Rpc-NameService-UserAccount-Get-Response-Error) |  |  |
| nsNameAttached | [string](#string) |  | this will use ReverseResolve to get current name user can buy many names, but only 1 name can be set as &#34;current&#34;: ETH address &lt;-&gt; name |
| nsNameType | [model.NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |
| namesCountLeft | [uint64](#uint64) |  | Number of names that the user can reserve |
| operationsCountLeft | [uint64](#uint64) |  | Number of operations: update name, add new data, etc |






<a name="anytype-Rpc-NameService-UserAccount-Get-Response-Error"></a>

### Rpc.NameService.UserAccount.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.NameService.UserAccount.Get.Response.Error.Code](#anytype-Rpc-NameService-UserAccount-Get-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Notification"></a>

### Rpc.Notification







<a name="anytype-Rpc-Notification-List"></a>

### Rpc.Notification.List







<a name="anytype-Rpc-Notification-List-Request"></a>

### Rpc.Notification.List.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| includeRead | [bool](#bool) |  |  |
| limit | [int64](#int64) |  |  |






<a name="anytype-Rpc-Notification-List-Response"></a>

### Rpc.Notification.List.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Notification.List.Response.Error](#anytype-Rpc-Notification-List-Response-Error) |  |  |
| notifications | [model.Notification](#anytype-model-Notification) | repeated |  |






<a name="anytype-Rpc-Notification-List-Response-Error"></a>

### Rpc.Notification.List.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Notification.List.Response.Error.Code](#anytype-Rpc-Notification-List-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Notification-Reply"></a>

### Rpc.Notification.Reply







<a name="anytype-Rpc-Notification-Reply-Request"></a>

### Rpc.Notification.Reply.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |
| actionType | [model.Notification.ActionType](#anytype-model-Notification-ActionType) |  |  |






<a name="anytype-Rpc-Notification-Reply-Response"></a>

### Rpc.Notification.Reply.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Notification.Reply.Response.Error](#anytype-Rpc-Notification-Reply-Response-Error) |  |  |






<a name="anytype-Rpc-Notification-Reply-Response-Error"></a>

### Rpc.Notification.Reply.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Notification.Reply.Response.Error.Code](#anytype-Rpc-Notification-Reply-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Notification-Test"></a>

### Rpc.Notification.Test







<a name="anytype-Rpc-Notification-Test-Request"></a>

### Rpc.Notification.Test.Request







<a name="anytype-Rpc-Notification-Test-Response"></a>

### Rpc.Notification.Test.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Notification.Test.Response.Error](#anytype-Rpc-Notification-Test-Response-Error) |  |  |
| notification | [model.Notification](#anytype-model-Notification) |  |  |






<a name="anytype-Rpc-Notification-Test-Response-Error"></a>

### Rpc.Notification.Test.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Notification.Test.Response.Error.Code](#anytype-Rpc-Notification-Test-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Object-ChatAdd"></a>

### Rpc.Object.ChatAdd







<a name="anytype-Rpc-Object-ChatAdd-Request"></a>

### Rpc.Object.ChatAdd.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ChatAdd-Response"></a>

### Rpc.Object.ChatAdd.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ChatAdd.Response.Error](#anytype-Rpc-Object-ChatAdd-Response-Error) |  |  |
| chatId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ChatAdd-Response-Error"></a>

### Rpc.Object.ChatAdd.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ChatAdd.Response.Error.Code](#anytype-Rpc-Object-ChatAdd-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Close"></a>

### Rpc.Object.Close







<a name="anytype-Rpc-Object-Close-Request"></a>

### Rpc.Object.Close.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | deprecated, GO-1926 |
| objectId | [string](#string) |  |  |
| spaceId | [string](#string) |  | Required only for date objects |






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
| spaceId | [string](#string) |  |  |
| objectTypeUniqueKey | [string](#string) |  |  |
| withChat | [bool](#bool) |  |  |






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
| spaceId | [string](#string) |  |  |
| withChat | [bool](#bool) |  |  |
| templateId | [string](#string) |  |  |






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






<a name="anytype-Rpc-Object-CreateFromUrl"></a>

### Rpc.Object.CreateFromUrl







<a name="anytype-Rpc-Object-CreateFromUrl-Request"></a>

### Rpc.Object.CreateFromUrl.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| objectTypeUniqueKey | [string](#string) |  |  |
| url | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| addPageContent | [bool](#bool) |  |  |
| withChat | [bool](#bool) |  |  |
| templateId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateFromUrl-Response"></a>

### Rpc.Object.CreateFromUrl.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateFromUrl.Response.Error](#anytype-Rpc-Object-CreateFromUrl-Response-Error) |  |  |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| chatId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateFromUrl-Response-Error"></a>

### Rpc.Object.CreateFromUrl.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateFromUrl.Response.Error.Code](#anytype-Rpc-Object-CreateFromUrl-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CreateObjectType"></a>

### Rpc.Object.CreateObjectType







<a name="anytype-Rpc-Object-CreateObjectType-Request"></a>

### Rpc.Object.CreateObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| internalFlags | [model.InternalFlag](#anytype-model-InternalFlag) | repeated |  |
| spaceId | [string](#string) |  |  |






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
| spaceId | [string](#string) |  |  |






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
| spaceId | [string](#string) |  |  |






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
| spaceId | [string](#string) |  |  |
| withChat | [bool](#bool) |  |  |






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






<a name="anytype-Rpc-Object-CrossSpaceSearchSubscribe"></a>

### Rpc.Object.CrossSpaceSearchSubscribe







<a name="anytype-Rpc-Object-CrossSpaceSearchSubscribe-Request"></a>

### Rpc.Object.CrossSpaceSearchSubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated | filters |
| sorts | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated | sorts |
| keys | [string](#string) | repeated | (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| source | [string](#string) | repeated |  |
| noDepSubscription | [bool](#bool) |  | disable dependent subscription |
| collectionId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response"></a>

### Rpc.Object.CrossSpaceSearchSubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CrossSpaceSearchSubscribe.Response.Error](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response-Error) |  |  |
| records | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| dependencies | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |
| subId | [string](#string) |  |  |
| counters | [Event.Object.Subscription.Counters](#anytype-Event-Object-Subscription-Counters) |  |  |






<a name="anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response-Error"></a>

### Rpc.Object.CrossSpaceSearchSubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CrossSpaceSearchSubscribe.Response.Error.Code](#anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CrossSpaceSearchUnsubscribe"></a>

### Rpc.Object.CrossSpaceSearchUnsubscribe







<a name="anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Request"></a>

### Rpc.Object.CrossSpaceSearchUnsubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response"></a>

### Rpc.Object.CrossSpaceSearchUnsubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CrossSpaceSearchUnsubscribe.Response.Error](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response-Error) |  |  |






<a name="anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response-Error"></a>

### Rpc.Object.CrossSpaceSearchUnsubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CrossSpaceSearchUnsubscribe.Response.Error.Code](#anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-DateByTimestamp"></a>

### Rpc.Object.DateByTimestamp







<a name="anytype-Rpc-Object-DateByTimestamp-Request"></a>

### Rpc.Object.DateByTimestamp.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| timestamp | [int64](#int64) |  |  |






<a name="anytype-Rpc-Object-DateByTimestamp-Response"></a>

### Rpc.Object.DateByTimestamp.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.DateByTimestamp.Response.Error](#anytype-Rpc-Object-DateByTimestamp-Response-Error) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Object-DateByTimestamp-Response-Error"></a>

### Rpc.Object.DateByTimestamp.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.DateByTimestamp.Response.Error.Code](#anytype-Rpc-Object-DateByTimestamp-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Object-Export"></a>

### Rpc.Object.Export







<a name="anytype-Rpc-Object-Export-Request"></a>

### Rpc.Object.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| objectId | [string](#string) |  | ids of documents for export, when empty - will export all available docs |
| format | [model.Export.Format](#anytype-model-Export-Format) |  | export format |






<a name="anytype-Rpc-Object-Export-Response"></a>

### Rpc.Object.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Export.Response.Error](#anytype-Rpc-Object-Export-Response-Error) |  |  |
| result | [string](#string) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-Export-Response-Error"></a>

### Rpc.Object.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Export.Response.Error.Code](#anytype-Rpc-Object-Export-Response-Error-Code) |  |  |
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

DEPRECATED, GO-1926 |
| keys | [string](#string) | repeated |  |
| spaceId | [string](#string) |  |  |
| collectionId | [string](#string) |  |  |
| setSource | [string](#string) | repeated |  |
| includeTypeEdges | [bool](#bool) |  |  |






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
| spaceId | [string](#string) |  |  |
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
| spaceId | [string](#string) |  |  |
| notionParams | [Rpc.Object.Import.Request.NotionParams](#anytype-Rpc-Object-Import-Request-NotionParams) |  |  |
| bookmarksParams | [Rpc.Object.Import.Request.BookmarksParams](#anytype-Rpc-Object-Import-Request-BookmarksParams) |  | for internal use |
| markdownParams | [Rpc.Object.Import.Request.MarkdownParams](#anytype-Rpc-Object-Import-Request-MarkdownParams) |  |  |
| htmlParams | [Rpc.Object.Import.Request.HtmlParams](#anytype-Rpc-Object-Import-Request-HtmlParams) |  |  |
| txtParams | [Rpc.Object.Import.Request.TxtParams](#anytype-Rpc-Object-Import-Request-TxtParams) |  |  |
| pbParams | [Rpc.Object.Import.Request.PbParams](#anytype-Rpc-Object-Import-Request-PbParams) |  |  |
| csvParams | [Rpc.Object.Import.Request.CsvParams](#anytype-Rpc-Object-Import-Request-CsvParams) |  |  |
| snapshots | [Rpc.Object.Import.Request.Snapshot](#anytype-Rpc-Object-Import-Request-Snapshot) | repeated | optional, for external developers usage |
| updateExistingObjects | [bool](#bool) |  |  |
| type | [model.Import.Type](#anytype-model-Import-Type) |  |  |
| mode | [Rpc.Object.Import.Request.Mode](#anytype-Rpc-Object-Import-Request-Mode) |  |  |
| noProgress | [bool](#bool) |  |  |
| isMigration | [bool](#bool) |  |  |
| isNewSpace | [bool](#bool) |  |  |






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
| createDirectoryPages | [bool](#bool) |  |  |
| includePropertiesAsBlock | [bool](#bool) |  |  |
| noCollection | [bool](#bool) |  |  |






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
| collectionTitle | [string](#string) |  |  |
| importType | [Rpc.Object.Import.Request.PbParams.Type](#anytype-Rpc-Object-Import-Request-PbParams-Type) |  |  |






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
| error | [Rpc.Object.Import.Response.Error](#anytype-Rpc-Object-Import-Response-Error) |  | deprecated |
| collectionId | [string](#string) |  | deprecated |
| objectsCount | [int64](#int64) |  | deprecated |






<a name="anytype-Rpc-Object-Import-Response-Error"></a>

### Rpc.Object.Import.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Import.Response.Error.Code](#anytype-Rpc-Object-Import-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ImportExperience"></a>

### Rpc.Object.ImportExperience







<a name="anytype-Rpc-Object-ImportExperience-Request"></a>

### Rpc.Object.ImportExperience.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| url | [string](#string) |  |  |
| title | [string](#string) |  |  |
| isNewSpace | [bool](#bool) |  |  |
| isAi | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-ImportExperience-Response"></a>

### Rpc.Object.ImportExperience.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ImportExperience.Response.Error](#anytype-Rpc-Object-ImportExperience-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-ImportExperience-Response-Error"></a>

### Rpc.Object.ImportExperience.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ImportExperience.Response.Error.Code](#anytype-Rpc-Object-ImportExperience-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Object-ImportUseCase"></a>

### Rpc.Object.ImportUseCase







<a name="anytype-Rpc-Object-ImportUseCase-Request"></a>

### Rpc.Object.ImportUseCase.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| useCase | [Rpc.Object.ImportUseCase.Request.UseCase](#anytype-Rpc-Object-ImportUseCase-Request-UseCase) |  |  |






<a name="anytype-Rpc-Object-ImportUseCase-Response"></a>

### Rpc.Object.ImportUseCase.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ImportUseCase.Response.Error](#anytype-Rpc-Object-ImportUseCase-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |
| startingObjectId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ImportUseCase-Response-Error"></a>

### Rpc.Object.ImportUseCase.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ImportUseCase.Response.Error.Code](#anytype-Rpc-Object-ImportUseCase-Response-Error-Code) |  |  |
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







<a name="anytype-Rpc-Object-ListExport-RelationsWhiteList"></a>

### Rpc.Object.ListExport.RelationsWhiteList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| layout | [model.ObjectType.Layout](#anytype-model-ObjectType-Layout) |  |  |
| allowedRelations | [string](#string) | repeated |  |






<a name="anytype-Rpc-Object-ListExport-Request"></a>

### Rpc.Object.ListExport.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| path | [string](#string) |  | the path where export files will place |
| objectIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |
| format | [model.Export.Format](#anytype-model-Export-Format) |  | export format |
| zip | [bool](#bool) |  | save as zip file |
| includeNested | [bool](#bool) |  | include all nested |
| includeFiles | [bool](#bool) |  | include all files |
| isJson | [bool](#bool) |  | for protobuf export |
| includeArchived | [bool](#bool) |  | for migration |
| noProgress | [bool](#bool) |  | for integrations like raycast and web publishing |
| linksStateFilters | [Rpc.Object.ListExport.StateFilters](#anytype-Rpc-Object-ListExport-StateFilters) |  |  |
| includeBacklinks | [bool](#bool) |  |  |
| includeSpace | [bool](#bool) |  |  |
| mdIncludePropertiesAndSchema | [bool](#bool) |  | include properties frontmatter and schema in directory for markdown export |






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






<a name="anytype-Rpc-Object-ListExport-StateFilters"></a>

### Rpc.Object.ListExport.StateFilters



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relationsWhiteList | [Rpc.Object.ListExport.RelationsWhiteList](#anytype-Rpc-Object-ListExport-RelationsWhiteList) | repeated |  |
| removeBlocks | [bool](#bool) |  |  |






<a name="anytype-Rpc-Object-ListModifyDetailValues"></a>

### Rpc.Object.ListModifyDetailValues







<a name="anytype-Rpc-Object-ListModifyDetailValues-Request"></a>

### Rpc.Object.ListModifyDetailValues.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| operations | [Rpc.Object.ListModifyDetailValues.Request.Operation](#anytype-Rpc-Object-ListModifyDetailValues-Request-Operation) | repeated |  |






<a name="anytype-Rpc-Object-ListModifyDetailValues-Request-Operation"></a>

### Rpc.Object.ListModifyDetailValues.Request.Operation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relationKey | [string](#string) |  |  |
| add | [google.protobuf.Value](#google-protobuf-Value) |  |  |
| set | [google.protobuf.Value](#google-protobuf-Value) |  |  |
| remove | [google.protobuf.Value](#google-protobuf-Value) |  |  |






<a name="anytype-Rpc-Object-ListModifyDetailValues-Response"></a>

### Rpc.Object.ListModifyDetailValues.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListModifyDetailValues.Response.Error](#anytype-Rpc-Object-ListModifyDetailValues-Response-Error) |  |  |






<a name="anytype-Rpc-Object-ListModifyDetailValues-Response-Error"></a>

### Rpc.Object.ListModifyDetailValues.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListModifyDetailValues.Response.Error.Code](#anytype-Rpc-Object-ListModifyDetailValues-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListSetDetails"></a>

### Rpc.Object.ListSetDetails







<a name="anytype-Rpc-Object-ListSetDetails-Request"></a>

### Rpc.Object.ListSetDetails.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| details | [model.Detail](#anytype-model-Detail) | repeated |  |






<a name="anytype-Rpc-Object-ListSetDetails-Response"></a>

### Rpc.Object.ListSetDetails.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListSetDetails.Response.Error](#anytype-Rpc-Object-ListSetDetails-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-ListSetDetails-Response-Error"></a>

### Rpc.Object.ListSetDetails.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListSetDetails.Response.Error.Code](#anytype-Rpc-Object-ListSetDetails-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Object-ListSetObjectType"></a>

### Rpc.Object.ListSetObjectType







<a name="anytype-Rpc-Object-ListSetObjectType-Request"></a>

### Rpc.Object.ListSetObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| objectTypeUniqueKey | [string](#string) |  |  |






<a name="anytype-Rpc-Object-ListSetObjectType-Response"></a>

### Rpc.Object.ListSetObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListSetObjectType.Response.Error](#anytype-Rpc-Object-ListSetObjectType-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-Object-ListSetObjectType-Response-Error"></a>

### Rpc.Object.ListSetObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListSetObjectType.Response.Error.Code](#anytype-Rpc-Object-ListSetObjectType-Response-Error-Code) |  |  |
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
| spaceId | [string](#string) |  | Required only for date objects |
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
| contextId | [string](#string) |  | deprecated, GO-1926 |
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
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype-model-Range) |  |  |






<a name="anytype-Rpc-Object-Redo-Response-Error"></a>

### Rpc.Object.Redo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Redo.Response.Error.Code](#anytype-Rpc-Object-Redo-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Refresh"></a>

### Rpc.Object.Refresh







<a name="anytype-Rpc-Object-Refresh-Request"></a>

### Rpc.Object.Refresh.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Refresh-Response"></a>

### Rpc.Object.Refresh.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Refresh.Response.Error](#anytype-Rpc-Object-Refresh-Response-Error) |  |  |






<a name="anytype-Rpc-Object-Refresh-Response-Error"></a>

### Rpc.Object.Refresh.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Refresh.Response.Error.Code](#anytype-Rpc-Object-Refresh-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Object-Search"></a>

### Rpc.Object.Search
deprecated in favor of SearchWithMeta






<a name="anytype-Rpc-Object-Search-Request"></a>

### Rpc.Object.Search.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |
| sorts | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated |  |
| fullText | [string](#string) |  |  |
| offset | [int32](#int32) |  |  |
| limit | [int32](#int32) |  |  |
| objectTypeFilter | [string](#string) | repeated | additional filter by objectTypes

DEPRECATED, GO-1926 |
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
| spaceId | [string](#string) |  |  |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated | filters |
| sorts | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated | sorts |
| limit | [int64](#int64) |  | results limit |
| offset | [int64](#int64) |  | initial offset; middleware will find afterId |
| keys | [string](#string) | repeated | (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| afterId | [string](#string) |  | (optional) pagination: middleware will return results after given id |
| beforeId | [string](#string) |  | (optional) pagination: middleware will return results before given id |
| source | [string](#string) | repeated |  |
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






<a name="anytype-Rpc-Object-SearchWithMeta"></a>

### Rpc.Object.SearchWithMeta







<a name="anytype-Rpc-Object-SearchWithMeta-Request"></a>

### Rpc.Object.SearchWithMeta.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| filters | [model.Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |
| sorts | [model.Block.Content.Dataview.Sort](#anytype-model-Block-Content-Dataview-Sort) | repeated |  |
| fullText | [string](#string) |  |  |
| offset | [int32](#int32) |  |  |
| limit | [int32](#int32) |  |  |
| objectTypeFilter | [string](#string) | repeated | additional filter by objectTypes

DEPRECATED, GO-1926 |
| keys | [string](#string) | repeated | needed keys in details for return, when empty - will return all |
| returnMeta | [bool](#bool) |  | add ResultMeta to each result |
| returnMetaRelationDetails | [bool](#bool) |  | add relation option details to meta |
| returnHTMLHighlightsInsteadOfRanges | [bool](#bool) |  | DEPRECATED |






<a name="anytype-Rpc-Object-SearchWithMeta-Response"></a>

### Rpc.Object.SearchWithMeta.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SearchWithMeta.Response.Error](#anytype-Rpc-Object-SearchWithMeta-Response-Error) |  |  |
| results | [model.Search.Result](#anytype-model-Search-Result) | repeated |  |






<a name="anytype-Rpc-Object-SearchWithMeta-Response-Error"></a>

### Rpc.Object.SearchWithMeta.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SearchWithMeta.Response.Error.Code](#anytype-Rpc-Object-SearchWithMeta-Response-Error-Code) |  |  |
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







<a name="anytype-Rpc-Object-SetDetails-Request"></a>

### Rpc.Object.SetDetails.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| details | [model.Detail](#anytype-model-Detail) | repeated |  |






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
| objectTypeUniqueKey | [string](#string) |  |  |






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
| contextId | [string](#string) |  | deprecated, GO-1926 |
| objectId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |
| spaceId | [string](#string) |  | Required only for date objects |
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
| spaceId | [string](#string) |  |  |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| ids | [string](#string) | repeated | ids for subscribe |
| keys | [string](#string) | repeated | sorts (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
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
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype-model-Range) |  |  |






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







<a name="anytype-Rpc-ObjectType-ListConflictingRelations"></a>

### Rpc.ObjectType.ListConflictingRelations







<a name="anytype-Rpc-ObjectType-ListConflictingRelations-Request"></a>

### Rpc.ObjectType.ListConflictingRelations.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| typeObjectId | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-ListConflictingRelations-Response"></a>

### Rpc.ObjectType.ListConflictingRelations.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.ListConflictingRelations.Response.Error](#anytype-Rpc-ObjectType-ListConflictingRelations-Response-Error) |  |  |
| relationIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectType-ListConflictingRelations-Response-Error"></a>

### Rpc.ObjectType.ListConflictingRelations.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.ListConflictingRelations.Response.Error.Code](#anytype-Rpc-ObjectType-ListConflictingRelations-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-Recommended"></a>

### Rpc.ObjectType.Recommended







<a name="anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet"></a>

### Rpc.ObjectType.Recommended.FeaturedRelationsSet







<a name="anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Request"></a>

### Rpc.ObjectType.Recommended.FeaturedRelationsSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| typeObjectId | [string](#string) |  |  |
| relationObjectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response"></a>

### Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response.Error](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response-Error) |  |  |






<a name="anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response-Error"></a>

### Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response.Error.Code](#anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-Recommended-RelationsSet"></a>

### Rpc.ObjectType.Recommended.RelationsSet







<a name="anytype-Rpc-ObjectType-Recommended-RelationsSet-Request"></a>

### Rpc.ObjectType.Recommended.RelationsSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| typeObjectId | [string](#string) |  |  |
| relationObjectIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectType-Recommended-RelationsSet-Response"></a>

### Rpc.ObjectType.Recommended.RelationsSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Recommended.RelationsSet.Response.Error](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Response-Error) |  |  |






<a name="anytype-Rpc-ObjectType-Recommended-RelationsSet-Response-Error"></a>

### Rpc.ObjectType.Recommended.RelationsSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Recommended.RelationsSet.Response.Error.Code](#anytype-Rpc-ObjectType-Recommended-RelationsSet-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






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






<a name="anytype-Rpc-ObjectType-ResolveLayoutConflicts"></a>

### Rpc.ObjectType.ResolveLayoutConflicts







<a name="anytype-Rpc-ObjectType-ResolveLayoutConflicts-Request"></a>

### Rpc.ObjectType.ResolveLayoutConflicts.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| typeObjectId | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response"></a>

### Rpc.ObjectType.ResolveLayoutConflicts.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.ResolveLayoutConflicts.Response.Error](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response-Error) |  |  |






<a name="anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response-Error"></a>

### Rpc.ObjectType.ResolveLayoutConflicts.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.ResolveLayoutConflicts.Response.Error.Code](#anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-ObjectType-SetOrder"></a>

### Rpc.ObjectType.SetOrder







<a name="anytype-Rpc-ObjectType-SetOrder-Request"></a>

### Rpc.ObjectType.SetOrder.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| typeIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-ObjectType-SetOrder-Response"></a>

### Rpc.ObjectType.SetOrder.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.SetOrder.Response.Error](#anytype-Rpc-ObjectType-SetOrder-Response-Error) |  |  |
| orderIds | [string](#string) | repeated | final list of order ids |






<a name="anytype-Rpc-ObjectType-SetOrder-Response-Error"></a>

### Rpc.ObjectType.SetOrder.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.SetOrder.Response.Error.Code](#anytype-Rpc-ObjectType-SetOrder-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Process-Subscribe"></a>

### Rpc.Process.Subscribe







<a name="anytype-Rpc-Process-Subscribe-Request"></a>

### Rpc.Process.Subscribe.Request







<a name="anytype-Rpc-Process-Subscribe-Response"></a>

### Rpc.Process.Subscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Process.Subscribe.Response.Error](#anytype-Rpc-Process-Subscribe-Response-Error) |  |  |






<a name="anytype-Rpc-Process-Subscribe-Response-Error"></a>

### Rpc.Process.Subscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Process.Subscribe.Response.Error.Code](#anytype-Rpc-Process-Subscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Process-Unsubscribe"></a>

### Rpc.Process.Unsubscribe







<a name="anytype-Rpc-Process-Unsubscribe-Request"></a>

### Rpc.Process.Unsubscribe.Request







<a name="anytype-Rpc-Process-Unsubscribe-Response"></a>

### Rpc.Process.Unsubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Process.Unsubscribe.Response.Error](#anytype-Rpc-Process-Unsubscribe-Response-Error) |  |  |






<a name="anytype-Rpc-Process-Unsubscribe-Response-Error"></a>

### Rpc.Process.Unsubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Process.Unsubscribe.Response.Error.Code](#anytype-Rpc-Process-Unsubscribe-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing"></a>

### Rpc.Publishing







<a name="anytype-Rpc-Publishing-Create"></a>

### Rpc.Publishing.Create







<a name="anytype-Rpc-Publishing-Create-Request"></a>

### Rpc.Publishing.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| objectId | [string](#string) |  |  |
| uri | [string](#string) |  |  |
| joinSpace | [bool](#bool) |  |  |






<a name="anytype-Rpc-Publishing-Create-Response"></a>

### Rpc.Publishing.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Publishing.Create.Response.Error](#anytype-Rpc-Publishing-Create-Response-Error) |  |  |
| uri | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-Create-Response-Error"></a>

### Rpc.Publishing.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Publishing.Create.Response.Error.Code](#anytype-Rpc-Publishing-Create-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-GetStatus"></a>

### Rpc.Publishing.GetStatus







<a name="anytype-Rpc-Publishing-GetStatus-Request"></a>

### Rpc.Publishing.GetStatus.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-GetStatus-Response"></a>

### Rpc.Publishing.GetStatus.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Publishing.GetStatus.Response.Error](#anytype-Rpc-Publishing-GetStatus-Response-Error) |  |  |
| publish | [Rpc.Publishing.PublishState](#anytype-Rpc-Publishing-PublishState) |  |  |






<a name="anytype-Rpc-Publishing-GetStatus-Response-Error"></a>

### Rpc.Publishing.GetStatus.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Publishing.GetStatus.Response.Error.Code](#anytype-Rpc-Publishing-GetStatus-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-List"></a>

### Rpc.Publishing.List







<a name="anytype-Rpc-Publishing-List-Request"></a>

### Rpc.Publishing.List.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-List-Response"></a>

### Rpc.Publishing.List.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Publishing.List.Response.Error](#anytype-Rpc-Publishing-List-Response-Error) |  |  |
| publishes | [Rpc.Publishing.PublishState](#anytype-Rpc-Publishing-PublishState) | repeated |  |






<a name="anytype-Rpc-Publishing-List-Response-Error"></a>

### Rpc.Publishing.List.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Publishing.List.Response.Error.Code](#anytype-Rpc-Publishing-List-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-PublishState"></a>

### Rpc.Publishing.PublishState



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| objectId | [string](#string) |  |  |
| uri | [string](#string) |  |  |
| status | [Rpc.Publishing.PublishStatus](#anytype-Rpc-Publishing-PublishStatus) |  |  |
| version | [string](#string) |  |  |
| timestamp | [int64](#int64) |  |  |
| size | [int64](#int64) |  |  |
| joinSpace | [bool](#bool) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Publishing-Remove"></a>

### Rpc.Publishing.Remove







<a name="anytype-Rpc-Publishing-Remove-Request"></a>

### Rpc.Publishing.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| objectId | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-Remove-Response"></a>

### Rpc.Publishing.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Publishing.Remove.Response.Error](#anytype-Rpc-Publishing-Remove-Response-Error) |  |  |






<a name="anytype-Rpc-Publishing-Remove-Response-Error"></a>

### Rpc.Publishing.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Publishing.Remove.Response.Error.Code](#anytype-Rpc-Publishing-Remove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-ResolveUri"></a>

### Rpc.Publishing.ResolveUri







<a name="anytype-Rpc-Publishing-ResolveUri-Request"></a>

### Rpc.Publishing.ResolveUri.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| uri | [string](#string) |  |  |






<a name="anytype-Rpc-Publishing-ResolveUri-Response"></a>

### Rpc.Publishing.ResolveUri.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Publishing.ResolveUri.Response.Error](#anytype-Rpc-Publishing-ResolveUri-Response-Error) |  |  |
| publish | [Rpc.Publishing.PublishState](#anytype-Rpc-Publishing-PublishState) |  |  |






<a name="anytype-Rpc-Publishing-ResolveUri-Response-Error"></a>

### Rpc.Publishing.ResolveUri.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Publishing.ResolveUri.Response.Error.Code](#anytype-Rpc-Publishing-ResolveUri-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-PushNotification"></a>

### Rpc.PushNotification







<a name="anytype-Rpc-PushNotification-RegisterToken"></a>

### Rpc.PushNotification.RegisterToken







<a name="anytype-Rpc-PushNotification-RegisterToken-Request"></a>

### Rpc.PushNotification.RegisterToken.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |
| platform | [Rpc.PushNotification.RegisterToken.Platform](#anytype-Rpc-PushNotification-RegisterToken-Platform) |  |  |






<a name="anytype-Rpc-PushNotification-RegisterToken-Response"></a>

### Rpc.PushNotification.RegisterToken.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.PushNotification.RegisterToken.Response.Error](#anytype-Rpc-PushNotification-RegisterToken-Response-Error) |  |  |






<a name="anytype-Rpc-PushNotification-RegisterToken-Response-Error"></a>

### Rpc.PushNotification.RegisterToken.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.PushNotification.RegisterToken.Response.Error.Code](#anytype-Rpc-PushNotification-RegisterToken-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-PushNotification-ResetIds"></a>

### Rpc.PushNotification.ResetIds







<a name="anytype-Rpc-PushNotification-ResetIds-Request"></a>

### Rpc.PushNotification.ResetIds.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| chatIds | [string](#string) | repeated |  |






<a name="anytype-Rpc-PushNotification-ResetIds-Response"></a>

### Rpc.PushNotification.ResetIds.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.PushNotification.ResetIds.Response.Error](#anytype-Rpc-PushNotification-ResetIds-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-PushNotification-ResetIds-Response-Error"></a>

### Rpc.PushNotification.ResetIds.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.PushNotification.ResetIds.Response.Error.Code](#anytype-Rpc-PushNotification-ResetIds-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-PushNotification-SetForceModeIds"></a>

### Rpc.PushNotification.SetForceModeIds







<a name="anytype-Rpc-PushNotification-SetForceModeIds-Request"></a>

### Rpc.PushNotification.SetForceModeIds.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| chatIds | [string](#string) | repeated |  |
| mode | [Rpc.PushNotification.Mode](#anytype-Rpc-PushNotification-Mode) |  |  |






<a name="anytype-Rpc-PushNotification-SetForceModeIds-Response"></a>

### Rpc.PushNotification.SetForceModeIds.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.PushNotification.SetForceModeIds.Response.Error](#anytype-Rpc-PushNotification-SetForceModeIds-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-PushNotification-SetForceModeIds-Response-Error"></a>

### Rpc.PushNotification.SetForceModeIds.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.PushNotification.SetForceModeIds.Response.Error.Code](#anytype-Rpc-PushNotification-SetForceModeIds-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-PushNotification-SetSpaceMode"></a>

### Rpc.PushNotification.SetSpaceMode







<a name="anytype-Rpc-PushNotification-SetSpaceMode-Request"></a>

### Rpc.PushNotification.SetSpaceMode.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| mode | [Rpc.PushNotification.Mode](#anytype-Rpc-PushNotification-Mode) |  |  |






<a name="anytype-Rpc-PushNotification-SetSpaceMode-Response"></a>

### Rpc.PushNotification.SetSpaceMode.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.PushNotification.SetSpaceMode.Response.Error](#anytype-Rpc-PushNotification-SetSpaceMode-Response-Error) |  |  |
| event | [ResponseEvent](#anytype-ResponseEvent) |  |  |






<a name="anytype-Rpc-PushNotification-SetSpaceMode-Response-Error"></a>

### Rpc.PushNotification.SetSpaceMode.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.PushNotification.SetSpaceMode.Response.Error.Code](#anytype-Rpc-PushNotification-SetSpaceMode-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Relation-ListWithValue"></a>

### Rpc.Relation.ListWithValue







<a name="anytype-Rpc-Relation-ListWithValue-Request"></a>

### Rpc.Relation.ListWithValue.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  |  |






<a name="anytype-Rpc-Relation-ListWithValue-Response"></a>

### Rpc.Relation.ListWithValue.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Relation.ListWithValue.Response.Error](#anytype-Rpc-Relation-ListWithValue-Response-Error) |  |  |
| list | [Rpc.Relation.ListWithValue.Response.ResponseItem](#anytype-Rpc-Relation-ListWithValue-Response-ResponseItem) | repeated |  |






<a name="anytype-Rpc-Relation-ListWithValue-Response-Error"></a>

### Rpc.Relation.ListWithValue.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Relation.ListWithValue.Response.Error.Code](#anytype-Rpc-Relation-ListWithValue-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Relation-ListWithValue-Response-ResponseItem"></a>

### Rpc.Relation.ListWithValue.Response.ResponseItem



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relationKey | [string](#string) |  |  |
| counter | [int64](#int64) |  |  |






<a name="anytype-Rpc-Relation-Option"></a>

### Rpc.Relation.Option







<a name="anytype-Rpc-Relation-Option-SetOrder"></a>

### Rpc.Relation.Option.SetOrder







<a name="anytype-Rpc-Relation-Option-SetOrder-Request"></a>

### Rpc.Relation.Option.SetOrder.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| relationKey | [string](#string) |  |  |
| relationOptionOrder | [string](#string) | repeated | result order of relation option ids |






<a name="anytype-Rpc-Relation-Option-SetOrder-Response"></a>

### Rpc.Relation.Option.SetOrder.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Relation.Option.SetOrder.Response.Error](#anytype-Rpc-Relation-Option-SetOrder-Response-Error) |  |  |
| relationOptionOrder | [string](#string) | repeated | final order of relation option ids with their lexids |






<a name="anytype-Rpc-Relation-Option-SetOrder-Response-Error"></a>

### Rpc.Relation.Option.SetOrder.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Relation.Option.SetOrder.Response.Error.Code](#anytype-Rpc-Relation-Option-SetOrder-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Space"></a>

### Rpc.Space







<a name="anytype-Rpc-Space-Delete"></a>

### Rpc.Space.Delete







<a name="anytype-Rpc-Space-Delete-Request"></a>

### Rpc.Space.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-Delete-Response"></a>

### Rpc.Space.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.Delete.Response.Error](#anytype-Rpc-Space-Delete-Response-Error) |  |  |
| timestamp | [int64](#int64) |  |  |






<a name="anytype-Rpc-Space-Delete-Response-Error"></a>

### Rpc.Space.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.Delete.Response.Error.Code](#anytype-Rpc-Space-Delete-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteChange"></a>

### Rpc.Space.InviteChange







<a name="anytype-Rpc-Space-InviteChange-Request"></a>

### Rpc.Space.InviteChange.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| permissions | [model.ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |






<a name="anytype-Rpc-Space-InviteChange-Response"></a>

### Rpc.Space.InviteChange.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.InviteChange.Response.Error](#anytype-Rpc-Space-InviteChange-Response-Error) |  |  |






<a name="anytype-Rpc-Space-InviteChange-Response-Error"></a>

### Rpc.Space.InviteChange.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.InviteChange.Response.Error.Code](#anytype-Rpc-Space-InviteChange-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteGenerate"></a>

### Rpc.Space.InviteGenerate







<a name="anytype-Rpc-Space-InviteGenerate-Request"></a>

### Rpc.Space.InviteGenerate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| inviteType | [model.InviteType](#anytype-model-InviteType) |  |  |
| permissions | [model.ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |






<a name="anytype-Rpc-Space-InviteGenerate-Response"></a>

### Rpc.Space.InviteGenerate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.InviteGenerate.Response.Error](#anytype-Rpc-Space-InviteGenerate-Response-Error) |  |  |
| inviteCid | [string](#string) |  |  |
| inviteFileKey | [string](#string) |  |  |
| inviteType | [model.InviteType](#anytype-model-InviteType) |  |  |
| permissions | [model.ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |






<a name="anytype-Rpc-Space-InviteGenerate-Response-Error"></a>

### Rpc.Space.InviteGenerate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.InviteGenerate.Response.Error.Code](#anytype-Rpc-Space-InviteGenerate-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteGetCurrent"></a>

### Rpc.Space.InviteGetCurrent







<a name="anytype-Rpc-Space-InviteGetCurrent-Request"></a>

### Rpc.Space.InviteGetCurrent.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteGetCurrent-Response"></a>

### Rpc.Space.InviteGetCurrent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.InviteGetCurrent.Response.Error](#anytype-Rpc-Space-InviteGetCurrent-Response-Error) |  |  |
| inviteCid | [string](#string) |  |  |
| inviteFileKey | [string](#string) |  |  |
| inviteType | [model.InviteType](#anytype-model-InviteType) |  |  |
| permissions | [model.ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |






<a name="anytype-Rpc-Space-InviteGetCurrent-Response-Error"></a>

### Rpc.Space.InviteGetCurrent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.InviteGetCurrent.Response.Error.Code](#anytype-Rpc-Space-InviteGetCurrent-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteGetGuest"></a>

### Rpc.Space.InviteGetGuest







<a name="anytype-Rpc-Space-InviteGetGuest-Request"></a>

### Rpc.Space.InviteGetGuest.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteGetGuest-Response"></a>

### Rpc.Space.InviteGetGuest.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.InviteGetGuest.Response.Error](#anytype-Rpc-Space-InviteGetGuest-Response-Error) |  |  |
| inviteCid | [string](#string) |  |  |
| inviteFileKey | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteGetGuest-Response-Error"></a>

### Rpc.Space.InviteGetGuest.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.InviteGetGuest.Response.Error.Code](#anytype-Rpc-Space-InviteGetGuest-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteRevoke"></a>

### Rpc.Space.InviteRevoke







<a name="anytype-Rpc-Space-InviteRevoke-Request"></a>

### Rpc.Space.InviteRevoke.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteRevoke-Response"></a>

### Rpc.Space.InviteRevoke.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.InviteRevoke.Response.Error](#anytype-Rpc-Space-InviteRevoke-Response-Error) |  |  |






<a name="anytype-Rpc-Space-InviteRevoke-Response-Error"></a>

### Rpc.Space.InviteRevoke.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.InviteRevoke.Response.Error.Code](#anytype-Rpc-Space-InviteRevoke-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteView"></a>

### Rpc.Space.InviteView







<a name="anytype-Rpc-Space-InviteView-Request"></a>

### Rpc.Space.InviteView.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inviteCid | [string](#string) |  |  |
| inviteFileKey | [string](#string) |  |  |






<a name="anytype-Rpc-Space-InviteView-Response"></a>

### Rpc.Space.InviteView.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.InviteView.Response.Error](#anytype-Rpc-Space-InviteView-Response-Error) |  |  |
| spaceId | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |
| spaceIconCid | [string](#string) |  |  |
| creatorName | [string](#string) |  |  |
| creatorIconCid | [string](#string) |  |  |
| spaceIconOption | [uint32](#uint32) |  |  |
| spaceUxType | [uint32](#uint32) |  |  |
| isGuestUserInvite | [bool](#bool) |  | deprecated, use inviteType |
| inviteType | [model.InviteType](#anytype-model-InviteType) |  |  |






<a name="anytype-Rpc-Space-InviteView-Response-Error"></a>

### Rpc.Space.InviteView.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.InviteView.Response.Error.Code](#anytype-Rpc-Space-InviteView-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-Join"></a>

### Rpc.Space.Join







<a name="anytype-Rpc-Space-Join-Request"></a>

### Rpc.Space.Join.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| networkId | [string](#string) |  | not-empty only for self-hosting |
| spaceId | [string](#string) |  |  |
| inviteCid | [string](#string) |  |  |
| inviteFileKey | [string](#string) |  |  |






<a name="anytype-Rpc-Space-Join-Response"></a>

### Rpc.Space.Join.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.Join.Response.Error](#anytype-Rpc-Space-Join-Response-Error) |  |  |






<a name="anytype-Rpc-Space-Join-Response-Error"></a>

### Rpc.Space.Join.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.Join.Response.Error.Code](#anytype-Rpc-Space-Join-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-JoinCancel"></a>

### Rpc.Space.JoinCancel







<a name="anytype-Rpc-Space-JoinCancel-Request"></a>

### Rpc.Space.JoinCancel.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-JoinCancel-Response"></a>

### Rpc.Space.JoinCancel.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.JoinCancel.Response.Error](#anytype-Rpc-Space-JoinCancel-Response-Error) |  |  |






<a name="anytype-Rpc-Space-JoinCancel-Response-Error"></a>

### Rpc.Space.JoinCancel.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.JoinCancel.Response.Error.Code](#anytype-Rpc-Space-JoinCancel-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-LeaveApprove"></a>

### Rpc.Space.LeaveApprove







<a name="anytype-Rpc-Space-LeaveApprove-Request"></a>

### Rpc.Space.LeaveApprove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| identities | [string](#string) | repeated |  |






<a name="anytype-Rpc-Space-LeaveApprove-Response"></a>

### Rpc.Space.LeaveApprove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.LeaveApprove.Response.Error](#anytype-Rpc-Space-LeaveApprove-Response-Error) |  |  |






<a name="anytype-Rpc-Space-LeaveApprove-Response-Error"></a>

### Rpc.Space.LeaveApprove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.LeaveApprove.Response.Error.Code](#anytype-Rpc-Space-LeaveApprove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-MakeShareable"></a>

### Rpc.Space.MakeShareable







<a name="anytype-Rpc-Space-MakeShareable-Request"></a>

### Rpc.Space.MakeShareable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-MakeShareable-Response"></a>

### Rpc.Space.MakeShareable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.MakeShareable.Response.Error](#anytype-Rpc-Space-MakeShareable-Response-Error) |  |  |






<a name="anytype-Rpc-Space-MakeShareable-Response-Error"></a>

### Rpc.Space.MakeShareable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.MakeShareable.Response.Error.Code](#anytype-Rpc-Space-MakeShareable-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-ParticipantPermissionsChange"></a>

### Rpc.Space.ParticipantPermissionsChange







<a name="anytype-Rpc-Space-ParticipantPermissionsChange-Request"></a>

### Rpc.Space.ParticipantPermissionsChange.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| changes | [model.ParticipantPermissionChange](#anytype-model-ParticipantPermissionChange) | repeated |  |






<a name="anytype-Rpc-Space-ParticipantPermissionsChange-Response"></a>

### Rpc.Space.ParticipantPermissionsChange.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.ParticipantPermissionsChange.Response.Error](#anytype-Rpc-Space-ParticipantPermissionsChange-Response-Error) |  |  |






<a name="anytype-Rpc-Space-ParticipantPermissionsChange-Response-Error"></a>

### Rpc.Space.ParticipantPermissionsChange.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.ParticipantPermissionsChange.Response.Error.Code](#anytype-Rpc-Space-ParticipantPermissionsChange-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-ParticipantRemove"></a>

### Rpc.Space.ParticipantRemove







<a name="anytype-Rpc-Space-ParticipantRemove-Request"></a>

### Rpc.Space.ParticipantRemove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| identities | [string](#string) | repeated |  |






<a name="anytype-Rpc-Space-ParticipantRemove-Response"></a>

### Rpc.Space.ParticipantRemove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.ParticipantRemove.Response.Error](#anytype-Rpc-Space-ParticipantRemove-Response-Error) |  |  |






<a name="anytype-Rpc-Space-ParticipantRemove-Response-Error"></a>

### Rpc.Space.ParticipantRemove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.ParticipantRemove.Response.Error.Code](#anytype-Rpc-Space-ParticipantRemove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-RequestApprove"></a>

### Rpc.Space.RequestApprove







<a name="anytype-Rpc-Space-RequestApprove-Request"></a>

### Rpc.Space.RequestApprove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| identity | [string](#string) |  |  |
| permissions | [model.ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |






<a name="anytype-Rpc-Space-RequestApprove-Response"></a>

### Rpc.Space.RequestApprove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.RequestApprove.Response.Error](#anytype-Rpc-Space-RequestApprove-Response-Error) |  |  |






<a name="anytype-Rpc-Space-RequestApprove-Response-Error"></a>

### Rpc.Space.RequestApprove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.RequestApprove.Response.Error.Code](#anytype-Rpc-Space-RequestApprove-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-RequestDecline"></a>

### Rpc.Space.RequestDecline







<a name="anytype-Rpc-Space-RequestDecline-Request"></a>

### Rpc.Space.RequestDecline.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| identity | [string](#string) |  |  |






<a name="anytype-Rpc-Space-RequestDecline-Response"></a>

### Rpc.Space.RequestDecline.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.RequestDecline.Response.Error](#anytype-Rpc-Space-RequestDecline-Response-Error) |  |  |






<a name="anytype-Rpc-Space-RequestDecline-Response-Error"></a>

### Rpc.Space.RequestDecline.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.RequestDecline.Response.Error.Code](#anytype-Rpc-Space-RequestDecline-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-SetOrder"></a>

### Rpc.Space.SetOrder







<a name="anytype-Rpc-Space-SetOrder-Request"></a>

### Rpc.Space.SetOrder.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceViewId | [string](#string) |  |  |
| spaceViewOrder | [string](#string) | repeated | result order of space view ids |






<a name="anytype-Rpc-Space-SetOrder-Response"></a>

### Rpc.Space.SetOrder.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.SetOrder.Response.Error](#anytype-Rpc-Space-SetOrder-Response-Error) |  |  |
| spaceViewOrder | [string](#string) | repeated | final order of space view ids with their lexids |






<a name="anytype-Rpc-Space-SetOrder-Response-Error"></a>

### Rpc.Space.SetOrder.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.SetOrder.Response.Error.Code](#anytype-Rpc-Space-SetOrder-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-StopSharing"></a>

### Rpc.Space.StopSharing







<a name="anytype-Rpc-Space-StopSharing-Request"></a>

### Rpc.Space.StopSharing.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-StopSharing-Response"></a>

### Rpc.Space.StopSharing.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.StopSharing.Response.Error](#anytype-Rpc-Space-StopSharing-Response-Error) |  |  |






<a name="anytype-Rpc-Space-StopSharing-Response-Error"></a>

### Rpc.Space.StopSharing.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.StopSharing.Response.Error.Code](#anytype-Rpc-Space-StopSharing-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-Rpc-Space-UnsetOrder"></a>

### Rpc.Space.UnsetOrder







<a name="anytype-Rpc-Space-UnsetOrder-Request"></a>

### Rpc.Space.UnsetOrder.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceViewId | [string](#string) |  |  |






<a name="anytype-Rpc-Space-UnsetOrder-Response"></a>

### Rpc.Space.UnsetOrder.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Space.UnsetOrder.Response.Error](#anytype-Rpc-Space-UnsetOrder-Response-Error) |  |  |






<a name="anytype-Rpc-Space-UnsetOrder-Response-Error"></a>

### Rpc.Space.UnsetOrder.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Space.UnsetOrder.Response.Error.Code](#anytype-Rpc-Space-UnsetOrder-Response-Error-Code) |  |  |
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
| spaceId | [string](#string) |  |  |






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
| spaceId | [string](#string) |  |  |
| imageKind | [model.ImageKind](#anytype-model-ImageKind) |  |  |






<a name="anytype-Rpc-Unsplash-Download-Response"></a>

### Rpc.Unsplash.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Unsplash.Download.Response.Error](#anytype-Rpc-Unsplash-Download-Response-Error) |  |  |
| objectId | [string](#string) |  |  |






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
| fulltextPrimaryLanguage | [string](#string) |  | optional, default fts language |






<a name="anytype-Rpc-Wallet-Create-Response"></a>

### Rpc.Wallet.Create.Response
Middleware-to-front-end response, that can contain mnemonic of a created account and a NULL error or an empty mnemonic and a non-NULL error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Create.Response.Error](#anytype-Rpc-Wallet-Create-Response-Error) |  |  |
| mnemonic | [string](#string) |  | Mnemonic of a new account (sequence of words, divided by spaces) |
| accountKey | [string](#string) |  |  |






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
| mnemonic | [string](#string) |  | cold auth |
| appKey | [string](#string) |  | persistent app key, that can be used to restore session. Used for Local JSON API |
| token | [string](#string) |  | token from the previous session |
| accountKey | [string](#string) |  | private key of specific account |






<a name="anytype-Rpc-Wallet-CreateSession-Response"></a>

### Rpc.Wallet.CreateSession.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.CreateSession.Response.Error](#anytype-Rpc-Wallet-CreateSession-Response-Error) |  |  |
| token | [string](#string) |  |  |
| appToken | [string](#string) |  | in case of mnemonic auth, need to be persisted by client |
| accountId | [string](#string) |  | temp, should be replaced with AccountInfo message |






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
| mnemonic | [string](#string) |  | Mnemonic of a wallet to recover (mutually exclusive with accountKey) |
| fulltextPrimaryLanguage | [string](#string) |  | optional, default fts language |
| accountKey | [string](#string) |  | optional: serialized account master node (base64 encoded), used to auth account instead of mnemonic |






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
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | object details |
| useCase | [Rpc.Object.ImportUseCase.Request.UseCase](#anytype-Rpc-Object-ImportUseCase-Request-UseCase) |  | use case |
| withChat | [bool](#bool) |  | deprecated, use spaceUxType |






<a name="anytype-Rpc-Workspace-Create-Response"></a>

### Rpc.Workspace.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Create.Response.Error](#anytype-Rpc-Workspace-Create-Response-Error) |  |  |
| spaceId | [string](#string) |  |  |
| startingObjectId | [string](#string) |  |  |






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
| spaceId | [string](#string) |  |  |
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
| spaceId | [string](#string) |  |  |
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






<a name="anytype-Rpc-Workspace-Open"></a>

### Rpc.Workspace.Open







<a name="anytype-Rpc-Workspace-Open-Request"></a>

### Rpc.Workspace.Open.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| withChat | [bool](#bool) |  | deprecated, chat will be created automatically if space is shared |






<a name="anytype-Rpc-Workspace-Open-Response"></a>

### Rpc.Workspace.Open.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Open.Response.Error](#anytype-Rpc-Workspace-Open-Response-Error) |  |  |
| info | [model.Account.Info](#anytype-model-Account-Info) |  |  |






<a name="anytype-Rpc-Workspace-Open-Response-Error"></a>

### Rpc.Workspace.Open.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Open.Response.Error.Code](#anytype-Rpc-Workspace-Open-Response-Error-Code) |  |  |
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






<a name="anytype-Rpc-Workspace-SetInfo"></a>

### Rpc.Workspace.SetInfo







<a name="anytype-Rpc-Workspace-SetInfo-Request"></a>

### Rpc.Workspace.SetInfo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |






<a name="anytype-Rpc-Workspace-SetInfo-Response"></a>

### Rpc.Workspace.SetInfo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.SetInfo.Response.Error](#anytype-Rpc-Workspace-SetInfo-Response-Error) |  |  |






<a name="anytype-Rpc-Workspace-SetInfo-Response-Error"></a>

### Rpc.Workspace.SetInfo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.SetInfo.Response.Error.Code](#anytype-Rpc-Workspace-SetInfo-Response-Error-Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype-StreamRequest"></a>

### StreamRequest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| token | [string](#string) |  |  |





 


<a name="anytype-Rpc-AI-Autofill-Request-AutofillMode"></a>

### Rpc.AI.Autofill.Request.AutofillMode


| Name | Number | Description |
| ---- | ------ | ----------- |
| TAG | 0 |  |
| RELATION | 1 |  |
| TYPE | 2 |  |
| TITLE | 3 |  |
| DESCRIPTION | 4 | ... |



<a name="anytype-Rpc-AI-Autofill-Response-Error-Code"></a>

### Rpc.AI.Autofill.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 |  |
| ENDPOINT_NOT_REACHABLE | 101 |  |
| MODEL_NOT_FOUND | 102 |  |
| AUTH_REQUIRED | 103 | ... |



<a name="anytype-Rpc-AI-ListSummary-Response-Error-Code"></a>

### Rpc.AI.ListSummary.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 |  |
| ENDPOINT_NOT_REACHABLE | 101 |  |
| MODEL_NOT_FOUND | 102 |  |
| AUTH_REQUIRED | 103 | ... |



<a name="anytype-Rpc-AI-ObjectCreateFromUrl-Response-Error-Code"></a>

### Rpc.AI.ObjectCreateFromUrl.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 |  |
| ENDPOINT_NOT_REACHABLE | 101 |  |
| MODEL_NOT_FOUND | 102 |  |
| AUTH_REQUIRED | 103 | ... |



<a name="anytype-Rpc-AI-Provider"></a>

### Rpc.AI.Provider


| Name | Number | Description |
| ---- | ------ | ----------- |
| OLLAMA | 0 |  |
| OPENAI | 1 |  |
| LMSTUDIO | 2 |  |
| LLAMACPP | 3 | ... |



<a name="anytype-Rpc-AI-WritingTools-Request-Language"></a>

### Rpc.AI.WritingTools.Request.Language


| Name | Number | Description |
| ---- | ------ | ----------- |
| EN | 0 |  |
| ES | 1 |  |
| FR | 2 |  |
| DE | 3 |  |
| IT | 4 |  |
| PT | 5 |  |
| HI | 6 |  |
| TH | 7 | ... |



<a name="anytype-Rpc-AI-WritingTools-Request-WritingMode"></a>

### Rpc.AI.WritingTools.Request.WritingMode


| Name | Number | Description |
| ---- | ------ | ----------- |
| DEFAULT | 0 |  |
| SUMMARIZE | 1 |  |
| GRAMMAR | 2 |  |
| SHORTEN | 3 |  |
| EXPAND | 4 |  |
| BULLET | 5 |  |
| TABLE | 6 |  |
| CASUAL | 7 |  |
| FUNNY | 8 |  |
| CONFIDENT | 9 |  |
| STRAIGHTFORWARD | 10 |  |
| PROFESSIONAL | 11 |  |
| TRANSLATE | 12 | ... |



<a name="anytype-Rpc-AI-WritingTools-Response-Error-Code"></a>

### Rpc.AI.WritingTools.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 |  |
| ENDPOINT_NOT_REACHABLE | 101 |  |
| MODEL_NOT_FOUND | 102 |  |
| AUTH_REQUIRED | 103 |  |
| LANGUAGE_NOT_SUPPORTED | 104 | ... |



<a name="anytype-Rpc-Account-ChangeJsonApiAddr-Response-Error-Code"></a>

### Rpc.Account.ChangeJsonApiAddr.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 4 |  |



<a name="anytype-Rpc-Account-ChangeNetworkConfigAndRestart-Response-Error-Code"></a>

### Rpc.Account.ChangeNetworkConfigAndRestart.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 4 |  |
| ACCOUNT_FAILED_TO_STOP | 100 |  |
| CONFIG_FILE_NOT_FOUND | 200 |  |
| CONFIG_FILE_INVALID | 201 |  |
| CONFIG_FILE_NETWORK_ID_MISMATCH | 202 |  |



<a name="anytype-Rpc-Account-ConfigUpdate-Response-Error-Code"></a>

### Rpc.Account.ConfigUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| FAILED_TO_WRITE_CONFIG | 102 |  |



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
| FAILED_TO_STOP_RUNNING_NODE | 104 |  |
| FAILED_TO_WRITE_CONFIG | 105 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 106 |  |
| ACCOUNT_CREATION_IS_CANCELED | 107 |  |
| CONFIG_FILE_NOT_FOUND | 200 |  |
| CONFIG_FILE_INVALID | 201 |  |
| CONFIG_FILE_NETWORK_ID_MISMATCH | 202 |  |



<a name="anytype-Rpc-Account-Delete-Response-Error-Code"></a>

### Rpc.Account.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_ALREADY_DELETED | 101 |  |
| UNABLE_TO_CONNECT | 102 |  |



<a name="anytype-Rpc-Account-EnableLocalNetworkSync-Response-Error-Code"></a>

### Rpc.Account.EnableLocalNetworkSync.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 4 |  |



<a name="anytype-Rpc-Account-LocalLink-CreateApp-Response-Error-Code"></a>

### Rpc.Account.LocalLink.CreateApp.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |



<a name="anytype-Rpc-Account-LocalLink-ListApps-Response-Error-Code"></a>

### Rpc.Account.LocalLink.ListApps.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |



<a name="anytype-Rpc-Account-LocalLink-NewChallenge-Response-Error-Code"></a>

### Rpc.Account.LocalLink.NewChallenge.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| TOO_MANY_REQUESTS | 102 | protection from overuse |



<a name="anytype-Rpc-Account-LocalLink-RevokeApp-Response-Error-Code"></a>

### Rpc.Account.LocalLink.RevokeApp.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |



<a name="anytype-Rpc-Account-LocalLink-SolveChallenge-Response-Error-Code"></a>

### Rpc.Account.LocalLink.SolveChallenge.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| INVALID_CHALLENGE_ID | 102 |  |
| CHALLENGE_ATTEMPTS_EXCEEDED | 103 |  |
| INCORRECT_ANSWER | 104 |  |



<a name="anytype-Rpc-Account-Migrate-Response-Error-Code"></a>

### Rpc.Account.Migrate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Id or root path is wrong |
| ACCOUNT_NOT_FOUND | 101 |  |
| CANCELED | 102 |  |
| NOT_ENOUGH_FREE_SPACE | 103 | TODO: [storage] Add specific error codes for migration problems |



<a name="anytype-Rpc-Account-MigrateCancel-Response-Error-Code"></a>

### Rpc.Account.MigrateCancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Id or root path is wrong |



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



<a name="anytype-Rpc-Account-NetworkMode"></a>

### Rpc.Account.NetworkMode


| Name | Number | Description |
| ---- | ------ | ----------- |
| DefaultConfig | 0 | use network config that embedded in binary |
| LocalOnly | 1 | disable any-sync network and use only local p2p nodes |
| CustomConfig | 2 | use config provided in networkConfigFilePath |



<a name="anytype-Rpc-Account-Recover-Response-Error-Code"></a>

### Rpc.Account.Recover.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |
| NEED_TO_RECOVER_WALLET_FIRST | 102 |  |



<a name="anytype-Rpc-Account-RecoverFromLegacyExport-Response-Error-Code"></a>

### Rpc.Account.RecoverFromLegacyExport.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| DIFFERENT_ACCOUNT | 3 |  |



<a name="anytype-Rpc-Account-RevertDeletion-Response-Error-Code"></a>

### Rpc.Account.RevertDeletion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_ACTIVE | 101 |  |
| UNABLE_TO_CONNECT | 102 |  |



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
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |
| FAILED_TO_FETCH_REMOTE_NODE_HAS_INCOMPATIBLE_PROTO_VERSION | 110 |  |
| ACCOUNT_IS_DELETED | 111 |  |
| ACCOUNT_LOAD_IS_CANCELED | 112 |  |
| ACCOUNT_STORE_NOT_MIGRATED | 113 |  |
| CONFIG_FILE_NOT_FOUND | 200 |  |
| CONFIG_FILE_INVALID | 201 |  |
| CONFIG_FILE_NETWORK_ID_MISMATCH | 202 |  |



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



<a name="anytype-Rpc-App-SetDeviceState-Request-DeviceState"></a>

### Rpc.App.SetDeviceState.Request.DeviceState


| Name | Number | Description |
| ---- | ------ | ----------- |
| BACKGROUND | 0 | went to background on mobile, hibernated on desktop |
| FOREGROUND | 1 | went to foreground on mobile, woke from hibernation on desktop |



<a name="anytype-Rpc-App-SetDeviceState-Response-Error-Code"></a>

### Rpc.App.SetDeviceState.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-App-Shutdown-Response-Error-Code"></a>

### Rpc.App.Shutdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-Block-Preview-Response-Error-Code"></a>

### Rpc.Block.Preview.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Block-Replace-Response-Error-Code"></a>

### Rpc.Block.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Block-SetCarriage-Response-Error-Code"></a>

### Rpc.Block.SetCarriage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-BlockDataview-Relation-Set-Response-Error-Code"></a>

### Rpc.BlockDataview.Relation.Set.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-BlockDataview-Sort-SSort-Response-Error-Code"></a>

### Rpc.BlockDataview.Sort.SSort.Response.Error.Code


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
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-BlockFile-SetTargetObjectId-Response-Error-Code"></a>

### Rpc.BlockFile.SetTargetObjectId.Response.Error.Code


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



<a name="anytype-Rpc-BlockLatex-SetProcessor-Response-Error-Code"></a>

### Rpc.BlockLatex.SetProcessor.Response.Error.Code


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



<a name="anytype-Rpc-BlockWidget-SetViewId-Response-Error-Code"></a>

### Rpc.BlockWidget.SetViewId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Broadcast-PayloadEvent-Response-Error-Code"></a>

### Rpc.Broadcast.PayloadEvent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |



<a name="anytype-Rpc-Chat-AddMessage-Response-Error-Code"></a>

### Rpc.Chat.AddMessage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-DeleteMessage-Response-Error-Code"></a>

### Rpc.Chat.DeleteMessage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-EditMessageContent-Response-Error-Code"></a>

### Rpc.Chat.EditMessageContent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-GetMessages-Response-Error-Code"></a>

### Rpc.Chat.GetMessages.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-GetMessagesByIds-Response-Error-Code"></a>

### Rpc.Chat.GetMessagesByIds.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-ReadAll-Response-Error-Code"></a>

### Rpc.Chat.ReadAll.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-ReadMessages-ReadType"></a>

### Rpc.Chat.ReadMessages.ReadType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Messages | 0 |  |
| Mentions | 1 |  |



<a name="anytype-Rpc-Chat-ReadMessages-Response-Error-Code"></a>

### Rpc.Chat.ReadMessages.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| MESSAGES_NOT_FOUND | 100 | chat is empty or invalid beforeOrderId/lastDbState |



<a name="anytype-Rpc-Chat-SubscribeLastMessages-Response-Error-Code"></a>

### Rpc.Chat.SubscribeLastMessages.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-SubscribeToMessagePreviews-Response-Error-Code"></a>

### Rpc.Chat.SubscribeToMessagePreviews.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-ToggleMessageReaction-Response-Error-Code"></a>

### Rpc.Chat.ToggleMessageReaction.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-Unread-ReadType"></a>

### Rpc.Chat.Unread.ReadType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Messages | 0 |  |
| Mentions | 1 |  |



<a name="anytype-Rpc-Chat-Unread-Response-Error-Code"></a>

### Rpc.Chat.Unread.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Chat-Unsubscribe-Response-Error-Code"></a>

### Rpc.Chat.Unsubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Chat-UnsubscribeFromMessagePreviews-Response-Error-Code"></a>

### Rpc.Chat.UnsubscribeFromMessagePreviews.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Debug-AccountSelectTrace-Response-Error-Code"></a>

### Rpc.Debug.AccountSelectTrace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Debug-AnystoreObjectChanges-Request-OrderBy"></a>

### Rpc.Debug.AnystoreObjectChanges.Request.OrderBy


| Name | Number | Description |
| ---- | ------ | ----------- |
| ORDER_ID | 0 |  |
| ITERATION_ORDER | 1 |  |



<a name="anytype-Rpc-Debug-AnystoreObjectChanges-Response-Error-Code"></a>

### Rpc.Debug.AnystoreObjectChanges.Response.Error.Code


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



<a name="anytype-Rpc-Debug-ExportLog-Response-Error-Code"></a>

### Rpc.Debug.ExportLog.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_FOLDER | 3 |  |



<a name="anytype-Rpc-Debug-NetCheck-Response-Error-Code"></a>

### Rpc.Debug.NetCheck.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Debug-OpenedObjects-Response-Error-Code"></a>

### Rpc.Debug.OpenedObjects.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Debug-Ping-Response-Error-Code"></a>

### Rpc.Debug.Ping.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Debug-RunProfiler-Response-Error-Code"></a>

### Rpc.Debug.RunProfiler.Response.Error.Code


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



<a name="anytype-Rpc-Debug-StackGoroutines-Response-Error-Code"></a>

### Rpc.Debug.StackGoroutines.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Debug-Stat-Response-Error-Code"></a>

### Rpc.Debug.Stat.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Debug-Subscriptions-Response-Error-Code"></a>

### Rpc.Debug.Subscriptions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-Device-List-Response-Error-Code"></a>

### Rpc.Device.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Device-NetworkState-Set-Response-Error-Code"></a>

### Rpc.Device.NetworkState.Set.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |



<a name="anytype-Rpc-Device-SetName-Response-Error-Code"></a>

### Rpc.Device.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-File-CacheCancelDownload-Response-Error-Code"></a>

### Rpc.File.CacheCancelDownload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-File-CacheDownload-Response-Error-Code"></a>

### Rpc.File.CacheDownload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-File-DiscardPreload-Response-Error-Code"></a>

### Rpc.File.DiscardPreload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-File-Download-Response-Error-Code"></a>

### Rpc.File.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-File-NodeUsage-Response-Error-Code"></a>

### Rpc.File.NodeUsage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-File-Offload-Response-Error-Code"></a>

### Rpc.File.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |



<a name="anytype-Rpc-File-Reconcile-Response-Error-Code"></a>

### Rpc.File.Reconcile.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-File-SetAutoDownload-Response-Error-Code"></a>

### Rpc.File.SetAutoDownload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-File-SpaceOffload-Response-Error-Code"></a>

### Rpc.File.SpaceOffload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |



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



<a name="anytype-Rpc-Gallery-DownloadIndex-Response-Error-Code"></a>

### Rpc.Gallery.DownloadIndex.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNMARSHALLING_ERROR | 3 |  |
| DOWNLOAD_ERROR | 4 |  |



<a name="anytype-Rpc-Gallery-DownloadManifest-Response-Error-Code"></a>

### Rpc.Gallery.DownloadManifest.Response.Error.Code


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



<a name="anytype-Rpc-History-DiffVersions-Response-Error-Code"></a>

### Rpc.History.DiffVersions.Response.Error.Code


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



<a name="anytype-Rpc-Initial-SetParameters-Response-Error-Code"></a>

### Rpc.Initial.SetParameters.Response.Error.Code


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
| PRIVATE_LINK | 3 |  |



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



<a name="anytype-Rpc-Membership-CodeGetInfo-Response-Error-Code"></a>

### Rpc.Membership.CodeGetInfo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CODE_NOT_FOUND | 5 |  |
| CODE_ALREADY_USED | 6 |  |



<a name="anytype-Rpc-Membership-CodeRedeem-Response-Error-Code"></a>

### Rpc.Membership.CodeRedeem.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CODE_NOT_FOUND | 5 |  |
| CODE_ALREADY_USED | 6 |  |
| BAD_ANYNAME | 7 |  |



<a name="anytype-Rpc-Membership-Finalize-Response-Error-Code"></a>

### Rpc.Membership.Finalize.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| MEMBERSHIP_NOT_FOUND | 6 |  |
| MEMBERSHIP_WRONG_STATE | 7 |  |
| BAD_ANYNAME | 8 |  |
| CAN_NOT_CONNECT | 9 |  |



<a name="anytype-Rpc-Membership-GetPortalLinkUrl-Response-Error-Code"></a>

### Rpc.Membership.GetPortalLinkUrl.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| CAN_NOT_CONNECT | 6 |  |



<a name="anytype-Rpc-Membership-GetStatus-Response-Error-Code"></a>

### Rpc.Membership.GetStatus.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| MEMBERSHIP_NOT_FOUND | 6 |  |
| MEMBERSHIP_WRONG_STATE | 7 |  |
| CAN_NOT_CONNECT | 8 |  |



<a name="anytype-Rpc-Membership-GetTiers-Response-Error-Code"></a>

### Rpc.Membership.GetTiers.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| CAN_NOT_CONNECT | 6 |  |



<a name="anytype-Rpc-Membership-GetVerificationEmail-Response-Error-Code"></a>

### Rpc.Membership.GetVerificationEmail.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| EMAIL_WRONG_FORMAT | 6 |  |
| EMAIL_ALREADY_VERIFIED | 7 |  |
| EMAIL_ALREDY_SENT | 8 |  |
| EMAIL_FAILED_TO_SEND | 9 |  |
| MEMBERSHIP_ALREADY_EXISTS | 10 |  |
| CAN_NOT_CONNECT | 11 |  |



<a name="anytype-Rpc-Membership-GetVerificationEmailStatus-Response-Error-Code"></a>

### Rpc.Membership.GetVerificationEmailStatus.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CAN_NOT_CONNECT | 12 |  |



<a name="anytype-Rpc-Membership-IsNameValid-Response-Error-Code"></a>

### Rpc.Membership.IsNameValid.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| TOO_SHORT | 3 |  |
| TOO_LONG | 4 |  |
| HAS_INVALID_CHARS | 5 |  |
| TIER_FEATURES_NO_NAME | 6 |  |
| TIER_NOT_FOUND | 7 | if everything is fine - &#34;name is already taken&#34; check should be done in the NS see IsNameAvailable() |
| NOT_LOGGED_IN | 8 |  |
| PAYMENT_NODE_ERROR | 9 |  |
| CACHE_ERROR | 10 |  |
| CAN_NOT_RESERVE | 11 | for some probable future use (if needed) |
| CAN_NOT_CONNECT | 12 |  |
| NAME_IS_RESERVED | 13 | Same as if NameService.ResolveName returned that name is already occupied by some user |



<a name="anytype-Rpc-Membership-RegisterPaymentRequest-Response-Error-Code"></a>

### Rpc.Membership.RegisterPaymentRequest.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| TIER_NOT_FOUND | 6 |  |
| TIER_INVALID | 7 |  |
| PAYMENT_METHOD_INVALID | 8 |  |
| BAD_ANYNAME | 9 |  |
| MEMBERSHIP_ALREADY_EXISTS | 10 |  |
| CAN_NOT_CONNECT | 11 |  |
| EMAIL_WRONG_FORMAT | 12 | for tiers and payment methods that require that |



<a name="anytype-Rpc-Membership-VerifyAppStoreReceipt-Response-Error-Code"></a>

### Rpc.Membership.VerifyAppStoreReceipt.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| INVALID_RECEIPT | 6 |  |
| PURCHASE_REGISTRATION_ERROR | 7 |  |
| SUBSCRIPTION_RENEW_ERROR | 8 |  |



<a name="anytype-Rpc-Membership-VerifyEmailCode-Response-Error-Code"></a>

### Rpc.Membership.VerifyEmailCode.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| EMAIL_ALREADY_VERIFIED | 6 |  |
| CODE_EXPIRED | 7 |  |
| CODE_WRONG | 8 |  |
| MEMBERSHIP_NOT_FOUND | 9 |  |
| MEMBERSHIP_ALREADY_ACTIVE | 10 |  |
| CAN_NOT_CONNECT | 11 |  |



<a name="anytype-Rpc-MembershipV2-AnyNameAllocate-Response-Error-Code"></a>

### Rpc.MembershipV2.AnyNameAllocate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| MEMBERSHIP_NOT_FOUND | 6 |  |
| MEMBERSHIP_WRONG_STATE | 7 |  |
| BAD_ANYNAME | 8 |  |
| CAN_NOT_CONNECT | 9 |  |
| V2_CALL_NOT_ENABLED | 10 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-MembershipV2-AnyNameIsValid-Response-Error-Code"></a>

### Rpc.MembershipV2.AnyNameIsValid.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| TOO_SHORT | 3 |  |
| TOO_LONG | 4 |  |
| HAS_INVALID_CHARS | 5 |  |
| ACCOUNT_FEATURES_NO_NAME | 6 | if nothing bought |
| NOT_LOGGED_IN | 8 |  |
| PAYMENT_NODE_ERROR | 9 |  |
| CACHE_ERROR | 10 |  |
| CAN_NOT_RESERVE | 11 | for some probable future use (if needed) |
| CAN_NOT_CONNECT | 12 |  |
| NAME_IS_RESERVED | 13 | Same as if NameService.ResolveName returned that name is already occupied by some user |
| V2_CALL_NOT_ENABLED | 14 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-MembershipV2-CartGet-Response-Error-Code"></a>

### Rpc.MembershipV2.CartGet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_CONNECT | 3 |  |
| V2_CALL_NOT_ENABLED | 4 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-MembershipV2-CartUpdate-Response-Error-Code"></a>

### Rpc.MembershipV2.CartUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_CONNECT | 3 |  |
| BAD_PRODUCT | 4 |  |
| V2_CALL_NOT_ENABLED | 5 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-MembershipV2-GetPortalLink-Response-Error-Code"></a>

### Rpc.MembershipV2.GetPortalLink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| AUTH_BAD | 5 |  |
| V2_CALL_NOT_ENABLED | 6 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-MembershipV2-GetProducts-Response-Error-Code"></a>

### Rpc.MembershipV2.GetProducts.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| AUTH_BAD | 5 |  |
| CACHE_ERROR | 6 |  |
| CAN_NOT_CONNECT | 7 |  |
| V2_CALL_NOT_ENABLED | 8 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-MembershipV2-GetStatus-Response-Error-Code"></a>

### Rpc.MembershipV2.GetStatus.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| PAYMENT_NODE_ERROR | 4 |  |
| CACHE_ERROR | 5 |  |
| MEMBERSHIP_NOT_FOUND | 6 |  |
| MEMBERSHIP_WRONG_STATE | 7 |  |
| CAN_NOT_CONNECT | 8 |  |
| V2_CALL_NOT_ENABLED | 9 | set enableMembershipV2 in AccountCreate or AccountSelect |



<a name="anytype-Rpc-NameService-ResolveAnyId-Response-Error-Code"></a>

### Rpc.NameService.ResolveAnyId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_CONNECT | 3 |  |



<a name="anytype-Rpc-NameService-ResolveName-Response-Error-Code"></a>

### Rpc.NameService.ResolveName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_CONNECT | 3 |  |



<a name="anytype-Rpc-NameService-ResolveSpaceId-Response-Error-Code"></a>

### Rpc.NameService.ResolveSpaceId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_CONNECT | 3 |  |



<a name="anytype-Rpc-NameService-UserAccount-Get-Response-Error-Code"></a>

### Rpc.NameService.UserAccount.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_LOGGED_IN | 3 |  |
| BAD_NAME_RESOLVE | 4 |  |
| CAN_NOT_CONNECT | 5 |  |



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



<a name="anytype-Rpc-Notification-List-Response-Error-Code"></a>

### Rpc.Notification.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |



<a name="anytype-Rpc-Notification-Reply-Response-Error-Code"></a>

### Rpc.Notification.Reply.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |



<a name="anytype-Rpc-Notification-Test-Response-Error-Code"></a>

### Rpc.Notification.Test.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |



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



<a name="anytype-Rpc-Object-ChatAdd-Response-Error-Code"></a>

### Rpc.Object.ChatAdd.Response.Error.Code


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



<a name="anytype-Rpc-Object-CreateFromUrl-Response-Error-Code"></a>

### Rpc.Object.CreateFromUrl.Response.Error.Code


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
| BAD_INPUT | 2 | ... |



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



<a name="anytype-Rpc-Object-CrossSpaceSearchSubscribe-Response-Error-Code"></a>

### Rpc.Object.CrossSpaceSearchSubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-CrossSpaceSearchUnsubscribe-Response-Error-Code"></a>

### Rpc.Object.CrossSpaceSearchUnsubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-DateByTimestamp-Response-Error-Code"></a>

### Rpc.Object.DateByTimestamp.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-Duplicate-Response-Error-Code"></a>

### Rpc.Object.Duplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-Export-Response-Error-Code"></a>

### Rpc.Object.Export.Response.Error.Code


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
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |
| UNAUTHORIZED | 4 |  |
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



<a name="anytype-Rpc-Object-Import-Request-PbParams-Type"></a>

### Rpc.Object.Import.Request.PbParams.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| SPACE | 0 |  |
| EXPERIENCE | 1 |  |



<a name="anytype-Rpc-Object-Import-Response-Error-Code"></a>

### Rpc.Object.Import.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |
| NO_OBJECTS_TO_IMPORT | 5 |  |
| IMPORT_IS_CANCELED | 6 |  |
| LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED | 7 |  |
| FILE_LOAD_ERROR | 8 |  |
| INSUFFICIENT_PERMISSIONS | 9 |  |



<a name="anytype-Rpc-Object-ImportExperience-Response-Error-Code"></a>

### Rpc.Object.ImportExperience.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INSUFFICIENT_PERMISSION | 3 |  |



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
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |



<a name="anytype-Rpc-Object-ImportUseCase-Request-UseCase"></a>

### Rpc.Object.ImportUseCase.Request.UseCase


| Name | Number | Description |
| ---- | ------ | ----------- |
| NONE | 0 |  |
| GET_STARTED | 1 |  |
| DATA_SPACE | 2 |  |
| GUIDE_ONLY | 3 | only the guide without other tables |
| GET_STARTED_MOBILE | 4 |  |
| CHAT_SPACE | 5 |  |
| DATA_SPACE_MOBILE | 6 |  |



<a name="anytype-Rpc-Object-ImportUseCase-Response-Error-Code"></a>

### Rpc.Object.ImportUseCase.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype-Rpc-Object-ListExport-Response-Error-Code"></a>

### Rpc.Object.ListExport.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Object-ListModifyDetailValues-Response-Error-Code"></a>

### Rpc.Object.ListModifyDetailValues.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-ListSetDetails-Response-Error-Code"></a>

### Rpc.Object.ListSetDetails.Response.Error.Code


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



<a name="anytype-Rpc-Object-ListSetObjectType-Response-Error-Code"></a>

### Rpc.Object.ListSetObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Object-Open-Response-Error-Code"></a>

### Rpc.Object.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |
| OBJECT_DELETED | 4 | ... |



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



<a name="anytype-Rpc-Object-Refresh-Response-Error-Code"></a>

### Rpc.Object.Refresh.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| OBJECT_DELETED | 4 |  |



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



<a name="anytype-Rpc-Object-SearchWithMeta-Response-Error-Code"></a>

### Rpc.Object.SearchWithMeta.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



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
| OBJECT_DELETED | 4 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



<a name="anytype-Rpc-Object-SubscribeIds-Response-Error-Code"></a>

### Rpc.Object.SubscribeIds.Response.Error.Code


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



<a name="anytype-Rpc-ObjectType-ListConflictingRelations-Response-Error-Code"></a>

### Rpc.ObjectType.ListConflictingRelations.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| READONLY_OBJECT_TYPE | 3 |  |



<a name="anytype-Rpc-ObjectType-Recommended-FeaturedRelationsSet-Response-Error-Code"></a>

### Rpc.ObjectType.Recommended.FeaturedRelationsSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| READONLY_OBJECT_TYPE | 3 | ... |



<a name="anytype-Rpc-ObjectType-Recommended-RelationsSet-Response-Error-Code"></a>

### Rpc.ObjectType.Recommended.RelationsSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| READONLY_OBJECT_TYPE | 3 | ... |



<a name="anytype-Rpc-ObjectType-Relation-Add-Response-Error-Code"></a>

### Rpc.ObjectType.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| READONLY_OBJECT_TYPE | 3 | ... |



<a name="anytype-Rpc-ObjectType-Relation-Remove-Response-Error-Code"></a>

### Rpc.ObjectType.Relation.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| READONLY_OBJECT_TYPE | 3 | ... |



<a name="anytype-Rpc-ObjectType-ResolveLayoutConflicts-Response-Error-Code"></a>

### Rpc.ObjectType.ResolveLayoutConflicts.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-ObjectType-SetOrder-Response-Error-Code"></a>

### Rpc.ObjectType.SetOrder.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Process-Cancel-Response-Error-Code"></a>

### Rpc.Process.Cancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Process-Subscribe-Response-Error-Code"></a>

### Rpc.Process.Subscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Process-Unsubscribe-Response-Error-Code"></a>

### Rpc.Process.Unsubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Publishing-Create-Response-Error-Code"></a>

### Rpc.Publishing.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_OBJECT | 101 |  |
| NO_SUCH_SPACE | 102 |  |
| LIMIT_EXCEEDED | 103 |  |
| URL_ALREADY_TAKEN | 409 |  |



<a name="anytype-Rpc-Publishing-GetStatus-Response-Error-Code"></a>

### Rpc.Publishing.GetStatus.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_OBJECT | 101 |  |
| NO_SUCH_SPACE | 102 |  |



<a name="anytype-Rpc-Publishing-List-Response-Error-Code"></a>

### Rpc.Publishing.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 102 |  |



<a name="anytype-Rpc-Publishing-PublishStatus"></a>

### Rpc.Publishing.PublishStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| PublishStatusCreated | 0 | PublishStatusCreated means publish is created but not uploaded yet |
| PublishStatusPublished | 1 | PublishStatusCreated means publish is active |



<a name="anytype-Rpc-Publishing-Remove-Response-Error-Code"></a>

### Rpc.Publishing.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_OBJECT | 101 |  |
| NO_SUCH_SPACE | 102 |  |



<a name="anytype-Rpc-Publishing-ResolveUri-Response-Error-Code"></a>

### Rpc.Publishing.ResolveUri.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_URI | 101 |  |



<a name="anytype-Rpc-PushNotification-Mode"></a>

### Rpc.PushNotification.Mode


| Name | Number | Description |
| ---- | ------ | ----------- |
| All | 0 |  |
| Mentions | 1 |  |
| Nothing | 2 |  |



<a name="anytype-Rpc-PushNotification-RegisterToken-Platform"></a>

### Rpc.PushNotification.RegisterToken.Platform


| Name | Number | Description |
| ---- | ------ | ----------- |
| IOS | 0 |  |
| Android | 1 |  |



<a name="anytype-Rpc-PushNotification-RegisterToken-Response-Error-Code"></a>

### Rpc.PushNotification.RegisterToken.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-PushNotification-ResetIds-Response-Error-Code"></a>

### Rpc.PushNotification.ResetIds.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-PushNotification-SetForceModeIds-Response-Error-Code"></a>

### Rpc.PushNotification.SetForceModeIds.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-PushNotification-SetSpaceMode-Response-Error-Code"></a>

### Rpc.PushNotification.SetSpaceMode.Response.Error.Code


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



<a name="anytype-Rpc-Relation-ListWithValue-Response-Error-Code"></a>

### Rpc.Relation.ListWithValue.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Relation-Option-SetOrder-Response-Error-Code"></a>

### Rpc.Relation.Option.SetOrder.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Relation-Options-Response-Error-Code"></a>

### Rpc.Relation.Options.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Space-Delete-Response-Error-Code"></a>

### Rpc.Space.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |
| NOT_SHAREABLE | 105 |  |



<a name="anytype-Rpc-Space-InviteChange-Response-Error-Code"></a>

### Rpc.Space.InviteChange.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| INCORRECT_PERMISSIONS | 105 |  |



<a name="anytype-Rpc-Space-InviteGenerate-Response-Error-Code"></a>

### Rpc.Space.InviteGenerate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |
| NOT_SHAREABLE | 105 |  |



<a name="anytype-Rpc-Space-InviteGetCurrent-Response-Error-Code"></a>

### Rpc.Space.InviteGetCurrent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_ACTIVE_INVITE | 101 |  |



<a name="anytype-Rpc-Space-InviteGetGuest-Response-Error-Code"></a>

### Rpc.Space.InviteGetGuest.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INVALID_SPACE_TYPE | 101 |  |



<a name="anytype-Rpc-Space-InviteRevoke-Response-Error-Code"></a>

### Rpc.Space.InviteRevoke.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| LIMIT_REACHED | 103 |  |
| REQUEST_FAILED | 104 |  |
| NOT_SHAREABLE | 105 |  |



<a name="anytype-Rpc-Space-InviteView-Response-Error-Code"></a>

### Rpc.Space.InviteView.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INVITE_NOT_FOUND | 101 |  |
| INVITE_BAD_CONTENT | 102 |  |
| SPACE_IS_DELETED | 103 |  |



<a name="anytype-Rpc-Space-Join-Response-Error-Code"></a>

### Rpc.Space.Join.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| INVITE_NOT_FOUND | 103 |  |
| INVITE_BAD_CONTENT | 104 |  |
| REQUEST_FAILED | 105 |  |
| LIMIT_REACHED | 106 |  |
| NOT_SHAREABLE | 107 |  |
| DIFFERENT_NETWORK | 108 |  |



<a name="anytype-Rpc-Space-JoinCancel-Response-Error-Code"></a>

### Rpc.Space.JoinCancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |
| NO_SUCH_REQUEST | 105 |  |
| NOT_SHAREABLE | 106 |  |



<a name="anytype-Rpc-Space-LeaveApprove-Response-Error-Code"></a>

### Rpc.Space.LeaveApprove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |
| NO_APPROVE_REQUESTS | 105 |  |
| NOT_SHAREABLE | 106 |  |



<a name="anytype-Rpc-Space-MakeShareable-Response-Error-Code"></a>

### Rpc.Space.MakeShareable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |



<a name="anytype-Rpc-Space-ParticipantPermissionsChange-Response-Error-Code"></a>

### Rpc.Space.ParticipantPermissionsChange.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |
| PARTICIPANT_NOT_FOUND | 105 |  |
| INCORRECT_PERMISSIONS | 106 |  |
| NOT_SHAREABLE | 107 |  |



<a name="anytype-Rpc-Space-ParticipantRemove-Response-Error-Code"></a>

### Rpc.Space.ParticipantRemove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| PARTICIPANT_NOT_FOUND | 103 |  |
| REQUEST_FAILED | 104 |  |
| LIMIT_REACHED | 105 |  |
| NOT_SHAREABLE | 106 |  |



<a name="anytype-Rpc-Space-RequestApprove-Response-Error-Code"></a>

### Rpc.Space.RequestApprove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| NO_SUCH_REQUEST | 103 |  |
| INCORRECT_PERMISSIONS | 104 |  |
| REQUEST_FAILED | 105 |  |
| LIMIT_REACHED | 106 |  |
| NOT_SHAREABLE | 107 |  |



<a name="anytype-Rpc-Space-RequestDecline-Response-Error-Code"></a>

### Rpc.Space.RequestDecline.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |
| NO_SUCH_REQUEST | 105 |  |
| NOT_SHAREABLE | 106 |  |



<a name="anytype-Rpc-Space-SetOrder-Response-Error-Code"></a>

### Rpc.Space.SetOrder.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-Rpc-Space-StopSharing-Response-Error-Code"></a>

### Rpc.Space.StopSharing.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_SUCH_SPACE | 101 |  |
| SPACE_IS_DELETED | 102 |  |
| REQUEST_FAILED | 103 |  |
| LIMIT_REACHED | 104 |  |



<a name="anytype-Rpc-Space-UnsetOrder-Response-Error-Code"></a>

### Rpc.Space.UnsetOrder.Response.Error.Code


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
| APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT | 101 | means the client logged into another account or the account directory has been cleaned |



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



<a name="anytype-Rpc-Workspace-Open-Response-Error-Code"></a>

### Rpc.Workspace.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_LOAD | 100 |  |



<a name="anytype-Rpc-Workspace-Select-Response-Error-Code"></a>

### Rpc.Workspace.Select.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype-Rpc-Workspace-SetInfo-Response-Error-Code"></a>

### Rpc.Workspace.SetInfo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |


 

 

 



<a name="pb_protos_events-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/events.proto



<a name="anytype-Event"></a>

### Event
Event – type of message, that could be sent from a middleware to the
corresponding front-end.


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






<a name="anytype-Event-Account-LinkChallenge"></a>

### Event.Account.LinkChallenge



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| challenge | [string](#string) |  |  |
| clientInfo | [Event.Account.LinkChallenge.ClientInfo](#anytype-Event-Account-LinkChallenge-ClientInfo) |  |  |
| scope | [model.Account.Auth.LocalApiScope](#anytype-model-Account-Auth-LocalApiScope) |  |  |






<a name="anytype-Event-Account-LinkChallenge-ClientInfo"></a>

### Event.Account.LinkChallenge.ClientInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| processName | [string](#string) |  |  |
| processPath | [string](#string) |  |  |
| name | [string](#string) |  |  |
| signatureVerified | [bool](#bool) |  |  |






<a name="anytype-Event-Account-LinkChallengeHide"></a>

### Event.Account.LinkChallengeHide



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| challenge | [string](#string) |  | verify code before hiding to protect from MITM attacks |






<a name="anytype-Event-Account-Show"></a>

### Event.Account.Show
Message, that will be sent to the front on each account found after an
AccountRecoverRequest


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
2. Client C2 receives Event.Block.Add(Block A),
Event.Block.Update(Page.children) B. Partial block load
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
| viewId | [string](#string) |  | view id, client should double check this to make sure client |
| view | [model.Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) |  | doesn&#39;t switch the active view in the middle |






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
| endRelationKey | [string](#string) |  |  |
| groupBackgroundColors | [bool](#bool) |  | Enable backgrounds in groups |
| pageLimit | [int32](#int32) |  | Limit of objects shown in widget |
| defaultTemplateId | [string](#string) |  | Id of template object set default for the view |
| defaultObjectTypeId | [string](#string) |  | Default object type that is chosen for new object created |
| wrapContent | [bool](#bool) |  | within the view

Wrap content in view |






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
Middleware to front end event message, that will be sent on one of this
scenarios: Precondition: user A opened a block
1. User A drops a set of files/pictures/videos
2. User A creates a MediaBlock and drops a single media, that corresponds
to its type.


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
| targetObjectId | [Event.Block.Set.File.TargetObjectId](#anytype-Event-Block-Set-File-TargetObjectId) |  |  |






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






<a name="anytype-Event-Block-Set-File-TargetObjectId"></a>

### Event.Block.Set.File.TargetObjectId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






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
| processor | [Event.Block.Set.Latex.Processor](#anytype-Event-Block-Set-Latex-Processor) |  |  |






<a name="anytype-Event-Block-Set-Latex-Processor"></a>

### Event.Block.Set.Latex.Processor



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Latex.Processor](#anytype-model-Block-Content-Latex-Processor) |  |  |






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
| viewId | [Event.Block.Set.Widget.ViewId](#anytype-Event-Block-Set-Widget-ViewId) |  |  |






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






<a name="anytype-Event-Block-Set-Widget-ViewId"></a>

### Event.Block.Set.Widget.ViewId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype-Event-Chat"></a>

### Event.Chat







<a name="anytype-Event-Chat-Add"></a>

### Event.Chat.Add



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| orderId | [string](#string) |  |  |
| afterOrderId | [string](#string) |  |  |
| message | [model.ChatMessage](#anytype-model-ChatMessage) |  |  |
| subIds | [string](#string) | repeated |  |
| dependencies | [google.protobuf.Struct](#google-protobuf-Struct) | repeated |  |






<a name="anytype-Event-Chat-Delete"></a>

### Event.Chat.Delete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Chat-Update"></a>

### Event.Chat.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| message | [model.ChatMessage](#anytype-model-ChatMessage) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Chat-UpdateMentionReadStatus"></a>

### Event.Chat.UpdateMentionReadStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |
| isRead | [bool](#bool) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Chat-UpdateMessageReadStatus"></a>

### Event.Chat.UpdateMessageReadStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |
| isRead | [bool](#bool) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Chat-UpdateMessageSyncStatus"></a>

### Event.Chat.UpdateMessageSyncStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |
| isSynced | [bool](#bool) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Chat-UpdateReactions"></a>

### Event.Chat.UpdateReactions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| reactions | [model.ChatMessage.Reactions](#anytype-model-ChatMessage-Reactions) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-Chat-UpdateState"></a>

### Event.Chat.UpdateState



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| state | [model.ChatState](#anytype-model-ChatState) |  |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype-Event-File"></a>

### Event.File







<a name="anytype-Event-File-LimitReached"></a>

### Event.File.LimitReached



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| fileId | [string](#string) |  |  |






<a name="anytype-Event-File-LimitUpdated"></a>

### Event.File.LimitUpdated



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| bytesLimit | [uint64](#uint64) |  |  |






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
| spaceId | [string](#string) |  |  |






<a name="anytype-Event-Import"></a>

### Event.Import







<a name="anytype-Event-Import-Finish"></a>

### Event.Import.Finish



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootCollectionID | [string](#string) |  |  |
| objectsCount | [int64](#int64) |  |  |
| importType | [model.Import.Type](#anytype-model-Import-Type) |  |  |






<a name="anytype-Event-Membership"></a>

### Event.Membership







<a name="anytype-Event-Membership-TiersUpdate"></a>

### Event.Membership.TiersUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tiers | [model.MembershipTierData](#anytype-model-MembershipTierData) | repeated |  |






<a name="anytype-Event-Membership-Update"></a>

### Event.Membership.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| data | [model.Membership](#anytype-model-Membership) |  |  |






<a name="anytype-Event-MembershipV2"></a>

### Event.MembershipV2







<a name="anytype-Event-MembershipV2-ProductsUpdate"></a>

### Event.MembershipV2.ProductsUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| products | [model.MembershipV2.Product](#anytype-model-MembershipV2-Product) | repeated |  |






<a name="anytype-Event-MembershipV2-Update"></a>

### Event.MembershipV2.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| data | [model.MembershipV2.Data](#anytype-model-MembershipV2-Data) |  |  |






<a name="anytype-Event-Message"></a>

### Event.Message



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| accountShow | [Event.Account.Show](#anytype-Event-Account-Show) |  |  |
| accountDetails | [Event.Account.Details](#anytype-Event-Account-Details) |  |  |
| accountConfigUpdate | [Event.Account.Config.Update](#anytype-Event-Account-Config-Update) |  |  |
| accountUpdate | [Event.Account.Update](#anytype-Event-Account-Update) |  |  |
| accountLinkChallenge | [Event.Account.LinkChallenge](#anytype-Event-Account-LinkChallenge) |  |  |
| accountLinkChallengeHide | [Event.Account.LinkChallengeHide](#anytype-Event-Account-LinkChallengeHide) |  |  |
| objectDetailsSet | [Event.Object.Details.Set](#anytype-Event-Object-Details-Set) |  |  |
| objectDetailsAmend | [Event.Object.Details.Amend](#anytype-Event-Object-Details-Amend) |  |  |
| objectDetailsUnset | [Event.Object.Details.Unset](#anytype-Event-Object-Details-Unset) |  |  |
| objectRelationsAmend | [Event.Object.Relations.Amend](#anytype-Event-Object-Relations-Amend) |  |  |
| objectRelationsRemove | [Event.Object.Relations.Remove](#anytype-Event-Object-Relations-Remove) |  |  |
| objectRemove | [Event.Object.Remove](#anytype-Event-Object-Remove) |  |  |
| objectClose | [Event.Object.Close](#anytype-Event-Object-Close) |  |  |
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
| blockDataviewSourceSet | [Event.Block.Dataview.SourceSet](#anytype-Event-Block-Dataview-SourceSet) |  | deprecated, source is no longer used |
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
| fileLimitUpdated | [Event.File.LimitUpdated](#anytype-Event-File-LimitUpdated) |  |  |
| notificationSend | [Event.Notification.Send](#anytype-Event-Notification-Send) |  |  |
| notificationUpdate | [Event.Notification.Update](#anytype-Event-Notification-Update) |  |  |
| payloadBroadcast | [Event.Payload.Broadcast](#anytype-Event-Payload-Broadcast) |  |  |
| membershipUpdate | [Event.Membership.Update](#anytype-Event-Membership-Update) |  |  |
| membershipTiersUpdate | [Event.Membership.TiersUpdate](#anytype-Event-Membership-TiersUpdate) |  |  |
| spaceSyncStatusUpdate | [Event.Space.SyncStatus.Update](#anytype-Event-Space-SyncStatus-Update) |  |  |
| p2pStatusUpdate | [Event.P2PStatus.Update](#anytype-Event-P2PStatus-Update) |  |  |
| importFinish | [Event.Import.Finish](#anytype-Event-Import-Finish) |  |  |
| chatAdd | [Event.Chat.Add](#anytype-Event-Chat-Add) |  |  |
| chatUpdate | [Event.Chat.Update](#anytype-Event-Chat-Update) |  |  |
| chatUpdateReactions | [Event.Chat.UpdateReactions](#anytype-Event-Chat-UpdateReactions) |  |  |
| chatUpdateMessageReadStatus | [Event.Chat.UpdateMessageReadStatus](#anytype-Event-Chat-UpdateMessageReadStatus) |  | received to update per-message read status (if needed to |
| chatUpdateMentionReadStatus | [Event.Chat.UpdateMentionReadStatus](#anytype-Event-Chat-UpdateMentionReadStatus) |  | highlight the unread messages in the UI)

received to update per-message mention read status (if needed |
| chatUpdateMessageSyncStatus | [Event.Chat.UpdateMessageSyncStatus](#anytype-Event-Chat-UpdateMessageSyncStatus) |  | to highlight the unread mentions in the UI) |
| chatDelete | [Event.Chat.Delete](#anytype-Event-Chat-Delete) |  |  |
| chatStateUpdate | [Event.Chat.UpdateState](#anytype-Event-Chat-UpdateState) |  | in case new unread messages received or chat state changed |
| membershipV2Update | [Event.MembershipV2.Update](#anytype-Event-MembershipV2-Update) |  |  |
| membershipV2ProductsUpdate | [Event.MembershipV2.ProductsUpdate](#anytype-Event-MembershipV2-ProductsUpdate) |  |  |






<a name="anytype-Event-Notification"></a>

### Event.Notification







<a name="anytype-Event-Notification-Send"></a>

### Event.Notification.Send



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| notification | [model.Notification](#anytype-model-Notification) |  |  |






<a name="anytype-Event-Notification-Update"></a>

### Event.Notification.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| notification | [model.Notification](#anytype-model-Notification) |  |  |






<a name="anytype-Event-Object"></a>

### Event.Object







<a name="anytype-Event-Object-Close"></a>

### Event.Object.Close



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype-Event-Object-Details"></a>

### Event.Object.Details







<a name="anytype-Event-Object-Details-Amend"></a>

### Event.Object.Details.Amend
Amend (i.e. add a new key-value pair or update an existing key-value
pair) existing state


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
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  | can not be a partial state. Should replace client details |
| subIds | [string](#string) | repeated | state |






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






<a name="anytype-Event-P2PStatus"></a>

### Event.P2PStatus







<a name="anytype-Event-P2PStatus-Update"></a>

### Event.P2PStatus.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| status | [Event.P2PStatus.Status](#anytype-Event-P2PStatus-Status) |  |  |
| devicesCounter | [int64](#int64) |  |  |






<a name="anytype-Event-Payload"></a>

### Event.Payload







<a name="anytype-Event-Payload-Broadcast"></a>

### Event.Payload.Broadcast



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| payload | [string](#string) |  |  |






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






<a name="anytype-Event-Space"></a>

### Event.Space







<a name="anytype-Event-Space-SyncStatus"></a>

### Event.Space.SyncStatus







<a name="anytype-Event-Space-SyncStatus-Update"></a>

### Event.Space.SyncStatus.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| status | [Event.Space.Status](#anytype-Event-Space-Status) |  |  |
| network | [Event.Space.Network](#anytype-Event-Space-Network) |  |  |
| error | [Event.Space.SyncError](#anytype-Event-Space-SyncError) |  |  |
| syncingObjectsCounter | [int64](#int64) |  |  |
| notSyncedFilesCounter | [int64](#int64) |  |  |
| uploadingFilesCounter | [int64](#int64) |  |  |






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
Middleware to front end event message, that will be sent in this
scenario: Precondition: user A opened a block
1. User B opens the same block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that opened a block |






<a name="anytype-Event-User-Block-Left"></a>

### Event.User.Block.Left
Middleware to front end event message, that will be sent in this
scenario: Precondition: user A and user B opened the same block
1. User B closes the block
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that left the block |






<a name="anytype-Event-User-Block-SelectRange"></a>

### Event.User.Block.SelectRange
Middleware to front end event message, that will be sent in this
scenario: Precondition: user A and user B opened the same block
1. User B selects some inner blocks
2. User A receives a message about p.1


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype-Event-Account) |  | Account of the user, that selected blocks |
| blockIdsArray | [string](#string) | repeated | Ids of selected blocks. |






<a name="anytype-Event-User-Block-TextRange"></a>

### Event.User.Block.TextRange
Middleware to front end event message, that will be sent in this
scenario: Precondition: user A and user B opened the same block
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
| state | [Model.Process.State](#anytype-Model-Process-State) |  |  |
| progress | [Model.Process.Progress](#anytype-Model-Process-Progress) |  |  |
| spaceId | [string](#string) |  |  |
| dropFiles | [Model.Process.DropFiles](#anytype-Model-Process-DropFiles) |  |  |
| import | [Model.Process.Import](#anytype-Model-Process-Import) |  |  |
| export | [Model.Process.Export](#anytype-Model-Process-Export) |  |  |
| saveFile | [Model.Process.SaveFile](#anytype-Model-Process-SaveFile) |  |  |
| migration | [Model.Process.Migration](#anytype-Model-Process-Migration) |  |  |
| preloadFile | [Model.Process.PreloadFile](#anytype-Model-Process-PreloadFile) |  |  |
| error | [string](#string) |  |  |






<a name="anytype-Model-Process-DropFiles"></a>

### Model.Process.DropFiles







<a name="anytype-Model-Process-Export"></a>

### Model.Process.Export







<a name="anytype-Model-Process-Import"></a>

### Model.Process.Import







<a name="anytype-Model-Process-Migration"></a>

### Model.Process.Migration







<a name="anytype-Model-Process-PreloadFile"></a>

### Model.Process.PreloadFile







<a name="anytype-Model-Process-Progress"></a>

### Model.Process.Progress



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total | [int64](#int64) |  |  |
| done | [int64](#int64) |  |  |
| message | [string](#string) |  |  |






<a name="anytype-Model-Process-SaveFile"></a>

### Model.Process.SaveFile







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



<a name="anytype-Event-P2PStatus-Status"></a>

### Event.P2PStatus.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| NotConnected | 0 |  |
| NotPossible | 1 |  |
| Connected | 2 |  |
| Restricted | 3 | only for ios for now, fallback to NotPossible if not |



<a name="anytype-Event-Space-Network"></a>

### Event.Space.Network


| Name | Number | Description |
| ---- | ------ | ----------- |
| Anytype | 0 |  |
| SelfHost | 1 |  |
| LocalOnly | 2 |  |



<a name="anytype-Event-Space-Status"></a>

### Event.Space.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| Synced | 0 |  |
| Syncing | 1 |  |
| Error | 2 |  |
| Offline | 3 |  |
| NetworkNeedsUpdate | 4 |  |



<a name="anytype-Event-Space-SyncError"></a>

### Event.Space.SyncError


| Name | Number | Description |
| ---- | ------ | ----------- |
| Null | 0 |  |
| StorageLimitExceed | 1 |  |
| IncompatibleVersion | 2 |  |
| NetworkError | 3 |  |



<a name="anytype-Event-Status-Thread-SyncStatus"></a>

### Event.Status.Thread.SyncStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Offline | 1 |  |
| Syncing | 2 |  |
| Synced | 3 |  |
| Failed | 4 |  |
| IncompatibleVersion | 5 |  |
| NetworkNeedsUpdate | 6 |  |



<a name="anytype-Model-Process-State"></a>

### Model.Process.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Running | 1 |  |
| Done | 2 |  |
| Canceled | 3 |  |
| Error | 4 |  |


 

 

 



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
| startingPage | [string](#string) |  | deprecated |
| widgets | [WidgetBlock](#anytype-WidgetBlock) | repeated |  |






<a name="anytype-SnapshotWithType"></a>

### SnapshotWithType



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| sbType | [model.SmartBlockType](#anytype-model-SmartBlockType) |  |  |
| snapshot | [Change.Snapshot](#anytype-Change-Snapshot) |  |  |






<a name="anytype-WidgetBlock"></a>

### WidgetBlock



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| layout | [model.Block.Content.Widget.Layout](#anytype-model-Block-Content-Widget-Layout) |  |  |
| targetObjectId | [string](#string) |  |  |
| objectLimit | [int32](#int32) |  |  |





 

 

 

 



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
| objectTypeUrls | [string](#string) | repeated | DEPRECATED |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| relations | [Relation](#anytype-model-Relation) | repeated | DEPRECATED |
| snippet | [string](#string) |  |  |
| hasInboundLinks | [bool](#bool) |  | DEPRECATED |






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
| fulltextRebuild | [int32](#int32) |  | DEPRECATED increased in order to perform fulltext indexing for all type of objects (useful when we change fulltext config) |
| fulltextErase | [int32](#int32) |  | DEPRECATED remove all the fulltext indexes and add to reindex queue after |
| bundledTemplates | [string](#string) |  |  |
| bundledObjects | [int32](#int32) |  | anytypeProfile and maybe some others in the feature |
| filestoreKeysForceReindexCounter | [int32](#int32) |  |  |
| areOldFilesRemoved | [bool](#bool) |  |  |
| areDeletedObjectsReindexed | [bool](#bool) |  | DEPRECATED |
| linksErase | [int32](#int32) |  |  |
| marketplaceForceReindexCounter | [int32](#int32) |  |  |
| reindexDeletedObjects | [int32](#int32) |  |  |
| reindexParticipants | [int32](#int32) |  |  |
| reindexChats | [int32](#int32) |  |  |





 

 

 

 



<a name="pkg_lib_pb_model_protos_models-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pkg/lib/pb/model/protos/models.proto



<a name="anytype-model-Account"></a>

### Account
Contains basic information about a user account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | User&#39;s thread id |
| config | [Account.Config](#anytype-model-Account-Config) |  |  |
| status | [Account.Status](#anytype-model-Account-Status) |  |  |
| info | [Account.Info](#anytype-model-Account-Info) |  |  |






<a name="anytype-model-Account-Auth"></a>

### Account.Auth







<a name="anytype-model-Account-Auth-AppInfo"></a>

### Account.Auth.AppInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| appHash | [string](#string) |  |  |
| appName | [string](#string) |  | either from process or specified manually when creating |
| appKey | [string](#string) |  |  |
| createdAt | [int64](#int64) |  |  |
| expireAt | [int64](#int64) |  |  |
| scope | [Account.Auth.LocalApiScope](#anytype-model-Account-Auth-LocalApiScope) |  |  |
| isActive | [bool](#bool) |  |  |






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
| workspaceObjectId | [string](#string) |  | workspace object id. used for space-level chat |
| spaceChatId | [string](#string) |  | space-level chat if exists |
| deviceId | [string](#string) |  |  |
| accountSpaceId | [string](#string) |  | the first created private space. It&#39;s filled only when account is created |
| widgetsId | [string](#string) |  |  |
| spaceViewId | [string](#string) |  |  |
| techSpaceId | [string](#string) |  |  |
| gatewayUrl | [string](#string) |  | gateway url for fetching static files |
| localStoragePath | [string](#string) |  | path to local storage |
| timeZone | [string](#string) |  | time zone from config |
| analyticsId | [string](#string) |  |  |
| networkId | [string](#string) |  | network id to which anytype is connected |
| ethereumAddress | [string](#string) |  | we have Any PK AND Ethereum PK derived from one seed phrase |
| metaDataKey | [string](#string) |  | symmetric key for encrypting profile metadata |






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
| chat | [Block.Content.Chat](#anytype-model-Block-Content-Chat) |  |  |






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






<a name="anytype-model-Block-Content-Chat"></a>

### Block.Content.Chat







<a name="anytype-model-Block-Content-Dataview"></a>

### Block.Content.Dataview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated | can be set for detached(without TargetObjectId) inline sets |
| views | [Block.Content.Dataview.View](#anytype-model-Block-Content-Dataview-View) | repeated |  |
| activeView | [string](#string) |  | do not generate changes for this field |
| relations | [Relation](#anytype-model-Relation) | repeated | deprecated |
| groupOrders | [Block.Content.Dataview.GroupOrder](#anytype-model-Block-Content-Dataview-GroupOrder) | repeated |  |
| objectOrders | [Block.Content.Dataview.ObjectOrder](#anytype-model-Block-Content-Dataview-ObjectOrder) | repeated |  |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated |  |
| TargetObjectId | [string](#string) |  | empty for original set/collection objects and for detached inline sets |
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
| nestedFilters | [Block.Content.Dataview.Filter](#anytype-model-Block-Content-Dataview-Filter) | repeated |  |






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
| dateIncludeTime | [bool](#bool) |  | bool isReadOnly = 4; // deprecated

deprecated |
| timeFormat | [Block.Content.Dataview.Relation.TimeFormat](#anytype-model-Block-Content-Dataview-Relation-TimeFormat) |  | deprecated |
| dateFormat | [Block.Content.Dataview.Relation.DateFormat](#anytype-model-Block-Content-Dataview-Relation-DateFormat) |  | deprecated |
| formula | [Block.Content.Dataview.Relation.FormulaType](#anytype-model-Block-Content-Dataview-Relation-FormulaType) |  |  |
| align | [Block.Align](#anytype-model-Block-Align) |  |  |






<a name="anytype-model-Block-Content-Dataview-Sort"></a>

### Block.Content.Dataview.Sort



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| RelationKey | [string](#string) |  |  |
| type | [Block.Content.Dataview.Sort.Type](#anytype-model-Block-Content-Dataview-Sort-Type) |  |  |
| customOrder | [google.protobuf.Value](#google-protobuf-Value) | repeated |  |
| format | [RelationFormat](#anytype-model-RelationFormat) |  |  |
| includeTime | [bool](#bool) |  |  |
| id | [string](#string) |  |  |
| emptyPlacement | [Block.Content.Dataview.Sort.EmptyType](#anytype-model-Block-Content-Dataview-Sort-EmptyType) |  |  |
| noCollate | [bool](#bool) |  |  |






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
| pageLimit | [int32](#int32) |  | Limit of objects shown in widget |
| defaultTemplateId | [string](#string) |  | Default template that is chosen for new object created within the view |
| defaultObjectTypeId | [string](#string) |  | Default object type that is chosen for new object created within the view |
| endRelationKey | [string](#string) |  | Group view by this relationKey |
| wrapContent | [bool](#bool) |  | Wrap content in view |






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
| targetObjectId | [string](#string) |  |  |
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
| processor | [Block.Content.Latex.Processor](#anytype-model-Block-Content-Latex-Processor) |  |  |






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
| viewId | [string](#string) |  |  |
| autoAdded | [bool](#bool) |  |  |






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






<a name="anytype-model-ChatMessage"></a>

### ChatMessage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Unique message identifier |
| orderId | [string](#string) |  | Lexicographical id for message in order of tree traversal |
| creator | [string](#string) |  | Identifier for the message creator |
| createdAt | [int64](#int64) |  |  |
| modifiedAt | [int64](#int64) |  |  |
| stateId | [string](#string) |  | stateId is ever-increasing id (BSON ObjectId) for this message. Unlike orderId, this ID is ordered by the time messages are added. For example, it&#39;s useful to prevent accidental reading of messages from the past when a ChatReadMessages request is sent: a message from the past may appear, but the client is still unaware of it |
| replyToMessageId | [string](#string) |  | Identifier for the message being replied to |
| message | [ChatMessage.MessageContent](#anytype-model-ChatMessage-MessageContent) |  | Message content |
| attachments | [ChatMessage.Attachment](#anytype-model-ChatMessage-Attachment) | repeated | Attachments slice |
| reactions | [ChatMessage.Reactions](#anytype-model-ChatMessage-Reactions) |  | Reactions to the message |
| read | [bool](#bool) |  | Message read status |
| mentionRead | [bool](#bool) |  |  |
| hasMention | [bool](#bool) |  |  |
| synced | [bool](#bool) |  |  |






<a name="anytype-model-ChatMessage-Attachment"></a>

### ChatMessage.Attachment



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target | [string](#string) |  | Identifier for the attachment object |
| type | [ChatMessage.Attachment.AttachmentType](#anytype-model-ChatMessage-Attachment-AttachmentType) |  | Type of attachment |






<a name="anytype-model-ChatMessage-MessageContent"></a>

### ChatMessage.MessageContent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  | The text content of the message part |
| style | [Block.Content.Text.Style](#anytype-model-Block-Content-Text-Style) |  | The style/type of the message part |
| marks | [Block.Content.Text.Mark](#anytype-model-Block-Content-Text-Mark) | repeated | List of marks applied to the text |






<a name="anytype-model-ChatMessage-Reactions"></a>

### ChatMessage.Reactions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| reactions | [ChatMessage.Reactions.ReactionsEntry](#anytype-model-ChatMessage-Reactions-ReactionsEntry) | repeated | Map of emoji to list of user IDs |






<a name="anytype-model-ChatMessage-Reactions-IdentityList"></a>

### ChatMessage.Reactions.IdentityList



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated | List of user IDs |






<a name="anytype-model-ChatMessage-Reactions-ReactionsEntry"></a>

### ChatMessage.Reactions.ReactionsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [ChatMessage.Reactions.IdentityList](#anytype-model-ChatMessage-Reactions-IdentityList) |  |  |






<a name="anytype-model-ChatState"></a>

### ChatState



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [ChatState.UnreadState](#anytype-model-ChatState-UnreadState) |  | unread messages |
| mentions | [ChatState.UnreadState](#anytype-model-ChatState-UnreadState) |  | unread mentions |
| lastStateId | [string](#string) |  | reflects the state of the chat db at the moment of sending response/event that includes this state |
| order | [int64](#int64) |  | Order is serial number of this state. Client should apply chat state only if its order is greater than previously saved order |






<a name="anytype-model-ChatState-UnreadState"></a>

### ChatState.UnreadState



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| oldestOrderId | [string](#string) |  | oldest(in the lex sorting) unread message order id. Client should ALWAYS scroll through unread messages from the oldest to the newest |
| counter | [int32](#int32) |  | total number of unread messages |






<a name="anytype-model-Detail"></a>

### Detail



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google-protobuf-Value) |  | NUll - removes key |






<a name="anytype-model-DeviceInfo"></a>

### DeviceInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| addDate | [int64](#int64) |  |  |
| archived | [bool](#bool) |  |  |
| isConnected | [bool](#bool) |  |  |






<a name="anytype-model-Export"></a>

### Export







<a name="anytype-model-FileEncryptionKey"></a>

### FileEncryptionKey



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  |  |
| key | [string](#string) |  |  |






<a name="anytype-model-FileInfo"></a>

### FileInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fileId | [string](#string) |  |  |
| encryptionKeys | [FileEncryptionKey](#anytype-model-FileEncryptionKey) | repeated |  |






<a name="anytype-model-IdentityProfile"></a>

### IdentityProfile



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identity | [string](#string) |  |  |
| name | [string](#string) |  |  |
| iconCid | [string](#string) |  |  |
| iconEncryptionKeys | [FileEncryptionKey](#anytype-model-FileEncryptionKey) | repeated |  |
| description | [string](#string) |  |  |
| globalName | [string](#string) |  |  |






<a name="anytype-model-IdentityProfileWithKey"></a>

### IdentityProfileWithKey



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identityProfile | [IdentityProfile](#anytype-model-IdentityProfile) |  |  |
| requestMetadata | [bytes](#bytes) |  |  |






<a name="anytype-model-Import"></a>

### Import







<a name="anytype-model-InternalFlag"></a>

### InternalFlag



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [InternalFlag.Value](#anytype-model-InternalFlag-Value) |  |  |






<a name="anytype-model-Invite"></a>

### Invite



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| payload | [bytes](#bytes) |  |  |
| signature | [bytes](#bytes) |  |  |






<a name="anytype-model-InvitePayload"></a>

### InvitePayload



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| creatorIdentity | [string](#string) |  |  |
| creatorName | [string](#string) |  |  |
| creatorIconCid | [string](#string) |  |  |
| creatorIconEncryptionKeys | [FileEncryptionKey](#anytype-model-FileEncryptionKey) | repeated |  |
| aclKey | [bytes](#bytes) |  |  |
| spaceId | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |
| spaceIconCid | [string](#string) |  |  |
| spaceIconOption | [uint32](#uint32) |  |  |
| spaceUxType | [uint32](#uint32) |  |  |
| spaceIconEncryptionKeys | [FileEncryptionKey](#anytype-model-FileEncryptionKey) | repeated |  |
| inviteType | [InviteType](#anytype-model-InviteType) |  |  |
| guestKey | [bytes](#bytes) |  |  |






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






<a name="anytype-model-ManifestInfo"></a>

### ManifestInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema | [string](#string) |  |  |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| author | [string](#string) |  |  |
| license | [string](#string) |  |  |
| title | [string](#string) |  |  |
| description | [string](#string) |  |  |
| screenshots | [string](#string) | repeated |  |
| downloadLink | [string](#string) |  |  |
| fileSize | [int32](#int32) |  |  |
| categories | [string](#string) | repeated |  |
| language | [string](#string) |  |  |






<a name="anytype-model-Membership"></a>

### Membership



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| tier | [uint32](#uint32) |  | it was Tier before, changed to int32 to allow dynamic values |
| status | [Membership.Status](#anytype-model-Membership-Status) |  |  |
| dateStarted | [uint64](#uint64) |  |  |
| dateEnds | [uint64](#uint64) |  |  |
| isAutoRenew | [bool](#bool) |  |  |
| paymentMethod | [Membership.PaymentMethod](#anytype-model-Membership-PaymentMethod) |  |  |
| nsName | [string](#string) |  | can be empty if user did not ask for any name |
| nsNameType | [NameserviceNameType](#anytype-model-NameserviceNameType) |  |  |
| userEmail | [string](#string) |  | if the email was verified by the user or set during the checkout - it will be here |
| subscribeToNewsletter | [bool](#bool) |  |  |






<a name="anytype-model-MembershipTierData"></a>

### MembershipTierData



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [uint32](#uint32) |  | this is a unique Payment Node ID of the tier WARNING: tiers can be sorted differently, not according to their IDs! |
| name | [string](#string) |  | localazied name of the tier |
| description | [string](#string) |  | just a short technical description |
| isTest | [bool](#bool) |  | is this tier for testing and debugging only? |
| periodType | [MembershipTierData.PeriodType](#anytype-model-MembershipTierData-PeriodType) |  | how long is the period of the subscription |
| periodValue | [uint32](#uint32) |  | i.e. &#34;5 days&#34; or &#34;3 years&#34; |
| priceStripeUsdCents | [uint32](#uint32) |  | this one is a price we use ONLY on Stripe platform |
| anyNamesCountIncluded | [uint32](#uint32) |  | number of ANY NS names that this tier includes also in the &#34;features&#34; list (see below) |
| anyNameMinLength | [uint32](#uint32) |  | somename.any - is of len 8 |
| features | [string](#string) | repeated | localized strings for the features |
| colorStr | [string](#string) |  | green, blue, red, purple, custom |
| stripeProductId | [string](#string) |  | Stripe platform-specific data: |
| stripeManageUrl | [string](#string) |  |  |
| iosProductId | [string](#string) |  | iOS platform-specific data: |
| iosManageUrl | [string](#string) |  |  |
| androidProductId | [string](#string) |  | Android platform-specific data: |
| androidManageUrl | [string](#string) |  |  |
| offer | [string](#string) |  | &#34;limited offer&#34; or somehing like that |






<a name="anytype-model-MembershipV2"></a>

### MembershipV2







<a name="anytype-model-MembershipV2-Amount"></a>

### MembershipV2.Amount



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| currency | [string](#string) |  | ISO 4217 currency code |
| amountCents | [int64](#int64) |  | $0.01 = 1 $1.00 = 100 also supports negative amounts! some invoices can have negatice amount (refund) |






<a name="anytype-model-MembershipV2-Cart"></a>

### MembershipV2.Cart



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| products | [MembershipV2.CartProduct](#anytype-model-MembershipV2-CartProduct) | repeated | if you add Nx the same product - it will be Nx in the &#39;products&#39; array, i.e: each product instance has a unique index |
| total | [MembershipV2.Amount](#anytype-model-MembershipV2-Amount) |  | total amount of the cart (including discounts, etc) |
| totalNextInvoice | [MembershipV2.Amount](#anytype-model-MembershipV2-Amount) |  | in case you are paying in the middle of the period (for existing customers) the next invoice amount will also be generated |
| nextInvoiceDate | [uint64](#uint64) |  |  |






<a name="anytype-model-MembershipV2-CartProduct"></a>

### MembershipV2.CartProduct



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| product | [MembershipV2.Product](#anytype-model-MembershipV2-Product) |  |  |
| isYearly | [bool](#bool) |  | otherwise - monthly |
| remove | [bool](#bool) |  | set to true if you want to remove this item from the customer it&#39;s like setting -1 to some product |






<a name="anytype-model-MembershipV2-Data"></a>

### MembershipV2.Data



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| products | [MembershipV2.PurchasedProduct](#anytype-model-MembershipV2-PurchasedProduct) | repeated |  |
| nextInvoice | [MembershipV2.Invoice](#anytype-model-MembershipV2-Invoice) |  |  |
| teamOwnerID | [string](#string) |  |  |
| paymentProvider | [MembershipV2.PaymentProvider](#anytype-model-MembershipV2-PaymentProvider) |  |  |






<a name="anytype-model-MembershipV2-Features"></a>

### MembershipV2.Features



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| storageBytes | [uint64](#uint64) |  |  |
| spaceReaders | [uint32](#uint32) |  |  |
| spaceWriters | [uint32](#uint32) |  |  |
| sharedSpaces | [uint32](#uint32) |  |  |
| teamSeats | [uint32](#uint32) |  |  |
| anyNameCount | [uint32](#uint32) |  |  |
| anyNameMinLen | [uint32](#uint32) |  |  |
| privateSpaces | [uint32](#uint32) |  |  |






<a name="anytype-model-MembershipV2-Invoice"></a>

### MembershipV2.Invoice



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| date | [uint64](#uint64) |  |  |
| total | [MembershipV2.Amount](#anytype-model-MembershipV2-Amount) |  |  |






<a name="anytype-model-MembershipV2-Product"></a>

### MembershipV2.Product



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| description | [string](#string) |  |  |
| isTopLevel | [bool](#bool) |  |  |
| isHidden | [bool](#bool) |  |  |
| isIntro | [bool](#bool) |  | isIntro flag can be used as follows:

1. if current user&#39;s top level product has isIntro flag -&gt; then you&#39;d rather show a FULL list of all products to enable upgrading from CURRENT product 2. but if current user&#39;s top level product has no isIntro flag -&gt; then it means that this plan was aquired and user need to control it. then show &#34;second screen&#34; to control that product instead |
| isUpgradeable | [bool](#bool) |  | isUpgradeable can be used as follows:

if current user&#39;s top level product has isUpgradeable flag -&gt; show incentives to buy something else |
| pricesYearly | [MembershipV2.Amount](#anytype-model-MembershipV2-Amount) | repeated |  |
| pricesMonthly | [MembershipV2.Amount](#anytype-model-MembershipV2-Amount) | repeated |  |
| colorStr | [string](#string) |  | green, blue, red, purple, custom, etc |
| offer | [string](#string) |  |  |
| features | [MembershipV2.Features](#anytype-model-MembershipV2-Features) |  |  |






<a name="anytype-model-MembershipV2-ProductStatus"></a>

### MembershipV2.ProductStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [MembershipV2.ProductStatus.Status](#anytype-model-MembershipV2-ProductStatus-Status) |  |  |






<a name="anytype-model-MembershipV2-PurchaseInfo"></a>

### MembershipV2.PurchaseInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dateStarted | [uint64](#uint64) |  |  |
| dateEnds | [uint64](#uint64) |  |  |
| isAutoRenew | [bool](#bool) |  |  |
| period | [MembershipV2.Period](#anytype-model-MembershipV2-Period) |  |  |






<a name="anytype-model-MembershipV2-PurchasedProduct"></a>

### MembershipV2.PurchasedProduct



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| product | [MembershipV2.Product](#anytype-model-MembershipV2-Product) |  |  |
| purchaseInfo | [MembershipV2.PurchaseInfo](#anytype-model-MembershipV2-PurchaseInfo) |  |  |
| productStatus | [MembershipV2.ProductStatus](#anytype-model-MembershipV2-ProductStatus) |  |  |






<a name="anytype-model-Metadata"></a>

### Metadata



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identity | [Metadata.Payload.IdentityPayload](#anytype-model-Metadata-Payload-IdentityPayload) |  |  |






<a name="anytype-model-Metadata-Payload"></a>

### Metadata.Payload







<a name="anytype-model-Metadata-Payload-IdentityPayload"></a>

### Metadata.Payload.IdentityPayload



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| profileSymKey | [bytes](#bytes) |  |  |






<a name="anytype-model-Notification"></a>

### Notification



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| createTime | [int64](#int64) |  |  |
| status | [Notification.Status](#anytype-model-Notification-Status) |  |  |
| isLocal | [bool](#bool) |  |  |
| import | [Notification.Import](#anytype-model-Notification-Import) |  |  |
| export | [Notification.Export](#anytype-model-Notification-Export) |  |  |
| galleryImport | [Notification.GalleryImport](#anytype-model-Notification-GalleryImport) |  |  |
| requestToJoin | [Notification.RequestToJoin](#anytype-model-Notification-RequestToJoin) |  |  |
| test | [Notification.Test](#anytype-model-Notification-Test) |  |  |
| participantRequestApproved | [Notification.ParticipantRequestApproved](#anytype-model-Notification-ParticipantRequestApproved) |  |  |
| requestToLeave | [Notification.RequestToLeave](#anytype-model-Notification-RequestToLeave) |  |  |
| participantRemove | [Notification.ParticipantRemove](#anytype-model-Notification-ParticipantRemove) |  |  |
| participantRequestDecline | [Notification.ParticipantRequestDecline](#anytype-model-Notification-ParticipantRequestDecline) |  |  |
| participantPermissionsChange | [Notification.ParticipantPermissionsChange](#anytype-model-Notification-ParticipantPermissionsChange) |  |  |
| space | [string](#string) |  |  |
| aclHeadId | [string](#string) |  |  |






<a name="anytype-model-Notification-Export"></a>

### Notification.Export



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| errorCode | [Notification.Export.Code](#anytype-model-Notification-Export-Code) |  |  |
| exportType | [Export.Format](#anytype-model-Export-Format) |  |  |






<a name="anytype-model-Notification-GalleryImport"></a>

### Notification.GalleryImport



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| processId | [string](#string) |  |  |
| errorCode | [Import.ErrorCode](#anytype-model-Import-ErrorCode) |  |  |
| spaceId | [string](#string) |  |  |
| name | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-Import"></a>

### Notification.Import



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| processId | [string](#string) |  |  |
| errorCode | [Import.ErrorCode](#anytype-model-Import-ErrorCode) |  |  |
| importType | [Import.Type](#anytype-model-Import-Type) |  |  |
| spaceId | [string](#string) |  |  |
| name | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-ParticipantPermissionsChange"></a>

### Notification.ParticipantPermissionsChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| permissions | [ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-ParticipantRemove"></a>

### Notification.ParticipantRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identity | [string](#string) |  |  |
| identityName | [string](#string) |  |  |
| identityIcon | [string](#string) |  |  |
| spaceId | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-ParticipantRequestApproved"></a>

### Notification.ParticipantRequestApproved



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| permissions | [ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-ParticipantRequestDecline"></a>

### Notification.ParticipantRequestDecline



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-RequestToJoin"></a>

### Notification.RequestToJoin



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| identity | [string](#string) |  |  |
| identityName | [string](#string) |  |  |
| identityIcon | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-RequestToLeave"></a>

### Notification.RequestToLeave



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceId | [string](#string) |  |  |
| identity | [string](#string) |  |  |
| identityName | [string](#string) |  |  |
| identityIcon | [string](#string) |  |  |
| spaceName | [string](#string) |  |  |






<a name="anytype-model-Notification-Test"></a>

### Notification.Test







<a name="anytype-model-Object"></a>

### Object







<a name="anytype-model-Object-ChangePayload"></a>

### Object.ChangePayload



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| smartBlockType | [SmartBlockType](#anytype-model-SmartBlockType) |  |  |
| key | [string](#string) |  |  |
| data | [bytes](#bytes) |  |  |






<a name="anytype-model-ObjectType"></a>

### ObjectType



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  | leave empty in case you want to create the new one |
| name | [string](#string) |  | name of objectType in singular form (can be localized for bundled types) |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated | cannot contain more than one Relation with the same RelationType |
| layout | [ObjectType.Layout](#anytype-model-ObjectType-Layout) |  |  |
| iconEmoji | [string](#string) |  | emoji symbol |
| description | [string](#string) |  |  |
| hidden | [bool](#bool) |  |  |
| readonly | [bool](#bool) |  |  |
| types | [SmartBlockType](#anytype-model-SmartBlockType) | repeated |  |
| isArchived | [bool](#bool) |  | sets locally to hide object type from set and some other places |
| installedByDefault | [bool](#bool) |  |  |
| key | [string](#string) |  | name of objectType (can be localized for bundled types) |
| revision | [int64](#int64) |  | revision of system objectType. Used to check if we should change type content or not |
| restrictObjectCreation | [bool](#bool) |  | restricts creating objects of this type for users |
| iconColor | [int64](#int64) |  | color of object type icon |
| iconName | [string](#string) |  | name of object type icon |
| pluralName | [string](#string) |  | name of objectType in plural form (can be localized for bundled types) |






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
| blockParticipants | [ObjectView.BlockParticipant](#anytype-model-ObjectView-BlockParticipant) | repeated |  |






<a name="anytype-model-ObjectView-BlockParticipant"></a>

### ObjectView.BlockParticipant



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| participantId | [string](#string) |  |  |






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






<a name="anytype-model-ParticipantPermissionChange"></a>

### ParticipantPermissionChange



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| identity | [string](#string) |  |  |
| perms | [ParticipantPermissions](#anytype-model-ParticipantPermissions) |  |  |






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

deprecated, to be removed |
| creator | [string](#string) |  | creator profile id |
| revision | [int64](#int64) |  | revision of system relation. Used to check if we should change relation content or not |
| includeTime | [bool](#bool) |  | indicates whether value of relation with date format should be processed with seconds precision |






<a name="anytype-model-Relation-Option"></a>

### Relation.Option



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | id generated automatically if omitted |
| text | [string](#string) |  |  |
| color | [string](#string) |  | stored |
| relationKey | [string](#string) |  | 4 is reserved for old relation format

stored |
| orderId | [string](#string) |  | lexicographic id of relation option for ordering |






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






<a name="anytype-model-Search"></a>

### Search







<a name="anytype-model-Search-Meta"></a>

### Search.Meta



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| highlight | [string](#string) |  | truncated text with highlights |
| highlightRanges | [Range](#anytype-model-Range) | repeated | ranges of the highlight in the text (using utf-16 runes) |
| blockId | [string](#string) |  | block id where the highlight has been found |
| relationKey | [string](#string) |  | relation key of the block where the highlight has been found |
| relationDetails | [google.protobuf.Struct](#google-protobuf-Struct) |  | contains details for dependent object. E.g. relation option or type. todo: rename to dependantDetails |






<a name="anytype-model-Search-Result"></a>

### Search.Result



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| meta | [Search.Meta](#anytype-model-Search-Meta) | repeated | meta information about the search result |






<a name="anytype-model-SmartBlockSnapshotBase"></a>

### SmartBlockSnapshotBase



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [Block](#anytype-model-Block) | repeated |  |
| details | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| fileKeys | [google.protobuf.Struct](#google-protobuf-Struct) |  | **Deprecated.**  |
| extraRelations | [Relation](#anytype-model-Relation) | repeated | **Deprecated.**  |
| objectTypes | [string](#string) | repeated |  |
| collections | [google.protobuf.Struct](#google-protobuf-Struct) |  |  |
| removedCollectionKeys | [string](#string) | repeated |  |
| relationLinks | [RelationLink](#anytype-model-RelationLink) | repeated |  |
| key | [string](#string) |  | only used for pb backup purposes, ignored in other cases |
| originalCreatedTimestamp | [int64](#int64) |  | ignored in import/export in favor of createdDate relation. Used to store original user-side object creation timestamp |
| fileInfo | [FileInfo](#anytype-model-FileInfo) |  |  |






<a name="anytype-model-SpaceObjectHeader"></a>

### SpaceObjectHeader



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| spaceID | [string](#string) |  |  |





 


<a name="anytype-model-Account-Auth-LocalApiScope"></a>

### Account.Auth.LocalApiScope


| Name | Number | Description |
| ---- | ------ | ----------- |
| Limited | 0 | Used in WebClipper; AccountSelect(to be deprecated), ObjectSearch, ObjectShow, ObjectCreate, ObjectCreateFromURL, BlockPreview, BlockPaste, BroadcastPayloadEvent |
| JsonAPI | 1 | JSON API only, no direct grpc api calls allowed |
| Full | 2 | Full access, not available via LocalLink |



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
| AlignJustify | 3 |  |



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
| No | 0 |  |
| Or | 1 |  |
| And | 2 |  |



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
| LastYear | 12 |  |
| CurrentYear | 13 |  |
| NextYear | 14 |  |



<a name="anytype-model-Block-Content-Dataview-Relation-DateFormat"></a>

### Block.Content.Dataview.Relation.DateFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| MonthAbbrBeforeDay | 0 | Jul 30, 2020 |
| MonthAbbrAfterDay | 1 | 30 Jul 2020 |
| Short | 2 | 30/07/2020 |
| ShortUS | 3 | 07/30/2020 |
| ISO | 4 | 2020-07-30 |



<a name="anytype-model-Block-Content-Dataview-Relation-FormulaType"></a>

### Block.Content.Dataview.Relation.FormulaType


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Count | 1 |  |
| CountValue | 2 |  |
| CountDistinct | 3 |  |
| CountEmpty | 4 |  |
| CountNotEmpty | 5 |  |
| PercentEmpty | 6 |  |
| PercentNotEmpty | 7 |  |
| MathSum | 8 |  |
| MathAverage | 9 |  |
| MathMedian | 10 |  |
| MathMin | 11 |  |
| MathMax | 12 |  |
| Range | 13 |  |



<a name="anytype-model-Block-Content-Dataview-Relation-TimeFormat"></a>

### Block.Content.Dataview.Relation.TimeFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| Format12 | 0 |  |
| Format24 | 1 |  |



<a name="anytype-model-Block-Content-Dataview-Sort-EmptyType"></a>

### Block.Content.Dataview.Sort.EmptyType


| Name | Number | Description |
| ---- | ------ | ----------- |
| NotSpecified | 0 |  |
| Start | 1 |  |
| End | 2 |  |



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
| Calendar | 4 |  |
| Graph | 5 |  |



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



<a name="anytype-model-Block-Content-Latex-Processor"></a>

### Block.Content.Latex.Processor


| Name | Number | Description |
| ---- | ------ | ----------- |
| Latex | 0 |  |
| Mermaid | 1 |  |
| Chart | 2 |  |
| Youtube | 3 |  |
| Vimeo | 4 |  |
| Soundcloud | 5 |  |
| GoogleMaps | 6 |  |
| Miro | 7 |  |
| Figma | 8 |  |
| Twitter | 9 |  |
| OpenStreetMap | 10 |  |
| Reddit | 11 |  |
| Facebook | 12 |  |
| Instagram | 13 |  |
| Telegram | 14 |  |
| GithubGist | 15 |  |
| Codepen | 16 |  |
| Bilibili | 17 |  |
| Excalidraw | 18 |  |
| Kroki | 19 |  |
| Graphviz | 20 |  |
| Sketchfab | 21 |  |
| Image | 22 |  |
| Drawio | 23 |  |
| Spotify | 24 |  |



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
| View | 4 |  |



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



<a name="anytype-model-ChatMessage-Attachment-AttachmentType"></a>

### ChatMessage.Attachment.AttachmentType


| Name | Number | Description |
| ---- | ------ | ----------- |
| FILE | 0 | File attachment |
| IMAGE | 1 | Image attachment |
| LINK | 2 | Link attachment |



<a name="anytype-model-DeviceNetworkType"></a>

### DeviceNetworkType


| Name | Number | Description |
| ---- | ------ | ----------- |
| WIFI | 0 |  |
| CELLULAR | 1 |  |
| NOT_CONNECTED | 2 |  |



<a name="anytype-model-Export-Format"></a>

### Export.Format


| Name | Number | Description |
| ---- | ------ | ----------- |
| Markdown | 0 |  |
| Protobuf | 1 |  |
| JSON | 2 |  |
| DOT | 3 |  |
| SVG | 4 |  |
| GRAPH_JSON | 5 |  |



<a name="anytype-model-FileIndexingStatus"></a>

### FileIndexingStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| NotIndexed | 0 |  |
| Indexed | 1 |  |
| NotFound | 2 |  |



<a name="anytype-model-ImageKind"></a>

### ImageKind


| Name | Number | Description |
| ---- | ------ | ----------- |
| Basic | 0 |  |
| Cover | 1 |  |
| Icon | 2 |  |
| AutomaticallyAdded | 3 |  |



<a name="anytype-model-Import-ErrorCode"></a>

### Import.ErrorCode


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| INTERNAL_ERROR | 3 |  |
| FILE_LOAD_ERROR | 8 |  |
| IMPORT_IS_CANCELED | 6 |  |
| NOTION_NO_OBJECTS_IN_INTEGRATION | 5 |  |
| NOTION_SERVER_IS_UNAVAILABLE | 12 |  |
| NOTION_RATE_LIMIT_EXCEEDED | 13 |  |
| FILE_IMPORT_NO_OBJECTS_IN_ZIP_ARCHIVE | 14 |  |
| FILE_IMPORT_NO_OBJECTS_IN_DIRECTORY | 17 |  |
| HTML_WRONG_HTML_STRUCTURE | 10 |  |
| PB_NOT_ANYBLOCK_FORMAT | 11 |  |
| CSV_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED | 7 |  |
| INSUFFICIENT_PERMISSIONS | 9 |  |



<a name="anytype-model-Import-Type"></a>

### Import.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Notion | 0 |  |
| Markdown | 1 |  |
| External | 2 | external developers use it |
| Pb | 3 |  |
| Html | 4 |  |
| Txt | 5 |  |
| Csv | 6 |  |
| Obsidian | 7 | Markdown with obsidian improvements |



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



<a name="anytype-model-InviteType"></a>

### InviteType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Member | 0 | aclKey contains the key to sign the ACL record |
| Guest | 1 | guestKey contains the privateKey of the guest user |
| WithoutApprove | 2 | aclKey contains the key to sign the ACL record, but no approval needed |



<a name="anytype-model-LinkPreview-Type"></a>

### LinkPreview.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Page | 1 |  |
| Image | 2 |  |
| Text | 3 |  |



<a name="anytype-model-Membership-EmailVerificationStatus"></a>

### Membership.EmailVerificationStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| StatusNotVerified | 0 | user NEVER comleted the verification of the email |
| StatusCodeSent | 1 | user has asked for new code, but did not enter it yet (even if email was verified before, you can ask to UPDATE your e-mail) please wait, you can not ask for more codes yet |
| StatusVerified | 2 | the e-mail is finally verified |



<a name="anytype-model-Membership-PaymentMethod"></a>

### Membership.PaymentMethod


| Name | Number | Description |
| ---- | ------ | ----------- |
| MethodNone | 0 |  |
| MethodStripe | 1 |  |
| MethodCrypto | 2 |  |
| MethodInappApple | 3 |  |
| MethodInappGoogle | 4 |  |



<a name="anytype-model-Membership-Status"></a>

### Membership.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| StatusUnknown | 0 |  |
| StatusPending | 1 | please wait a bit more, we are still processing your request the payment is confirmed, but we need more time to do some side-effects: - increase limits - send emails - allocate names |
| StatusActive | 2 | the membership is active, ready to use! |
| StatusPendingRequiresFinalization | 3 | in some cases we need to finalize the process: - if user has bought membership directly without first calling the BuySubscription method in this case please call Finalize to finish the process |



<a name="anytype-model-MembershipTierData-PeriodType"></a>

### MembershipTierData.PeriodType


| Name | Number | Description |
| ---- | ------ | ----------- |
| PeriodTypeUnknown | 0 |  |
| PeriodTypeUnlimited | 1 |  |
| PeriodTypeDays | 2 |  |
| PeriodTypeWeeks | 3 |  |
| PeriodTypeMonths | 4 |  |
| PeriodTypeYears | 5 |  |



<a name="anytype-model-MembershipV2-PaymentProvider"></a>

### MembershipV2.PaymentProvider


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Stripe | 1 |  |
| Crypto | 2 |  |
| BillingPortal | 3 |  |
| AppStore | 4 |  |
| GooglePlay | 5 |  |



<a name="anytype-model-MembershipV2-Period"></a>

### MembershipV2.Period


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unlimited | 0 |  |
| Monthly | 1 |  |
| Yearly | 2 |  |
| ThreeYears | 3 |  |



<a name="anytype-model-MembershipV2-ProductStatus-Status"></a>

### MembershipV2.ProductStatus.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| StatusUnknown | 0 |  |
| StatusPending | 1 |  |
| StatusActive | 2 |  |
| StatusPendingRequiresAnyNameAllocation | 3 |  |



<a name="anytype-model-NameserviceNameType"></a>

### NameserviceNameType


| Name | Number | Description |
| ---- | ------ | ----------- |
| AnyName | 0 | .any suffix |



<a name="anytype-model-Notification-ActionType"></a>

### Notification.ActionType


| Name | Number | Description |
| ---- | ------ | ----------- |
| CLOSE | 0 |  |



<a name="anytype-model-Notification-Export-Code"></a>

### Notification.Export.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype-model-Notification-Status"></a>

### Notification.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| Created | 0 |  |
| Shown | 1 |  |
| Read | 2 |  |
| Replied | 3 |  |



<a name="anytype-model-ObjectOrigin"></a>

### ObjectOrigin


| Name | Number | Description |
| ---- | ------ | ----------- |
| none | 0 |  |
| clipboard | 1 |  |
| dragAndDrop | 2 |  |
| import | 3 |  |
| webclipper | 4 |  |
| sharingExtension | 5 |  |
| usecase | 6 |  |
| builtin | 7 |  |
| bookmark | 8 |  |
| api | 9 |  |



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
| audio | 15 |  |
| video | 16 |  |
| date | 17 |  |
| spaceView | 18 |  |
| participant | 19 |  |
| pdf | 20 |  |
| chatDeprecated | 21 | deprecated |
| chatDerived | 22 |  |
| tag | 23 |  |
| notification | 24 |  |
| missingObject | 25 |  |
| devices | 26 |  |



<a name="anytype-model-ParticipantPermissions"></a>

### ParticipantPermissions


| Name | Number | Description |
| ---- | ------ | ----------- |
| Reader | 0 |  |
| Writer | 1 |  |
| Owner | 2 |  |
| NoPermissions | 3 |  |



<a name="anytype-model-ParticipantStatus"></a>

### ParticipantStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| Joining | 0 |  |
| Active | 1 |  |
| Removed | 2 |  |
| Declined | 3 |  |
| Removing | 4 |  |
| Canceled | 5 |  |



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
| CreateObjectOfThisType | 9 | can be set only for types. Restricts creating objects of this type |
| Publish | 10 | object is not allowed to publish |



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
| BundledObjectType | 514 | DEPRECATED |
| AnytypeProfile | 515 |  |
| Date | 516 |  |
| Workspace | 518 |  |
| STRelation | 521 |  |
| STType | 528 |  |
| STRelationOption | 529 |  |
| SpaceView | 530 |  |
| Identity | 532 |  |
| Participant | 534 |  |
| MissingObject | 519 |  |
| FileObject | 533 |  |
| NotificationObject | 535 |  |
| DevicesObject | 536 |  |
| ChatObjectDeprecated | 537 | DEPRECATED Container for any-store based chats |
| ChatDerivedObject | 544 | Any-store based object for chat |
| AccountObject | 545 | Container for account data in tech space |



<a name="anytype-model-SpaceAccessType"></a>

### SpaceAccessType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Private | 0 |  |
| Personal | 1 |  |
| Shared | 2 |  |



<a name="anytype-model-SpaceShareableStatus"></a>

### SpaceShareableStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| StatusUnknown | 0 |  |
| StatusShareable | 1 |  |
| StatusNotShareable | 2 |  |



<a name="anytype-model-SpaceStatus"></a>

### SpaceStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 | Unknown means the space is not loaded yet |
| Loading | 1 | Loading - the space in progress of loading |
| Ok | 2 | Ok - the space loaded and available |
| Missing | 3 | Missing - the space is missing |
| Error | 4 | Error - the space loading ended with an error |
| RemoteWaitingDeletion | 5 | RemoteWaitingDeletion - network status is &#34;waiting deletion&#34; |
| RemoteDeleted | 6 | RemoteDeleted - the space is deleted in the current network |
| SpaceDeleted | 7 | SpaceDeleted - the space should be deleted in the network |
| SpaceActive | 8 | SpaceActive - the space is active in the network |
| SpaceJoining | 9 | SpaceJoining - the account is joining the space |
| SpaceRemoving | 10 | SpaceRemoving - the account is removing from space or the space is removed from network |



<a name="anytype-model-SpaceUxType"></a>

### SpaceUxType


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 | old value for chat, deprecated |
| Data | 1 | objects-first UX |
| Stream | 2 | stream UX (chat with limited amount of owners) |
| Chat | 3 | chat UX |
| OneToOne | 4 | onetoone UX (space with chat and immutable ACL between two participants) |



<a name="anytype-model-SyncError"></a>

### SyncError


| Name | Number | Description |
| ---- | ------ | ----------- |
| SyncErrorNull | 0 |  |
| SyncErrorIncompatibleVersion | 2 |  |
| SyncErrorNetworkError | 3 |  |
| SyncErrorOversized | 4 |  |



<a name="anytype-model-SyncStatus"></a>

### SyncStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| SyncStatusSynced | 0 |  |
| SyncStatusSyncing | 1 |  |
| SyncStatusError | 2 |  |
| SyncStatusQueued | 3 |  |


 

 

 



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

