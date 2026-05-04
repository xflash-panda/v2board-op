package util

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// SetupLogger configures the global logrus logger from a CLI log-mode string
// ("debug", "info", "error", and the other levels logrus.ParseLevel accepts).
// Debug mode enables full timestamps; other levels use the default formatter.
func SetupLogger(mode string) error {
	level, err := log.ParseLevel(mode)
	if err != nil {
		return fmt.Errorf("log mode %s not supported", mode)
	}
	log.SetLevel(level)
	if level == log.DebugLevel {
		log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
		log.SetReportCaller(false)
	} else {
		log.SetFormatter(&log.TextFormatter{})
	}
	return nil
}
