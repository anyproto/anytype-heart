#!/usr/bin/env python3
"""Apply v2 OpenAPI corrections that swag v2 cannot express."""

import json
import pathlib
import re
import sys
from copy import deepcopy


# These policies live at the router/middleware layer, outside any one handler
# comment, so swag cannot infer them. Keep the route sets beside the
# post-processor that makes those cross-cutting responses part of the public
# contract instead of duplicating declarations across 45 handlers.
DRY_RUN_CREATES = {
    "add_chat_message",
    "create_chat",
    "create_collection",
    "create_object",
    "create_property",
    "create_query",
    "create_space",
    "create_template",
    "create_type",
    "upload_file",
}

IDEMPOTENT_OPERATIONS = {
    "validate",
    "create_space",
    "update_space",
    "create_object",
    "create_template",
    "create_type",
    "update_type",
    "delete_type",
    "create_property",
    "update_property",
    "delete_property",
    "create_query",
    "create_collection",
    "upload_file",
    "patch_object",
    "delete_object",
    "create_chat",
    "add_chat_message",
    "edit_chat_message",
    "delete_chat_message",
    "toggle_chat_reaction",
    "read_chat",
}

WRITE_LIMITED_OPERATIONS = IDEMPOTENT_OPERATIONS - {"validate"}

# Operations whose request-size response is assertion-linked in the v2
# contract. Five already carry handler annotations; keeping the whole set here
# makes regeneration insensitive to whether a local annotation is reordered.
REQUEST_BODY_LIMITED_OPERATIONS = {
    "add_chat_message",
    "create_chat",
    "create_collection",
    "create_property",
    "create_query",
    "create_space",
    "edit_chat_message",
    "read_chat",
    "toggle_chat_reaction",
    "update_property",
    "update_space",
    "update_type",
    "upload_file",
}


def response_ref(name: str) -> dict:
    return {"$ref": f"#/components/responses/{name}"}


def response_components() -> dict:
    v2_error = {"$ref": "#/components/schemas/Error"}
    return {
        "BadRequest": {
            "description": "Invalid path, query or request input",
            "content": {"application/json": {"schema": v2_error}},
        },
        "Unauthorized": {
            "description": "Missing, unknown, revoked or expired key; shared authentication envelope",
            "content": {"application/json": {"schema": {"$ref": "#/components/schemas/UnauthorizedError"}}},
        },
        "Forbidden": {
            "description": "The shared key-scope gate uses the legacy envelope; an operation-specific refusal may use the v2 error envelope",
            "content": {"application/json": {"schema": {"anyOf": [
                {"$ref": "#/components/schemas/ForbiddenError"},
                v2_error,
            ]}}},
        },
        "Conflict": {
            "description": "Idempotency-Key conflict, or an operation-specific concurrency conflict",
            "content": {"application/json": {"schema": v2_error}},
        },
        "NotFound": {
            "description": "The space does not exist, or is not available (deleted, left, or still joining)",
            "content": {"application/json": {"schema": v2_error}},
        },
        "RequestTooLarge": {
            "description": "Request body exceeds this operation's documented cap",
            "content": {"application/json": {"schema": v2_error}},
        },
        "RateLimited": {
            "description": "Shared write rate limit; legacy error envelope",
            "content": {"application/json": {"schema": {
                "type": "object",
                "properties": {
                    "object": {"type": "string", "example": "error"},
                    "status": {"type": "integer", "example": 429},
                    "code": {"type": "string", "example": "rate_limit_exceeded"},
                    "message": {"type": "string", "example": "Rate limit exceeded"},
                },
            }}},
        },
    }


def operations(doc: dict):
    for path, path_item in doc["paths"].items():
        for operation in path_item.values():
            if isinstance(operation, dict) and "operationId" in operation:
                yield path, operation


