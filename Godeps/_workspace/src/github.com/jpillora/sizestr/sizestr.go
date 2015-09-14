package sizestr

import (
	"math"
	"strconv"
	"strings"
)

//String representations of each scale
var ScaleStrings = []string{"B", "KB", "MB", "GB", "TB", "PB", "XB"}
var lowerCase = false

//Default scale, could also be set to 1024
var defaultScale = float64(1000)

//Default number of Significant Figures
var defaultSigFigures = float64(3) //must 10^SigFigures >= Scale

func setCase() {
	for i, s := range ScaleStrings {
		if lowerCase {
			ScaleStrings[i] = strings.ToLower(s)
		} else {
			ScaleStrings[i] = strings.ToUpper(s)
		}
	}
}

//ToggleCase changes the case of the scale strings ("MB" -> "mb")
func ToggleCase() {
	lowerCase = !lowerCase
	setCase()
}

func UpperCase() {
	lowerCase = false
	setCase()
}

func LowerCase() {
	lowerCase = false
	setCase()
}

//Converts a byte count into a byte string
func ToString(n int64) string {
	return ToStringSigScale(n, defaultSigFigures, defaultScale)
}

//Converts a byte count into a byte string
func ToStringSig(n int64, sig float64) string {
	return ToStringSigScale(n, sig, defaultScale)
}

//Converts a byte count into a byte string
func ToStringSigScale(n int64, sig, scale float64) string {

	var f = float64(n)

	var i int
	for i, _ = range ScaleStrings {
		if f < scale {
			break
		}
		f = f / scale
	}

	f = ToPrecision(f, sig)
	if f == scale {
		return strconv.FormatFloat(f/scale, 'f', 0, 64) + ScaleStrings[i+1]
	}
	return strconv.FormatFloat(f, 'f', -1, 64) + ScaleStrings[i]
}

var log10 = math.Log(10)

//A Go implementation of JavaScript's Math.toPrecision
func ToPrecision(n, p float64) float64 {
	//credits http://stackoverflow.com/a/12055126/977939
	if n == 0 {
		return 0
	}
	e := math.Floor(math.Log10(math.Abs(n)))
	f := round(math.Exp(math.Abs(e-p+1) * log10))
	if e-p+1 < 0 {
		return round(n*f) / f
	}
	return round(n/f) * f
}

func round(n float64) float64 {
	return math.Floor(n + 0.5)
}
