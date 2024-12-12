package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const (
	baseURL = "http://localhost:31009/v1"
	// testSpaceId  = "bafyreifymx5ucm3fdc7vupfg7wakdo5qelni3jvlmawlnvjcppurn2b3di.2lcu0r85yg10d" // dev (entry space)
	// testSpaceId  = "bafyreiakofsfkgb7psju346cir2hit5hinhywaybi6vhp7hx4jw7hkngje.scoxzd7vu6rz" // HPI
	// testObjectId = "bafyreidhtlbbspxecab6xf4pi5zyxcmvwy6lqzursbjouq5fxovh6y3xwu"              // "Work Faster with Templates"
	// testTypeId   = "bafyreie3djy4mcldt3hgeet6bnjay2iajdyi2fvx556n6wcxii7brlni3i"              // Page (in dev space)
	// chatSpaceId  = "bafyreigryvrmerbtfswwz5kav2uq5dlvx3hl45kxn4nflg7lz46lneqs7m.2nvj2qik6ctdy" // Anytype Wiki space
	chatSpaceId = "bafyreiexhpzaf7uxzheubh7cjeusqukjnxfvvhh4at6bygljwvto2dttnm.2lcu0r85yg10d" // chat space
)

var log = logging.Logger("rest-api")

// ReplacePlaceholders replaces placeholders in the endpoint with actual values from parameters.
func ReplacePlaceholders(endpoint string, parameters map[string]interface{}) string {
	for key, value := range parameters {
		placeholder := fmt.Sprintf("{%s}", key)
		endpoint = strings.ReplaceAll(endpoint, placeholder, fmt.Sprintf("%v", value))
	}

	// Parse the base URL + endpoint
	u, err := url.Parse(baseURL + endpoint)
	if err != nil {
		log.Errorf("Failed to parse URL: %v\n", err)
		return ""
	}

	return u.String()
}

func main() {
	endpoints := []struct {
		method     string
		endpoint   string
		parameters map[string]interface{}
		body       map[string]interface{}
	}{
		// auth
		// {"POST", "/auth/displayCode", nil, nil},
		// {"GET", "/auth/token?challengeId={challengeId}&code={code}", map[string]interface{}{"challengeId": "6738dfc5cda913aad90e8c2a", "code": "2931"}, nil},

		// spaces
		// {"POST", "/spaces", nil, map[string]interface{}{"name": "New Space"}},
		// {"GET", "/spaces?limit={limit}&offset={offset}", map[string]interface{}{"limit": 100, "offset": 0}, nil},
		// {"GET", "/spaces/{space_id}/members?limit={limit}&offset={offset}", map[string]interface{}{"space_id": testSpaceId}, nil},

		// space_objects
		// {"GET", "/spaces/{space_id}/objects?limit={limit}&offset={offset}", map[string]interface{}{"space_id": testSpaceId, "limit": 100, "offset": 0}, nil},
		// {"GET", "/spaces/{space_id}/objects/{object_id}", map[string]interface{}{"space_id": testSpaceId, "object_id": testObjectId}, nil},
		// {"POST", "/spaces/{space_id}/objects", map[string]interface{}{"space_id": testSpaceId}, map[string]interface{}{"name": "New Object from demo", "icon_emoji": "💥", "template_id": "", "object_type_unique_key": "ot-page", "with_chat": false}},
		// {"PUT", "/spaces/{space_id}/objects/{object_id}", map[string]interface{}{"space_id": testSpaceId, "object_id": testObjectId}, map[string]interface{}{"name": "Updated Object"}},

		// types_and_templates
		// {"GET", "/spaces/{space_id}/objectTypes?limit={limit}&offset={offset}", map[string]interface{}{"space_id": testSpaceId, "limit": 100, "offset": 0}, nil},
		// {"GET", "/spaces/{space_id}/objectTypes/{type_id}/templates?limit={limit}&offset={offset}", map[string]interface{}{"space_id": testSpaceId, "type_id": testTypeId}, nil},

		// search
		// {"GET", "/objects?search={search}&object_type={object_type}&limit={limit}&offset={offset}", map[string]interface{}{"search": "writing", "object_type": testTypeId, "limit": 100, "offset": 0}, nil},

		// chat
		{"GET", "/spaces/{space_id}/chat/messages?limit={limit}&offset={offset}", map[string]interface{}{"space_id": chatSpaceId, "limit": 100, "offset": 0}, nil},
		// {"POST", "/spaces/{space_id}/chat/messages", map[string]interface{}{"space_id": chatSpaceId}, map[string]interface{}{"text": "new message from demo"}},
	}

	for _, ep := range endpoints {
		finalURL := ReplacePlaceholders(ep.endpoint, ep.parameters)

		var req *http.Request
		var err error

		if ep.body != nil {
			body, err := json.Marshal(ep.body)
			if err != nil {
				log.Errorf("Failed to marshal body for %s: %v\n", ep.endpoint, err)
				continue
			}
			req, err = http.NewRequest(ep.method, finalURL, bytes.NewBuffer(body))
			if err != nil {
				log.Errorf("Failed to create request for %s: %v\n", ep.endpoint, err)
			}
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, err = http.NewRequest(ep.method, finalURL, nil)
		}

		if err != nil {
			log.Errorf("Failed to create request for %s: %v\n", ep.endpoint, err)
			continue
		}

		// Execute the HTTP request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Errorf("Failed to make request to %s: %v\n", finalURL, err.Error())
			continue
		}
		defer resp.Body.Close()

		// Check the status code
		if resp.StatusCode != http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Errorf("Failes to read response body for request to %s with code %d.", finalURL, resp.StatusCode)
				continue
			}
			log.Errorf("Request to %s returned status code %d: %v\n", finalURL, resp.StatusCode, string(body))
			continue
		}

		// Read the response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("Failed to read response body for %s: %v\n", ep.endpoint, err)
			continue
		}

		// Log the response
		var prettyJSON bytes.Buffer
		err = json.Indent(&prettyJSON, body, "", "  ")
		if err != nil {
			log.Errorf("Failed to pretty print response body for %s: %v\n", ep.endpoint, err)
			log.Infof("%s\n", string(body))
			continue
		}

		log.Infof("Endpoint: %s, Status Code: %d, Body: %s\n", finalURL, resp.StatusCode, prettyJSON.String())
	}
}
