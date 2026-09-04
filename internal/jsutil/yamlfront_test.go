package jsutil

import (
	"encoding/json"
	"os"
	"testing"
)

type frontMatterCase struct {
	Input    string  `json:"input"`
	Expected *string `json:"expected"`
	Error    *string `json:"error"`
}

func TestLoadFrontMatchesYamlFrontMatter(t *testing.T) {
	raw, err := os.ReadFile("testdata/yamlfront.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var cases []frontMatterCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("golden file is empty")
	}
	for _, c := range cases {
		t.Run(c.Input, func(t *testing.T) {
			got, err := LoadFront(c.Input)
			if c.Expected == nil {
				if err == nil {
					t.Fatalf("expected an error (node said %q), got %s", *c.Error, StringifyOrEmpty(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFront(%q) failed: %v", c.Input, err)
			}
			if s := StringifyOrEmpty(got); s != *c.Expected {
				t.Errorf("LoadFront(%q)\n got: %s\nwant: %s", c.Input, s, *c.Expected)
			}
		})
	}
}
