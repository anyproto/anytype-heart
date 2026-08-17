package v2handler

// create.go holds the Phase-2 create/update handlers (APIV2.md §2). All
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

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
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

// maxV2StructuredBodySize caps the five typed Phase-2 JSON bodies (property
// create/update, set, collection, file-by-URL): each is a small descriptor,
// nothing like the 10 MiB document channel. Before these routes went through
// decodeStrictJSONBody they bound with ShouldBindJSON — unknown fields
// silently dropped while GET /v2/schemas advertises
// additionalProperties:false for exactly these kinds — and read UNBOUNDED
// bodies whenever no Idempotency-Key engaged the middleware cap (surface
// review M6).
const maxV2StructuredBodySize = 1 << 20 // 1 MiB

// readV2Body reads a bounded request body; a nil return means the error
// response was already written.
func readV2Body(c *gin.Context) []byte {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxV2CreateBodySize+1))
	if err != nil {
		RespondError(c, v2model.ValidationFailed("read request body: "+err.Error()))
		return nil
	}
	if len(body) > maxV2CreateBodySize {
		RespondError(c, v2model.RequestTooLarge("request body exceeds the 10 MiB limit"))
		return nil
	}
	return body
}

// respondV2Create writes a create/update result: 201 on a real create, 200
// on dry runs and updates, with the ETag header when known.
func respondV2Create(c *gin.Context, result *v2model.CreateResult, createdStatus int) {
	if result.Etag != "" {
		c.Header("ETag", v2service.QuoteEtag(result.Etag))
	}
	status := createdStatus
	if result.DryRun {
		status = http.StatusOK
	}
	c.JSON(status, result)
}

