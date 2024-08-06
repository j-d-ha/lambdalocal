package main

import "encoding/json"

func parseInnerJSON(data map[string]any) map[string]any {
	for k, v := range data {
		if vv, ok := v.(string); ok {
			newJSON := any(nil)
			if err := json.Unmarshal([]byte(vv), &newJSON); err != nil {
				continue
			}
			data[k] = newJSON
		}
	}

	return data
}
