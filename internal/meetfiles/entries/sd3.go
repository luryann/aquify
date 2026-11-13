package entries

import (
	"bytes"
	"fmt"
	"strings"
)

const sd3RecordWidth = 160

// SD3 renders the SDIF v3 entry file: A0 file header, B1 meet, C1 team, one
// D0 per individual entry (swimmers by last then first name, entries by event
// number) with the swimmer's D3 following their first D0, and a Z0 trailer.
func SD3(in Input) ([]byte, error) {
	p, err := prepare(in)
	if err != nil {
		return nil, err
	}
	return p.sd3(), nil
}

func (p *prepared) sd3() []byte {
	in := p.in
	var buf bytes.Buffer
	write := func(parts ...string) {
		line := strings.Join(parts, "")
		buf.WriteString(padRight(line, sd3RecordWidth))
		buf.WriteString("\r\n")
	}

	// A0: org 1 (USA Swimming), SDIF V3, file code 01 (meet registrations).
	write("A01V3      01",
		padRight("Meet Entries", 30),
		padRight(in.SoftwareName, 20),
		padRight(in.SoftwareVersion, 10),
		padRight(in.GeneratedBy, 20),
		strings.Repeat(" ", 12), // contact phone
		mmddyyyy(in.GeneratedAt),
	)

	// B1: the facility rides in the first address line; the rest of the
	// address block (address 2, city, state, postal code, country, meet code)
	// is left blank.
	write("B11        ",
		padRight(in.Meet.Name, 30),
		padRight(in.Meet.Facility, 22),
		strings.Repeat(" ", 58),
		mmddyyyy(in.Meet.Start),
		mmddyyyy(in.Meet.End),
		padLeft("0", 4), // altitude
		strings.Repeat(" ", 8),
		courseLetter(in.Meet.Course),
	)

	write("C11        ",
		padRight(strings.ToUpper(in.Club.LSC)+strings.ToUpper(in.Club.Code), 6),
		padRight(in.Club.Name, 30),
		strings.Repeat(" ", 16), // team abbreviation
		padRight(in.Club.Address1, 22),
		padRight(in.Club.Address2, 22),
		padRight(in.Club.City, 20),
		padRight(in.Club.State, 2),
		padRight(in.Club.PostalCode, 10),
	)

	dRecords := 0
	for _, sw := range p.byName {
		s := sw.swimmer
		name := s.LastName + ", " + s.FirstName
		if mi := strings.TrimSpace(s.MiddleInitial); mi != "" {
			name += " " + mi[:1]
		}
		id := strings.ToUpper(strings.TrimSpace(s.USSID))
		for i, e := range sw.entries {
			seed := "NT"
			if e.SeedTimeMs > 0 {
				seed = clockTime(e.SeedTimeMs)
			}
			write("D01        ",
				padRight(name, 28),
				padRight(id, 12),
				"A",   // attached
				"   ", // citizenship
				mmddyyyy(s.DOB),
				padRight(fmt.Sprintf("%d", ageOn(s.DOB, in.Meet.AgeUpDate)), 2), // left-justified, unlike HY3
				s.Gender,
				e.Event.Gender,
				padLeft(fmt.Sprintf("%d", e.Event.Distance), 4),
				fmt.Sprintf("%d", strokeDigit(e.Event.Stroke)),
				padLeft(e.Event.Number, 4),
				sdifAgeCode(e.Event.MinAge, e.Event.MaxAge),
				strings.Repeat(" ", 8), // date of swim
				padLeft(seed, 8),
				courseLetter(p.seedCourse(e)),
			)
			dRecords++
			if i == 0 {
				preferred := strings.TrimSpace(s.PreferredName)
				if preferred == "" {
					preferred = s.FirstName
				}
				// D3: full member ID, preferred first name, blank ethnicity
				// and nine "F" affiliation flags.
				write("D3", padRight(id, 14), padRight(preferred, 15), "  ", "FFFFFFFFF")
				dRecords++
			}
		}
	}

	// Z0 mirrors the reference file: the D count includes D3 records and the
	// swimmer count is one more than the number of athletes.
	write("Z01        01",
		strings.Repeat(" ", 30),
		padLeft("1", 3), // B records
		padLeft("1", 3), // meets
		padLeft("1", 4), // C records
		padLeft("1", 4), // teams
		padLeft(fmt.Sprintf("%d", dRecords), 6),
		padLeft(fmt.Sprintf("%d", len(p.byName)+1), 6),
		padLeft("0", 5), // E records
		padLeft("0", 6), // F records
		padLeft("0", 6), // G records
		padLeft("1", 5), // batch number
		padLeft("0", 3), // new members
		padLeft("0", 3), // renewals
		padLeft("0", 3), // changes
		padLeft("0", 3), // deletes
	)
	return buf.Bytes()
}

// sdifAgeCode is the four-character SDIF event age code: lower then upper
// bound, "UN" for no lower bound and "OV" for no upper bound.
func sdifAgeCode(minAge, maxAge *int) string {
	lower, upper := "UN", "OV"
	if minAge != nil && *minAge > 0 && *minAge < 100 {
		lower = fmt.Sprintf("%02d", *minAge)
	}
	if maxAge != nil && *maxAge > 0 && *maxAge < 100 {
		upper = fmt.Sprintf("%02d", *maxAge)
	}
	return lower + upper
}
