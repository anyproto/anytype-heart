package handler

// v2_create.go holds the Phase-2 create/update handlers (APIV2.md §2). All
// POST routes run behind the C8 idempotency middleware; every mutation
// honors ?dry_run=true (C9) via the context flag the dry-run middleware
// sets.

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/service"
)

// v2DryRunContextKey mirrors the key the server's ensureDryRun middleware
// sets (C9); the handler package cannot import server (import cycle).
const v2DryRunContextKey = "dry_run"

// isV2DryRun reports whether the request asked for a dry run.
func isV2DryRun(c *gin.Context) bool {
	return c.GetBool(v2DryRunContextKey)
}

// maxV2CreateBodySize bounds create request bodies (matches /v2/validate).
const maxV2CreateBodySize = 10 << 20 // 10 MiB

// readV2Body reads a bounded request body; a nil return means the error
// response was already written.
func readV2Body(c *gin.Context) []byte {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxV2CreateBodySize+1))
	if err != nil {
		RespondV2Error(c, apimodel.V2ValidationFailed("read request body: "+err.Error()))
		return nil
	}
	if len(body) > maxV2CreateBodySize {
		RespondV2Error(c, apimodel.V2RequestTooLarge("request body exceeds the 10 MiB limit"))
		return nil
	}
	return body
}

// respondV2Create writes a create/update result: 201 on a real create, 200
// on dry runs and updates, with the ETag header when known.
func respondV2Create(c *gin.Context, result *apimodel.V2CreateResult, createdStatus int) {
	if result.Etag != "" {
		c.Header("ETag", service.QuoteEtag(result.Etag))
	}
	status := createdStatus
	if result.DryRun {
		status = http.StatusOK
	}
	c.JSON(status, result)
}

