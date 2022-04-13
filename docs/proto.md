# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [pb/protos/service/service.proto](#pb/protos/service/service.proto)
    - [ClientCommands](#anytype.ClientCommands)
  
- [pb/protos/changes.proto](#pb/protos/changes.proto)
    - [Change](#anytype.Change)
    - [Change.BlockCreate](#anytype.Change.BlockCreate)
    - [Change.BlockDuplicate](#anytype.Change.BlockDuplicate)
    - [Change.BlockMove](#anytype.Change.BlockMove)
    - [Change.BlockRemove](#anytype.Change.BlockRemove)
    - [Change.BlockUpdate](#anytype.Change.BlockUpdate)
    - [Change.Content](#anytype.Change.Content)
    - [Change.DetailsSet](#anytype.Change.DetailsSet)
    - [Change.DetailsUnset](#anytype.Change.DetailsUnset)
    - [Change.FileKeys](#anytype.Change.FileKeys)
    - [Change.FileKeys.KeysEntry](#anytype.Change.FileKeys.KeysEntry)
    - [Change.ObjectTypeAdd](#anytype.Change.ObjectTypeAdd)
    - [Change.ObjectTypeRemove](#anytype.Change.ObjectTypeRemove)
    - [Change.RelationAdd](#anytype.Change.RelationAdd)
    - [Change.RelationRemove](#anytype.Change.RelationRemove)
    - [Change.RelationUpdate](#anytype.Change.RelationUpdate)
    - [Change.RelationUpdate.Dict](#anytype.Change.RelationUpdate.Dict)
    - [Change.RelationUpdate.ObjectTypes](#anytype.Change.RelationUpdate.ObjectTypes)
    - [Change.Snapshot](#anytype.Change.Snapshot)
    - [Change.Snapshot.LogHeadsEntry](#anytype.Change.Snapshot.LogHeadsEntry)
    - [Change.StoreKeySet](#anytype.Change.StoreKeySet)
    - [Change.StoreKeyUnset](#anytype.Change.StoreKeyUnset)
  
- [pb/protos/commands.proto](#pb/protos/commands.proto)
    - [Empty](#anytype.Empty)
    - [Rpc](#anytype.Rpc)
    - [Rpc.Account](#anytype.Rpc.Account)
    - [Rpc.Account.Config](#anytype.Rpc.Account.Config)
    - [Rpc.Account.Create](#anytype.Rpc.Account.Create)
    - [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request)
    - [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response)
    - [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error)
    - [Rpc.Account.Delete](#anytype.Rpc.Account.Delete)
    - [Rpc.Account.Delete.Request](#anytype.Rpc.Account.Delete.Request)
    - [Rpc.Account.Delete.Response](#anytype.Rpc.Account.Delete.Response)
    - [Rpc.Account.Delete.Response.Error](#anytype.Rpc.Account.Delete.Response.Error)
    - [Rpc.Account.GetConfig](#anytype.Rpc.Account.GetConfig)
    - [Rpc.Account.GetConfig.Get](#anytype.Rpc.Account.GetConfig.Get)
    - [Rpc.Account.GetConfig.Get.Request](#anytype.Rpc.Account.GetConfig.Get.Request)
    - [Rpc.Account.Info](#anytype.Rpc.Account.Info)
    - [Rpc.Account.Recover](#anytype.Rpc.Account.Recover)
    - [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request)
    - [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response)
    - [Rpc.Account.Recover.Response.Error](#anytype.Rpc.Account.Recover.Response.Error)
    - [Rpc.Account.Select](#anytype.Rpc.Account.Select)
    - [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request)
    - [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response)
    - [Rpc.Account.Select.Response.Error](#anytype.Rpc.Account.Select.Response.Error)
    - [Rpc.Account.Stop](#anytype.Rpc.Account.Stop)
    - [Rpc.Account.Stop.Request](#anytype.Rpc.Account.Stop.Request)
    - [Rpc.Account.Stop.Response](#anytype.Rpc.Account.Stop.Response)
    - [Rpc.Account.Stop.Response.Error](#anytype.Rpc.Account.Stop.Response.Error)
    - [Rpc.App](#anytype.Rpc.App)
    - [Rpc.App.GetVersion](#anytype.Rpc.App.GetVersion)
    - [Rpc.App.GetVersion.Request](#anytype.Rpc.App.GetVersion.Request)
    - [Rpc.App.GetVersion.Response](#anytype.Rpc.App.GetVersion.Response)
    - [Rpc.App.GetVersion.Response.Error](#anytype.Rpc.App.GetVersion.Response.Error)
    - [Rpc.App.Shutdown](#anytype.Rpc.App.Shutdown)
    - [Rpc.App.Shutdown.Request](#anytype.Rpc.App.Shutdown.Request)
    - [Rpc.App.Shutdown.Response](#anytype.Rpc.App.Shutdown.Response)
    - [Rpc.App.Shutdown.Response.Error](#anytype.Rpc.App.Shutdown.Response.Error)
    - [Rpc.Block](#anytype.Rpc.Block)
    - [Rpc.Block.Copy](#anytype.Rpc.Block.Copy)
    - [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request)
    - [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response)
    - [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error)
    - [Rpc.Block.Create](#anytype.Rpc.Block.Create)
    - [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request)
    - [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response)
    - [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error)
    - [Rpc.Block.Cut](#anytype.Rpc.Block.Cut)
    - [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request)
    - [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response)
    - [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error)
    - [Rpc.Block.Download](#anytype.Rpc.Block.Download)
    - [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request)
    - [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response)
    - [Rpc.Block.Download.Response.Error](#anytype.Rpc.Block.Download.Response.Error)
    - [Rpc.Block.Export](#anytype.Rpc.Block.Export)
    - [Rpc.Block.Export.Request](#anytype.Rpc.Block.Export.Request)
    - [Rpc.Block.Export.Response](#anytype.Rpc.Block.Export.Response)
    - [Rpc.Block.Export.Response.Error](#anytype.Rpc.Block.Export.Response.Error)
    - [Rpc.Block.ListConvertToObjects](#anytype.Rpc.Block.ListConvertToObjects)
    - [Rpc.Block.ListConvertToObjects.Request](#anytype.Rpc.Block.ListConvertToObjects.Request)
    - [Rpc.Block.ListConvertToObjects.Response](#anytype.Rpc.Block.ListConvertToObjects.Response)
    - [Rpc.Block.ListConvertToObjects.Response.Error](#anytype.Rpc.Block.ListConvertToObjects.Response.Error)
    - [Rpc.Block.ListDuplicate](#anytype.Rpc.Block.ListDuplicate)
    - [Rpc.Block.ListDuplicate.Request](#anytype.Rpc.Block.ListDuplicate.Request)
    - [Rpc.Block.ListDuplicate.Response](#anytype.Rpc.Block.ListDuplicate.Response)
    - [Rpc.Block.ListDuplicate.Response.Error](#anytype.Rpc.Block.ListDuplicate.Response.Error)
    - [Rpc.Block.ListMoveToExistingObject](#anytype.Rpc.Block.ListMoveToExistingObject)
    - [Rpc.Block.ListMoveToExistingObject.Request](#anytype.Rpc.Block.ListMoveToExistingObject.Request)
    - [Rpc.Block.ListMoveToExistingObject.Response](#anytype.Rpc.Block.ListMoveToExistingObject.Response)
    - [Rpc.Block.ListMoveToExistingObject.Response.Error](#anytype.Rpc.Block.ListMoveToExistingObject.Response.Error)
    - [Rpc.Block.ListMoveToNewObject](#anytype.Rpc.Block.ListMoveToNewObject)
    - [Rpc.Block.ListMoveToNewObject.Request](#anytype.Rpc.Block.ListMoveToNewObject.Request)
    - [Rpc.Block.ListMoveToNewObject.Response](#anytype.Rpc.Block.ListMoveToNewObject.Response)
    - [Rpc.Block.ListMoveToNewObject.Response.Error](#anytype.Rpc.Block.ListMoveToNewObject.Response.Error)
    - [Rpc.Block.ListSetAlign](#anytype.Rpc.Block.ListSetAlign)
    - [Rpc.Block.ListSetAlign.Request](#anytype.Rpc.Block.ListSetAlign.Request)
    - [Rpc.Block.ListSetAlign.Response](#anytype.Rpc.Block.ListSetAlign.Response)
    - [Rpc.Block.ListSetAlign.Response.Error](#anytype.Rpc.Block.ListSetAlign.Response.Error)
    - [Rpc.Block.ListSetBackgroundColor](#anytype.Rpc.Block.ListSetBackgroundColor)
    - [Rpc.Block.ListSetBackgroundColor.Request](#anytype.Rpc.Block.ListSetBackgroundColor.Request)
    - [Rpc.Block.ListSetBackgroundColor.Response](#anytype.Rpc.Block.ListSetBackgroundColor.Response)
    - [Rpc.Block.ListSetBackgroundColor.Response.Error](#anytype.Rpc.Block.ListSetBackgroundColor.Response.Error)
    - [Rpc.Block.ListSetFields](#anytype.Rpc.Block.ListSetFields)
    - [Rpc.Block.ListSetFields.Request](#anytype.Rpc.Block.ListSetFields.Request)
    - [Rpc.Block.ListSetFields.Request.BlockField](#anytype.Rpc.Block.ListSetFields.Request.BlockField)
    - [Rpc.Block.ListSetFields.Response](#anytype.Rpc.Block.ListSetFields.Response)
    - [Rpc.Block.ListSetFields.Response.Error](#anytype.Rpc.Block.ListSetFields.Response.Error)
    - [Rpc.Block.ListTurnInto](#anytype.Rpc.Block.ListTurnInto)
    - [Rpc.Block.ListTurnInto.Request](#anytype.Rpc.Block.ListTurnInto.Request)
    - [Rpc.Block.ListTurnInto.Response](#anytype.Rpc.Block.ListTurnInto.Response)
    - [Rpc.Block.ListTurnInto.Response.Error](#anytype.Rpc.Block.ListTurnInto.Response.Error)
    - [Rpc.Block.ListUpdate](#anytype.Rpc.Block.ListUpdate)
    - [Rpc.Block.ListUpdate.Request](#anytype.Rpc.Block.ListUpdate.Request)
    - [Rpc.Block.ListUpdate.Request.Text](#anytype.Rpc.Block.ListUpdate.Request.Text)
    - [Rpc.Block.Merge](#anytype.Rpc.Block.Merge)
    - [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request)
    - [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response)
    - [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error)
    - [Rpc.Block.Paste](#anytype.Rpc.Block.Paste)
    - [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request)
    - [Rpc.Block.Paste.Request.File](#anytype.Rpc.Block.Paste.Request.File)
    - [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response)
    - [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error)
    - [Rpc.Block.Replace](#anytype.Rpc.Block.Replace)
    - [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request)
    - [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response)
    - [Rpc.Block.Replace.Response.Error](#anytype.Rpc.Block.Replace.Response.Error)
    - [Rpc.Block.SetFields](#anytype.Rpc.Block.SetFields)
    - [Rpc.Block.SetFields.Request](#anytype.Rpc.Block.SetFields.Request)
    - [Rpc.Block.SetFields.Response](#anytype.Rpc.Block.SetFields.Response)
    - [Rpc.Block.SetFields.Response.Error](#anytype.Rpc.Block.SetFields.Response.Error)
    - [Rpc.Block.SetRestrictions](#anytype.Rpc.Block.SetRestrictions)
    - [Rpc.Block.SetRestrictions.Request](#anytype.Rpc.Block.SetRestrictions.Request)
    - [Rpc.Block.SetRestrictions.Response](#anytype.Rpc.Block.SetRestrictions.Response)
    - [Rpc.Block.SetRestrictions.Response.Error](#anytype.Rpc.Block.SetRestrictions.Response.Error)
    - [Rpc.Block.Split](#anytype.Rpc.Block.Split)
    - [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request)
    - [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response)
    - [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error)
    - [Rpc.Block.Unlink](#anytype.Rpc.Block.Unlink)
    - [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request)
    - [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response)
    - [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error)
    - [Rpc.Block.Upload](#anytype.Rpc.Block.Upload)
    - [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request)
    - [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response)
    - [Rpc.Block.Upload.Response.Error](#anytype.Rpc.Block.Upload.Response.Error)
    - [Rpc.BlockBookmark](#anytype.Rpc.BlockBookmark)
    - [Rpc.BlockBookmark.CreateAndFetch](#anytype.Rpc.BlockBookmark.CreateAndFetch)
    - [Rpc.BlockBookmark.CreateAndFetch.Request](#anytype.Rpc.BlockBookmark.CreateAndFetch.Request)
    - [Rpc.BlockBookmark.CreateAndFetch.Response](#anytype.Rpc.BlockBookmark.CreateAndFetch.Response)
    - [Rpc.BlockBookmark.CreateAndFetch.Response.Error](#anytype.Rpc.BlockBookmark.CreateAndFetch.Response.Error)
    - [Rpc.BlockBookmark.Fetch](#anytype.Rpc.BlockBookmark.Fetch)
    - [Rpc.BlockBookmark.Fetch.Request](#anytype.Rpc.BlockBookmark.Fetch.Request)
    - [Rpc.BlockBookmark.Fetch.Response](#anytype.Rpc.BlockBookmark.Fetch.Response)
    - [Rpc.BlockBookmark.Fetch.Response.Error](#anytype.Rpc.BlockBookmark.Fetch.Response.Error)
    - [Rpc.BlockDataview](#anytype.Rpc.BlockDataview)
    - [Rpc.BlockDataview.Relation](#anytype.Rpc.BlockDataview.Relation)
    - [Rpc.BlockDataview.Relation.Add](#anytype.Rpc.BlockDataview.Relation.Add)
    - [Rpc.BlockDataview.Relation.Add.Request](#anytype.Rpc.BlockDataview.Relation.Add.Request)
    - [Rpc.BlockDataview.Relation.Add.Response](#anytype.Rpc.BlockDataview.Relation.Add.Response)
    - [Rpc.BlockDataview.Relation.Add.Response.Error](#anytype.Rpc.BlockDataview.Relation.Add.Response.Error)
    - [Rpc.BlockDataview.Relation.Delete](#anytype.Rpc.BlockDataview.Relation.Delete)
    - [Rpc.BlockDataview.Relation.Delete.Request](#anytype.Rpc.BlockDataview.Relation.Delete.Request)
    - [Rpc.BlockDataview.Relation.Delete.Response](#anytype.Rpc.BlockDataview.Relation.Delete.Response)
    - [Rpc.BlockDataview.Relation.Delete.Response.Error](#anytype.Rpc.BlockDataview.Relation.Delete.Response.Error)
    - [Rpc.BlockDataview.Relation.ListAvailable](#anytype.Rpc.BlockDataview.Relation.ListAvailable)
    - [Rpc.BlockDataview.Relation.ListAvailable.Request](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Request)
    - [Rpc.BlockDataview.Relation.ListAvailable.Response](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Response)
    - [Rpc.BlockDataview.Relation.ListAvailable.Response.Error](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Response.Error)
    - [Rpc.BlockDataview.Relation.Update](#anytype.Rpc.BlockDataview.Relation.Update)
    - [Rpc.BlockDataview.Relation.Update.Request](#anytype.Rpc.BlockDataview.Relation.Update.Request)
    - [Rpc.BlockDataview.Relation.Update.Response](#anytype.Rpc.BlockDataview.Relation.Update.Response)
    - [Rpc.BlockDataview.Relation.Update.Response.Error](#anytype.Rpc.BlockDataview.Relation.Update.Response.Error)
    - [Rpc.BlockDataview.SetSource](#anytype.Rpc.BlockDataview.SetSource)
    - [Rpc.BlockDataview.SetSource.Request](#anytype.Rpc.BlockDataview.SetSource.Request)
    - [Rpc.BlockDataview.SetSource.Response](#anytype.Rpc.BlockDataview.SetSource.Response)
    - [Rpc.BlockDataview.SetSource.Response.Error](#anytype.Rpc.BlockDataview.SetSource.Response.Error)
    - [Rpc.BlockDataview.View](#anytype.Rpc.BlockDataview.View)
    - [Rpc.BlockDataview.View.Create](#anytype.Rpc.BlockDataview.View.Create)
    - [Rpc.BlockDataview.View.Create.Request](#anytype.Rpc.BlockDataview.View.Create.Request)
    - [Rpc.BlockDataview.View.Create.Response](#anytype.Rpc.BlockDataview.View.Create.Response)
    - [Rpc.BlockDataview.View.Create.Response.Error](#anytype.Rpc.BlockDataview.View.Create.Response.Error)
    - [Rpc.BlockDataview.View.Delete](#anytype.Rpc.BlockDataview.View.Delete)
    - [Rpc.BlockDataview.View.Delete.Request](#anytype.Rpc.BlockDataview.View.Delete.Request)
    - [Rpc.BlockDataview.View.Delete.Response](#anytype.Rpc.BlockDataview.View.Delete.Response)
    - [Rpc.BlockDataview.View.Delete.Response.Error](#anytype.Rpc.BlockDataview.View.Delete.Response.Error)
    - [Rpc.BlockDataview.View.SetActive](#anytype.Rpc.BlockDataview.View.SetActive)
    - [Rpc.BlockDataview.View.SetActive.Request](#anytype.Rpc.BlockDataview.View.SetActive.Request)
    - [Rpc.BlockDataview.View.SetActive.Response](#anytype.Rpc.BlockDataview.View.SetActive.Response)
    - [Rpc.BlockDataview.View.SetActive.Response.Error](#anytype.Rpc.BlockDataview.View.SetActive.Response.Error)
    - [Rpc.BlockDataview.View.SetPosition](#anytype.Rpc.BlockDataview.View.SetPosition)
    - [Rpc.BlockDataview.View.SetPosition.Request](#anytype.Rpc.BlockDataview.View.SetPosition.Request)
    - [Rpc.BlockDataview.View.SetPosition.Response](#anytype.Rpc.BlockDataview.View.SetPosition.Response)
    - [Rpc.BlockDataview.View.SetPosition.Response.Error](#anytype.Rpc.BlockDataview.View.SetPosition.Response.Error)
    - [Rpc.BlockDataview.View.Update](#anytype.Rpc.BlockDataview.View.Update)
    - [Rpc.BlockDataview.View.Update.Request](#anytype.Rpc.BlockDataview.View.Update.Request)
    - [Rpc.BlockDataview.View.Update.Response](#anytype.Rpc.BlockDataview.View.Update.Response)
    - [Rpc.BlockDataview.View.Update.Response.Error](#anytype.Rpc.BlockDataview.View.Update.Response.Error)
    - [Rpc.BlockDataviewRecord](#anytype.Rpc.BlockDataviewRecord)
    - [Rpc.BlockDataviewRecord.AddRelationOption](#anytype.Rpc.BlockDataviewRecord.AddRelationOption)
    - [Rpc.BlockDataviewRecord.AddRelationOption.Request](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Request)
    - [Rpc.BlockDataviewRecord.AddRelationOption.Response](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response)
    - [Rpc.BlockDataviewRecord.AddRelationOption.Response.Error](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response.Error)
    - [Rpc.BlockDataviewRecord.Create](#anytype.Rpc.BlockDataviewRecord.Create)
    - [Rpc.BlockDataviewRecord.Create.Request](#anytype.Rpc.BlockDataviewRecord.Create.Request)
    - [Rpc.BlockDataviewRecord.Create.Response](#anytype.Rpc.BlockDataviewRecord.Create.Response)
    - [Rpc.BlockDataviewRecord.Create.Response.Error](#anytype.Rpc.BlockDataviewRecord.Create.Response.Error)
    - [Rpc.BlockDataviewRecord.Delete](#anytype.Rpc.BlockDataviewRecord.Delete)
    - [Rpc.BlockDataviewRecord.Delete.Request](#anytype.Rpc.BlockDataviewRecord.Delete.Request)
    - [Rpc.BlockDataviewRecord.Delete.Response](#anytype.Rpc.BlockDataviewRecord.Delete.Response)
    - [Rpc.BlockDataviewRecord.Delete.Response.Error](#anytype.Rpc.BlockDataviewRecord.Delete.Response.Error)
    - [Rpc.BlockDataviewRecord.DeleteRelationOption](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption)
    - [Rpc.BlockDataviewRecord.DeleteRelationOption.Request](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Request)
    - [Rpc.BlockDataviewRecord.DeleteRelationOption.Response](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response)
    - [Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error)
    - [Rpc.BlockDataviewRecord.Update](#anytype.Rpc.BlockDataviewRecord.Update)
    - [Rpc.BlockDataviewRecord.Update.Request](#anytype.Rpc.BlockDataviewRecord.Update.Request)
    - [Rpc.BlockDataviewRecord.Update.Response](#anytype.Rpc.BlockDataviewRecord.Update.Response)
    - [Rpc.BlockDataviewRecord.Update.Response.Error](#anytype.Rpc.BlockDataviewRecord.Update.Response.Error)
    - [Rpc.BlockDataviewRecord.UpdateRelationOption](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption)
    - [Rpc.BlockDataviewRecord.UpdateRelationOption.Request](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Request)
    - [Rpc.BlockDataviewRecord.UpdateRelationOption.Response](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response)
    - [Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error)
    - [Rpc.BlockDiv](#anytype.Rpc.BlockDiv)
    - [Rpc.BlockDiv.ListSetStyle](#anytype.Rpc.BlockDiv.ListSetStyle)
    - [Rpc.BlockDiv.ListSetStyle.Request](#anytype.Rpc.BlockDiv.ListSetStyle.Request)
    - [Rpc.BlockDiv.ListSetStyle.Response](#anytype.Rpc.BlockDiv.ListSetStyle.Response)
    - [Rpc.BlockDiv.ListSetStyle.Response.Error](#anytype.Rpc.BlockDiv.ListSetStyle.Response.Error)
    - [Rpc.BlockFile](#anytype.Rpc.BlockFile)
    - [Rpc.BlockFile.CreateAndUpload](#anytype.Rpc.BlockFile.CreateAndUpload)
    - [Rpc.BlockFile.CreateAndUpload.Request](#anytype.Rpc.BlockFile.CreateAndUpload.Request)
    - [Rpc.BlockFile.CreateAndUpload.Response](#anytype.Rpc.BlockFile.CreateAndUpload.Response)
    - [Rpc.BlockFile.CreateAndUpload.Response.Error](#anytype.Rpc.BlockFile.CreateAndUpload.Response.Error)
    - [Rpc.BlockFile.ListSetStyle](#anytype.Rpc.BlockFile.ListSetStyle)
    - [Rpc.BlockFile.ListSetStyle.Request](#anytype.Rpc.BlockFile.ListSetStyle.Request)
    - [Rpc.BlockFile.ListSetStyle.Response](#anytype.Rpc.BlockFile.ListSetStyle.Response)
    - [Rpc.BlockFile.ListSetStyle.Response.Error](#anytype.Rpc.BlockFile.ListSetStyle.Response.Error)
    - [Rpc.BlockFile.SetName](#anytype.Rpc.BlockFile.SetName)
    - [Rpc.BlockFile.SetName.Request](#anytype.Rpc.BlockFile.SetName.Request)
    - [Rpc.BlockFile.SetName.Response](#anytype.Rpc.BlockFile.SetName.Response)
    - [Rpc.BlockFile.SetName.Response.Error](#anytype.Rpc.BlockFile.SetName.Response.Error)
    - [Rpc.BlockImage](#anytype.Rpc.BlockImage)
    - [Rpc.BlockImage.SetName](#anytype.Rpc.BlockImage.SetName)
    - [Rpc.BlockImage.SetName.Request](#anytype.Rpc.BlockImage.SetName.Request)
    - [Rpc.BlockImage.SetName.Response](#anytype.Rpc.BlockImage.SetName.Response)
    - [Rpc.BlockImage.SetName.Response.Error](#anytype.Rpc.BlockImage.SetName.Response.Error)
    - [Rpc.BlockImage.SetWidth](#anytype.Rpc.BlockImage.SetWidth)
    - [Rpc.BlockImage.SetWidth.Request](#anytype.Rpc.BlockImage.SetWidth.Request)
    - [Rpc.BlockImage.SetWidth.Response](#anytype.Rpc.BlockImage.SetWidth.Response)
    - [Rpc.BlockImage.SetWidth.Response.Error](#anytype.Rpc.BlockImage.SetWidth.Response.Error)
    - [Rpc.BlockLatex](#anytype.Rpc.BlockLatex)
    - [Rpc.BlockLatex.SetText](#anytype.Rpc.BlockLatex.SetText)
    - [Rpc.BlockLatex.SetText.Request](#anytype.Rpc.BlockLatex.SetText.Request)
    - [Rpc.BlockLatex.SetText.Response](#anytype.Rpc.BlockLatex.SetText.Response)
    - [Rpc.BlockLatex.SetText.Response.Error](#anytype.Rpc.BlockLatex.SetText.Response.Error)
    - [Rpc.BlockLink](#anytype.Rpc.BlockLink)
    - [Rpc.BlockLink.CreateLinkToNewObject](#anytype.Rpc.BlockLink.CreateLinkToNewObject)
    - [Rpc.BlockLink.CreateLinkToNewObject.Request](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Request)
    - [Rpc.BlockLink.CreateLinkToNewObject.Response](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Response)
    - [Rpc.BlockLink.CreateLinkToNewObject.Response.Error](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Response.Error)
    - [Rpc.BlockLink.CreateLinkToNewSet](#anytype.Rpc.BlockLink.CreateLinkToNewSet)
    - [Rpc.BlockLink.CreateLinkToNewSet.Request](#anytype.Rpc.BlockLink.CreateLinkToNewSet.Request)
    - [Rpc.BlockLink.CreateLinkToNewSet.Response](#anytype.Rpc.BlockLink.CreateLinkToNewSet.Response)
    - [Rpc.BlockLink.CreateLinkToNewSet.Response.Error](#anytype.Rpc.BlockLink.CreateLinkToNewSet.Response.Error)
    - [Rpc.BlockLink.SetTargetBlockId](#anytype.Rpc.BlockLink.SetTargetBlockId)
    - [Rpc.BlockLink.SetTargetBlockId.Request](#anytype.Rpc.BlockLink.SetTargetBlockId.Request)
    - [Rpc.BlockLink.SetTargetBlockId.Response](#anytype.Rpc.BlockLink.SetTargetBlockId.Response)
    - [Rpc.BlockLink.SetTargetBlockId.Response.Error](#anytype.Rpc.BlockLink.SetTargetBlockId.Response.Error)
    - [Rpc.BlockRelation](#anytype.Rpc.BlockRelation)
    - [Rpc.BlockRelation.Add](#anytype.Rpc.BlockRelation.Add)
    - [Rpc.BlockRelation.Add.Request](#anytype.Rpc.BlockRelation.Add.Request)
    - [Rpc.BlockRelation.Add.Response](#anytype.Rpc.BlockRelation.Add.Response)
    - [Rpc.BlockRelation.Add.Response.Error](#anytype.Rpc.BlockRelation.Add.Response.Error)
    - [Rpc.BlockRelation.SetKey](#anytype.Rpc.BlockRelation.SetKey)
    - [Rpc.BlockRelation.SetKey.Request](#anytype.Rpc.BlockRelation.SetKey.Request)
    - [Rpc.BlockRelation.SetKey.Response](#anytype.Rpc.BlockRelation.SetKey.Response)
    - [Rpc.BlockRelation.SetKey.Response.Error](#anytype.Rpc.BlockRelation.SetKey.Response.Error)
    - [Rpc.BlockText](#anytype.Rpc.BlockText)
    - [Rpc.BlockText.ListSetColor](#anytype.Rpc.BlockText.ListSetColor)
    - [Rpc.BlockText.ListSetColor.Request](#anytype.Rpc.BlockText.ListSetColor.Request)
    - [Rpc.BlockText.ListSetColor.Response](#anytype.Rpc.BlockText.ListSetColor.Response)
    - [Rpc.BlockText.ListSetColor.Response.Error](#anytype.Rpc.BlockText.ListSetColor.Response.Error)
    - [Rpc.BlockText.ListSetMark](#anytype.Rpc.BlockText.ListSetMark)
    - [Rpc.BlockText.ListSetMark.Request](#anytype.Rpc.BlockText.ListSetMark.Request)
    - [Rpc.BlockText.ListSetMark.Response](#anytype.Rpc.BlockText.ListSetMark.Response)
    - [Rpc.BlockText.ListSetMark.Response.Error](#anytype.Rpc.BlockText.ListSetMark.Response.Error)
    - [Rpc.BlockText.ListSetStyle](#anytype.Rpc.BlockText.ListSetStyle)
    - [Rpc.BlockText.ListSetStyle.Request](#anytype.Rpc.BlockText.ListSetStyle.Request)
    - [Rpc.BlockText.ListSetStyle.Response](#anytype.Rpc.BlockText.ListSetStyle.Response)
    - [Rpc.BlockText.ListSetStyle.Response.Error](#anytype.Rpc.BlockText.ListSetStyle.Response.Error)
    - [Rpc.BlockText.SetChecked](#anytype.Rpc.BlockText.SetChecked)
    - [Rpc.BlockText.SetChecked.Request](#anytype.Rpc.BlockText.SetChecked.Request)
    - [Rpc.BlockText.SetChecked.Response](#anytype.Rpc.BlockText.SetChecked.Response)
    - [Rpc.BlockText.SetChecked.Response.Error](#anytype.Rpc.BlockText.SetChecked.Response.Error)
    - [Rpc.BlockText.SetColor](#anytype.Rpc.BlockText.SetColor)
    - [Rpc.BlockText.SetColor.Request](#anytype.Rpc.BlockText.SetColor.Request)
    - [Rpc.BlockText.SetColor.Response](#anytype.Rpc.BlockText.SetColor.Response)
    - [Rpc.BlockText.SetColor.Response.Error](#anytype.Rpc.BlockText.SetColor.Response.Error)
    - [Rpc.BlockText.SetIcon](#anytype.Rpc.BlockText.SetIcon)
    - [Rpc.BlockText.SetIcon.Request](#anytype.Rpc.BlockText.SetIcon.Request)
    - [Rpc.BlockText.SetIcon.Response](#anytype.Rpc.BlockText.SetIcon.Response)
    - [Rpc.BlockText.SetIcon.Response.Error](#anytype.Rpc.BlockText.SetIcon.Response.Error)
    - [Rpc.BlockText.SetMarks](#anytype.Rpc.BlockText.SetMarks)
    - [Rpc.BlockText.SetMarks.Get](#anytype.Rpc.BlockText.SetMarks.Get)
    - [Rpc.BlockText.SetMarks.Get.Request](#anytype.Rpc.BlockText.SetMarks.Get.Request)
    - [Rpc.BlockText.SetMarks.Get.Response](#anytype.Rpc.BlockText.SetMarks.Get.Response)
    - [Rpc.BlockText.SetMarks.Get.Response.Error](#anytype.Rpc.BlockText.SetMarks.Get.Response.Error)
    - [Rpc.BlockText.SetStyle](#anytype.Rpc.BlockText.SetStyle)
    - [Rpc.BlockText.SetStyle.Request](#anytype.Rpc.BlockText.SetStyle.Request)
    - [Rpc.BlockText.SetStyle.Response](#anytype.Rpc.BlockText.SetStyle.Response)
    - [Rpc.BlockText.SetStyle.Response.Error](#anytype.Rpc.BlockText.SetStyle.Response.Error)
    - [Rpc.BlockText.SetText](#anytype.Rpc.BlockText.SetText)
    - [Rpc.BlockText.SetText.Request](#anytype.Rpc.BlockText.SetText.Request)
    - [Rpc.BlockText.SetText.Response](#anytype.Rpc.BlockText.SetText.Response)
    - [Rpc.BlockText.SetText.Response.Error](#anytype.Rpc.BlockText.SetText.Response.Error)
    - [Rpc.BlockVideo](#anytype.Rpc.BlockVideo)
    - [Rpc.BlockVideo.SetName](#anytype.Rpc.BlockVideo.SetName)
    - [Rpc.BlockVideo.SetName.Request](#anytype.Rpc.BlockVideo.SetName.Request)
    - [Rpc.BlockVideo.SetName.Response](#anytype.Rpc.BlockVideo.SetName.Response)
    - [Rpc.BlockVideo.SetName.Response.Error](#anytype.Rpc.BlockVideo.SetName.Response.Error)
    - [Rpc.BlockVideo.SetWidth](#anytype.Rpc.BlockVideo.SetWidth)
    - [Rpc.BlockVideo.SetWidth.Request](#anytype.Rpc.BlockVideo.SetWidth.Request)
    - [Rpc.BlockVideo.SetWidth.Response](#anytype.Rpc.BlockVideo.SetWidth.Response)
    - [Rpc.BlockVideo.SetWidth.Response.Error](#anytype.Rpc.BlockVideo.SetWidth.Response.Error)
    - [Rpc.Debug](#anytype.Rpc.Debug)
    - [Rpc.Debug.ExportLocalstore](#anytype.Rpc.Debug.ExportLocalstore)
    - [Rpc.Debug.ExportLocalstore.Request](#anytype.Rpc.Debug.ExportLocalstore.Request)
    - [Rpc.Debug.ExportLocalstore.Response](#anytype.Rpc.Debug.ExportLocalstore.Response)
    - [Rpc.Debug.ExportLocalstore.Response.Error](#anytype.Rpc.Debug.ExportLocalstore.Response.Error)
    - [Rpc.Debug.Ping](#anytype.Rpc.Debug.Ping)
    - [Rpc.Debug.Ping.Request](#anytype.Rpc.Debug.Ping.Request)
    - [Rpc.Debug.Ping.Response](#anytype.Rpc.Debug.Ping.Response)
    - [Rpc.Debug.Ping.Response.Error](#anytype.Rpc.Debug.Ping.Response.Error)
    - [Rpc.Debug.Sync](#anytype.Rpc.Debug.Sync)
    - [Rpc.Debug.Sync.Request](#anytype.Rpc.Debug.Sync.Request)
    - [Rpc.Debug.Sync.Response](#anytype.Rpc.Debug.Sync.Response)
    - [Rpc.Debug.Sync.Response.Error](#anytype.Rpc.Debug.Sync.Response.Error)
    - [Rpc.Debug.Thread](#anytype.Rpc.Debug.Thread)
    - [Rpc.Debug.Thread.Request](#anytype.Rpc.Debug.Thread.Request)
    - [Rpc.Debug.Thread.Response](#anytype.Rpc.Debug.Thread.Response)
    - [Rpc.Debug.Thread.Response.Error](#anytype.Rpc.Debug.Thread.Response.Error)
    - [Rpc.Debug.Tree](#anytype.Rpc.Debug.Tree)
    - [Rpc.Debug.Tree.Request](#anytype.Rpc.Debug.Tree.Request)
    - [Rpc.Debug.Tree.Response](#anytype.Rpc.Debug.Tree.Response)
    - [Rpc.Debug.Tree.Response.Error](#anytype.Rpc.Debug.Tree.Response.Error)
    - [Rpc.Debug.logInfo](#anytype.Rpc.Debug.logInfo)
    - [Rpc.Debug.threadInfo](#anytype.Rpc.Debug.threadInfo)
    - [Rpc.File](#anytype.Rpc.File)
    - [Rpc.File.Download](#anytype.Rpc.File.Download)
    - [Rpc.File.Download.Request](#anytype.Rpc.File.Download.Request)
    - [Rpc.File.Download.Response](#anytype.Rpc.File.Download.Response)
    - [Rpc.File.Download.Response.Error](#anytype.Rpc.File.Download.Response.Error)
    - [Rpc.File.Drop](#anytype.Rpc.File.Drop)
    - [Rpc.File.Drop.Request](#anytype.Rpc.File.Drop.Request)
    - [Rpc.File.Drop.Response](#anytype.Rpc.File.Drop.Response)
    - [Rpc.File.Drop.Response.Error](#anytype.Rpc.File.Drop.Response.Error)
    - [Rpc.File.ListOffload](#anytype.Rpc.File.ListOffload)
    - [Rpc.File.ListOffload.Request](#anytype.Rpc.File.ListOffload.Request)
    - [Rpc.File.ListOffload.Response](#anytype.Rpc.File.ListOffload.Response)
    - [Rpc.File.ListOffload.Response.Error](#anytype.Rpc.File.ListOffload.Response.Error)
    - [Rpc.File.Offload](#anytype.Rpc.File.Offload)
    - [Rpc.File.Offload.Request](#anytype.Rpc.File.Offload.Request)
    - [Rpc.File.Offload.Response](#anytype.Rpc.File.Offload.Response)
    - [Rpc.File.Offload.Response.Error](#anytype.Rpc.File.Offload.Response.Error)
    - [Rpc.File.Upload](#anytype.Rpc.File.Upload)
    - [Rpc.File.Upload.Request](#anytype.Rpc.File.Upload.Request)
    - [Rpc.File.Upload.Response](#anytype.Rpc.File.Upload.Response)
    - [Rpc.File.Upload.Response.Error](#anytype.Rpc.File.Upload.Response.Error)
    - [Rpc.GenericErrorResponse](#anytype.Rpc.GenericErrorResponse)
    - [Rpc.GenericErrorResponse.Error](#anytype.Rpc.GenericErrorResponse.Error)
    - [Rpc.History](#anytype.Rpc.History)
    - [Rpc.History.GetVersions](#anytype.Rpc.History.GetVersions)
    - [Rpc.History.GetVersions.Request](#anytype.Rpc.History.GetVersions.Request)
    - [Rpc.History.GetVersions.Response](#anytype.Rpc.History.GetVersions.Response)
    - [Rpc.History.GetVersions.Response.Error](#anytype.Rpc.History.GetVersions.Response.Error)
    - [Rpc.History.SetVersion](#anytype.Rpc.History.SetVersion)
    - [Rpc.History.SetVersion.Request](#anytype.Rpc.History.SetVersion.Request)
    - [Rpc.History.SetVersion.Response](#anytype.Rpc.History.SetVersion.Response)
    - [Rpc.History.SetVersion.Response.Error](#anytype.Rpc.History.SetVersion.Response.Error)
    - [Rpc.History.ShowVersion](#anytype.Rpc.History.ShowVersion)
    - [Rpc.History.ShowVersion.Request](#anytype.Rpc.History.ShowVersion.Request)
    - [Rpc.History.ShowVersion.Response](#anytype.Rpc.History.ShowVersion.Response)
    - [Rpc.History.ShowVersion.Response.Error](#anytype.Rpc.History.ShowVersion.Response.Error)
    - [Rpc.History.Version](#anytype.Rpc.History.Version)
    - [Rpc.LinkPreview](#anytype.Rpc.LinkPreview)
    - [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request)
    - [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response)
    - [Rpc.LinkPreview.Response.Error](#anytype.Rpc.LinkPreview.Response.Error)
    - [Rpc.Log](#anytype.Rpc.Log)
    - [Rpc.Log.Send](#anytype.Rpc.Log.Send)
    - [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request)
    - [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response)
    - [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error)
    - [Rpc.Metrics](#anytype.Rpc.Metrics)
    - [Rpc.Metrics.SetParameters](#anytype.Rpc.Metrics.SetParameters)
    - [Rpc.Metrics.SetParameters.Request](#anytype.Rpc.Metrics.SetParameters.Request)
    - [Rpc.Metrics.SetParameters.Response](#anytype.Rpc.Metrics.SetParameters.Response)
    - [Rpc.Metrics.SetParameters.Response.Error](#anytype.Rpc.Metrics.SetParameters.Response.Error)
    - [Rpc.Navigation](#anytype.Rpc.Navigation)
    - [Rpc.Navigation.GetObjectInfoWithLinks](#anytype.Rpc.Navigation.GetObjectInfoWithLinks)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error)
    - [Rpc.Navigation.ListObjects](#anytype.Rpc.Navigation.ListObjects)
    - [Rpc.Navigation.ListObjects.Request](#anytype.Rpc.Navigation.ListObjects.Request)
    - [Rpc.Navigation.ListObjects.Response](#anytype.Rpc.Navigation.ListObjects.Response)
    - [Rpc.Navigation.ListObjects.Response.Error](#anytype.Rpc.Navigation.ListObjects.Response.Error)
    - [Rpc.Object](#anytype.Rpc.Object)
    - [Rpc.Object.AddWithObjectId](#anytype.Rpc.Object.AddWithObjectId)
    - [Rpc.Object.AddWithObjectId.Request](#anytype.Rpc.Object.AddWithObjectId.Request)
    - [Rpc.Object.AddWithObjectId.Response](#anytype.Rpc.Object.AddWithObjectId.Response)
    - [Rpc.Object.AddWithObjectId.Response.Error](#anytype.Rpc.Object.AddWithObjectId.Response.Error)
    - [Rpc.Object.ApplyTemplate](#anytype.Rpc.Object.ApplyTemplate)
    - [Rpc.Object.ApplyTemplate.Request](#anytype.Rpc.Object.ApplyTemplate.Request)
    - [Rpc.Object.ApplyTemplate.Response](#anytype.Rpc.Object.ApplyTemplate.Response)
    - [Rpc.Object.ApplyTemplate.Response.Error](#anytype.Rpc.Object.ApplyTemplate.Response.Error)
    - [Rpc.Object.Close](#anytype.Rpc.Object.Close)
    - [Rpc.Object.Close.Request](#anytype.Rpc.Object.Close.Request)
    - [Rpc.Object.Close.Response](#anytype.Rpc.Object.Close.Response)
    - [Rpc.Object.Close.Response.Error](#anytype.Rpc.Object.Close.Response.Error)
    - [Rpc.Object.Create](#anytype.Rpc.Object.Create)
    - [Rpc.Object.Create.Request](#anytype.Rpc.Object.Create.Request)
    - [Rpc.Object.Create.Response](#anytype.Rpc.Object.Create.Response)
    - [Rpc.Object.Create.Response.Error](#anytype.Rpc.Object.Create.Response.Error)
    - [Rpc.Object.CreateSet](#anytype.Rpc.Object.CreateSet)
    - [Rpc.Object.CreateSet.Request](#anytype.Rpc.Object.CreateSet.Request)
    - [Rpc.Object.CreateSet.Response](#anytype.Rpc.Object.CreateSet.Response)
    - [Rpc.Object.CreateSet.Response.Error](#anytype.Rpc.Object.CreateSet.Response.Error)
    - [Rpc.Object.Duplicate](#anytype.Rpc.Object.Duplicate)
    - [Rpc.Object.Duplicate.Request](#anytype.Rpc.Object.Duplicate.Request)
    - [Rpc.Object.Duplicate.Response](#anytype.Rpc.Object.Duplicate.Response)
    - [Rpc.Object.Duplicate.Response.Error](#anytype.Rpc.Object.Duplicate.Response.Error)
    - [Rpc.Object.Export](#anytype.Rpc.Object.Export)
    - [Rpc.Object.Export.Request](#anytype.Rpc.Object.Export.Request)
    - [Rpc.Object.Export.Response](#anytype.Rpc.Object.Export.Response)
    - [Rpc.Object.Export.Response.Error](#anytype.Rpc.Object.Export.Response.Error)
    - [Rpc.Object.Graph](#anytype.Rpc.Object.Graph)
    - [Rpc.Object.Graph.Edge](#anytype.Rpc.Object.Graph.Edge)
    - [Rpc.Object.Graph.Node](#anytype.Rpc.Object.Graph.Node)
    - [Rpc.Object.Graph.Request](#anytype.Rpc.Object.Graph.Request)
    - [Rpc.Object.Graph.Response](#anytype.Rpc.Object.Graph.Response)
    - [Rpc.Object.Graph.Response.Error](#anytype.Rpc.Object.Graph.Response.Error)
    - [Rpc.Object.ImportMarkdown](#anytype.Rpc.Object.ImportMarkdown)
    - [Rpc.Object.ImportMarkdown.Request](#anytype.Rpc.Object.ImportMarkdown.Request)
    - [Rpc.Object.ImportMarkdown.Response](#anytype.Rpc.Object.ImportMarkdown.Response)
    - [Rpc.Object.ImportMarkdown.Response.Error](#anytype.Rpc.Object.ImportMarkdown.Response.Error)
    - [Rpc.Object.ListDelete](#anytype.Rpc.Object.ListDelete)
    - [Rpc.Object.ListDelete.Request](#anytype.Rpc.Object.ListDelete.Request)
    - [Rpc.Object.ListDelete.Response](#anytype.Rpc.Object.ListDelete.Response)
    - [Rpc.Object.ListDelete.Response.Error](#anytype.Rpc.Object.ListDelete.Response.Error)
    - [Rpc.Object.ListSetIsArchived](#anytype.Rpc.Object.ListSetIsArchived)
    - [Rpc.Object.ListSetIsArchived.Request](#anytype.Rpc.Object.ListSetIsArchived.Request)
    - [Rpc.Object.ListSetIsArchived.Response](#anytype.Rpc.Object.ListSetIsArchived.Response)
    - [Rpc.Object.ListSetIsArchived.Response.Error](#anytype.Rpc.Object.ListSetIsArchived.Response.Error)
    - [Rpc.Object.ListSetIsFavorite](#anytype.Rpc.Object.ListSetIsFavorite)
    - [Rpc.Object.ListSetIsFavorite.Request](#anytype.Rpc.Object.ListSetIsFavorite.Request)
    - [Rpc.Object.ListSetIsFavorite.Response](#anytype.Rpc.Object.ListSetIsFavorite.Response)
    - [Rpc.Object.ListSetIsFavorite.Response.Error](#anytype.Rpc.Object.ListSetIsFavorite.Response.Error)
    - [Rpc.Object.Open](#anytype.Rpc.Object.Open)
    - [Rpc.Object.Open.Request](#anytype.Rpc.Object.Open.Request)
    - [Rpc.Object.Open.Response](#anytype.Rpc.Object.Open.Response)
    - [Rpc.Object.Open.Response.Error](#anytype.Rpc.Object.Open.Response.Error)
    - [Rpc.Object.OpenBreadcrumbs](#anytype.Rpc.Object.OpenBreadcrumbs)
    - [Rpc.Object.OpenBreadcrumbs.Request](#anytype.Rpc.Object.OpenBreadcrumbs.Request)
    - [Rpc.Object.OpenBreadcrumbs.Response](#anytype.Rpc.Object.OpenBreadcrumbs.Response)
    - [Rpc.Object.OpenBreadcrumbs.Response.Error](#anytype.Rpc.Object.OpenBreadcrumbs.Response.Error)
    - [Rpc.Object.Redo](#anytype.Rpc.Object.Redo)
    - [Rpc.Object.Redo.Request](#anytype.Rpc.Object.Redo.Request)
    - [Rpc.Object.Redo.Response](#anytype.Rpc.Object.Redo.Response)
    - [Rpc.Object.Redo.Response.Error](#anytype.Rpc.Object.Redo.Response.Error)
    - [Rpc.Object.Search](#anytype.Rpc.Object.Search)
    - [Rpc.Object.Search.Request](#anytype.Rpc.Object.Search.Request)
    - [Rpc.Object.Search.Response](#anytype.Rpc.Object.Search.Response)
    - [Rpc.Object.Search.Response.Error](#anytype.Rpc.Object.Search.Response.Error)
    - [Rpc.Object.SearchSubscribe](#anytype.Rpc.Object.SearchSubscribe)
    - [Rpc.Object.SearchSubscribe.Request](#anytype.Rpc.Object.SearchSubscribe.Request)
    - [Rpc.Object.SearchSubscribe.Response](#anytype.Rpc.Object.SearchSubscribe.Response)
    - [Rpc.Object.SearchSubscribe.Response.Error](#anytype.Rpc.Object.SearchSubscribe.Response.Error)
    - [Rpc.Object.SearchUnsubscribe](#anytype.Rpc.Object.SearchUnsubscribe)
    - [Rpc.Object.SearchUnsubscribe.Request](#anytype.Rpc.Object.SearchUnsubscribe.Request)
    - [Rpc.Object.SearchUnsubscribe.Response](#anytype.Rpc.Object.SearchUnsubscribe.Response)
    - [Rpc.Object.SearchUnsubscribe.Response.Error](#anytype.Rpc.Object.SearchUnsubscribe.Response.Error)
    - [Rpc.Object.SetBreadcrumbs](#anytype.Rpc.Object.SetBreadcrumbs)
    - [Rpc.Object.SetBreadcrumbs.Request](#anytype.Rpc.Object.SetBreadcrumbs.Request)
    - [Rpc.Object.SetBreadcrumbs.Response](#anytype.Rpc.Object.SetBreadcrumbs.Response)
    - [Rpc.Object.SetBreadcrumbs.Response.Error](#anytype.Rpc.Object.SetBreadcrumbs.Response.Error)
    - [Rpc.Object.SetDetails](#anytype.Rpc.Object.SetDetails)
    - [Rpc.Object.SetDetails.Detail](#anytype.Rpc.Object.SetDetails.Detail)
    - [Rpc.Object.SetDetails.Request](#anytype.Rpc.Object.SetDetails.Request)
    - [Rpc.Object.SetDetails.Response](#anytype.Rpc.Object.SetDetails.Response)
    - [Rpc.Object.SetDetails.Response.Error](#anytype.Rpc.Object.SetDetails.Response.Error)
    - [Rpc.Object.SetIsArchived](#anytype.Rpc.Object.SetIsArchived)
    - [Rpc.Object.SetIsArchived.Request](#anytype.Rpc.Object.SetIsArchived.Request)
    - [Rpc.Object.SetIsArchived.Response](#anytype.Rpc.Object.SetIsArchived.Response)
    - [Rpc.Object.SetIsArchived.Response.Error](#anytype.Rpc.Object.SetIsArchived.Response.Error)
    - [Rpc.Object.SetIsFavorite](#anytype.Rpc.Object.SetIsFavorite)
    - [Rpc.Object.SetIsFavorite.Request](#anytype.Rpc.Object.SetIsFavorite.Request)
    - [Rpc.Object.SetIsFavorite.Response](#anytype.Rpc.Object.SetIsFavorite.Response)
    - [Rpc.Object.SetIsFavorite.Response.Error](#anytype.Rpc.Object.SetIsFavorite.Response.Error)
    - [Rpc.Object.SetLayout](#anytype.Rpc.Object.SetLayout)
    - [Rpc.Object.SetLayout.Request](#anytype.Rpc.Object.SetLayout.Request)
    - [Rpc.Object.SetLayout.Response](#anytype.Rpc.Object.SetLayout.Response)
    - [Rpc.Object.SetLayout.Response.Error](#anytype.Rpc.Object.SetLayout.Response.Error)
    - [Rpc.Object.SetObjectType](#anytype.Rpc.Object.SetObjectType)
    - [Rpc.Object.SetObjectType.Request](#anytype.Rpc.Object.SetObjectType.Request)
    - [Rpc.Object.SetObjectType.Response](#anytype.Rpc.Object.SetObjectType.Response)
    - [Rpc.Object.SetObjectType.Response.Error](#anytype.Rpc.Object.SetObjectType.Response.Error)
    - [Rpc.Object.ShareByLink](#anytype.Rpc.Object.ShareByLink)
    - [Rpc.Object.ShareByLink.Request](#anytype.Rpc.Object.ShareByLink.Request)
    - [Rpc.Object.ShareByLink.Response](#anytype.Rpc.Object.ShareByLink.Response)
    - [Rpc.Object.ShareByLink.Response.Error](#anytype.Rpc.Object.ShareByLink.Response.Error)
    - [Rpc.Object.Show](#anytype.Rpc.Object.Show)
    - [Rpc.Object.Show.Request](#anytype.Rpc.Object.Show.Request)
    - [Rpc.Object.Show.Response](#anytype.Rpc.Object.Show.Response)
    - [Rpc.Object.Show.Response.Error](#anytype.Rpc.Object.Show.Response.Error)
    - [Rpc.Object.SubscribeIds](#anytype.Rpc.Object.SubscribeIds)
    - [Rpc.Object.SubscribeIds.Request](#anytype.Rpc.Object.SubscribeIds.Request)
    - [Rpc.Object.SubscribeIds.Response](#anytype.Rpc.Object.SubscribeIds.Response)
    - [Rpc.Object.SubscribeIds.Response.Error](#anytype.Rpc.Object.SubscribeIds.Response.Error)
    - [Rpc.Object.ToSet](#anytype.Rpc.Object.ToSet)
    - [Rpc.Object.ToSet.Request](#anytype.Rpc.Object.ToSet.Request)
    - [Rpc.Object.ToSet.Response](#anytype.Rpc.Object.ToSet.Response)
    - [Rpc.Object.ToSet.Response.Error](#anytype.Rpc.Object.ToSet.Response.Error)
    - [Rpc.Object.Undo](#anytype.Rpc.Object.Undo)
    - [Rpc.Object.Undo.Request](#anytype.Rpc.Object.Undo.Request)
    - [Rpc.Object.Undo.Response](#anytype.Rpc.Object.Undo.Response)
    - [Rpc.Object.Undo.Response.Error](#anytype.Rpc.Object.Undo.Response.Error)
    - [Rpc.Object.UndoRedoCounter](#anytype.Rpc.Object.UndoRedoCounter)
    - [Rpc.ObjectRelation](#anytype.Rpc.ObjectRelation)
    - [Rpc.ObjectRelation.Add](#anytype.Rpc.ObjectRelation.Add)
    - [Rpc.ObjectRelation.Add.Request](#anytype.Rpc.ObjectRelation.Add.Request)
    - [Rpc.ObjectRelation.Add.Response](#anytype.Rpc.ObjectRelation.Add.Response)
    - [Rpc.ObjectRelation.Add.Response.Error](#anytype.Rpc.ObjectRelation.Add.Response.Error)
    - [Rpc.ObjectRelation.AddFeatured](#anytype.Rpc.ObjectRelation.AddFeatured)
    - [Rpc.ObjectRelation.AddFeatured.Request](#anytype.Rpc.ObjectRelation.AddFeatured.Request)
    - [Rpc.ObjectRelation.AddFeatured.Response](#anytype.Rpc.ObjectRelation.AddFeatured.Response)
    - [Rpc.ObjectRelation.AddFeatured.Response.Error](#anytype.Rpc.ObjectRelation.AddFeatured.Response.Error)
    - [Rpc.ObjectRelation.Delete](#anytype.Rpc.ObjectRelation.Delete)
    - [Rpc.ObjectRelation.Delete.Request](#anytype.Rpc.ObjectRelation.Delete.Request)
    - [Rpc.ObjectRelation.Delete.Response](#anytype.Rpc.ObjectRelation.Delete.Response)
    - [Rpc.ObjectRelation.Delete.Response.Error](#anytype.Rpc.ObjectRelation.Delete.Response.Error)
    - [Rpc.ObjectRelation.ListAvailable](#anytype.Rpc.ObjectRelation.ListAvailable)
    - [Rpc.ObjectRelation.ListAvailable.Request](#anytype.Rpc.ObjectRelation.ListAvailable.Request)
    - [Rpc.ObjectRelation.ListAvailable.Response](#anytype.Rpc.ObjectRelation.ListAvailable.Response)
    - [Rpc.ObjectRelation.ListAvailable.Response.Error](#anytype.Rpc.ObjectRelation.ListAvailable.Response.Error)
    - [Rpc.ObjectRelation.RemoveFeatured](#anytype.Rpc.ObjectRelation.RemoveFeatured)
    - [Rpc.ObjectRelation.RemoveFeatured.Request](#anytype.Rpc.ObjectRelation.RemoveFeatured.Request)
    - [Rpc.ObjectRelation.RemoveFeatured.Response](#anytype.Rpc.ObjectRelation.RemoveFeatured.Response)
    - [Rpc.ObjectRelation.RemoveFeatured.Response.Error](#anytype.Rpc.ObjectRelation.RemoveFeatured.Response.Error)
    - [Rpc.ObjectRelation.Update](#anytype.Rpc.ObjectRelation.Update)
    - [Rpc.ObjectRelation.Update.Request](#anytype.Rpc.ObjectRelation.Update.Request)
    - [Rpc.ObjectRelation.Update.Response](#anytype.Rpc.ObjectRelation.Update.Response)
    - [Rpc.ObjectRelation.Update.Response.Error](#anytype.Rpc.ObjectRelation.Update.Response.Error)
    - [Rpc.ObjectRelationOption](#anytype.Rpc.ObjectRelationOption)
    - [Rpc.ObjectRelationOption.Add](#anytype.Rpc.ObjectRelationOption.Add)
    - [Rpc.ObjectRelationOption.Add.Request](#anytype.Rpc.ObjectRelationOption.Add.Request)
    - [Rpc.ObjectRelationOption.Add.Response](#anytype.Rpc.ObjectRelationOption.Add.Response)
    - [Rpc.ObjectRelationOption.Add.Response.Error](#anytype.Rpc.ObjectRelationOption.Add.Response.Error)
    - [Rpc.ObjectRelationOption.Delete](#anytype.Rpc.ObjectRelationOption.Delete)
    - [Rpc.ObjectRelationOption.Delete.Request](#anytype.Rpc.ObjectRelationOption.Delete.Request)
    - [Rpc.ObjectRelationOption.Delete.Response](#anytype.Rpc.ObjectRelationOption.Delete.Response)
    - [Rpc.ObjectRelationOption.Delete.Response.Error](#anytype.Rpc.ObjectRelationOption.Delete.Response.Error)
    - [Rpc.ObjectRelationOption.Update](#anytype.Rpc.ObjectRelationOption.Update)
    - [Rpc.ObjectRelationOption.Update.Request](#anytype.Rpc.ObjectRelationOption.Update.Request)
    - [Rpc.ObjectRelationOption.Update.Response](#anytype.Rpc.ObjectRelationOption.Update.Response)
    - [Rpc.ObjectRelationOption.Update.Response.Error](#anytype.Rpc.ObjectRelationOption.Update.Response.Error)
    - [Rpc.ObjectType](#anytype.Rpc.ObjectType)
    - [Rpc.ObjectType.Create](#anytype.Rpc.ObjectType.Create)
    - [Rpc.ObjectType.Create.Request](#anytype.Rpc.ObjectType.Create.Request)
    - [Rpc.ObjectType.Create.Response](#anytype.Rpc.ObjectType.Create.Response)
    - [Rpc.ObjectType.Create.Response.Error](#anytype.Rpc.ObjectType.Create.Response.Error)
    - [Rpc.ObjectType.List](#anytype.Rpc.ObjectType.List)
    - [Rpc.ObjectType.List.Request](#anytype.Rpc.ObjectType.List.Request)
    - [Rpc.ObjectType.List.Response](#anytype.Rpc.ObjectType.List.Response)
    - [Rpc.ObjectType.List.Response.Error](#anytype.Rpc.ObjectType.List.Response.Error)
    - [Rpc.ObjectType.Relation](#anytype.Rpc.ObjectType.Relation)
    - [Rpc.ObjectType.Relation.Add](#anytype.Rpc.ObjectType.Relation.Add)
    - [Rpc.ObjectType.Relation.Add.Request](#anytype.Rpc.ObjectType.Relation.Add.Request)
    - [Rpc.ObjectType.Relation.Add.Response](#anytype.Rpc.ObjectType.Relation.Add.Response)
    - [Rpc.ObjectType.Relation.Add.Response.Error](#anytype.Rpc.ObjectType.Relation.Add.Response.Error)
    - [Rpc.ObjectType.Relation.List](#anytype.Rpc.ObjectType.Relation.List)
    - [Rpc.ObjectType.Relation.List.Request](#anytype.Rpc.ObjectType.Relation.List.Request)
    - [Rpc.ObjectType.Relation.List.Response](#anytype.Rpc.ObjectType.Relation.List.Response)
    - [Rpc.ObjectType.Relation.List.Response.Error](#anytype.Rpc.ObjectType.Relation.List.Response.Error)
    - [Rpc.ObjectType.Relation.Remove](#anytype.Rpc.ObjectType.Relation.Remove)
    - [Rpc.ObjectType.Relation.Remove.Request](#anytype.Rpc.ObjectType.Relation.Remove.Request)
    - [Rpc.ObjectType.Relation.Remove.Response](#anytype.Rpc.ObjectType.Relation.Remove.Response)
    - [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error)
    - [Rpc.ObjectType.Relation.Update](#anytype.Rpc.ObjectType.Relation.Update)
    - [Rpc.ObjectType.Relation.Update.Request](#anytype.Rpc.ObjectType.Relation.Update.Request)
    - [Rpc.ObjectType.Relation.Update.Response](#anytype.Rpc.ObjectType.Relation.Update.Response)
    - [Rpc.ObjectType.Relation.Update.Response.Error](#anytype.Rpc.ObjectType.Relation.Update.Response.Error)
    - [Rpc.Process](#anytype.Rpc.Process)
    - [Rpc.Process.Cancel](#anytype.Rpc.Process.Cancel)
    - [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request)
    - [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response)
    - [Rpc.Process.Cancel.Response.Error](#anytype.Rpc.Process.Cancel.Response.Error)
    - [Rpc.Template](#anytype.Rpc.Template)
    - [Rpc.Template.Clone](#anytype.Rpc.Template.Clone)
    - [Rpc.Template.Clone.Request](#anytype.Rpc.Template.Clone.Request)
    - [Rpc.Template.Clone.Response](#anytype.Rpc.Template.Clone.Response)
    - [Rpc.Template.Clone.Response.Error](#anytype.Rpc.Template.Clone.Response.Error)
    - [Rpc.Template.CreateFromObject](#anytype.Rpc.Template.CreateFromObject)
    - [Rpc.Template.CreateFromObject.Request](#anytype.Rpc.Template.CreateFromObject.Request)
    - [Rpc.Template.CreateFromObject.Response](#anytype.Rpc.Template.CreateFromObject.Response)
    - [Rpc.Template.CreateFromObject.Response.Error](#anytype.Rpc.Template.CreateFromObject.Response.Error)
    - [Rpc.Template.CreateFromObjectType](#anytype.Rpc.Template.CreateFromObjectType)
    - [Rpc.Template.CreateFromObjectType.Request](#anytype.Rpc.Template.CreateFromObjectType.Request)
    - [Rpc.Template.CreateFromObjectType.Response](#anytype.Rpc.Template.CreateFromObjectType.Response)
    - [Rpc.Template.CreateFromObjectType.Response.Error](#anytype.Rpc.Template.CreateFromObjectType.Response.Error)
    - [Rpc.Template.ExportAll](#anytype.Rpc.Template.ExportAll)
    - [Rpc.Template.ExportAll.Request](#anytype.Rpc.Template.ExportAll.Request)
    - [Rpc.Template.ExportAll.Response](#anytype.Rpc.Template.ExportAll.Response)
    - [Rpc.Template.ExportAll.Response.Error](#anytype.Rpc.Template.ExportAll.Response.Error)
    - [Rpc.Unsplash](#anytype.Rpc.Unsplash)
    - [Rpc.Unsplash.Download](#anytype.Rpc.Unsplash.Download)
    - [Rpc.Unsplash.Download.Request](#anytype.Rpc.Unsplash.Download.Request)
    - [Rpc.Unsplash.Download.Response](#anytype.Rpc.Unsplash.Download.Response)
    - [Rpc.Unsplash.Download.Response.Error](#anytype.Rpc.Unsplash.Download.Response.Error)
    - [Rpc.Unsplash.Search](#anytype.Rpc.Unsplash.Search)
    - [Rpc.Unsplash.Search.Request](#anytype.Rpc.Unsplash.Search.Request)
    - [Rpc.Unsplash.Search.Response](#anytype.Rpc.Unsplash.Search.Response)
    - [Rpc.Unsplash.Search.Response.Error](#anytype.Rpc.Unsplash.Search.Response.Error)
    - [Rpc.Unsplash.Search.Response.Picture](#anytype.Rpc.Unsplash.Search.Response.Picture)
    - [Rpc.Wallet](#anytype.Rpc.Wallet)
    - [Rpc.Wallet.Convert](#anytype.Rpc.Wallet.Convert)
    - [Rpc.Wallet.Convert.Request](#anytype.Rpc.Wallet.Convert.Request)
    - [Rpc.Wallet.Convert.Response](#anytype.Rpc.Wallet.Convert.Response)
    - [Rpc.Wallet.Convert.Response.Error](#anytype.Rpc.Wallet.Convert.Response.Error)
    - [Rpc.Wallet.Create](#anytype.Rpc.Wallet.Create)
    - [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request)
    - [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response)
    - [Rpc.Wallet.Create.Response.Error](#anytype.Rpc.Wallet.Create.Response.Error)
    - [Rpc.Wallet.Recover](#anytype.Rpc.Wallet.Recover)
    - [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request)
    - [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response)
    - [Rpc.Wallet.Recover.Response.Error](#anytype.Rpc.Wallet.Recover.Response.Error)
    - [Rpc.Workspace](#anytype.Rpc.Workspace)
    - [Rpc.Workspace.Create](#anytype.Rpc.Workspace.Create)
    - [Rpc.Workspace.Create.Request](#anytype.Rpc.Workspace.Create.Request)
    - [Rpc.Workspace.Create.Response](#anytype.Rpc.Workspace.Create.Response)
    - [Rpc.Workspace.Create.Response.Error](#anytype.Rpc.Workspace.Create.Response.Error)
    - [Rpc.Workspace.Export](#anytype.Rpc.Workspace.Export)
    - [Rpc.Workspace.Export.Request](#anytype.Rpc.Workspace.Export.Request)
    - [Rpc.Workspace.Export.Response](#anytype.Rpc.Workspace.Export.Response)
    - [Rpc.Workspace.Export.Response.Error](#anytype.Rpc.Workspace.Export.Response.Error)
    - [Rpc.Workspace.GetAll](#anytype.Rpc.Workspace.GetAll)
    - [Rpc.Workspace.GetAll.Request](#anytype.Rpc.Workspace.GetAll.Request)
    - [Rpc.Workspace.GetAll.Response](#anytype.Rpc.Workspace.GetAll.Response)
    - [Rpc.Workspace.GetAll.Response.Error](#anytype.Rpc.Workspace.GetAll.Response.Error)
    - [Rpc.Workspace.GetCurrent](#anytype.Rpc.Workspace.GetCurrent)
    - [Rpc.Workspace.GetCurrent.Request](#anytype.Rpc.Workspace.GetCurrent.Request)
    - [Rpc.Workspace.GetCurrent.Response](#anytype.Rpc.Workspace.GetCurrent.Response)
    - [Rpc.Workspace.GetCurrent.Response.Error](#anytype.Rpc.Workspace.GetCurrent.Response.Error)
    - [Rpc.Workspace.Select](#anytype.Rpc.Workspace.Select)
    - [Rpc.Workspace.Select.Request](#anytype.Rpc.Workspace.Select.Request)
    - [Rpc.Workspace.Select.Response](#anytype.Rpc.Workspace.Select.Response)
    - [Rpc.Workspace.Select.Response.Error](#anytype.Rpc.Workspace.Select.Response.Error)
    - [Rpc.Workspace.SetIsHighlighted](#anytype.Rpc.Workspace.SetIsHighlighted)
    - [Rpc.Workspace.SetIsHighlighted.Request](#anytype.Rpc.Workspace.SetIsHighlighted.Request)
    - [Rpc.Workspace.SetIsHighlighted.Response](#anytype.Rpc.Workspace.SetIsHighlighted.Response)
    - [Rpc.Workspace.SetIsHighlighted.Response.Error](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error)
  
    - [Rpc.Account.Create.Response.Error.Code](#anytype.Rpc.Account.Create.Response.Error.Code)
    - [Rpc.Account.Delete.Response.Error.Code](#anytype.Rpc.Account.Delete.Response.Error.Code)
    - [Rpc.Account.Recover.Response.Error.Code](#anytype.Rpc.Account.Recover.Response.Error.Code)
    - [Rpc.Account.Select.Response.Error.Code](#anytype.Rpc.Account.Select.Response.Error.Code)
    - [Rpc.Account.Stop.Response.Error.Code](#anytype.Rpc.Account.Stop.Response.Error.Code)
    - [Rpc.App.GetVersion.Response.Error.Code](#anytype.Rpc.App.GetVersion.Response.Error.Code)
    - [Rpc.App.Shutdown.Response.Error.Code](#anytype.Rpc.App.Shutdown.Response.Error.Code)
    - [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code)
    - [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code)
    - [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code)
    - [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code)
    - [Rpc.Block.Export.Response.Error.Code](#anytype.Rpc.Block.Export.Response.Error.Code)
    - [Rpc.Block.ListConvertToObjects.Response.Error.Code](#anytype.Rpc.Block.ListConvertToObjects.Response.Error.Code)
    - [Rpc.Block.ListDuplicate.Response.Error.Code](#anytype.Rpc.Block.ListDuplicate.Response.Error.Code)
    - [Rpc.Block.ListMoveToExistingObject.Response.Error.Code](#anytype.Rpc.Block.ListMoveToExistingObject.Response.Error.Code)
    - [Rpc.Block.ListMoveToNewObject.Response.Error.Code](#anytype.Rpc.Block.ListMoveToNewObject.Response.Error.Code)
    - [Rpc.Block.ListSetAlign.Response.Error.Code](#anytype.Rpc.Block.ListSetAlign.Response.Error.Code)
    - [Rpc.Block.ListSetBackgroundColor.Response.Error.Code](#anytype.Rpc.Block.ListSetBackgroundColor.Response.Error.Code)
    - [Rpc.Block.ListSetFields.Response.Error.Code](#anytype.Rpc.Block.ListSetFields.Response.Error.Code)
    - [Rpc.Block.ListTurnInto.Response.Error.Code](#anytype.Rpc.Block.ListTurnInto.Response.Error.Code)
    - [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code)
    - [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code)
    - [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code)
    - [Rpc.Block.SetFields.Response.Error.Code](#anytype.Rpc.Block.SetFields.Response.Error.Code)
    - [Rpc.Block.SetRestrictions.Response.Error.Code](#anytype.Rpc.Block.SetRestrictions.Response.Error.Code)
    - [Rpc.Block.Split.Request.Mode](#anytype.Rpc.Block.Split.Request.Mode)
    - [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code)
    - [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code)
    - [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code)
    - [Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code)
    - [Rpc.BlockBookmark.Fetch.Response.Error.Code](#anytype.Rpc.BlockBookmark.Fetch.Response.Error.Code)
    - [Rpc.BlockDataview.Relation.Add.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.Add.Response.Error.Code)
    - [Rpc.BlockDataview.Relation.Delete.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.Delete.Response.Error.Code)
    - [Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code)
    - [Rpc.BlockDataview.Relation.Update.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.Update.Response.Error.Code)
    - [Rpc.BlockDataview.SetSource.Response.Error.Code](#anytype.Rpc.BlockDataview.SetSource.Response.Error.Code)
    - [Rpc.BlockDataview.View.Create.Response.Error.Code](#anytype.Rpc.BlockDataview.View.Create.Response.Error.Code)
    - [Rpc.BlockDataview.View.Delete.Response.Error.Code](#anytype.Rpc.BlockDataview.View.Delete.Response.Error.Code)
    - [Rpc.BlockDataview.View.SetActive.Response.Error.Code](#anytype.Rpc.BlockDataview.View.SetActive.Response.Error.Code)
    - [Rpc.BlockDataview.View.SetPosition.Response.Error.Code](#anytype.Rpc.BlockDataview.View.SetPosition.Response.Error.Code)
    - [Rpc.BlockDataview.View.Update.Response.Error.Code](#anytype.Rpc.BlockDataview.View.Update.Response.Error.Code)
    - [Rpc.BlockDataviewRecord.AddRelationOption.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response.Error.Code)
    - [Rpc.BlockDataviewRecord.Create.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.Create.Response.Error.Code)
    - [Rpc.BlockDataviewRecord.Delete.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.Delete.Response.Error.Code)
    - [Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error.Code)
    - [Rpc.BlockDataviewRecord.Update.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.Update.Response.Error.Code)
    - [Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error.Code)
    - [Rpc.BlockDiv.ListSetStyle.Response.Error.Code](#anytype.Rpc.BlockDiv.ListSetStyle.Response.Error.Code)
    - [Rpc.BlockFile.CreateAndUpload.Response.Error.Code](#anytype.Rpc.BlockFile.CreateAndUpload.Response.Error.Code)
    - [Rpc.BlockFile.ListSetStyle.Response.Error.Code](#anytype.Rpc.BlockFile.ListSetStyle.Response.Error.Code)
    - [Rpc.BlockFile.SetName.Response.Error.Code](#anytype.Rpc.BlockFile.SetName.Response.Error.Code)
    - [Rpc.BlockImage.SetName.Response.Error.Code](#anytype.Rpc.BlockImage.SetName.Response.Error.Code)
    - [Rpc.BlockImage.SetWidth.Response.Error.Code](#anytype.Rpc.BlockImage.SetWidth.Response.Error.Code)
    - [Rpc.BlockLatex.SetText.Response.Error.Code](#anytype.Rpc.BlockLatex.SetText.Response.Error.Code)
    - [Rpc.BlockLink.CreateLinkToNewObject.Response.Error.Code](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Response.Error.Code)
    - [Rpc.BlockLink.CreateLinkToNewSet.Response.Error.Code](#anytype.Rpc.BlockLink.CreateLinkToNewSet.Response.Error.Code)
    - [Rpc.BlockLink.SetTargetBlockId.Response.Error.Code](#anytype.Rpc.BlockLink.SetTargetBlockId.Response.Error.Code)
    - [Rpc.BlockRelation.Add.Response.Error.Code](#anytype.Rpc.BlockRelation.Add.Response.Error.Code)
    - [Rpc.BlockRelation.SetKey.Response.Error.Code](#anytype.Rpc.BlockRelation.SetKey.Response.Error.Code)
    - [Rpc.BlockText.ListSetColor.Response.Error.Code](#anytype.Rpc.BlockText.ListSetColor.Response.Error.Code)
    - [Rpc.BlockText.ListSetMark.Response.Error.Code](#anytype.Rpc.BlockText.ListSetMark.Response.Error.Code)
    - [Rpc.BlockText.ListSetStyle.Response.Error.Code](#anytype.Rpc.BlockText.ListSetStyle.Response.Error.Code)
    - [Rpc.BlockText.SetChecked.Response.Error.Code](#anytype.Rpc.BlockText.SetChecked.Response.Error.Code)
    - [Rpc.BlockText.SetColor.Response.Error.Code](#anytype.Rpc.BlockText.SetColor.Response.Error.Code)
    - [Rpc.BlockText.SetIcon.Response.Error.Code](#anytype.Rpc.BlockText.SetIcon.Response.Error.Code)
    - [Rpc.BlockText.SetMarks.Get.Response.Error.Code](#anytype.Rpc.BlockText.SetMarks.Get.Response.Error.Code)
    - [Rpc.BlockText.SetStyle.Response.Error.Code](#anytype.Rpc.BlockText.SetStyle.Response.Error.Code)
    - [Rpc.BlockText.SetText.Response.Error.Code](#anytype.Rpc.BlockText.SetText.Response.Error.Code)
    - [Rpc.BlockVideo.SetName.Response.Error.Code](#anytype.Rpc.BlockVideo.SetName.Response.Error.Code)
    - [Rpc.BlockVideo.SetWidth.Response.Error.Code](#anytype.Rpc.BlockVideo.SetWidth.Response.Error.Code)
    - [Rpc.Debug.ExportLocalstore.Response.Error.Code](#anytype.Rpc.Debug.ExportLocalstore.Response.Error.Code)
    - [Rpc.Debug.Ping.Response.Error.Code](#anytype.Rpc.Debug.Ping.Response.Error.Code)
    - [Rpc.Debug.Sync.Response.Error.Code](#anytype.Rpc.Debug.Sync.Response.Error.Code)
    - [Rpc.Debug.Thread.Response.Error.Code](#anytype.Rpc.Debug.Thread.Response.Error.Code)
    - [Rpc.Debug.Tree.Response.Error.Code](#anytype.Rpc.Debug.Tree.Response.Error.Code)
    - [Rpc.File.Download.Response.Error.Code](#anytype.Rpc.File.Download.Response.Error.Code)
    - [Rpc.File.Drop.Response.Error.Code](#anytype.Rpc.File.Drop.Response.Error.Code)
    - [Rpc.File.ListOffload.Response.Error.Code](#anytype.Rpc.File.ListOffload.Response.Error.Code)
    - [Rpc.File.Offload.Response.Error.Code](#anytype.Rpc.File.Offload.Response.Error.Code)
    - [Rpc.File.Upload.Response.Error.Code](#anytype.Rpc.File.Upload.Response.Error.Code)
    - [Rpc.GenericErrorResponse.Error.Code](#anytype.Rpc.GenericErrorResponse.Error.Code)
    - [Rpc.History.GetVersions.Response.Error.Code](#anytype.Rpc.History.GetVersions.Response.Error.Code)
    - [Rpc.History.SetVersion.Response.Error.Code](#anytype.Rpc.History.SetVersion.Response.Error.Code)
    - [Rpc.History.ShowVersion.Response.Error.Code](#anytype.Rpc.History.ShowVersion.Response.Error.Code)
    - [Rpc.LinkPreview.Response.Error.Code](#anytype.Rpc.LinkPreview.Response.Error.Code)
    - [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code)
    - [Rpc.Metrics.SetParameters.Response.Error.Code](#anytype.Rpc.Metrics.SetParameters.Response.Error.Code)
    - [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code)
    - [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype.Rpc.Navigation.ListObjects.Response.Error.Code)
    - [Rpc.Object.AddWithObjectId.Response.Error.Code](#anytype.Rpc.Object.AddWithObjectId.Response.Error.Code)
    - [Rpc.Object.ApplyTemplate.Response.Error.Code](#anytype.Rpc.Object.ApplyTemplate.Response.Error.Code)
    - [Rpc.Object.Close.Response.Error.Code](#anytype.Rpc.Object.Close.Response.Error.Code)
    - [Rpc.Object.Create.Response.Error.Code](#anytype.Rpc.Object.Create.Response.Error.Code)
    - [Rpc.Object.CreateSet.Response.Error.Code](#anytype.Rpc.Object.CreateSet.Response.Error.Code)
    - [Rpc.Object.Duplicate.Response.Error.Code](#anytype.Rpc.Object.Duplicate.Response.Error.Code)
    - [Rpc.Object.Export.Format](#anytype.Rpc.Object.Export.Format)
    - [Rpc.Object.Export.Response.Error.Code](#anytype.Rpc.Object.Export.Response.Error.Code)
    - [Rpc.Object.Graph.Edge.Type](#anytype.Rpc.Object.Graph.Edge.Type)
    - [Rpc.Object.Graph.Response.Error.Code](#anytype.Rpc.Object.Graph.Response.Error.Code)
    - [Rpc.Object.ImportMarkdown.Response.Error.Code](#anytype.Rpc.Object.ImportMarkdown.Response.Error.Code)
    - [Rpc.Object.ListDelete.Response.Error.Code](#anytype.Rpc.Object.ListDelete.Response.Error.Code)
    - [Rpc.Object.ListSetIsArchived.Response.Error.Code](#anytype.Rpc.Object.ListSetIsArchived.Response.Error.Code)
    - [Rpc.Object.ListSetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.ListSetIsFavorite.Response.Error.Code)
    - [Rpc.Object.Open.Response.Error.Code](#anytype.Rpc.Object.Open.Response.Error.Code)
    - [Rpc.Object.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Object.OpenBreadcrumbs.Response.Error.Code)
    - [Rpc.Object.Redo.Response.Error.Code](#anytype.Rpc.Object.Redo.Response.Error.Code)
    - [Rpc.Object.Search.Response.Error.Code](#anytype.Rpc.Object.Search.Response.Error.Code)
    - [Rpc.Object.SearchSubscribe.Response.Error.Code](#anytype.Rpc.Object.SearchSubscribe.Response.Error.Code)
    - [Rpc.Object.SearchUnsubscribe.Response.Error.Code](#anytype.Rpc.Object.SearchUnsubscribe.Response.Error.Code)
    - [Rpc.Object.SetBreadcrumbs.Response.Error.Code](#anytype.Rpc.Object.SetBreadcrumbs.Response.Error.Code)
    - [Rpc.Object.SetDetails.Response.Error.Code](#anytype.Rpc.Object.SetDetails.Response.Error.Code)
    - [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype.Rpc.Object.SetIsArchived.Response.Error.Code)
    - [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.SetIsFavorite.Response.Error.Code)
    - [Rpc.Object.SetLayout.Response.Error.Code](#anytype.Rpc.Object.SetLayout.Response.Error.Code)
    - [Rpc.Object.SetObjectType.Response.Error.Code](#anytype.Rpc.Object.SetObjectType.Response.Error.Code)
    - [Rpc.Object.ShareByLink.Response.Error.Code](#anytype.Rpc.Object.ShareByLink.Response.Error.Code)
    - [Rpc.Object.Show.Response.Error.Code](#anytype.Rpc.Object.Show.Response.Error.Code)
    - [Rpc.Object.SubscribeIds.Response.Error.Code](#anytype.Rpc.Object.SubscribeIds.Response.Error.Code)
    - [Rpc.Object.ToSet.Response.Error.Code](#anytype.Rpc.Object.ToSet.Response.Error.Code)
    - [Rpc.Object.Undo.Response.Error.Code](#anytype.Rpc.Object.Undo.Response.Error.Code)
    - [Rpc.ObjectRelation.Add.Response.Error.Code](#anytype.Rpc.ObjectRelation.Add.Response.Error.Code)
    - [Rpc.ObjectRelation.AddFeatured.Response.Error.Code](#anytype.Rpc.ObjectRelation.AddFeatured.Response.Error.Code)
    - [Rpc.ObjectRelation.Delete.Response.Error.Code](#anytype.Rpc.ObjectRelation.Delete.Response.Error.Code)
    - [Rpc.ObjectRelation.ListAvailable.Response.Error.Code](#anytype.Rpc.ObjectRelation.ListAvailable.Response.Error.Code)
    - [Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code](#anytype.Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code)
    - [Rpc.ObjectRelation.Update.Response.Error.Code](#anytype.Rpc.ObjectRelation.Update.Response.Error.Code)
    - [Rpc.ObjectRelationOption.Add.Response.Error.Code](#anytype.Rpc.ObjectRelationOption.Add.Response.Error.Code)
    - [Rpc.ObjectRelationOption.Delete.Response.Error.Code](#anytype.Rpc.ObjectRelationOption.Delete.Response.Error.Code)
    - [Rpc.ObjectRelationOption.Update.Response.Error.Code](#anytype.Rpc.ObjectRelationOption.Update.Response.Error.Code)
    - [Rpc.ObjectType.Create.Response.Error.Code](#anytype.Rpc.ObjectType.Create.Response.Error.Code)
    - [Rpc.ObjectType.List.Response.Error.Code](#anytype.Rpc.ObjectType.List.Response.Error.Code)
    - [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code)
    - [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.List.Response.Error.Code)
    - [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code)
    - [Rpc.ObjectType.Relation.Update.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code)
    - [Rpc.Process.Cancel.Response.Error.Code](#anytype.Rpc.Process.Cancel.Response.Error.Code)
    - [Rpc.Template.Clone.Response.Error.Code](#anytype.Rpc.Template.Clone.Response.Error.Code)
    - [Rpc.Template.CreateFromObject.Response.Error.Code](#anytype.Rpc.Template.CreateFromObject.Response.Error.Code)
    - [Rpc.Template.CreateFromObjectType.Response.Error.Code](#anytype.Rpc.Template.CreateFromObjectType.Response.Error.Code)
    - [Rpc.Template.ExportAll.Response.Error.Code](#anytype.Rpc.Template.ExportAll.Response.Error.Code)
    - [Rpc.Unsplash.Download.Response.Error.Code](#anytype.Rpc.Unsplash.Download.Response.Error.Code)
    - [Rpc.Unsplash.Search.Response.Error.Code](#anytype.Rpc.Unsplash.Search.Response.Error.Code)
    - [Rpc.Wallet.Convert.Response.Error.Code](#anytype.Rpc.Wallet.Convert.Response.Error.Code)
    - [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code)
    - [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code)
    - [Rpc.Workspace.Create.Response.Error.Code](#anytype.Rpc.Workspace.Create.Response.Error.Code)
    - [Rpc.Workspace.Export.Response.Error.Code](#anytype.Rpc.Workspace.Export.Response.Error.Code)
    - [Rpc.Workspace.GetAll.Response.Error.Code](#anytype.Rpc.Workspace.GetAll.Response.Error.Code)
    - [Rpc.Workspace.GetCurrent.Response.Error.Code](#anytype.Rpc.Workspace.GetCurrent.Response.Error.Code)
    - [Rpc.Workspace.Select.Response.Error.Code](#anytype.Rpc.Workspace.Select.Response.Error.Code)
    - [Rpc.Workspace.SetIsHighlighted.Response.Error.Code](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error.Code)
  
- [pb/protos/events.proto](#pb/protos/events.proto)
    - [Event](#anytype.Event)
    - [Event.Account](#anytype.Event.Account)
    - [Event.Account.Config](#anytype.Event.Account.Config)
    - [Event.Account.Config.Update](#anytype.Event.Account.Config.Update)
    - [Event.Account.Details](#anytype.Event.Account.Details)
    - [Event.Account.Show](#anytype.Event.Account.Show)
    - [Event.Account.Update](#anytype.Event.Account.Update)
    - [Event.Block](#anytype.Event.Block)
    - [Event.Block.Add](#anytype.Event.Block.Add)
    - [Event.Block.Dataview](#anytype.Event.Block.Dataview)
    - [Event.Block.Dataview.RecordsDelete](#anytype.Event.Block.Dataview.RecordsDelete)
    - [Event.Block.Dataview.RecordsInsert](#anytype.Event.Block.Dataview.RecordsInsert)
    - [Event.Block.Dataview.RecordsSet](#anytype.Event.Block.Dataview.RecordsSet)
    - [Event.Block.Dataview.RecordsUpdate](#anytype.Event.Block.Dataview.RecordsUpdate)
    - [Event.Block.Dataview.RelationDelete](#anytype.Event.Block.Dataview.RelationDelete)
    - [Event.Block.Dataview.RelationSet](#anytype.Event.Block.Dataview.RelationSet)
    - [Event.Block.Dataview.SourceSet](#anytype.Event.Block.Dataview.SourceSet)
    - [Event.Block.Dataview.ViewDelete](#anytype.Event.Block.Dataview.ViewDelete)
    - [Event.Block.Dataview.ViewOrder](#anytype.Event.Block.Dataview.ViewOrder)
    - [Event.Block.Dataview.ViewSet](#anytype.Event.Block.Dataview.ViewSet)
    - [Event.Block.Delete](#anytype.Event.Block.Delete)
    - [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload)
    - [Event.Block.Fill](#anytype.Event.Block.Fill)
    - [Event.Block.Fill.Align](#anytype.Event.Block.Fill.Align)
    - [Event.Block.Fill.BackgroundColor](#anytype.Event.Block.Fill.BackgroundColor)
    - [Event.Block.Fill.Bookmark](#anytype.Event.Block.Fill.Bookmark)
    - [Event.Block.Fill.Bookmark.Description](#anytype.Event.Block.Fill.Bookmark.Description)
    - [Event.Block.Fill.Bookmark.FaviconHash](#anytype.Event.Block.Fill.Bookmark.FaviconHash)
    - [Event.Block.Fill.Bookmark.ImageHash](#anytype.Event.Block.Fill.Bookmark.ImageHash)
    - [Event.Block.Fill.Bookmark.Title](#anytype.Event.Block.Fill.Bookmark.Title)
    - [Event.Block.Fill.Bookmark.Type](#anytype.Event.Block.Fill.Bookmark.Type)
    - [Event.Block.Fill.Bookmark.Url](#anytype.Event.Block.Fill.Bookmark.Url)
    - [Event.Block.Fill.ChildrenIds](#anytype.Event.Block.Fill.ChildrenIds)
    - [Event.Block.Fill.DatabaseRecords](#anytype.Event.Block.Fill.DatabaseRecords)
    - [Event.Block.Fill.Details](#anytype.Event.Block.Fill.Details)
    - [Event.Block.Fill.Div](#anytype.Event.Block.Fill.Div)
    - [Event.Block.Fill.Div.Style](#anytype.Event.Block.Fill.Div.Style)
    - [Event.Block.Fill.Fields](#anytype.Event.Block.Fill.Fields)
    - [Event.Block.Fill.File](#anytype.Event.Block.Fill.File)
    - [Event.Block.Fill.File.Hash](#anytype.Event.Block.Fill.File.Hash)
    - [Event.Block.Fill.File.Mime](#anytype.Event.Block.Fill.File.Mime)
    - [Event.Block.Fill.File.Name](#anytype.Event.Block.Fill.File.Name)
    - [Event.Block.Fill.File.Size](#anytype.Event.Block.Fill.File.Size)
    - [Event.Block.Fill.File.State](#anytype.Event.Block.Fill.File.State)
    - [Event.Block.Fill.File.Style](#anytype.Event.Block.Fill.File.Style)
    - [Event.Block.Fill.File.Type](#anytype.Event.Block.Fill.File.Type)
    - [Event.Block.Fill.File.Width](#anytype.Event.Block.Fill.File.Width)
    - [Event.Block.Fill.Link](#anytype.Event.Block.Fill.Link)
    - [Event.Block.Fill.Link.Fields](#anytype.Event.Block.Fill.Link.Fields)
    - [Event.Block.Fill.Link.Style](#anytype.Event.Block.Fill.Link.Style)
    - [Event.Block.Fill.Link.TargetBlockId](#anytype.Event.Block.Fill.Link.TargetBlockId)
    - [Event.Block.Fill.Restrictions](#anytype.Event.Block.Fill.Restrictions)
    - [Event.Block.Fill.Text](#anytype.Event.Block.Fill.Text)
    - [Event.Block.Fill.Text.Checked](#anytype.Event.Block.Fill.Text.Checked)
    - [Event.Block.Fill.Text.Color](#anytype.Event.Block.Fill.Text.Color)
    - [Event.Block.Fill.Text.Marks](#anytype.Event.Block.Fill.Text.Marks)
    - [Event.Block.Fill.Text.Style](#anytype.Event.Block.Fill.Text.Style)
    - [Event.Block.Fill.Text.Text](#anytype.Event.Block.Fill.Text.Text)
    - [Event.Block.MarksInfo](#anytype.Event.Block.MarksInfo)
    - [Event.Block.Set](#anytype.Event.Block.Set)
    - [Event.Block.Set.Align](#anytype.Event.Block.Set.Align)
    - [Event.Block.Set.BackgroundColor](#anytype.Event.Block.Set.BackgroundColor)
    - [Event.Block.Set.Bookmark](#anytype.Event.Block.Set.Bookmark)
    - [Event.Block.Set.Bookmark.Description](#anytype.Event.Block.Set.Bookmark.Description)
    - [Event.Block.Set.Bookmark.FaviconHash](#anytype.Event.Block.Set.Bookmark.FaviconHash)
    - [Event.Block.Set.Bookmark.ImageHash](#anytype.Event.Block.Set.Bookmark.ImageHash)
    - [Event.Block.Set.Bookmark.Title](#anytype.Event.Block.Set.Bookmark.Title)
    - [Event.Block.Set.Bookmark.Type](#anytype.Event.Block.Set.Bookmark.Type)
    - [Event.Block.Set.Bookmark.Url](#anytype.Event.Block.Set.Bookmark.Url)
    - [Event.Block.Set.ChildrenIds](#anytype.Event.Block.Set.ChildrenIds)
    - [Event.Block.Set.Div](#anytype.Event.Block.Set.Div)
    - [Event.Block.Set.Div.Style](#anytype.Event.Block.Set.Div.Style)
    - [Event.Block.Set.Fields](#anytype.Event.Block.Set.Fields)
    - [Event.Block.Set.File](#anytype.Event.Block.Set.File)
    - [Event.Block.Set.File.Hash](#anytype.Event.Block.Set.File.Hash)
    - [Event.Block.Set.File.Mime](#anytype.Event.Block.Set.File.Mime)
    - [Event.Block.Set.File.Name](#anytype.Event.Block.Set.File.Name)
    - [Event.Block.Set.File.Size](#anytype.Event.Block.Set.File.Size)
    - [Event.Block.Set.File.State](#anytype.Event.Block.Set.File.State)
    - [Event.Block.Set.File.Style](#anytype.Event.Block.Set.File.Style)
    - [Event.Block.Set.File.Type](#anytype.Event.Block.Set.File.Type)
    - [Event.Block.Set.File.Width](#anytype.Event.Block.Set.File.Width)
    - [Event.Block.Set.Latex](#anytype.Event.Block.Set.Latex)
    - [Event.Block.Set.Latex.Text](#anytype.Event.Block.Set.Latex.Text)
    - [Event.Block.Set.Link](#anytype.Event.Block.Set.Link)
    - [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields)
    - [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style)
    - [Event.Block.Set.Link.TargetBlockId](#anytype.Event.Block.Set.Link.TargetBlockId)
    - [Event.Block.Set.Relation](#anytype.Event.Block.Set.Relation)
    - [Event.Block.Set.Relation.Key](#anytype.Event.Block.Set.Relation.Key)
    - [Event.Block.Set.Restrictions](#anytype.Event.Block.Set.Restrictions)
    - [Event.Block.Set.Text](#anytype.Event.Block.Set.Text)
    - [Event.Block.Set.Text.Checked](#anytype.Event.Block.Set.Text.Checked)
    - [Event.Block.Set.Text.Color](#anytype.Event.Block.Set.Text.Color)
    - [Event.Block.Set.Text.IconEmoji](#anytype.Event.Block.Set.Text.IconEmoji)
    - [Event.Block.Set.Text.IconImage](#anytype.Event.Block.Set.Text.IconImage)
    - [Event.Block.Set.Text.Marks](#anytype.Event.Block.Set.Text.Marks)
    - [Event.Block.Set.Text.Style](#anytype.Event.Block.Set.Text.Style)
    - [Event.Block.Set.Text.Text](#anytype.Event.Block.Set.Text.Text)
    - [Event.Message](#anytype.Event.Message)
    - [Event.Object](#anytype.Event.Object)
    - [Event.Object.Details](#anytype.Event.Object.Details)
    - [Event.Object.Details.Amend](#anytype.Event.Object.Details.Amend)
    - [Event.Object.Details.Amend.KeyValue](#anytype.Event.Object.Details.Amend.KeyValue)
    - [Event.Object.Details.Set](#anytype.Event.Object.Details.Set)
    - [Event.Object.Details.Unset](#anytype.Event.Object.Details.Unset)
    - [Event.Object.Relation](#anytype.Event.Object.Relation)
    - [Event.Object.Relation.Remove](#anytype.Event.Object.Relation.Remove)
    - [Event.Object.Relation.Set](#anytype.Event.Object.Relation.Set)
    - [Event.Object.Relations](#anytype.Event.Object.Relations)
    - [Event.Object.Relations.Amend](#anytype.Event.Object.Relations.Amend)
    - [Event.Object.Relations.Remove](#anytype.Event.Object.Relations.Remove)
    - [Event.Object.Relations.Set](#anytype.Event.Object.Relations.Set)
    - [Event.Object.Remove](#anytype.Event.Object.Remove)
    - [Event.Object.Show](#anytype.Event.Object.Show)
    - [Event.Object.Show.RelationWithValuePerObject](#anytype.Event.Object.Show.RelationWithValuePerObject)
    - [Event.Object.Subscription](#anytype.Event.Object.Subscription)
    - [Event.Object.Subscription.Add](#anytype.Event.Object.Subscription.Add)
    - [Event.Object.Subscription.Counters](#anytype.Event.Object.Subscription.Counters)
    - [Event.Object.Subscription.Position](#anytype.Event.Object.Subscription.Position)
    - [Event.Object.Subscription.Remove](#anytype.Event.Object.Subscription.Remove)
    - [Event.Ping](#anytype.Event.Ping)
    - [Event.Process](#anytype.Event.Process)
    - [Event.Process.Done](#anytype.Event.Process.Done)
    - [Event.Process.New](#anytype.Event.Process.New)
    - [Event.Process.Update](#anytype.Event.Process.Update)
    - [Event.Status](#anytype.Event.Status)
    - [Event.Status.Thread](#anytype.Event.Status.Thread)
    - [Event.Status.Thread.Account](#anytype.Event.Status.Thread.Account)
    - [Event.Status.Thread.Cafe](#anytype.Event.Status.Thread.Cafe)
    - [Event.Status.Thread.Cafe.PinStatus](#anytype.Event.Status.Thread.Cafe.PinStatus)
    - [Event.Status.Thread.Device](#anytype.Event.Status.Thread.Device)
    - [Event.Status.Thread.Summary](#anytype.Event.Status.Thread.Summary)
    - [Event.User](#anytype.Event.User)
    - [Event.User.Block](#anytype.Event.User.Block)
    - [Event.User.Block.Join](#anytype.Event.User.Block.Join)
    - [Event.User.Block.Left](#anytype.Event.User.Block.Left)
    - [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange)
    - [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange)
    - [Model](#anytype.Model)
    - [Model.Process](#anytype.Model.Process)
    - [Model.Process.Progress](#anytype.Model.Process.Progress)
    - [ResponseEvent](#anytype.ResponseEvent)
  
    - [Event.Status.Thread.SyncStatus](#anytype.Event.Status.Thread.SyncStatus)
    - [Model.Process.State](#anytype.Model.Process.State)
    - [Model.Process.Type](#anytype.Model.Process.Type)
  
- [pkg/lib/pb/model/protos/localstore.proto](#pkg/lib/pb/model/protos/localstore.proto)
    - [ObjectDetails](#anytype.model.ObjectDetails)
    - [ObjectInfo](#anytype.model.ObjectInfo)
    - [ObjectInfoWithLinks](#anytype.model.ObjectInfoWithLinks)
    - [ObjectInfoWithOutboundLinks](#anytype.model.ObjectInfoWithOutboundLinks)
    - [ObjectInfoWithOutboundLinksIDs](#anytype.model.ObjectInfoWithOutboundLinksIDs)
    - [ObjectLinks](#anytype.model.ObjectLinks)
    - [ObjectLinksInfo](#anytype.model.ObjectLinksInfo)
    - [ObjectStoreChecksums](#anytype.model.ObjectStoreChecksums)
  
- [pkg/lib/pb/model/protos/models.proto](#pkg/lib/pb/model/protos/models.proto)
    - [Account](#anytype.model.Account)
    - [Account.Avatar](#anytype.model.Account.Avatar)
    - [Account.Config](#anytype.model.Account.Config)
    - [Account.Status](#anytype.model.Account.Status)
    - [Block](#anytype.model.Block)
    - [Block.Content](#anytype.model.Block.Content)
    - [Block.Content.Bookmark](#anytype.model.Block.Content.Bookmark)
    - [Block.Content.Dataview](#anytype.model.Block.Content.Dataview)
    - [Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter)
    - [Block.Content.Dataview.Relation](#anytype.model.Block.Content.Dataview.Relation)
    - [Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort)
    - [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View)
    - [Block.Content.Div](#anytype.model.Block.Content.Div)
    - [Block.Content.FeaturedRelations](#anytype.model.Block.Content.FeaturedRelations)
    - [Block.Content.File](#anytype.model.Block.Content.File)
    - [Block.Content.Icon](#anytype.model.Block.Content.Icon)
    - [Block.Content.Latex](#anytype.model.Block.Content.Latex)
    - [Block.Content.Layout](#anytype.model.Block.Content.Layout)
    - [Block.Content.Link](#anytype.model.Block.Content.Link)
    - [Block.Content.Relation](#anytype.model.Block.Content.Relation)
    - [Block.Content.Smartblock](#anytype.model.Block.Content.Smartblock)
    - [Block.Content.TableOfContents](#anytype.model.Block.Content.TableOfContents)
    - [Block.Content.Text](#anytype.model.Block.Content.Text)
    - [Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark)
    - [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks)
    - [Block.Restrictions](#anytype.model.Block.Restrictions)
    - [BlockMetaOnly](#anytype.model.BlockMetaOnly)
    - [Layout](#anytype.model.Layout)
    - [LinkPreview](#anytype.model.LinkPreview)
    - [ObjectType](#anytype.model.ObjectType)
    - [Range](#anytype.model.Range)
    - [Relation](#anytype.model.Relation)
    - [Relation.Option](#anytype.model.Relation.Option)
    - [RelationOptions](#anytype.model.RelationOptions)
    - [RelationWithValue](#anytype.model.RelationWithValue)
    - [Relations](#anytype.model.Relations)
    - [Restrictions](#anytype.model.Restrictions)
    - [Restrictions.DataviewRestrictions](#anytype.model.Restrictions.DataviewRestrictions)
    - [SmartBlockSnapshotBase](#anytype.model.SmartBlockSnapshotBase)
    - [ThreadCreateQueueEntry](#anytype.model.ThreadCreateQueueEntry)
    - [ThreadDeeplinkPayload](#anytype.model.ThreadDeeplinkPayload)
  
    - [Account.StatusType](#anytype.model.Account.StatusType)
    - [Block.Align](#anytype.model.Block.Align)
    - [Block.Content.Dataview.Filter.Condition](#anytype.model.Block.Content.Dataview.Filter.Condition)
    - [Block.Content.Dataview.Filter.Operator](#anytype.model.Block.Content.Dataview.Filter.Operator)
    - [Block.Content.Dataview.Relation.DateFormat](#anytype.model.Block.Content.Dataview.Relation.DateFormat)
    - [Block.Content.Dataview.Relation.TimeFormat](#anytype.model.Block.Content.Dataview.Relation.TimeFormat)
    - [Block.Content.Dataview.Sort.Type](#anytype.model.Block.Content.Dataview.Sort.Type)
    - [Block.Content.Dataview.View.Size](#anytype.model.Block.Content.Dataview.View.Size)
    - [Block.Content.Dataview.View.Type](#anytype.model.Block.Content.Dataview.View.Type)
    - [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style)
    - [Block.Content.File.State](#anytype.model.Block.Content.File.State)
    - [Block.Content.File.Style](#anytype.model.Block.Content.File.Style)
    - [Block.Content.File.Type](#anytype.model.Block.Content.File.Type)
    - [Block.Content.Layout.Style](#anytype.model.Block.Content.Layout.Style)
    - [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style)
    - [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type)
    - [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style)
    - [Block.Position](#anytype.model.Block.Position)
    - [LinkPreview.Type](#anytype.model.LinkPreview.Type)
    - [ObjectType.Layout](#anytype.model.ObjectType.Layout)
    - [Relation.DataSource](#anytype.model.Relation.DataSource)
    - [Relation.Option.Scope](#anytype.model.Relation.Option.Scope)
    - [Relation.Scope](#anytype.model.Relation.Scope)
    - [RelationFormat](#anytype.model.RelationFormat)
    - [Restrictions.DataviewRestriction](#anytype.model.Restrictions.DataviewRestriction)
    - [Restrictions.ObjectRestriction](#anytype.model.Restrictions.ObjectRestriction)
    - [SmartBlockType](#anytype.model.SmartBlockType)
  
- [Scalar Value Types](#scalar-value-types)



<a name="pb/protos/service/service.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/service/service.proto


 

 

 


<a name="anytype.ClientCommands"></a>

### ClientCommands


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| AppVersionGet | [Rpc.App.GetVersion.Request](#anytype.Rpc.App.GetVersion.Request) | [Rpc.App.GetVersion.Response](#anytype.Rpc.App.GetVersion.Response) |  |
| AppShutdown | [Rpc.App.Shutdown.Request](#anytype.Rpc.App.Shutdown.Request) | [Rpc.App.Shutdown.Response](#anytype.Rpc.App.Shutdown.Response) |  |
| WalletCreate | [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request) | [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response) | Wallet *** |
| WalletRecover | [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request) | [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response) |  |
| WalletConvert | [Rpc.Wallet.Convert.Request](#anytype.Rpc.Wallet.Convert.Request) | [Rpc.Wallet.Convert.Response](#anytype.Rpc.Wallet.Convert.Response) |  |
| WorkspaceCreate | [Rpc.Workspace.Create.Request](#anytype.Rpc.Workspace.Create.Request) | [Rpc.Workspace.Create.Response](#anytype.Rpc.Workspace.Create.Response) | Workspace *** |
| WorkspaceSelect | [Rpc.Workspace.Select.Request](#anytype.Rpc.Workspace.Select.Request) | [Rpc.Workspace.Select.Response](#anytype.Rpc.Workspace.Select.Response) |  |
| WorkspaceGetCurrent | [Rpc.Workspace.GetCurrent.Request](#anytype.Rpc.Workspace.GetCurrent.Request) | [Rpc.Workspace.GetCurrent.Response](#anytype.Rpc.Workspace.GetCurrent.Response) |  |
| WorkspaceGetAll | [Rpc.Workspace.GetAll.Request](#anytype.Rpc.Workspace.GetAll.Request) | [Rpc.Workspace.GetAll.Response](#anytype.Rpc.Workspace.GetAll.Response) |  |
| WorkspaceSetIsHighlighted | [Rpc.Workspace.SetIsHighlighted.Request](#anytype.Rpc.Workspace.SetIsHighlighted.Request) | [Rpc.Workspace.SetIsHighlighted.Response](#anytype.Rpc.Workspace.SetIsHighlighted.Response) |  |
| WorkspaceExport | [Rpc.Workspace.Export.Request](#anytype.Rpc.Workspace.Export.Request) | [Rpc.Workspace.Export.Response](#anytype.Rpc.Workspace.Export.Response) |  |
| AccountRecover | [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request) | [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response) | Account *** |
| AccountCreate | [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request) | [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response) |  |
| AccountDelete | [Rpc.Account.Delete.Request](#anytype.Rpc.Account.Delete.Request) | [Rpc.Account.Delete.Response](#anytype.Rpc.Account.Delete.Response) |  |
| AccountSelect | [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request) | [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response) |  |
| AccountStop | [Rpc.Account.Stop.Request](#anytype.Rpc.Account.Stop.Request) | [Rpc.Account.Stop.Response](#anytype.Rpc.Account.Stop.Response) |  |
| ObjectOpen | [Rpc.Object.Open.Request](#anytype.Rpc.Object.Open.Request) | [Rpc.Object.Open.Response](#anytype.Rpc.Object.Open.Response) | Object *** |
| ObjectClose | [Rpc.Object.Close.Request](#anytype.Rpc.Object.Close.Request) | [Rpc.Object.Close.Response](#anytype.Rpc.Object.Close.Response) |  |
| ObjectShow | [Rpc.Object.Show.Request](#anytype.Rpc.Object.Show.Request) | [Rpc.Object.Show.Response](#anytype.Rpc.Object.Show.Response) |  |
| ObjectCreate | [Rpc.Object.Create.Request](#anytype.Rpc.Object.Create.Request) | [Rpc.Object.Create.Response](#anytype.Rpc.Object.Create.Response) | ObjectCreate just creates the new page, without adding the link to it from some other page |
| ObjectCreateSet | [Rpc.Object.CreateSet.Request](#anytype.Rpc.Object.CreateSet.Request) | [Rpc.Object.CreateSet.Response](#anytype.Rpc.Object.CreateSet.Response) | ObjectCreateSet just creates the new set, without adding the link to it from some other page |
| ObjectGraph | [Rpc.Object.Graph.Request](#anytype.Rpc.Object.Graph.Request) | [Rpc.Object.Graph.Response](#anytype.Rpc.Object.Graph.Response) |  |
| ObjectSearch | [Rpc.Object.Search.Request](#anytype.Rpc.Object.Search.Request) | [Rpc.Object.Search.Response](#anytype.Rpc.Object.Search.Response) |  |
| ObjectSearchSubscribe | [Rpc.Object.SearchSubscribe.Request](#anytype.Rpc.Object.SearchSubscribe.Request) | [Rpc.Object.SearchSubscribe.Response](#anytype.Rpc.Object.SearchSubscribe.Response) |  |
| ObjectSubscribeIds | [Rpc.Object.SubscribeIds.Request](#anytype.Rpc.Object.SubscribeIds.Request) | [Rpc.Object.SubscribeIds.Response](#anytype.Rpc.Object.SubscribeIds.Response) |  |
| ObjectSearchUnsubscribe | [Rpc.Object.SearchUnsubscribe.Request](#anytype.Rpc.Object.SearchUnsubscribe.Request) | [Rpc.Object.SearchUnsubscribe.Response](#anytype.Rpc.Object.SearchUnsubscribe.Response) |  |
| ObjectSetDetails | [Rpc.Object.SetDetails.Request](#anytype.Rpc.Object.SetDetails.Request) | [Rpc.Object.SetDetails.Response](#anytype.Rpc.Object.SetDetails.Response) |  |
| ObjectDuplicate | [Rpc.Object.Duplicate.Request](#anytype.Rpc.Object.Duplicate.Request) | [Rpc.Object.Duplicate.Response](#anytype.Rpc.Object.Duplicate.Response) |  |
| ObjectSetObjectType | [Rpc.Object.SetObjectType.Request](#anytype.Rpc.Object.SetObjectType.Request) | [Rpc.Object.SetObjectType.Response](#anytype.Rpc.Object.SetObjectType.Response) | ObjectSetObjectType sets an existing object type to the object so it will appear in sets and suggests relations from this type |
| ObjectSetLayout | [Rpc.Object.SetLayout.Request](#anytype.Rpc.Object.SetLayout.Request) | [Rpc.Object.SetLayout.Response](#anytype.Rpc.Object.SetLayout.Response) |  |
| ObjectSetIsFavorite | [Rpc.Object.SetIsFavorite.Request](#anytype.Rpc.Object.SetIsFavorite.Request) | [Rpc.Object.SetIsFavorite.Response](#anytype.Rpc.Object.SetIsFavorite.Response) |  |
| ObjectSetIsArchived | [Rpc.Object.SetIsArchived.Request](#anytype.Rpc.Object.SetIsArchived.Request) | [Rpc.Object.SetIsArchived.Response](#anytype.Rpc.Object.SetIsArchived.Response) |  |
| ObjectListDelete | [Rpc.Object.ListDelete.Request](#anytype.Rpc.Object.ListDelete.Request) | [Rpc.Object.ListDelete.Response](#anytype.Rpc.Object.ListDelete.Response) |  |
| ObjectListSetIsArchived | [Rpc.Object.ListSetIsArchived.Request](#anytype.Rpc.Object.ListSetIsArchived.Request) | [Rpc.Object.ListSetIsArchived.Response](#anytype.Rpc.Object.ListSetIsArchived.Response) |  |
| ObjectListSetIsFavorite | [Rpc.Object.ListSetIsFavorite.Request](#anytype.Rpc.Object.ListSetIsFavorite.Request) | [Rpc.Object.ListSetIsFavorite.Response](#anytype.Rpc.Object.ListSetIsFavorite.Response) |  |
| ObjectApplyTemplate | [Rpc.Object.ApplyTemplate.Request](#anytype.Rpc.Object.ApplyTemplate.Request) | [Rpc.Object.ApplyTemplate.Response](#anytype.Rpc.Object.ApplyTemplate.Response) |  |
| ObjectToSet | [Rpc.Object.ToSet.Request](#anytype.Rpc.Object.ToSet.Request) | [Rpc.Object.ToSet.Response](#anytype.Rpc.Object.ToSet.Response) | ObjectToSet creates new set from given object and removes object |
| ObjectAddWithObjectId | [Rpc.Object.AddWithObjectId.Request](#anytype.Rpc.Object.AddWithObjectId.Request) | [Rpc.Object.AddWithObjectId.Response](#anytype.Rpc.Object.AddWithObjectId.Response) |  |
| ObjectShareByLink | [Rpc.Object.ShareByLink.Request](#anytype.Rpc.Object.ShareByLink.Request) | [Rpc.Object.ShareByLink.Response](#anytype.Rpc.Object.ShareByLink.Response) |  |
| ObjectOpenBreadcrumbs | [Rpc.Object.OpenBreadcrumbs.Request](#anytype.Rpc.Object.OpenBreadcrumbs.Request) | [Rpc.Object.OpenBreadcrumbs.Response](#anytype.Rpc.Object.OpenBreadcrumbs.Response) |  |
| ObjectSetBreadcrumbs | [Rpc.Object.SetBreadcrumbs.Request](#anytype.Rpc.Object.SetBreadcrumbs.Request) | [Rpc.Object.SetBreadcrumbs.Response](#anytype.Rpc.Object.SetBreadcrumbs.Response) |  |
| ObjectUndo | [Rpc.Object.Undo.Request](#anytype.Rpc.Object.Undo.Request) | [Rpc.Object.Undo.Response](#anytype.Rpc.Object.Undo.Response) |  |
| ObjectRedo | [Rpc.Object.Redo.Request](#anytype.Rpc.Object.Redo.Request) | [Rpc.Object.Redo.Response](#anytype.Rpc.Object.Redo.Response) |  |
| ObjectImportMarkdown | [Rpc.Object.ImportMarkdown.Request](#anytype.Rpc.Object.ImportMarkdown.Request) | [Rpc.Object.ImportMarkdown.Response](#anytype.Rpc.Object.ImportMarkdown.Response) |  |
| ObjectExport | [Rpc.Object.Export.Request](#anytype.Rpc.Object.Export.Request) | [Rpc.Object.Export.Response](#anytype.Rpc.Object.Export.Response) |  |
| ObjectRelationAdd | [Rpc.ObjectRelation.Add.Request](#anytype.Rpc.ObjectRelation.Add.Request) | [Rpc.ObjectRelation.Add.Response](#anytype.Rpc.ObjectRelation.Add.Response) | Object Relations *** |
| ObjectRelationUpdate | [Rpc.ObjectRelation.Update.Request](#anytype.Rpc.ObjectRelation.Update.Request) | [Rpc.ObjectRelation.Update.Response](#anytype.Rpc.ObjectRelation.Update.Response) |  |
| ObjectRelationDelete | [Rpc.ObjectRelation.Delete.Request](#anytype.Rpc.ObjectRelation.Delete.Request) | [Rpc.ObjectRelation.Delete.Response](#anytype.Rpc.ObjectRelation.Delete.Response) |  |
| ObjectRelationAddFeatured | [Rpc.ObjectRelation.AddFeatured.Request](#anytype.Rpc.ObjectRelation.AddFeatured.Request) | [Rpc.ObjectRelation.AddFeatured.Response](#anytype.Rpc.ObjectRelation.AddFeatured.Response) |  |
| ObjectRelationRemoveFeatured | [Rpc.ObjectRelation.RemoveFeatured.Request](#anytype.Rpc.ObjectRelation.RemoveFeatured.Request) | [Rpc.ObjectRelation.RemoveFeatured.Response](#anytype.Rpc.ObjectRelation.RemoveFeatured.Response) |  |
| ObjectRelationListAvailable | [Rpc.ObjectRelation.ListAvailable.Request](#anytype.Rpc.ObjectRelation.ListAvailable.Request) | [Rpc.ObjectRelation.ListAvailable.Response](#anytype.Rpc.ObjectRelation.ListAvailable.Response) |  |
| ObjectRelationOptionAdd | [Rpc.ObjectRelationOption.Add.Request](#anytype.Rpc.ObjectRelationOption.Add.Request) | [Rpc.ObjectRelationOption.Add.Response](#anytype.Rpc.ObjectRelationOption.Add.Response) |  |
| ObjectRelationOptionUpdate | [Rpc.ObjectRelationOption.Update.Request](#anytype.Rpc.ObjectRelationOption.Update.Request) | [Rpc.ObjectRelationOption.Update.Response](#anytype.Rpc.ObjectRelationOption.Update.Response) |  |
| ObjectRelationOptionDelete | [Rpc.ObjectRelationOption.Delete.Request](#anytype.Rpc.ObjectRelationOption.Delete.Request) | [Rpc.ObjectRelationOption.Delete.Response](#anytype.Rpc.ObjectRelationOption.Delete.Response) |  |
| ObjectTypeCreate | [Rpc.ObjectType.Create.Request](#anytype.Rpc.ObjectType.Create.Request) | [Rpc.ObjectType.Create.Response](#anytype.Rpc.ObjectType.Create.Response) | ObjectType commands *** |
| ObjectTypeList | [Rpc.ObjectType.List.Request](#anytype.Rpc.ObjectType.List.Request) | [Rpc.ObjectType.List.Response](#anytype.Rpc.ObjectType.List.Response) | ObjectTypeList lists all object types both bundled and created by user |
| ObjectTypeRelationList | [Rpc.ObjectType.Relation.List.Request](#anytype.Rpc.ObjectType.Relation.List.Request) | [Rpc.ObjectType.Relation.List.Response](#anytype.Rpc.ObjectType.Relation.List.Response) |  |
| ObjectTypeRelationAdd | [Rpc.ObjectType.Relation.Add.Request](#anytype.Rpc.ObjectType.Relation.Add.Request) | [Rpc.ObjectType.Relation.Add.Response](#anytype.Rpc.ObjectType.Relation.Add.Response) |  |
| ObjectTypeRelationUpdate | [Rpc.ObjectType.Relation.Update.Request](#anytype.Rpc.ObjectType.Relation.Update.Request) | [Rpc.ObjectType.Relation.Update.Response](#anytype.Rpc.ObjectType.Relation.Update.Response) |  |
| ObjectTypeRelationRemove | [Rpc.ObjectType.Relation.Remove.Request](#anytype.Rpc.ObjectType.Relation.Remove.Request) | [Rpc.ObjectType.Relation.Remove.Response](#anytype.Rpc.ObjectType.Relation.Remove.Response) |  |
| HistoryShowVersion | [Rpc.History.ShowVersion.Request](#anytype.Rpc.History.ShowVersion.Request) | [Rpc.History.ShowVersion.Response](#anytype.Rpc.History.ShowVersion.Response) |  |
| HistoryGetVersions | [Rpc.History.GetVersions.Request](#anytype.Rpc.History.GetVersions.Request) | [Rpc.History.GetVersions.Response](#anytype.Rpc.History.GetVersions.Response) |  |
| HistorySetVersion | [Rpc.History.SetVersion.Request](#anytype.Rpc.History.SetVersion.Request) | [Rpc.History.SetVersion.Response](#anytype.Rpc.History.SetVersion.Response) |  |
| FileOffload | [Rpc.File.Offload.Request](#anytype.Rpc.File.Offload.Request) | [Rpc.File.Offload.Response](#anytype.Rpc.File.Offload.Response) | Files *** |
| FileListOffload | [Rpc.File.ListOffload.Request](#anytype.Rpc.File.ListOffload.Request) | [Rpc.File.ListOffload.Response](#anytype.Rpc.File.ListOffload.Response) |  |
| FileUpload | [Rpc.File.Upload.Request](#anytype.Rpc.File.Upload.Request) | [Rpc.File.Upload.Response](#anytype.Rpc.File.Upload.Response) |  |
| FileDownload | [Rpc.File.Download.Request](#anytype.Rpc.File.Download.Request) | [Rpc.File.Download.Response](#anytype.Rpc.File.Download.Response) |  |
| FileDrop | [Rpc.File.Drop.Request](#anytype.Rpc.File.Drop.Request) | [Rpc.File.Drop.Response](#anytype.Rpc.File.Drop.Response) |  |
| NavigationListObjects | [Rpc.Navigation.ListObjects.Request](#anytype.Rpc.Navigation.ListObjects.Request) | [Rpc.Navigation.ListObjects.Response](#anytype.Rpc.Navigation.ListObjects.Response) |  |
| NavigationGetObjectInfoWithLinks | [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request) | [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response) |  |
| TemplateCreateFromObject | [Rpc.Template.CreateFromObject.Request](#anytype.Rpc.Template.CreateFromObject.Request) | [Rpc.Template.CreateFromObject.Response](#anytype.Rpc.Template.CreateFromObject.Response) |  |
| TemplateCreateFromObjectType | [Rpc.Template.CreateFromObjectType.Request](#anytype.Rpc.Template.CreateFromObjectType.Request) | [Rpc.Template.CreateFromObjectType.Response](#anytype.Rpc.Template.CreateFromObjectType.Response) |  |
| TemplateClone | [Rpc.Template.Clone.Request](#anytype.Rpc.Template.Clone.Request) | [Rpc.Template.Clone.Response](#anytype.Rpc.Template.Clone.Response) |  |
| TemplateExportAll | [Rpc.Template.ExportAll.Request](#anytype.Rpc.Template.ExportAll.Request) | [Rpc.Template.ExportAll.Response](#anytype.Rpc.Template.ExportAll.Response) |  |
| LinkPreview | [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request) | [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response) |  |
| UnsplashSearch | [Rpc.Unsplash.Search.Request](#anytype.Rpc.Unsplash.Search.Request) | [Rpc.Unsplash.Search.Response](#anytype.Rpc.Unsplash.Search.Response) |  |
| UnsplashDownload | [Rpc.Unsplash.Download.Request](#anytype.Rpc.Unsplash.Download.Request) | [Rpc.Unsplash.Download.Response](#anytype.Rpc.Unsplash.Download.Response) | UnsplashDownload downloads picture from unsplash by ID, put it to the IPFS and returns the hash. The artist info is available in the object details |
| BlockUpload | [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request) | [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response) | General Block commands *** |
| BlockReplace | [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request) | [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response) |  |
| BlockCreate | [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request) | [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response) |  |
| BlockUnlink | [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request) | [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response) |  |
| BlockSplit | [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request) | [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response) |  |
| BlockMerge | [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request) | [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response) |  |
| BlockCopy | [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request) | [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response) |  |
| BlockPaste | [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request) | [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response) |  |
| BlockCut | [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request) | [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response) |  |
| BlockSetFields | [Rpc.Block.SetFields.Request](#anytype.Rpc.Block.SetFields.Request) | [Rpc.Block.SetFields.Response](#anytype.Rpc.Block.SetFields.Response) |  |
| BlockSetRestrictions | [Rpc.Block.SetRestrictions.Request](#anytype.Rpc.Block.SetRestrictions.Request) | [Rpc.Block.SetRestrictions.Response](#anytype.Rpc.Block.SetRestrictions.Response) |  |
| BlockExport | [Rpc.Block.Export.Request](#anytype.Rpc.Block.Export.Request) | [Rpc.Block.Export.Response](#anytype.Rpc.Block.Export.Response) |  |
| BlockListMoveToExistingObject | [Rpc.Block.ListMoveToExistingObject.Request](#anytype.Rpc.Block.ListMoveToExistingObject.Request) | [Rpc.Block.ListMoveToExistingObject.Response](#anytype.Rpc.Block.ListMoveToExistingObject.Response) |  |
| BlockListMoveToNewObject | [Rpc.Block.ListMoveToNewObject.Request](#anytype.Rpc.Block.ListMoveToNewObject.Request) | [Rpc.Block.ListMoveToNewObject.Response](#anytype.Rpc.Block.ListMoveToNewObject.Response) |  |
| BlockListConvertToObjects | [Rpc.Block.ListConvertToObjects.Request](#anytype.Rpc.Block.ListConvertToObjects.Request) | [Rpc.Block.ListConvertToObjects.Response](#anytype.Rpc.Block.ListConvertToObjects.Response) |  |
| BlockListSetFields | [Rpc.Block.ListSetFields.Request](#anytype.Rpc.Block.ListSetFields.Request) | [Rpc.Block.ListSetFields.Response](#anytype.Rpc.Block.ListSetFields.Response) |  |
| BlockListDuplicate | [Rpc.Block.ListDuplicate.Request](#anytype.Rpc.Block.ListDuplicate.Request) | [Rpc.Block.ListDuplicate.Response](#anytype.Rpc.Block.ListDuplicate.Response) |  |
| BlockListSetBackgroundColor | [Rpc.Block.ListSetBackgroundColor.Request](#anytype.Rpc.Block.ListSetBackgroundColor.Request) | [Rpc.Block.ListSetBackgroundColor.Response](#anytype.Rpc.Block.ListSetBackgroundColor.Response) |  |
| BlockListSetAlign | [Rpc.Block.ListSetAlign.Request](#anytype.Rpc.Block.ListSetAlign.Request) | [Rpc.Block.ListSetAlign.Response](#anytype.Rpc.Block.ListSetAlign.Response) |  |
| BlockListTurnInto | [Rpc.Block.ListTurnInto.Request](#anytype.Rpc.Block.ListTurnInto.Request) | [Rpc.Block.ListTurnInto.Response](#anytype.Rpc.Block.ListTurnInto.Response) |  |
| BlockTextSetText | [Rpc.BlockText.SetText.Request](#anytype.Rpc.BlockText.SetText.Request) | [Rpc.BlockText.SetText.Response](#anytype.Rpc.BlockText.SetText.Response) | Text Block commands *** |
| BlockTextSetColor | [Rpc.BlockText.SetColor.Request](#anytype.Rpc.BlockText.SetColor.Request) | [Rpc.BlockText.SetColor.Response](#anytype.Rpc.BlockText.SetColor.Response) |  |
| BlockTextSetStyle | [Rpc.BlockText.SetStyle.Request](#anytype.Rpc.BlockText.SetStyle.Request) | [Rpc.BlockText.SetStyle.Response](#anytype.Rpc.BlockText.SetStyle.Response) |  |
| BlockTextSetChecked | [Rpc.BlockText.SetChecked.Request](#anytype.Rpc.BlockText.SetChecked.Request) | [Rpc.BlockText.SetChecked.Response](#anytype.Rpc.BlockText.SetChecked.Response) |  |
| BlockTextSetIcon | [Rpc.BlockText.SetIcon.Request](#anytype.Rpc.BlockText.SetIcon.Request) | [Rpc.BlockText.SetIcon.Response](#anytype.Rpc.BlockText.SetIcon.Response) |  |
| BlockTextListSetColor | [Rpc.BlockText.ListSetColor.Request](#anytype.Rpc.BlockText.ListSetColor.Request) | [Rpc.BlockText.ListSetColor.Response](#anytype.Rpc.BlockText.ListSetColor.Response) |  |
| BlockTextListSetMark | [Rpc.BlockText.ListSetMark.Request](#anytype.Rpc.BlockText.ListSetMark.Request) | [Rpc.BlockText.ListSetMark.Response](#anytype.Rpc.BlockText.ListSetMark.Response) |  |
| BlockTextListSetStyle | [Rpc.BlockText.ListSetStyle.Request](#anytype.Rpc.BlockText.ListSetStyle.Request) | [Rpc.BlockText.ListSetStyle.Response](#anytype.Rpc.BlockText.ListSetStyle.Response) |  |
| BlockFileSetName | [Rpc.BlockFile.SetName.Request](#anytype.Rpc.BlockFile.SetName.Request) | [Rpc.BlockFile.SetName.Response](#anytype.Rpc.BlockFile.SetName.Response) | File block commands *** |
| BlockImageSetName | [Rpc.BlockImage.SetName.Request](#anytype.Rpc.BlockImage.SetName.Request) | [Rpc.BlockImage.SetName.Response](#anytype.Rpc.BlockImage.SetName.Response) |  |
| BlockImageSetWidth | [Rpc.BlockImage.SetWidth.Request](#anytype.Rpc.BlockImage.SetWidth.Request) | [Rpc.BlockImage.SetWidth.Response](#anytype.Rpc.BlockImage.SetWidth.Response) |  |
| BlockVideoSetName | [Rpc.BlockVideo.SetName.Request](#anytype.Rpc.BlockVideo.SetName.Request) | [Rpc.BlockVideo.SetName.Response](#anytype.Rpc.BlockVideo.SetName.Response) |  |
| BlockVideoSetWidth | [Rpc.BlockVideo.SetWidth.Request](#anytype.Rpc.BlockVideo.SetWidth.Request) | [Rpc.BlockVideo.SetWidth.Response](#anytype.Rpc.BlockVideo.SetWidth.Response) |  |
| BlockFileCreateAndUpload | [Rpc.BlockFile.CreateAndUpload.Request](#anytype.Rpc.BlockFile.CreateAndUpload.Request) | [Rpc.BlockFile.CreateAndUpload.Response](#anytype.Rpc.BlockFile.CreateAndUpload.Response) |  |
| BlockFileListSetStyle | [Rpc.BlockFile.ListSetStyle.Request](#anytype.Rpc.BlockFile.ListSetStyle.Request) | [Rpc.BlockFile.ListSetStyle.Response](#anytype.Rpc.BlockFile.ListSetStyle.Response) |  |
| BlockDataviewViewCreate | [Rpc.BlockDataview.View.Create.Request](#anytype.Rpc.BlockDataview.View.Create.Request) | [Rpc.BlockDataview.View.Create.Response](#anytype.Rpc.BlockDataview.View.Create.Response) | Dataview block commands *** |
| BlockDataviewViewDelete | [Rpc.BlockDataview.View.Delete.Request](#anytype.Rpc.BlockDataview.View.Delete.Request) | [Rpc.BlockDataview.View.Delete.Response](#anytype.Rpc.BlockDataview.View.Delete.Response) |  |
| BlockDataviewViewUpdate | [Rpc.BlockDataview.View.Update.Request](#anytype.Rpc.BlockDataview.View.Update.Request) | [Rpc.BlockDataview.View.Update.Response](#anytype.Rpc.BlockDataview.View.Update.Response) |  |
| BlockDataviewViewSetActive | [Rpc.BlockDataview.View.SetActive.Request](#anytype.Rpc.BlockDataview.View.SetActive.Request) | [Rpc.BlockDataview.View.SetActive.Response](#anytype.Rpc.BlockDataview.View.SetActive.Response) |  |
| BlockDataviewViewSetPosition | [Rpc.BlockDataview.View.SetPosition.Request](#anytype.Rpc.BlockDataview.View.SetPosition.Request) | [Rpc.BlockDataview.View.SetPosition.Response](#anytype.Rpc.BlockDataview.View.SetPosition.Response) |  |
| BlockDataviewSetSource | [Rpc.BlockDataview.SetSource.Request](#anytype.Rpc.BlockDataview.SetSource.Request) | [Rpc.BlockDataview.SetSource.Response](#anytype.Rpc.BlockDataview.SetSource.Response) |  |
| BlockDataviewAddRelation | [Rpc.BlockDataview.Relation.Add.Request](#anytype.Rpc.BlockDataview.Relation.Add.Request) | [Rpc.BlockDataview.Relation.Add.Response](#anytype.Rpc.BlockDataview.Relation.Add.Response) |  |
| BlockDataviewUpdateRelation | [Rpc.BlockDataview.Relation.Update.Request](#anytype.Rpc.BlockDataview.Relation.Update.Request) | [Rpc.BlockDataview.Relation.Update.Response](#anytype.Rpc.BlockDataview.Relation.Update.Response) |  |
| BlockDataviewDeleteRelation | [Rpc.BlockDataview.Relation.Delete.Request](#anytype.Rpc.BlockDataview.Relation.Delete.Request) | [Rpc.BlockDataview.Relation.Delete.Response](#anytype.Rpc.BlockDataview.Relation.Delete.Response) |  |
| BlockDataviewListAvailableRelation | [Rpc.BlockDataview.Relation.ListAvailable.Request](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Request) | [Rpc.BlockDataview.Relation.ListAvailable.Response](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Response) |  |
| BlockDataviewRecordCreate | [Rpc.BlockDataviewRecord.Create.Request](#anytype.Rpc.BlockDataviewRecord.Create.Request) | [Rpc.BlockDataviewRecord.Create.Response](#anytype.Rpc.BlockDataviewRecord.Create.Response) |  |
| BlockDataviewRecordUpdate | [Rpc.BlockDataviewRecord.Update.Request](#anytype.Rpc.BlockDataviewRecord.Update.Request) | [Rpc.BlockDataviewRecord.Update.Response](#anytype.Rpc.BlockDataviewRecord.Update.Response) |  |
| BlockDataviewRecordDelete | [Rpc.BlockDataviewRecord.Delete.Request](#anytype.Rpc.BlockDataviewRecord.Delete.Request) | [Rpc.BlockDataviewRecord.Delete.Response](#anytype.Rpc.BlockDataviewRecord.Delete.Response) |  |
| BlockDataviewRecordRelationOptionAdd | [Rpc.BlockDataviewRecord.AddRelationOption.Request](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Request) | [Rpc.BlockDataviewRecord.AddRelationOption.Response](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response) |  |
| BlockDataviewRecordRelationOptionUpdate | [Rpc.BlockDataviewRecord.UpdateRelationOption.Request](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Request) | [Rpc.BlockDataviewRecord.UpdateRelationOption.Response](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response) |  |
| BlockDataviewRecordRelationOptionDelete | [Rpc.BlockDataviewRecord.DeleteRelationOption.Request](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Request) | [Rpc.BlockDataviewRecord.DeleteRelationOption.Response](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response) |  |
| BlockLinkCreateLinkToNewObject | [Rpc.BlockLink.CreateLinkToNewObject.Request](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Request) | [Rpc.BlockLink.CreateLinkToNewObject.Response](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Response) | Other specific block commands *** |
| BlockBookmarkFetch | [Rpc.BlockBookmark.Fetch.Request](#anytype.Rpc.BlockBookmark.Fetch.Request) | [Rpc.BlockBookmark.Fetch.Response](#anytype.Rpc.BlockBookmark.Fetch.Response) |  |
| BlockBookmarkCreateAndFetch | [Rpc.BlockBookmark.CreateAndFetch.Request](#anytype.Rpc.BlockBookmark.CreateAndFetch.Request) | [Rpc.BlockBookmark.CreateAndFetch.Response](#anytype.Rpc.BlockBookmark.CreateAndFetch.Response) |  |
| BlockRelationSetKey | [Rpc.BlockRelation.SetKey.Request](#anytype.Rpc.BlockRelation.SetKey.Request) | [Rpc.BlockRelation.SetKey.Response](#anytype.Rpc.BlockRelation.SetKey.Response) |  |
| BlockRelationAdd | [Rpc.BlockRelation.Add.Request](#anytype.Rpc.BlockRelation.Add.Request) | [Rpc.BlockRelation.Add.Response](#anytype.Rpc.BlockRelation.Add.Response) |  |
| BlockDivListSetStyle | [Rpc.BlockDiv.ListSetStyle.Request](#anytype.Rpc.BlockDiv.ListSetStyle.Request) | [Rpc.BlockDiv.ListSetStyle.Response](#anytype.Rpc.BlockDiv.ListSetStyle.Response) |  |
| BlockLatexSetText | [Rpc.BlockLatex.SetText.Request](#anytype.Rpc.BlockLatex.SetText.Request) | [Rpc.BlockLatex.SetText.Response](#anytype.Rpc.BlockLatex.SetText.Response) |  |
| ProcessCancel | [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request) | [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request) | [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response) |  |
| DebugSync | [Rpc.Debug.Sync.Request](#anytype.Rpc.Debug.Sync.Request) | [Rpc.Debug.Sync.Response](#anytype.Rpc.Debug.Sync.Response) |  |
| DebugThread | [Rpc.Debug.Thread.Request](#anytype.Rpc.Debug.Thread.Request) | [Rpc.Debug.Thread.Response](#anytype.Rpc.Debug.Thread.Response) |  |
| DebugTree | [Rpc.Debug.Tree.Request](#anytype.Rpc.Debug.Tree.Request) | [Rpc.Debug.Tree.Response](#anytype.Rpc.Debug.Tree.Response) |  |
| DebugExportLocalstore | [Rpc.Debug.ExportLocalstore.Request](#anytype.Rpc.Debug.ExportLocalstore.Request) | [Rpc.Debug.ExportLocalstore.Response](#anytype.Rpc.Debug.ExportLocalstore.Response) |  |
| DebugPing | [Rpc.Debug.Ping.Request](#anytype.Rpc.Debug.Ping.Request) | [Rpc.Debug.Ping.Response](#anytype.Rpc.Debug.Ping.Response) |  |
| MetricsSetParameters | [Rpc.Metrics.SetParameters.Request](#anytype.Rpc.Metrics.SetParameters.Request) | [Rpc.Metrics.SetParameters.Response](#anytype.Rpc.Metrics.SetParameters.Response) |  |
| ListenEvents | [Empty](#anytype.Empty) | [Event](#anytype.Event) stream | used only for lib-server via grpc |

 



<a name="pb/protos/changes.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/changes.proto



<a name="anytype.Change"></a>

### Change
the element of change tree used to store and internal apply smartBlock history


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| previous_ids | [string](#string) | repeated | ids of previous changes |
| last_snapshot_id | [string](#string) |  | id of the last snapshot |
| previous_meta_ids | [string](#string) | repeated | ids of the last changes with details/relations content |
| content | [Change.Content](#anytype.Change.Content) | repeated | set of actions to apply |
| snapshot | [Change.Snapshot](#anytype.Change.Snapshot) |  | snapshot - when not null, the Content will be ignored |
| fileKeys | [Change.FileKeys](#anytype.Change.FileKeys) | repeated | file keys related to changes content |
| timestamp | [int64](#int64) |  | creation timestamp |






<a name="anytype.Change.BlockCreate"></a>

### Change.BlockCreate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| blocks | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Change.BlockDuplicate"></a>

### Change.BlockDuplicate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype.Change.BlockMove"></a>

### Change.BlockMove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| ids | [string](#string) | repeated |  |






<a name="anytype.Change.BlockRemove"></a>

### Change.BlockRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |






<a name="anytype.Change.BlockUpdate"></a>

### Change.BlockUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| events | [Event.Message](#anytype.Event.Message) | repeated |  |






<a name="anytype.Change.Content"></a>

### Change.Content



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockCreate | [Change.BlockCreate](#anytype.Change.BlockCreate) |  |  |
| blockUpdate | [Change.BlockUpdate](#anytype.Change.BlockUpdate) |  |  |
| blockRemove | [Change.BlockRemove](#anytype.Change.BlockRemove) |  |  |
| blockMove | [Change.BlockMove](#anytype.Change.BlockMove) |  |  |
| blockDuplicate | [Change.BlockDuplicate](#anytype.Change.BlockDuplicate) |  |  |
| detailsSet | [Change.DetailsSet](#anytype.Change.DetailsSet) |  |  |
| detailsUnset | [Change.DetailsUnset](#anytype.Change.DetailsUnset) |  |  |
| relationAdd | [Change.RelationAdd](#anytype.Change.RelationAdd) |  |  |
| relationRemove | [Change.RelationRemove](#anytype.Change.RelationRemove) |  |  |
| relationUpdate | [Change.RelationUpdate](#anytype.Change.RelationUpdate) |  |  |
| objectTypeAdd | [Change.ObjectTypeAdd](#anytype.Change.ObjectTypeAdd) |  |  |
| objectTypeRemove | [Change.ObjectTypeRemove](#anytype.Change.ObjectTypeRemove) |  |  |
| storeKeySet | [Change.StoreKeySet](#anytype.Change.StoreKeySet) |  |  |
| storeKeyUnset | [Change.StoreKeyUnset](#anytype.Change.StoreKeyUnset) |  |  |






<a name="anytype.Change.DetailsSet"></a>

### Change.DetailsSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  |  |






<a name="anytype.Change.DetailsUnset"></a>

### Change.DetailsUnset



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |






<a name="anytype.Change.FileKeys"></a>

### Change.FileKeys



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| keys | [Change.FileKeys.KeysEntry](#anytype.Change.FileKeys.KeysEntry) | repeated |  |






<a name="anytype.Change.FileKeys.KeysEntry"></a>

### Change.FileKeys.KeysEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="anytype.Change.ObjectTypeAdd"></a>

### Change.ObjectTypeAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype.Change.ObjectTypeRemove"></a>

### Change.ObjectTypeRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype.Change.RelationAdd"></a>

### Change.RelationAdd



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Change.RelationRemove"></a>

### Change.RelationRemove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |






<a name="anytype.Change.RelationUpdate"></a>

### Change.RelationUpdate



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| format | [model.RelationFormat](#anytype.model.RelationFormat) |  |  |
| name | [string](#string) |  |  |
| defaultValue | [google.protobuf.Value](#google.protobuf.Value) |  |  |
| objectTypes | [Change.RelationUpdate.ObjectTypes](#anytype.Change.RelationUpdate.ObjectTypes) |  |  |
| multi | [bool](#bool) |  |  |
| selectDict | [Change.RelationUpdate.Dict](#anytype.Change.RelationUpdate.Dict) |  |  |






<a name="anytype.Change.RelationUpdate.Dict"></a>

### Change.RelationUpdate.Dict



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dict | [model.Relation.Option](#anytype.model.Relation.Option) | repeated |  |






<a name="anytype.Change.RelationUpdate.ObjectTypes"></a>

### Change.RelationUpdate.ObjectTypes



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypes | [string](#string) | repeated |  |






<a name="anytype.Change.Snapshot"></a>

### Change.Snapshot



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| logHeads | [Change.Snapshot.LogHeadsEntry](#anytype.Change.Snapshot.LogHeadsEntry) | repeated | logId -&gt; lastChangeId |
| data | [model.SmartBlockSnapshotBase](#anytype.model.SmartBlockSnapshotBase) |  | snapshot data |
| fileKeys | [Change.FileKeys](#anytype.Change.FileKeys) | repeated | all file keys related to doc |






<a name="anytype.Change.Snapshot.LogHeadsEntry"></a>

### Change.Snapshot.LogHeadsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="anytype.Change.StoreKeySet"></a>

### Change.StoreKeySet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  |  |






<a name="anytype.Change.StoreKeyUnset"></a>

### Change.StoreKeyUnset



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |





 

 

 

 



<a name="pb/protos/commands.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/commands.proto



<a name="anytype.Empty"></a>

### Empty







<a name="anytype.Rpc"></a>

### Rpc
Rpc is a namespace, that agregates all of the service commands between client and middleware.
Structure: Topic &gt; Subtopic &gt; Subsub... &gt; Action &gt; (Request, Response).
Request – message from a client.
Response – message from a middleware.






<a name="anytype.Rpc.Account"></a>

### Rpc.Account







<a name="anytype.Rpc.Account.Config"></a>

### Rpc.Account.Config



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enableDataview | [bool](#bool) |  |  |
| enableDebug | [bool](#bool) |  |  |
| enableReleaseChannelSwitch | [bool](#bool) |  |  |
| enableSpaces | [bool](#bool) |  |  |
| extra | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.Account.Create"></a>

### Rpc.Account.Create







<a name="anytype.Rpc.Account.Create.Request"></a>

### Rpc.Account.Create.Request
Front end to middleware request-to-create-an account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Account name |
| avatarLocalPath | [string](#string) |  | Path to an image, that will be used as an avatar of this account |
| alphaInviteCode | [string](#string) |  |  |






<a name="anytype.Rpc.Account.Create.Response"></a>

### Rpc.Account.Create.Response
Middleware-to-front-end response for an account creation request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error) |  | Error while trying to create an account |
| account | [model.Account](#anytype.model.Account) |  | A newly created account; In case of a failure, i.e. error is non-NULL, the account model should contain empty/default-value fields |
| config | [Rpc.Account.Config](#anytype.Rpc.Account.Config) |  | deprecated, use account |
| info | [Rpc.Account.Info](#anytype.Rpc.Account.Info) |  |  |






<a name="anytype.Rpc.Account.Create.Response.Error"></a>

### Rpc.Account.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Create.Response.Error.Code](#anytype.Rpc.Account.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Account.Delete"></a>

### Rpc.Account.Delete







<a name="anytype.Rpc.Account.Delete.Request"></a>

### Rpc.Account.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| revert | [bool](#bool) |  |  |






<a name="anytype.Rpc.Account.Delete.Response"></a>

### Rpc.Account.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Delete.Response.Error](#anytype.Rpc.Account.Delete.Response.Error) |  | Error while trying to recover an account |
| status | [model.Account.Status](#anytype.model.Account.Status) |  |  |






<a name="anytype.Rpc.Account.Delete.Response.Error"></a>

### Rpc.Account.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Delete.Response.Error.Code](#anytype.Rpc.Account.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Account.GetConfig"></a>

### Rpc.Account.GetConfig







<a name="anytype.Rpc.Account.GetConfig.Get"></a>

### Rpc.Account.GetConfig.Get







<a name="anytype.Rpc.Account.GetConfig.Get.Request"></a>

### Rpc.Account.GetConfig.Get.Request







<a name="anytype.Rpc.Account.Info"></a>

### Rpc.Account.Info



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| homeBlockId | [string](#string) |  | home dashboard block id |
| archiveBlockId | [string](#string) |  | archive block id |
| profileBlockId | [string](#string) |  | profile block id |
| marketplaceTypeId | [string](#string) |  | marketplace type id |
| marketplaceRelationId | [string](#string) |  | marketplace relation id |
| marketplaceTemplateId | [string](#string) |  | marketplace template id |
| deviceId | [string](#string) |  |  |
| gatewayUrl | [string](#string) |  | gateway url for fetching static files |






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
| account | [model.Account](#anytype.model.Account) |  | Selected account |
| config | [Rpc.Account.Config](#anytype.Rpc.Account.Config) |  | deprecated, use account |
| info | [Rpc.Account.Info](#anytype.Rpc.Account.Info) |  |  |






<a name="anytype.Rpc.Account.Select.Response.Error"></a>

### Rpc.Account.Select.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Select.Response.Error.Code](#anytype.Rpc.Account.Select.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Account.Stop"></a>

### Rpc.Account.Stop







<a name="anytype.Rpc.Account.Stop.Request"></a>

### Rpc.Account.Stop.Request
Front end to middleware request to stop currently running account node and optionally remove the locally stored data


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| removeData | [bool](#bool) |  |  |






<a name="anytype.Rpc.Account.Stop.Response"></a>

### Rpc.Account.Stop.Response
Middleware-to-front-end response for an account stop request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Stop.Response.Error](#anytype.Rpc.Account.Stop.Response.Error) |  | Error while trying to launch/select an account |






<a name="anytype.Rpc.Account.Stop.Response.Error"></a>

### Rpc.Account.Stop.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Stop.Response.Error.Code](#anytype.Rpc.Account.Stop.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.App"></a>

### Rpc.App







<a name="anytype.Rpc.App.GetVersion"></a>

### Rpc.App.GetVersion







<a name="anytype.Rpc.App.GetVersion.Request"></a>

### Rpc.App.GetVersion.Request







<a name="anytype.Rpc.App.GetVersion.Response"></a>

### Rpc.App.GetVersion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.App.GetVersion.Response.Error](#anytype.Rpc.App.GetVersion.Response.Error) |  |  |
| version | [string](#string) |  |  |
| details | [string](#string) |  | build date, branch and commit |






<a name="anytype.Rpc.App.GetVersion.Response.Error"></a>

### Rpc.App.GetVersion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.App.GetVersion.Response.Error.Code](#anytype.Rpc.App.GetVersion.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.App.Shutdown"></a>

### Rpc.App.Shutdown







<a name="anytype.Rpc.App.Shutdown.Request"></a>

### Rpc.App.Shutdown.Request







<a name="anytype.Rpc.App.Shutdown.Response"></a>

### Rpc.App.Shutdown.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.App.Shutdown.Response.Error](#anytype.Rpc.App.Shutdown.Response.Error) |  |  |






<a name="anytype.Rpc.App.Shutdown.Response.Error"></a>

### Rpc.App.Shutdown.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.App.Shutdown.Response.Error.Code](#anytype.Rpc.App.Shutdown.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block"></a>

### Rpc.Block
Block commands






<a name="anytype.Rpc.Block.Copy"></a>

### Rpc.Block.Copy







<a name="anytype.Rpc.Block.Copy.Request"></a>

### Rpc.Block.Copy.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype.model.Block) | repeated |  |
| selectedTextRange | [model.Range](#anytype.model.Range) |  |  |






<a name="anytype.Rpc.Block.Copy.Response"></a>

### Rpc.Block.Copy.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Rpc.Block.Copy.Response.Error"></a>

### Rpc.Block.Copy.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Create"></a>

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






<a name="anytype.Rpc.Block.Create.Request"></a>

### Rpc.Block.Create.Request
common simple block command


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| targetId | [string](#string) |  | id of the closest block |
| block | [model.Block](#anytype.model.Block) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.Block.Create.Response"></a>

### Rpc.Block.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Create.Response.Error"></a>

### Rpc.Block.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Cut"></a>

### Rpc.Block.Cut







<a name="anytype.Rpc.Block.Cut.Request"></a>

### Rpc.Block.Cut.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype.model.Block) | repeated |  |
| selectedTextRange | [model.Range](#anytype.model.Range) |  |  |






<a name="anytype.Rpc.Block.Cut.Response"></a>

### Rpc.Block.Cut.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype.model.Block) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Cut.Response.Error"></a>

### Rpc.Block.Cut.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Download"></a>

### Rpc.Block.Download







<a name="anytype.Rpc.Block.Download.Request"></a>

### Rpc.Block.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Download.Response"></a>

### Rpc.Block.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Download.Response.Error](#anytype.Rpc.Block.Download.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Download.Response.Error"></a>

### Rpc.Block.Download.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Export"></a>

### Rpc.Block.Export







<a name="anytype.Rpc.Block.Export.Request"></a>

### Rpc.Block.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Rpc.Block.Export.Response"></a>

### Rpc.Block.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Export.Response.Error](#anytype.Rpc.Block.Export.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Export.Response.Error"></a>

### Rpc.Block.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Export.Response.Error.Code](#anytype.Rpc.Block.Export.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListConvertToObjects"></a>

### Rpc.Block.ListConvertToObjects







<a name="anytype.Rpc.Block.ListConvertToObjects.Request"></a>

### Rpc.Block.ListConvertToObjects.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| objectType | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListConvertToObjects.Response"></a>

### Rpc.Block.ListConvertToObjects.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListConvertToObjects.Response.Error](#anytype.Rpc.Block.ListConvertToObjects.Response.Error) |  |  |
| linkIds | [string](#string) | repeated |  |






<a name="anytype.Rpc.Block.ListConvertToObjects.Response.Error"></a>

### Rpc.Block.ListConvertToObjects.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListConvertToObjects.Response.Error.Code](#anytype.Rpc.Block.ListConvertToObjects.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListDuplicate"></a>

### Rpc.Block.ListDuplicate
Makes blocks copy by given ids and paste it to shown place






<a name="anytype.Rpc.Block.ListDuplicate.Request"></a>

### Rpc.Block.ListDuplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| targetId | [string](#string) |  | id of the closest block |
| blockIds | [string](#string) | repeated | id of block for duplicate |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.Block.ListDuplicate.Response"></a>

### Rpc.Block.ListDuplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListDuplicate.Response.Error](#anytype.Rpc.Block.ListDuplicate.Response.Error) |  |  |
| blockIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListDuplicate.Response.Error"></a>

### Rpc.Block.ListDuplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListDuplicate.Response.Error.Code](#anytype.Rpc.Block.ListDuplicate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListMoveToExistingObject"></a>

### Rpc.Block.ListMoveToExistingObject







<a name="anytype.Rpc.Block.ListMoveToExistingObject.Request"></a>

### Rpc.Block.ListMoveToExistingObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| targetContextId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype.model.Block.Position) |  | position relatively to the dropTargetId simple block |






<a name="anytype.Rpc.Block.ListMoveToExistingObject.Response"></a>

### Rpc.Block.ListMoveToExistingObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListMoveToExistingObject.Response.Error](#anytype.Rpc.Block.ListMoveToExistingObject.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListMoveToExistingObject.Response.Error"></a>

### Rpc.Block.ListMoveToExistingObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListMoveToExistingObject.Response.Error.Code](#anytype.Rpc.Block.ListMoveToExistingObject.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListMoveToNewObject"></a>

### Rpc.Block.ListMoveToNewObject







<a name="anytype.Rpc.Block.ListMoveToNewObject.Request"></a>

### Rpc.Block.ListMoveToNewObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | new object details |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype.model.Block.Position) |  | position relatively to the dropTargetId simple block |






<a name="anytype.Rpc.Block.ListMoveToNewObject.Response"></a>

### Rpc.Block.ListMoveToNewObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListMoveToNewObject.Response.Error](#anytype.Rpc.Block.ListMoveToNewObject.Response.Error) |  |  |
| linkId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListMoveToNewObject.Response.Error"></a>

### Rpc.Block.ListMoveToNewObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListMoveToNewObject.Response.Error.Code](#anytype.Rpc.Block.ListMoveToNewObject.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListSetAlign"></a>

### Rpc.Block.ListSetAlign







<a name="anytype.Rpc.Block.ListSetAlign.Request"></a>

### Rpc.Block.ListSetAlign.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated | when empty - align will be applied as layoutAlign |
| align | [model.Block.Align](#anytype.model.Block.Align) |  |  |






<a name="anytype.Rpc.Block.ListSetAlign.Response"></a>

### Rpc.Block.ListSetAlign.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetAlign.Response.Error](#anytype.Rpc.Block.ListSetAlign.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListSetAlign.Response.Error"></a>

### Rpc.Block.ListSetAlign.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetAlign.Response.Error.Code](#anytype.Rpc.Block.ListSetAlign.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListSetBackgroundColor"></a>

### Rpc.Block.ListSetBackgroundColor







<a name="anytype.Rpc.Block.ListSetBackgroundColor.Request"></a>

### Rpc.Block.ListSetBackgroundColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListSetBackgroundColor.Response"></a>

### Rpc.Block.ListSetBackgroundColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetBackgroundColor.Response.Error](#anytype.Rpc.Block.ListSetBackgroundColor.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListSetBackgroundColor.Response.Error"></a>

### Rpc.Block.ListSetBackgroundColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetBackgroundColor.Response.Error.Code](#anytype.Rpc.Block.ListSetBackgroundColor.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListSetFields"></a>

### Rpc.Block.ListSetFields







<a name="anytype.Rpc.Block.ListSetFields.Request"></a>

### Rpc.Block.ListSetFields.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockFields | [Rpc.Block.ListSetFields.Request.BlockField](#anytype.Rpc.Block.ListSetFields.Request.BlockField) | repeated |  |






<a name="anytype.Rpc.Block.ListSetFields.Request.BlockField"></a>

### Rpc.Block.ListSetFields.Request.BlockField



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.Block.ListSetFields.Response"></a>

### Rpc.Block.ListSetFields.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListSetFields.Response.Error](#anytype.Rpc.Block.ListSetFields.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListSetFields.Response.Error"></a>

### Rpc.Block.ListSetFields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListSetFields.Response.Error.Code](#anytype.Rpc.Block.ListSetFields.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListTurnInto"></a>

### Rpc.Block.ListTurnInto







<a name="anytype.Rpc.Block.ListTurnInto.Request"></a>

### Rpc.Block.ListTurnInto.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Rpc.Block.ListTurnInto.Response"></a>

### Rpc.Block.ListTurnInto.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ListTurnInto.Response.Error](#anytype.Rpc.Block.ListTurnInto.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ListTurnInto.Response.Error"></a>

### Rpc.Block.ListTurnInto.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ListTurnInto.Response.Error.Code](#anytype.Rpc.Block.ListTurnInto.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ListUpdate"></a>

### Rpc.Block.ListUpdate







<a name="anytype.Rpc.Block.ListUpdate.Request"></a>

### Rpc.Block.ListUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| text | [Rpc.Block.ListUpdate.Request.Text](#anytype.Rpc.Block.ListUpdate.Request.Text) |  |  |
| backgroundColor | [string](#string) |  |  |
| align | [model.Block.Align](#anytype.model.Block.Align) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| divStyle | [model.Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |
| fileStyle | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Rpc.Block.ListUpdate.Request.Text"></a>

### Rpc.Block.ListUpdate.Request.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |
| color | [string](#string) |  |  |
| mark | [model.Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark) |  |  |






<a name="anytype.Rpc.Block.Merge"></a>

### Rpc.Block.Merge







<a name="anytype.Rpc.Block.Merge.Request"></a>

### Rpc.Block.Merge.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| firstBlockId | [string](#string) |  |  |
| secondBlockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Merge.Response"></a>

### Rpc.Block.Merge.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Merge.Response.Error"></a>

### Rpc.Block.Merge.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Paste"></a>

### Rpc.Block.Paste







<a name="anytype.Rpc.Block.Paste.Request"></a>

### Rpc.Block.Paste.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| focusedBlockId | [string](#string) |  |  |
| selectedTextRange | [model.Range](#anytype.model.Range) |  |  |
| selectedBlockIds | [string](#string) | repeated |  |
| isPartOfBlock | [bool](#bool) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype.model.Block) | repeated |  |
| fileSlot | [Rpc.Block.Paste.Request.File](#anytype.Rpc.Block.Paste.Request.File) | repeated |  |






<a name="anytype.Rpc.Block.Paste.Request.File"></a>

### Rpc.Block.Paste.Request.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| data | [bytes](#bytes) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Paste.Response"></a>

### Rpc.Block.Paste.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error) |  |  |
| blockIds | [string](#string) | repeated |  |
| caretPosition | [int32](#int32) |  |  |
| isSameBlockCaret | [bool](#bool) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Paste.Response.Error"></a>

### Rpc.Block.Paste.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Replace"></a>

### Rpc.Block.Replace







<a name="anytype.Rpc.Block.Replace.Request"></a>

### Rpc.Block.Replace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| block | [model.Block](#anytype.model.Block) |  |  |






<a name="anytype.Rpc.Block.Replace.Response"></a>

### Rpc.Block.Replace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Replace.Response.Error](#anytype.Rpc.Block.Replace.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Replace.Response.Error"></a>

### Rpc.Block.Replace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.SetFields"></a>

### Rpc.Block.SetFields







<a name="anytype.Rpc.Block.SetFields.Request"></a>

### Rpc.Block.SetFields.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.Block.SetFields.Response"></a>

### Rpc.Block.SetFields.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.SetFields.Response.Error](#anytype.Rpc.Block.SetFields.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.SetFields.Response.Error"></a>

### Rpc.Block.SetFields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.SetFields.Response.Error.Code](#anytype.Rpc.Block.SetFields.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.SetRestrictions"></a>

### Rpc.Block.SetRestrictions







<a name="anytype.Rpc.Block.SetRestrictions.Request"></a>

### Rpc.Block.SetRestrictions.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| restrictions | [model.Block.Restrictions](#anytype.model.Block.Restrictions) |  |  |






<a name="anytype.Rpc.Block.SetRestrictions.Response"></a>

### Rpc.Block.SetRestrictions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.SetRestrictions.Response.Error](#anytype.Rpc.Block.SetRestrictions.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.SetRestrictions.Response.Error"></a>

### Rpc.Block.SetRestrictions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.SetRestrictions.Response.Error.Code](#anytype.Rpc.Block.SetRestrictions.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Split"></a>

### Rpc.Block.Split







<a name="anytype.Rpc.Block.Split.Request"></a>

### Rpc.Block.Split.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype.model.Range) |  |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |
| mode | [Rpc.Block.Split.Request.Mode](#anytype.Rpc.Block.Split.Request.Mode) |  |  |






<a name="anytype.Rpc.Block.Split.Response"></a>

### Rpc.Block.Split.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Split.Response.Error"></a>

### Rpc.Block.Split.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Unlink"></a>

### Rpc.Block.Unlink
Remove blocks from the childrenIds of its parents






<a name="anytype.Rpc.Block.Unlink.Request"></a>

### Rpc.Block.Unlink.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| blockIds | [string](#string) | repeated | targets to remove |






<a name="anytype.Rpc.Block.Unlink.Response"></a>

### Rpc.Block.Unlink.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Unlink.Response.Error"></a>

### Rpc.Block.Unlink.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Upload"></a>

### Rpc.Block.Upload







<a name="anytype.Rpc.Block.Upload.Request"></a>

### Rpc.Block.Upload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| filePath | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Upload.Response"></a>

### Rpc.Block.Upload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Upload.Response.Error](#anytype.Rpc.Block.Upload.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Upload.Response.Error"></a>

### Rpc.Block.Upload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockBookmark"></a>

### Rpc.BlockBookmark







<a name="anytype.Rpc.BlockBookmark.CreateAndFetch"></a>

### Rpc.BlockBookmark.CreateAndFetch







<a name="anytype.Rpc.BlockBookmark.CreateAndFetch.Request"></a>

### Rpc.BlockBookmark.CreateAndFetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.BlockBookmark.CreateAndFetch.Response"></a>

### Rpc.BlockBookmark.CreateAndFetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockBookmark.CreateAndFetch.Response.Error](#anytype.Rpc.BlockBookmark.CreateAndFetch.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockBookmark.CreateAndFetch.Response.Error"></a>

### Rpc.BlockBookmark.CreateAndFetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockBookmark.Fetch"></a>

### Rpc.BlockBookmark.Fetch







<a name="anytype.Rpc.BlockBookmark.Fetch.Request"></a>

### Rpc.BlockBookmark.Fetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.BlockBookmark.Fetch.Response"></a>

### Rpc.BlockBookmark.Fetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockBookmark.Fetch.Response.Error](#anytype.Rpc.BlockBookmark.Fetch.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockBookmark.Fetch.Response.Error"></a>

### Rpc.BlockBookmark.Fetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockBookmark.Fetch.Response.Error.Code](#anytype.Rpc.BlockBookmark.Fetch.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview"></a>

### Rpc.BlockDataview







<a name="anytype.Rpc.BlockDataview.Relation"></a>

### Rpc.BlockDataview.Relation







<a name="anytype.Rpc.BlockDataview.Relation.Add"></a>

### Rpc.BlockDataview.Relation.Add







<a name="anytype.Rpc.BlockDataview.Relation.Add.Request"></a>

### Rpc.BlockDataview.Relation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Add.Response"></a>

### Rpc.BlockDataview.Relation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.Add.Response.Error](#anytype.Rpc.BlockDataview.Relation.Add.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| relationKey | [string](#string) |  | deprecated |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Add.Response.Error"></a>

### Rpc.BlockDataview.Relation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.Add.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Delete"></a>

### Rpc.BlockDataview.Relation.Delete







<a name="anytype.Rpc.BlockDataview.Relation.Delete.Request"></a>

### Rpc.BlockDataview.Relation.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Delete.Response"></a>

### Rpc.BlockDataview.Relation.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.Delete.Response.Error](#anytype.Rpc.BlockDataview.Relation.Delete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Delete.Response.Error"></a>

### Rpc.BlockDataview.Relation.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.Delete.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.ListAvailable"></a>

### Rpc.BlockDataview.Relation.ListAvailable







<a name="anytype.Rpc.BlockDataview.Relation.ListAvailable.Request"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.ListAvailable.Response"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.ListAvailable.Response.Error](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Response.Error) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.BlockDataview.Relation.ListAvailable.Response.Error"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Update"></a>

### Rpc.BlockDataview.Relation.Update







<a name="anytype.Rpc.BlockDataview.Relation.Update.Request"></a>

### Rpc.BlockDataview.Relation.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | key of relation to update |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Update.Response"></a>

### Rpc.BlockDataview.Relation.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.Relation.Update.Response.Error](#anytype.Rpc.BlockDataview.Relation.Update.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.Relation.Update.Response.Error"></a>

### Rpc.BlockDataview.Relation.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.Relation.Update.Response.Error.Code](#anytype.Rpc.BlockDataview.Relation.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.SetSource"></a>

### Rpc.BlockDataview.SetSource







<a name="anytype.Rpc.BlockDataview.SetSource.Request"></a>

### Rpc.BlockDataview.SetSource.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype.Rpc.BlockDataview.SetSource.Response"></a>

### Rpc.BlockDataview.SetSource.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.SetSource.Response.Error](#anytype.Rpc.BlockDataview.SetSource.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.SetSource.Response.Error"></a>

### Rpc.BlockDataview.SetSource.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.SetSource.Response.Error.Code](#anytype.Rpc.BlockDataview.SetSource.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.View"></a>

### Rpc.BlockDataview.View







<a name="anytype.Rpc.BlockDataview.View.Create"></a>

### Rpc.BlockDataview.View.Create







<a name="anytype.Rpc.BlockDataview.View.Create.Request"></a>

### Rpc.BlockDataview.View.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to insert the new block |
| view | [model.Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Create.Response"></a>

### Rpc.BlockDataview.View.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.Create.Response.Error](#anytype.Rpc.BlockDataview.View.Create.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| viewId | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Create.Response.Error"></a>

### Rpc.BlockDataview.View.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.Create.Response.Error.Code](#anytype.Rpc.BlockDataview.View.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Delete"></a>

### Rpc.BlockDataview.View.Delete







<a name="anytype.Rpc.BlockDataview.View.Delete.Request"></a>

### Rpc.BlockDataview.View.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| blockId | [string](#string) |  | id of the dataview |
| viewId | [string](#string) |  | id of the view to remove |






<a name="anytype.Rpc.BlockDataview.View.Delete.Response"></a>

### Rpc.BlockDataview.View.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.Delete.Response.Error](#anytype.Rpc.BlockDataview.View.Delete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Delete.Response.Error"></a>

### Rpc.BlockDataview.View.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.Delete.Response.Error.Code](#anytype.Rpc.BlockDataview.View.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.View.SetActive"></a>

### Rpc.BlockDataview.View.SetActive
set the current active view (persisted only within a session)






<a name="anytype.Rpc.BlockDataview.View.SetActive.Request"></a>

### Rpc.BlockDataview.View.SetActive.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block |
| viewId | [string](#string) |  | id of active view |
| offset | [uint32](#uint32) |  |  |
| limit | [uint32](#uint32) |  |  |






<a name="anytype.Rpc.BlockDataview.View.SetActive.Response"></a>

### Rpc.BlockDataview.View.SetActive.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.SetActive.Response.Error](#anytype.Rpc.BlockDataview.View.SetActive.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.View.SetActive.Response.Error"></a>

### Rpc.BlockDataview.View.SetActive.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.SetActive.Response.Error.Code](#anytype.Rpc.BlockDataview.View.SetActive.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.View.SetPosition"></a>

### Rpc.BlockDataview.View.SetPosition







<a name="anytype.Rpc.BlockDataview.View.SetPosition.Request"></a>

### Rpc.BlockDataview.View.SetPosition.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| blockId | [string](#string) |  | id of the dataview |
| viewId | [string](#string) |  | id of the view to remove |
| position | [uint32](#uint32) |  | index of view position (0 - means first) |






<a name="anytype.Rpc.BlockDataview.View.SetPosition.Response"></a>

### Rpc.BlockDataview.View.SetPosition.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.SetPosition.Response.Error](#anytype.Rpc.BlockDataview.View.SetPosition.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.View.SetPosition.Response.Error"></a>

### Rpc.BlockDataview.View.SetPosition.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.SetPosition.Response.Error.Code](#anytype.Rpc.BlockDataview.View.SetPosition.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Update"></a>

### Rpc.BlockDataview.View.Update







<a name="anytype.Rpc.BlockDataview.View.Update.Request"></a>

### Rpc.BlockDataview.View.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| view | [model.Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Update.Response"></a>

### Rpc.BlockDataview.View.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataview.View.Update.Response.Error](#anytype.Rpc.BlockDataview.View.Update.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataview.View.Update.Response.Error"></a>

### Rpc.BlockDataview.View.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataview.View.Update.Response.Error.Code](#anytype.Rpc.BlockDataview.View.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord"></a>

### Rpc.BlockDataviewRecord







<a name="anytype.Rpc.BlockDataviewRecord.AddRelationOption"></a>

### Rpc.BlockDataviewRecord.AddRelationOption
RecordRelationOptionAdd may return existing option in case object specified with recordId already have the option with the same name or ID






<a name="anytype.Rpc.BlockDataviewRecord.AddRelationOption.Request"></a>

### Rpc.BlockDataviewRecord.AddRelationOption.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |
| recordId | [string](#string) |  | id of record which is used to add an option |






<a name="anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response"></a>

### Rpc.BlockDataviewRecord.AddRelationOption.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataviewRecord.AddRelationOption.Response.Error](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response.Error"></a>

### Rpc.BlockDataviewRecord.AddRelationOption.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataviewRecord.AddRelationOption.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Create"></a>

### Rpc.BlockDataviewRecord.Create







<a name="anytype.Rpc.BlockDataviewRecord.Create.Request"></a>

### Rpc.BlockDataviewRecord.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| record | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| templateId | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Create.Response"></a>

### Rpc.BlockDataviewRecord.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataviewRecord.Create.Response.Error](#anytype.Rpc.BlockDataviewRecord.Create.Response.Error) |  |  |
| record | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Create.Response.Error"></a>

### Rpc.BlockDataviewRecord.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataviewRecord.Create.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Delete"></a>

### Rpc.BlockDataviewRecord.Delete







<a name="anytype.Rpc.BlockDataviewRecord.Delete.Request"></a>

### Rpc.BlockDataviewRecord.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| recordId | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Delete.Response"></a>

### Rpc.BlockDataviewRecord.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataviewRecord.Delete.Response.Error](#anytype.Rpc.BlockDataviewRecord.Delete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Delete.Response.Error"></a>

### Rpc.BlockDataviewRecord.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataviewRecord.Delete.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.DeleteRelationOption"></a>

### Rpc.BlockDataviewRecord.DeleteRelationOption







<a name="anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Request"></a>

### Rpc.BlockDataviewRecord.DeleteRelationOption.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | relation key to add the option |
| optionId | [string](#string) |  | id of select options to remove |
| recordId | [string](#string) |  | id of record which is used to delete an option |






<a name="anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response"></a>

### Rpc.BlockDataviewRecord.DeleteRelationOption.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error"></a>

### Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Update"></a>

### Rpc.BlockDataviewRecord.Update







<a name="anytype.Rpc.BlockDataviewRecord.Update.Request"></a>

### Rpc.BlockDataviewRecord.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| recordId | [string](#string) |  |  |
| record | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Update.Response"></a>

### Rpc.BlockDataviewRecord.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataviewRecord.Update.Response.Error](#anytype.Rpc.BlockDataviewRecord.Update.Response.Error) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.Update.Response.Error"></a>

### Rpc.BlockDataviewRecord.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataviewRecord.Update.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.UpdateRelationOption"></a>

### Rpc.BlockDataviewRecord.UpdateRelationOption







<a name="anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Request"></a>

### Rpc.BlockDataviewRecord.UpdateRelationOption.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |
| recordId | [string](#string) |  | id of record which is used to update an option |






<a name="anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response"></a>

### Rpc.BlockDataviewRecord.UpdateRelationOption.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error"></a>

### Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error.Code](#anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockDiv"></a>

### Rpc.BlockDiv







<a name="anytype.Rpc.BlockDiv.ListSetStyle"></a>

### Rpc.BlockDiv.ListSetStyle







<a name="anytype.Rpc.BlockDiv.ListSetStyle.Request"></a>

### Rpc.BlockDiv.ListSetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |






<a name="anytype.Rpc.BlockDiv.ListSetStyle.Response"></a>

### Rpc.BlockDiv.ListSetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockDiv.ListSetStyle.Response.Error](#anytype.Rpc.BlockDiv.ListSetStyle.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockDiv.ListSetStyle.Response.Error"></a>

### Rpc.BlockDiv.ListSetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockDiv.ListSetStyle.Response.Error.Code](#anytype.Rpc.BlockDiv.ListSetStyle.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockFile"></a>

### Rpc.BlockFile







<a name="anytype.Rpc.BlockFile.CreateAndUpload"></a>

### Rpc.BlockFile.CreateAndUpload







<a name="anytype.Rpc.BlockFile.CreateAndUpload.Request"></a>

### Rpc.BlockFile.CreateAndUpload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| fileType | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |






<a name="anytype.Rpc.BlockFile.CreateAndUpload.Response"></a>

### Rpc.BlockFile.CreateAndUpload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.CreateAndUpload.Response.Error](#anytype.Rpc.BlockFile.CreateAndUpload.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockFile.CreateAndUpload.Response.Error"></a>

### Rpc.BlockFile.CreateAndUpload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.CreateAndUpload.Response.Error.Code](#anytype.Rpc.BlockFile.CreateAndUpload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockFile.ListSetStyle"></a>

### Rpc.BlockFile.ListSetStyle







<a name="anytype.Rpc.BlockFile.ListSetStyle.Request"></a>

### Rpc.BlockFile.ListSetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Rpc.BlockFile.ListSetStyle.Response"></a>

### Rpc.BlockFile.ListSetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.ListSetStyle.Response.Error](#anytype.Rpc.BlockFile.ListSetStyle.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockFile.ListSetStyle.Response.Error"></a>

### Rpc.BlockFile.ListSetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.ListSetStyle.Response.Error.Code](#anytype.Rpc.BlockFile.ListSetStyle.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockFile.SetName"></a>

### Rpc.BlockFile.SetName







<a name="anytype.Rpc.BlockFile.SetName.Request"></a>

### Rpc.BlockFile.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.BlockFile.SetName.Response"></a>

### Rpc.BlockFile.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockFile.SetName.Response.Error](#anytype.Rpc.BlockFile.SetName.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockFile.SetName.Response.Error"></a>

### Rpc.BlockFile.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockFile.SetName.Response.Error.Code](#anytype.Rpc.BlockFile.SetName.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockImage"></a>

### Rpc.BlockImage







<a name="anytype.Rpc.BlockImage.SetName"></a>

### Rpc.BlockImage.SetName







<a name="anytype.Rpc.BlockImage.SetName.Request"></a>

### Rpc.BlockImage.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.BlockImage.SetName.Response"></a>

### Rpc.BlockImage.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockImage.SetName.Response.Error](#anytype.Rpc.BlockImage.SetName.Response.Error) |  |  |






<a name="anytype.Rpc.BlockImage.SetName.Response.Error"></a>

### Rpc.BlockImage.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockImage.SetName.Response.Error.Code](#anytype.Rpc.BlockImage.SetName.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockImage.SetWidth"></a>

### Rpc.BlockImage.SetWidth







<a name="anytype.Rpc.BlockImage.SetWidth.Request"></a>

### Rpc.BlockImage.SetWidth.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype.Rpc.BlockImage.SetWidth.Response"></a>

### Rpc.BlockImage.SetWidth.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockImage.SetWidth.Response.Error](#anytype.Rpc.BlockImage.SetWidth.Response.Error) |  |  |






<a name="anytype.Rpc.BlockImage.SetWidth.Response.Error"></a>

### Rpc.BlockImage.SetWidth.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockImage.SetWidth.Response.Error.Code](#anytype.Rpc.BlockImage.SetWidth.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockLatex"></a>

### Rpc.BlockLatex







<a name="anytype.Rpc.BlockLatex.SetText"></a>

### Rpc.BlockLatex.SetText







<a name="anytype.Rpc.BlockLatex.SetText.Request"></a>

### Rpc.BlockLatex.SetText.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |






<a name="anytype.Rpc.BlockLatex.SetText.Response"></a>

### Rpc.BlockLatex.SetText.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLatex.SetText.Response.Error](#anytype.Rpc.BlockLatex.SetText.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockLatex.SetText.Response.Error"></a>

### Rpc.BlockLatex.SetText.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLatex.SetText.Response.Error.Code](#anytype.Rpc.BlockLatex.SetText.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockLink"></a>

### Rpc.BlockLink







<a name="anytype.Rpc.BlockLink.CreateLinkToNewObject"></a>

### Rpc.BlockLink.CreateLinkToNewObject







<a name="anytype.Rpc.BlockLink.CreateLinkToNewObject.Request"></a>

### Rpc.BlockLink.CreateLinkToNewObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | new object details |
| templateId | [string](#string) |  | optional template id for creating from template |
| targetId | [string](#string) |  | link block params

id of the closest simple block |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  | link block fields |






<a name="anytype.Rpc.BlockLink.CreateLinkToNewObject.Response"></a>

### Rpc.BlockLink.CreateLinkToNewObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLink.CreateLinkToNewObject.Response.Error](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockLink.CreateLinkToNewObject.Response.Error"></a>

### Rpc.BlockLink.CreateLinkToNewObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLink.CreateLinkToNewObject.Response.Error.Code](#anytype.Rpc.BlockLink.CreateLinkToNewObject.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockLink.CreateLinkToNewSet"></a>

### Rpc.BlockLink.CreateLinkToNewSet







<a name="anytype.Rpc.BlockLink.CreateLinkToNewSet.Request"></a>

### Rpc.BlockLink.CreateLinkToNewSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| targetId | [string](#string) |  | id of the closest block |
| source | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | details |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.BlockLink.CreateLinkToNewSet.Response"></a>

### Rpc.BlockLink.CreateLinkToNewSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLink.CreateLinkToNewSet.Response.Error](#anytype.Rpc.BlockLink.CreateLinkToNewSet.Response.Error) |  |  |
| blockId | [string](#string) |  | (optional) id of the link block pointing to this set |
| targetId | [string](#string) |  | id of the new set |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockLink.CreateLinkToNewSet.Response.Error"></a>

### Rpc.BlockLink.CreateLinkToNewSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLink.CreateLinkToNewSet.Response.Error.Code](#anytype.Rpc.BlockLink.CreateLinkToNewSet.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockLink.SetTargetBlockId"></a>

### Rpc.BlockLink.SetTargetBlockId







<a name="anytype.Rpc.BlockLink.SetTargetBlockId.Request"></a>

### Rpc.BlockLink.SetTargetBlockId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| targetBlockId | [string](#string) |  |  |






<a name="anytype.Rpc.BlockLink.SetTargetBlockId.Response"></a>

### Rpc.BlockLink.SetTargetBlockId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockLink.SetTargetBlockId.Response.Error](#anytype.Rpc.BlockLink.SetTargetBlockId.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockLink.SetTargetBlockId.Response.Error"></a>

### Rpc.BlockLink.SetTargetBlockId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockLink.SetTargetBlockId.Response.Error.Code](#anytype.Rpc.BlockLink.SetTargetBlockId.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockRelation"></a>

### Rpc.BlockRelation







<a name="anytype.Rpc.BlockRelation.Add"></a>

### Rpc.BlockRelation.Add







<a name="anytype.Rpc.BlockRelation.Add.Request"></a>

### Rpc.BlockRelation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.BlockRelation.Add.Response"></a>

### Rpc.BlockRelation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockRelation.Add.Response.Error](#anytype.Rpc.BlockRelation.Add.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockRelation.Add.Response.Error"></a>

### Rpc.BlockRelation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockRelation.Add.Response.Error.Code](#anytype.Rpc.BlockRelation.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockRelation.SetKey"></a>

### Rpc.BlockRelation.SetKey







<a name="anytype.Rpc.BlockRelation.SetKey.Request"></a>

### Rpc.BlockRelation.SetKey.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| key | [string](#string) |  |  |






<a name="anytype.Rpc.BlockRelation.SetKey.Response"></a>

### Rpc.BlockRelation.SetKey.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockRelation.SetKey.Response.Error](#anytype.Rpc.BlockRelation.SetKey.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockRelation.SetKey.Response.Error"></a>

### Rpc.BlockRelation.SetKey.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockRelation.SetKey.Response.Error.Code](#anytype.Rpc.BlockRelation.SetKey.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText"></a>

### Rpc.BlockText







<a name="anytype.Rpc.BlockText.ListSetColor"></a>

### Rpc.BlockText.ListSetColor







<a name="anytype.Rpc.BlockText.ListSetColor.Request"></a>

### Rpc.BlockText.ListSetColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.ListSetColor.Response"></a>

### Rpc.BlockText.ListSetColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListSetColor.Response.Error](#anytype.Rpc.BlockText.ListSetColor.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.ListSetColor.Response.Error"></a>

### Rpc.BlockText.ListSetColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListSetColor.Response.Error.Code](#anytype.Rpc.BlockText.ListSetColor.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.ListSetMark"></a>

### Rpc.BlockText.ListSetMark







<a name="anytype.Rpc.BlockText.ListSetMark.Request"></a>

### Rpc.BlockText.ListSetMark.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| mark | [model.Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark) |  |  |






<a name="anytype.Rpc.BlockText.ListSetMark.Response"></a>

### Rpc.BlockText.ListSetMark.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListSetMark.Response.Error](#anytype.Rpc.BlockText.ListSetMark.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.ListSetMark.Response.Error"></a>

### Rpc.BlockText.ListSetMark.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListSetMark.Response.Error.Code](#anytype.Rpc.BlockText.ListSetMark.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.ListSetStyle"></a>

### Rpc.BlockText.ListSetStyle







<a name="anytype.Rpc.BlockText.ListSetStyle.Request"></a>

### Rpc.BlockText.ListSetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Rpc.BlockText.ListSetStyle.Response"></a>

### Rpc.BlockText.ListSetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.ListSetStyle.Response.Error](#anytype.Rpc.BlockText.ListSetStyle.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.ListSetStyle.Response.Error"></a>

### Rpc.BlockText.ListSetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.ListSetStyle.Response.Error.Code](#anytype.Rpc.BlockText.ListSetStyle.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetChecked"></a>

### Rpc.BlockText.SetChecked







<a name="anytype.Rpc.BlockText.SetChecked.Request"></a>

### Rpc.BlockText.SetChecked.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype.Rpc.BlockText.SetChecked.Response"></a>

### Rpc.BlockText.SetChecked.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetChecked.Response.Error](#anytype.Rpc.BlockText.SetChecked.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.SetChecked.Response.Error"></a>

### Rpc.BlockText.SetChecked.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetChecked.Response.Error.Code](#anytype.Rpc.BlockText.SetChecked.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetColor"></a>

### Rpc.BlockText.SetColor







<a name="anytype.Rpc.BlockText.SetColor.Request"></a>

### Rpc.BlockText.SetColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetColor.Response"></a>

### Rpc.BlockText.SetColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetColor.Response.Error](#anytype.Rpc.BlockText.SetColor.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.SetColor.Response.Error"></a>

### Rpc.BlockText.SetColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetColor.Response.Error.Code](#anytype.Rpc.BlockText.SetColor.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetIcon"></a>

### Rpc.BlockText.SetIcon







<a name="anytype.Rpc.BlockText.SetIcon.Request"></a>

### Rpc.BlockText.SetIcon.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| iconImage | [string](#string) |  | in case both image and emoji are set, image has a priority to show |
| iconEmoji | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetIcon.Response"></a>

### Rpc.BlockText.SetIcon.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetIcon.Response.Error](#anytype.Rpc.BlockText.SetIcon.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.SetIcon.Response.Error"></a>

### Rpc.BlockText.SetIcon.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetIcon.Response.Error.Code](#anytype.Rpc.BlockText.SetIcon.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetMarks"></a>

### Rpc.BlockText.SetMarks







<a name="anytype.Rpc.BlockText.SetMarks.Get"></a>

### Rpc.BlockText.SetMarks.Get
Get marks list in the selected range in text block.






<a name="anytype.Rpc.BlockText.SetMarks.Get.Request"></a>

### Rpc.BlockText.SetMarks.Get.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype.model.Range) |  |  |






<a name="anytype.Rpc.BlockText.SetMarks.Get.Response"></a>

### Rpc.BlockText.SetMarks.Get.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetMarks.Get.Response.Error](#anytype.Rpc.BlockText.SetMarks.Get.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.SetMarks.Get.Response.Error"></a>

### Rpc.BlockText.SetMarks.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetMarks.Get.Response.Error.Code](#anytype.Rpc.BlockText.SetMarks.Get.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetStyle"></a>

### Rpc.BlockText.SetStyle







<a name="anytype.Rpc.BlockText.SetStyle.Request"></a>

### Rpc.BlockText.SetStyle.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Rpc.BlockText.SetStyle.Response"></a>

### Rpc.BlockText.SetStyle.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetStyle.Response.Error](#anytype.Rpc.BlockText.SetStyle.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.SetStyle.Response.Error"></a>

### Rpc.BlockText.SetStyle.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetStyle.Response.Error.Code](#anytype.Rpc.BlockText.SetStyle.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockText.SetText"></a>

### Rpc.BlockText.SetText







<a name="anytype.Rpc.BlockText.SetText.Request"></a>

### Rpc.BlockText.SetText.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |
| marks | [model.Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) |  |  |






<a name="anytype.Rpc.BlockText.SetText.Response"></a>

### Rpc.BlockText.SetText.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockText.SetText.Response.Error](#anytype.Rpc.BlockText.SetText.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockText.SetText.Response.Error"></a>

### Rpc.BlockText.SetText.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockText.SetText.Response.Error.Code](#anytype.Rpc.BlockText.SetText.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockVideo"></a>

### Rpc.BlockVideo







<a name="anytype.Rpc.BlockVideo.SetName"></a>

### Rpc.BlockVideo.SetName







<a name="anytype.Rpc.BlockVideo.SetName.Request"></a>

### Rpc.BlockVideo.SetName.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.BlockVideo.SetName.Response"></a>

### Rpc.BlockVideo.SetName.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockVideo.SetName.Response.Error](#anytype.Rpc.BlockVideo.SetName.Response.Error) |  |  |






<a name="anytype.Rpc.BlockVideo.SetName.Response.Error"></a>

### Rpc.BlockVideo.SetName.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockVideo.SetName.Response.Error.Code](#anytype.Rpc.BlockVideo.SetName.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockVideo.SetWidth"></a>

### Rpc.BlockVideo.SetWidth







<a name="anytype.Rpc.BlockVideo.SetWidth.Request"></a>

### Rpc.BlockVideo.SetWidth.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype.Rpc.BlockVideo.SetWidth.Response"></a>

### Rpc.BlockVideo.SetWidth.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockVideo.SetWidth.Response.Error](#anytype.Rpc.BlockVideo.SetWidth.Response.Error) |  |  |






<a name="anytype.Rpc.BlockVideo.SetWidth.Response.Error"></a>

### Rpc.BlockVideo.SetWidth.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockVideo.SetWidth.Response.Error.Code](#anytype.Rpc.BlockVideo.SetWidth.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug"></a>

### Rpc.Debug







<a name="anytype.Rpc.Debug.ExportLocalstore"></a>

### Rpc.Debug.ExportLocalstore







<a name="anytype.Rpc.Debug.ExportLocalstore.Request"></a>

### Rpc.Debug.ExportLocalstore.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| docIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |






<a name="anytype.Rpc.Debug.ExportLocalstore.Response"></a>

### Rpc.Debug.ExportLocalstore.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.ExportLocalstore.Response.Error](#anytype.Rpc.Debug.ExportLocalstore.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Debug.ExportLocalstore.Response.Error"></a>

### Rpc.Debug.ExportLocalstore.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.ExportLocalstore.Response.Error.Code](#anytype.Rpc.Debug.ExportLocalstore.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.Ping"></a>

### Rpc.Debug.Ping







<a name="anytype.Rpc.Debug.Ping.Request"></a>

### Rpc.Debug.Ping.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  |  |
| numberOfEventsToSend | [int32](#int32) |  |  |






<a name="anytype.Rpc.Debug.Ping.Response"></a>

### Rpc.Debug.Ping.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Ping.Response.Error](#anytype.Rpc.Debug.Ping.Response.Error) |  |  |
| index | [int32](#int32) |  |  |






<a name="anytype.Rpc.Debug.Ping.Response.Error"></a>

### Rpc.Debug.Ping.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Ping.Response.Error.Code](#anytype.Rpc.Debug.Ping.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.Sync"></a>

### Rpc.Debug.Sync







<a name="anytype.Rpc.Debug.Sync.Request"></a>

### Rpc.Debug.Sync.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| recordsTraverseLimit | [int32](#int32) |  | 0 means no limit |
| skipEmptyLogs | [bool](#bool) |  | do not set if you want the whole picture |
| tryToDownloadRemoteRecords | [bool](#bool) |  | if try we will try to download remote records in case missing |






<a name="anytype.Rpc.Debug.Sync.Response"></a>

### Rpc.Debug.Sync.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Sync.Response.Error](#anytype.Rpc.Debug.Sync.Response.Error) |  |  |
| threads | [Rpc.Debug.threadInfo](#anytype.Rpc.Debug.threadInfo) | repeated |  |
| deviceId | [string](#string) |  |  |
| totalThreads | [int32](#int32) |  |  |
| threadsWithoutReplInOwnLog | [int32](#int32) |  |  |
| threadsWithoutHeadDownloaded | [int32](#int32) |  |  |
| totalRecords | [int32](#int32) |  |  |
| totalSize | [int32](#int32) |  |  |






<a name="anytype.Rpc.Debug.Sync.Response.Error"></a>

### Rpc.Debug.Sync.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Sync.Response.Error.Code](#anytype.Rpc.Debug.Sync.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.Thread"></a>

### Rpc.Debug.Thread







<a name="anytype.Rpc.Debug.Thread.Request"></a>

### Rpc.Debug.Thread.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| threadId | [string](#string) |  |  |
| skipEmptyLogs | [bool](#bool) |  | do not set if you want the whole picture |
| tryToDownloadRemoteRecords | [bool](#bool) |  | if try we will try to download remote records in case missing |






<a name="anytype.Rpc.Debug.Thread.Response"></a>

### Rpc.Debug.Thread.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Thread.Response.Error](#anytype.Rpc.Debug.Thread.Response.Error) |  |  |
| info | [Rpc.Debug.threadInfo](#anytype.Rpc.Debug.threadInfo) |  |  |






<a name="anytype.Rpc.Debug.Thread.Response.Error"></a>

### Rpc.Debug.Thread.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Thread.Response.Error.Code](#anytype.Rpc.Debug.Thread.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.Tree"></a>

### Rpc.Debug.Tree







<a name="anytype.Rpc.Debug.Tree.Request"></a>

### Rpc.Debug.Tree.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| path | [string](#string) |  |  |
| unanonymized | [bool](#bool) |  | set to true to disable mocking of the actual data inside changes |






<a name="anytype.Rpc.Debug.Tree.Response"></a>

### Rpc.Debug.Tree.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Tree.Response.Error](#anytype.Rpc.Debug.Tree.Response.Error) |  |  |
| filename | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.Tree.Response.Error"></a>

### Rpc.Debug.Tree.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Tree.Response.Error.Code](#anytype.Rpc.Debug.Tree.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.logInfo"></a>

### Rpc.Debug.logInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| head | [string](#string) |  |  |
| headDownloaded | [bool](#bool) |  |  |
| totalRecords | [int32](#int32) |  |  |
| totalSize | [int32](#int32) |  |  |
| firstRecordTs | [int32](#int32) |  |  |
| firstRecordVer | [int32](#int32) |  |  |
| lastRecordTs | [int32](#int32) |  |  |
| lastRecordVer | [int32](#int32) |  |  |
| lastPullSecAgo | [int32](#int32) |  |  |
| upStatus | [string](#string) |  |  |
| downStatus | [string](#string) |  |  |
| error | [string](#string) |  |  |






<a name="anytype.Rpc.Debug.threadInfo"></a>

### Rpc.Debug.threadInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| logsWithDownloadedHead | [int32](#int32) |  |  |
| logsWithWholeTreeDownloaded | [int32](#int32) |  |  |
| logs | [Rpc.Debug.logInfo](#anytype.Rpc.Debug.logInfo) | repeated |  |
| ownLogHasCafeReplicator | [bool](#bool) |  |  |
| cafeLastPullSecAgo | [int32](#int32) |  |  |
| cafeUpStatus | [string](#string) |  |  |
| cafeDownStatus | [string](#string) |  |  |
| totalRecords | [int32](#int32) |  |  |
| totalSize | [int32](#int32) |  |  |
| error | [string](#string) |  |  |






<a name="anytype.Rpc.File"></a>

### Rpc.File







<a name="anytype.Rpc.File.Download"></a>

### Rpc.File.Download







<a name="anytype.Rpc.File.Download.Request"></a>

### Rpc.File.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| path | [string](#string) |  | path to save file. Temp directory is used if empty |






<a name="anytype.Rpc.File.Download.Response"></a>

### Rpc.File.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Download.Response.Error](#anytype.Rpc.File.Download.Response.Error) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype.Rpc.File.Download.Response.Error"></a>

### Rpc.File.Download.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Download.Response.Error.Code](#anytype.Rpc.File.Download.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.File.Drop"></a>

### Rpc.File.Drop







<a name="anytype.Rpc.File.Drop.Request"></a>

### Rpc.File.Drop.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype.model.Block.Position) |  | position relatively to the dropTargetId simple block |
| localFilePaths | [string](#string) | repeated |  |






<a name="anytype.Rpc.File.Drop.Response"></a>

### Rpc.File.Drop.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Drop.Response.Error](#anytype.Rpc.File.Drop.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.File.Drop.Response.Error"></a>

### Rpc.File.Drop.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Drop.Response.Error.Code](#anytype.Rpc.File.Drop.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.File.ListOffload"></a>

### Rpc.File.ListOffload







<a name="anytype.Rpc.File.ListOffload.Request"></a>

### Rpc.File.ListOffload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| onlyIds | [string](#string) | repeated | empty means all |
| includeNotPinned | [bool](#bool) |  | false mean not-yet-pinned files will be not |






<a name="anytype.Rpc.File.ListOffload.Response"></a>

### Rpc.File.ListOffload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.ListOffload.Response.Error](#anytype.Rpc.File.ListOffload.Response.Error) |  |  |
| filesOffloaded | [int32](#int32) |  |  |
| bytesOffloaded | [uint64](#uint64) |  |  |






<a name="anytype.Rpc.File.ListOffload.Response.Error"></a>

### Rpc.File.ListOffload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.ListOffload.Response.Error.Code](#anytype.Rpc.File.ListOffload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.File.Offload"></a>

### Rpc.File.Offload







<a name="anytype.Rpc.File.Offload.Request"></a>

### Rpc.File.Offload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| includeNotPinned | [bool](#bool) |  |  |






<a name="anytype.Rpc.File.Offload.Response"></a>

### Rpc.File.Offload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Offload.Response.Error](#anytype.Rpc.File.Offload.Response.Error) |  |  |
| bytesOffloaded | [uint64](#uint64) |  |  |






<a name="anytype.Rpc.File.Offload.Response.Error"></a>

### Rpc.File.Offload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Offload.Response.Error.Code](#anytype.Rpc.File.Offload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.File.Upload"></a>

### Rpc.File.Upload







<a name="anytype.Rpc.File.Upload.Request"></a>

### Rpc.File.Upload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| type | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |
| disableEncryption | [bool](#bool) |  | deprecated, has no affect |
| style | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Rpc.File.Upload.Response"></a>

### Rpc.File.Upload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Upload.Response.Error](#anytype.Rpc.File.Upload.Response.Error) |  |  |
| hash | [string](#string) |  |  |






<a name="anytype.Rpc.File.Upload.Response.Error"></a>

### Rpc.File.Upload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Upload.Response.Error.Code](#anytype.Rpc.File.Upload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.GenericErrorResponse"></a>

### Rpc.GenericErrorResponse



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.GenericErrorResponse.Error](#anytype.Rpc.GenericErrorResponse.Error) |  |  |






<a name="anytype.Rpc.GenericErrorResponse.Error"></a>

### Rpc.GenericErrorResponse.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.GenericErrorResponse.Error.Code](#anytype.Rpc.GenericErrorResponse.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.History"></a>

### Rpc.History







<a name="anytype.Rpc.History.GetVersions"></a>

### Rpc.History.GetVersions
returns list of versions (changes)






<a name="anytype.Rpc.History.GetVersions.Request"></a>

### Rpc.History.GetVersions.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) |  |  |
| lastVersionId | [string](#string) |  | when indicated, results will include versions before given id |
| limit | [int32](#int32) |  | desired count of versions |






<a name="anytype.Rpc.History.GetVersions.Response"></a>

### Rpc.History.GetVersions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.GetVersions.Response.Error](#anytype.Rpc.History.GetVersions.Response.Error) |  |  |
| versions | [Rpc.History.Version](#anytype.Rpc.History.Version) | repeated |  |






<a name="anytype.Rpc.History.GetVersions.Response.Error"></a>

### Rpc.History.GetVersions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.GetVersions.Response.Error.Code](#anytype.Rpc.History.GetVersions.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.History.SetVersion"></a>

### Rpc.History.SetVersion







<a name="anytype.Rpc.History.SetVersion.Request"></a>

### Rpc.History.SetVersion.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) |  |  |
| versionId | [string](#string) |  |  |






<a name="anytype.Rpc.History.SetVersion.Response"></a>

### Rpc.History.SetVersion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.SetVersion.Response.Error](#anytype.Rpc.History.SetVersion.Response.Error) |  |  |






<a name="anytype.Rpc.History.SetVersion.Response.Error"></a>

### Rpc.History.SetVersion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.SetVersion.Response.Error.Code](#anytype.Rpc.History.SetVersion.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.History.ShowVersion"></a>

### Rpc.History.ShowVersion
returns blockShow event for given version






<a name="anytype.Rpc.History.ShowVersion.Request"></a>

### Rpc.History.ShowVersion.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) |  |  |
| versionId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.History.ShowVersion.Response"></a>

### Rpc.History.ShowVersion.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.ShowVersion.Response.Error](#anytype.Rpc.History.ShowVersion.Response.Error) |  |  |
| objectShow | [Event.Object.Show](#anytype.Event.Object.Show) |  |  |
| version | [Rpc.History.Version](#anytype.Rpc.History.Version) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.History.ShowVersion.Response.Error"></a>

### Rpc.History.ShowVersion.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.ShowVersion.Response.Error.Code](#anytype.Rpc.History.ShowVersion.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.History.Version"></a>

### Rpc.History.Version



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| previousIds | [string](#string) | repeated |  |
| authorId | [string](#string) |  |  |
| authorName | [string](#string) |  |  |
| time | [int64](#int64) |  |  |
| groupId | [int64](#int64) |  |  |






<a name="anytype.Rpc.LinkPreview"></a>

### Rpc.LinkPreview







<a name="anytype.Rpc.LinkPreview.Request"></a>

### Rpc.LinkPreview.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.LinkPreview.Response"></a>

### Rpc.LinkPreview.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.LinkPreview.Response.Error](#anytype.Rpc.LinkPreview.Response.Error) |  |  |
| linkPreview | [model.LinkPreview](#anytype.model.LinkPreview) |  |  |






<a name="anytype.Rpc.LinkPreview.Response.Error"></a>

### Rpc.LinkPreview.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.LinkPreview.Response.Error.Code](#anytype.Rpc.LinkPreview.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Metrics"></a>

### Rpc.Metrics







<a name="anytype.Rpc.Metrics.SetParameters"></a>

### Rpc.Metrics.SetParameters







<a name="anytype.Rpc.Metrics.SetParameters.Request"></a>

### Rpc.Metrics.SetParameters.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| platform | [string](#string) |  |  |






<a name="anytype.Rpc.Metrics.SetParameters.Response"></a>

### Rpc.Metrics.SetParameters.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Metrics.SetParameters.Response.Error](#anytype.Rpc.Metrics.SetParameters.Response.Error) |  |  |






<a name="anytype.Rpc.Metrics.SetParameters.Response.Error"></a>

### Rpc.Metrics.SetParameters.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Metrics.SetParameters.Response.Error.Code](#anytype.Rpc.Metrics.SetParameters.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Navigation"></a>

### Rpc.Navigation







<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks"></a>

### Rpc.Navigation.GetObjectInfoWithLinks
Get the info for page alongside with info for all inbound and outbound links from/to this page






<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| context | [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context) |  |  |






<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error) |  |  |
| object | [model.ObjectInfoWithLinks](#anytype.model.ObjectInfoWithLinks) |  |  |






<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Navigation.ListObjects"></a>

### Rpc.Navigation.ListObjects







<a name="anytype.Rpc.Navigation.ListObjects.Request"></a>

### Rpc.Navigation.ListObjects.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| context | [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context) |  |  |
| fullText | [string](#string) |  |  |
| limit | [int32](#int32) |  |  |
| offset | [int32](#int32) |  |  |






<a name="anytype.Rpc.Navigation.ListObjects.Response"></a>

### Rpc.Navigation.ListObjects.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Navigation.ListObjects.Response.Error](#anytype.Rpc.Navigation.ListObjects.Response.Error) |  |  |
| objects | [model.ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |






<a name="anytype.Rpc.Navigation.ListObjects.Response.Error"></a>

### Rpc.Navigation.ListObjects.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype.Rpc.Navigation.ListObjects.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object"></a>

### Rpc.Object







<a name="anytype.Rpc.Object.AddWithObjectId"></a>

### Rpc.Object.AddWithObjectId







<a name="anytype.Rpc.Object.AddWithObjectId.Request"></a>

### Rpc.Object.AddWithObjectId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| payload | [string](#string) |  |  |






<a name="anytype.Rpc.Object.AddWithObjectId.Response"></a>

### Rpc.Object.AddWithObjectId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.AddWithObjectId.Response.Error](#anytype.Rpc.Object.AddWithObjectId.Response.Error) |  |  |






<a name="anytype.Rpc.Object.AddWithObjectId.Response.Error"></a>

### Rpc.Object.AddWithObjectId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.AddWithObjectId.Response.Error.Code](#anytype.Rpc.Object.AddWithObjectId.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ApplyTemplate"></a>

### Rpc.Object.ApplyTemplate







<a name="anytype.Rpc.Object.ApplyTemplate.Request"></a>

### Rpc.Object.ApplyTemplate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| templateId | [string](#string) |  | id of template |






<a name="anytype.Rpc.Object.ApplyTemplate.Response"></a>

### Rpc.Object.ApplyTemplate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ApplyTemplate.Response.Error](#anytype.Rpc.Object.ApplyTemplate.Response.Error) |  |  |






<a name="anytype.Rpc.Object.ApplyTemplate.Response.Error"></a>

### Rpc.Object.ApplyTemplate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ApplyTemplate.Response.Error.Code](#anytype.Rpc.Object.ApplyTemplate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Close"></a>

### Rpc.Object.Close







<a name="anytype.Rpc.Object.Close.Request"></a>

### Rpc.Object.Close.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| blockId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Close.Response"></a>

### Rpc.Object.Close.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Close.Response.Error](#anytype.Rpc.Object.Close.Response.Error) |  |  |






<a name="anytype.Rpc.Object.Close.Response.Error"></a>

### Rpc.Object.Close.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Close.Response.Error.Code](#anytype.Rpc.Object.Close.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Create"></a>

### Rpc.Object.Create







<a name="anytype.Rpc.Object.Create.Request"></a>

### Rpc.Object.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | object details |






<a name="anytype.Rpc.Object.Create.Response"></a>

### Rpc.Object.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Create.Response.Error](#anytype.Rpc.Object.Create.Response.Error) |  |  |
| pageId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.Create.Response.Error"></a>

### Rpc.Object.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Create.Response.Error.Code](#anytype.Rpc.Object.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.CreateSet"></a>

### Rpc.Object.CreateSet







<a name="anytype.Rpc.Object.CreateSet.Request"></a>

### Rpc.Object.CreateSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | if omitted the name of page will be the same with object type |
| templateId | [string](#string) |  | optional template id for creating from template |






<a name="anytype.Rpc.Object.CreateSet.Response"></a>

### Rpc.Object.CreateSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.CreateSet.Response.Error](#anytype.Rpc.Object.CreateSet.Response.Error) |  |  |
| id | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.CreateSet.Response.Error"></a>

### Rpc.Object.CreateSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.CreateSet.Response.Error.Code](#anytype.Rpc.Object.CreateSet.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Duplicate"></a>

### Rpc.Object.Duplicate







<a name="anytype.Rpc.Object.Duplicate.Request"></a>

### Rpc.Object.Duplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Duplicate.Response"></a>

### Rpc.Object.Duplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Duplicate.Response.Error](#anytype.Rpc.Object.Duplicate.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.Object.Duplicate.Response.Error"></a>

### Rpc.Object.Duplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Duplicate.Response.Error.Code](#anytype.Rpc.Object.Duplicate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Export"></a>

### Rpc.Object.Export







<a name="anytype.Rpc.Object.Export.Request"></a>

### Rpc.Object.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| docIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |
| format | [Rpc.Object.Export.Format](#anytype.Rpc.Object.Export.Format) |  | export format |
| zip | [bool](#bool) |  | save as zip file |
| includeNested | [bool](#bool) |  | include all nested |
| includeFiles | [bool](#bool) |  | include all files |






<a name="anytype.Rpc.Object.Export.Response"></a>

### Rpc.Object.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Export.Response.Error](#anytype.Rpc.Object.Export.Response.Error) |  |  |
| path | [string](#string) |  |  |
| succeed | [int32](#int32) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.Export.Response.Error"></a>

### Rpc.Object.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Export.Response.Error.Code](#anytype.Rpc.Object.Export.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Graph"></a>

### Rpc.Object.Graph







<a name="anytype.Rpc.Object.Graph.Edge"></a>

### Rpc.Object.Graph.Edge



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) |  |  |
| target | [string](#string) |  |  |
| name | [string](#string) |  |  |
| type | [Rpc.Object.Graph.Edge.Type](#anytype.Rpc.Object.Graph.Edge.Type) |  |  |
| description | [string](#string) |  |  |
| iconImage | [string](#string) |  |  |
| iconEmoji | [string](#string) |  |  |
| hidden | [bool](#bool) |  |  |






<a name="anytype.Rpc.Object.Graph.Node"></a>

### Rpc.Object.Graph.Node



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [string](#string) |  |  |
| name | [string](#string) |  |  |
| layout | [int32](#int32) |  |  |
| description | [string](#string) |  |  |
| iconImage | [string](#string) |  |  |
| iconEmoji | [string](#string) |  |  |
| done | [bool](#bool) |  |  |
| relationFormat | [int32](#int32) |  |  |
| snippet | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Graph.Request"></a>

### Rpc.Object.Graph.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filters | [model.Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated |  |
| limit | [int32](#int32) |  |  |
| objectTypeFilter | [string](#string) | repeated | additional filter by objectTypes |






<a name="anytype.Rpc.Object.Graph.Response"></a>

### Rpc.Object.Graph.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Graph.Response.Error](#anytype.Rpc.Object.Graph.Response.Error) |  |  |
| nodes | [Rpc.Object.Graph.Node](#anytype.Rpc.Object.Graph.Node) | repeated |  |
| edges | [Rpc.Object.Graph.Edge](#anytype.Rpc.Object.Graph.Edge) | repeated |  |






<a name="anytype.Rpc.Object.Graph.Response.Error"></a>

### Rpc.Object.Graph.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Graph.Response.Error.Code](#anytype.Rpc.Object.Graph.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ImportMarkdown"></a>

### Rpc.Object.ImportMarkdown







<a name="anytype.Rpc.Object.ImportMarkdown.Request"></a>

### Rpc.Object.ImportMarkdown.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| importPath | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ImportMarkdown.Response"></a>

### Rpc.Object.ImportMarkdown.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ImportMarkdown.Response.Error](#anytype.Rpc.Object.ImportMarkdown.Response.Error) |  |  |
| rootLinkIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.ImportMarkdown.Response.Error"></a>

### Rpc.Object.ImportMarkdown.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ImportMarkdown.Response.Error.Code](#anytype.Rpc.Object.ImportMarkdown.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ListDelete"></a>

### Rpc.Object.ListDelete







<a name="anytype.Rpc.Object.ListDelete.Request"></a>

### Rpc.Object.ListDelete.Request
Deletes the object, keys from the local store and unsubscribe from remote changes. Also offloads all orphan files


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated | objects to remove |






<a name="anytype.Rpc.Object.ListDelete.Response"></a>

### Rpc.Object.ListDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListDelete.Response.Error](#anytype.Rpc.Object.ListDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.ListDelete.Response.Error"></a>

### Rpc.Object.ListDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListDelete.Response.Error.Code](#anytype.Rpc.Object.ListDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ListSetIsArchived"></a>

### Rpc.Object.ListSetIsArchived







<a name="anytype.Rpc.Object.ListSetIsArchived.Request"></a>

### Rpc.Object.ListSetIsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isArchived | [bool](#bool) |  |  |






<a name="anytype.Rpc.Object.ListSetIsArchived.Response"></a>

### Rpc.Object.ListSetIsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListSetIsArchived.Response.Error](#anytype.Rpc.Object.ListSetIsArchived.Response.Error) |  |  |






<a name="anytype.Rpc.Object.ListSetIsArchived.Response.Error"></a>

### Rpc.Object.ListSetIsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListSetIsArchived.Response.Error.Code](#anytype.Rpc.Object.ListSetIsArchived.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ListSetIsFavorite"></a>

### Rpc.Object.ListSetIsFavorite







<a name="anytype.Rpc.Object.ListSetIsFavorite.Request"></a>

### Rpc.Object.ListSetIsFavorite.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isFavorite | [bool](#bool) |  |  |






<a name="anytype.Rpc.Object.ListSetIsFavorite.Response"></a>

### Rpc.Object.ListSetIsFavorite.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ListSetIsFavorite.Response.Error](#anytype.Rpc.Object.ListSetIsFavorite.Response.Error) |  |  |






<a name="anytype.Rpc.Object.ListSetIsFavorite.Response.Error"></a>

### Rpc.Object.ListSetIsFavorite.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ListSetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.ListSetIsFavorite.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Open"></a>

### Rpc.Object.Open







<a name="anytype.Rpc.Object.Open.Request"></a>

### Rpc.Object.Open.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| blockId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Open.Response"></a>

### Rpc.Object.Open.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Open.Response.Error](#anytype.Rpc.Object.Open.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.Open.Response.Error"></a>

### Rpc.Object.Open.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Open.Response.Error.Code](#anytype.Rpc.Object.Open.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.OpenBreadcrumbs"></a>

### Rpc.Object.OpenBreadcrumbs







<a name="anytype.Rpc.Object.OpenBreadcrumbs.Request"></a>

### Rpc.Object.OpenBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.OpenBreadcrumbs.Response"></a>

### Rpc.Object.OpenBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.OpenBreadcrumbs.Response.Error](#anytype.Rpc.Object.OpenBreadcrumbs.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.OpenBreadcrumbs.Response.Error"></a>

### Rpc.Object.OpenBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Object.OpenBreadcrumbs.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Redo"></a>

### Rpc.Object.Redo







<a name="anytype.Rpc.Object.Redo.Request"></a>

### Rpc.Object.Redo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |






<a name="anytype.Rpc.Object.Redo.Response"></a>

### Rpc.Object.Redo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Redo.Response.Error](#anytype.Rpc.Object.Redo.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| counters | [Rpc.Object.UndoRedoCounter](#anytype.Rpc.Object.UndoRedoCounter) |  |  |






<a name="anytype.Rpc.Object.Redo.Response.Error"></a>

### Rpc.Object.Redo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Redo.Response.Error.Code](#anytype.Rpc.Object.Redo.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Search"></a>

### Rpc.Object.Search







<a name="anytype.Rpc.Object.Search.Request"></a>

### Rpc.Object.Search.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filters | [model.Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated |  |
| sorts | [model.Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort) | repeated |  |
| fullText | [string](#string) |  |  |
| offset | [int32](#int32) |  |  |
| limit | [int32](#int32) |  |  |
| objectTypeFilter | [string](#string) | repeated | additional filter by objectTypes

deprecated, to be removed |
| keys | [string](#string) | repeated | needed keys in details for return, when empty - will return all |






<a name="anytype.Rpc.Object.Search.Response"></a>

### Rpc.Object.Search.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Search.Response.Error](#anytype.Rpc.Object.Search.Response.Error) |  |  |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |






<a name="anytype.Rpc.Object.Search.Response.Error"></a>

### Rpc.Object.Search.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Search.Response.Error.Code](#anytype.Rpc.Object.Search.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SearchSubscribe"></a>

### Rpc.Object.SearchSubscribe







<a name="anytype.Rpc.Object.SearchSubscribe.Request"></a>

### Rpc.Object.SearchSubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| filters | [model.Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated | filters |
| sorts | [model.Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort) | repeated | sorts |
| limit | [int64](#int64) |  | results limit |
| offset | [int64](#int64) |  | initial offset; middleware will find afterId |
| keys | [string](#string) | repeated | (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| afterId | [string](#string) |  | (optional) pagination: middleware will return results after given id |
| beforeId | [string](#string) |  | (optional) pagination: middleware will return results before given id |
| source | [string](#string) | repeated |  |
| ignoreWorkspace | [string](#string) |  |  |
| noDepSubscription | [bool](#bool) |  | disable dependent subscription |






<a name="anytype.Rpc.Object.SearchSubscribe.Response"></a>

### Rpc.Object.SearchSubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SearchSubscribe.Response.Error](#anytype.Rpc.Object.SearchSubscribe.Response.Error) |  |  |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| dependencies | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| subId | [string](#string) |  |  |
| counters | [Event.Object.Subscription.Counters](#anytype.Event.Object.Subscription.Counters) |  |  |






<a name="anytype.Rpc.Object.SearchSubscribe.Response.Error"></a>

### Rpc.Object.SearchSubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SearchSubscribe.Response.Error.Code](#anytype.Rpc.Object.SearchSubscribe.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SearchUnsubscribe"></a>

### Rpc.Object.SearchUnsubscribe







<a name="anytype.Rpc.Object.SearchUnsubscribe.Request"></a>

### Rpc.Object.SearchUnsubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subIds | [string](#string) | repeated |  |






<a name="anytype.Rpc.Object.SearchUnsubscribe.Response"></a>

### Rpc.Object.SearchUnsubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SearchUnsubscribe.Response.Error](#anytype.Rpc.Object.SearchUnsubscribe.Response.Error) |  |  |






<a name="anytype.Rpc.Object.SearchUnsubscribe.Response.Error"></a>

### Rpc.Object.SearchUnsubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SearchUnsubscribe.Response.Error.Code](#anytype.Rpc.Object.SearchUnsubscribe.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetBreadcrumbs"></a>

### Rpc.Object.SetBreadcrumbs







<a name="anytype.Rpc.Object.SetBreadcrumbs.Request"></a>

### Rpc.Object.SetBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| breadcrumbsId | [string](#string) |  |  |
| ids | [string](#string) | repeated | page ids |






<a name="anytype.Rpc.Object.SetBreadcrumbs.Response"></a>

### Rpc.Object.SetBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetBreadcrumbs.Response.Error](#anytype.Rpc.Object.SetBreadcrumbs.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.SetBreadcrumbs.Response.Error"></a>

### Rpc.Object.SetBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetBreadcrumbs.Response.Error.Code](#anytype.Rpc.Object.SetBreadcrumbs.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetDetails"></a>

### Rpc.Object.SetDetails







<a name="anytype.Rpc.Object.SetDetails.Detail"></a>

### Rpc.Object.SetDetails.Detail



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  | NUll - removes key |






<a name="anytype.Rpc.Object.SetDetails.Request"></a>

### Rpc.Object.SetDetails.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| details | [Rpc.Object.SetDetails.Detail](#anytype.Rpc.Object.SetDetails.Detail) | repeated |  |






<a name="anytype.Rpc.Object.SetDetails.Response"></a>

### Rpc.Object.SetDetails.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetDetails.Response.Error](#anytype.Rpc.Object.SetDetails.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.SetDetails.Response.Error"></a>

### Rpc.Object.SetDetails.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetDetails.Response.Error.Code](#anytype.Rpc.Object.SetDetails.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetIsArchived"></a>

### Rpc.Object.SetIsArchived







<a name="anytype.Rpc.Object.SetIsArchived.Request"></a>

### Rpc.Object.SetIsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| isArchived | [bool](#bool) |  |  |






<a name="anytype.Rpc.Object.SetIsArchived.Response"></a>

### Rpc.Object.SetIsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetIsArchived.Response.Error](#anytype.Rpc.Object.SetIsArchived.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.SetIsArchived.Response.Error"></a>

### Rpc.Object.SetIsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype.Rpc.Object.SetIsArchived.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetIsFavorite"></a>

### Rpc.Object.SetIsFavorite







<a name="anytype.Rpc.Object.SetIsFavorite.Request"></a>

### Rpc.Object.SetIsFavorite.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| isFavorite | [bool](#bool) |  |  |






<a name="anytype.Rpc.Object.SetIsFavorite.Response"></a>

### Rpc.Object.SetIsFavorite.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetIsFavorite.Response.Error](#anytype.Rpc.Object.SetIsFavorite.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.SetIsFavorite.Response.Error"></a>

### Rpc.Object.SetIsFavorite.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.SetIsFavorite.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetLayout"></a>

### Rpc.Object.SetLayout







<a name="anytype.Rpc.Object.SetLayout.Request"></a>

### Rpc.Object.SetLayout.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| layout | [model.ObjectType.Layout](#anytype.model.ObjectType.Layout) |  |  |






<a name="anytype.Rpc.Object.SetLayout.Response"></a>

### Rpc.Object.SetLayout.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetLayout.Response.Error](#anytype.Rpc.Object.SetLayout.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.SetLayout.Response.Error"></a>

### Rpc.Object.SetLayout.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetLayout.Response.Error.Code](#anytype.Rpc.Object.SetLayout.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetObjectType"></a>

### Rpc.Object.SetObjectType







<a name="anytype.Rpc.Object.SetObjectType.Request"></a>

### Rpc.Object.SetObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| objectTypeUrl | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SetObjectType.Response"></a>

### Rpc.Object.SetObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetObjectType.Response.Error](#anytype.Rpc.Object.SetObjectType.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.SetObjectType.Response.Error"></a>

### Rpc.Object.SetObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetObjectType.Response.Error.Code](#anytype.Rpc.Object.SetObjectType.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ShareByLink"></a>

### Rpc.Object.ShareByLink







<a name="anytype.Rpc.Object.ShareByLink.Request"></a>

### Rpc.Object.ShareByLink.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ShareByLink.Response"></a>

### Rpc.Object.ShareByLink.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| link | [string](#string) |  |  |
| error | [Rpc.Object.ShareByLink.Response.Error](#anytype.Rpc.Object.ShareByLink.Response.Error) |  |  |






<a name="anytype.Rpc.Object.ShareByLink.Response.Error"></a>

### Rpc.Object.ShareByLink.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ShareByLink.Response.Error.Code](#anytype.Rpc.Object.ShareByLink.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Show"></a>

### Rpc.Object.Show







<a name="anytype.Rpc.Object.Show.Request"></a>

### Rpc.Object.Show.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| blockId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Show.Response"></a>

### Rpc.Object.Show.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Show.Response.Error](#anytype.Rpc.Object.Show.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.Show.Response.Error"></a>

### Rpc.Object.Show.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Show.Response.Error.Code](#anytype.Rpc.Object.Show.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SubscribeIds"></a>

### Rpc.Object.SubscribeIds







<a name="anytype.Rpc.Object.SubscribeIds.Request"></a>

### Rpc.Object.SubscribeIds.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| ids | [string](#string) | repeated | ids for subscribe |
| keys | [string](#string) | repeated | sorts (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| ignoreWorkspace | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SubscribeIds.Response"></a>

### Rpc.Object.SubscribeIds.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SubscribeIds.Response.Error](#anytype.Rpc.Object.SubscribeIds.Response.Error) |  |  |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| dependencies | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| subId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.SubscribeIds.Response.Error"></a>

### Rpc.Object.SubscribeIds.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SubscribeIds.Response.Error.Code](#anytype.Rpc.Object.SubscribeIds.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ToSet"></a>

### Rpc.Object.ToSet







<a name="anytype.Rpc.Object.ToSet.Request"></a>

### Rpc.Object.ToSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype.Rpc.Object.ToSet.Response"></a>

### Rpc.Object.ToSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ToSet.Response.Error](#anytype.Rpc.Object.ToSet.Response.Error) |  |  |
| setId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.ToSet.Response.Error"></a>

### Rpc.Object.ToSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ToSet.Response.Error.Code](#anytype.Rpc.Object.ToSet.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.Undo"></a>

### Rpc.Object.Undo







<a name="anytype.Rpc.Object.Undo.Request"></a>

### Rpc.Object.Undo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |






<a name="anytype.Rpc.Object.Undo.Response"></a>

### Rpc.Object.Undo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Undo.Response.Error](#anytype.Rpc.Object.Undo.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| counters | [Rpc.Object.UndoRedoCounter](#anytype.Rpc.Object.UndoRedoCounter) |  |  |






<a name="anytype.Rpc.Object.Undo.Response.Error"></a>

### Rpc.Object.Undo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Undo.Response.Error.Code](#anytype.Rpc.Object.Undo.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.UndoRedoCounter"></a>

### Rpc.Object.UndoRedoCounter
Available undo/redo operations


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| undo | [int32](#int32) |  |  |
| redo | [int32](#int32) |  |  |






<a name="anytype.Rpc.ObjectRelation"></a>

### Rpc.ObjectRelation







<a name="anytype.Rpc.ObjectRelation.Add"></a>

### Rpc.ObjectRelation.Add







<a name="anytype.Rpc.ObjectRelation.Add.Request"></a>

### Rpc.ObjectRelation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.ObjectRelation.Add.Response"></a>

### Rpc.ObjectRelation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.Add.Response.Error](#anytype.Rpc.ObjectRelation.Add.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| relationKey | [string](#string) |  | deprecated |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.ObjectRelation.Add.Response.Error"></a>

### Rpc.ObjectRelation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.Add.Response.Error.Code](#anytype.Rpc.ObjectRelation.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.AddFeatured"></a>

### Rpc.ObjectRelation.AddFeatured







<a name="anytype.Rpc.ObjectRelation.AddFeatured.Request"></a>

### Rpc.ObjectRelation.AddFeatured.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype.Rpc.ObjectRelation.AddFeatured.Response"></a>

### Rpc.ObjectRelation.AddFeatured.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.AddFeatured.Response.Error](#anytype.Rpc.ObjectRelation.AddFeatured.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectRelation.AddFeatured.Response.Error"></a>

### Rpc.ObjectRelation.AddFeatured.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.AddFeatured.Response.Error.Code](#anytype.Rpc.ObjectRelation.AddFeatured.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.Delete"></a>

### Rpc.ObjectRelation.Delete







<a name="anytype.Rpc.ObjectRelation.Delete.Request"></a>

### Rpc.ObjectRelation.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.Delete.Response"></a>

### Rpc.ObjectRelation.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.Delete.Response.Error](#anytype.Rpc.ObjectRelation.Delete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectRelation.Delete.Response.Error"></a>

### Rpc.ObjectRelation.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.Delete.Response.Error.Code](#anytype.Rpc.ObjectRelation.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.ListAvailable"></a>

### Rpc.ObjectRelation.ListAvailable







<a name="anytype.Rpc.ObjectRelation.ListAvailable.Request"></a>

### Rpc.ObjectRelation.ListAvailable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.ListAvailable.Response"></a>

### Rpc.ObjectRelation.ListAvailable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.ListAvailable.Response.Error](#anytype.Rpc.ObjectRelation.ListAvailable.Response.Error) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.ObjectRelation.ListAvailable.Response.Error"></a>

### Rpc.ObjectRelation.ListAvailable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.ListAvailable.Response.Error.Code](#anytype.Rpc.ObjectRelation.ListAvailable.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.RemoveFeatured"></a>

### Rpc.ObjectRelation.RemoveFeatured







<a name="anytype.Rpc.ObjectRelation.RemoveFeatured.Request"></a>

### Rpc.ObjectRelation.RemoveFeatured.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype.Rpc.ObjectRelation.RemoveFeatured.Response"></a>

### Rpc.ObjectRelation.RemoveFeatured.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.RemoveFeatured.Response.Error](#anytype.Rpc.ObjectRelation.RemoveFeatured.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectRelation.RemoveFeatured.Response.Error"></a>

### Rpc.ObjectRelation.RemoveFeatured.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code](#anytype.Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelation.Update"></a>

### Rpc.ObjectRelation.Update







<a name="anytype.Rpc.ObjectRelation.Update.Request"></a>

### Rpc.ObjectRelation.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | key of relation to update |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.ObjectRelation.Update.Response"></a>

### Rpc.ObjectRelation.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelation.Update.Response.Error](#anytype.Rpc.ObjectRelation.Update.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectRelation.Update.Response.Error"></a>

### Rpc.ObjectRelation.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelation.Update.Response.Error.Code](#anytype.Rpc.ObjectRelation.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelationOption"></a>

### Rpc.ObjectRelationOption







<a name="anytype.Rpc.ObjectRelationOption.Add"></a>

### Rpc.ObjectRelationOption.Add







<a name="anytype.Rpc.ObjectRelationOption.Add.Request"></a>

### Rpc.ObjectRelationOption.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |






<a name="anytype.Rpc.ObjectRelationOption.Add.Response"></a>

### Rpc.ObjectRelationOption.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelationOption.Add.Response.Error](#anytype.Rpc.ObjectRelationOption.Add.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  |  |






<a name="anytype.Rpc.ObjectRelationOption.Add.Response.Error"></a>

### Rpc.ObjectRelationOption.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelationOption.Add.Response.Error.Code](#anytype.Rpc.ObjectRelationOption.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelationOption.Delete"></a>

### Rpc.ObjectRelationOption.Delete







<a name="anytype.Rpc.ObjectRelationOption.Delete.Request"></a>

### Rpc.ObjectRelationOption.Delete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | relation key to add the option |
| optionId | [string](#string) |  | id of select options to remove |
| confirmRemoveAllValuesInRecords | [bool](#bool) |  | confirm remove all values in records |






<a name="anytype.Rpc.ObjectRelationOption.Delete.Response"></a>

### Rpc.ObjectRelationOption.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelationOption.Delete.Response.Error](#anytype.Rpc.ObjectRelationOption.Delete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectRelationOption.Delete.Response.Error"></a>

### Rpc.ObjectRelationOption.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelationOption.Delete.Response.Error.Code](#anytype.Rpc.ObjectRelationOption.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectRelationOption.Update"></a>

### Rpc.ObjectRelationOption.Update







<a name="anytype.Rpc.ObjectRelationOption.Update.Request"></a>

### Rpc.ObjectRelationOption.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |






<a name="anytype.Rpc.ObjectRelationOption.Update.Response"></a>

### Rpc.ObjectRelationOption.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectRelationOption.Update.Response.Error](#anytype.Rpc.ObjectRelationOption.Update.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectRelationOption.Update.Response.Error"></a>

### Rpc.ObjectRelationOption.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectRelationOption.Update.Response.Error.Code](#anytype.Rpc.ObjectRelationOption.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType"></a>

### Rpc.ObjectType







<a name="anytype.Rpc.ObjectType.Create"></a>

### Rpc.ObjectType.Create







<a name="anytype.Rpc.ObjectType.Create.Request"></a>

### Rpc.ObjectType.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectType | [model.ObjectType](#anytype.model.ObjectType) |  |  |






<a name="anytype.Rpc.ObjectType.Create.Response"></a>

### Rpc.ObjectType.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Create.Response.Error](#anytype.Rpc.ObjectType.Create.Response.Error) |  |  |
| objectType | [model.ObjectType](#anytype.model.ObjectType) |  |  |






<a name="anytype.Rpc.ObjectType.Create.Response.Error"></a>

### Rpc.ObjectType.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Create.Response.Error.Code](#anytype.Rpc.ObjectType.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType.List"></a>

### Rpc.ObjectType.List







<a name="anytype.Rpc.ObjectType.List.Request"></a>

### Rpc.ObjectType.List.Request







<a name="anytype.Rpc.ObjectType.List.Response"></a>

### Rpc.ObjectType.List.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.List.Response.Error](#anytype.Rpc.ObjectType.List.Response.Error) |  |  |
| objectTypes | [model.ObjectType](#anytype.model.ObjectType) | repeated |  |






<a name="anytype.Rpc.ObjectType.List.Response.Error"></a>

### Rpc.ObjectType.List.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.List.Response.Error.Code](#anytype.Rpc.ObjectType.List.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType.Relation"></a>

### Rpc.ObjectType.Relation







<a name="anytype.Rpc.ObjectType.Relation.Add"></a>

### Rpc.ObjectType.Relation.Add







<a name="anytype.Rpc.ObjectType.Relation.Add.Request"></a>

### Rpc.ObjectType.Relation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.ObjectType.Relation.Add.Response"></a>

### Rpc.ObjectType.Relation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Add.Response.Error](#anytype.Rpc.ObjectType.Relation.Add.Response.Error) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.ObjectType.Relation.Add.Response.Error"></a>

### Rpc.ObjectType.Relation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.List"></a>

### Rpc.ObjectType.Relation.List







<a name="anytype.Rpc.ObjectType.Relation.List.Request"></a>

### Rpc.ObjectType.Relation.List.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| appendRelationsFromOtherTypes | [bool](#bool) |  | add relations from other object types in the end |






<a name="anytype.Rpc.ObjectType.Relation.List.Response"></a>

### Rpc.ObjectType.Relation.List.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.List.Response.Error](#anytype.Rpc.ObjectType.Relation.List.Response.Error) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.ObjectType.Relation.List.Response.Error"></a>

### Rpc.ObjectType.Relation.List.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.List.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.Remove"></a>

### Rpc.ObjectType.Relation.Remove







<a name="anytype.Rpc.ObjectType.Relation.Remove.Request"></a>

### Rpc.ObjectType.Relation.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| relationKey | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.Remove.Response"></a>

### Rpc.ObjectType.Relation.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.Remove.Response.Error"></a>

### Rpc.ObjectType.Relation.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.Update"></a>

### Rpc.ObjectType.Relation.Update







<a name="anytype.Rpc.ObjectType.Relation.Update.Request"></a>

### Rpc.ObjectType.Relation.Update.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) |  |  |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.Update.Response"></a>

### Rpc.ObjectType.Relation.Update.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Update.Response.Error](#anytype.Rpc.ObjectType.Relation.Update.Response.Error) |  |  |






<a name="anytype.Rpc.ObjectType.Relation.Update.Response.Error"></a>

### Rpc.ObjectType.Relation.Update.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Update.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Process"></a>

### Rpc.Process







<a name="anytype.Rpc.Process.Cancel"></a>

### Rpc.Process.Cancel







<a name="anytype.Rpc.Process.Cancel.Request"></a>

### Rpc.Process.Cancel.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.Rpc.Process.Cancel.Response"></a>

### Rpc.Process.Cancel.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Process.Cancel.Response.Error](#anytype.Rpc.Process.Cancel.Response.Error) |  |  |






<a name="anytype.Rpc.Process.Cancel.Response.Error"></a>

### Rpc.Process.Cancel.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Process.Cancel.Response.Error.Code](#anytype.Rpc.Process.Cancel.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Template"></a>

### Rpc.Template







<a name="anytype.Rpc.Template.Clone"></a>

### Rpc.Template.Clone







<a name="anytype.Rpc.Template.Clone.Request"></a>

### Rpc.Template.Clone.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of template block for cloning |






<a name="anytype.Rpc.Template.Clone.Response"></a>

### Rpc.Template.Clone.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.Clone.Response.Error](#anytype.Rpc.Template.Clone.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.Template.Clone.Response.Error"></a>

### Rpc.Template.Clone.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.Clone.Response.Error.Code](#anytype.Rpc.Template.Clone.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Template.CreateFromObject"></a>

### Rpc.Template.CreateFromObject







<a name="anytype.Rpc.Template.CreateFromObject.Request"></a>

### Rpc.Template.CreateFromObject.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of block for making them template |






<a name="anytype.Rpc.Template.CreateFromObject.Response"></a>

### Rpc.Template.CreateFromObject.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.CreateFromObject.Response.Error](#anytype.Rpc.Template.CreateFromObject.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.Template.CreateFromObject.Response.Error"></a>

### Rpc.Template.CreateFromObject.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.CreateFromObject.Response.Error.Code](#anytype.Rpc.Template.CreateFromObject.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Template.CreateFromObjectType"></a>

### Rpc.Template.CreateFromObjectType







<a name="anytype.Rpc.Template.CreateFromObjectType.Request"></a>

### Rpc.Template.CreateFromObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectType | [string](#string) |  | id of desired object type |






<a name="anytype.Rpc.Template.CreateFromObjectType.Response"></a>

### Rpc.Template.CreateFromObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.CreateFromObjectType.Response.Error](#anytype.Rpc.Template.CreateFromObjectType.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.Template.CreateFromObjectType.Response.Error"></a>

### Rpc.Template.CreateFromObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.CreateFromObjectType.Response.Error.Code](#anytype.Rpc.Template.CreateFromObjectType.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Template.ExportAll"></a>

### Rpc.Template.ExportAll







<a name="anytype.Rpc.Template.ExportAll.Request"></a>

### Rpc.Template.ExportAll.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |






<a name="anytype.Rpc.Template.ExportAll.Response"></a>

### Rpc.Template.ExportAll.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Template.ExportAll.Response.Error](#anytype.Rpc.Template.ExportAll.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Template.ExportAll.Response.Error"></a>

### Rpc.Template.ExportAll.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Template.ExportAll.Response.Error.Code](#anytype.Rpc.Template.ExportAll.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Unsplash"></a>

### Rpc.Unsplash







<a name="anytype.Rpc.Unsplash.Download"></a>

### Rpc.Unsplash.Download







<a name="anytype.Rpc.Unsplash.Download.Request"></a>

### Rpc.Unsplash.Download.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pictureId | [string](#string) |  |  |






<a name="anytype.Rpc.Unsplash.Download.Response"></a>

### Rpc.Unsplash.Download.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Unsplash.Download.Response.Error](#anytype.Rpc.Unsplash.Download.Response.Error) |  |  |
| hash | [string](#string) |  |  |






<a name="anytype.Rpc.Unsplash.Download.Response.Error"></a>

### Rpc.Unsplash.Download.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Unsplash.Download.Response.Error.Code](#anytype.Rpc.Unsplash.Download.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Unsplash.Search"></a>

### Rpc.Unsplash.Search







<a name="anytype.Rpc.Unsplash.Search.Request"></a>

### Rpc.Unsplash.Search.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| query | [string](#string) |  | empty means random images |
| limit | [int32](#int32) |  | may be omitted if the request was cached previously with another limit |






<a name="anytype.Rpc.Unsplash.Search.Response"></a>

### Rpc.Unsplash.Search.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Unsplash.Search.Response.Error](#anytype.Rpc.Unsplash.Search.Response.Error) |  |  |
| pictures | [Rpc.Unsplash.Search.Response.Picture](#anytype.Rpc.Unsplash.Search.Response.Picture) | repeated |  |






<a name="anytype.Rpc.Unsplash.Search.Response.Error"></a>

### Rpc.Unsplash.Search.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Unsplash.Search.Response.Error.Code](#anytype.Rpc.Unsplash.Search.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Unsplash.Search.Response.Picture"></a>

### Rpc.Unsplash.Search.Response.Picture



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [string](#string) |  |  |
| artist | [string](#string) |  |  |
| artistUrl | [string](#string) |  |  |






<a name="anytype.Rpc.Wallet"></a>

### Rpc.Wallet







<a name="anytype.Rpc.Wallet.Convert"></a>

### Rpc.Wallet.Convert







<a name="anytype.Rpc.Wallet.Convert.Request"></a>

### Rpc.Wallet.Convert.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| mnemonic | [string](#string) |  | Mnemonic of a wallet to convert |
| entropy | [string](#string) |  | entropy of a wallet to convert |






<a name="anytype.Rpc.Wallet.Convert.Response"></a>

### Rpc.Wallet.Convert.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Convert.Response.Error](#anytype.Rpc.Wallet.Convert.Response.Error) |  | Error while trying to recover a wallet |
| entropy | [string](#string) |  |  |
| mnemonic | [string](#string) |  |  |






<a name="anytype.Rpc.Wallet.Convert.Response.Error"></a>

### Rpc.Wallet.Convert.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Convert.Response.Error.Code](#anytype.Rpc.Wallet.Convert.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






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






<a name="anytype.Rpc.Workspace"></a>

### Rpc.Workspace







<a name="anytype.Rpc.Workspace.Create"></a>

### Rpc.Workspace.Create







<a name="anytype.Rpc.Workspace.Create.Request"></a>

### Rpc.Workspace.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.Create.Response"></a>

### Rpc.Workspace.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Create.Response.Error](#anytype.Rpc.Workspace.Create.Response.Error) |  |  |
| workspaceId | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.Create.Response.Error"></a>

### Rpc.Workspace.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Create.Response.Error.Code](#anytype.Rpc.Workspace.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.Export"></a>

### Rpc.Workspace.Export







<a name="anytype.Rpc.Workspace.Export.Request"></a>

### Rpc.Workspace.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| workspaceId | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.Export.Response"></a>

### Rpc.Workspace.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Export.Response.Error](#anytype.Rpc.Workspace.Export.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Workspace.Export.Response.Error"></a>

### Rpc.Workspace.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Export.Response.Error.Code](#anytype.Rpc.Workspace.Export.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.GetAll"></a>

### Rpc.Workspace.GetAll







<a name="anytype.Rpc.Workspace.GetAll.Request"></a>

### Rpc.Workspace.GetAll.Request







<a name="anytype.Rpc.Workspace.GetAll.Response"></a>

### Rpc.Workspace.GetAll.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.GetAll.Response.Error](#anytype.Rpc.Workspace.GetAll.Response.Error) |  |  |
| workspaceIds | [string](#string) | repeated |  |






<a name="anytype.Rpc.Workspace.GetAll.Response.Error"></a>

### Rpc.Workspace.GetAll.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.GetAll.Response.Error.Code](#anytype.Rpc.Workspace.GetAll.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.GetCurrent"></a>

### Rpc.Workspace.GetCurrent







<a name="anytype.Rpc.Workspace.GetCurrent.Request"></a>

### Rpc.Workspace.GetCurrent.Request







<a name="anytype.Rpc.Workspace.GetCurrent.Response"></a>

### Rpc.Workspace.GetCurrent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.GetCurrent.Response.Error](#anytype.Rpc.Workspace.GetCurrent.Response.Error) |  |  |
| workspaceId | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.GetCurrent.Response.Error"></a>

### Rpc.Workspace.GetCurrent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.GetCurrent.Response.Error.Code](#anytype.Rpc.Workspace.GetCurrent.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.Select"></a>

### Rpc.Workspace.Select







<a name="anytype.Rpc.Workspace.Select.Request"></a>

### Rpc.Workspace.Select.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| workspaceId | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.Select.Response"></a>

### Rpc.Workspace.Select.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Select.Response.Error](#anytype.Rpc.Workspace.Select.Response.Error) |  |  |






<a name="anytype.Rpc.Workspace.Select.Response.Error"></a>

### Rpc.Workspace.Select.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Select.Response.Error.Code](#anytype.Rpc.Workspace.Select.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Workspace.SetIsHighlighted"></a>

### Rpc.Workspace.SetIsHighlighted







<a name="anytype.Rpc.Workspace.SetIsHighlighted.Request"></a>

### Rpc.Workspace.SetIsHighlighted.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| isHighlighted | [bool](#bool) |  |  |






<a name="anytype.Rpc.Workspace.SetIsHighlighted.Response"></a>

### Rpc.Workspace.SetIsHighlighted.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.SetIsHighlighted.Response.Error](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error) |  |  |






<a name="anytype.Rpc.Workspace.SetIsHighlighted.Response.Error"></a>

### Rpc.Workspace.SetIsHighlighted.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.SetIsHighlighted.Response.Error.Code](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error.Code) |  |  |
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
| FAILED_TO_STOP_RUNNING_NODE | 104 |  |
| BAD_INVITE_CODE | 900 |  |
| NET_ERROR | 901 | means general network error |
| NET_CONNECTION_REFUSED | 902 | means we wasn&#39;t able to connect to the cafe server |
| NET_OFFLINE | 903 | client can additionally support this error code to notify user that device is offline |



<a name="anytype.Rpc.Account.Delete.Response.Error.Code"></a>

### Rpc.Account.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_ALREADY_DELETED | 101 |  |
| ACCOUNT_IS_ACTIVE | 102 |  |



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
| FAILED_TO_STOP_RUNNING_NODE | 107 |  |
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |
| ACCOUNT_IS_DELETED | 109 |  |



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
| FAILED_TO_STOP_SEARCHER_NODE | 106 |  |
| FAILED_TO_RECOVER_PREDEFINED_BLOCKS | 107 |  |
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |



<a name="anytype.Rpc.Account.Stop.Response.Error.Code"></a>

### Rpc.Account.Stop.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | Id or root path is wrong |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| FAILED_TO_STOP_NODE | 102 |  |
| FAILED_TO_REMOVE_ACCOUNT_DATA | 103 |  |



<a name="anytype.Rpc.App.GetVersion.Response.Error.Code"></a>

### Rpc.App.GetVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| VERSION_IS_EMPTY | 3 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.App.Shutdown.Response.Error.Code"></a>

### Rpc.App.Shutdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |



<a name="anytype.Rpc.Block.Copy.Response.Error.Code"></a>

### Rpc.Block.Copy.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Create.Response.Error.Code"></a>

### Rpc.Block.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Cut.Response.Error.Code"></a>

### Rpc.Block.Cut.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Download.Response.Error.Code"></a>

### Rpc.Block.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Export.Response.Error.Code"></a>

### Rpc.Block.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListConvertToObjects.Response.Error.Code"></a>

### Rpc.Block.ListConvertToObjects.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListDuplicate.Response.Error.Code"></a>

### Rpc.Block.ListDuplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListMoveToExistingObject.Response.Error.Code"></a>

### Rpc.Block.ListMoveToExistingObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListMoveToNewObject.Response.Error.Code"></a>

### Rpc.Block.ListMoveToNewObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListSetAlign.Response.Error.Code"></a>

### Rpc.Block.ListSetAlign.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListSetBackgroundColor.Response.Error.Code"></a>

### Rpc.Block.ListSetBackgroundColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListSetFields.Response.Error.Code"></a>

### Rpc.Block.ListSetFields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ListTurnInto.Response.Error.Code"></a>

### Rpc.Block.ListTurnInto.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Merge.Response.Error.Code"></a>

### Rpc.Block.Merge.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Paste.Response.Error.Code"></a>

### Rpc.Block.Paste.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Replace.Response.Error.Code"></a>

### Rpc.Block.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.SetFields.Response.Error.Code"></a>

### Rpc.Block.SetFields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.SetRestrictions.Response.Error.Code"></a>

### Rpc.Block.SetRestrictions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Split.Request.Mode"></a>

### Rpc.Block.Split.Request.Mode


| Name | Number | Description |
| ---- | ------ | ----------- |
| BOTTOM | 0 | new block will be created under existing |
| TOP | 1 | new block will be created above existing |
| INNER | 2 | new block will be created as the first children of existing |
| TITLE | 3 | new block will be created after header (not required for set at client side, will auto set for title block) |



<a name="anytype.Rpc.Block.Split.Response.Error.Code"></a>

### Rpc.Block.Split.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Unlink.Response.Error.Code"></a>

### Rpc.Block.Unlink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Upload.Response.Error.Code"></a>

### Rpc.Block.Upload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code"></a>

### Rpc.BlockBookmark.CreateAndFetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockBookmark.Fetch.Response.Error.Code"></a>

### Rpc.BlockBookmark.Fetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataview.Relation.Add.Response.Error.Code"></a>

### Rpc.BlockDataview.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataview.Relation.Delete.Response.Error.Code"></a>

### Rpc.BlockDataview.Relation.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code"></a>

### Rpc.BlockDataview.Relation.ListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_A_DATAVIEW_BLOCK | 3 | ... |



<a name="anytype.Rpc.BlockDataview.Relation.Update.Response.Error.Code"></a>

### Rpc.BlockDataview.Relation.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataview.SetSource.Response.Error.Code"></a>

### Rpc.BlockDataview.SetSource.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataview.View.Create.Response.Error.Code"></a>

### Rpc.BlockDataview.View.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataview.View.Delete.Response.Error.Code"></a>

### Rpc.BlockDataview.View.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataview.View.SetActive.Response.Error.Code"></a>

### Rpc.BlockDataview.View.SetActive.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataview.View.SetPosition.Response.Error.Code"></a>

### Rpc.BlockDataview.View.SetPosition.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataview.View.Update.Response.Error.Code"></a>

### Rpc.BlockDataview.View.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataviewRecord.AddRelationOption.Response.Error.Code"></a>

### Rpc.BlockDataviewRecord.AddRelationOption.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataviewRecord.Create.Response.Error.Code"></a>

### Rpc.BlockDataviewRecord.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataviewRecord.Delete.Response.Error.Code"></a>

### Rpc.BlockDataviewRecord.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error.Code"></a>

### Rpc.BlockDataviewRecord.DeleteRelationOption.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDataviewRecord.Update.Response.Error.Code"></a>

### Rpc.BlockDataviewRecord.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error.Code"></a>

### Rpc.BlockDataviewRecord.UpdateRelationOption.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockDiv.ListSetStyle.Response.Error.Code"></a>

### Rpc.BlockDiv.ListSetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockFile.CreateAndUpload.Response.Error.Code"></a>

### Rpc.BlockFile.CreateAndUpload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockFile.ListSetStyle.Response.Error.Code"></a>

### Rpc.BlockFile.ListSetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockFile.SetName.Response.Error.Code"></a>

### Rpc.BlockFile.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockImage.SetName.Response.Error.Code"></a>

### Rpc.BlockImage.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockImage.SetWidth.Response.Error.Code"></a>

### Rpc.BlockImage.SetWidth.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockLatex.SetText.Response.Error.Code"></a>

### Rpc.BlockLatex.SetText.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockLink.CreateLinkToNewObject.Response.Error.Code"></a>

### Rpc.BlockLink.CreateLinkToNewObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockLink.CreateLinkToNewSet.Response.Error.Code"></a>

### Rpc.BlockLink.CreateLinkToNewSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 | ... |



<a name="anytype.Rpc.BlockLink.SetTargetBlockId.Response.Error.Code"></a>

### Rpc.BlockLink.SetTargetBlockId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockRelation.Add.Response.Error.Code"></a>

### Rpc.BlockRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockRelation.SetKey.Response.Error.Code"></a>

### Rpc.BlockRelation.SetKey.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.BlockText.ListSetColor.Response.Error.Code"></a>

### Rpc.BlockText.ListSetColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.ListSetMark.Response.Error.Code"></a>

### Rpc.BlockText.ListSetMark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.ListSetStyle.Response.Error.Code"></a>

### Rpc.BlockText.ListSetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.SetChecked.Response.Error.Code"></a>

### Rpc.BlockText.SetChecked.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.SetColor.Response.Error.Code"></a>

### Rpc.BlockText.SetColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.SetIcon.Response.Error.Code"></a>

### Rpc.BlockText.SetIcon.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.SetMarks.Get.Response.Error.Code"></a>

### Rpc.BlockText.SetMarks.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.SetStyle.Response.Error.Code"></a>

### Rpc.BlockText.SetStyle.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockText.SetText.Response.Error.Code"></a>

### Rpc.BlockText.SetText.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockVideo.SetName.Response.Error.Code"></a>

### Rpc.BlockVideo.SetName.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockVideo.SetWidth.Response.Error.Code"></a>

### Rpc.BlockVideo.SetWidth.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Debug.ExportLocalstore.Response.Error.Code"></a>

### Rpc.Debug.ExportLocalstore.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Debug.Ping.Response.Error.Code"></a>

### Rpc.Debug.Ping.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Debug.Sync.Response.Error.Code"></a>

### Rpc.Debug.Sync.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Debug.Thread.Response.Error.Code"></a>

### Rpc.Debug.Thread.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Debug.Tree.Response.Error.Code"></a>

### Rpc.Debug.Tree.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.File.Download.Response.Error.Code"></a>

### Rpc.File.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |



<a name="anytype.Rpc.File.Drop.Response.Error.Code"></a>

### Rpc.File.Drop.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.File.ListOffload.Response.Error.Code"></a>

### Rpc.File.ListOffload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |



<a name="anytype.Rpc.File.Offload.Response.Error.Code"></a>

### Rpc.File.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |
| FILE_NOT_YET_PINNED | 104 |  |



<a name="anytype.Rpc.File.Upload.Response.Error.Code"></a>

### Rpc.File.Upload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.GenericErrorResponse.Error.Code"></a>

### Rpc.GenericErrorResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.History.GetVersions.Response.Error.Code"></a>

### Rpc.History.GetVersions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.History.SetVersion.Response.Error.Code"></a>

### Rpc.History.SetVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.History.ShowVersion.Response.Error.Code"></a>

### Rpc.History.ShowVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.LinkPreview.Response.Error.Code"></a>

### Rpc.LinkPreview.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype.Rpc.Metrics.SetParameters.Response.Error.Code"></a>

### Rpc.Metrics.SetParameters.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Navigation.Context"></a>

### Rpc.Navigation.Context


| Name | Number | Description |
| ---- | ------ | ----------- |
| Navigation | 0 |  |
| MoveTo | 1 | do not show sets/archive |
| LinkTo | 2 | same for mention, do not show sets/archive |



<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code"></a>

### Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Navigation.ListObjects.Response.Error.Code"></a>

### Rpc.Navigation.ListObjects.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.AddWithObjectId.Response.Error.Code"></a>

### Rpc.Object.AddWithObjectId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.ApplyTemplate.Response.Error.Code"></a>

### Rpc.Object.ApplyTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Close.Response.Error.Code"></a>

### Rpc.Object.Close.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Create.Response.Error.Code"></a>

### Rpc.Object.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.CreateSet.Response.Error.Code"></a>

### Rpc.Object.CreateSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype.Rpc.Object.Duplicate.Response.Error.Code"></a>

### Rpc.Object.Duplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Export.Format"></a>

### Rpc.Object.Export.Format


| Name | Number | Description |
| ---- | ------ | ----------- |
| Markdown | 0 |  |
| Protobuf | 1 |  |
| JSON | 2 |  |
| DOT | 3 |  |
| SVG | 4 |  |
| GRAPH_JSON | 5 |  |



<a name="anytype.Rpc.Object.Export.Response.Error.Code"></a>

### Rpc.Object.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Graph.Edge.Type"></a>

### Rpc.Object.Graph.Edge.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Link | 0 |  |
| Relation | 1 |  |



<a name="anytype.Rpc.Object.Graph.Response.Error.Code"></a>

### Rpc.Object.Graph.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.ImportMarkdown.Response.Error.Code"></a>

### Rpc.Object.ImportMarkdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.ListDelete.Response.Error.Code"></a>

### Rpc.Object.ListDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.ListSetIsArchived.Response.Error.Code"></a>

### Rpc.Object.ListSetIsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.ListSetIsFavorite.Response.Error.Code"></a>

### Rpc.Object.ListSetIsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Open.Response.Error.Code"></a>

### Rpc.Object.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



<a name="anytype.Rpc.Object.OpenBreadcrumbs.Response.Error.Code"></a>

### Rpc.Object.OpenBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Redo.Response.Error.Code"></a>

### Rpc.Object.Redo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype.Rpc.Object.Search.Response.Error.Code"></a>

### Rpc.Object.Search.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SearchSubscribe.Response.Error.Code"></a>

### Rpc.Object.SearchSubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SearchUnsubscribe.Response.Error.Code"></a>

### Rpc.Object.SearchUnsubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Object.SetBreadcrumbs.Response.Error.Code"></a>

### Rpc.Object.SetBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SetDetails.Response.Error.Code"></a>

### Rpc.Object.SetDetails.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SetIsArchived.Response.Error.Code"></a>

### Rpc.Object.SetIsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SetIsFavorite.Response.Error.Code"></a>

### Rpc.Object.SetIsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SetLayout.Response.Error.Code"></a>

### Rpc.Object.SetLayout.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.SetObjectType.Response.Error.Code"></a>

### Rpc.Object.SetObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype.Rpc.Object.ShareByLink.Response.Error.Code"></a>

### Rpc.Object.ShareByLink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Show.Response.Error.Code"></a>

### Rpc.Object.Show.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



<a name="anytype.Rpc.Object.SubscribeIds.Response.Error.Code"></a>

### Rpc.Object.SubscribeIds.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.ToSet.Response.Error.Code"></a>

### Rpc.Object.ToSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.Undo.Response.Error.Code"></a>

### Rpc.Object.Undo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype.Rpc.ObjectRelation.Add.Response.Error.Code"></a>

### Rpc.ObjectRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.ObjectRelation.AddFeatured.Response.Error.Code"></a>

### Rpc.ObjectRelation.AddFeatured.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectRelation.Delete.Response.Error.Code"></a>

### Rpc.ObjectRelation.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.ObjectRelation.ListAvailable.Response.Error.Code"></a>

### Rpc.ObjectRelation.ListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code"></a>

### Rpc.ObjectRelation.RemoveFeatured.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectRelation.Update.Response.Error.Code"></a>

### Rpc.ObjectRelation.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.ObjectRelationOption.Add.Response.Error.Code"></a>

### Rpc.ObjectRelationOption.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.ObjectRelationOption.Delete.Response.Error.Code"></a>

### Rpc.ObjectRelationOption.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| SOME_RECORDS_HAS_RELATION_VALUE_WITH_THIS_OPTION | 3 | need to confirm with confirmRemoveAllValuesInRecords=true |



<a name="anytype.Rpc.ObjectRelationOption.Update.Response.Error.Code"></a>

### Rpc.ObjectRelationOption.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.ObjectType.Create.Response.Error.Code"></a>

### Rpc.ObjectType.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 | ... |



<a name="anytype.Rpc.ObjectType.List.Response.Error.Code"></a>

### Rpc.ObjectType.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code"></a>

### Rpc.ObjectType.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 | ... |



<a name="anytype.Rpc.ObjectType.Relation.List.Response.Error.Code"></a>

### Rpc.ObjectType.Relation.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 | ... |



<a name="anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code"></a>

### Rpc.ObjectType.Relation.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 | ... |



<a name="anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code"></a>

### Rpc.ObjectType.Relation.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 | ... |



<a name="anytype.Rpc.Process.Cancel.Response.Error.Code"></a>

### Rpc.Process.Cancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Template.Clone.Response.Error.Code"></a>

### Rpc.Template.Clone.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Template.CreateFromObject.Response.Error.Code"></a>

### Rpc.Template.CreateFromObject.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Template.CreateFromObjectType.Response.Error.Code"></a>

### Rpc.Template.CreateFromObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Template.ExportAll.Response.Error.Code"></a>

### Rpc.Template.ExportAll.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Unsplash.Download.Response.Error.Code"></a>

### Rpc.Unsplash.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 | ... |



<a name="anytype.Rpc.Unsplash.Search.Response.Error.Code"></a>

### Rpc.Unsplash.Search.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 | ... |



<a name="anytype.Rpc.Wallet.Convert.Response.Error.Code"></a>

### Rpc.Wallet.Convert.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; wallet successfully recovered |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 | mnemonic is wrong |



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



<a name="anytype.Rpc.Workspace.Create.Response.Error.Code"></a>

### Rpc.Workspace.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Workspace.Export.Response.Error.Code"></a>

### Rpc.Workspace.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Workspace.GetAll.Response.Error.Code"></a>

### Rpc.Workspace.GetAll.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Workspace.GetCurrent.Response.Error.Code"></a>

### Rpc.Workspace.GetCurrent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Workspace.Select.Response.Error.Code"></a>

### Rpc.Workspace.Select.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Workspace.SetIsHighlighted.Response.Error.Code"></a>

### Rpc.Workspace.SetIsHighlighted.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |


 

 

 



<a name="pb/protos/events.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/events.proto



<a name="anytype.Event"></a>

### Event
Event – type of message, that could be sent from a middleware to the corresponding front-end.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype.Event.Message) | repeated |  |
| contextId | [string](#string) |  |  |
| initiator | [model.Account](#anytype.model.Account) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Event.Account"></a>

### Event.Account







<a name="anytype.Event.Account.Config"></a>

### Event.Account.Config







<a name="anytype.Event.Account.Config.Update"></a>

### Event.Account.Config.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [model.Account.Config](#anytype.model.Account.Config) |  |  |
| status | [model.Account.Status](#anytype.model.Account.Status) |  |  |






<a name="anytype.Event.Account.Details"></a>

### Event.Account.Details



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| profileId | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Account.Show"></a>

### Event.Account.Show
Message, that will be sent to the front on each account found after an AccountRecoverRequest


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  | Number of an account in an all found accounts list |
| account | [model.Account](#anytype.model.Account) |  | An Account, that has been found for the mnemonic |






<a name="anytype.Event.Account.Update"></a>

### Event.Account.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [model.Account.Config](#anytype.model.Account.Config) |  |  |
| status | [model.Account.Status](#anytype.model.Account.Status) |  |  |






<a name="anytype.Event.Block"></a>

### Event.Block







<a name="anytype.Event.Block.Add"></a>

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
| blocks | [model.Block](#anytype.model.Block) | repeated | id -&gt; block |






<a name="anytype.Event.Block.Dataview"></a>

### Event.Block.Dataview







<a name="anytype.Event.Block.Dataview.RecordsDelete"></a>

### Event.Block.Dataview.RecordsDelete
sent when client should remove existing records on the active view


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id, client should double check this to make sure client doesn&#39;t switch the active view in the middle |
| removed | [string](#string) | repeated |  |






<a name="anytype.Event.Block.Dataview.RecordsInsert"></a>

### Event.Block.Dataview.RecordsInsert
sent when client should insert new records on the active view


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id, client should double check this to make sure client doesn&#39;t switch the active view in the middle |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| insertPosition | [uint32](#uint32) |  | position to insert |






<a name="anytype.Event.Block.Dataview.RecordsSet"></a>

### Event.Block.Dataview.RecordsSet
sent when the active view&#39;s visible records should be replaced


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id, client should double check this to make sure client doesn&#39;t switch the active view in the middle |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| total | [uint32](#uint32) |  | total number of records |






<a name="anytype.Event.Block.Dataview.RecordsUpdate"></a>

### Event.Block.Dataview.RecordsUpdate
sent when client should update existing records on the active view


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id, client should double check this to make sure client doesn&#39;t switch the active view in the middle |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated | records to update. Use &#39;id&#39; field to get records ids |






<a name="anytype.Event.Block.Dataview.RelationDelete"></a>

### Event.Block.Dataview.RelationDelete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| relationKey | [string](#string) |  | relation key to remove |






<a name="anytype.Event.Block.Dataview.RelationSet"></a>

### Event.Block.Dataview.RelationSet
sent when the dataview relation has been changed or added


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| relationKey | [string](#string) |  | relation key to update |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Event.Block.Dataview.SourceSet"></a>

### Event.Block.Dataview.SourceSet



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| source | [string](#string) | repeated |  |






<a name="anytype.Event.Block.Dataview.ViewDelete"></a>

### Event.Block.Dataview.ViewDelete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id to remove |






<a name="anytype.Event.Block.Dataview.ViewOrder"></a>

### Event.Block.Dataview.ViewOrder



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewIds | [string](#string) | repeated | view ids in new order |






<a name="anytype.Event.Block.Dataview.ViewSet"></a>

### Event.Block.Dataview.ViewSet
sent when the view have been changed or added


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | dataview block&#39;s id |
| viewId | [string](#string) |  | view id, client should double check this to make sure client doesn&#39;t switch the active view in the middle |
| view | [model.Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) |  |  |
| offset | [uint32](#uint32) |  | middleware will try to preserve the current aciveview&#39;s offset&amp;limit but may reset it in case it becomes invalid or not actual anymore |
| limit | [uint32](#uint32) |  |  |






<a name="anytype.Event.Block.Delete"></a>

### Event.Block.Delete



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockIds | [string](#string) | repeated |  |






<a name="anytype.Event.Block.FilesUpload"></a>

### Event.Block.FilesUpload
Middleware to front end event message, that will be sent on one of this scenarios:
Precondition: user A opened a block
1. User A drops a set of files/pictures/videos
2. User A creates a MediaBlock and drops a single media, that corresponds to its type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  | if empty =&gt; create new blocks |
| filePath | [string](#string) | repeated | filepaths to the files |






<a name="anytype.Event.Block.Fill"></a>

### Event.Block.Fill







<a name="anytype.Event.Block.Fill.Align"></a>

### Event.Block.Fill.Align



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| align | [model.Block.Align](#anytype.model.Block.Align) |  |  |






<a name="anytype.Event.Block.Fill.BackgroundColor"></a>

### Event.Block.Fill.BackgroundColor



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| backgroundColor | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark"></a>

### Event.Block.Fill.Bookmark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [Event.Block.Fill.Bookmark.Url](#anytype.Event.Block.Fill.Bookmark.Url) |  |  |
| title | [Event.Block.Fill.Bookmark.Title](#anytype.Event.Block.Fill.Bookmark.Title) |  |  |
| description | [Event.Block.Fill.Bookmark.Description](#anytype.Event.Block.Fill.Bookmark.Description) |  |  |
| imageHash | [Event.Block.Fill.Bookmark.ImageHash](#anytype.Event.Block.Fill.Bookmark.ImageHash) |  |  |
| faviconHash | [Event.Block.Fill.Bookmark.FaviconHash](#anytype.Event.Block.Fill.Bookmark.FaviconHash) |  |  |
| type | [Event.Block.Fill.Bookmark.Type](#anytype.Event.Block.Fill.Bookmark.Type) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark.Description"></a>

### Event.Block.Fill.Bookmark.Description



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark.FaviconHash"></a>

### Event.Block.Fill.Bookmark.FaviconHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark.ImageHash"></a>

### Event.Block.Fill.Bookmark.ImageHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark.Title"></a>

### Event.Block.Fill.Bookmark.Title



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark.Type"></a>

### Event.Block.Fill.Bookmark.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.LinkPreview.Type](#anytype.model.LinkPreview.Type) |  |  |






<a name="anytype.Event.Block.Fill.Bookmark.Url"></a>

### Event.Block.Fill.Bookmark.Url



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.ChildrenIds"></a>

### Event.Block.Fill.ChildrenIds



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| childrenIds | [string](#string) | repeated |  |






<a name="anytype.Event.Block.Fill.DatabaseRecords"></a>

### Event.Block.Fill.DatabaseRecords



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |






<a name="anytype.Event.Block.Fill.Details"></a>

### Event.Block.Fill.Details



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Block.Fill.Div"></a>

### Event.Block.Fill.Div



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| style | [Event.Block.Fill.Div.Style](#anytype.Event.Block.Fill.Div.Style) |  |  |






<a name="anytype.Event.Block.Fill.Div.Style"></a>

### Event.Block.Fill.Div.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |






<a name="anytype.Event.Block.Fill.Fields"></a>

### Event.Block.Fill.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Block.Fill.File"></a>

### Event.Block.Fill.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Event.Block.Fill.File.Type](#anytype.Event.Block.Fill.File.Type) |  |  |
| state | [Event.Block.Fill.File.State](#anytype.Event.Block.Fill.File.State) |  |  |
| mime | [Event.Block.Fill.File.Mime](#anytype.Event.Block.Fill.File.Mime) |  |  |
| hash | [Event.Block.Fill.File.Hash](#anytype.Event.Block.Fill.File.Hash) |  |  |
| name | [Event.Block.Fill.File.Name](#anytype.Event.Block.Fill.File.Name) |  |  |
| size | [Event.Block.Fill.File.Size](#anytype.Event.Block.Fill.File.Size) |  |  |
| style | [Event.Block.Fill.File.Style](#anytype.Event.Block.Fill.File.Style) |  |  |






<a name="anytype.Event.Block.Fill.File.Hash"></a>

### Event.Block.Fill.File.Hash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.File.Mime"></a>

### Event.Block.Fill.File.Mime



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.File.Name"></a>

### Event.Block.Fill.File.Name



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.File.Size"></a>

### Event.Block.Fill.File.Size



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int64](#int64) |  |  |






<a name="anytype.Event.Block.Fill.File.State"></a>

### Event.Block.Fill.File.State



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.State](#anytype.model.Block.Content.File.State) |  |  |






<a name="anytype.Event.Block.Fill.File.Style"></a>

### Event.Block.Fill.File.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Event.Block.Fill.File.Type"></a>

### Event.Block.Fill.File.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |






<a name="anytype.Event.Block.Fill.File.Width"></a>

### Event.Block.Fill.File.Width



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) |  |  |






<a name="anytype.Event.Block.Fill.Link"></a>

### Event.Block.Fill.Link



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| targetBlockId | [Event.Block.Fill.Link.TargetBlockId](#anytype.Event.Block.Fill.Link.TargetBlockId) |  |  |
| style | [Event.Block.Fill.Link.Style](#anytype.Event.Block.Fill.Link.Style) |  |  |
| fields | [Event.Block.Fill.Link.Fields](#anytype.Event.Block.Fill.Link.Fields) |  |  |






<a name="anytype.Event.Block.Fill.Link.Fields"></a>

### Event.Block.Fill.Link.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Block.Fill.Link.Style"></a>

### Event.Block.Fill.Link.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) |  |  |






<a name="anytype.Event.Block.Fill.Link.TargetBlockId"></a>

### Event.Block.Fill.Link.TargetBlockId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Restrictions"></a>

### Event.Block.Fill.Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| restrictions | [model.Block.Restrictions](#anytype.model.Block.Restrictions) |  |  |






<a name="anytype.Event.Block.Fill.Text"></a>

### Event.Block.Fill.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [Event.Block.Fill.Text.Text](#anytype.Event.Block.Fill.Text.Text) |  |  |
| style | [Event.Block.Fill.Text.Style](#anytype.Event.Block.Fill.Text.Style) |  |  |
| marks | [Event.Block.Fill.Text.Marks](#anytype.Event.Block.Fill.Text.Marks) |  |  |
| checked | [Event.Block.Fill.Text.Checked](#anytype.Event.Block.Fill.Text.Checked) |  |  |
| color | [Event.Block.Fill.Text.Color](#anytype.Event.Block.Fill.Text.Color) |  |  |






<a name="anytype.Event.Block.Fill.Text.Checked"></a>

### Event.Block.Fill.Text.Checked



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) |  |  |






<a name="anytype.Event.Block.Fill.Text.Color"></a>

### Event.Block.Fill.Text.Color



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Fill.Text.Marks"></a>

### Event.Block.Fill.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) |  |  |






<a name="anytype.Event.Block.Fill.Text.Style"></a>

### Event.Block.Fill.Text.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Event.Block.Fill.Text.Text"></a>

### Event.Block.Fill.Text.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.MarksInfo"></a>

### Event.Block.MarksInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marksInRange | [model.Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type) | repeated |  |






<a name="anytype.Event.Block.Set"></a>

### Event.Block.Set







<a name="anytype.Event.Block.Set.Align"></a>

### Event.Block.Set.Align



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| align | [model.Block.Align](#anytype.model.Block.Align) |  |  |






<a name="anytype.Event.Block.Set.BackgroundColor"></a>

### Event.Block.Set.BackgroundColor



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| backgroundColor | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Bookmark"></a>

### Event.Block.Set.Bookmark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [Event.Block.Set.Bookmark.Url](#anytype.Event.Block.Set.Bookmark.Url) |  |  |
| title | [Event.Block.Set.Bookmark.Title](#anytype.Event.Block.Set.Bookmark.Title) |  |  |
| description | [Event.Block.Set.Bookmark.Description](#anytype.Event.Block.Set.Bookmark.Description) |  |  |
| imageHash | [Event.Block.Set.Bookmark.ImageHash](#anytype.Event.Block.Set.Bookmark.ImageHash) |  |  |
| faviconHash | [Event.Block.Set.Bookmark.FaviconHash](#anytype.Event.Block.Set.Bookmark.FaviconHash) |  |  |
| type | [Event.Block.Set.Bookmark.Type](#anytype.Event.Block.Set.Bookmark.Type) |  |  |






<a name="anytype.Event.Block.Set.Bookmark.Description"></a>

### Event.Block.Set.Bookmark.Description



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Bookmark.FaviconHash"></a>

### Event.Block.Set.Bookmark.FaviconHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Bookmark.ImageHash"></a>

### Event.Block.Set.Bookmark.ImageHash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Bookmark.Title"></a>

### Event.Block.Set.Bookmark.Title



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Bookmark.Type"></a>

### Event.Block.Set.Bookmark.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.LinkPreview.Type](#anytype.model.LinkPreview.Type) |  |  |






<a name="anytype.Event.Block.Set.Bookmark.Url"></a>

### Event.Block.Set.Bookmark.Url



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.ChildrenIds"></a>

### Event.Block.Set.ChildrenIds



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| childrenIds | [string](#string) | repeated |  |






<a name="anytype.Event.Block.Set.Div"></a>

### Event.Block.Set.Div



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| style | [Event.Block.Set.Div.Style](#anytype.Event.Block.Set.Div.Style) |  |  |






<a name="anytype.Event.Block.Set.Div.Style"></a>

### Event.Block.Set.Div.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |






<a name="anytype.Event.Block.Set.Fields"></a>

### Event.Block.Set.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Block.Set.File"></a>

### Event.Block.Set.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Event.Block.Set.File.Type](#anytype.Event.Block.Set.File.Type) |  |  |
| state | [Event.Block.Set.File.State](#anytype.Event.Block.Set.File.State) |  |  |
| mime | [Event.Block.Set.File.Mime](#anytype.Event.Block.Set.File.Mime) |  |  |
| hash | [Event.Block.Set.File.Hash](#anytype.Event.Block.Set.File.Hash) |  |  |
| name | [Event.Block.Set.File.Name](#anytype.Event.Block.Set.File.Name) |  |  |
| size | [Event.Block.Set.File.Size](#anytype.Event.Block.Set.File.Size) |  |  |
| style | [Event.Block.Set.File.Style](#anytype.Event.Block.Set.File.Style) |  |  |






<a name="anytype.Event.Block.Set.File.Hash"></a>

### Event.Block.Set.File.Hash



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.File.Mime"></a>

### Event.Block.Set.File.Mime



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.File.Name"></a>

### Event.Block.Set.File.Name



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.File.Size"></a>

### Event.Block.Set.File.Size



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int64](#int64) |  |  |






<a name="anytype.Event.Block.Set.File.State"></a>

### Event.Block.Set.File.State



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.State](#anytype.model.Block.Content.File.State) |  |  |






<a name="anytype.Event.Block.Set.File.Style"></a>

### Event.Block.Set.File.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Event.Block.Set.File.Type"></a>

### Event.Block.Set.File.Type



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |






<a name="anytype.Event.Block.Set.File.Width"></a>

### Event.Block.Set.File.Width



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) |  |  |






<a name="anytype.Event.Block.Set.Latex"></a>

### Event.Block.Set.Latex



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [Event.Block.Set.Latex.Text](#anytype.Event.Block.Set.Latex.Text) |  |  |






<a name="anytype.Event.Block.Set.Latex.Text"></a>

### Event.Block.Set.Latex.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Link"></a>

### Event.Block.Set.Link



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| targetBlockId | [Event.Block.Set.Link.TargetBlockId](#anytype.Event.Block.Set.Link.TargetBlockId) |  |  |
| style | [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style) |  |  |
| fields | [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields) |  |  |






<a name="anytype.Event.Block.Set.Link.Fields"></a>

### Event.Block.Set.Link.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Block.Set.Link.Style"></a>

### Event.Block.Set.Link.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) |  |  |






<a name="anytype.Event.Block.Set.Link.TargetBlockId"></a>

### Event.Block.Set.Link.TargetBlockId



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Relation"></a>

### Event.Block.Set.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| key | [Event.Block.Set.Relation.Key](#anytype.Event.Block.Set.Relation.Key) |  |  |






<a name="anytype.Event.Block.Set.Relation.Key"></a>

### Event.Block.Set.Relation.Key



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Restrictions"></a>

### Event.Block.Set.Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| restrictions | [model.Block.Restrictions](#anytype.model.Block.Restrictions) |  |  |






<a name="anytype.Event.Block.Set.Text"></a>

### Event.Block.Set.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| text | [Event.Block.Set.Text.Text](#anytype.Event.Block.Set.Text.Text) |  |  |
| style | [Event.Block.Set.Text.Style](#anytype.Event.Block.Set.Text.Style) |  |  |
| marks | [Event.Block.Set.Text.Marks](#anytype.Event.Block.Set.Text.Marks) |  |  |
| checked | [Event.Block.Set.Text.Checked](#anytype.Event.Block.Set.Text.Checked) |  |  |
| color | [Event.Block.Set.Text.Color](#anytype.Event.Block.Set.Text.Color) |  |  |
| iconEmoji | [Event.Block.Set.Text.IconEmoji](#anytype.Event.Block.Set.Text.IconEmoji) |  |  |
| iconImage | [Event.Block.Set.Text.IconImage](#anytype.Event.Block.Set.Text.IconImage) |  |  |






<a name="anytype.Event.Block.Set.Text.Checked"></a>

### Event.Block.Set.Text.Checked



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) |  |  |






<a name="anytype.Event.Block.Set.Text.Color"></a>

### Event.Block.Set.Text.Color



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Text.IconEmoji"></a>

### Event.Block.Set.Text.IconEmoji



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Text.IconImage"></a>

### Event.Block.Set.Text.IconImage



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.Text.Marks"></a>

### Event.Block.Set.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) |  |  |






<a name="anytype.Event.Block.Set.Text.Style"></a>

### Event.Block.Set.Text.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Event.Block.Set.Text.Text"></a>

### Event.Block.Set.Text.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Message"></a>

### Event.Message



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountShow | [Event.Account.Show](#anytype.Event.Account.Show) |  |  |
| accountDetails | [Event.Account.Details](#anytype.Event.Account.Details) |  |  |
| accountConfigUpdate | [Event.Account.Config.Update](#anytype.Event.Account.Config.Update) |  |  |
| accountUpdate | [Event.Account.Update](#anytype.Event.Account.Update) |  |  |
| objectDetailsSet | [Event.Object.Details.Set](#anytype.Event.Object.Details.Set) |  |  |
| objectDetailsAmend | [Event.Object.Details.Amend](#anytype.Event.Object.Details.Amend) |  |  |
| objectDetailsUnset | [Event.Object.Details.Unset](#anytype.Event.Object.Details.Unset) |  |  |
| objectRelationsSet | [Event.Object.Relations.Set](#anytype.Event.Object.Relations.Set) |  |  |
| objectRelationsAmend | [Event.Object.Relations.Amend](#anytype.Event.Object.Relations.Amend) |  |  |
| objectRelationsRemove | [Event.Object.Relations.Remove](#anytype.Event.Object.Relations.Remove) |  |  |
| objectRemove | [Event.Object.Remove](#anytype.Event.Object.Remove) |  |  |
| objectShow | [Event.Object.Show](#anytype.Event.Object.Show) |  |  |
| subscriptionAdd | [Event.Object.Subscription.Add](#anytype.Event.Object.Subscription.Add) |  |  |
| subscriptionRemove | [Event.Object.Subscription.Remove](#anytype.Event.Object.Subscription.Remove) |  |  |
| subscriptionPosition | [Event.Object.Subscription.Position](#anytype.Event.Object.Subscription.Position) |  |  |
| subscriptionCounters | [Event.Object.Subscription.Counters](#anytype.Event.Object.Subscription.Counters) |  |  |
| blockAdd | [Event.Block.Add](#anytype.Event.Block.Add) |  |  |
| blockDelete | [Event.Block.Delete](#anytype.Event.Block.Delete) |  |  |
| filesUpload | [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload) |  |  |
| marksInfo | [Event.Block.MarksInfo](#anytype.Event.Block.MarksInfo) |  |  |
| blockSetFields | [Event.Block.Set.Fields](#anytype.Event.Block.Set.Fields) |  |  |
| blockSetChildrenIds | [Event.Block.Set.ChildrenIds](#anytype.Event.Block.Set.ChildrenIds) |  |  |
| blockSetRestrictions | [Event.Block.Set.Restrictions](#anytype.Event.Block.Set.Restrictions) |  |  |
| blockSetBackgroundColor | [Event.Block.Set.BackgroundColor](#anytype.Event.Block.Set.BackgroundColor) |  |  |
| blockSetText | [Event.Block.Set.Text](#anytype.Event.Block.Set.Text) |  |  |
| blockSetFile | [Event.Block.Set.File](#anytype.Event.Block.Set.File) |  |  |
| blockSetLink | [Event.Block.Set.Link](#anytype.Event.Block.Set.Link) |  |  |
| blockSetBookmark | [Event.Block.Set.Bookmark](#anytype.Event.Block.Set.Bookmark) |  |  |
| blockSetAlign | [Event.Block.Set.Align](#anytype.Event.Block.Set.Align) |  |  |
| blockSetDiv | [Event.Block.Set.Div](#anytype.Event.Block.Set.Div) |  |  |
| blockSetRelation | [Event.Block.Set.Relation](#anytype.Event.Block.Set.Relation) |  |  |
| blockSetLatex | [Event.Block.Set.Latex](#anytype.Event.Block.Set.Latex) |  |  |
| blockDataviewRecordsSet | [Event.Block.Dataview.RecordsSet](#anytype.Event.Block.Dataview.RecordsSet) |  |  |
| blockDataviewRecordsUpdate | [Event.Block.Dataview.RecordsUpdate](#anytype.Event.Block.Dataview.RecordsUpdate) |  |  |
| blockDataviewRecordsInsert | [Event.Block.Dataview.RecordsInsert](#anytype.Event.Block.Dataview.RecordsInsert) |  |  |
| blockDataviewRecordsDelete | [Event.Block.Dataview.RecordsDelete](#anytype.Event.Block.Dataview.RecordsDelete) |  |  |
| blockDataviewSourceSet | [Event.Block.Dataview.SourceSet](#anytype.Event.Block.Dataview.SourceSet) |  |  |
| blockDataviewViewSet | [Event.Block.Dataview.ViewSet](#anytype.Event.Block.Dataview.ViewSet) |  |  |
| blockDataviewViewDelete | [Event.Block.Dataview.ViewDelete](#anytype.Event.Block.Dataview.ViewDelete) |  |  |
| blockDataviewViewOrder | [Event.Block.Dataview.ViewOrder](#anytype.Event.Block.Dataview.ViewOrder) |  |  |
| blockDataviewRelationDelete | [Event.Block.Dataview.RelationDelete](#anytype.Event.Block.Dataview.RelationDelete) |  |  |
| blockDataviewRelationSet | [Event.Block.Dataview.RelationSet](#anytype.Event.Block.Dataview.RelationSet) |  |  |
| userBlockJoin | [Event.User.Block.Join](#anytype.Event.User.Block.Join) |  |  |
| userBlockLeft | [Event.User.Block.Left](#anytype.Event.User.Block.Left) |  |  |
| userBlockSelectRange | [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange) |  |  |
| userBlockTextRange | [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange) |  |  |
| ping | [Event.Ping](#anytype.Event.Ping) |  |  |
| processNew | [Event.Process.New](#anytype.Event.Process.New) |  |  |
| processUpdate | [Event.Process.Update](#anytype.Event.Process.Update) |  |  |
| processDone | [Event.Process.Done](#anytype.Event.Process.Done) |  |  |
| threadStatus | [Event.Status.Thread](#anytype.Event.Status.Thread) |  |  |






<a name="anytype.Event.Object"></a>

### Event.Object







<a name="anytype.Event.Object.Details"></a>

### Event.Object.Details







<a name="anytype.Event.Object.Details.Amend"></a>

### Event.Object.Details.Amend
Amend (i.e. add a new key-value pair or update an existing key-value pair) existing state


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| details | [Event.Object.Details.Amend.KeyValue](#anytype.Event.Object.Details.Amend.KeyValue) | repeated | slice of changed key-values |
| subIds | [string](#string) | repeated |  |






<a name="anytype.Event.Object.Details.Amend.KeyValue"></a>

### Event.Object.Details.Amend.KeyValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  | should not be null |






<a name="anytype.Event.Object.Details.Set"></a>

### Event.Object.Details.Set
Overwrite current state


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | can not be a partial state. Should replace client details state |
| subIds | [string](#string) | repeated |  |






<a name="anytype.Event.Object.Details.Unset"></a>

### Event.Object.Details.Unset
Unset existing detail keys


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| keys | [string](#string) | repeated |  |
| subIds | [string](#string) | repeated |  |






<a name="anytype.Event.Object.Relation"></a>

### Event.Object.Relation







<a name="anytype.Event.Object.Relation.Remove"></a>

### Event.Object.Relation.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| relationKey | [string](#string) |  |  |






<a name="anytype.Event.Object.Relation.Set"></a>

### Event.Object.Relation.Set



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| relationKey | [string](#string) |  |  |
| relation | [model.Relation](#anytype.model.Relation) |  | missing value means relation should be removed |






<a name="anytype.Event.Object.Relations"></a>

### Event.Object.Relations







<a name="anytype.Event.Object.Relations.Amend"></a>

### Event.Object.Relations.Amend



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Event.Object.Relations.Remove"></a>

### Event.Object.Relations.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| keys | [string](#string) | repeated |  |






<a name="anytype.Event.Object.Relations.Set"></a>

### Event.Object.Relations.Set



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | context objectId |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Event.Object.Remove"></a>

### Event.Object.Remove



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated | notifies that objects were removed |






<a name="anytype.Event.Object.Show"></a>

### Event.Object.Show
Works with a smart blocks: Page, Dashboard
Dashboard opened, click on a page, Rpc.Block.open, Block.ShowFullscreen(PageBlock)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootId | [string](#string) |  | Root block id |
| blocks | [model.Block](#anytype.model.Block) | repeated | dependent simple blocks (descendants) |
| details | [Event.Object.Details.Set](#anytype.Event.Object.Details.Set) | repeated | details for the current and dependent objects |
| type | [model.SmartBlockType](#anytype.model.SmartBlockType) |  |  |
| objectTypes | [model.ObjectType](#anytype.model.ObjectType) | repeated | objectTypes contains ONLY to get layouts for the actual and all dependent objects. Relations are currently omitted // todo: switch to other pb model |
| relations | [model.Relation](#anytype.model.Relation) | repeated | combined relations of object&#39;s type &#43; extra relations. If object doesn&#39;t has some relation key in the details this means client should hide it and only suggest when adding existing one |
| restrictions | [model.Restrictions](#anytype.model.Restrictions) |  | object restrictions |






<a name="anytype.Event.Object.Show.RelationWithValuePerObject"></a>

### Event.Object.Show.RelationWithValuePerObject



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) |  |  |
| relations | [model.RelationWithValue](#anytype.model.RelationWithValue) | repeated |  |






<a name="anytype.Event.Object.Subscription"></a>

### Event.Object.Subscription







<a name="anytype.Event.Object.Subscription.Add"></a>

### Event.Object.Subscription.Add
Adds new document to subscriptions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | object id |
| afterId | [string](#string) |  | id of previous doc in order, empty means first |
| subId | [string](#string) |  | subscription id |






<a name="anytype.Event.Object.Subscription.Counters"></a>

### Event.Object.Subscription.Counters



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total | [int64](#int64) |  | total available records |
| nextCount | [int64](#int64) |  | how many records available after |
| prevCount | [int64](#int64) |  | how many records available before |
| subId | [string](#string) |  | subscription id |






<a name="anytype.Event.Object.Subscription.Position"></a>

### Event.Object.Subscription.Position
Indicates new position of document


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | object id |
| afterId | [string](#string) |  | id of previous doc in order, empty means first |
| subId | [string](#string) |  | subscription id |






<a name="anytype.Event.Object.Subscription.Remove"></a>

### Event.Object.Subscription.Remove
Removes document from subscription


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | object id |
| subId | [string](#string) |  | subscription id |






<a name="anytype.Event.Ping"></a>

### Event.Ping



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  |  |






<a name="anytype.Event.Process"></a>

### Event.Process







<a name="anytype.Event.Process.Done"></a>

### Event.Process.Done



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype.Model.Process) |  |  |






<a name="anytype.Event.Process.New"></a>

### Event.Process.New



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype.Model.Process) |  |  |






<a name="anytype.Event.Process.Update"></a>

### Event.Process.Update



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype.Model.Process) |  |  |






<a name="anytype.Event.Status"></a>

### Event.Status







<a name="anytype.Event.Status.Thread"></a>

### Event.Status.Thread



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| summary | [Event.Status.Thread.Summary](#anytype.Event.Status.Thread.Summary) |  |  |
| cafe | [Event.Status.Thread.Cafe](#anytype.Event.Status.Thread.Cafe) |  |  |
| accounts | [Event.Status.Thread.Account](#anytype.Event.Status.Thread.Account) | repeated |  |






<a name="anytype.Event.Status.Thread.Account"></a>

### Event.Status.Thread.Account



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| imageHash | [string](#string) |  |  |
| online | [bool](#bool) |  |  |
| lastPulled | [int64](#int64) |  |  |
| lastEdited | [int64](#int64) |  |  |
| devices | [Event.Status.Thread.Device](#anytype.Event.Status.Thread.Device) | repeated |  |






<a name="anytype.Event.Status.Thread.Cafe"></a>

### Event.Status.Thread.Cafe



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Event.Status.Thread.SyncStatus](#anytype.Event.Status.Thread.SyncStatus) |  |  |
| lastPulled | [int64](#int64) |  |  |
| lastPushSucceed | [bool](#bool) |  |  |
| files | [Event.Status.Thread.Cafe.PinStatus](#anytype.Event.Status.Thread.Cafe.PinStatus) |  |  |






<a name="anytype.Event.Status.Thread.Cafe.PinStatus"></a>

### Event.Status.Thread.Cafe.PinStatus



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pinning | [int32](#int32) |  |  |
| pinned | [int32](#int32) |  |  |
| failed | [int32](#int32) |  |  |
| updated | [int64](#int64) |  |  |






<a name="anytype.Event.Status.Thread.Device"></a>

### Event.Status.Thread.Device



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| online | [bool](#bool) |  |  |
| lastPulled | [int64](#int64) |  |  |
| lastEdited | [int64](#int64) |  |  |






<a name="anytype.Event.Status.Thread.Summary"></a>

### Event.Status.Thread.Summary



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Event.Status.Thread.SyncStatus](#anytype.Event.Status.Thread.SyncStatus) |  |  |






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
| range | [model.Range](#anytype.model.Range) |  | Range of the selection |






<a name="anytype.Model"></a>

### Model







<a name="anytype.Model.Process"></a>

### Model.Process



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Model.Process.Type](#anytype.Model.Process.Type) |  |  |
| state | [Model.Process.State](#anytype.Model.Process.State) |  |  |
| progress | [Model.Process.Progress](#anytype.Model.Process.Progress) |  |  |






<a name="anytype.Model.Process.Progress"></a>

### Model.Process.Progress



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total | [int64](#int64) |  |  |
| done | [int64](#int64) |  |  |
| message | [string](#string) |  |  |






<a name="anytype.ResponseEvent"></a>

### ResponseEvent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype.Event.Message) | repeated |  |
| contextId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |





 


<a name="anytype.Event.Status.Thread.SyncStatus"></a>

### Event.Status.Thread.SyncStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Offline | 1 |  |
| Syncing | 2 |  |
| Synced | 3 |  |
| Failed | 4 |  |



<a name="anytype.Model.Process.State"></a>

### Model.Process.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Running | 1 |  |
| Done | 2 |  |
| Canceled | 3 |  |
| Error | 4 |  |



<a name="anytype.Model.Process.Type"></a>

### Model.Process.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| DropFiles | 0 |  |
| Import | 1 |  |
| Export | 2 |  |
| SaveFile | 3 |  |
| RecoverAccount | 4 |  |


 

 

 



<a name="pkg/lib/pb/model/protos/localstore.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pkg/lib/pb/model/protos/localstore.proto



<a name="anytype.model.ObjectDetails"></a>

### ObjectDetails



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.model.ObjectInfo"></a>

### ObjectInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| objectTypeUrls | [string](#string) | repeated | deprecated |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |
| snippet | [string](#string) |  |  |
| hasInboundLinks | [bool](#bool) |  |  |
| objectType | [SmartBlockType](#anytype.model.SmartBlockType) |  |  |






<a name="anytype.model.ObjectInfoWithLinks"></a>

### ObjectInfoWithLinks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| info | [ObjectInfo](#anytype.model.ObjectInfo) |  |  |
| links | [ObjectLinksInfo](#anytype.model.ObjectLinksInfo) |  |  |






<a name="anytype.model.ObjectInfoWithOutboundLinks"></a>

### ObjectInfoWithOutboundLinks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| info | [ObjectInfo](#anytype.model.ObjectInfo) |  |  |
| outboundLinks | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |






<a name="anytype.model.ObjectInfoWithOutboundLinksIDs"></a>

### ObjectInfoWithOutboundLinksIDs



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| info | [ObjectInfo](#anytype.model.ObjectInfo) |  |  |
| outboundLinks | [string](#string) | repeated |  |






<a name="anytype.model.ObjectLinks"></a>

### ObjectLinks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inboundIDs | [string](#string) | repeated |  |
| outboundIDs | [string](#string) | repeated |  |






<a name="anytype.model.ObjectLinksInfo"></a>

### ObjectLinksInfo



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inbound | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |
| outbound | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |






<a name="anytype.model.ObjectStoreChecksums"></a>

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





 

 

 

 



<a name="pkg/lib/pb/model/protos/models.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pkg/lib/pb/model/protos/models.proto



<a name="anytype.model.Account"></a>

### Account
Contains basic information about a user account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | User&#39;s thread id |
| name | [string](#string) |  | User name, that associated with this account |
| avatar | [Account.Avatar](#anytype.model.Account.Avatar) |  | Avatar of a user&#39;s account |
| config | [Account.Config](#anytype.model.Account.Config) |  |  |
| status | [Account.Status](#anytype.model.Account.Status) |  |  |






<a name="anytype.model.Account.Avatar"></a>

### Account.Avatar
Avatar of a user&#39;s account. It could be an image or color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| image | [Block.Content.File](#anytype.model.Block.Content.File) |  | Image of the avatar. Contains the hash to retrieve the image. |
| color | [string](#string) |  | Color of the avatar, used if image not set. |






<a name="anytype.model.Account.Config"></a>

### Account.Config



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enableDataview | [bool](#bool) |  |  |
| enableDebug | [bool](#bool) |  |  |
| enableReleaseChannelSwitch | [bool](#bool) |  |  |
| enableSpaces | [bool](#bool) |  |  |
| extra | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.model.Account.Status"></a>

### Account.Status



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| statusType | [Account.StatusType](#anytype.model.Account.StatusType) |  |  |
| deletionDate | [int64](#int64) |  |  |






<a name="anytype.model.Block"></a>

### Block



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| restrictions | [Block.Restrictions](#anytype.model.Block.Restrictions) |  |  |
| childrenIds | [string](#string) | repeated |  |
| backgroundColor | [string](#string) |  |  |
| align | [Block.Align](#anytype.model.Block.Align) |  |  |
| smartblock | [Block.Content.Smartblock](#anytype.model.Block.Content.Smartblock) |  |  |
| text | [Block.Content.Text](#anytype.model.Block.Content.Text) |  |  |
| file | [Block.Content.File](#anytype.model.Block.Content.File) |  |  |
| layout | [Block.Content.Layout](#anytype.model.Block.Content.Layout) |  |  |
| div | [Block.Content.Div](#anytype.model.Block.Content.Div) |  |  |
| bookmark | [Block.Content.Bookmark](#anytype.model.Block.Content.Bookmark) |  |  |
| icon | [Block.Content.Icon](#anytype.model.Block.Content.Icon) |  |  |
| link | [Block.Content.Link](#anytype.model.Block.Content.Link) |  |  |
| dataview | [Block.Content.Dataview](#anytype.model.Block.Content.Dataview) |  |  |
| relation | [Block.Content.Relation](#anytype.model.Block.Content.Relation) |  |  |
| featuredRelations | [Block.Content.FeaturedRelations](#anytype.model.Block.Content.FeaturedRelations) |  |  |
| latex | [Block.Content.Latex](#anytype.model.Block.Content.Latex) |  |  |
| tableOfContents | [Block.Content.TableOfContents](#anytype.model.Block.Content.TableOfContents) |  |  |






<a name="anytype.model.Block.Content"></a>

### Block.Content







<a name="anytype.model.Block.Content.Bookmark"></a>

### Block.Content.Bookmark
Bookmark is to keep a web-link and to preview a content.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| title | [string](#string) |  |  |
| description | [string](#string) |  |  |
| imageHash | [string](#string) |  |  |
| faviconHash | [string](#string) |  |  |
| type | [LinkPreview.Type](#anytype.model.LinkPreview.Type) |  |  |






<a name="anytype.model.Block.Content.Dataview"></a>

### Block.Content.Dataview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| views | [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) | repeated |  |
| relations | [Relation](#anytype.model.Relation) | repeated | index 3 is deprecated, was used for schemaURL in old-format sets |
| activeView | [string](#string) |  | saved within a session |






<a name="anytype.model.Block.Content.Dataview.Filter"></a>

### Block.Content.Dataview.Filter



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator | [Block.Content.Dataview.Filter.Operator](#anytype.model.Block.Content.Dataview.Filter.Operator) |  | looks not applicable? |
| RelationKey | [string](#string) |  |  |
| relationProperty | [string](#string) |  |  |
| condition | [Block.Content.Dataview.Filter.Condition](#anytype.model.Block.Content.Dataview.Filter.Condition) |  |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  |  |






<a name="anytype.model.Block.Content.Dataview.Relation"></a>

### Block.Content.Dataview.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| isVisible | [bool](#bool) |  |  |
| width | [int32](#int32) |  | the displayed column % calculated based on other visible relations |
| dateIncludeTime | [bool](#bool) |  |  |
| timeFormat | [Block.Content.Dataview.Relation.TimeFormat](#anytype.model.Block.Content.Dataview.Relation.TimeFormat) |  |  |
| dateFormat | [Block.Content.Dataview.Relation.DateFormat](#anytype.model.Block.Content.Dataview.Relation.DateFormat) |  |  |






<a name="anytype.model.Block.Content.Dataview.Sort"></a>

### Block.Content.Dataview.Sort



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| RelationKey | [string](#string) |  |  |
| type | [Block.Content.Dataview.Sort.Type](#anytype.model.Block.Content.Dataview.Sort.Type) |  |  |






<a name="anytype.model.Block.Content.Dataview.View"></a>

### Block.Content.Dataview.View



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [Block.Content.Dataview.View.Type](#anytype.model.Block.Content.Dataview.View.Type) |  |  |
| name | [string](#string) |  |  |
| sorts | [Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort) | repeated |  |
| filters | [Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated |  |
| relations | [Block.Content.Dataview.Relation](#anytype.model.Block.Content.Dataview.Relation) | repeated | relations fields/columns options, also used to provide the order |
| coverRelationKey | [string](#string) |  | Relation used for cover in gallery |
| hideIcon | [bool](#bool) |  | Hide icon near name |
| cardSize | [Block.Content.Dataview.View.Size](#anytype.model.Block.Content.Dataview.View.Size) |  | Gallery card size |
| coverFit | [bool](#bool) |  | Image fits container |






<a name="anytype.model.Block.Content.Div"></a>

### Block.Content.Div
Divider: block, that contains only one horizontal thin line


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |






<a name="anytype.model.Block.Content.FeaturedRelations"></a>

### Block.Content.FeaturedRelations







<a name="anytype.model.Block.Content.File"></a>

### Block.Content.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| name | [string](#string) |  |  |
| type | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |
| mime | [string](#string) |  |  |
| size | [int64](#int64) |  |  |
| addedAt | [int64](#int64) |  |  |
| state | [Block.Content.File.State](#anytype.model.Block.Content.File.State) |  |  |
| style | [Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.model.Block.Content.Icon"></a>

### Block.Content.Icon



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |






<a name="anytype.model.Block.Content.Latex"></a>

### Block.Content.Latex



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |






<a name="anytype.model.Block.Content.Layout"></a>

### Block.Content.Layout
Layout have no visual representation, but affects on blocks, that it contains.
Row/Column layout blocks creates only automatically, after some of a D&amp;D operations, for example


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Layout.Style](#anytype.model.Block.Content.Layout.Style) |  |  |






<a name="anytype.model.Block.Content.Link"></a>

### Block.Content.Link
Link: block to link some content from an external sources.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetBlockId | [string](#string) |  | id of the target block |
| style | [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) |  | deprecated |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.model.Block.Content.Relation"></a>

### Block.Content.Relation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |






<a name="anytype.model.Block.Content.Smartblock"></a>

### Block.Content.Smartblock







<a name="anytype.model.Block.Content.TableOfContents"></a>

### Block.Content.TableOfContents







<a name="anytype.model.Block.Content.Text"></a>

### Block.Content.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |
| marks | [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) |  | list of marks to apply to the text |
| checked | [bool](#bool) |  |  |
| color | [string](#string) |  |  |
| iconEmoji | [string](#string) |  | used with style Callout |
| iconImage | [string](#string) |  | in case both image and emoji are set, image should has a priority in the UI |






<a name="anytype.model.Block.Content.Text.Mark"></a>

### Block.Content.Text.Mark



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Range](#anytype.model.Range) |  | range of symbols to apply this mark. From(symbol) To(symbol) |
| type | [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type) |  |  |
| param | [string](#string) |  | link, color, etc |






<a name="anytype.model.Block.Content.Text.Marks"></a>

### Block.Content.Text.Marks



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marks | [Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark) | repeated |  |






<a name="anytype.model.Block.Restrictions"></a>

### Block.Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| read | [bool](#bool) |  |  |
| edit | [bool](#bool) |  |  |
| remove | [bool](#bool) |  |  |
| drag | [bool](#bool) |  |  |
| dropOn | [bool](#bool) |  |  |






<a name="anytype.model.BlockMetaOnly"></a>

### BlockMetaOnly
Used to decode block meta only, without the content itself


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.model.Layout"></a>

### Layout



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [ObjectType.Layout](#anytype.model.ObjectType.Layout) |  |  |
| name | [string](#string) |  |  |
| requiredRelations | [Relation](#anytype.model.Relation) | repeated | relations required for this object type |






<a name="anytype.model.LinkPreview"></a>

### LinkPreview



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| title | [string](#string) |  |  |
| description | [string](#string) |  |  |
| imageUrl | [string](#string) |  |  |
| faviconUrl | [string](#string) |  |  |
| type | [LinkPreview.Type](#anytype.model.LinkPreview.Type) |  |  |






<a name="anytype.model.ObjectType"></a>

### ObjectType



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  | leave empty in case you want to create the new one |
| name | [string](#string) |  | name of objectType (can be localized for bundled types) |
| relations | [Relation](#anytype.model.Relation) | repeated | cannot contain more than one Relation with the same RelationType |
| layout | [ObjectType.Layout](#anytype.model.ObjectType.Layout) |  |  |
| iconEmoji | [string](#string) |  | emoji symbol |
| description | [string](#string) |  |  |
| hidden | [bool](#bool) |  |  |
| readonly | [bool](#bool) |  |  |
| types | [SmartBlockType](#anytype.model.SmartBlockType) | repeated |  |
| isArchived | [bool](#bool) |  | sets locally to hide object type from set and some other places |






<a name="anytype.model.Range"></a>

### Range
General purpose structure, uses in Mark.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) |  |  |
| to | [int32](#int32) |  |  |






<a name="anytype.model.Relation"></a>

### Relation
Relation describe the human-interpreted relation type. It may be something like &#34;Date of creation, format=date&#34; or &#34;Assignee, format=objectId, objectType=person&#34;


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  | Key under which the value is stored in the map. Must be unique for the object type. It usually auto-generated bsonid, but also may be something human-readable in case of prebuilt types. |
| format | [RelationFormat](#anytype.model.RelationFormat) |  | format of the underlying data |
| name | [string](#string) |  | name to show (can be localized for bundled types) |
| defaultValue | [google.protobuf.Value](#google.protobuf.Value) |  |  |
| dataSource | [Relation.DataSource](#anytype.model.Relation.DataSource) |  | where the data is stored |
| hidden | [bool](#bool) |  | internal, not displayed to user (e.g. coverX, coverY) |
| readOnly | [bool](#bool) |  | value not editable by user tobe renamed to readonlyValue |
| readOnlyRelation | [bool](#bool) |  | relation metadata, eg name and format is not editable by user |
| multi | [bool](#bool) |  | allow multiple values (stored in pb list) |
| objectTypes | [string](#string) | repeated | URL of object type, empty to allow link to any object |
| selectDict | [Relation.Option](#anytype.model.Relation.Option) | repeated | index 10, 11 was used in internal-only builds. Can be reused, but may break some test accounts

default dictionary with unique values to choose for select/multiSelect format |
| maxCount | [int32](#int32) |  | max number of values can be set for this relation. 0 means no limit. 1 means the value can be stored in non-repeated field |
| description | [string](#string) |  |  |
| scope | [Relation.Scope](#anytype.model.Relation.Scope) |  | on-store fields, injected only locally

scope from which this relation have been aggregated |
| creator | [string](#string) |  | creator profile id |






<a name="anytype.model.Relation.Option"></a>

### Relation.Option



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | id generated automatically if omitted |
| text | [string](#string) |  |  |
| color | [string](#string) |  | stored |
| scope | [Relation.Option.Scope](#anytype.model.Relation.Option.Scope) |  | on-store contains only local-scope relations. All others injected on-the-fly |






<a name="anytype.model.RelationOptions"></a>

### RelationOptions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| options | [Relation.Option](#anytype.model.Relation.Option) | repeated |  |






<a name="anytype.model.RelationWithValue"></a>

### RelationWithValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relation | [Relation](#anytype.model.Relation) |  |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  |  |






<a name="anytype.model.Relations"></a>

### Relations



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relations | [Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.model.Restrictions"></a>

### Restrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| object | [Restrictions.ObjectRestriction](#anytype.model.Restrictions.ObjectRestriction) | repeated |  |
| dataview | [Restrictions.DataviewRestrictions](#anytype.model.Restrictions.DataviewRestrictions) | repeated |  |






<a name="anytype.model.Restrictions.DataviewRestrictions"></a>

### Restrictions.DataviewRestrictions



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| restrictions | [Restrictions.DataviewRestriction](#anytype.model.Restrictions.DataviewRestriction) | repeated |  |






<a name="anytype.model.SmartBlockSnapshotBase"></a>

### SmartBlockSnapshotBase



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [Block](#anytype.model.Block) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| fileKeys | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| extraRelations | [Relation](#anytype.model.Relation) | repeated |  |
| objectTypes | [string](#string) | repeated |  |
| collections | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.model.ThreadCreateQueueEntry"></a>

### ThreadCreateQueueEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| collectionThread | [string](#string) |  |  |
| threadId | [string](#string) |  |  |






<a name="anytype.model.ThreadDeeplinkPayload"></a>

### ThreadDeeplinkPayload



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| addrs | [string](#string) | repeated |  |





 


<a name="anytype.model.Account.StatusType"></a>

### Account.StatusType


| Name | Number | Description |
| ---- | ------ | ----------- |
| Active | 0 |  |
| PendingDeletion | 1 |  |
| StartedDeletion | 2 |  |
| Deleted | 3 |  |



<a name="anytype.model.Block.Align"></a>

### Block.Align


| Name | Number | Description |
| ---- | ------ | ----------- |
| AlignLeft | 0 |  |
| AlignCenter | 1 |  |
| AlignRight | 2 |  |



<a name="anytype.model.Block.Content.Dataview.Filter.Condition"></a>

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
| In | 9 |  |
| NotIn | 10 |  |
| Empty | 11 |  |
| NotEmpty | 12 |  |
| AllIn | 13 |  |
| NotAllIn | 14 |  |



<a name="anytype.model.Block.Content.Dataview.Filter.Operator"></a>

### Block.Content.Dataview.Filter.Operator


| Name | Number | Description |
| ---- | ------ | ----------- |
| And | 0 |  |
| Or | 1 |  |



<a name="anytype.model.Block.Content.Dataview.Relation.DateFormat"></a>

### Block.Content.Dataview.Relation.DateFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| MonthAbbrBeforeDay | 0 | Jul 30, 2020 |
| MonthAbbrAfterDay | 1 | 30 Jul 2020 |
| Short | 2 | 30/07/2020 |
| ShortUS | 3 | 07/30/2020 |
| ISO | 4 | 2020-07-30 |



<a name="anytype.model.Block.Content.Dataview.Relation.TimeFormat"></a>

### Block.Content.Dataview.Relation.TimeFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| Format12 | 0 |  |
| Format24 | 1 |  |



<a name="anytype.model.Block.Content.Dataview.Sort.Type"></a>

### Block.Content.Dataview.Sort.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Asc | 0 |  |
| Desc | 1 |  |



<a name="anytype.model.Block.Content.Dataview.View.Size"></a>

### Block.Content.Dataview.View.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| Small | 0 |  |
| Medium | 1 |  |
| Large | 2 |  |



<a name="anytype.model.Block.Content.Dataview.View.Type"></a>

### Block.Content.Dataview.View.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Table | 0 |  |
| List | 1 |  |
| Gallery | 2 |  |
| Kanban | 3 |  |



<a name="anytype.model.Block.Content.Div.Style"></a>

### Block.Content.Div.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Line | 0 |  |
| Dots | 1 |  |



<a name="anytype.model.Block.Content.File.State"></a>

### Block.Content.File.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 | There is no file and preview, it&#39;s an empty block, that waits files. |
| Uploading | 1 | There is still no file/preview, but file already uploading |
| Done | 2 | File and preview downloaded |
| Error | 3 | Error while uploading |



<a name="anytype.model.Block.Content.File.Style"></a>

### Block.Content.File.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Auto | 0 | all types expect File and None has Embed style by default |
| Link | 1 |  |
| Embed | 2 |  |



<a name="anytype.model.Block.Content.File.Type"></a>

### Block.Content.File.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| File | 1 |  |
| Image | 2 |  |
| Video | 3 |  |
| Audio | 4 |  |
| PDF | 5 |  |



<a name="anytype.model.Block.Content.Layout.Style"></a>

### Block.Content.Layout.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Row | 0 |  |
| Column | 1 |  |
| Div | 2 |  |
| Header | 3 |  |



<a name="anytype.model.Block.Content.Link.Style"></a>

### Block.Content.Link.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Page | 0 |  |
| Dataview | 1 |  |
| Dashboard | 2 |  |
| Archive | 3 | ... |



<a name="anytype.model.Block.Content.Text.Mark.Type"></a>

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



<a name="anytype.model.Block.Content.Text.Style"></a>

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
| Title | 7 | currently only only one block of this style can exists on a page |
| Checkbox | 8 |  |
| Marked | 9 |  |
| Numbered | 10 |  |
| Toggle | 11 |  |
| Description | 12 | currently only only one block of this style can exists on a page |
| Callout | 13 | currently only only one block of this style can exists on a page |



<a name="anytype.model.Block.Position"></a>

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



<a name="anytype.model.LinkPreview.Type"></a>

### LinkPreview.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Page | 1 |  |
| Image | 2 |  |
| Text | 3 |  |



<a name="anytype.model.ObjectType.Layout"></a>

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
| database | 20 | to be released later |



<a name="anytype.model.Relation.DataSource"></a>

### Relation.DataSource


| Name | Number | Description |
| ---- | ------ | ----------- |
| details | 0 | default, stored inside the object&#39;s details |
| derived | 1 | stored locally, e.g. in badger or generated on the fly |
| account | 2 | stored in the account DB. means existing only for specific anytype account |
| local | 3 | stored locally |



<a name="anytype.model.Relation.Option.Scope"></a>

### Relation.Option.Scope


| Name | Number | Description |
| ---- | ------ | ----------- |
| local | 0 | stored within the object/aggregated from set |
| relation | 1 | aggregated from all relation of this relation&#39;s key |
| format | 2 | aggregated from all relations of this relation&#39;s format |



<a name="anytype.model.Relation.Scope"></a>

### Relation.Scope


| Name | Number | Description |
| ---- | ------ | ----------- |
| object | 0 | stored within the object |
| type | 1 | stored within the object type |
| setOfTheSameType | 2 | aggregated from the dataview of sets of the same object type |
| objectsOfTheSameType | 3 | aggregated from the dataview of sets of the same object type |
| library | 4 | aggregated from relations library |



<a name="anytype.model.RelationFormat"></a>

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



<a name="anytype.model.Restrictions.DataviewRestriction"></a>

### Restrictions.DataviewRestriction


| Name | Number | Description |
| ---- | ------ | ----------- |
| DVNone | 0 |  |
| DVRelation | 1 |  |
| DVCreateObject | 2 |  |
| DVViews | 3 |  |



<a name="anytype.model.Restrictions.ObjectRestriction"></a>

### Restrictions.ObjectRestriction


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Delete | 1 | restricts delete |
| Relations | 2 | restricts work with relations |
| Blocks | 3 | restricts work with blocks |
| Details | 4 | restricts work with details |
| TypeChange | 5 |  |
| LayoutChange | 6 |  |
| Template | 7 |  |



<a name="anytype.model.SmartBlockType"></a>

### SmartBlockType


| Name | Number | Description |
| ---- | ------ | ----------- |
| AccountOld | 0 |  |
| Breadcrumbs | 1 |  |
| Page | 16 |  |
| ProfilePage | 17 |  |
| Home | 32 |  |
| Archive | 48 |  |
| Database | 64 |  |
| Set | 65 | only have dataview simpleblock |
| STObjectType | 96 | have relations list |
| File | 256 |  |
| Template | 288 |  |
| BundledTemplate | 289 |  |
| MarketplaceType | 272 |  |
| MarketplaceRelation | 273 |  |
| MarketplaceTemplate | 274 |  |
| BundledRelation | 512 |  |
| IndexedRelation | 513 |  |
| BundledObjectType | 514 |  |
| AnytypeProfile | 515 |  |
| Date | 516 |  |
| WorkspaceOld | 517 | deprecated thread-based workspace |
| Workspace | 518 |  |


 

 

 



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

