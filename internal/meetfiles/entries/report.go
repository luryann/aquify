package entries

import (
	"fmt"
	"strings"
)

// Report geometry in points from the top-left of a US Letter page. The
// numbers reproduce the reference entry reports.
const (
	marginLeft   = 40.0
	marginRight  = 572.0
	columnOffset = 278.0 // second column starts this far right of the first
	timeRight    = 245.0 // right edge of the seed time within a column
	firstPageTop = 148.0
	nextPageTop  = 81.0
	flowBottom   = 731.0
	entryHeight  = 25.0
	headerHeight = 14.0
	groupGap     = 6.0
)

var (
	colorNavy      = rgb{0.196, 0.196, 0.49}
	colorTableHead = rgb{0.2, 0.2, 0.49}
	colorBlack     = gray(0)
	colorWhite     = gray(1)
	colorSubtitle  = gray(0.314)
	colorGenerated = gray(0.471)
	colorRule      = gray(0.71)
	colorFooter    = gray(0.549)
	colorRowShade  = gray(0.97)
	colorBrand     = rgb{0.922, 0.365, 0.118}
	summaryCenters = [4]float64{299.5, 354.5, 409.5, 464.5}
	// The reference sets the "Total" heading a third of a point left of its column.
	summaryHeadings = [4]float64{299.5, 354.5, 409.5, 464.14}
	summaryCostEdge = 566.0
)

// reportRow is one entry line: a label on the left, the seed time on the
// right and the seed source underneath.
type reportRow struct {
	label  string
	time   string
	source string
}

type reportGroup struct {
	title string
	rows  []reportRow
}

func (p *prepared) reportByEvent() []byte {
	groups := make([]reportGroup, 0, len(p.byEvent))
	for _, ev := range p.byEvent {
		g := reportGroup{title: eventLabel(ev.event)}
		for _, e := range ev.entries {
			g.rows = append(g.rows, p.reportRow(e.Swimmer.DisplayName(), e))
		}
		groups = append(groups, g)
	}
	return p.report(groups)
}

func (p *prepared) reportBySwimmer() []byte {
	groups := make([]reportGroup, 0, len(p.byReportName))
	for _, sw := range p.byReportName {
		g := reportGroup{title: sw.swimmer.DisplayName()}
		for _, e := range sw.entries {
			g.rows = append(g.rows, p.reportRow(eventLabel(e.Event), e))
		}
		groups = append(groups, g)
	}
	return p.report(groups)
}

func (p *prepared) reportRow(label string, e Entry) reportRow {
	row := reportRow{label: label, time: "NT", source: "Manually added"}
	if e.SeedTimeMs > 0 {
		row.time = clockTime(e.SeedTimeMs) + courseLetter(p.seedCourse(e))
	}
	if meet := e.SeedMeet; strings.TrimSpace(meet) != "" && e.SeedTimeMs > 0 {
		if r := []rune(meet); len(r) > 30 {
			meet = string(r[:30])
		}
		row.source = meet
		if !e.SeedDate.IsZero() {
			row.source += " - " + e.SeedDate.Format("01/02/2006")
		}
	}
	return row
}

// eventLabel is e.g. "#4 Mixed 8 & under 25 Free".
func eventLabel(ev Event) string {
	gender := "Mixed"
	switch ev.Gender {
	case "F":
		gender = "Female"
	case "M":
		gender = "Male"
	}
	hasMin := ev.MinAge != nil && *ev.MinAge > 0
	hasMax := ev.MaxAge != nil && *ev.MaxAge > 0 && *ev.MaxAge < 99
	age := "Open"
	switch {
	case hasMin && hasMax:
		age = fmt.Sprintf("%d-%d", *ev.MinAge, *ev.MaxAge)
	case hasMax:
		age = fmt.Sprintf("%d & under", *ev.MaxAge)
	case hasMin:
		age = fmt.Sprintf("%d & over", *ev.MinAge)
	}
	return fmt.Sprintf("#%s %s %s %d %s", ev.Number, gender, age, ev.Distance, strokeLabel(ev.Stroke))
}

