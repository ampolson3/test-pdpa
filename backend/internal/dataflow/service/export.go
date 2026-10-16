package service

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"pdpa-platform/internal/platform/docs/render"
)

// Paper is one of DFG-08's selectable export sizes — always rendered landscape, since the diagram flows
// left-to-right (org-unit lanes → activities → recipients → countries), never top-to-bottom.
type Paper string

const (
	PaperA3     Paper = "a3"
	PaperA4     Paper = "a4"
	PaperLetter Paper = "letter"
)

// paperCSSName is the CSS paged-media size keyword for each Paper — Chromium's print-to-pdf (and Gotenberg's
// own Chromium route, with preferCssPageSize) already honours a page's own `@page{size:...}` rule, the exact
// mechanism PLT-16's own pageCSS relies on for A4, so there is no need to compute point dimensions by hand.
func paperCSSName(p Paper) (string, error) {
	switch p {
	case PaperA3, "":
		return "A3", nil
	case PaperA4:
		return "A4", nil
	case PaperLetter:
		return "Letter", nil
	}
	return "", fmt.Errorf("%w: paper", ErrInvalid)
}

// paperPixels is the same three sizes in landscape device pixels at 150dpi — "readable clearly at A3" (DFG-08's
// own acceptance criterion) for the PNG screenshot path, which has no @page CSS to lean on.
func paperPixels(p Paper) (int, int, error) {
	switch p {
	case PaperA3, "":
		return 2481, 1754, nil // 16.54in x 11.69in @150dpi
	case PaperA4:
		return 1754, 1240, nil // 11.69in x 8.27in @150dpi
	case PaperLetter:
		return 1650, 1275, nil // 11in x 8.5in @150dpi
	}
	return 0, 0, fmt.Errorf("%w: paper", ErrInvalid)
}

// ExportFormat is DFG-08's own three output formats.
type ExportFormat string

const (
	FormatSVG ExportFormat = "svg"
	FormatPNG ExportFormat = "png"
	FormatPDF ExportFormat = "pdf"
)

// ExportDiagram is DFG-08's own acceptance criterion: the same diagram Graph() already computes live from
// RoPA, as a standalone image file sized for print. SVG is written directly (always crisp — no renderer
// needed); PNG and PDF wrap that same SVG in a minimal self-contained HTML page (Sarabun embedded, the same
// font PLT-16's own render.HTML already uses, so Thai labels never fall back to a missing-glyph tofu) and
// hand it to the shared platform/docs/render renderers — the diagram is not a PLT-16 document, but the
// rendering backends (Gotenberg in production, a local headless Chromium in development) are the same
// shared, stateless capability every other module's own PDF export already reuses.
func (s *Service) ExportDiagram(ctx context.Context, legalEntityID *uuid.UUID, format ExportFormat, paper Paper) ([]byte, string, error) {
	g, err := s.Graph(ctx, legalEntityID)
	if err != nil {
		return nil, "", err
	}
	l := layoutGraph(g)
	svg := renderSVG(l)

	switch format {
	case FormatSVG, "":
		return svg, "dataflow.svg", nil
	case FormatPNG:
		w, h, err := paperPixels(paper)
		if err != nil {
			return nil, "", err
		}
		if s.Image == nil {
			return nil, "", render.ErrNoImageRenderer
		}
		b, err := s.Image.PNG(ctx, screenshotHTML(svg, l.Width, l.Height, w, h), w, h)
		return b, "dataflow.png", err
	case FormatPDF:
		name, err := paperCSSName(paper)
		if err != nil {
			return nil, "", err
		}
		if s.PDF == nil {
			return nil, "", render.ErrNoRenderer
		}
		b, err := s.PDF.PDF(ctx, printHTML(svg, name))
		return b, "dataflow.pdf", err
	}
	return nil, "", fmt.Errorf("%w: format", ErrInvalid)
}

func screenshotHTML(svg []byte, contentW, contentH float64, pxW, pxH int) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><style>%s`, render.FontFaces())
	fmt.Fprintf(&b, `html,body{margin:0;width:%dpx;height:%dpx;background:#fff;display:flex;align-items:center;justify-content:center}`, pxW, pxH)
	b.WriteString(`svg{max-width:100%;max-height:100%}</style></head><body>`)
	b.Write(svg)
	b.WriteString(`</body></html>`)
	_ = contentW
	_ = contentH
	return []byte(b.String())
}

