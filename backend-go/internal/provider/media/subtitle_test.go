package media

import (
	"strings"
	"testing"
)

func TestSegmentNarrationSkipsBlankAndSplitsPunctuation(t *testing.T) {
	segments := SegmentNarration([]NarrationSceneInput{
		{Index: 0, Narration: "   ", DurationMs: 3000},
		{Index: 1, Narration: "欢迎观看。今天分享新品！", DurationMs: 8000},
		{Index: 2, Narration: "", DurationMs: 2000},
	}, 1)
	if len(segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(segments))
	}
	if segments[0].Text != "欢迎观看。" || segments[1].Text != "今天分享新品！" {
		t.Fatalf("unexpected texts: %+v", segments)
	}
	if segments[0].StartMs < 3000 {
		t.Fatalf("first speakable segment should start after blank scene, got %d", segments[0].StartMs)
	}
}

func TestEstimateNarrationDurationIncludesPunctuationPause(t *testing.T) {
	plain := EstimateNarrationDurationMs("一二三四五六七八", 1)
	paused := EstimateNarrationDurationMs("一二三四，五六七八。", 1)
	if paused <= plain {
		t.Fatalf("paused=%d plain=%d, punctuation should increase duration", paused, plain)
	}
}

func TestWrapSubtitleLinesBreaksLongText(t *testing.T) {
	lines := WrapSubtitleLines("这是一段需要自动换行的中文旁白内容用于字幕测试", 8)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped lines, got %#v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 8 {
			t.Fatalf("line too long: %q", line)
		}
	}
}

func TestBuildSubtitleCuesRejectsNonMonotonic(t *testing.T) {
	_, err := BuildSubtitleCues([]NarrationSegment{
		{Text: "a", StartMs: 1000, DurationMs: 1000},
		{Text: "b", StartMs: 500, DurationMs: 1000},
	})
	if err == nil {
		t.Fatal("expected non-monotonic error")
	}
}

func TestRenderSRTAndASSPresets(t *testing.T) {
	cues, err := BuildSubtitleCues([]NarrationSegment{
		{Text: "欢迎观看知启云", StartMs: 0, DurationMs: 1500},
		{Text: "自动混剪上线", StartMs: 1600, DurationMs: 1400},
	})
	if err != nil {
		t.Fatalf("cues: %v", err)
	}
	srt := RenderSRT(cues)
	if !strings.Contains(srt, "00:00:00,000 --> 00:00:01,500") {
		t.Fatalf("unexpected srt: %s", srt)
	}
	ass, err := RenderASS(cues, SubtitleStyle{Preset: SubtitlePresetClean, Position: SubtitlePositionBottom})
	if err != nil {
		t.Fatalf("ass clean: %v", err)
	}
	if !strings.Contains(ass, "Source Han Sans SC") || !strings.Contains(ass, "Dialogue:") {
		t.Fatalf("unexpected ass: %s", ass)
	}
	emphasis, err := RenderASS(cues, SubtitleStyle{Preset: SubtitlePresetEmphasis, Position: SubtitlePositionCenter, FontName: "Noto Sans CJK SC"})
	if err != nil {
		t.Fatalf("ass emphasis: %v", err)
	}
	if !strings.Contains(emphasis, "Noto Sans CJK SC") || !strings.Contains(emphasis, ",5,") {
		t.Fatalf("emphasis style missing: %s", emphasis)
	}
	if _, err := RenderASS(cues, SubtitleStyle{FontName: "Comic Sans"}); err == nil {
		t.Fatal("expected font whitelist rejection")
	}
}

func TestConcatenatePCM16MonoWAVs(t *testing.T) {
	a := SilencePCM16MonoWAV(100, 24000)
	b := SilencePCM16MonoWAV(200, 24000)
	merged, err := ConcatenatePCM16MonoWAVs([][]byte{a, b})
	if err != nil {
		t.Fatalf("concat: %v", err)
	}
	rate, pcm, err := decodePCM16MonoWAV(merged)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rate != 24000 {
		t.Fatalf("rate=%d", rate)
	}
	wantSamples := (100 + 200) * 24000 / 1000
	if len(pcm)/2 != wantSamples {
		t.Fatalf("samples=%d want=%d", len(pcm)/2, wantSamples)
	}
}
