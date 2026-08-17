package handlers

import (
	"net/http"
	"time"
)

// GetTimezoneLocation extracts the IANA timezone from the X-Timezone header.
// If missing, invalid, or empty, it returns time.UTC as a fallback.
func GetTimezoneLocation(r *http.Request) *time.Location {
	ianaName := r.Header.Get("X-Timezone")
	if ianaName == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(ianaName)
	if err != nil {
		return time.UTC
	}
	return loc
}