def apply_response_policies(doc: dict) -> None:
    doc["components"]["responses"] = response_components()
    seen = set()
    for path, operation in operations(doc):
        operation_id = operation["operationId"]
        seen.add(operation_id)
        responses = operation.setdefault("responses", {})
        responses.setdefault("400", response_ref("BadRequest"))
        responses["401"] = response_ref("Unauthorized")
        responses["403"] = response_ref("Forbidden")
        # Every space-scoped operation resolves the space before any success
        # path — usually ensureSpace/ensureSpaceWrite as the opening
        # statement, sometimes a lookup that answers the same 404 (get_space,
        # update_space) — so a well-shaped id for a space that does not exist
        # is a 404 on all of them. Deriving that from the path rather than a
        # hand list is what keeps a route added later correct by
        # construction: the earlier sweep declared the router-level policies
        # and left 404 to per-handler annotations, which is why it reached
        # only 28 of 37.
        if "{space_id}" in path:
            responses.setdefault("404", response_ref("NotFound"))
        if operation_id in IDEMPOTENT_OPERATIONS:
            responses["409"] = response_ref("Conflict")
        if operation_id in REQUEST_BODY_LIMITED_OPERATIONS:
            responses["413"] = response_ref("RequestTooLarge")
        if operation_id in WRITE_LIMITED_OPERATIONS:
            responses["429"] = response_ref("RateLimited")
        if operation_id in RESOURCE_LIMITED_OPERATIONS:
            responses["429"] = {
                "description": "Too many streams held at once; close one, retrying cannot succeed",
                "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}},
            }
        if operation_id in DRY_RUN_CREATES:
            if "201" not in responses:
                raise ValueError(f"dry-run create {operation_id} has no 201 response to mirror")
            responses["200"] = deepcopy(responses["201"])
            responses["200"]["description"] = "Dry run; validation result without committing"

    governed = DRY_RUN_CREATES | IDEMPOTENT_OPERATIONS | REQUEST_BODY_LIMITED_OPERATIONS
    missing = governed - seen
    if missing:
        raise ValueError(f"response policy names missing operations: {sorted(missing)}")


# swag emits no schema-level `required` for response models, so every member
# of the error envelope reads as optional — including the three that are
# always present. A generated client then types the whole envelope optional
# and a consumer branches on fields that cannot be absent. C6 promises ONE
# error shape; the machine-readable document has to say the same thing.
SCHEMA_REQUIRED = {
    "Error": ["status", "code", "message", "issues"],
    "Issue": ["message"],
    # the shared v1 envelope the pre-handler refusals answer in (§8.9) sets
    # all four members too, so the same argument applies to it
    "UnauthorizedError": ["object", "status", "code", "message"],
    "ForbiddenError": ["object", "status", "code", "message"],
}


# swag emits application/json alongside the declared @Produce for a plain
# string success, so the stream's 200 advertised a JSON body it never sends.
# The route answers text/event-stream and nothing else.
STREAM_OPERATION = "stream_chat_messages"

# Operations that answer 429 for a RESOURCE they cap rather than a rate they
# throttle. Until the chat stream there was no such thing, so "declares 429"
# and "goes through the shared write limiter" were the same set — and the
# conformance test asserted it. A concurrency refusal is v2's own, so it
# carries the C6 envelope, not the legacy shape the shared limiter uses.
RESOURCE_LIMITED_OPERATIONS = {STREAM_OPERATION}


def apply_stream_content_type(doc: dict) -> None:
    for _, operation in operations(doc):
        if operation["operationId"] != STREAM_OPERATION:
            continue
        ok = operation.get("responses", {}).get("200")
        if ok is None or "content" not in ok:
            raise ValueError(f"{STREAM_OPERATION} has no 200 content to narrow")
        stream = ok["content"].get("text/event-stream")
        if stream is None:
            raise ValueError(f"{STREAM_OPERATION} does not declare text/event-stream")
        ok["content"] = {"text/event-stream": stream}
        return
    raise ValueError(f"response policy names a missing operation: {STREAM_OPERATION}")


def apply_schema_required(doc: dict) -> None:
    for name, required in SCHEMA_REQUIRED.items():
        schema = doc["components"]["schemas"][name]
        # a renamed json tag would otherwise leave a required name with no
        # matching property — the same silent drift the three operation-name
        # sets above are checked against
        unknown = [field for field in required if field not in schema.get("properties", {})]
        if unknown:
            raise ValueError(f"{name}.required names absent properties: {unknown}")
        schema["required"] = list(required)


def apply_yaml_stream_content_type(lines: list[str]) -> list[str]:
    """The YAML twin of apply_stream_content_type."""
    for i, line in enumerate(lines):
        if line.strip() != f"operationId: {STREAM_OPERATION}":
            continue
        ok = next(j for j in range(i, len(lines)) if lines[j] == '        "200":\n')
        content = next(j for j in range(ok, len(lines)) if lines[j] == "          content:\n")
        json_at = next(j for j in range(content, len(lines)) if lines[j] == "            application/json:\n")
        stream_at = next(j for j in range(content, len(lines)) if lines[j] == "            text/event-stream:\n")
        if json_at > stream_at:
            raise ValueError("unexpected media-type order under the stream 200")
        del lines[json_at:stream_at]
        return lines
    raise ValueError(f"response policy names a missing YAML operation: {STREAM_OPERATION}")


