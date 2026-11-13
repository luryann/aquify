package entries

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A deliberately small PDF 1.3 writer: US Letter pages, the three standard
// Helvetica faces, text, hairlines and filled rectangles. Coordinates passed
// to it are in points from the top-left corner, like the layout code thinks;
// the writer flips them into PDF space.

const (
	pageWidth  = 612.0
	pageHeight = 792.0
)

type pdfFont int

const (
	fontRegular pdfFont = iota + 1 // F1 Helvetica
	fontBold                       // F2 Helvetica-Bold
	fontOblique                    // F3 Helvetica-Oblique
)

type rgb struct{ r, g, b float64 }

func gray(v float64) rgb { return rgb{v, v, v} }

type pdfDoc struct {
	pages []*bytes.Buffer
	cur   *bytes.Buffer
}

func (d *pdfDoc) addPage() {
	d.cur = &bytes.Buffer{}
	d.pages = append(d.pages, d.cur)
	d.cur.WriteString("0.2 w\n")
}

func (d *pdfDoc) page(i int) { d.cur = d.pages[i] }

func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func round3(v float64) float64 {
	return float64(int64(v*1000+0.5)) / 1000
}

func (c rgb) op(fill bool) string {
	if c.r == c.g && c.g == c.b {
		if fill {
			return num(c.r) + " g\n"
		}
		return num(c.r) + " G\n"
	}
	suffix := " RG\n"
	if fill {
		suffix = " rg\n"
	}
	return num(c.r) + " " + num(c.g) + " " + num(c.b) + suffix
}

// text draws s with its left edge at x and baseline at y.
func (d *pdfDoc) text(font pdfFont, size float64, color rgb, x, y float64, s string) {
	fmt.Fprintf(d.cur, "BT\n/F%d %s Tf\n%s%s %s Td\n(%s) Tj\nET\n",
		int(font), num(size), color.op(true), num(round3(x)), num(round3(pageHeight-y)), pdfEscape(winAnsi(s)))
}

func (d *pdfDoc) textRight(font pdfFont, size float64, color rgb, right, y float64, s string) {
	d.text(font, size, color, right-textWidth(font, size, s), y, s)
}

func (d *pdfDoc) textCenter(font pdfFont, size float64, color rgb, center, y float64, s string) {
	d.text(font, size, color, center-textWidth(font, size, s)/2, y, s)
}

func (d *pdfDoc) hline(color rgb, x1, x2, y float64) {
	fmt.Fprintf(d.cur, "%s%s %s m\n%s %s l\nS\n", color.op(false), num(x1), num(pageHeight-y), num(x2), num(pageHeight-y))
}

// rect fills the rectangle whose top-left corner is (x, y).
func (d *pdfDoc) rect(color rgb, x, y, w, h float64) {
	fmt.Fprintf(d.cur, "%s%s %s %s %s re\nf\n", color.op(true), num(x), num(pageHeight-y), num(w), num(-h))
}

// winAnsi maps a string to WinAnsi (CP-1252) bytes; unmappable runes become '?'.
func winAnsi(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 0x20 && r < 0x7f, r >= 0xa0 && r <= 0xff:
			out = append(out, byte(r))
		case r == '\t' || r == '\n' || r == '\r':
			out = append(out, ' ')
		default:
			if b, ok := cp1252[r]; ok {
				out = append(out, b)
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

var cp1252 = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87,
	'ˆ': 0x88, '‰': 0x89, 'Š': 0x8a, '‹': 0x8b, 'Œ': 0x8c, 'Ž': 0x8e,
	'‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97,
	'˜': 0x98, '™': 0x99, 'š': 0x9a, '›': 0x9b, 'œ': 0x9c, 'ž': 0x9e, 'Ÿ': 0x9f,
}

func pdfEscape(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c == '(' || c == ')' || c == '\\' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// textWidth measures s in points, including pair kerning, with the same
// metrics the reference reports were laid out with.
func textWidth(font pdfFont, size float64, s string) float64 {
	widths, kerning := &helveticaWidths, helveticaKerning
	if font == fontBold {
		widths, kerning = &helveticaBoldWidths, helveticaBoldKerning
	}
	b := winAnsi(s)
	units := 0
	for i, c := range b {
		if c >= 32 {
			units += int(widths[c-32])
		}
		if i > 0 {
			units += int(kerning[uint16(b[i-1])<<8|uint16(c)])
		}
	}
	return float64(units) * size / 1000
}

// bytes serialises the document.
func (d *pdfDoc) bytes(created time.Time) []byte {
	var out bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	out.WriteString("%PDF-1.3\n%\xba\xdf\xac\xe0\n")

	// Object numbers: 1 catalog, 2 page tree, 3-5 fonts, then a page object
	// and its content stream per page, then the info dictionary.
	const firstPageObj = 6
	kids := make([]string, len(d.pages))
	for i := range d.pages {
		kids[i] = fmt.Sprintf("%d 0 R", firstPageObj+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(d.pages)))
	for _, name := range []string{"Helvetica", "Helvetica-Bold", "Helvetica-Oblique"} {
		obj(fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding >>", name))
	}
	for i, pg := range d.pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >> /Contents %d 0 R >>",
			num(pageWidth), num(pageHeight), firstPageObj+2*i+1))
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", pg.Len(), pg.String()))
	}
	_, offset := created.Zone()
	sign := '+'
	if offset < 0 {
		sign, offset = '-', -offset
	}
	obj(fmt.Sprintf("<< /Producer (%s) /CreationDate (D:%s%c%02d'%02d') >>",
		DefaultSoftwareName, created.Format("20060102150405"), sign, offset/3600, offset%3600/60))

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, off := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, len(offsets), xref)
	return out.Bytes()
}
