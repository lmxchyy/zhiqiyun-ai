package connector

import (
	"fmt"
	"strings"
)

type PromptBuilder interface {
	Build(Intent) string
}

type EcommerceImagePromptBuilder struct{}

func (EcommerceImagePromptBuilder) Build(intent Intent) string {
	subject := strings.TrimSpace(intent.Subject)
	if subject == "" {
		subject = "商品"
	}
	scene := strings.TrimSpace(intent.Scene)
	if scene == "" {
		scene = "电商主图"
	}
	return fmt.Sprintf("为%s制作一张%s概念效果图。产品主体突出，采用专业商业摄影布光和电商主图构图，呈现高清、真实、精致的材质与细节；使用与产品定位匹配的简洁背景并预留卖点文案区域。画面比例1:1，避免乱码、变形、品牌误用和水印，不虚构具体促销价格。未提供真实商品参考图，因此仅作为概念效果图。", subject, scene)
}
