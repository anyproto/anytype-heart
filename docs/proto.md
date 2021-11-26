# Protocol Documentation
<a name="top"/>

## Table of Contents
* [service.proto](#service.proto)
 * [ClientCommands](#anytype.ClientCommands)
* [changes.proto](#changes.proto)
 * [Change](#anytype.Change)
 * [Change.BlockCreate](#anytype.Change.BlockCreate)
 * [Change.BlockDuplicate](#anytype.Change.BlockDuplicate)
 * [Change.BlockMove](#anytype.Change.BlockMove)
 * [Change.BlockRemove](#anytype.Change.BlockRemove)
 * [Change.BlockUpdate](#anytype.Change.BlockUpdate)
 * [Change.Content](#anytype.Change.Content)
 * [Change.DetailsSet](#anytype.Change.DetailsSet)
 * [Change.DetailsUnset](#anytype.Change.DetailsUnset)
 * [Change.FileKeys](#anytype.Change.FileKeys)
 * [Change.FileKeys.KeysEntry](#anytype.Change.FileKeys.KeysEntry)
 * [Change.ObjectTypeAdd](#anytype.Change.ObjectTypeAdd)
 * [Change.ObjectTypeRemove](#anytype.Change.ObjectTypeRemove)
 * [Change.RelationAdd](#anytype.Change.RelationAdd)
 * [Change.RelationRemove](#anytype.Change.RelationRemove)
 * [Change.RelationUpdate](#anytype.Change.RelationUpdate)
 * [Change.RelationUpdate.Dict](#anytype.Change.RelationUpdate.Dict)
 * [Change.RelationUpdate.ObjectTypes](#anytype.Change.RelationUpdate.ObjectTypes)
 * [Change.Snapshot](#anytype.Change.Snapshot)
 * [Change.Snapshot.LogHeadsEntry](#anytype.Change.Snapshot.LogHeadsEntry)
 * [Change.StoreKeySet](#anytype.Change.StoreKeySet)
 * [Change.StoreKeyUnset](#anytype.Change.StoreKeyUnset)
* [commands.proto](#commands.proto)
 * [Empty](#anytype.Empty)
 * [Rpc](#anytype.Rpc)
 * [Rpc.Account](#anytype.Rpc.Account)
 * [Rpc.Account.Config](#anytype.Rpc.Account.Config)
 * [Rpc.Account.Create](#anytype.Rpc.Account.Create)
 * [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request)
 * [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response)
 * [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error)
 * [Rpc.Account.Recover](#anytype.Rpc.Account.Recover)
 * [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request)
 * [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response)
 * [Rpc.Account.Recover.Response.Error](#anytype.Rpc.Account.Recover.Response.Error)
 * [Rpc.Account.Select](#anytype.Rpc.Account.Select)
 * [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request)
 * [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response)
 * [Rpc.Account.Select.Response.Error](#anytype.Rpc.Account.Select.Response.Error)
 * [Rpc.Account.Stop](#anytype.Rpc.Account.Stop)
 * [Rpc.Account.Stop.Request](#anytype.Rpc.Account.Stop.Request)
 * [Rpc.Account.Stop.Response](#anytype.Rpc.Account.Stop.Response)
 * [Rpc.Account.Stop.Response.Error](#anytype.Rpc.Account.Stop.Response.Error)
 * [Rpc.ApplyTemplate](#anytype.Rpc.ApplyTemplate)
 * [Rpc.ApplyTemplate.Request](#anytype.Rpc.ApplyTemplate.Request)
 * [Rpc.ApplyTemplate.Response](#anytype.Rpc.ApplyTemplate.Response)
 * [Rpc.ApplyTemplate.Response.Error](#anytype.Rpc.ApplyTemplate.Response.Error)
 * [Rpc.Block](#anytype.Rpc.Block)
 * [Rpc.Block.Bookmark](#anytype.Rpc.Block.Bookmark)
 * [Rpc.Block.Bookmark.CreateAndFetch](#anytype.Rpc.Block.Bookmark.CreateAndFetch)
 * [Rpc.Block.Bookmark.CreateAndFetch.Request](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Request)
 * [Rpc.Block.Bookmark.CreateAndFetch.Response](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response)
 * [Rpc.Block.Bookmark.CreateAndFetch.Response.Error](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error)
 * [Rpc.Block.Bookmark.Fetch](#anytype.Rpc.Block.Bookmark.Fetch)
 * [Rpc.Block.Bookmark.Fetch.Request](#anytype.Rpc.Block.Bookmark.Fetch.Request)
 * [Rpc.Block.Bookmark.Fetch.Response](#anytype.Rpc.Block.Bookmark.Fetch.Response)
 * [Rpc.Block.Bookmark.Fetch.Response.Error](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error)
 * [Rpc.Block.Close](#anytype.Rpc.Block.Close)
 * [Rpc.Block.Close.Request](#anytype.Rpc.Block.Close.Request)
 * [Rpc.Block.Close.Response](#anytype.Rpc.Block.Close.Response)
 * [Rpc.Block.Close.Response.Error](#anytype.Rpc.Block.Close.Response.Error)
 * [Rpc.Block.Copy](#anytype.Rpc.Block.Copy)
 * [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request)
 * [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response)
 * [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error)
 * [Rpc.Block.Create](#anytype.Rpc.Block.Create)
 * [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request)
 * [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response)
 * [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error)
 * [Rpc.Block.CreatePage](#anytype.Rpc.Block.CreatePage)
 * [Rpc.Block.CreatePage.Request](#anytype.Rpc.Block.CreatePage.Request)
 * [Rpc.Block.CreatePage.Response](#anytype.Rpc.Block.CreatePage.Response)
 * [Rpc.Block.CreatePage.Response.Error](#anytype.Rpc.Block.CreatePage.Response.Error)
 * [Rpc.Block.CreateSet](#anytype.Rpc.Block.CreateSet)
 * [Rpc.Block.CreateSet.Request](#anytype.Rpc.Block.CreateSet.Request)
 * [Rpc.Block.CreateSet.Response](#anytype.Rpc.Block.CreateSet.Response)
 * [Rpc.Block.CreateSet.Response.Error](#anytype.Rpc.Block.CreateSet.Response.Error)
 * [Rpc.Block.Cut](#anytype.Rpc.Block.Cut)
 * [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request)
 * [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response)
 * [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error)
 * [Rpc.Block.Dataview](#anytype.Rpc.Block.Dataview)
 * [Rpc.Block.Dataview.RecordCreate](#anytype.Rpc.Block.Dataview.RecordCreate)
 * [Rpc.Block.Dataview.RecordCreate.Request](#anytype.Rpc.Block.Dataview.RecordCreate.Request)
 * [Rpc.Block.Dataview.RecordCreate.Response](#anytype.Rpc.Block.Dataview.RecordCreate.Response)
 * [Rpc.Block.Dataview.RecordCreate.Response.Error](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error)
 * [Rpc.Block.Dataview.RecordDelete](#anytype.Rpc.Block.Dataview.RecordDelete)
 * [Rpc.Block.Dataview.RecordDelete.Request](#anytype.Rpc.Block.Dataview.RecordDelete.Request)
 * [Rpc.Block.Dataview.RecordDelete.Response](#anytype.Rpc.Block.Dataview.RecordDelete.Response)
 * [Rpc.Block.Dataview.RecordDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error)
 * [Rpc.Block.Dataview.RecordRelationOptionAdd](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd)
 * [Rpc.Block.Dataview.RecordRelationOptionAdd.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Request)
 * [Rpc.Block.Dataview.RecordRelationOptionAdd.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response)
 * [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error)
 * [Rpc.Block.Dataview.RecordRelationOptionDelete](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete)
 * [Rpc.Block.Dataview.RecordRelationOptionDelete.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Request)
 * [Rpc.Block.Dataview.RecordRelationOptionDelete.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response)
 * [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error)
 * [Rpc.Block.Dataview.RecordRelationOptionUpdate](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate)
 * [Rpc.Block.Dataview.RecordRelationOptionUpdate.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Request)
 * [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response)
 * [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error)
 * [Rpc.Block.Dataview.RecordUpdate](#anytype.Rpc.Block.Dataview.RecordUpdate)
 * [Rpc.Block.Dataview.RecordUpdate.Request](#anytype.Rpc.Block.Dataview.RecordUpdate.Request)
 * [Rpc.Block.Dataview.RecordUpdate.Response](#anytype.Rpc.Block.Dataview.RecordUpdate.Response)
 * [Rpc.Block.Dataview.RecordUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error)
 * [Rpc.Block.Dataview.RelationAdd](#anytype.Rpc.Block.Dataview.RelationAdd)
 * [Rpc.Block.Dataview.RelationAdd.Request](#anytype.Rpc.Block.Dataview.RelationAdd.Request)
 * [Rpc.Block.Dataview.RelationAdd.Response](#anytype.Rpc.Block.Dataview.RelationAdd.Response)
 * [Rpc.Block.Dataview.RelationAdd.Response.Error](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error)
 * [Rpc.Block.Dataview.RelationDelete](#anytype.Rpc.Block.Dataview.RelationDelete)
 * [Rpc.Block.Dataview.RelationDelete.Request](#anytype.Rpc.Block.Dataview.RelationDelete.Request)
 * [Rpc.Block.Dataview.RelationDelete.Response](#anytype.Rpc.Block.Dataview.RelationDelete.Response)
 * [Rpc.Block.Dataview.RelationDelete.Response.Error](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error)
 * [Rpc.Block.Dataview.RelationListAvailable](#anytype.Rpc.Block.Dataview.RelationListAvailable)
 * [Rpc.Block.Dataview.RelationListAvailable.Request](#anytype.Rpc.Block.Dataview.RelationListAvailable.Request)
 * [Rpc.Block.Dataview.RelationListAvailable.Response](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response)
 * [Rpc.Block.Dataview.RelationListAvailable.Response.Error](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error)
 * [Rpc.Block.Dataview.RelationUpdate](#anytype.Rpc.Block.Dataview.RelationUpdate)
 * [Rpc.Block.Dataview.RelationUpdate.Request](#anytype.Rpc.Block.Dataview.RelationUpdate.Request)
 * [Rpc.Block.Dataview.RelationUpdate.Response](#anytype.Rpc.Block.Dataview.RelationUpdate.Response)
 * [Rpc.Block.Dataview.RelationUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error)
 * [Rpc.Block.Dataview.SetSource](#anytype.Rpc.Block.Dataview.SetSource)
 * [Rpc.Block.Dataview.SetSource.Request](#anytype.Rpc.Block.Dataview.SetSource.Request)
 * [Rpc.Block.Dataview.SetSource.Response](#anytype.Rpc.Block.Dataview.SetSource.Response)
 * [Rpc.Block.Dataview.SetSource.Response.Error](#anytype.Rpc.Block.Dataview.SetSource.Response.Error)
 * [Rpc.Block.Dataview.ViewCreate](#anytype.Rpc.Block.Dataview.ViewCreate)
 * [Rpc.Block.Dataview.ViewCreate.Request](#anytype.Rpc.Block.Dataview.ViewCreate.Request)
 * [Rpc.Block.Dataview.ViewCreate.Response](#anytype.Rpc.Block.Dataview.ViewCreate.Response)
 * [Rpc.Block.Dataview.ViewCreate.Response.Error](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error)
 * [Rpc.Block.Dataview.ViewDelete](#anytype.Rpc.Block.Dataview.ViewDelete)
 * [Rpc.Block.Dataview.ViewDelete.Request](#anytype.Rpc.Block.Dataview.ViewDelete.Request)
 * [Rpc.Block.Dataview.ViewDelete.Response](#anytype.Rpc.Block.Dataview.ViewDelete.Response)
 * [Rpc.Block.Dataview.ViewDelete.Response.Error](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error)
 * [Rpc.Block.Dataview.ViewSetActive](#anytype.Rpc.Block.Dataview.ViewSetActive)
 * [Rpc.Block.Dataview.ViewSetActive.Request](#anytype.Rpc.Block.Dataview.ViewSetActive.Request)
 * [Rpc.Block.Dataview.ViewSetActive.Response](#anytype.Rpc.Block.Dataview.ViewSetActive.Response)
 * [Rpc.Block.Dataview.ViewSetActive.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error)
 * [Rpc.Block.Dataview.ViewSetPosition](#anytype.Rpc.Block.Dataview.ViewSetPosition)
 * [Rpc.Block.Dataview.ViewSetPosition.Request](#anytype.Rpc.Block.Dataview.ViewSetPosition.Request)
 * [Rpc.Block.Dataview.ViewSetPosition.Response](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response)
 * [Rpc.Block.Dataview.ViewSetPosition.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error)
 * [Rpc.Block.Dataview.ViewUpdate](#anytype.Rpc.Block.Dataview.ViewUpdate)
 * [Rpc.Block.Dataview.ViewUpdate.Request](#anytype.Rpc.Block.Dataview.ViewUpdate.Request)
 * [Rpc.Block.Dataview.ViewUpdate.Response](#anytype.Rpc.Block.Dataview.ViewUpdate.Response)
 * [Rpc.Block.Dataview.ViewUpdate.Response.Error](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error)
 * [Rpc.Block.Download](#anytype.Rpc.Block.Download)
 * [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request)
 * [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response)
 * [Rpc.Block.Download.Response.Error](#anytype.Rpc.Block.Download.Response.Error)
 * [Rpc.Block.Export](#anytype.Rpc.Block.Export)
 * [Rpc.Block.Export.Request](#anytype.Rpc.Block.Export.Request)
 * [Rpc.Block.Export.Response](#anytype.Rpc.Block.Export.Response)
 * [Rpc.Block.Export.Response.Error](#anytype.Rpc.Block.Export.Response.Error)
 * [Rpc.Block.File](#anytype.Rpc.Block.File)
 * [Rpc.Block.File.CreateAndUpload](#anytype.Rpc.Block.File.CreateAndUpload)
 * [Rpc.Block.File.CreateAndUpload.Request](#anytype.Rpc.Block.File.CreateAndUpload.Request)
 * [Rpc.Block.File.CreateAndUpload.Response](#anytype.Rpc.Block.File.CreateAndUpload.Response)
 * [Rpc.Block.File.CreateAndUpload.Response.Error](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error)
 * [Rpc.Block.Get](#anytype.Rpc.Block.Get)
 * [Rpc.Block.Get.Marks](#anytype.Rpc.Block.Get.Marks)
 * [Rpc.Block.Get.Marks.Request](#anytype.Rpc.Block.Get.Marks.Request)
 * [Rpc.Block.Get.Marks.Response](#anytype.Rpc.Block.Get.Marks.Response)
 * [Rpc.Block.Get.Marks.Response.Error](#anytype.Rpc.Block.Get.Marks.Response.Error)
 * [Rpc.Block.GetPublicWebURL](#anytype.Rpc.Block.GetPublicWebURL)
 * [Rpc.Block.GetPublicWebURL.Request](#anytype.Rpc.Block.GetPublicWebURL.Request)
 * [Rpc.Block.GetPublicWebURL.Response](#anytype.Rpc.Block.GetPublicWebURL.Response)
 * [Rpc.Block.GetPublicWebURL.Response.Error](#anytype.Rpc.Block.GetPublicWebURL.Response.Error)
 * [Rpc.Block.ImportMarkdown](#anytype.Rpc.Block.ImportMarkdown)
 * [Rpc.Block.ImportMarkdown.Request](#anytype.Rpc.Block.ImportMarkdown.Request)
 * [Rpc.Block.ImportMarkdown.Response](#anytype.Rpc.Block.ImportMarkdown.Response)
 * [Rpc.Block.ImportMarkdown.Response.Error](#anytype.Rpc.Block.ImportMarkdown.Response.Error)
 * [Rpc.Block.Merge](#anytype.Rpc.Block.Merge)
 * [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request)
 * [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response)
 * [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error)
 * [Rpc.Block.ObjectType](#anytype.Rpc.Block.ObjectType)
 * [Rpc.Block.ObjectType.Set](#anytype.Rpc.Block.ObjectType.Set)
 * [Rpc.Block.ObjectType.Set.Request](#anytype.Rpc.Block.ObjectType.Set.Request)
 * [Rpc.Block.ObjectType.Set.Response](#anytype.Rpc.Block.ObjectType.Set.Response)
 * [Rpc.Block.ObjectType.Set.Response.Error](#anytype.Rpc.Block.ObjectType.Set.Response.Error)
 * [Rpc.Block.Open](#anytype.Rpc.Block.Open)
 * [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request)
 * [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response)
 * [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error)
 * [Rpc.Block.OpenBreadcrumbs](#anytype.Rpc.Block.OpenBreadcrumbs)
 * [Rpc.Block.OpenBreadcrumbs.Request](#anytype.Rpc.Block.OpenBreadcrumbs.Request)
 * [Rpc.Block.OpenBreadcrumbs.Response](#anytype.Rpc.Block.OpenBreadcrumbs.Response)
 * [Rpc.Block.OpenBreadcrumbs.Response.Error](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error)
 * [Rpc.Block.Paste](#anytype.Rpc.Block.Paste)
 * [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request)
 * [Rpc.Block.Paste.Request.File](#anytype.Rpc.Block.Paste.Request.File)
 * [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response)
 * [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error)
 * [Rpc.Block.Redo](#anytype.Rpc.Block.Redo)
 * [Rpc.Block.Redo.Request](#anytype.Rpc.Block.Redo.Request)
 * [Rpc.Block.Redo.Response](#anytype.Rpc.Block.Redo.Response)
 * [Rpc.Block.Redo.Response.Error](#anytype.Rpc.Block.Redo.Response.Error)
 * [Rpc.Block.Relation](#anytype.Rpc.Block.Relation)
 * [Rpc.Block.Relation.Add](#anytype.Rpc.Block.Relation.Add)
 * [Rpc.Block.Relation.Add.Request](#anytype.Rpc.Block.Relation.Add.Request)
 * [Rpc.Block.Relation.Add.Response](#anytype.Rpc.Block.Relation.Add.Response)
 * [Rpc.Block.Relation.Add.Response.Error](#anytype.Rpc.Block.Relation.Add.Response.Error)
 * [Rpc.Block.Relation.SetKey](#anytype.Rpc.Block.Relation.SetKey)
 * [Rpc.Block.Relation.SetKey.Request](#anytype.Rpc.Block.Relation.SetKey.Request)
 * [Rpc.Block.Relation.SetKey.Response](#anytype.Rpc.Block.Relation.SetKey.Response)
 * [Rpc.Block.Relation.SetKey.Response.Error](#anytype.Rpc.Block.Relation.SetKey.Response.Error)
 * [Rpc.Block.Replace](#anytype.Rpc.Block.Replace)
 * [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request)
 * [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response)
 * [Rpc.Block.Replace.Response.Error](#anytype.Rpc.Block.Replace.Response.Error)
 * [Rpc.Block.Set](#anytype.Rpc.Block.Set)
 * [Rpc.Block.Set.Details](#anytype.Rpc.Block.Set.Details)
 * [Rpc.Block.Set.Details.Detail](#anytype.Rpc.Block.Set.Details.Detail)
 * [Rpc.Block.Set.Details.Request](#anytype.Rpc.Block.Set.Details.Request)
 * [Rpc.Block.Set.Details.Response](#anytype.Rpc.Block.Set.Details.Response)
 * [Rpc.Block.Set.Details.Response.Error](#anytype.Rpc.Block.Set.Details.Response.Error)
 * [Rpc.Block.Set.Fields](#anytype.Rpc.Block.Set.Fields)
 * [Rpc.Block.Set.Fields.Request](#anytype.Rpc.Block.Set.Fields.Request)
 * [Rpc.Block.Set.Fields.Response](#anytype.Rpc.Block.Set.Fields.Response)
 * [Rpc.Block.Set.Fields.Response.Error](#anytype.Rpc.Block.Set.Fields.Response.Error)
 * [Rpc.Block.Set.File](#anytype.Rpc.Block.Set.File)
 * [Rpc.Block.Set.File.Name](#anytype.Rpc.Block.Set.File.Name)
 * [Rpc.Block.Set.File.Name.Request](#anytype.Rpc.Block.Set.File.Name.Request)
 * [Rpc.Block.Set.File.Name.Response](#anytype.Rpc.Block.Set.File.Name.Response)
 * [Rpc.Block.Set.File.Name.Response.Error](#anytype.Rpc.Block.Set.File.Name.Response.Error)
 * [Rpc.Block.Set.Image](#anytype.Rpc.Block.Set.Image)
 * [Rpc.Block.Set.Image.Name](#anytype.Rpc.Block.Set.Image.Name)
 * [Rpc.Block.Set.Image.Name.Request](#anytype.Rpc.Block.Set.Image.Name.Request)
 * [Rpc.Block.Set.Image.Name.Response](#anytype.Rpc.Block.Set.Image.Name.Response)
 * [Rpc.Block.Set.Image.Name.Response.Error](#anytype.Rpc.Block.Set.Image.Name.Response.Error)
 * [Rpc.Block.Set.Image.Width](#anytype.Rpc.Block.Set.Image.Width)
 * [Rpc.Block.Set.Image.Width.Request](#anytype.Rpc.Block.Set.Image.Width.Request)
 * [Rpc.Block.Set.Image.Width.Response](#anytype.Rpc.Block.Set.Image.Width.Response)
 * [Rpc.Block.Set.Image.Width.Response.Error](#anytype.Rpc.Block.Set.Image.Width.Response.Error)
 * [Rpc.Block.Set.Latex](#anytype.Rpc.Block.Set.Latex)
 * [Rpc.Block.Set.Latex.Text](#anytype.Rpc.Block.Set.Latex.Text)
 * [Rpc.Block.Set.Latex.Text.Request](#anytype.Rpc.Block.Set.Latex.Text.Request)
 * [Rpc.Block.Set.Latex.Text.Response](#anytype.Rpc.Block.Set.Latex.Text.Response)
 * [Rpc.Block.Set.Latex.Text.Response.Error](#anytype.Rpc.Block.Set.Latex.Text.Response.Error)
 * [Rpc.Block.Set.Link](#anytype.Rpc.Block.Set.Link)
 * [Rpc.Block.Set.Link.TargetBlockId](#anytype.Rpc.Block.Set.Link.TargetBlockId)
 * [Rpc.Block.Set.Link.TargetBlockId.Request](#anytype.Rpc.Block.Set.Link.TargetBlockId.Request)
 * [Rpc.Block.Set.Link.TargetBlockId.Response](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response)
 * [Rpc.Block.Set.Link.TargetBlockId.Response.Error](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error)
 * [Rpc.Block.Set.Page](#anytype.Rpc.Block.Set.Page)
 * [Rpc.Block.Set.Page.IsArchived](#anytype.Rpc.Block.Set.Page.IsArchived)
 * [Rpc.Block.Set.Page.IsArchived.Request](#anytype.Rpc.Block.Set.Page.IsArchived.Request)
 * [Rpc.Block.Set.Page.IsArchived.Response](#anytype.Rpc.Block.Set.Page.IsArchived.Response)
 * [Rpc.Block.Set.Page.IsArchived.Response.Error](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error)
 * [Rpc.Block.Set.Restrictions](#anytype.Rpc.Block.Set.Restrictions)
 * [Rpc.Block.Set.Restrictions.Request](#anytype.Rpc.Block.Set.Restrictions.Request)
 * [Rpc.Block.Set.Restrictions.Response](#anytype.Rpc.Block.Set.Restrictions.Response)
 * [Rpc.Block.Set.Restrictions.Response.Error](#anytype.Rpc.Block.Set.Restrictions.Response.Error)
 * [Rpc.Block.Set.Text](#anytype.Rpc.Block.Set.Text)
 * [Rpc.Block.Set.Text.Checked](#anytype.Rpc.Block.Set.Text.Checked)
 * [Rpc.Block.Set.Text.Checked.Request](#anytype.Rpc.Block.Set.Text.Checked.Request)
 * [Rpc.Block.Set.Text.Checked.Response](#anytype.Rpc.Block.Set.Text.Checked.Response)
 * [Rpc.Block.Set.Text.Checked.Response.Error](#anytype.Rpc.Block.Set.Text.Checked.Response.Error)
 * [Rpc.Block.Set.Text.Color](#anytype.Rpc.Block.Set.Text.Color)
 * [Rpc.Block.Set.Text.Color.Request](#anytype.Rpc.Block.Set.Text.Color.Request)
 * [Rpc.Block.Set.Text.Color.Response](#anytype.Rpc.Block.Set.Text.Color.Response)
 * [Rpc.Block.Set.Text.Color.Response.Error](#anytype.Rpc.Block.Set.Text.Color.Response.Error)
 * [Rpc.Block.Set.Text.Style](#anytype.Rpc.Block.Set.Text.Style)
 * [Rpc.Block.Set.Text.Style.Request](#anytype.Rpc.Block.Set.Text.Style.Request)
 * [Rpc.Block.Set.Text.Style.Response](#anytype.Rpc.Block.Set.Text.Style.Response)
 * [Rpc.Block.Set.Text.Style.Response.Error](#anytype.Rpc.Block.Set.Text.Style.Response.Error)
 * [Rpc.Block.Set.Text.Text](#anytype.Rpc.Block.Set.Text.Text)
 * [Rpc.Block.Set.Text.Text.Request](#anytype.Rpc.Block.Set.Text.Text.Request)
 * [Rpc.Block.Set.Text.Text.Response](#anytype.Rpc.Block.Set.Text.Text.Response)
 * [Rpc.Block.Set.Text.Text.Response.Error](#anytype.Rpc.Block.Set.Text.Text.Response.Error)
 * [Rpc.Block.Set.Video](#anytype.Rpc.Block.Set.Video)
 * [Rpc.Block.Set.Video.Name](#anytype.Rpc.Block.Set.Video.Name)
 * [Rpc.Block.Set.Video.Name.Request](#anytype.Rpc.Block.Set.Video.Name.Request)
 * [Rpc.Block.Set.Video.Name.Response](#anytype.Rpc.Block.Set.Video.Name.Response)
 * [Rpc.Block.Set.Video.Name.Response.Error](#anytype.Rpc.Block.Set.Video.Name.Response.Error)
 * [Rpc.Block.Set.Video.Width](#anytype.Rpc.Block.Set.Video.Width)
 * [Rpc.Block.Set.Video.Width.Request](#anytype.Rpc.Block.Set.Video.Width.Request)
 * [Rpc.Block.Set.Video.Width.Response](#anytype.Rpc.Block.Set.Video.Width.Response)
 * [Rpc.Block.Set.Video.Width.Response.Error](#anytype.Rpc.Block.Set.Video.Width.Response.Error)
 * [Rpc.Block.SetBreadcrumbs](#anytype.Rpc.Block.SetBreadcrumbs)
 * [Rpc.Block.SetBreadcrumbs.Request](#anytype.Rpc.Block.SetBreadcrumbs.Request)
 * [Rpc.Block.SetBreadcrumbs.Response](#anytype.Rpc.Block.SetBreadcrumbs.Response)
 * [Rpc.Block.SetBreadcrumbs.Response.Error](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error)
 * [Rpc.Block.Show](#anytype.Rpc.Block.Show)
 * [Rpc.Block.Show.Request](#anytype.Rpc.Block.Show.Request)
 * [Rpc.Block.Show.Response](#anytype.Rpc.Block.Show.Response)
 * [Rpc.Block.Show.Response.Error](#anytype.Rpc.Block.Show.Response.Error)
 * [Rpc.Block.Split](#anytype.Rpc.Block.Split)
 * [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request)
 * [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response)
 * [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error)
 * [Rpc.Block.Undo](#anytype.Rpc.Block.Undo)
 * [Rpc.Block.Undo.Request](#anytype.Rpc.Block.Undo.Request)
 * [Rpc.Block.Undo.Response](#anytype.Rpc.Block.Undo.Response)
 * [Rpc.Block.Undo.Response.Error](#anytype.Rpc.Block.Undo.Response.Error)
 * [Rpc.Block.UndoRedoCounter](#anytype.Rpc.Block.UndoRedoCounter)
 * [Rpc.Block.Unlink](#anytype.Rpc.Block.Unlink)
 * [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request)
 * [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response)
 * [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error)
 * [Rpc.Block.UpdateContent](#anytype.Rpc.Block.UpdateContent)
 * [Rpc.Block.UpdateContent.Request](#anytype.Rpc.Block.UpdateContent.Request)
 * [Rpc.Block.UpdateContent.Response](#anytype.Rpc.Block.UpdateContent.Response)
 * [Rpc.Block.UpdateContent.Response.Error](#anytype.Rpc.Block.UpdateContent.Response.Error)
 * [Rpc.Block.Upload](#anytype.Rpc.Block.Upload)
 * [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request)
 * [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response)
 * [Rpc.Block.Upload.Response.Error](#anytype.Rpc.Block.Upload.Response.Error)
 * [Rpc.BlockList](#anytype.Rpc.BlockList)
 * [Rpc.BlockList.ConvertChildrenToPages](#anytype.Rpc.BlockList.ConvertChildrenToPages)
 * [Rpc.BlockList.ConvertChildrenToPages.Request](#anytype.Rpc.BlockList.ConvertChildrenToPages.Request)
 * [Rpc.BlockList.ConvertChildrenToPages.Response](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response)
 * [Rpc.BlockList.ConvertChildrenToPages.Response.Error](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error)
 * [Rpc.BlockList.Duplicate](#anytype.Rpc.BlockList.Duplicate)
 * [Rpc.BlockList.Duplicate.Request](#anytype.Rpc.BlockList.Duplicate.Request)
 * [Rpc.BlockList.Duplicate.Response](#anytype.Rpc.BlockList.Duplicate.Response)
 * [Rpc.BlockList.Duplicate.Response.Error](#anytype.Rpc.BlockList.Duplicate.Response.Error)
 * [Rpc.BlockList.Move](#anytype.Rpc.BlockList.Move)
 * [Rpc.BlockList.Move.Request](#anytype.Rpc.BlockList.Move.Request)
 * [Rpc.BlockList.Move.Response](#anytype.Rpc.BlockList.Move.Response)
 * [Rpc.BlockList.Move.Response.Error](#anytype.Rpc.BlockList.Move.Response.Error)
 * [Rpc.BlockList.MoveToNewPage](#anytype.Rpc.BlockList.MoveToNewPage)
 * [Rpc.BlockList.MoveToNewPage.Request](#anytype.Rpc.BlockList.MoveToNewPage.Request)
 * [Rpc.BlockList.MoveToNewPage.Response](#anytype.Rpc.BlockList.MoveToNewPage.Response)
 * [Rpc.BlockList.MoveToNewPage.Response.Error](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error)
 * [Rpc.BlockList.Set](#anytype.Rpc.BlockList.Set)
 * [Rpc.BlockList.Set.Align](#anytype.Rpc.BlockList.Set.Align)
 * [Rpc.BlockList.Set.Align.Request](#anytype.Rpc.BlockList.Set.Align.Request)
 * [Rpc.BlockList.Set.Align.Response](#anytype.Rpc.BlockList.Set.Align.Response)
 * [Rpc.BlockList.Set.Align.Response.Error](#anytype.Rpc.BlockList.Set.Align.Response.Error)
 * [Rpc.BlockList.Set.BackgroundColor](#anytype.Rpc.BlockList.Set.BackgroundColor)
 * [Rpc.BlockList.Set.BackgroundColor.Request](#anytype.Rpc.BlockList.Set.BackgroundColor.Request)
 * [Rpc.BlockList.Set.BackgroundColor.Response](#anytype.Rpc.BlockList.Set.BackgroundColor.Response)
 * [Rpc.BlockList.Set.BackgroundColor.Response.Error](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error)
 * [Rpc.BlockList.Set.Div](#anytype.Rpc.BlockList.Set.Div)
 * [Rpc.BlockList.Set.Div.Style](#anytype.Rpc.BlockList.Set.Div.Style)
 * [Rpc.BlockList.Set.Div.Style.Request](#anytype.Rpc.BlockList.Set.Div.Style.Request)
 * [Rpc.BlockList.Set.Div.Style.Response](#anytype.Rpc.BlockList.Set.Div.Style.Response)
 * [Rpc.BlockList.Set.Div.Style.Response.Error](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error)
 * [Rpc.BlockList.Set.Fields](#anytype.Rpc.BlockList.Set.Fields)
 * [Rpc.BlockList.Set.Fields.Request](#anytype.Rpc.BlockList.Set.Fields.Request)
 * [Rpc.BlockList.Set.Fields.Request.BlockField](#anytype.Rpc.BlockList.Set.Fields.Request.BlockField)
 * [Rpc.BlockList.Set.Fields.Response](#anytype.Rpc.BlockList.Set.Fields.Response)
 * [Rpc.BlockList.Set.Fields.Response.Error](#anytype.Rpc.BlockList.Set.Fields.Response.Error)
 * [Rpc.BlockList.Set.File](#anytype.Rpc.BlockList.Set.File)
 * [Rpc.BlockList.Set.File.Style](#anytype.Rpc.BlockList.Set.File.Style)
 * [Rpc.BlockList.Set.File.Style.Request](#anytype.Rpc.BlockList.Set.File.Style.Request)
 * [Rpc.BlockList.Set.File.Style.Response](#anytype.Rpc.BlockList.Set.File.Style.Response)
 * [Rpc.BlockList.Set.File.Style.Response.Error](#anytype.Rpc.BlockList.Set.File.Style.Response.Error)
 * [Rpc.BlockList.Set.Text](#anytype.Rpc.BlockList.Set.Text)
 * [Rpc.BlockList.Set.Text.Color](#anytype.Rpc.BlockList.Set.Text.Color)
 * [Rpc.BlockList.Set.Text.Color.Request](#anytype.Rpc.BlockList.Set.Text.Color.Request)
 * [Rpc.BlockList.Set.Text.Color.Response](#anytype.Rpc.BlockList.Set.Text.Color.Response)
 * [Rpc.BlockList.Set.Text.Color.Response.Error](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error)
 * [Rpc.BlockList.Set.Text.Mark](#anytype.Rpc.BlockList.Set.Text.Mark)
 * [Rpc.BlockList.Set.Text.Mark.Request](#anytype.Rpc.BlockList.Set.Text.Mark.Request)
 * [Rpc.BlockList.Set.Text.Mark.Response](#anytype.Rpc.BlockList.Set.Text.Mark.Response)
 * [Rpc.BlockList.Set.Text.Mark.Response.Error](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error)
 * [Rpc.BlockList.Set.Text.Style](#anytype.Rpc.BlockList.Set.Text.Style)
 * [Rpc.BlockList.Set.Text.Style.Request](#anytype.Rpc.BlockList.Set.Text.Style.Request)
 * [Rpc.BlockList.Set.Text.Style.Response](#anytype.Rpc.BlockList.Set.Text.Style.Response)
 * [Rpc.BlockList.Set.Text.Style.Response.Error](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error)
 * [Rpc.BlockList.TurnInto](#anytype.Rpc.BlockList.TurnInto)
 * [Rpc.BlockList.TurnInto.Request](#anytype.Rpc.BlockList.TurnInto.Request)
 * [Rpc.BlockList.TurnInto.Response](#anytype.Rpc.BlockList.TurnInto.Response)
 * [Rpc.BlockList.TurnInto.Response.Error](#anytype.Rpc.BlockList.TurnInto.Response.Error)
 * [Rpc.CloneTemplate](#anytype.Rpc.CloneTemplate)
 * [Rpc.CloneTemplate.Request](#anytype.Rpc.CloneTemplate.Request)
 * [Rpc.CloneTemplate.Response](#anytype.Rpc.CloneTemplate.Response)
 * [Rpc.CloneTemplate.Response.Error](#anytype.Rpc.CloneTemplate.Response.Error)
 * [Rpc.Config](#anytype.Rpc.Config)
 * [Rpc.Config.Get](#anytype.Rpc.Config.Get)
 * [Rpc.Config.Get.Request](#anytype.Rpc.Config.Get.Request)
 * [Rpc.Config.Get.Response](#anytype.Rpc.Config.Get.Response)
 * [Rpc.Config.Get.Response.Error](#anytype.Rpc.Config.Get.Response.Error)
 * [Rpc.Debug](#anytype.Rpc.Debug)
 * [Rpc.Debug.Sync](#anytype.Rpc.Debug.Sync)
 * [Rpc.Debug.Sync.Request](#anytype.Rpc.Debug.Sync.Request)
 * [Rpc.Debug.Sync.Response](#anytype.Rpc.Debug.Sync.Response)
 * [Rpc.Debug.Sync.Response.Error](#anytype.Rpc.Debug.Sync.Response.Error)
 * [Rpc.Debug.Thread](#anytype.Rpc.Debug.Thread)
 * [Rpc.Debug.Thread.Request](#anytype.Rpc.Debug.Thread.Request)
 * [Rpc.Debug.Thread.Response](#anytype.Rpc.Debug.Thread.Response)
 * [Rpc.Debug.Thread.Response.Error](#anytype.Rpc.Debug.Thread.Response.Error)
 * [Rpc.Debug.Tree](#anytype.Rpc.Debug.Tree)
 * [Rpc.Debug.Tree.Request](#anytype.Rpc.Debug.Tree.Request)
 * [Rpc.Debug.Tree.Response](#anytype.Rpc.Debug.Tree.Response)
 * [Rpc.Debug.Tree.Response.Error](#anytype.Rpc.Debug.Tree.Response.Error)
 * [Rpc.Debug.logInfo](#anytype.Rpc.Debug.logInfo)
 * [Rpc.Debug.threadInfo](#anytype.Rpc.Debug.threadInfo)
 * [Rpc.DownloadFile](#anytype.Rpc.DownloadFile)
 * [Rpc.DownloadFile.Request](#anytype.Rpc.DownloadFile.Request)
 * [Rpc.DownloadFile.Response](#anytype.Rpc.DownloadFile.Response)
 * [Rpc.DownloadFile.Response.Error](#anytype.Rpc.DownloadFile.Response.Error)
 * [Rpc.Export](#anytype.Rpc.Export)
 * [Rpc.Export.Request](#anytype.Rpc.Export.Request)
 * [Rpc.Export.Response](#anytype.Rpc.Export.Response)
 * [Rpc.Export.Response.Error](#anytype.Rpc.Export.Response.Error)
 * [Rpc.ExportLocalstore](#anytype.Rpc.ExportLocalstore)
 * [Rpc.ExportLocalstore.Request](#anytype.Rpc.ExportLocalstore.Request)
 * [Rpc.ExportLocalstore.Response](#anytype.Rpc.ExportLocalstore.Response)
 * [Rpc.ExportLocalstore.Response.Error](#anytype.Rpc.ExportLocalstore.Response.Error)
 * [Rpc.ExportTemplates](#anytype.Rpc.ExportTemplates)
 * [Rpc.ExportTemplates.Request](#anytype.Rpc.ExportTemplates.Request)
 * [Rpc.ExportTemplates.Response](#anytype.Rpc.ExportTemplates.Response)
 * [Rpc.ExportTemplates.Response.Error](#anytype.Rpc.ExportTemplates.Response.Error)
 * [Rpc.ExternalDrop](#anytype.Rpc.ExternalDrop)
 * [Rpc.ExternalDrop.Content](#anytype.Rpc.ExternalDrop.Content)
 * [Rpc.ExternalDrop.Content.Request](#anytype.Rpc.ExternalDrop.Content.Request)
 * [Rpc.ExternalDrop.Content.Response](#anytype.Rpc.ExternalDrop.Content.Response)
 * [Rpc.ExternalDrop.Content.Response.Error](#anytype.Rpc.ExternalDrop.Content.Response.Error)
 * [Rpc.ExternalDrop.Files](#anytype.Rpc.ExternalDrop.Files)
 * [Rpc.ExternalDrop.Files.Request](#anytype.Rpc.ExternalDrop.Files.Request)
 * [Rpc.ExternalDrop.Files.Response](#anytype.Rpc.ExternalDrop.Files.Response)
 * [Rpc.ExternalDrop.Files.Response.Error](#anytype.Rpc.ExternalDrop.Files.Response.Error)
 * [Rpc.File](#anytype.Rpc.File)
 * [Rpc.File.Offload](#anytype.Rpc.File.Offload)
 * [Rpc.File.Offload.Request](#anytype.Rpc.File.Offload.Request)
 * [Rpc.File.Offload.Response](#anytype.Rpc.File.Offload.Response)
 * [Rpc.File.Offload.Response.Error](#anytype.Rpc.File.Offload.Response.Error)
 * [Rpc.FileList](#anytype.Rpc.FileList)
 * [Rpc.FileList.Offload](#anytype.Rpc.FileList.Offload)
 * [Rpc.FileList.Offload.Request](#anytype.Rpc.FileList.Offload.Request)
 * [Rpc.FileList.Offload.Response](#anytype.Rpc.FileList.Offload.Response)
 * [Rpc.FileList.Offload.Response.Error](#anytype.Rpc.FileList.Offload.Response.Error)
 * [Rpc.GenericErrorResponse](#anytype.Rpc.GenericErrorResponse)
 * [Rpc.GenericErrorResponse.Error](#anytype.Rpc.GenericErrorResponse.Error)
 * [Rpc.History](#anytype.Rpc.History)
 * [Rpc.History.SetVersion](#anytype.Rpc.History.SetVersion)
 * [Rpc.History.SetVersion.Request](#anytype.Rpc.History.SetVersion.Request)
 * [Rpc.History.SetVersion.Response](#anytype.Rpc.History.SetVersion.Response)
 * [Rpc.History.SetVersion.Response.Error](#anytype.Rpc.History.SetVersion.Response.Error)
 * [Rpc.History.Show](#anytype.Rpc.History.Show)
 * [Rpc.History.Show.Request](#anytype.Rpc.History.Show.Request)
 * [Rpc.History.Show.Response](#anytype.Rpc.History.Show.Response)
 * [Rpc.History.Show.Response.Error](#anytype.Rpc.History.Show.Response.Error)
 * [Rpc.History.Versions](#anytype.Rpc.History.Versions)
 * [Rpc.History.Versions.Request](#anytype.Rpc.History.Versions.Request)
 * [Rpc.History.Versions.Response](#anytype.Rpc.History.Versions.Response)
 * [Rpc.History.Versions.Response.Error](#anytype.Rpc.History.Versions.Response.Error)
 * [Rpc.History.Versions.Version](#anytype.Rpc.History.Versions.Version)
 * [Rpc.LinkPreview](#anytype.Rpc.LinkPreview)
 * [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request)
 * [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response)
 * [Rpc.LinkPreview.Response.Error](#anytype.Rpc.LinkPreview.Response.Error)
 * [Rpc.Log](#anytype.Rpc.Log)
 * [Rpc.Log.Send](#anytype.Rpc.Log.Send)
 * [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request)
 * [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response)
 * [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error)
 * [Rpc.MakeTemplate](#anytype.Rpc.MakeTemplate)
 * [Rpc.MakeTemplate.Request](#anytype.Rpc.MakeTemplate.Request)
 * [Rpc.MakeTemplate.Response](#anytype.Rpc.MakeTemplate.Response)
 * [Rpc.MakeTemplate.Response.Error](#anytype.Rpc.MakeTemplate.Response.Error)
 * [Rpc.MakeTemplateByObjectType](#anytype.Rpc.MakeTemplateByObjectType)
 * [Rpc.MakeTemplateByObjectType.Request](#anytype.Rpc.MakeTemplateByObjectType.Request)
 * [Rpc.MakeTemplateByObjectType.Response](#anytype.Rpc.MakeTemplateByObjectType.Response)
 * [Rpc.MakeTemplateByObjectType.Response.Error](#anytype.Rpc.MakeTemplateByObjectType.Response.Error)
 * [Rpc.Navigation](#anytype.Rpc.Navigation)
 * [Rpc.Navigation.GetObjectInfoWithLinks](#anytype.Rpc.Navigation.GetObjectInfoWithLinks)
 * [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request)
 * [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response)
 * [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error)
 * [Rpc.Navigation.ListObjects](#anytype.Rpc.Navigation.ListObjects)
 * [Rpc.Navigation.ListObjects.Request](#anytype.Rpc.Navigation.ListObjects.Request)
 * [Rpc.Navigation.ListObjects.Response](#anytype.Rpc.Navigation.ListObjects.Response)
 * [Rpc.Navigation.ListObjects.Response.Error](#anytype.Rpc.Navigation.ListObjects.Response.Error)
 * [Rpc.Object](#anytype.Rpc.Object)
 * [Rpc.Object.AddWithObjectId](#anytype.Rpc.Object.AddWithObjectId)
 * [Rpc.Object.AddWithObjectId.Request](#anytype.Rpc.Object.AddWithObjectId.Request)
 * [Rpc.Object.AddWithObjectId.Response](#anytype.Rpc.Object.AddWithObjectId.Response)
 * [Rpc.Object.AddWithObjectId.Response.Error](#anytype.Rpc.Object.AddWithObjectId.Response.Error)
 * [Rpc.Object.FeaturedRelation](#anytype.Rpc.Object.FeaturedRelation)
 * [Rpc.Object.FeaturedRelation.Add](#anytype.Rpc.Object.FeaturedRelation.Add)
 * [Rpc.Object.FeaturedRelation.Add.Request](#anytype.Rpc.Object.FeaturedRelation.Add.Request)
 * [Rpc.Object.FeaturedRelation.Add.Response](#anytype.Rpc.Object.FeaturedRelation.Add.Response)
 * [Rpc.Object.FeaturedRelation.Add.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error)
 * [Rpc.Object.FeaturedRelation.Remove](#anytype.Rpc.Object.FeaturedRelation.Remove)
 * [Rpc.Object.FeaturedRelation.Remove.Request](#anytype.Rpc.Object.FeaturedRelation.Remove.Request)
 * [Rpc.Object.FeaturedRelation.Remove.Response](#anytype.Rpc.Object.FeaturedRelation.Remove.Response)
 * [Rpc.Object.FeaturedRelation.Remove.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error)
 * [Rpc.Object.Graph](#anytype.Rpc.Object.Graph)
 * [Rpc.Object.Graph.Edge](#anytype.Rpc.Object.Graph.Edge)
 * [Rpc.Object.Graph.Node](#anytype.Rpc.Object.Graph.Node)
 * [Rpc.Object.Graph.Request](#anytype.Rpc.Object.Graph.Request)
 * [Rpc.Object.Graph.Response](#anytype.Rpc.Object.Graph.Response)
 * [Rpc.Object.Graph.Response.Error](#anytype.Rpc.Object.Graph.Response.Error)
 * [Rpc.Object.RelationAdd](#anytype.Rpc.Object.RelationAdd)
 * [Rpc.Object.RelationAdd.Request](#anytype.Rpc.Object.RelationAdd.Request)
 * [Rpc.Object.RelationAdd.Response](#anytype.Rpc.Object.RelationAdd.Response)
 * [Rpc.Object.RelationAdd.Response.Error](#anytype.Rpc.Object.RelationAdd.Response.Error)
 * [Rpc.Object.RelationDelete](#anytype.Rpc.Object.RelationDelete)
 * [Rpc.Object.RelationDelete.Request](#anytype.Rpc.Object.RelationDelete.Request)
 * [Rpc.Object.RelationDelete.Response](#anytype.Rpc.Object.RelationDelete.Response)
 * [Rpc.Object.RelationDelete.Response.Error](#anytype.Rpc.Object.RelationDelete.Response.Error)
 * [Rpc.Object.RelationListAvailable](#anytype.Rpc.Object.RelationListAvailable)
 * [Rpc.Object.RelationListAvailable.Request](#anytype.Rpc.Object.RelationListAvailable.Request)
 * [Rpc.Object.RelationListAvailable.Response](#anytype.Rpc.Object.RelationListAvailable.Response)
 * [Rpc.Object.RelationListAvailable.Response.Error](#anytype.Rpc.Object.RelationListAvailable.Response.Error)
 * [Rpc.Object.RelationOptionAdd](#anytype.Rpc.Object.RelationOptionAdd)
 * [Rpc.Object.RelationOptionAdd.Request](#anytype.Rpc.Object.RelationOptionAdd.Request)
 * [Rpc.Object.RelationOptionAdd.Response](#anytype.Rpc.Object.RelationOptionAdd.Response)
 * [Rpc.Object.RelationOptionAdd.Response.Error](#anytype.Rpc.Object.RelationOptionAdd.Response.Error)
 * [Rpc.Object.RelationOptionDelete](#anytype.Rpc.Object.RelationOptionDelete)
 * [Rpc.Object.RelationOptionDelete.Request](#anytype.Rpc.Object.RelationOptionDelete.Request)
 * [Rpc.Object.RelationOptionDelete.Response](#anytype.Rpc.Object.RelationOptionDelete.Response)
 * [Rpc.Object.RelationOptionDelete.Response.Error](#anytype.Rpc.Object.RelationOptionDelete.Response.Error)
 * [Rpc.Object.RelationOptionUpdate](#anytype.Rpc.Object.RelationOptionUpdate)
 * [Rpc.Object.RelationOptionUpdate.Request](#anytype.Rpc.Object.RelationOptionUpdate.Request)
 * [Rpc.Object.RelationOptionUpdate.Response](#anytype.Rpc.Object.RelationOptionUpdate.Response)
 * [Rpc.Object.RelationOptionUpdate.Response.Error](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error)
 * [Rpc.Object.RelationUpdate](#anytype.Rpc.Object.RelationUpdate)
 * [Rpc.Object.RelationUpdate.Request](#anytype.Rpc.Object.RelationUpdate.Request)
 * [Rpc.Object.RelationUpdate.Response](#anytype.Rpc.Object.RelationUpdate.Response)
 * [Rpc.Object.RelationUpdate.Response.Error](#anytype.Rpc.Object.RelationUpdate.Response.Error)
 * [Rpc.Object.Search](#anytype.Rpc.Object.Search)
 * [Rpc.Object.Search.Request](#anytype.Rpc.Object.Search.Request)
 * [Rpc.Object.Search.Response](#anytype.Rpc.Object.Search.Response)
 * [Rpc.Object.Search.Response.Error](#anytype.Rpc.Object.Search.Response.Error)
 * [Rpc.Object.SetIsArchived](#anytype.Rpc.Object.SetIsArchived)
 * [Rpc.Object.SetIsArchived.Request](#anytype.Rpc.Object.SetIsArchived.Request)
 * [Rpc.Object.SetIsArchived.Response](#anytype.Rpc.Object.SetIsArchived.Response)
 * [Rpc.Object.SetIsArchived.Response.Error](#anytype.Rpc.Object.SetIsArchived.Response.Error)
 * [Rpc.Object.SetIsFavorite](#anytype.Rpc.Object.SetIsFavorite)
 * [Rpc.Object.SetIsFavorite.Request](#anytype.Rpc.Object.SetIsFavorite.Request)
 * [Rpc.Object.SetIsFavorite.Response](#anytype.Rpc.Object.SetIsFavorite.Response)
 * [Rpc.Object.SetIsFavorite.Response.Error](#anytype.Rpc.Object.SetIsFavorite.Response.Error)
 * [Rpc.Object.SetLayout](#anytype.Rpc.Object.SetLayout)
 * [Rpc.Object.SetLayout.Request](#anytype.Rpc.Object.SetLayout.Request)
 * [Rpc.Object.SetLayout.Response](#anytype.Rpc.Object.SetLayout.Response)
 * [Rpc.Object.SetLayout.Response.Error](#anytype.Rpc.Object.SetLayout.Response.Error)
 * [Rpc.Object.ShareByLink](#anytype.Rpc.Object.ShareByLink)
 * [Rpc.Object.ShareByLink.Request](#anytype.Rpc.Object.ShareByLink.Request)
 * [Rpc.Object.ShareByLink.Response](#anytype.Rpc.Object.ShareByLink.Response)
 * [Rpc.Object.ShareByLink.Response.Error](#anytype.Rpc.Object.ShareByLink.Response.Error)
 * [Rpc.Object.ToSet](#anytype.Rpc.Object.ToSet)
 * [Rpc.Object.ToSet.Request](#anytype.Rpc.Object.ToSet.Request)
 * [Rpc.Object.ToSet.Response](#anytype.Rpc.Object.ToSet.Response)
 * [Rpc.Object.ToSet.Response.Error](#anytype.Rpc.Object.ToSet.Response.Error)
 * [Rpc.ObjectList](#anytype.Rpc.ObjectList)
 * [Rpc.ObjectList.Delete](#anytype.Rpc.ObjectList.Delete)
 * [Rpc.ObjectList.Delete.Request](#anytype.Rpc.ObjectList.Delete.Request)
 * [Rpc.ObjectList.Delete.Response](#anytype.Rpc.ObjectList.Delete.Response)
 * [Rpc.ObjectList.Delete.Response.Error](#anytype.Rpc.ObjectList.Delete.Response.Error)
 * [Rpc.ObjectList.Set](#anytype.Rpc.ObjectList.Set)
 * [Rpc.ObjectList.Set.IsArchived](#anytype.Rpc.ObjectList.Set.IsArchived)
 * [Rpc.ObjectList.Set.IsArchived.Request](#anytype.Rpc.ObjectList.Set.IsArchived.Request)
 * [Rpc.ObjectList.Set.IsArchived.Response](#anytype.Rpc.ObjectList.Set.IsArchived.Response)
 * [Rpc.ObjectList.Set.IsArchived.Response.Error](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error)
 * [Rpc.ObjectList.Set.IsFavorite](#anytype.Rpc.ObjectList.Set.IsFavorite)
 * [Rpc.ObjectList.Set.IsFavorite.Request](#anytype.Rpc.ObjectList.Set.IsFavorite.Request)
 * [Rpc.ObjectList.Set.IsFavorite.Response](#anytype.Rpc.ObjectList.Set.IsFavorite.Response)
 * [Rpc.ObjectList.Set.IsFavorite.Response.Error](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error)
 * [Rpc.ObjectType](#anytype.Rpc.ObjectType)
 * [Rpc.ObjectType.Create](#anytype.Rpc.ObjectType.Create)
 * [Rpc.ObjectType.Create.Request](#anytype.Rpc.ObjectType.Create.Request)
 * [Rpc.ObjectType.Create.Response](#anytype.Rpc.ObjectType.Create.Response)
 * [Rpc.ObjectType.Create.Response.Error](#anytype.Rpc.ObjectType.Create.Response.Error)
 * [Rpc.ObjectType.List](#anytype.Rpc.ObjectType.List)
 * [Rpc.ObjectType.List.Request](#anytype.Rpc.ObjectType.List.Request)
 * [Rpc.ObjectType.List.Response](#anytype.Rpc.ObjectType.List.Response)
 * [Rpc.ObjectType.List.Response.Error](#anytype.Rpc.ObjectType.List.Response.Error)
 * [Rpc.ObjectType.Relation](#anytype.Rpc.ObjectType.Relation)
 * [Rpc.ObjectType.Relation.Add](#anytype.Rpc.ObjectType.Relation.Add)
 * [Rpc.ObjectType.Relation.Add.Request](#anytype.Rpc.ObjectType.Relation.Add.Request)
 * [Rpc.ObjectType.Relation.Add.Response](#anytype.Rpc.ObjectType.Relation.Add.Response)
 * [Rpc.ObjectType.Relation.Add.Response.Error](#anytype.Rpc.ObjectType.Relation.Add.Response.Error)
 * [Rpc.ObjectType.Relation.List](#anytype.Rpc.ObjectType.Relation.List)
 * [Rpc.ObjectType.Relation.List.Request](#anytype.Rpc.ObjectType.Relation.List.Request)
 * [Rpc.ObjectType.Relation.List.Response](#anytype.Rpc.ObjectType.Relation.List.Response)
 * [Rpc.ObjectType.Relation.List.Response.Error](#anytype.Rpc.ObjectType.Relation.List.Response.Error)
 * [Rpc.ObjectType.Relation.Remove](#anytype.Rpc.ObjectType.Relation.Remove)
 * [Rpc.ObjectType.Relation.Remove.Request](#anytype.Rpc.ObjectType.Relation.Remove.Request)
 * [Rpc.ObjectType.Relation.Remove.Response](#anytype.Rpc.ObjectType.Relation.Remove.Response)
 * [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error)
 * [Rpc.ObjectType.Relation.Update](#anytype.Rpc.ObjectType.Relation.Update)
 * [Rpc.ObjectType.Relation.Update.Request](#anytype.Rpc.ObjectType.Relation.Update.Request)
 * [Rpc.ObjectType.Relation.Update.Response](#anytype.Rpc.ObjectType.Relation.Update.Response)
 * [Rpc.ObjectType.Relation.Update.Response.Error](#anytype.Rpc.ObjectType.Relation.Update.Response.Error)
 * [Rpc.Page](#anytype.Rpc.Page)
 * [Rpc.Page.Create](#anytype.Rpc.Page.Create)
 * [Rpc.Page.Create.Request](#anytype.Rpc.Page.Create.Request)
 * [Rpc.Page.Create.Response](#anytype.Rpc.Page.Create.Response)
 * [Rpc.Page.Create.Response.Error](#anytype.Rpc.Page.Create.Response.Error)
 * [Rpc.Ping](#anytype.Rpc.Ping)
 * [Rpc.Ping.Request](#anytype.Rpc.Ping.Request)
 * [Rpc.Ping.Response](#anytype.Rpc.Ping.Response)
 * [Rpc.Ping.Response.Error](#anytype.Rpc.Ping.Response.Error)
 * [Rpc.Process](#anytype.Rpc.Process)
 * [Rpc.Process.Cancel](#anytype.Rpc.Process.Cancel)
 * [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request)
 * [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response)
 * [Rpc.Process.Cancel.Response.Error](#anytype.Rpc.Process.Cancel.Response.Error)
 * [Rpc.Set](#anytype.Rpc.Set)
 * [Rpc.Set.Create](#anytype.Rpc.Set.Create)
 * [Rpc.Set.Create.Request](#anytype.Rpc.Set.Create.Request)
 * [Rpc.Set.Create.Response](#anytype.Rpc.Set.Create.Response)
 * [Rpc.Set.Create.Response.Error](#anytype.Rpc.Set.Create.Response.Error)
 * [Rpc.Shutdown](#anytype.Rpc.Shutdown)
 * [Rpc.Shutdown.Request](#anytype.Rpc.Shutdown.Request)
 * [Rpc.Shutdown.Response](#anytype.Rpc.Shutdown.Response)
 * [Rpc.Shutdown.Response.Error](#anytype.Rpc.Shutdown.Response.Error)
 * [Rpc.UploadFile](#anytype.Rpc.UploadFile)
 * [Rpc.UploadFile.Request](#anytype.Rpc.UploadFile.Request)
 * [Rpc.UploadFile.Response](#anytype.Rpc.UploadFile.Response)
 * [Rpc.UploadFile.Response.Error](#anytype.Rpc.UploadFile.Response.Error)
 * [Rpc.Version](#anytype.Rpc.Version)
 * [Rpc.Version.Get](#anytype.Rpc.Version.Get)
 * [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request)
 * [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response)
 * [Rpc.Version.Get.Response.Error](#anytype.Rpc.Version.Get.Response.Error)
 * [Rpc.Wallet](#anytype.Rpc.Wallet)
 * [Rpc.Wallet.Convert](#anytype.Rpc.Wallet.Convert)
 * [Rpc.Wallet.Convert.Request](#anytype.Rpc.Wallet.Convert.Request)
 * [Rpc.Wallet.Convert.Response](#anytype.Rpc.Wallet.Convert.Response)
 * [Rpc.Wallet.Convert.Response.Error](#anytype.Rpc.Wallet.Convert.Response.Error)
 * [Rpc.Wallet.Create](#anytype.Rpc.Wallet.Create)
 * [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request)
 * [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response)
 * [Rpc.Wallet.Create.Response.Error](#anytype.Rpc.Wallet.Create.Response.Error)
 * [Rpc.Wallet.Recover](#anytype.Rpc.Wallet.Recover)
 * [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request)
 * [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response)
 * [Rpc.Wallet.Recover.Response.Error](#anytype.Rpc.Wallet.Recover.Response.Error)
 * [Rpc.Workspace](#anytype.Rpc.Workspace)
 * [Rpc.Workspace.Create](#anytype.Rpc.Workspace.Create)
 * [Rpc.Workspace.Create.Request](#anytype.Rpc.Workspace.Create.Request)
 * [Rpc.Workspace.Create.Response](#anytype.Rpc.Workspace.Create.Response)
 * [Rpc.Workspace.Create.Response.Error](#anytype.Rpc.Workspace.Create.Response.Error)
 * [Rpc.Workspace.GetAll](#anytype.Rpc.Workspace.GetAll)
 * [Rpc.Workspace.GetAll.Request](#anytype.Rpc.Workspace.GetAll.Request)
 * [Rpc.Workspace.GetAll.Response](#anytype.Rpc.Workspace.GetAll.Response)
 * [Rpc.Workspace.GetAll.Response.Error](#anytype.Rpc.Workspace.GetAll.Response.Error)
 * [Rpc.Workspace.GetCurrent](#anytype.Rpc.Workspace.GetCurrent)
 * [Rpc.Workspace.GetCurrent.Request](#anytype.Rpc.Workspace.GetCurrent.Request)
 * [Rpc.Workspace.GetCurrent.Response](#anytype.Rpc.Workspace.GetCurrent.Response)
 * [Rpc.Workspace.GetCurrent.Response.Error](#anytype.Rpc.Workspace.GetCurrent.Response.Error)
 * [Rpc.Workspace.Select](#anytype.Rpc.Workspace.Select)
 * [Rpc.Workspace.Select.Request](#anytype.Rpc.Workspace.Select.Request)
 * [Rpc.Workspace.Select.Response](#anytype.Rpc.Workspace.Select.Response)
 * [Rpc.Workspace.Select.Response.Error](#anytype.Rpc.Workspace.Select.Response.Error)
 * [Rpc.Workspace.SetIsHighlighted](#anytype.Rpc.Workspace.SetIsHighlighted)
 * [Rpc.Workspace.SetIsHighlighted.Request](#anytype.Rpc.Workspace.SetIsHighlighted.Request)
 * [Rpc.Workspace.SetIsHighlighted.Response](#anytype.Rpc.Workspace.SetIsHighlighted.Response)
 * [Rpc.Workspace.SetIsHighlighted.Response.Error](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error)
 * [Rpc.Account.Create.Response.Error.Code](#anytype.Rpc.Account.Create.Response.Error.Code)
 * [Rpc.Account.Recover.Response.Error.Code](#anytype.Rpc.Account.Recover.Response.Error.Code)
 * [Rpc.Account.Select.Response.Error.Code](#anytype.Rpc.Account.Select.Response.Error.Code)
 * [Rpc.Account.Stop.Response.Error.Code](#anytype.Rpc.Account.Stop.Response.Error.Code)
 * [Rpc.ApplyTemplate.Response.Error.Code](#anytype.Rpc.ApplyTemplate.Response.Error.Code)
 * [Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code)
 * [Rpc.Block.Bookmark.Fetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code)
 * [Rpc.Block.Close.Response.Error.Code](#anytype.Rpc.Block.Close.Response.Error.Code)
 * [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code)
 * [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code)
 * [Rpc.Block.CreatePage.Response.Error.Code](#anytype.Rpc.Block.CreatePage.Response.Error.Code)
 * [Rpc.Block.CreateSet.Response.Error.Code](#anytype.Rpc.Block.CreateSet.Response.Error.Code)
 * [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code)
 * [Rpc.Block.Dataview.RecordCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error.Code)
 * [Rpc.Block.Dataview.RecordDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error.Code)
 * [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code)
 * [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code)
 * [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code)
 * [Rpc.Block.Dataview.RecordUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error.Code)
 * [Rpc.Block.Dataview.RelationAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error.Code)
 * [Rpc.Block.Dataview.RelationDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error.Code)
 * [Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code)
 * [Rpc.Block.Dataview.RelationUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error.Code)
 * [Rpc.Block.Dataview.SetSource.Response.Error.Code](#anytype.Rpc.Block.Dataview.SetSource.Response.Error.Code)
 * [Rpc.Block.Dataview.ViewCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error.Code)
 * [Rpc.Block.Dataview.ViewDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error.Code)
 * [Rpc.Block.Dataview.ViewSetActive.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error.Code)
 * [Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code)
 * [Rpc.Block.Dataview.ViewUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error.Code)
 * [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code)
 * [Rpc.Block.Export.Response.Error.Code](#anytype.Rpc.Block.Export.Response.Error.Code)
 * [Rpc.Block.File.CreateAndUpload.Response.Error.Code](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code)
 * [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code)
 * [Rpc.Block.GetPublicWebURL.Response.Error.Code](#anytype.Rpc.Block.GetPublicWebURL.Response.Error.Code)
 * [Rpc.Block.ImportMarkdown.Response.Error.Code](#anytype.Rpc.Block.ImportMarkdown.Response.Error.Code)
 * [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code)
 * [Rpc.Block.ObjectType.Set.Response.Error.Code](#anytype.Rpc.Block.ObjectType.Set.Response.Error.Code)
 * [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code)
 * [Rpc.Block.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code)
 * [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code)
 * [Rpc.Block.Redo.Response.Error.Code](#anytype.Rpc.Block.Redo.Response.Error.Code)
 * [Rpc.Block.Relation.Add.Response.Error.Code](#anytype.Rpc.Block.Relation.Add.Response.Error.Code)
 * [Rpc.Block.Relation.SetKey.Response.Error.Code](#anytype.Rpc.Block.Relation.SetKey.Response.Error.Code)
 * [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code)
 * [Rpc.Block.Set.Details.Response.Error.Code](#anytype.Rpc.Block.Set.Details.Response.Error.Code)
 * [Rpc.Block.Set.Fields.Response.Error.Code](#anytype.Rpc.Block.Set.Fields.Response.Error.Code)
 * [Rpc.Block.Set.File.Name.Response.Error.Code](#anytype.Rpc.Block.Set.File.Name.Response.Error.Code)
 * [Rpc.Block.Set.Image.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Name.Response.Error.Code)
 * [Rpc.Block.Set.Image.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Width.Response.Error.Code)
 * [Rpc.Block.Set.Latex.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Latex.Text.Response.Error.Code)
 * [Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code)
 * [Rpc.Block.Set.Page.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code)
 * [Rpc.Block.Set.Restrictions.Response.Error.Code](#anytype.Rpc.Block.Set.Restrictions.Response.Error.Code)
 * [Rpc.Block.Set.Text.Checked.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code)
 * [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code)
 * [Rpc.Block.Set.Text.Style.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Style.Response.Error.Code)
 * [Rpc.Block.Set.Text.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Text.Response.Error.Code)
 * [Rpc.Block.Set.Video.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Name.Response.Error.Code)
 * [Rpc.Block.Set.Video.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Width.Response.Error.Code)
 * [Rpc.Block.SetBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error.Code)
 * [Rpc.Block.Show.Response.Error.Code](#anytype.Rpc.Block.Show.Response.Error.Code)
 * [Rpc.Block.Split.Request.Mode](#anytype.Rpc.Block.Split.Request.Mode)
 * [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code)
 * [Rpc.Block.Undo.Response.Error.Code](#anytype.Rpc.Block.Undo.Response.Error.Code)
 * [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code)
 * [Rpc.Block.UpdateContent.Response.Error.Code](#anytype.Rpc.Block.UpdateContent.Response.Error.Code)
 * [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code)
 * [Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code)
 * [Rpc.BlockList.Duplicate.Response.Error.Code](#anytype.Rpc.BlockList.Duplicate.Response.Error.Code)
 * [Rpc.BlockList.Move.Response.Error.Code](#anytype.Rpc.BlockList.Move.Response.Error.Code)
 * [Rpc.BlockList.MoveToNewPage.Response.Error.Code](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code)
 * [Rpc.BlockList.Set.Align.Response.Error.Code](#anytype.Rpc.BlockList.Set.Align.Response.Error.Code)
 * [Rpc.BlockList.Set.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code)
 * [Rpc.BlockList.Set.Div.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code)
 * [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code)
 * [Rpc.BlockList.Set.File.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.File.Style.Response.Error.Code)
 * [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code)
 * [Rpc.BlockList.Set.Text.Mark.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error.Code)
 * [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code)
 * [Rpc.BlockList.TurnInto.Response.Error.Code](#anytype.Rpc.BlockList.TurnInto.Response.Error.Code)
 * [Rpc.CloneTemplate.Response.Error.Code](#anytype.Rpc.CloneTemplate.Response.Error.Code)
 * [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code)
 * [Rpc.Debug.Sync.Response.Error.Code](#anytype.Rpc.Debug.Sync.Response.Error.Code)
 * [Rpc.Debug.Thread.Response.Error.Code](#anytype.Rpc.Debug.Thread.Response.Error.Code)
 * [Rpc.Debug.Tree.Response.Error.Code](#anytype.Rpc.Debug.Tree.Response.Error.Code)
 * [Rpc.DownloadFile.Response.Error.Code](#anytype.Rpc.DownloadFile.Response.Error.Code)
 * [Rpc.Export.Format](#anytype.Rpc.Export.Format)
 * [Rpc.Export.Response.Error.Code](#anytype.Rpc.Export.Response.Error.Code)
 * [Rpc.ExportLocalstore.Response.Error.Code](#anytype.Rpc.ExportLocalstore.Response.Error.Code)
 * [Rpc.ExportTemplates.Response.Error.Code](#anytype.Rpc.ExportTemplates.Response.Error.Code)
 * [Rpc.ExternalDrop.Content.Response.Error.Code](#anytype.Rpc.ExternalDrop.Content.Response.Error.Code)
 * [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code)
 * [Rpc.File.Offload.Response.Error.Code](#anytype.Rpc.File.Offload.Response.Error.Code)
 * [Rpc.FileList.Offload.Response.Error.Code](#anytype.Rpc.FileList.Offload.Response.Error.Code)
 * [Rpc.GenericErrorResponse.Error.Code](#anytype.Rpc.GenericErrorResponse.Error.Code)
 * [Rpc.History.SetVersion.Response.Error.Code](#anytype.Rpc.History.SetVersion.Response.Error.Code)
 * [Rpc.History.Show.Response.Error.Code](#anytype.Rpc.History.Show.Response.Error.Code)
 * [Rpc.History.Versions.Response.Error.Code](#anytype.Rpc.History.Versions.Response.Error.Code)
 * [Rpc.LinkPreview.Response.Error.Code](#anytype.Rpc.LinkPreview.Response.Error.Code)
 * [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level)
 * [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code)
 * [Rpc.MakeTemplate.Response.Error.Code](#anytype.Rpc.MakeTemplate.Response.Error.Code)
 * [Rpc.MakeTemplateByObjectType.Response.Error.Code](#anytype.Rpc.MakeTemplateByObjectType.Response.Error.Code)
 * [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context)
 * [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code)
 * [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype.Rpc.Navigation.ListObjects.Response.Error.Code)
 * [Rpc.Object.AddWithObjectId.Response.Error.Code](#anytype.Rpc.Object.AddWithObjectId.Response.Error.Code)
 * [Rpc.Object.FeaturedRelation.Add.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error.Code)
 * [Rpc.Object.FeaturedRelation.Remove.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error.Code)
 * [Rpc.Object.Graph.Edge.Type](#anytype.Rpc.Object.Graph.Edge.Type)
 * [Rpc.Object.Graph.Response.Error.Code](#anytype.Rpc.Object.Graph.Response.Error.Code)
 * [Rpc.Object.RelationAdd.Response.Error.Code](#anytype.Rpc.Object.RelationAdd.Response.Error.Code)
 * [Rpc.Object.RelationDelete.Response.Error.Code](#anytype.Rpc.Object.RelationDelete.Response.Error.Code)
 * [Rpc.Object.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Object.RelationListAvailable.Response.Error.Code)
 * [Rpc.Object.RelationOptionAdd.Response.Error.Code](#anytype.Rpc.Object.RelationOptionAdd.Response.Error.Code)
 * [Rpc.Object.RelationOptionDelete.Response.Error.Code](#anytype.Rpc.Object.RelationOptionDelete.Response.Error.Code)
 * [Rpc.Object.RelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error.Code)
 * [Rpc.Object.RelationUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationUpdate.Response.Error.Code)
 * [Rpc.Object.Search.Response.Error.Code](#anytype.Rpc.Object.Search.Response.Error.Code)
 * [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype.Rpc.Object.SetIsArchived.Response.Error.Code)
 * [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.SetIsFavorite.Response.Error.Code)
 * [Rpc.Object.SetLayout.Response.Error.Code](#anytype.Rpc.Object.SetLayout.Response.Error.Code)
 * [Rpc.Object.ShareByLink.Response.Error.Code](#anytype.Rpc.Object.ShareByLink.Response.Error.Code)
 * [Rpc.Object.ToSet.Response.Error.Code](#anytype.Rpc.Object.ToSet.Response.Error.Code)
 * [Rpc.ObjectList.Delete.Response.Error.Code](#anytype.Rpc.ObjectList.Delete.Response.Error.Code)
 * [Rpc.ObjectList.Set.IsArchived.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error.Code)
 * [Rpc.ObjectList.Set.IsFavorite.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error.Code)
 * [Rpc.ObjectType.Create.Response.Error.Code](#anytype.Rpc.ObjectType.Create.Response.Error.Code)
 * [Rpc.ObjectType.List.Response.Error.Code](#anytype.Rpc.ObjectType.List.Response.Error.Code)
 * [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code)
 * [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.List.Response.Error.Code)
 * [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code)
 * [Rpc.ObjectType.Relation.Update.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code)
 * [Rpc.Page.Create.Response.Error.Code](#anytype.Rpc.Page.Create.Response.Error.Code)
 * [Rpc.Ping.Response.Error.Code](#anytype.Rpc.Ping.Response.Error.Code)
 * [Rpc.Process.Cancel.Response.Error.Code](#anytype.Rpc.Process.Cancel.Response.Error.Code)
 * [Rpc.Set.Create.Response.Error.Code](#anytype.Rpc.Set.Create.Response.Error.Code)
 * [Rpc.Shutdown.Response.Error.Code](#anytype.Rpc.Shutdown.Response.Error.Code)
 * [Rpc.UploadFile.Response.Error.Code](#anytype.Rpc.UploadFile.Response.Error.Code)
 * [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code)
 * [Rpc.Wallet.Convert.Response.Error.Code](#anytype.Rpc.Wallet.Convert.Response.Error.Code)
 * [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code)
 * [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code)
 * [Rpc.Workspace.Create.Response.Error.Code](#anytype.Rpc.Workspace.Create.Response.Error.Code)
 * [Rpc.Workspace.GetAll.Response.Error.Code](#anytype.Rpc.Workspace.GetAll.Response.Error.Code)
 * [Rpc.Workspace.GetCurrent.Response.Error.Code](#anytype.Rpc.Workspace.GetCurrent.Response.Error.Code)
 * [Rpc.Workspace.Select.Response.Error.Code](#anytype.Rpc.Workspace.Select.Response.Error.Code)
 * [Rpc.Workspace.SetIsHighlighted.Response.Error.Code](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error.Code)
* [events.proto](#events.proto)
 * [Event](#anytype.Event)
 * [Event.Account](#anytype.Event.Account)
 * [Event.Account.Config](#anytype.Event.Account.Config)
 * [Event.Account.Config.Update](#anytype.Event.Account.Config.Update)
 * [Event.Account.Details](#anytype.Event.Account.Details)
 * [Event.Account.Show](#anytype.Event.Account.Show)
 * [Event.Block](#anytype.Event.Block)
 * [Event.Block.Add](#anytype.Event.Block.Add)
 * [Event.Block.Dataview](#anytype.Event.Block.Dataview)
 * [Event.Block.Dataview.RecordsDelete](#anytype.Event.Block.Dataview.RecordsDelete)
 * [Event.Block.Dataview.RecordsInsert](#anytype.Event.Block.Dataview.RecordsInsert)
 * [Event.Block.Dataview.RecordsSet](#anytype.Event.Block.Dataview.RecordsSet)
 * [Event.Block.Dataview.RecordsUpdate](#anytype.Event.Block.Dataview.RecordsUpdate)
 * [Event.Block.Dataview.RelationDelete](#anytype.Event.Block.Dataview.RelationDelete)
 * [Event.Block.Dataview.RelationSet](#anytype.Event.Block.Dataview.RelationSet)
 * [Event.Block.Dataview.SourceSet](#anytype.Event.Block.Dataview.SourceSet)
 * [Event.Block.Dataview.ViewDelete](#anytype.Event.Block.Dataview.ViewDelete)
 * [Event.Block.Dataview.ViewOrder](#anytype.Event.Block.Dataview.ViewOrder)
 * [Event.Block.Dataview.ViewSet](#anytype.Event.Block.Dataview.ViewSet)
 * [Event.Block.Delete](#anytype.Event.Block.Delete)
 * [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload)
 * [Event.Block.Fill](#anytype.Event.Block.Fill)
 * [Event.Block.Fill.Align](#anytype.Event.Block.Fill.Align)
 * [Event.Block.Fill.BackgroundColor](#anytype.Event.Block.Fill.BackgroundColor)
 * [Event.Block.Fill.Bookmark](#anytype.Event.Block.Fill.Bookmark)
 * [Event.Block.Fill.Bookmark.Description](#anytype.Event.Block.Fill.Bookmark.Description)
 * [Event.Block.Fill.Bookmark.FaviconHash](#anytype.Event.Block.Fill.Bookmark.FaviconHash)
 * [Event.Block.Fill.Bookmark.ImageHash](#anytype.Event.Block.Fill.Bookmark.ImageHash)
 * [Event.Block.Fill.Bookmark.Title](#anytype.Event.Block.Fill.Bookmark.Title)
 * [Event.Block.Fill.Bookmark.Type](#anytype.Event.Block.Fill.Bookmark.Type)
 * [Event.Block.Fill.Bookmark.Url](#anytype.Event.Block.Fill.Bookmark.Url)
 * [Event.Block.Fill.ChildrenIds](#anytype.Event.Block.Fill.ChildrenIds)
 * [Event.Block.Fill.DatabaseRecords](#anytype.Event.Block.Fill.DatabaseRecords)
 * [Event.Block.Fill.Details](#anytype.Event.Block.Fill.Details)
 * [Event.Block.Fill.Div](#anytype.Event.Block.Fill.Div)
 * [Event.Block.Fill.Div.Style](#anytype.Event.Block.Fill.Div.Style)
 * [Event.Block.Fill.Fields](#anytype.Event.Block.Fill.Fields)
 * [Event.Block.Fill.File](#anytype.Event.Block.Fill.File)
 * [Event.Block.Fill.File.Hash](#anytype.Event.Block.Fill.File.Hash)
 * [Event.Block.Fill.File.Mime](#anytype.Event.Block.Fill.File.Mime)
 * [Event.Block.Fill.File.Name](#anytype.Event.Block.Fill.File.Name)
 * [Event.Block.Fill.File.Size](#anytype.Event.Block.Fill.File.Size)
 * [Event.Block.Fill.File.State](#anytype.Event.Block.Fill.File.State)
 * [Event.Block.Fill.File.Style](#anytype.Event.Block.Fill.File.Style)
 * [Event.Block.Fill.File.Type](#anytype.Event.Block.Fill.File.Type)
 * [Event.Block.Fill.File.Width](#anytype.Event.Block.Fill.File.Width)
 * [Event.Block.Fill.Link](#anytype.Event.Block.Fill.Link)
 * [Event.Block.Fill.Link.Fields](#anytype.Event.Block.Fill.Link.Fields)
 * [Event.Block.Fill.Link.Style](#anytype.Event.Block.Fill.Link.Style)
 * [Event.Block.Fill.Link.TargetBlockId](#anytype.Event.Block.Fill.Link.TargetBlockId)
 * [Event.Block.Fill.Restrictions](#anytype.Event.Block.Fill.Restrictions)
 * [Event.Block.Fill.Text](#anytype.Event.Block.Fill.Text)
 * [Event.Block.Fill.Text.Checked](#anytype.Event.Block.Fill.Text.Checked)
 * [Event.Block.Fill.Text.Color](#anytype.Event.Block.Fill.Text.Color)
 * [Event.Block.Fill.Text.Marks](#anytype.Event.Block.Fill.Text.Marks)
 * [Event.Block.Fill.Text.Style](#anytype.Event.Block.Fill.Text.Style)
 * [Event.Block.Fill.Text.Text](#anytype.Event.Block.Fill.Text.Text)
 * [Event.Block.MarksInfo](#anytype.Event.Block.MarksInfo)
 * [Event.Block.Set](#anytype.Event.Block.Set)
 * [Event.Block.Set.Align](#anytype.Event.Block.Set.Align)
 * [Event.Block.Set.BackgroundColor](#anytype.Event.Block.Set.BackgroundColor)
 * [Event.Block.Set.Bookmark](#anytype.Event.Block.Set.Bookmark)
 * [Event.Block.Set.Bookmark.Description](#anytype.Event.Block.Set.Bookmark.Description)
 * [Event.Block.Set.Bookmark.FaviconHash](#anytype.Event.Block.Set.Bookmark.FaviconHash)
 * [Event.Block.Set.Bookmark.ImageHash](#anytype.Event.Block.Set.Bookmark.ImageHash)
 * [Event.Block.Set.Bookmark.Title](#anytype.Event.Block.Set.Bookmark.Title)
 * [Event.Block.Set.Bookmark.Type](#anytype.Event.Block.Set.Bookmark.Type)
 * [Event.Block.Set.Bookmark.Url](#anytype.Event.Block.Set.Bookmark.Url)
 * [Event.Block.Set.ChildrenIds](#anytype.Event.Block.Set.ChildrenIds)
 * [Event.Block.Set.Div](#anytype.Event.Block.Set.Div)
 * [Event.Block.Set.Div.Style](#anytype.Event.Block.Set.Div.Style)
 * [Event.Block.Set.Fields](#anytype.Event.Block.Set.Fields)
 * [Event.Block.Set.File](#anytype.Event.Block.Set.File)
 * [Event.Block.Set.File.Hash](#anytype.Event.Block.Set.File.Hash)
 * [Event.Block.Set.File.Mime](#anytype.Event.Block.Set.File.Mime)
 * [Event.Block.Set.File.Name](#anytype.Event.Block.Set.File.Name)
 * [Event.Block.Set.File.Size](#anytype.Event.Block.Set.File.Size)
 * [Event.Block.Set.File.State](#anytype.Event.Block.Set.File.State)
 * [Event.Block.Set.File.Style](#anytype.Event.Block.Set.File.Style)
 * [Event.Block.Set.File.Type](#anytype.Event.Block.Set.File.Type)
 * [Event.Block.Set.File.Width](#anytype.Event.Block.Set.File.Width)
 * [Event.Block.Set.Latex](#anytype.Event.Block.Set.Latex)
 * [Event.Block.Set.Latex.Text](#anytype.Event.Block.Set.Latex.Text)
 * [Event.Block.Set.Link](#anytype.Event.Block.Set.Link)
 * [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields)
 * [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style)
 * [Event.Block.Set.Link.TargetBlockId](#anytype.Event.Block.Set.Link.TargetBlockId)
 * [Event.Block.Set.Relation](#anytype.Event.Block.Set.Relation)
 * [Event.Block.Set.Relation.Key](#anytype.Event.Block.Set.Relation.Key)
 * [Event.Block.Set.Restrictions](#anytype.Event.Block.Set.Restrictions)
 * [Event.Block.Set.Text](#anytype.Event.Block.Set.Text)
 * [Event.Block.Set.Text.Checked](#anytype.Event.Block.Set.Text.Checked)
 * [Event.Block.Set.Text.Color](#anytype.Event.Block.Set.Text.Color)
 * [Event.Block.Set.Text.Marks](#anytype.Event.Block.Set.Text.Marks)
 * [Event.Block.Set.Text.Style](#anytype.Event.Block.Set.Text.Style)
 * [Event.Block.Set.Text.Text](#anytype.Event.Block.Set.Text.Text)
 * [Event.Message](#anytype.Event.Message)
 * [Event.Object](#anytype.Event.Object)
 * [Event.Object.Details](#anytype.Event.Object.Details)
 * [Event.Object.Details.Amend](#anytype.Event.Object.Details.Amend)
 * [Event.Object.Details.Amend.KeyValue](#anytype.Event.Object.Details.Amend.KeyValue)
 * [Event.Object.Details.Set](#anytype.Event.Object.Details.Set)
 * [Event.Object.Details.Unset](#anytype.Event.Object.Details.Unset)
 * [Event.Object.Relation](#anytype.Event.Object.Relation)
 * [Event.Object.Relation.Remove](#anytype.Event.Object.Relation.Remove)
 * [Event.Object.Relation.Set](#anytype.Event.Object.Relation.Set)
 * [Event.Object.Relations](#anytype.Event.Object.Relations)
 * [Event.Object.Relations.Amend](#anytype.Event.Object.Relations.Amend)
 * [Event.Object.Relations.Remove](#anytype.Event.Object.Relations.Remove)
 * [Event.Object.Relations.Set](#anytype.Event.Object.Relations.Set)
 * [Event.Object.Remove](#anytype.Event.Object.Remove)
 * [Event.Object.Show](#anytype.Event.Object.Show)
 * [Event.Object.Show.RelationWithValuePerObject](#anytype.Event.Object.Show.RelationWithValuePerObject)
 * [Event.Ping](#anytype.Event.Ping)
 * [Event.Process](#anytype.Event.Process)
 * [Event.Process.Done](#anytype.Event.Process.Done)
 * [Event.Process.New](#anytype.Event.Process.New)
 * [Event.Process.Update](#anytype.Event.Process.Update)
 * [Event.Status](#anytype.Event.Status)
 * [Event.Status.Thread](#anytype.Event.Status.Thread)
 * [Event.Status.Thread.Account](#anytype.Event.Status.Thread.Account)
 * [Event.Status.Thread.Cafe](#anytype.Event.Status.Thread.Cafe)
 * [Event.Status.Thread.Cafe.PinStatus](#anytype.Event.Status.Thread.Cafe.PinStatus)
 * [Event.Status.Thread.Device](#anytype.Event.Status.Thread.Device)
 * [Event.Status.Thread.Summary](#anytype.Event.Status.Thread.Summary)
 * [Event.User](#anytype.Event.User)
 * [Event.User.Block](#anytype.Event.User.Block)
 * [Event.User.Block.Join](#anytype.Event.User.Block.Join)
 * [Event.User.Block.Left](#anytype.Event.User.Block.Left)
 * [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange)
 * [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange)
 * [Model](#anytype.Model)
 * [Model.Process](#anytype.Model.Process)
 * [Model.Process.Progress](#anytype.Model.Process.Progress)
 * [ResponseEvent](#anytype.ResponseEvent)
 * [Event.Status.Thread.SyncStatus](#anytype.Event.Status.Thread.SyncStatus)
 * [Model.Process.State](#anytype.Model.Process.State)
 * [Model.Process.Type](#anytype.Model.Process.Type)
* [localstore.proto](#localstore.proto)
 * [ObjectDetails](#anytype.model.ObjectDetails)
 * [ObjectInfo](#anytype.model.ObjectInfo)
 * [ObjectInfoWithLinks](#anytype.model.ObjectInfoWithLinks)
 * [ObjectInfoWithOutboundLinks](#anytype.model.ObjectInfoWithOutboundLinks)
 * [ObjectInfoWithOutboundLinksIDs](#anytype.model.ObjectInfoWithOutboundLinksIDs)
 * [ObjectLinks](#anytype.model.ObjectLinks)
 * [ObjectLinksInfo](#anytype.model.ObjectLinksInfo)
 * [ObjectStoreChecksums](#anytype.model.ObjectStoreChecksums)
* [models.proto](#models.proto)
 * [Account](#anytype.model.Account)
 * [Account.Avatar](#anytype.model.Account.Avatar)
 * [Account.Config](#anytype.model.Account.Config)
 * [Block](#anytype.model.Block)
 * [Block.Content](#anytype.model.Block.Content)
 * [Block.Content.Bookmark](#anytype.model.Block.Content.Bookmark)
 * [Block.Content.Dataview](#anytype.model.Block.Content.Dataview)
 * [Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter)
 * [Block.Content.Dataview.Relation](#anytype.model.Block.Content.Dataview.Relation)
 * [Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort)
 * [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View)
 * [Block.Content.Div](#anytype.model.Block.Content.Div)
 * [Block.Content.FeaturedRelations](#anytype.model.Block.Content.FeaturedRelations)
 * [Block.Content.File](#anytype.model.Block.Content.File)
 * [Block.Content.Icon](#anytype.model.Block.Content.Icon)
 * [Block.Content.Latex](#anytype.model.Block.Content.Latex)
 * [Block.Content.Layout](#anytype.model.Block.Content.Layout)
 * [Block.Content.Link](#anytype.model.Block.Content.Link)
 * [Block.Content.Relation](#anytype.model.Block.Content.Relation)
 * [Block.Content.Smartblock](#anytype.model.Block.Content.Smartblock)
 * [Block.Content.Text](#anytype.model.Block.Content.Text)
 * [Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark)
 * [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks)
 * [Block.Restrictions](#anytype.model.Block.Restrictions)
 * [BlockMetaOnly](#anytype.model.BlockMetaOnly)
 * [Layout](#anytype.model.Layout)
 * [LinkPreview](#anytype.model.LinkPreview)
 * [ObjectType](#anytype.model.ObjectType)
 * [Range](#anytype.model.Range)
 * [Relation](#anytype.model.Relation)
 * [Relation.Option](#anytype.model.Relation.Option)
 * [RelationOptions](#anytype.model.RelationOptions)
 * [RelationWithValue](#anytype.model.RelationWithValue)
 * [Relations](#anytype.model.Relations)
 * [Restrictions](#anytype.model.Restrictions)
 * [Restrictions.DataviewRestrictions](#anytype.model.Restrictions.DataviewRestrictions)
 * [SmartBlockSnapshotBase](#anytype.model.SmartBlockSnapshotBase)
 * [ThreadCreateQueueEntry](#anytype.model.ThreadCreateQueueEntry)
 * [ThreadDeeplinkPayload](#anytype.model.ThreadDeeplinkPayload)
 * [Block.Align](#anytype.model.Block.Align)
 * [Block.Content.Dataview.Filter.Condition](#anytype.model.Block.Content.Dataview.Filter.Condition)
 * [Block.Content.Dataview.Filter.Operator](#anytype.model.Block.Content.Dataview.Filter.Operator)
 * [Block.Content.Dataview.Relation.DateFormat](#anytype.model.Block.Content.Dataview.Relation.DateFormat)
 * [Block.Content.Dataview.Relation.TimeFormat](#anytype.model.Block.Content.Dataview.Relation.TimeFormat)
 * [Block.Content.Dataview.Sort.Type](#anytype.model.Block.Content.Dataview.Sort.Type)
 * [Block.Content.Dataview.View.Size](#anytype.model.Block.Content.Dataview.View.Size)
 * [Block.Content.Dataview.View.Type](#anytype.model.Block.Content.Dataview.View.Type)
 * [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style)
 * [Block.Content.File.State](#anytype.model.Block.Content.File.State)
 * [Block.Content.File.Style](#anytype.model.Block.Content.File.Style)
 * [Block.Content.File.Type](#anytype.model.Block.Content.File.Type)
 * [Block.Content.Layout.Style](#anytype.model.Block.Content.Layout.Style)
 * [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style)
 * [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type)
 * [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style)
 * [Block.Position](#anytype.model.Block.Position)
 * [LinkPreview.Type](#anytype.model.LinkPreview.Type)
 * [ObjectType.Layout](#anytype.model.ObjectType.Layout)
 * [Relation.DataSource](#anytype.model.Relation.DataSource)
 * [Relation.Option.Scope](#anytype.model.Relation.Option.Scope)
 * [Relation.Scope](#anytype.model.Relation.Scope)
 * [RelationFormat](#anytype.model.RelationFormat)
 * [Restrictions.DataviewRestriction](#anytype.model.Restrictions.DataviewRestriction)
 * [Restrictions.ObjectRestriction](#anytype.model.Restrictions.ObjectRestriction)
 * [SmartBlockType](#anytype.model.SmartBlockType)
* [Scalar Value Types](#scalar-value-types)

<a name="service.proto"/>
<p align="right"><a href="#top">Top</a></p>

## service.proto






<a name="anytype.ClientCommands"/>
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
| BlockUpdateContent | [Rpc.Block.UpdateContent.Request](#anytype.Rpc.Block.UpdateContent.Request) | [Rpc.Block.UpdateContent.Response](#anytype.Rpc.Block.UpdateContent.Response) |  |
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
| BlockDataviewViewCreate | [Rpc.Block.Dataview.ViewCreate.Request](#anytype.Rpc.Block.Dataview.ViewCreate.Request) | [Rpc.Block.Dataview.ViewCreate.Response](#anytype.Rpc.Block.Dataview.ViewCreate.Response) |  |
| BlockDataviewViewDelete | [Rpc.Block.Dataview.ViewDelete.Request](#anytype.Rpc.Block.Dataview.ViewDelete.Request) | [Rpc.Block.Dataview.ViewDelete.Response](#anytype.Rpc.Block.Dataview.ViewDelete.Response) |  |
| BlockDataviewViewUpdate | [Rpc.Block.Dataview.ViewUpdate.Request](#anytype.Rpc.Block.Dataview.ViewUpdate.Request) | [Rpc.Block.Dataview.ViewUpdate.Response](#anytype.Rpc.Block.Dataview.ViewUpdate.Response) |  |
| BlockDataviewViewSetActive | [Rpc.Block.Dataview.ViewSetActive.Request](#anytype.Rpc.Block.Dataview.ViewSetActive.Request) | [Rpc.Block.Dataview.ViewSetActive.Response](#anytype.Rpc.Block.Dataview.ViewSetActive.Response) |  |
| BlockDataviewViewSetPosition | [Rpc.Block.Dataview.ViewSetPosition.Request](#anytype.Rpc.Block.Dataview.ViewSetPosition.Request) | [Rpc.Block.Dataview.ViewSetPosition.Response](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response) |  |
| BlockDataviewSetSource | [Rpc.Block.Dataview.SetSource.Request](#anytype.Rpc.Block.Dataview.SetSource.Request) | [Rpc.Block.Dataview.SetSource.Response](#anytype.Rpc.Block.Dataview.SetSource.Response) |  |
| BlockDataviewRelationAdd | [Rpc.Block.Dataview.RelationAdd.Request](#anytype.Rpc.Block.Dataview.RelationAdd.Request) | [Rpc.Block.Dataview.RelationAdd.Response](#anytype.Rpc.Block.Dataview.RelationAdd.Response) |  |
| BlockDataviewRelationUpdate | [Rpc.Block.Dataview.RelationUpdate.Request](#anytype.Rpc.Block.Dataview.RelationUpdate.Request) | [Rpc.Block.Dataview.RelationUpdate.Response](#anytype.Rpc.Block.Dataview.RelationUpdate.Response) |  |
| BlockDataviewRelationDelete | [Rpc.Block.Dataview.RelationDelete.Request](#anytype.Rpc.Block.Dataview.RelationDelete.Request) | [Rpc.Block.Dataview.RelationDelete.Response](#anytype.Rpc.Block.Dataview.RelationDelete.Response) |  |
| BlockDataviewRelationListAvailable | [Rpc.Block.Dataview.RelationListAvailable.Request](#anytype.Rpc.Block.Dataview.RelationListAvailable.Request) | [Rpc.Block.Dataview.RelationListAvailable.Response](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response) |  |
| BlockDataviewRecordCreate | [Rpc.Block.Dataview.RecordCreate.Request](#anytype.Rpc.Block.Dataview.RecordCreate.Request) | [Rpc.Block.Dataview.RecordCreate.Response](#anytype.Rpc.Block.Dataview.RecordCreate.Response) |  |
| BlockDataviewRecordUpdate | [Rpc.Block.Dataview.RecordUpdate.Request](#anytype.Rpc.Block.Dataview.RecordUpdate.Request) | [Rpc.Block.Dataview.RecordUpdate.Response](#anytype.Rpc.Block.Dataview.RecordUpdate.Response) |  |
| BlockDataviewRecordDelete | [Rpc.Block.Dataview.RecordDelete.Request](#anytype.Rpc.Block.Dataview.RecordDelete.Request) | [Rpc.Block.Dataview.RecordDelete.Response](#anytype.Rpc.Block.Dataview.RecordDelete.Response) |  |
| BlockDataviewRecordRelationOptionAdd | [Rpc.Block.Dataview.RecordRelationOptionAdd.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Request) | [Rpc.Block.Dataview.RecordRelationOptionAdd.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response) |  |
| BlockDataviewRecordRelationOptionUpdate | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Request) | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response) |  |
| BlockDataviewRecordRelationOptionDelete | [Rpc.Block.Dataview.RecordRelationOptionDelete.Request](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Request) | [Rpc.Block.Dataview.RecordRelationOptionDelete.Response](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response) |  |
| BlockObjectTypeSet | [Rpc.Block.ObjectType.Set.Request](#anytype.Rpc.Block.ObjectType.Set.Request) | [Rpc.Block.ObjectType.Set.Response](#anytype.Rpc.Block.ObjectType.Set.Response) |  |
| NavigationListObjects | [Rpc.Navigation.ListObjects.Request](#anytype.Rpc.Navigation.ListObjects.Request) | [Rpc.Navigation.ListObjects.Response](#anytype.Rpc.Navigation.ListObjects.Response) |  |
| NavigationGetObjectInfoWithLinks | [Rpc.Navigation.GetObjectInfoWithLinks.Request](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request) | [Rpc.Navigation.GetObjectInfoWithLinks.Response](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response) |  |
| ObjectGraph | [Rpc.Object.Graph.Request](#anytype.Rpc.Object.Graph.Request) | [Rpc.Object.Graph.Response](#anytype.Rpc.Object.Graph.Response) |  |
| ObjectSearch | [Rpc.Object.Search.Request](#anytype.Rpc.Object.Search.Request) | [Rpc.Object.Search.Response](#anytype.Rpc.Object.Search.Response) |  |
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
| ObjectToSet | [Rpc.Object.ToSet.Request](#anytype.Rpc.Object.ToSet.Request) | [Rpc.Object.ToSet.Response](#anytype.Rpc.Object.ToSet.Response) |  |
| ObjectListDelete | [Rpc.ObjectList.Delete.Request](#anytype.Rpc.ObjectList.Delete.Request) | [Rpc.ObjectList.Delete.Response](#anytype.Rpc.ObjectList.Delete.Response) |  |
| ObjectListSetIsArchived | [Rpc.ObjectList.Set.IsArchived.Request](#anytype.Rpc.ObjectList.Set.IsArchived.Request) | [Rpc.ObjectList.Set.IsArchived.Response](#anytype.Rpc.ObjectList.Set.IsArchived.Response) |  |
| ObjectListSetIsFavorite | [Rpc.ObjectList.Set.IsFavorite.Request](#anytype.Rpc.ObjectList.Set.IsFavorite.Request) | [Rpc.ObjectList.Set.IsFavorite.Response](#anytype.Rpc.ObjectList.Set.IsFavorite.Response) |  |
| BlockSetDetails | [Rpc.Block.Set.Details.Request](#anytype.Rpc.Block.Set.Details.Request) | [Rpc.Block.Set.Details.Response](#anytype.Rpc.Block.Set.Details.Response) |  |
| PageCreate | [Rpc.Page.Create.Request](#anytype.Rpc.Page.Create.Request) | [Rpc.Page.Create.Response](#anytype.Rpc.Page.Create.Response) |  |
| SetCreate | [Rpc.Set.Create.Request](#anytype.Rpc.Set.Create.Request) | [Rpc.Set.Create.Response](#anytype.Rpc.Set.Create.Response) |  |
| ObjectTypeCreate | [Rpc.ObjectType.Create.Request](#anytype.Rpc.ObjectType.Create.Request) | [Rpc.ObjectType.Create.Response](#anytype.Rpc.ObjectType.Create.Response) |  |
| ObjectTypeList | [Rpc.ObjectType.List.Request](#anytype.Rpc.ObjectType.List.Request) | [Rpc.ObjectType.List.Response](#anytype.Rpc.ObjectType.List.Response) |  |
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
| ExportTemplates | [Rpc.ExportTemplates.Request](#anytype.Rpc.ExportTemplates.Request) | [Rpc.ExportTemplates.Response](#anytype.Rpc.ExportTemplates.Response) |  |
| ExportLocalstore | [Rpc.ExportLocalstore.Request](#anytype.Rpc.ExportLocalstore.Request) | [Rpc.ExportLocalstore.Response](#anytype.Rpc.ExportLocalstore.Response) |  |
| MakeTemplate | [Rpc.MakeTemplate.Request](#anytype.Rpc.MakeTemplate.Request) | [Rpc.MakeTemplate.Response](#anytype.Rpc.MakeTemplate.Response) |  |
| MakeTemplateByObjectType | [Rpc.MakeTemplateByObjectType.Request](#anytype.Rpc.MakeTemplateByObjectType.Request) | [Rpc.MakeTemplateByObjectType.Response](#anytype.Rpc.MakeTemplateByObjectType.Response) |  |
| CloneTemplate | [Rpc.CloneTemplate.Request](#anytype.Rpc.CloneTemplate.Request) | [Rpc.CloneTemplate.Response](#anytype.Rpc.CloneTemplate.Response) |  |
| ApplyTemplate | [Rpc.ApplyTemplate.Request](#anytype.Rpc.ApplyTemplate.Request) | [Rpc.ApplyTemplate.Response](#anytype.Rpc.ApplyTemplate.Response) |  |
| DebugSync | [Rpc.Debug.Sync.Request](#anytype.Rpc.Debug.Sync.Request) | [Rpc.Debug.Sync.Response](#anytype.Rpc.Debug.Sync.Response) |  |
| DebugThread | [Rpc.Debug.Thread.Request](#anytype.Rpc.Debug.Thread.Request) | [Rpc.Debug.Thread.Response](#anytype.Rpc.Debug.Thread.Response) |  |
| DebugTree | [Rpc.Debug.Tree.Request](#anytype.Rpc.Debug.Tree.Request) | [Rpc.Debug.Tree.Response](#anytype.Rpc.Debug.Tree.Response) |  |
| ListenEvents | [Empty](#anytype.Empty) | [Event](#anytype.Event) |  |


<a name="changes.proto"/>
<p align="right"><a href="#top">Top</a></p>

## changes.proto



<a name="anytype.Change"/>
### Change


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| previous_ids | [string](#string) | repeated |  |
| last_snapshot_id | [string](#string) | optional |  |
| previous_meta_ids | [string](#string) | repeated |  |
| content | [Change.Content](#anytype.Change.Content) | repeated |  |
| snapshot | [Change.Snapshot](#anytype.Change.Snapshot) | optional |  |
| fileKeys | [Change.FileKeys](#anytype.Change.FileKeys) | repeated |  |
| timestamp | [int64](#int64) | optional |  |


<a name="anytype.Change.BlockCreate"/>
### Change.BlockCreate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| blocks | [Block](#anytype.model.Block) | repeated |  |


<a name="anytype.Change.BlockDuplicate"/>
### Change.BlockDuplicate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| ids | [string](#string) | repeated |  |


<a name="anytype.Change.BlockMove"/>
### Change.BlockMove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| ids | [string](#string) | repeated |  |


<a name="anytype.Change.BlockRemove"/>
### Change.BlockRemove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |


<a name="anytype.Change.BlockUpdate"/>
### Change.BlockUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| events | [Event.Message](#anytype.Event.Message) | repeated |  |


<a name="anytype.Change.Content"/>
### Change.Content


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockCreate | [Change.BlockCreate](#anytype.Change.BlockCreate) | optional |  |
| blockUpdate | [Change.BlockUpdate](#anytype.Change.BlockUpdate) | optional |  |
| blockRemove | [Change.BlockRemove](#anytype.Change.BlockRemove) | optional |  |
| blockMove | [Change.BlockMove](#anytype.Change.BlockMove) | optional |  |
| blockDuplicate | [Change.BlockDuplicate](#anytype.Change.BlockDuplicate) | optional |  |
| detailsSet | [Change.DetailsSet](#anytype.Change.DetailsSet) | optional |  |
| detailsUnset | [Change.DetailsUnset](#anytype.Change.DetailsUnset) | optional |  |
| relationAdd | [Change.RelationAdd](#anytype.Change.RelationAdd) | optional |  |
| relationRemove | [Change.RelationRemove](#anytype.Change.RelationRemove) | optional |  |
| relationUpdate | [Change.RelationUpdate](#anytype.Change.RelationUpdate) | optional |  |
| objectTypeAdd | [Change.ObjectTypeAdd](#anytype.Change.ObjectTypeAdd) | optional |  |
| objectTypeRemove | [Change.ObjectTypeRemove](#anytype.Change.ObjectTypeRemove) | optional |  |
| storeKeySet | [Change.StoreKeySet](#anytype.Change.StoreKeySet) | optional |  |
| storeKeyUnset | [Change.StoreKeyUnset](#anytype.Change.StoreKeyUnset) | optional |  |


<a name="anytype.Change.DetailsSet"/>
### Change.DetailsSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| value | [Value](#google.protobuf.Value) | optional |  |


<a name="anytype.Change.DetailsUnset"/>
### Change.DetailsUnset


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |


<a name="anytype.Change.FileKeys"/>
### Change.FileKeys


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) | optional |  |
| keys | [Change.FileKeys.KeysEntry](#anytype.Change.FileKeys.KeysEntry) | repeated |  |


<a name="anytype.Change.FileKeys.KeysEntry"/>
### Change.FileKeys.KeysEntry


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| value | [string](#string) | optional |  |


<a name="anytype.Change.ObjectTypeAdd"/>
### Change.ObjectTypeAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |


<a name="anytype.Change.ObjectTypeRemove"/>
### Change.ObjectTypeRemove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |


<a name="anytype.Change.RelationAdd"/>
### Change.RelationAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Change.RelationRemove"/>
### Change.RelationRemove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |


<a name="anytype.Change.RelationUpdate"/>
### Change.RelationUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| format | [RelationFormat](#anytype.model.RelationFormat) | optional |  |
| name | [string](#string) | optional |  |
| defaultValue | [Value](#google.protobuf.Value) | optional |  |
| objectTypes | [Change.RelationUpdate.ObjectTypes](#anytype.Change.RelationUpdate.ObjectTypes) | optional |  |
| multi | [bool](#bool) | optional |  |
| selectDict | [Change.RelationUpdate.Dict](#anytype.Change.RelationUpdate.Dict) | optional |  |


<a name="anytype.Change.RelationUpdate.Dict"/>
### Change.RelationUpdate.Dict


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dict | [Relation.Option](#anytype.model.Relation.Option) | repeated |  |


<a name="anytype.Change.RelationUpdate.ObjectTypes"/>
### Change.RelationUpdate.ObjectTypes


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypes | [string](#string) | repeated |  |


<a name="anytype.Change.Snapshot"/>
### Change.Snapshot


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| logHeads | [Change.Snapshot.LogHeadsEntry](#anytype.Change.Snapshot.LogHeadsEntry) | repeated |  |
| data | [SmartBlockSnapshotBase](#anytype.model.SmartBlockSnapshotBase) | optional |  |
| fileKeys | [Change.FileKeys](#anytype.Change.FileKeys) | repeated |  |


<a name="anytype.Change.Snapshot.LogHeadsEntry"/>
### Change.Snapshot.LogHeadsEntry


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| value | [string](#string) | optional |  |


<a name="anytype.Change.StoreKeySet"/>
### Change.StoreKeySet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |
| value | [Value](#google.protobuf.Value) | optional |  |


<a name="anytype.Change.StoreKeyUnset"/>
### Change.StoreKeyUnset


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | repeated |  |






<a name="commands.proto"/>
<p align="right"><a href="#top">Top</a></p>

## commands.proto



<a name="anytype.Empty"/>
### Empty


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc"/>
### Rpc


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account"/>
### Rpc.Account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account.Config"/>
### Rpc.Account.Config


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enableDataview | [bool](#bool) | optional |  |
| enableDebug | [bool](#bool) | optional |  |
| enableReleaseChannelSwitch | [bool](#bool) | optional |  |
| enableSpaces | [bool](#bool) | optional |  |
| extra | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.Account.Create"/>
### Rpc.Account.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account.Create.Request"/>
### Rpc.Account.Create.Request
Front end to middleware request-to-create-an account

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) | optional |  |
| avatarLocalPath | [string](#string) | optional |  |
| avatarColor | [string](#string) | optional |  |
| alphaInviteCode | [string](#string) | optional |  |


<a name="anytype.Rpc.Account.Create.Response"/>
### Rpc.Account.Create.Response
Middleware-to-front-end response for an account creation request, that can contain a NULL error and created account or a non-NULL error and an empty account

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error) | optional |  |
| account | [Account](#anytype.model.Account) | optional |  |
| config | [Rpc.Account.Config](#anytype.Rpc.Account.Config) | optional |  |


<a name="anytype.Rpc.Account.Create.Response.Error"/>
### Rpc.Account.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Create.Response.Error.Code](#anytype.Rpc.Account.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Account.Recover"/>
### Rpc.Account.Recover


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account.Recover.Request"/>
### Rpc.Account.Recover.Request
Front end to middleware request-to-start-search of an accounts for a recovered mnemonic.
Each of an account that would be found will come with an AccountAdd event

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account.Recover.Response"/>
### Rpc.Account.Recover.Response
Middleware-to-front-end response to an account recover request, that can contain a NULL error and created account or a non-NULL error and an empty account

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Recover.Response.Error](#anytype.Rpc.Account.Recover.Response.Error) | optional |  |


<a name="anytype.Rpc.Account.Recover.Response.Error"/>
### Rpc.Account.Recover.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Recover.Response.Error.Code](#anytype.Rpc.Account.Recover.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Account.Select"/>
### Rpc.Account.Select


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account.Select.Request"/>
### Rpc.Account.Select.Request
Front end to middleware request-to-launch-a specific account using account id and a root path
User can select an account from those, that came with an AccountAdd events

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| rootPath | [string](#string) | optional |  |


<a name="anytype.Rpc.Account.Select.Response"/>
### Rpc.Account.Select.Response
Middleware-to-front-end response for an account select request, that can contain a NULL error and selected account or a non-NULL error and an empty account

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Select.Response.Error](#anytype.Rpc.Account.Select.Response.Error) | optional |  |
| account | [Account](#anytype.model.Account) | optional |  |
| config | [Rpc.Account.Config](#anytype.Rpc.Account.Config) | optional |  |


<a name="anytype.Rpc.Account.Select.Response.Error"/>
### Rpc.Account.Select.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Select.Response.Error.Code](#anytype.Rpc.Account.Select.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Account.Stop"/>
### Rpc.Account.Stop


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Account.Stop.Request"/>
### Rpc.Account.Stop.Request
Front end to middleware request to stop currently running account node and optionally remove the locally stored data

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| removeData | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Account.Stop.Response"/>
### Rpc.Account.Stop.Response
Middleware-to-front-end response for an account stop request

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Stop.Response.Error](#anytype.Rpc.Account.Stop.Response.Error) | optional |  |


<a name="anytype.Rpc.Account.Stop.Response.Error"/>
### Rpc.Account.Stop.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Account.Stop.Response.Error.Code](#anytype.Rpc.Account.Stop.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ApplyTemplate"/>
### Rpc.ApplyTemplate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ApplyTemplate.Request"/>
### Rpc.ApplyTemplate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| templateId | [string](#string) | optional |  |


<a name="anytype.Rpc.ApplyTemplate.Response"/>
### Rpc.ApplyTemplate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ApplyTemplate.Response.Error](#anytype.Rpc.ApplyTemplate.Response.Error) | optional |  |


<a name="anytype.Rpc.ApplyTemplate.Response.Error"/>
### Rpc.ApplyTemplate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ApplyTemplate.Response.Error.Code](#anytype.Rpc.ApplyTemplate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block"/>
### Rpc.Block


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Bookmark"/>
### Rpc.Block.Bookmark


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch"/>
### Rpc.Block.Bookmark.CreateAndFetch


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Request"/>
### Rpc.Block.Bookmark.CreateAndFetch.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| url | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Response"/>
### Rpc.Block.Bookmark.CreateAndFetch.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Bookmark.CreateAndFetch.Response.Error](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error"/>
### Rpc.Block.Bookmark.CreateAndFetch.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Bookmark.Fetch"/>
### Rpc.Block.Bookmark.Fetch


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Bookmark.Fetch.Request"/>
### Rpc.Block.Bookmark.Fetch.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| url | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Bookmark.Fetch.Response"/>
### Rpc.Block.Bookmark.Fetch.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Bookmark.Fetch.Response.Error](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Bookmark.Fetch.Response.Error"/>
### Rpc.Block.Bookmark.Fetch.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Bookmark.Fetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Close"/>
### Rpc.Block.Close


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Close.Request"/>
### Rpc.Block.Close.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Close.Response"/>
### Rpc.Block.Close.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Close.Response.Error](#anytype.Rpc.Block.Close.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Close.Response.Error"/>
### Rpc.Block.Close.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Close.Response.Error.Code](#anytype.Rpc.Block.Close.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Copy"/>
### Rpc.Block.Copy


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Copy.Request"/>
### Rpc.Block.Copy.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blocks | [Block](#anytype.model.Block) | repeated |  |
| selectedTextRange | [Range](#anytype.model.Range) | optional |  |


<a name="anytype.Rpc.Block.Copy.Response"/>
### Rpc.Block.Copy.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error) | optional |  |
| textSlot | [string](#string) | optional |  |
| htmlSlot | [string](#string) | optional |  |
| anySlot | [Block](#anytype.model.Block) | repeated |  |


<a name="anytype.Rpc.Block.Copy.Response.Error"/>
### Rpc.Block.Copy.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Create"/>
### Rpc.Block.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Create.Request"/>
### Rpc.Block.Create.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| block | [Block](#anytype.model.Block) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |


<a name="anytype.Rpc.Block.Create.Response"/>
### Rpc.Block.Create.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Create.Response.Error](#anytype.Rpc.Block.Create.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Create.Response.Error"/>
### Rpc.Block.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.CreatePage"/>
### Rpc.Block.CreatePage


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.CreatePage.Request"/>
### Rpc.Block.CreatePage.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |
| templateId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.Block.CreatePage.Response"/>
### Rpc.Block.CreatePage.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CreatePage.Response.Error](#anytype.Rpc.Block.CreatePage.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.CreatePage.Response.Error"/>
### Rpc.Block.CreatePage.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CreatePage.Response.Error.Code](#anytype.Rpc.Block.CreatePage.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.CreateSet"/>
### Rpc.Block.CreateSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.CreateSet.Request"/>
### Rpc.Block.CreateSet.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| source | [string](#string) | repeated |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |


<a name="anytype.Rpc.Block.CreateSet.Response"/>
### Rpc.Block.CreateSet.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CreateSet.Response.Error](#anytype.Rpc.Block.CreateSet.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.CreateSet.Response.Error"/>
### Rpc.Block.CreateSet.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CreateSet.Response.Error.Code](#anytype.Rpc.Block.CreateSet.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Cut"/>
### Rpc.Block.Cut


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Cut.Request"/>
### Rpc.Block.Cut.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blocks | [Block](#anytype.model.Block) | repeated |  |
| selectedTextRange | [Range](#anytype.model.Range) | optional |  |


<a name="anytype.Rpc.Block.Cut.Response"/>
### Rpc.Block.Cut.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error) | optional |  |
| textSlot | [string](#string) | optional |  |
| htmlSlot | [string](#string) | optional |  |
| anySlot | [Block](#anytype.model.Block) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Cut.Response.Error"/>
### Rpc.Block.Cut.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview"/>
### Rpc.Block.Dataview


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordCreate"/>
### Rpc.Block.Dataview.RecordCreate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordCreate.Request"/>
### Rpc.Block.Dataview.RecordCreate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| record | [Struct](#google.protobuf.Struct) | optional |  |
| templateId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordCreate.Response"/>
### Rpc.Block.Dataview.RecordCreate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordCreate.Response.Error](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error) | optional |  |
| record | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordCreate.Response.Error"/>
### Rpc.Block.Dataview.RecordCreate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordCreate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordDelete"/>
### Rpc.Block.Dataview.RecordDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordDelete.Request"/>
### Rpc.Block.Dataview.RecordDelete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| recordId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordDelete.Response"/>
### Rpc.Block.Dataview.RecordDelete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordDelete.Response.Error"/>
### Rpc.Block.Dataview.RecordDelete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordDelete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd"/>
### Rpc.Block.Dataview.RecordRelationOptionAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Request"/>
### Rpc.Block.Dataview.RecordRelationOptionAdd.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| option | [Relation.Option](#anytype.model.Relation.Option) | optional |  |
| recordId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response"/>
### Rpc.Block.Dataview.RecordRelationOptionAdd.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| option | [Relation.Option](#anytype.model.Relation.Option) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error"/>
### Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete"/>
### Rpc.Block.Dataview.RecordRelationOptionDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Request"/>
### Rpc.Block.Dataview.RecordRelationOptionDelete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| optionId | [string](#string) | optional |  |
| recordId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response"/>
### Rpc.Block.Dataview.RecordRelationOptionDelete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error"/>
### Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate"/>
### Rpc.Block.Dataview.RecordRelationOptionUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Request"/>
### Rpc.Block.Dataview.RecordRelationOptionUpdate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| option | [Relation.Option](#anytype.model.Relation.Option) | optional |  |
| recordId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response"/>
### Rpc.Block.Dataview.RecordRelationOptionUpdate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error"/>
### Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordUpdate"/>
### Rpc.Block.Dataview.RecordUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Request"/>
### Rpc.Block.Dataview.RecordUpdate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| recordId | [string](#string) | optional |  |
| record | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Response"/>
### Rpc.Block.Dataview.RecordUpdate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RecordUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error"/>
### Rpc.Block.Dataview.RecordUpdate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RecordUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationAdd"/>
### Rpc.Block.Dataview.RelationAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RelationAdd.Request"/>
### Rpc.Block.Dataview.RelationAdd.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationAdd.Response"/>
### Rpc.Block.Dataview.RelationAdd.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationAdd.Response.Error](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| relationKey | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationAdd.Response.Error"/>
### Rpc.Block.Dataview.RelationAdd.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationAdd.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationAdd.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationDelete"/>
### Rpc.Block.Dataview.RelationDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RelationDelete.Request"/>
### Rpc.Block.Dataview.RelationDelete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationDelete.Response"/>
### Rpc.Block.Dataview.RelationDelete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationDelete.Response.Error](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationDelete.Response.Error"/>
### Rpc.Block.Dataview.RelationDelete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationDelete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationListAvailable"/>
### Rpc.Block.Dataview.RelationListAvailable


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Request"/>
### Rpc.Block.Dataview.RelationListAvailable.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Response"/>
### Rpc.Block.Dataview.RelationListAvailable.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationListAvailable.Response.Error](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error"/>
### Rpc.Block.Dataview.RelationListAvailable.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationUpdate"/>
### Rpc.Block.Dataview.RelationUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Request"/>
### Rpc.Block.Dataview.RelationUpdate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Response"/>
### Rpc.Block.Dataview.RelationUpdate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.RelationUpdate.Response.Error](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error"/>
### Rpc.Block.Dataview.RelationUpdate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.RelationUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.SetSource"/>
### Rpc.Block.Dataview.SetSource


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.SetSource.Request"/>
### Rpc.Block.Dataview.SetSource.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| source | [string](#string) | repeated |  |


<a name="anytype.Rpc.Block.Dataview.SetSource.Response"/>
### Rpc.Block.Dataview.SetSource.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.SetSource.Response.Error](#anytype.Rpc.Block.Dataview.SetSource.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.SetSource.Response.Error"/>
### Rpc.Block.Dataview.SetSource.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.SetSource.Response.Error.Code](#anytype.Rpc.Block.Dataview.SetSource.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewCreate"/>
### Rpc.Block.Dataview.ViewCreate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.ViewCreate.Request"/>
### Rpc.Block.Dataview.ViewCreate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| view | [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewCreate.Response"/>
### Rpc.Block.Dataview.ViewCreate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewCreate.Response.Error](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| viewId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewCreate.Response.Error"/>
### Rpc.Block.Dataview.ViewCreate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewCreate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewCreate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewDelete"/>
### Rpc.Block.Dataview.ViewDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.ViewDelete.Request"/>
### Rpc.Block.Dataview.ViewDelete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewDelete.Response"/>
### Rpc.Block.Dataview.ViewDelete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewDelete.Response.Error](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewDelete.Response.Error"/>
### Rpc.Block.Dataview.ViewDelete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewDelete.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewDelete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewSetActive"/>
### Rpc.Block.Dataview.ViewSetActive


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Request"/>
### Rpc.Block.Dataview.ViewSetActive.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| offset | [uint32](#uint32) | optional |  |
| limit | [uint32](#uint32) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Response"/>
### Rpc.Block.Dataview.ViewSetActive.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewSetActive.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error"/>
### Rpc.Block.Dataview.ViewSetActive.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewSetActive.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewSetPosition"/>
### Rpc.Block.Dataview.ViewSetPosition


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Request"/>
### Rpc.Block.Dataview.ViewSetPosition.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| position | [uint32](#uint32) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Response"/>
### Rpc.Block.Dataview.ViewSetPosition.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewSetPosition.Response.Error](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error"/>
### Rpc.Block.Dataview.ViewSetPosition.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewUpdate"/>
### Rpc.Block.Dataview.ViewUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Request"/>
### Rpc.Block.Dataview.ViewUpdate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| view | [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Response"/>
### Rpc.Block.Dataview.ViewUpdate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Dataview.ViewUpdate.Response.Error](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error"/>
### Rpc.Block.Dataview.ViewUpdate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Dataview.ViewUpdate.Response.Error.Code](#anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Download"/>
### Rpc.Block.Download


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Download.Request"/>
### Rpc.Block.Download.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Download.Response"/>
### Rpc.Block.Download.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Download.Response.Error](#anytype.Rpc.Block.Download.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Download.Response.Error"/>
### Rpc.Block.Download.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Export"/>
### Rpc.Block.Export


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Export.Request"/>
### Rpc.Block.Export.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blocks | [Block](#anytype.model.Block) | repeated |  |


<a name="anytype.Rpc.Block.Export.Response"/>
### Rpc.Block.Export.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Export.Response.Error](#anytype.Rpc.Block.Export.Response.Error) | optional |  |
| path | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Export.Response.Error"/>
### Rpc.Block.Export.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Export.Response.Error.Code](#anytype.Rpc.Block.Export.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.File"/>
### Rpc.Block.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.File.CreateAndUpload"/>
### Rpc.Block.File.CreateAndUpload


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.File.CreateAndUpload.Request"/>
### Rpc.Block.File.CreateAndUpload.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| url | [string](#string) | optional |  |
| localPath | [string](#string) | optional |  |
| fileType | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) | optional |  |


<a name="anytype.Rpc.Block.File.CreateAndUpload.Response"/>
### Rpc.Block.File.CreateAndUpload.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.File.CreateAndUpload.Response.Error](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.File.CreateAndUpload.Response.Error"/>
### Rpc.Block.File.CreateAndUpload.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.File.CreateAndUpload.Response.Error.Code](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Get"/>
### Rpc.Block.Get


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Get.Marks"/>
### Rpc.Block.Get.Marks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Get.Marks.Request"/>
### Rpc.Block.Get.Marks.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| range | [Range](#anytype.model.Range) | optional |  |


<a name="anytype.Rpc.Block.Get.Marks.Response"/>
### Rpc.Block.Get.Marks.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Get.Marks.Response.Error](#anytype.Rpc.Block.Get.Marks.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Get.Marks.Response.Error"/>
### Rpc.Block.Get.Marks.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.GetPublicWebURL"/>
### Rpc.Block.GetPublicWebURL


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.GetPublicWebURL.Request"/>
### Rpc.Block.GetPublicWebURL.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.GetPublicWebURL.Response"/>
### Rpc.Block.GetPublicWebURL.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.GetPublicWebURL.Response.Error](#anytype.Rpc.Block.GetPublicWebURL.Response.Error) | optional |  |
| url | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.GetPublicWebURL.Response.Error"/>
### Rpc.Block.GetPublicWebURL.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.GetPublicWebURL.Response.Error.Code](#anytype.Rpc.Block.GetPublicWebURL.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.ImportMarkdown"/>
### Rpc.Block.ImportMarkdown


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.ImportMarkdown.Request"/>
### Rpc.Block.ImportMarkdown.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| importPath | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.ImportMarkdown.Response"/>
### Rpc.Block.ImportMarkdown.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ImportMarkdown.Response.Error](#anytype.Rpc.Block.ImportMarkdown.Response.Error) | optional |  |
| rootLinkIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.ImportMarkdown.Response.Error"/>
### Rpc.Block.ImportMarkdown.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ImportMarkdown.Response.Error.Code](#anytype.Rpc.Block.ImportMarkdown.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Merge"/>
### Rpc.Block.Merge


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Merge.Request"/>
### Rpc.Block.Merge.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| firstBlockId | [string](#string) | optional |  |
| secondBlockId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Merge.Response"/>
### Rpc.Block.Merge.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Merge.Response.Error"/>
### Rpc.Block.Merge.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.ObjectType"/>
### Rpc.Block.ObjectType


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.ObjectType.Set"/>
### Rpc.Block.ObjectType.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.ObjectType.Set.Request"/>
### Rpc.Block.ObjectType.Set.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| objectTypeUrl | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.ObjectType.Set.Response"/>
### Rpc.Block.ObjectType.Set.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.ObjectType.Set.Response.Error](#anytype.Rpc.Block.ObjectType.Set.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.ObjectType.Set.Response.Error"/>
### Rpc.Block.ObjectType.Set.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.ObjectType.Set.Response.Error.Code](#anytype.Rpc.Block.ObjectType.Set.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Open"/>
### Rpc.Block.Open


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Open.Request"/>
### Rpc.Block.Open.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| traceId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Open.Response"/>
### Rpc.Block.Open.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Open.Response.Error"/>
### Rpc.Block.Open.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.OpenBreadcrumbs"/>
### Rpc.Block.OpenBreadcrumbs


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.OpenBreadcrumbs.Request"/>
### Rpc.Block.OpenBreadcrumbs.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| traceId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.OpenBreadcrumbs.Response"/>
### Rpc.Block.OpenBreadcrumbs.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.OpenBreadcrumbs.Response.Error](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.OpenBreadcrumbs.Response.Error"/>
### Rpc.Block.OpenBreadcrumbs.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Paste"/>
### Rpc.Block.Paste


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Paste.Request"/>
### Rpc.Block.Paste.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| focusedBlockId | [string](#string) | optional |  |
| selectedTextRange | [Range](#anytype.model.Range) | optional |  |
| selectedBlockIds | [string](#string) | repeated |  |
| isPartOfBlock | [bool](#bool) | optional |  |
| textSlot | [string](#string) | optional |  |
| htmlSlot | [string](#string) | optional |  |
| anySlot | [Block](#anytype.model.Block) | repeated |  |
| fileSlot | [Rpc.Block.Paste.Request.File](#anytype.Rpc.Block.Paste.Request.File) | repeated |  |


<a name="anytype.Rpc.Block.Paste.Request.File"/>
### Rpc.Block.Paste.Request.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) | optional |  |
| data | [bytes](#bytes) | optional |  |
| localPath | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Paste.Response"/>
### Rpc.Block.Paste.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error) | optional |  |
| blockIds | [string](#string) | repeated |  |
| caretPosition | [int32](#int32) | optional |  |
| isSameBlockCaret | [bool](#bool) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Paste.Response.Error"/>
### Rpc.Block.Paste.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Redo"/>
### Rpc.Block.Redo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Redo.Request"/>
### Rpc.Block.Redo.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Redo.Response"/>
### Rpc.Block.Redo.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Redo.Response.Error](#anytype.Rpc.Block.Redo.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| counters | [Rpc.Block.UndoRedoCounter](#anytype.Rpc.Block.UndoRedoCounter) | optional |  |


<a name="anytype.Rpc.Block.Redo.Response.Error"/>
### Rpc.Block.Redo.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Redo.Response.Error.Code](#anytype.Rpc.Block.Redo.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Relation"/>
### Rpc.Block.Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Relation.Add"/>
### Rpc.Block.Relation.Add


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Relation.Add.Request"/>
### Rpc.Block.Relation.Add.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Block.Relation.Add.Response"/>
### Rpc.Block.Relation.Add.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Relation.Add.Response.Error](#anytype.Rpc.Block.Relation.Add.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Relation.Add.Response.Error"/>
### Rpc.Block.Relation.Add.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Relation.Add.Response.Error.Code](#anytype.Rpc.Block.Relation.Add.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Relation.SetKey"/>
### Rpc.Block.Relation.SetKey


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Relation.SetKey.Request"/>
### Rpc.Block.Relation.SetKey.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| key | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Relation.SetKey.Response"/>
### Rpc.Block.Relation.SetKey.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Relation.SetKey.Response.Error](#anytype.Rpc.Block.Relation.SetKey.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Relation.SetKey.Response.Error"/>
### Rpc.Block.Relation.SetKey.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Relation.SetKey.Response.Error.Code](#anytype.Rpc.Block.Relation.SetKey.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Replace"/>
### Rpc.Block.Replace


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Replace.Request"/>
### Rpc.Block.Replace.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| block | [Block](#anytype.model.Block) | optional |  |


<a name="anytype.Rpc.Block.Replace.Response"/>
### Rpc.Block.Replace.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Replace.Response.Error](#anytype.Rpc.Block.Replace.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Replace.Response.Error"/>
### Rpc.Block.Replace.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set"/>
### Rpc.Block.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Details"/>
### Rpc.Block.Set.Details


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Details.Detail"/>
### Rpc.Block.Set.Details.Detail


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| value | [Value](#google.protobuf.Value) | optional |  |


<a name="anytype.Rpc.Block.Set.Details.Request"/>
### Rpc.Block.Set.Details.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| details | [Rpc.Block.Set.Details.Detail](#anytype.Rpc.Block.Set.Details.Detail) | repeated |  |


<a name="anytype.Rpc.Block.Set.Details.Response"/>
### Rpc.Block.Set.Details.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Details.Response.Error](#anytype.Rpc.Block.Set.Details.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Details.Response.Error"/>
### Rpc.Block.Set.Details.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Details.Response.Error.Code](#anytype.Rpc.Block.Set.Details.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Fields"/>
### Rpc.Block.Set.Fields


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Fields.Request"/>
### Rpc.Block.Set.Fields.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.Block.Set.Fields.Response"/>
### Rpc.Block.Set.Fields.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Fields.Response.Error](#anytype.Rpc.Block.Set.Fields.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Fields.Response.Error"/>
### Rpc.Block.Set.Fields.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Fields.Response.Error.Code](#anytype.Rpc.Block.Set.Fields.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.File"/>
### Rpc.Block.Set.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.File.Name"/>
### Rpc.Block.Set.File.Name


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.File.Name.Request"/>
### Rpc.Block.Set.File.Name.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| name | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.File.Name.Response"/>
### Rpc.Block.Set.File.Name.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.File.Name.Response.Error](#anytype.Rpc.Block.Set.File.Name.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.File.Name.Response.Error"/>
### Rpc.Block.Set.File.Name.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.File.Name.Response.Error.Code](#anytype.Rpc.Block.Set.File.Name.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Image"/>
### Rpc.Block.Set.Image


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Image.Name"/>
### Rpc.Block.Set.Image.Name


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Image.Name.Request"/>
### Rpc.Block.Set.Image.Name.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| name | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Image.Name.Response"/>
### Rpc.Block.Set.Image.Name.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Image.Name.Response.Error](#anytype.Rpc.Block.Set.Image.Name.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Set.Image.Name.Response.Error"/>
### Rpc.Block.Set.Image.Name.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Image.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Name.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Image.Width"/>
### Rpc.Block.Set.Image.Width


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Image.Width.Request"/>
### Rpc.Block.Set.Image.Width.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| width | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Block.Set.Image.Width.Response"/>
### Rpc.Block.Set.Image.Width.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Image.Width.Response.Error](#anytype.Rpc.Block.Set.Image.Width.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Set.Image.Width.Response.Error"/>
### Rpc.Block.Set.Image.Width.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Image.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Width.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Latex"/>
### Rpc.Block.Set.Latex


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Latex.Text"/>
### Rpc.Block.Set.Latex.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Latex.Text.Request"/>
### Rpc.Block.Set.Latex.Text.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| text | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Latex.Text.Response"/>
### Rpc.Block.Set.Latex.Text.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Latex.Text.Response.Error](#anytype.Rpc.Block.Set.Latex.Text.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Latex.Text.Response.Error"/>
### Rpc.Block.Set.Latex.Text.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Latex.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Latex.Text.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Link"/>
### Rpc.Block.Set.Link


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Link.TargetBlockId"/>
### Rpc.Block.Set.Link.TargetBlockId


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Request"/>
### Rpc.Block.Set.Link.TargetBlockId.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| targetBlockId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Response"/>
### Rpc.Block.Set.Link.TargetBlockId.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Link.TargetBlockId.Response.Error](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error"/>
### Rpc.Block.Set.Link.TargetBlockId.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Page"/>
### Rpc.Block.Set.Page


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Page.IsArchived"/>
### Rpc.Block.Set.Page.IsArchived


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Page.IsArchived.Request"/>
### Rpc.Block.Set.Page.IsArchived.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| isArchived | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Block.Set.Page.IsArchived.Response"/>
### Rpc.Block.Set.Page.IsArchived.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Page.IsArchived.Response.Error](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Set.Page.IsArchived.Response.Error"/>
### Rpc.Block.Set.Page.IsArchived.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Page.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Restrictions"/>
### Rpc.Block.Set.Restrictions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Restrictions.Request"/>
### Rpc.Block.Set.Restrictions.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| restrictions | [Block.Restrictions](#anytype.model.Block.Restrictions) | optional |  |


<a name="anytype.Rpc.Block.Set.Restrictions.Response"/>
### Rpc.Block.Set.Restrictions.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Restrictions.Response.Error](#anytype.Rpc.Block.Set.Restrictions.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Restrictions.Response.Error"/>
### Rpc.Block.Set.Restrictions.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Restrictions.Response.Error.Code](#anytype.Rpc.Block.Set.Restrictions.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Text"/>
### Rpc.Block.Set.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Text.Checked"/>
### Rpc.Block.Set.Text.Checked


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Text.Checked.Request"/>
### Rpc.Block.Set.Text.Checked.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| checked | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Checked.Response"/>
### Rpc.Block.Set.Text.Checked.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Checked.Response.Error](#anytype.Rpc.Block.Set.Text.Checked.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Checked.Response.Error"/>
### Rpc.Block.Set.Text.Checked.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Checked.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Color"/>
### Rpc.Block.Set.Text.Color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Text.Color.Request"/>
### Rpc.Block.Set.Text.Color.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| color | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Color.Response"/>
### Rpc.Block.Set.Text.Color.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Color.Response.Error](#anytype.Rpc.Block.Set.Text.Color.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Color.Response.Error"/>
### Rpc.Block.Set.Text.Color.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Style"/>
### Rpc.Block.Set.Text.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Text.Style.Request"/>
### Rpc.Block.Set.Text.Style.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Style.Response"/>
### Rpc.Block.Set.Text.Style.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Style.Response.Error](#anytype.Rpc.Block.Set.Text.Style.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Style.Response.Error"/>
### Rpc.Block.Set.Text.Style.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Style.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Style.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Text"/>
### Rpc.Block.Set.Text.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Text.Text.Request"/>
### Rpc.Block.Set.Text.Text.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| text | [string](#string) | optional |  |
| marks | [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Text.Response"/>
### Rpc.Block.Set.Text.Text.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.Text.Response.Error](#anytype.Rpc.Block.Set.Text.Text.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Set.Text.Text.Response.Error"/>
### Rpc.Block.Set.Text.Text.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Text.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Video"/>
### Rpc.Block.Set.Video


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Video.Name"/>
### Rpc.Block.Set.Video.Name


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Video.Name.Request"/>
### Rpc.Block.Set.Video.Name.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| name | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Video.Name.Response"/>
### Rpc.Block.Set.Video.Name.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Video.Name.Response.Error](#anytype.Rpc.Block.Set.Video.Name.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Set.Video.Name.Response.Error"/>
### Rpc.Block.Set.Video.Name.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Video.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Name.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Set.Video.Width"/>
### Rpc.Block.Set.Video.Width


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Set.Video.Width.Request"/>
### Rpc.Block.Set.Video.Width.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| width | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Block.Set.Video.Width.Response"/>
### Rpc.Block.Set.Video.Width.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Video.Width.Response.Error](#anytype.Rpc.Block.Set.Video.Width.Response.Error) | optional |  |


<a name="anytype.Rpc.Block.Set.Video.Width.Response.Error"/>
### Rpc.Block.Set.Video.Width.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Video.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Width.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.SetBreadcrumbs"/>
### Rpc.Block.SetBreadcrumbs


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.SetBreadcrumbs.Request"/>
### Rpc.Block.SetBreadcrumbs.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| breadcrumbsId | [string](#string) | optional |  |
| ids | [string](#string) | repeated |  |


<a name="anytype.Rpc.Block.SetBreadcrumbs.Response"/>
### Rpc.Block.SetBreadcrumbs.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.SetBreadcrumbs.Response.Error](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.SetBreadcrumbs.Response.Error"/>
### Rpc.Block.SetBreadcrumbs.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.SetBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.SetBreadcrumbs.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Show"/>
### Rpc.Block.Show


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Show.Request"/>
### Rpc.Block.Show.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| traceId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Show.Response"/>
### Rpc.Block.Show.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Show.Response.Error](#anytype.Rpc.Block.Show.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Show.Response.Error"/>
### Rpc.Block.Show.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Show.Response.Error.Code](#anytype.Rpc.Block.Show.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Split"/>
### Rpc.Block.Split


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Split.Request"/>
### Rpc.Block.Split.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| range | [Range](#anytype.model.Range) | optional |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |
| mode | [Rpc.Block.Split.Request.Mode](#anytype.Rpc.Block.Split.Request.Mode) | optional |  |


<a name="anytype.Rpc.Block.Split.Response"/>
### Rpc.Block.Split.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error) | optional |  |
| blockId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Split.Response.Error"/>
### Rpc.Block.Split.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Undo"/>
### Rpc.Block.Undo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Undo.Request"/>
### Rpc.Block.Undo.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Undo.Response"/>
### Rpc.Block.Undo.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Undo.Response.Error](#anytype.Rpc.Block.Undo.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| counters | [Rpc.Block.UndoRedoCounter](#anytype.Rpc.Block.UndoRedoCounter) | optional |  |


<a name="anytype.Rpc.Block.Undo.Response.Error"/>
### Rpc.Block.Undo.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Undo.Response.Error.Code](#anytype.Rpc.Block.Undo.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.UndoRedoCounter"/>
### Rpc.Block.UndoRedoCounter


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| undo | [int32](#int32) | optional |  |
| redo | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Block.Unlink"/>
### Rpc.Block.Unlink


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Unlink.Request"/>
### Rpc.Block.Unlink.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |


<a name="anytype.Rpc.Block.Unlink.Response"/>
### Rpc.Block.Unlink.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Unlink.Response.Error"/>
### Rpc.Block.Unlink.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.UpdateContent"/>
### Rpc.Block.UpdateContent


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.UpdateContent.Request"/>
### Rpc.Block.UpdateContent.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| block | [Block](#anytype.model.Block) | optional |  |


<a name="anytype.Rpc.Block.UpdateContent.Response"/>
### Rpc.Block.UpdateContent.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.UpdateContent.Response.Error](#anytype.Rpc.Block.UpdateContent.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.UpdateContent.Response.Error"/>
### Rpc.Block.UpdateContent.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.UpdateContent.Response.Error.Code](#anytype.Rpc.Block.UpdateContent.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Upload"/>
### Rpc.Block.Upload


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Block.Upload.Request"/>
### Rpc.Block.Upload.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockId | [string](#string) | optional |  |
| filePath | [string](#string) | optional |  |
| url | [string](#string) | optional |  |


<a name="anytype.Rpc.Block.Upload.Response"/>
### Rpc.Block.Upload.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Upload.Response.Error](#anytype.Rpc.Block.Upload.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Block.Upload.Response.Error"/>
### Rpc.Block.Upload.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList"/>
### Rpc.BlockList


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.ConvertChildrenToPages"/>
### Rpc.BlockList.ConvertChildrenToPages


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Request"/>
### Rpc.BlockList.ConvertChildrenToPages.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| objectType | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Response"/>
### Rpc.BlockList.ConvertChildrenToPages.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.ConvertChildrenToPages.Response.Error](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error) | optional |  |
| linkIds | [string](#string) | repeated |  |


<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error"/>
### Rpc.BlockList.ConvertChildrenToPages.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Duplicate"/>
### Rpc.BlockList.Duplicate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Duplicate.Request"/>
### Rpc.BlockList.Duplicate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| targetId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |


<a name="anytype.Rpc.BlockList.Duplicate.Response"/>
### Rpc.BlockList.Duplicate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Duplicate.Response.Error](#anytype.Rpc.BlockList.Duplicate.Response.Error) | optional |  |
| blockIds | [string](#string) | repeated |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Duplicate.Response.Error"/>
### Rpc.BlockList.Duplicate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Duplicate.Response.Error.Code](#anytype.Rpc.BlockList.Duplicate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Move"/>
### Rpc.BlockList.Move


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Move.Request"/>
### Rpc.BlockList.Move.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| targetContextId | [string](#string) | optional |  |
| dropTargetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |


<a name="anytype.Rpc.BlockList.Move.Response"/>
### Rpc.BlockList.Move.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Move.Response.Error](#anytype.Rpc.BlockList.Move.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Move.Response.Error"/>
### Rpc.BlockList.Move.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Move.Response.Error.Code](#anytype.Rpc.BlockList.Move.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.MoveToNewPage"/>
### Rpc.BlockList.MoveToNewPage


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.MoveToNewPage.Request"/>
### Rpc.BlockList.MoveToNewPage.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |
| dropTargetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |


<a name="anytype.Rpc.BlockList.MoveToNewPage.Response"/>
### Rpc.BlockList.MoveToNewPage.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.MoveToNewPage.Response.Error](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error) | optional |  |
| linkId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.MoveToNewPage.Response.Error"/>
### Rpc.BlockList.MoveToNewPage.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.MoveToNewPage.Response.Error.Code](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set"/>
### Rpc.BlockList.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Align"/>
### Rpc.BlockList.Set.Align


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Align.Request"/>
### Rpc.BlockList.Set.Align.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| align | [Block.Align](#anytype.model.Block.Align) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Align.Response"/>
### Rpc.BlockList.Set.Align.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Align.Response.Error](#anytype.Rpc.BlockList.Set.Align.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Align.Response.Error"/>
### Rpc.BlockList.Set.Align.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Align.Response.Error.Code](#anytype.Rpc.BlockList.Set.Align.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.BackgroundColor"/>
### Rpc.BlockList.Set.BackgroundColor


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Request"/>
### Rpc.BlockList.Set.BackgroundColor.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Response"/>
### Rpc.BlockList.Set.BackgroundColor.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.BackgroundColor.Response.Error](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error"/>
### Rpc.BlockList.Set.BackgroundColor.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Div"/>
### Rpc.BlockList.Set.Div


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Div.Style"/>
### Rpc.BlockList.Set.Div.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Div.Style.Request"/>
### Rpc.BlockList.Set.Div.Style.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| style | [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Div.Style.Response"/>
### Rpc.BlockList.Set.Div.Style.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Div.Style.Response.Error](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Div.Style.Response.Error"/>
### Rpc.BlockList.Set.Div.Style.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Div.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Fields"/>
### Rpc.BlockList.Set.Fields


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Fields.Request"/>
### Rpc.BlockList.Set.Fields.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockFields | [Rpc.BlockList.Set.Fields.Request.BlockField](#anytype.Rpc.BlockList.Set.Fields.Request.BlockField) | repeated |  |


<a name="anytype.Rpc.BlockList.Set.Fields.Request.BlockField"/>
### Rpc.BlockList.Set.Fields.Request.BlockField


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Fields.Response"/>
### Rpc.BlockList.Set.Fields.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Fields.Response.Error](#anytype.Rpc.BlockList.Set.Fields.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Fields.Response.Error"/>
### Rpc.BlockList.Set.Fields.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.File"/>
### Rpc.BlockList.Set.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.File.Style"/>
### Rpc.BlockList.Set.File.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.File.Style.Request"/>
### Rpc.BlockList.Set.File.Style.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| style | [Block.Content.File.Style](#anytype.model.Block.Content.File.Style) | optional |  |


<a name="anytype.Rpc.BlockList.Set.File.Style.Response"/>
### Rpc.BlockList.Set.File.Style.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.File.Style.Response.Error](#anytype.Rpc.BlockList.Set.File.Style.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.File.Style.Response.Error"/>
### Rpc.BlockList.Set.File.Style.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.File.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.File.Style.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text"/>
### Rpc.BlockList.Set.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Text.Color"/>
### Rpc.BlockList.Set.Text.Color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Text.Color.Request"/>
### Rpc.BlockList.Set.Text.Color.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Color.Response"/>
### Rpc.BlockList.Set.Text.Color.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.Color.Response.Error](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Color.Response.Error"/>
### Rpc.BlockList.Set.Text.Color.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Mark"/>
### Rpc.BlockList.Set.Text.Mark


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Text.Mark.Request"/>
### Rpc.BlockList.Set.Text.Mark.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| mark | [Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Mark.Response"/>
### Rpc.BlockList.Set.Text.Mark.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.Mark.Response.Error](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Mark.Response.Error"/>
### Rpc.BlockList.Set.Text.Mark.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Mark.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Mark.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Style"/>
### Rpc.BlockList.Set.Text.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.Set.Text.Style.Request"/>
### Rpc.BlockList.Set.Text.Style.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Style.Response"/>
### Rpc.BlockList.Set.Text.Style.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.Style.Response.Error](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.Set.Text.Style.Response.Error"/>
### Rpc.BlockList.Set.Text.Style.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.BlockList.TurnInto"/>
### Rpc.BlockList.TurnInto


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.BlockList.TurnInto.Request"/>
### Rpc.BlockList.TurnInto.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| blockIds | [string](#string) | repeated |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |


<a name="anytype.Rpc.BlockList.TurnInto.Response"/>
### Rpc.BlockList.TurnInto.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.TurnInto.Response.Error](#anytype.Rpc.BlockList.TurnInto.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.BlockList.TurnInto.Response.Error"/>
### Rpc.BlockList.TurnInto.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.TurnInto.Response.Error.Code](#anytype.Rpc.BlockList.TurnInto.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.CloneTemplate"/>
### Rpc.CloneTemplate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.CloneTemplate.Request"/>
### Rpc.CloneTemplate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |


<a name="anytype.Rpc.CloneTemplate.Response"/>
### Rpc.CloneTemplate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.CloneTemplate.Response.Error](#anytype.Rpc.CloneTemplate.Response.Error) | optional |  |
| id | [string](#string) | optional |  |


<a name="anytype.Rpc.CloneTemplate.Response.Error"/>
### Rpc.CloneTemplate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.CloneTemplate.Response.Error.Code](#anytype.Rpc.CloneTemplate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Config"/>
### Rpc.Config


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Config.Get"/>
### Rpc.Config.Get


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Config.Get.Request"/>
### Rpc.Config.Get.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Config.Get.Response"/>
### Rpc.Config.Get.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Config.Get.Response.Error](#anytype.Rpc.Config.Get.Response.Error) | optional |  |
| homeBlockId | [string](#string) | optional |  |
| archiveBlockId | [string](#string) | optional |  |
| profileBlockId | [string](#string) | optional |  |
| marketplaceTypeId | [string](#string) | optional |  |
| marketplaceRelationId | [string](#string) | optional |  |
| marketplaceTemplateId | [string](#string) | optional |  |
| gatewayUrl | [string](#string) | optional |  |


<a name="anytype.Rpc.Config.Get.Response.Error"/>
### Rpc.Config.Get.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug"/>
### Rpc.Debug


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Debug.Sync"/>
### Rpc.Debug.Sync


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Debug.Sync.Request"/>
### Rpc.Debug.Sync.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| recordsTraverseLimit | [int32](#int32) | optional |  |
| skipEmptyLogs | [bool](#bool) | optional |  |
| tryToDownloadRemoteRecords | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Debug.Sync.Response"/>
### Rpc.Debug.Sync.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Sync.Response.Error](#anytype.Rpc.Debug.Sync.Response.Error) | optional |  |
| threads | [Rpc.Debug.threadInfo](#anytype.Rpc.Debug.threadInfo) | repeated |  |
| deviceId | [string](#string) | optional |  |
| totalThreads | [int32](#int32) | optional |  |
| threadsWithoutReplInOwnLog | [int32](#int32) | optional |  |
| threadsWithoutHeadDownloaded | [int32](#int32) | optional |  |
| totalRecords | [int32](#int32) | optional |  |
| totalSize | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Debug.Sync.Response.Error"/>
### Rpc.Debug.Sync.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Sync.Response.Error.Code](#anytype.Rpc.Debug.Sync.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug.Thread"/>
### Rpc.Debug.Thread


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Debug.Thread.Request"/>
### Rpc.Debug.Thread.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| threadId | [string](#string) | optional |  |
| skipEmptyLogs | [bool](#bool) | optional |  |
| tryToDownloadRemoteRecords | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Debug.Thread.Response"/>
### Rpc.Debug.Thread.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Thread.Response.Error](#anytype.Rpc.Debug.Thread.Response.Error) | optional |  |
| info | [Rpc.Debug.threadInfo](#anytype.Rpc.Debug.threadInfo) | optional |  |


<a name="anytype.Rpc.Debug.Thread.Response.Error"/>
### Rpc.Debug.Thread.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Thread.Response.Error.Code](#anytype.Rpc.Debug.Thread.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug.Tree"/>
### Rpc.Debug.Tree


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Debug.Tree.Request"/>
### Rpc.Debug.Tree.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) | optional |  |
| path | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug.Tree.Response"/>
### Rpc.Debug.Tree.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Debug.Tree.Response.Error](#anytype.Rpc.Debug.Tree.Response.Error) | optional |  |
| filename | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug.Tree.Response.Error"/>
### Rpc.Debug.Tree.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Debug.Tree.Response.Error.Code](#anytype.Rpc.Debug.Tree.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug.logInfo"/>
### Rpc.Debug.logInfo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| head | [string](#string) | optional |  |
| headDownloaded | [bool](#bool) | optional |  |
| totalRecords | [int32](#int32) | optional |  |
| totalSize | [int32](#int32) | optional |  |
| firstRecordTs | [int32](#int32) | optional |  |
| firstRecordVer | [int32](#int32) | optional |  |
| lastRecordTs | [int32](#int32) | optional |  |
| lastRecordVer | [int32](#int32) | optional |  |
| lastPullSecAgo | [int32](#int32) | optional |  |
| upStatus | [string](#string) | optional |  |
| downStatus | [string](#string) | optional |  |
| error | [string](#string) | optional |  |


<a name="anytype.Rpc.Debug.threadInfo"/>
### Rpc.Debug.threadInfo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| logsWithDownloadedHead | [int32](#int32) | optional |  |
| logsWithWholeTreeDownloaded | [int32](#int32) | optional |  |
| logs | [Rpc.Debug.logInfo](#anytype.Rpc.Debug.logInfo) | repeated |  |
| ownLogHasCafeReplicator | [bool](#bool) | optional |  |
| cafeLastPullSecAgo | [int32](#int32) | optional |  |
| cafeUpStatus | [string](#string) | optional |  |
| cafeDownStatus | [string](#string) | optional |  |
| totalRecords | [int32](#int32) | optional |  |
| totalSize | [int32](#int32) | optional |  |
| error | [string](#string) | optional |  |


<a name="anytype.Rpc.DownloadFile"/>
### Rpc.DownloadFile


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.DownloadFile.Request"/>
### Rpc.DownloadFile.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) | optional |  |
| path | [string](#string) | optional |  |


<a name="anytype.Rpc.DownloadFile.Response"/>
### Rpc.DownloadFile.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.DownloadFile.Response.Error](#anytype.Rpc.DownloadFile.Response.Error) | optional |  |
| localPath | [string](#string) | optional |  |


<a name="anytype.Rpc.DownloadFile.Response.Error"/>
### Rpc.DownloadFile.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.DownloadFile.Response.Error.Code](#anytype.Rpc.DownloadFile.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Export"/>
### Rpc.Export


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Export.Request"/>
### Rpc.Export.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | optional |  |
| docIds | [string](#string) | repeated |  |
| format | [Rpc.Export.Format](#anytype.Rpc.Export.Format) | optional |  |
| zip | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Export.Response"/>
### Rpc.Export.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Export.Response.Error](#anytype.Rpc.Export.Response.Error) | optional |  |
| path | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Export.Response.Error"/>
### Rpc.Export.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Export.Response.Error.Code](#anytype.Rpc.Export.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ExportLocalstore"/>
### Rpc.ExportLocalstore


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ExportLocalstore.Request"/>
### Rpc.ExportLocalstore.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | optional |  |
| docIds | [string](#string) | repeated |  |


<a name="anytype.Rpc.ExportLocalstore.Response"/>
### Rpc.ExportLocalstore.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExportLocalstore.Response.Error](#anytype.Rpc.ExportLocalstore.Response.Error) | optional |  |
| path | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.ExportLocalstore.Response.Error"/>
### Rpc.ExportLocalstore.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExportLocalstore.Response.Error.Code](#anytype.Rpc.ExportLocalstore.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ExportTemplates"/>
### Rpc.ExportTemplates


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ExportTemplates.Request"/>
### Rpc.ExportTemplates.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| path | [string](#string) | optional |  |


<a name="anytype.Rpc.ExportTemplates.Response"/>
### Rpc.ExportTemplates.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExportTemplates.Response.Error](#anytype.Rpc.ExportTemplates.Response.Error) | optional |  |
| path | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.ExportTemplates.Response.Error"/>
### Rpc.ExportTemplates.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExportTemplates.Response.Error.Code](#anytype.Rpc.ExportTemplates.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ExternalDrop"/>
### Rpc.ExternalDrop


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ExternalDrop.Content"/>
### Rpc.ExternalDrop.Content


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ExternalDrop.Content.Request"/>
### Rpc.ExternalDrop.Content.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| focusedBlockId | [string](#string) | optional |  |
| content | [bytes](#bytes) | optional |  |


<a name="anytype.Rpc.ExternalDrop.Content.Response"/>
### Rpc.ExternalDrop.Content.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExternalDrop.Content.Response.Error](#anytype.Rpc.ExternalDrop.Content.Response.Error) | optional |  |


<a name="anytype.Rpc.ExternalDrop.Content.Response.Error"/>
### Rpc.ExternalDrop.Content.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExternalDrop.Content.Response.Error.Code](#anytype.Rpc.ExternalDrop.Content.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ExternalDrop.Files"/>
### Rpc.ExternalDrop.Files


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ExternalDrop.Files.Request"/>
### Rpc.ExternalDrop.Files.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| dropTargetId | [string](#string) | optional |  |
| position | [Block.Position](#anytype.model.Block.Position) | optional |  |
| localFilePaths | [string](#string) | repeated |  |


<a name="anytype.Rpc.ExternalDrop.Files.Response"/>
### Rpc.ExternalDrop.Files.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExternalDrop.Files.Response.Error](#anytype.Rpc.ExternalDrop.Files.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.ExternalDrop.Files.Response.Error"/>
### Rpc.ExternalDrop.Files.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.File"/>
### Rpc.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.File.Offload"/>
### Rpc.File.Offload


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.File.Offload.Request"/>
### Rpc.File.Offload.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| includeNotPinned | [bool](#bool) | optional |  |


<a name="anytype.Rpc.File.Offload.Response"/>
### Rpc.File.Offload.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.File.Offload.Response.Error](#anytype.Rpc.File.Offload.Response.Error) | optional |  |
| bytesOffloaded | [uint64](#uint64) | optional |  |


<a name="anytype.Rpc.File.Offload.Response.Error"/>
### Rpc.File.Offload.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.File.Offload.Response.Error.Code](#anytype.Rpc.File.Offload.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.FileList"/>
### Rpc.FileList


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.FileList.Offload"/>
### Rpc.FileList.Offload


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.FileList.Offload.Request"/>
### Rpc.FileList.Offload.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| onlyIds | [string](#string) | repeated |  |
| includeNotPinned | [bool](#bool) | optional |  |


<a name="anytype.Rpc.FileList.Offload.Response"/>
### Rpc.FileList.Offload.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.FileList.Offload.Response.Error](#anytype.Rpc.FileList.Offload.Response.Error) | optional |  |
| filesOffloaded | [int32](#int32) | optional |  |
| bytesOffloaded | [uint64](#uint64) | optional |  |


<a name="anytype.Rpc.FileList.Offload.Response.Error"/>
### Rpc.FileList.Offload.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.FileList.Offload.Response.Error.Code](#anytype.Rpc.FileList.Offload.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.GenericErrorResponse"/>
### Rpc.GenericErrorResponse


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.GenericErrorResponse.Error](#anytype.Rpc.GenericErrorResponse.Error) | optional |  |


<a name="anytype.Rpc.GenericErrorResponse.Error"/>
### Rpc.GenericErrorResponse.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.GenericErrorResponse.Error.Code](#anytype.Rpc.GenericErrorResponse.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.History"/>
### Rpc.History


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.History.SetVersion"/>
### Rpc.History.SetVersion


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.History.SetVersion.Request"/>
### Rpc.History.SetVersion.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) | optional |  |
| versionId | [string](#string) | optional |  |


<a name="anytype.Rpc.History.SetVersion.Response"/>
### Rpc.History.SetVersion.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.SetVersion.Response.Error](#anytype.Rpc.History.SetVersion.Response.Error) | optional |  |


<a name="anytype.Rpc.History.SetVersion.Response.Error"/>
### Rpc.History.SetVersion.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.SetVersion.Response.Error.Code](#anytype.Rpc.History.SetVersion.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.History.Show"/>
### Rpc.History.Show


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.History.Show.Request"/>
### Rpc.History.Show.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) | optional |  |
| versionId | [string](#string) | optional |  |
| traceId | [string](#string) | optional |  |


<a name="anytype.Rpc.History.Show.Response"/>
### Rpc.History.Show.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.Show.Response.Error](#anytype.Rpc.History.Show.Response.Error) | optional |  |
| objectShow | [Event.Object.Show](#anytype.Event.Object.Show) | optional |  |
| version | [Rpc.History.Versions.Version](#anytype.Rpc.History.Versions.Version) | optional |  |
| traceId | [string](#string) | optional |  |


<a name="anytype.Rpc.History.Show.Response.Error"/>
### Rpc.History.Show.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.Show.Response.Error.Code](#anytype.Rpc.History.Show.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.History.Versions"/>
### Rpc.History.Versions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.History.Versions.Request"/>
### Rpc.History.Versions.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pageId | [string](#string) | optional |  |
| lastVersionId | [string](#string) | optional |  |
| limit | [int32](#int32) | optional |  |


<a name="anytype.Rpc.History.Versions.Response"/>
### Rpc.History.Versions.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.History.Versions.Response.Error](#anytype.Rpc.History.Versions.Response.Error) | optional |  |
| versions | [Rpc.History.Versions.Version](#anytype.Rpc.History.Versions.Version) | repeated |  |


<a name="anytype.Rpc.History.Versions.Response.Error"/>
### Rpc.History.Versions.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.History.Versions.Response.Error.Code](#anytype.Rpc.History.Versions.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.History.Versions.Version"/>
### Rpc.History.Versions.Version


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| previousIds | [string](#string) | repeated |  |
| authorId | [string](#string) | optional |  |
| authorName | [string](#string) | optional |  |
| time | [int64](#int64) | optional |  |
| groupId | [int64](#int64) | optional |  |


<a name="anytype.Rpc.LinkPreview"/>
### Rpc.LinkPreview


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.LinkPreview.Request"/>
### Rpc.LinkPreview.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |


<a name="anytype.Rpc.LinkPreview.Response"/>
### Rpc.LinkPreview.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.LinkPreview.Response.Error](#anytype.Rpc.LinkPreview.Response.Error) | optional |  |
| linkPreview | [LinkPreview](#anytype.model.LinkPreview) | optional |  |


<a name="anytype.Rpc.LinkPreview.Response.Error"/>
### Rpc.LinkPreview.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.LinkPreview.Response.Error.Code](#anytype.Rpc.LinkPreview.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Log"/>
### Rpc.Log


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Log.Send"/>
### Rpc.Log.Send


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Log.Send.Request"/>
### Rpc.Log.Send.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| message | [string](#string) | optional |  |
| level | [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level) | optional |  |


<a name="anytype.Rpc.Log.Send.Response"/>
### Rpc.Log.Send.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error) | optional |  |


<a name="anytype.Rpc.Log.Send.Response.Error"/>
### Rpc.Log.Send.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.MakeTemplate"/>
### Rpc.MakeTemplate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.MakeTemplate.Request"/>
### Rpc.MakeTemplate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |


<a name="anytype.Rpc.MakeTemplate.Response"/>
### Rpc.MakeTemplate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MakeTemplate.Response.Error](#anytype.Rpc.MakeTemplate.Response.Error) | optional |  |
| id | [string](#string) | optional |  |


<a name="anytype.Rpc.MakeTemplate.Response.Error"/>
### Rpc.MakeTemplate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MakeTemplate.Response.Error.Code](#anytype.Rpc.MakeTemplate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.MakeTemplateByObjectType"/>
### Rpc.MakeTemplateByObjectType


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.MakeTemplateByObjectType.Request"/>
### Rpc.MakeTemplateByObjectType.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectType | [string](#string) | optional |  |


<a name="anytype.Rpc.MakeTemplateByObjectType.Response"/>
### Rpc.MakeTemplateByObjectType.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.MakeTemplateByObjectType.Response.Error](#anytype.Rpc.MakeTemplateByObjectType.Response.Error) | optional |  |
| id | [string](#string) | optional |  |


<a name="anytype.Rpc.MakeTemplateByObjectType.Response.Error"/>
### Rpc.MakeTemplateByObjectType.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.MakeTemplateByObjectType.Response.Error.Code](#anytype.Rpc.MakeTemplateByObjectType.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Navigation"/>
### Rpc.Navigation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks"/>
### Rpc.Navigation.GetObjectInfoWithLinks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Request"/>
### Rpc.Navigation.GetObjectInfoWithLinks.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) | optional |  |
| context | [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context) | optional |  |


<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response"/>
### Rpc.Navigation.GetObjectInfoWithLinks.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error) | optional |  |
| object | [ObjectInfoWithLinks](#anytype.model.ObjectInfoWithLinks) | optional |  |


<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error"/>
### Rpc.Navigation.GetObjectInfoWithLinks.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code](#anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Navigation.ListObjects"/>
### Rpc.Navigation.ListObjects


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Navigation.ListObjects.Request"/>
### Rpc.Navigation.ListObjects.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| context | [Rpc.Navigation.Context](#anytype.Rpc.Navigation.Context) | optional |  |
| fullText | [string](#string) | optional |  |
| limit | [int32](#int32) | optional |  |
| offset | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Navigation.ListObjects.Response"/>
### Rpc.Navigation.ListObjects.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Navigation.ListObjects.Response.Error](#anytype.Rpc.Navigation.ListObjects.Response.Error) | optional |  |
| objects | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |


<a name="anytype.Rpc.Navigation.ListObjects.Response.Error"/>
### Rpc.Navigation.ListObjects.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Navigation.ListObjects.Response.Error.Code](#anytype.Rpc.Navigation.ListObjects.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object"/>
### Rpc.Object


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.AddWithObjectId"/>
### Rpc.Object.AddWithObjectId


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.AddWithObjectId.Request"/>
### Rpc.Object.AddWithObjectId.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) | optional |  |
| payload | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.AddWithObjectId.Response"/>
### Rpc.Object.AddWithObjectId.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.AddWithObjectId.Response.Error](#anytype.Rpc.Object.AddWithObjectId.Response.Error) | optional |  |


<a name="anytype.Rpc.Object.AddWithObjectId.Response.Error"/>
### Rpc.Object.AddWithObjectId.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.AddWithObjectId.Response.Error.Code](#anytype.Rpc.Object.AddWithObjectId.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.FeaturedRelation"/>
### Rpc.Object.FeaturedRelation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.FeaturedRelation.Add"/>
### Rpc.Object.FeaturedRelation.Add


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.FeaturedRelation.Add.Request"/>
### Rpc.Object.FeaturedRelation.Add.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relations | [string](#string) | repeated |  |


<a name="anytype.Rpc.Object.FeaturedRelation.Add.Response"/>
### Rpc.Object.FeaturedRelation.Add.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.FeaturedRelation.Add.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.FeaturedRelation.Add.Response.Error"/>
### Rpc.Object.FeaturedRelation.Add.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.FeaturedRelation.Add.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Add.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.FeaturedRelation.Remove"/>
### Rpc.Object.FeaturedRelation.Remove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Request"/>
### Rpc.Object.FeaturedRelation.Remove.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relations | [string](#string) | repeated |  |


<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Response"/>
### Rpc.Object.FeaturedRelation.Remove.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.FeaturedRelation.Remove.Response.Error](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error"/>
### Rpc.Object.FeaturedRelation.Remove.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.FeaturedRelation.Remove.Response.Error.Code](#anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.Graph"/>
### Rpc.Object.Graph


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.Graph.Edge"/>
### Rpc.Object.Graph.Edge


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | optional |  |
| target | [string](#string) | optional |  |
| name | [string](#string) | optional |  |
| type | [Rpc.Object.Graph.Edge.Type](#anytype.Rpc.Object.Graph.Edge.Type) | optional |  |
| description | [string](#string) | optional |  |
| iconImage | [string](#string) | optional |  |
| iconEmoji | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.Graph.Node"/>
### Rpc.Object.Graph.Node


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| type | [string](#string) | optional |  |
| name | [string](#string) | optional |  |
| layout | [int32](#int32) | optional |  |
| description | [string](#string) | optional |  |
| iconImage | [string](#string) | optional |  |
| iconEmoji | [string](#string) | optional |  |
| done | [bool](#bool) | optional |  |
| relationFormat | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Object.Graph.Request"/>
### Rpc.Object.Graph.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filters | [Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated |  |
| limit | [int32](#int32) | optional |  |
| objectTypeFilter | [string](#string) | repeated |  |


<a name="anytype.Rpc.Object.Graph.Response"/>
### Rpc.Object.Graph.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Graph.Response.Error](#anytype.Rpc.Object.Graph.Response.Error) | optional |  |
| nodes | [Rpc.Object.Graph.Node](#anytype.Rpc.Object.Graph.Node) | repeated |  |
| edges | [Rpc.Object.Graph.Edge](#anytype.Rpc.Object.Graph.Edge) | repeated |  |


<a name="anytype.Rpc.Object.Graph.Response.Error"/>
### Rpc.Object.Graph.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Graph.Response.Error.Code](#anytype.Rpc.Object.Graph.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationAdd"/>
### Rpc.Object.RelationAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationAdd.Request"/>
### Rpc.Object.RelationAdd.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Object.RelationAdd.Response"/>
### Rpc.Object.RelationAdd.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationAdd.Response.Error](#anytype.Rpc.Object.RelationAdd.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| relationKey | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Object.RelationAdd.Response.Error"/>
### Rpc.Object.RelationAdd.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationAdd.Response.Error.Code](#anytype.Rpc.Object.RelationAdd.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationDelete"/>
### Rpc.Object.RelationDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationDelete.Request"/>
### Rpc.Object.RelationDelete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationDelete.Response"/>
### Rpc.Object.RelationDelete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationDelete.Response.Error](#anytype.Rpc.Object.RelationDelete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.RelationDelete.Response.Error"/>
### Rpc.Object.RelationDelete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationDelete.Response.Error.Code](#anytype.Rpc.Object.RelationDelete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationListAvailable"/>
### Rpc.Object.RelationListAvailable


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationListAvailable.Request"/>
### Rpc.Object.RelationListAvailable.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationListAvailable.Response"/>
### Rpc.Object.RelationListAvailable.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationListAvailable.Response.Error](#anytype.Rpc.Object.RelationListAvailable.Response.Error) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Rpc.Object.RelationListAvailable.Response.Error"/>
### Rpc.Object.RelationListAvailable.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationListAvailable.Response.Error.Code](#anytype.Rpc.Object.RelationListAvailable.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionAdd"/>
### Rpc.Object.RelationOptionAdd


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationOptionAdd.Request"/>
### Rpc.Object.RelationOptionAdd.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| option | [Relation.Option](#anytype.model.Relation.Option) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionAdd.Response"/>
### Rpc.Object.RelationOptionAdd.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationOptionAdd.Response.Error](#anytype.Rpc.Object.RelationOptionAdd.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |
| option | [Relation.Option](#anytype.model.Relation.Option) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionAdd.Response.Error"/>
### Rpc.Object.RelationOptionAdd.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationOptionAdd.Response.Error.Code](#anytype.Rpc.Object.RelationOptionAdd.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionDelete"/>
### Rpc.Object.RelationOptionDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationOptionDelete.Request"/>
### Rpc.Object.RelationOptionDelete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| optionId | [string](#string) | optional |  |
| confirmRemoveAllValuesInRecords | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionDelete.Response"/>
### Rpc.Object.RelationOptionDelete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationOptionDelete.Response.Error](#anytype.Rpc.Object.RelationOptionDelete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionDelete.Response.Error"/>
### Rpc.Object.RelationOptionDelete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationOptionDelete.Response.Error.Code](#anytype.Rpc.Object.RelationOptionDelete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionUpdate"/>
### Rpc.Object.RelationOptionUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationOptionUpdate.Request"/>
### Rpc.Object.RelationOptionUpdate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| option | [Relation.Option](#anytype.model.Relation.Option) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionUpdate.Response"/>
### Rpc.Object.RelationOptionUpdate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationOptionUpdate.Response.Error](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.RelationOptionUpdate.Response.Error"/>
### Rpc.Object.RelationOptionUpdate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationOptionUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationOptionUpdate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.RelationUpdate"/>
### Rpc.Object.RelationUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.RelationUpdate.Request"/>
### Rpc.Object.RelationUpdate.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.Object.RelationUpdate.Response"/>
### Rpc.Object.RelationUpdate.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.RelationUpdate.Response.Error](#anytype.Rpc.Object.RelationUpdate.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.RelationUpdate.Response.Error"/>
### Rpc.Object.RelationUpdate.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.RelationUpdate.Response.Error.Code](#anytype.Rpc.Object.RelationUpdate.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.Search"/>
### Rpc.Object.Search


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.Search.Request"/>
### Rpc.Object.Search.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| filters | [Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated |  |
| sorts | [Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort) | repeated |  |
| fullText | [string](#string) | optional |  |
| offset | [int32](#int32) | optional |  |
| limit | [int32](#int32) | optional |  |
| objectTypeFilter | [string](#string) | repeated |  |
| keys | [string](#string) | repeated |  |
| ignoreWorkspace | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Object.Search.Response"/>
### Rpc.Object.Search.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.Search.Response.Error](#anytype.Rpc.Object.Search.Response.Error) | optional |  |
| records | [Struct](#google.protobuf.Struct) | repeated |  |


<a name="anytype.Rpc.Object.Search.Response.Error"/>
### Rpc.Object.Search.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.Search.Response.Error.Code](#anytype.Rpc.Object.Search.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.SetIsArchived"/>
### Rpc.Object.SetIsArchived


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.SetIsArchived.Request"/>
### Rpc.Object.SetIsArchived.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| isArchived | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Object.SetIsArchived.Response"/>
### Rpc.Object.SetIsArchived.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetIsArchived.Response.Error](#anytype.Rpc.Object.SetIsArchived.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.SetIsArchived.Response.Error"/>
### Rpc.Object.SetIsArchived.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetIsArchived.Response.Error.Code](#anytype.Rpc.Object.SetIsArchived.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.SetIsFavorite"/>
### Rpc.Object.SetIsFavorite


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.SetIsFavorite.Request"/>
### Rpc.Object.SetIsFavorite.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| isFavorite | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Object.SetIsFavorite.Response"/>
### Rpc.Object.SetIsFavorite.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetIsFavorite.Response.Error](#anytype.Rpc.Object.SetIsFavorite.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.SetIsFavorite.Response.Error"/>
### Rpc.Object.SetIsFavorite.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetIsFavorite.Response.Error.Code](#anytype.Rpc.Object.SetIsFavorite.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.SetLayout"/>
### Rpc.Object.SetLayout


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.SetLayout.Request"/>
### Rpc.Object.SetLayout.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| layout | [ObjectType.Layout](#anytype.model.ObjectType.Layout) | optional |  |


<a name="anytype.Rpc.Object.SetLayout.Response"/>
### Rpc.Object.SetLayout.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.SetLayout.Response.Error](#anytype.Rpc.Object.SetLayout.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Object.SetLayout.Response.Error"/>
### Rpc.Object.SetLayout.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.SetLayout.Response.Error.Code](#anytype.Rpc.Object.SetLayout.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.ShareByLink"/>
### Rpc.Object.ShareByLink


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.ShareByLink.Request"/>
### Rpc.Object.ShareByLink.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.ShareByLink.Response"/>
### Rpc.Object.ShareByLink.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| link | [string](#string) | optional |  |
| error | [Rpc.Object.ShareByLink.Response.Error](#anytype.Rpc.Object.ShareByLink.Response.Error) | optional |  |


<a name="anytype.Rpc.Object.ShareByLink.Response.Error"/>
### Rpc.Object.ShareByLink.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ShareByLink.Response.Error.Code](#anytype.Rpc.Object.ShareByLink.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.ToSet"/>
### Rpc.Object.ToSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Object.ToSet.Request"/>
### Rpc.Object.ToSet.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) | optional |  |
| source | [string](#string) | repeated |  |


<a name="anytype.Rpc.Object.ToSet.Response"/>
### Rpc.Object.ToSet.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Object.ToSet.Response.Error](#anytype.Rpc.Object.ToSet.Response.Error) | optional |  |
| setId | [string](#string) | optional |  |


<a name="anytype.Rpc.Object.ToSet.Response.Error"/>
### Rpc.Object.ToSet.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Object.ToSet.Response.Error.Code](#anytype.Rpc.Object.ToSet.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectList"/>
### Rpc.ObjectList


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectList.Delete"/>
### Rpc.ObjectList.Delete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectList.Delete.Request"/>
### Rpc.ObjectList.Delete.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |


<a name="anytype.Rpc.ObjectList.Delete.Response"/>
### Rpc.ObjectList.Delete.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectList.Delete.Response.Error](#anytype.Rpc.ObjectList.Delete.Response.Error) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.ObjectList.Delete.Response.Error"/>
### Rpc.ObjectList.Delete.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectList.Delete.Response.Error.Code](#anytype.Rpc.ObjectList.Delete.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectList.Set"/>
### Rpc.ObjectList.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectList.Set.IsArchived"/>
### Rpc.ObjectList.Set.IsArchived


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectList.Set.IsArchived.Request"/>
### Rpc.ObjectList.Set.IsArchived.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isArchived | [bool](#bool) | optional |  |


<a name="anytype.Rpc.ObjectList.Set.IsArchived.Response"/>
### Rpc.ObjectList.Set.IsArchived.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectList.Set.IsArchived.Response.Error](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error) | optional |  |


<a name="anytype.Rpc.ObjectList.Set.IsArchived.Response.Error"/>
### Rpc.ObjectList.Set.IsArchived.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectList.Set.IsArchived.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsArchived.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectList.Set.IsFavorite"/>
### Rpc.ObjectList.Set.IsFavorite


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Request"/>
### Rpc.ObjectList.Set.IsFavorite.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectIds | [string](#string) | repeated |  |
| isFavorite | [bool](#bool) | optional |  |


<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Response"/>
### Rpc.ObjectList.Set.IsFavorite.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectList.Set.IsFavorite.Response.Error](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error) | optional |  |


<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error"/>
### Rpc.ObjectList.Set.IsFavorite.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectList.Set.IsFavorite.Response.Error.Code](#anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType"/>
### Rpc.ObjectType


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Create"/>
### Rpc.ObjectType.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Create.Request"/>
### Rpc.ObjectType.Create.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectType | [ObjectType](#anytype.model.ObjectType) | optional |  |


<a name="anytype.Rpc.ObjectType.Create.Response"/>
### Rpc.ObjectType.Create.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Create.Response.Error](#anytype.Rpc.ObjectType.Create.Response.Error) | optional |  |
| objectType | [ObjectType](#anytype.model.ObjectType) | optional |  |


<a name="anytype.Rpc.ObjectType.Create.Response.Error"/>
### Rpc.ObjectType.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Create.Response.Error.Code](#anytype.Rpc.ObjectType.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType.List"/>
### Rpc.ObjectType.List


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.List.Request"/>
### Rpc.ObjectType.List.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.List.Response"/>
### Rpc.ObjectType.List.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.List.Response.Error](#anytype.Rpc.ObjectType.List.Response.Error) | optional |  |
| objectTypes | [ObjectType](#anytype.model.ObjectType) | repeated |  |


<a name="anytype.Rpc.ObjectType.List.Response.Error"/>
### Rpc.ObjectType.List.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.List.Response.Error.Code](#anytype.Rpc.ObjectType.List.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation"/>
### Rpc.ObjectType.Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Relation.Add"/>
### Rpc.ObjectType.Relation.Add


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Relation.Add.Request"/>
### Rpc.ObjectType.Relation.Add.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Rpc.ObjectType.Relation.Add.Response"/>
### Rpc.ObjectType.Relation.Add.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Add.Response.Error](#anytype.Rpc.ObjectType.Relation.Add.Response.Error) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Rpc.ObjectType.Relation.Add.Response.Error"/>
### Rpc.ObjectType.Relation.Add.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Add.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.List"/>
### Rpc.ObjectType.Relation.List


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Relation.List.Request"/>
### Rpc.ObjectType.Relation.List.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) | optional |  |
| appendRelationsFromOtherTypes | [bool](#bool) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.List.Response"/>
### Rpc.ObjectType.Relation.List.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.List.Response.Error](#anytype.Rpc.ObjectType.Relation.List.Response.Error) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Rpc.ObjectType.Relation.List.Response.Error"/>
### Rpc.ObjectType.Relation.List.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.List.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.List.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.Remove"/>
### Rpc.ObjectType.Relation.Remove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Relation.Remove.Request"/>
### Rpc.ObjectType.Relation.Remove.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.Remove.Response"/>
### Rpc.ObjectType.Relation.Remove.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Remove.Response.Error](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.Remove.Response.Error"/>
### Rpc.ObjectType.Relation.Remove.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Remove.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.Update"/>
### Rpc.ObjectType.Relation.Update


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.ObjectType.Relation.Update.Request"/>
### Rpc.ObjectType.Relation.Update.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectTypeUrl | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.Update.Response"/>
### Rpc.ObjectType.Relation.Update.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ObjectType.Relation.Update.Response.Error](#anytype.Rpc.ObjectType.Relation.Update.Response.Error) | optional |  |


<a name="anytype.Rpc.ObjectType.Relation.Update.Response.Error"/>
### Rpc.ObjectType.Relation.Update.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ObjectType.Relation.Update.Response.Error.Code](#anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Page"/>
### Rpc.Page


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Page.Create"/>
### Rpc.Page.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Page.Create.Request"/>
### Rpc.Page.Create.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Rpc.Page.Create.Response"/>
### Rpc.Page.Create.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Page.Create.Response.Error](#anytype.Rpc.Page.Create.Response.Error) | optional |  |
| pageId | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Page.Create.Response.Error"/>
### Rpc.Page.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Page.Create.Response.Error.Code](#anytype.Rpc.Page.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Ping"/>
### Rpc.Ping


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Ping.Request"/>
### Rpc.Ping.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) | optional |  |
| numberOfEventsToSend | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Ping.Response"/>
### Rpc.Ping.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Ping.Response.Error](#anytype.Rpc.Ping.Response.Error) | optional |  |
| index | [int32](#int32) | optional |  |


<a name="anytype.Rpc.Ping.Response.Error"/>
### Rpc.Ping.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Ping.Response.Error.Code](#anytype.Rpc.Ping.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Process"/>
### Rpc.Process


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Process.Cancel"/>
### Rpc.Process.Cancel


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Process.Cancel.Request"/>
### Rpc.Process.Cancel.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |


<a name="anytype.Rpc.Process.Cancel.Response"/>
### Rpc.Process.Cancel.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Process.Cancel.Response.Error](#anytype.Rpc.Process.Cancel.Response.Error) | optional |  |


<a name="anytype.Rpc.Process.Cancel.Response.Error"/>
### Rpc.Process.Cancel.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Process.Cancel.Response.Error.Code](#anytype.Rpc.Process.Cancel.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Set"/>
### Rpc.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Set.Create"/>
### Rpc.Set.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Set.Create.Request"/>
### Rpc.Set.Create.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |
| templateId | [string](#string) | optional |  |


<a name="anytype.Rpc.Set.Create.Response"/>
### Rpc.Set.Create.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Set.Create.Response.Error](#anytype.Rpc.Set.Create.Response.Error) | optional |  |
| id | [string](#string) | optional |  |
| event | [ResponseEvent](#anytype.ResponseEvent) | optional |  |


<a name="anytype.Rpc.Set.Create.Response.Error"/>
### Rpc.Set.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Set.Create.Response.Error.Code](#anytype.Rpc.Set.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Shutdown"/>
### Rpc.Shutdown


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Shutdown.Request"/>
### Rpc.Shutdown.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Shutdown.Response"/>
### Rpc.Shutdown.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Shutdown.Response.Error](#anytype.Rpc.Shutdown.Response.Error) | optional |  |


<a name="anytype.Rpc.Shutdown.Response.Error"/>
### Rpc.Shutdown.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Shutdown.Response.Error.Code](#anytype.Rpc.Shutdown.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.UploadFile"/>
### Rpc.UploadFile


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.UploadFile.Request"/>
### Rpc.UploadFile.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |
| localPath | [string](#string) | optional |  |
| type | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) | optional |  |
| disableEncryption | [bool](#bool) | optional |  |
| style | [Block.Content.File.Style](#anytype.model.Block.Content.File.Style) | optional |  |


<a name="anytype.Rpc.UploadFile.Response"/>
### Rpc.UploadFile.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.UploadFile.Response.Error](#anytype.Rpc.UploadFile.Response.Error) | optional |  |
| hash | [string](#string) | optional |  |


<a name="anytype.Rpc.UploadFile.Response.Error"/>
### Rpc.UploadFile.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.UploadFile.Response.Error.Code](#anytype.Rpc.UploadFile.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Version"/>
### Rpc.Version


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Version.Get"/>
### Rpc.Version.Get


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Version.Get.Request"/>
### Rpc.Version.Get.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Version.Get.Response"/>
### Rpc.Version.Get.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Version.Get.Response.Error](#anytype.Rpc.Version.Get.Response.Error) | optional |  |
| version | [string](#string) | optional |  |
| details | [string](#string) | optional |  |


<a name="anytype.Rpc.Version.Get.Response.Error"/>
### Rpc.Version.Get.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet"/>
### Rpc.Wallet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Wallet.Convert"/>
### Rpc.Wallet.Convert


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Wallet.Convert.Request"/>
### Rpc.Wallet.Convert.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| mnemonic | [string](#string) | optional |  |
| entropy | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Convert.Response"/>
### Rpc.Wallet.Convert.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Convert.Response.Error](#anytype.Rpc.Wallet.Convert.Response.Error) | optional |  |
| entropy | [string](#string) | optional |  |
| mnemonic | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Convert.Response.Error"/>
### Rpc.Wallet.Convert.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Convert.Response.Error.Code](#anytype.Rpc.Wallet.Convert.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Create"/>
### Rpc.Wallet.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Wallet.Create.Request"/>
### Rpc.Wallet.Create.Request
Front-end-to-middleware request to create a new wallet

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Create.Response"/>
### Rpc.Wallet.Create.Response
Middleware-to-front-end response, that can contain mnemonic of a created account and a NULL error or an empty mnemonic and a non-NULL error

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Create.Response.Error](#anytype.Rpc.Wallet.Create.Response.Error) | optional |  |
| mnemonic | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Create.Response.Error"/>
### Rpc.Wallet.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Recover"/>
### Rpc.Wallet.Recover


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Wallet.Recover.Request"/>
### Rpc.Wallet.Recover.Request
Front end to middleware request-to-recover-a wallet with this mnemonic and a rootPath

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootPath | [string](#string) | optional |  |
| mnemonic | [string](#string) | optional |  |


<a name="anytype.Rpc.Wallet.Recover.Response"/>
### Rpc.Wallet.Recover.Response
Middleware-to-front-end response, that can contain a NULL error or a non-NULL error

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Wallet.Recover.Response.Error](#anytype.Rpc.Wallet.Recover.Response.Error) | optional |  |


<a name="anytype.Rpc.Wallet.Recover.Response.Error"/>
### Rpc.Wallet.Recover.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace"/>
### Rpc.Workspace


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.Create"/>
### Rpc.Workspace.Create


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.Create.Request"/>
### Rpc.Workspace.Create.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.Create.Response"/>
### Rpc.Workspace.Create.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Create.Response.Error](#anytype.Rpc.Workspace.Create.Response.Error) | optional |  |
| workspaceId | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.Create.Response.Error"/>
### Rpc.Workspace.Create.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Create.Response.Error.Code](#anytype.Rpc.Workspace.Create.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.GetAll"/>
### Rpc.Workspace.GetAll


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.GetAll.Request"/>
### Rpc.Workspace.GetAll.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.GetAll.Response"/>
### Rpc.Workspace.GetAll.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.GetAll.Response.Error](#anytype.Rpc.Workspace.GetAll.Response.Error) | optional |  |
| workspaceIds | [string](#string) | repeated |  |


<a name="anytype.Rpc.Workspace.GetAll.Response.Error"/>
### Rpc.Workspace.GetAll.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.GetAll.Response.Error.Code](#anytype.Rpc.Workspace.GetAll.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.GetCurrent"/>
### Rpc.Workspace.GetCurrent


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.GetCurrent.Request"/>
### Rpc.Workspace.GetCurrent.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.GetCurrent.Response"/>
### Rpc.Workspace.GetCurrent.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.GetCurrent.Response.Error](#anytype.Rpc.Workspace.GetCurrent.Response.Error) | optional |  |
| workspaceId | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.GetCurrent.Response.Error"/>
### Rpc.Workspace.GetCurrent.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.GetCurrent.Response.Error.Code](#anytype.Rpc.Workspace.GetCurrent.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.Select"/>
### Rpc.Workspace.Select


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.Select.Request"/>
### Rpc.Workspace.Select.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| workspaceId | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.Select.Response"/>
### Rpc.Workspace.Select.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.Select.Response.Error](#anytype.Rpc.Workspace.Select.Response.Error) | optional |  |


<a name="anytype.Rpc.Workspace.Select.Response.Error"/>
### Rpc.Workspace.Select.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.Select.Response.Error.Code](#anytype.Rpc.Workspace.Select.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |


<a name="anytype.Rpc.Workspace.SetIsHighlighted"/>
### Rpc.Workspace.SetIsHighlighted


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Rpc.Workspace.SetIsHighlighted.Request"/>
### Rpc.Workspace.SetIsHighlighted.Request


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) | optional |  |
| isHighlighted | [bool](#bool) | optional |  |


<a name="anytype.Rpc.Workspace.SetIsHighlighted.Response"/>
### Rpc.Workspace.SetIsHighlighted.Response


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Workspace.SetIsHighlighted.Response.Error](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error) | optional |  |


<a name="anytype.Rpc.Workspace.SetIsHighlighted.Response.Error"/>
### Rpc.Workspace.SetIsHighlighted.Response.Error


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Workspace.SetIsHighlighted.Response.Error.Code](#anytype.Rpc.Workspace.SetIsHighlighted.Response.Error.Code) | optional |  |
| description | [string](#string) | optional |  |



<a name="anytype.Rpc.Account.Create.Response.Error.Code"/>
### Rpc.Account.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE | 101 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME | 102 |  |
| ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR | 103 |  |
| FAILED_TO_STOP_RUNNING_NODE | 104 |  |
| BAD_INVITE_CODE | 900 |  |

<a name="anytype.Rpc.Account.Recover.Response.Error.Code"/>
### Rpc.Account.Recover.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NO_ACCOUNTS_FOUND | 101 |  |
| NEED_TO_RECOVER_WALLET_FIRST | 102 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 103 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 104 |  |
| FAILED_TO_RUN_NODE | 105 |  |
| WALLET_RECOVER_NOT_PERFORMED | 106 |  |
| FAILED_TO_STOP_RUNNING_NODE | 107 |  |
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |

<a name="anytype.Rpc.Account.Select.Response.Error.Code"/>
### Rpc.Account.Select.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |
| LOCAL_REPO_EXISTS_BUT_CORRUPTED | 102 |  |
| FAILED_TO_RUN_NODE | 103 |  |
| FAILED_TO_FIND_ACCOUNT_INFO | 104 |  |
| LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET | 105 |  |
| FAILED_TO_STOP_SEARCHER_NODE | 106 |  |
| FAILED_TO_RECOVER_PREDEFINED_BLOCKS | 107 |  |
| ANOTHER_ANYTYPE_PROCESS_IS_RUNNING | 108 |  |

<a name="anytype.Rpc.Account.Stop.Response.Error.Code"/>
### Rpc.Account.Stop.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| ACCOUNT_IS_NOT_RUNNING | 101 |  |
| FAILED_TO_STOP_NODE | 102 |  |
| FAILED_TO_REMOVE_ACCOUNT_DATA | 103 |  |

<a name="anytype.Rpc.ApplyTemplate.Response.Error.Code"/>
### Rpc.ApplyTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code"/>
### Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code"/>
### Rpc.Block.Bookmark.Fetch.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Close.Response.Error.Code"/>
### Rpc.Block.Close.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Copy.Response.Error.Code"/>
### Rpc.Block.Copy.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Create.Response.Error.Code"/>
### Rpc.Block.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.CreatePage.Response.Error.Code"/>
### Rpc.Block.CreatePage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.CreateSet.Response.Error.Code"/>
### Rpc.Block.CreateSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |

<a name="anytype.Rpc.Block.Cut.Response.Error.Code"/>
### Rpc.Block.Cut.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RecordCreate.Response.Error.Code"/>
### Rpc.Block.Dataview.RecordCreate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RecordDelete.Response.Error.Code"/>
### Rpc.Block.Dataview.RecordDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code"/>
### Rpc.Block.Dataview.RecordRelationOptionAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code"/>
### Rpc.Block.Dataview.RecordRelationOptionDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code"/>
### Rpc.Block.Dataview.RecordRelationOptionUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RecordUpdate.Response.Error.Code"/>
### Rpc.Block.Dataview.RecordUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RelationAdd.Response.Error.Code"/>
### Rpc.Block.Dataview.RelationAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RelationDelete.Response.Error.Code"/>
### Rpc.Block.Dataview.RelationDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code"/>
### Rpc.Block.Dataview.RelationListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_A_DATAVIEW_BLOCK | 3 |  |

<a name="anytype.Rpc.Block.Dataview.RelationUpdate.Response.Error.Code"/>
### Rpc.Block.Dataview.RelationUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.SetSource.Response.Error.Code"/>
### Rpc.Block.Dataview.SetSource.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.ViewCreate.Response.Error.Code"/>
### Rpc.Block.Dataview.ViewCreate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.ViewDelete.Response.Error.Code"/>
### Rpc.Block.Dataview.ViewDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.ViewSetActive.Response.Error.Code"/>
### Rpc.Block.Dataview.ViewSetActive.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code"/>
### Rpc.Block.Dataview.ViewSetPosition.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Dataview.ViewUpdate.Response.Error.Code"/>
### Rpc.Block.Dataview.ViewUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Download.Response.Error.Code"/>
### Rpc.Block.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Export.Response.Error.Code"/>
### Rpc.Block.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code"/>
### Rpc.Block.File.CreateAndUpload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Get.Marks.Response.Error.Code"/>
### Rpc.Block.Get.Marks.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.GetPublicWebURL.Response.Error.Code"/>
### Rpc.Block.GetPublicWebURL.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.ImportMarkdown.Response.Error.Code"/>
### Rpc.Block.ImportMarkdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Merge.Response.Error.Code"/>
### Rpc.Block.Merge.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.ObjectType.Set.Response.Error.Code"/>
### Rpc.Block.ObjectType.Set.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |

<a name="anytype.Rpc.Block.Open.Response.Error.Code"/>
### Rpc.Block.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 |  |

<a name="anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code"/>
### Rpc.Block.OpenBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Paste.Response.Error.Code"/>
### Rpc.Block.Paste.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Redo.Response.Error.Code"/>
### Rpc.Block.Redo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 |  |

<a name="anytype.Rpc.Block.Relation.Add.Response.Error.Code"/>
### Rpc.Block.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Relation.SetKey.Response.Error.Code"/>
### Rpc.Block.Relation.SetKey.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Replace.Response.Error.Code"/>
### Rpc.Block.Replace.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Details.Response.Error.Code"/>
### Rpc.Block.Set.Details.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Fields.Response.Error.Code"/>
### Rpc.Block.Set.Fields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.File.Name.Response.Error.Code"/>
### Rpc.Block.Set.File.Name.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Image.Name.Response.Error.Code"/>
### Rpc.Block.Set.Image.Name.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Image.Width.Response.Error.Code"/>
### Rpc.Block.Set.Image.Width.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Latex.Text.Response.Error.Code"/>
### Rpc.Block.Set.Latex.Text.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code"/>
### Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code"/>
### Rpc.Block.Set.Page.IsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Restrictions.Response.Error.Code"/>
### Rpc.Block.Set.Restrictions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code"/>
### Rpc.Block.Set.Text.Checked.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Text.Color.Response.Error.Code"/>
### Rpc.Block.Set.Text.Color.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Text.Style.Response.Error.Code"/>
### Rpc.Block.Set.Text.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Text.Text.Response.Error.Code"/>
### Rpc.Block.Set.Text.Text.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Video.Name.Response.Error.Code"/>
### Rpc.Block.Set.Video.Name.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Set.Video.Width.Response.Error.Code"/>
### Rpc.Block.Set.Video.Width.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.SetBreadcrumbs.Response.Error.Code"/>
### Rpc.Block.SetBreadcrumbs.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Show.Response.Error.Code"/>
### Rpc.Block.Show.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |
| ANYTYPE_NEEDS_UPGRADE | 10 |  |

<a name="anytype.Rpc.Block.Split.Request.Mode"/>
### Rpc.Block.Split.Request.Mode


| Name | Number | Description |
| ---- | ------ | ----------- |
| BOTTOM | 0 |  |
| TOP | 1 |  |
| INNER | 2 |  |
| TITLE | 3 |  |

<a name="anytype.Rpc.Block.Split.Response.Error.Code"/>
### Rpc.Block.Split.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Undo.Response.Error.Code"/>
### Rpc.Block.Undo.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| CAN_NOT_MOVE | 3 |  |

<a name="anytype.Rpc.Block.Unlink.Response.Error.Code"/>
### Rpc.Block.Unlink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.UpdateContent.Response.Error.Code"/>
### Rpc.Block.UpdateContent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Block.Upload.Response.Error.Code"/>
### Rpc.Block.Upload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code"/>
### Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Duplicate.Response.Error.Code"/>
### Rpc.BlockList.Duplicate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Move.Response.Error.Code"/>
### Rpc.BlockList.Move.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code"/>
### Rpc.BlockList.MoveToNewPage.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.Align.Response.Error.Code"/>
### Rpc.BlockList.Set.Align.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code"/>
### Rpc.BlockList.Set.BackgroundColor.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code"/>
### Rpc.BlockList.Set.Div.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.Fields.Response.Error.Code"/>
### Rpc.BlockList.Set.Fields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.File.Style.Response.Error.Code"/>
### Rpc.BlockList.Set.File.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code"/>
### Rpc.BlockList.Set.Text.Color.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.Text.Mark.Response.Error.Code"/>
### Rpc.BlockList.Set.Text.Mark.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code"/>
### Rpc.BlockList.Set.Text.Style.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.BlockList.TurnInto.Response.Error.Code"/>
### Rpc.BlockList.TurnInto.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.CloneTemplate.Response.Error.Code"/>
### Rpc.CloneTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Config.Get.Response.Error.Code"/>
### Rpc.Config.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |

<a name="anytype.Rpc.Debug.Sync.Response.Error.Code"/>
### Rpc.Debug.Sync.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Debug.Thread.Response.Error.Code"/>
### Rpc.Debug.Thread.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Debug.Tree.Response.Error.Code"/>
### Rpc.Debug.Tree.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.DownloadFile.Response.Error.Code"/>
### Rpc.DownloadFile.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 3 |  |

<a name="anytype.Rpc.Export.Format"/>
### Rpc.Export.Format


| Name | Number | Description |
| ---- | ------ | ----------- |
| Markdown | 0 |  |
| Protobuf | 1 |  |
| JSON | 2 |  |
| DOT | 3 |  |
| SVG | 4 |  |
| GRAPH_JSON | 5 |  |

<a name="anytype.Rpc.Export.Response.Error.Code"/>
### Rpc.Export.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ExportLocalstore.Response.Error.Code"/>
### Rpc.ExportLocalstore.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ExportTemplates.Response.Error.Code"/>
### Rpc.ExportTemplates.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ExternalDrop.Content.Response.Error.Code"/>
### Rpc.ExternalDrop.Content.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ExternalDrop.Files.Response.Error.Code"/>
### Rpc.ExternalDrop.Files.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.File.Offload.Response.Error.Code"/>
### Rpc.File.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 |  |
| FILE_NOT_YET_PINNED | 104 |  |

<a name="anytype.Rpc.FileList.Offload.Response.Error.Code"/>
### Rpc.FileList.Offload.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 103 |  |

<a name="anytype.Rpc.GenericErrorResponse.Error.Code"/>
### Rpc.GenericErrorResponse.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.History.SetVersion.Response.Error.Code"/>
### Rpc.History.SetVersion.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.History.Show.Response.Error.Code"/>
### Rpc.History.Show.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.History.Versions.Response.Error.Code"/>
### Rpc.History.Versions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.LinkPreview.Response.Error.Code"/>
### Rpc.LinkPreview.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Log.Send.Request.Level"/>
### Rpc.Log.Send.Request.Level


| Name | Number | Description |
| ---- | ------ | ----------- |
| DEBUG | 0 |  |
| ERROR | 1 |  |
| FATAL | 2 |  |
| INFO | 3 |  |
| PANIC | 4 |  |
| WARNING | 5 |  |

<a name="anytype.Rpc.Log.Send.Response.Error.Code"/>
### Rpc.Log.Send.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |

<a name="anytype.Rpc.MakeTemplate.Response.Error.Code"/>
### Rpc.MakeTemplate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.MakeTemplateByObjectType.Response.Error.Code"/>
### Rpc.MakeTemplateByObjectType.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Navigation.Context"/>
### Rpc.Navigation.Context


| Name | Number | Description |
| ---- | ------ | ----------- |
| Navigation | 0 |  |
| MoveTo | 1 |  |
| LinkTo | 2 |  |

<a name="anytype.Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code"/>
### Rpc.Navigation.GetObjectInfoWithLinks.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Navigation.ListObjects.Response.Error.Code"/>
### Rpc.Navigation.ListObjects.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.AddWithObjectId.Response.Error.Code"/>
### Rpc.Object.AddWithObjectId.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.FeaturedRelation.Add.Response.Error.Code"/>
### Rpc.Object.FeaturedRelation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.FeaturedRelation.Remove.Response.Error.Code"/>
### Rpc.Object.FeaturedRelation.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.Graph.Edge.Type"/>
### Rpc.Object.Graph.Edge.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Link | 0 |  |
| Relation | 1 |  |

<a name="anytype.Rpc.Object.Graph.Response.Error.Code"/>
### Rpc.Object.Graph.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.RelationAdd.Response.Error.Code"/>
### Rpc.Object.RelationAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.RelationDelete.Response.Error.Code"/>
### Rpc.Object.RelationDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.RelationListAvailable.Response.Error.Code"/>
### Rpc.Object.RelationListAvailable.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.RelationOptionAdd.Response.Error.Code"/>
### Rpc.Object.RelationOptionAdd.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.RelationOptionDelete.Response.Error.Code"/>
### Rpc.Object.RelationOptionDelete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| SOME_RECORDS_HAS_RELATION_VALUE_WITH_THIS_OPTION | 3 |  |

<a name="anytype.Rpc.Object.RelationOptionUpdate.Response.Error.Code"/>
### Rpc.Object.RelationOptionUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.RelationUpdate.Response.Error.Code"/>
### Rpc.Object.RelationUpdate.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.Search.Response.Error.Code"/>
### Rpc.Object.Search.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.SetIsArchived.Response.Error.Code"/>
### Rpc.Object.SetIsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.SetIsFavorite.Response.Error.Code"/>
### Rpc.Object.SetIsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.SetLayout.Response.Error.Code"/>
### Rpc.Object.SetLayout.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.ShareByLink.Response.Error.Code"/>
### Rpc.Object.ShareByLink.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Object.ToSet.Response.Error.Code"/>
### Rpc.Object.ToSet.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ObjectList.Delete.Response.Error.Code"/>
### Rpc.ObjectList.Delete.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ObjectList.Set.IsArchived.Response.Error.Code"/>
### Rpc.ObjectList.Set.IsArchived.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ObjectList.Set.IsFavorite.Response.Error.Code"/>
### Rpc.ObjectList.Set.IsFavorite.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ObjectType.Create.Response.Error.Code"/>
### Rpc.ObjectType.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |

<a name="anytype.Rpc.ObjectType.List.Response.Error.Code"/>
### Rpc.ObjectType.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.ObjectType.Relation.Add.Response.Error.Code"/>
### Rpc.ObjectType.Relation.Add.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 |  |

<a name="anytype.Rpc.ObjectType.Relation.List.Response.Error.Code"/>
### Rpc.ObjectType.Relation.List.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |

<a name="anytype.Rpc.ObjectType.Relation.Remove.Response.Error.Code"/>
### Rpc.ObjectType.Relation.Remove.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 |  |

<a name="anytype.Rpc.ObjectType.Relation.Update.Response.Error.Code"/>
### Rpc.ObjectType.Relation.Update.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |
| READONLY_OBJECT_TYPE | 4 |  |

<a name="anytype.Rpc.Page.Create.Response.Error.Code"/>
### Rpc.Page.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Ping.Response.Error.Code"/>
### Rpc.Ping.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Process.Cancel.Response.Error.Code"/>
### Rpc.Process.Cancel.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Set.Create.Response.Error.Code"/>
### Rpc.Set.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| UNKNOWN_OBJECT_TYPE_URL | 3 |  |

<a name="anytype.Rpc.Shutdown.Response.Error.Code"/>
### Rpc.Shutdown.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |

<a name="anytype.Rpc.UploadFile.Response.Error.Code"/>
### Rpc.UploadFile.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Version.Get.Response.Error.Code"/>
### Rpc.Version.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| VERSION_IS_EMPTY | 3 |  |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |

<a name="anytype.Rpc.Wallet.Convert.Response.Error.Code"/>
### Rpc.Wallet.Convert.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Wallet.Create.Response.Error.Code"/>
### Rpc.Wallet.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |

<a name="anytype.Rpc.Wallet.Recover.Response.Error.Code"/>
### Rpc.Wallet.Recover.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| FAILED_TO_CREATE_LOCAL_REPO | 101 |  |

<a name="anytype.Rpc.Workspace.Create.Response.Error.Code"/>
### Rpc.Workspace.Create.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Workspace.GetAll.Response.Error.Code"/>
### Rpc.Workspace.GetAll.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Workspace.GetCurrent.Response.Error.Code"/>
### Rpc.Workspace.GetCurrent.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Workspace.Select.Response.Error.Code"/>
### Rpc.Workspace.Select.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |

<a name="anytype.Rpc.Workspace.SetIsHighlighted.Response.Error.Code"/>
### Rpc.Workspace.SetIsHighlighted.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |




<a name="events.proto"/>
<p align="right"><a href="#top">Top</a></p>

## events.proto



<a name="anytype.Event"/>
### Event


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype.Event.Message) | repeated |  |
| contextId | [string](#string) | optional |  |
| initiator | [Account](#anytype.model.Account) | optional |  |
| traceId | [string](#string) | optional |  |


<a name="anytype.Event.Account"/>
### Event.Account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Account.Config"/>
### Event.Account.Config


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Account.Config.Update"/>
### Event.Account.Config.Update


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [Account.Config](#anytype.model.Account.Config) | optional |  |


<a name="anytype.Event.Account.Details"/>
### Event.Account.Details


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| profileId | [string](#string) | optional |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Account.Show"/>
### Event.Account.Show
Message, that will be sent to the front on each account found after an AccountRecoverRequest

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) | optional |  |
| account | [Account](#anytype.model.Account) | optional |  |


<a name="anytype.Event.Block"/>
### Event.Block


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Block.Add"/>
### Event.Block.Add


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [Block](#anytype.model.Block) | repeated |  |


<a name="anytype.Event.Block.Dataview"/>
### Event.Block.Dataview


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Block.Dataview.RecordsDelete"/>
### Event.Block.Dataview.RecordsDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| removed | [string](#string) | repeated |  |


<a name="anytype.Event.Block.Dataview.RecordsInsert"/>
### Event.Block.Dataview.RecordsInsert


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| records | [Struct](#google.protobuf.Struct) | repeated |  |
| insertPosition | [uint32](#uint32) | optional |  |


<a name="anytype.Event.Block.Dataview.RecordsSet"/>
### Event.Block.Dataview.RecordsSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| records | [Struct](#google.protobuf.Struct) | repeated |  |
| total | [uint32](#uint32) | optional |  |


<a name="anytype.Event.Block.Dataview.RecordsUpdate"/>
### Event.Block.Dataview.RecordsUpdate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| records | [Struct](#google.protobuf.Struct) | repeated |  |


<a name="anytype.Event.Block.Dataview.RelationDelete"/>
### Event.Block.Dataview.RelationDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |


<a name="anytype.Event.Block.Dataview.RelationSet"/>
### Event.Block.Dataview.RelationSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Event.Block.Dataview.SourceSet"/>
### Event.Block.Dataview.SourceSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| source | [string](#string) | repeated |  |


<a name="anytype.Event.Block.Dataview.ViewDelete"/>
### Event.Block.Dataview.ViewDelete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |


<a name="anytype.Event.Block.Dataview.ViewOrder"/>
### Event.Block.Dataview.ViewOrder


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewIds | [string](#string) | repeated |  |


<a name="anytype.Event.Block.Dataview.ViewSet"/>
### Event.Block.Dataview.ViewSet


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| viewId | [string](#string) | optional |  |
| view | [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) | optional |  |
| offset | [uint32](#uint32) | optional |  |
| limit | [uint32](#uint32) | optional |  |


<a name="anytype.Event.Block.Delete"/>
### Event.Block.Delete


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockIds | [string](#string) | repeated |  |


<a name="anytype.Event.Block.FilesUpload"/>
### Event.Block.FilesUpload
Middleware to front end event message, that will be sent on one of this scenarios:
Precondition: user A opened a block
1. User A drops a set of files/pictures/videos
2. User A creates a MediaBlock and drops a single media, that corresponds to its type.

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) | optional |  |
| filePath | [string](#string) | repeated |  |


<a name="anytype.Event.Block.Fill"/>
### Event.Block.Fill


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Block.Fill.Align"/>
### Event.Block.Fill.Align


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| align | [Block.Align](#anytype.model.Block.Align) | optional |  |


<a name="anytype.Event.Block.Fill.BackgroundColor"/>
### Event.Block.Fill.BackgroundColor


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| backgroundColor | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark"/>
### Event.Block.Fill.Bookmark


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| url | [Event.Block.Fill.Bookmark.Url](#anytype.Event.Block.Fill.Bookmark.Url) | optional |  |
| title | [Event.Block.Fill.Bookmark.Title](#anytype.Event.Block.Fill.Bookmark.Title) | optional |  |
| description | [Event.Block.Fill.Bookmark.Description](#anytype.Event.Block.Fill.Bookmark.Description) | optional |  |
| imageHash | [Event.Block.Fill.Bookmark.ImageHash](#anytype.Event.Block.Fill.Bookmark.ImageHash) | optional |  |
| faviconHash | [Event.Block.Fill.Bookmark.FaviconHash](#anytype.Event.Block.Fill.Bookmark.FaviconHash) | optional |  |
| type | [Event.Block.Fill.Bookmark.Type](#anytype.Event.Block.Fill.Bookmark.Type) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark.Description"/>
### Event.Block.Fill.Bookmark.Description


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark.FaviconHash"/>
### Event.Block.Fill.Bookmark.FaviconHash


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark.ImageHash"/>
### Event.Block.Fill.Bookmark.ImageHash


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark.Title"/>
### Event.Block.Fill.Bookmark.Title


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark.Type"/>
### Event.Block.Fill.Bookmark.Type


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [LinkPreview.Type](#anytype.model.LinkPreview.Type) | optional |  |


<a name="anytype.Event.Block.Fill.Bookmark.Url"/>
### Event.Block.Fill.Bookmark.Url


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.ChildrenIds"/>
### Event.Block.Fill.ChildrenIds


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| childrenIds | [string](#string) | repeated |  |


<a name="anytype.Event.Block.Fill.DatabaseRecords"/>
### Event.Block.Fill.DatabaseRecords


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| records | [Struct](#google.protobuf.Struct) | repeated |  |


<a name="anytype.Event.Block.Fill.Details"/>
### Event.Block.Fill.Details


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Block.Fill.Div"/>
### Event.Block.Fill.Div


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| style | [Event.Block.Fill.Div.Style](#anytype.Event.Block.Fill.Div.Style) | optional |  |


<a name="anytype.Event.Block.Fill.Div.Style"/>
### Event.Block.Fill.Div.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) | optional |  |


<a name="anytype.Event.Block.Fill.Fields"/>
### Event.Block.Fill.Fields


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Block.Fill.File"/>
### Event.Block.Fill.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| type | [Event.Block.Fill.File.Type](#anytype.Event.Block.Fill.File.Type) | optional |  |
| state | [Event.Block.Fill.File.State](#anytype.Event.Block.Fill.File.State) | optional |  |
| mime | [Event.Block.Fill.File.Mime](#anytype.Event.Block.Fill.File.Mime) | optional |  |
| hash | [Event.Block.Fill.File.Hash](#anytype.Event.Block.Fill.File.Hash) | optional |  |
| name | [Event.Block.Fill.File.Name](#anytype.Event.Block.Fill.File.Name) | optional |  |
| size | [Event.Block.Fill.File.Size](#anytype.Event.Block.Fill.File.Size) | optional |  |
| style | [Event.Block.Fill.File.Style](#anytype.Event.Block.Fill.File.Style) | optional |  |


<a name="anytype.Event.Block.Fill.File.Hash"/>
### Event.Block.Fill.File.Hash


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.File.Mime"/>
### Event.Block.Fill.File.Mime


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.File.Name"/>
### Event.Block.Fill.File.Name


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.File.Size"/>
### Event.Block.Fill.File.Size


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int64](#int64) | optional |  |


<a name="anytype.Event.Block.Fill.File.State"/>
### Event.Block.Fill.File.State


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.File.State](#anytype.model.Block.Content.File.State) | optional |  |


<a name="anytype.Event.Block.Fill.File.Style"/>
### Event.Block.Fill.File.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.File.Style](#anytype.model.Block.Content.File.Style) | optional |  |


<a name="anytype.Event.Block.Fill.File.Type"/>
### Event.Block.Fill.File.Type


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) | optional |  |


<a name="anytype.Event.Block.Fill.File.Width"/>
### Event.Block.Fill.File.Width


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) | optional |  |


<a name="anytype.Event.Block.Fill.Link"/>
### Event.Block.Fill.Link


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| targetBlockId | [Event.Block.Fill.Link.TargetBlockId](#anytype.Event.Block.Fill.Link.TargetBlockId) | optional |  |
| style | [Event.Block.Fill.Link.Style](#anytype.Event.Block.Fill.Link.Style) | optional |  |
| fields | [Event.Block.Fill.Link.Fields](#anytype.Event.Block.Fill.Link.Fields) | optional |  |


<a name="anytype.Event.Block.Fill.Link.Fields"/>
### Event.Block.Fill.Link.Fields


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Block.Fill.Link.Style"/>
### Event.Block.Fill.Link.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) | optional |  |


<a name="anytype.Event.Block.Fill.Link.TargetBlockId"/>
### Event.Block.Fill.Link.TargetBlockId


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Restrictions"/>
### Event.Block.Fill.Restrictions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| restrictions | [Block.Restrictions](#anytype.model.Block.Restrictions) | optional |  |


<a name="anytype.Event.Block.Fill.Text"/>
### Event.Block.Fill.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| text | [Event.Block.Fill.Text.Text](#anytype.Event.Block.Fill.Text.Text) | optional |  |
| style | [Event.Block.Fill.Text.Style](#anytype.Event.Block.Fill.Text.Style) | optional |  |
| marks | [Event.Block.Fill.Text.Marks](#anytype.Event.Block.Fill.Text.Marks) | optional |  |
| checked | [Event.Block.Fill.Text.Checked](#anytype.Event.Block.Fill.Text.Checked) | optional |  |
| color | [Event.Block.Fill.Text.Color](#anytype.Event.Block.Fill.Text.Color) | optional |  |


<a name="anytype.Event.Block.Fill.Text.Checked"/>
### Event.Block.Fill.Text.Checked


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) | optional |  |


<a name="anytype.Event.Block.Fill.Text.Color"/>
### Event.Block.Fill.Text.Color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Fill.Text.Marks"/>
### Event.Block.Fill.Text.Marks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) | optional |  |


<a name="anytype.Event.Block.Fill.Text.Style"/>
### Event.Block.Fill.Text.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |


<a name="anytype.Event.Block.Fill.Text.Text"/>
### Event.Block.Fill.Text.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.MarksInfo"/>
### Event.Block.MarksInfo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marksInRange | [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type) | repeated |  |


<a name="anytype.Event.Block.Set"/>
### Event.Block.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Block.Set.Align"/>
### Event.Block.Set.Align


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| align | [Block.Align](#anytype.model.Block.Align) | optional |  |


<a name="anytype.Event.Block.Set.BackgroundColor"/>
### Event.Block.Set.BackgroundColor


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| backgroundColor | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark"/>
### Event.Block.Set.Bookmark


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| url | [Event.Block.Set.Bookmark.Url](#anytype.Event.Block.Set.Bookmark.Url) | optional |  |
| title | [Event.Block.Set.Bookmark.Title](#anytype.Event.Block.Set.Bookmark.Title) | optional |  |
| description | [Event.Block.Set.Bookmark.Description](#anytype.Event.Block.Set.Bookmark.Description) | optional |  |
| imageHash | [Event.Block.Set.Bookmark.ImageHash](#anytype.Event.Block.Set.Bookmark.ImageHash) | optional |  |
| faviconHash | [Event.Block.Set.Bookmark.FaviconHash](#anytype.Event.Block.Set.Bookmark.FaviconHash) | optional |  |
| type | [Event.Block.Set.Bookmark.Type](#anytype.Event.Block.Set.Bookmark.Type) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark.Description"/>
### Event.Block.Set.Bookmark.Description


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark.FaviconHash"/>
### Event.Block.Set.Bookmark.FaviconHash


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark.ImageHash"/>
### Event.Block.Set.Bookmark.ImageHash


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark.Title"/>
### Event.Block.Set.Bookmark.Title


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark.Type"/>
### Event.Block.Set.Bookmark.Type


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [LinkPreview.Type](#anytype.model.LinkPreview.Type) | optional |  |


<a name="anytype.Event.Block.Set.Bookmark.Url"/>
### Event.Block.Set.Bookmark.Url


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.ChildrenIds"/>
### Event.Block.Set.ChildrenIds


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| childrenIds | [string](#string) | repeated |  |


<a name="anytype.Event.Block.Set.Div"/>
### Event.Block.Set.Div


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| style | [Event.Block.Set.Div.Style](#anytype.Event.Block.Set.Div.Style) | optional |  |


<a name="anytype.Event.Block.Set.Div.Style"/>
### Event.Block.Set.Div.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) | optional |  |


<a name="anytype.Event.Block.Set.Fields"/>
### Event.Block.Set.Fields


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Block.Set.File"/>
### Event.Block.Set.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| type | [Event.Block.Set.File.Type](#anytype.Event.Block.Set.File.Type) | optional |  |
| state | [Event.Block.Set.File.State](#anytype.Event.Block.Set.File.State) | optional |  |
| mime | [Event.Block.Set.File.Mime](#anytype.Event.Block.Set.File.Mime) | optional |  |
| hash | [Event.Block.Set.File.Hash](#anytype.Event.Block.Set.File.Hash) | optional |  |
| name | [Event.Block.Set.File.Name](#anytype.Event.Block.Set.File.Name) | optional |  |
| size | [Event.Block.Set.File.Size](#anytype.Event.Block.Set.File.Size) | optional |  |
| style | [Event.Block.Set.File.Style](#anytype.Event.Block.Set.File.Style) | optional |  |


<a name="anytype.Event.Block.Set.File.Hash"/>
### Event.Block.Set.File.Hash


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.File.Mime"/>
### Event.Block.Set.File.Mime


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.File.Name"/>
### Event.Block.Set.File.Name


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.File.Size"/>
### Event.Block.Set.File.Size


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int64](#int64) | optional |  |


<a name="anytype.Event.Block.Set.File.State"/>
### Event.Block.Set.File.State


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.File.State](#anytype.model.Block.Content.File.State) | optional |  |


<a name="anytype.Event.Block.Set.File.Style"/>
### Event.Block.Set.File.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.File.Style](#anytype.model.Block.Content.File.Style) | optional |  |


<a name="anytype.Event.Block.Set.File.Type"/>
### Event.Block.Set.File.Type


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) | optional |  |


<a name="anytype.Event.Block.Set.File.Width"/>
### Event.Block.Set.File.Width


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [int32](#int32) | optional |  |


<a name="anytype.Event.Block.Set.Latex"/>
### Event.Block.Set.Latex


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| text | [Event.Block.Set.Latex.Text](#anytype.Event.Block.Set.Latex.Text) | optional |  |


<a name="anytype.Event.Block.Set.Latex.Text"/>
### Event.Block.Set.Latex.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Link"/>
### Event.Block.Set.Link


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| targetBlockId | [Event.Block.Set.Link.TargetBlockId](#anytype.Event.Block.Set.Link.TargetBlockId) | optional |  |
| style | [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style) | optional |  |
| fields | [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields) | optional |  |


<a name="anytype.Event.Block.Set.Link.Fields"/>
### Event.Block.Set.Link.Fields


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Block.Set.Link.Style"/>
### Event.Block.Set.Link.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) | optional |  |


<a name="anytype.Event.Block.Set.Link.TargetBlockId"/>
### Event.Block.Set.Link.TargetBlockId


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Relation"/>
### Event.Block.Set.Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| key | [Event.Block.Set.Relation.Key](#anytype.Event.Block.Set.Relation.Key) | optional |  |


<a name="anytype.Event.Block.Set.Relation.Key"/>
### Event.Block.Set.Relation.Key


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Restrictions"/>
### Event.Block.Set.Restrictions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| restrictions | [Block.Restrictions](#anytype.model.Block.Restrictions) | optional |  |


<a name="anytype.Event.Block.Set.Text"/>
### Event.Block.Set.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| text | [Event.Block.Set.Text.Text](#anytype.Event.Block.Set.Text.Text) | optional |  |
| style | [Event.Block.Set.Text.Style](#anytype.Event.Block.Set.Text.Style) | optional |  |
| marks | [Event.Block.Set.Text.Marks](#anytype.Event.Block.Set.Text.Marks) | optional |  |
| checked | [Event.Block.Set.Text.Checked](#anytype.Event.Block.Set.Text.Checked) | optional |  |
| color | [Event.Block.Set.Text.Color](#anytype.Event.Block.Set.Text.Color) | optional |  |


<a name="anytype.Event.Block.Set.Text.Checked"/>
### Event.Block.Set.Text.Checked


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) | optional |  |


<a name="anytype.Event.Block.Set.Text.Color"/>
### Event.Block.Set.Text.Color


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Block.Set.Text.Marks"/>
### Event.Block.Set.Text.Marks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) | optional |  |


<a name="anytype.Event.Block.Set.Text.Style"/>
### Event.Block.Set.Text.Style


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |


<a name="anytype.Event.Block.Set.Text.Text"/>
### Event.Block.Set.Text.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) | optional |  |


<a name="anytype.Event.Message"/>
### Event.Message


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountShow | [Event.Account.Show](#anytype.Event.Account.Show) | optional |  |
| accountDetails | [Event.Account.Details](#anytype.Event.Account.Details) | optional |  |
| accountConfigUpdate | [Event.Account.Config.Update](#anytype.Event.Account.Config.Update) | optional |  |
| objectDetailsSet | [Event.Object.Details.Set](#anytype.Event.Object.Details.Set) | optional |  |
| objectDetailsAmend | [Event.Object.Details.Amend](#anytype.Event.Object.Details.Amend) | optional |  |
| objectDetailsUnset | [Event.Object.Details.Unset](#anytype.Event.Object.Details.Unset) | optional |  |
| objectRelationsSet | [Event.Object.Relations.Set](#anytype.Event.Object.Relations.Set) | optional |  |
| objectRelationsAmend | [Event.Object.Relations.Amend](#anytype.Event.Object.Relations.Amend) | optional |  |
| objectRelationsRemove | [Event.Object.Relations.Remove](#anytype.Event.Object.Relations.Remove) | optional |  |
| objectRemove | [Event.Object.Remove](#anytype.Event.Object.Remove) | optional |  |
| objectShow | [Event.Object.Show](#anytype.Event.Object.Show) | optional |  |
| blockAdd | [Event.Block.Add](#anytype.Event.Block.Add) | optional |  |
| blockDelete | [Event.Block.Delete](#anytype.Event.Block.Delete) | optional |  |
| filesUpload | [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload) | optional |  |
| marksInfo | [Event.Block.MarksInfo](#anytype.Event.Block.MarksInfo) | optional |  |
| blockSetFields | [Event.Block.Set.Fields](#anytype.Event.Block.Set.Fields) | optional |  |
| blockSetChildrenIds | [Event.Block.Set.ChildrenIds](#anytype.Event.Block.Set.ChildrenIds) | optional |  |
| blockSetRestrictions | [Event.Block.Set.Restrictions](#anytype.Event.Block.Set.Restrictions) | optional |  |
| blockSetBackgroundColor | [Event.Block.Set.BackgroundColor](#anytype.Event.Block.Set.BackgroundColor) | optional |  |
| blockSetText | [Event.Block.Set.Text](#anytype.Event.Block.Set.Text) | optional |  |
| blockSetFile | [Event.Block.Set.File](#anytype.Event.Block.Set.File) | optional |  |
| blockSetLink | [Event.Block.Set.Link](#anytype.Event.Block.Set.Link) | optional |  |
| blockSetBookmark | [Event.Block.Set.Bookmark](#anytype.Event.Block.Set.Bookmark) | optional |  |
| blockSetAlign | [Event.Block.Set.Align](#anytype.Event.Block.Set.Align) | optional |  |
| blockSetDiv | [Event.Block.Set.Div](#anytype.Event.Block.Set.Div) | optional |  |
| blockSetRelation | [Event.Block.Set.Relation](#anytype.Event.Block.Set.Relation) | optional |  |
| blockSetLatex | [Event.Block.Set.Latex](#anytype.Event.Block.Set.Latex) | optional |  |
| blockDataviewRecordsSet | [Event.Block.Dataview.RecordsSet](#anytype.Event.Block.Dataview.RecordsSet) | optional |  |
| blockDataviewRecordsUpdate | [Event.Block.Dataview.RecordsUpdate](#anytype.Event.Block.Dataview.RecordsUpdate) | optional |  |
| blockDataviewRecordsInsert | [Event.Block.Dataview.RecordsInsert](#anytype.Event.Block.Dataview.RecordsInsert) | optional |  |
| blockDataviewRecordsDelete | [Event.Block.Dataview.RecordsDelete](#anytype.Event.Block.Dataview.RecordsDelete) | optional |  |
| blockDataviewSourceSet | [Event.Block.Dataview.SourceSet](#anytype.Event.Block.Dataview.SourceSet) | optional |  |
| blockDataviewViewSet | [Event.Block.Dataview.ViewSet](#anytype.Event.Block.Dataview.ViewSet) | optional |  |
| blockDataviewViewDelete | [Event.Block.Dataview.ViewDelete](#anytype.Event.Block.Dataview.ViewDelete) | optional |  |
| blockDataviewViewOrder | [Event.Block.Dataview.ViewOrder](#anytype.Event.Block.Dataview.ViewOrder) | optional |  |
| blockDataviewRelationDelete | [Event.Block.Dataview.RelationDelete](#anytype.Event.Block.Dataview.RelationDelete) | optional |  |
| blockDataviewRelationSet | [Event.Block.Dataview.RelationSet](#anytype.Event.Block.Dataview.RelationSet) | optional |  |
| userBlockJoin | [Event.User.Block.Join](#anytype.Event.User.Block.Join) | optional |  |
| userBlockLeft | [Event.User.Block.Left](#anytype.Event.User.Block.Left) | optional |  |
| userBlockSelectRange | [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange) | optional |  |
| userBlockTextRange | [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange) | optional |  |
| ping | [Event.Ping](#anytype.Event.Ping) | optional |  |
| processNew | [Event.Process.New](#anytype.Event.Process.New) | optional |  |
| processUpdate | [Event.Process.Update](#anytype.Event.Process.Update) | optional |  |
| processDone | [Event.Process.Done](#anytype.Event.Process.Done) | optional |  |
| threadStatus | [Event.Status.Thread](#anytype.Event.Status.Thread) | optional |  |


<a name="anytype.Event.Object"/>
### Event.Object


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Object.Details"/>
### Event.Object.Details


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Object.Details.Amend"/>
### Event.Object.Details.Amend


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| details | [Event.Object.Details.Amend.KeyValue](#anytype.Event.Object.Details.Amend.KeyValue) | repeated |  |


<a name="anytype.Event.Object.Details.Amend.KeyValue"/>
### Event.Object.Details.Amend.KeyValue


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| value | [Value](#google.protobuf.Value) | optional |  |


<a name="anytype.Event.Object.Details.Set"/>
### Event.Object.Details.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.Event.Object.Details.Unset"/>
### Event.Object.Details.Unset


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| keys | [string](#string) | repeated |  |


<a name="anytype.Event.Object.Relation"/>
### Event.Object.Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Object.Relation.Remove"/>
### Event.Object.Relation.Remove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |


<a name="anytype.Event.Object.Relation.Set"/>
### Event.Object.Relation.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| relationKey | [string](#string) | optional |  |
| relation | [Relation](#anytype.model.Relation) | optional |  |


<a name="anytype.Event.Object.Relations"/>
### Event.Object.Relations


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Object.Relations.Amend"/>
### Event.Object.Relations.Amend


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Event.Object.Relations.Remove"/>
### Event.Object.Relations.Remove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| keys | [string](#string) | repeated |  |


<a name="anytype.Event.Object.Relations.Set"/>
### Event.Object.Relations.Set


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.Event.Object.Remove"/>
### Event.Object.Remove


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ids | [string](#string) | repeated |  |


<a name="anytype.Event.Object.Show"/>
### Event.Object.Show


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootId | [string](#string) | optional |  |
| blocks | [Block](#anytype.model.Block) | repeated |  |
| details | [Event.Object.Details.Set](#anytype.Event.Object.Details.Set) | repeated |  |
| type | [SmartBlockType](#anytype.model.SmartBlockType) | optional |  |
| objectTypes | [ObjectType](#anytype.model.ObjectType) | repeated |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |
| restrictions | [Restrictions](#anytype.model.Restrictions) | optional |  |


<a name="anytype.Event.Object.Show.RelationWithValuePerObject"/>
### Event.Object.Show.RelationWithValuePerObject


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| objectId | [string](#string) | optional |  |
| relations | [RelationWithValue](#anytype.model.RelationWithValue) | repeated |  |


<a name="anytype.Event.Ping"/>
### Event.Ping


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) | optional |  |


<a name="anytype.Event.Process"/>
### Event.Process


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Process.Done"/>
### Event.Process.Done


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype.Model.Process) | optional |  |


<a name="anytype.Event.Process.New"/>
### Event.Process.New


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype.Model.Process) | optional |  |


<a name="anytype.Event.Process.Update"/>
### Event.Process.Update


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| process | [Model.Process](#anytype.Model.Process) | optional |  |


<a name="anytype.Event.Status"/>
### Event.Status


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.Status.Thread"/>
### Event.Status.Thread


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| summary | [Event.Status.Thread.Summary](#anytype.Event.Status.Thread.Summary) | optional |  |
| cafe | [Event.Status.Thread.Cafe](#anytype.Event.Status.Thread.Cafe) | optional |  |
| accounts | [Event.Status.Thread.Account](#anytype.Event.Status.Thread.Account) | repeated |  |


<a name="anytype.Event.Status.Thread.Account"/>
### Event.Status.Thread.Account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| name | [string](#string) | optional |  |
| imageHash | [string](#string) | optional |  |
| online | [bool](#bool) | optional |  |
| lastPulled | [int64](#int64) | optional |  |
| lastEdited | [int64](#int64) | optional |  |
| devices | [Event.Status.Thread.Device](#anytype.Event.Status.Thread.Device) | repeated |  |


<a name="anytype.Event.Status.Thread.Cafe"/>
### Event.Status.Thread.Cafe


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Event.Status.Thread.SyncStatus](#anytype.Event.Status.Thread.SyncStatus) | optional |  |
| lastPulled | [int64](#int64) | optional |  |
| lastPushSucceed | [bool](#bool) | optional |  |
| files | [Event.Status.Thread.Cafe.PinStatus](#anytype.Event.Status.Thread.Cafe.PinStatus) | optional |  |


<a name="anytype.Event.Status.Thread.Cafe.PinStatus"/>
### Event.Status.Thread.Cafe.PinStatus


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| pinning | [int32](#int32) | optional |  |
| pinned | [int32](#int32) | optional |  |
| failed | [int32](#int32) | optional |  |
| updated | [int64](#int64) | optional |  |


<a name="anytype.Event.Status.Thread.Device"/>
### Event.Status.Thread.Device


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) | optional |  |
| online | [bool](#bool) | optional |  |
| lastPulled | [int64](#int64) | optional |  |
| lastEdited | [int64](#int64) | optional |  |


<a name="anytype.Event.Status.Thread.Summary"/>
### Event.Status.Thread.Summary


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| status | [Event.Status.Thread.SyncStatus](#anytype.Event.Status.Thread.SyncStatus) | optional |  |


<a name="anytype.Event.User"/>
### Event.User


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.User.Block"/>
### Event.User.Block


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Event.User.Block.Join"/>
### Event.User.Block.Join
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A opened a block
1. User B opens the same block
2. User A receives a message about p.1

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) | optional |  |


<a name="anytype.Event.User.Block.Left"/>
### Event.User.Block.Left
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B closes the block
2. User A receives a message about p.1

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) | optional |  |


<a name="anytype.Event.User.Block.SelectRange"/>
### Event.User.Block.SelectRange
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B selects some inner blocks
2. User A receives a message about p.1

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) | optional |  |
| blockIdsArray | [string](#string) | repeated |  |


<a name="anytype.Event.User.Block.TextRange"/>
### Event.User.Block.TextRange
Middleware to front end event message, that will be sent in this scenario:
Precondition: user A and user B opened the same block
1. User B sets cursor or selects a text region into a text block
2. User A receives a message about p.1

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| account | [Event.Account](#anytype.Event.Account) | optional |  |
| blockId | [string](#string) | optional |  |
| range | [Range](#anytype.model.Range) | optional |  |


<a name="anytype.Model"/>
### Model


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.Model.Process"/>
### Model.Process


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| type | [Model.Process.Type](#anytype.Model.Process.Type) | optional |  |
| state | [Model.Process.State](#anytype.Model.Process.State) | optional |  |
| progress | [Model.Process.Progress](#anytype.Model.Process.Progress) | optional |  |


<a name="anytype.Model.Process.Progress"/>
### Model.Process.Progress


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| total | [int64](#int64) | optional |  |
| done | [int64](#int64) | optional |  |
| message | [string](#string) | optional |  |


<a name="anytype.ResponseEvent"/>
### ResponseEvent


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype.Event.Message) | repeated |  |
| contextId | [string](#string) | optional |  |
| traceId | [string](#string) | optional |  |



<a name="anytype.Event.Status.Thread.SyncStatus"/>
### Event.Status.Thread.SyncStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Offline | 1 |  |
| Syncing | 2 |  |
| Synced | 3 |  |
| Failed | 4 |  |

<a name="anytype.Model.Process.State"/>
### Model.Process.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Running | 1 |  |
| Done | 2 |  |
| Canceled | 3 |  |
| Error | 4 |  |

<a name="anytype.Model.Process.Type"/>
### Model.Process.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| DropFiles | 0 |  |
| Import | 1 |  |
| Export | 2 |  |
| SaveFile | 3 |  |
| RecoverAccount | 4 |  |




<a name="localstore.proto"/>
<p align="right"><a href="#top">Top</a></p>

## localstore.proto



<a name="anytype.model.ObjectDetails"/>
### ObjectDetails


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| details | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.model.ObjectInfo"/>
### ObjectInfo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| objectTypeUrls | [string](#string) | repeated |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |
| snippet | [string](#string) | optional |  |
| hasInboundLinks | [bool](#bool) | optional |  |
| objectType | [SmartBlockType](#anytype.model.SmartBlockType) | optional |  |


<a name="anytype.model.ObjectInfoWithLinks"/>
### ObjectInfoWithLinks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| info | [ObjectInfo](#anytype.model.ObjectInfo) | optional |  |
| links | [ObjectLinksInfo](#anytype.model.ObjectLinksInfo) | optional |  |


<a name="anytype.model.ObjectInfoWithOutboundLinks"/>
### ObjectInfoWithOutboundLinks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| info | [ObjectInfo](#anytype.model.ObjectInfo) | optional |  |
| outboundLinks | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |


<a name="anytype.model.ObjectInfoWithOutboundLinksIDs"/>
### ObjectInfoWithOutboundLinksIDs


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| info | [ObjectInfo](#anytype.model.ObjectInfo) | optional |  |
| outboundLinks | [string](#string) | repeated |  |


<a name="anytype.model.ObjectLinks"/>
### ObjectLinks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inboundIDs | [string](#string) | repeated |  |
| outboundIDs | [string](#string) | repeated |  |


<a name="anytype.model.ObjectLinksInfo"/>
### ObjectLinksInfo


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inbound | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |
| outbound | [ObjectInfo](#anytype.model.ObjectInfo) | repeated |  |


<a name="anytype.model.ObjectStoreChecksums"/>
### ObjectStoreChecksums


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| bundledObjectTypes | [string](#string) | optional |  |
| bundledRelations | [string](#string) | optional |  |
| bundledLayouts | [string](#string) | optional |  |
| objectsForceReindexCounter | [int32](#int32) | optional |  |
| filesForceReindexCounter | [int32](#int32) | optional |  |
| idxRebuildCounter | [int32](#int32) | optional |  |
| fulltextRebuild | [int32](#int32) | optional |  |
| bundledTemplates | [string](#string) | optional |  |
| bundledObjects | [int32](#int32) | optional |  |






<a name="models.proto"/>
<p align="right"><a href="#top">Top</a></p>

## models.proto



<a name="anytype.model.Account"/>
### Account
Contains basic information about a user account

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| name | [string](#string) | optional |  |
| avatar | [Account.Avatar](#anytype.model.Account.Avatar) | optional |  |


<a name="anytype.model.Account.Avatar"/>
### Account.Avatar
Avatar of a user's account. It could be an image or color

| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| image | [Block.Content.File](#anytype.model.Block.Content.File) | optional |  |
| color | [string](#string) | optional |  |


<a name="anytype.model.Account.Config"/>
### Account.Config


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| enableDataview | [bool](#bool) | optional |  |
| enableDebug | [bool](#bool) | optional |  |
| enableReleaseChannelSwitch | [bool](#bool) | optional |  |
| enableSpaces | [bool](#bool) | optional |  |
| extra | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.model.Block"/>
### Block


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |
| restrictions | [Block.Restrictions](#anytype.model.Block.Restrictions) | optional |  |
| childrenIds | [string](#string) | repeated |  |
| backgroundColor | [string](#string) | optional |  |
| align | [Block.Align](#anytype.model.Block.Align) | optional |  |
| smartblock | [Block.Content.Smartblock](#anytype.model.Block.Content.Smartblock) | optional |  |
| text | [Block.Content.Text](#anytype.model.Block.Content.Text) | optional |  |
| file | [Block.Content.File](#anytype.model.Block.Content.File) | optional |  |
| layout | [Block.Content.Layout](#anytype.model.Block.Content.Layout) | optional |  |
| div | [Block.Content.Div](#anytype.model.Block.Content.Div) | optional |  |
| bookmark | [Block.Content.Bookmark](#anytype.model.Block.Content.Bookmark) | optional |  |
| icon | [Block.Content.Icon](#anytype.model.Block.Content.Icon) | optional |  |
| link | [Block.Content.Link](#anytype.model.Block.Content.Link) | optional |  |
| dataview | [Block.Content.Dataview](#anytype.model.Block.Content.Dataview) | optional |  |
| relation | [Block.Content.Relation](#anytype.model.Block.Content.Relation) | optional |  |
| featuredRelations | [Block.Content.FeaturedRelations](#anytype.model.Block.Content.FeaturedRelations) | optional |  |
| latex | [Block.Content.Latex](#anytype.model.Block.Content.Latex) | optional |  |


<a name="anytype.model.Block.Content"/>
### Block.Content


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.model.Block.Content.Bookmark"/>
### Block.Content.Bookmark


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |
| title | [string](#string) | optional |  |
| description | [string](#string) | optional |  |
| imageHash | [string](#string) | optional |  |
| faviconHash | [string](#string) | optional |  |
| type | [LinkPreview.Type](#anytype.model.LinkPreview.Type) | optional |  |


<a name="anytype.model.Block.Content.Dataview"/>
### Block.Content.Dataview


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| source | [string](#string) | repeated |  |
| views | [Block.Content.Dataview.View](#anytype.model.Block.Content.Dataview.View) | repeated |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |
| activeView | [string](#string) | optional |  |


<a name="anytype.model.Block.Content.Dataview.Filter"/>
### Block.Content.Dataview.Filter


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| operator | [Block.Content.Dataview.Filter.Operator](#anytype.model.Block.Content.Dataview.Filter.Operator) | optional |  |
| RelationKey | [string](#string) | optional |  |
| relationProperty | [string](#string) | optional |  |
| condition | [Block.Content.Dataview.Filter.Condition](#anytype.model.Block.Content.Dataview.Filter.Condition) | optional |  |
| value | [Value](#google.protobuf.Value) | optional |  |


<a name="anytype.model.Block.Content.Dataview.Relation"/>
### Block.Content.Dataview.Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| isVisible | [bool](#bool) | optional |  |
| width | [int32](#int32) | optional |  |
| dateIncludeTime | [bool](#bool) | optional |  |
| timeFormat | [Block.Content.Dataview.Relation.TimeFormat](#anytype.model.Block.Content.Dataview.Relation.TimeFormat) | optional |  |
| dateFormat | [Block.Content.Dataview.Relation.DateFormat](#anytype.model.Block.Content.Dataview.Relation.DateFormat) | optional |  |


<a name="anytype.model.Block.Content.Dataview.Sort"/>
### Block.Content.Dataview.Sort


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| RelationKey | [string](#string) | optional |  |
| type | [Block.Content.Dataview.Sort.Type](#anytype.model.Block.Content.Dataview.Sort.Type) | optional |  |


<a name="anytype.model.Block.Content.Dataview.View"/>
### Block.Content.Dataview.View


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| type | [Block.Content.Dataview.View.Type](#anytype.model.Block.Content.Dataview.View.Type) | optional |  |
| name | [string](#string) | optional |  |
| sorts | [Block.Content.Dataview.Sort](#anytype.model.Block.Content.Dataview.Sort) | repeated |  |
| filters | [Block.Content.Dataview.Filter](#anytype.model.Block.Content.Dataview.Filter) | repeated |  |
| relations | [Block.Content.Dataview.Relation](#anytype.model.Block.Content.Dataview.Relation) | repeated |  |
| coverRelationKey | [string](#string) | optional |  |
| hideIcon | [bool](#bool) | optional |  |
| cardSize | [Block.Content.Dataview.View.Size](#anytype.model.Block.Content.Dataview.View.Size) | optional |  |
| coverFit | [bool](#bool) | optional |  |


<a name="anytype.model.Block.Content.Div"/>
### Block.Content.Div


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) | optional |  |


<a name="anytype.model.Block.Content.FeaturedRelations"/>
### Block.Content.FeaturedRelations


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.model.Block.Content.File"/>
### Block.Content.File


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) | optional |  |
| name | [string](#string) | optional |  |
| type | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) | optional |  |
| mime | [string](#string) | optional |  |
| size | [int64](#int64) | optional |  |
| addedAt | [int64](#int64) | optional |  |
| state | [Block.Content.File.State](#anytype.model.Block.Content.File.State) | optional |  |
| style | [Block.Content.File.Style](#anytype.model.Block.Content.File.Style) | optional |  |


<a name="anytype.model.Block.Content.Icon"/>
### Block.Content.Icon


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) | optional |  |


<a name="anytype.model.Block.Content.Latex"/>
### Block.Content.Latex


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) | optional |  |


<a name="anytype.model.Block.Content.Layout"/>
### Block.Content.Layout


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Layout.Style](#anytype.model.Block.Content.Layout.Style) | optional |  |


<a name="anytype.model.Block.Content.Link"/>
### Block.Content.Link


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| targetBlockId | [string](#string) | optional |  |
| style | [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.model.Block.Content.Relation"/>
### Block.Content.Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |


<a name="anytype.model.Block.Content.Smartblock"/>
### Block.Content.Smartblock


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |


<a name="anytype.model.Block.Content.Text"/>
### Block.Content.Text


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) | optional |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) | optional |  |
| marks | [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) | optional |  |
| checked | [bool](#bool) | optional |  |
| color | [string](#string) | optional |  |


<a name="anytype.model.Block.Content.Text.Mark"/>
### Block.Content.Text.Mark


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Range](#anytype.model.Range) | optional |  |
| type | [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type) | optional |  |
| param | [string](#string) | optional |  |


<a name="anytype.model.Block.Content.Text.Marks"/>
### Block.Content.Text.Marks


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| marks | [Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark) | repeated |  |


<a name="anytype.model.Block.Restrictions"/>
### Block.Restrictions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| read | [bool](#bool) | optional |  |
| edit | [bool](#bool) | optional |  |
| remove | [bool](#bool) | optional |  |
| drag | [bool](#bool) | optional |  |
| dropOn | [bool](#bool) | optional |  |


<a name="anytype.model.BlockMetaOnly"/>
### BlockMetaOnly


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| fields | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.model.Layout"/>
### Layout


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [ObjectType.Layout](#anytype.model.ObjectType.Layout) | optional |  |
| name | [string](#string) | optional |  |
| requiredRelations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.model.LinkPreview"/>
### LinkPreview


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |
| title | [string](#string) | optional |  |
| description | [string](#string) | optional |  |
| imageUrl | [string](#string) | optional |  |
| faviconUrl | [string](#string) | optional |  |
| type | [LinkPreview.Type](#anytype.model.LinkPreview.Type) | optional |  |


<a name="anytype.model.ObjectType"/>
### ObjectType


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) | optional |  |
| name | [string](#string) | optional |  |
| relations | [Relation](#anytype.model.Relation) | repeated |  |
| layout | [ObjectType.Layout](#anytype.model.ObjectType.Layout) | optional |  |
| iconEmoji | [string](#string) | optional |  |
| description | [string](#string) | optional |  |
| hidden | [bool](#bool) | optional |  |
| readonly | [bool](#bool) | optional |  |
| types | [SmartBlockType](#anytype.model.SmartBlockType) | repeated |  |
| isArchived | [bool](#bool) | optional |  |


<a name="anytype.model.Range"/>
### Range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) | optional |  |
| to | [int32](#int32) | optional |  |


<a name="anytype.model.Relation"/>
### Relation


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| format | [RelationFormat](#anytype.model.RelationFormat) | optional |  |
| name | [string](#string) | optional |  |
| defaultValue | [Value](#google.protobuf.Value) | optional |  |
| dataSource | [Relation.DataSource](#anytype.model.Relation.DataSource) | optional |  |
| hidden | [bool](#bool) | optional |  |
| readOnly | [bool](#bool) | optional |  |
| readOnlyRelation | [bool](#bool) | optional |  |
| multi | [bool](#bool) | optional |  |
| objectTypes | [string](#string) | repeated |  |
| selectDict | [Relation.Option](#anytype.model.Relation.Option) | repeated |  |
| maxCount | [int32](#int32) | optional |  |
| description | [string](#string) | optional |  |
| scope | [Relation.Scope](#anytype.model.Relation.Scope) | optional |  |
| creator | [string](#string) | optional |  |


<a name="anytype.model.Relation.Option"/>
### Relation.Option


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) | optional |  |
| text | [string](#string) | optional |  |
| color | [string](#string) | optional |  |
| scope | [Relation.Option.Scope](#anytype.model.Relation.Option.Scope) | optional |  |


<a name="anytype.model.RelationOptions"/>
### RelationOptions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| options | [Relation.Option](#anytype.model.Relation.Option) | repeated |  |


<a name="anytype.model.RelationWithValue"/>
### RelationWithValue


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relation | [Relation](#anytype.model.Relation) | optional |  |
| value | [Value](#google.protobuf.Value) | optional |  |


<a name="anytype.model.Relations"/>
### Relations


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| relations | [Relation](#anytype.model.Relation) | repeated |  |


<a name="anytype.model.Restrictions"/>
### Restrictions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| object | [Restrictions.ObjectRestriction](#anytype.model.Restrictions.ObjectRestriction) | repeated |  |
| dataview | [Restrictions.DataviewRestrictions](#anytype.model.Restrictions.DataviewRestrictions) | repeated |  |


<a name="anytype.model.Restrictions.DataviewRestrictions"/>
### Restrictions.DataviewRestrictions


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blockId | [string](#string) | optional |  |
| restrictions | [Restrictions.DataviewRestriction](#anytype.model.Restrictions.DataviewRestriction) | repeated |  |


<a name="anytype.model.SmartBlockSnapshotBase"/>
### SmartBlockSnapshotBase


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blocks | [Block](#anytype.model.Block) | repeated |  |
| details | [Struct](#google.protobuf.Struct) | optional |  |
| fileKeys | [Struct](#google.protobuf.Struct) | optional |  |
| extraRelations | [Relation](#anytype.model.Relation) | repeated |  |
| objectTypes | [string](#string) | repeated |  |
| collections | [Struct](#google.protobuf.Struct) | optional |  |


<a name="anytype.model.ThreadCreateQueueEntry"/>
### ThreadCreateQueueEntry


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| collectionThread | [string](#string) | optional |  |
| threadId | [string](#string) | optional |  |


<a name="anytype.model.ThreadDeeplinkPayload"/>
### ThreadDeeplinkPayload


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) | optional |  |
| addrs | [string](#string) | repeated |  |



<a name="anytype.model.Block.Align"/>
### Block.Align


| Name | Number | Description |
| ---- | ------ | ----------- |
| AlignLeft | 0 |  |
| AlignCenter | 1 |  |
| AlignRight | 2 |  |

<a name="anytype.model.Block.Content.Dataview.Filter.Condition"/>
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

<a name="anytype.model.Block.Content.Dataview.Filter.Operator"/>
### Block.Content.Dataview.Filter.Operator


| Name | Number | Description |
| ---- | ------ | ----------- |
| And | 0 |  |
| Or | 1 |  |

<a name="anytype.model.Block.Content.Dataview.Relation.DateFormat"/>
### Block.Content.Dataview.Relation.DateFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| MonthAbbrBeforeDay | 0 |  |
| MonthAbbrAfterDay | 1 |  |
| Short | 2 |  |
| ShortUS | 3 |  |
| ISO | 4 |  |

<a name="anytype.model.Block.Content.Dataview.Relation.TimeFormat"/>
### Block.Content.Dataview.Relation.TimeFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| Format12 | 0 |  |
| Format24 | 1 |  |

<a name="anytype.model.Block.Content.Dataview.Sort.Type"/>
### Block.Content.Dataview.Sort.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Asc | 0 |  |
| Desc | 1 |  |

<a name="anytype.model.Block.Content.Dataview.View.Size"/>
### Block.Content.Dataview.View.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| Small | 0 |  |
| Medium | 1 |  |
| Large | 2 |  |

<a name="anytype.model.Block.Content.Dataview.View.Type"/>
### Block.Content.Dataview.View.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Table | 0 |  |
| List | 1 |  |
| Gallery | 2 |  |
| Kanban | 3 |  |

<a name="anytype.model.Block.Content.Div.Style"/>
### Block.Content.Div.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Line | 0 |  |
| Dots | 1 |  |

<a name="anytype.model.Block.Content.File.State"/>
### Block.Content.File.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 |  |
| Uploading | 1 |  |
| Done | 2 |  |
| Error | 3 |  |

<a name="anytype.model.Block.Content.File.Style"/>
### Block.Content.File.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Auto | 0 |  |
| Link | 1 |  |
| Embed | 2 |  |

<a name="anytype.model.Block.Content.File.Type"/>
### Block.Content.File.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| File | 1 |  |
| Image | 2 |  |
| Video | 3 |  |
| Audio | 4 |  |
| PDF | 5 |  |

<a name="anytype.model.Block.Content.Layout.Style"/>
### Block.Content.Layout.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Row | 0 |  |
| Column | 1 |  |
| Div | 2 |  |
| Header | 3 |  |

<a name="anytype.model.Block.Content.Link.Style"/>
### Block.Content.Link.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Page | 0 |  |
| Dataview | 1 |  |
| Dashboard | 2 |  |
| Archive | 3 |  |

<a name="anytype.model.Block.Content.Text.Mark.Type"/>
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

<a name="anytype.model.Block.Content.Text.Style"/>
### Block.Content.Text.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Paragraph | 0 |  |
| Header1 | 1 |  |
| Header2 | 2 |  |
| Header3 | 3 |  |
| Header4 | 4 |  |
| Quote | 5 |  |
| Code | 6 |  |
| Title | 7 |  |
| Checkbox | 8 |  |
| Marked | 9 |  |
| Numbered | 10 |  |
| Toggle | 11 |  |
| Description | 12 |  |

<a name="anytype.model.Block.Position"/>
### Block.Position


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Top | 1 |  |
| Bottom | 2 |  |
| Left | 3 |  |
| Right | 4 |  |
| Inner | 5 |  |
| Replace | 6 |  |
| InnerFirst | 7 |  |

<a name="anytype.model.LinkPreview.Type"/>
### LinkPreview.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Page | 1 |  |
| Image | 2 |  |
| Text | 3 |  |

<a name="anytype.model.ObjectType.Layout"/>
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
| database | 20 |  |

<a name="anytype.model.Relation.DataSource"/>
### Relation.DataSource


| Name | Number | Description |
| ---- | ------ | ----------- |
| details | 0 |  |
| derived | 1 |  |
| account | 2 |  |
| local | 3 |  |

<a name="anytype.model.Relation.Option.Scope"/>
### Relation.Option.Scope


| Name | Number | Description |
| ---- | ------ | ----------- |
| local | 0 |  |
| relation | 1 |  |
| format | 2 |  |

<a name="anytype.model.Relation.Scope"/>
### Relation.Scope


| Name | Number | Description |
| ---- | ------ | ----------- |
| object | 0 |  |
| type | 1 |  |
| setOfTheSameType | 2 |  |
| objectsOfTheSameType | 3 |  |
| library | 4 |  |

<a name="anytype.model.RelationFormat"/>
### RelationFormat


| Name | Number | Description |
| ---- | ------ | ----------- |
| longtext | 0 |  |
| shorttext | 1 |  |
| number | 2 |  |
| status | 3 |  |
| tag | 11 |  |
| date | 4 |  |
| file | 5 |  |
| checkbox | 6 |  |
| url | 7 |  |
| email | 8 |  |
| phone | 9 |  |
| emoji | 10 |  |
| object | 100 |  |
| relations | 101 |  |

<a name="anytype.model.Restrictions.DataviewRestriction"/>
### Restrictions.DataviewRestriction


| Name | Number | Description |
| ---- | ------ | ----------- |
| DVNone | 0 |  |
| DVRelation | 1 |  |
| DVCreateObject | 2 |  |
| DVViews | 3 |  |

<a name="anytype.model.Restrictions.ObjectRestriction"/>
### Restrictions.ObjectRestriction


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| Delete | 1 |  |
| Relations | 2 |  |
| Blocks | 3 |  |
| Details | 4 |  |
| TypeChange | 5 |  |
| LayoutChange | 6 |  |
| Template | 7 |  |

<a name="anytype.model.SmartBlockType"/>
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
| Set | 65 |  |
| STObjectType | 96 |  |
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
| WorkspaceOld | 517 |  |
| Workspace | 518 |  |





<a name="scalar-value-types"/>
## Scalar Value Types

| .proto Type | Notes | C++ Type | Java Type | Python Type |
| ----------- | ----- | -------- | --------- | ----------- |
| <a name="double"/> double |  | double | double | float |
| <a name="float"/> float |  | float | float | float |
| <a name="int32"/> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int |
| <a name="int64"/> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long |
| <a name="uint32"/> uint32 | Uses variable-length encoding. | uint32 | int | int/long |
| <a name="uint64"/> uint64 | Uses variable-length encoding. | uint64 | long | int/long |
| <a name="sint32"/> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int |
| <a name="sint64"/> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long |
| <a name="fixed32"/> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int |
| <a name="fixed64"/> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long |
| <a name="sfixed32"/> sfixed32 | Always four bytes. | int32 | int | int |
| <a name="sfixed64"/> sfixed64 | Always eight bytes. | int64 | long | int/long |
| <a name="bool"/> bool |  | bool | boolean | boolean |
| <a name="string"/> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode |
| <a name="bytes"/> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str |
