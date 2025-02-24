package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/go-shiori/go-readability"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/pemistahl/lingua-go"

	"github.com/anyproto/anytype-heart/core/ai/parsing"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/editor/clipboard"
	editorsb "github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/linkpreview"
)

const (
	CName       = "ai"
	httpTimeout = 30 * time.Second
)

var log = logging.Logger(CName)

var (
	ErrRateLimitExceeded    = errors.New("rate limit exceeded")
	ErrUnsupportedLanguage  = errors.New("unsupported input language detected")
	ErrEndpointNotReachable = errors.New("endpoint not reachable")
	ErrModelNotFound        = errors.New("model not found at specified endpoint")
	ErrAuthRequired         = errors.New("api key not provided or invalid for endpoint")
)

type AI interface {
	WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (WritingToolsResult, error)
	Autofill(ctx context.Context, params *pb.RpcAIAutofillRequest) (AutofillResult, error)
	ListSummary(ctx context.Context, params *pb.RpcAIListSummaryRequest) (string, error)
	CreateObjectFromUrl(ctx context.Context, provider *pb.RpcAIProviderConfig, details *types.Struct, spaceId string, url string) (id string, resultDetails *domain.Details, err error)

	WebsiteProcess(ctx context.Context, provider *pb.RpcAIProviderConfig, websiteData []byte) (*WebsiteProcessResult, error)
	ClassifyWebsiteContent(ctx context.Context, content string) (string, error)

	app.ComponentRunnable
}

type AIService struct {
	mu                 sync.Mutex
	exportService      export.Export
	sourceService      source.Service
	linkPreviewService linkpreview.LinkPreview
	spaceService       space.Service
	objectCreator      objectcreator.Service
	blockService       *block.Service
	bmPolicy           *bluemonday.Policy

	apiConfig       *APIConfig
	httpClient      HttpClient
	responseParser  parsing.ResponseParser
	componentCtx    context.Context
	componentCancel context.CancelFunc
}

type APIConfig struct {
	Provider     pb.RpcAIProvider
	Model        string
	Endpoint     string
	AuthRequired bool
	AuthToken    string
}

type PromptConfig struct {
	SystemPrompt string
	UserPrompt   string
	Temperature  float32
	JSONMode     bool
}

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type WritingToolsResult struct {
	Answer string
}

type AutofillResult struct {
	Choices []string
}

type WebsiteProcessResult struct {
	Type            string            // "recipe", "company", or "event"
	Relations       map[string]string // e.g. {"portions": "2", "prep_time": "40 minutes", ...}
	MarkdownSummary string            // e.g. "## Pasta with tomato sauce and basil.\n A classic Italian dish ..."
}

func New() AI {
	return &AIService{
		httpClient: &http.Client{Timeout: httpTimeout},
	}
}

func (ai *AIService) Init(a *app.App) (err error) {
	ai.exportService = app.MustComponent[export.Export](a)
	ai.sourceService = app.MustComponent[source.Service](a)
	ai.linkPreviewService = app.MustComponent[linkpreview.LinkPreview](a)
	ai.spaceService = app.MustComponent[space.Service](a)
	ai.objectCreator = app.MustComponent[objectcreator.Service](a)
	ai.blockService = a.MustComponent(block.CName).(*block.Service)
	ai.bmPolicy = HTMLSanitizePolicy()
	ai.componentCtx, ai.componentCancel = context.WithCancel(context.Background())

	return nil
}

func (ai *AIService) Name() (name string) {
	return CName
}

func (ai *AIService) Run(_ context.Context) (err error) {
	return nil
}

func (ai *AIService) Close(_ context.Context) (err error) {
	ai.componentCancel()
	return nil
}

