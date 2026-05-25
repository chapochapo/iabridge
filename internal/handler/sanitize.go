package handler

import (
	"html/template"
	"regexp"
)

// sanitizeHTML strips dangerous constructs from an HTML string received from
// archive.org metadata and returns a template.HTML value safe to render directly.
//
// It removes: <script>, <style>, <iframe>, <object>, <embed>, <form>, <base>,
// <link>, <meta> tags (and their content for script/style); event handler
// attributes (on*=…); and javascript: in href/src/action attributes.
//
// This is an allowlist-minus approach using regexp, adequate for trusted content
// from archive.org. It is not a full HTML parser — do not use for arbitrary input.
var (
	reScript    = regexp.MustCompile(`(?is)<script[^>]*>.*?</script\s*>`)
	reStyle     = regexp.MustCompile(`(?is)<style[^>]*>.*?</style\s*>`)
	reDanger    = regexp.MustCompile(`(?i)<\/?(iframe|object|embed|form|base|link|meta|input|button|textarea|select)(\s[^>]*)?>`)
	reEventAttr = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)
	reJSURL     = regexp.MustCompile(`(?i)(href|src|action)\s*=\s*(?:"javascript:[^"]*"|'javascript:[^']*')`)
)

func sanitizeHTML(s string) template.HTML {
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reDanger.ReplaceAllString(s, "")
	s = reEventAttr.ReplaceAllString(s, "")
	s = reJSURL.ReplaceAllString(s, `$1="#"`)
	return template.HTML(s)
}
