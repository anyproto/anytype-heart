package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"

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
	spaceId   string
	typesLock sync.Mutex
	types     map[string]*ObjectType
	relations map[string]*Relation
	options   map[string]*Option
	service   subscription.Service
	addTool   AddToolFunc
	api       *api.APIClient
}

type AddToolFunc func(tool openai.Tool, caller ToolCaller)

func newSubscriber(config *assistantConfig, service subscription.Service, tool AddToolFunc) *subscriber {
	return &subscriber{
		spaceId:   config.SpaceId,
		types:     make(map[string]*ObjectType),
		relations: make(map[string]*Relation),
		options:   make(map[string]*Option),
		service:   service,
		addTool:   tool,
		api:       api.NewAPIClient(config.apiListenAddr, config.apiKey, config.SpaceId),
	}
}
func (s *subscriber) Run(ctx context.Context) error {
	go func() {
		err := s.listTypes(ctx)
		if err != nil {
			log.Error("list types", zap.Error(err))
		}
	}()
	go func() {
		err := s.listRelations(ctx)
		if err != nil {
			log.Error("list relations", zap.Error(err))
		}
	}()
	return nil
}

type ObjectType struct {
	Id           string
	Key          string
	Name         string
	Hidden       bool
	Description  string
	RelationKeys []string
}

type Relation struct {
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

	return ObjectType{
		Id:          details.GetString(bundle.RelationKeyId),
		Key:         details.GetString(bundle.RelationKeyUniqueKey),
		Hidden:      details.GetBool(bundle.RelationKeyIsHidden),
		Name:        details.GetString(bundle.RelationKeyName),
		Description: details.GetString(bundle.RelationKeyDescription),
	}
}
func detailsToRelation(details *domain.Details) Relation {
	if details == nil {
		return Relation{}
	}

	return Relation{
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

func (o *objectTypeCaller) CallTool(_ string, params any) (*mcp.ToolCallResult, error) {
	// Create a function arguments string from the params

	// Create a tool call object to pass to HandleToolCall
	tool := api.ApiTool{
		Tool: openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "create_object",
				Description: "Use this endpoint to create a new object. You should first check available object types with the List types endpoint, then create the appropriate object type with this endpoint. When creating content, use structured markdown for the body field. If the content is based on a web page, include the source URL. The endpoint returns the full details of the newly created object.",
				Parameters:  json.RawMessage(`{"properties":{"body":{"description":"The body of the object","type":"string"},"description":{"description":"The description of the object","type":"string"},"icon":{"description":"The icon of the object","properties":{"color":{"description":"The color of the icon","enum":["grey","yellow","orange","red","pink","purple","blue","ice","teal","lime"],"type":"string"},"emoji":{"description":"The emoji of the icon","type":"string"},"file":{"description":"The file of the icon","type":"string"},"format":{"description":"The type of the icon","enum":["emoji","file","icon"],"type":"string"},"name":{"description":"The name of the icon","type":"string"}},"type":"object"},"name":{"description":"The name of the object","type":"string"},"properties":{"description":"Properties to set on the object","type":"object"},"source":{"description":"The source url, only applicable for bookmarks","type":"string"},"template_id":{"description":"The id of the template to use","type":"string"},"type_key":{"description":"The key of the type of object to create","type":"string"}},"required":[],"type":"object"}`),
			},
		},
		Method: "POST",
		Path:   "/spaces/{space_id}/objects",
	}

	// Process the tool call using the existing handler
	result, err := o.api.HandleToolCall(tool, nil)
	if err != nil {
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
	var properties = map[string]interface{}{
		"body": map[string]interface{}{
			"type":        "string",
			"description": "The structured markdown body of the object. Do not add title or description into body",
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
	for _, relKey := range o.ot.RelationKeys {
		rel := o.subscriber.relations[relKey]
		if rel == nil {
			continue
		}
		if _, ok := properties[rel.Key]; ok {
			continue
		}

		if rel.Format == model.RelationFormat_longtext || rel.Format == model.RelationFormat_shorttext || rel.Format == model.RelationFormat_email || rel.Format == model.RelationFormat_url || rel.Format == model.RelationFormat_phone {
			properties[rel.Key] = map[string]interface{}{
				"type":        "string",
				"description": rel.Name,
			}
		} else if rel.Format == model.RelationFormat_number {
			properties[rel.Key] = map[string]interface{}{
				"type":        "integer",
				"description": rel.Name,
			}
		} else if rel.Format == model.RelationFormat_checkbox {
			properties[rel.Key] = map[string]interface{}{
				"type":        "bool",
				"description": rel.Name,
			}
		} else if rel.Format == model.RelationFormat_date {
			properties[rel.Key] = map[string]interface{}{
				"type":        "integer",
				"description": rel.Name,
			}
		}
	}
	o.subscriber.typesLock.Unlock()
	cleanName := strings.ToLower(strings.ReplaceAll(o.ot.Name, " ", "_"))
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "create_" + cleanName + "_object",
			Description: o.ot.Description,
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   []string{"body"},
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
					s.initTypeTool(&ot)
				}

				s.typesLock.Unlock()
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
	s.addTool(tool, caller)
}

func (s *subscriber) listRelations(ctx context.Context) error {
	typesSub := mb.New[*pb.EventMessage](0)
	subscriptionService := s.service
	subResp, err := subscriptionService.Search(subscription.SubscribeRequest{
		SpaceId: s.spaceId,
		Keys:    []string{bundle.RelationKeyName.String(), bundle.RelationKeyRelationKey.String(), bundle.RelationKeyRelationFormat.String()},
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
					bundle.TypeKeyTemplate.URL(),
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
			ot := detailsToRelation(record)
			s.relations[ot.Key] = &ot
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
				ot := detailsToRelation(domain.NewDetailsFromProto(ev.Details))

				s.typesLock.Lock()
				s.relations[ot.Key] = &ot
				s.typesLock.Unlock()
			}
		}
	}
	return nil
}
