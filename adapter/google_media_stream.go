package adapter

import (
	"context"
	"fmt"
	"io"
	"iter"
	"time"

	"github.com/YspCoder/omnigo/dto"
)

func (a *GoogleAdaptor) StreamMedia(ctx context.Context, cfg *ProviderConfig, request *dto.MediaRequest) (dto.TokenStream, error) {
	if request == nil {
		return nil, fmt.Errorf("google media request is required")
	}
	if googleUsesInteractions(cfg, request) {
		return a.interactionStream(ctx, cfg, request)
	}
	c, err := a.getClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	streamCtx, cancel := context.WithCancel(ctx)
	if request.Type == dto.MediaTypeImage && isGoogleGenerateContentImageModel(normalizeGoogleMediaModel(request.Model)) {
		contents, err := a.toMediaPromptContents(streamCtx, c, request)
		if err != nil {
			cancel()
			return nil, err
		}
		config, err := a.toImgContentCfg(request)
		if err != nil {
			cancel()
			return nil, err
		}
		next, stop := iter.Pull2(c.Models.GenerateContentStream(streamCtx, normalizeGoogleMediaModel(request.Model), contents, config))
		return newCancellableStream(&googleStream{next: next, stop: stop}, cancel), nil
	}
	response, err := a.Media(streamCtx, cfg, request)
	if err != nil {
		cancel()
		return nil, err
	}
	if request.Type == dto.MediaTypeVideo && !dto.IsSucceeded(response.Status) && !dto.IsFailed(response.Status) {
		return newCancellableStream(&googleVideoStream{ctx: streamCtx, adaptor: a, cfg: cfg, id: response.TaskID}, cancel), nil
	}
	cancel()
	stream := &googleMediaResultStream{}
	if dto.IsFailed(response.Status) {
		stream.err = fmt.Errorf("google media failed (%s): %s", response.ErrorCode, response.ErrorMessage)
	}
	for i, data := range response.Data {
		stream.pending = append(stream.pending, googleImageToken(i, data))
	}
	if request.Type == dto.MediaTypeVideo && !dto.IsFailed(response.Status) {
		stream.pending = append(stream.pending, googleVideoToken(response.URL, response.Video.B64JSON, response.Video.MIMEType))
	}
	if usage := usagePointer(response.Usage); usage != nil {
		stream.pending = append(stream.pending, &dto.StreamToken{Type: "usage", Usage: usage})
	}
	return stream, nil
}

func googleImageToken(index int, data dto.ImageData) *dto.StreamToken {
	token := &dto.StreamToken{Type: "image", Index: index, URL: data.URL, Text: data.B64JSON}
	if data.MIMEType != "" {
		token.Metadata = map[string]interface{}{"mime_type": data.MIMEType}
	}
	return token
}

type googleMediaResultStream struct {
	pending []*dto.StreamToken
	err     error
	mediaStreamState
}

func (s *googleMediaResultStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	if err := s.streamError(ctx); err != nil {
		return nil, err
	}
	if len(s.pending) > 0 {
		token := s.pending[0]
		s.pending = s.pending[1:]
		return token, nil
	}
	s.done.Store(true)
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

type googleVideoStream struct {
	ctx     context.Context
	adaptor *GoogleAdaptor
	cfg     *ProviderConfig
	id      string
	last    string
	mediaStreamState
}

func (s *googleVideoStream) Next(ctx context.Context) (*dto.StreamToken, error) {
	if err := s.streamError(ctx); err != nil {
		return nil, err
	}
	for {
		status, err := s.adaptor.TaskStatus(s.ctx, s.cfg, s.id)
		if err != nil {
			return nil, err
		}
		state := status.Output.TaskStatus
		if dto.IsSucceeded(state) {
			s.done.Store(true)
			token := googleVideoToken(status.Output.VideoURL, status.Output.VideoB64JSON, status.Output.VideoMIMEType)
			token.Usage = streamUsage(status.Usage)
			return token, nil
		}
		if dto.IsFailed(state) {
			return s.failWithUsage(&dto.StreamToken{Type: "error", Text: status.Output.Message, Usage: streamUsage(status.Usage)}, fmt.Errorf("google video failed (%s): %s", status.Output.Code, status.Output.Message))
		}
		if state != s.last {
			s.last = state
			return &dto.StreamToken{Type: "progress", Text: state, Usage: streamUsage(status.Usage)}, nil
		}
		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func googleVideoToken(url, encoded, mime string) *dto.StreamToken {
	token := &dto.StreamToken{Type: "url", URL: url}
	if encoded != "" {
		token.Type, token.Text = "video", encoded
		token.Metadata = map[string]interface{}{"mime_type": mime}
	}
	return token
}
