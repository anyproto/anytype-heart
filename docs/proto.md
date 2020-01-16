# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [pb/protos/service/service.proto](#pb/protos/service/service.proto)
  
  
  
    - [ClientCommands](#anytype.ClientCommands)
  

- [pb/protos/commands.proto](#pb/protos/commands.proto)
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
    - [Rpc.Block.Download](#anytype.Rpc.Block.Download)
    - [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request)
    - [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response)
    - [Rpc.Block.Download.Response.Error](#anytype.Rpc.Block.Download.Response.Error)
    - [Rpc.Block.Get](#anytype.Rpc.Block.Get)
    - [Rpc.Block.Get.Marks](#anytype.Rpc.Block.Get.Marks)
    - [Rpc.Block.Get.Marks.Request](#anytype.Rpc.Block.Get.Marks.Request)
    - [Rpc.Block.Get.Marks.Response](#anytype.Rpc.Block.Get.Marks.Response)
    - [Rpc.Block.Get.Marks.Response.Error](#anytype.Rpc.Block.Get.Marks.Response.Error)
    - [Rpc.Block.History](#anytype.Rpc.Block.History)
    - [Rpc.Block.History.Move](#anytype.Rpc.Block.History.Move)
    - [Rpc.Block.History.Move.Request](#anytype.Rpc.Block.History.Move.Request)
    - [Rpc.Block.History.Move.Response](#anytype.Rpc.Block.History.Move.Response)
    - [Rpc.Block.History.Move.Response.Error](#anytype.Rpc.Block.History.Move.Response.Error)
    - [Rpc.Block.Merge](#anytype.Rpc.Block.Merge)
    - [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request)
    - [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response)
    - [Rpc.Block.Merge.Response.Error](#anytype.Rpc.Block.Merge.Response.Error)
    - [Rpc.Block.Open](#anytype.Rpc.Block.Open)
    - [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request)
    - [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response)
    - [Rpc.Block.Open.Response.Error](#anytype.Rpc.Block.Open.Response.Error)
    - [Rpc.Block.Paste](#anytype.Rpc.Block.Paste)
    - [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request)
    - [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response)
    - [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error)
    - [Rpc.Block.Replace](#anytype.Rpc.Block.Replace)
    - [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request)
    - [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response)
    - [Rpc.Block.Replace.Response.Error](#anytype.Rpc.Block.Replace.Response.Error)
    - [Rpc.Block.Set](#anytype.Rpc.Block.Set)
    - [Rpc.Block.Set.Fields](#anytype.Rpc.Block.Set.Fields)
    - [Rpc.Block.Set.Fields.Request](#anytype.Rpc.Block.Set.Fields.Request)
    - [Rpc.Block.Set.Fields.Response](#anytype.Rpc.Block.Set.Fields.Response)
    - [Rpc.Block.Set.Fields.Response.Error](#anytype.Rpc.Block.Set.Fields.Response.Error)
    - [Rpc.Block.Set.File](#anytype.Rpc.Block.Set.File)
    - [Rpc.Block.Set.File.Name](#anytype.Rpc.Block.Set.File.Name)
    - [Rpc.Block.Set.File.Name.Request](#anytype.Rpc.Block.Set.File.Name.Request)
    - [Rpc.Block.Set.File.Name.Response](#anytype.Rpc.Block.Set.File.Name.Response)
    - [Rpc.Block.Set.File.Name.Response.Error](#anytype.Rpc.Block.Set.File.Name.Response.Error)
    - [Rpc.Block.Set.Icon](#anytype.Rpc.Block.Set.Icon)
    - [Rpc.Block.Set.Icon.Name](#anytype.Rpc.Block.Set.Icon.Name)
    - [Rpc.Block.Set.Icon.Name.Request](#anytype.Rpc.Block.Set.Icon.Name.Request)
    - [Rpc.Block.Set.Icon.Name.Response](#anytype.Rpc.Block.Set.Icon.Name.Response)
    - [Rpc.Block.Set.Icon.Name.Response.Error](#anytype.Rpc.Block.Set.Icon.Name.Response.Error)
    - [Rpc.Block.Set.Image](#anytype.Rpc.Block.Set.Image)
    - [Rpc.Block.Set.Image.Name](#anytype.Rpc.Block.Set.Image.Name)
    - [Rpc.Block.Set.Image.Name.Request](#anytype.Rpc.Block.Set.Image.Name.Request)
    - [Rpc.Block.Set.Image.Name.Response](#anytype.Rpc.Block.Set.Image.Name.Response)
    - [Rpc.Block.Set.Image.Name.Response.Error](#anytype.Rpc.Block.Set.Image.Name.Response.Error)
    - [Rpc.Block.Set.Image.Width](#anytype.Rpc.Block.Set.Image.Width)
    - [Rpc.Block.Set.Image.Width.Request](#anytype.Rpc.Block.Set.Image.Width.Request)
    - [Rpc.Block.Set.Image.Width.Response](#anytype.Rpc.Block.Set.Image.Width.Response)
    - [Rpc.Block.Set.Image.Width.Response.Error](#anytype.Rpc.Block.Set.Image.Width.Response.Error)
    - [Rpc.Block.Set.IsArchived](#anytype.Rpc.Block.Set.IsArchived)
    - [Rpc.Block.Set.IsArchived.Request](#anytype.Rpc.Block.Set.IsArchived.Request)
    - [Rpc.Block.Set.IsArchived.Response](#anytype.Rpc.Block.Set.IsArchived.Response)
    - [Rpc.Block.Set.IsArchived.Response.Error](#anytype.Rpc.Block.Set.IsArchived.Response.Error)
    - [Rpc.Block.Set.Link](#anytype.Rpc.Block.Set.Link)
    - [Rpc.Block.Set.Link.TargetBlockId](#anytype.Rpc.Block.Set.Link.TargetBlockId)
    - [Rpc.Block.Set.Link.TargetBlockId.Request](#anytype.Rpc.Block.Set.Link.TargetBlockId.Request)
    - [Rpc.Block.Set.Link.TargetBlockId.Response](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response)
    - [Rpc.Block.Set.Link.TargetBlockId.Response.Error](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error)
    - [Rpc.Block.Set.Restrictions](#anytype.Rpc.Block.Set.Restrictions)
    - [Rpc.Block.Set.Restrictions.Request](#anytype.Rpc.Block.Set.Restrictions.Request)
    - [Rpc.Block.Set.Restrictions.Response](#anytype.Rpc.Block.Set.Restrictions.Response)
    - [Rpc.Block.Set.Restrictions.Response.Error](#anytype.Rpc.Block.Set.Restrictions.Response.Error)
    - [Rpc.Block.Set.Text](#anytype.Rpc.Block.Set.Text)
    - [Rpc.Block.Set.Text.BackgroundColor](#anytype.Rpc.Block.Set.Text.BackgroundColor)
    - [Rpc.Block.Set.Text.BackgroundColor.Request](#anytype.Rpc.Block.Set.Text.BackgroundColor.Request)
    - [Rpc.Block.Set.Text.BackgroundColor.Response](#anytype.Rpc.Block.Set.Text.BackgroundColor.Response)
    - [Rpc.Block.Set.Text.BackgroundColor.Response.Error](#anytype.Rpc.Block.Set.Text.BackgroundColor.Response.Error)
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
    - [Rpc.Block.Unlink](#anytype.Rpc.Block.Unlink)
    - [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request)
    - [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response)
    - [Rpc.Block.Unlink.Response.Error](#anytype.Rpc.Block.Unlink.Response.Error)
    - [Rpc.Block.Upload](#anytype.Rpc.Block.Upload)
    - [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request)
    - [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response)
    - [Rpc.Block.Upload.Response.Error](#anytype.Rpc.Block.Upload.Response.Error)
    - [Rpc.BlockList](#anytype.Rpc.BlockList)
    - [Rpc.BlockList.Duplicate](#anytype.Rpc.BlockList.Duplicate)
    - [Rpc.BlockList.Duplicate.Request](#anytype.Rpc.BlockList.Duplicate.Request)
    - [Rpc.BlockList.Duplicate.Response](#anytype.Rpc.BlockList.Duplicate.Response)
    - [Rpc.BlockList.Duplicate.Response.Error](#anytype.Rpc.BlockList.Duplicate.Response.Error)
    - [Rpc.BlockList.Move](#anytype.Rpc.BlockList.Move)
    - [Rpc.BlockList.Move.Request](#anytype.Rpc.BlockList.Move.Request)
    - [Rpc.BlockList.Move.Response](#anytype.Rpc.BlockList.Move.Response)
    - [Rpc.BlockList.Move.Response.Error](#anytype.Rpc.BlockList.Move.Response.Error)
    - [Rpc.BlockList.Set](#anytype.Rpc.BlockList.Set)
    - [Rpc.BlockList.Set.Fields](#anytype.Rpc.BlockList.Set.Fields)
    - [Rpc.BlockList.Set.Fields.Request](#anytype.Rpc.BlockList.Set.Fields.Request)
    - [Rpc.BlockList.Set.Fields.Request.BlockField](#anytype.Rpc.BlockList.Set.Fields.Request.BlockField)
    - [Rpc.BlockList.Set.Fields.Response](#anytype.Rpc.BlockList.Set.Fields.Response)
    - [Rpc.BlockList.Set.Fields.Response.Error](#anytype.Rpc.BlockList.Set.Fields.Response.Error)
    - [Rpc.BlockList.Set.Text](#anytype.Rpc.BlockList.Set.Text)
    - [Rpc.BlockList.Set.Text.BackgroundColor](#anytype.Rpc.BlockList.Set.Text.BackgroundColor)
    - [Rpc.BlockList.Set.Text.BackgroundColor.Request](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Request)
    - [Rpc.BlockList.Set.Text.BackgroundColor.Response](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response)
    - [Rpc.BlockList.Set.Text.BackgroundColor.Response.Error](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response.Error)
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
    - [Rpc.GetPageRecords](#anytype.Rpc.GetPageRecords)
    - [Rpc.GetPageRecords.Request](#anytype.Rpc.GetPageRecords.Request)
    - [Rpc.GetPageRecords.Response](#anytype.Rpc.GetPageRecords.Response)
    - [Rpc.GetPageRecords.Response.Error](#anytype.Rpc.GetPageRecords.Response.Error)
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
    - [Rpc.Log](#anytype.Rpc.Log)
    - [Rpc.Log.Send](#anytype.Rpc.Log.Send)
    - [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request)
    - [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response)
    - [Rpc.Log.Send.Response.Error](#anytype.Rpc.Log.Send.Response.Error)
    - [Rpc.Ping](#anytype.Rpc.Ping)
    - [Rpc.Ping.Request](#anytype.Rpc.Ping.Request)
    - [Rpc.Ping.Response](#anytype.Rpc.Ping.Response)
    - [Rpc.Ping.Response.Error](#anytype.Rpc.Ping.Response.Error)
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
    - [Rpc.Block.Close.Response.Error.Code](#anytype.Rpc.Block.Close.Response.Error.Code)
    - [Rpc.Block.Copy.Response.Error.Code](#anytype.Rpc.Block.Copy.Response.Error.Code)
    - [Rpc.Block.Create.Response.Error.Code](#anytype.Rpc.Block.Create.Response.Error.Code)
    - [Rpc.Block.Download.Response.Error.Code](#anytype.Rpc.Block.Download.Response.Error.Code)
    - [Rpc.Block.Get.Marks.Response.Error.Code](#anytype.Rpc.Block.Get.Marks.Response.Error.Code)
    - [Rpc.Block.History.Move.Response.Error.Code](#anytype.Rpc.Block.History.Move.Response.Error.Code)
    - [Rpc.Block.Merge.Response.Error.Code](#anytype.Rpc.Block.Merge.Response.Error.Code)
    - [Rpc.Block.Open.Response.Error.Code](#anytype.Rpc.Block.Open.Response.Error.Code)
    - [Rpc.Block.Paste.Response.Error.Code](#anytype.Rpc.Block.Paste.Response.Error.Code)
    - [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code)
    - [Rpc.Block.Set.Fields.Response.Error.Code](#anytype.Rpc.Block.Set.Fields.Response.Error.Code)
    - [Rpc.Block.Set.File.Name.Response.Error.Code](#anytype.Rpc.Block.Set.File.Name.Response.Error.Code)
    - [Rpc.Block.Set.Icon.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Icon.Name.Response.Error.Code)
    - [Rpc.Block.Set.Image.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Name.Response.Error.Code)
    - [Rpc.Block.Set.Image.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Image.Width.Response.Error.Code)
    - [Rpc.Block.Set.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.IsArchived.Response.Error.Code)
    - [Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response.Error.Code)
    - [Rpc.Block.Set.Restrictions.Response.Error.Code](#anytype.Rpc.Block.Set.Restrictions.Response.Error.Code)
    - [Rpc.Block.Set.Text.BackgroundColor.Response.Error.Code](#anytype.Rpc.Block.Set.Text.BackgroundColor.Response.Error.Code)
    - [Rpc.Block.Set.Text.Checked.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Checked.Response.Error.Code)
    - [Rpc.Block.Set.Text.Color.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Color.Response.Error.Code)
    - [Rpc.Block.Set.Text.Style.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Style.Response.Error.Code)
    - [Rpc.Block.Set.Text.Text.Response.Error.Code](#anytype.Rpc.Block.Set.Text.Text.Response.Error.Code)
    - [Rpc.Block.Set.Video.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Name.Response.Error.Code)
    - [Rpc.Block.Set.Video.Width.Response.Error.Code](#anytype.Rpc.Block.Set.Video.Width.Response.Error.Code)
    - [Rpc.Block.Split.Response.Error.Code](#anytype.Rpc.Block.Split.Response.Error.Code)
    - [Rpc.Block.Unlink.Response.Error.Code](#anytype.Rpc.Block.Unlink.Response.Error.Code)
    - [Rpc.Block.Upload.Response.Error.Code](#anytype.Rpc.Block.Upload.Response.Error.Code)
    - [Rpc.BlockList.Duplicate.Response.Error.Code](#anytype.Rpc.BlockList.Duplicate.Response.Error.Code)
    - [Rpc.BlockList.Move.Response.Error.Code](#anytype.Rpc.BlockList.Move.Response.Error.Code)
    - [Rpc.BlockList.Set.Fields.Response.Error.Code](#anytype.Rpc.BlockList.Set.Fields.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Color.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Color.Response.Error.Code)
    - [Rpc.BlockList.Set.Text.Style.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.Style.Response.Error.Code)
    - [Rpc.Config.Get.Response.Error.Code](#anytype.Rpc.Config.Get.Response.Error.Code)
    - [Rpc.ExternalDrop.Content.Response.Error.Code](#anytype.Rpc.ExternalDrop.Content.Response.Error.Code)
    - [Rpc.ExternalDrop.Files.Response.Error.Code](#anytype.Rpc.ExternalDrop.Files.Response.Error.Code)
    - [Rpc.GetPageRecords.Response.Error.Code](#anytype.Rpc.GetPageRecords.Response.Error.Code)
    - [Rpc.Ipfs.File.Get.Response.Error.Code](#anytype.Rpc.Ipfs.File.Get.Response.Error.Code)
    - [Rpc.Ipfs.Image.Get.Blob.Response.Error.Code](#anytype.Rpc.Ipfs.Image.Get.Blob.Response.Error.Code)
    - [Rpc.Ipfs.Image.Get.File.Response.Error.Code](#anytype.Rpc.Ipfs.Image.Get.File.Response.Error.Code)
    - [Rpc.Log.Send.Request.Level](#anytype.Rpc.Log.Send.Request.Level)
    - [Rpc.Log.Send.Response.Error.Code](#anytype.Rpc.Log.Send.Response.Error.Code)
    - [Rpc.Ping.Response.Error.Code](#anytype.Rpc.Ping.Response.Error.Code)
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
    - [Event.Block.Set.ChildrenIds](#anytype.Event.Block.Set.ChildrenIds)
    - [Event.Block.Set.Fields](#anytype.Event.Block.Set.Fields)
    - [Event.Block.Set.File](#anytype.Event.Block.Set.File)
    - [Event.Block.Set.File.Hash](#anytype.Event.Block.Set.File.Hash)
    - [Event.Block.Set.File.Mime](#anytype.Event.Block.Set.File.Mime)
    - [Event.Block.Set.File.Name](#anytype.Event.Block.Set.File.Name)
    - [Event.Block.Set.File.Size](#anytype.Event.Block.Set.File.Size)
    - [Event.Block.Set.File.State](#anytype.Event.Block.Set.File.State)
    - [Event.Block.Set.File.Type](#anytype.Event.Block.Set.File.Type)
    - [Event.Block.Set.File.Width](#anytype.Event.Block.Set.File.Width)
    - [Event.Block.Set.Icon](#anytype.Event.Block.Set.Icon)
    - [Event.Block.Set.Icon.Name](#anytype.Event.Block.Set.Icon.Name)
    - [Event.Block.Set.IsArchived](#anytype.Event.Block.Set.IsArchived)
    - [Event.Block.Set.Link](#anytype.Event.Block.Set.Link)
    - [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields)
    - [Event.Block.Set.Link.IsArchived](#anytype.Event.Block.Set.Link.IsArchived)
    - [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style)
    - [Event.Block.Set.Restrictions](#anytype.Event.Block.Set.Restrictions)
    - [Event.Block.Set.Text](#anytype.Event.Block.Set.Text)
    - [Event.Block.Set.Text.BackgroundColor](#anytype.Event.Block.Set.Text.BackgroundColor)
    - [Event.Block.Set.Text.Checked](#anytype.Event.Block.Set.Text.Checked)
    - [Event.Block.Set.Text.Color](#anytype.Event.Block.Set.Text.Color)
    - [Event.Block.Set.Text.Marks](#anytype.Event.Block.Set.Text.Marks)
    - [Event.Block.Set.Text.Style](#anytype.Event.Block.Set.Text.Style)
    - [Event.Block.Set.Text.Text](#anytype.Event.Block.Set.Text.Text)
    - [Event.Block.Show](#anytype.Event.Block.Show)
    - [Event.Message](#anytype.Event.Message)
    - [Event.Ping](#anytype.Event.Ping)
    - [Event.User](#anytype.Event.User)
    - [Event.User.Block](#anytype.Event.User.Block)
    - [Event.User.Block.Join](#anytype.Event.User.Block.Join)
    - [Event.User.Block.Left](#anytype.Event.User.Block.Left)
    - [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange)
    - [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange)
  
  
  
  

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
    - [Image](#anytype.model.Image)
    - [PageRecord](#anytype.model.PageRecord)
    - [Range](#anytype.model.Range)
    - [Video](#anytype.model.Video)
  
    - [Block.Content.Dashboard.Style](#anytype.model.Block.Content.Dashboard.Style)
    - [Block.Content.File.State](#anytype.model.Block.Content.File.State)
    - [Block.Content.File.Type](#anytype.model.Block.Content.File.Type)
    - [Block.Content.Layout.Style](#anytype.model.Block.Content.Layout.Style)
    - [Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style)
    - [Block.Content.Page.Style](#anytype.model.Block.Content.Page.Style)
    - [Block.Content.Text.Mark.Type](#anytype.model.Block.Content.Text.Mark.Type)
    - [Block.Content.Text.Style](#anytype.model.Block.Content.Text.Style)
    - [Block.Position](#anytype.model.Block.Position)
    - [Image.Size](#anytype.model.Image.Size)
    - [Image.Style](#anytype.model.Image.Style)
    - [Video.Size](#anytype.model.Video.Size)
  
  
  

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
| ImageGetBlob | [Rpc.Ipfs.Image.Get.Blob.Request](#anytype.Rpc.Ipfs.Image.Get.Blob.Request) | [Rpc.Ipfs.Image.Get.Blob.Response](#anytype.Rpc.Ipfs.Image.Get.Blob.Response) |  |
| VersionGet | [Rpc.Version.Get.Request](#anytype.Rpc.Version.Get.Request) | [Rpc.Version.Get.Response](#anytype.Rpc.Version.Get.Response) |  |
| LogSend | [Rpc.Log.Send.Request](#anytype.Rpc.Log.Send.Request) | [Rpc.Log.Send.Response](#anytype.Rpc.Log.Send.Response) |  |
| ConfigGet | [Rpc.Config.Get.Request](#anytype.Rpc.Config.Get.Request) | [Rpc.Config.Get.Response](#anytype.Rpc.Config.Get.Response) |  |
| ExternalDropFiles | [Rpc.ExternalDrop.Files.Request](#anytype.Rpc.ExternalDrop.Files.Request) | [Rpc.ExternalDrop.Files.Response](#anytype.Rpc.ExternalDrop.Files.Response) |  |
| ExternalDropContent | [Rpc.ExternalDrop.Content.Request](#anytype.Rpc.ExternalDrop.Content.Request) | [Rpc.ExternalDrop.Content.Response](#anytype.Rpc.ExternalDrop.Content.Response) |  |
| GetPageRecords | [Rpc.GetPageRecords.Request](#anytype.Rpc.GetPageRecords.Request) | [Rpc.GetPageRecords.Response](#anytype.Rpc.GetPageRecords.Response) |  |
| BlockUpload | [Rpc.Block.Upload.Request](#anytype.Rpc.Block.Upload.Request) | [Rpc.Block.Upload.Response](#anytype.Rpc.Block.Upload.Response) |  |
| BlockReplace | [Rpc.Block.Replace.Request](#anytype.Rpc.Block.Replace.Request) | [Rpc.Block.Replace.Response](#anytype.Rpc.Block.Replace.Response) |  |
| BlockOpen | [Rpc.Block.Open.Request](#anytype.Rpc.Block.Open.Request) | [Rpc.Block.Open.Response](#anytype.Rpc.Block.Open.Response) |  |
| BlockCreate | [Rpc.Block.Create.Request](#anytype.Rpc.Block.Create.Request) | [Rpc.Block.Create.Response](#anytype.Rpc.Block.Create.Response) |  |
| BlockUnlink | [Rpc.Block.Unlink.Request](#anytype.Rpc.Block.Unlink.Request) | [Rpc.Block.Unlink.Response](#anytype.Rpc.Block.Unlink.Response) |  |
| BlockClose | [Rpc.Block.Close.Request](#anytype.Rpc.Block.Close.Request) | [Rpc.Block.Close.Response](#anytype.Rpc.Block.Close.Response) |  |
| BlockDownload | [Rpc.Block.Download.Request](#anytype.Rpc.Block.Download.Request) | [Rpc.Block.Download.Response](#anytype.Rpc.Block.Download.Response) |  |
| BlockGetMarks | [Rpc.Block.Get.Marks.Request](#anytype.Rpc.Block.Get.Marks.Request) | [Rpc.Block.Get.Marks.Response](#anytype.Rpc.Block.Get.Marks.Response) |  |
| BlockHistoryMove | [Rpc.Block.History.Move.Request](#anytype.Rpc.Block.History.Move.Request) | [Rpc.Block.History.Move.Response](#anytype.Rpc.Block.History.Move.Response) |  |
| BlockSetFields | [Rpc.Block.Set.Fields.Request](#anytype.Rpc.Block.Set.Fields.Request) | [Rpc.Block.Set.Fields.Response](#anytype.Rpc.Block.Set.Fields.Response) |  |
| BlockSetRestrictions | [Rpc.Block.Set.Restrictions.Request](#anytype.Rpc.Block.Set.Restrictions.Request) | [Rpc.Block.Set.Restrictions.Response](#anytype.Rpc.Block.Set.Restrictions.Response) |  |
| BlockSetIsArchived | [Rpc.Block.Set.IsArchived.Request](#anytype.Rpc.Block.Set.IsArchived.Request) | [Rpc.Block.Set.IsArchived.Response](#anytype.Rpc.Block.Set.IsArchived.Response) |  |
| BlockListMove | [Rpc.BlockList.Move.Request](#anytype.Rpc.BlockList.Move.Request) | [Rpc.BlockList.Move.Response](#anytype.Rpc.BlockList.Move.Response) |  |
| BlockListSetFields | [Rpc.BlockList.Set.Fields.Request](#anytype.Rpc.BlockList.Set.Fields.Request) | [Rpc.BlockList.Set.Fields.Response](#anytype.Rpc.BlockList.Set.Fields.Response) |  |
| BlockListSetTextStyle | [Rpc.BlockList.Set.Text.Style.Request](#anytype.Rpc.BlockList.Set.Text.Style.Request) | [Rpc.BlockList.Set.Text.Style.Response](#anytype.Rpc.BlockList.Set.Text.Style.Response) |  |
| BlockListDuplicate | [Rpc.BlockList.Duplicate.Request](#anytype.Rpc.BlockList.Duplicate.Request) | [Rpc.BlockList.Duplicate.Response](#anytype.Rpc.BlockList.Duplicate.Response) |  |
| BlockSetTextText | [Rpc.Block.Set.Text.Text.Request](#anytype.Rpc.Block.Set.Text.Text.Request) | [Rpc.Block.Set.Text.Text.Response](#anytype.Rpc.Block.Set.Text.Text.Response) |  |
| BlockSetTextColor | [Rpc.Block.Set.Text.Color.Request](#anytype.Rpc.Block.Set.Text.Color.Request) | [Rpc.Block.Set.Text.Color.Response](#anytype.Rpc.Block.Set.Text.Color.Response) |  |
| BlockListSetTextColor | [Rpc.BlockList.Set.Text.Color.Request](#anytype.Rpc.BlockList.Set.Text.Color.Request) | [Rpc.BlockList.Set.Text.Color.Response](#anytype.Rpc.BlockList.Set.Text.Color.Response) |  |
| BlockSetTextBackgroundColor | [Rpc.Block.Set.Text.BackgroundColor.Request](#anytype.Rpc.Block.Set.Text.BackgroundColor.Request) | [Rpc.Block.Set.Text.BackgroundColor.Response](#anytype.Rpc.Block.Set.Text.BackgroundColor.Response) |  |
| BlockListSetTextBackgroundColor | [Rpc.BlockList.Set.Text.BackgroundColor.Request](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Request) | [Rpc.BlockList.Set.Text.BackgroundColor.Response](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response) |  |
| BlockSetTextStyle | [Rpc.Block.Set.Text.Style.Request](#anytype.Rpc.Block.Set.Text.Style.Request) | [Rpc.Block.Set.Text.Style.Response](#anytype.Rpc.Block.Set.Text.Style.Response) |  |
| BlockSetTextChecked | [Rpc.Block.Set.Text.Checked.Request](#anytype.Rpc.Block.Set.Text.Checked.Request) | [Rpc.Block.Set.Text.Checked.Response](#anytype.Rpc.Block.Set.Text.Checked.Response) |  |
| BlockSplit | [Rpc.Block.Split.Request](#anytype.Rpc.Block.Split.Request) | [Rpc.Block.Split.Response](#anytype.Rpc.Block.Split.Response) |  |
| BlockMerge | [Rpc.Block.Merge.Request](#anytype.Rpc.Block.Merge.Request) | [Rpc.Block.Merge.Response](#anytype.Rpc.Block.Merge.Response) |  |
| BlockCopy | [Rpc.Block.Copy.Request](#anytype.Rpc.Block.Copy.Request) | [Rpc.Block.Copy.Response](#anytype.Rpc.Block.Copy.Response) |  |
| BlockPaste | [Rpc.Block.Paste.Request](#anytype.Rpc.Block.Paste.Request) | [Rpc.Block.Paste.Response](#anytype.Rpc.Block.Paste.Response) |  |
| BlockSetFileName | [Rpc.Block.Set.File.Name.Request](#anytype.Rpc.Block.Set.File.Name.Request) | [Rpc.Block.Set.File.Name.Response](#anytype.Rpc.Block.Set.File.Name.Response) |  |
| BlockSetImageName | [Rpc.Block.Set.Image.Name.Request](#anytype.Rpc.Block.Set.Image.Name.Request) | [Rpc.Block.Set.Image.Name.Response](#anytype.Rpc.Block.Set.Image.Name.Response) |  |
| BlockSetImageWidth | [Rpc.Block.Set.Image.Width.Request](#anytype.Rpc.Block.Set.Image.Width.Request) | [Rpc.Block.Set.Image.Width.Response](#anytype.Rpc.Block.Set.Image.Width.Response) |  |
| BlockSetVideoName | [Rpc.Block.Set.Video.Name.Request](#anytype.Rpc.Block.Set.Video.Name.Request) | [Rpc.Block.Set.Video.Name.Response](#anytype.Rpc.Block.Set.Video.Name.Response) |  |
| BlockSetVideoWidth | [Rpc.Block.Set.Video.Width.Request](#anytype.Rpc.Block.Set.Video.Width.Request) | [Rpc.Block.Set.Video.Width.Response](#anytype.Rpc.Block.Set.Video.Width.Response) |  |
| BlockSetIconName | [Rpc.Block.Set.Icon.Name.Request](#anytype.Rpc.Block.Set.Icon.Name.Request) | [Rpc.Block.Set.Icon.Name.Response](#anytype.Rpc.Block.Set.Icon.Name.Response) |  |
| BlockSetLinkTargetBlockId | [Rpc.Block.Set.Link.TargetBlockId.Request](#anytype.Rpc.Block.Set.Link.TargetBlockId.Request) | [Rpc.Block.Set.Link.TargetBlockId.Response](#anytype.Rpc.Block.Set.Link.TargetBlockId.Response) |  |
| Ping | [Rpc.Ping.Request](#anytype.Rpc.Ping.Request) | [Rpc.Ping.Response](#anytype.Rpc.Ping.Response) |  |

 



<a name="pb/protos/commands.proto"></a>
<p align="right"><a href="#top">Top</a></p>

## pb/protos/commands.proto



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






<a name="anytype.Rpc.Block"></a>

### Rpc.Block
Namespace, that agregates subtopics and actions, that relates to blocks.






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
| blockIds | [string](#string) | repeated |  |






<a name="anytype.Rpc.Block.Copy.Response"></a>

### Rpc.Block.Copy.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Copy.Response.Error](#anytype.Rpc.Block.Copy.Response.Error) |  |  |
| clipboardText | [string](#string) |  |  |
| clipboardHtml | [string](#string) |  | string clipboardAny = 4; Client already knows blockIds |






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






<a name="anytype.Rpc.Block.History"></a>

### Rpc.Block.History
Block history: switch between versions (lib context: switch block head), move forward or backward
**Example scenario**
1. User -&gt; MacOS Front: CMD&#43;Z
2. Front -&gt; MW: Rpc.Block.History.Move.Request(blockId, false)
3. MW -&gt; Lib: ?? TODO
4. Lib: switches current block header to a previous one
5. Lib -&gt; MW: prev version of block
6. MW -&gt; Front: BlockShow(block.prevVersion)






<a name="anytype.Rpc.Block.History.Move"></a>

### Rpc.Block.History.Move







<a name="anytype.Rpc.Block.History.Move.Request"></a>

### Rpc.Block.History.Move.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  | id of the context block |
| blockId | [string](#string) |  |  |
| moveForward | [bool](#bool) |  | Move direction. If true, move forward |






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
| textSlot | [string](#string) |  |  |
| htmlSlot | [string](#string) |  |  |
| anySlot | [model.Block](#anytype.model.Block) | repeated |  |






<a name="anytype.Rpc.Block.Paste.Response"></a>

### Rpc.Block.Paste.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Paste.Response.Error](#anytype.Rpc.Block.Paste.Response.Error) |  |  |






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






<a name="anytype.Rpc.Block.Replace.Response.Error"></a>

### Rpc.Block.Replace.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Replace.Response.Error.Code](#anytype.Rpc.Block.Replace.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set"></a>

### Rpc.Block.Set







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






<a name="anytype.Rpc.Block.Set.Icon"></a>

### Rpc.Block.Set.Icon







<a name="anytype.Rpc.Block.Set.Icon.Name"></a>

### Rpc.Block.Set.Icon.Name







<a name="anytype.Rpc.Block.Set.Icon.Name.Request"></a>

### Rpc.Block.Set.Icon.Name.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| name | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Icon.Name.Response"></a>

### Rpc.Block.Set.Icon.Name.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Icon.Name.Response.Error](#anytype.Rpc.Block.Set.Icon.Name.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Icon.Name.Response.Error"></a>

### Rpc.Block.Set.Icon.Name.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Icon.Name.Response.Error.Code](#anytype.Rpc.Block.Set.Icon.Name.Response.Error.Code) |  |  |
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






<a name="anytype.Rpc.Block.Set.IsArchived"></a>

### Rpc.Block.Set.IsArchived







<a name="anytype.Rpc.Block.Set.IsArchived.Request"></a>

### Rpc.Block.Set.IsArchived.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| IsArchived | [bool](#bool) |  |  |






<a name="anytype.Rpc.Block.Set.IsArchived.Response"></a>

### Rpc.Block.Set.IsArchived.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.IsArchived.Response.Error](#anytype.Rpc.Block.Set.IsArchived.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.IsArchived.Response.Error"></a>

### Rpc.Block.Set.IsArchived.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.IsArchived.Response.Error.Code](#anytype.Rpc.Block.Set.IsArchived.Response.Error.Code) |  |  |
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







<a name="anytype.Rpc.Block.Set.Text.BackgroundColor"></a>

### Rpc.Block.Set.Text.BackgroundColor







<a name="anytype.Rpc.Block.Set.Text.BackgroundColor.Request"></a>

### Rpc.Block.Set.Text.BackgroundColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockId | [string](#string) |  |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.Block.Set.Text.BackgroundColor.Response"></a>

### Rpc.Block.Set.Text.BackgroundColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.Block.Set.Text.BackgroundColor.Response.Error](#anytype.Rpc.Block.Set.Text.BackgroundColor.Response.Error) |  |  |






<a name="anytype.Rpc.Block.Set.Text.BackgroundColor.Response.Error"></a>

### Rpc.Block.Set.Text.BackgroundColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.Block.Set.Text.BackgroundColor.Response.Error.Code](#anytype.Rpc.Block.Set.Text.BackgroundColor.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






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
| cursorPosition | [int32](#int32) |  |  |






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






<a name="anytype.Rpc.BlockList.Set"></a>

### Rpc.BlockList.Set







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







<a name="anytype.Rpc.BlockList.Set.Text.BackgroundColor"></a>

### Rpc.BlockList.Set.Text.BackgroundColor







<a name="anytype.Rpc.BlockList.Set.Text.BackgroundColor.Request"></a>

### Rpc.BlockList.Set.Text.BackgroundColor.Request



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| contextId | [string](#string) |  |  |
| blockIds | [string](#string) | repeated |  |
| color | [string](#string) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response"></a>

### Rpc.BlockList.Set.Text.BackgroundColor.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.BlockList.Set.Text.BackgroundColor.Response.Error](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response.Error) |  |  |






<a name="anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response.Error"></a>

### Rpc.BlockList.Set.Text.BackgroundColor.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.BlockList.Set.Text.BackgroundColor.Response.Error.Code](#anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response.Error.Code) |  |  |
| description | [string](#string) |  |  |






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
| focusedBlockId | [string](#string) |  | can be null |
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






<a name="anytype.Rpc.GetPageRecords"></a>

### Rpc.GetPageRecords







<a name="anytype.Rpc.GetPageRecords.Request"></a>

### Rpc.GetPageRecords.Request







<a name="anytype.Rpc.GetPageRecords.Response"></a>

### Rpc.GetPageRecords.Response



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| error | [Rpc.GetPageRecords.Response.Error](#anytype.Rpc.GetPageRecords.Response.Error) |  |  |
| pageRecords | [model.PageRecord](#anytype.model.PageRecord) | repeated |  |






<a name="anytype.Rpc.GetPageRecords.Response.Error"></a>

### Rpc.GetPageRecords.Response.Error



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [Rpc.GetPageRecords.Response.Error.Code](#anytype.Rpc.GetPageRecords.Response.Error.Code) |  |  |
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
| version | [string](#string) |  | BuildDate, GitCommit, GitBranch, GitState |






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
| FAILED_TO_STOP_SEARCHER_NODE | 106 |  |



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



<a name="anytype.Rpc.Block.Download.Response.Error.Code"></a>

### Rpc.Block.Download.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Get.Marks.Response.Error.Code"></a>

### Rpc.Block.Get.Marks.Response.Error.Code


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



<a name="anytype.Rpc.Block.Set.Icon.Name.Response.Error.Code"></a>

### Rpc.Block.Set.Icon.Name.Response.Error.Code


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



<a name="anytype.Rpc.Block.Set.IsArchived.Response.Error.Code"></a>

### Rpc.Block.Set.IsArchived.Response.Error.Code


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



<a name="anytype.Rpc.Block.Set.Restrictions.Response.Error.Code"></a>

### Rpc.Block.Set.Restrictions.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.Block.Set.Text.BackgroundColor.Response.Error.Code"></a>

### Rpc.Block.Set.Text.BackgroundColor.Response.Error.Code


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



<a name="anytype.Rpc.BlockList.Set.Fields.Response.Error.Code"></a>

### Rpc.BlockList.Set.Fields.Response.Error.Code


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL | 0 |  |
| UNKNOWN_ERROR | 1 |  |
| BAD_INPUT | 2 | ... |



<a name="anytype.Rpc.BlockList.Set.Text.BackgroundColor.Response.Error.Code"></a>

### Rpc.BlockList.Set.Text.BackgroundColor.Response.Error.Code


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



<a name="anytype.Rpc.GetPageRecords.Response.Error.Code"></a>

### Rpc.GetPageRecords.Response.Error.Code


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
| blockId | [string](#string) |  | TODO: repeated string blockIds? |






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







<a name="anytype.Event.Block.Set.ChildrenIds"></a>

### Event.Block.Set.ChildrenIds



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| childrenIds | [string](#string) | repeated |  |






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






<a name="anytype.Event.Block.Set.Icon"></a>

### Event.Block.Set.Icon



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [Event.Block.Set.Icon.Name](#anytype.Event.Block.Set.Icon.Name) |  |  |






<a name="anytype.Event.Block.Set.Icon.Name"></a>

### Event.Block.Set.Icon.Name



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






<a name="anytype.Event.Block.Set.IsArchived"></a>

### Event.Block.Set.IsArchived



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| IsArchived | [bool](#bool) |  |  |






<a name="anytype.Event.Block.Set.Link"></a>

### Event.Block.Set.Link



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| style | [Event.Block.Set.Link.Style](#anytype.Event.Block.Set.Link.Style) |  |  |
| fields | [Event.Block.Set.Link.Fields](#anytype.Event.Block.Set.Link.Fields) |  |  |
| isArchived | [Event.Block.Set.Link.IsArchived](#anytype.Event.Block.Set.Link.IsArchived) |  |  |






<a name="anytype.Event.Block.Set.Link.Fields"></a>

### Event.Block.Set.Link.Fields



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |






<a name="anytype.Event.Block.Set.Link.IsArchived"></a>

### Event.Block.Set.Link.IsArchived



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [bool](#bool) |  |  |






<a name="anytype.Event.Block.Set.Link.Style"></a>

### Event.Block.Set.Link.Style



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [model.Block.Content.Link.Style](#anytype.model.Block.Content.Link.Style) |  |  |






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
| backgroundColor | [Event.Block.Set.Text.BackgroundColor](#anytype.Event.Block.Set.Text.BackgroundColor) |  |  |






<a name="anytype.Event.Block.Set.Text.BackgroundColor"></a>

### Event.Block.Set.Text.BackgroundColor



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  |  |






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
| blockSetIsArchived | [Event.Block.Set.IsArchived](#anytype.Event.Block.Set.IsArchived) |  |  |
| blockSetText | [Event.Block.Set.Text](#anytype.Event.Block.Set.Text) |  |  |
| blockSetFile | [Event.Block.Set.File](#anytype.Event.Block.Set.File) |  |  |
| blockSetIcon | [Event.Block.Set.Icon](#anytype.Event.Block.Set.Icon) |  |  |
| blockSetLink | [Event.Block.Set.Link](#anytype.Event.Block.Set.Link) |  |  |
| blockShow | [Event.Block.Show](#anytype.Event.Block.Show) |  |  |
| userBlockJoin | [Event.User.Block.Join](#anytype.Event.User.Block.Join) |  |  |
| userBlockLeft | [Event.User.Block.Left](#anytype.Event.User.Block.Left) |  |  |
| userBlockSelectRange | [Event.User.Block.SelectRange](#anytype.Event.User.Block.SelectRange) |  |  |
| userBlockTextRange | [Event.User.Block.TextRange](#anytype.Event.User.Block.TextRange) |  |  |
| ping | [Event.Ping](#anytype.Event.Ping) |  |  |






<a name="anytype.Event.Ping"></a>

### Event.Ping



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| index | [int32](#int32) |  |  |






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
| image | [Image](#anytype.model.Image) |  | Image of the avatar. Contains hash and size |
| color | [string](#string) |  | Color of the avatar, if no image |






<a name="anytype.model.Block"></a>

### Block



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| fields | [google.protobuf.Struct](#google.protobuf.Struct) |  |  |
| restrictions | [Block.Restrictions](#anytype.model.Block.Restrictions) |  |  |
| childrenIds | [string](#string) | repeated |  |
| isArchived | [bool](#bool) |  |  |
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

Model.Link.Preview preview = 1;






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






<a name="anytype.model.Block.Content.File"></a>

### Block.Content.File



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| localFilePath | [string](#string) |  |  |
| previewFilePath | [string](#string) |  |  |
| state | [Block.Content.File.State](#anytype.model.Block.Content.File.State) |  |  |
| type | [Block.Content.File.Type](#anytype.model.Block.Content.File.Type) |  |  |
| size | [int64](#int64) |  |  |
| addedAt | [int64](#int64) |  |  |






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
| isArchived | [bool](#bool) |  |  |






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
| backgroundColor | [string](#string) |  |  |






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






<a name="anytype.model.Image"></a>

### Image



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| sizes | [Image.Size](#anytype.model.Image.Size) | repeated |  |
| style | [Image.Style](#anytype.model.Image.Style) |  |  |






<a name="anytype.model.PageRecord"></a>

### PageRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| name | [string](#string) |  |  |
| icon | [string](#string) |  |  |






<a name="anytype.model.Range"></a>

### Range
General purpose structure, uses in Mark.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| from | [int32](#int32) |  |  |
| to | [int32](#int32) |  |  |






<a name="anytype.model.Video"></a>

### Video



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| sizes | [Video.Size](#anytype.model.Video.Size) | repeated |  |





 


<a name="anytype.model.Block.Content.Dashboard.Style"></a>

### Block.Content.Dashboard.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| MainScreen | 0 |  |
| Archive | 1 |  |



<a name="anytype.model.Block.Content.File.State"></a>

### Block.Content.File.State


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 | There is no file and preview, it&#39;s an empty block, that waits files. |
| Uploading | 1 | There is still no file/preview, but file already uploading |
| PreviewDownloaded | 2 | File exists, preview downloaded, but file is not. |
| Downloading | 3 | File exists, preview downloaded, but file downloading |
| Done | 4 | File and preview downloaded |



<a name="anytype.model.Block.Content.File.Type"></a>

### Block.Content.File.Type


| Name | Number | Description |
| ---- | ------ | ----------- |
| File | 0 |  |
| Image | 1 |  |
| Video | 2 |  |



<a name="anytype.model.Block.Content.Layout.Style"></a>

### Block.Content.Layout.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Row | 0 |  |
| Column | 1 |  |



<a name="anytype.model.Block.Content.Link.Style"></a>

### Block.Content.Link.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Page | 0 |  |
| Dataview | 1 | ... |



<a name="anytype.model.Block.Content.Page.Style"></a>

### Block.Content.Page.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Empty | 0 | Ordinary page, without additional fields |
| Task | 1 | Page with a task fields |
| Set | 2 | Page, that organize a set of blocks by a specific criterio |



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



<a name="anytype.model.Image.Size"></a>

### Image.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| Large | 0 |  |
| Small | 1 |  |
| Thumb | 2 |  |



<a name="anytype.model.Image.Style"></a>

### Image.Style


| Name | Number | Description |
| ---- | ------ | ----------- |
| Picture | 0 |  |
| File | 1 |  |



<a name="anytype.model.Video.Size"></a>

### Video.Size


| Name | Number | Description |
| ---- | ------ | ----------- |
| SD360p | 0 |  |
| SD480p | 1 |  |
| HD720p | 2 |  |
| HD1080p | 3 |  |
| UHD1440p | 4 |  |
| UHD2160p | 5 |  |


 

 

 



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