// report lays groups out in two columns per page, adds the meet summary on
// its own page, then stamps every page with the footer.
func (p *prepared) report(groups []reportGroup) []byte {
	in := p.in
	doc := &pdfDoc{}
	doc.addPage()

	center := pageWidth / 2
	doc.textCenter(fontBold, 14, colorNavy, center, 72, in.Meet.Name)
	lsc, code := strings.ToUpper(strings.TrimSpace(in.Club.LSC)), strings.ToUpper(strings.TrimSpace(in.Club.Code))
	doc.textCenter(fontRegular, 9, colorSubtitle, center, 88, fmt.Sprintf("%s – %s  |  %s (%s)",
		in.Meet.Start.Format("01/02/2006"), in.Meet.End.Format("01/02/2006"), strings.TrimSpace(in.Club.Name), clubLabel(lsc, code)))
	doc.textCenter(fontRegular, 8, colorGenerated, center, 100, "Generated: "+in.GeneratedAt.Format("Jan 2, 2006"))
	doc.hline(colorRule, marginLeft, marginRight, 112)
	doc.textCenter(fontBold, 11, colorNavy, center, 126, "Meet Entries")
	doc.hline(colorRule, marginLeft, marginRight, 134)

	top, column, y := firstPageTop, 0, firstPageTop
	advance := func() {
		if column == 0 {
			column = 1
		} else {
			doc.addPage()
			column, top = 0, nextPageTop
		}
		y = top
	}
	for _, g := range groups {
		if y != top {
			y += groupGap
		}
		// Keep a group title with its first entry.
		if y+headerHeight+entryHeight > flowBottom {
			advance()
		}
		x := marginLeft + float64(column)*columnOffset
		doc.text(fontBold, 10, colorBlack, x, y, g.title)
		y += headerHeight
		for _, row := range g.rows {
			if y+entryHeight > flowBottom {
				advance()
			}
			x = marginLeft + float64(column)*columnOffset
			doc.text(fontRegular, 9, colorBlack, x+8, y, row.label)
			doc.textRight(fontRegular, 9, colorBlack, x+timeRight, y, row.time)
			doc.text(fontOblique, 7.5, colorSubtitle, x+16, y+11, row.source)
			y += entryHeight
		}
	}

	doc.addPage()
	p.summaryPage(doc)

	stamp := fmt.Sprintf("Generated: %s %d%s, %s", in.GeneratedAt.Format("Jan"), in.GeneratedAt.Day(),
		ordinalSuffix(in.GeneratedAt.Day()), in.GeneratedAt.Format("2006 3:04 PM"))
	for i := range doc.pages {
		doc.page(i)
		doc.textCenter(fontBold, 9, colorBrand, center, 760, DefaultSoftwareName)
		doc.text(fontRegular, 7, colorFooter, marginLeft, 766, stamp)
		doc.text(fontRegular, 7, colorFooter, marginLeft, 775, "By: "+strings.TrimSpace(in.GeneratedBy))
		doc.textRight(fontRegular, 8, colorFooter, marginRight, 772, fmt.Sprintf("Page %d of %d", i+1, len(doc.pages)))
	}
	return doc.bytes(in.GeneratedAt)
}

func ordinalSuffix(day int) string {
	if day%100 >= 11 && day%100 <= 13 {
		return "th"
	}
	switch day % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	}
	return "th"
}

// summary holds the meet summary table: counts per event gender (female,
// male, mixed) and costs in cents.
type summary struct {
	entries     [3]int
	athletes    [3]int
	athleteAll  int
	entryCost   int
	athleteCost int
	relayCost   int
}

func genderColumn(g string) int {
	switch g {
	case "F":
		return 0
	case "M":
		return 1
	}
	return 2
}

func (p *prepared) summary() summary {
	var s summary
	for _, ev := range p.byEvent {
		s.entries[genderColumn(ev.event.Gender)] += len(ev.entries)
		s.entryCost += len(ev.entries) * ev.event.FeeCents
	}
	// An athlete counts once under each event gender they are entered in.
	for _, sw := range p.byName {
		var in [3]bool
		for _, e := range sw.entries {
			in[genderColumn(e.Event.Gender)] = true
		}
		for i, ok := range in {
			if ok {
				s.athletes[i]++
			}
		}
	}
	s.athleteAll = len(p.byName)
	s.athleteCost = s.athleteAll * p.in.Meet.AthleteFeeCents
	s.relayCost = p.in.RelayTeams * p.in.Meet.RelayFeeCents
	return s
}

func dollars(cents int) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func (p *prepared) summaryPage(doc *pdfDoc) {
	s := p.summary()
	doc.text(fontBold, 11, colorNavy, marginLeft, 82, "Meet Summary")
	doc.rect(colorTableHead, marginLeft, 90, marginRight-marginLeft, 20)
	for i, label := range []string{"Female", "Male", "Mixed", "Total"} {
		doc.textCenter(fontBold, 9, colorWhite, summaryHeadings[i], 103, label)
	}
	doc.textRight(fontBold, 9, colorWhite, summaryCostEdge, 103, "Cost")

	type row struct {
		label  string
		counts *[4]int
		cost   int
	}
	total := func(c [3]int) int { return c[0] + c[1] + c[2] }
	rows := []row{
		{"Individual Entries", &[4]int{s.entries[0], s.entries[1], s.entries[2], total(s.entries)}, s.entryCost},
		{"Individual Athletes", &[4]int{s.athletes[0], s.athletes[1], s.athletes[2], s.athleteAll}, s.athleteCost},
		{"Relay Entries", &[4]int{0, 0, p.in.RelayTeams, p.in.RelayTeams}, s.relayCost},
		{"Total", nil, s.entryCost + s.athleteCost + s.relayCost},
	}
	for i, r := range rows {
		rowTop := 110 + 18*float64(i)
		if i%2 == 1 {
			doc.rect(colorRowShade, marginLeft, rowTop, marginRight-marginLeft, 18)
		}
		baseline := rowTop + 12
		doc.text(fontRegular, 9, colorBlack, marginLeft+6, baseline, r.label)
		if r.counts != nil {
			for c, n := range r.counts {
				doc.textCenter(fontRegular, 9, colorBlack, summaryCenters[c], baseline, fmt.Sprintf("%d", n))
			}
		}
		doc.textRight(fontRegular, 9, colorBlack, summaryCostEdge, baseline, dollars(r.cost))
	}
}