func printHTML(svg []byte, pageSize string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><style>%s`, render.FontFaces())
	fmt.Fprintf(&b, `@page{size:%s landscape;margin:8mm}body{margin:0;background:#fff}svg{width:100%%;height:auto;display:block}`, pageSize)
	b.WriteString(`</style></head><body>`)
	b.Write(svg)
	b.WriteString(`</body></html>`)
	return []byte(b.String())
}

// --- layout: a deterministic left-to-right grid, not ELK (Go has no JS layout engine available) — org-unit
// lanes with their activities on the left, a shared recipients column, then a countries column, each
// activity's disposal nodes stacked just beneath it. Same node/edge colours as apps/admin's own React Flow
// page (NODE_COLOR/EDGE_COLOR in data-flow-content.tsx) so an exported image matches the on-screen diagram.

const (
	laneLabelW           = 170.0
	actW, actH           = 210.0, 72.0
	actGapY              = 26.0
	laneGapY             = 30.0
	colGapX              = 170.0
	nodeW, nodeH         = 190.0, 58.0
	nodeGapY             = 22.0
	disposalW, disposalH = 120.0, 30.0
	marginX, marginY     = 30.0, 30.0
	legendH              = 70.0
)

type box struct{ X, Y, W, H float64 }

func (b box) cx() float64 { return b.X + b.W/2 }
func (b box) cy() float64 { return b.Y + b.H/2 }

type layout struct {
	Graph  Graph
	Boxes  map[string]box
	Lanes  map[string]box // org_unit background rects, separate from their label box
	Width  float64
	Height float64
}

func layoutGraph(g Graph) layout {
	l := layout{Graph: g, Boxes: map[string]box{}, Lanes: map[string]box{}}

	childrenByParent := map[string][]Node{}
	for _, n := range g.Nodes {
		if n.ParentID != "" {
			childrenByParent[n.ParentID] = append(childrenByParent[n.ParentID], n)
		}
	}

	actColX := marginX + laneLabelW
	laneY := marginY
	for _, ou := range g.Nodes {
		if ou.Type != "org_unit" {
			continue
		}
		acts := childrenByParent[ou.ID]
		laneContentH := float64(len(acts))*(actH+actGapY) - actGapY
		if laneContentH < actH {
			laneContentH = actH
		}
		l.Lanes[ou.ID] = box{X: marginX, Y: laneY, W: actColX + actW - marginX, H: laneContentH}
		for i, act := range acts {
			ay := laneY + float64(i)*(actH+actGapY)
			l.Boxes[act.ID] = box{X: actColX, Y: ay, W: actW, H: actH}
			for j, d := range childrenByParent[act.ID] {
				if d.Type != "disposal" {
					continue
				}
				l.Boxes[d.ID] = box{X: actColX + float64(j)*(disposalW+8), Y: ay + actH + 6, W: disposalW, H: disposalH}
			}
		}
		laneY += laneContentH + laneGapY
	}
	lanesBottom := laneY

	recipientX := actColX + actW + colGapX
	ry := marginY
	for _, n := range g.Nodes {
		if n.Type != "recipient" {
			continue
		}
		l.Boxes[n.ID] = box{X: recipientX, Y: ry, W: nodeW, H: nodeH}
		ry += nodeH + nodeGapY
	}
	recipientsBottom := ry

	countryX := recipientX + nodeW + colGapX
	cy := marginY
	for _, n := range g.Nodes {
		if n.Type != "country" {
			continue
		}
		l.Boxes[n.ID] = box{X: countryX, Y: cy, W: nodeW, H: nodeH}
		cy += nodeH + nodeGapY
	}
	countriesBottom := cy

	maxBottom := lanesBottom
	if recipientsBottom > maxBottom {
		maxBottom = recipientsBottom
	}
	if countriesBottom > maxBottom {
		maxBottom = countriesBottom
	}

	l.Width = countryX + nodeW + marginX
	l.Height = maxBottom + legendH + marginY
	return l
}

var nodeColor = map[string]string{"activity": "#0ea5e9", "recipient": "#8b5cf6", "country": "#f59e0b", "disposal": "#64748b"}
var edgeColor = map[string]string{"send": "#8b5cf6", "transfer": "#f59e0b", "destroy": "#64748b"}

const warningColor = "#dc2626"

func esc(s string) string { return html.EscapeString(s) }

func renderSVG(l layout) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" font-family="Sarabun, 'Noto Sans Thai', sans-serif">`,
		fnum(l.Width), fnum(l.Height))
	b.WriteString(`<rect x="0" y="0" width="100%" height="100%" fill="#ffffff"/>`)

	for _, n := range l.Graph.Nodes {
		if n.Type == "org_unit" {
			lane := l.Lanes[n.ID]
			fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" rx="8" fill="rgba(241,245,249,0.6)" stroke="#cbd5e1" stroke-width="1"/>`,
				fnum(lane.X), fnum(lane.Y), fnum(lane.W), fnum(lane.H))
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="13" font-weight="600" fill="#334155">%s</text>`,
				fnum(lane.X+8), fnum(lane.Y+18), esc(n.Label))
		}
	}

	// edges, drawn before node boxes so arrowheads/lines sit under the boxes' fill
	for _, e := range l.Graph.Edges {
		src, ok1 := l.Boxes[e.Source]
		dst, ok2 := l.Boxes[e.Target]
		if !ok1 || !ok2 {
			continue
		}
		color := edgeColor[e.Type]
		if color == "" {
			color = "#334155"
		}
		dash := ""
		width := "1.5"
		if e.Warning {
			color = warningColor
			dash = ` stroke-dasharray="7 4"`
			width = "2.5"
		}
		x1, y1, x2, y2 := src.X+src.W, src.cy(), dst.X, dst.cy()
		if e.Type == "destroy" {
			x1, y1, x2, y2 = src.cx(), src.Y+src.H, dst.cx(), dst.Y
		}
		fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s" stroke="%s" stroke-width="%s"%s/>`,
			fnum(x1), fnum(y1), fnum(x2), fnum(y2), color, width, dash)
		if e.Label != "" {
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="10" fill="%s">%s</text>`, fnum((x1+x2)/2), fnum((y1+y2)/2-4), color, esc(e.Label))
		}
	}

	for _, n := range l.Graph.Nodes {
		if n.Type == "org_unit" {
			continue
		}
		bx, ok := l.Boxes[n.ID]
		if !ok {
			continue
		}
		color := nodeColor[n.Type]
		if color == "" {
			color = "#334155"
		}
		fill := "#ffffff"
		stroke := color
		strokeW := "2"
		if n.Warning {
			fill = "#fef2f2"
			stroke = warningColor
		}
		if n.Type == "disposal" {
			strokeW = "1"
		}
		fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" rx="6" fill="%s" stroke="%s" stroke-width="%s"/>`,
			fnum(bx.X), fnum(bx.Y), fnum(bx.W), fnum(bx.H), fill, stroke, strokeW)
		lines := nodeLines(n)
		fs := 12.0
		if n.Type == "disposal" {
			fs = 10
		}
		for i, line := range lines {
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%s" text-anchor="middle" fill="#1e293b">%s</text>`,
				fnum(bx.cx()), fnum(bx.Y+fs+6+float64(i)*(fs+3)), fnum(fs), esc(line))
		}
	}

	renderLegend(&b, l.Height-legendH+20, l.Width)
	b.WriteString(`</svg>`)
	return []byte(b.String())
}

