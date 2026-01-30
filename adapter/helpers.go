// Package adapter provides shared helpers.
package adapter

func copyOptions(options map[string]interface{}) map[string]interface{} {
	if options == nil {
		return map[string]interface{}{}
	}
	copied := make(map[string]interface{}, len(options))
	for key, value := range options {
		copied[key] = value
	}
	return copied
}
