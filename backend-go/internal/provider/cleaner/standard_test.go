package cleaner

import (
	"context"
	"testing"

	knowledgeapp "xianzhi-ai/backend-go/internal/app/knowledge"
)

func TestStandardRemovesRepeatedHeaderAndDuplicateUnit(t *testing.T) {
	units := []knowledgeapp.DocumentUnit{
		{Content: "企业内部资料\n第一章 产品介绍\n企业内部资料"},
		{Content: "企业内部资料\n第二章 使用方法\n企业内部资料"},
		{Content: "企业内部资料\n第二章 使用方法\n企业内部资料"},
		{Content: "企业内部资料\n第三章 售后服务\n企业内部资料"},
		{Content: "企业内部资料\n第四章 联系方式\n企业内部资料"},
	}
	result, metadata, err := (Standard{}).Normalize(context.Background(), units)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 4 || metadata["removedDuplicateUnits"] != 1 {
		t.Fatalf("result=%#v metadata=%#v", result, metadata)
	}
	for _, unit := range result {
		if unit.Content == "" || unit.Title == "" {
			t.Fatalf("unit was not normalized: %#v", unit)
		}
	}
}
