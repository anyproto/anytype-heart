package core
/*
import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mattbaird/jsonpatch"
	pb "github.com/anytypeio/go-anytype-library/pb"
	tpb "github.com/textileio/go-textile/pb"
)

const debugDir = "merge_debug"
func (doc *Document) followParentsUntilTheVersion(hash string, versionOffset int) (*DocumentVersion, error) {
	block := doc.node.Datastore().Blocks().Get(hash)
	if block.Type == tpb.Block_FILES {
		if versionOffset == 0 {
			return doc.GetVersion(block.Id)
		}
		versionOffset--
	}

	if len(block.Parents) == 0 {
		return nil, nil
	}

	sort.Strings(block.Parents)
	return doc.followParentsUntilTheVersion(block.Parents[0], versionOffset)
}


func flattenBlocksTraverse(parent string, blocks []*DocumentBlock, blocksMap map[string]*DocumentBlock, blocksList []string) []string {
	for _, block := range blocks {
		block.parentId = parent
		blocksMap[block.Id] = block
		blocksList = append(blocksList, block.Id)
		if len(block.Children) > 0 {
			blocksList = flattenBlocksTraverse(block.Id, block.Children, blocksMap, blocksList)
			block.Children = nil
		}
	}
	return blocksList
}

func flattenBlocksMap(blocks []*DocumentBlock) (blocksMap map[string]*DocumentBlock, blocksList []string) {
	blocksMap = make(map[string]*DocumentBlock)
	blocksList = flattenBlocksTraverse("", blocks, blocksMap, blocksList)
	return blocksMap, blocksList
}

func flattenBlocksTraverse2(parent string, blocks []*DocumentBlock, blocksMap map[string]*DocumentBlock, blocksList map[string][]string) {
	var child []string
	for _, block := range blocks {
		block.parentId = parent
		blocksMap[block.Id] = block
		if _, exists := blocksList[block.Id]; !exists {
			blocksList[block.Id] = []string{}
		}

		child = append(child, block.Id)

		if len(block.Children) > 0 {
			flattenBlocksTraverse2(block.Id, block.Children, blocksMap, blocksList)
			block.Children = nil
		}
	}

	blocksList[parent] = child
}

func flattenBlocksMap2(blocks []*DocumentBlock) (blocksMap map[string]*DocumentBlock, blocksList map[string][]string) {
	blocksMap = make(map[string]*DocumentBlock)
	blocksList = make(map[string][]string)

	flattenBlocksTraverse2("root", blocks, blocksMap, blocksList)
	return
}

func blocksTree2(blocksMap map[string]*DocumentBlock, root string, blocksList map[string][]string) []*DocumentBlock {
	var tree []*DocumentBlock
	for _, blockId := range blocksList[root] {
		block, exists := blocksMap[blockId]
		if !exists {
			continue
		}

		block.Children = blocksTree2(blocksMap, block.Id, blocksList)
		block.parentId = ""

		tree = append(tree, block)
	}

	return tree
}

func blocksTree(blocksMap map[string]*DocumentBlock, blocksList []string) []*DocumentBlock {
	var tree []*DocumentBlock
	for _, blockId := range blocksList {
		block, exists := blocksMap[blockId]
		if !exists {
			continue
		}

		if block.parentId != "" {
			blocksMap[block.parentId].Children = append(blocksMap[block.parentId].Children, block)
			block.parentId = ""
		} else {
			tree = append(tree, block)
		}
	}

	return tree
}

func mergePatches(patchSubordinary []jsonpatch.JsonPatchOperation, patchDominant []jsonpatch.JsonPatchOperation) []jsonpatch.JsonPatchOperation {
	var merged []jsonpatch.JsonPatchOperation
	var adds = make(map[string]struct{})
	var removes = make(map[string]struct{})

	for _, operation := range patchDominant {
		if _, ok := operation.Value.(string); !ok {
			continue
		}
		if operation.Operation == "add" {
			adds[operation.Value.(string)] = struct{}{}
		}
		if operation.Operation == "remove" {
			adds[operation.Value.(string)] = struct{}{}
		}
	}

	for _, operation := range patchSubordinary {
		if _, ok := operation.Value.(string); !ok {
			merged = append(merged, operation)
			continue
		}
		if operation.Operation == "add" {
			if _, exists := adds[operation.Value.(string)]; exists {
				continue
			}
		}

		if operation.Operation == "remove" {
			if _, exists := removes[operation.Value.(string)]; exists {
				continue
			}
		}

		merged = append(merged, operation)
	}

	merged = append(merged, patchDominant...)
	sort.Sort(jsonpatch.ByOperationAndPath(merged))
	return merged
}

func getUser(user *pb.User, id string) string {
	var s string
	if id == "" {
		s = "local_"
	}

	if user != nil {
		s += shortId(user.Address)
	}

	return s
}

func mergeVersions(ancestor, version1, version2 *DocumentVersion) (*DocumentVersion, error) {
	blocksAncestor, listAncestor := flattenBlocksMap2(ancestor.Blocks)
	blocks1, list1 := flattenBlocksMap2(version1.Blocks)
	blocks2, list2 := flattenBlocksMap2(version2.Blocks)

	blocksAncestorJSON, err := json.Marshal(blocksAncestor)
	if err != nil {
		return nil, err
	}

	blocks1JSON, err := json.Marshal(blocks1)
	if err != nil {
		return nil, err
	}

	blocks2JSON, err := json.Marshal(blocks2)
	if err != nil {
		return nil, err
	}

	listAncsetorJSON, err := json.Marshal(listAncestor)
	if err != nil {
		return nil, err
	}

	list1JSON, err := json.Marshal(list1)
	if err != nil {
		return nil, err
	}

	list2JSON, err := json.Marshal(list2)
	if err != nil {
		return nil, err
	}

	patchBlocks1, err := jsonpatch.CreatePatch(blocksAncestorJSON, blocks1JSON)
	if err != nil {
		return nil, err
	}

	patchBlocks2, err := jsonpatch.CreatePatch(blocksAncestorJSON, blocks2JSON)
	if err != nil {
		return nil, err
	}

	patchList1, err := jsonpatch.CreatePatch(listAncsetorJSON, list1JSON)
	if err != nil {
		return nil, err
	}

	patchList2, err := jsonpatch.CreatePatch(listAncsetorJSON, list2JSON)
	if err != nil {
		return nil, err
	}

	var patchListFinal []jsonpatch.JsonPatchOperation
	var patchBlocksFinal []jsonpatch.JsonPatchOperation

	mergedVersion := &DocumentVersion{Merged: true}
	var predominantVersion *DocumentVersion

	// choose version1 as the predominant version in case:
	// 1) it is lexicographically less than version2
	// or
	// 2) this is a new version that was just created locally and need to be merged with the existing version
	// otherwise use version2 as the predominant version
	if strings.Compare(version1.Id, version2.Id) == -1 || version1.Id == "" {
		predominantVersion = version1
		patchBlocksFinal = mergePatches(patchBlocks2, patchBlocks1)
		patchListFinal = mergePatches(patchList2, patchList1)
	} else {
		predominantVersion = version2
		patchBlocksFinal = mergePatches(patchBlocks1, patchBlocks2)
		patchListFinal = mergePatches(patchList1, patchList2)
	}
	//spew.Dump(string(listAncsetorJSON), string(list1JSON), string(list2JSON), patchList1, patchList2)

	mergedVersion.Name = predominantVersion.Name
	mergedVersion.Icon = predominantVersion.Icon
	// todo: we shouldn't add user here
	// it was added to create the unique Source hash and protect from a bug
	// when 2 peers create the same file with different nonce
	var newTime time.Time
	if version1.Date != nil && version2.Date != nil {
		if version1.Date.After(*version2.Date) {
			newTime = version1.Date.Add(1e6)
		} else {
			newTime = version2.Date.Add(1e6)
		}
		mergedVersion.Date = &newTime
		log.Debugf("1 = %s, 2 = %s -> %s", version1.Date.String(), version2.Date.String(), mergedVersion.Date.String())
	}

	patchBlocksFinalJson, err := json.Marshal(patchBlocksFinal)
	if err != nil {
		return nil, err
	}

	patchListFinalJson, err := json.Marshal(patchListFinal)
	if err != nil {
		return nil, err
	}

	if os.Getenv("ANYTYPE_DEBUG_MERGE") == "1" {
		err = os.Mkdir(debugDir, 0655)
		if err != nil {
			log.Errorf("can't create merge_debug folder")
		}

		ts := time.Now().Unix()

		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_blocks_ancestor_%s.json", ts, getUser(ancestor.User, ancestor.Id)), blocksAncestorJSON, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_blocks_1_%s.json", ts, getUser(version1.User, version1.Id)), blocks1JSON, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_blocks_2_%s.json", ts, getUser(version2.User, version2.Id)), blocks2JSON, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_list_ancestor.json", ts), blocks2JSON, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_list_1.json", ts), blocks2JSON, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_list_2.json", ts), blocks2JSON, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_blocks_patch.json", ts), patchBlocksFinalJson, 0655)
		ioutil.WriteFile(fmt.Sprintf(debugDir+"/%d_list_patch.json", ts), patchListFinalJson, 0655)
	}
	patchBlocks, err := jsonpatch.DecodePatch(patchBlocksFinalJson)
	if err != nil {
		return nil, err
	}

	patchList, err := jsonpatch.DecodePatch(patchListFinalJson)
	if err != nil {
		return nil, err
	}

	mergedBlocksJSON, err := patchBlocks.Apply(blocksAncestorJSON)
	if err != nil {
		return nil, err
	}

	mergedListJSON, err := patchList.Apply(listAncsetorJSON)
	if err != nil {
		return nil, err
	}

	log.Debugf("[MERGE] final list: %s", string(mergedListJSON))

	var mergedBlocks = make(map[string]*DocumentBlock)
	err = json.Unmarshal(mergedBlocksJSON, &mergedBlocks)
	if err != nil {
		return nil, err
	}

	var mergedList map[string][]string
	//	spew.Dump(mergedListJSON)
	err = json.Unmarshal(mergedListJSON, &mergedList)
	if err != nil {
		return nil, err
	}

	mergedVersion.Blocks = blocksTree2(mergedBlocks, "root", mergedList)

	if version1.Id != "" {
		mergedVersion.Parents = append(mergedVersion.Parents, version1.Id)
	}

	if version2.Id != "" {
		mergedVersion.Parents = append(mergedVersion.Parents, version2.Id)
	}

	sort.Strings(mergedVersion.Parents)

	return mergedVersion, nil
}

/*func (doc *Document) mergeVersionsByHashes(hash1, hash2 string) (mergedVersion *DocumentVersion, err error) {
	ancestor := doc.Thread.FollowParentsForTheFirstAncestor(hash1, hash2)
	if ancestor == nil {
		return nil, fmt.Errorf("no ancestor found for %s and %s", hash1, hash2)
	}

	version1Block := doc.Thread.followParentsUntilBlock(hash1, pb.Block_FILES)
	if version1Block == nil {
		return nil, fmt.Errorf("failed to found closest FILES block for %ss", hash1)
	}

	version1, err := doc.GetVersion(version1Block.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to get version for %s: %s", version1Block.Id, err.Error())
	}

	version2Block := doc.Thread.followParentsUntilBlock(hash2, pb.Block_FILES)
	if version2Block == nil {
		return nil, fmt.Errorf("failed to found closest FILES block for %ss", hash2)
	}

	version2, err := doc.GetVersion(version2Block.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to get version for %s: %s", version2Block.Id, err.Error())
	}

	if version1.Id == version2.Id {
		log.Debugf("[MERGE] versions are equal")
		return nil, nil
	}

	versionAncsetorBlock := doc.Thread.followParentsUntilBlock(ancestor.B58String(), pb.Block_FILES)
	versionAncsetor, err := doc.GetVersion(versionAncsetorBlock.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to get version for %s: %s", version2.Id, err.Error())
	}

	log.Debugf("(%p) [MERGE] versions to merge: %s(ANCESTOR) %s and %s", doc.textile, versionAncsetor.Id, version1.Id, version2.Id)

	return mergeVersions(versionAncsetor, version1, version2)
}
*/
