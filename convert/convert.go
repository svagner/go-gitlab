package convert

import (
	"bytes"
	"encoding/json"
)

func ConvertToJSON_HTML(data interface{}) string {
	var result bytes.Buffer
	tmp, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	json.HTMLEscape(&result, tmp)
	return result.String()
}