// WritingTools generates a text response based on the input text and mode.
func (ai *AIService) WritingTools(ctx context.Context, params *pb.RpcAIWritingToolsRequest) (WritingToolsResult, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	log.Infof("received writing tools request with text: %s", strings.ReplaceAll(params.Text, "\n", "\\n"))
	text := strings.ToLower(strings.TrimSpace(params.Text))

	// check supported languages for llama models
	if !(params.Config.Provider == pb.RpcAI_OPENAI) {
		languages := []lingua.Language{lingua.English, lingua.Spanish, lingua.French, lingua.German, lingua.Italian, lingua.Portuguese, lingua.Hindi, lingua.Thai}
		detector := lingua.NewLanguageDetectorBuilder().
			FromLanguages(languages...).
			WithLowAccuracyMode().
			Build()

		if language, exists := detector.DetectLanguageOf(text); !exists {
			log.Errorf("unsupported language detected: %s", language)
			return WritingToolsResult{}, fmt.Errorf("%w: %s", ErrUnsupportedLanguage, language)
		}
	}

	ai.setAPIConfig(params.Config)
	promptConfig := &PromptConfig{
		SystemPrompt: writingToolsPrompts[params.Mode].System,
		UserPrompt:   fmt.Sprintf(writingToolsPrompts[params.Mode].User, text),
		Temperature:  params.Config.Temperature,
		JSONMode:     params.Mode != 0, // use json mode for all modes except default
	}
	ai.responseParser = parsing.NewWritingToolsParser()

	answer, err := ai.chat(ctx, int(params.Mode), promptConfig)
	if err != nil {
		return WritingToolsResult{}, err
	}

	// extract answer value from json response, except for default mode
	if params.Mode != 0 {
		rawAnswer, err := ai.responseParser.ExtractContent(answer, int(params.Mode))
		if err != nil {
			return WritingToolsResult{}, err
		}

		strResult, err := rawAnswer.String()
		if err != nil {
			return WritingToolsResult{}, err
		}

		// fix lmstudio newline issue for table responses
		strResult = strings.ReplaceAll(strResult, "\\\\n", "\n")
		return WritingToolsResult{Answer: strResult}, nil
	}

	return WritingToolsResult{Answer: answer}, nil
}

// Autofill generates suggestions based on the provided options and context.
func (ai *AIService) Autofill(ctx context.Context, params *pb.RpcAIAutofillRequest) (AutofillResult, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	optionsStr := strings.Join(params.Options, ", ")
	contextStr := strings.Join(params.Context, " ")
	log.Infof("received autofill request with options: %s and context: %s", params.Options, params.Context)

	ai.setAPIConfig(params.Config)
	promptConfig := &PromptConfig{
		SystemPrompt: autofillPrompts[params.Mode].System,
		// TODO: create prompt with options and context
		UserPrompt:  fmt.Sprintf(autofillPrompts[params.Mode].User, optionsStr, contextStr),
		Temperature: params.Config.Temperature,
		JSONMode:    true,
	}
	ai.responseParser = parsing.NewAutofillParser()

	answer, err := ai.chat(ctx, int(params.Mode), promptConfig)
	if err != nil {
		return AutofillResult{}, err
	}

	rawAnswer, err := ai.responseParser.ExtractContent(answer, int(params.Mode))
	if err != nil {
		return AutofillResult{}, err
	}

	strResult, err := rawAnswer.String()
	if err != nil {
		return AutofillResult{}, err
	}
	return AutofillResult{Choices: []string{strResult}}, nil
}

