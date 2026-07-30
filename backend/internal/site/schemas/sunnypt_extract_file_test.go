package schemas

import (
	"os"
	"testing"
)

func TestSunnyExtractFromFile(t *testing.T) {
	b, err := os.ReadFile(`D:/PTPatronus/tmp/go_news_body.txt`)
	if err != nil {
		t.Skip(err)
	}
	c := sunnyExtractContentField(string(b))
	if c == "" {
		t.Fatal("empty extract")
	}
	items := sunnyParseNewsMD(c)
	if len(items) < 1 {
		t.Fatalf("items=%d content head=%q", len(items), c[:min(80, len(c))])
	}
	t.Logf("items=%d first=%s %s", len(items), items[0].Date, items[0].Title)
}
