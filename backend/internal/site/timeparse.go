package site

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var absTimeFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-0215:04:05",
	"2006-01-0215:04:05 -07:00",
	"2006-01-02 15:04 -0700",
	"2006-01-02 15:04 -07:00",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02 15:04:05",
	"2006/01/02",
	"02/01/2006 15:04:05",
	"02/01/2006",
	"2006年1月2日 15:04:05",
	"02-01-2006, 15:04:05",
	"02-01-2006 15:04:05",
	"02_01_2006",
	"2006.01.02",
	time.RFC3339,
	"January 02, 2006, 15:04:05",
	"January 2, 2006, 15:04:05",
	"January 02, 2006, 15:04",
	"January 2, 2006, 15:04",
	"Jan 02 2006, 15:04",
	"Jan 2 2006, 15:04",
	"Jan 2, 2006",
	"2 Jan 2006",
	"15:04 02-Jan-2006",
	"010206 15:04:05",
}

var anisourceTimeRe = regexp.MustCompile(`(?i)Date:\s*(\d{4}-\d{2}-\d{2})\s+at\s+(\d{1,2}:\d{2})`)

func anisourceTime(raw string) time.Time {
	m := anisourceTimeRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) != 3 {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02 15:04 -0700", m[1]+" "+m[2]+" -0900")
	if err != nil {
		return time.Time{}
	}
	return t
}

var btetreeTorrentIDRe = regexp.MustCompile(`(?i)torrentId=(\d+)`)

var btetreeYearThresholds = [][2]int{
	{2004, 2204},
	{2005, 10368},
	{2006, 21306},
	{2007, 502958},
	{2008, 512029},
	{2009, 521346},
	{2010, 531218},
	{2011, 541379},
	{2012, 551321},
	{2013, 561096},
	{2014, 569309},
	{2015, 577205},
	{2016, 584158},
	{2017, 590609},
	{2018, 597003},
	{2019, 603585},
	{2020, 608148},
	{2021, 611609},
	{2022, 614780},
	{2023, 617776},
	{2024, 621061},
	{2025, 623102},
	{2026, 625197},
}

func btetreeTime(raw string) time.Time {
	m := btetreeTorrentIDRe.FindStringSubmatch(raw)
	if len(m) != 2 {
		return time.Time{}
	}
	torrentID, _ := strconv.Atoi(m[1])
	year := 2003
	for _, threshold := range btetreeYearThresholds {
		if torrentID < threshold[1] {
			break
		}
		year = threshold[0]
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<table><tbody><tr>" + raw + "</tr></tbody></table>"))
	if err != nil {
		return time.Time{}
	}
	mdhm := strings.TrimSpace(doc.Find("td:nth-child(5)").First().Text())
	if mdhm == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006/01/02 15:04", fmt.Sprintf("%04d/%s", year, mdhm))
	if err != nil {
		return time.Time{}
	}
	return t
}

var relativeTimeTokenRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(seconds?|secs?|minutes?|mins?|hours?|days?|weeks?)`)

// cnRelativeTimeRe 中文相对时间：N 天/小时/分钟/周 前。

var cnRelativeTimeRe = regexp.MustCompile(`(\d+)\s*(天|小时|小時|分钟|分鐘|周|週)前`)

// parseTimeBestEffort 尽力解析时间串：先试相对时间（N mins ago / N分钟前，相对 now），
// 再试常见绝对格式。无法识别返回零值（调用方据此保留原值）。

func parseTimeBestEffort(s string) time.Time {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Date/time submitted:")
	s = strings.TrimSpace(s)
	if t, ok := parseTodayYesterdayAt(s); ok {
		return t
	}
	if t, ok := parseRelativeTime(s); ok {
		return t
	}
	for _, layout := range absTimeFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseTodayYesterdayAt(s string) (time.Time, bool) {
	m := todayYesterdayAtRe.FindStringSubmatch(s)
	if len(m) != 5 {
		return time.Time{}, false
	}
	hh, _ := strconv.Atoi(m[2])
	mm, _ := strconv.Atoi(m[3])
	ss := 0
	if m[4] != "" {
		ss, _ = strconv.Atoi(m[4])
	}
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, ss, 0, now.Location())
	if strings.EqualFold(m[1], "yesterday") {
		day = day.AddDate(0, 0, -1)
	}
	return day, true
}

// parseRelativeTime 解析相对时间串，相对 time.Now()。匹配返回 (时刻, true)。

var dmyHMSRe = regexp.MustCompile(`(\d{2}):(\d{2}):(\d{2})\s*(\d{2})/(\d{2})/(\d{4})`)

var todayYesterdayAtRe = regexp.MustCompile(`(?i)^(today|yesterday)\s+(?:at\s+)?(\d{1,2}):(\d{2})(?::(\d{2}))?$`)

func parseDMYHMS(s string) time.Time {
	m := dmyHMSRe.FindStringSubmatch(s)
	if len(m) != 7 {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02 15:04:05", m[6]+"-"+m[5]+"-"+m[4]+" "+m[1]+":"+m[2]+":"+m[3])
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseRelativeTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	now := time.Now()
	if m := cnRelativeTimeRe.FindStringSubmatch(s); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "天":
			return now.AddDate(0, 0, -n), true
		case "小时", "小時":
			return now.Add(-time.Duration(n) * time.Hour), true
		case "分钟", "分鐘":
			return now.Add(-time.Duration(n) * time.Minute), true
		case "周", "週":
			return now.AddDate(0, 0, -7*n), true
		}
	}
	matches := relativeTimeTokenRe.FindAllStringSubmatch(s, -1)
	if len(matches) > 0 && (strings.Contains(strings.ToLower(s), "ago") || len(matches[0][0]) == len(s)) {
		var dur time.Duration
		for _, m := range matches {
			n, _ := strconv.ParseFloat(m[1], 64)
			unit := strings.ToLower(m[2])
			switch {
			case strings.HasPrefix(unit, "sec"):
				dur += time.Duration(n * float64(time.Second))
			case strings.HasPrefix(unit, "min"):
				dur += time.Duration(n * float64(time.Minute))
			case strings.HasPrefix(unit, "hour"):
				dur += time.Duration(n * float64(time.Hour))
			case strings.HasPrefix(unit, "day"):
				dur += time.Duration(n * float64(24*time.Hour))
			case strings.HasPrefix(unit, "week"):
				dur += time.Duration(n * float64(7*24*time.Hour))
			}
		}
		if dur > 0 {
			return now.Add(-dur), true
		}
	}
	return time.Time{}, false
}
