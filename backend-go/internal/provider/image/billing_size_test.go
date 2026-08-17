package image

import "testing"

func TestNormalizeImageBillingSizeTierPresetsAndBoundaries(t *testing.T) {
	tests := []struct {
		size string
		want string
	}{
		{size: "auto", want: ImageBillingSizeAuto},
		{size: "AUTO", want: ImageBillingSizeAuto},

		{size: "1280x720", want: ImageBillingSizeTier720},
		{size: "720x1280", want: ImageBillingSizeTier720},
		{size: "1024x640", want: ImageBillingSizeTier720},

		{size: "1024x1024", want: ImageBillingSizeTier1K},
		{size: "1536x1024", want: ImageBillingSizeTier1K},
		{size: "1024x1536", want: ImageBillingSizeTier1K},
		{size: "1024x1280", want: ImageBillingSizeTier1K},

		{size: "2048x1152", want: ImageBillingSizeTier2K},
		{size: "1152x2048", want: ImageBillingSizeTier2K},
		{size: "2048x2048", want: ImageBillingSizeTier2K},
		{size: "1600x1024", want: ImageBillingSizeTier2K},
		{size: "1792x1024", want: ImageBillingSizeTier2K},
		{size: "1536x1536", want: ImageBillingSizeTier2K},
		{size: "1280x1280", want: ImageBillingSizeTier2K},

		{size: "3840x2160", want: ImageBillingSizeTier4K},
		{size: "2160x3840", want: ImageBillingSizeTier4K},
		{size: "2880x2880", want: ImageBillingSizeTier4K},
		{size: "2304x1280", want: ImageBillingSizeTier4K},
		{size: "2560x1440", want: ImageBillingSizeTier4K},
	}
	for _, tt := range tests {
		got, err := NormalizeImageBillingSizeTier(tt.size)
		if err != nil {
			t.Fatalf("size %s: %v", tt.size, err)
		}
		if got != tt.want {
			t.Fatalf("size %s tier = %s, want %s", tt.size, got, tt.want)
		}
	}
}

func TestNormalizeImageBillingSizeTierRejectsIllegalSizes(t *testing.T) {
	for _, size := range []string{"1023x1024", "4096x4096", "64x64", "3840x3840", "100x100"} {
		if _, err := NormalizeImageBillingSizeTier(size); err == nil {
			t.Fatalf("illegal size %s should not receive a billing tier", size)
		}
	}
}
