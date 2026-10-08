"""The tool-name vocabulary: one capability, its tool name per surface.

A capability is the API operation it reaches, spelled as the operationId
(`patch_object`). Two surfaces serve them:

- `bridge`: the external anytype-mcp bridge, `API-patch-object`, whose
  calls nest the request body under `body`;
- `full`: heart's /mcp/full, where the tool name IS the operationId and the
  body members are flat beside the path and query arguments.

A host may prefix either with its own server namespace
(`mcp__anytype__API_patch_object`, `mcp__anytype__patch_object`).

Every runner file reads tool names and body arguments through this module;
test_tool_names.py fails if a bridge-shaped literal appears anywhere else.
"""

SURFACES = ("bridge", "full")
DEFAULT_SURFACE = "bridge"

_HOST_PREFIX = "mcp__anytype__"
_BRIDGE_PREFIXES = ("API-", "API_")


def is_bridge_name(name):
    """Whether a tool name is spelled the bridge's way (host prefix allowed)."""
    return (name or "").removeprefix(_HOST_PREFIX).startswith(_BRIDGE_PREFIXES)


def capability(name):
    """Any spelling of a tool name, as its capability (`patch_object`)."""
    bare = (name or "").removeprefix(_HOST_PREFIX)
    for prefix in _BRIDGE_PREFIXES:
        if bare.startswith(prefix):
            bare = bare[len(prefix):]
            break
    return bare.replace("-", "_").lower()


def body(name, arguments):
    """The request-body members of a call to `name`: the bridge nests them
    under `body`; full passes them flat, so the arguments themselves are the
    body (path and query arguments sit beside them and are ignored by every
    reader that looks for a body member). The returned dict is the one a
    caller may mutate to change the forwarded body."""
    if not isinstance(arguments, dict):
        return {}
    nested = arguments.get("body")
    if is_bridge_name(name) or isinstance(nested, dict):
        # no /mcp/full tool takes a member named body (pinned by
        # test_tool_names against the golden tools list), so a nested body
        # dict identifies a bridge call even when only the capability is known
        return nested if isinstance(nested, dict) else {}
    return arguments


def caller_retry_key(arguments):
    """The retry key the caller chose for a call, on either surface: the
    bridge's `request_key` argument or full's `idempotency_key` (exposed on
    toggle_chat_reaction only; everywhere else the server mints it)."""
    if not isinstance(arguments, dict):
        return None
    return arguments.get("idempotency_key") or arguments.get("request_key")


class Vocabulary:
    """Tool names and argument shapes of one surface, for callers that
    issue calls."""

    def __init__(self, surface=DEFAULT_SURFACE):
        if surface not in SURFACES:
            raise ValueError(f"unknown surface {surface!r}; surfaces: {', '.join(SURFACES)}")
        self.surface = surface

    def tool(self, cap):
        """The tool name serving capability `cap` on this surface."""
        if self.surface == "bridge":
            return _BRIDGE_PREFIXES[0] + cap.replace("_", "-")
        return cap

    def arguments(self, params, body_members=None):
        """Call arguments: path/query params plus body members, nested or
        flat as the surface takes them."""
        out = dict(params)
        if body_members is not None:
            if self.surface == "bridge":
                out["body"] = body_members
            else:
                out.update(body_members)
        return out

    def is_capability(self, name, *caps):
        return capability(name) in caps


def vocabulary_for(manifest):
    """The vocabulary of the surface a run recorded (bridge for old runs)."""
    return Vocabulary((manifest or {}).get("surface", DEFAULT_SURFACE))
