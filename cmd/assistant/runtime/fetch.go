package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/buke/quickjs-go"
)

func installFetch(ctx *quickjs.Context, client *http.Client) (cleanup func(), err error) {
	// Helper: (obj) => Object.entries(obj)
	entriesFn := ctx.Eval(`(obj) => Object.entries(obj)`)
	if entriesFn.IsException() {
		defer entriesFn.Free()
		return nil, ctx.Exception()
	}

	// --- fetch(url, init) -> plain object { ok, status, statusText, url, headers, body } ---
	// body is auto-parsed as JSON if content-type is json, otherwise string
	fetchFn := ctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) < 1 {
			return ctx.ThrowError(errors.New("fetch: url is required"))
		}

		url := args[0].ToString()
		method := "GET"
		var body []byte
		reqHeaders := make(http.Header)

		// Parse init
		if len(args) >= 2 && args[1] != nil && args[1].IsObject() {
			init := args[1]

			if m := init.Get("method"); m != nil {
				if !m.IsUndefined() && !m.IsNull() {
					method = strings.ToUpper(m.ToString())
				}
				m.Free()
			}

			// headers: object {k:v}
			if h := init.Get("headers"); h != nil {
				if h.IsObject() {
					// pairs = Object.entries(h)
					nullThis := ctx.NewNull()
					pairs := entriesFn.Execute(nullThis, h)
					nullThis.Free()
					h.Free()

					if pairs != nil {
						defer pairs.Free()
						if pairs.IsException() {
							return ctx.ThrowError(errors.New("fetch: Object.entries(headers) threw"))
						}
						if pairs.IsArray() {
							n := pairs.Len()
							for i := int64(0); i < n; i++ {
								row := pairs.GetIdx(i)
								if row == nil {
									continue
								}
								if row.IsArray() && row.Len() >= 2 {
									kVal := row.GetIdx(0)
									vVal := row.GetIdx(1)

									k := kVal.ToString()
									v := vVal.ToString()

									kVal.Free()
									vVal.Free()

									reqHeaders.Add(k, v)
								}
								row.Free()
							}
						}
					}
				} else {
					h.Free()
				}
			}

			// body: string only (minimal)
			if b := init.Get("body"); b != nil {
				if !b.IsUndefined() && !b.IsNull() {
					body = []byte(b.ToString())
				}
				b.Free()
			}
		}

		// Perform HTTP request synchronously
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		req, e := http.NewRequest(method, url, bodyReader)
		if e != nil {
			return ctx.ThrowError(errors.New("fetch: " + e.Error()))
		}
		for k, vs := range reqHeaders {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}

		resp, e := client.Do(req)
		if e != nil {
			return ctx.ThrowError(errors.New("fetch: " + e.Error()))
		}
		defer resp.Body.Close()

		respBody, e := io.ReadAll(resp.Body)
		if e != nil {
			return ctx.ThrowError(errors.New("fetch: " + e.Error()))
		}

		// Convert headers to map[string]string (first value only, lowercased keys)
		headers := make(map[string]string, len(resp.Header))
		for k, vs := range resp.Header {
			if len(vs) > 0 {
				headers[strings.ToLower(k)] = vs[0]
			}
		}

		statusText := ""
		parts := strings.SplitN(resp.Status, " ", 2)
		if len(parts) == 2 {
			statusText = parts[1]
		}

		// Auto-parse body based on Content-Type
		var responseBody interface{}
		contentType := strings.ToLower(resp.Header.Get("Content-Type"))
		if strings.Contains(contentType, "application/json") {
			var jsonBody interface{}
			if err := json.Unmarshal(respBody, &jsonBody); err != nil {
				// Parsing failed, fall back to string
				responseBody = string(respBody)
			} else {
				responseBody = jsonBody
			}
		} else {
			responseBody = string(respBody)
		}

		// Build plain response object
		result := map[string]interface{}{
			"ok":         resp.StatusCode >= 200 && resp.StatusCode <= 299,
			"status":     resp.StatusCode,
			"statusText": statusText,
			"url":        resp.Request.URL.String(),
			"headers":    headers,
			"body":       responseBody,
		}

		jsResult, e := ctx.Marshal(result)
		if e != nil {
			return ctx.ThrowError(errors.New("fetch: marshal result: " + e.Error()))
		}
		return jsResult
	})

	// Set raw fetch, then wrap with tracer
	ctx.Globals().Set("__rawFetch", fetchFn)
	wrapCode := ctx.Eval(`globalThis.fetch = globalThis.__wrapEffect("fetch", globalThis.__rawFetch)`)
	if wrapCode.IsException() {
		defer wrapCode.Free()
		return nil, ctx.Exception()
	}
	wrapCode.Free()

	return func() {
		entriesFn.Free()
	}, nil
}