// nodeLines is the one or two lines of text shown inside a node box — label first, then whatever subtitle
// this type carries (role/status for an activity, recipient role for a recipient, the disposal method
// itself); long labels are not wrapped (an exported image is viewed zoomed, same as the on-screen diagram).
func nodeLines(n Node) []string {
	switch n.Type {
	case "activity":
		sub := n.ActivityCode
		if n.ActivityStatus != "" {
			sub += " · " + n.ActivityStatus
		}
		return []string{n.Label, sub}
	case "recipient":
		sub := n.RecipientRole
		if n.Warning {
			sub += " · missing DPA"
		}
		return []string{n.Label, sub}
	case "country":
		return []string{n.Label}
	case "disposal":
		return []string{n.Label}
	}
	return []string{n.Label}
}

func renderLegend(b *strings.Builder, y, width float64) {
	items := []struct{ color, label string }{
		{nodeColor["activity"], "Activity"}, {nodeColor["recipient"], "Recipient"},
		{nodeColor["country"], "Country"}, {nodeColor["disposal"], "Disposal"}, {warningColor, "Missing DPA / basis"},
	}
	x := marginX
	for _, it := range items {
		fmt.Fprintf(b, `<rect x="%s" y="%s" width="14" height="14" fill="none" stroke="%s" stroke-width="2"/>`, fnum(x), fnum(y), it.color)
		fmt.Fprintf(b, `<text x="%s" y="%s" font-size="11" fill="#334155">%s</text>`, fnum(x+20), fnum(y+11), esc(it.label))
		x += 170
	}
	_ = width
}

func fnum(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }
