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
    - [Rpc.ApplyTemplate](#anytype.Rpc.ApplyTemplate)
    - [Rpc.ApplyTemplate.Request](#anytype.Rpc.ApplyTemplate.Request)
    - [Rpc.ApplyTemplate.Response](#anytype.Rpc.ApplyTemplate.Response)
    - [Rpc.ApplyTemplate.Response.Error](#anytype.Rpc.ApplyTemplate.Response.Error)
    - [Rpc.Block](#anytype.Rpc.Block)
    - [Rpc.Block.Bookmark](#anytype.Rpc.Block.Bookmark)
    - [Rpc.Block.Bookmark.CreateAndFetch](#anytype.Rpc.Block.Bookmark.CreateAndFetch)
    - [Rpc.Block.Bookmark.CreateAndFetch.Request](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Request)
    - [Rpc.Block.Bookmark.CreateAndFetch.Response](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response)
    - [Rpc.Block.Bookmark.CreateAndFetch.Response.Error](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error)
    - [Rpc.Block.Bookmark.Fetch](#anytype.Rpc.Block.Bookmark.Fetch)
    - [Rpc.Block.Bookmark.Fetch.Request](#anytype.Rpc.Block.Bookmark.Fetch.Request)
    - [Rpc.Block.Bookmark.Fetch.Response](#anytype.Rpc.Block.Bookmark.Fetch.Response)
    - [Rpc.Block.Bookmark.Fetch.Response.Error](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error)
    - [Rpc.Block.Close](#anytype.Rpc.Block.Close)
    - [Rpc.Block.Close.Request](#anytype.Rpc.Block.Close.Request)
    - [Rpc.Block.Close.Response](#anytype.Rpc.Block.Close.Response)
    - [Rpc.Block.Close.Response.Error](#anytype.Rpc.Block.Close.Response.Error)
    - [Rpc.Block.Copy](#anytype.Rpc.Block.Copy)
    - [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request)
    - [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response)
    - [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error)
    - [Rpc.Block.Create](#anytype.Rpc.Block.Create)
    - [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request)
    - [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response)
    - [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error)
    - [Rpc.Block.CreatePage](#anytype.Rpc.Block.CreatePage)
    - [Rpc.Block.CreatePage.Request](#anytype.Rpc.Block.CreatePage.Request)
    - [Rpc.Block.CreatePage.Response](#anytype.Rpc.Block.CreatePage.Response)
    - [Rpc.Block.CreatePage.Response.Error](#anytype.Rpc.Block.CreatePage.Response.Error)
    - [Rpc.Block.CreateSet](#anytype.Rpc.Block.CreateSet)
    - [Rpc.Block.CreateSet.Request](#anytype.Rpc.Block.CreateSet.Request)
    - [Rpc.Block.CreateSet.Response](#anytype.Rpc.Block.CreateSet.Response)
    - [Rpc.Block.CreateSet.Response.Error](#anytype.Rpc.Block.CreateSet.Response.Error)
    - [Rpc.Block.Cut](#anytype.Rpc.Block.Cut)
    - [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request)
    - [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response)
    - [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error)
    - [Rpc.Block.Dataview](#anytype.Rpc.Block.Dataview)
    - [Rpc.Block.Dataview.RecordCreate](#anytype.Rpc.Block.Dataview.RecordCreate)
    - [Rpc.Block.Dataview.RecordCreate.Request](#anytype.Rpc.Block.Dataview.RecordCreate.Request)
    - [Rpc.Block.Dataview.RecordCreate.Response](#anytype.Rpc.Block.Dataview.RecordCreate.Response)
    - [Rpc.Block.Dataview.RecordCreate.Response.Error](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error)
    - [Rpc.Block.Dataview.RecordDelete](#anytype.Rpc.Block.Dataview.RecordDelete)
    - [Rpc.Block.Dataview.RecordDelete.Request](#anytype.Rpc.Block.Dataview.RecordDelete.Request)
    - [Rpc.Block.Dataview.RecordDelete.Response](#anytype.Rpc.Block.Dataview.RecordDelete.Response)
    - [Rpc.Block.Dataview.RecordDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error)
    - [Rpc.Block.Dataview.RecordRelationOptionAdd](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd)
    - [Rpc.Block.Dataview.RecordRelationOptionAdd.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Request)
    - [Rpc.Block.Dataview.RecordRelationOptionAdd.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response)
    - [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error)
    - [Rpc.Block.Dataview.RecordRelationOptionDelete](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete)
    - [Rpc.Block.Dataview.RecordRelationOptionDelete.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Request)
    - [Rpc.Block.Dataview.RecordRelationOptionDelete.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response)
    - [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error)
    - [Rpc.Block.Dataview.RecordRelationOptionUpdate](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate)
    - [Rpc.Block.Dataview.RecordRelationOptionUpdate.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Request)
    - [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response)
    - [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error)
    - [Rpc.Block.Dataview.RecordUpdate](#anytype.Rpc.Block.Dataview.RecordUpdate)
    - [Rpc.Block.Dataview.RecordUpdate.Request](#anytype.Rpc.Block.Dataview.RecordUpdate.Request)
    - [Rpc.Block.Dataview.RecordUpdate.Response](#anytype.Rpc.Block.Dataview.RecordUpdate.Response)
    - [Rpc.Block.Dataview.RecordUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error)
    - [Rpc.Block.Dataview.RelationAdd](#anytype.Rpc.Block.Dataview.RelationAdd)
    - [Rpc.Block.Dataview.RelationAdd.Request](#anytype.Rpc.Block.Dataview.RelationAdd.Request)
    - [Rpc.Block.Dataview.RelationAdd.Response](#anytype.Rpc.Block.Dataview.RelationAdd.Response)
    - [Rpc.Block.Dataview.RelationAdd.Response.Error](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error)
    - [Rpc.Block.Dataview.RelationDelete](#anytype.Rpc.Block.Dataview.RelationDelete)
    - [Rpc.Block.Dataview.RelationDelete.Request](#anytype.Rpc.Block.Dataview.RelationDelete.Request)
    - [Rpc.Block.Dataview.RelationDelete.Response](#anytype.Rpc.Block.Dataview.RelationDelete.Response)
    - [Rpc.Block.Dataview.RelationDelete.Response.Error](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error)
    - [Rpc.Block.Dataview.RelationListAvailable](#anytype.Rpc.Block.Dataview.RelationListAvailable)
    - [Rpc.Block.Dataview.RelationListAvailable.Request](#anytype.Rpc.Block.Dataview.RelationListAvailable.Request)
    - [Rpc.Block.Dataview.RelationListAvailable.Response](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response)
    - [Rpc.Block.Dataview.RelationListAvailable.Response.Error](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error)
    - [Rpc.Block.Dataview.RelationUpdate](#anytype.Rpc.Block.Dataview.RelationUpdate)
    - [Rpc.Block.Dataview.RelationUpdate.Request](#anytype.Rpc.Block.Dataview.RelationUpdate.Request)
    - [Rpc.Block.Dataview.RelationUpdate.Response](#anytype.Rpc.Block.Dataview.RelationUpdate.Response)
    - [Rpc.Block.Dataview.RelationUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error)
    - [Rpc.Block.Dataview.SetSource](#anytype.Rpc.Block.Dataview.SetSource)
    - [Rpc.Block.Dataview.SetSource.Request](#anytype.Rpc.Block.Dataview.SetSource.Request)
    - [Rpc.Block.Dataview.SetSource.Response](#anytype.Rpc.Block.Dataview.SetSource.Response)
    - [Rpc.Block.Dataview.SetSource.Response.Error](#anytype.Rpc.Block.Dataview.SetSource.Response.Error)
    - [Rpc.Block.Dataview.ViewCreate](#anytype.Rpc.Block.Dataview.ViewCreate)
    - [Rpc.Block.Dataview.ViewCreate.Request](#anytype.Rpc.Block.Dataview.ViewCreate.Request)
    - [Rpc.Block.Dataview.ViewCreate.Response](#anytype.Rpc.Block.Dataview.ViewCreate.Response)
    - [Rpc.Block.Dataview.ViewCreate.Response.Error](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error)
    - [Rpc.Block.Dataview.ViewDelete](#anytype.Rpc.Block.Dataview.ViewDelete)
    - [Rpc.Block.Dataview.ViewDelete.Request](#anytype.Rpc.Block.Dataview.ViewDelete.Request)
    - [Rpc.Block.Dataview.ViewDelete.Response](#anytype.Rpc.Block.Dataview.ViewDelete.Response)
    - [Rpc.Block.Dataview.ViewDelete.Response.Error](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error)
    - [Rpc.Block.Dataview.ViewSetActive](#anytype.Rpc.Block.Dataview.ViewSetActive)
    - [Rpc.Block.Dataview.ViewSetActive.Request](#anytype.Rpc.Block.Dataview.ViewSetActive.Request)
    - [Rpc.Block.Dataview.ViewSetActive.Response](#anytype.Rpc.Block.Dataview.ViewSetActive.Response)
    - [Rpc.Block.Dataview.ViewSetActive.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error)
    - [Rpc.Block.Dataview.ViewSetPosition](#anytype.Rpc.Block.Dataview.ViewSetPosition)
    - [Rpc.Block.Dataview.ViewSetPosition.Request](#anytype.Rpc.Block.Dataview.ViewSetPosition.Request)
    - [Rpc.Block.Dataview.ViewSetPosition.Response](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response)
    - [Rpc.Block.Dataview.ViewSetPosition.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error)
    - [Rpc.Block.Dataview.ViewUpdate](#anytype.Rpc.Block.Dataview.ViewUpdate)
    - [Rpc.Block.Dataview.ViewUpdate.Request](#anytype.Rpc.Block.Dataview.ViewUpdate.Request)
    - [Rpc.Block.Dataview.ViewUpdate.Response](#anytype.Rpc.Block.Dataview.ViewUpdate.Response)
    - [Rpc.Block.Dataview.ViewUpdate.Response.Error](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error)
    - [Rpc.Block.Download](#anytype.Rpc.Block.Download)
    - [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request)
    - [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response)
    - [Rpc.Block.Download.Response.Error](#anytype.Rpc.Block.Download.Response.Error)
    - [Rpc.Block.Export](#anytype.Rpc.Block.Export)
    - [Rpc.Block.Export.Request](#anytype.Rpc.Block.Export.Request)
    - [Rpc.Block.Export.Response](#anytype.Rpc.Block.Export.Response)
    - [Rpc.Block.Export.Response.Error](#anytype.Rpc.Block.Export.Response.Error)
    - [Rpc.Block.File](#anytype.Rpc.Block.File)
    - [Rpc.Block.File.CreateAndUpload](#anytype.Rpc.Block.File.CreateAndUpload)
    - [Rpc.Block.File.CreateAndUpload.Request](#anytype.Rpc.Block.File.CreateAndUpload.Request)
    - [Rpc.Block.File.CreateAndUpload.Response](#anytype.Rpc.Block.File.CreateAndUpload.Response)
    - [Rpc.Block.File.CreateAndUpload.Response.Error](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error)
    - [Rpc.Block.Get](#anytype.Rpc.Block.Get)
    - [Rpc.Block.Get.Marks](#anytype.Rpc.Block.Get.Marks)
    - [Rpc.Block.Get.Marks.Request](#anytype.Rpc.Block.Get.Marks.Request)
    - [Rpc.Block.Get.Marks.Response](#anytype.Rpc.Block.Get.Marks.Response)
    - [Rpc.Block.Get.Marks.Response.Error](#anytype.Rpc.Block.Get.Marks.Response.Error)
    - [Rpc.Block.GetPublicWebURL](#anytype.Rpc.Block.GetPublicWebURL)
    - [Rpc.Block.GetPublicWebURL.Request](#anytype.Rpc.Block.GetPublicWebURL.Request)
    - [Rpc.Block.GetPublicWebURL.Response](#anytype.Rpc.Block.GetPublicWebURL.Response)
    - [Rpc.Block.GetPublicWebURL.Response.Error](#anytype.Rpc.Block.GetPublicWebURL.Response.Error)
    - [Rpc.Block.ImportMarkdown](#anytype.Rpc.Block.ImportMarkdown)
    - [Rpc.Block.ImportMarkdown.Request](#anytype.Rpc.Block.ImportMarkdown.Request)
    - [Rpc.Block.ImportMarkdown.Response](#anytype.Rpc.Block.ImportMarkdown.Response)
    - [Rpc.Block.ImportMarkdown.Response.Error](#anytype.Rpc.Block.ImportMarkdown.Response.Error)
    - [Rpc.Block.Merge](#anytype.Rpc.Block.Merge)
    - [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request)
    - [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response)
    - [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error)
    - [Rpc.Block.ObjectType](#anytype.Rpc.Block.ObjectType)
    - [Rpc.Block.ObjectType.Set](#anytype.Rpc.Block.ObjectType.Set)
    - [Rpc.Block.ObjectType.Set.Request](#anytype.Rpc.Block.ObjectType.Set.Request)
    - [Rpc.Block.ObjectType.Set.Response](#anytype.Rpc.Block.ObjectType.Set.Response)
    - [Rpc.Block.ObjectType.Set.Response.Error](#anytype.Rpc.Block.ObjectType.Set.Response.Error)
    - [Rpc.Block.Open](#anytype.Rpc.Block.Open)
    - [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request)
    - [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response)
    - [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error)
    - [Rpc.Block.OpenBreadcrumbs](#anytype.Rpc.Block.OpenBreadcrumbs)
    - [Rpc.Block.OpenBreadcrumbs.Request](#anytype.Rpc.Block.OpenBreadcrumbs.Request)
    - [Rpc.Block.OpenBreadcrumbs.Response](#anytype.Rpc.Block.OpenBreadcrumbs.Response)
    - [Rpc.Block.OpenBreadcrumbs.Response.Error](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error)
    - [Rpc.Block.Paste](#anytype.Rpc.Block.Paste)
    - [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request)
    - [Rpc.Block.Paste.Request.File](#anytype.Rpc.Block.Paste.Request.File)
    - [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response)
    - [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error)
    - [Rpc.Block.Redo](#anytype.Rpc.Block.Redo)
    - [Rpc.Block.Redo.Request](#anytype.Rpc.Block.Redo.Request)
    - [Rpc.Block.Redo.Response](#anytype.Rpc.Block.Redo.Response)
    - [Rpc.Block.Redo.Response.Error](#anytype.Rpc.Block.Redo.Response.Error)
    - [Rpc.Block.Relation](#anytype.Rpc.Block.Relation)
    - [Rpc.Block.Relation.Add](#anytype.Rpc.Block.Relation.Add)
    - [Rpc.Block.Relation.Add.Request](#anytype.Rpc.Block.Relation.Add.Request)
    - [Rpc.Block.Relation.Add.Response](#anytype.Rpc.Block.Relation.Add.Response)
    - [Rpc.Block.Relation.Add.Response.Error](#anytype.Rpc.Block.Relation.Add.Response.Error)
    - [Rpc.Block.Relation.SetKey](#anytype.Rpc.Block.Relation.SetKey)
    - [Rpc.Block.Relation.SetKey.Request](#anytype.Rpc.Block.Relation.SetKey.Request)
    - [Rpc.Block.Relation.SetKey.Response](#anytype.Rpc.Block.Relation.SetKey.Response)
    - [Rpc.Block.Relation.SetKey.Response.Error](#anytype.Rpc.Block.Relation.SetKey.Response.Error)
    - [Rpc.Block.Replace](#anytype.Rpc.Block.Replace)
    - [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request)
    - [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response)
    - [Rpc.Block.Replace.Response.Error](#anytype.Rpc.Block.Replace.Response.Error)
    - [Rpc.Block.Set](#anytype.Rpc.Block.Set)
    - [Rpc.Block.Set.Details](#anytype.Rpc.Block.Set.Details)
    - [Rpc.Block.Set.Details.Detail](#anytype.Rpc.Block.Set.Details.Detail)
    - [Rpc.Block.Set.Details.Request](#anytype.Rpc.Block.Set.Details.Request)
    - [Rpc.Block.Set.Details.Response](#anytype.Rpc.Block.Set.Details.Response)
    - [Rpc.Block.Set.Details.Response.Error](#anytype.Rpc.Block.Set.Details.Response.Error)
    - [Rpc.Block.Set.Fields](#anytype.Rpc.Block.Set.Fields)
    - [Rpc.Block.Set.Fields.Request](#anytype.Rpc.Block.Set.Fields.Request)
    - [Rpc.Block.Set.Fields.Response](#anytype.Rpc.Block.Set.Fields.Response)
    - [Rpc.Block.Set.Fields.Response.Error](#anytype.Rpc.Block.Set.Fields.Response.Error)
    - [Rpc.Block.Set.File](#anytype.Rpc.Block.Set.File)
    - [Rpc.Block.Set.File.Name](#anytype.Rpc.Block.Set.File.Name)
    - [Rpc.Block.Set.File.Name.Request](#anytype.Rpc.Block.Set.File.Name.Request)
    - [Rpc.Block.Set.File.Name.Response](#anytype.Rpc.Block.Set.File.Name.Response)
    - [Rpc.Block.Set.File.Name.Response.Error](#anytype.Rpc.Block.Set.File.Name.Response.Error)
    - [Rpc.Block.Set.Image](#anytype.Rpc.Block.Set.Image)
    - [Rpc.Block.Set.Image.Name](#anytype.Rpc.Block.Set.Image.Name)
    - [Rpc.Block.Set.Image.Name.Request](#anytype.Rpc.Block.Set.Image.Name.Request)
    - [Rpc.Block.Set.Image.Name.Response](#anytype.Rpc.Block.Set.Image.Name.Response)
    - [Rpc.Block.Set.Image.Name.Response.Error](#anytype.Rpc.Block.Set.Image.Name.Response.Error)
    - [Rpc.Block.Set.Image.Width](#anytype.Rpc.Block.Set.Image.Width)
    - [Rpc.Block.Set.Image.Width.Request](#anytype.Rpc.Block.Set.Image.Width.Request)
    - [Rpc.Block.Set.Image.Width.Response](#anytype.Rpc.Block.Set.Image.Width.Response)
    - [Rpc.Block.Set.Image.Width.Response.Error](#anytype.Rpc.Block.Set.Image.Width.Response.Error)
    - [Rpc.Block.Set.Latex](#anytype.Rpc.Block.Set.Latex)
    - [Rpc.Block.Set.Latex.Text](#anytype.Rpc.Block.Set.Latex.Text)
    - [Rpc.Block.Set.Latex.Text.Request](#anytype.Rpc.Block.Set.Latex.Text.Request)
    - [Rpc.Block.Set.Latex.Text.Response](#anytype.Rpc.Block.Set.Latex.Text.Response)
    - [Rpc.Block.Set.Latex.Text.Response.Error](#anytype.Rpc.Block.Set.Latex.Text.Response.Error)
    - [Rpc.Block.Set.Link](#anytype.Rpc.Block.Set.Link)
    - [Rpc.Block.Set.Link.TargetBlockId](#anytype.Rpc.Block.Set.Link.TargetBlockId)
    - [Rpc.Block.Set.Link.TargetBlockId.Request](#anytype.Rpc.Block.Set.Link.TargetBlockId.Request)
    - [Rpc.Block.Set.Link.TargetBlockId.Response](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response)
    - [Rpc.Block.Set.Link.TargetBlockId.Response.Error](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error)
    - [Rpc.Block.Set.Page](#anytype.Rpc.Block.Set.Page)
    - [Rpc.Block.Set.Page.IsArchived](#anytype.Rpc.Block.Set.Page.IsArchived)
    - [Rpc.Block.Set.Page.IsArchived.Request](#anytype.Rpc.Block.Set.Page.IsArchived.Request)
    - [Rpc.Block.Set.Page.IsArchived.Response](#anytype.Rpc.Block.Set.Page.IsArchived.Response)
    - [Rpc.Block.Set.Page.IsArchived.Response.Error](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error)
    - [Rpc.Block.Set.Restrictions](#anytype.Rpc.Block.Set.Restrictions)
    - [Rpc.Block.Set.Restrictions.Request](#anytype.Rpc.Block.Set.Restrictions.Request)
    - [Rpc.Block.Set.Restrictions.Response](#anytype.Rpc.Block.Set.Restrictions.Response)
    - [Rpc.Block.Set.Restrictions.Response.Error](#anytype.Rpc.Block.Set.Restrictions.Response.Error)
    - [Rpc.Block.Set.Text](#anytype.Rpc.Block.Set.Text)
    - [Rpc.Block.Set.Text.Checked](#anytype.Rpc.Block.Set.Text.Checked)
    - [Rpc.Block.Set.Text.Checked.Request](#anytype.Rpc.Block.Set.Text.Checked.Request)
    - [Rpc.Block.Set.Text.Checked.Response](#anytype.Rpc.Block.Set.Text.Checked.Response)
    - [Rpc.Block.Set.Text.Checked.Response.Error](#anytype.Rpc.Block.Set.Text.Checked.Response.Error)
    - [Rpc.Block.Set.Text.Color](#anytype.Rpc.Block.Set.Text.Color)
    - [Rpc.Block.Set.Text.Color.Request](#anytype.Rpc.Block.Set.Text.Color.Request)
    - [Rpc.Block.Set.Text.Color.Response](#anytype.Rpc.Block.Set.Text.Color.Response)
    - [Rpc.Block.Set.Text.Color.Response.Error](#anytype.Rpc.Block.Set.Text.Color.Response.Error)
    - [Rpc.Block.Set.Text.Icon](#anytype.Rpc.Block.Set.Text.Icon)
    - [Rpc.Block.Set.Text.Icon.Request](#anytype.Rpc.Block.Set.Text.Icon.Request)
    - [Rpc.Block.Set.Text.Icon.Response](#anytype.Rpc.Block.Set.Text.Icon.Response)
    - [Rpc.Block.Set.Text.Icon.Response.Error](#anytype.Rpc.Block.Set.Text.Icon.Response.Error)
    - [Rpc.Block.Set.Text.Style](#anytype.Rpc.Block.Set.Text.Style)
    - [Rpc.Block.Set.Text.Style.Request](#anytype.Rpc.Block.Set.Text.Style.Request)
    - [Rpc.Block.Set.Text.Style.Response](#anytype.Rpc.Block.Set.Text.Style.Response)
    - [Rpc.Block.Set.Text.Style.Response.Error](#anytype.Rpc.Block.Set.Text.Style.Response.Error)
    - [Rpc.Block.Set.Text.Text](#anytype.Rpc.Block.Set.Text.Text)
    - [Rpc.Block.Set.Text.Text.Request](#anytype.Rpc.Block.Set.Text.Text.Request)
    - [Rpc.Block.Set.Text.Text.Response](#anytype.Rpc.Block.Set.Text.Text.Response)
    - [Rpc.Block.Set.Text.Text.Response.Error](#anytype.Rpc.Block.Set.Text.Text.Response.Error)
    - [Rpc.Block.Set.Video](#anytype.Rpc.Block.Set.Video)
    - [Rpc.Block.Set.Video.Name](#anytype.Rpc.Block.Set.Video.Name)
    - [Rpc.Block.Set.Video.Name.Request](#anytype.Rpc.Block.Set.Video.Name.Request)
    - [Rpc.Block.Set.Video.Name.Response](#anytype.Rpc.Block.Set.Video.Name.Response)
    - [Rpc.Block.Set.Video.Name.Response.Error](#anytype.Rpc.Block.Set.Video.Name.Response.Error)
    - [Rpc.Block.Set.Video.Width](#anytype.Rpc.Block.Set.Video.Width)
    - [Rpc.Block.Set.Video.Width.Request](#anytype.Rpc.Block.Set.Video.Width.Request)
    - [Rpc.Block.Set.Video.Width.Response](#anytype.Rpc.Block.Set.Video.Width.Response)
    - [Rpc.Block.Set.Video.Width.Response.Error](#anytype.Rpc.Block.Set.Video.Width.Response.Error)
    - [Rpc.Block.SetBreadcrumbs](#anytype.Rpc.Block.SetBreadcrumbs)
    - [Rpc.Block.SetBreadcrumbs.Request](#anytype.Rpc.Block.SetBreadcrumbs.Request)
    - [Rpc.Block.SetBreadcrumbs.Response](#anytype.Rpc.Block.SetBreadcrumbs.Response)
    - [Rpc.Block.SetBreadcrumbs.Response.Error](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error)
    - [Rpc.Block.Show](#anytype.Rpc.Block.Show)
    - [Rpc.Block.Show.Request](#anytype.Rpc.Block.Show.Request)
    - [Rpc.Block.Show.Response](#anytype.Rpc.Block.Show.Response)
    - [Rpc.Block.Show.Response.Error](#anytype.Rpc.Block.Show.Response.Error)
    - [Rpc.Block.Split](#anytype.Rpc.Block.Split)
    - [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request)
    - [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response)
    - [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error)
    - [Rpc.Block.Undo](#anytype.Rpc.Block.Undo)
    - [Rpc.Block.Undo.Request](#anytype.Rpc.Block.Undo.Request)
    - [Rpc.Block.Undo.Response](#anytype.Rpc.Block.Undo.Response)
    - [Rpc.Block.Undo.Response.Error](#anytype.Rpc.Block.Undo.Response.Error)
    - [Rpc.Block.UndoRedoCounter](#anytype.Rpc.Block.UndoRedoCounter)
    - [Rpc.Block.Unlink](#anytype.Rpc.Block.Unlink)
    - [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request)
    - [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response)
    - [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error)
    - [Rpc.Block.UpdateContent](#anytype.Rpc.Block.UpdateContent)
    - [Rpc.Block.UpdateContent.Request](#anytype.Rpc.Block.UpdateContent.Request)
    - [Rpc.Block.UpdateContent.Response](#anytype.Rpc.Block.UpdateContent.Response)
    - [Rpc.Block.UpdateContent.Response.Error](#anytype.Rpc.Block.UpdateContent.Response.Error)
    - [Rpc.Block.Upload](#anytype.Rpc.Block.Upload)
    - [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request)
    - [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response)
    - [Rpc.Block.Upload.Response.Error](#anytype.Rpc.Block.Upload.Response.Error)
    - [Rpc.BlockList](#anytype.Rpc.BlockList)
    - [Rpc.BlockList.ConvertChildrenToPages](#anytype.Rpc.BlockList.ConvertChildrenToPages)
    - [Rpc.BlockList.ConvertChildrenToPages.Request](#anytype.Rpc.BlockList.ConvertChildrenToPages.Request)
    - [Rpc.BlockList.ConvertChildrenToPages.Response](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response)
    - [Rpc.BlockList.ConvertChildrenToPages.Response.Error](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error)
    - [Rpc.BlockList.Duplicate](#anytype.Rpc.BlockList.Duplicate)
    - [Rpc.BlockList.Duplicate.Request](#anytype.Rpc.BlockList.Duplicate.Request)
    - [Rpc.BlockList.Duplicate.Response](#anytype.Rpc.BlockList.Duplicate.Response)
    - [Rpc.BlockList.Duplicate.Response.Error](#anytype.Rpc.BlockList.Duplicate.Response.Error)
    - [Rpc.BlockList.Move](#anytype.Rpc.BlockList.Move)
    - [Rpc.BlockList.Move.Request](#anytype.Rpc.BlockList.Move.Request)
    - [Rpc.BlockList.Move.Response](#anytype.Rpc.BlockList.Move.Response)
    - [Rpc.BlockList.Move.Response.Error](#anytype.Rpc.BlockList.Move.Response.Error)
    - [Rpc.BlockList.MoveToNewPage](#anytype.Rpc.BlockList.MoveToNewPage)
    - [Rpc.BlockList.MoveToNewPage.Request](#anytype.Rpc.BlockList.MoveToNewPage.Request)
    - [Rpc.BlockList.MoveToNewPage.Response](#anytype.Rpc.BlockList.MoveToNewPage.Response)
    - [Rpc.BlockList.MoveToNewPage.Response.Error](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error)
    - [Rpc.BlockList.Set](#anytype.Rpc.BlockList.Set)
    - [Rpc.BlockList.Set.Align](#anytype.Rpc.BlockList.Set.Align)
    - [Rpc.BlockList.Set.Align.Request](#anytype.Rpc.BlockList.Set.Align.Request)
    - [Rpc.BlockList.Set.Align.Response](#anytype.Rpc.BlockList.Set.Align.Response)
    - [Rpc.BlockList.Set.Align.Response.Error](#anytype.Rpc.BlockList.Set.Align.Response.Error)
    - [Rpc.BlockList.Set.BackgroundColor](#anytype.Rpc.BlockList.Set.BackgroundColor)
    - [Rpc.BlockList.Set.BackgroundColor.Request](#anytype.Rpc.BlockList.Set.BackgroundColor.Request)
    - [Rpc.BlockList.Set.BackgroundColor.Response](#anytype.Rpc.BlockList.Set.BackgroundColor.Response)
    - [Rpc.BlockList.Set.BackgroundColor.Response.Error](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error)
    - [Rpc.BlockList.Set.Div](#anytype.Rpc.BlockList.Set.Div)
    - [Rpc.BlockList.Set.Div.Style](#anytype.Rpc.BlockList.Set.Div.Style)
    - [Rpc.BlockList.Set.Div.Style.Request](#anytype.Rpc.BlockList.Set.Div.Style.Request)
    - [Rpc.BlockList.Set.Div.Style.Response](#anytype.Rpc.BlockList.Set.Div.Style.Response)
    - [Rpc.BlockList.Set.Div.Style.Response.Error](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error)
    - [Rpc.BlockList.Set.Fields](#anytype.Rpc.BlockList.Set.Fields)
    - [Rpc.BlockList.Set.Fields.Request](#anytype.Rpc.BlockList.Set.Fields.Request)
    - [Rpc.BlockList.Set.Fields.Request.BlockField](#anytype.Rpc.BlockList.Set.Fields.Request.BlockField)
    - [Rpc.BlockList.Set.Fields.Response](#anytype.Rpc.BlockList.Set.Fields.Response)
    - [Rpc.BlockList.Set.Fields.Response.Error](#anytype.Rpc.BlockList.Set.Fields.Response.Error)
    - [Rpc.BlockList.Set.File](#anytype.Rpc.BlockList.Set.File)
    - [Rpc.BlockList.Set.File.Style](#anytype.Rpc.BlockList.Set.File.Style)
    - [Rpc.BlockList.Set.File.Style.Request](#anytype.Rpc.BlockList.Set.File.Style.Request)
    - [Rpc.BlockList.Set.File.Style.Response](#anytype.Rpc.BlockList.Set.File.Style.Response)
    - [Rpc.BlockList.Set.File.Style.Response.Error](#anytype.Rpc.BlockList.Set.File.Style.Response.Error)
    - [Rpc.BlockList.Set.Text](#anytype.Rpc.BlockList.Set.Text)
    - [Rpc.BlockList.Set.Text.Color](#anytype.Rpc.BlockList.Set.Text.Color)
    - [Rpc.BlockList.Set.Text.Color.Request](#anytype.Rpc.BlockList.Set.Text.Color.Request)
    - [Rpc.BlockList.Set.Text.Color.Response](#anytype.Rpc.BlockList.Set.Text.Color.Response)
    - [Rpc.BlockList.Set.Text.Color.Response.Error](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error)
    - [Rpc.BlockList.Set.Text.Mark](#anytype.Rpc.BlockList.Set.Text.Mark)
    - [Rpc.BlockList.Set.Text.Mark.Request](#anytype.Rpc.BlockList.Set.Text.Mark.Request)
    - [Rpc.BlockList.Set.Text.Mark.Response](#anytype.Rpc.BlockList.Set.Text.Mark.Response)
    - [Rpc.BlockList.Set.Text.Mark.Response.Error](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error)
    - [Rpc.BlockList.Set.Text.Style](#anytype.Rpc.BlockList.Set.Text.Style)
    - [Rpc.BlockList.Set.Text.Style.Request](#anytype.Rpc.BlockList.Set.Text.Style.Request)
    - [Rpc.BlockList.Set.Text.Style.Response](#anytype.Rpc.BlockList.Set.Text.Style.Response)
    - [Rpc.BlockList.Set.Text.Style.Response.Error](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error)
    - [Rpc.BlockList.TurnInto](#anytype.Rpc.BlockList.TurnInto)
    - [Rpc.BlockList.TurnInto.Request](#anytype.Rpc.BlockList.TurnInto.Request)
    - [Rpc.BlockList.TurnInto.Response](#anytype.Rpc.BlockList.TurnInto.Response)
    - [Rpc.BlockList.TurnInto.Response.Error](#anytype.Rpc.BlockList.TurnInto.Response.Error)
    - [Rpc.CloneTemplate](#anytype.Rpc.CloneTemplate)
    - [Rpc.CloneTemplate.Request](#anytype.Rpc.CloneTemplate.Request)
    - [Rpc.CloneTemplate.Response](#anytype.Rpc.CloneTemplate.Response)
    - [Rpc.CloneTemplate.Response.Error](#anytype.Rpc.CloneTemplate.Response.Error)
    - [Rpc.Config](#anytype.Rpc.Config)
    - [Rpc.Config.Get](#anytype.Rpc.Config.Get)
    - [Rpc.Config.Get.Request](#anytype.Rpc.Config.Get.Request)
    - [Rpc.Config.Get.Response](#anytype.Rpc.Config.Get.Response)
    - [Rpc.Config.Get.Response.Error](#anytype.Rpc.Config.Get.Response.Error)
    - [Rpc.Debug](#anytype.Rpc.Debug)
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
    - [Rpc.DownloadFile](#anytype.Rpc.DownloadFile)
    - [Rpc.DownloadFile.Request](#anytype.Rpc.DownloadFile.Request)
    - [Rpc.DownloadFile.Response](#anytype.Rpc.DownloadFile.Response)
    - [Rpc.DownloadFile.Response.Error](#anytype.Rpc.DownloadFile.Response.Error)
    - [Rpc.Export](#anytype.Rpc.Export)
    - [Rpc.Export.Request](#anytype.Rpc.Export.Request)
    - [Rpc.Export.Response](#anytype.Rpc.Export.Response)
    - [Rpc.Export.Response.Error](#anytype.Rpc.Export.Response.Error)
    - [Rpc.ExportLocalstore](#anytype.Rpc.ExportLocalstore)
    - [Rpc.ExportLocalstore.Request](#anytype.Rpc.ExportLocalstore.Request)
    - [Rpc.ExportLocalstore.Response](#anytype.Rpc.ExportLocalstore.Response)
    - [Rpc.ExportLocalstore.Response.Error](#anytype.Rpc.ExportLocalstore.Response.Error)
    - [Rpc.ExportTemplates](#anytype.Rpc.ExportTemplates)
    - [Rpc.ExportTemplates.Request](#anytype.Rpc.ExportTemplates.Request)
    - [Rpc.ExportTemplates.Response](#anytype.Rpc.ExportTemplates.Response)
    - [Rpc.ExportTemplates.Response.Error](#anytype.Rpc.ExportTemplates.Response.Error)
    - [Rpc.ExportWorkspace](#anytype.Rpc.ExportWorkspace)
    - [Rpc.ExportWorkspace.Request](#anytype.Rpc.ExportWorkspace.Request)
    - [Rpc.ExportWorkspace.Response](#anytype.Rpc.ExportWorkspace.Response)
    - [Rpc.ExportWorkspace.Response.Error](#anytype.Rpc.ExportWorkspace.Response.Error)
    - [Rpc.ExternalDrop](#anytype.Rpc.ExternalDrop)
    - [Rpc.ExternalDrop.Content](#anytype.Rpc.ExternalDrop.Content)
    - [Rpc.ExternalDrop.Content.Request](#anytype.Rpc.ExternalDrop.Content.Request)
    - [Rpc.ExternalDrop.Content.Response](#anytype.Rpc.ExternalDrop.Content.Response)
    - [Rpc.ExternalDrop.Content.Response.Error](#anytype.Rpc.ExternalDrop.Content.Response.Error)
    - [Rpc.ExternalDrop.Files](#anytype.Rpc.ExternalDrop.Files)
    - [Rpc.ExternalDrop.Files.Request](#anytype.Rpc.ExternalDrop.Files.Request)
    - [Rpc.ExternalDrop.Files.Response](#anytype.Rpc.ExternalDrop.Files.Response)
    - [Rpc.ExternalDrop.Files.Response.Error](#anytype.Rpc.ExternalDrop.Files.Response.Error)
    - [Rpc.File](#anytype.Rpc.File)
    - [Rpc.File.Offload](#anytype.Rpc.File.Offload)
    - [Rpc.File.Offload.Request](#anytype.Rpc.File.Offload.Request)
    - [Rpc.File.Offload.Response](#anytype.Rpc.File.Offload.Response)
    - [Rpc.File.Offload.Response.Error](#anytype.Rpc.File.Offload.Response.Error)
    - [Rpc.FileList](#anytype.Rpc.FileList)
    - [Rpc.FileList.Offload](#anytype.Rpc.FileList.Offload)
    - [Rpc.FileList.Offload.Request](#anytype.Rpc.FileList.Offload.Request)
    - [Rpc.FileList.Offload.Response](#anytype.Rpc.FileList.Offload.Response)
    - [Rpc.FileList.Offload.Response.Error](#anytype.Rpc.FileList.Offload.Response.Error)
    - [Rpc.GenericErrorResponse](#anytype.Rpc.GenericErrorResponse)
    - [Rpc.GenericErrorResponse.Error](#anytype.Rpc.GenericErrorResponse.Error)
    - [Rpc.History](#anytype.Rpc.History)
    - [Rpc.History.SetVersion](#anytype.Rpc.History.SetVersion)
    - [Rpc.History.SetVersion.Request](#anytype.Rpc.History.SetVersion.Request)
    - [Rpc.History.SetVersion.Response](#anytype.Rpc.History.SetVersion.Response)
    - [Rpc.History.SetVersion.Response.Error](#anytype.Rpc.History.SetVersion.Response.Error)
    - [Rpc.History.Show](#anytype.Rpc.History.Show)
    - [Rpc.History.Show.Request](#anytype.Rpc.History.Show.Request)
    - [Rpc.History.Show.Response](#anytype.Rpc.History.Show.Response)
    - [Rpc.History.Show.Response.Error](#anytype.Rpc.History.Show.Response.Error)
    - [Rpc.History.Versions](#anytype.Rpc.History.Versions)
    - [Rpc.History.Versions.Request](#anytype.Rpc.History.Versions.Request)
    - [Rpc.History.Versions.Response](#anytype.Rpc.History.Versions.Response)
    - [Rpc.History.Versions.Response.Error](#anytype.Rpc.History.Versions.Response.Error)
    - [Rpc.History.Versions.Version](#anytype.Rpc.History.Versions.Version)
    - [Rpc.LinkPreview](#anytype.Rpc.LinkPreview)
    - [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request)
    - [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response)
    - [Rpc.LinkPreview.Response.Error](#anytype.Rpc.LinkPreview.Response.Error)
    - [Rpc.Log](#anytype.Rpc.Log)
    - [Rpc.Log.Send](#anytype.Rpc.Log.Send)
    - [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request)
    - [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response)
    - [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error)
    - [Rpc.MakeTemplate](#anytype.Rpc.MakeTemplate)
    - [Rpc.MakeTemplate.Request](#anytype.Rpc.MakeTemplate.Request)
    - [Rpc.MakeTemplate.Response](#anytype.Rpc.MakeTemplate.Response)
    - [Rpc.MakeTemplate.Response.Error](#anytype.Rpc.MakeTemplate.Response.Error)
    - [Rpc.MakeTemplateByObjectType](#anytype.Rpc.MakeTemplateByObjectType)
    - [Rpc.MakeTemplateByObjectType.Request](#anytype.Rpc.MakeTemplateByObjectType.Request)
    - [Rpc.MakeTemplateByObjectType.Response](#anytype.Rpc.MakeTemplateByObjectType.Response)
    - [Rpc.MakeTemplateByObjectType.Response.Error](#anytype.Rpc.MakeTemplateByObjectType.Response.Error)
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
    - [Rpc.Object.FeaturedRelation](#anytype.Rpc.Object.FeaturedRelation)
    - [Rpc.Object.FeaturedRelation.Add](#anytype.Rpc.Object.FeaturedRelation.Add)
    - [Rpc.Object.FeaturedRelation.Add.Request](#anytype.Rpc.Object.FeaturedRelation.Add.Request)
    - [Rpc.Object.FeaturedRelation.Add.Response](#anytype.Rpc.Object.FeaturedRelation.Add.Response)
    - [Rpc.Object.FeaturedRelation.Add.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error)
    - [Rpc.Object.FeaturedRelation.Remove](#anytype.Rpc.Object.FeaturedRelation.Remove)
    - [Rpc.Object.FeaturedRelation.Remove.Request](#anytype.Rpc.Object.FeaturedRelation.Remove.Request)
    - [Rpc.Object.FeaturedRelation.Remove.Response](#anytype.Rpc.Object.FeaturedRelation.Remove.Response)
    - [Rpc.Object.FeaturedRelation.Remove.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error)
    - [Rpc.Object.Graph](#anytype.Rpc.Object.Graph)
    - [Rpc.Object.Graph.Edge](#anytype.Rpc.Object.Graph.Edge)
    - [Rpc.Object.Graph.Node](#anytype.Rpc.Object.Graph.Node)
    - [Rpc.Object.Graph.Request](#anytype.Rpc.Object.Graph.Request)
    - [Rpc.Object.Graph.Response](#anytype.Rpc.Object.Graph.Response)
    - [Rpc.Object.Graph.Response.Error](#anytype.Rpc.Object.Graph.Response.Error)
    - [Rpc.Object.IdsSubscribe](#anytype.Rpc.Object.IdsSubscribe)
    - [Rpc.Object.IdsSubscribe.Request](#anytype.Rpc.Object.IdsSubscribe.Request)
    - [Rpc.Object.IdsSubscribe.Response](#anytype.Rpc.Object.IdsSubscribe.Response)
    - [Rpc.Object.IdsSubscribe.Response.Error](#anytype.Rpc.Object.IdsSubscribe.Response.Error)
    - [Rpc.Object.RelationAdd](#anytype.Rpc.Object.RelationAdd)
    - [Rpc.Object.RelationAdd.Request](#anytype.Rpc.Object.RelationAdd.Request)
    - [Rpc.Object.RelationAdd.Response](#anytype.Rpc.Object.RelationAdd.Response)
    - [Rpc.Object.RelationAdd.Response.Error](#anytype.Rpc.Object.RelationAdd.Response.Error)
    - [Rpc.Object.RelationDelete](#anytype.Rpc.Object.RelationDelete)
    - [Rpc.Object.RelationDelete.Request](#anytype.Rpc.Object.RelationDelete.Request)
    - [Rpc.Object.RelationDelete.Response](#anytype.Rpc.Object.RelationDelete.Response)
    - [Rpc.Object.RelationDelete.Response.Error](#anytype.Rpc.Object.RelationDelete.Response.Error)
    - [Rpc.Object.RelationListAvailable](#anytype.Rpc.Object.RelationListAvailable)
    - [Rpc.Object.RelationListAvailable.Request](#anytype.Rpc.Object.RelationListAvailable.Request)
    - [Rpc.Object.RelationListAvailable.Response](#anytype.Rpc.Object.RelationListAvailable.Response)
    - [Rpc.Object.RelationListAvailable.Response.Error](#anytype.Rpc.Object.RelationListAvailable.Response.Error)
    - [Rpc.Object.RelationOptionAdd](#anytype.Rpc.Object.RelationOptionAdd)
    - [Rpc.Object.RelationOptionAdd.Request](#anytype.Rpc.Object.RelationOptionAdd.Request)
    - [Rpc.Object.RelationOptionAdd.Response](#anytype.Rpc.Object.RelationOptionAdd.Response)
    - [Rpc.Object.RelationOptionAdd.Response.Error](#anytype.Rpc.Object.RelationOptionAdd.Response.Error)
    - [Rpc.Object.RelationOptionDelete](#anytype.Rpc.Object.RelationOptionDelete)
    - [Rpc.Object.RelationOptionDelete.Request](#anytype.Rpc.Object.RelationOptionDelete.Request)
    - [Rpc.Object.RelationOptionDelete.Response](#anytype.Rpc.Object.RelationOptionDelete.Response)
    - [Rpc.Object.RelationOptionDelete.Response.Error](#anytype.Rpc.Object.RelationOptionDelete.Response.Error)
    - [Rpc.Object.RelationOptionUpdate](#anytype.Rpc.Object.RelationOptionUpdate)
    - [Rpc.Object.RelationOptionUpdate.Request](#anytype.Rpc.Object.RelationOptionUpdate.Request)
    - [Rpc.Object.RelationOptionUpdate.Response](#anytype.Rpc.Object.RelationOptionUpdate.Response)
    - [Rpc.Object.RelationOptionUpdate.Response.Error](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error)
    - [Rpc.Object.RelationUpdate](#anytype.Rpc.Object.RelationUpdate)
    - [Rpc.Object.RelationUpdate.Request](#anytype.Rpc.Object.RelationUpdate.Request)
    - [Rpc.Object.RelationUpdate.Response](#anytype.Rpc.Object.RelationUpdate.Response)
    - [Rpc.Object.RelationUpdate.Response.Error](#anytype.Rpc.Object.RelationUpdate.Response.Error)
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
    - [Rpc.Object.ShareByLink](#anytype.Rpc.Object.ShareByLink)
    - [Rpc.Object.ShareByLink.Request](#anytype.Rpc.Object.ShareByLink.Request)
    - [Rpc.Object.ShareByLink.Response](#anytype.Rpc.Object.ShareByLink.Response)
    - [Rpc.Object.ShareByLink.Response.Error](#anytype.Rpc.Object.ShareByLink.Response.Error)
    - [Rpc.Object.ToSet](#anytype.Rpc.Object.ToSet)
    - [Rpc.Object.ToSet.Request](#anytype.Rpc.Object.ToSet.Request)
    - [Rpc.Object.ToSet.Response](#anytype.Rpc.Object.ToSet.Response)
    - [Rpc.Object.ToSet.Response.Error](#anytype.Rpc.Object.ToSet.Response.Error)
    - [Rpc.ObjectDuplicate](#anytype.Rpc.ObjectDuplicate)
    - [Rpc.ObjectDuplicate.Request](#anytype.Rpc.ObjectDuplicate.Request)
    - [Rpc.ObjectDuplicate.Response](#anytype.Rpc.ObjectDuplicate.Response)
    - [Rpc.ObjectDuplicate.Response.Error](#anytype.Rpc.ObjectDuplicate.Response.Error)
    - [Rpc.ObjectList](#anytype.Rpc.ObjectList)
    - [Rpc.ObjectList.Delete](#anytype.Rpc.ObjectList.Delete)
    - [Rpc.ObjectList.Delete.Request](#anytype.Rpc.ObjectList.Delete.Request)
    - [Rpc.ObjectList.Delete.Response](#anytype.Rpc.ObjectList.Delete.Response)
    - [Rpc.ObjectList.Delete.Response.Error](#anytype.Rpc.ObjectList.Delete.Response.Error)
    - [Rpc.ObjectList.Set](#anytype.Rpc.ObjectList.Set)
    - [Rpc.ObjectList.Set.IsArchived](#anytype.Rpc.ObjectList.Set.IsArchived)
    - [Rpc.ObjectList.Set.IsArchived.Request](#anytype.Rpc.ObjectList.Set.IsArchived.Request)
    - [Rpc.ObjectList.Set.IsArchived.Response](#anytype.Rpc.ObjectList.Set.IsArchived.Response)
    - [Rpc.ObjectList.Set.IsArchived.Response.Error](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error)
    - [Rpc.ObjectList.Set.IsFavorite](#anytype.Rpc.ObjectList.Set.IsFavorite)
    - [Rpc.ObjectList.Set.IsFavorite.Request](#anytype.Rpc.ObjectList.Set.IsFavorite.Request)
    - [Rpc.ObjectList.Set.IsFavorite.Response](#anytype.Rpc.ObjectList.Set.IsFavorite.Response)
    - [Rpc.ObjectList.Set.IsFavorite.Response.Error](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error)
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
    - [Rpc.Page](#anytype.Rpc.Page)
    - [Rpc.Page.Create](#anytype.Rpc.Page.Create)
    - [Rpc.Page.Create.Request](#anytype.Rpc.Page.Create.Request)
    - [Rpc.Page.Create.Response](#anytype.Rpc.Page.Create.Response)
    - [Rpc.Page.Create.Response.Error](#anytype.Rpc.Page.Create.Response.Error)
    - [Rpc.Ping](#anytype.Rpc.Ping)
    - [Rpc.Ping.Request](#anytype.Rpc.Ping.Request)
    - [Rpc.Ping.Response](#anytype.Rpc.Ping.Response)
    - [Rpc.Ping.Response.Error](#anytype.Rpc.Ping.Response.Error)
    - [Rpc.Process](#anytype.Rpc.Process)
    - [Rpc.Process.Cancel](#anytype.Rpc.Process.Cancel)
    - [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request)
    - [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response)
    - [Rpc.Process.Cancel.Response.Error](#anytype.Rpc.Process.Cancel.Response.Error)
    - [Rpc.Set](#anytype.Rpc.Set)
    - [Rpc.Set.Create](#anytype.Rpc.Set.Create)
    - [Rpc.Set.Create.Request](#anytype.Rpc.Set.Create.Request)
    - [Rpc.Set.Create.Response](#anytype.Rpc.Set.Create.Response)
    - [Rpc.Set.Create.Response.Error](#anytype.Rpc.Set.Create.Response.Error)
    - [Rpc.Shutdown](#anytype.Rpc.Shutdown)
    - [Rpc.Shutdown.Request](#anytype.Rpc.Shutdown.Request)
    - [Rpc.Shutdown.Response](#anytype.Rpc.Shutdown.Response)
    - [Rpc.Shutdown.Response.Error](#anytype.Rpc.Shutdown.Response.Error)
    - [Rpc.UnsplashDownload](#anytype.Rpc.UnsplashDownload)
    - [Rpc.UnsplashDownload.Request](#anytype.Rpc.UnsplashDownload.Request)
    - [Rpc.UnsplashDownload.Response](#anytype.Rpc.UnsplashDownload.Response)
    - [Rpc.UnsplashDownload.Response.Error](#anytype.Rpc.UnsplashDownload.Response.Error)
    - [Rpc.UnsplashSearch](#anytype.Rpc.UnsplashSearch)
    - [Rpc.UnsplashSearch.Request](#anytype.Rpc.UnsplashSearch.Request)
    - [Rpc.UnsplashSearch.Response](#anytype.Rpc.UnsplashSearch.Response)
    - [Rpc.UnsplashSearch.Response.Error](#anytype.Rpc.UnsplashSearch.Response.Error)
    - [Rpc.UnsplashSearch.Response.Picture](#anytype.Rpc.UnsplashSearch.Response.Picture)
    - [Rpc.UploadFile](#anytype.Rpc.UploadFile)
    - [Rpc.UploadFile.Request](#anytype.Rpc.UploadFile.Request)
    - [Rpc.UploadFile.Response](#anytype.Rpc.UploadFile.Response)
    - [Rpc.UploadFile.Response.Error](#anytype.Rpc.UploadFile.Response.Error)
    - [Rpc.Version](#anytype.Rpc.Version)
    - [Rpc.Version.Get](#anytype.Rpc.Version.Get)
    - [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request)
    - [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response)
    - [Rpc.Version.Get.Response.Error](#anytype.Rpc.Version.Get.Response.Error)
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
    - [Rpc.ApplyTemplate.Response.Error.Code](#anytype.Rpc.ApplyTemplate.Response.Error.Code)
    - [Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code)
    - [Rpc.Block.Bookmark.Fetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code)
    - [Rpc.Block.Close.Response.Error.Code](#anytype.Rpc.Block.Close.Response.Error.Code)
    - [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code)
    - [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code)
    - [Rpc.Block.CreatePage.Response.Error.Code](#anytype.Rpc.Block.CreatePage.Response.Error.Code)
    - [Rpc.Block.CreateSet.Response.Error.Code](#anytype.Rpc.Block.CreateSet.Response.Error.Code)
    - [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code)
    - [Rpc.Block.Dataview.RecordCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error.Code)
    - [Rpc.Block.Dataview.RecordDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error.Code)
    - [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code)
    - [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code)
    - [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code)
    - [Rpc.Block.Dataview.RecordUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error.Code)
    - [Rpc.Block.Dataview.RelationAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error.Code)
    - [Rpc.Block.Dataview.RelationDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error.Code)
    - [Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code)
    - [Rpc.Block.Dataview.RelationUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error.Code)
    - [Rpc.Block.Dataview.SetSource.Response.Error.Code](#anytype.Rpc.Block.Dataview.SetSource.Response.Error.Code)
    - [Rpc.Block.Dataview.ViewCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error.Code)
    - [Rpc.Block.Dataview.ViewDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error.Code)
    - [Rpc.Block.Dataview.ViewSetActive.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error.Code)
    - [Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code)
    - [Rpc.Block.Dataview.ViewUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error.Code)
    - [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code)
    - [Rpc.Block.Export.Response.Error.Code](#anytype.Rpc.Block.Export.Response.Error.Code)
    - [Rpc.Block.File.CreateAndUpload.Response.Error.Code](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code)
    - [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code)
    - [Rpc.Block.GetPublicWebURL.Response.Error.Code](#anytype.Rpc.Block.GetPublicWebURL.Response.Error.Code)
    - [Rpc.Block.ImportMarkdown.Response.Error.Code](#anytype.Rpc.Block.ImportMarkdown.Response.Error.Code)
    - [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code)
    - [Rpc.Block.ObjectType.Set.Response.Error.Code](#anytype.Rpc.Block.ObjectType.Set.Response.Error.Code)
    - [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code)
    - [Rpc.Block.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code)
    - [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code)
    - [Rpc.Block.Redo.Response.Error.Code](#anytype.Rpc.Block.Redo.Response.Error.Code)
    - [Rpc.Block.Relation.Add.Response.Error.Code](#anytype.Rpc.Block.Relation.Add.Response.Error.Code)
    - [Rpc.Block.Relation.SetKey.Response.Error.Code](#anytype.Rpc.Block.Relation.SetKey.Response.Error.Code)
    - [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code)
    - [Rpc.Block.Set.Details.Response.Error.Code](#anytype.Rpc.Block.Set.Details.Response.Error.Code)
    - [Rpc.Block.Set.Fields.Response.Error.Code](#anytype.Rpc.Block.Set.Fields.Response.Error.Code)
    - [Rpc.Block.Set.File.Name.Response.Error.Code](#anytype.Rpc.Block.Set.File.Name.Response.Error.Code)
    - [Rpc.Block.Set.Image.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Name.Response.Error.Code)
    - [Rpc.Block.Set.Image.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Width.Response.Error.Code)
    - [Rpc.Block.Set.Latex.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Latex.Text.Response.Error.Code)
    - [Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code)
    - [Rpc.Block.Set.Page.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code)
    - [Rpc.Block.Set.Restrictions.Response.Error.Code](#anytype.Rpc.Block.Set.Restrictions.Response.Error.Code)
    - [Rpc.Block.Set.Text.Checked.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code)
    - [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code)
    - [Rpc.Block.Set.Text.Icon.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Icon.Response.Error.Code)
    - [Rpc.Block.Set.Text.Style.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Style.Response.Error.Code)
    - [Rpc.Block.Set.Text.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Text.Response.Error.Code)
    - [Rpc.Block.Set.Video.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Name.Response.Error.Code)
    - [Rpc.Block.Set.Video.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Width.Response.Error.Code)
    - [Rpc.Block.SetBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error.Code)
    - [Rpc.Block.Show.Response.Error.Code](#anytype.Rpc.Block.Show.Response.Error.Code)
    - [Rpc.Block.Split.Request.Mode](#anytype.Rpc.Block.Split.Request.Mode)
    - [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code)
    - [Rpc.Block.Undo.Response.Error.Code](#anytype.Rpc.Block.Undo.Response.Error.Code)
    - [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code)
    - [Rpc.Block.UpdateContent.Response.Error.Code](#anytype.Rpc.Block.UpdateContent.Response.Error.Code)
    - [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code)
    - [Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code)
    - [Rpc.BlockList.Duplicate.Response.Error.Code](#anytype.Rpc.BlockList.Duplicate.Response.Error.Code)
    - [Rpc.BlockList.Move.Response.Error.Code](#anytype.Rpc.BlockList.Move.Response.Error.Code)
    - [Rpc.BlockList.MoveToNewPage.Response.Error.Code](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code)
    - [Rpc.BlockList.Set.Align.Response.Error.Code](#anytype.Rpc.BlockList.Set.Align.Response.Error.Code)
    - [Rpc.BlockList.Set.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code)
    - [Rpc.BlockList.Set.Div.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code)
    - [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code)
    - [Rpc.BlockList.Set.File.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.File.Style.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Mark.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code)
    - [Rpc.BlockList.TurnInto.Response.Error.Code](#anytype.Rpc.BlockList.TurnInto.Response.Error.Code)
    - [Rpc.CloneTemplate.Response.Error.Code](#anytype.Rpc.CloneTemplate.Response.Error.Code)
    - [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code)
    - [Rpc.Debug.Sync.Response.Error.Code](#anytype.Rpc.Debug.Sync.Response.Error.Code)
    - [Rpc.Debug.Thread.Response.Error.Code](#anytype.Rpc.Debug.Thread.Response.Error.Code)
    - [Rpc.Debug.Tree.Response.Error.Code](#anytype.Rpc.Debug.Tree.Response.Error.Code)
    - [Rpc.DownloadFile.Response.Error.Code](#anytype.Rpc.DownloadFile.Response.Error.Code)
    - [Rpc.Export.Format](#anytype.Rpc.Export.Format)
    - [Rpc.Export.Response.Error.Code](#anytype.Rpc.Export.Response.Error.Code)
    - [Rpc.ExportLocalstore.Response.Error.Code](#anytype.Rpc.ExportLocalstore.Response.Error.Code)
    - [Rpc.ExportTemplates.Response.Error.Code](#anytype.Rpc.ExportTemplates.Response.Error.Code)
    - [Rpc.ExportWorkspace.Response.Error.Code](#anytype.Rpc.ExportWorkspace.Response.Error.Code)
    - [Rpc.ExternalDrop.Content.Response.Error.Code](#anytype.Rpc.ExternalDrop.Content.Response.Error.Code)
    - [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code)
    - [Rpc.File.Offload.Response.Error.Code](#anytype.Rpc.File.Offload.Response.Error.Code)
    - [Rpc.FileList.Offload.Response.Error.Code](#anytype.Rpc.FileList.Offload.Response.Error.Code)
    - [Rpc.GenericErrorResponse.Error.Code](#anytype.Rpc.GenericErrorResponse.Error.Code)
    - [Rpc.History.SetVersion.Response.Error.Code](#anytype.Rpc.History.SetVersion.Response.Error.Code)
    - [Rpc.History.Show.Response.Error.Code](#anytype.Rpc.History.Show.Response.Error.Code)
    - [Rpc.History.Versions.Response.Error.Code](#anytype.Rpc.History.Versions.Response.Error.Code)
    - [Rpc.LinkPreview.Response.Error.Code](#anytype.Rpc.LinkPreview.Response.Error.Code)
    - [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code)
    - [Rpc.MakeTemplate.Response.Error.Code](#anytype.Rpc.MakeTemplate.Response.Error.Code)
    - [Rpc.MakeTemplateByObjectType.Response.Error.Code](#anytype.Rpc.MakeTemplateByObjectType.Response.Error.Code)
    - [Rpc.Metrics.SetParameters.Response.Error.Code](#anytype.Rpc.Metrics.SetParameters.Response.Error.Code)
    - [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context)
    - [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code)
    - [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype.Rpc.Navigation.ListObjects.Response.Error.Code)
    - [Rpc.Object.AddWithObjectId.Response.Error.Code](#anytype.Rpc.Object.AddWithObjectId.Response.Error.Code)
    - [Rpc.Object.FeaturedRelation.Add.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error.Code)
    - [Rpc.Object.FeaturedRelation.Remove.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error.Code)
    - [Rpc.Object.Graph.Edge.Type](#anytype.Rpc.Object.Graph.Edge.Type)
    - [Rpc.Object.Graph.Response.Error.Code](#anytype.Rpc.Object.Graph.Response.Error.Code)
    - [Rpc.Object.IdsSubscribe.Response.Error.Code](#anytype.Rpc.Object.IdsSubscribe.Response.Error.Code)
    - [Rpc.Object.RelationAdd.Response.Error.Code](#anytype.Rpc.Object.RelationAdd.Response.Error.Code)
    - [Rpc.Object.RelationDelete.Response.Error.Code](#anytype.Rpc.Object.RelationDelete.Response.Error.Code)
    - [Rpc.Object.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Object.RelationListAvailable.Response.Error.Code)
    - [Rpc.Object.RelationOptionAdd.Response.Error.Code](#anytype.Rpc.Object.RelationOptionAdd.Response.Error.Code)
    - [Rpc.Object.RelationOptionDelete.Response.Error.Code](#anytype.Rpc.Object.RelationOptionDelete.Response.Error.Code)
    - [Rpc.Object.RelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error.Code)
    - [Rpc.Object.RelationUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationUpdate.Response.Error.Code)
    - [Rpc.Object.Search.Response.Error.Code](#anytype.Rpc.Object.Search.Response.Error.Code)
    - [Rpc.Object.SearchSubscribe.Response.Error.Code](#anytype.Rpc.Object.SearchSubscribe.Response.Error.Code)
    - [Rpc.Object.SearchUnsubscribe.Response.Error.Code](#anytype.Rpc.Object.SearchUnsubscribe.Response.Error.Code)
    - [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype.Rpc.Object.SetIsArchived.Response.Error.Code)
    - [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.SetIsFavorite.Response.Error.Code)
    - [Rpc.Object.SetLayout.Response.Error.Code](#anytype.Rpc.Object.SetLayout.Response.Error.Code)
    - [Rpc.Object.ShareByLink.Response.Error.Code](#anytype.Rpc.Object.ShareByLink.Response.Error.Code)
    - [Rpc.Object.ToSet.Response.Error.Code](#anytype.Rpc.Object.ToSet.Response.Error.Code)
    - [Rpc.ObjectDuplicate.Response.Error.Code](#anytype.Rpc.ObjectDuplicate.Response.Error.Code)
    - [Rpc.ObjectList.Delete.Response.Error.Code](#anytype.Rpc.ObjectList.Delete.Response.Error.Code)
    - [Rpc.ObjectList.Set.IsArchived.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error.Code)
    - [Rpc.ObjectList.Set.IsFavorite.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error.Code)
    - [Rpc.ObjectType.Create.Response.Error.Code](#anytype.Rpc.ObjectType.Create.Response.Error.Code)
    - [Rpc.ObjectType.List.Response.Error.Code](#anytype.Rpc.ObjectType.List.Response.Error.Code)
    - [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code)
    - [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.List.Response.Error.Code)
    - [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code)
    - [Rpc.ObjectType.Relation.Update.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code)
    - [Rpc.Page.Create.Response.Error.Code](#anytype.Rpc.Page.Create.Response.Error.Code)
    - [Rpc.Ping.Response.Error.Code](#anytype.Rpc.Ping.Response.Error.Code)
    - [Rpc.Process.Cancel.Response.Error.Code](#anytype.Rpc.Process.Cancel.Response.Error.Code)
    - [Rpc.Set.Create.Response.Error.Code](#anytype.Rpc.Set.Create.Response.Error.Code)
    - [Rpc.Shutdown.Response.Error.Code](#anytype.Rpc.Shutdown.Response.Error.Code)
    - [Rpc.UnsplashDownload.Response.Error.Code](#anytype.Rpc.UnsplashDownload.Response.Error.Code)
    - [Rpc.UnsplashSearch.Response.Error.Code](#anytype.Rpc.UnsplashSearch.Response.Error.Code)
    - [Rpc.UploadFile.Response.Error.Code](#anytype.Rpc.UploadFile.Response.Error.Code)
    - [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code)
    - [Rpc.Wallet.Convert.Response.Error.Code](#anytype.Rpc.Wallet.Convert.Response.Error.Code)
    - [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code)
    - [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code)
    - [Rpc.Workspace.Create.Response.Error.Code](#anytype.Rpc.Workspace.Create.Response.Error.Code)
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
    - [Account.Info](#anytype.model.Account.Info)
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
  
    - [Account.Status](#anytype.model.Account.Status)
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
| ObjectAddWithObjectId | [Rpc.Object.AddWithObjectId.Request](#anytype.Rpc.Object.AddWithObjectId.Request) | [Rpc.Object.AddWithObjectId.Response](#anytype.Rpc.Object.AddWithObjectId.Response) |  |
| ObjectShareByLink | [Rpc.Object.ShareByLink.Request](#anytype.Rpc.Object.ShareByLink.Request) | [Rpc.Object.ShareByLink.Response](#anytype.Rpc.Object.ShareByLink.Response) |  |
| WalletCreate | [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request) | [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response) |  |
| WalletRecover | [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request) | [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response) |  |
| WalletConvert | [Rpc.Wallet.Convert.Request](#anytype.Rpc.Wallet.Convert.Request) | [Rpc.Wallet.Convert.Response](#anytype.Rpc.Wallet.Convert.Response) |  |
| WorkspaceCreate | [Rpc.Workspace.Create.Request](#anytype.Rpc.Workspace.Create.Request) | [Rpc.Workspace.Create.Response](#anytype.Rpc.Workspace.Create.Response) |  |
| WorkspaceSelect | [Rpc.Workspace.Select.Request](#anytype.Rpc.Workspace.Select.Request) | [Rpc.Workspace.Select.Response](#anytype.Rpc.Workspace.Select.Response) |  |
| WorkspaceGetCurrent | [Rpc.Workspace.GetCurrent.Request](#anytype.Rpc.Workspace.GetCurrent.Request) | [Rpc.Workspace.GetCurrent.Response](#anytype.Rpc.Workspace.GetCurrent.Response) |  |
| WorkspaceGetAll | [Rpc.Workspace.GetAll.Request](#anytype.Rpc.Workspace.GetAll.Request) | [Rpc.Workspace.GetAll.Response](#anytype.Rpc.Workspace.GetAll.Response) |  |
| WorkspaceSetIsHighlighted | [Rpc.Workspace.SetIsHighlighted.Request](#anytype.Rpc.Workspace.SetIsHighlighted.Request) | [Rpc.Workspace.SetIsHighlighted.Response](#anytype.Rpc.Workspace.SetIsHighlighted.Response) |  |
| AccountRecover | [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request) | [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response) |  |
| AccountCreate | [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request) | [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response) |  |
| AccountDelete | [Rpc.Account.Delete.Request](#anytype.Rpc.Account.Delete.Request) | [Rpc.Account.Delete.Response](#anytype.Rpc.Account.Delete.Response) |  |
| AccountSelect | [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request) | [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response) |  |
| AccountStop | [Rpc.Account.Stop.Request](#anytype.Rpc.Account.Stop.Request) | [Rpc.Account.Stop.Response](#anytype.Rpc.Account.Stop.Response) |  |
| FileOffload | [Rpc.File.Offload.Request](#anytype.Rpc.File.Offload.Request) | [Rpc.File.Offload.Response](#anytype.Rpc.File.Offload.Response) |  |
| FileListOffload | [Rpc.FileList.Offload.Request](#anytype.Rpc.FileList.Offload.Request) | [Rpc.FileList.Offload.Response](#anytype.Rpc.FileList.Offload.Response) |  |
| VersionGet | [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request) | [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request) | [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response) |  |
| ConfigGet | [Rpc.Config.Get.Request](#anytype.Rpc.Config.Get.Request) | [Rpc.Config.Get.Response](#anytype.Rpc.Config.Get.Response) |  |
| Shutdown | [Rpc.Shutdown.Request](#anytype.Rpc.Shutdown.Request) | [Rpc.Shutdown.Response](#anytype.Rpc.Shutdown.Response) |  |
| ExternalDropFiles | [Rpc.ExternalDrop.Files.Request](#anytype.Rpc.ExternalDrop.Files.Request) | [Rpc.ExternalDrop.Files.Response](#anytype.Rpc.ExternalDrop.Files.Response) |  |
| ExternalDropContent | [Rpc.ExternalDrop.Content.Request](#anytype.Rpc.ExternalDrop.Content.Request) | [Rpc.ExternalDrop.Content.Response](#anytype.Rpc.ExternalDrop.Content.Response) |  |
| LinkPreview | [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request) | [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response) |  |
| UploadFile | [Rpc.UploadFile.Request](#anytype.Rpc.UploadFile.Request) | [Rpc.UploadFile.Response](#anytype.Rpc.UploadFile.Response) |  |
| DownloadFile | [Rpc.DownloadFile.Request](#anytype.Rpc.DownloadFile.Request) | [Rpc.DownloadFile.Response](#anytype.Rpc.DownloadFile.Response) |  |
| BlockUpload | [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request) | [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response) |  |
| BlockReplace | [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request) | [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response) |  |
| BlockUpdateContent | [Rpc.Block.UpdateContent.Request](#anytype.Rpc.Block.UpdateContent.Request) | [Rpc.Block.UpdateContent.Response](#anytype.Rpc.Block.UpdateContent.Response) | BlockUpdateContent allows to update any simple block content to the new value of the same type |
| BlockOpen | [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request) | [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response) |  |
| BlockShow | [Rpc.Block.Show.Request](#anytype.Rpc.Block.Show.Request) | [Rpc.Block.Show.Response](#anytype.Rpc.Block.Show.Response) |  |
| BlockGetPublicWebURL | [Rpc.Block.GetPublicWebURL.Request](#anytype.Rpc.Block.GetPublicWebURL.Request) | [Rpc.Block.GetPublicWebURL.Response](#anytype.Rpc.Block.GetPublicWebURL.Response) |  |
| BlockOpenBreadcrumbs | [Rpc.Block.OpenBreadcrumbs.Request](#anytype.Rpc.Block.OpenBreadcrumbs.Request) | [Rpc.Block.OpenBreadcrumbs.Response](#anytype.Rpc.Block.OpenBreadcrumbs.Response) |  |
| BlockSetBreadcrumbs | [Rpc.Block.SetBreadcrumbs.Request](#anytype.Rpc.Block.SetBreadcrumbs.Request) | [Rpc.Block.SetBreadcrumbs.Response](#anytype.Rpc.Block.SetBreadcrumbs.Response) |  |
| BlockCreate | [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request) | [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response) |  |
| BlockCreatePage | [Rpc.Block.CreatePage.Request](#anytype.Rpc.Block.CreatePage.Request) | [Rpc.Block.CreatePage.Response](#anytype.Rpc.Block.CreatePage.Response) |  |
| BlockCreateSet | [Rpc.Block.CreateSet.Request](#anytype.Rpc.Block.CreateSet.Request) | [Rpc.Block.CreateSet.Response](#anytype.Rpc.Block.CreateSet.Response) |  |
| BlockUnlink | [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request) | [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response) |  |
| BlockClose | [Rpc.Block.Close.Request](#anytype.Rpc.Block.Close.Request) | [Rpc.Block.Close.Response](#anytype.Rpc.Block.Close.Response) |  |
| BlockDownload | [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request) | [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response) |  |
| BlockGetMarks | [Rpc.Block.Get.Marks.Request](#anytype.Rpc.Block.Get.Marks.Request) | [Rpc.Block.Get.Marks.Response](#anytype.Rpc.Block.Get.Marks.Response) |  |
| BlockUndo | [Rpc.Block.Undo.Request](#anytype.Rpc.Block.Undo.Request) | [Rpc.Block.Undo.Response](#anytype.Rpc.Block.Undo.Response) |  |
| BlockRedo | [Rpc.Block.Redo.Request](#anytype.Rpc.Block.Redo.Request) | [Rpc.Block.Redo.Response](#anytype.Rpc.Block.Redo.Response) |  |
| BlockSetFields | [Rpc.Block.Set.Fields.Request](#anytype.Rpc.Block.Set.Fields.Request) | [Rpc.Block.Set.Fields.Response](#anytype.Rpc.Block.Set.Fields.Response) |  |
| BlockSetRestrictions | [Rpc.Block.Set.Restrictions.Request](#anytype.Rpc.Block.Set.Restrictions.Request) | [Rpc.Block.Set.Restrictions.Response](#anytype.Rpc.Block.Set.Restrictions.Response) |  |
| BlockListMove | [Rpc.BlockList.Move.Request](#anytype.Rpc.BlockList.Move.Request) | [Rpc.BlockList.Move.Response](#anytype.Rpc.BlockList.Move.Response) |  |
| BlockListMoveToNewPage | [Rpc.BlockList.MoveToNewPage.Request](#anytype.Rpc.BlockList.MoveToNewPage.Request) | [Rpc.BlockList.MoveToNewPage.Response](#anytype.Rpc.BlockList.MoveToNewPage.Response) |  |
| BlockListConvertChildrenToPages | [Rpc.BlockList.ConvertChildrenToPages.Request](#anytype.Rpc.BlockList.ConvertChildrenToPages.Request) | [Rpc.BlockList.ConvertChildrenToPages.Response](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response) |  |
| BlockListSetFields | [Rpc.BlockList.Set.Fields.Request](#anytype.Rpc.BlockList.Set.Fields.Request) | [Rpc.BlockList.Set.Fields.Response](#anytype.Rpc.BlockList.Set.Fields.Response) |  |
| BlockListSetTextStyle | [Rpc.BlockList.Set.Text.Style.Request](#anytype.Rpc.BlockList.Set.Text.Style.Request) | [Rpc.BlockList.Set.Text.Style.Response](#anytype.Rpc.BlockList.Set.Text.Style.Response) |  |
| BlockListDuplicate | [Rpc.BlockList.Duplicate.Request](#anytype.Rpc.BlockList.Duplicate.Request) | [Rpc.BlockList.Duplicate.Response](#anytype.Rpc.BlockList.Duplicate.Response) |  |
| BlockListSetBackgroundColor | [Rpc.BlockList.Set.BackgroundColor.Request](#anytype.Rpc.BlockList.Set.BackgroundColor.Request) | [Rpc.BlockList.Set.BackgroundColor.Response](#anytype.Rpc.BlockList.Set.BackgroundColor.Response) |  |
| BlockListSetAlign | [Rpc.BlockList.Set.Align.Request](#anytype.Rpc.BlockList.Set.Align.Request) | [Rpc.BlockList.Set.Align.Response](#anytype.Rpc.BlockList.Set.Align.Response) |  |
| BlockListSetDivStyle | [Rpc.BlockList.Set.Div.Style.Request](#anytype.Rpc.BlockList.Set.Div.Style.Request) | [Rpc.BlockList.Set.Div.Style.Response](#anytype.Rpc.BlockList.Set.Div.Style.Response) |  |
| BlockListSetFileStyle | [Rpc.BlockList.Set.File.Style.Request](#anytype.Rpc.BlockList.Set.File.Style.Request) | [Rpc.BlockList.Set.File.Style.Response](#anytype.Rpc.BlockList.Set.File.Style.Response) |  |
| BlockListTurnInto | [Rpc.BlockList.TurnInto.Request](#anytype.Rpc.BlockList.TurnInto.Request) | [Rpc.BlockList.TurnInto.Response](#anytype.Rpc.BlockList.TurnInto.Response) |  |
| BlockSetLatexText | [Rpc.Block.Set.Latex.Text.Request](#anytype.Rpc.Block.Set.Latex.Text.Request) | [Rpc.Block.Set.Latex.Text.Response](#anytype.Rpc.Block.Set.Latex.Text.Response) |  |
| BlockSetTextText | [Rpc.Block.Set.Text.Text.Request](#anytype.Rpc.Block.Set.Text.Text.Request) | [Rpc.Block.Set.Text.Text.Response](#anytype.Rpc.Block.Set.Text.Text.Response) |  |
| BlockSetTextColor | [Rpc.Block.Set.Text.Color.Request](#anytype.Rpc.Block.Set.Text.Color.Request) | [Rpc.Block.Set.Text.Color.Response](#anytype.Rpc.Block.Set.Text.Color.Response) |  |
| BlockListSetTextColor | [Rpc.BlockList.Set.Text.Color.Request](#anytype.Rpc.BlockList.Set.Text.Color.Request) | [Rpc.BlockList.Set.Text.Color.Response](#anytype.Rpc.BlockList.Set.Text.Color.Response) |  |
| BlockListSetTextMark | [Rpc.BlockList.Set.Text.Mark.Request](#anytype.Rpc.BlockList.Set.Text.Mark.Request) | [Rpc.BlockList.Set.Text.Mark.Response](#anytype.Rpc.BlockList.Set.Text.Mark.Response) |  |
| BlockSetTextStyle | [Rpc.Block.Set.Text.Style.Request](#anytype.Rpc.Block.Set.Text.Style.Request) | [Rpc.Block.Set.Text.Style.Response](#anytype.Rpc.Block.Set.Text.Style.Response) |  |
| BlockSetTextChecked | [Rpc.Block.Set.Text.Checked.Request](#anytype.Rpc.Block.Set.Text.Checked.Request) | [Rpc.Block.Set.Text.Checked.Response](#anytype.Rpc.Block.Set.Text.Checked.Response) |  |
| BlockSetTextIcon | [Rpc.Block.Set.Text.Icon.Request](#anytype.Rpc.Block.Set.Text.Icon.Request) | [Rpc.Block.Set.Text.Icon.Response](#anytype.Rpc.Block.Set.Text.Icon.Response) |  |
| BlockSplit | [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request) | [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response) |  |
| BlockMerge | [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request) | [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response) |  |
| BlockCopy | [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request) | [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response) |  |
| BlockPaste | [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request) | [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response) |  |
| BlockCut | [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request) | [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response) |  |
| BlockExport | [Rpc.Block.Export.Request](#anytype.Rpc.Block.Export.Request) | [Rpc.Block.Export.Response](#anytype.Rpc.Block.Export.Response) |  |
| BlockImportMarkdown | [Rpc.Block.ImportMarkdown.Request](#anytype.Rpc.Block.ImportMarkdown.Request) | [Rpc.Block.ImportMarkdown.Response](#anytype.Rpc.Block.ImportMarkdown.Response) |  |
| BlockSetFileName | [Rpc.Block.Set.File.Name.Request](#anytype.Rpc.Block.Set.File.Name.Request) | [Rpc.Block.Set.File.Name.Response](#anytype.Rpc.Block.Set.File.Name.Response) |  |
| BlockSetImageName | [Rpc.Block.Set.Image.Name.Request](#anytype.Rpc.Block.Set.Image.Name.Request) | [Rpc.Block.Set.Image.Name.Response](#anytype.Rpc.Block.Set.Image.Name.Response) |  |
| BlockSetImageWidth | [Rpc.Block.Set.Image.Width.Request](#anytype.Rpc.Block.Set.Image.Width.Request) | [Rpc.Block.Set.Image.Width.Response](#anytype.Rpc.Block.Set.Image.Width.Response) |  |
| BlockSetVideoName | [Rpc.Block.Set.Video.Name.Request](#anytype.Rpc.Block.Set.Video.Name.Request) | [Rpc.Block.Set.Video.Name.Response](#anytype.Rpc.Block.Set.Video.Name.Response) |  |
| BlockSetVideoWidth | [Rpc.Block.Set.Video.Width.Request](#anytype.Rpc.Block.Set.Video.Width.Request) | [Rpc.Block.Set.Video.Width.Response](#anytype.Rpc.Block.Set.Video.Width.Response) |  |
| BlockSetLinkTargetBlockId | [Rpc.Block.Set.Link.TargetBlockId.Request](#anytype.Rpc.Block.Set.Link.TargetBlockId.Request) | [Rpc.Block.Set.Link.TargetBlockId.Response](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response) |  |
| BlockBookmarkFetch | [Rpc.Block.Bookmark.Fetch.Request](#anytype.Rpc.Block.Bookmark.Fetch.Request) | [Rpc.Block.Bookmark.Fetch.Response](#anytype.Rpc.Block.Bookmark.Fetch.Response) |  |
| BlockBookmarkCreateAndFetch | [Rpc.Block.Bookmark.CreateAndFetch.Request](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Request) | [Rpc.Block.Bookmark.CreateAndFetch.Response](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response) |  |
| BlockFileCreateAndUpload | [Rpc.Block.File.CreateAndUpload.Request](#anytype.Rpc.Block.File.CreateAndUpload.Request) | [Rpc.Block.File.CreateAndUpload.Response](#anytype.Rpc.Block.File.CreateAndUpload.Response) |  |
| BlockRelationSetKey | [Rpc.Block.Relation.SetKey.Request](#anytype.Rpc.Block.Relation.SetKey.Request) | [Rpc.Block.Relation.SetKey.Response](#anytype.Rpc.Block.Relation.SetKey.Response) |  |
| BlockRelationAdd | [Rpc.Block.Relation.Add.Request](#anytype.Rpc.Block.Relation.Add.Request) | [Rpc.Block.Relation.Add.Response](#anytype.Rpc.Block.Relation.Add.Response) |  |
| BlockDataviewViewCreate | [Rpc.Block.Dataview.ViewCreate.Request](#anytype.Rpc.Block.Dataview.ViewCreate.Request) | [Rpc.Block.Dataview.ViewCreate.Response](#anytype.Rpc.Block.Dataview.ViewCreate.Response) | ## Dataview # View |
| BlockDataviewViewDelete | [Rpc.Block.Dataview.ViewDelete.Request](#anytype.Rpc.Block.Dataview.ViewDelete.Request) | [Rpc.Block.Dataview.ViewDelete.Response](#anytype.Rpc.Block.Dataview.ViewDelete.Response) |  |
| BlockDataviewViewUpdate | [Rpc.Block.Dataview.ViewUpdate.Request](#anytype.Rpc.Block.Dataview.ViewUpdate.Request) | [Rpc.Block.Dataview.ViewUpdate.Response](#anytype.Rpc.Block.Dataview.ViewUpdate.Response) |  |
| BlockDataviewViewSetActive | [Rpc.Block.Dataview.ViewSetActive.Request](#anytype.Rpc.Block.Dataview.ViewSetActive.Request) | [Rpc.Block.Dataview.ViewSetActive.Response](#anytype.Rpc.Block.Dataview.ViewSetActive.Response) |  |
| BlockDataviewViewSetPosition | [Rpc.Block.Dataview.ViewSetPosition.Request](#anytype.Rpc.Block.Dataview.ViewSetPosition.Request) | [Rpc.Block.Dataview.ViewSetPosition.Response](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response) |  |
| BlockDataviewSetSource | [Rpc.Block.Dataview.SetSource.Request](#anytype.Rpc.Block.Dataview.SetSource.Request) | [Rpc.Block.Dataview.SetSource.Response](#anytype.Rpc.Block.Dataview.SetSource.Response) |  |
| BlockDataviewRelationAdd | [Rpc.Block.Dataview.RelationAdd.Request](#anytype.Rpc.Block.Dataview.RelationAdd.Request) | [Rpc.Block.Dataview.RelationAdd.Response](#anytype.Rpc.Block.Dataview.RelationAdd.Response) | # Relation |
| BlockDataviewRelationUpdate | [Rpc.Block.Dataview.RelationUpdate.Request](#anytype.Rpc.Block.Dataview.RelationUpdate.Request) | [Rpc.Block.Dataview.RelationUpdate.Response](#anytype.Rpc.Block.Dataview.RelationUpdate.Response) |  |
| BlockDataviewRelationDelete | [Rpc.Block.Dataview.RelationDelete.Request](#anytype.Rpc.Block.Dataview.RelationDelete.Request) | [Rpc.Block.Dataview.RelationDelete.Response](#anytype.Rpc.Block.Dataview.RelationDelete.Response) |  |
| BlockDataviewRelationListAvailable | [Rpc.Block.Dataview.RelationListAvailable.Request](#anytype.Rpc.Block.Dataview.RelationListAvailable.Request) | [Rpc.Block.Dataview.RelationListAvailable.Response](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response) |  |
| BlockDataviewRecordCreate | [Rpc.Block.Dataview.RecordCreate.Request](#anytype.Rpc.Block.Dataview.RecordCreate.Request) | [Rpc.Block.Dataview.RecordCreate.Response](#anytype.Rpc.Block.Dataview.RecordCreate.Response) | # Record |
| BlockDataviewRecordUpdate | [Rpc.Block.Dataview.RecordUpdate.Request](#anytype.Rpc.Block.Dataview.RecordUpdate.Request) | [Rpc.Block.Dataview.RecordUpdate.Response](#anytype.Rpc.Block.Dataview.RecordUpdate.Response) |  |
| BlockDataviewRecordDelete | [Rpc.Block.Dataview.RecordDelete.Request](#anytype.Rpc.Block.Dataview.RecordDelete.Request) | [Rpc.Block.Dataview.RecordDelete.Response](#anytype.Rpc.Block.Dataview.RecordDelete.Response) |  |
| BlockDataviewRecordRelationOptionAdd | [Rpc.Block.Dataview.RecordRelationOptionAdd.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Request) | [Rpc.Block.Dataview.RecordRelationOptionAdd.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response) |  |
| BlockDataviewRecordRelationOptionUpdate | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Request) | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response) |  |
| BlockDataviewRecordRelationOptionDelete | [Rpc.Block.Dataview.RecordRelationOptionDelete.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Request) | [Rpc.Block.Dataview.RecordRelationOptionDelete.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response) |  |
| BlockObjectTypeSet | [Rpc.Block.ObjectType.Set.Request](#anytype.Rpc.Block.ObjectType.Set.Request) | [Rpc.Block.ObjectType.Set.Response](#anytype.Rpc.Block.ObjectType.Set.Response) | ## Object&#39;s relations set an existing object type to the object so it will appear in sets and suggests relations from this type TODO: rename BlockObjectTypeSet -&gt; ObjectObjectTypeSet |
| NavigationListObjects | [Rpc.Navigation.ListObjects.Request](#anytype.Rpc.Navigation.ListObjects.Request) | [Rpc.Navigation.ListObjects.Response](#anytype.Rpc.Navigation.ListObjects.Response) |  |
| NavigationGetObjectInfoWithLinks | [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request) | [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response) |  |
| ObjectGraph | [Rpc.Object.Graph.Request](#anytype.Rpc.Object.Graph.Request) | [Rpc.Object.Graph.Response](#anytype.Rpc.Object.Graph.Response) |  |
| ObjectSearch | [Rpc.Object.Search.Request](#anytype.Rpc.Object.Search.Request) | [Rpc.Object.Search.Response](#anytype.Rpc.Object.Search.Response) |  |
| ObjectSearchSubscribe | [Rpc.Object.SearchSubscribe.Request](#anytype.Rpc.Object.SearchSubscribe.Request) | [Rpc.Object.SearchSubscribe.Response](#anytype.Rpc.Object.SearchSubscribe.Response) |  |
| ObjectIdsSubscribe | [Rpc.Object.IdsSubscribe.Request](#anytype.Rpc.Object.IdsSubscribe.Request) | [Rpc.Object.IdsSubscribe.Response](#anytype.Rpc.Object.IdsSubscribe.Response) |  |
| ObjectSearchUnsubscribe | [Rpc.Object.SearchUnsubscribe.Request](#anytype.Rpc.Object.SearchUnsubscribe.Request) | [Rpc.Object.SearchUnsubscribe.Response](#anytype.Rpc.Object.SearchUnsubscribe.Response) |  |
| ObjectRelationAdd | [Rpc.Object.RelationAdd.Request](#anytype.Rpc.Object.RelationAdd.Request) | [Rpc.Object.RelationAdd.Response](#anytype.Rpc.Object.RelationAdd.Response) |  |
| ObjectRelationUpdate | [Rpc.Object.RelationUpdate.Request](#anytype.Rpc.Object.RelationUpdate.Request) | [Rpc.Object.RelationUpdate.Response](#anytype.Rpc.Object.RelationUpdate.Response) |  |
| ObjectRelationDelete | [Rpc.Object.RelationDelete.Request](#anytype.Rpc.Object.RelationDelete.Request) | [Rpc.Object.RelationDelete.Response](#anytype.Rpc.Object.RelationDelete.Response) |  |
| ObjectRelationOptionAdd | [Rpc.Object.RelationOptionAdd.Request](#anytype.Rpc.Object.RelationOptionAdd.Request) | [Rpc.Object.RelationOptionAdd.Response](#anytype.Rpc.Object.RelationOptionAdd.Response) |  |
| ObjectRelationOptionUpdate | [Rpc.Object.RelationOptionUpdate.Request](#anytype.Rpc.Object.RelationOptionUpdate.Request) | [Rpc.Object.RelationOptionUpdate.Response](#anytype.Rpc.Object.RelationOptionUpdate.Response) |  |
| ObjectRelationOptionDelete | [Rpc.Object.RelationOptionDelete.Request](#anytype.Rpc.Object.RelationOptionDelete.Request) | [Rpc.Object.RelationOptionDelete.Response](#anytype.Rpc.Object.RelationOptionDelete.Response) |  |
| ObjectRelationListAvailable | [Rpc.Object.RelationListAvailable.Request](#anytype.Rpc.Object.RelationListAvailable.Request) | [Rpc.Object.RelationListAvailable.Response](#anytype.Rpc.Object.RelationListAvailable.Response) |  |
| ObjectSetLayout | [Rpc.Object.SetLayout.Request](#anytype.Rpc.Object.SetLayout.Request) | [Rpc.Object.SetLayout.Response](#anytype.Rpc.Object.SetLayout.Response) |  |
| ObjectFeaturedRelationAdd | [Rpc.Object.FeaturedRelation.Add.Request](#anytype.Rpc.Object.FeaturedRelation.Add.Request) | [Rpc.Object.FeaturedRelation.Add.Response](#anytype.Rpc.Object.FeaturedRelation.Add.Response) |  |
| ObjectFeaturedRelationRemove | [Rpc.Object.FeaturedRelation.Remove.Request](#anytype.Rpc.Object.FeaturedRelation.Remove.Request) | [Rpc.Object.FeaturedRelation.Remove.Response](#anytype.Rpc.Object.FeaturedRelation.Remove.Response) |  |
| ObjectSetIsFavorite | [Rpc.Object.SetIsFavorite.Request](#anytype.Rpc.Object.SetIsFavorite.Request) | [Rpc.Object.SetIsFavorite.Response](#anytype.Rpc.Object.SetIsFavorite.Response) |  |
| ObjectSetIsArchived | [Rpc.Object.SetIsArchived.Request](#anytype.Rpc.Object.SetIsArchived.Request) | [Rpc.Object.SetIsArchived.Response](#anytype.Rpc.Object.SetIsArchived.Response) |  |
| ObjectToSet | [Rpc.Object.ToSet.Request](#anytype.Rpc.Object.ToSet.Request) | [Rpc.Object.ToSet.Response](#anytype.Rpc.Object.ToSet.Response) | ObjectToSet creates new set from given object and removes object |
| ObjectListDelete | [Rpc.ObjectList.Delete.Request](#anytype.Rpc.ObjectList.Delete.Request) | [Rpc.ObjectList.Delete.Response](#anytype.Rpc.ObjectList.Delete.Response) |  |
| ObjectListSetIsArchived | [Rpc.ObjectList.Set.IsArchived.Request](#anytype.Rpc.ObjectList.Set.IsArchived.Request) | [Rpc.ObjectList.Set.IsArchived.Response](#anytype.Rpc.ObjectList.Set.IsArchived.Response) |  |
| ObjectListSetIsFavorite | [Rpc.ObjectList.Set.IsFavorite.Request](#anytype.Rpc.ObjectList.Set.IsFavorite.Request) | [Rpc.ObjectList.Set.IsFavorite.Response](#anytype.Rpc.ObjectList.Set.IsFavorite.Response) |  |
| BlockSetDetails | [Rpc.Block.Set.Details.Request](#anytype.Rpc.Block.Set.Details.Request) | [Rpc.Block.Set.Details.Response](#anytype.Rpc.Block.Set.Details.Response) | TODO: rename BlockSetDetails -&gt; ObjectSetDetails |
| PageCreate | [Rpc.Page.Create.Request](#anytype.Rpc.Page.Create.Request) | [Rpc.Page.Create.Response](#anytype.Rpc.Page.Create.Response) | PageCreate just creates the new page, without adding the link to it from some other page TODO: rename PageCreate -&gt; ObjectCreate |
| SetCreate | [Rpc.Set.Create.Request](#anytype.Rpc.Set.Create.Request) | [Rpc.Set.Create.Response](#anytype.Rpc.Set.Create.Response) | SetCreate just creates the new set, without adding the link to it from some other page |
| MetricsSetParameters | [Rpc.Metrics.SetParameters.Request](#anytype.Rpc.Metrics.SetParameters.Request) | [Rpc.Metrics.SetParameters.Response](#anytype.Rpc.Metrics.SetParameters.Response) |  |
| ObjectTypeCreate | [Rpc.ObjectType.Create.Request](#anytype.Rpc.ObjectType.Create.Request) | [Rpc.ObjectType.Create.Response](#anytype.Rpc.ObjectType.Create.Response) | ## ObjectType |
| ObjectTypeList | [Rpc.ObjectType.List.Request](#anytype.Rpc.ObjectType.List.Request) | [Rpc.ObjectType.List.Response](#anytype.Rpc.ObjectType.List.Response) | ObjectTypeList lists all object types both bundled and created by user |
| ObjectTypeRelationList | [Rpc.ObjectType.Relation.List.Request](#anytype.Rpc.ObjectType.Relation.List.Request) | [Rpc.ObjectType.Relation.List.Response](#anytype.Rpc.ObjectType.Relation.List.Response) |  |
| ObjectTypeRelationAdd | [Rpc.ObjectType.Relation.Add.Request](#anytype.Rpc.ObjectType.Relation.Add.Request) | [Rpc.ObjectType.Relation.Add.Response](#anytype.Rpc.ObjectType.Relation.Add.Response) |  |
| ObjectTypeRelationUpdate | [Rpc.ObjectType.Relation.Update.Request](#anytype.Rpc.ObjectType.Relation.Update.Request) | [Rpc.ObjectType.Relation.Update.Response](#anytype.Rpc.ObjectType.Relation.Update.Response) |  |
| ObjectTypeRelationRemove | [Rpc.ObjectType.Relation.Remove.Request](#anytype.Rpc.ObjectType.Relation.Remove.Request) | [Rpc.ObjectType.Relation.Remove.Response](#anytype.Rpc.ObjectType.Relation.Remove.Response) |  |
| Ping | [Rpc.Ping.Request](#anytype.Rpc.Ping.Request) | [Rpc.Ping.Response](#anytype.Rpc.Ping.Response) |  |
| ProcessCancel | [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request) | [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response) |  |
| HistoryShow | [Rpc.History.Show.Request](#anytype.Rpc.History.Show.Request) | [Rpc.History.Show.Response](#anytype.Rpc.History.Show.Response) |  |
| HistoryVersions | [Rpc.History.Versions.Request](#anytype.Rpc.History.Versions.Request) | [Rpc.History.Versions.Response](#anytype.Rpc.History.Versions.Response) |  |
| HistorySetVersion | [Rpc.History.SetVersion.Request](#anytype.Rpc.History.SetVersion.Request) | [Rpc.History.SetVersion.Response](#anytype.Rpc.History.SetVersion.Response) |  |
| Export | [Rpc.Export.Request](#anytype.Rpc.Export.Request) | [Rpc.Export.Response](#anytype.Rpc.Export.Response) |  |
| ExportWorkspace | [Rpc.ExportWorkspace.Request](#anytype.Rpc.ExportWorkspace.Request) | [Rpc.ExportWorkspace.Response](#anytype.Rpc.ExportWorkspace.Response) |  |
| ExportTemplates | [Rpc.ExportTemplates.Request](#anytype.Rpc.ExportTemplates.Request) | [Rpc.ExportTemplates.Response](#anytype.Rpc.ExportTemplates.Response) |  |
| ExportLocalstore | [Rpc.ExportLocalstore.Request](#anytype.Rpc.ExportLocalstore.Request) | [Rpc.ExportLocalstore.Response](#anytype.Rpc.ExportLocalstore.Response) |  |
| MakeTemplate | [Rpc.MakeTemplate.Request](#anytype.Rpc.MakeTemplate.Request) | [Rpc.MakeTemplate.Response](#anytype.Rpc.MakeTemplate.Response) |  |
| MakeTemplateByObjectType | [Rpc.MakeTemplateByObjectType.Request](#anytype.Rpc.MakeTemplateByObjectType.Request) | [Rpc.MakeTemplateByObjectType.Response](#anytype.Rpc.MakeTemplateByObjectType.Response) |  |
| CloneTemplate | [Rpc.CloneTemplate.Request](#anytype.Rpc.CloneTemplate.Request) | [Rpc.CloneTemplate.Response](#anytype.Rpc.CloneTemplate.Response) |  |
| ObjectDuplicate | [Rpc.ObjectDuplicate.Request](#anytype.Rpc.ObjectDuplicate.Request) | [Rpc.ObjectDuplicate.Response](#anytype.Rpc.ObjectDuplicate.Response) |  |
| UnsplashSearch | [Rpc.UnsplashSearch.Request](#anytype.Rpc.UnsplashSearch.Request) | [Rpc.UnsplashSearch.Response](#anytype.Rpc.UnsplashSearch.Response) |  |
| UnsplashDownload | [Rpc.UnsplashDownload.Request](#anytype.Rpc.UnsplashDownload.Request) | [Rpc.UnsplashDownload.Response](#anytype.Rpc.UnsplashDownload.Response) | UnsplashDownload downloads picture from unsplash by ID, put it to the IPFS and returns the hash. The artist info is available in the object details |
| ApplyTemplate | [Rpc.ApplyTemplate.Request](#anytype.Rpc.ApplyTemplate.Request) | [Rpc.ApplyTemplate.Response](#anytype.Rpc.ApplyTemplate.Response) |  |
| DebugSync | [Rpc.Debug.Sync.Request](#anytype.Rpc.Debug.Sync.Request) | [Rpc.Debug.Sync.Response](#anytype.Rpc.Debug.Sync.Response) |  |
| DebugThread | [Rpc.Debug.Thread.Request](#anytype.Rpc.Debug.Thread.Request) | [Rpc.Debug.Thread.Response](#anytype.Rpc.Debug.Thread.Response) |  |
| DebugTree | [Rpc.Debug.Tree.Request](#anytype.Rpc.Debug.Tree.Request) | [Rpc.Debug.Tree.Response](#anytype.Rpc.Debug.Tree.Response) |  |
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
Namespace, that agregates subtopics and actions, that relates to account.






<a name="anytype.Rpc.Account.Config"></a>

### Rpc.Account.Config
TODO: use model from models.proto


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
| config | [Rpc.Account.Config](#anytype.Rpc.Account.Config) |  |  |
| info | [model.Account.Info](#anytype.model.Account.Info) |  |  |






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
| info | [model.Account.Info](#anytype.model.Account.Info) |  |  |






<a name="anytype.Rpc.Account.Delete.Response.Error"></a>

### Rpc.Account.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Delete.Response.Error.Code](#anytype.Rpc.Account.Delete.Response.Error.Code) |  |  |
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
| account | [model.Account](#anytype.model.Account) |  | Selected account |
| config | [Rpc.Account.Config](#anytype.Rpc.Account.Config) |  |  |






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






<a name="anytype.Rpc.ApplyTemplate"></a>

### Rpc.ApplyTemplate







<a name="anytype.Rpc.ApplyTemplate.Request"></a>

### Rpc.ApplyTemplate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| templateId | [string](#string) |  | id of template |






<a name="anytype.Rpc.ApplyTemplate.Response"></a>

### Rpc.ApplyTemplate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ApplyTemplate.Response.Error](#anytype.Rpc.ApplyTemplate.Response.Error) |  |  |






<a name="anytype.Rpc.ApplyTemplate.Response.Error"></a>

### Rpc.ApplyTemplate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ApplyTemplate.Response.Error.Code](#anytype.Rpc.ApplyTemplate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block"></a>

### Rpc.Block
Namespace, that agregates subtopics and actions, that relates to blocks.






<a name="anytype.Rpc.Block.Bookmark"></a>

### Rpc.Block.Bookmark







<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch"></a>

### Rpc.Block.Bookmark.CreateAndFetch







<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Request"></a>

### Rpc.Block.Bookmark.CreateAndFetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Response"></a>

### Rpc.Block.Bookmark.CreateAndFetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Bookmark.CreateAndFetch.Response.Error](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error"></a>

### Rpc.Block.Bookmark.CreateAndFetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Bookmark.Fetch"></a>

### Rpc.Block.Bookmark.Fetch







<a name="anytype.Rpc.Block.Bookmark.Fetch.Request"></a>

### Rpc.Block.Bookmark.Fetch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Bookmark.Fetch.Response"></a>

### Rpc.Block.Bookmark.Fetch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Bookmark.Fetch.Response.Error](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Bookmark.Fetch.Response.Error"></a>

### Rpc.Block.Bookmark.Fetch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Bookmark.Fetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Close"></a>

### Rpc.Block.Close
Block.Close – it means unsubscribe from a block.
Precondition: block should be opened.






<a name="anytype.Rpc.Block.Close.Request"></a>

### Rpc.Block.Close.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| blockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Close.Response"></a>

### Rpc.Block.Close.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Close.Response.Error](#anytype.Rpc.Block.Close.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Close.Response.Error"></a>

### Rpc.Block.Close.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Close.Response.Error.Code](#anytype.Rpc.Block.Close.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






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






<a name="anytype.Rpc.Block.CreatePage"></a>

### Rpc.Block.CreatePage







<a name="anytype.Rpc.Block.CreatePage.Request"></a>

### Rpc.Block.CreatePage.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | new page details |
| templateId | [string](#string) |  | optional template id for creating from template |
| targetId | [string](#string) |  | link block params

id of the closest simple block |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  | link block fields |






<a name="anytype.Rpc.Block.CreatePage.Response"></a>

### Rpc.Block.CreatePage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CreatePage.Response.Error](#anytype.Rpc.Block.CreatePage.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.CreatePage.Response.Error"></a>

### Rpc.Block.CreatePage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CreatePage.Response.Error.Code](#anytype.Rpc.Block.CreatePage.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.CreateSet"></a>

### Rpc.Block.CreateSet







<a name="anytype.Rpc.Block.CreateSet.Request"></a>

### Rpc.Block.CreateSet.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| targetId | [string](#string) |  | id of the closest block |
| source | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | details |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.Block.CreateSet.Response"></a>

### Rpc.Block.CreateSet.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CreateSet.Response.Error](#anytype.Rpc.Block.CreateSet.Response.Error) |  |  |
| blockId | [string](#string) |  | (optional) id of the link block pointing to this set |
| targetId | [string](#string) |  | id of the new set |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.CreateSet.Response.Error"></a>

### Rpc.Block.CreateSet.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CreateSet.Response.Error.Code](#anytype.Rpc.Block.CreateSet.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Dataview"></a>

### Rpc.Block.Dataview







<a name="anytype.Rpc.Block.Dataview.RecordCreate"></a>

### Rpc.Block.Dataview.RecordCreate







<a name="anytype.Rpc.Block.Dataview.RecordCreate.Request"></a>

### Rpc.Block.Dataview.RecordCreate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| record | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| templateId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordCreate.Response"></a>

### Rpc.Block.Dataview.RecordCreate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordCreate.Response.Error](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error) |  |  |
| record | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordCreate.Response.Error"></a>

### Rpc.Block.Dataview.RecordCreate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordDelete"></a>

### Rpc.Block.Dataview.RecordDelete







<a name="anytype.Rpc.Block.Dataview.RecordDelete.Request"></a>

### Rpc.Block.Dataview.RecordDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| recordId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordDelete.Response"></a>

### Rpc.Block.Dataview.RecordDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordDelete.Response.Error"></a>

### Rpc.Block.Dataview.RecordDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd"></a>

### Rpc.Block.Dataview.RecordRelationOptionAdd
RecordRelationOptionAdd may return existing option in case object specified with recordId already have the option with the same name or ID






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Request"></a>

### Rpc.Block.Dataview.RecordRelationOptionAdd.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |
| recordId | [string](#string) |  | id of record which is used to add an option |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response"></a>

### Rpc.Block.Dataview.RecordRelationOptionAdd.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error"></a>

### Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete"></a>

### Rpc.Block.Dataview.RecordRelationOptionDelete







<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Request"></a>

### Rpc.Block.Dataview.RecordRelationOptionDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | relation key to add the option |
| optionId | [string](#string) |  | id of select options to remove |
| recordId | [string](#string) |  | id of record which is used to delete an option |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response"></a>

### Rpc.Block.Dataview.RecordRelationOptionDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error"></a>

### Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate"></a>

### Rpc.Block.Dataview.RecordRelationOptionUpdate







<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Request"></a>

### Rpc.Block.Dataview.RecordRelationOptionUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |
| recordId | [string](#string) |  | id of record which is used to update an option |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response"></a>

### Rpc.Block.Dataview.RecordRelationOptionUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error"></a>

### Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordUpdate"></a>

### Rpc.Block.Dataview.RecordUpdate







<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Request"></a>

### Rpc.Block.Dataview.RecordUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| recordId | [string](#string) |  |  |
| record | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Response"></a>

### Rpc.Block.Dataview.RecordUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error"></a>

### Rpc.Block.Dataview.RecordUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationAdd"></a>

### Rpc.Block.Dataview.RelationAdd







<a name="anytype.Rpc.Block.Dataview.RelationAdd.Request"></a>

### Rpc.Block.Dataview.RelationAdd.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationAdd.Response"></a>

### Rpc.Block.Dataview.RelationAdd.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationAdd.Response.Error](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| relationKey | [string](#string) |  | deprecated |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationAdd.Response.Error"></a>

### Rpc.Block.Dataview.RelationAdd.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationDelete"></a>

### Rpc.Block.Dataview.RelationDelete







<a name="anytype.Rpc.Block.Dataview.RelationDelete.Request"></a>

### Rpc.Block.Dataview.RelationDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationDelete.Response"></a>

### Rpc.Block.Dataview.RelationDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationDelete.Response.Error](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationDelete.Response.Error"></a>

### Rpc.Block.Dataview.RelationDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationListAvailable"></a>

### Rpc.Block.Dataview.RelationListAvailable







<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Request"></a>

### Rpc.Block.Dataview.RelationListAvailable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Response"></a>

### Rpc.Block.Dataview.RelationListAvailable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationListAvailable.Response.Error](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error"></a>

### Rpc.Block.Dataview.RelationListAvailable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationUpdate"></a>

### Rpc.Block.Dataview.RelationUpdate







<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Request"></a>

### Rpc.Block.Dataview.RelationUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to add relation |
| relationKey | [string](#string) |  | key of relation to update |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Response"></a>

### Rpc.Block.Dataview.RelationUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error"></a>

### Rpc.Block.Dataview.RelationUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.SetSource"></a>

### Rpc.Block.Dataview.SetSource







<a name="anytype.Rpc.Block.Dataview.SetSource.Request"></a>

### Rpc.Block.Dataview.SetSource.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| source | [string](#string) | repeated |  |






<a name="anytype.Rpc.Block.Dataview.SetSource.Response"></a>

### Rpc.Block.Dataview.SetSource.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.SetSource.Response.Error](#anytype.Rpc.Block.Dataview.SetSource.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.SetSource.Response.Error"></a>

### Rpc.Block.Dataview.SetSource.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.SetSource.Response.Error.Code](#anytype.Rpc.Block.Dataview.SetSource.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewCreate"></a>

### Rpc.Block.Dataview.ViewCreate







<a name="anytype.Rpc.Block.Dataview.ViewCreate.Request"></a>

### Rpc.Block.Dataview.ViewCreate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to insert the new block |
| view | [model.Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewCreate.Response"></a>

### Rpc.Block.Dataview.ViewCreate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewCreate.Response.Error](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| viewId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewCreate.Response.Error"></a>

### Rpc.Block.Dataview.ViewCreate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewDelete"></a>

### Rpc.Block.Dataview.ViewDelete







<a name="anytype.Rpc.Block.Dataview.ViewDelete.Request"></a>

### Rpc.Block.Dataview.ViewDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| blockId | [string](#string) |  | id of the dataview |
| viewId | [string](#string) |  | id of the view to remove |






<a name="anytype.Rpc.Block.Dataview.ViewDelete.Response"></a>

### Rpc.Block.Dataview.ViewDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewDelete.Response.Error](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewDelete.Response.Error"></a>

### Rpc.Block.Dataview.ViewDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewSetActive"></a>

### Rpc.Block.Dataview.ViewSetActive
set the current active view (persisted only within a session)






<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Request"></a>

### Rpc.Block.Dataview.ViewSetActive.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block |
| viewId | [string](#string) |  | id of active view |
| offset | [uint32](#uint32) |  |  |
| limit | [uint32](#uint32) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Response"></a>

### Rpc.Block.Dataview.ViewSetActive.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewSetActive.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error"></a>

### Rpc.Block.Dataview.ViewSetActive.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewSetActive.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewSetPosition"></a>

### Rpc.Block.Dataview.ViewSetPosition







<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Request"></a>

### Rpc.Block.Dataview.ViewSetPosition.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| blockId | [string](#string) |  | id of the dataview |
| viewId | [string](#string) |  | id of the view to remove |
| position | [uint32](#uint32) |  | index of view position (0 - means first) |






<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Response"></a>

### Rpc.Block.Dataview.ViewSetPosition.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewSetPosition.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error"></a>

### Rpc.Block.Dataview.ViewSetPosition.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewUpdate"></a>

### Rpc.Block.Dataview.ViewUpdate







<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Request"></a>

### Rpc.Block.Dataview.ViewUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  | id of dataview block to update |
| viewId | [string](#string) |  | id of view to update |
| view | [model.Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Response"></a>

### Rpc.Block.Dataview.ViewUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewUpdate.Response.Error](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error"></a>

### Rpc.Block.Dataview.ViewUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.File"></a>

### Rpc.Block.File







<a name="anytype.Rpc.Block.File.CreateAndUpload"></a>

### Rpc.Block.File.CreateAndUpload







<a name="anytype.Rpc.Block.File.CreateAndUpload.Request"></a>

### Rpc.Block.File.CreateAndUpload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| fileType | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |






<a name="anytype.Rpc.Block.File.CreateAndUpload.Response"></a>

### Rpc.Block.File.CreateAndUpload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.File.CreateAndUpload.Response.Error](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.File.CreateAndUpload.Response.Error"></a>

### Rpc.Block.File.CreateAndUpload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.File.CreateAndUpload.Response.Error.Code](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Get"></a>

### Rpc.Block.Get







<a name="anytype.Rpc.Block.Get.Marks"></a>

### Rpc.Block.Get.Marks
Get marks list in the selected range in text block.






<a name="anytype.Rpc.Block.Get.Marks.Request"></a>

### Rpc.Block.Get.Marks.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| range | [model.Range](#anytype.model.Range) |  |  |






<a name="anytype.Rpc.Block.Get.Marks.Response"></a>

### Rpc.Block.Get.Marks.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Get.Marks.Response.Error](#anytype.Rpc.Block.Get.Marks.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Get.Marks.Response.Error"></a>

### Rpc.Block.Get.Marks.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.GetPublicWebURL"></a>

### Rpc.Block.GetPublicWebURL







<a name="anytype.Rpc.Block.GetPublicWebURL.Request"></a>

### Rpc.Block.GetPublicWebURL.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.GetPublicWebURL.Response"></a>

### Rpc.Block.GetPublicWebURL.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.GetPublicWebURL.Response.Error](#anytype.Rpc.Block.GetPublicWebURL.Response.Error) |  |  |
| url | [string](#string) |  |  |






<a name="anytype.Rpc.Block.GetPublicWebURL.Response.Error"></a>

### Rpc.Block.GetPublicWebURL.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.GetPublicWebURL.Response.Error.Code](#anytype.Rpc.Block.GetPublicWebURL.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ImportMarkdown"></a>

### Rpc.Block.ImportMarkdown







<a name="anytype.Rpc.Block.ImportMarkdown.Request"></a>

### Rpc.Block.ImportMarkdown.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| importPath | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ImportMarkdown.Response"></a>

### Rpc.Block.ImportMarkdown.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ImportMarkdown.Response.Error](#anytype.Rpc.Block.ImportMarkdown.Response.Error) |  |  |
| rootLinkIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ImportMarkdown.Response.Error"></a>

### Rpc.Block.ImportMarkdown.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ImportMarkdown.Response.Error.Code](#anytype.Rpc.Block.ImportMarkdown.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






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






<a name="anytype.Rpc.Block.ObjectType"></a>

### Rpc.Block.ObjectType







<a name="anytype.Rpc.Block.ObjectType.Set"></a>

### Rpc.Block.ObjectType.Set







<a name="anytype.Rpc.Block.ObjectType.Set.Request"></a>

### Rpc.Block.ObjectType.Set.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| objectTypeUrl | [string](#string) |  |  |






<a name="anytype.Rpc.Block.ObjectType.Set.Response"></a>

### Rpc.Block.ObjectType.Set.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ObjectType.Set.Response.Error](#anytype.Rpc.Block.ObjectType.Set.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.ObjectType.Set.Response.Error"></a>

### Rpc.Block.ObjectType.Set.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ObjectType.Set.Response.Error.Code](#anytype.Rpc.Block.ObjectType.Set.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Open"></a>

### Rpc.Block.Open
Works with a smart blocks (block-organizers, like page, dashboard etc)
**Example scenario**
1A. On front-end start.
    1. Front -&gt; MW: Rpc.Block.Open.Request(dashboard.id)
    2. MW -&gt; Front: BlockShow(dashboard)
    3. MW -&gt; Front: Rpc.Block.Open.Response(err)
1B. User clicks on a page icon on the dashboard.
    1. Front -&gt; MW: Rpc.Block.Close.Request(dashboard.id)
Get close response first, then open request:
    2. MW -&gt; Front: Rpc.Block.Close.Response(err)
    3. Front -&gt; MW: Rpc.Block.Open.Request(page.id)
    4. MW -&gt; Front: BlockShow(&lt;page, blocks&gt;)
    5. MW -&gt; Front: Rpc.Block.Open.Response(err)
Image/Video/File blocks then:
    6. MW -&gt; Front: BlockShow(&lt;blocks&gt;)






<a name="anytype.Rpc.Block.Open.Request"></a>

### Rpc.Block.Open.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| blockId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Open.Response"></a>

### Rpc.Block.Open.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Open.Response.Error"></a>

### Rpc.Block.Open.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.OpenBreadcrumbs"></a>

### Rpc.Block.OpenBreadcrumbs







<a name="anytype.Rpc.Block.OpenBreadcrumbs.Request"></a>

### Rpc.Block.OpenBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.OpenBreadcrumbs.Response"></a>

### Rpc.Block.OpenBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.OpenBreadcrumbs.Response.Error](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.OpenBreadcrumbs.Response.Error"></a>

### Rpc.Block.OpenBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Redo"></a>

### Rpc.Block.Redo







<a name="anytype.Rpc.Block.Redo.Request"></a>

### Rpc.Block.Redo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |






<a name="anytype.Rpc.Block.Redo.Response"></a>

### Rpc.Block.Redo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Redo.Response.Error](#anytype.Rpc.Block.Redo.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| counters | [Rpc.Block.UndoRedoCounter](#anytype.Rpc.Block.UndoRedoCounter) |  |  |






<a name="anytype.Rpc.Block.Redo.Response.Error"></a>

### Rpc.Block.Redo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Redo.Response.Error.Code](#anytype.Rpc.Block.Redo.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Relation"></a>

### Rpc.Block.Relation







<a name="anytype.Rpc.Block.Relation.Add"></a>

### Rpc.Block.Relation.Add







<a name="anytype.Rpc.Block.Relation.Add.Request"></a>

### Rpc.Block.Relation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Block.Relation.Add.Response"></a>

### Rpc.Block.Relation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Relation.Add.Response.Error](#anytype.Rpc.Block.Relation.Add.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Relation.Add.Response.Error"></a>

### Rpc.Block.Relation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Relation.Add.Response.Error.Code](#anytype.Rpc.Block.Relation.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Relation.SetKey"></a>

### Rpc.Block.Relation.SetKey







<a name="anytype.Rpc.Block.Relation.SetKey.Request"></a>

### Rpc.Block.Relation.SetKey.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| key | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Relation.SetKey.Response"></a>

### Rpc.Block.Relation.SetKey.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Relation.SetKey.Response.Error](#anytype.Rpc.Block.Relation.SetKey.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Relation.SetKey.Response.Error"></a>

### Rpc.Block.Relation.SetKey.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Relation.SetKey.Response.Error.Code](#anytype.Rpc.Block.Relation.SetKey.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Set"></a>

### Rpc.Block.Set







<a name="anytype.Rpc.Block.Set.Details"></a>

### Rpc.Block.Set.Details







<a name="anytype.Rpc.Block.Set.Details.Detail"></a>

### Rpc.Block.Set.Details.Detail



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [google.protobuf.Value](#google.protobuf.Value) |  | NUll - removes key |






<a name="anytype.Rpc.Block.Set.Details.Request"></a>

### Rpc.Block.Set.Details.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| details | [Rpc.Block.Set.Details.Detail](#anytype.Rpc.Block.Set.Details.Detail) | repeated |  |






<a name="anytype.Rpc.Block.Set.Details.Response"></a>

### Rpc.Block.Set.Details.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Details.Response.Error](#anytype.Rpc.Block.Set.Details.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Details.Response.Error"></a>

### Rpc.Block.Set.Details.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Details.Response.Error.Code](#anytype.Rpc.Block.Set.Details.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Fields"></a>

### Rpc.Block.Set.Fields







<a name="anytype.Rpc.Block.Set.Fields.Request"></a>

### Rpc.Block.Set.Fields.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.Block.Set.Fields.Response"></a>

### Rpc.Block.Set.Fields.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Fields.Response.Error](#anytype.Rpc.Block.Set.Fields.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Fields.Response.Error"></a>

### Rpc.Block.Set.Fields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Fields.Response.Error.Code](#anytype.Rpc.Block.Set.Fields.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.File"></a>

### Rpc.Block.Set.File







<a name="anytype.Rpc.Block.Set.File.Name"></a>

### Rpc.Block.Set.File.Name







<a name="anytype.Rpc.Block.Set.File.Name.Request"></a>

### Rpc.Block.Set.File.Name.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.File.Name.Response"></a>

### Rpc.Block.Set.File.Name.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.File.Name.Response.Error](#anytype.Rpc.Block.Set.File.Name.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.File.Name.Response.Error"></a>

### Rpc.Block.Set.File.Name.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.File.Name.Response.Error.Code](#anytype.Rpc.Block.Set.File.Name.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Image"></a>

### Rpc.Block.Set.Image







<a name="anytype.Rpc.Block.Set.Image.Name"></a>

### Rpc.Block.Set.Image.Name







<a name="anytype.Rpc.Block.Set.Image.Name.Request"></a>

### Rpc.Block.Set.Image.Name.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Image.Name.Response"></a>

### Rpc.Block.Set.Image.Name.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Image.Name.Response.Error](#anytype.Rpc.Block.Set.Image.Name.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Image.Name.Response.Error"></a>

### Rpc.Block.Set.Image.Name.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Image.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Name.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Image.Width"></a>

### Rpc.Block.Set.Image.Width







<a name="anytype.Rpc.Block.Set.Image.Width.Request"></a>

### Rpc.Block.Set.Image.Width.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype.Rpc.Block.Set.Image.Width.Response"></a>

### Rpc.Block.Set.Image.Width.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Image.Width.Response.Error](#anytype.Rpc.Block.Set.Image.Width.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Image.Width.Response.Error"></a>

### Rpc.Block.Set.Image.Width.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Image.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Width.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Latex"></a>

### Rpc.Block.Set.Latex







<a name="anytype.Rpc.Block.Set.Latex.Text"></a>

### Rpc.Block.Set.Latex.Text







<a name="anytype.Rpc.Block.Set.Latex.Text.Request"></a>

### Rpc.Block.Set.Latex.Text.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Latex.Text.Response"></a>

### Rpc.Block.Set.Latex.Text.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Latex.Text.Response.Error](#anytype.Rpc.Block.Set.Latex.Text.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Latex.Text.Response.Error"></a>

### Rpc.Block.Set.Latex.Text.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Latex.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Latex.Text.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Link"></a>

### Rpc.Block.Set.Link







<a name="anytype.Rpc.Block.Set.Link.TargetBlockId"></a>

### Rpc.Block.Set.Link.TargetBlockId







<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Request"></a>

### Rpc.Block.Set.Link.TargetBlockId.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| targetBlockId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Response"></a>

### Rpc.Block.Set.Link.TargetBlockId.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Link.TargetBlockId.Response.Error](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error"></a>

### Rpc.Block.Set.Link.TargetBlockId.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Page"></a>

### Rpc.Block.Set.Page







<a name="anytype.Rpc.Block.Set.Page.IsArchived"></a>

### Rpc.Block.Set.Page.IsArchived







<a name="anytype.Rpc.Block.Set.Page.IsArchived.Request"></a>

### Rpc.Block.Set.Page.IsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| isArchived | [bool](#bool) |  |  |






<a name="anytype.Rpc.Block.Set.Page.IsArchived.Response"></a>

### Rpc.Block.Set.Page.IsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Page.IsArchived.Response.Error](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Page.IsArchived.Response.Error"></a>

### Rpc.Block.Set.Page.IsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Page.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Restrictions"></a>

### Rpc.Block.Set.Restrictions







<a name="anytype.Rpc.Block.Set.Restrictions.Request"></a>

### Rpc.Block.Set.Restrictions.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| restrictions | [model.Block.Restrictions](#anytype.model.Block.Restrictions) |  |  |






<a name="anytype.Rpc.Block.Set.Restrictions.Response"></a>

### Rpc.Block.Set.Restrictions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Restrictions.Response.Error](#anytype.Rpc.Block.Set.Restrictions.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Restrictions.Response.Error"></a>

### Rpc.Block.Set.Restrictions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Restrictions.Response.Error.Code](#anytype.Rpc.Block.Set.Restrictions.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text"></a>

### Rpc.Block.Set.Text







<a name="anytype.Rpc.Block.Set.Text.Checked"></a>

### Rpc.Block.Set.Text.Checked







<a name="anytype.Rpc.Block.Set.Text.Checked.Request"></a>

### Rpc.Block.Set.Text.Checked.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| checked | [bool](#bool) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Checked.Response"></a>

### Rpc.Block.Set.Text.Checked.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Checked.Response.Error](#anytype.Rpc.Block.Set.Text.Checked.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Checked.Response.Error"></a>

### Rpc.Block.Set.Text.Checked.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Checked.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Color"></a>

### Rpc.Block.Set.Text.Color







<a name="anytype.Rpc.Block.Set.Text.Color.Request"></a>

### Rpc.Block.Set.Text.Color.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Color.Response"></a>

### Rpc.Block.Set.Text.Color.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Color.Response.Error](#anytype.Rpc.Block.Set.Text.Color.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Color.Response.Error"></a>

### Rpc.Block.Set.Text.Color.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Icon"></a>

### Rpc.Block.Set.Text.Icon







<a name="anytype.Rpc.Block.Set.Text.Icon.Request"></a>

### Rpc.Block.Set.Text.Icon.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| iconImage | [string](#string) |  | in case both image and emoji are set, image has a priority to show |
| iconEmoji | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Icon.Response"></a>

### Rpc.Block.Set.Text.Icon.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Icon.Response.Error](#anytype.Rpc.Block.Set.Text.Icon.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Icon.Response.Error"></a>

### Rpc.Block.Set.Text.Icon.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Icon.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Icon.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Style"></a>

### Rpc.Block.Set.Text.Style







<a name="anytype.Rpc.Block.Set.Text.Style.Request"></a>

### Rpc.Block.Set.Text.Style.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Style.Response"></a>

### Rpc.Block.Set.Text.Style.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Style.Response.Error](#anytype.Rpc.Block.Set.Text.Style.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Style.Response.Error"></a>

### Rpc.Block.Set.Text.Style.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Style.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Style.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Text"></a>

### Rpc.Block.Set.Text.Text







<a name="anytype.Rpc.Block.Set.Text.Text.Request"></a>

### Rpc.Block.Set.Text.Text.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| text | [string](#string) |  |  |
| marks | [model.Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Text.Response"></a>

### Rpc.Block.Set.Text.Text.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Text.Response.Error](#anytype.Rpc.Block.Set.Text.Text.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Set.Text.Text.Response.Error"></a>

### Rpc.Block.Set.Text.Text.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Text.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Video"></a>

### Rpc.Block.Set.Video







<a name="anytype.Rpc.Block.Set.Video.Name"></a>

### Rpc.Block.Set.Video.Name







<a name="anytype.Rpc.Block.Set.Video.Name.Request"></a>

### Rpc.Block.Set.Video.Name.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Video.Name.Response"></a>

### Rpc.Block.Set.Video.Name.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Video.Name.Response.Error](#anytype.Rpc.Block.Set.Video.Name.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Video.Name.Response.Error"></a>

### Rpc.Block.Set.Video.Name.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Video.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Name.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Video.Width"></a>

### Rpc.Block.Set.Video.Width







<a name="anytype.Rpc.Block.Set.Video.Width.Request"></a>

### Rpc.Block.Set.Video.Width.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| width | [int32](#int32) |  |  |






<a name="anytype.Rpc.Block.Set.Video.Width.Response"></a>

### Rpc.Block.Set.Video.Width.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Video.Width.Response.Error](#anytype.Rpc.Block.Set.Video.Width.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Video.Width.Response.Error"></a>

### Rpc.Block.Set.Video.Width.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Video.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Width.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.SetBreadcrumbs"></a>

### Rpc.Block.SetBreadcrumbs







<a name="anytype.Rpc.Block.SetBreadcrumbs.Request"></a>

### Rpc.Block.SetBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| breadcrumbsId | [string](#string) |  |  |
| ids | [string](#string) | repeated | page ids |






<a name="anytype.Rpc.Block.SetBreadcrumbs.Response"></a>

### Rpc.Block.SetBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.SetBreadcrumbs.Response.Error](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.SetBreadcrumbs.Response.Error"></a>

### Rpc.Block.SetBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.SetBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Show"></a>

### Rpc.Block.Show







<a name="anytype.Rpc.Block.Show.Request"></a>

### Rpc.Block.Show.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context blo1k |
| blockId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Show.Response"></a>

### Rpc.Block.Show.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Show.Response.Error](#anytype.Rpc.Block.Show.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.Show.Response.Error"></a>

### Rpc.Block.Show.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Show.Response.Error.Code](#anytype.Rpc.Block.Show.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Undo"></a>

### Rpc.Block.Undo







<a name="anytype.Rpc.Block.Undo.Request"></a>

### Rpc.Block.Undo.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |






<a name="anytype.Rpc.Block.Undo.Response"></a>

### Rpc.Block.Undo.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Undo.Response.Error](#anytype.Rpc.Block.Undo.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| counters | [Rpc.Block.UndoRedoCounter](#anytype.Rpc.Block.UndoRedoCounter) |  |  |






<a name="anytype.Rpc.Block.Undo.Response.Error"></a>

### Rpc.Block.Undo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Undo.Response.Error.Code](#anytype.Rpc.Block.Undo.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.UndoRedoCounter"></a>

### Rpc.Block.UndoRedoCounter
Available undo/redo operations


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| undo | [int32](#int32) |  |  |
| redo | [int32](#int32) |  |  |






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






<a name="anytype.Rpc.Block.UpdateContent"></a>

### Rpc.Block.UpdateContent







<a name="anytype.Rpc.Block.UpdateContent.Request"></a>

### Rpc.Block.UpdateContent.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| block | [model.Block](#anytype.model.Block) |  |  |






<a name="anytype.Rpc.Block.UpdateContent.Response"></a>

### Rpc.Block.UpdateContent.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.UpdateContent.Response.Error](#anytype.Rpc.Block.UpdateContent.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Block.UpdateContent.Response.Error"></a>

### Rpc.Block.UpdateContent.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.UpdateContent.Response.Error.Code](#anytype.Rpc.Block.UpdateContent.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.BlockList"></a>

### Rpc.BlockList







<a name="anytype.Rpc.BlockList.ConvertChildrenToPages"></a>

### Rpc.BlockList.ConvertChildrenToPages







<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Request"></a>

### Rpc.BlockList.ConvertChildrenToPages.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| objectType | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Response"></a>

### Rpc.BlockList.ConvertChildrenToPages.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.ConvertChildrenToPages.Response.Error](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error) |  |  |
| linkIds | [string](#string) | repeated |  |






<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error"></a>

### Rpc.BlockList.ConvertChildrenToPages.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Duplicate"></a>

### Rpc.BlockList.Duplicate
Makes blocks copy by given ids and paste it to shown place






<a name="anytype.Rpc.BlockList.Duplicate.Request"></a>

### Rpc.BlockList.Duplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| targetId | [string](#string) |  | id of the closest block |
| blockIds | [string](#string) | repeated | id of block for duplicate |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.BlockList.Duplicate.Response"></a>

### Rpc.BlockList.Duplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Duplicate.Response.Error](#anytype.Rpc.BlockList.Duplicate.Response.Error) |  |  |
| blockIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Duplicate.Response.Error"></a>

### Rpc.BlockList.Duplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Duplicate.Response.Error.Code](#anytype.Rpc.BlockList.Duplicate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Move"></a>

### Rpc.BlockList.Move







<a name="anytype.Rpc.BlockList.Move.Request"></a>

### Rpc.BlockList.Move.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| targetContextId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype.model.Block.Position) |  | position relatively to the dropTargetId simple block |






<a name="anytype.Rpc.BlockList.Move.Response"></a>

### Rpc.BlockList.Move.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Move.Response.Error](#anytype.Rpc.BlockList.Move.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Move.Response.Error"></a>

### Rpc.BlockList.Move.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Move.Response.Error.Code](#anytype.Rpc.BlockList.Move.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.MoveToNewPage"></a>

### Rpc.BlockList.MoveToNewPage







<a name="anytype.Rpc.BlockList.MoveToNewPage.Request"></a>

### Rpc.BlockList.MoveToNewPage.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | page details |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype.model.Block.Position) |  | position relatively to the dropTargetId simple block |






<a name="anytype.Rpc.BlockList.MoveToNewPage.Response"></a>

### Rpc.BlockList.MoveToNewPage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.MoveToNewPage.Response.Error](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error) |  |  |
| linkId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.MoveToNewPage.Response.Error"></a>

### Rpc.BlockList.MoveToNewPage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.MoveToNewPage.Response.Error.Code](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set"></a>

### Rpc.BlockList.Set







<a name="anytype.Rpc.BlockList.Set.Align"></a>

### Rpc.BlockList.Set.Align







<a name="anytype.Rpc.BlockList.Set.Align.Request"></a>

### Rpc.BlockList.Set.Align.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated | when empty - align will be applied as layoutAlign |
| align | [model.Block.Align](#anytype.model.Block.Align) |  |  |






<a name="anytype.Rpc.BlockList.Set.Align.Response"></a>

### Rpc.BlockList.Set.Align.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Align.Response.Error](#anytype.Rpc.BlockList.Set.Align.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.Align.Response.Error"></a>

### Rpc.BlockList.Set.Align.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Align.Response.Error.Code](#anytype.Rpc.BlockList.Set.Align.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.BackgroundColor"></a>

### Rpc.BlockList.Set.BackgroundColor







<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Request"></a>

### Rpc.BlockList.Set.BackgroundColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Response"></a>

### Rpc.BlockList.Set.BackgroundColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.BackgroundColor.Response.Error](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error"></a>

### Rpc.BlockList.Set.BackgroundColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Div"></a>

### Rpc.BlockList.Set.Div







<a name="anytype.Rpc.BlockList.Set.Div.Style"></a>

### Rpc.BlockList.Set.Div.Style







<a name="anytype.Rpc.BlockList.Set.Div.Style.Request"></a>

### Rpc.BlockList.Set.Div.Style.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |






<a name="anytype.Rpc.BlockList.Set.Div.Style.Response"></a>

### Rpc.BlockList.Set.Div.Style.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Div.Style.Response.Error](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.Div.Style.Response.Error"></a>

### Rpc.BlockList.Set.Div.Style.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Div.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Fields"></a>

### Rpc.BlockList.Set.Fields







<a name="anytype.Rpc.BlockList.Set.Fields.Request"></a>

### Rpc.BlockList.Set.Fields.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockFields | [Rpc.BlockList.Set.Fields.Request.BlockField](#anytype.Rpc.BlockList.Set.Fields.Request.BlockField) | repeated |  |






<a name="anytype.Rpc.BlockList.Set.Fields.Request.BlockField"></a>

### Rpc.BlockList.Set.Fields.Request.BlockField



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Rpc.BlockList.Set.Fields.Response"></a>

### Rpc.BlockList.Set.Fields.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Fields.Response.Error](#anytype.Rpc.BlockList.Set.Fields.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.Fields.Response.Error"></a>

### Rpc.BlockList.Set.Fields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.File"></a>

### Rpc.BlockList.Set.File







<a name="anytype.Rpc.BlockList.Set.File.Style"></a>

### Rpc.BlockList.Set.File.Style







<a name="anytype.Rpc.BlockList.Set.File.Style.Request"></a>

### Rpc.BlockList.Set.File.Style.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Rpc.BlockList.Set.File.Style.Response"></a>

### Rpc.BlockList.Set.File.Style.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.File.Style.Response.Error](#anytype.Rpc.BlockList.Set.File.Style.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.File.Style.Response.Error"></a>

### Rpc.BlockList.Set.File.Style.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.File.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.File.Style.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text"></a>

### Rpc.BlockList.Set.Text
commands acceptable only for text blocks, others will be ignored






<a name="anytype.Rpc.BlockList.Set.Text.Color"></a>

### Rpc.BlockList.Set.Text.Color







<a name="anytype.Rpc.BlockList.Set.Text.Color.Request"></a>

### Rpc.BlockList.Set.Text.Color.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Color.Response"></a>

### Rpc.BlockList.Set.Text.Color.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.Color.Response.Error](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Color.Response.Error"></a>

### Rpc.BlockList.Set.Text.Color.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Mark"></a>

### Rpc.BlockList.Set.Text.Mark







<a name="anytype.Rpc.BlockList.Set.Text.Mark.Request"></a>

### Rpc.BlockList.Set.Text.Mark.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| mark | [model.Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Mark.Response"></a>

### Rpc.BlockList.Set.Text.Mark.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.Mark.Response.Error](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Mark.Response.Error"></a>

### Rpc.BlockList.Set.Text.Mark.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Mark.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Style"></a>

### Rpc.BlockList.Set.Text.Style







<a name="anytype.Rpc.BlockList.Set.Text.Style.Request"></a>

### Rpc.BlockList.Set.Text.Style.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Style.Response"></a>

### Rpc.BlockList.Set.Text.Style.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.Style.Response.Error](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.Style.Response.Error"></a>

### Rpc.BlockList.Set.Text.Style.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.TurnInto"></a>

### Rpc.BlockList.TurnInto







<a name="anytype.Rpc.BlockList.TurnInto.Request"></a>

### Rpc.BlockList.TurnInto.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| style | [model.Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |






<a name="anytype.Rpc.BlockList.TurnInto.Response"></a>

### Rpc.BlockList.TurnInto.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.TurnInto.Response.Error](#anytype.Rpc.BlockList.TurnInto.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.BlockList.TurnInto.Response.Error"></a>

### Rpc.BlockList.TurnInto.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.TurnInto.Response.Error.Code](#anytype.Rpc.BlockList.TurnInto.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.CloneTemplate"></a>

### Rpc.CloneTemplate







<a name="anytype.Rpc.CloneTemplate.Request"></a>

### Rpc.CloneTemplate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of template block for cloning |






<a name="anytype.Rpc.CloneTemplate.Response"></a>

### Rpc.CloneTemplate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.CloneTemplate.Response.Error](#anytype.Rpc.CloneTemplate.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.CloneTemplate.Response.Error"></a>

### Rpc.CloneTemplate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.CloneTemplate.Response.Error.Code](#anytype.Rpc.CloneTemplate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Config"></a>

### Rpc.Config







<a name="anytype.Rpc.Config.Get"></a>

### Rpc.Config.Get







<a name="anytype.Rpc.Config.Get.Request"></a>

### Rpc.Config.Get.Request







<a name="anytype.Rpc.Config.Get.Response"></a>

### Rpc.Config.Get.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Config.Get.Response.Error](#anytype.Rpc.Config.Get.Response.Error) |  |  |
| homeBlockId | [string](#string) |  | home dashboard block id |
| archiveBlockId | [string](#string) |  | archive block id |
| profileBlockId | [string](#string) |  | profile block id |
| marketplaceTypeId | [string](#string) |  | marketplace type id |
| marketplaceRelationId | [string](#string) |  | marketplace relation id |
| marketplaceTemplateId | [string](#string) |  | marketplace template id |
| deviceId | [string](#string) |  |  |
| gatewayUrl | [string](#string) |  | gateway url for fetching static files |






<a name="anytype.Rpc.Config.Get.Response.Error"></a>

### Rpc.Config.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Debug"></a>

### Rpc.Debug







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






<a name="anytype.Rpc.DownloadFile"></a>

### Rpc.DownloadFile







<a name="anytype.Rpc.DownloadFile.Request"></a>

### Rpc.DownloadFile.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| path | [string](#string) |  | path to save file. Temp directory is used if empty |






<a name="anytype.Rpc.DownloadFile.Response"></a>

### Rpc.DownloadFile.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.DownloadFile.Response.Error](#anytype.Rpc.DownloadFile.Response.Error) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype.Rpc.DownloadFile.Response.Error"></a>

### Rpc.DownloadFile.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.DownloadFile.Response.Error.Code](#anytype.Rpc.DownloadFile.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Export"></a>

### Rpc.Export







<a name="anytype.Rpc.Export.Request"></a>

### Rpc.Export.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| docIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |
| format | [Rpc.Export.Format](#anytype.Rpc.Export.Format) |  | export format |
| zip | [bool](#bool) |  | save as zip file |
| includeNested | [bool](#bool) |  | include all nested |
| includeFiles | [bool](#bool) |  | include all files |






<a name="anytype.Rpc.Export.Response"></a>

### Rpc.Export.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Export.Response.Error](#anytype.Rpc.Export.Response.Error) |  |  |
| path | [string](#string) |  |  |
| succeed | [int32](#int32) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Export.Response.Error"></a>

### Rpc.Export.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Export.Response.Error.Code](#anytype.Rpc.Export.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ExportLocalstore"></a>

### Rpc.ExportLocalstore







<a name="anytype.Rpc.ExportLocalstore.Request"></a>

### Rpc.ExportLocalstore.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| docIds | [string](#string) | repeated | ids of documents for export, when empty - will export all available docs |






<a name="anytype.Rpc.ExportLocalstore.Response"></a>

### Rpc.ExportLocalstore.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExportLocalstore.Response.Error](#anytype.Rpc.ExportLocalstore.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ExportLocalstore.Response.Error"></a>

### Rpc.ExportLocalstore.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExportLocalstore.Response.Error.Code](#anytype.Rpc.ExportLocalstore.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ExportTemplates"></a>

### Rpc.ExportTemplates







<a name="anytype.Rpc.ExportTemplates.Request"></a>

### Rpc.ExportTemplates.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |






<a name="anytype.Rpc.ExportTemplates.Response"></a>

### Rpc.ExportTemplates.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExportTemplates.Response.Error](#anytype.Rpc.ExportTemplates.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ExportTemplates.Response.Error"></a>

### Rpc.ExportTemplates.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExportTemplates.Response.Error.Code](#anytype.Rpc.ExportTemplates.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ExportWorkspace"></a>

### Rpc.ExportWorkspace







<a name="anytype.Rpc.ExportWorkspace.Request"></a>

### Rpc.ExportWorkspace.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) |  | the path where export files will place |
| workspaceId | [string](#string) |  |  |






<a name="anytype.Rpc.ExportWorkspace.Response"></a>

### Rpc.ExportWorkspace.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExportWorkspace.Response.Error](#anytype.Rpc.ExportWorkspace.Response.Error) |  |  |
| path | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ExportWorkspace.Response.Error"></a>

### Rpc.ExportWorkspace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExportWorkspace.Response.Error.Code](#anytype.Rpc.ExportWorkspace.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ExternalDrop"></a>

### Rpc.ExternalDrop







<a name="anytype.Rpc.ExternalDrop.Content"></a>

### Rpc.ExternalDrop.Content







<a name="anytype.Rpc.ExternalDrop.Content.Request"></a>

### Rpc.ExternalDrop.Content.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| focusedBlockId | [string](#string) |  | can be null |
| content | [bytes](#bytes) |  | TODO |






<a name="anytype.Rpc.ExternalDrop.Content.Response"></a>

### Rpc.ExternalDrop.Content.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExternalDrop.Content.Response.Error](#anytype.Rpc.ExternalDrop.Content.Response.Error) |  |  |






<a name="anytype.Rpc.ExternalDrop.Content.Response.Error"></a>

### Rpc.ExternalDrop.Content.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExternalDrop.Content.Response.Error.Code](#anytype.Rpc.ExternalDrop.Content.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ExternalDrop.Files"></a>

### Rpc.ExternalDrop.Files







<a name="anytype.Rpc.ExternalDrop.Files.Request"></a>

### Rpc.ExternalDrop.Files.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| dropTargetId | [string](#string) |  | id of the simple block to insert considering position |
| position | [model.Block.Position](#anytype.model.Block.Position) |  | position relatively to the dropTargetId simple block |
| localFilePaths | [string](#string) | repeated |  |






<a name="anytype.Rpc.ExternalDrop.Files.Response"></a>

### Rpc.ExternalDrop.Files.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExternalDrop.Files.Response.Error](#anytype.Rpc.ExternalDrop.Files.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ExternalDrop.Files.Response.Error"></a>

### Rpc.ExternalDrop.Files.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.File"></a>

### Rpc.File







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






<a name="anytype.Rpc.FileList"></a>

### Rpc.FileList







<a name="anytype.Rpc.FileList.Offload"></a>

### Rpc.FileList.Offload







<a name="anytype.Rpc.FileList.Offload.Request"></a>

### Rpc.FileList.Offload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| onlyIds | [string](#string) | repeated | empty means all |
| includeNotPinned | [bool](#bool) |  | false mean not-yet-pinned files will be not |






<a name="anytype.Rpc.FileList.Offload.Response"></a>

### Rpc.FileList.Offload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.FileList.Offload.Response.Error](#anytype.Rpc.FileList.Offload.Response.Error) |  |  |
| filesOffloaded | [int32](#int32) |  |  |
| bytesOffloaded | [uint64](#uint64) |  |  |






<a name="anytype.Rpc.FileList.Offload.Response.Error"></a>

### Rpc.FileList.Offload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.FileList.Offload.Response.Error.Code](#anytype.Rpc.FileList.Offload.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.History.Show"></a>

### Rpc.History.Show
returns blockShow event for given version






<a name="anytype.Rpc.History.Show.Request"></a>

### Rpc.History.Show.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) |  |  |
| versionId | [string](#string) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.History.Show.Response"></a>

### Rpc.History.Show.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.Show.Response.Error](#anytype.Rpc.History.Show.Response.Error) |  |  |
| objectShow | [Event.Object.Show](#anytype.Event.Object.Show) |  |  |
| version | [Rpc.History.Versions.Version](#anytype.Rpc.History.Versions.Version) |  |  |
| traceId | [string](#string) |  |  |






<a name="anytype.Rpc.History.Show.Response.Error"></a>

### Rpc.History.Show.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.Show.Response.Error.Code](#anytype.Rpc.History.Show.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.History.Versions"></a>

### Rpc.History.Versions
returns list of versions (changes)






<a name="anytype.Rpc.History.Versions.Request"></a>

### Rpc.History.Versions.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) |  |  |
| lastVersionId | [string](#string) |  | when indicated, results will include versions before given id |
| limit | [int32](#int32) |  | desired count of versions |






<a name="anytype.Rpc.History.Versions.Response"></a>

### Rpc.History.Versions.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.Versions.Response.Error](#anytype.Rpc.History.Versions.Response.Error) |  |  |
| versions | [Rpc.History.Versions.Version](#anytype.Rpc.History.Versions.Version) | repeated |  |






<a name="anytype.Rpc.History.Versions.Response.Error"></a>

### Rpc.History.Versions.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.Versions.Response.Error.Code](#anytype.Rpc.History.Versions.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.History.Versions.Version"></a>

### Rpc.History.Versions.Version



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
Namespace, that agregates log subtopics and actions.
Usage: send request with topic (Level) and description (message) from client to middleware to log.






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






<a name="anytype.Rpc.MakeTemplate"></a>

### Rpc.MakeTemplate







<a name="anytype.Rpc.MakeTemplate.Request"></a>

### Rpc.MakeTemplate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of block for making them template |






<a name="anytype.Rpc.MakeTemplate.Response"></a>

### Rpc.MakeTemplate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MakeTemplate.Response.Error](#anytype.Rpc.MakeTemplate.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.MakeTemplate.Response.Error"></a>

### Rpc.MakeTemplate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MakeTemplate.Response.Error.Code](#anytype.Rpc.MakeTemplate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.MakeTemplateByObjectType"></a>

### Rpc.MakeTemplateByObjectType







<a name="anytype.Rpc.MakeTemplateByObjectType.Request"></a>

### Rpc.MakeTemplateByObjectType.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectType | [string](#string) |  | id of desired object type |






<a name="anytype.Rpc.MakeTemplateByObjectType.Response"></a>

### Rpc.MakeTemplateByObjectType.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MakeTemplateByObjectType.Response.Error](#anytype.Rpc.MakeTemplateByObjectType.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.MakeTemplateByObjectType.Response.Error"></a>

### Rpc.MakeTemplateByObjectType.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MakeTemplateByObjectType.Response.Error.Code](#anytype.Rpc.MakeTemplateByObjectType.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Object.FeaturedRelation"></a>

### Rpc.Object.FeaturedRelation







<a name="anytype.Rpc.Object.FeaturedRelation.Add"></a>

### Rpc.Object.FeaturedRelation.Add







<a name="anytype.Rpc.Object.FeaturedRelation.Add.Request"></a>

### Rpc.Object.FeaturedRelation.Add.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype.Rpc.Object.FeaturedRelation.Add.Response"></a>

### Rpc.Object.FeaturedRelation.Add.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.FeaturedRelation.Add.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.FeaturedRelation.Add.Response.Error"></a>

### Rpc.Object.FeaturedRelation.Add.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.FeaturedRelation.Add.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.FeaturedRelation.Remove"></a>

### Rpc.Object.FeaturedRelation.Remove







<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Request"></a>

### Rpc.Object.FeaturedRelation.Remove.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relations | [string](#string) | repeated |  |






<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Response"></a>

### Rpc.Object.FeaturedRelation.Remove.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.FeaturedRelation.Remove.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error"></a>

### Rpc.Object.FeaturedRelation.Remove.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.FeaturedRelation.Remove.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Object.IdsSubscribe"></a>

### Rpc.Object.IdsSubscribe







<a name="anytype.Rpc.Object.IdsSubscribe.Request"></a>

### Rpc.Object.IdsSubscribe.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| subId | [string](#string) |  | (optional) subscription identifier client can provide some string or middleware will generate it automatically if subId is already registered on middleware, the new query will replace previous subscription |
| ids | [string](#string) | repeated | ids for subscribe |
| keys | [string](#string) | repeated | sorts (required) needed keys in details for return, for object fields mw will return (and subscribe) objects as dependent |
| ignoreWorkspace | [string](#string) |  |  |






<a name="anytype.Rpc.Object.IdsSubscribe.Response"></a>

### Rpc.Object.IdsSubscribe.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.IdsSubscribe.Response.Error](#anytype.Rpc.Object.IdsSubscribe.Response.Error) |  |  |
| records | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| dependencies | [google.protobuf.Struct](#google.protobuf.Struct) | repeated |  |
| subId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.IdsSubscribe.Response.Error"></a>

### Rpc.Object.IdsSubscribe.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.IdsSubscribe.Response.Error.Code](#anytype.Rpc.Object.IdsSubscribe.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationAdd"></a>

### Rpc.Object.RelationAdd







<a name="anytype.Rpc.Object.RelationAdd.Request"></a>

### Rpc.Object.RelationAdd.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Object.RelationAdd.Response"></a>

### Rpc.Object.RelationAdd.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationAdd.Response.Error](#anytype.Rpc.Object.RelationAdd.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| relationKey | [string](#string) |  | deprecated |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Object.RelationAdd.Response.Error"></a>

### Rpc.Object.RelationAdd.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationAdd.Response.Error.Code](#anytype.Rpc.Object.RelationAdd.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationDelete"></a>

### Rpc.Object.RelationDelete







<a name="anytype.Rpc.Object.RelationDelete.Request"></a>

### Rpc.Object.RelationDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationDelete.Response"></a>

### Rpc.Object.RelationDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationDelete.Response.Error](#anytype.Rpc.Object.RelationDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.RelationDelete.Response.Error"></a>

### Rpc.Object.RelationDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationDelete.Response.Error.Code](#anytype.Rpc.Object.RelationDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationListAvailable"></a>

### Rpc.Object.RelationListAvailable







<a name="anytype.Rpc.Object.RelationListAvailable.Request"></a>

### Rpc.Object.RelationListAvailable.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationListAvailable.Response"></a>

### Rpc.Object.RelationListAvailable.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationListAvailable.Response.Error](#anytype.Rpc.Object.RelationListAvailable.Response.Error) |  |  |
| relations | [model.Relation](#anytype.model.Relation) | repeated |  |






<a name="anytype.Rpc.Object.RelationListAvailable.Response.Error"></a>

### Rpc.Object.RelationListAvailable.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Object.RelationListAvailable.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationOptionAdd"></a>

### Rpc.Object.RelationOptionAdd
RelationOptionAdd may return existing option in case dataview already has one with the same text






<a name="anytype.Rpc.Object.RelationOptionAdd.Request"></a>

### Rpc.Object.RelationOptionAdd.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |






<a name="anytype.Rpc.Object.RelationOptionAdd.Response"></a>

### Rpc.Object.RelationOptionAdd.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationOptionAdd.Response.Error](#anytype.Rpc.Object.RelationOptionAdd.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  |  |






<a name="anytype.Rpc.Object.RelationOptionAdd.Response.Error"></a>

### Rpc.Object.RelationOptionAdd.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationOptionAdd.Response.Error.Code](#anytype.Rpc.Object.RelationOptionAdd.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationOptionDelete"></a>

### Rpc.Object.RelationOptionDelete







<a name="anytype.Rpc.Object.RelationOptionDelete.Request"></a>

### Rpc.Object.RelationOptionDelete.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | relation key to add the option |
| optionId | [string](#string) |  | id of select options to remove |
| confirmRemoveAllValuesInRecords | [bool](#bool) |  | confirm remove all values in records |






<a name="anytype.Rpc.Object.RelationOptionDelete.Response"></a>

### Rpc.Object.RelationOptionDelete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationOptionDelete.Response.Error](#anytype.Rpc.Object.RelationOptionDelete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.RelationOptionDelete.Response.Error"></a>

### Rpc.Object.RelationOptionDelete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationOptionDelete.Response.Error.Code](#anytype.Rpc.Object.RelationOptionDelete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationOptionUpdate"></a>

### Rpc.Object.RelationOptionUpdate







<a name="anytype.Rpc.Object.RelationOptionUpdate.Request"></a>

### Rpc.Object.RelationOptionUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | relation key to add the option |
| option | [model.Relation.Option](#anytype.model.Relation.Option) |  | id of select options will be autogenerated |






<a name="anytype.Rpc.Object.RelationOptionUpdate.Response"></a>

### Rpc.Object.RelationOptionUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationOptionUpdate.Response.Error](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.RelationOptionUpdate.Response.Error"></a>

### Rpc.Object.RelationOptionUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Object.RelationUpdate"></a>

### Rpc.Object.RelationUpdate







<a name="anytype.Rpc.Object.RelationUpdate.Request"></a>

### Rpc.Object.RelationUpdate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| relationKey | [string](#string) |  | key of relation to update |
| relation | [model.Relation](#anytype.model.Relation) |  |  |






<a name="anytype.Rpc.Object.RelationUpdate.Response"></a>

### Rpc.Object.RelationUpdate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationUpdate.Response.Error](#anytype.Rpc.Object.RelationUpdate.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Object.RelationUpdate.Response.Error"></a>

### Rpc.Object.RelationUpdate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationUpdate.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.ObjectDuplicate"></a>

### Rpc.ObjectDuplicate







<a name="anytype.Rpc.ObjectDuplicate.Request"></a>

### Rpc.ObjectDuplicate.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of template block for cloning |






<a name="anytype.Rpc.ObjectDuplicate.Response"></a>

### Rpc.ObjectDuplicate.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectDuplicate.Response.Error](#anytype.Rpc.ObjectDuplicate.Response.Error) |  |  |
| id | [string](#string) |  | created template id |






<a name="anytype.Rpc.ObjectDuplicate.Response.Error"></a>

### Rpc.ObjectDuplicate.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectDuplicate.Response.Error.Code](#anytype.Rpc.ObjectDuplicate.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectList"></a>

### Rpc.ObjectList







<a name="anytype.Rpc.ObjectList.Delete"></a>

### Rpc.ObjectList.Delete







<a name="anytype.Rpc.ObjectList.Delete.Request"></a>

### Rpc.ObjectList.Delete.Request
Deletes the object, keys from the local store and unsubscribe from remote changes. Also offloads all orphan files


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated | objects to remove |






<a name="anytype.Rpc.ObjectList.Delete.Response"></a>

### Rpc.ObjectList.Delete.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectList.Delete.Response.Error](#anytype.Rpc.ObjectList.Delete.Response.Error) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.ObjectList.Delete.Response.Error"></a>

### Rpc.ObjectList.Delete.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectList.Delete.Response.Error.Code](#anytype.Rpc.ObjectList.Delete.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectList.Set"></a>

### Rpc.ObjectList.Set







<a name="anytype.Rpc.ObjectList.Set.IsArchived"></a>

### Rpc.ObjectList.Set.IsArchived







<a name="anytype.Rpc.ObjectList.Set.IsArchived.Request"></a>

### Rpc.ObjectList.Set.IsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isArchived | [bool](#bool) |  |  |






<a name="anytype.Rpc.ObjectList.Set.IsArchived.Response"></a>

### Rpc.ObjectList.Set.IsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectList.Set.IsArchived.Response.Error](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error) |  |  |






<a name="anytype.Rpc.ObjectList.Set.IsArchived.Response.Error"></a>

### Rpc.ObjectList.Set.IsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectList.Set.IsArchived.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.ObjectList.Set.IsFavorite"></a>

### Rpc.ObjectList.Set.IsFavorite







<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Request"></a>

### Rpc.ObjectList.Set.IsFavorite.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isFavorite | [bool](#bool) |  |  |






<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Response"></a>

### Rpc.ObjectList.Set.IsFavorite.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectList.Set.IsFavorite.Response.Error](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error) |  |  |






<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error"></a>

### Rpc.ObjectList.Set.IsFavorite.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectList.Set.IsFavorite.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Page"></a>

### Rpc.Page







<a name="anytype.Rpc.Page.Create"></a>

### Rpc.Page.Create







<a name="anytype.Rpc.Page.Create.Request"></a>

### Rpc.Page.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | page details |






<a name="anytype.Rpc.Page.Create.Response"></a>

### Rpc.Page.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Page.Create.Response.Error](#anytype.Rpc.Page.Create.Response.Error) |  |  |
| pageId | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Page.Create.Response.Error"></a>

### Rpc.Page.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Page.Create.Response.Error.Code](#anytype.Rpc.Page.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Ping"></a>

### Rpc.Ping







<a name="anytype.Rpc.Ping.Request"></a>

### Rpc.Ping.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  |  |
| numberOfEventsToSend | [int32](#int32) |  |  |






<a name="anytype.Rpc.Ping.Response"></a>

### Rpc.Ping.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Ping.Response.Error](#anytype.Rpc.Ping.Response.Error) |  |  |
| index | [int32](#int32) |  |  |






<a name="anytype.Rpc.Ping.Response.Error"></a>

### Rpc.Ping.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Ping.Response.Error.Code](#anytype.Rpc.Ping.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Set"></a>

### Rpc.Set







<a name="anytype.Rpc.Set.Create"></a>

### Rpc.Set.Create







<a name="anytype.Rpc.Set.Create.Request"></a>

### Rpc.Set.Create.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | if omitted the name of page will be the same with object type |
| templateId | [string](#string) |  | optional template id for creating from template |






<a name="anytype.Rpc.Set.Create.Response"></a>

### Rpc.Set.Create.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Set.Create.Response.Error](#anytype.Rpc.Set.Create.Response.Error) |  |  |
| id | [string](#string) |  |  |
| event | [ResponseEvent](#anytype.ResponseEvent) |  |  |






<a name="anytype.Rpc.Set.Create.Response.Error"></a>

### Rpc.Set.Create.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Set.Create.Response.Error.Code](#anytype.Rpc.Set.Create.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Shutdown"></a>

### Rpc.Shutdown







<a name="anytype.Rpc.Shutdown.Request"></a>

### Rpc.Shutdown.Request







<a name="anytype.Rpc.Shutdown.Response"></a>

### Rpc.Shutdown.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Shutdown.Response.Error](#anytype.Rpc.Shutdown.Response.Error) |  |  |






<a name="anytype.Rpc.Shutdown.Response.Error"></a>

### Rpc.Shutdown.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Shutdown.Response.Error.Code](#anytype.Rpc.Shutdown.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.UnsplashDownload"></a>

### Rpc.UnsplashDownload







<a name="anytype.Rpc.UnsplashDownload.Request"></a>

### Rpc.UnsplashDownload.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pictureId | [string](#string) |  |  |






<a name="anytype.Rpc.UnsplashDownload.Response"></a>

### Rpc.UnsplashDownload.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.UnsplashDownload.Response.Error](#anytype.Rpc.UnsplashDownload.Response.Error) |  |  |
| hash | [string](#string) |  |  |






<a name="anytype.Rpc.UnsplashDownload.Response.Error"></a>

### Rpc.UnsplashDownload.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.UnsplashDownload.Response.Error.Code](#anytype.Rpc.UnsplashDownload.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.UnsplashSearch"></a>

### Rpc.UnsplashSearch







<a name="anytype.Rpc.UnsplashSearch.Request"></a>

### Rpc.UnsplashSearch.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| query | [string](#string) |  | empty means random images |
| limit | [int32](#int32) |  | may be omitted if the request was cached previously with another limit |






<a name="anytype.Rpc.UnsplashSearch.Response"></a>

### Rpc.UnsplashSearch.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.UnsplashSearch.Response.Error](#anytype.Rpc.UnsplashSearch.Response.Error) |  |  |
| pictures | [Rpc.UnsplashSearch.Response.Picture](#anytype.Rpc.UnsplashSearch.Response.Picture) | repeated |  |






<a name="anytype.Rpc.UnsplashSearch.Response.Error"></a>

### Rpc.UnsplashSearch.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.UnsplashSearch.Response.Error.Code](#anytype.Rpc.UnsplashSearch.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.UnsplashSearch.Response.Picture"></a>

### Rpc.UnsplashSearch.Response.Picture



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| url | [string](#string) |  |  |
| artist | [string](#string) |  |  |
| artistUrl | [string](#string) |  |  |






<a name="anytype.Rpc.UploadFile"></a>

### Rpc.UploadFile







<a name="anytype.Rpc.UploadFile.Request"></a>

### Rpc.UploadFile.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| type | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |
| disableEncryption | [bool](#bool) |  | deprecated, has no affect |
| style | [model.Block.Content.File.Style](#anytype.model.Block.Content.File.Style) |  |  |






<a name="anytype.Rpc.UploadFile.Response"></a>

### Rpc.UploadFile.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.UploadFile.Response.Error](#anytype.Rpc.UploadFile.Response.Error) |  |  |
| hash | [string](#string) |  |  |






<a name="anytype.Rpc.UploadFile.Response.Error"></a>

### Rpc.UploadFile.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.UploadFile.Response.Error.Code](#anytype.Rpc.UploadFile.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Version"></a>

### Rpc.Version
Get info about a version of a middleware.
Info is a string, that contains: BuildDate, GitCommit, GitBranch, GitState






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
| details | [string](#string) |  | build date, branch and commit |






<a name="anytype.Rpc.Version.Get.Response.Error"></a>

### Rpc.Version.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Wallet"></a>

### Rpc.Wallet
Namespace, that aggregates subtopics and actions, that relates to wallet.






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



<a name="anytype.Rpc.Account.Delete.Response.Error.Code"></a>

### Rpc.Account.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 | No error; |
| UNKNOWN_ERROR | 1 | Any other errors |
| BAD_INPUT | 2 |  |



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



<a name="anytype.Rpc.ApplyTemplate.Response.Error.Code"></a>

### Rpc.ApplyTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code"></a>

### Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code"></a>

### Rpc.Block.Bookmark.Fetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Close.Response.Error.Code"></a>

### Rpc.Block.Close.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



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



<a name="anytype.Rpc.Block.CreatePage.Response.Error.Code"></a>

### Rpc.Block.CreatePage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.CreateSet.Response.Error.Code"></a>

### Rpc.Block.CreateSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 | ... |



<a name="anytype.Rpc.Block.Cut.Response.Error.Code"></a>

### Rpc.Block.Cut.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.RecordCreate.Response.Error.Code"></a>

### Rpc.Block.Dataview.RecordCreate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.RecordDelete.Response.Error.Code"></a>

### Rpc.Block.Dataview.RecordDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code"></a>

### Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code"></a>

### Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code"></a>

### Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error.Code"></a>

### Rpc.Block.Dataview.RecordUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.RelationAdd.Response.Error.Code"></a>

### Rpc.Block.Dataview.RelationAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.RelationDelete.Response.Error.Code"></a>

### Rpc.Block.Dataview.RelationDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code"></a>

### Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_A_DATAVIEW_BLOCK | 3 | ... |



<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error.Code"></a>

### Rpc.Block.Dataview.RelationUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.SetSource.Response.Error.Code"></a>

### Rpc.Block.Dataview.SetSource.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Dataview.ViewCreate.Response.Error.Code"></a>

### Rpc.Block.Dataview.ViewCreate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.ViewDelete.Response.Error.Code"></a>

### Rpc.Block.Dataview.ViewDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error.Code"></a>

### Rpc.Block.Dataview.ViewSetActive.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code"></a>

### Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error.Code"></a>

### Rpc.Block.Dataview.ViewUpdate.Response.Error.Code


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



<a name="anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code"></a>

### Rpc.Block.File.CreateAndUpload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Get.Marks.Response.Error.Code"></a>

### Rpc.Block.Get.Marks.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.GetPublicWebURL.Response.Error.Code"></a>

### Rpc.Block.GetPublicWebURL.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.ImportMarkdown.Response.Error.Code"></a>

### Rpc.Block.ImportMarkdown.Response.Error.Code


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



<a name="anytype.Rpc.Block.ObjectType.Set.Response.Error.Code"></a>

### Rpc.Block.ObjectType.Set.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype.Rpc.Block.Open.Response.Error.Code"></a>

### Rpc.Block.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



<a name="anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code"></a>

### Rpc.Block.OpenBreadcrumbs.Response.Error.Code


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



<a name="anytype.Rpc.Block.Redo.Response.Error.Code"></a>

### Rpc.Block.Redo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype.Rpc.Block.Relation.Add.Response.Error.Code"></a>

### Rpc.Block.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Relation.SetKey.Response.Error.Code"></a>

### Rpc.Block.Relation.SetKey.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Block.Replace.Response.Error.Code"></a>

### Rpc.Block.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Details.Response.Error.Code"></a>

### Rpc.Block.Set.Details.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Fields.Response.Error.Code"></a>

### Rpc.Block.Set.Fields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.File.Name.Response.Error.Code"></a>

### Rpc.Block.Set.File.Name.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Image.Name.Response.Error.Code"></a>

### Rpc.Block.Set.Image.Name.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Image.Width.Response.Error.Code"></a>

### Rpc.Block.Set.Image.Width.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Latex.Text.Response.Error.Code"></a>

### Rpc.Block.Set.Latex.Text.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code"></a>

### Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code"></a>

### Rpc.Block.Set.Page.IsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Restrictions.Response.Error.Code"></a>

### Rpc.Block.Set.Restrictions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code"></a>

### Rpc.Block.Set.Text.Checked.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Text.Color.Response.Error.Code"></a>

### Rpc.Block.Set.Text.Color.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Text.Icon.Response.Error.Code"></a>

### Rpc.Block.Set.Text.Icon.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Text.Style.Response.Error.Code"></a>

### Rpc.Block.Set.Text.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Text.Text.Response.Error.Code"></a>

### Rpc.Block.Set.Text.Text.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Video.Name.Response.Error.Code"></a>

### Rpc.Block.Set.Video.Name.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Video.Width.Response.Error.Code"></a>

### Rpc.Block.Set.Video.Width.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.SetBreadcrumbs.Response.Error.Code"></a>

### Rpc.Block.SetBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Show.Response.Error.Code"></a>

### Rpc.Block.Show.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 | failed to read unknown data format – need to upgrade anytype |



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



<a name="anytype.Rpc.Block.Undo.Response.Error.Code"></a>

### Rpc.Block.Undo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 | ... |



<a name="anytype.Rpc.Block.Unlink.Response.Error.Code"></a>

### Rpc.Block.Unlink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.UpdateContent.Response.Error.Code"></a>

### Rpc.Block.UpdateContent.Response.Error.Code


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



<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code"></a>

### Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Duplicate.Response.Error.Code"></a>

### Rpc.BlockList.Duplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Move.Response.Error.Code"></a>

### Rpc.BlockList.Move.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code"></a>

### Rpc.BlockList.MoveToNewPage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Align.Response.Error.Code"></a>

### Rpc.BlockList.Set.Align.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code"></a>

### Rpc.BlockList.Set.BackgroundColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code"></a>

### Rpc.BlockList.Set.Div.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Fields.Response.Error.Code"></a>

### Rpc.BlockList.Set.Fields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.File.Style.Response.Error.Code"></a>

### Rpc.BlockList.Set.File.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code"></a>

### Rpc.BlockList.Set.Text.Color.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Text.Mark.Response.Error.Code"></a>

### Rpc.BlockList.Set.Text.Mark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code"></a>

### Rpc.BlockList.Set.Text.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.TurnInto.Response.Error.Code"></a>

### Rpc.BlockList.TurnInto.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.CloneTemplate.Response.Error.Code"></a>

### Rpc.CloneTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Config.Get.Response.Error.Code"></a>

### Rpc.Config.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |



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



<a name="anytype.Rpc.DownloadFile.Response.Error.Code"></a>

### Rpc.DownloadFile.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |



<a name="anytype.Rpc.Export.Format"></a>

### Rpc.Export.Format


| Name | Number | Description |
| ---- | ------ | ----------- |
| Markdown | 0 |  |
| Protobuf | 1 |  |
| JSON | 2 |  |
| DOT | 3 |  |
| SVG | 4 |  |
| GRAPH_JSON | 5 |  |



<a name="anytype.Rpc.Export.Response.Error.Code"></a>

### Rpc.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ExportLocalstore.Response.Error.Code"></a>

### Rpc.ExportLocalstore.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ExportTemplates.Response.Error.Code"></a>

### Rpc.ExportTemplates.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ExportWorkspace.Response.Error.Code"></a>

### Rpc.ExportWorkspace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ExternalDrop.Content.Response.Error.Code"></a>

### Rpc.ExternalDrop.Content.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ExternalDrop.Files.Response.Error.Code"></a>

### Rpc.ExternalDrop.Files.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.File.Offload.Response.Error.Code"></a>

### Rpc.File.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |
| FILE_NOT_YET_PINNED | 104 |  |



<a name="anytype.Rpc.FileList.Offload.Response.Error.Code"></a>

### Rpc.FileList.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 | ... |



<a name="anytype.Rpc.GenericErrorResponse.Error.Code"></a>

### Rpc.GenericErrorResponse.Error.Code


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



<a name="anytype.Rpc.History.Show.Response.Error.Code"></a>

### Rpc.History.Show.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.History.Versions.Response.Error.Code"></a>

### Rpc.History.Versions.Response.Error.Code


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



<a name="anytype.Rpc.MakeTemplate.Response.Error.Code"></a>

### Rpc.MakeTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.MakeTemplateByObjectType.Response.Error.Code"></a>

### Rpc.MakeTemplateByObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



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



<a name="anytype.Rpc.Object.FeaturedRelation.Add.Response.Error.Code"></a>

### Rpc.Object.FeaturedRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error.Code"></a>

### Rpc.Object.FeaturedRelation.Remove.Response.Error.Code


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



<a name="anytype.Rpc.Object.IdsSubscribe.Response.Error.Code"></a>

### Rpc.Object.IdsSubscribe.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.RelationAdd.Response.Error.Code"></a>

### Rpc.Object.RelationAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Object.RelationDelete.Response.Error.Code"></a>

### Rpc.Object.RelationDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Object.RelationListAvailable.Response.Error.Code"></a>

### Rpc.Object.RelationListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Object.RelationOptionAdd.Response.Error.Code"></a>

### Rpc.Object.RelationOptionAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Object.RelationOptionDelete.Response.Error.Code"></a>

### Rpc.Object.RelationOptionDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| SOME_RECORDS_HAS_RELATION_VALUE_WITH_THIS_OPTION | 3 | need to confirm with confirmRemoveAllValuesInRecords=true |



<a name="anytype.Rpc.Object.RelationOptionUpdate.Response.Error.Code"></a>

### Rpc.Object.RelationOptionUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Object.RelationUpdate.Response.Error.Code"></a>

### Rpc.Object.RelationUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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



<a name="anytype.Rpc.Object.ShareByLink.Response.Error.Code"></a>

### Rpc.Object.ShareByLink.Response.Error.Code


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



<a name="anytype.Rpc.ObjectDuplicate.Response.Error.Code"></a>

### Rpc.ObjectDuplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectList.Delete.Response.Error.Code"></a>

### Rpc.ObjectList.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectList.Set.IsArchived.Response.Error.Code"></a>

### Rpc.ObjectList.Set.IsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error.Code"></a>

### Rpc.ObjectList.Set.IsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



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



<a name="anytype.Rpc.Page.Create.Response.Error.Code"></a>

### Rpc.Page.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Ping.Response.Error.Code"></a>

### Rpc.Ping.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Process.Cancel.Response.Error.Code"></a>

### Rpc.Process.Cancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



<a name="anytype.Rpc.Set.Create.Response.Error.Code"></a>

### Rpc.Set.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |



<a name="anytype.Rpc.Shutdown.Response.Error.Code"></a>

### Rpc.Shutdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |



<a name="anytype.Rpc.UnsplashDownload.Response.Error.Code"></a>

### Rpc.UnsplashDownload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 | ... |



<a name="anytype.Rpc.UnsplashSearch.Response.Error.Code"></a>

### Rpc.UnsplashSearch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| RATE_LIMIT_EXCEEDED | 100 | ... |



<a name="anytype.Rpc.UploadFile.Response.Error.Code"></a>

### Rpc.UploadFile.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |



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
| info | [model.Account.Info](#anytype.model.Account.Info) |  |  |






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






<a name="anytype.model.Account.Info"></a>

### Account.Info



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Account.Status](#anytype.model.Account.Status) |  |  |
| toBeDeletedAt | [int64](#int64) |  |  |






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





 


<a name="anytype.model.Account.Status"></a>

### Account.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| IsActive | 0 |  |
| IsPendingDeletion | 1 |  |
| IsDeleted | 2 |  |



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

