package logrus_sentry

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/getsentry/raven-go"
)

var (
	severityMap = map[logrus.Level]raven.Severity{
		logrus.DebugLevel: raven.DEBUG,
		logrus.InfoLevel:  raven.INFO,
		logrus.WarnLevel:  raven.WARNING,
		logrus.ErrorLevel: raven.ERROR,
		logrus.FatalLevel: raven.FATAL,
		logrus.PanicLevel: raven.FATAL,
	}
)

func getAndDel(d logrus.Fields, key string) (string, bool) {
	var (
		ok  bool
		v   interface{}
		val string
	)
	if v, ok = d[key]; !ok {
		return "", false
	}

	if val, ok = v.(string); !ok {
		return "", false
	}
	delete(d, key)
	return val, true
}

func getAndDelRequest(d logrus.Fields, key string) (*http.Request, bool) {
	var (
		ok  bool
		v   interface{}
		req *http.Request
	)
	if v, ok = d[key]; !ok {
		return nil, false
	}
	if req, ok = v.(*http.Request); !ok || req == nil {
		return nil, false
	}
	delete(d, key)
	return req, true
}

// SentryHook delivers logs to a sentry server.
type SentryHook struct {
	// Timeout sets the time to wait for a delivery error from the sentry server.
	// If this is set to zero the server will not wait for any response and will
	// consider the message correctly sent
	Timeout time.Duration

	client             *raven.Client
	levels             []logrus.Level
	appPackagePrefixes []string
}

// NewSentryHook creates a hook to be added to an instance of logger
// and initializes the raven client.
// This method sets the timeout to 100 milliseconds.
func NewSentryHook(DSN string, levels []logrus.Level, appPackagePrefixes []string) (*SentryHook, error) {
	client, err := raven.New(DSN)
	if err != nil {
		return nil, err
	}
	return &SentryHook{
		Timeout:            100 * time.Millisecond,
		client:             client,
		levels:             levels,
		appPackagePrefixes: appPackagePrefixes,
	}, nil
}

// NewWithTagsSentryHook creates a hook with tags to be added to an instance
// of logger and initializes the raven client. This method sets the timeout to
// 100 milliseconds.
func NewWithTagsSentryHook(DSN string, tags map[string]string, levels []logrus.Level, appPackagePrefixes []string) (*SentryHook, error) {
	client, err := raven.NewWithTags(DSN, tags)
	if err != nil {
		return nil, err
	}
	return &SentryHook{
		Timeout:            100 * time.Millisecond,
		client:             client,
		levels:             levels,
		appPackagePrefixes: appPackagePrefixes,
	}, nil
}

// NewWithClientSentryHook creates a hook using an initialized raven client.
// This method sets the timeout to 100 milliseconds.
func NewWithClientSentryHook(client *raven.Client, levels []logrus.Level, appPackagePrefixes []string) (*SentryHook, error) {
	return &SentryHook{
		Timeout:            100 * time.Millisecond,
		client:             client,
		levels:             levels,
		appPackagePrefixes: appPackagePrefixes,
	}, nil
}

// Called when an event should be sent to sentry
// Special fields that sentry uses to give more information to the server
// are extracted from entry.Data (if they are found)
// These fields are: logger, server_name and http_request
func (hook *SentryHook) Fire(entry *logrus.Entry) error {
	packet := raven.NewPacket(entry.Message)
	packet.Timestamp = raven.Timestamp(entry.Time)
	packet.Level = severityMap[entry.Level]
	packet.Interfaces = append(
		packet.Interfaces,
		raven.NewException(
			errors.New(entry.Message),
			raven.NewStacktrace(1, 10, hook.appPackagePrefixes),
		),
	)
	d := entry.Data

	if logger, ok := getAndDel(d, "logger"); ok {
		packet.Logger = logger
	}
	if serverName, ok := getAndDel(d, "server_name"); ok {
		packet.ServerName = serverName
	}
	if req, ok := getAndDelRequest(d, "http_request"); ok {
		packet.Interfaces = append(packet.Interfaces, raven.NewHttp(req))
	}
	packet.Extra = map[string]interface{}(d)

	_, errCh := hook.client.Capture(packet, nil)
	timeout := hook.Timeout
	if timeout != 0 {
		timeoutCh := time.After(timeout)
		select {
		case err := <-errCh:
			return err
		case <-timeoutCh:
			return fmt.Errorf("no response from sentry server in %s", timeout)
		}
	}
	return nil
}

// Levels returns the available logging levels.
func (hook *SentryHook) Levels() []logrus.Level {
	return hook.levels
}
