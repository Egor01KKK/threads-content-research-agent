package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/mattn/go-isatty"
)

// Format is an output encoding.
type Format string

const (
	FormatAuto  Format = "auto"
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatJSONL Format = "jsonl"
	FormatCSV   Format = "csv"
	FormatTSV   Format = "tsv"
	FormatYAML  Format = "yaml"
	FormatURL   Format = "url"
	FormatRaw   Format = "raw"
)

// Row is one output record: an ordered set of named columns plus the original
// value (used by json/jsonl/yaml and templates).
type Row struct {
	Cols  []string
	Vals  []string
	Value any
}

// Output renders rows in the selected format, incrementally for streaming.
type Output struct {
	format   Format
	fields   []string
	noHeader bool
	template *template.Template
	w        io.Writer

	tw         *tabwriter.Writer
	csvw       *csv.Writer
	headerDone bool
	jsonFirst  bool
	jsonOpen   bool
}

func newOutput(g *globalFlags) *Output {
	o := &Output{w: os.Stdout, noHeader: g.noHeader}
	o.format = resolveFormat(g.output)
	if g.fields != "" {
		o.fields = splitComma(g.fields)
	}
	if g.template != "" {
		o.template = template.Must(template.New("row").Parse(g.template + "\n"))
		o.format = FormatRaw
	}
	return o
}

func resolveFormat(s string) Format {
	switch Format(s) {
	case FormatAuto, "":
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return FormatTable
		}
		return FormatJSONL
	default:
		return Format(s)
	}
}

// Format returns the resolved output format.
func (o *Output) Format() Format { return o.format }

// Emit renders one row.
func (o *Output) Emit(r Row) error {
	cols, vals := o.project(r)
	switch o.format {
	case FormatTable:
		return o.emitTable(cols, vals)
	case FormatCSV, FormatTSV:
		return o.emitCSV(cols, vals)
	case FormatJSONL:
		return o.emitJSONL(r.Value)
	case FormatJSON:
		return o.emitJSON(r.Value)
	case FormatYAML:
		return o.emitYAML(cols, vals)
	case FormatURL:
		return o.emitField(r, "url")
	case FormatRaw:
		if o.template != nil {
			return o.template.Execute(o.w, templateData(r))
		}
		return o.emitField(r, "")
	default:
		return o.emitJSONL(r.Value)
	}
}

func (o *Output) project(r Row) (cols, vals []string) {
	if len(o.fields) == 0 {
		return r.Cols, r.Vals
	}
	idx := map[string]int{}
	for i, c := range r.Cols {
		idx[c] = i
	}
	// A requested field resolves first against the column aliases, then against
	// the JSON keys and Go struct field names, so --fields and --template share
	// one vocabulary and can also reach fields that have no column alias (such as
	// user_id or quote_count).
	var data map[string]any
	for _, f := range o.fields {
		cols = append(cols, f)
		if i, ok := idx[f]; ok && i < len(r.Vals) {
			vals = append(vals, r.Vals[i])
			continue
		}
		if data == nil {
			if m, ok := templateData(r).(map[string]any); ok {
				data = m
			} else {
				data = map[string]any{}
			}
		}
		vals = append(vals, cell(data[f]))
	}
	return cols, vals
}

// cell renders a value pulled from the merged field map as a flat string, with
// whole-number floats (the shape JSON gives every number) printed without a
// trailing ".0".
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if x == math.Trunc(x) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func (o *Output) emitTable(cols, vals []string) error {
	if o.tw == nil {
		o.tw = tabwriter.NewWriter(o.w, 0, 0, 2, ' ', 0)
	}
	if !o.headerDone && !o.noHeader {
		if _, err := fmt.Fprintln(o.tw, strings.Join(upperAll(cols), "\t")); err != nil {
			return err
		}
		o.headerDone = true
	}
	clean := make([]string, len(vals))
	for i, v := range vals {
		clean[i] = strings.ReplaceAll(v, "\t", " ")
	}
	_, err := fmt.Fprintln(o.tw, strings.Join(clean, "\t"))
	return err
}

