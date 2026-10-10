package readlater

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Tags that stay in an archived page. Everything else is unwrapped (its text
// stays) or, for the ones in dropTags, removed with its content.
var keepTags = map[atom.Atom]bool{
	atom.P: true, atom.Br: true, atom.Hr: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Ul: true, atom.Ol: true, atom.Li: true, atom.Dl: true, atom.Dt: true, atom.Dd: true,
	atom.Blockquote: true, atom.Pre: true, atom.Code: true, atom.Em: true, atom.Strong: true, atom.B: true, atom.I: true,
	atom.U: true, atom.S: true, atom.Del: true, atom.Ins: true, atom.Sub: true, atom.Sup: true, atom.Small: true, atom.Mark: true, atom.Kbd: true,
	atom.A: true, atom.Img: true, atom.Figure: true, atom.Figcaption: true,
	atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tfoot: true, atom.Tr: true, atom.Th: true, atom.Td: true, atom.Caption: true,
	atom.Div: true, atom.Span: true, atom.Section: true, atom.Article: true, atom.Details: true, atom.Summary: true,
}

var dropTags = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Iframe: true, atom.Object: true, atom.Embed: true, atom.Form: true, atom.Input: true,
	atom.Button: true, atom.Select: true, atom.Textarea: true, atom.Svg: true, atom.Math: true, atom.Noscript: true, atom.Template: true,
	atom.Video: true, atom.Audio: true, atom.Link: true, atom.Meta: true, atom.Head: true, atom.Title: true, atom.Canvas: true,
	atom.Source: true, atom.Track: true, atom.Applet: true, atom.Frame: true, atom.Frameset: true, atom.Base: true,
}

// sanitizer cleans the HTML of a saved page. Links and image addresses are made
// absolute against base. image is called for each picture; it returns the
// address to keep, or "" to drop the picture.
type sanitizer struct {
	base  *url.URL
	image func(abs string) string
	count int
}

// clean returns safe HTML for the fragment. Only the tags in keepTags survive,
// with a few attributes each. No script, style, form, frame or event handler
// remains, and only http, https and mailto links are kept.
func (s *sanitizer) clean(fragment string) string {
	nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return ""
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		s.copyInto(root, n)
	}
	var b strings.Builder
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&b, c); err != nil {
			return ""
		}
	}
	return b.String()
}

// copyInto adds a cleaned copy of n under parent.
func (s *sanitizer) copyInto(parent, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		parent.AppendChild(&html.Node{Type: html.TextNode, Data: n.Data})
	case html.ElementNode:
		if dropTags[n.DataAtom] {
			return
		}
		if !keepTags[n.DataAtom] {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				s.copyInto(parent, c)
			}
			return
		}
		out := &html.Node{Type: html.ElementNode, Data: n.Data, DataAtom: n.DataAtom}
		if !s.setAttrs(out, n) {
			return
		}
		parent.AppendChild(out)
		if n.DataAtom == atom.Img || n.DataAtom == atom.Br || n.DataAtom == atom.Hr {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			s.copyInto(out, c)
		}
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

// setAttrs copies the allowed attributes. It returns false when the element
// has to be dropped (a picture that cannot be kept).
func (s *sanitizer) setAttrs(out, in *html.Node) bool {
	set := func(k, v string) { out.Attr = append(out.Attr, html.Attribute{Key: k, Val: v}) }
	if cls := ownClasses(attr(in, "class")); cls != "" {
		set("class", cls)
	}
	switch in.DataAtom {
	case atom.A:
		if href := s.absolute(attr(in, "href"), true); href != "" {
			set("href", href)
			set("target", "_blank")
			set("rel", "noopener noreferrer nofollow")
		}
	case atom.Img:
		src := s.absolute(attr(in, "src"), false)
		if src == "" {
			return false
		}
		if s.image != nil {
			if src = s.image(src); src == "" {
				return false
			}
		}
		s.count++
		set("src", src)
		set("alt", attr(in, "alt"))
		set("loading", "lazy")
	case atom.Td, atom.Th:
		for _, k := range []string{"colspan", "rowspan"} {
			if v := attr(in, k); v != "" && len(v) <= 3 && strings.Trim(v, "0123456789") == "" {
				set(k, v)
			}
		}
	}
	return true
}

// absolute resolves a link against the page address. Only http and https are
// kept, and mailto when mail is true. Other schemes (javascript:, data:) give "".
func (s *sanitizer) absolute(raw string, mail bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if s.base != nil {
		u = s.base.ResolveReference(u)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return ""
		}
		return u.String()
	case "mailto":
		if mail {
			return u.String()
		}
	}
	return ""
}

// ownClasses keeps only the class names this app styles itself (xc-...). The
// classes of other sites mean nothing here.
func ownClasses(v string) string {
	var keep []string
	for _, c := range strings.Fields(v) {
		if strings.HasPrefix(c, "xc-") && strings.Trim(c, "abcdefghijklmnopqrstuvwxyz-") == "" {
			keep = append(keep, c)
		}
	}
	return strings.Join(keep, " ")
}
