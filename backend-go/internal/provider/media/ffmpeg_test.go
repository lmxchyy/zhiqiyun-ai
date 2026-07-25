package media

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"xianzhi-ai/backend-go/internal/app/smartvideo"
)

type fakeCommandRunner struct {
	run func(context.Context, string, []string) (CommandResult, error)
}

func (r fakeCommandRunner) Run(ctx context.Context, executable string, args []string, _, _ int64) (CommandResult, error) {
	return r.run(ctx, executable, args)
}

func TestProbeVideoParsesRotationAudioAndVariableFrameRate(t *testing.T) {
	raw := `{
	  "streams":[
	    {"codec_type":"video","codec_name":"h264","pix_fmt":"yuv420p","width":1920,"height":1080,
	     "avg_frame_rate":"30000/1001","r_frame_rate":"30/1","bit_rate":"1200000",
	     "side_data_list":[{"rotation":90}]},
	    {"codec_type":"audio","codec_name":"aac","sample_rate":"48000","channels":2}
	  ],
	  "format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","format_long_name":"QuickTime / MOV",
	    "duration":"12.345","bit_rate":"1300000","tags":{"encoder":"Lavf","major_brand":"isom"}}
	}`
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(_ context.Context, _ string, args []string) (CommandResult, error) {
		if len(args) == 1 && args[0] == "-version" {
			return CommandResult{Stdout: []byte("ffprobe version 6.1.1\n")}, nil
		}
		return CommandResult{Stdout: []byte(raw)}, nil
	}}, "ffprobe", "ffmpeg", time.Second, time.Second)
	metadata, filtered, err := adapter.ProbeVideo(context.Background(), smartvideo.LocalMedia{Path: "internal-source"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.DurationMS != 12345 || metadata.DisplayWidth != 1080 || metadata.DisplayHeight != 1920 || metadata.Rotation != 90 {
		t.Fatalf("unexpected dimensions/duration: %+v", metadata)
	}
	if metadata.FPSNumerator != 30000 || metadata.FPSDenominator != 1001 || !metadata.HasAudio ||
		metadata.AudioCodec != "aac" || metadata.AudioSampleRate != 48000 || metadata.AudioChannels != 2 {
		t.Fatalf("unexpected streams: %+v", metadata)
	}
	if len(filtered.Warnings) != 1 || filtered.Warnings[0] != "variable_frame_rate_detected" {
		t.Fatalf("unexpected VFR warning: %+v", filtered)
	}
}

func TestProbeRejectsInvalidJSON(t *testing.T) {
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(_ context.Context, _ string, _ []string) (CommandResult, error) {
		return CommandResult{Stdout: []byte("{")}, nil
	}}, "ffprobe", "ffmpeg", time.Second, time.Second)
	_, _, err := adapter.ProbeVideo(context.Background(), smartvideo.LocalMedia{Path: "internal-source"})
	assertMediaErrorCode(t, err, smartvideo.MediaErrorInvalidJSON)
}

func TestProbeTimeoutReturnsStableError(t *testing.T) {
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(ctx context.Context, _ string, _ []string) (CommandResult, error) {
		<-ctx.Done()
		return CommandResult{}, ctx.Err()
	}}, "ffprobe", "ffmpeg", 10*time.Millisecond, time.Second)
	_, _, err := adapter.ProbeVideo(context.Background(), smartvideo.LocalMedia{Path: "internal-source"})
	assertMediaErrorCode(t, err, smartvideo.MediaErrorTimeout)
}

func TestProbeNonZeroExitDoesNotExposeCommandOrPath(t *testing.T) {
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(_ context.Context, _ string, _ []string) (CommandResult, error) {
		return CommandResult{Stderr: []byte("secret-path?X-Amz-Signature=secret")}, errors.New("exit status 1")
	}}, "ffprobe", "ffmpeg", time.Second, time.Second)
	_, _, err := adapter.ProbeVideo(context.Background(), smartvideo.LocalMedia{Path: "C:/private/source.mp4"})
	assertMediaErrorCode(t, err, smartvideo.MediaErrorProbeFailed)
	if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "Signature") {
		t.Fatalf("error leaked internal input or signed query: %v", err)
	}
}

func TestProbeRejectsDocumentWithoutVideoStream(t *testing.T) {
	raw := `{"streams":[{"codec_type":"audio","codec_name":"aac"}],"format":{"format_name":"aac"}}`
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(_ context.Context, _ string, args []string) (CommandResult, error) {
		if len(args) == 1 {
			return CommandResult{Stdout: []byte("ffprobe version 6.1.1")}, nil
		}
		return CommandResult{Stdout: []byte(raw)}, nil
	}}, "ffprobe", "ffmpeg", time.Second, time.Second)
	_, _, err := adapter.ProbeVideo(context.Background(), smartvideo.LocalMedia{Path: "internal-source"})
	assertMediaErrorCode(t, err, smartvideo.MediaErrorNoVideoStream)
}

func TestProbeImageParsesTypedMetadata(t *testing.T) {
	raw := `{"streams":[{"codec_type":"video","codec_name":"png","pix_fmt":"rgba","width":800,"height":600,"nb_frames":"2","tags":{"orientation":"6"}}],"format":{"format_name":"png_pipe","format_long_name":"piped png sequence"}}`
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(_ context.Context, _ string, args []string) (CommandResult, error) {
		if len(args) == 1 && args[0] == "-version" {
			return CommandResult{Stdout: []byte("ffprobe version 6.1.1")}, nil
		}
		return CommandResult{Stdout: []byte(raw)}, nil
	}}, "ffprobe", "ffmpeg", time.Second, time.Second)
	metadata, filtered, err := adapter.ProbeImage(context.Background(), smartvideo.LocalMedia{Path: "internal-source"})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Format != "png" || metadata.MIMEType != "image/png" || metadata.Width != 800 ||
		metadata.Height != 600 || metadata.Orientation != 6 || !metadata.Animated || metadata.FrameCount != 2 ||
		metadata.ColorSpace != "rgba" {
		t.Fatalf("unexpected image metadata: %+v", metadata)
	}
	if len(filtered.StreamCodecs) != 1 || filtered.StreamCodecs[0] != "video:png" {
		t.Fatalf("unexpected filtered result: %+v", filtered)
	}
}

func TestProbeMissingCommandReturnsStableError(t *testing.T) {
	adapter := NewFFmpegAdapter(fakeCommandRunner{run: func(_ context.Context, _ string, _ []string) (CommandResult, error) {
		return CommandResult{}, os.ErrNotExist
	}}, "ffprobe", "ffmpeg", time.Second, time.Second)
	_, _, err := adapter.ProbeVideo(context.Background(), smartvideo.LocalMedia{Path: "internal-source"})
	assertMediaErrorCode(t, err, smartvideo.MediaErrorToolMissing)
}

func assertMediaErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var mediaErr *smartvideo.MediaError
	if !errors.As(err, &mediaErr) || mediaErr.Code != code {
		t.Fatalf("error = %v, want media code %s", err, code)
	}
}
