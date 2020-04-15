# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [pb/protos/service/service.proto](#pb/protos/service/service.proto)
  
  
  
    - [ClientCommands](#anytype.ClientCommands)
  

- [pb/protos/commands.proto](#pb/protos/commands.proto)
    - [Empty](#anytype.Empty)
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
    - [Rpc.Account.Stop](#anytype.Rpc.Account.Stop)
    - [Rpc.Account.Stop.Request](#anytype.Rpc.Account.Stop.Request)
    - [Rpc.Account.Stop.Response](#anytype.Rpc.Account.Stop.Response)
    - [Rpc.Account.Stop.Response.Error](#anytype.Rpc.Account.Stop.Response.Error)
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
    - [Rpc.Block.Cut](#anytype.Rpc.Block.Cut)
    - [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request)
    - [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response)
    - [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error)
    - [Rpc.Block.CutBreadcrumbs](#anytype.Rpc.Block.CutBreadcrumbs)
    - [Rpc.Block.CutBreadcrumbs.Request](#anytype.Rpc.Block.CutBreadcrumbs.Request)
    - [Rpc.Block.CutBreadcrumbs.Response](#anytype.Rpc.Block.CutBreadcrumbs.Response)
    - [Rpc.Block.CutBreadcrumbs.Response.Error](#anytype.Rpc.Block.CutBreadcrumbs.Response.Error)
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
    - [Rpc.Block.Merge](#anytype.Rpc.Block.Merge)
    - [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request)
    - [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response)
    - [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error)
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
    - [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response)
    - [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error)
    - [Rpc.Block.Redo](#anytype.Rpc.Block.Redo)
    - [Rpc.Block.Redo.Request](#anytype.Rpc.Block.Redo.Request)
    - [Rpc.Block.Redo.Response](#anytype.Rpc.Block.Redo.Response)
    - [Rpc.Block.Redo.Response.Error](#anytype.Rpc.Block.Redo.Response.Error)
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
    - [Rpc.Block.Split](#anytype.Rpc.Block.Split)
    - [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request)
    - [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response)
    - [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error)
    - [Rpc.Block.Undo](#anytype.Rpc.Block.Undo)
    - [Rpc.Block.Undo.Request](#anytype.Rpc.Block.Undo.Request)
    - [Rpc.Block.Undo.Response](#anytype.Rpc.Block.Undo.Response)
    - [Rpc.Block.Undo.Response.Error](#anytype.Rpc.Block.Undo.Response.Error)
    - [Rpc.Block.Unlink](#anytype.Rpc.Block.Unlink)
    - [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request)
    - [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response)
    - [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error)
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
    - [Rpc.BlockList.Set.Text](#anytype.Rpc.BlockList.Set.Text)
    - [Rpc.BlockList.Set.Text.Color](#anytype.Rpc.BlockList.Set.Text.Color)
    - [Rpc.BlockList.Set.Text.Color.Request](#anytype.Rpc.BlockList.Set.Text.Color.Request)
    - [Rpc.BlockList.Set.Text.Color.Response](#anytype.Rpc.BlockList.Set.Text.Color.Response)
    - [Rpc.BlockList.Set.Text.Color.Response.Error](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error)
    - [Rpc.BlockList.Set.Text.Style](#anytype.Rpc.BlockList.Set.Text.Style)
    - [Rpc.BlockList.Set.Text.Style.Request](#anytype.Rpc.BlockList.Set.Text.Style.Request)
    - [Rpc.BlockList.Set.Text.Style.Response](#anytype.Rpc.BlockList.Set.Text.Style.Response)
    - [Rpc.BlockList.Set.Text.Style.Response.Error](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error)
    - [Rpc.Config](#anytype.Rpc.Config)
    - [Rpc.Config.Get](#anytype.Rpc.Config.Get)
    - [Rpc.Config.Get.Request](#anytype.Rpc.Config.Get.Request)
    - [Rpc.Config.Get.Response](#anytype.Rpc.Config.Get.Response)
    - [Rpc.Config.Get.Response.Error](#anytype.Rpc.Config.Get.Response.Error)
    - [Rpc.ExternalDrop](#anytype.Rpc.ExternalDrop)
    - [Rpc.ExternalDrop.Content](#anytype.Rpc.ExternalDrop.Content)
    - [Rpc.ExternalDrop.Content.Request](#anytype.Rpc.ExternalDrop.Content.Request)
    - [Rpc.ExternalDrop.Content.Response](#anytype.Rpc.ExternalDrop.Content.Response)
    - [Rpc.ExternalDrop.Content.Response.Error](#anytype.Rpc.ExternalDrop.Content.Response.Error)
    - [Rpc.ExternalDrop.Files](#anytype.Rpc.ExternalDrop.Files)
    - [Rpc.ExternalDrop.Files.Request](#anytype.Rpc.ExternalDrop.Files.Request)
    - [Rpc.ExternalDrop.Files.Response](#anytype.Rpc.ExternalDrop.Files.Response)
    - [Rpc.ExternalDrop.Files.Response.Error](#anytype.Rpc.ExternalDrop.Files.Response.Error)
    - [Rpc.Ipfs](#anytype.Rpc.Ipfs)
    - [Rpc.Ipfs.File](#anytype.Rpc.Ipfs.File)
    - [Rpc.Ipfs.File.Get](#anytype.Rpc.Ipfs.File.Get)
    - [Rpc.Ipfs.File.Get.Request](#anytype.Rpc.Ipfs.File.Get.Request)
    - [Rpc.Ipfs.File.Get.Response](#anytype.Rpc.Ipfs.File.Get.Response)
    - [Rpc.Ipfs.File.Get.Response.Error](#anytype.Rpc.Ipfs.File.Get.Response.Error)
    - [Rpc.Ipfs.Image](#anytype.Rpc.Ipfs.Image)
    - [Rpc.Ipfs.Image.Get](#anytype.Rpc.Ipfs.Image.Get)
    - [Rpc.Ipfs.Image.Get.Blob](#anytype.Rpc.Ipfs.Image.Get.Blob)
    - [Rpc.Ipfs.Image.Get.Blob.Request](#anytype.Rpc.Ipfs.Image.Get.Blob.Request)
    - [Rpc.Ipfs.Image.Get.Blob.Response](#anytype.Rpc.Ipfs.Image.Get.Blob.Response)
    - [Rpc.Ipfs.Image.Get.Blob.Response.Error](#anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error)
    - [Rpc.Ipfs.Image.Get.File](#anytype.Rpc.Ipfs.Image.Get.File)
    - [Rpc.Ipfs.Image.Get.File.Request](#anytype.Rpc.Ipfs.Image.Get.File.Request)
    - [Rpc.Ipfs.Image.Get.File.Response](#anytype.Rpc.Ipfs.Image.Get.File.Response)
    - [Rpc.Ipfs.Image.Get.File.Response.Error](#anytype.Rpc.Ipfs.Image.Get.File.Response.Error)
    - [Rpc.LinkPreview](#anytype.Rpc.LinkPreview)
    - [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request)
    - [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response)
    - [Rpc.LinkPreview.Response.Error](#anytype.Rpc.LinkPreview.Response.Error)
    - [Rpc.Log](#anytype.Rpc.Log)
    - [Rpc.Log.Send](#anytype.Rpc.Log.Send)
    - [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request)
    - [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response)
    - [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error)
    - [Rpc.Ping](#anytype.Rpc.Ping)
    - [Rpc.Ping.Request](#anytype.Rpc.Ping.Request)
    - [Rpc.Ping.Response](#anytype.Rpc.Ping.Response)
    - [Rpc.Ping.Response.Error](#anytype.Rpc.Ping.Response.Error)
    - [Rpc.Process](#anytype.Rpc.Process)
    - [Rpc.Process.Cancel](#anytype.Rpc.Process.Cancel)
    - [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request)
    - [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response)
    - [Rpc.Process.Cancel.Response.Error](#anytype.Rpc.Process.Cancel.Response.Error)
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
    - [Rpc.Account.Stop.Response.Error.Code](#anytype.Rpc.Account.Stop.Response.Error.Code)
    - [Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response.Error.Code)
    - [Rpc.Block.Bookmark.Fetch.Response.Error.Code](#anytype.Rpc.Block.Bookmark.Fetch.Response.Error.Code)
    - [Rpc.Block.Close.Response.Error.Code](#anytype.Rpc.Block.Close.Response.Error.Code)
    - [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code)
    - [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code)
    - [Rpc.Block.CreatePage.Response.Error.Code](#anytype.Rpc.Block.CreatePage.Response.Error.Code)
    - [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code)
    - [Rpc.Block.CutBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.CutBreadcrumbs.Response.Error.Code)
    - [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code)
    - [Rpc.Block.Export.Response.Error.Code](#anytype.Rpc.Block.Export.Response.Error.Code)
    - [Rpc.Block.File.CreateAndUpload.Response.Error.Code](#anytype.Rpc.Block.File.CreateAndUpload.Response.Error.Code)
    - [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code)
    - [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code)
    - [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code)
    - [Rpc.Block.OpenBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.OpenBreadcrumbs.Response.Error.Code)
    - [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code)
    - [Rpc.Block.Redo.Response.Error.Code](#anytype.Rpc.Block.Redo.Response.Error.Code)
    - [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code)
    - [Rpc.Block.Set.Details.Response.Error.Code](#anytype.Rpc.Block.Set.Details.Response.Error.Code)
    - [Rpc.Block.Set.Fields.Response.Error.Code](#anytype.Rpc.Block.Set.Fields.Response.Error.Code)
    - [Rpc.Block.Set.File.Name.Response.Error.Code](#anytype.Rpc.Block.Set.File.Name.Response.Error.Code)
    - [Rpc.Block.Set.Image.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Name.Response.Error.Code)
    - [Rpc.Block.Set.Image.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Width.Response.Error.Code)
    - [Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code)
    - [Rpc.Block.Set.Page.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.Page.IsArchived.Response.Error.Code)
    - [Rpc.Block.Set.Restrictions.Response.Error.Code](#anytype.Rpc.Block.Set.Restrictions.Response.Error.Code)
    - [Rpc.Block.Set.Text.Checked.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code)
    - [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code)
    - [Rpc.Block.Set.Text.Style.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Style.Response.Error.Code)
    - [Rpc.Block.Set.Text.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Text.Response.Error.Code)
    - [Rpc.Block.Set.Video.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Name.Response.Error.Code)
    - [Rpc.Block.Set.Video.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Width.Response.Error.Code)
    - [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code)
    - [Rpc.Block.Undo.Response.Error.Code](#anytype.Rpc.Block.Undo.Response.Error.Code)
    - [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code)
    - [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code)
    - [Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response.Error.Code)
    - [Rpc.BlockList.Duplicate.Response.Error.Code](#anytype.Rpc.BlockList.Duplicate.Response.Error.Code)
    - [Rpc.BlockList.Move.Response.Error.Code](#anytype.Rpc.BlockList.Move.Response.Error.Code)
    - [Rpc.BlockList.MoveToNewPage.Response.Error.Code](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error.Code)
    - [Rpc.BlockList.Set.Align.Response.Error.Code](#anytype.Rpc.BlockList.Set.Align.Response.Error.Code)
    - [Rpc.BlockList.Set.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.BackgroundColor.Response.Error.Code)
    - [Rpc.BlockList.Set.Div.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Div.Style.Response.Error.Code)
    - [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code)
    - [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code)
    - [Rpc.ExternalDrop.Content.Response.Error.Code](#anytype.Rpc.ExternalDrop.Content.Response.Error.Code)
    - [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code)
    - [Rpc.Ipfs.File.Get.Response.Error.Code](#anytype.Rpc.Ipfs.File.Get.Response.Error.Code)
    - [Rpc.Ipfs.Image.Get.Blob.Response.Error.Code](#anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error.Code)
    - [Rpc.Ipfs.Image.Get.File.Response.Error.Code](#anytype.Rpc.Ipfs.Image.Get.File.Response.Error.Code)
    - [Rpc.LinkPreview.Response.Error.Code](#anytype.Rpc.LinkPreview.Response.Error.Code)
    - [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code)
    - [Rpc.Ping.Response.Error.Code](#anytype.Rpc.Ping.Response.Error.Code)
    - [Rpc.Process.Cancel.Response.Error.Code](#anytype.Rpc.Process.Cancel.Response.Error.Code)
    - [Rpc.UploadFile.Response.Error.Code](#anytype.Rpc.UploadFile.Response.Error.Code)
    - [Rpc.Version.Get.Response.Error.Code](#anytype.Rpc.Version.Get.Response.Error.Code)
    - [Rpc.Wallet.Create.Response.Error.Code](#anytype.Rpc.Wallet.Create.Response.Error.Code)
    - [Rpc.Wallet.Recover.Response.Error.Code](#anytype.Rpc.Wallet.Recover.Response.Error.Code)
  
  
  

- [pb/protos/events.proto](#pb/protos/events.proto)
    - [Event](#anytype.Event)
    - [Event.Account](#anytype.Event.Account)
    - [Event.Account.Show](#anytype.Event.Account.Show)
    - [Event.Block](#anytype.Event.Block)
    - [Event.Block.Add](#anytype.Event.Block.Add)
    - [Event.Block.Delete](#anytype.Event.Block.Delete)
    - [Event.Block.FilesUpload](#anytype.Event.Block.FilesUpload)
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
    - [Event.Block.Set.Details](#anytype.Event.Block.Set.Details)
    - [Event.Block.Set.Div](#anytype.Event.Block.Set.Div)
    - [Event.Block.Set.Div.Style](#anytype.Event.Block.Set.Div.Style)
    - [Event.Block.Set.Fields](#anytype.Event.Block.Set.Fields)
    - [Event.Block.Set.File](#anytype.Event.Block.Set.File)
    - [Event.Block.Set.File.Hash](#anytype.Event.Block.Set.File.Hash)
    - [Event.Block.Set.File.Mime](#anytype.Event.Block.Set.File.Mime)
    - [Event.Block.Set.File.Name](#anytype.Event.Block.Set.File.Name)
    - [Event.Block.Set.File.Size](#anytype.Event.Block.Set.File.Size)
    - [Event.Block.Set.File.State](#anytype.Event.Block.Set.File.State)
    - [Event.Block.Set.File.Type](#anytype.Event.Block.Set.File.Type)
    - [Event.Block.Set.File.Width](#anytype.Event.Block.Set.File.Width)
    - [Event.Block.Set.Link](#anytype.Event.Block.Set.Link)
    - [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields)
    - [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style)
    - [Event.Block.Set.Link.TargetBlockId](#anytype.Event.Block.Set.Link.TargetBlockId)
    - [Event.Block.Set.Restrictions](#anytype.Event.Block.Set.Restrictions)
    - [Event.Block.Set.Text](#anytype.Event.Block.Set.Text)
    - [Event.Block.Set.Text.Checked](#anytype.Event.Block.Set.Text.Checked)
    - [Event.Block.Set.Text.Color](#anytype.Event.Block.Set.Text.Color)
    - [Event.Block.Set.Text.Marks](#anytype.Event.Block.Set.Text.Marks)
    - [Event.Block.Set.Text.Style](#anytype.Event.Block.Set.Text.Style)
    - [Event.Block.Set.Text.Text](#anytype.Event.Block.Set.Text.Text)
    - [Event.Block.Show](#anytype.Event.Block.Show)
    - [Event.Message](#anytype.Event.Message)
    - [Event.Ping](#anytype.Event.Ping)
    - [Event.Process](#anytype.Event.Process)
    - [Event.Process.Done](#anytype.Event.Process.Done)
    - [Event.Process.New](#anytype.Event.Process.New)
    - [Event.Process.Update](#anytype.Event.Process.Update)
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
  
    - [Model.Process.State](#anytype.Model.Process.State)
    - [Model.Process.Type](#anytype.Model.Process.Type)
  
  
  

- [vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto](#vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto)
    - [Account](#anytype.model.Account)
    - [Account.Avatar](#anytype.model.Account.Avatar)
    - [Block](#anytype.model.Block)
    - [Block.Content](#anytype.model.Block.Content)
    - [Block.Content.Bookmark](#anytype.model.Block.Content.Bookmark)
    - [Block.Content.Dashboard](#anytype.model.Block.Content.Dashboard)
    - [Block.Content.Dataview](#anytype.model.Block.Content.Dataview)
    - [Block.Content.Div](#anytype.model.Block.Content.Div)
    - [Block.Content.File](#anytype.model.Block.Content.File)
    - [Block.Content.Icon](#anytype.model.Block.Content.Icon)
    - [Block.Content.Layout](#anytype.model.Block.Content.Layout)
    - [Block.Content.Link](#anytype.model.Block.Content.Link)
    - [Block.Content.Page](#anytype.model.Block.Content.Page)
    - [Block.Content.Text](#anytype.model.Block.Content.Text)
    - [Block.Content.Text.Mark](#anytype.model.Block.Content.Text.Mark)
    - [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks)
    - [Block.Restrictions](#anytype.model.Block.Restrictions)
    - [BlockMetaOnly](#anytype.model.BlockMetaOnly)
    - [LinkPreview](#anytype.model.LinkPreview)
    - [Range](#anytype.model.Range)
    - [SmartBlock](#anytype.model.SmartBlock)
  
    - [Block.Align](#anytype.model.Block.Align)
    - [Block.Content.Dashboard.Style](#anytype.model.Block.Content.Dashboard.Style)
    - [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style)
    - [Block.Content.File.State](#anytype.model.Block.Content.File.State)
    - [Block.Content.File.Type](#anytype.model.Block.Content.File.Type)
    - [Block.Content.Layout.Style](#anytype.model.Block.Content.Layout.Style)
    - [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style)
    - [Block.Content.Page.Style](#anytype.model.Block.Content.Page.Style)
    - [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type)
    - [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style)
    - [Block.Position](#anytype.model.Block.Position)
    - [LinkPreview.Type](#anytype.model.LinkPreview.Type)
    - [SmartBlock.Type](#anytype.model.SmartBlock.Type)
  
  
  

- [Scalar Value Types](#scalar-value-types)



<a name="pb/protos/service/service.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/service/service.proto


 

 

 


<a name="anytype.ClientCommands"></a>

### ClientCommands


| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| WalletCreate | [Rpc.Wallet.Create.Request](#anytype.Rpc.Wallet.Create.Request) | [Rpc.Wallet.Create.Response](#anytype.Rpc.Wallet.Create.Response) |  |
| WalletRecover | [Rpc.Wallet.Recover.Request](#anytype.Rpc.Wallet.Recover.Request) | [Rpc.Wallet.Recover.Response](#anytype.Rpc.Wallet.Recover.Response) |  |
| AccountRecover | [Rpc.Account.Recover.Request](#anytype.Rpc.Account.Recover.Request) | [Rpc.Account.Recover.Response](#anytype.Rpc.Account.Recover.Response) |  |
| AccountCreate | [Rpc.Account.Create.Request](#anytype.Rpc.Account.Create.Request) | [Rpc.Account.Create.Response](#anytype.Rpc.Account.Create.Response) |  |
| AccountSelect | [Rpc.Account.Select.Request](#anytype.Rpc.Account.Select.Request) | [Rpc.Account.Select.Response](#anytype.Rpc.Account.Select.Response) |  |
| AccountStop | [Rpc.Account.Stop.Request](#anytype.Rpc.Account.Stop.Request) | [Rpc.Account.Stop.Response](#anytype.Rpc.Account.Stop.Response) |  |
| ImageGetBlob | [Rpc.Ipfs.Image.Get.Blob.Request](#anytype.Rpc.Ipfs.Image.Get.Blob.Request) | [Rpc.Ipfs.Image.Get.Blob.Response](#anytype.Rpc.Ipfs.Image.Get.Blob.Response) |  |
| VersionGet | [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request) | [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request) | [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response) |  |
| ConfigGet | [Rpc.Config.Get.Request](#anytype.Rpc.Config.Get.Request) | [Rpc.Config.Get.Response](#anytype.Rpc.Config.Get.Response) |  |
| ExternalDropFiles | [Rpc.ExternalDrop.Files.Request](#anytype.Rpc.ExternalDrop.Files.Request) | [Rpc.ExternalDrop.Files.Response](#anytype.Rpc.ExternalDrop.Files.Response) |  |
| ExternalDropContent | [Rpc.ExternalDrop.Content.Request](#anytype.Rpc.ExternalDrop.Content.Request) | [Rpc.ExternalDrop.Content.Response](#anytype.Rpc.ExternalDrop.Content.Response) |  |
| LinkPreview | [Rpc.LinkPreview.Request](#anytype.Rpc.LinkPreview.Request) | [Rpc.LinkPreview.Response](#anytype.Rpc.LinkPreview.Response) |  |
| UploadFile | [Rpc.UploadFile.Request](#anytype.Rpc.UploadFile.Request) | [Rpc.UploadFile.Response](#anytype.Rpc.UploadFile.Response) |  |
| BlockUpload | [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request) | [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response) |  |
| BlockReplace | [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request) | [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response) |  |
| BlockOpen | [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request) | [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response) |  |
| BlockOpenBreadcrumbs | [Rpc.Block.OpenBreadcrumbs.Request](#anytype.Rpc.Block.OpenBreadcrumbs.Request) | [Rpc.Block.OpenBreadcrumbs.Response](#anytype.Rpc.Block.OpenBreadcrumbs.Response) |  |
| BlockCutBreadcrumbs | [Rpc.Block.CutBreadcrumbs.Request](#anytype.Rpc.Block.CutBreadcrumbs.Request) | [Rpc.Block.CutBreadcrumbs.Response](#anytype.Rpc.Block.CutBreadcrumbs.Response) |  |
| BlockCreate | [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request) | [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response) |  |
| BlockCreatePage | [Rpc.Block.CreatePage.Request](#anytype.Rpc.Block.CreatePage.Request) | [Rpc.Block.CreatePage.Response](#anytype.Rpc.Block.CreatePage.Response) |  |
| BlockUnlink | [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request) | [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response) |  |
| BlockClose | [Rpc.Block.Close.Request](#anytype.Rpc.Block.Close.Request) | [Rpc.Block.Close.Response](#anytype.Rpc.Block.Close.Response) |  |
| BlockDownload | [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request) | [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response) |  |
| BlockGetMarks | [Rpc.Block.Get.Marks.Request](#anytype.Rpc.Block.Get.Marks.Request) | [Rpc.Block.Get.Marks.Response](#anytype.Rpc.Block.Get.Marks.Response) |  |
| BlockUndo | [Rpc.Block.Undo.Request](#anytype.Rpc.Block.Undo.Request) | [Rpc.Block.Undo.Response](#anytype.Rpc.Block.Undo.Response) |  |
| BlockRedo | [Rpc.Block.Redo.Request](#anytype.Rpc.Block.Redo.Request) | [Rpc.Block.Redo.Response](#anytype.Rpc.Block.Redo.Response) |  |
| BlockSetFields | [Rpc.Block.Set.Fields.Request](#anytype.Rpc.Block.Set.Fields.Request) | [Rpc.Block.Set.Fields.Response](#anytype.Rpc.Block.Set.Fields.Response) |  |
| BlockSetRestrictions | [Rpc.Block.Set.Restrictions.Request](#anytype.Rpc.Block.Set.Restrictions.Request) | [Rpc.Block.Set.Restrictions.Response](#anytype.Rpc.Block.Set.Restrictions.Response) |  |
| BlockSetDetails | [Rpc.Block.Set.Details.Request](#anytype.Rpc.Block.Set.Details.Request) | [Rpc.Block.Set.Details.Response](#anytype.Rpc.Block.Set.Details.Response) |  |
| BlockSetPageIsArchived | [Rpc.Block.Set.Page.IsArchived.Request](#anytype.Rpc.Block.Set.Page.IsArchived.Request) | [Rpc.Block.Set.Page.IsArchived.Response](#anytype.Rpc.Block.Set.Page.IsArchived.Response) |  |
| BlockListMove | [Rpc.BlockList.Move.Request](#anytype.Rpc.BlockList.Move.Request) | [Rpc.BlockList.Move.Response](#anytype.Rpc.BlockList.Move.Response) |  |
| BlockListMoveToNewPage | [Rpc.BlockList.MoveToNewPage.Request](#anytype.Rpc.BlockList.MoveToNewPage.Request) | [Rpc.BlockList.MoveToNewPage.Response](#anytype.Rpc.BlockList.MoveToNewPage.Response) |  |
| BlockListConvertChildrenToPages | [Rpc.BlockList.ConvertChildrenToPages.Request](#anytype.Rpc.BlockList.ConvertChildrenToPages.Request) | [Rpc.BlockList.ConvertChildrenToPages.Response](#anytype.Rpc.BlockList.ConvertChildrenToPages.Response) |  |
| BlockListSetFields | [Rpc.BlockList.Set.Fields.Request](#anytype.Rpc.BlockList.Set.Fields.Request) | [Rpc.BlockList.Set.Fields.Response](#anytype.Rpc.BlockList.Set.Fields.Response) |  |
| BlockListSetTextStyle | [Rpc.BlockList.Set.Text.Style.Request](#anytype.Rpc.BlockList.Set.Text.Style.Request) | [Rpc.BlockList.Set.Text.Style.Response](#anytype.Rpc.BlockList.Set.Text.Style.Response) |  |
| BlockListDuplicate | [Rpc.BlockList.Duplicate.Request](#anytype.Rpc.BlockList.Duplicate.Request) | [Rpc.BlockList.Duplicate.Response](#anytype.Rpc.BlockList.Duplicate.Response) |  |
| BlockListSetBackgroundColor | [Rpc.BlockList.Set.BackgroundColor.Request](#anytype.Rpc.BlockList.Set.BackgroundColor.Request) | [Rpc.BlockList.Set.BackgroundColor.Response](#anytype.Rpc.BlockList.Set.BackgroundColor.Response) |  |
| BlockListSetAlign | [Rpc.BlockList.Set.Align.Request](#anytype.Rpc.BlockList.Set.Align.Request) | [Rpc.BlockList.Set.Align.Response](#anytype.Rpc.BlockList.Set.Align.Response) |  |
| BlockListSetDivStyle | [Rpc.BlockList.Set.Div.Style.Request](#anytype.Rpc.BlockList.Set.Div.Style.Request) | [Rpc.BlockList.Set.Div.Style.Response](#anytype.Rpc.BlockList.Set.Div.Style.Response) |  |
| BlockSetTextText | [Rpc.Block.Set.Text.Text.Request](#anytype.Rpc.Block.Set.Text.Text.Request) | [Rpc.Block.Set.Text.Text.Response](#anytype.Rpc.Block.Set.Text.Text.Response) |  |
| BlockSetTextColor | [Rpc.Block.Set.Text.Color.Request](#anytype.Rpc.Block.Set.Text.Color.Request) | [Rpc.Block.Set.Text.Color.Response](#anytype.Rpc.Block.Set.Text.Color.Response) |  |
| BlockListSetTextColor | [Rpc.BlockList.Set.Text.Color.Request](#anytype.Rpc.BlockList.Set.Text.Color.Request) | [Rpc.BlockList.Set.Text.Color.Response](#anytype.Rpc.BlockList.Set.Text.Color.Response) |  |
| BlockSetTextStyle | [Rpc.Block.Set.Text.Style.Request](#anytype.Rpc.Block.Set.Text.Style.Request) | [Rpc.Block.Set.Text.Style.Response](#anytype.Rpc.Block.Set.Text.Style.Response) |  |
| BlockSetTextChecked | [Rpc.Block.Set.Text.Checked.Request](#anytype.Rpc.Block.Set.Text.Checked.Request) | [Rpc.Block.Set.Text.Checked.Response](#anytype.Rpc.Block.Set.Text.Checked.Response) |  |
| BlockSplit | [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request) | [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response) |  |
| BlockMerge | [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request) | [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response) |  |
| BlockCopy | [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request) | [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response) |  |
| BlockPaste | [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request) | [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response) |  |
| BlockCut | [Rpc.Block.Cut.Request](#anytype.Rpc.Block.Cut.Request) | [Rpc.Block.Cut.Response](#anytype.Rpc.Block.Cut.Response) |  |
| BlockExport | [Rpc.Block.Export.Request](#anytype.Rpc.Block.Export.Request) | [Rpc.Block.Export.Response](#anytype.Rpc.Block.Export.Response) |  |
| BlockSetFileName | [Rpc.Block.Set.File.Name.Request](#anytype.Rpc.Block.Set.File.Name.Request) | [Rpc.Block.Set.File.Name.Response](#anytype.Rpc.Block.Set.File.Name.Response) |  |
| BlockSetImageName | [Rpc.Block.Set.Image.Name.Request](#anytype.Rpc.Block.Set.Image.Name.Request) | [Rpc.Block.Set.Image.Name.Response](#anytype.Rpc.Block.Set.Image.Name.Response) |  |
| BlockSetImageWidth | [Rpc.Block.Set.Image.Width.Request](#anytype.Rpc.Block.Set.Image.Width.Request) | [Rpc.Block.Set.Image.Width.Response](#anytype.Rpc.Block.Set.Image.Width.Response) |  |
| BlockSetVideoName | [Rpc.Block.Set.Video.Name.Request](#anytype.Rpc.Block.Set.Video.Name.Request) | [Rpc.Block.Set.Video.Name.Response](#anytype.Rpc.Block.Set.Video.Name.Response) |  |
| BlockSetVideoWidth | [Rpc.Block.Set.Video.Width.Request](#anytype.Rpc.Block.Set.Video.Width.Request) | [Rpc.Block.Set.Video.Width.Response](#anytype.Rpc.Block.Set.Video.Width.Response) |  |
| BlockSetLinkTargetBlockId | [Rpc.Block.Set.Link.TargetBlockId.Request](#anytype.Rpc.Block.Set.Link.TargetBlockId.Request) | [Rpc.Block.Set.Link.TargetBlockId.Response](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response) |  |
| BlockBookmarkFetch | [Rpc.Block.Bookmark.Fetch.Request](#anytype.Rpc.Block.Bookmark.Fetch.Request) | [Rpc.Block.Bookmark.Fetch.Response](#anytype.Rpc.Block.Bookmark.Fetch.Response) |  |
| BlockBookmarkCreateAndFetch | [Rpc.Block.Bookmark.CreateAndFetch.Request](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Request) | [Rpc.Block.Bookmark.CreateAndFetch.Response](#anytype.Rpc.Block.Bookmark.CreateAndFetch.Response) |  |
| BlockFileCreateAndUpload | [Rpc.Block.File.CreateAndUpload.Request](#anytype.Rpc.Block.File.CreateAndUpload.Request) | [Rpc.Block.File.CreateAndUpload.Response](#anytype.Rpc.Block.File.CreateAndUpload.Response) |  |
| Ping | [Rpc.Ping.Request](#anytype.Rpc.Ping.Request) | [Rpc.Ping.Response](#anytype.Rpc.Ping.Response) |  |
| ProcessCancel | [Rpc.Process.Cancel.Request](#anytype.Rpc.Process.Cancel.Request) | [Rpc.Process.Cancel.Response](#anytype.Rpc.Process.Cancel.Response) |  |
| ListenEvents | [Empty](#anytype.Empty) | [Event](#anytype.Event) stream | used only for lib-debug via grpc |

 



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
| alphaInviteCode | [string](#string) |  |  |






<a name="anytype.Rpc.Account.Create.Response"></a>

### Rpc.Account.Create.Response
Middleware-to-front-end response for an account creation request, that can contain a NULL error and created account or a non-NULL error and an empty account


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Account.Create.Response.Error](#anytype.Rpc.Account.Create.Response.Error) |  | Error while trying to create an account |
| account | [model.Account](#anytype.model.Account) |  | A newly created account; In case of a failure, i.e. error is non-NULL, the account model should contain empty/default-value fields |






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
| account | [model.Account](#anytype.model.Account) |  | Selected account |






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






<a name="anytype.Rpc.Block.Copy.Response"></a>

### Rpc.Block.Copy.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error) |  |  |
| html | [string](#string) |  |  |






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
| targetId | [string](#string) |  | id of the closest block |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  | page details |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.Block.CreatePage.Response"></a>

### Rpc.Block.CreatePage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CreatePage.Response.Error](#anytype.Rpc.Block.CreatePage.Response.Error) |  |  |
| blockId | [string](#string) |  |  |
| targetId | [string](#string) |  |  |






<a name="anytype.Rpc.Block.CreatePage.Response.Error"></a>

### Rpc.Block.CreatePage.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CreatePage.Response.Error.Code](#anytype.Rpc.Block.CreatePage.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Cut"></a>

### Rpc.Block.Cut







<a name="anytype.Rpc.Block.Cut.Request"></a>

### Rpc.Block.Cut.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blocks | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Rpc.Block.Cut.Response"></a>

### Rpc.Block.Cut.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Cut.Response.Error](#anytype.Rpc.Block.Cut.Response.Error) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Rpc.Block.Cut.Response.Error"></a>

### Rpc.Block.Cut.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Cut.Response.Error.Code](#anytype.Rpc.Block.Cut.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.CutBreadcrumbs"></a>

### Rpc.Block.CutBreadcrumbs







<a name="anytype.Rpc.Block.CutBreadcrumbs.Request"></a>

### Rpc.Block.CutBreadcrumbs.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| breadcrumbsId | [string](#string) |  |  |
| index | [int32](#int32) |  | 0 - for full reset |






<a name="anytype.Rpc.Block.CutBreadcrumbs.Response"></a>

### Rpc.Block.CutBreadcrumbs.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.CutBreadcrumbs.Response.Error](#anytype.Rpc.Block.CutBreadcrumbs.Response.Error) |  |  |






<a name="anytype.Rpc.Block.CutBreadcrumbs.Response.Error"></a>

### Rpc.Block.CutBreadcrumbs.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.CutBreadcrumbs.Response.Error.Code](#anytype.Rpc.Block.CutBreadcrumbs.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Get.Marks.Response.Error"></a>

### Rpc.Block.Get.Marks.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Merge.Response.Error"></a>

### Rpc.Block.Merge.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code) |  |  |
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
| breadcrumbsIds | [string](#string) | repeated | optional ids of breadcrubms blocks |






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
| copyTextRange | [model.Range](#anytype.model.Range) |  |  |
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Rpc.Block.Paste.Response"></a>

### Rpc.Block.Paste.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error) |  |  |
| blockIds | [string](#string) | repeated |  |
| caretPosition | [int32](#int32) |  |  |






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






<a name="anytype.Rpc.Block.Redo.Response.Error"></a>

### Rpc.Block.Redo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Redo.Response.Error.Code](#anytype.Rpc.Block.Redo.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Set.Text.Color.Response.Error"></a>

### Rpc.Block.Set.Text.Color.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Split.Response"></a>

### Rpc.Block.Split.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Split.Response.Error](#anytype.Rpc.Block.Split.Response.Error) |  |  |
| blockId | [string](#string) |  |  |






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






<a name="anytype.Rpc.Block.Undo.Response.Error"></a>

### Rpc.Block.Undo.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Undo.Response.Error.Code](#anytype.Rpc.Block.Undo.Response.Error.Code) |  |  |
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
| dropTargetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.BlockList.Move.Response"></a>

### Rpc.BlockList.Move.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Move.Response.Error](#anytype.Rpc.BlockList.Move.Response.Error) |  |  |






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
| dropTargetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |






<a name="anytype.Rpc.BlockList.MoveToNewPage.Response"></a>

### Rpc.BlockList.MoveToNewPage.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.MoveToNewPage.Response.Error](#anytype.Rpc.BlockList.MoveToNewPage.Response.Error) |  |  |
| linkId | [string](#string) |  |  |






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
| blockIds | [string](#string) | repeated |  |
| align | [model.Block.Align](#anytype.model.Block.Align) |  |  |






<a name="anytype.Rpc.BlockList.Set.Align.Response"></a>

### Rpc.BlockList.Set.Align.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Align.Response.Error](#anytype.Rpc.BlockList.Set.Align.Response.Error) |  |  |






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






<a name="anytype.Rpc.BlockList.Set.Fields.Response.Error"></a>

### Rpc.BlockList.Set.Fields.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text"></a>

### Rpc.BlockList.Set.Text







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






<a name="anytype.Rpc.BlockList.Set.Text.Color.Response.Error"></a>

### Rpc.BlockList.Set.Text.Color.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.BlockList.Set.Text.Style.Response.Error"></a>

### Rpc.BlockList.Set.Text.Style.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code) |  |  |
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
| archiveBlockId | [string](#string) |  | home dashboard block id |
| gatewayUrl | [string](#string) |  | gateway url for fetching static files |






<a name="anytype.Rpc.Config.Get.Response.Error"></a>

### Rpc.Config.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code) |  |  |
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
| dropTargetId | [string](#string) |  |  |
| position | [model.Block.Position](#anytype.model.Block.Position) |  |  |
| localFilePaths | [string](#string) | repeated |  |






<a name="anytype.Rpc.ExternalDrop.Files.Response"></a>

### Rpc.ExternalDrop.Files.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.ExternalDrop.Files.Response.Error](#anytype.Rpc.ExternalDrop.Files.Response.Error) |  |  |






<a name="anytype.Rpc.ExternalDrop.Files.Response.Error"></a>

### Rpc.ExternalDrop.Files.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs"></a>

### Rpc.Ipfs
Namespace, that agregates subtopics and actions to work with IPFS directly (get files, blobs, images, etc)






<a name="anytype.Rpc.Ipfs.File"></a>

### Rpc.Ipfs.File







<a name="anytype.Rpc.Ipfs.File.Get"></a>

### Rpc.Ipfs.File.Get







<a name="anytype.Rpc.Ipfs.File.Get.Request"></a>

### Rpc.Ipfs.File.Get.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.File.Get.Response"></a>

### Rpc.Ipfs.File.Get.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Ipfs.File.Get.Response.Error](#anytype.Rpc.Ipfs.File.Get.Response.Error) |  |  |
| data | [bytes](#bytes) |  |  |
| media | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.File.Get.Response.Error"></a>

### Rpc.Ipfs.File.Get.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Ipfs.File.Get.Response.Error.Code](#anytype.Rpc.Ipfs.File.Get.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.Image"></a>

### Rpc.Ipfs.Image







<a name="anytype.Rpc.Ipfs.Image.Get"></a>

### Rpc.Ipfs.Image.Get







<a name="anytype.Rpc.Ipfs.Image.Get.Blob"></a>

### Rpc.Ipfs.Image.Get.Blob







<a name="anytype.Rpc.Ipfs.Image.Get.Blob.Request"></a>

### Rpc.Ipfs.Image.Get.Blob.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| wantWidth | [int32](#int32) |  |  |






<a name="anytype.Rpc.Ipfs.Image.Get.Blob.Response"></a>

### Rpc.Ipfs.Image.Get.Blob.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Ipfs.Image.Get.Blob.Response.Error](#anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error) |  |  |
| blob | [bytes](#bytes) |  |  |






<a name="anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error"></a>

### Rpc.Ipfs.Image.Get.Blob.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Ipfs.Image.Get.Blob.Response.Error.Code](#anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.Image.Get.File"></a>

### Rpc.Ipfs.Image.Get.File







<a name="anytype.Rpc.Ipfs.Image.Get.File.Request"></a>

### Rpc.Ipfs.Image.Get.File.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [string](#string) |  |  |
| wantWidth | [int32](#int32) |  |  |






<a name="anytype.Rpc.Ipfs.Image.Get.File.Response"></a>

### Rpc.Ipfs.Image.Get.File.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Ipfs.Image.Get.File.Response.Error](#anytype.Rpc.Ipfs.Image.Get.File.Response.Error) |  |  |
| localPath | [string](#string) |  |  |






<a name="anytype.Rpc.Ipfs.Image.Get.File.Response.Error"></a>

### Rpc.Ipfs.Image.Get.File.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Ipfs.Image.Get.File.Response.Error.Code](#anytype.Rpc.Ipfs.Image.Get.File.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






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






<a name="anytype.Rpc.UploadFile"></a>

### Rpc.UploadFile







<a name="anytype.Rpc.UploadFile.Request"></a>

### Rpc.UploadFile.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  |  |
| localPath | [string](#string) |  |  |
| type | [model.Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |






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
Namespace, that agregates subtopics and actions, that relates to wallet.






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
| FAILED_TO_STOP_RUNNING_NODE | 104 |  |
| BAD_INVITE_CODE | 900 |  |



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



<a name="anytype.Rpc.Block.Cut.Response.Error.Code"></a>

### Rpc.Block.Cut.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.CutBreadcrumbs.Response.Error.Code"></a>

### Rpc.Block.CutBreadcrumbs.Response.Error.Code


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



<a name="anytype.Rpc.Block.Merge.Response.Error.Code"></a>

### Rpc.Block.Merge.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Open.Response.Error.Code"></a>

### Rpc.Block.Open.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



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



<a name="anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code"></a>

### Rpc.BlockList.Set.Text.Color.Response.Error.Code


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



<a name="anytype.Rpc.Config.Get.Response.Error.Code"></a>

### Rpc.Config.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 |  |
| NODE_NOT_STARTED | 101 |  |



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



<a name="anytype.Rpc.Ipfs.File.Get.Response.Error.Code"></a>

### Rpc.Ipfs.File.Get.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |



<a name="anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error.Code"></a>

### Rpc.Ipfs.Image.Get.Blob.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |
| NODE_NOT_STARTED | 103 |  |



<a name="anytype.Rpc.Ipfs.Image.Get.File.Response.Error.Code"></a>

### Rpc.Ipfs.Image.Get.File.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |
| NOT_FOUND | 101 |  |
| TIMEOUT | 102 |  |
| NODE_NOT_STARTED | 103 |  |



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






<a name="anytype.Event.Account"></a>

### Event.Account







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






<a name="anytype.Event.Block.Set.Details"></a>

### Event.Block.Set.Details



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| details | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






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






<a name="anytype.Event.Block.Show"></a>

### Event.Block.Show
Works with a smart blocks: Page, Dashboard
Dashboard opened, click on a page, Rpc.Block.open, Block.ShowFullscreen(PageBlock)


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rootId | [string](#string) |  | Root block id |
| blocks | [model.Block](#anytype.model.Block) | repeated | dependent blocks (descendants) |
| details | [Event.Block.Set.Details](#anytype.Event.Block.Set.Details) | repeated | details for current and dependent smart blocks |






<a name="anytype.Event.Message"></a>

### Event.Message



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| accountShow | [Event.Account.Show](#anytype.Event.Account.Show) |  |  |
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
| blockSetDetails | [Event.Block.Set.Details](#anytype.Event.Block.Set.Details) |  |  |
| blockSetDiv | [Event.Block.Set.Div](#anytype.Event.Block.Set.Div) |  |  |
| blockShow | [Event.Block.Show](#anytype.Event.Block.Show) |  |  |
| userBlockJoin | [Event.User.Block.Join](#anytype.Event.User.Block.Join) |  |  |
| userBlockLeft | [Event.User.Block.Left](#anytype.Event.User.Block.Left) |  |  |
| userBlockSelectRange | [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange) |  |  |
| userBlockTextRange | [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange) |  |  |
| ping | [Event.Ping](#anytype.Event.Ping) |  |  |
| processNew | [Event.Process.New](#anytype.Event.Process.New) |  |  |
| processUpdate | [Event.Process.Update](#anytype.Event.Process.Update) |  |  |
| processDone | [Event.Process.Done](#anytype.Event.Process.Done) |  |  |






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






<a name="anytype.ResponseEvent"></a>

### ResponseEvent



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| messages | [Event.Message](#anytype.Event.Message) | repeated |  |
| contextId | [string](#string) |  |  |





 


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


 

 

 



<a name="vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## vendor/github.com/anytypeio/go-anytype-library/pb/model/protos/models.proto



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
| dashboard | [Block.Content.Dashboard](#anytype.model.Block.Content.Dashboard) |  |  |
| page | [Block.Content.Page](#anytype.model.Block.Content.Page) |  |  |
| dataview | [Block.Content.Dataview](#anytype.model.Block.Content.Dataview) |  |  |
| text | [Block.Content.Text](#anytype.model.Block.Content.Text) |  |  |
| file | [Block.Content.File](#anytype.model.Block.Content.File) |  |  |
| layout | [Block.Content.Layout](#anytype.model.Block.Content.Layout) |  |  |
| div | [Block.Content.Div](#anytype.model.Block.Content.Div) |  |  |
| bookmark | [Block.Content.Bookmark](#anytype.model.Block.Content.Bookmark) |  |  |
| icon | [Block.Content.Icon](#anytype.model.Block.Content.Icon) |  |  |
| link | [Block.Content.Link](#anytype.model.Block.Content.Link) |  |  |






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






<a name="anytype.model.Block.Content.Dashboard"></a>

### Block.Content.Dashboard
Block type to organize pages on the main screen (main purpose)
It also can be mounted on a page.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Dashboard.Style](#anytype.model.Block.Content.Dashboard.Style) |  |  |






<a name="anytype.model.Block.Content.Dataview"></a>

### Block.Content.Dataview







<a name="anytype.model.Block.Content.Div"></a>

### Block.Content.Div
Divider: block, that contains only one horizontal thin line


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Div.Style](#anytype.model.Block.Content.Div.Style) |  |  |






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






<a name="anytype.model.Block.Content.Icon"></a>

### Block.Content.Icon



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |






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
| style | [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.model.Block.Content.Page"></a>

### Block.Content.Page



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| style | [Block.Content.Page.Style](#anytype.model.Block.Content.Page.Style) |  |  |






<a name="anytype.model.Block.Content.Text"></a>

### Block.Content.Text



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| text | [string](#string) |  |  |
| style | [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style) |  |  |
| marks | [Block.Content.Text.Marks](#anytype.model.Block.Content.Text.Marks) |  | list of marks to apply to the text |
| checked | [bool](#bool) |  |  |
| color | [string](#string) |  |  |






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






<a name="anytype.model.Range"></a>

### Range
General purpose structure, uses in Mark.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) |  |  |
| to | [int32](#int32) |  |  |






<a name="anytype.model.SmartBlock"></a>

### SmartBlock



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| type | [SmartBlock.Type](#anytype.model.SmartBlock.Type) |  |  |





 


<a name="anytype.model.Block.Align"></a>

### Block.Align


| Name | Number | Description |
| ---- | ------ | ----------- |
| AlignLeft | 0 |  |
| AlignCenter | 1 |  |
| AlignRight | 2 |  |



<a name="anytype.model.Block.Content.Dashboard.Style"></a>

### Block.Content.Dashboard.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| MainScreen | 0 |  |
| Archive | 1 |  |



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



<a name="anytype.model.Block.Content.File.Type"></a>

### Block.Content.File.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| None | 0 |  |
| File | 1 |  |
| Image | 2 |  |
| Video | 3 |  |



<a name="anytype.model.Block.Content.Layout.Style"></a>

### Block.Content.Layout.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Row | 0 |  |
| Column | 1 |  |
| Div | 2 |  |



<a name="anytype.model.Block.Content.Link.Style"></a>

### Block.Content.Link.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Page | 0 |  |
| Dataview | 1 |  |
| Dashboard | 2 |  |
| Archive | 3 | ... |



<a name="anytype.model.Block.Content.Page.Style"></a>

### Block.Content.Page.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 | Ordinary page, without additional fields |
| Task | 1 | Page with a task fields |
| Set | 2 | Page, that organize a set of blocks by a specific criterio |
| Profile | 3 |  |
| Breadcrumbs | 101 |  |



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



<a name="anytype.model.Block.Content.Text.Style"></a>

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



<a name="anytype.model.Block.Position"></a>

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



<a name="anytype.model.LinkPreview.Type"></a>

### LinkPreview.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Unknown | 0 |  |
| Page | 1 |  |
| Image | 2 |  |
| Text | 3 |  |



<a name="anytype.model.SmartBlock.Type"></a>

### SmartBlock.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| Dashboard | 0 |  |
| Page | 1 |  |
| Archive | 2 |  |
| Breadcrumbs | 3 |  |
| Dataview | 4 |  |


 

 

 



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

