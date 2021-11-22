package ociutil

import "time"

const sdkFormat = "2006-01-02T15:04:05.999Z07:00"

func FormatSDKTime(dateTime time.Time) string {
	return dateTime.Format(sdkFormat)
}

func ParseSDKTime(val string) (time.Time, error) {
	return time.Parse(sdkFormat, val)
}
