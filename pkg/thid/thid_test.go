package thid

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		in     string
		kind   Kind
		handle string
		code   string
		pk     string
		url    string
	}{
		{"@zuck", KindProfile, "zuck", "", "", "https://www.threads.com/@zuck"},
		{"zuck", KindProfile, "zuck", "", "", "https://www.threads.com/@zuck"},
		{"https://www.threads.com/@zuck", KindProfile, "zuck", "", "", "https://www.threads.com/@zuck"},
		{
			"https://www.threads.com/@zuck/post/C8abcde",
			KindPost, "zuck", "C8abcde", "", "https://www.threads.com/@zuck/post/C8abcde",
		},
		{
			// .net host is rewritten to .com.
			"https://www.threads.net/@zuck/post/C8abcde",
			KindPost, "zuck", "C8abcde", "", "https://www.threads.com/@zuck/post/C8abcde",
		},
		{"123456789012345", KindPost, "", "", "123456789012345", ""},
		{"", KindUnknown, "", "", "", ""},
	}
	for _, c := range cases {
		got := Classify(c.in)
		if got.Kind != c.kind {
			t.Errorf("Classify(%q).Kind = %q, want %q", c.in, got.Kind, c.kind)
		}
		if got.Handle != c.handle {
			t.Errorf("Classify(%q).Handle = %q, want %q", c.in, got.Handle, c.handle)
		}
		if got.Shortcode != c.code {
			t.Errorf("Classify(%q).Shortcode = %q, want %q", c.in, got.Shortcode, c.code)
		}
		if got.PK != c.pk {
			t.Errorf("Classify(%q).PK = %q, want %q", c.in, got.PK, c.pk)
		}
		if got.URL != c.url {
			t.Errorf("Classify(%q).URL = %q, want %q", c.in, got.URL, c.url)
		}
	}
}

func TestShortcodePKRoundTrip(t *testing.T) {
	// A handful of known pk values round-trip through the base64 codec.
	for _, pk := range []string{"1", "63", "64", "4096", "3141592653589793"} {
		code := PKToShortcode(pk)
		if code == "" {
			t.Fatalf("PKToShortcode(%q) empty", pk)
		}
		back := ShortcodeToPK(code)
		if back != pk {
			t.Errorf("round trip %q -> %q -> %q", pk, code, back)
		}
	}
}

func TestShortcodeToPKRejectsBadChars(t *testing.T) {
	if got := ShortcodeToPK("!!!"); got != "" {
		t.Errorf("ShortcodeToPK(bad) = %q, want empty", got)
	}
}