// ListSummary answers user questions about a list of items.
func (ai *AIService) ListSummary(ctx context.Context, params *pb.RpcAIListSummaryRequest) (string, error) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	res, err := ai.exportService.ExportInMemory(ctx, params.SpaceId, params.ObjectIds, model.Export_Markdown, true)
	if err != nil {
		return "", err
	}

	s := strings.Builder{}
	for _, r := range res {
		s.Write(r)
		s.WriteString("\n==========\n\n")
	}
	ai.setAPIConfig(params.Config)
	promptConfig := &PromptConfig{
		SystemPrompt: listSummaryPrompts["list"].System,
		UserPrompt:   fmt.Sprintf(listSummaryPrompts["list"].User, params.Prompt, s.String()),
		Temperature:  params.Config.Temperature,
		JSONMode:     true,
	}
	ai.responseParser = parsing.NewWritingToolsParser()

	answer, err := ai.chat(ctx, int(pb.RpcAIWritingToolsRequest_SUMMARIZE), promptConfig)
	if err != nil {
		return "", err
	}

	rawAnswer, err := ai.responseParser.ExtractContent(answer, int(pb.RpcAIWritingToolsRequest_SUMMARIZE))
	if err != nil {
		return "", err
	}

	answerStr, err := rawAnswer.String()
	blocks, rootBlockIds, err := anymark.MarkdownToBlocks([]byte(answerStr), "", nil)
	resultId := uuid.New().String()
	if len(rootBlockIds) == 0 {
		return "", fmt.Errorf("no root block ids found")
	}

	blocks = append(blocks, &model.Block{Id: resultId, ChildrenIds: rootBlockIds, Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}})
	dc := state.NewDocFromSnapshot(resultId, &pb.ChangeSnapshot{
		Data: &model.SmartBlockSnapshotBase{
			Blocks:  blocks,
			Details: common.GetCommonDetails(resultId, "AI response", "✨", model.ObjectType_basic).ToProto(),
			Key:     bundle.TypeKeyPage.String(),
		},
	})

	st := dc.NewState()
	if err = ai.sourceService.RegisterStaticSource(ai.sourceService.NewStaticSource(source.StaticSourceParams{
		Id: domain.FullID{
			SpaceID:  params.SpaceId,
			ObjectID: resultId,
		},
		SbType: smartblock.SmartBlockTypeEphemeralVirtualObject,
		State:  st,
	})); err != nil {
		return "", err
	}

	return resultId, nil
}

func (ai *AIService) processBookmark(ctx context.Context, spaceId, objectId string, body []byte, provider *pb.RpcAIProviderConfig) (id string, details *domain.Details, err error) {
	result, err := ai.WebsiteProcess(ctx, provider, body)
	if err != nil {
		log.Errorf("website process via llm: %v", err)
		return id, details, nil
	}

	if !bundle.HasObjectTypeByKey(domain.TypeKey(result.Type)) {
		return id, details, nil
	}

	var idsToInstallIfMissing []string
	idsToInstallIfMissing = append(idsToInstallIfMissing, domain.TypeKey(result.Type).BundledURL())
	details = domain.NewDetails()
	for k, v := range result.Relations {
		if !bundle.HasRelation(domain.RelationKey(k)) {
			continue
		}
		idsToInstallIfMissing = append(idsToInstallIfMissing, domain.RelationKey(k).BundledURL())

		details.SetString(domain.RelationKey(k), v)
	}

	space, err := ai.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", nil, fmt.Errorf("get space: %w", err)
	}
	_, _, err = ai.objectCreator.InstallBundledObjects(ctx, space, idsToInstallIfMissing, false)
	if err != nil {
		return "", nil, fmt.Errorf("install bundled objects: %w", err)
	}
	cctx := session.NewContext()
	err = space.Do(objectId, func(sb editorsb.SmartBlock) error {
		st := sb.NewState()
		st.SetObjectTypeKey(domain.TypeKey(result.Type))
		// temp fix
		st.SetLocalDetail(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
		st.Details().Delete(bundle.RelationKeyLayout)
		var relationBlocks []*model.Block

		pictureHash := st.Details().GetString(bundle.RelationKeyPicture)
		if pictureHash != "" {
			st.SetDetail(bundle.RelationKeyCoverId, domain.String(pictureHash))
			st.SetDetail(bundle.RelationKeyCoverType, domain.Int64(1))
		}
		for k, v := range details.Iterate() {
			st.SetDetail(k, v)
			if !v.IsEmpty() && k != bundle.RelationKeyName {
				relationBlocks = append(relationBlocks, &model.Block{
					Content: &model.BlockContentOfRelation{
						Relation: &model.BlockContentRelation{
							Key: k.String(),
						},
					},
				})
			}
		}

		var paddingBlocks []*model.Block
		paddingBlocks = append(paddingBlocks, &model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "",
				},
			},
		})
		paddingBlocks = append(paddingBlocks, &model.Block{
			Content: &model.BlockContentOfDiv{
				Div: &model.BlockContentDiv{
					Style: model.BlockContentDiv_Dots,
				},
			},
		})
		paddingBlocks = append(paddingBlocks, &model.Block{
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "",
				},
			},
		})

		err = sb.Apply(st)
		if err != nil {
			return err
		}

		cb := sb.(clipboard.Clipboard)
		_, _, _, _, err = cb.Paste(cctx, &pb.RpcBlockPasteRequest{
			ContextId: id,
			AnySlot:   relationBlocks,
		}, "")
		_, _, _, _, err = cb.Paste(cctx, &pb.RpcBlockPasteRequest{
			ContextId: id,
			AnySlot:   paddingBlocks,
		}, "")
		_, _, _, _, err = cb.Paste(cctx, &pb.RpcBlockPasteRequest{
			ContextId: id,
			TextSlot:  result.MarkdownSummary,
		}, "")

		if err != nil {
			return fmt.Errorf("paste block: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("apply smart block: %w", err)
	}

	err = ai.blockService.CreateTypeWidgetIfMissing(ctx, spaceId, domain.TypeKey(result.Type))
	if err != nil {
		log.Errorf("create type widget: %v", err)
	}

	return id, details, nil
}

