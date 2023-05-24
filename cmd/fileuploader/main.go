//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"

	_ "net/http/pprof"
)

func main() {
	if len(os.Args) > 2 {
		invite := os.Args[1]
		path := os.Args[2]
		var files []string
		mw := core.New()
		rootPath, _ := ioutil.TempDir(os.TempDir(), "anytype_*")

		defer os.RemoveAll(rootPath)

		mw.EventSender = event.NewCallbackSender(func(event *pb.Event) {
			// nothing to do
		})

		ctx := context.Background()

		_ = mw.WalletCreate(ctx, &pb.RpcWalletCreateRequest{RootPath: rootPath})
		_ = mw.AccountCreate(ctx, &pb.RpcAccountCreateRequest{Name: "profile", AlphaInviteCode: invite})

		var cids []string
		err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			files = append(files, path)
			if info.IsDir() {
				return nil
			}
			if strings.HasSuffix(strings.ToLower(path), "ds_store") {
				return nil
			}

			resp := mw.FileUpload(ctx, &pb.RpcFileUploadRequest{LocalPath: path, DisableEncryption: true})
			if int(resp.Error.Code) != 0 {
				return fmt.Errorf(resp.Error.Description)
			}
			cids = append(cids, resp.Hash)
			fmt.Println(path)
			fmt.Println(resp.Hash)
			return nil
		})
		if err != nil {
			panic(err.Error())
		}

		// fs := mw.GetApp().MustComponent(pin.CName).(pin.FilePinService)
		// for {
		// 	r := fs.PinStatus(cids...)
		// 	var pinned int
		// 	var failed int
		// 	var inprog int
		// 	for k, f := range r {
		// 		if f.Status == pb2.PinStatus_Done {
		// 			pinned++
		// 		}
		// 		if f.Status == pb2.PinStatus_Failed {
		// 			failed++
		// 		}
		// 		if f.Status == pb2.PinStatus_Queued {
		// 			fmt.Printf("%s still in progress\n", k)
		// 			inprog++
		// 		}
		// 	}
		// 	fmt.Printf("%d pinned, %d in-progress, %d failed from %d\n", pinned, inprog, failed, len(r))
		//
		// 	if len(r) == len(cids) {
		// 		fmt.Println("all pinned")
		// 		os.Exit(0)
		// 	}
		// 	time.Sleep(time.Second * 10)
		// }
	}
}