// CreateObjectV2Handler creates an object from an AnyBlock document or the shortcut
//
//	@Summary		Create object (AnyBlock)
//	@Description	Creates one object. The body is either a full flat AnyBlock document (discriminator: presence of version or blocks) or the shortcut {type, name, properties, markdown}. Unknown select option names are created (SPEC §3); unknown type or property keys are rejected with did-you-mean errors. Honors Idempotency-Key (C8) and ?dry_run=true (C9).
//	@Id				v2_create_object
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2CreateResult	"Created object id + etag"
//	@Failure		400			{object}	apimodel.V2Error		"Validation or reference failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects [post]
func CreateObjectV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.CreateObject(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateTemplateV2Handler creates a template from an AnyBlock document
//
//	@Summary		Create template (AnyBlock)
//	@Description	Creates a template: an AnyBlock document with templateFor naming the target type key, routed through the generic object-create path. Honors Idempotency-Key and ?dry_run=true.
//	@Id				v2_create_template
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2CreateResult	"Created template id"
//	@Failure		400			{object}	apimodel.V2Error		"Validation or reference failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/templates [post]
func CreateTemplateV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.CreateTemplate(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateTypeV2Handler creates a type from a kind:"objectType" document
//
//	@Summary		Create type (AnyBlock type document)
//	@Description	Creates an object type from a kind:"objectType" AnyBlock document. typeProperties entries naming unknown property keys create those properties atomically with the type (SPEC §2a create-missing). Honors Idempotency-Key and ?dry_run=true.
//	@Id				v2_create_type
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2CreateResult	"Created type id + key"
//	@Failure		400			{object}	apimodel.V2Error		"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types [post]
func CreateTypeV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.CreateType(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UpdateTypeV2Handler updates a type (type-document semantics)
//
//	@Summary		Update type
//	@Description	Partial type-document update: properties changes the type's own details (name, description, iconEmoji, recommendedLayout); typeProperties, when present, rebuilds the recommended property lists, creating missing properties (SPEC §2a).
//	@Id				v2_update_type
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			type		path		string					true	"Type key"
//	@Success		200			{object}	apimodel.V2CreateResult	"Updated type"
//	@Failure		404			{object}	apimodel.V2Error		"Type not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types/{type} [patch]
func UpdateTypeV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.UpdateType(c.Request.Context(), c.Param("space_id"), c.Param("type"), body, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// DeleteTypeV2Handler archives a type
//
//	@Summary		Delete type (archive)
//	@Description	Archives the type object (v1 parity; hard delete is a deferred ?permanent extension).
//	@Id				v2_delete_type
//	@Tags			V2
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			type		path		string					true	"Type key"
//	@Success		200			{object}	apimodel.V2CreateResult	"Archived type"
//	@Failure		404			{object}	apimodel.V2Error		"Type not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types/{type} [delete]
func DeleteTypeV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteType(c.Request.Context(), c.Param("space_id"), c.Param("type"), isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// CreatePropertyV2Handler creates a property
//
//	@Summary		Create property
//	@Description	Creates a property: {key?, name, format, options?:[{name,color?}]}. Formats use the AnyBlock vocabulary (text, select, multiSelect, …). Honors Idempotency-Key and ?dry_run=true.
//	@Id				v2_create_property
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2CreateResult	"Created property id + key"
//	@Failure		400			{object}	apimodel.V2Error		"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/properties [post]
func CreatePropertyV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2CreatePropertyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondV2Error(c, apimodel.V2ValidationFailed("invalid property body: "+err.Error()))
			return
		}
		result, err := s.CreateProperty(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UpdatePropertyV2Handler updates a property
//
//	@Summary		Update property
//	@Description	Updates a property's display name. The key is identity and cannot change.
//	@Id				v2_update_property
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			key			path		string					true	"Property key"
//	@Success		200			{object}	apimodel.V2CreateResult	"Updated property"
//	@Failure		404			{object}	apimodel.V2Error		"Property not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/properties/{key} [patch]
func UpdatePropertyV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2UpdatePropertyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondV2Error(c, apimodel.V2ValidationFailed("invalid property patch: "+err.Error()))
			return
		}
		result, err := s.UpdateProperty(c.Request.Context(), c.Param("space_id"), c.Param("key"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// DeletePropertyV2Handler archives a property
//
//	@Summary		Delete property (archive)
//	@Description	Archives the property object.
//	@Id				v2_delete_property
//	@Tags			V2
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			key			path		string					true	"Property key"
//	@Success		200			{object}	apimodel.V2CreateResult	"Archived property"
//	@Failure		404			{object}	apimodel.V2Error		"Property not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/properties/{key} [delete]
func DeletePropertyV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteProperty(c.Request.Context(), c.Param("space_id"), c.Param("key"), isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// CreateSetV2Handler creates a set with its views in one change set
//
//	@Summary		Create set
//	@Description	Creates a set querying one type: {name, type, filters?, sorts?, views?}. The set's initial state carries a fully-formed dataview block, so filters/sorts/views land atomically (§8/R10). Filter/sort property keys are validated against the type's actual keys (R9). Honors Idempotency-Key and ?dry_run=true.
//	@Id				v2_create_set
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2CreateResult	"Created set id"
//	@Failure		400			{object}	apimodel.V2Error		"Validation or reference failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/sets [post]
func CreateSetV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2CreateSetRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondV2Error(c, apimodel.V2ValidationFailed("invalid set body: "+err.Error()))
			return
		}
		result, err := s.CreateSet(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateCollectionV2Handler creates a collection
//
//	@Summary		Create collection
//	@Description	Creates a collection: {name, items?}. Items are validated against the space. Honors Idempotency-Key and ?dry_run=true.
//	@Id				v2_create_collection
//	@Tags			V2
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2CreateResult	"Created collection id"
//	@Failure		400			{object}	apimodel.V2Error		"Validation or reference failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/collections [post]
func CreateCollectionV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req apimodel.V2CreateCollectionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondV2Error(c, apimodel.V2ValidationFailed("invalid collection body: "+err.Error()))
			return
		}
		result, err := s.CreateCollection(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UploadFileV2Handler uploads a file (multipart or URL)
//
//	@Summary		Upload file
//	@Description	Uploads one file and returns the file object id that file/image blocks and iconImage values reference (R11). Send multipart/form-data with a file field, or JSON {"url": …}.
//	@Id				v2_upload_file
//	@Tags			V2
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			space_id	path		string						true	"Space id"
//	@Param			dry_run		query		bool						false	"Validate and report without committing"
//	@Success		201			{object}	apimodel.V2FileUploadResult	"Created file object id"
//	@Failure		400			{object}	apimodel.V2Error			"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/files [post]
func UploadFileV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		spaceId := c.Param("space_id")
		dryRun := isV2DryRun(c)

		if strings.HasPrefix(c.ContentType(), "multipart/") {
			localPath, cleanup, ok := stageV2Upload(c)
			if !ok {
				return
			}
			defer cleanup()
			result, err := s.UploadFile(c.Request.Context(), spaceId, localPath, "", dryRun)
			if err != nil {
				RespondV2Error(c, err)
				return
			}
			respondUpload(c, result)
			return
		}

		var req apimodel.V2UploadFileRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			RespondV2Error(c, apimodel.V2ValidationFailed("invalid upload body: "+err.Error(),
				apimodel.V2Issue{Message: "send multipart/form-data with a file field, or JSON {\"url\": …}"}))
			return
		}
		result, err := s.UploadFile(c.Request.Context(), spaceId, "", req.Url, dryRun)
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		respondUpload(c, result)
	}
}

func respondUpload(c *gin.Context, result *apimodel.V2FileUploadResult) {
	status := http.StatusCreated
	if result.DryRun {
		status = http.StatusOK
	}
	c.JSON(status, result)
}

// stageV2Upload copies the multipart file into a private temp dir so the
// upload pipeline sees the caller-supplied filename (mirrors the v1
// handler's staging, including the path-traversal guard).
func stageV2Upload(c *gin.Context) (localPath string, cleanup func(), ok bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		RespondV2Error(c, apimodel.V2ValidationFailed("missing file field in multipart form"))
		return "", nil, false
	}
	file, err := fileHeader.Open()
	if err != nil {
		RespondV2Error(c, apimodel.V2ValidationFailed("read uploaded file: "+err.Error()))
		return "", nil, false
	}
	defer file.Close()

	tempDir, err := os.MkdirTemp("", "anytype-upload-")
	if err != nil {
		RespondV2Error(c, err)
		return "", nil, false
	}
	cleanup = func() { _ = os.RemoveAll(tempDir) }

	name := filepath.Base(fileHeader.Filename)
	if name == "." || name == ".." || name == "" || strings.ContainsRune(name, 0) {
		name = "upload"
	}
	tempPath := filepath.Join(tempDir, name)
	tempFile, err := os.Create(tempPath)
	if err != nil {
		cleanup()
		RespondV2Error(c, err)
		return "", nil, false
	}
	_, err = io.Copy(tempFile, file)
	tempFile.Close()
	if err != nil {
		cleanup()
		RespondV2Error(c, err)
		return "", nil, false
	}
	return tempPath, cleanup, true
}

// SchemaIndexV2Handler lists the discoverable schemas
//
//	@Summary		List schemas
//	@Description	The §5 discovery index: every create kind with its endpoint and schema URL.
//	@Id				v2_list_schemas
//	@Tags			V2
//	@Produce		json
//	@Success		200	{object}	apimodel.V2SchemaIndex	"Schema index"
//	@Security		bearerauth
//	@Router			/v2/schemas [get]
func SchemaIndexV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.SchemaIndex())
	}
}