func (ai *AIService) cleanResponse(body []byte) (resp []byte) {
	defer func() {
		if e := recover(); e != nil {
			resp = body
		}
	}()
	return ai.bmPolicy.SanitizeBytes(body)
}
func (ai *AIService) CreateObjectFromUrl(ctx context.Context, provider *pb.RpcAIProviderConfig, details *types.Struct, spaceId string, url string) (id string, resultDetails *domain.Details, err error) {
	_, body, isFile, err := ai.linkPreviewService.Fetch(ctx, url)
	if err != nil {
		return "", nil, fmt.Errorf("could not fetch website: %w", err)
	}

	body = ai.cleanResponse(body)
	resultDetails = domain.NewDetailsFromProto(details)
	resultDetails.SetString(bundle.RelationKeySource, url)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyBookmark,
		Details:       resultDetails,
	}
	id, resultDetails, err = ai.objectCreator.CreateObject(ctx, spaceId, createReq)
	if err != nil {
		return "", nil, fmt.Errorf("create as bookmark: %w", err)
	}

	if !isFile {
		go func(spaceId string, body []byte, provider *pb.RpcAIProviderConfig) {
			_, _, err = ai.processBookmark(ai.componentCtx, spaceId, id, body, provider)
			if err != nil {
				log.Errorf("ai process bookmark: %v", err)
			}
		}(spaceId, body, provider)
	}

	return id, resultDetails, nil
}