func (o *Output) emitCSV(cols, vals []string) error {
	if o.csvw == nil {
		o.csvw = csv.NewWriter(o.w)
		if o.format == FormatTSV {
			o.csvw.Comma = '\t'
		}
	}
	if !o.headerDone && !o.noHeader {
		if err := o.csvw.Write(cols); err != nil {
			return err
		}
		o.headerDone = true
	}
	return o.csvw.Write(vals)
}

func (o *Output) emitJSONL(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(o.w, string(b))
	return err
}

func (o *Output) emitJSON(v any) error {
	if !o.jsonOpen {
		if _, err := fmt.Fprint(o.w, "["); err != nil {
			return err
		}
		o.jsonOpen = true
		o.jsonFirst = true
	}
	if !o.jsonFirst {
		if _, err := fmt.Fprint(o.w, ","); err != nil {
			return err
		}
	}
	o.jsonFirst = false
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(o.w, "\n  "+string(b))
	return err
}

func (o *Output) emitYAML(cols, vals []string) error {
	if _, err := fmt.Fprintln(o.w, "-"); err != nil {
		return err
	}
	for i, c := range cols {
		v := ""
		if i < len(vals) {
			v = vals[i]
		}
		if _, err := fmt.Fprintf(o.w, "  %s: %q\n", c, v); err != nil {
			return err
		}
	}
	return nil
}

func (o *Output) emitField(r Row, field string) error {
	if field == "" && len(r.Vals) > 0 {
		_, err := fmt.Fprintln(o.w, r.Vals[0])
		return err
	}
	// The url format accepts any of the URL-bearing columns, in preference order.
	candidates := []string{field}
	if field == "url" {
		candidates = []string{"url", "permalink", "canonical_url"}
	}
	for _, want := range candidates {
		for i, c := range r.Cols {
			if c == want && i < len(r.Vals) && r.Vals[i] != "" {
				_, err := fmt.Fprintln(o.w, r.Vals[i])
				return err
			}
		}
	}
	return nil
}

// Flush finalises buffered formats. Call once at the end of a command.
func (o *Output) Flush() error {
	if o.tw != nil {
		return o.tw.Flush()
	}
	if o.csvw != nil {
		o.csvw.Flush()
		return o.csvw.Error()
	}
	if o.jsonOpen {
		_, err := fmt.Fprintln(o.w, "\n]")
		return err
	}
	return nil
}

// Raw writes bytes straight to stdout (for --raw content).
func (o *Output) Raw(b []byte) error {
	_, err := o.w.Write(b)
	return err
}

// templateData builds the value a --template executes against. It merges three
// naming vocabularies so a template can reach a field by whichever name the user
// reaches for: the Go struct field names (e.g. {{.LikeCount}}), the JSON keys
// (e.g. {{.like_count}}), and the friendly column aliases shown by --fields and
// the table header (e.g. {{.likes}}). JSON keys keep their native types so
// template arithmetic on numbers still works.
func templateData(r Row) any {
	m := map[string]any{}

	// Go struct field names, via reflection over the original value.
	rv := reflect.ValueOf(r.Value)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			if f := rt.Field(i); f.IsExported() {
				m[f.Name] = rv.Field(i).Interface()
			}
		}
	}

	// JSON keys (id, like_count, permalink, ...), with native types preserved.
	if b, err := json.Marshal(r.Value); err == nil {
		var jm map[string]any
		if json.Unmarshal(b, &jm) == nil {
			maps.Copy(m, jm)
		}
	}

	// Friendly column aliases (likes, replies, media, ...) as rendered strings.
	for i, c := range r.Cols {
		if i < len(r.Vals) {
			m[c] = r.Vals[i]
		}
	}

	if len(m) == 0 {
		return r.Value
	}
	return m
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func upperAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToUpper(s)
	}
	return out
}
