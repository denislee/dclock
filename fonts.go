package main

import (
	"log"
	"os"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
)

// FontOption represents a selectable font family.
type FontOption struct {
	Name     string
	Typeface font.Typeface
}

var (
	fontCollection []font.FontFace
	fontOptions    []FontOption
)

// systemFontProbes are well-known Linux font paths. The first existing path
// for a given name is loaded; on success it becomes a selectable option.
var systemFontProbes = []struct {
	name  string
	paths []string
}{
	{"DejaVu Sans", []string{
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	}},
	{"DejaVu Sans Mono", []string{
		"/usr/share/fonts/TTF/DejaVuSansMono.ttf",
		"/usr/share/fonts/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
	}},
	{"DejaVu Serif", []string{
		"/usr/share/fonts/TTF/DejaVuSerif.ttf",
		"/usr/share/fonts/dejavu/DejaVuSerif.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSerif.ttf",
	}},
	{"Liberation Sans", []string{
		"/usr/share/fonts/TTF/LiberationSans-Regular.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
	}},
	{"Liberation Mono", []string{
		"/usr/share/fonts/TTF/LiberationMono-Regular.ttf",
		"/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
	}},
	{"Noto Sans", []string{
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",
		"/usr/share/fonts/TTF/NotoSans-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
	}},
	{"Noto Mono", []string{
		"/usr/share/fonts/noto/NotoMono-Regular.ttf",
		"/usr/share/fonts/TTF/NotoMono-Regular.ttf",
		"/usr/share/fonts/truetype/noto/NotoMono-Regular.ttf",
	}},
	{"JetBrains Mono", []string{
		"/usr/share/fonts/JetBrainsMono/JetBrainsMono-Regular.ttf",
		"/usr/share/fonts/TTF/JetBrainsMono-Regular.ttf",
		"/usr/share/fonts/truetype/jetbrains-mono/JetBrainsMono-Regular.ttf",
	}},
	{"Source Code Pro", []string{
		"/usr/share/fonts/adobe-source-code-pro/SourceCodePro-Regular.otf",
		"/usr/share/fonts/OTF/SourceCodePro-Regular.otf",
		"/usr/share/fonts/truetype/source-code-pro/SourceCodePro-Regular.ttf",
	}},
	{"Fira Sans", []string{
		"/usr/share/fonts/fira-sans/FiraSans-Regular.otf",
		"/usr/share/fonts/OTF/FiraSans-Regular.otf",
		"/usr/share/fonts/truetype/fira-sans/FiraSans-Regular.ttf",
	}},
	{"Fira Mono", []string{
		"/usr/share/fonts/fira-mono/FiraMono-Regular.otf",
		"/usr/share/fonts/OTF/FiraMono-Regular.otf",
		"/usr/share/fonts/truetype/fira-mono/FiraMono-Regular.ttf",
	}},
}

// InitFonts builds the global font collection (always includes Go faces, plus
// whichever system fonts are present) and the dropdown option list.
func InitFonts() {
	fontCollection = append(fontCollection, gofont.Collection()...)
	fontOptions = []FontOption{
		{"Go", "Go"},
		{"Go Mono", "Go Mono"},
		{"Go Smallcaps", "Go Smallcaps"},
	}
	for _, probe := range systemFontProbes {
		for _, p := range probe.paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			face, err := opentype.Parse(data)
			if err != nil {
				log.Printf("font %s: parse %s: %v", probe.name, p, err)
				continue
			}
			tf := font.Typeface(probe.name)
			fontCollection = append(fontCollection, font.FontFace{
				Font: font.Font{Typeface: tf},
				Face: face,
			})
			fontOptions = append(fontOptions, FontOption{Name: probe.name, Typeface: tf})
			break
		}
	}
}

// ResolveFont returns the typeface for the configured face name, falling back
// to the first option if the name isn't known.
func ResolveFont(name string) font.Typeface {
	for _, o := range fontOptions {
		if equalFold(o.Name, name) {
			return o.Typeface
		}
	}
	if len(fontOptions) > 0 {
		return fontOptions[0].Typeface
	}
	return "Go"
}

// FontIndex returns the index of name in fontOptions, or -1.
func FontIndex(name string) int {
	for i, o := range fontOptions {
		if equalFold(o.Name, name) {
			return i
		}
	}
	return -1
}
