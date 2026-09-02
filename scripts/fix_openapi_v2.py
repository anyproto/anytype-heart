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
    for path_item in doc["paths"].values():
        for operation in path_item.values():
            if isinstance(operation, dict) and "operationId" in operation:
                yield operation


def apply_response_policies(doc: dict) -> None:
    doc["components"]["responses"] = response_components()
    seen = set()
    for operation in operations(doc):
        operation_id = operation["operationId"]
        seen.add(operation_id)
        responses = operation.setdefault("responses", {})
        responses.setdefault("400", response_ref("BadRequest"))
        responses["401"] = response_ref("Unauthorized")
        responses["403"] = response_ref("Forbidden")
        if operation_id in IDEMPOTENT_OPERATIONS:
            responses["409"] = response_ref("Conflict")
        if operation_id in REQUEST_BODY_LIMITED_OPERATIONS:
            responses["413"] = response_ref("RequestTooLarge")
        if operation_id in WRITE_LIMITED_OPERATIONS:
            responses["429"] = response_ref("RateLimited")
        if operation_id in DRY_RUN_CREATES:
            if "201" not in responses:
                raise ValueError(f"dry-run create {operation_id} has no 201 response to mirror")
            responses["200"] = deepcopy(responses["201"])
            responses["200"]["description"] = "Dry run; validation result without committing"

    governed = DRY_RUN_CREATES | IDEMPOTENT_OPERATIONS | REQUEST_BODY_LIMITED_OPERATIONS
    missing = governed - seen
    if missing:
        raise ValueError(f"response policy names missing operations: {sorted(missing)}")


def upload_request_body():
    return {
        "content": {
            "application/json": {
                "schema": {
                    "additionalProperties": False,
                    "properties": {
                        "name": {"type": "string"},
                        "url": {"type": "string"},
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
        if operation_id in IDEMPOTENT_OPERATIONS:
            blocks["409"] = yaml_response_ref("409", "Conflict")
        if operation_id in REQUEST_BODY_LIMITED_OPERATIONS:
            blocks["413"] = yaml_response_ref("413", "RequestTooLarge")
        if operation_id in WRITE_LIMITED_OPERATIONS:
            blocks["429"] = yaml_response_ref("429", "RateLimited")
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
    path.write_text("".join(lines))


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: fix_openapi_v2.py <v2-openapi-directory>")
    directory = pathlib.Path(sys.argv[1])
    fix_json(directory / "openapi.json")
    fix_yaml(directory / "openapi.yaml")


if __name__ == "__main__":
    main()
