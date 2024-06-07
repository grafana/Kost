package utils

import (
	"math"
)

func BytesToGiB(bytes int64) float64 {
	gb := float64(bytes) / (math.Pow(2, 30))
	return gb
}
