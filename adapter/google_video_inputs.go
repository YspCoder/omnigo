package adapter

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/YspCoder/omnigo/dto"
	"github.com/YspCoder/omnigo/utils"
	"google.golang.org/genai"
)

// Veo accepts at most three asset reference images per request.
const maxGoogleVideoReferenceImages = 3

// googleVideoFrames holds the image inputs of a Veo request. Callers pass them
// in Extra:
//
//	first_frame       string    start frame (image-to-video)
//	last_frame        string    end frame; requires first_frame
//	reference_images  []string  asset references (reference-to-video)
//
// Values may be HTTP(S) URLs, data URLs or gs:// URIs. Reference images cannot
// be combined with frames, matching the Veo task types.
type googleVideoFrames struct {
	first, last string
	references  []string
}

func googleVideoFramesFromRequest(r *dto.MediaRequest) (googleVideoFrames, error) {
	frames := googleVideoFrames{
		first:      strings.TrimSpace(utils.GetStringExtra(r.Extra, "first_frame")),
		last:       strings.TrimSpace(utils.GetStringExtra(r.Extra, "last_frame")),
		references: compactGoogleImageInputs(utils.GetStringSliceExtra(r.Extra, "reference_images")),
	}
	if frames.last != "" && frames.first == "" {
		return frames, fmt.Errorf("google video last_frame requires first_frame")
	}
	if len(frames.references) > 0 && frames.first != "" {
		return frames, fmt.Errorf("google video reference_images cannot be combined with first_frame or last_frame")
	}
	if len(frames.references) > maxGoogleVideoReferenceImages {
		return frames, fmt.Errorf("google video accepts at most %d reference_images", maxGoogleVideoReferenceImages)
	}
	return frames, nil
}

// apply loads the frames, sets the end frame and references on config, and
// returns the start frame for GenerateVideos.
func (f googleVideoFrames) apply(ctx context.Context, httpClient *http.Client, config *genai.GenerateVideosConfig) (*genai.Image, error) {
	var first *genai.Image
	var err error
	if f.first != "" {
		if first, err = googleVideoImage(ctx, httpClient, f.first); err != nil {
			return nil, err
		}
	}
	if f.last != "" {
		if config.LastFrame, err = googleVideoImage(ctx, httpClient, f.last); err != nil {
			return nil, err
		}
	}
	for _, raw := range f.references {
		image, err := googleVideoImage(ctx, httpClient, raw)
		if err != nil {
			return nil, err
		}
		config.ReferenceImages = append(config.ReferenceImages, &genai.VideoGenerationReferenceImage{
			Image:         image,
			ReferenceType: genai.VideoGenerationReferenceTypeAsset,
		})
	}
	return first, nil
}

func googleVideoImage(ctx context.Context, httpClient *http.Client, raw string) (*genai.Image, error) {
	if strings.HasPrefix(strings.ToLower(raw), "gs://") {
		return &genai.Image{GCSURI: raw, MIMEType: googleImageMIMEType(raw)}, nil
	}
	part, err := googleImagePartFromInput(ctx, httpClient, raw)
	if err != nil {
		return nil, err
	}
	if part.InlineData == nil {
		return nil, fmt.Errorf("unsupported google video image input %q", raw)
	}
	return &genai.Image{ImageBytes: part.InlineData.Data, MIMEType: part.InlineData.MIMEType}, nil
}
