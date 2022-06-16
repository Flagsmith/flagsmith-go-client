package fixtures

import (
	"io"
	"net/http"
)

const BaseURL = "http://localhost:8000/api/v1/"
const EnvironmentAPIKey = "test_key"
const Feature1Value = "some_value"
const Feature1Name = "feature_1"
const Feature1ID = 1

const EnvironmentJson = `
{
	"api_key": "B62qaMZNwfiqT76p38ggrQ",
	"project": {
		"name": "Test project",
		"organisation": {
			"feature_analytics": false,
			"name": "Test Org",
			"id": 1,
			"persist_trait_data": true,
			"stop_serving_flags": false
		},
		"id": 1,
		"hide_disabled_flags": false,
		"segments": [{
			"id": 1,
			"name": "Test Segment",
			"feature_states": [],
			"rules": [{
				"type": "ALL",
				"conditions": [],
				"rules": [{
					"type": "ALL",
					"rules": [],
					"conditions": [{
						"operator": "EQUAL",
						"property_": "foo",
						"value": "bar"
					}]
				}]
			}]
		}]
	},
	"segment_overrides": [],
	"id": 1,
	"feature_states": [{
		"multivariate_feature_state_values": [],
		"feature_state_value": "some_value",
		"id": 1,
		"featurestate_uuid": "40eb539d-3713-4720-bbd4-829dbef10d51",
		"feature": {
			"name": "feature_1",
			"type": "STANDARD",
			"id": 1
		},
		"segment_id": null,
		"enabled": true
	}]
}
`

const FlagsJson = `
[{
	"id": 1,
	"feature": {
		"id": 1,
		"name": "feature_1",
		"created_date": "2019-08-27T14:53:45.698555Z",
		"initial_value": null,
		"description": null,
		"default_enabled": false,
		"type": "STANDARD",
		"project": 1
	},
	"feature_state_value": "some_value",
	"enabled": true,
	"environment": 1,
	"identity": null,
	"feature_segment": null
}]
`
const IdentityResponseJson = `
{
	"flags": [{
		"id": 1,
		"feature": {
			"id": 1,
			"name": "feature_1",
			"created_date": "2019-08-27T14:53:45.698555Z",
			"initial_value": null,
			"description": null,
			"default_enabled": false,
			"type": "STANDARD",
			"project": 1
		},
		"feature_state_value": "some_value",
		"enabled": true,
		"environment": 1,
		"identity": null,
		"feature_segment": null
	}],
	"traits": [{
		"trait_key": "foo",
		"trait_value": "bar"
	}]
}

`

func EnvironmentDocumentHandler(rw http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/api/v1/environment-document/" {
		panic("Wrong path")
	}
	if req.Header.Get("X-Environment-Key") != EnvironmentAPIKey {
		panic("Wrong API key")
	}

	rw.Header().Set("Content-Type", "application/json")

	rw.WriteHeader(http.StatusOK)
	_, err := io.WriteString(rw, EnvironmentJson)
	if err != nil {
		panic(err)
	}

}