def apply_yaml_schema_required(lines: list[str]) -> list[str]:
    # bound the search to components.schemas: response components are indented
    # the same, so an unbounded index would let a reusable response named
    # Error capture the splice
    schemas = lines.index("  schemas:\n", lines.index("components:\n") + 1)
    for name, required in SCHEMA_REQUIRED.items():
        start = lines.index(f"    {name}:\n", schemas)
        end = next((i for i in range(start + 1, len(lines))
                    if lines[i].strip() and not lines[i].startswith("     ")), len(lines))
        if "      required:\n" in lines[start:end]:
            continue  # already applied; every other step here re-runs cleanly
        # keys are emitted alphabetically, so `required` sits between
        # `properties` and the schema's own closing `type: object`
        close = next(i for i in range(start + 1, end) if lines[i] == "      type: object\n")
        lines[close:close] = ["      required:\n"] + [f"      - {field}\n" for field in required]
    return lines


def upload_request_body():
    return {
        "content": {
            "application/json": {
                "schema": {
                    "additionalProperties": False,
                    "properties": {
                        # the service enforces both bounds (M6); a published
                        # schema that omits them describes a laxer contract
                        # than the one the route actually has
                        "name": {
                            "type": "string",
                            "maxLength": 4096,
                            "description": "names the stored object; omit to keep the name the source gives",
                        },
                        "url": {"type": "string", "maxLength": 4096},
                    },
                    "required": ["url"],
                    "type": "object",
                }
            },
            "multipart/form-data": {
                "schema": {
                    "additionalProperties": False,
                    "properties": {
                        "file": {"format": "binary", "type": "string"},
                    },
                    "required": ["file"],
                    "type": "object",
                }
            },
        },
        "required": True,
    }


UPLOAD_YAML = """      requestBody:
        content:
          application/json:
            schema:
              additionalProperties: false
              properties:
                name:
                  type: string
                url:
                  type: string
              required:
              - url
              type: object
          multipart/form-data:
            schema:
              additionalProperties: false
              properties:
                file:
                  format: binary
                  type: string
              required:
              - file
              type: object
        required: true
"""


def fix_json(path: pathlib.Path) -> None:
    doc = json.loads(path.read_text())
    doc["components"]["securitySchemes"]["bearerauth"].pop("bearerFormat", None)
    doc["paths"]["/v2/spaces/{space_id}/files"]["post"]["requestBody"] = upload_request_body()
    apply_response_policies(doc)
    apply_stream_content_type(doc)
    apply_schema_required(doc)
    path.write_text(json.dumps(doc, ensure_ascii=False, indent=2) + "\n")


RESPONSES_YAML = """  responses:
    BadRequest:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
      description: Invalid path, query or request input
    Unauthorized:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/UnauthorizedError'
      description: Missing, unknown, revoked or expired key; shared authentication envelope
    Forbidden:
      content:
        application/json:
          schema:
            anyOf:
            - $ref: '#/components/schemas/ForbiddenError'
            - $ref: '#/components/schemas/Error'
      description: The shared key-scope gate uses the legacy envelope; an operation-specific refusal may use the v2 error envelope
    Conflict:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
      description: Idempotency-Key conflict, or an operation-specific concurrency conflict
    NotFound:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
      description: The space does not exist, or is not available (deleted, left, or still joining)
    RequestTooLarge:
      content:
        application/json:
          schema:
            $ref: '#/components/schemas/Error'
      description: Request body exceeds this operation's documented cap
    RateLimited:
      content:
        application/json:
          schema:
            properties:
              code:
                example: rate_limit_exceeded
                type: string
              message:
                example: Rate limit exceeded
                type: string
              object:
                example: error
                type: string
              status:
                example: 429
                type: integer
            type: object
      description: Shared write rate limit; legacy error envelope
"""


def yaml_response_ref(status: str, name: str) -> list[str]:
    return [f'        "{status}":\n', f"          $ref: '#/components/responses/{name}'\n"]


def yaml_response_blocks(lines: list[str], start: int, end: int) -> dict[str, list[str]]:
    headers = []
    for i in range(start + 1, end):
        match = re.match(r'^        "([0-9]{3})":\s*$', lines[i].rstrip("\n"))
        if match:
            headers.append((i, match.group(1)))
    blocks = {}
    for pos, (index, status) in enumerate(headers):
        block_end = headers[pos + 1][0] if pos + 1 < len(headers) else end
        blocks[status] = lines[index:block_end]
    return blocks


