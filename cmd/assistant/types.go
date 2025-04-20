package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/cmd/assistant/api"
	"github.com/anyproto/anytype-heart/cmd/assistant/mcp"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type subscriber struct {
	spaceId             string
	typesLock           sync.Mutex
	types               map[string]*ObjectType
	relations           map[string]*Relation
	relationJsonKeyToId map[string]string
	options             map[string]*Option
	service             subscription.Service
	setTool             AddToolFunc
	api                 *api.APIClient
}

type AddToolFunc func(tool openai.Tool, caller ToolCaller)

func newSubscriber(config *assistantConfig, service subscription.Service, tool AddToolFunc) *subscriber {
	return &subscriber{
		spaceId:             config.SpaceId,
		types:               make(map[string]*ObjectType),
		relations:           make(map[string]*Relation),
		relationJsonKeyToId: make(map[string]string),
		options:             make(map[string]*Option),
		service:             service,
		setTool:             tool,
		api:                 api.NewAPIClient(apiBaseURL(config.apiListenAddr), config.apiKey, config.SpaceId),
	}
}
func (s *subscriber) Run(ctx context.Context) error {
	go func() {
		err := s.listRelations(ctx)
		if err != nil {
			log.Error("list relations", zap.Error(err))
		}
	}()
	go func() {
		time.Sleep(time.Second * 2)
		err := s.listTypes(ctx)
		if err != nil {
			log.Error("list types", zap.Error(err))
		}
	}()
	return nil
}

type ObjectType struct {
	Id          string
	Key         string
	Name        string
	Hidden      bool
	Layout      model.ObjectTypeLayout
	Description string
	RelationIds []string
}

type Relation struct {
	Id     string
	Name   string
	Key    string
	Format model.RelationFormat
}

type Option struct {
	Name        string
	Key         string
	RelationKey string
}

func detailsToObjectType(details *domain.Details) ObjectType {
	if details == nil {
		return ObjectType{}
	}

	recommendedRelations := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	recommendedRelations = append(recommendedRelations, details.GetStringList(bundle.RelationKeyFeaturedRelations)...)
	return ObjectType{
		Id:          details.GetString(bundle.RelationKeyId),
		Key:         details.GetString(bundle.RelationKeyUniqueKey),
		Hidden:      details.GetBool(bundle.RelationKeyIsHidden),
		Layout:      model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyRecommendedLayout)),
		Name:        details.GetString(bundle.RelationKeyName),
		Description: details.GetString(bundle.RelationKeyDescription),
		RelationIds: recommendedRelations,
	}
}
func detailsToRelation(details *domain.Details) Relation {
	if details == nil {
		return Relation{}
	}

	return Relation{
		Id:     details.GetString(bundle.RelationKeyId),
		Key:    details.GetString(bundle.RelationKeyRelationKey),
		Name:   details.GetString(bundle.RelationKeyName),
		Format: model.RelationFormat(details.GetInt64(bundle.RelationKeyRelationFormat)),
	}
}

type objectTypeCaller struct {
	ot         *ObjectType
	api        *api.APIClient
	subscriber *subscriber
}

var rootFields = []string{
	"body",
	"description",
	"icon",
	"name",
	"source",
}

