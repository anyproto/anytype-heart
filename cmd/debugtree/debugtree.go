package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"runtime"
	"time"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/debug/debugtree"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/goccy/go-graphviz"
)

var (
	file        = flag.String("f", "", "path to debug file")
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
	dt, err := debugtree.Open(*file)
	if err != nil {
		log.Fatal("can't open debug file:", err)
	}
	defer dt.Close()
	fmt.Println(dt.Stats().MlString())

	fmt.Println("build tree...")
	st := time.Now()
	t, _, err := change.BuildTree(dt)
	if err != nil {
		log.Fatal("build tree error:", err)
	}
	fmt.Printf("build tree done in %.1fs\n", time.Since(st).Seconds())

	if *changeIdx != -1 {
		id := ""
		i := 0
		t.Iterate(t.RootId(), func(c *change.Change) (isContinue bool) {
			if i == *changeIdx {
				id = c.Id
				fmt.Println("Change:")
				fmt.Println(pbtypes.Sprint(c.Change))
				return false
			} else {
				i++
			}
			return true
		})
		if id != "" {
			if t, err = change.BuildTreeBefore(dt, id, true); err != nil {
				log.Fatal("build tree before error:", err)
			}
		}
	}

	fmt.Printf("Tree len:\t%d\n", t.Len())
	fmt.Printf("Tree root:\t%s\n", t.RootId())

	if *printState {
		fmt.Println("Building state...")
		stt := time.Now()
		s, err := dt.BuildStateByTree(t)
		if err != nil {
			log.Fatal("can't build state:", err)
		}
		dur := time.Since(stt)
		fmt.Println(s.StringDebug())
		sbt, _ := smartblock.SmartBlockTypeFromID(s.RootId())
		fmt.Printf("Smarblock type:\t%v\n", sbt.ToProto())
		if *fileHashes {
			fmt.Println("File keys:")
			for _, fk := range s.GetFileKeys() {
				fmt.Printf("\t%s: %d\n", fk.Hash, len(fk.Keys))
			}
		}
		fmt.Println("state building time:", dur)
	}

	if *objectStore {
		fmt.Println("fetch object store info..")
		ls, err := dt.LocalStore()
		if err != nil {
			fmt.Println("can't open objectStore info:", err)
		} else {
			fmt.Println(pbtypes.Sprint(ls))
		}
	}

	if *makeTree {
		fmt.Println("saving tree file...")
		gv, err := t.Graphviz()
		if err != nil {
			log.Fatal("can't make graphviz data:", err)
		}
		gvo, err := graphviz.ParseBytes([]byte(gv))
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
