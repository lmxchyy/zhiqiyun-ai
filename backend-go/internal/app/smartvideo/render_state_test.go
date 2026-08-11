package smartvideo

import "testing"

func TestRenderStateMachine(t *testing.T) {
	allowed := [][2]string{
		{RenderStatusCreated, RenderStatusQueued},
		{RenderStatusQueued, RenderStatusProcessing},
		{RenderStatusProcessing, RenderStatusSynthesizing},
		{RenderStatusSynthesizing, RenderStatusRendering},
		{RenderStatusRendering, RenderStatusUploading},
		{RenderStatusUploading, RenderStatusPublishing},
		{RenderStatusPublishing, RenderStatusSucceeded},
	}
	for _, transition := range allowed {
		if err := ValidateRenderTransition(transition[0], transition[1]); err != nil {
			t.Fatalf("expected %s -> %s: %v", transition[0], transition[1], err)
		}
	}
	for _, transition := range [][2]string{
		{RenderStatusCreated, RenderStatusSucceeded},
		{RenderStatusUploading, RenderStatusRendering},
		{RenderStatusSucceeded, RenderStatusQueued},
		{RenderStatusProcessing, RenderStatusRendering},
		{RenderStatusRendering, RenderStatusCancelled},
	} {
		if err := ValidateRenderTransition(transition[0], transition[1]); err == nil {
			t.Fatalf("unexpected transition %s -> %s", transition[0], transition[1])
		}
	}
}
