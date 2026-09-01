fmt:
	@echo 'Formatting with prettier...'
	@npx prettier --write "./**" 2> /dev/null || true
	@echo 'Formatting with goimports...'
	@goimports -w -l `find . -type f -name '*.go' -not -path './vendor/*'`

lint: run-linter

# strip-openapi-prefixes <output-dir> <instance-name> <model-package>
#
# swag names every schema "<package>.<Type>"; the published documents use bare
# type names. The rules are per document — v1's models live in package
# apimodel, v2's in v2model — and a missed rule is visible in the schema names
# on the very first generation, not silent. --instanceName also prefixes swag's
# own output files, hence <instance-name>_swagger.{json,yaml} as the inputs.
define strip-openapi-prefixes
@jq '.components.schemas |= with_entries(.key |= (gsub("$(3)\\."; "") | gsub("$(3)_"; "") | gsub("pagination\\."; "") | gsub("pagination_"; "") | gsub("util\\."; "") | gsub("util_"; ""))) | walk(if type == "string" then (gsub("$(3)\\."; "") | gsub("$(3)_"; "") | gsub("pagination\\."; "") | gsub("pagination_"; "") | gsub("util\\."; "") | gsub("util_"; "")) else . end)' "$(1)/$(2)_swagger.json" > "$(1)/openapi.json" && rm "$(1)/$(2)_swagger.json"
@sed -i.bak 's/$(3)[._]//g; s/pagination[._]//g; s/util[._]//g' "$(1)/$(2)_swagger.yaml" && rm -f "$(1)/$(2)_swagger.yaml.bak" && mv "$(1)/$(2)_swagger.yaml" "$(1)/openapi.yaml"
endef

# One OpenAPI document per API version. A v2 reader must never have to scroll
# past a v1 endpoint, and the two documents are also what let v2's schema names
# be bare (no V2 prefix) without colliding with v1's inside one components map.
#
# --outputTypes json,yaml deliberately skips swag's docs.go: nothing in this
# binary ever read swag's global registry — the /docs routes serve the embedded
# openapi.{json,yaml} bytes — so generating it only compiled a second copy of
# every document into the binary.
#
# -d is comma-separated and the general-info file must live in the FIRST
# directory: core/api/service.go for v1, core/api/v2/doc.go for v2. The shared
# packages (util, pagination) are parsed into both documents on purpose — each
# document has to stand alone.
openapi: setup-swag
	@echo 'Generating openapi docs...'
	@deps/swag init --v3.1 -q --outputTypes json,yaml --instanceName v1 --exclude core/api/v2 -d core/api -g service.go -o $(OPENAPI_DOCS_DIR)/v1
	@deps/swag init --v3.1 -q --outputTypes json,yaml --instanceName v2 -d core/api/v2,core/api/util,core/api/pagination -g doc.go -o $(OPENAPI_DOCS_DIR)/v2
	@echo 'Removing package prefixes from definitions...'
	$(call strip-openapi-prefixes,$(OPENAPI_DOCS_DIR)/v1,v1,apimodel)
	$(call strip-openapi-prefixes,$(OPENAPI_DOCS_DIR)/v2,v2,v2model)
	# swag v2 hardcodes bearerFormat: JWT and cannot assign different schemas
	# to two request media types. Keep those v2-only corrections deterministic.
	@python3 scripts/fix_openapi_v2.py "$(OPENAPI_DOCS_DIR)/v2"
	@echo 'Formatting openapi docs...'
	@deps/swag fmt -d core/api
