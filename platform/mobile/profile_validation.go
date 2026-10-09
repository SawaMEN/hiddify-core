package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/hiddify/hiddify-core/v2/config"
	"github.com/sagernet/sing-box/experimental/libbox"
)

type profileValidationSession struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var profileValidations = struct {
	sync.Mutex
	sessions map[string]*profileValidationSession
}{sessions: make(map[string]*profileValidationSession)}

// BeginProfileValidation registers cancellation before entering the blocking native parser.
// It does not start a service, change global settings or open a local RPC listener.
func BeginProfileValidation(id string) error {
	if len(id) == 0 || len(id) > 64 {
		return errors.New("invalid profile validation ID")
	}
	profileValidations.Lock()
	defer profileValidations.Unlock()
	if _, exists := profileValidations.sessions[id]; exists {
		return errors.New("profile validation already registered")
	}
	if len(profileValidations.sessions) >= 4 {
		return errors.New("too many profile validations")
	}
	ctx, cancel := context.WithTimeout(libbox.BaseContext(nil), 45*time.Second)
	profileValidations.sessions[id] = &profileValidationSession{ctx: ctx, cancel: cancel}
	return nil
}

// CancelProfileValidation only interrupts the import owning this ID, never the running VPN.
func CancelProfileValidation(id string) {
	profileValidations.Lock()
	session := profileValidations.sessions[id]
	profileValidations.Unlock()
	if session != nil {
		session.cancel()
	}
}

// FinishProfileValidation is required even when cancellation wins before ValidateProfile starts.
func FinishProfileValidation(id string) {
	profileValidations.Lock()
	session := profileValidations.sessions[id]
	delete(profileValidations.sessions, id)
	profileValidations.Unlock()
	if session != nil {
		session.cancel()
	}
}

// ValidateProfile uses the same parser and sing-box validation as hcore.Parse, with private
// options instead of settingsSnapshot(). It neither persists source nor applies overrides.
func ValidateProfile(id, content, settingsJSON string) (err error) {
	defer config.DeferPanicToError("profile validation", func(recovered error) { err = recovered })
	profileValidations.Lock()
	session := profileValidations.sessions[id]
	profileValidations.Unlock()
	if session == nil {
		return errors.New("profile validation not registered")
	}
	if err := session.ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" || len(content) > config.MaxConfigBytes {
		return errors.New("profile configuration is empty or exceeds 8 MiB")
	}
	if len(settingsJSON) > config.MaxConfigBytes {
		return errors.New("profile settings exceed 8 MiB")
	}
	options := config.DefaultHiddifyOptions()
	if settingsJSON != "" {
		if err := json.Unmarshal([]byte(settingsJSON), options); err != nil {
			return err
		}
	}
	_, err = config.ParseConfig(session.ctx, &config.ReadOptions{Content: content}, false, options, true)
	if cancelled := session.ctx.Err(); cancelled != nil {
		return cancelled
	}
	return err
}
