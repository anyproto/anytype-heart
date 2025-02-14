//go:build cgo

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/goccy/go-graphviz"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/debug/exporter"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	file        = flag.String("f", "", "path to debug file")
	fromRoot    = flag.Bool("r", true, "build from root of the tree")
	makeJson    = flag.Bool("j", true, "generate json file")
	makeTree    = flag.Bool("t", true, "generate graphviz file")
	printState  = flag.Bool("s", true, "print result state debug")
	changeIdx   = flag.Int("c", -1, "build tree before given index and print change")
	objectStore = flag.Bool("o", true, "show object store info")
	fileHashes  = flag.Bool("h", true, "show file hashes in state")
)

func main() {
	flag.Parse()
	if *file == "" {
		flag.PrintDefaults()
		return
	}
	fmt.Println("opening file...")
	var (
		st  = time.Now()
		ctx = context.Background()
	)
	res, err := exporter.ImportStorage(ctx, *file)
	if err != nil {
		log.Fatal("can't import the tree:", err)
	}
	defer res.Store.Close()
	objectTree, err := res.CreateReadableTree(*fromRoot, "")
	if err != nil {
		log.Fatal("can't create readable tree:", err)
	}
	fmt.Printf("open archive done in %.1fs\n", time.Since(st).Seconds())
	importer := exporter.NewTreeImporter(objectTree)
	if *makeJson {
		treeJson, err := importer.Json()
		if err != nil {
			log.Fatal("can't build json:", err)
		}
		res, err := json.MarshalIndent(treeJson, "", "  ")
		if err != nil {
			log.Fatal("can't marshall json:", err)
		}
		tf, err := ioutil.TempFile("", "tree_*.json")
		if err != nil {
			log.Fatal("can't create temp file:", err)
		}
		fmt.Println("tree json file:", tf.Name())
		tf.Write(res)
		tf.Close()
	}

	if *changeIdx != -1 {
		ch, err := importer.ChangeAt(*changeIdx)
		if err != nil {
			log.Fatal("can't get the change in tree: ", err)
		}
		fmt.Println("Change:")
		fmt.Println(pbtypes.Sprint(ch.Model))
		objectTree, err = res.CreateReadableTree(*fromRoot, ch.Id)
		if err != nil {
			log.Fatal("can't create readable tree:", err)
		}
		importer = exporter.NewTreeImporter(objectTree)
	}
	ot := importer.ObjectTree()
	di, err := ot.Debug(state.ChangeParser{})
	if err != nil {
		log.Fatal("can't get debug info from tree", err)
	}
	fmt.Printf("Tree root:\t%s\nTree len:\t%d\nTree heads:\t%s\n",
		ot.Root().Id,
		di.TreeLen,
		strings.Join(di.Heads, ","))

	if *printState {
		fmt.Println("Building state...")
		stt := time.Now()
		s, err := importer.State()
		if err != nil {
			log.Fatal("can't build state:", err)
		}
		dur := time.Since(stt)
		fmt.Println(s.StringDebug())

		payload := &model.ObjectChangePayload{}
		err = proto.Unmarshal(ot.ChangeInfo().ChangePayload, payload)
		if err != nil {
			return
		}
		sbt := smartblock.SmartBlockType(payload.SmartBlockType)
		fmt.Printf("Smarblock type:\t%v\n", sbt.ToProto())
		if *fileHashes {
			fmt.Println("File keys:")
			for _, fk := range s.GetAndUnsetFileKeys() {
				fmt.Printf("\t%s: %d\n", fk.Hash, len(fk.Keys))
			}
		}
		fmt.Println("state building time:", dur)
	}

	if *objectStore {
		fmt.Println("fetch object store info..")
		f, err := os.Open(filepath.Join(res.FolderPath, "localstore.json"))
		if err != nil {
			log.Fatal("can't open objectStore info:", err)
		}
		info := &model.ObjectInfo{}
		if err = jsonpb.Unmarshal(f, info); err != nil {
			log.Fatal("can't unmarshal objectStore info:", err)
		}
		defer f.Close()
		fmt.Println(pbtypes.Sprint(info))
	}

	if *makeTree {
		gvo, err := graphviz.ParseBytes([]byte(di.Graphviz))
		if err != nil {
			log.Fatal("can't open graphviz data:", err)
		}
		tf, err := ioutil.TempFile("", "tree_*.svg")
		if err != nil {
			log.Fatal("can't create temp file:", err)
		}
		ctx := context.Background()
		g, err := graphviz.New(ctx)
		if err != nil {
			log.Fatal("can't open graphviz:", err)
		}
		err = g.Render(ctx, gvo, graphviz.SVG, tf)
		if err != nil {
			log.Fatal("can't render graphviz:", err)
		}
		fmt.Println("tree file:", tf.Name())
		tf.Close()
		open(tf.Name())
	}
}

func open(path string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", path).Start()
	case "windows":
		err = exec.Command("rundll32", "rl.dll,FileProtocolHandler", path).Start()
	case "darwin":
		err = exec.Command("open", path).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}
}
