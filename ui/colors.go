package ui

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	colorReset = "\033[0m"
	colorBold  = "\033[1m"
	colorDim   = "\033[2m"

	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorGray   = "\033[90m"

	colorBrightRed    = "\033[91m"
	colorBrightGreen  = "\033[92m"
	colorBrightYellow = "\033[93m"
	colorBrightBlue   = "\033[94m"
	colorBrightPurple = "\033[95m"
	colorBrightCyan   = "\033[96m"
	colorBrightWhite  = "\033[97m"
)

func Colorize(text, color string) string {
	if runtime.GOOS == "windows" {
		return text
	}

	return color + text + colorReset
}

func Red(text string) string    { return Colorize(text, colorRed) }
func Green(text string) string  { return Colorize(text, colorGreen) }
func Yellow(text string) string { return Colorize(text, colorYellow) }
func Blue(text string) string   { return Colorize(text, colorBlue) }
func Purple(text string) string { return Colorize(text, colorPurple) }
func Cyan(text string) string   { return Colorize(text, colorCyan) }
func White(text string) string  { return Colorize(text, colorWhite) }
func Gray(text string) string   { return Colorize(text, colorGray) }

func BrightRed(text string) string    { return Colorize(text, colorBrightRed) }
func BrightGreen(text string) string  { return Colorize(text, colorBrightGreen) }
func BrightYellow(text string) string { return Colorize(text, colorBrightYellow) }
func BrightBlue(text string) string   { return Colorize(text, colorBrightBlue) }
func BrightPurple(text string) string { return Colorize(text, colorBrightPurple) }
func BrightCyan(text string) string   { return Colorize(text, colorCyan) }
func BrightWhite(text string) string  { return Colorize(text, colorBrightWhite) }
func BrightGray(text string) string   { return Colorize(text, colorGray) }

func Bold(text string) string { return Colorize(text, colorBold) }
func Dim(text string) string  { return Colorize(text, colorDim) }

func Success(text string) string { return BrightGreen(text) }
func Error(text string) string   { return BrightRed(text) }
func Warning(text string) string { return BrightYellow(text) }
func Info(text string) string    { return BrightBlue(text) }
func Tool(text string) string    { return BrightCyan(text) }
func AI(text string) string      { return BrightPurple(text) }
func User(text string) string    { return BrightPurple(text) }

func Header(title string) string {
	title = strings.ToUpper(title)
	return BrightPurple(Bold(title))
}

func Progress(current, total int, description string) string {
	if total == 0 {
		return ""
	}

	percentage := float64(current) / float64(total) * 100
	barLength := 30
	filled := int(float64(barLength) * float64(current) / float64(total))

	bar := BrightGreen(strings.Repeat("█", filled)) +
		Dim(strings.Repeat("░", barLength-filled))

	return fmt.Sprintf("%s [%s] %.1f%% %s",
		BrightBlue("Progress:"), bar, percentage, description)
}
