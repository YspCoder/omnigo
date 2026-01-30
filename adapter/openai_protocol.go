package adapter

// OpenAIProtocolAdaptor indicates the adaptor uses OpenAI-compatible chat protocol.
type OpenAIProtocolAdaptor interface {
	IsOpenAIProtocol() bool
}

// IsOpenAIProtocol marks OpenAIAdaptor as OpenAI-compatible.
func (a *OpenAIAdaptor) IsOpenAIProtocol() bool {
	return true
}
