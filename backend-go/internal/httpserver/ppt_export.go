package httpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	pptapp "xianzhi-ai/backend-go/internal/app/ppt"
)

const (
	pptxSlideWidth  = 12192000
	pptxSlideHeight = 6858000
)

type pptExportRequest struct {
	TaskID string `json:"taskId"`
}

type pptxMedia struct {
	FileName string
	Content  []byte
}

type pptxImageResolver func(context.Context, string, string) (string, []byte, bool)

func validatePPTExportTask(task pptapp.Task) error {
	if task.Stage != pptapp.StageReady || pptapp.ValidateTaskStage(task) != nil {
		return newPPTAgentError(http.StatusConflict, "PPT_EXPORT_NOT_READY", "PPT 尚未完成，暂不可导出")
	}
	if strings.TrimSpace(task.BillingTaskID) == "" || task.SlideCount <= 0 || len(task.Slides) != task.SlideCount {
		return newPPTAgentError(http.StatusConflict, "PPT_EXPORT_INCOMPLETE", "PPT 内容不完整，暂不可导出")
	}
	pages := make(map[int]struct{}, task.SlideCount)
	ids := make(map[string]struct{}, task.SlideCount)
	for _, slide := range task.Slides {
		id := strings.TrimSpace(slide.ID)
		if id == "" || slide.Page < 1 || slide.Page > task.SlideCount || len(pptapp.NormalizeSlideIR(slide).Blocks) == 0 {
			return newPPTAgentError(http.StatusConflict, "PPT_EXPORT_INCOMPLETE", "PPT 内容不完整，暂不可导出")
		}
		if _, exists := pages[slide.Page]; exists {
			return newPPTAgentError(http.StatusConflict, "PPT_EXPORT_INCOMPLETE", "PPT 内容不完整，暂不可导出")
		}
		if _, exists := ids[id]; exists {
			return newPPTAgentError(http.StatusConflict, "PPT_EXPORT_INCOMPLETE", "PPT 内容不完整，暂不可导出")
		}
		pages[slide.Page] = struct{}{}
		ids[id] = struct{}{}
	}
	return nil
}

func buildPPTX(task pptapp.Task) ([]byte, error) {
	return buildPPTXWithImageResolver(context.Background(), task, nil)
}

