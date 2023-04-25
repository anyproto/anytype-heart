//go:build !nogrpcserver && !_test

package main

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

//go:embed rules.json
var rulesJSON []byte

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

func processRules(s *pb.ChangeSnapshot) {
	if err := json.Unmarshal(rulesJSON, &rules); err != nil {
		fmt.Println("Failed to unmarshal rules.json:", err)
		return
	}
	id := pbtypes.GetString(s.Data.Details, bundle.RelationKeyId.String())

	for i, r := range rules {
		if r.ObjectID != id && r.ObjectID != "" {
			continue
		}

		switch r.Entity {
		case relationLink:
			doRelationLinkRule(s, &r)
		case detail:
			doDetailRule(s, &r)
		case objectType:
			doObjectTypeRule(s, &r)
		case dataViewTarget:
			doDataViewTargetRule(s, &r)
		case linkTarget:
			doLinkTargetRule(s, &r)
		default:
			fmt.Println("Invalid entity in rule", i, ":", r.Entity)
		}
	}
}

func doRelationLinkRule(s *pb.ChangeSnapshot, r *rule) {
	if r.RelationLink == nil || r.RelationLink.Key == "" {
		fmt.Println("Invalid Relation link provided in relation-rule")
		return
	}
	switch r.Action {
	case remove:
		for i, relLink := range s.Data.RelationLinks {
			if relLink.Key == r.RelationLink.Key {
				s.Data.RelationLinks = append(s.Data.RelationLinks[:i], s.Data.RelationLinks[i+1:]...)
				break
			}
		}
	case add:
		s.Data.RelationLinks = append(s.Data.RelationLinks, r.RelationLink)
	case change:
		for i, relLink := range s.Data.RelationLinks {
			if relLink.Key == r.RelationLink.Key {
				s.Data.RelationLinks[i] = r.RelationLink
				break
			}
		}
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}

func doDetailRule(s *pb.ChangeSnapshot, r *rule) {
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

func doObjectTypeRule(s *pb.ChangeSnapshot, r *rule) {
	if r.ObjectType == "" {
		fmt.Println("No object type provided in objectType-rule")
		return
	}
	switch r.Action {
	case remove:
		for i, ot := range s.Data.ObjectTypes {
			if ot == r.ObjectType {
				s.Data.ObjectTypes = append(s.Data.ObjectTypes[:i], s.Data.ObjectTypes[i+1:]...)
				break
			}
		}
	case add:
		s.Data.ObjectTypes = append(s.Data.ObjectTypes, r.ObjectType)
	case change:
		for i, ot := range s.Data.ObjectTypes {
			if ot == r.ObjectType {
				s.Data.ObjectTypes[i] = r.ObjectType
				break
			}
		}
	default:
		fmt.Printf(errInvalidAction, r.Action)
	}
}

func doDataViewTargetRule(s *pb.ChangeSnapshot, r *rule) {
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

func doLinkTargetRule(s *pb.ChangeSnapshot, r *rule) {
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