// WebsiteProcess fetches a URL, classifies it, and extracts relations and a summary. Should be called internally only.
func (ai *AIService) WebsiteProcess(ctx context.Context, provider *pb.RpcAIProviderConfig, websiteData []byte) (*WebsiteProcessResult, error) {
	article, err := readability.FromReader(bytes.NewReader(websiteData), nil)
	if err != nil {
		return nil, fmt.Errorf("could not process website content: %w", err)
	}
	content := article.Content
	if content == "" {
		return nil, fmt.Errorf("website content is empty")
	}

	ai.setAPIConfig(provider)
	websiteType, err := ai.ClassifyWebsiteContent(ctx, article.TextContent)
	if err != nil {
		return nil, fmt.Errorf("could not classify website content: %w", err)
	}

	prompts, ok := websiteExtractionPrompts[websiteType]
	if !ok {
		return nil, fmt.Errorf("no extraction prompts for website type: %s", websiteType)
	}

	relationPrompt := fmt.Sprintf(prompts.RelationPrompt, content)
	summaryPrompt := fmt.Sprintf(prompts.SummaryPrompt, content)
	websiteTypeToMode := map[string]int{"recipe": 1, "company": 2, "event": 3}

	var (
		relationsResult map[string]string
		summaryResult   string
		relErr, sumErr  error
	)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		promptConfig := &PromptConfig{
			SystemPrompt: "",
			UserPrompt:   relationPrompt,
			Temperature:  provider.Temperature,
			JSONMode:     true,
		}
		ai.responseParser = parsing.NewWebsiteProcessParser()

		answer, err := ai.chat(ctx, websiteTypeToMode[websiteType], promptConfig)
		if err != nil {
			relErr = err
			return
		}
		rawAnswer, err := ai.responseParser.ExtractContent(answer, websiteTypeToMode[websiteType])
		if err != nil {
			relErr = err
			return
		}

		mapResult, err := rawAnswer.Map()
		if err != nil {
			relErr = err
			return
		}
		relationsResult = mapResult
	}()

	go func() {
		defer wg.Done()
		promptConfig := &PromptConfig{
			SystemPrompt: "",
			UserPrompt:   summaryPrompt,
			Temperature:  provider.Temperature,
			JSONMode:     false,
		}
		ai.responseParser = parsing.NewWebsiteProcessParser()
		answer, err := ai.chat(ctx, 2, promptConfig)
		if err != nil {
			sumErr = err
			return
		}
		summaryResult = answer
	}()

	wg.Wait()
	if relErr != nil {
		return nil, fmt.Errorf("relation extraction failed: %w", relErr)
	}
	if sumErr != nil {
		return nil, fmt.Errorf("summary extraction failed: %w", sumErr)
	}

	log.Infof("website processed successfully, type: %s", websiteType)
	log.Debugf("relations: %v", relationsResult)
	log.Debugf("summary: %s", summaryResult)

	return &WebsiteProcessResult{
		Type:            websiteType,
		Relations:       relationsResult,
		MarkdownSummary: summaryResult,
	}, nil
}

// ClassifyWebsiteContent classifies content into a single category.
func (ai *AIService) ClassifyWebsiteContent(ctx context.Context, content string) (string, error) {
	systemPrompt := classificationPrompts["type"].System
	userPrompt := fmt.Sprintf(classificationPrompts["type"].User, content[:min(len(content), 1000)])
	promptConfig := &PromptConfig{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  0,
		JSONMode:     false,
	}

	answer, err := ai.chat(ctx, 0, promptConfig)
	if err != nil {
		return "", err
	}

	classification := strings.ToLower(strings.TrimSpace(answer))
	switch classification {
	case "recipe", "company", "event":
		return classification, nil
	default:
		return "", fmt.Errorf("invalid classification: %s", classification)
	}
}

func (ai *AIService) setAPIConfig(params *pb.RpcAIProviderConfig) {
	ai.apiConfig = &APIConfig{
		Provider:     params.Provider,
		Endpoint:     params.Endpoint,
		Model:        params.Model,
		AuthRequired: params.Provider == pb.RpcAI_OPENAI,
		AuthToken:    params.Token,
	}
}