func buildPPTXWithImageResolver(ctx context.Context, task pptapp.Task, resolveImage pptxImageResolver) ([]byte, error) {
	slides := task.Slides
	if len(slides) == 0 && task.Outline != nil {
		for index, outlineSlide := range task.Outline.Slides {
			slides = append(slides, pptapp.NormalizeSlideIR(pptapp.Slide{
				ID: fmt.Sprintf("slide_%d", index+1), Page: index + 1, Layout: outlineSlide.Layout,
				Blocks: []pptapp.SlideBlock{
					{Type: "title", Text: outlineSlide.Title},
					{Type: "paragraph", Text: outlineSlide.Summary},
					{Type: "bullets", Items: append([]string(nil), outlineSlide.BulletPoints...)},
				},
			}))
		}
	}
	if len(slides) == 0 {
		slides = []pptapp.Slide{pptapp.NormalizeSlideIR(pptapp.Slide{
			ID: "slide_1", Page: 1, Layout: "cover",
			Blocks: []pptapp.SlideBlock{
				{Type: "title", Text: firstPPTXNonEmpty(task.Title, task.Prompt, "Presentation")},
				{Type: "paragraph", Text: task.Prompt},
			},
		})}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	mediaBySlide := make(map[int]pptxMedia)
	mediaIndex := 1
	for index, slide := range slides {
		content := pptxSlideContentForTask(task, slide)
		if media, ok := resolvePPTXSlideImage(ctx, resolveImage, task.TenantID, content.ImageRef, mediaIndex); ok {
			mediaBySlide[index] = media
			mediaIndex++
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := addPPTXString(zw, "[Content_Types].xml", pptxContentTypesXML(len(slides))); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "_rels/.rels", pptxRootRelsXML()); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "docProps/core.xml", pptxCoreXML(task)); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "docProps/app.xml", pptxAppXML(len(slides))); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/presentation.xml", pptxPresentationXML(len(slides))); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/_rels/presentation.xml.rels", pptxPresentationRelsXML(len(slides))); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/slideMasters/slideMaster1.xml", pptxSlideMasterXML()); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/slideMasters/_rels/slideMaster1.xml.rels", pptxSlideMasterRelsXML()); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/slideLayouts/slideLayout1.xml", pptxSlideLayoutXML()); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/slideLayouts/_rels/slideLayout1.xml.rels", pptxSlideLayoutRelsXML()); err != nil {
		return nil, err
	}
	if err := addPPTXString(zw, "ppt/theme/theme1.xml", pptxThemeXML()); err != nil {
		return nil, err
	}
	for index, slide := range slides {
		if err := addPPTXString(zw, fmt.Sprintf("ppt/slides/slide%d.xml", index+1), pptxSlideXML(task, slide, index, len(slides), mediaBySlide[index])); err != nil {
			return nil, err
		}
		if err := addPPTXString(zw, fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", index+1), pptxSlideRelsXML(mediaBySlide[index])); err != nil {
			return nil, err
		}
	}
	for _, media := range mediaBySlide {
		if err := addPPTXBytes(zw, "ppt/media/"+media.FileName, media.Content); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func pptxDownloadFileName(task pptapp.Task) string {
	title := sanitizePPTXFileName(firstPPTXNonEmpty(task.Title, task.Prompt, "presentation"))
	if title == "" {
		title = "presentation"
	}
	if !strings.HasSuffix(strings.ToLower(title), ".pptx") {
		title += ".pptx"
	}
	return title
}

func pptxContentDisposition(fileName string) string {
	ascii := sanitizePPTXASCIIFileName(fileName)
	if ascii == "" {
		ascii = "presentation.pptx"
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(fileName))
}

func pptxContentTypesXML(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Default Extension="png" ContentType="image/png"/>`)
	b.WriteString(`<Default Extension="jpg" ContentType="image/jpeg"/>`)
	b.WriteString(`<Default Extension="jpeg" ContentType="image/jpeg"/>`)
	b.WriteString(`<Default Extension="webp" ContentType="image/webp"/>`)
	b.WriteString(`<Default Extension="gif" ContentType="image/gif"/>`)
	b.WriteString(`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>`)
	b.WriteString(`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`)
	for i := 1; i <= slideCount; i++ {
		b.WriteString(fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, i))
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func pptxRootRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
		`</Relationships>`
}

func pptxCoreXML(task pptapp.Task) string {
	now := time.Now().UTC().Format(time.RFC3339)
	title := pptxEscape(firstPPTXNonEmpty(task.Title, task.Prompt, "Presentation"))
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`+
		`<dc:title>%s</dc:title><dc:creator>Xianzhi AI</dc:creator><cp:lastModifiedBy>Xianzhi AI</cp:lastModifiedBy>`+
		`<dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified>`+
		`</cp:coreProperties>`, title, now, now)
}

func pptxAppXML(slideCount int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">`+
		`<Application>Xianzhi AI</Application><PresentationFormat>Widescreen</PresentationFormat><Slides>%d</Slides><Notes>0</Notes><HiddenSlides>0</HiddenSlides>`+
		`</Properties>`, slideCount)
}

func pptxPresentationXML(slideCount int) string {
	var slideIDs strings.Builder
	for i := 1; i <= slideCount; i++ {
		slideIDs.WriteString(fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, 255+i, i+1))
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>` +
		`<p:sldIdLst>` + slideIDs.String() + `</p:sldIdLst>` +
		fmt.Sprintf(`<p:sldSz cx="%d" cy="%d" type="screen16x9"/><p:notesSz cx="6858000" cy="9144000"/>`, pptxSlideWidth, pptxSlideHeight) +
		`</p:presentation>`
}

func pptxPresentationRelsXML(slideCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	b.WriteString(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>`)
	for i := 1; i <= slideCount; i++ {
		b.WriteString(fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i+1, i))
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func pptxSlideMasterXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld>` +
		`<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>` +
		`<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>` +
		`<p:txStyles><p:titleStyle>` + pptxTextStyleLevels(4000, "+mj-lt", "+mj-ea", "+mj-cs") + `</p:titleStyle>` +
		`<p:bodyStyle>` + pptxTextStyleLevels(1800, "+mn-lt", "+mn-ea", "+mn-cs") + `</p:bodyStyle>` +
		`<p:otherStyle><a:defPPr><a:defRPr lang="zh-CN"/></a:defPPr>` + pptxTextStyleLevels(1800, "+mn-lt", "+mn-ea", "+mn-cs") + `</p:otherStyle></p:txStyles>` +
		`</p:sldMaster>`
}

func pptxSlideMasterRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="../theme/theme1.xml"/>` +
		`</Relationships>`
}

func pptxSlideLayoutXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" type="blank" preserve="1">` +
		`<p:cSld name="Blank"><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr></p:spTree></p:cSld>` +
		`<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>` +
		`</p:sldLayout>`
}

func pptxSlideLayoutRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="../slideMasters/slideMaster1.xml"/>` +
		`</Relationships>`
}

func pptxThemeXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Xianzhi AI">` +
		`<a:themeElements><a:clrScheme name="Xianzhi AI">` +
		`<a:dk1><a:srgbClr val="111827"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="1F2937"/></a:dk2><a:lt2><a:srgbClr val="F8FAFC"/></a:lt2>` +
		`<a:accent1><a:srgbClr val="4F46E5"/></a:accent1><a:accent2><a:srgbClr val="06B6D4"/></a:accent2><a:accent3><a:srgbClr val="22C55E"/></a:accent3><a:accent4><a:srgbClr val="F97316"/></a:accent4><a:accent5><a:srgbClr val="A855F7"/></a:accent5><a:accent6><a:srgbClr val="14B8A6"/></a:accent6>` +
		`<a:hlink><a:srgbClr val="2563EB"/></a:hlink><a:folHlink><a:srgbClr val="7C3AED"/></a:folHlink></a:clrScheme>` +
		`<a:fontScheme name="Xianzhi AI"><a:majorFont><a:latin typeface="Microsoft YaHei"/><a:ea typeface="Microsoft YaHei"/><a:cs typeface="Arial"/></a:majorFont><a:minorFont><a:latin typeface="Microsoft YaHei"/><a:ea typeface="Microsoft YaHei"/><a:cs typeface="Arial"/></a:minorFont></a:fontScheme>` +
		pptxFormatSchemeXML() +
		`</a:themeElements><a:objectDefaults/><a:extraClrSchemeLst/></a:theme>`
}

func pptxTextStyleLevels(size int, latin string, eastAsian string, complex string) string {
	var b strings.Builder
	for level := 1; level <= 9; level++ {
		marL := (level - 1) * 457200
		b.WriteString(fmt.Sprintf(`<a:lvl%dpPr marL="%d" algn="l" defTabSz="457200" rtl="0" eaLnBrk="1" latinLnBrk="0" hangingPunct="1"><a:defRPr sz="%d" kern="1200"><a:solidFill><a:schemeClr val="tx1"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/><a:cs typeface="%s"/></a:defRPr></a:lvl%dpPr>`, level, marL, size, latin, eastAsian, complex, level))
	}
	return b.String()
}

