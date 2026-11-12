package main

import (
	"image/color"
	"math"
	"sync"
)

// The label of a focused field and the label of an unfocused one have to be
// told apart by color, and no pair of palette colors does that in every
// theme: Primary against FgSecondary or FgMuted is under 1.5:1 in most of the
// bundled ones. labelColors derives the pair from the palette instead.

const (
	// labelFocusContrast is the WCAG contrast ratio sought between the label
	// of a focused field and the label of an unfocused one.
	labelFocusContrast = 3.0
	// labelMinContrast is the WCAG contrast ratio an unfocused label keeps
	// against the background.
	labelMinContrast = 3.0
	// labelSteps is how finely the derived colors are searched for.
	labelSteps = 50
)

// LabelFocused and LabelUnfocused are the label colors a theme sets itself
// (label_focused and label_unfocused). A nil one is derived by labelColors.
var LabelFocused, LabelUnfocused color.Color

var labelColorCache struct {
	sync.Mutex
	key                [6]uint64
	focused, unfocused color.Color
}

// labelColors returns the label colors for a field with focus and for one
// without, in the current theme.
func labelColors() (focused, unfocused color.Color) {
	if Primary == nil || BgBase == nil || FgPrimary == nil || FgMuted == nil {
		focused, unfocused = Primary, FgSecondary
		if LabelFocused != nil {
			focused = LabelFocused
		}
		if LabelUnfocused != nil {
			unfocused = LabelUnfocused
		}
		return focused, unfocused
	}
	key := [6]uint64{
		colorCacheKey(Primary), colorCacheKey(BgBase), colorCacheKey(FgPrimary),
		colorCacheKey(FgMuted), colorCacheKey(LabelFocused), colorCacheKey(LabelUnfocused),
	}
	labelColorCache.Lock()
	defer labelColorCache.Unlock()
	if labelColorCache.focused == nil || labelColorCache.key != key {
		labelColorCache.key = key
		labelColorCache.focused, labelColorCache.unfocused = deriveLabelColors()
	}
	return labelColorCache.focused, labelColorCache.unfocused
}

// deriveLabelColors works out the label colors from the palette, keeping the
// ones the theme set.
//
// The focused label is Primary and the unfocused one comes from
// unfocusedLabelFor. An unfocused label has to stay readable, which puts it
// at least labelMinContrast above the background, so Primary needs about
// labelMinContrast times labelFocusContrast against the background to leave
// room for it. Where Primary is darker than that, the focused label is
// lightened, at Primary's hue, only as far as it takes.
func deriveLabelColors() (focused, unfocused color.Color) {
	unfocusedFor := func(focused color.Color) color.Color {
		if LabelUnfocused != nil {
			return LabelUnfocused
		}
		return unfocusedLabelFor(focused)
	}
	if LabelFocused != nil {
		return LabelFocused, unfocusedFor(LabelFocused)
	}
	focused = Primary
	unfocused = unfocusedFor(focused)
	// Away from the background is toward white on a dark theme.
	lighter := contrastRatio(color.White, BgBase) >= contrastRatio(color.Black, BgBase)
	for step := 1; step <= labelSteps && contrastRatio(focused, unfocused) < labelFocusContrast; step++ {
		focused = shiftLightness(Primary, float64(step)/labelSteps, lighter)
		unfocused = unfocusedFor(focused)
	}
	return focused, unfocused
}

// unfocusedLabelFor returns the unfocused label color to go with focused.
//
// It blends FgMuted toward the background until the result contrasts with
// focused as much as it does with the background, and stops early if another
// step would leave it harder to read than labelMinContrast. A focused color
// that is itself close to the background leaves little room on that side;
// FgPrimary is then further from it and is used.
func unfocusedLabelFor(focused color.Color) color.Color {
	focus, background := relativeLuminance(focused), relativeLuminance(BgBase)
	dimmed := FgMuted
	for step := 1; step <= labelSteps; step++ {
		pastFocused := (relativeLuminance(dimmed) < focus) == (background < focus)
		if pastFocused && contrastRatio(dimmed, focused) >= contrastRatio(dimmed, BgBase) {
			break
		}
		next := mixColor(FgMuted, BgBase, float64(step)/labelSteps)
		if contrastRatio(next, BgBase) < labelMinContrast {
			break
		}
		dimmed = next
	}
	if contrastRatio(FgPrimary, focused) > contrastRatio(dimmed, focused) {
		return FgPrimary
	}
	return dimmed
}

