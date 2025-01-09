package warewulfd

import (
	"fmt"
	"log/syslog"
	"os"
	"strconv"
	"time"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

const (
	WAREWULFD_LOGFILE = "/var/log/warewulfd.log"
)

var loginit bool

func DaemonFormatter(logLevel int, rec *wwlog.LogRecord) string {
	return "[" + rec.Time.Format(time.UnixDate) + "] " + wwlog.DefaultFormatter(logLevel, rec)
}

func DaemonInitLogging() error {
	if loginit {
		return nil
	}

	wwlog.SetLogFormatter(DaemonFormatter)

	level_str, ok := os.LookupEnv("WAREWULFD_LOGLEVEL")
	if ok {
		level, err := strconv.Atoi(level_str)
		if err == nil {
			wwlog.SetLogLevel(level)
		}
	} else {
		wwlog.SetLogLevel(wwlog.INFO)
	}

	conf := warewulfconf.Get()

	if conf.Warewulf.Syslog() {

		wwlog.Debug("Changing log output to syslog")

		logwriter, err := syslog.New(syslog.LOG_NOTICE, "warewulfd")
		if err != nil {
			return fmt.Errorf("Could not create syslog writer: %w", err)
		}

		wwlog.SetLogFormatter(wwlog.DefaultFormatter)
		wwlog.SetLogWriter(logwriter)

	}

	loginit = true

	return nil
}