func pptxFormatSchemeXML() string {
	return `<a:fmtScheme name="Xianzhi AI">` +
		`<a:fillStyleLst>` +
		`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:gradFill rotWithShape="1"><a:gsLst><a:gs pos="0"><a:schemeClr val="phClr"><a:tint val="50000"/><a:satMod val="300000"/></a:schemeClr></a:gs><a:gs pos="35000"><a:schemeClr val="phClr"><a:tint val="37000"/><a:satMod val="300000"/></a:schemeClr></a:gs><a:gs pos="100000"><a:schemeClr val="phClr"><a:tint val="15000"/><a:satMod val="350000"/></a:schemeClr></a:gs></a:gsLst><a:lin ang="16200000" scaled="1"/></a:gradFill>` +
		`<a:gradFill rotWithShape="1"><a:gsLst><a:gs pos="0"><a:schemeClr val="phClr"><a:shade val="51000"/><a:satMod val="130000"/></a:schemeClr></a:gs><a:gs pos="80000"><a:schemeClr val="phClr"><a:shade val="93000"/><a:satMod val="130000"/></a:schemeClr></a:gs><a:gs pos="100000"><a:schemeClr val="phClr"><a:shade val="94000"/><a:satMod val="135000"/></a:schemeClr></a:gs></a:gsLst><a:lin ang="16200000" scaled="0"/></a:gradFill>` +
		`</a:fillStyleLst>` +
		`<a:lnStyleLst>` +
		pptxThemeLineXML(6350) + pptxThemeLineXML(12700) + pptxThemeLineXML(19050) +
		`</a:lnStyleLst>` +
		`<a:effectStyleLst>` +
		`<a:effectStyle><a:effectLst/></a:effectStyle>` +
		`<a:effectStyle><a:effectLst><a:outerShdw blurRad="40000" dist="20000" dir="5400000" rotWithShape="0"><a:srgbClr val="000000"><a:alpha val="20000"/></a:srgbClr></a:outerShdw></a:effectLst></a:effectStyle>` +
		`<a:effectStyle><a:effectLst><a:outerShdw blurRad="80000" dist="30000" dir="5400000" rotWithShape="0"><a:srgbClr val="000000"><a:alpha val="18000"/></a:srgbClr></a:outerShdw></a:effectLst></a:effectStyle>` +
		`</a:effectStyleLst>` +
		`<a:bgFillStyleLst>` +
		`<a:solidFill><a:schemeClr val="phClr"/></a:solidFill>` +
		`<a:solidFill><a:schemeClr val="phClr"><a:tint val="95000"/><a:satMod val="170000"/></a:schemeClr></a:solidFill>` +
		`<a:gradFill rotWithShape="1"><a:gsLst><a:gs pos="0"><a:schemeClr val="phClr"><a:tint val="93000"/><a:satMod val="150000"/></a:schemeClr></a:gs><a:gs pos="100000"><a:schemeClr val="phClr"><a:shade val="98000"/><a:satMod val="130000"/></a:schemeClr></a:gs></a:gsLst><a:lin ang="5400000" scaled="0"/></a:gradFill>` +
		`</a:bgFillStyleLst>` +
		`</a:fmtScheme>`
}

func pptxThemeLineXML(width int) string {
	return fmt.Sprintf(`<a:ln w="%d" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/><a:miter lim="800000"/></a:ln>`, width)
}

func pptxSlideXML(task pptapp.Task, slide pptapp.Slide, index int, total int, media pptxMedia) string {
	content := pptxSlideContentForTask(task, slide)
	accent := pptxAccentColor(task.Theme)
	background := pptxBackgroundColor(task.Theme, index)
	titleColor := "111827"
	bodyColor := "334155"
	if pptxIsDarkBackground(background) {
		titleColor = "F8FAFC"
		bodyColor = "E2E8F0"
	}

	hasImage := len(media.Content) > 0
	titleWidth := 7000000
	bodyWidth := 6100000
	if !hasImage {
		titleWidth = 10400000
		bodyWidth = 10100000
	}

	var shapes strings.Builder
	shapes.WriteString(pptxTextShape(2, "Page", 760000, 520000, 1300000, 320000, pptxParagraph(fmt.Sprintf("%d / %d", index+1, total), 1600, accent, true)))
	shapes.WriteString(pptxTextShape(3, "Title", 760000, 940000, titleWidth, 950000, pptxParagraph(content.Title, 3900, titleColor, true)))
	shapes.WriteString(pptxTextShape(4, "Summary", 760000, 1960000, bodyWidth, 1080000, pptxSlideBodyParagraphs(content, bodyColor)))
	if len(content.Bullets) > 0 {
		shapes.WriteString(pptxTextShape(5, "Bullets", 900000, 3100000, bodyWidth-300000, 2400000, pptxBulletParagraphs(content.Bullets, bodyColor)))
	}
	if hasImage {
		shapes.WriteString(pptxPicture(10, "Slide image", 7460000, 1600000, 3900000, 3420000))
	} else {
		shapes.WriteString(pptxAccentPanel(10, 7440000, 1780000, 3600000, 2900000, accent, pptxIsDarkBackground(background)))
	}
	shapes.WriteString(pptxFooterShape(20, firstPPTXNonEmpty(task.Title, task.Prompt, "Presentation"), accent, pptxIsDarkBackground(background)))

	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:bg><p:bgPr><a:solidFill><a:srgbClr val="` + background + `"/></a:solidFill><a:effectLst/></p:bgPr></p:bg>` +
		`<p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>` +
		shapes.String() +
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`
}

