package sparkline

import "strings"

const glyphs = "▁▂▃▄▅▆▇█"

func Render(values []int, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat("·", width)
	}
	sampled := sample(values, width)
	min, max := sampled[0], sampled[0]
	for _, value := range sampled[1:] {
		if value < min {
			min = value
		}
		if value > max {
			max = value
		}
	}
	out := strings.Builder{}
	scale := max - min
	for _, value := range sampled {
		index := 0
		if scale > 0 {
			if value == max {
				index = 7
			} else if value > min {
				index = 1 + (value-min)*5/scale
			}
		}
		out.WriteString(string([]rune(glyphs)[index]))
	}
	return out.String()
}

func sample(values []int, width int) []int {
	out := make([]int, width)
	for i := range width {
		index := i * len(values) / width
		out[i] = values[index]
	}
	return out
}
