package adapter

import (
	"encoding/json"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
)

// requestTemperature preserves an explicit zero in Options while leaving an
// unset MediaRequest temperature optional. Per-request options take precedence.
func requestTemperature(request *dto.MediaRequest) (float64, bool) {
	switch value := request.Options["temperature"].(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case json.Number:
		if number, err := value.Float64(); err == nil {
			return number, true
		}
	case int, int8, int16, int32, int64:
		if number, ok := utils.GetIntExtra(request.Options, "temperature"); ok {
			return float64(number), true
		}
	}
	return request.Temperature, request.Temperature != 0
}
