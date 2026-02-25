// Claude completion module
// Usage: import { complete } from "claude"

export function complete(prompt) {
  const apiKey = globalThis.__claudeKey;

  const response = fetch("https://api.anthropic.com/v1/messages", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "x-api-key": apiKey,
      "anthropic-version": "2023-06-01"
    },
    body: JSON.stringify({
      model: "claude-sonnet-4-20250514",
      max_tokens: 4096,
      messages: [
        { role: "user", content: prompt }
      ]
    })
  });

  if (!response.ok) {
    throw new Error("Claude API error: " + response.status + " " + JSON.stringify(response.body));
  }

  // Claude returns content as array of content blocks
  var content = response.body?.content;
  if (!content || content.length === 0) {
    return null;
  }

  // Find text block
  for (var i = 0; i < content.length; i++) {
    if (content[i].type === "text") {
      return content[i].text;
    }
  }

  return null;
}
