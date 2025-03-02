package warewulfd

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	warewulfconf "github.com/warewulf/warewulf/internal/pkg/config"
	"github.com/warewulf/warewulf/internal/pkg/util"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd/api"
	"github.com/warewulf/warewulf/internal/pkg/warewulfd/nodedb"
	"github.com/warewulf/warewulf/internal/pkg/wwlog"
)

// TODO: https://github.com/danderson/netboot/blob/master/pixiecore/dhcp.go
// TODO: https://github.com/pin/tftp
/*
wrapper type for the server mux as shim requests http://efiboot//grub.efi
which is filtered out by http to `301 Moved Permanently` what
shim.efi can't handle. So filter out `//` before they hit go/http.
Makes go/http more to behave like apache
*/
type slashFix struct {
	mux http.Handler
}

/*
Filter out the '//'
*/
func (h *slashFix) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.Replace(r.URL.Path, "//", "/", -1)
	h.mux.ServeHTTP(w, r)
}

func defaultHandler() *slashFix {
	var wwHandler http.ServeMux
	wwHandler.HandleFunc("/provision/", ProvisionSend)
	wwHandler.HandleFunc("/ipxe/", ProvisionSend)
	wwHandler.HandleFunc("/efiboot/", ProvisionSend)
	wwHandler.HandleFunc("/kernel/", ProvisionSend)
	wwHandler.HandleFunc("/container/", ProvisionSend)
	wwHandler.HandleFunc("/overlay-system/", ProvisionSend)
	wwHandler.HandleFunc("/overlay-runtime/", ProvisionSend)
	wwHandler.HandleFunc("/overlay-file/", OverlaySend)
	wwHandler.HandleFunc("/status", nodedb.StatusSend)
	return &slashFix{&wwHandler}
}

func RunServer() error {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	go func() {
		for range c {
			wwlog.Info("Received SIGHUP, reloading...")
			nodedb.Reload()
		}
	}()

	nodedb.Reload()

	conf := warewulfconf.Get()
	daemonPort := conf.Warewulf.Port

	auth := warewulfconf.NewAuthentication()
	if util.IsFile(conf.Paths.AuthenticationConf()) {
		if err := auth.Read(conf.Paths.AuthenticationConf()); err != nil {
			wwlog.Warn("%w\n", err)
		}
	}

	apiHandler := api.Handler(auth)
	defaultHandler := defaultHandler()
	dispatchHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") && conf.API != nil && conf.API.Enabled() {
			apiHandler.ServeHTTP(w, r)
		} else {
			defaultHandler.ServeHTTP(w, r)
		}
	})
	if err := http.ListenAndServe(":"+strconv.Itoa(daemonPort), dispatchHandler); err != nil {
		return fmt.Errorf("could not start listening service: %w", err)
	}

	return nil
}