type pptxSlideContent struct {
	Title      string
	Subtitles  []string
	Paragraphs []string
	Bullets    []string
	ImageRef   string
}

func pptxSlideContentForTask(task pptapp.Task, slide pptapp.Slide) pptxSlideContent {
	slide = pptapp.NormalizeSlideIR(slide)
	content := pptxSlideContent{}
	for _, block := range slide.Blocks {
		switch block.Type {
		case "title":
			if content.Title == "" {
				content.Title = strings.TrimSpace(block.Text)
			}
		case "subtitle":
			if text := strings.TrimSpace(block.Text); text != "" {
				content.Subtitles = append(content.Subtitles, text)
			}
		case "paragraph":
			if text := strings.TrimSpace(block.Text); text != "" {
				content.Paragraphs = append(content.Paragraphs, text)
			}
		case "bullets":
			for _, item := range block.Items {
				if item = strings.TrimSpace(item); item != "" {
					content.Bullets = append(content.Bullets, item)
				}
			}
		case "image":
			if content.ImageRef == "" {
				content.ImageRef = strings.TrimSpace(block.ImageRef)
			}
		case "note":
			// Notes remain data-only until the exporter supports OOXML notes parts.
		}
	}
	content.Title = firstPPTXNonEmpty(content.Title, task.Title, "Slide")
	return content
}

func pptxSlideBodyParagraphs(content pptxSlideContent, color string) string {
	var body strings.Builder
	for _, subtitle := range content.Subtitles {
		body.WriteString(pptxParagraph(subtitle, 2100, color, true))
	}
	for _, paragraph := range content.Paragraphs {
		body.WriteString(pptxParagraph(paragraph, 1900, color, false))
	}
	if body.Len() == 0 {
		return pptxParagraph("", 1900, color, false)
	}
	return body.String()
}

func pptxSlideRelsXML(media pptxMedia) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	b.WriteString(`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`)
	if len(media.Content) > 0 {
		b.WriteString(fmt.Sprintf(`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/%s"/>`, pptxEscape(media.FileName)))
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

func pptxTextShape(id int, name string, x int, y int, cx int, cy int, body string) string {
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="square"/><a:lstStyle/>%s</p:txBody></p:sp>`,
		id, pptxEscape(name), x, y, cx, cy, body)
}

func pptxParagraph(text string, size int, color string, bold bool) string {
	boldAttr := ""
	if bold {
		boldAttr = ` b="1"`
	}
	return fmt.Sprintf(`<a:p><a:r><a:rPr lang="zh-CN" sz="%d"%s><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="Microsoft YaHei"/><a:ea typeface="Microsoft YaHei"/></a:rPr><a:t>%s</a:t></a:r><a:endParaRPr lang="zh-CN"/></a:p>`,
		size, boldAttr, color, pptxEscape(truncatePPTXText(text, 180)))
}

func pptxBulletParagraphs(points []string, color string) string {
	var b strings.Builder
	for _, point := range points {
		point = strings.TrimSpace(point)
		if point == "" {
			continue
		}
		b.WriteString(fmt.Sprintf(`<a:p><a:pPr marL="320000" indent="-220000"><a:buChar char="&#8226;"/></a:pPr><a:r><a:rPr lang="zh-CN" sz="1800"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="Microsoft YaHei"/><a:ea typeface="Microsoft YaHei"/></a:rPr><a:t>%s</a:t></a:r><a:endParaRPr lang="zh-CN"/></a:p>`, color, pptxEscape(truncatePPTXText(point, 120))))
	}
	if b.Len() == 0 {
		return pptxParagraph("", 1800, color, false)
	}
	return b.String()
}

func pptxPicture(id int, name string, x int, y int, cx int, cy int) string {
	return fmt.Sprintf(`<p:pic><p:nvPicPr><p:cNvPr id="%d" name="%s"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="rId2"/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom><a:ln w="12700"><a:solidFill><a:srgbClr val="E5E7EB"/></a:solidFill></a:ln></p:spPr></p:pic>`,
		id, pptxEscape(name), x, y, cx, cy)
}

func pptxAccentPanel(id int, x int, y int, cx int, cy int, accent string, dark bool) string {
	textColor := "FFFFFF"
	panelFill := accent
	if dark {
		textColor = "111827"
		panelFill = "F8FAFC"
	}
	body := pptxParagraph("AI generated presentation", 2100, textColor, true) +
		pptxParagraph("PPTX export by Xianzhi AI", 1450, textColor, false)
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Accent panel"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"><a:alpha val="90000"/></a:srgbClr></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr><p:txBody><a:bodyPr anchor="ctr" wrap="square"/><a:lstStyle/>%s</p:txBody></p:sp>`,
		id, x, y, cx, cy, panelFill, body)
}

