package web

import (
	"embed"
	"html/template"
	"regexp"
	"strings"
)

//go:embed icons/*.svg
var iconFS embed.FS

// stripped of the license comment and given a uniform class so CSS can size it.
var iconCache = map[string]template.HTML{}

var licenseComment = regexp.MustCompile(`(?s)<!--.*?-->`)

func loadIcons() error {
	entries, err := iconFS.ReadDir("icons")
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".svg")
		b, err := iconFS.ReadFile("icons/" + e.Name())
		if err != nil {
			return err
		}
		svg := licenseComment.ReplaceAllString(string(b), "")
		svg = strings.TrimSpace(svg)
		// Tag with a class hook; icons inherit color via stroke="currentColor".
		svg = strings.Replace(svg, "<svg", `<svg class="icon"`, 1)
		iconCache[name] = template.HTML(svg) //nolint:gosec // trusted local SVG assets
	}
	return nil
}

// iconHTML returns the inline SVG for a Lucide icon name, or empty if unknown.
func iconHTML(name string) template.HTML {
	return iconCache[name]
}
