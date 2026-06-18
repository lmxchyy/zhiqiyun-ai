package httpserver

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"
)

const (
	previewImageWidth  = 1920
	previewImageHeight = 1080
)

func promptPreviewImage(prompt string) string {
	svg := genericPromptSVG(prompt)
	lowerPrompt := strings.ToLower(prompt)
	if strings.Contains(prompt, "\u732b") || strings.Contains(lowerPrompt, "cat") || strings.Contains(lowerPrompt, "kitten") {
		svg = catPromptSVG(prompt)
	}
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func catPromptSVG(prompt string) string {
	escapedPrompt := html.EscapeString(prompt)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 960 540" role="img" aria-label="%s">
  <defs>
    <linearGradient id="bg" x1="0" x2="1" y1="0" y2="1">
      <stop offset="0" stop-color="#fff7ed"/>
      <stop offset="1" stop-color="#dbeafe"/>
    </linearGradient>
    <filter id="shadow" x="-20%%" y="-20%%" width="140%%" height="140%%">
      <feDropShadow dx="0" dy="18" stdDeviation="18" flood-color="#475467" flood-opacity=".22"/>
    </filter>
  </defs>
  <rect width="960" height="540" fill="url(#bg)"/>
  <circle cx="178" cy="122" r="72" fill="#fed7aa" opacity=".7"/>
  <circle cx="780" cy="104" r="92" fill="#bfdbfe" opacity=".7"/>
  <g id="cat-subject" filter="url(#shadow)">
    <ellipse cx="480" cy="358" rx="178" ry="116" fill="#f8fafc"/>
    <path d="M332 292 L378 184 L443 274 Z" fill="#f8fafc"/>
    <path d="M628 292 L582 184 L517 274 Z" fill="#f8fafc"/>
    <path d="M374 260 L389 221 L420 262 Z" fill="#fb923c" opacity=".72"/>
    <path d="M586 260 L571 221 L540 262 Z" fill="#fb923c" opacity=".72"/>
    <circle cx="418" cy="334" r="18" fill="#111827"/>
    <circle cx="542" cy="334" r="18" fill="#111827"/>
    <circle cx="412" cy="328" r="6" fill="#fff"/>
    <circle cx="536" cy="328" r="6" fill="#fff"/>
    <path d="M480 358 l-18 20 h36 z" fill="#fb7185"/>
    <path d="M462 390 q18 18 36 0" fill="none" stroke="#111827" stroke-width="7" stroke-linecap="round"/>
    <path d="M374 372 h-86 M374 394 h-100 M586 372 h86 M586 394 h100" stroke="#111827" stroke-width="7" stroke-linecap="round"/>
    <path d="M334 414 q-86 24-124-42 q44-7 74 18 q32 26 50 24" fill="none" stroke="#f8fafc" stroke-width="42" stroke-linecap="round"/>
  </g>
  <text x="480" y="492" text-anchor="middle" font-family="Arial, 'Microsoft YaHei', sans-serif" font-size="24" fill="#344054">%s</text>
</svg>`, previewImageWidth, previewImageHeight, escapedPrompt, escapedPrompt)
}

func genericPromptSVG(prompt string) string {
	escapedPrompt := html.EscapeString(prompt)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 960 540" role="img" aria-label="%s">
  <defs>
    <linearGradient id="bg" x1="0" x2="1" y1="0" y2="1">
      <stop offset="0" stop-color="#ecfeff"/>
      <stop offset="1" stop-color="#fef3c7"/>
    </linearGradient>
  </defs>
  <rect width="960" height="540" fill="url(#bg)"/>
  <rect x="184" y="96" width="592" height="316" rx="36" fill="#ffffff" opacity=".82"/>
  <circle cx="310" cy="220" r="62" fill="#38bdf8" opacity=".75"/>
  <rect x="410" y="166" width="238" height="108" rx="24" fill="#111827" opacity=".9"/>
  <path d="M272 344 C374 286 440 386 530 320 C606 264 650 322 704 288" fill="none" stroke="#f97316" stroke-width="18" stroke-linecap="round"/>
  <text x="480" y="470" text-anchor="middle" font-family="Arial, 'Microsoft YaHei', sans-serif" font-size="24" fill="#344054">%s</text>
</svg>`, previewImageWidth, previewImageHeight, escapedPrompt, escapedPrompt)
}
