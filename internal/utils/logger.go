package utils

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
)

type SanitizedWriter struct {
	inner *log.Logger
}

func NewSanitizedLogger() *SanitizedWriter {
	return NewSanitizedLoggerWithPrefix("")
}

func NewSanitizedLoggerWithPrefix(prefix string) *SanitizedWriter {
	logger := log.New(os.Stdout, prefix, log.Ldate|log.Ltime|log.Lmsgprefix)
	return &SanitizedWriter{inner: logger}
}

func (s *SanitizedWriter) Info(format string, args ...interface{}) {
	s.inner.Println("INFO: " + sanitizeLogMessage(format, args...))
}

func (s *SanitizedWriter) Debug(format string, args ...interface{}) {
	s.inner.Println("DEBUG: " + sanitizeLogMessage(format, args...))
}

func (s *SanitizedWriter) Warning(format string, args ...interface{}) {
	s.inner.Println("WARN: " + sanitizeLogMessage(format, args...))
}

func (s *SanitizedWriter) Error(format string, args ...interface{}) {
	s.inner.Println("ERROR: " + sanitizeLogMessage(format, args...))
}

var urlPattern = regexp.MustCompile(`https?://[^\s'"<>]+`)
var awsKeyPattern = regexp.MustCompile(`(YCAJEu[A-Za-z0-9_\-]+)`)
var awsSecretPattern = regexp.MustCompile(`(YCON[A-Za-z0-9_\-]+)`)

func sanitizeLogMessage(format string, args ...interface{}) string {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}

	msg = urlPattern.ReplaceAllStringFunc(msg, func(rawURL string) string {
		return maskURL(rawURL)
	})

	msg = awsKeyPattern.ReplaceAllStringFunc(msg, func(match string) string {
		if len(match) > 8 {
			return match[:4] + "****" + match[len(match)-4:]
		}
		return "****"
	})

	msg = awsSecretPattern.ReplaceAllStringFunc(msg, func(match string) string {
		if len(match) > 8 {
			return match[:4] + "****" + match[len(match)-4:]
		}
		return "****"
	})

	return msg
}

func maskURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "https://****/****"
	}
	return parsed.Scheme + "://****/****"
}