func relationNameCleaner(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	return strings.ToLower(name)
}
func (o *objectTypeCaller) CallTool(name string, params any) (*mcp.ToolCallResult, error) {
	// Create a function arguments string from the params
	fmt.Printf("creator type tool %s params: %+v\n", name, params)
	// Create a tool call object to pass to HandleToolCall
	tool := api.ApiTool{
		Tool: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:       "create_object",
				Parameters: json.RawMessage(`{"properties":{"body":{"description":"The body of the object","type":"string"},"description":{"description":"The description of the object","type":"string"},"icon":{"description":"The icon of the object","properties":{"color":{"description":"The color of the icon","enum":["grey","yellow","orange","red","pink","purple","blue","ice","teal","lime"],"type":"string"},"emoji":{"description":"The emoji of the icon","type":"string"},"file":{"description":"The file of the icon","type":"string"},"format":{"description":"The type of the icon","enum":["emoji","file","icon"],"type":"string"},"name":{"description":"The name of the icon","type":"string"}},"type":"object"},"name":{"description":"The name of the object","type":"string"},"properties":{"description":"Properties to set on the object","type":"object"},"source":{"description":"The source url, only applicable for bookmarks","type":"string"},"template_id":{"description":"The id of the template to use","type":"string"},"type_key":{"description":"The key of the type of object to create","type":"string"}},"required":[],"type":"object"}`),
			},
		},
		Method: "POST",
		Path:   "/spaces/{space_id}/objects",
	}

	args := params.(map[string]interface{})
	args["properties"] = make(map[string]interface{})
	var deleteKeys []string
	for k, v := range args {
		if k == "properties" {
			continue
		}
		if !slices.Contains(rootFields, k) {
			args["properties"].(map[string]interface{})[k] = v
			deleteKeys = append(deleteKeys, k)
		}
	}
	for _, k := range deleteKeys {
		delete(args, k)
	}
	paramsMap := args["properties"].(map[string]interface{})
	params2 := make(map[string]interface{})
	for k, v := range paramsMap {
		if id, ok := o.subscriber.relationJsonKeyToId[relationNameCleaner(k)]; ok {
			if rel, ok := o.subscriber.relations[id]; ok {
				params2[rel.Key] = v
			}
		} else {
			fmt.Printf("cannot find relation %s in type %s\n", k, o.ot.Name)
		}
	}
	args["properties"] = params2
	args["type_key"] = o.ot.Key
	// Process the tool call using the existing handler
	result, err := o.api.HandleToolCall(tool, args)
	if err != nil {
		fmt.Printf("Error calling tool: %v\n", err)
		return &mcp.ToolCallResult{
			IsError: true,
			Content: []mcp.ToolCallResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Error: %v", err),
				},
			},
		}, nil
	}

	// Convert the result to a ToolCallResult
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return &mcp.ToolCallResult{
			IsError: true,
			Content: []mcp.ToolCallResultContent{
				{
					Type: "text",
					Text: fmt.Sprintf("Error marshaling result: %v", err),
				},
			},
		}, nil
	}

	return &mcp.ToolCallResult{
		IsError: false,
		Content: []mcp.ToolCallResultContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

func (o *objectTypeCaller) Tool() openai.Tool {
	relationNames := []string{}
	var properties = map[string]interface{}{
		"body": map[string]interface{}{
			"type":        "string",
			"description": "The markdown body of the object. Do not add title or description here",
		},
		"description": map[string]interface{}{
			"type":        "string",
			"description": "The description of the object",
		},
		"icon": map[string]interface{}{
			"type":        "object",
			"description": "The icon of the object",
			"properties": map[string]interface{}{
				"emoji": map[string]interface{}{
					"type":        "string",
					"description": "The emoji of the icon",
				},
			},
		},
		"name": map[string]interface{}{
			"type":        "string",
			"description": "The name of the object",
		},
		"source": map[string]interface{}{
			"type":        "string",
			"description": "The source url",
		}}

	o.subscriber.typesLock.Lock()
	for _, relId := range o.ot.RelationIds {
		rel := o.subscriber.relations[relId]
		if rel == nil {
			fmt.Printf("relation %s not found for type %s\n", relId, o.ot.Name)
			continue
		}
		if _, ok := properties[rel.Key]; ok {
			continue
		}

		jsonKey := relationNameCleaner(rel.Name)
		if rel.Format == model.RelationFormat_longtext || rel.Format == model.RelationFormat_shorttext || rel.Format == model.RelationFormat_email || rel.Format == model.RelationFormat_url || rel.Format == model.RelationFormat_phone {
			relationNames = append(relationNames, jsonKey)
			properties[jsonKey] = map[string]interface{}{
				"type":        "string",
				"description": rel.Name + ". Plaintext, no markdown",
			}
		} else if rel.Format == model.RelationFormat_number {
			relationNames = append(relationNames, jsonKey)
			properties[jsonKey] = map[string]interface{}{
				"type": "integer",
				// "description": rel.Name,
			}
		} else if rel.Format == model.RelationFormat_checkbox {
			relationNames = append(relationNames, jsonKey)
			properties[jsonKey] = map[string]interface{}{
				"type": "boolean",
				// "description": rel.Name,
			}
		} else if rel.Format == model.RelationFormat_date {
			relationNames = append(relationNames, jsonKey)
			properties[jsonKey] = map[string]interface{}{
				"type":        "string",
				"description": rel.Name + " in RFC3339 format",
			}
		}
	}
	var required = []string{"body"}
	if o.ot.Layout != model.ObjectType_note {
		required = append(required, "name")
	}
	fmt.Printf("create object type %s properties: %+v, %v\n", o.ot.Key, properties, required)
	o.subscriber.typesLock.Unlock()
	cleanName := strings.ToLower(strings.ReplaceAll(o.ot.Name, " ", "_"))
	desc := ""
	if o.ot.Description != "" {
		desc = ": %s" + o.ot.Description
	}
	relationNames = append(relationNames, "description", "icon", "name", "source")

	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "create_" + cleanName + "_object",
			Description: "Create a new object of type " + o.ot.Name + desc + ". Populate as many properties (" + strings.Join(relationNames, ", ") + ") as possible with accurate information. Provide detailed content in the 'body' property formatted in Markdown. Do not include the title or description in the 'body'",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   required,
			},
		},
	}
}

