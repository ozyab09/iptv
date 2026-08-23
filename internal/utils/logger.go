package utils

import (
	"fmt"
	"io"
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

// Supported credential patterns:
// - Yandex Cloud: YCAJEu... (key), YCON... (secret)
// - AWS: AKIA... / ASIA... (access key)
var awsKeyPattern = regexp.MustCompile(`(YCAJEu[A-Za-z0-9_\-]+|AKIA[A-Za-z0-9_\-]{16}|ASIA[A-Za-z0-9_\-]{16})`)
var awsSecretPattern = regexp.MustCompile(`(YCON[A-Za-z0-9_\-]+|\*{10,}[A-Za-z0-9+/=]{10,})`)

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

// sanitizingWriter sanitizes URLs and credentials before writing, so stdlib
// loggers that bypass SanitizedWriter (e.g. net/http transport messages like
// "Unsolicited response received on idle HTTP channel") never leak them to
// the log.
type sanitizingWriter struct {
	w io.Writer
}

func (s sanitizingWriter) Write(p []byte) (int, error) {
	return s.w.Write([]byte(sanitizeLogMessage("%s", string(p))))
}

// init redirects the global stdlib log through the sanitizer. net/http logs
// transport-level messages (HLS probe leftovers, idle-channel warnings) via
// the global log on some toolchains and via Transport.ErrorLog (falling back
// to the global log) on others — redirecting the global logger masks URLs and
// credentials in both cases, keeping the output stream (stderr) unchanged.
func init() {
	log.SetOutput(sanitizingWriter{w: os.Stderr})
}