// CreateObjectHandler creates an object from an AnyBlock document or the shortcut
//
//	@Summary		Create object (AnyBlock)
//	@Description	Creates one object. The body is either a full flat AnyBlock document (discriminator: presence of version or blocks) or the shortcut {type, name, properties, markdown}. Unknown select option names are created (SPEC §3); unknown type or property keys are rejected with did-you-mean errors. Honors Idempotency-Key (C8) and ?dry_run=true (C9).
//	@Id				create_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	v2model.CreateResult	"Created object id + etag"
//	@Failure		400			{object}	v2model.Error			"Validation or reference failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/objects [post]
func CreateObjectHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.CreateObject(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateTemplateHandler creates a template from an AnyBlock document
//
//	@Summary		Create template (AnyBlock)
//	@Description	Creates a template: an AnyBlock document with templateFor naming the target type key, routed through the generic object-create path. Honors Idempotency-Key and ?dry_run=true.
//	@Id				create_template
//	@Tags			Templates
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	v2model.CreateResult	"Created template id"
//	@Failure		400			{object}	v2model.Error			"Validation or reference failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/templates [post]
func CreateTemplateHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.CreateTemplate(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateTypeHandler creates a type from a kind:"objectType" document
//
//	@Summary		Create type (AnyBlock type document)
//	@Description	Creates an object type from a kind:"objectType" AnyBlock document. typeProperties entries naming unknown property keys create those properties atomically with the type (SPEC §2a create-missing). Honors Idempotency-Key and ?dry_run=true.
//	@Id				create_type
//	@Tags			Types
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	v2model.CreateResult	"Created type id + key"
//	@Failure		400			{object}	v2model.Error			"Validation failure"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types [post]
func CreateTypeHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.CreateType(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UpdateTypeHandler updates a type (type-document semantics)
//
//	@Summary		Update type
//	@Description	Partial type-document update: properties changes the type's own details (name, description, icon_emoji, recommended_layout); typeProperties, when present, rebuilds the recommended property lists, creating missing properties (SPEC §2a).
//	@Id				update_type
//	@Tags			Types
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			type		path		string					true	"Type key"
//	@Success		200			{object}	v2model.CreateResult	"Updated type"
//	@Failure		404			{object}	v2model.Error			"Type not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types/{type} [patch]
func UpdateTypeHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := readV2Body(c)
		if body == nil {
			return
		}
		result, err := s.UpdateType(c.Request.Context(), c.Param("space_id"), c.Param("type"), body, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// DeleteTypeHandler archives a type
//
//	@Summary		Delete type (archive)
//	@Description	Archives the type object (v1 parity; hard delete is a deferred ?permanent extension).
//	@Id				delete_type
//	@Tags			Types
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			type		path		string					true	"Type key"
//	@Success		200			{object}	v2model.CreateResult	"Archived type"
//	@Failure		404			{object}	v2model.Error			"Type not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/types/{type} [delete]
func DeleteTypeHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteType(c.Request.Context(), c.Param("space_id"), c.Param("type"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// CreatePropertyHandler creates a property
//
//	@Summary		Create property
//	@Description	Creates a property: {key?, name, format, options?:[{name,color?}]}. Formats use the AnyBlock vocabulary (text, select, multiSelect, …). The body binds strictly: unknown fields are rejected with the field named, and the bounds the property schema advertises (name/key lengths, key pattern, option count) are enforced. Honors Idempotency-Key and ?dry_run=true.
//	@Id				create_property
//	@Tags			Properties
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	v2model.CreateResult	"Created property id + key"
//	@Failure		400			{object}	v2model.Error			"Validation failure"
//	@Failure		413			{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/properties [post]
func CreatePropertyHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreatePropertyRequest
		if !decodeStrictJSONBody(c, &req,
			"the property body is {key?, name, format, options?} — GET /v2/schemas/property for the schema",
			maxV2StructuredBodySize, "property") {
			return
		}
		result, err := s.CreateProperty(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UpdatePropertyHandler updates a property
//
//	@Summary		Update property
//	@Description	Updates a property's display name. The key is identity and cannot change. The body binds strictly: unknown fields are rejected with the field named.
//	@Id				update_property
//	@Tags			Properties
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			key			path		string					true	"Property key"
//	@Success		200			{object}	v2model.CreateResult	"Updated property"
//	@Failure		400			{object}	v2model.Error			"Validation failure"
//	@Failure		404			{object}	v2model.Error			"Property not found"
//	@Failure		413			{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/properties/{key} [patch]
func UpdatePropertyHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.UpdatePropertyRequest
		if !decodeStrictJSONBody(c, &req,
			"the property patch takes name — the key is identity and cannot change",
			maxV2StructuredBodySize, "property") {
			return
		}
		result, err := s.UpdateProperty(c.Request.Context(), c.Param("space_id"), c.Param("key"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// DeletePropertyHandler archives a property
//
//	@Summary		Delete property (archive)
//	@Description	Archives the property object.
//	@Id				delete_property
//	@Tags			Properties
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			key			path		string					true	"Property key"
//	@Success		200			{object}	v2model.CreateResult	"Archived property"
//	@Failure		404			{object}	v2model.Error			"Property not found"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/properties/{key} [delete]
func DeletePropertyHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := s.DeleteProperty(c.Request.Context(), c.Param("space_id"), c.Param("key"), isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// CreateSetHandler creates a set with its views in one change set
//
//	@Summary		Create set
//	@Description	Creates a set querying one type: {name, type, filters?, sorts?, views?}. The set's initial state carries a fully-formed dataview block, so filters/sorts/views land atomically (§8/R10). Filter/sort property keys are validated against the type's actual keys (R9). The body binds strictly (unknown fields rejected) and the set schema's advertised bounds are enforced. Honors Idempotency-Key and ?dry_run=true.
//	@Id				create_set
//	@Tags			Lists
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	v2model.CreateResult	"Created set id"
//	@Failure		400			{object}	v2model.Error			"Validation or reference failure"
//	@Failure		413			{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/sets [post]
func CreateSetHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreateSetRequest
		if !decodeStrictJSONBody(c, &req,
			"the set body is {name, type, filter?/filters?, sorts?, views?} — GET /v2/schemas/set for the schema",
			maxV2StructuredBodySize, "set") {
			return
		}
		result, err := s.CreateSet(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateCollectionHandler creates a collection
//
//	@Summary		Create collection
//	@Description	Creates a collection: {name, items?}. Items are validated against the space. The body binds strictly (unknown fields rejected) and the collection schema's advertised bounds (name length, item count) are enforced. Honors Idempotency-Key and ?dry_run=true.
//	@Id				create_collection
//	@Tags			Lists
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Success		201			{object}	v2model.CreateResult	"Created collection id"
//	@Failure		400			{object}	v2model.Error			"Validation or reference failure"
//	@Failure		413			{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/collections [post]
func CreateCollectionHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req v2model.CreateCollectionRequest
		if !decodeStrictJSONBody(c, &req,
			"the collection body is {name, items?} — GET /v2/schemas/collection for the schema",
			maxV2StructuredBodySize, "collection") {
			return
		}
		result, err := s.CreateCollection(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UploadFileHandler uploads a file (multipart or URL)
//
//	@Summary		Upload file
//	@Description	Uploads one file and returns the file object id that file/image blocks and icon_image values reference (R11). Send multipart/form-data with a file field, or JSON {"url": …} — the JSON body binds strictly (unknown fields rejected, 1 MiB cap). A URL the source refuses or that cannot be fetched is a 400 naming /url; only genuine server faults answer 500.
//	@Id				upload_file
//	@Tags			Files
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			space_id	path		string						true	"Space id"
//	@Param			dry_run		query		bool						false	"Validate and report without committing"
//	@Success		201			{object}	v2model.FileUploadResult	"Created file object id"
//	@Failure		400			{object}	v2model.Error				"Validation failure, or a source URL that did not yield the file"
//	@Failure		413			{object}	v2model.Error				"JSON request body exceeds the 1 MiB cap"
//	@Security		bearerauth
//	@Router			/v2/spaces/{space_id}/files [post]
func UploadFileHandler(s *v2service.Service) gin.HandlerFunc {
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
				RespondError(c, err)
				return
			}
			respondUpload(c, result)
			return
		}

		var req v2model.UploadFileRequest
		if !decodeStrictJSONBody(c, &req,
			"send multipart/form-data with a file field, or JSON {\"url\": …} — GET /v2/schemas/file for the schema",
			maxV2StructuredBodySize, "file") {
			return
		}
		result, err := s.UploadFile(c.Request.Context(), spaceId, "", req.Url, dryRun)
		if err != nil {
			RespondError(c, err)
			return
		}
		respondUpload(c, result)
	}
}

func respondUpload(c *gin.Context, result *v2model.FileUploadResult) {
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
		RespondError(c, v2model.ValidationFailed("missing file field in multipart form"))
		return "", nil, false
	}
	file, err := fileHeader.Open()
	if err != nil {
		RespondError(c, v2model.ValidationFailed("read uploaded file: "+err.Error()))
		return "", nil, false
	}
	defer file.Close()

	tempDir, err := os.MkdirTemp("", "anytype-upload-")
	if err != nil {
		RespondError(c, err)
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
		RespondError(c, err)
		return "", nil, false
	}
	_, err = io.Copy(tempFile, file)
	tempFile.Close()
	if err != nil {
		cleanup()
		RespondError(c, err)
		return "", nil, false
	}
	return tempPath, cleanup, true
}

// SchemaIndexHandler lists the discoverable schemas
//
//	@Summary		List schemas
//	@Description	The §5 discovery index: every create kind with its endpoint and schema URL.
//	@Id				list_schemas
//	@Tags			Schemas
//	@Produce		json
//	@Success		200	{object}	v2model.SchemaIndex	"Schema index"
//	@Security		bearerauth
//	@Router			/v2/schemas [get]
func SchemaIndexHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.SchemaIndex())
	}
}

// SchemaKindHandler serves one kind's schema + worked example
//
//	@Summary		Get schema for a kind
//	@Description	One kind's JSON Schema and worked example (C12/C13). Kinds: object, shortcut, type, template, property, set, collection, file, filters.
//	@Id				get_schema
//	@Tags			Schemas
//	@Produce		json
//	@Param			kind	path		string				true	"Schema kind"
//	@Success		200		{object}	v2model.SchemaEntry	"Schema + example"
//	@Failure		404		{object}	v2model.Error		"Unknown kind"
//	@Security		bearerauth
//	@Router			/v2/schemas/{kind} [get]
func SchemaKindHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := s.SchemaKind(c.Param("kind"))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}

// SchemaOpHandler serves one PATCH op's schema + minimal example
//
//	@Summary		Get schema for a PATCH op
//	@Description	One op's tiny strict schema (C13) and a minimal example that is an INSTANCE of it — one op object, ready to drop into the PATCH body's ops array. Ops: setProperties, updateBlock, replaceSubtree, insertBlocks, moveBlock, deleteBlock, replaceText, setCell, updateView, insertView, moveView, deleteView, addItems, removeItems.
//	@Id				get_op_schema
//	@Tags			Schemas
//	@Produce		json
//	@Param			op	path		string				true	"Op name"
//	@Success		200	{object}	v2model.SchemaEntry	"Schema + example"
//	@Failure		404	{object}	v2model.Error		"Unknown op"
//	@Security		bearerauth
//	@Router			/v2/schemas/ops/{op} [get]
func SchemaOpHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry, err := s.SchemaOp(c.Param("op"))
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, entry)
	}
}
