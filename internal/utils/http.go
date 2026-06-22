package utils

import (
	"crypto/tls"
	"net/http"
	"time"
)

var HTTPClient = &http.Client{
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
	Timeout: 30 * time.Minute,
}
