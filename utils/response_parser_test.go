package utils

import "testing"

func TestExtractFunctionCallsSupportsMultilineJSON(t *testing.T) {
	response := "before\n<function_call>{\n  \"name\": \"lookup\",\n  \"arguments\": {\n    \"query\": \"omnigo\"\n  }\n}</function_call>\nafter"

	calls, err := ExtractFunctionCalls(response)
	if err != nil {
		t.Fatalf("ExtractFunctionCalls error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(calls))
	}
	if calls[0]["name"] != "lookup" {
		t.Fatalf("name = %#v, want lookup", calls[0]["name"])
	}
	args, ok := calls[0]["arguments"].(map[string]interface{})
	if !ok || args["query"] != "omnigo" {
		t.Fatalf("arguments = %#v, want query=omnigo", calls[0]["arguments"])
	}
}

func TestCleanResponseSupportsMultilineFunctionCall(t *testing.T) {
	response := "before\n<function_call>{\n  \"name\": \"lookup\"\n}</function_call>\nafter"

	cleaned, calls, err := CleanResponse(response)
	if err != nil {
		t.Fatalf("CleanResponse error = %v", err)
	}
	if cleaned != "before\n\nafter" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "before\n\nafter")
	}
	if len(calls) != 1 || calls[0] != "{\n  \"name\": \"lookup\"\n}" {
		t.Fatalf("calls = %#v", calls)
	}
}