func HTMLSanitizePolicy() *bluemonday.Policy {

	p := bluemonday.NewPolicy()

	// /////////////////////
	// Global attributes //
	// /////////////////////

	// "class" is not permitted as we are not allowing users to style their own
	// content

	p.AllowStandardAttributes()

	// //////////////////////////////
	// Declarations and structure //
	// //////////////////////////////

	// "xml" "xslt" "DOCTYPE" "html" "head" are not permitted as we are
	// expecting user generated content to be a fragment of HTML and not a full
	// document.

	// ////////////////////////
	// Sectioning root tags //
	// ////////////////////////

	// "article" and "aside" are permitted and takes no attributes
	p.AllowElements("article", "aside")

	// "body" is not permitted as we are expecting user generated content to be a fragment
	// of HTML and not a full document.

	// "details" is permitted, including the "open" attribute which can either
	// be blank or the value "open".
	p.AllowAttrs(
		"open",
	).Matching(regexp.MustCompile(`(?i)^(|open)$`)).OnElements("details")

	// "fieldset" is not permitted as we are not allowing forms to be created.

	// "figure" is permitted and takes no attributes
	p.AllowElements("figure")

	// "nav" is not permitted as it is assumed that the site (and not the user)
	// has defined navigation elements

	// "section" is permitted and takes no attributes
	p.AllowElements("section")

	// "summary" is permitted and takes no attributes
	p.AllowElements("summary")

	// ////////////////////////
	// Headings and footers //
	// ////////////////////////

	// "footer" is not permitted as we expect user content to be a fragment and
	// not structural to this extent

	// "h1" through "h6" are permitted and take no attributes
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")

	// "header" is not permitted as we expect user content to be a fragment and
	// not structural to this extent

	// "hgroup" is permitted and takes no attributes
	p.AllowElements("hgroup")

	// ///////////////////////////////////
	// Content grouping and separating //
	// ///////////////////////////////////

	// "blockquote" is permitted, including the "cite" attribute which must be
	// a standard URL.
	p.AllowAttrs("cite").OnElements("blockquote")

	// "br" "div" "hr" "p" "span" "wbr" are permitted and take no attributes
	p.AllowElements("br", "div", "hr", "p", "span", "wbr")

	// "link" is not permitted

	// ///////////////////
	// Phrase elements //
	// ///////////////////

	// The following are all inline phrasing elements
	p.AllowElements("abbr", "acronym", "cite", "code", "dfn", "em",
		"figcaption", "mark", "s", "samp", "strong", "sub", "sup", "var")

	// "q" is permitted and "cite" is a URL and handled by URL policies
	p.AllowAttrs("cite").OnElements("q")

	// "time" is permitted
	p.AllowAttrs("datetime").Matching(bluemonday.ISO8601).OnElements("time")

	// //////////////////
	// Style elements //
	// //////////////////

	// block and inline elements that impart no semantic meaning but style the
	// document
	p.AllowElements("b", "i", "pre", "small", "strike", "tt", "u")

	// "style" is not permitted as we are not yet sanitising CSS and it is an
	// XSS attack vector

	// ////////////////////
	// HTML5 Formatting //
	// ////////////////////

	// "bdi" "bdo" are permitted
	p.AllowAttrs("dir").Matching(bluemonday.Direction).OnElements("bdi", "bdo")

	// "rp" "rt" "ruby" are permitted
	p.AllowElements("rp", "rt", "ruby")

	// /////////////////////////
	// HTML5 Change tracking //
	// /////////////////////////

	// "del" "ins" are permitted
	p.AllowAttrs("cite").Matching(bluemonday.Paragraph).OnElements("del", "ins")
	p.AllowAttrs("datetime").Matching(bluemonday.ISO8601).OnElements("del", "ins")

	// /////////
	// Lists //
	// /////////

	p.AllowLists()

	// //////////
	// Tables //
	// //////////

	p.AllowTables()

	// /////////
	// Forms //
	// /////////

	// By and large, forms are not permitted. However there are some form
	// elements that can be used to present data, and we do permit those
	//
	// "button" "fieldset" "input" "keygen" "label" "output" "select" "datalist"
	// "textarea" "optgroup" "option" are all not permitted

	// "meter" is permitted
	p.AllowAttrs(
		"value",
		"min",
		"max",
		"low",
		"high",
		"optimum",
	).Matching(bluemonday.Number).OnElements("meter")

	// "progress" is permitted
	p.AllowAttrs("value", "max").Matching(bluemonday.Number).OnElements("progress")

	// ////////////////////
	// Embedded content //
	// ////////////////////

	// Vast majority not permitted
	// "audio" "canvas" "embed" "iframe" "object" "param" "source" "svg" "track"
	// "video" are all not permitted

	p.AllowImages()

	p.AddSpaceWhenStrippingTag(true)
	return p
}
