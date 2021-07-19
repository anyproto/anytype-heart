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
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/debug/debugtree"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/goccy/go-graphviz"
)

var (
	file       = flag.String("f", "", "path to debug file")
	makeTree   = flag.Bool("t", false, "generate graphviz file")
	printState = flag.Bool("s", false, "print result state debug")
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
	t, _, err := change.BuildTree(dt)
	if err != nil {
		log.Fatal("build tree error:", err)
	}

	fmt.Printf("Tree len:\t%d\n", t.Len())
	fmt.Printf("Tree root:\t%s\n", t.RootId())

	if *printState {
		fmt.Println("Building state...")
		stt := time.Now()
		root := t.Root()
		if root == nil || root.GetSnapshot() == nil {
			log.Fatal("root missing or not a snapshot")
		}
		s := state.NewDocFromSnapshot("", root.GetSnapshot()).(*state.State)
		s.SetChangeId(root.Id)
		st, err := change.BuildStateSimpleCRDT(s, t)
		if err != nil {
			return
		}
		if _, _, err = state.ApplyStateFast(st); err != nil {
			return
		}
		dur := time.Since(stt)
		fmt.Println(s.StringDebug())
		sbt, _ := smartblock.SmartBlockTypeFromID(st.RootId())
		fmt.Printf("Smarblock type:\t%v\n", sbt.ToProto())
		fmt.Println("state building time:", dur)
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