func pptxFooterShape(id int, title string, accent string, dark bool) string {
	color := "64748B"
	if dark {
		color = "CBD5E1"
	}
	body := pptxParagraph(truncatePPTXText(title, 60), 1200, color, false)
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Footer"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="760000" y="6280000"/><a:ext cx="10400000" cy="260000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/><a:ln><a:solidFill><a:srgbClr val="%s"><a:alpha val="50000"/></a:srgbClr></a:solidFill></a:ln></p:spPr><p:txBody><a:bodyPr wrap="square"/><a:lstStyle/>%s</p:txBody></p:sp>`, id, accent, body)
}

func resolvePPTXSlideImage(ctx context.Context, resolveImage pptxImageResolver, expectedTenantID string, raw string, index int) (pptxMedia, bool) {
	if resolveImage == nil {
		return pptxMedia{}, false
	}
	tenantID, fileID, ok := parsePPTStorageReference(raw)
	if !ok || strings.TrimSpace(expectedTenantID) == "" || tenantID != strings.TrimSpace(expectedTenantID) {
		return pptxMedia{}, false
	}
	contentType, data, ok := resolveImage(ctx, tenantID, fileID)
	if !ok || len(data) == 0 || len(data) > 8<<20 {
		return pptxMedia{}, false
	}
	ext, _ := pptxImageType(contentType, data)
	if ext == "" {
		return pptxMedia{}, false
	}
	return pptxMedia{
		FileName: fmt.Sprintf("image%d.%s", index, ext),
		Content:  data,
	}, true
}

func pptxImageType(contentType string, data []byte) (string, string) {
	switch strings.ToLower(contentType) {
	case "image/png":
		return "png", "image/png"
	case "image/jpeg", "image/jpg":
		return "jpg", "image/jpeg"
	case "image/webp":
		return "webp", "image/webp"
	case "image/gif":
		return "gif", "image/gif"
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "png", "image/png"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "jpg", "image/jpeg"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "webp", "image/webp"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "gif", "image/gif"
	}
	return "", ""
}

func pptxContentTypeFromExt(ext string) string {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	default:
		return ""
	}
}

func addPPTXString(zw *zip.Writer, name string, body string) error {
	return addPPTXBytes(zw, name, []byte(body))
}

func addPPTXBytes(zw *zip.Writer, name string, body []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

func pptxAccentColor(theme string) string {
	switch strings.TrimSpace(theme) {
	case "technology", "tech-blue":
		return "2563EB"
	case "education":
		return "7C3AED"
	case "marketing":
		return "F97316"
	case "roadshow":
		return "06B6D4"
	case "medical":
		return "14B8A6"
	default:
		return "4F46E5"
	}
}

func pptxBackgroundColor(theme string, index int) string {
	if index == 0 {
		switch strings.TrimSpace(theme) {
		case "technology", "tech-blue":
			return "EFF6FF"
		case "education":
			return "F5F3FF"
		case "marketing":
			return "FFF7ED"
		case "roadshow":
			return "ECFEFF"
		case "medical":
			return "F0FDFA"
		}
	}
	return "F8FAFC"
}

func pptxIsDarkBackground(hex string) bool {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return false
	}
	var r, g, b int
	_, _ = fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r*299+g*587+b*114 < 128000
}

func firstPPTXNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncatePPTXText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes-1]) + "..."
}

func pptxEscape(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	return html.EscapeString(value)
}

func sanitizePPTXFileName(value string) string {
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|', '\r', '\n', '\t':
			return '-'
		default:
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}
	}, strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, ". -")
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80])
	}
	return value
}

func sanitizePPTXASCIIFileName(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), ".-_")
}
