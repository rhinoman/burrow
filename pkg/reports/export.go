package reports

import (
	"bytes"
	"fmt"
	"html"

	"github.com/yuin/goldmark"
)

// ExportHTML converts markdown to a self-contained HTML document.
func ExportHTML(markdown, title string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("converting markdown to HTML: %w", err)
	}

	escaped := html.EscapeString(title)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
  body { max-width: 48em; margin: 2em auto; padding: 0 1em; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; line-height: 1.6; color: #1a1a1a; }
  h1, h2, h3 { margin-top: 1.5em; }
  code { background: #f4f4f4; padding: 0.15em 0.3em; border-radius: 3px; font-size: 0.9em; }
  pre { background: #f4f4f4; padding: 1em; border-radius: 4px; overflow-x: auto; }
  pre code { background: none; padding: 0; }
  blockquote { border-left: 3px solid #ccc; margin-left: 0; padding-left: 1em; color: #555; }
  table { border-collapse: collapse; width: 100%%; }
  th, td { border: 1px solid #ddd; padding: 0.5em; text-align: left; }
  th { background: #f4f4f4; }
</style>
</head>
<body>
%s
</body>
</html>`, escaped, buf.String()), nil
}