// SchemaKindV2Handler serves one kind's schema + worked example
//
//	@Summary		Get schema for a kind
//	@Description	One kind's JSON Schema and worked example (C12/C13). Kinds: object, shortcut, type, template, property, set, collection, file, filters.
//	@Id				v2_get_schema
//	@Tags			V2
//	@Produce		json
//	@Param			kind	path		string					true	"Schema kind"
//	@Success		200		{object}	apimodel.V2SchemaEntry	"Schema + example"
//	@Failure		404		{object}	apimodel.V2Error		"Unknown kind"
//	@Security		bearerauth
//	@Router			/v2/schemas/{kind} [get]
func SchemaKindV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := s.SchemaKind(c.Param("kind"))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}

// SchemaOpV2Handler serves one PATCH op's schema + minimal example
//
//	@Summary		Get schema for a PATCH op
//	@Description	One op's tiny strict schema (C13) and a single-op minimal example. Ops: setProperties, updateBlock, replaceBlock, replaceSubtree, insertBlocks, moveBlock, deleteBlock, replaceText, setCell, addItems, removeItems.
//	@Id				v2_get_op_schema
//	@Tags			V2
//	@Produce		json
//	@Param			op	path		string					true	"Op name"
//	@Success		200	{object}	apimodel.V2SchemaEntry	"Schema + example"
//	@Failure		404	{object}	apimodel.V2Error		"Unknown op"
//	@Security		bearerauth
//	@Router			/v2/schemas/ops/{op} [get]
func SchemaOpV2Handler(s *service.V2Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := s.SchemaOp(c.Param("op"))
		if err != nil {
			RespondV2Error(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}