// relativeLuminance returns the WCAG 2 relative luminance of c.
func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	channel := func(v uint32) float64 {
		s := float64(v>>8) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

// contrastRatio returns the WCAG 2 contrast ratio between a and b, from 1 to 21.
func contrastRatio(a, b color.Color) float64 {
	lighter, darker := relativeLuminance(a), relativeLuminance(b)
	if lighter < darker {
		lighter, darker = darker, lighter
	}
	return (lighter + 0.05) / (darker + 0.05)
}

// shiftLightness moves c the fraction t of the way to white, or to black when
// lighter is false, along OKLab lightness so that it keeps its hue. A plain
// blend with white would grey it. Chroma is given up only as far as it takes
// to stay inside sRGB.
func shiftLightness(c color.Color, t float64, lighter bool) color.Color {
	l, a, b := oklabFromColor(c)
	target := 0.0
	if lighter {
		target = 1
	}
	l += (target - l) * t
	const chromaSteps = 40
	for step := chromaSteps; ; step-- {
		chroma := float64(step) / chromaSteps
		red, green, blue := linearFromOKLab(l, a*chroma, b*chroma)
		inGamut := math.Min(red, math.Min(green, blue)) >= -1e-4 && math.Max(red, math.Max(green, blue)) <= 1+1e-4
		if inGamut || step == 0 {
			return color.RGBA{srgbEncode(red), srgbEncode(green), srgbEncode(blue), 255}
		}
	}
}

// oklabFromColor converts c to OKLab (Björn Ottosson, 2020).
func oklabFromColor(c color.Color) (l, a, b float64) {
	r16, g16, b16, _ := c.RGBA()
	linear := func(v uint32) float64 {
		s := float64(v>>8) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	red, green, blue := linear(r16), linear(g16), linear(b16)
	long := math.Cbrt(0.4122214708*red + 0.5363325363*green + 0.0514459929*blue)
	medium := math.Cbrt(0.2119034982*red + 0.6806995451*green + 0.1073969566*blue)
	short := math.Cbrt(0.0883024619*red + 0.2817188376*green + 0.6299787005*blue)
	return 0.2104542553*long + 0.7936177850*medium - 0.0040720468*short,
		1.9779984951*long - 2.4285922050*medium + 0.4505937099*short,
		0.0259040371*long + 0.7827717662*medium - 0.8086757660*short
}

// linearFromOKLab converts an OKLab color to linear sRGB, which is outside
// 0 to 1 when the color is outside the sRGB gamut.
func linearFromOKLab(l, a, b float64) (red, green, blue float64) {
	long := math.Pow(l+0.3963377774*a+0.2158037573*b, 3)
	medium := math.Pow(l-0.1055613458*a-0.0638541728*b, 3)
	short := math.Pow(l-0.0894841775*a-1.2914855480*b, 3)
	return 4.0767416621*long - 3.3077115913*medium + 0.2309699292*short,
		-1.2684380046*long + 2.6097574011*medium - 0.3413193965*short,
		-0.0041960863*long - 0.7034186147*medium + 1.7076147010*short
}

// srgbEncode converts a linear sRGB channel to its 8-bit encoded value.
func srgbEncode(linear float64) uint8 {
	linear = math.Max(0, math.Min(1, linear))
	encoded := 12.92 * linear
	if linear > 0.0031308 {
		encoded = 1.055*math.Pow(linear, 1/2.4) - 0.055
	}
	return uint8(encoded*255 + 0.5)
}
