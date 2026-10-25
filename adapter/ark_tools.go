package adapter

import (
	"encoding/json"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model/responses"
)

func arkResponseToolCalls(response *responses.ResponseObject) []dto.ToolCall {
	var calls []dto.ToolCall
	for _, item := range response.GetOutput() {
		if call := item.GetFunctionToolCall(); call != nil {
			calls = append(calls, dto.ToolCall{ID: call.GetCallId(), Type: "function", Function: dto.ToolCallFunction{Name: call.GetName(), Arguments: json.RawMessage(call.GetArguments())}})
		}
	}
	return calls
}

type arkToolStreamState struct {
	id, name, arguments string
}

func (s *arkStream) nextPending() *dto.StreamToken {
	if len(s.pending) == 0 {
		return nil
	}
	token := s.pending[0]
	s.pending = s.pending[1:]
	return token
}

func (s *arkStream) toolEvent(event *responses.Event) *dto.StreamToken {
	if item := event.GetItem(); item != nil && item.GetItem().GetFunctionToolCall() != nil {
		return s.toolDelta(int(item.GetOutputIndex()), item.GetItem().GetFunctionToolCall(), "", true)
	}
	if delta := event.GetFunctionCallArguments(); delta != nil {
		return s.toolDelta(int(delta.GetOutputIndex()), nil, firstNonEmptyString(delta.GetDelta(), delta.GetArguments()), false)
	}
	if done := event.GetFunctionCallArgumentsDone(); done != nil {
		return s.toolDelta(int(done.GetOutputIndex()), nil, firstNonEmptyString(done.GetArguments(), done.GetDelta()), true)
	}
	if item := event.GetItemDone(); item != nil && item.GetItem().GetFunctionToolCall() != nil {
		return s.toolDelta(int(item.GetOutputIndex()), item.GetItem().GetFunctionToolCall(), "", true)
	}
	return nil
}

func (s *arkStream) toolDelta(index int, call *responses.ItemFunctionToolCall, arguments string, complete bool) *dto.StreamToken {
	if s.tools == nil {
		s.tools = make(map[int]*arkToolStreamState)
	}
	state := s.tools[index]
	if state == nil {
		state = &arkToolStreamState{}
		s.tools[index] = state
	}
	delta := dto.ToolCallDelta{Index: index, Type: "function"}
	if call != nil {
		if state.id == "" {
			state.id, delta.ID = call.GetCallId(), call.GetCallId()
		}
		if state.name == "" {
			state.name, delta.Function.Name = call.GetName(), call.GetName()
		}
		arguments = call.GetArguments()
	}
	if complete {
		if strings.HasPrefix(arguments, state.arguments) {
			arguments = strings.TrimPrefix(arguments, state.arguments)
		} else {
			arguments = ""
		}
	}
	state.arguments += arguments
	delta.Function.Arguments = arguments
	if delta.ID == "" && delta.Function.Name == "" && arguments == "" {
		return nil
	}
	return &dto.StreamToken{Type: "function_call", ToolCalls: []dto.ToolCallDelta{delta}}
}
