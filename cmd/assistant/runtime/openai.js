// OpenAI completion module
// Usage: import { complete } from "openai"

export function complete(prompt) {
  const apiKey = globalThis.__openaiKey;

  const response = fetch("https://api.openai.com/v1/chat/completions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Authorization": "Bearer " + apiKey
    },
    body: JSON.stringify({
      model: "gpt-4o-mini",
      messages: [
        { role: "user", content: prompt }
      ]
    })
  });

  if (!response.ok) {
    throw new Error("OpenAI API error: " + response.status + " " + response.text());
  }

  const data = response.json();
  return data?.choices?.[0]?.message?.content ?? null;
}
