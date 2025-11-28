package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/olebedev/when"
	"github.com/olebedev/when/rules/common"
	"github.com/olebedev/when/rules/en"
)

var w *when.Parser

func InitNLP() {
	fmt.Println("[NLP] Initializing Rules...")
	w = when.New(nil)
	w.Add(en.All...)
	w.Add(common.All...)
}

func getMonthWindow(t time.Time) (int64, int64) {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	end := start.AddDate(0, 1, 0).Add(-1 * time.Second)
	return start.Unix(), end.Unix()
}

// NEW: Manual Regex Helpers
func manualFallback(query string) (string, int64, int64) {
	lower := strings.ToLower(query)
	now := time.Now()

	// 1. "Last Month"
	if strings.Contains(lower, "last month") {
		// Go back 1 month
		lastMonth := now.AddDate(0, -1, 0)
		start, end := getMonthWindow(lastMonth)

		// Remove "last month" and "from" if present
		clean := strings.ReplaceAll(lower, "last month", "")
		clean = strings.ReplaceAll(clean, "from", "")
		return strings.TrimSpace(clean), start, end
	}

	// 2. "Last Week"
	if strings.Contains(lower, "last week") {
		// Go back 7 days from now
		// Or strictly: Previous Monday to Sunday
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}

		// Start of THIS week
		startOfThisWeek := now.AddDate(0, 0, -1*(weekday-1))
		// Start of LAST week = Start of This Week - 7 days
		startOfLastWeek := startOfThisWeek.AddDate(0, 0, -7)
		start := time.Date(startOfLastWeek.Year(), startOfLastWeek.Month(), startOfLastWeek.Day(), 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 0, 7).Add(-1 * time.Second)

		clean := strings.ReplaceAll(lower, "last week", "")
		clean = strings.ReplaceAll(clean, "from", "")
		return strings.TrimSpace(clean), start.Unix(), end.Unix()
	}

	// 3. "Yesterday"
	if strings.Contains(lower, "yesterday") {
		yesterday := now.AddDate(0, 0, -1)
		y, m, d := yesterday.Date()
		start := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 0, 1).Add(-1 * time.Second)

		clean := strings.ReplaceAll(lower, "yesterday", "")
		clean = strings.ReplaceAll(clean, "from", "")
		return strings.TrimSpace(clean), start.Unix(), end.Unix()
	}

	return query, 0, 0
}

func ParseDateQuery(query string) (string, int64, int64) {
	if w == nil {
		InitNLP()
	}

	// 1. Try Library First
	res, err := w.Parse(query, time.Now())

	// 2. IF LIBRARY FAILS -> USE MANUAL FALLBACK
	if err != nil || res == nil {
		fmt.Printf("[NLP Debug] Library missed it. Trying Manual Fallback for: '%s'\n", query)
		return manualFallback(query)
	}

	fmt.Printf("[NLP Debug] Library Found: '%s'\n", res.Text)

	// Clean Query Logic (Library Success)
	startIdx := res.Index
	endIdx := res.Index + len(res.Text)
	cleanQuery := strings.TrimSpace(query[:startIdx] + query[endIdx:])

	foundText := strings.ToLower(res.Text)
	target := res.Time
	var minTime, maxTime int64

	// Logic for Library Results
	if strings.Contains(foundText, "month") ||
		strings.Contains(foundText, "jan") || strings.Contains(foundText, "feb") ||
		strings.Contains(foundText, "mar") || strings.Contains(foundText, "apr") ||
		strings.Contains(foundText, "may") || strings.Contains(foundText, "jun") ||
		strings.Contains(foundText, "jul") || strings.Contains(foundText, "aug") ||
		strings.Contains(foundText, "sep") || strings.Contains(foundText, "oct") ||
		strings.Contains(foundText, "nov") || strings.Contains(foundText, "dec") {
		minTime, maxTime = getMonthWindow(target)
	} else if strings.Contains(foundText, "week") {
		weekday := int(target.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startOfWeek := target.AddDate(0, 0, -1*(weekday-1))
		start := time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, startOfWeek.Location())
		end := start.AddDate(0, 0, 7).Add(-1 * time.Second)
		minTime = start.Unix()
		maxTime = end.Unix()
	} else if strings.Contains(foundText, "year") || strings.Contains(foundText, "202") {
		start := time.Date(target.Year(), 1, 1, 0, 0, 0, 0, target.Location())
		end := start.AddDate(1, 0, 0).Add(-1 * time.Second)
		minTime = start.Unix()
		maxTime = end.Unix()
	} else {
		// Default 24h
		y, m, d := target.Date()
		start := time.Date(y, m, d, 0, 0, 0, 0, target.Location())
		end := start.AddDate(0, 0, 1).Add(-1 * time.Second)
		minTime = start.Unix()
		maxTime = end.Unix()
	}

	return cleanQuery, minTime, maxTime
}