func (s *subscriber) listTypes(ctx context.Context) error {
	typesSub := mb.New[*pb.EventMessage](0)
	subscriptionService := s.service
	subResp, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: s.spaceId,
		Keys:    []string{bundle.RelationKeyName.String(), bundle.RelationKeyUniqueKey.String(), bundle.RelationKeyRecommendedRelations.String(), bundle.RelationKeyFeaturedRelations.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_objectType),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
			{
				RelationKey: bundle.RelationKeyIsHiddenDiscovery,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
			{
				RelationKey: bundle.RelationKeyUniqueKey,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: domain.StringList([]string{
					bundle.TypeKeyTemplate.URL(),
					bundle.TypeKeySet.URL(),
					bundle.TypeKeyObjectType.URL(),
					bundle.TypeKeyFile.URL(),
					bundle.TypeKeySpace.URL(),
					bundle.TypeKeyCollection.URL(),
					bundle.TypeKeySpaceView.URL(),
					bundle.TypeKeyNote.URL(),
					bundle.TypeKeyParticipant.URL(),
					bundle.TypeKeyRelation.URL(),
					bundle.TypeKeyImage.URL(),
					bundle.TypeKeyVideo.URL(),
					bundle.TypeKeyAudio.URL(),
					bundle.TypeKeyDate.URL(),
					bundle.TypeKeyDashboard.URL(),
					bundle.TypeKeyChatDerived.URL(),
					bundle.TypeKeyChat.URL(),
					bundle.TypeKeyChatDerived.URL(),
					bundle.TypeKeyRelationOption.URL(),
				}),
			},
		},
		Internal:      true,
		InternalQueue: typesSub,
	})
	if err != nil {
		return fmt.Errorf("subscribe to chats: %w", err)
	}
	defer func() {
		err = subscriptionService.Unsubscribe(subResp.SubId)
		if err != nil {
			log.Error("unsubscribe from chats", zap.Error(err))
		}
	}()

	if len(subResp.Records) > 0 {
		s.typesLock.Lock()
		var ots []*ObjectType
		for _, record := range subResp.Records {
			ot := detailsToObjectType(record)
			if ot.Hidden {
				continue
			}
			s.types[ot.Key] = &ot
			ots = append(ots, &ot)
		}
		s.typesLock.Unlock()

		fmt.Printf("Got %d types: %v\n", len(s.types), s.types)
		for _, ot := range ots {
			s.initTypeTool(ot)
		}
	} else {
		for {
			msg, err := typesSub.WaitOne(ctx)
			if err != nil {
				return fmt.Errorf("wait: %w", err)
			}
			log.Warn("wait for types: handling", zap.Any("event", msg))
			if ev := msg.GetObjectDetailsSet(); ev != nil {
				ot := detailsToObjectType(domain.NewDetailsFromProto(ev.Details))
				if ot.Hidden {
					continue
				}
				s.typesLock.Lock()

				if _, ok := s.types[ot.Key]; ok {
					s.types[ot.Key] = &ot
				} else {
					s.types[ot.Key] = &ot
				}
				s.typesLock.Unlock()
				// hack to wait for relations first
				time.Sleep(time.Second)
				s.initTypeTool(&ot)
			}
		}
	}
	return nil
}

func (s *subscriber) initTypeTool(ot *ObjectType) {
	caller := &objectTypeCaller{
		ot:         ot,
		api:        s.api,
		subscriber: s,
	}
	tool := caller.Tool()
	s.setTool(tool, caller)
}

func (s *subscriber) listRelations(ctx context.Context) error {
	typesSub := mb.New[*pb.EventMessage](0)
	subscriptionService := s.service
	subResp, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: s.spaceId,
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyRelationKey.String(), bundle.RelationKeyRelationFormat.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_relation),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
			{
				RelationKey: bundle.RelationKeyIsHiddenDiscovery,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: domain.StringList([]string{
					bundle.RelationKeyCreatedDate.String(),
				}),
			},
		},
		Internal:      true,
		InternalQueue: typesSub,
	})
	if err != nil {
		return fmt.Errorf("subscribe to chats: %w", err)
	}
	defer func() {
		err = subscriptionService.Unsubscribe(subResp.SubId)
		if err != nil {
			log.Error("unsubscribe from chats", zap.Error(err))
		}
	}()

	if len(subResp.Records) > 0 {
		s.typesLock.Lock()
		for _, record := range subResp.Records {
			rel := detailsToRelation(record)
			s.relations[rel.Id] = &rel
			// todo: ensure no duplicate names
			s.relationJsonKeyToId[relationNameCleaner(rel.Name)] = rel.Id
		}
		s.typesLock.Unlock()
		fmt.Printf("Got %d relations: %v\n", len(s.relations), s.relations)
	} else {
		for {
			msg, err := typesSub.WaitOne(ctx)
			if err != nil {
				return fmt.Errorf("wait: %w", err)
			}
			log.Warn("wait for types: handling", zap.Any("event", msg))
			if ev := msg.GetObjectDetailsSet(); ev != nil {
				rel := detailsToRelation(domain.NewDetailsFromProto(ev.Details))

				s.typesLock.Lock()
				s.relations[rel.Id] = &rel
				s.relationJsonKeyToId[relationNameCleaner(rel.Name)] = rel.Id
				s.typesLock.Unlock()
			}
		}
	}
	return nil
}
