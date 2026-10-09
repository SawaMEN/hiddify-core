// Android root companion: no Android VpnService or Java platform callback.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/hiddify/hiddify-core/v2/hcore"
	"github.com/sagernet/sing-box/experimental/libbox"
)

type envelope struct {
	Base               string          `json:"base"`
	Port               int             `json:"port"`
	Secret             string          `json:"secret"`
	Options            json.RawMessage `json:"options"`
	Config             string          `json:"config"`
	AutoStart          bool            `json:"autoStart"`
	Name               string          `json:"name"`
	Debug              bool            `json:"debug"`
	DisableMemoryLimit bool            `json:"disableMemoryLimit"`
}

func run() error {
	if os.Getuid() != 0 {
		return fmt.Errorf("root permission required")
	}
	if len(os.Args) != 2 {
		return fmt.Errorf("expected private configuration file")
	}
	content, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var args envelope
	if err = json.Unmarshal(content, &args); err != nil {
		return err
	}
	if args.Port < 1 || args.Port > 65535 || len(args.Secret) < 32 || !filepath.IsAbs(args.Base) {
		return fmt.Errorf("invalid root configuration")
	}
	if err = hcore.Setup(&hcore.SetupRequest{BasePath: args.Base, WorkingDir: args.Base, TempDir: filepath.Join(args.Base, "tmp"), Mode: hcore.SetupMode_GRPC_BACKGROUND_INSECURE, Listen: fmt.Sprintf("127.0.0.1:%d", args.Port), Secret: args.Secret, Debug: args.Debug}, nil); err != nil {
		return err
	}
	defer hcore.Close(hcore.SetupMode_GRPC_BACKGROUND_INSECURE)
	if err = config.SetNetworkPrivacyPolicyJSON(string(args.Options)); err != nil {
		return err
	}
	if _, err = hcore.ChangeHiddifySettings(&hcore.ChangeHiddifySettingsRequest{HiddifySettingsJson: string(args.Options)}, false); err != nil {
		return err
	}
	if args.AutoStart {
		if _, err = hcore.StartService(libbox.BaseContext(nil), &hcore.StartRequest{ConfigPath: args.Config, ConfigName: args.Name, DisableMemoryLimit: args.DisableMemoryLimit}); err != nil {
			return err
		}
	}
	// The app owns stdin. EOF (including an app crash) triggers normal sing-tun route cleanup.
	lostParent := make(chan struct{})
	go func() { io.Copy(io.Discard, os.Stdin); close(lostParent) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	fmt.Println("HC_ROOT_READY")
	select {
	case <-lostParent:
	case <-signals:
	}
	// Bound graceful shutdown even if a native transport never returns.
	time.AfterFunc(10*time.Second, func() { os.Exit(1) })
	_, err = hcore.Stop()
	return err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
