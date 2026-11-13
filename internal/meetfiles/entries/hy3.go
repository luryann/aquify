package entries

import (
	"bytes"
	"fmt"
	"strings"
)

const hy3DataWidth = 128

// hy3Vendor is the vendor tag meet software expects in the A1 record of a
// Hy-Tek format file; the producing software is named in the next field.
const hy3Vendor = "Hy-Tek, Ltd"

// HY3 renders the Hy-Tek entry file: A1 file header, B1/B2 meet, C1-C3 team,
// then one D1 per athlete (numbered in order of first appearance by event)
// followed by an E1 for each of their entries in event order. Every line is
// 128 data columns plus a two-digit checksum.
func HY3(in Input) ([]byte, error) {
	p, err := prepare(in)
	if err != nil {
		return nil, err
	}
	return p.hy3(), nil
}

func (p *prepared) hy3() []byte {
	in := p.in
	var buf bytes.Buffer
	write := func(parts ...string) {
		line := padRight(strings.Join(parts, ""), hy3DataWidth)
		buf.WriteString(line)
		buf.WriteString(HY3Checksum(line))
		buf.WriteString("\r\n")
	}

	// A1: file type 02 = meet entries.
	write("A102",
		padRight("Meet Entries", 25),
		padRight(hy3Vendor, 15),
		padRight(in.SoftwareName+" "+in.SoftwareVersion, 14),
		mmddyyyy(in.GeneratedAt),
		" ",
		padLeft(in.GeneratedAt.Format("3:04 PM"), 8),
		padRight(in.Club.Name, 53),
	)
	write("B1",
		padRight(in.Meet.Name, 45),
		padRight(in.Meet.Facility, 45),
		mmddyyyy(in.Meet.Start),
		mmddyyyy(in.Meet.End),
		mmddyyyy(in.Meet.AgeUpDate),
		padLeft("0", 4), // elevation
	)
	write("B2",
		strings.Repeat(" ", 96),
		courseLetter(in.Meet.Course),
		padLeft("0.00", 7),
	)
	write("C1",
		padRight(strings.ToUpper(in.Club.Code), 5),
		padRight(in.Club.Name, 30),
		padRight(in.Club.Name, 16), // short name
		padRight(strings.ToUpper(in.Club.LSC), 2),
	)
	country := strings.TrimSpace(in.Club.Country)
	if country == "" {
		country = "USA"
	}
	write("C2",
		padRight(in.Club.Address1, 30),
		padRight(in.Club.Address2, 30),
		padRight(in.Club.City, 30),
		padRight(in.Club.State, 2),
		padRight(in.Club.PostalCode, 10),
		padRight(country, 3),
	)
	write("C3")

	for i, sw := range p.byAppearance {
		s := sw.swimmer
		number := padLeft(fmt.Sprintf("%d", i+1), 5)
		middle := " "
		if mi := strings.TrimSpace(s.MiddleInitial); mi != "" {
			middle = mi[:1]
		}
		write("D1", s.Gender, number,
			padRight(s.LastName, 20),
			padRight(s.FirstName, 20),
			padRight(s.PreferredName, 20),
			padRight(middle, 1),
			padRight(strings.ToUpper(strings.TrimSpace(s.USSID)), 14),
			strings.Repeat(" ", 5),
			mmddyyyy(s.DOB),
			" ",
			padLeft(fmt.Sprintf("%d", ageOn(s.DOB, in.Meet.AgeUpDate)), 2),
			padLeft("0", 6),
			strings.Repeat(" ", 19),
			"N",
		)
		for _, e := range sw.entries {
			n, suffix, _ := splitEventNumber(e.Event.Number)
			seed := fmt.Sprintf("%.2f", float64(e.SeedTimeMs/10)/100)
			course := courseLetter(p.seedCourse(e))
			write("E1", s.Gender, number,
				padRight(s.LastName, 5),
				s.Gender,
				e.Event.Gender,
				padLeft(fmt.Sprintf("%d", e.Event.Distance), 6),
				strokeLetter(e.Event.Stroke),
				padLeft(fmt.Sprintf("%d", hy3MinAge(e.Event.MinAge)), 3),
				padLeft(fmt.Sprintf("%d", hy3MaxAge(e.Event.MaxAge)), 3),
				"  0S",
				padLeft(fmt.Sprintf("%.2f", float64(e.Event.FeeCents)/100), 6),
				padLeft(fmt.Sprintf("%d", n), 3),
				padRight(suffix, 1),
				padLeft(seed, 8), course,
				padLeft(seed, 8), course,
				padLeft("0.00", 8),
				padLeft("0.00", 8),
				"   NN",
				strings.Repeat(" ", 15),
				"N",
			)
		}
	}
	return buf.Bytes()
}

// hy3OpenMaxAge is how Hy-Tek writes "no upper age limit".
const hy3OpenMaxAge = 109

func hy3MinAge(minAge *int) int {
	if minAge == nil || *minAge < 0 {
		return 0
	}
	return *minAge
}

func hy3MaxAge(maxAge *int) int {
	if maxAge == nil || *maxAge <= 0 || *maxAge >= 99 {
		return hy3OpenMaxAge
	}
	return *maxAge
}

// HY3Checksum computes the two checksum digits that close every HY3 line:
// characters in odd columns count once and even columns twice; the sum is
// divided by 21, 205 is added, and the last two digits are written in
// reverse order.
func HY3Checksum(line string) string {
	sum := 0
	for i := 0; i < hy3DataWidth && i < len(line); i++ {
		if i%2 == 0 {
			sum += int(line[i])
		} else {
			sum += 2 * int(line[i])
		}
	}
	v := sum/21 + 205
	return fmt.Sprintf("%d%d", v%10, (v/10)%10)
}
