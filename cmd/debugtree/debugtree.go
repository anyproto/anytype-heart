//go:build cgo

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/goccy/go-graphviz"
	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/debug/treearchive"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	file        = flag.String("f", "", "path to debug file")
	fromRoot    = flag.Bool("r", false, "build from root of the tree")
	makeJson    = flag.Bool("j", false, "generate json file")
	makeTree    = flag.Bool("t", false, "generate graphviz file")
	printState  = flag.Bool("s", false, "print result state debug")
	changeIdx   = flag.Int("c", -1, "build tree before given index and print change")
	objectStore = flag.Bool("o", false, "show object store info")
	fileHashes  = flag.Bool("h", false, "show file hashes in state")
)

func main() {
	flag.Parse()
	if *file == "" {
		flag.PrintDefaults()
		return
	}
	fmt.Println("opening file...")
	st := time.Now()
	archive, err := treearchive.Open(*file)
	if err != nil {
		log.Fatal("can't open debug file:", err)
	}
	defer archive.Close()
	fmt.Printf("open archive done in %.1fs\n", time.Since(st).Seconds())

	importer := treearchive.NewTreeImporter(archive.ListStorage(), archive.TreeStorage())
	st = time.Now()
	err = importer.Import(*fromRoot, "")
	if err != nil {
		log.Fatal("can't import the tree", err)
	}
	fmt.Printf("import tree done in %.1fs\n", time.Since(st).Seconds())

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
		err = importer.Import(*fromRoot, ch.Id)
		if err != nil {
			log.Fatal("can't import the tree before", ch.Id, err)
		}
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
		s, err := importer.State(false)
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
		ls, err := archive.LocalStore()
		if err != nil {
			fmt.Println("can't open objectStore info:", err)
		} else {
			fmt.Println(pbtypes.Sprint(ls))
		}
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
		g := graphviz.New()
		g.Render(gvo, graphviz.SVG, tf)
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
