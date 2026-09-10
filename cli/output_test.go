package cli

import (
	"bytes"
	"strings"
	"testing"
	"text/template"

	"github.com/Egor01KKK/threads-content-research-agent/threads"
)

func row() Row {
	p := &threads.Post{
		ID:        "42",
		Shortcode: "ABC",
		Text:      "hello world",
		Username:  "zuck",
		Permalink: "https://www.threads.com/@zuck/post/ABC",
		LikeCount: 568,
	}
	return postRow(p)
}

func render(t *testing.T, format Format, fields []string) string {
	t.Helper()
	var buf bytes.Buffer
	o := &Output{format: format, fields: fields, w: &buf}
	if err := o.Emit(row()); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if err := o.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return buf.String()
}

func TestEmitJSONL(t *testing.T) {
	got := render(t, FormatJSONL, nil)
	if !strings.Contains(got, `"username":"zuck"`) {
		t.Errorf("jsonl = %q", got)
	}
}

func TestEmitJSONArray(t *testing.T) {
	got := render(t, FormatJSON, nil)
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(strings.TrimSpace(got), "]") {
		t.Errorf("json not array-wrapped: %q", got)
	}
}

func TestEmitCSVHeader(t *testing.T) {
	got := render(t, FormatCSV, nil)
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if !strings.HasPrefix(lines[0], "id,username,timestamp,likes") {
		t.Errorf("csv header = %q", lines[0])
	}
	if !strings.Contains(lines[1], "zuck") {
		t.Errorf("csv row = %q", lines[1])
	}
}

func TestEmitURL(t *testing.T) {
	got := strings.TrimSpace(render(t, FormatURL, nil))
	if got != "https://www.threads.com/@zuck/post/ABC" {
		t.Errorf("url = %q", got)
	}
}

func TestEmitFieldsProjection(t *testing.T) {
	got := render(t, FormatCSV, []string{"username", "likes"})
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if lines[0] != "username,likes" {
		t.Errorf("projected header = %q", lines[0])
	}
	if lines[1] != "zuck,568" {
		t.Errorf("projected row = %q", lines[1])
	}
}

// --fields resolves the column alias (likes), the JSON key (like_count), and a
// JSON-only field with no column alias (user_id), all in one request.
func TestEmitFieldsVocabularies(t *testing.T) {
	got := render(t, FormatCSV, []string{"likes", "like_count", "user_id"})
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if lines[0] != "likes,like_count,user_id" {
		t.Errorf("header = %q", lines[0])
	}
	if lines[1] != "568,568," {
		t.Errorf("row = %q", lines[1])
	}
}

// A --template must reach a field by any of the three vocabularies th uses
// elsewhere: the Go struct field name, the JSON key, and the column alias.
func TestTemplateFieldVocabularies(t *testing.T) {
	cases := map[string]string{
		"struct field": "{{.LikeCount}}",
		"json key":     "{{.like_count}}",
		"column alias": "{{.likes}}",
	}
	for name, tmpl := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			o := &Output{
				format:   FormatRaw,
				template: template.Must(template.New("row").Parse(tmpl)),
				w:        &buf,
			}
			if err := o.Emit(row()); err != nil {
				t.Fatalf("emit: %v", err)
			}
			if strings.TrimSpace(buf.String()) != "568" {
				t.Errorf("%s template = %q, want 568", name, buf.String())
			}
		})
	}
}
