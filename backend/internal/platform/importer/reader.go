package importer

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
)

// rowReader streams a file's rows; the first is the header.
type rowReader interface {
	Next() ([]string, error) // io.EOF at the end
	Close() error
}

// ErrUnreadable: the file isn't a CSV or .xlsx we can read.
var ErrUnreadable = errors.New("importer: file cannot be read as CSV or Excel")

// openRows reads CSV (UTF-8, with or without BOM — what Excel's "CSV UTF-8" writes; comma or semicolon)
// or .xlsx (first sheet, streamed). It spools the stream to a temp file because excelize needs random access.
func openRows(r io.Reader, fileName string) (rowReader, error) {
	tmp, err := os.CreateTemp("", "pdpa-import-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() { tmp.Close(); os.Remove(tmp.Name()) }
	if _, err := io.Copy(tmp, r); err != nil {
		cleanup()
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}

	if strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
		f, err := excelize.OpenReader(tmp)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
		}
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			f.Close()
			cleanup()
			return nil, ErrUnreadable
		}
		rows, err := f.Rows(sheets[0])
		if err != nil {
			f.Close()
			cleanup()
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
		}
		return &xlsxRows{f: f, rows: rows, cleanup: cleanup}, nil
	}

	br := bufio.NewReader(tmp)
	if bom, _ := br.Peek(3); bytes.Equal(bom, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	first, _ := br.Peek(4096)
	cr := csv.NewReader(br)
	if line, _, _ := bytes.Cut(first, []byte("\n")); bytes.Count(line, []byte(";")) > bytes.Count(line, []byte(",")) {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = false
	return &csvRows{r: cr, cleanup: cleanup}, nil
}

type csvRows struct {
	r       *csv.Reader
	cleanup func()
}

func (c *csvRows) Next() ([]string, error) {
	rec, err := c.r.Read()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return rec, err
}

func (c *csvRows) Close() error { c.cleanup(); return nil }

type xlsxRows struct {
	f       *excelize.File
	rows    *excelize.Rows
	cleanup func()
}

func (x *xlsxRows) Next() ([]string, error) {
	if !x.rows.Next() {
		if err := x.rows.Error(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
		}
		return nil, io.EOF
	}
	cols, err := x.rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	return cols, nil
}

func (x *xlsxRows) Close() error {
	x.rows.Close()
	x.f.Close()
	x.cleanup()
	return nil
}
