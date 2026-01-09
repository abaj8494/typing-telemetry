package stats

import "time"

type Summary struct {
	TodayKeystrokes int64
	TodayWords      int64
	WeekKeystrokes  int64
	WeekWords       int64
	AvgPerDay       float64
	PeakHour        int
	PeakHourCount   int64
}

type DayData struct {
	Date       time.Time
	Keystrokes int64
	Words      int64
}

func CalculateWeeklyAverage(days []DayData) float64 {
	if len(days) == 0 {
		return 0
	}
	var total int64
	for _, d := range days {
		total += d.Keystrokes
	}
	return float64(total) / float64(len(days))
}

func FindPeakHour(hourlyData []int64) (hour int, count int64) {
	for h, c := range hourlyData {
		if c > count {
			hour = h
			count = c
		}
	}
	return
}

func FormatKeystrokeCount(count int64) string {
	if count >= 1000000 {
		return formatFloat(float64(count)/1000000) + "M"
	}
	if count >= 1000 {
		return formatFloat(float64(count)/1000) + "K"
	}
	return formatInt(count)
}

func formatFloat(f float64) string {
	intPart := int64(f)
	if f == float64(intPart) {
		return formatInt(intPart)
	}
	// Get first decimal digit
	decimalPart := int((f - float64(intPart)) * 10)
	return formatInt(intPart) + "." + string(byte('0'+decimalPart))
}

func formatInt(i int64) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
