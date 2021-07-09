package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"runtime"

	"github.com/anytypeio/go-anytype-middleware/change"
	"github.com/anytypeio/go-anytype-middleware/core/debug/debugtree"
	"github.com/goccy/go-graphviz"
)

var (
	file     = flag.String("f", "", "path to debug file")
	makeTree = flag.Bool("t", false, "generate graphviz file")
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