def apply_yaml_response_policies(lines: list[str]) -> list[str]:
    operation_ids = []
    for line in lines:
        match = re.match(r"^      operationId: (\S+)\s*$", line.rstrip("\n"))
        if match:
            operation_ids.append(match.group(1))

    governed = DRY_RUN_CREATES | IDEMPOTENT_OPERATIONS | REQUEST_BODY_LIMITED_OPERATIONS
    missing = governed - set(operation_ids)
    if missing:
        raise ValueError(f"response policy names missing YAML operations: {sorted(missing)}")

    def path_of(operation_line: int) -> str:
        for i in range(operation_line, -1, -1):
            match = re.match(r"^  (/\S*):\s*$", lines[i].rstrip("\n"))
            if match:
                return match.group(1)
        raise ValueError("operation outside any path item")

    # Work bottom-up so replacing one response section cannot invalidate the
    # indexes of operations that remain to be processed.
    for operation_id in reversed(operation_ids):
        operation_line = next(i for i, line in enumerate(lines)
                              if line.strip() == f"operationId: {operation_id}")
        response_start = next(i for i in range(operation_line + 1, len(lines))
                              if lines[i] == "      responses:\n")
        response_end = next(i for i in range(response_start + 1, len(lines))
                            if lines[i].strip() and len(lines[i]) - len(lines[i].lstrip()) == 6)
        blocks = yaml_response_blocks(lines, response_start, response_end)
        blocks.setdefault("400", yaml_response_ref("400", "BadRequest"))
        blocks["401"] = yaml_response_ref("401", "Unauthorized")
        blocks["403"] = yaml_response_ref("403", "Forbidden")
        if "{space_id}" in path_of(operation_line):
            blocks.setdefault("404", yaml_response_ref("404", "NotFound"))
        if operation_id in IDEMPOTENT_OPERATIONS:
            blocks["409"] = yaml_response_ref("409", "Conflict")
        if operation_id in REQUEST_BODY_LIMITED_OPERATIONS:
            blocks["413"] = yaml_response_ref("413", "RequestTooLarge")
        if operation_id in WRITE_LIMITED_OPERATIONS:
            blocks["429"] = yaml_response_ref("429", "RateLimited")
        if operation_id in RESOURCE_LIMITED_OPERATIONS:
            blocks["429"] = [
                '        "429":\n',
                "          content:\n",
                "            application/json:\n",
                "              schema:\n",
                "                $ref: '#/components/schemas/Error'\n",
                "          description: Too many streams held at once; close one, retrying cannot succeed\n",
            ]
        if operation_id in DRY_RUN_CREATES:
            if "201" not in blocks:
                raise ValueError(f"dry-run create {operation_id} has no YAML 201 response to mirror")
            dry = list(blocks["201"])
            dry[0] = '        "200":\n'
            dry = ["          description: Dry run; validation result without committing\n"
                   if line.startswith("          description:") else line for line in dry]
            blocks["200"] = dry
        replacement = ["      responses:\n"]
        for status in sorted(blocks, key=int):
            replacement.extend(blocks[status])
        lines[response_start:response_end] = replacement
    return lines


def fix_yaml(path: pathlib.Path) -> None:
    lines = [line for line in path.read_text().splitlines(keepends=True)
             if line.strip() != "bearerFormat: JWT"]
    start = lines.index("  /v2/spaces/{space_id}/files:\n")
    end = next(i for i in range(start + 1, len(lines))
               if lines[i].startswith("  /v2/") and not lines[i].startswith("  /v2/spaces/{space_id}/files:"))
    request = next(i for i in range(start, end) if lines[i] == "      requestBody:\n")
    responses = next(i for i in range(request, end) if lines[i] == "      responses:\n")
    lines[request:responses] = UPLOAD_YAML.splitlines(keepends=True)
    components = lines.index("components:\n")
    schemas = lines.index("  schemas:\n", components + 1)
    if "  responses:\n" in lines[components + 1:schemas]:
        existing = lines.index("  responses:\n", components + 1, schemas)
        lines[existing:schemas] = RESPONSES_YAML.splitlines(keepends=True)
    else:
        lines[schemas:schemas] = RESPONSES_YAML.splitlines(keepends=True)
    lines = apply_yaml_response_policies(lines)
    lines = apply_yaml_stream_content_type(lines)
    lines = apply_yaml_schema_required(lines)
    path.write_text("".join(lines))


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: fix_openapi_v2.py <v2-openapi-directory>")
    directory = pathlib.Path(sys.argv[1])
    fix_json(directory / "openapi.json")
    fix_yaml(directory / "openapi.yaml")


if __name__ == "__main__":
    main()
