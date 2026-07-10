package embedding

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

type Deterministic struct {
	dimension int
}

func NewDeterministic(dimension int) Deterministic {
	if dimension <= 0 {
		dimension = 256
	}
	return Deterministic{dimension: dimension}
}

func (p Deterministic) Code() string   { return "deterministic" }
func (p Deterministic) Dimension() int { return p.dimension }

func (p Deterministic) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, 0, len(texts))
	for _, text := range texts {
		vector := make([]float32, p.dimension)
		for _, token := range tokenize(text) {
			hasher := fnv.New64a()
			_, _ = hasher.Write([]byte(token))
			value := hasher.Sum64()
			index := int(value % uint64(p.dimension))
			sign := float32(1)
			if value&(1<<63) != 0 {
				sign = -1
			}
			vector[index] += sign
		}
		normalize(vector)
		result = append(result, vector)
	}
	return result, nil
}

func tokenize(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	tokens := make([]string, 0)
	word := strings.Builder{}
	flush := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	var previousHan rune
	for _, current := range value {
		if unicode.Is(unicode.Han, current) {
			flush()
			tokens = append(tokens, string(current))
			if previousHan != 0 {
				tokens = append(tokens, string([]rune{previousHan, current}))
			}
			previousHan = current
			continue
		}
		previousHan = 0
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			word.WriteRune(current)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func normalize(vector []float32) {
	sum := float64(0)
	for _, value := range vector {
		sum += float64(value * value)
	}
	if sum == 0 {
		return
	}
	length := float32(math.Sqrt(sum))
	for index := range vector {
		vector[index] /= length
	}
}
