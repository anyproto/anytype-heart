//go:build !nogrpcserver && !_test

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type action string

const (
	remove action = "remove"
	change action = "change"
	add    action = "add"
)

type entityType string

const (
	relationLink   entityType = "relationLink"
	detail         entityType = "detail"
	objectType     entityType = "objectType"
	dataViewTarget entityType = "dataViewTarget"
	linkTarget     entityType = "linkTarget"
)

type rule struct {
	Action       action              `json:"action"`
	Entity       entityType          `json:"entity"`
	ObjectID     string              `json:"objectID,omitempty"`
	DetailKey    string              `json:"detailKey,omitempty"`
	DetailValue  *types.Value        `json:"detailValue,omitempty"`
	ObjectType   string              `json:"objectType,omitempty"`
	RelationLink *model.RelationLink `json:"relationLink,omitempty"`
	BlockID      string              `json:"blockID,omitempty"`
	TargetID     string              `json:"targetID,omitempty"`
}

var (
	rules []rule

	errInvalidAction = "invalid action %s in rule provided"
)

func readRules(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("failed to open %s to get processing rules: %w", fileName, err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read rules from file: %w", err)
	}
	if err = json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("failed to unmarshal json in file %s: %w", fileName, err)
	}
	return nil
}

func processRules(s *pb.ChangeSnapshot) {
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())

	for i, r := range rules {
		if r.ObjectID != id && r.ObjectID != "" {
			continue
		}

		switch r.Entity {
		case relationLink:
			doRelationLinkRule(s, r)
		case detail:
			doDetailRule(s, r)
		case objectType:
			doObjectTypeRule(s, r)
		case dataViewTarget:
			doDataViewTargetRule(s, r)
		case linkTarget:
			doLinkTargetRule(s, r)
		default:
			fmt.Println("Invalid entity in rule", i, ":", r.Entity)
		}
	}
}

func doRelationLinkRule(s *pb.ChangeSnapshot, r rule) {
	if r.RelationLink == nil || r.RelationLink.Key == "" {
		fmt.Println("Invalid Relation link provided in relation-rule")
		return
	}
	switch r.Action {
	case remove:
		s.Data.RelationLinks = lo.Reject(s.Data.RelationLinks, func(relLink *model.RelationLink, _ int) bool {
			return relLink.Key == r.RelationLink.Key
		})
	case add:
		s.Data.RelationLinks = append(s.Data.RelationLinks, r.RelationLink)
	case change:
		s.Data.RelationLinks = slice.ReplaceFirstBy(s.Data.RelationLinks, r.RelationLink, func(rl *model.RelationLink) bool {
			return rl.Key == r.RelationLink.Key
		})
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}

func doDetailRule(s *pb.ChangeSnapshot, r rule) {
	if r.DetailKey == "" {
		fmt.Println("No detail key provided in detail-rule")
		return
	}
	switch r.Action {
	case remove:
		delete(s.Data.Details.Fields, r.DetailKey)
	case change, add:
		s.Data.Details.Fields[r.DetailKey] = r.DetailValue
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}

func doObjectTypeRule(s *pb.ChangeSnapshot, r rule) {
	if r.ObjectType == "" {
		fmt.Println("No object type provided in objectType-rule")
		return
	}
	switch r.Action {
	case remove:
		s.Data.ObjectTypes = slice.RemoveMut(s.Data.ObjectTypes, r.ObjectType)
	case add:
		s.Data.ObjectTypes = append(s.Data.ObjectTypes, r.ObjectType)
	case change:
		return
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}

func doDataViewTargetRule(s *pb.ChangeSnapshot, r rule) {
	if r.BlockID == "" {
		fmt.Println("Block id is not provided for dataViewTarget-rule")
		return
	}
	var (
		dv *model.BlockContentOfDataview
		ok = false
	)
	for _, b := range s.Data.Blocks {
		if b.Id == r.BlockID {
			dv, ok = b.Content.(*model.BlockContentOfDataview)
		}
	}
	if !ok {
		fmt.Println("Failed to process rule as block" + r.BlockID + "of object" +
			pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String()) + "is not dataview block")
		return
	}
	switch r.Action {
	case remove:
		dv.Dataview.TargetObjectId = ""
	case change, add:
		dv.Dataview.TargetObjectId = r.TargetID
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}

func doLinkTargetRule(s *pb.ChangeSnapshot, r rule) {
	if r.BlockID == "" {
		fmt.Println("Block id is not provided for linkTarget-rule")
		return
	}
	var (
		l  *model.BlockContentOfLink
		ok = false
	)
	for _, b := range s.Data.Blocks {
		if b.Id == r.BlockID {
			l, ok = b.Content.(*model.BlockContentOfLink)
		}
	}
	if !ok {
		fmt.Println("Failed to process rule as block" + r.BlockID + "of object" +
			pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String()) + "is not link block")
		return
	}
	switch r.Action {
	case remove:
		l.Link.TargetBlockId = ""
	case change, add:
		l.Link.TargetBlockId = r.TargetID
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}
