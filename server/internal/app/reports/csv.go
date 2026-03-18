package reports

import (
	"bytes"
	"encoding/csv"
	"fmt"
)

type CSVDocument struct {
	Filename string
	Data     []byte
}

func BuildCSV(filename string, headers []string, rows [][]string) (*CSVDocument, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(headers); err != nil {
		return nil, fmt.Errorf("write csv headers: %w", err)
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("write csv row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}
	return &CSVDocument{Filename: filename, Data: buf.Bytes()}, nil
}
