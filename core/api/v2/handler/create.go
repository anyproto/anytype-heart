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

// v2CreateOptionsContextKey mirrors the key the server's
// ensureCreateOptions middleware sets.
const v2CreateOptionsContextKey = "create_options"

// mayCreateOptions reports whether this request consented to minting select
// options for names that match nothing (A2). Absent means no.
func mayCreateOptions(c *gin.Context) bool {
	return c.GetBool(v2CreateOptionsContextKey)
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
//	@Summary		Create an object
//	@Description	A select value naming an option the property does not hold is refused unless `create_options=true` is set. An unknown type or property key is rejected either way, with the closest matches named. The body is either a full AnyBlock document or the shortcut {type, name, properties, markdown}; `version` or `blocks` picks the document form.
//	@Id				create_object
//	@Tags			Objects
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Param			create_options	query	bool					false	"Create select options for names the property does not hold yet (default false: an unmatched name is refused)"
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
		result, err := s.CreateObject(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c), mayCreateOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateTemplateHandler creates a template from an AnyBlock document
//
//	@Summary		Create a template
//	@Description	`template_for` names the type key this template starts an object of.
//	@Id				create_template
//	@Tags			Templates
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Param			create_options	query	bool					false	"Create select options for names the property does not hold yet (default false: an unmatched name is refused)"
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
		result, err := s.CreateTemplate(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c), mayCreateOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateTypeHandler creates a type from a kind:"object_type" document
//
//	@Summary		Create a type
//	@Description	A `type_settings.property_definitions` entry naming a property that does not exist creates it alongside the type. The body is an AnyBlock document with kind "object_type"; the type's api key, layout and plural name live in `type_settings`.
//	@Id				create_type
//	@Tags			Types
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Param			create_options	query	bool					false	"Create select options for names the property does not hold yet (default false: an unmatched name is refused)"
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
		result, err := s.CreateType(c.Request.Context(), c.Param("space_id"), body, isV2DryRun(c), mayCreateOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// UpdateTypeHandler updates a type (type-document semantics)
//
//	@Summary		Update a type
//	@Description	`type_settings.property_definitions`, when present, replaces the recommended property lists rather than adding to them, and creates any property that does not exist yet. `properties` takes name and description; the layout is `type_settings.layout` and the icon is the typed envelope `icon`. Any other key is refused.
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
		result, err := s.UpdateType(c.Request.Context(), c.Param("space_id"), c.Param("type"), body, isV2DryRun(c), mayCreateOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusOK)
	}
}

// DeleteTypeHandler archives a type
//
//	@Summary	Delete a type
//	@Id			delete_type
//	@Tags		Types
//	@Produce	json
//	@Param		space_id	path		string					true	"Space id"
//	@Param		type		path		string					true	"Type key"
//	@Success	200			{object}	v2model.CreateResult	"Archived type"
//	@Failure	404			{object}	v2model.Error			"No live type with this key. A type that is already deleted is a 404 too, not a second delete."
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/types/{type} [delete]
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
//	@Summary	Create a property
//	@Id			create_property
//	@Tags		Properties
//	@Accept		json
//	@Produce	json
//	@Param		space_id	path		string					true	"Space id"
//	@Param		dry_run		query		bool					false	"Validate and report without committing"
//	@Success	201			{object}	v2model.CreateResult	"Created property id + key"
//	@Failure	400			{object}	v2model.Error			"Validation failure"
//	@Failure	413			{object}	v2model.Error			"Request body exceeds the 1 MiB cap"
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties [post]
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
//	@Summary		Update a property
//	@Description	Only the display name can change. The key is the property's identity, and its format is fixed once it exists.
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
//	@Summary	Delete a property
//	@Id			delete_property
//	@Tags		Properties
//	@Produce	json
//	@Param		space_id	path		string					true	"Space id"
//	@Param		key			path		string					true	"Property key"
//	@Success	200			{object}	v2model.CreateResult	"Archived property"
//	@Failure	404			{object}	v2model.Error			"No live property with this key. A property that is already deleted is a 404 too, not a second delete."
//	@Security	bearerauth
//	@Router		/v2/spaces/{space_id}/properties/{key} [delete]
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
//	@Summary		Create a set
//	@Description	Filter and sort property keys are checked against the type the set queries; a key that type does not carry is refused.
//	@Id				create_set
//	@Tags			Lists
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Param			create_options	query	bool					false	"Create select options for names the property does not hold yet (default false: an unmatched name is refused)"
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
		result, err := s.CreateSet(c.Request.Context(), c.Param("space_id"), req, isV2DryRun(c), mayCreateOptions(c))
		if err != nil {
			RespondError(c, err)
			return
		}
		respondV2Create(c, result, http.StatusCreated)
	}
}

// CreateCollectionHandler creates a collection
//
//	@Summary		Create a collection
//	@Description	Item ids are checked against the space; an id that does not resolve there is refused rather than dropped.
//	@Id				create_collection
//	@Tags			Lists
//	@Accept			json
//	@Produce		json
//	@Param			space_id	path		string					true	"Space id"
//	@Param			dry_run		query		bool					false	"Validate and report without committing"
//	@Param			create_options	query	bool					false	"Create select options for names the property does not hold yet (default false: an unmatched name is refused)"
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
//	@Summary		Upload a file
//	@Description	Send multipart/form-data with a `file` field, or JSON {"url": …}. A source that refuses the fetch, or a URL that cannot be fetched, is a 400 naming /url; only a genuine server fault answers 500. The id that comes back is the one file blocks, image blocks and icon_image values reference.
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
//	@Summary	List the available schemas
//	@Id			list_schemas
//	@Tags		Schemas
//	@Produce	json
//	@Success	200	{object}	v2model.SchemaIndex	"Schema index"
//	@Security	bearerauth
//	@Router		/v2/schemas [get]
func SchemaIndexHandler(s *v2service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, s.SchemaIndex())
	}
}

// SchemaKindHandler serves one kind's schema + worked example
//
//	@Summary	Get the schema for one kind
//	@Id			get_schema
//	@Tags		Schemas
//	@Produce	json
//	@Param		kind	path		string				true	"Schema kind, as listed by GET /v2/schemas"
//	@Success	200		{object}	v2model.SchemaEntry	"Schema + example"
//	@Failure	404		{object}	v2model.Error		"Unknown kind"
//	@Security	bearerauth
//	@Router		/v2/schemas/{kind} [get]
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
//	@Summary		Get the schema for one edit op
//	@Description	The example is a single op object, ready to drop into an edit request's `ops` array, not a whole request body.
//	@Id				get_op_schema
//	@Tags			Schemas
//	@Produce		json
//	@Param			op	path		string				true	"Op name: set_properties, update_block, replace_subtree, insert_blocks, move_block, delete_block, replace_text, set_cell, update_view, insert_view, move_view, delete_view, add_items, remove_items"
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
