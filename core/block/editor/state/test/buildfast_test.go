package debug

// TODO: revive at some point
// func TestBuildFast(t *testing.T) {
// 	// Specify the directory you want to iterate
// 	dir := "./testdata"
//
// 	// Read the directory
// 	files, err := ioutil.ReadDir(dir)
// 	if err != nil {
// 		t.Fatalf("Failed to read dir: %s", err)
// 	}
//
// 	// Iterate over the files
// 	for _, file := range files {
// 		t.Run(file.Name(), func(t *testing.T) {
// 			filePath := filepath.Join(dir, file.Name())
//
// 			// open the file
// 			f, err := os.Open(filePath)
// 			if err != nil {
// 				t.Fatalf("Failed to open file: %s", err)
// 			}
// 			defer f.Close()
//
// 			testBuildFast(t, filePath)
// 		})
// 	}
// }
//
// func testBuildFast(b *testing.T, filepath string) {
// 	// todo: replace with less heavy tree
// 	archive, err := treearchive.Open(filepath)
// 	if err != nil {
// 		require.NoError(b, err)
// 	}
// 	defer archive.Close()
//
// 	importer := exporter.NewTreeImporter(archive.ListStorage(), archive.TreeStorage())
//
// 	err = importer.Import(false, "")
// 	if err != nil {
// 		log.Fatal("can't import the tree", err)
// 	}
//
// 	start := time.Now()
// 	_, err = importer.State()
// 	if err != nil {
// 		log.Fatal("can't build state:", err)
// 	}
// 	b.Logf("fast build took %s", time.Since(start))
//
// 	importer2 := exporter.NewTreeImporter(archive.ListStorage(), archive.TreeStorage())
//
// 	err = importer2.Import(false, "")
// 	if err != nil {
// 		log.Fatal("can't import the tree", err)
// 	}
//
// }
